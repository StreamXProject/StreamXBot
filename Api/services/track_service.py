import re
import random
import datetime
import hashlib
import time
from typing import Any, Optional
import inspect
import asyncio

from pymongo import UpdateOne

from Api.deps.db import get_audio_tracks_collection
from Api.schemas.browse import BrowseItem, BrowseResponse
from stream.database.MongoDb import db_handler

def _as_str_id(value: Any) -> str:
    if value is None:
        return ""
    return str(value)

def _clean_url(value: Any) -> str:
    if not isinstance(value, str):
        return ""
    s = value.strip()
    if len(s) >= 2 and s[0] == "`" and s[-1] == "`":
        s = s[1:-1].strip()
    return s


def _normalize_mime_type(mime: Any) -> str | None:
    if not mime:
        return None
    raw = str(mime).split(";")[0].strip().lower()
    if raw in {"audio/flac", "audio/x-flac"} or raw.endswith("/x-flac"):
        return "audio/flac"
    if raw in {"audio/wav", "audio/x-wav", "audio/wave"}:
        return "audio/wav"
    if raw in {"audio/mp3", "audio/mpeg"}:
        return "audio/mpeg"
    if raw in {"audio/m4a", "audio/x-m4a", "audio/mp4"}:
        return "audio/mp4"
    if raw in {"audio/ogg", "application/ogg"}:
        return "audio/ogg"
    if raw in {"audio/aac"}:
        return "audio/aac"
    return str(mime).strip()


def _normalize_spotify(doc: dict) -> None:
    spotify = doc.get("spotify")
    if isinstance(spotify, dict):
        spotify["url"] = _clean_url(spotify.get("url") or spotify.get("spotify_url"))
        spotify["cover_url"] = _clean_url(spotify.get("cover_url"))
        spotify.pop("spotify_url", None)
        spotify.pop("links", None)

    telegram = doc.get("telegram")
    if isinstance(telegram, dict) and "mime_type" in telegram and telegram["mime_type"]:
        telegram["mime_type"] = _normalize_mime_type(telegram["mime_type"])


def _browse_item_from_doc(doc: dict, liked_set: set[str] | None = None) -> BrowseItem:
    audio = doc.get("audio") or {}
    spotify = doc.get("spotify") or {}
    if isinstance(spotify, dict):
        spotify["url"] = _clean_url(spotify.get("url") or spotify.get("spotify_url"))
        spotify["cover_url"] = _clean_url(spotify.get("cover_url"))

    t = audio.get("type")
    if isinstance(t, str) and t:
        t = t.upper()

    tid = _as_str_id(doc.get("_id"))
    is_liked = bool(liked_set and tid in liked_set)
    titles = doc.get("titles") or (audio.get("titles") if isinstance(audio, dict) else None)

    return BrowseItem(
        _id=tid,
        source_chat_id=doc.get("source_chat_id"),
        source_message_id=doc.get("source_message_id"),
        topic_id=doc.get("topic_id"),
        topic_name=doc.get("topic_name"),
        title=audio.get("title"),
        artist=audio.get("artist"),
        album=audio.get("album"),
        album_id=audio.get("album_id"),
        duration_sec=audio.get("duration_sec"),
        type=t,
        sampling_rate_hz=audio.get("sampling_rate_hz"),
        spotify_url=_clean_url(spotify.get("url") or spotify.get("spotify_url")),
        cover_url=_clean_url(spotify.get("cover_url")),
        titles=titles,
        created_at=doc.get("created_at") or doc.get("updated_at"),
        updated_at=doc.get("updated_at"),
        liked=is_liked,
    )


async def get_user_liked_track_ids(user_id: int | None, track_ids: list[str]) -> set[str]:
    if not user_id or user_id <= 0 or not track_ids:
        return set()
    clean_ids = [str(tid).strip() for tid in track_ids if str(tid).strip()]
    if not clean_ids:
        return set()
    col = db_handler.get_collection("user_favourites").collection
    cursor = col.find(
        {"user_id": int(user_id), "track_id": {"$in": clean_ids}},
        {"track_id": 1}
    )
    liked_set: set[str] = set()
    async for doc in cursor:
        tid = doc.get("track_id")
        if isinstance(tid, str) and tid:
            liked_set.add(tid)
    return liked_set


async def attach_liked_to_dicts(docs: list[dict], user_id: int | None) -> list[dict]:
    if not docs:
        return docs
    tids = [str(d.get("_id") or d.get("id") or "").strip() for d in docs]
    liked_set = await get_user_liked_track_ids(user_id, tids) if (user_id and user_id > 0) else set()
    for d in docs:
        tid = str(d.get("_id") or d.get("id") or "").strip()
        d["liked"] = tid in liked_set
    return docs


def _search_pattern(q: str) -> str:
    s = (q or "").strip()
    if not s:
        return ""
    tokens = [t for t in s.split() if t.strip()]
    if not tokens:
        return ""
    return ".*".join(re.escape(t) for t in tokens[:8])


async def browse_tracks(
    channel_id: Optional[int] = None,
    page: int = 1,
    per_page: int = 20,
    user_id: Optional[int] = None,
    topic_name: Optional[str] = None,
    topic_id: Optional[int] = None,
) -> BrowseResponse:
    per_page = int(per_page)
    if per_page <= 0:
        per_page = 20
    page = int(page)
    if page < 1:
        page = 1
    skip = (page - 1) * per_page

    col = get_audio_tracks_collection()
    query: dict[str, Any] = {"deleted": {"$ne": True}}
    if channel_id is not None:
        query["source_chat_id"] = int(channel_id)
    if topic_name:
        t_clean = topic_name.strip()
        if t_clean:
            query["topic_name"] = t_clean
    if topic_id is not None:
        query["topic_id"] = int(topic_id)

    sort = [("source_message_id", -1)] if channel_id is not None else [("created_at", -1), ("source_message_id", -1), ("_id", -1)]
    projection = {
        "_id": 1,
        "source_chat_id": 1,
        "source_message_id": 1,
        "audio": 1,
        "spotify": 1,
        "created_at": 1,
        "updated_at": 1,
        "topic_id": 1,
        "topic_name": 1,
    }

    total, docs = await asyncio.gather(
        col.count_documents(query),
        col.find(query, projection).sort(sort).skip(skip).limit(per_page).to_list(length=per_page),
    )

    track_ids = [_as_str_id(d.get("_id")) for d in docs]
    liked_set = await get_user_liked_track_ids(user_id, track_ids)

    items: list[BrowseItem] = []
    cover_url: str | None = None
    for doc in docs:
        item = _browse_item_from_doc(doc, liked_set)
        items.append(item)
        if not cover_url and item.cover_url:
            cover_url = item.cover_url

    return BrowseResponse(page=page, per_page=per_page, total=total, items=items, cover_url=cover_url)


_TOPICS_CACHE: dict[str, tuple[float, dict]] = {}
_TOPICS_CACHE_TTL = 60.0


async def get_unique_topics(
    *,
    channel_id: Optional[int] = None,
    limit: int = 100,
    refresh: bool = False,
) -> dict:
    limit = max(1, min(int(limit), 500))
    cache_key = f"{channel_id}:{limit}"

    now = time.time()
    if not refresh and cache_key in _TOPICS_CACHE:
        ts, cached = _TOPICS_CACHE[cache_key]
        if now - ts < _TOPICS_CACHE_TTL:
            return cached

    col = get_audio_tracks_collection()
    match_filter: dict[str, Any] = {
        "deleted": {"$ne": True},
        "topic_name": {"$exists": True, "$type": "string", "$nin": ["", "null", "None"]},
    }
    if channel_id is not None:
        match_filter["source_chat_id"] = int(channel_id)

    pipeline = [
        {"$match": match_filter},
        {
            "$sort": {
                "updated_at": -1,
                "source_message_id": -1,
            }
        },
        {
            "$group": {
                "_id": "$topic_name",
                "topic_id": {"$first": "$topic_id"},
                "source_chat_id": {"$first": "$source_chat_id"},
                "cover_url": {"$first": "$spotify.cover_url"},
                "big_cover_url": {"$first": "$spotify.big_cover_url"},
                "raw_thumbnails": {"$push": "$spotify.cover_url"},
                "count": {"$sum": 1},
                "latest_updated_at": {"$first": "$updated_at"},
            }
        },
        {
            "$project": {
                "topic_id": 1,
                "source_chat_id": 1,
                "cover_url": 1,
                "big_cover_url": 1,
                "thumbnails": {"$slice": ["$raw_thumbnails", 8]},
                "count": 1,
                "latest_updated_at": 1,
            }
        },
        {"$sort": {"count": -1, "_id": 1}},
        {"$limit": limit},
    ]

    cursor = await col.aggregate(pipeline, allowDiskUse=True)
    docs = await cursor.to_list(length=limit)

    items: list[dict] = []
    topic_names: list[str] = []

    for d in docs:
        name = str(d.get("_id") or "").strip()
        if not name:
            continue
        topic_names.append(name)

        cover = _clean_url(d.get("big_cover_url") or d.get("cover_url"))
        raw_thumbs = d.get("thumbnails") or []
        thumbs = [
            _clean_url(t)
            for t in raw_thumbs
            if isinstance(t, str) and _clean_url(t)
        ]
        unique_thumbs = list(dict.fromkeys(thumbs))[:4]
        if not cover and unique_thumbs:
            cover = unique_thumbs[0]

        items.append({
            "name": name,
            "topic_name": name,
            "topic_id": d.get("topic_id"),
            "count": int(d.get("count") or 0),
            "cover_url": cover or None,
            "thumbnail_url": cover or None,
            "normal_thumbnail": cover or None,
            "thumbnails": unique_thumbs,
            "source_chat_id": d.get("source_chat_id"),
            "endpoint": f"/topics/{name}/tracks",
        })

    result = {
        "ok": True,
        "total": len(items),
        "items": items,
        "topics": topic_names,
    }
    _TOPICS_CACHE[cache_key] = (now, result)
    return result


async def browse_topic_tracks(
    topic_name: str,
    *,
    channel_id: Optional[int] = None,
    page: int = 1,
    per_page: int = 20,
    user_id: Optional[int] = None,
) -> BrowseResponse:
    raw_topic = (topic_name or "").strip()
    if not raw_topic:
        return BrowseResponse(page=page, per_page=per_page, total=0, items=[], cover_url=None)

    per_page = int(per_page)
    if per_page <= 0:
        per_page = 20
    if per_page > 100:
        per_page = 100
    page = int(page)
    if page < 1:
        page = 1
    skip = (page - 1) * per_page

    col = get_audio_tracks_collection()

    # Fast path: B-tree indexed exact match query
    query: dict[str, Any] = {
        "deleted": {"$ne": True},
        "topic_name": raw_topic,
    }
    if channel_id is not None:
        query["source_chat_id"] = int(channel_id)

    projection = {
        "_id": 1,
        "source_chat_id": 1,
        "source_message_id": 1,
        "audio": 1,
        "spotify": 1,
        "created_at": 1,
        "updated_at": 1,
        "topic_id": 1,
        "topic_name": 1,
    }
    sort = [("created_at", -1), ("source_message_id", -1), ("_id", -1)]

    total, docs = await asyncio.gather(
        col.count_documents(query),
        col.find(query, projection).sort(sort).skip(skip).limit(per_page).to_list(length=per_page),
    )

    # Fallback to case-insensitive match if exact match returned 0
    if total == 0:
        regex_query: dict[str, Any] = {
            "deleted": {"$ne": True},
            "topic_name": {"$regex": f"^{re.escape(raw_topic)}$", "$options": "i"},
        }
        if channel_id is not None:
            regex_query["source_chat_id"] = int(channel_id)

        total, docs = await asyncio.gather(
            col.count_documents(regex_query),
            col.find(regex_query, projection).sort(sort).skip(skip).limit(per_page).to_list(length=per_page),
        )

    track_ids = [_as_str_id(d.get("_id")) for d in docs]
    liked_set = await get_user_liked_track_ids(user_id, track_ids)

    items: list[BrowseItem] = []
    cover_url: str | None = None
    for doc in docs:
        item = _browse_item_from_doc(doc, liked_set)
        items.append(item)
        if not cover_url and item.cover_url:
            cover_url = item.cover_url

    return BrowseResponse(page=page, per_page=per_page, total=total, items=items, cover_url=cover_url)


async def search_tracks(q: str, *, channel_id: Optional[int], page: int, per_page: int, user_id: Optional[int] = None) -> BrowseResponse:
    per_page = int(per_page)
    if per_page <= 0:
        per_page = 20
    if per_page > 50:
        per_page = 50
    page = int(page)
    if page < 1:
        page = 1
    skip = (page - 1) * per_page

    pattern = _search_pattern(q)
    if not pattern:
        return BrowseResponse(page=page, per_page=per_page, total=0, items=[])

    col = get_audio_tracks_collection()
    query: dict[str, Any] = {
        "deleted": {"$ne": True},
        "$or": [
            {"audio.title": {"$regex": pattern, "$options": "i"}},
            {"audio.artist": {"$regex": pattern, "$options": "i"}},
            {"audio.performer": {"$regex": pattern, "$options": "i"}},
            {"audio.album": {"$regex": pattern, "$options": "i"}},
            {"titles.romanized": {"$regex": pattern, "$options": "i"}},
            {"titles.original": {"$regex": pattern, "$options": "i"}},
            {"titles.translations.en": {"$regex": pattern, "$options": "i"}},
        ],
    }
    if channel_id is not None:
        query["source_chat_id"] = int(channel_id)

    projection = {
        "_id": 1,
        "source_chat_id": 1,
        "source_message_id": 1,
        "audio": 1,
        "spotify": 1,
        "titles": 1,
        "created_at": 1,
        "updated_at": 1,
    }

    total = await col.count_documents(query)
    cursor = col.find(query, projection).sort([("created_at", -1), ("source_message_id", -1)]).skip(skip).limit(per_page)

    docs = await cursor.to_list(length=per_page)
    track_ids = [_as_str_id(d.get("_id")) for d in docs]
    liked_set = await get_user_liked_track_ids(user_id, track_ids)

    items: list[BrowseItem] = []
    for doc in docs:
        items.append(_browse_item_from_doc(doc, liked_set))

    return BrowseResponse(page=page, per_page=per_page, total=total, items=items)

def _smart_interleave_artists(raw_docs: list[dict], rng: random.Random) -> list[dict]:
    if len(raw_docs) <= 2:
        return raw_docs

    groups: dict[str, list[dict]] = {}
    for doc in raw_docs:
        audio = doc.get("audio") if isinstance(doc.get("audio"), dict) else {}
        spotify = doc.get("spotify") if isinstance(doc.get("spotify"), dict) else {}
        artist = str(audio.get("artist") or spotify.get("artist") or "unknown").strip().lower()
        if artist not in groups:
            groups[artist] = []
        groups[artist].append(doc)

    for artist_list in groups.values():
        rng.shuffle(artist_list)

    sorted_artists = sorted(groups.keys(), key=lambda k: len(groups[k]), reverse=True)

    out: list[dict] = []
    while sorted_artists:
        for artist in list(sorted_artists):
            if groups[artist]:
                out.append(groups[artist].pop(0))
            if not groups[artist]:
                sorted_artists.remove(artist)

    return out


async def random_tracks(
    *,
    limit: int = 100,
    seed: int | None = None,
    channel_id: Optional[int] = None,
    user_id: Optional[int] = None,
    liked: Optional[bool] = None,
    genre: Optional[str] = None,
    artist: Optional[str] = None,
    source: Optional[str] = None,
    lossless: Optional[bool] = None,
) -> BrowseResponse:
    limit = int(limit)
    if limit <= 0:
        limit = 50
    if limit > 200:
        limit = 200

    query: dict[str, Any] = {"deleted": {"$ne": True}}
    if channel_id is not None:
        query["source_chat_id"] = int(channel_id)

    if liked and user_id is not None:
        fav_col = db_handler.get_collection("user_favourites").collection
        fav_cursor = fav_col.find({"user_id": int(user_id)}, {"_id": 0, "track_id": 1})
        fav_ids = [doc.get("track_id") async for doc in fav_cursor if doc.get("track_id")]
        if fav_ids:
            query["_id"] = {"$in": fav_ids}
        else:
            return BrowseResponse(page=1, per_page=limit, total=0, items=[])

    if genre:
        query["$or"] = [
            {"audio.genre": {"$regex": re.escape(genre), "$options": "i"}},
            {"spotify.genre": {"$regex": re.escape(genre), "$options": "i"}},
        ]

    if artist:
        query["$or"] = [
            {"audio.artist": {"$regex": re.escape(artist), "$options": "i"}},
            {"spotify.artist": {"$regex": re.escape(artist), "$options": "i"}},
        ]

    if source:
        if source.lower() == "telegram":
            query["source_chat_id"] = {"$exists": True, "$ne": None}
        elif source.lower() == "local":
            query["is_local"] = True

    if lossless:
        query["audio.type"] = {"$in": ["FLAC", "ALAC", "WAV"]}

    col = get_audio_tracks_collection()
    total = await col.count_documents(query)
    if total <= 0:
        return BrowseResponse(page=1, per_page=limit, total=0, items=[])

    rng: random.Random
    if seed is None:
        rng = random.SystemRandom()
    else:
        rng = random.Random(int(seed))

    projection = {
        "_id": 1,
        "source_chat_id": 1,
        "source_message_id": 1,
        "audio": 1,
        "spotify": 1,
        "created_at": 1,
        "updated_at": 1,
    }

    sample_size = min(limit, total)
    pipeline = [
        {"$match": query},
        {"$sample": {"size": sample_size}},
        {"$project": projection},
    ]

    cursor = col.aggregate(pipeline)
    while inspect.iscoroutine(cursor) or inspect.isawaitable(cursor):
        cursor = await cursor
    if hasattr(cursor, "to_list"):
        to_list_fn = getattr(cursor, "to_list")
        res = to_list_fn(length=sample_size)
        if inspect.isawaitable(res) or inspect.iscoroutine(res):
            raw_docs = await res
        else:
            raw_docs = list(res)
    else:
        raw_docs = [doc async for doc in cursor]

    raw_docs = _smart_interleave_artists(raw_docs, rng)

    track_ids = [_as_str_id(d.get("_id")) for d in raw_docs]
    liked_set = await get_user_liked_track_ids(user_id, track_ids)
    items = [_browse_item_from_doc(d, liked_set) for d in raw_docs]

    return BrowseResponse(page=1, per_page=limit, total=total, items=items)

async def get_browse_items_by_ids(track_ids: list[str], user_id: Optional[int] = None) -> list[BrowseItem]:
    ids = [str(x) for x in (track_ids or []) if str(x)]
    if not ids:
        return []

    col = get_audio_tracks_collection()
    projection = {
        "_id": 1,
        "source_chat_id": 1,
        "source_message_id": 1,
        "audio": 1,
        "spotify": 1,
        "created_at": 1,
        "updated_at": 1,
    }
    cursor = col.find({"_id": {"$in": ids}, "deleted": {"$ne": True}}, projection)
    docs: list[dict] = []
    async for doc in cursor:
        docs.append(doc)

    liked_set = await get_user_liked_track_ids(user_id, ids)
    by_id = {_as_str_id(d.get("_id")): d for d in docs if d.get("_id")}
    items: list[BrowseItem] = []
    for tid in ids:
        d = by_id.get(tid)
        if not d:
            continue
        items.append(_browse_item_from_doc(d, liked_set))
    return items

def _track_thumbnail_url(track: dict) -> str:
    spotify = track.get("spotify") if isinstance(track.get("spotify"), dict) else {}
    telegram = track.get("telegram") if isinstance(track.get("telegram"), dict) else {}
    audio = track.get("audio") if isinstance(track.get("audio"), dict) else {}

    candidates = [
        spotify.get("cover_url"),
        spotify.get("cover"),
        spotify.get("thumbnail"),
        telegram.get("thumb_url"),
        telegram.get("thumbnail_url"),
        telegram.get("thumb"),
        telegram.get("thumbnail"),
        audio.get("cover_url"),
        audio.get("thumbnail"),
    ]
    for c in candidates:
        if isinstance(c, str) and c.strip():
            return c.strip()
    return ""


async def get_daily_playlist_thumbnail_info(
    *,
    key: str,
    date: str,
    channel_id: int | None,
    limit: int = 4,
) -> dict[str, object]:
    k = _canon_daily_playlist_key(key)
    d = (date or "").strip()

    cover_url = None
    thumbnails: list[str] = []
    try:
        track_ids = await generate_daily_playlist(key=k, date=d, channel_id=channel_id, limit=limit)
        if track_ids:
            tracks = await get_tracks_by_ids(track_ids[:limit])
            for t in tracks:
                if isinstance(t, dict):
                    url = _track_thumbnail_url(t)
                    if url and url not in thumbnails:
                        thumbnails.append(url)
            if thumbnails:
                cover_url = thumbnails[0]
    except Exception:
        pass

    return {
        "key": k,
        "date": d,
        "channel_id": int(channel_id) if channel_id is not None else None,
        "cover_url": cover_url,
        "normal_thumbnail": cover_url,
        "thumbnails": thumbnails,
    }


async def get_user_top_played_thumbnail_info(*, user_id: int, limit: int = 4) -> dict[str, object]:
    uid = int(user_id)
    cache_col = db_handler.get_collection("user_top_played_cache").collection
    cached = await cache_col.find_one({"_id": str(uid)}, {"_id": 0, "track_ids": 1, "cover_url": 1, "normal_thumbnail": 1})

    cover_url = cached.get("cover_url") if isinstance(cached, dict) else None
    normal_url = cached.get("normal_thumbnail") if isinstance(cached, dict) else None

    return {
        "user_id": uid,
        "cover_url": cover_url,
        "normal_thumbnail": normal_url,
    }

def _daily_playlist_seed(*, key: str, date: str, channel_id: int | None) -> int:
    scope = str(int(channel_id)) if channel_id is not None else ""
    seed_src = f"{key}|{date}|{scope}".encode("utf-8", errors="ignore")
    return int(hashlib.sha1(seed_src).hexdigest()[:8], 16)

def _canon_daily_playlist_key(key: str) -> str:
    k = (key or "").strip().lower()
    if k in {"random", "mix", "daily", "daily-playlist"}:
        return "random"
    if k in {"top", "top-played", "top-playlist"}:
        return "top-played"
    if k in {"trending", "trending-today"}:
        return "trending"
    if k in {"late-night", "late-night-mix", "night"}:
        return "late-night"
    if k in {"rising", "rising-tracks"}:
        return "rising"
    if k in {"surprise", "surprise-me"}:
        return "surprise"
    return k


async def generate_daily_playlist(*, key: str, date: str, channel_id: int | None, limit: int) -> list[str]:
    key = _canon_daily_playlist_key(key)
    if not key:
        return []

    limit = int(limit)
    if limit <= 0:
        limit = 75
    if limit > 75:
        limit = 75

    scope = str(int(channel_id)) if channel_id is not None else ""
    doc_id = f"{key}:{date}:{scope}"

    col = db_handler.get_collection("daily_playlists").collection
    doc = await col.find_one({"_id": doc_id}, {"_id": 0, "track_ids": 1})
    if isinstance(doc, dict) and isinstance(doc.get("track_ids"), list) and doc["track_ids"]:
        return [str(x) for x in doc["track_ids"] if str(x)][:limit]

    seed = _daily_playlist_seed(key=key, date=date, channel_id=channel_id)

    if key in {"random", "mix"}:
        res = await random_tracks(limit=limit, seed=seed, channel_id=channel_id)
        track_ids = [str(it.id) for it in (res.items or []) if getattr(it, "id", None)]
        await col.update_one(
            {"_id": doc_id},
            {
                "$setOnInsert": {
                    "key": key,
                    "date": date,
                    "channel_id": int(channel_id) if channel_id is not None else None,
                    "track_ids": track_ids,
                    "generated_at": float(time.time()),
                }
            },
            upsert=True,
        )
        return track_ids

    if key in {"top", "top-played"}:
        gcol = db_handler.globalplayback_collection.collection
        cursor = gcol.find({}, {"_id": 1}).sort([("plays", -1)]).limit(500)
        candidate_ids: list[str] = []
        async for row in cursor:
            tid = str(row.get("_id") or "").strip()
            if tid:
                candidate_ids.append(tid)
        if not candidate_ids:
            return []

        rng = random.Random(int(seed))
        rng.shuffle(candidate_ids)
        picked = candidate_ids[:limit]
        await col.update_one(
            {"_id": doc_id},
            {
                "$setOnInsert": {
                    "key": key,
                    "date": date,
                    "channel_id": int(channel_id) if channel_id is not None else None,
                    "track_ids": picked,
                    "generated_at": float(time.time()),
                }
            },
            upsert=True,
        )
        return picked

    if key in {"trending"}:
        now = float(time.time())
        since = now - 24 * 3600
        ucol = db_handler.userplayback_collection.collection
        cur = await ucol.aggregate(
            [
                {"$match": {"played_at": {"$gte": since}}},
                {"$group": {"_id": "$track_id", "plays": {"$sum": 1}}},
                {"$sort": {"plays": -1}},
                {"$limit": 500},
            ]
        )
        rows = await cur.to_list(length=500)
        candidate_ids = [str(r.get("_id") or "").strip() for r in (rows or []) if str(r.get("_id") or "").strip()]
        if not candidate_ids:
            return []
        rng = random.Random(int(seed))
        rng.shuffle(candidate_ids)
        picked = candidate_ids[:limit]
        await col.update_one(
            {"_id": doc_id},
            {
                "$setOnInsert": {
                    "key": key,
                    "date": date,
                    "channel_id": int(channel_id) if channel_id is not None else None,
                    "track_ids": picked,
                    "generated_at": float(time.time()),
                }
            },
            upsert=True,
        )
        return picked

    if key in {"rediscover"}:
        now = float(time.time())
        cutoff = now - 30 * 24 * 3600
        gcol = db_handler.globalplayback_collection.collection
        cursor = gcol.find({"last_played_at": {"$lt": cutoff}}, {"_id": 1}).sort([("plays", -1)]).limit(500)
        candidate_ids: list[str] = []
        async for row in cursor:
            tid = str(row.get("_id") or "").strip()
            if tid:
                candidate_ids.append(tid)
        if not candidate_ids:
            return []
        rng = random.Random(int(seed))
        rng.shuffle(candidate_ids)
        picked = candidate_ids[:limit]
        await col.update_one(
            {"_id": doc_id},
            {
                "$setOnInsert": {
                    "key": key,
                    "date": date,
                    "channel_id": int(channel_id) if channel_id is not None else None,
                    "track_ids": picked,
                    "generated_at": float(time.time()),
                }
            },
            upsert=True,
        )
        return picked

    if key in {"late-night"}:
        now = float(time.time())
        since = now - 30 * 24 * 3600
        ucol = db_handler.userplayback_collection.collection
        cur = await ucol.aggregate(
            [
                {"$match": {"played_at": {"$gte": since}}},
                {"$addFields": {"_dt": {"$toDate": {"$multiply": ["$played_at", 1000]}}}},
                {"$addFields": {"_hour": {"$hour": "$_dt"}}},
                {"$match": {"$or": [{"_hour": {"$gte": 22}}, {"_hour": {"$lte": 3}}]}},
                {"$group": {"_id": "$track_id", "plays": {"$sum": 1}}},
                {"$sort": {"plays": -1}},
                {"$limit": 500},
            ]
        )
        rows = await cur.to_list(length=500)
        candidate_ids = [str(r.get("_id") or "").strip() for r in (rows or []) if str(r.get("_id") or "").strip()]
        if not candidate_ids:
            return []
        rng = random.Random(int(seed))
        rng.shuffle(candidate_ids)
        picked = candidate_ids[:limit]
        await col.update_one(
            {"_id": doc_id},
            {
                "$setOnInsert": {
                    "key": key,
                    "date": date,
                    "channel_id": int(channel_id) if channel_id is not None else None,
                    "track_ids": picked,
                    "generated_at": float(time.time()),
                }
            },
            upsert=True,
        )
        return picked

    if key in {"rising"}:
        now = float(time.time())
        since = now - 3 * 24 * 3600
        ucol = db_handler.userplayback_collection.collection
        cur = await ucol.aggregate(
            [
                {"$match": {"played_at": {"$gte": since}}},
                {"$group": {"_id": "$track_id", "plays3": {"$sum": 1}}},
                {"$sort": {"plays3": -1}},
                {"$limit": 2000},
            ]
        )
        rows = await cur.to_list(length=2000)
        if not rows:
            return []
        plays3_by_id: dict[str, int] = {}
        ids: list[str] = []
        for r in rows:
            tid = str(r.get("_id") or "").strip()
            if not tid:
                continue
            p3 = int(r.get("plays3") or 0)
            if p3 <= 0:
                continue
            plays3_by_id[tid] = p3
            ids.append(tid)
        if not ids:
            return []
        gcol = db_handler.globalplayback_collection.collection
        cursor = gcol.find({"_id": {"$in": ids}}, {"_id": 1, "plays": 1})
        plays_all: dict[str, int] = {}
        async for row in cursor:
            tid = str(row.get("_id") or "").strip()
            if not tid:
                continue
            plays_all[tid] = int(row.get("plays") or 0)
        scored: list[tuple[float, int, str]] = []
        for tid in ids:
            p3 = int(plays3_by_id.get(tid) or 0)
            pall = int(plays_all.get(tid) or 0)
            denom = float(pall if pall > 0 else 1)
            score = float(p3) / denom
            scored.append((score, p3, tid))
        scored.sort(key=lambda x: (x[0], x[1]), reverse=True)
        candidate_ids = [tid for _, _, tid in scored[:500]]
        if not candidate_ids:
            return []
        rng = random.Random(int(seed))
        rng.shuffle(candidate_ids)
        picked = candidate_ids[:limit]
        await col.update_one(
            {"_id": doc_id},
            {
                "$setOnInsert": {
                    "key": key,
                    "date": date,
                    "channel_id": int(channel_id) if channel_id is not None else None,
                    "track_ids": picked,
                    "generated_at": float(time.time()),
                }
            },
            upsert=True,
        )
        return picked

    if key in {"surprise"}:
        gcol = db_handler.globalplayback_collection.collection
        cursor = gcol.find({}, {"_id": 1, "plays": 1}).sort([("plays", -1)]).limit(2000)
        candidates: list[tuple[str, int]] = []
        async for row in cursor:
            tid = str(row.get("_id") or "").strip()
            if not tid:
                continue
            candidates.append((tid, int(row.get("plays") or 0)))
        if not candidates:
            return []
        rng = random.Random(int(seed))
        pool = candidates[:]
        picked: list[str] = []
        while pool and len(picked) < limit:
            weights = [float((p + 1)) for _, p in pool]
            total = float(sum(weights))
            if total <= 0:
                idx = rng.randrange(0, len(pool))
            else:
                r = rng.random() * total
                acc = 0.0
                idx = 0
                for i, w in enumerate(weights):
                    acc += w
                    if acc >= r:
                        idx = i
                        break
            tid, _ = pool.pop(idx)
            picked.append(tid)
        await col.update_one(
            {"_id": doc_id},
            {
                "$setOnInsert": {
                    "key": key,
                    "date": date,
                    "channel_id": int(channel_id) if channel_id is not None else None,
                    "track_ids": picked,
                    "generated_at": float(time.time()),
                }
            },
            upsert=True,
        )
        return picked

    return []


async def get_daily_playlist(*, key: str, date: str | None = None, channel_id: int | None, limit: int, user_id: Optional[int] = None) -> BrowseResponse:
    key = _canon_daily_playlist_key(key)
    if not date:
        date = datetime.datetime.utcnow().date().isoformat()

    track_ids = await generate_daily_playlist(key=key, date=str(date), channel_id=channel_id, limit=int(limit))
    if not track_ids:
        return BrowseResponse(page=1, per_page=int(limit), total=0, items=[])
    items = await get_browse_items_by_ids(track_ids[: int(limit)], user_id=user_id)
    return BrowseResponse(page=1, per_page=int(limit), total=len(items), items=items, cover_url=None)

async def refresh_daily_playlist_cache(
    *,
    key: str,
    date: str,
    channel_id: int | None,
    limit: int = 75,
    refresh_cover: bool = True,
) -> dict[str, object]:
    k = _canon_daily_playlist_key(key)
    if k not in {"random", "top-played", "trending", "rediscover", "late-night", "rising", "surprise"}:
        raise ValueError("unknown daily playlist key")

    d = (date or "").strip()
    if not d:
        raise ValueError("date is required")

    scope = str(int(channel_id)) if channel_id is not None else ""
    doc_id = f"{k}:{d}:{scope}"

    col = db_handler.get_collection("daily_playlists").collection
    await col.delete_one({"_id": doc_id})

    track_ids = await generate_daily_playlist(key=k, date=d, channel_id=channel_id, limit=int(limit))

    cover_items = await get_browse_items_by_ids(track_ids[:4])
    collage_urls = [(it.cover_url or "").strip() for it in (cover_items or []) if (it.cover_url or "").strip()]

    cover_url = None
    normal_thumbnail = None

    res = {
        "key": k,
        "date": d,
        "channel_id": int(channel_id) if channel_id is not None else None,
        "track_count": len(track_ids),
        "cover_url": cover_url,
        "normal_thumbnail": normal_thumbnail,
        "generated_at": time.time(),
    }
    await col.insert_one({"_id": doc_id, "track_ids": track_ids, **res})
    return res


async def refresh_daily_playlists_bulk(
    *,
    date: str,
    keys: list[str] | None = None,
    channel_ids: list[int | None] | None = None,
    limit: int = 75,
) -> dict[str, object]:
    if not keys:
        keys = ["random", "top-played", "trending", "rediscover", "late-night", "rising", "surprise"]
    canon_keys = []
    for k in keys:
        ck = _canon_daily_playlist_key(k)
        if ck in {"random", "top-played", "trending", "rediscover", "late-night", "rising", "surprise"} and ck not in canon_keys:
            canon_keys.append(ck)
    if not canon_keys:
        raise ValueError("no valid keys")

    if channel_ids is None:
        channel_ids = [None]

    results: list[dict[str, object]] = []
    ok = 0
    failed = 0
    for cid in channel_ids:
        for k in canon_keys:
            try:
                res = await refresh_daily_playlist_cache(key=k, date=date, channel_id=cid, limit=int(limit), refresh_cover=True)
                results.append(res)
                ok += 1
            except Exception as e:
                results.append(
                    {
                        "key": k,
                        "date": date,
                        "channel_id": int(cid) if cid is not None else None,
                        "error": str(e) or "failed",
                    }
                )
                failed += 1
    return {"ok": ok, "failed": failed, "results": results}


async def refresh_user_top_played_cache(*, user_id: int, limit: int = 500, refresh_cover: bool = True, force_cover: bool = True) -> dict[str, object]:
    uid = int(user_id)
    if uid <= 0:
        raise ValueError("user_id must be positive")
    limit = int(limit)
    if limit <= 0:
        limit = 100
    if limit > 1000:
        limit = 1000

    col = db_handler.userplayback_collection.collection
    match = {"user_id": int(uid)}
    rows_cur = await col.aggregate(
        [
            {"$match": match},
            {"$group": {"_id": "$track_id", "plays": {"$sum": 1}, "last_played_at": {"$max": "$played_at"}}},
            {"$sort": {"plays": -1, "last_played_at": -1}},
            {"$limit": int(limit)},
        ]
    )
    rows = await rows_cur.to_list(length=limit)

    track_ids: list[str] = []
    for r in rows or []:
        tid = (r.get("_id") or "").strip() if isinstance(r.get("_id"), str) else str(r.get("_id") or "").strip()
        if tid:
            track_ids.append(tid)

    cover_items = await get_browse_items_by_ids(track_ids[:4])
    collage_urls = [(it.cover_url or "").strip() for it in (cover_items or []) if (it.cover_url or "").strip()]

    cache_col = db_handler.get_collection("user_top_played_cache").collection
    now = float(time.time())
    cover_id: str | None = None
    cover_url: str | None = None
    normal_thumbnail: str | None = None
    await cache_col.update_one(
        {"_id": str(uid)},
        {"$set": {"user_id": int(uid), "track_ids": track_ids, "generated_at": now, "cover_id": cover_id, "cover_url": cover_url, "normal_thumbnail": normal_thumbnail}},
        upsert=True,
    )

    return {
        "user_id": int(uid),
        "track_count": len(track_ids),
        "generated_at": now,
        "cover_id": cover_id,
        "cover_url": cover_url,
        "normal_thumbnail": normal_thumbnail,
    }


async def refresh_user_top_played_cache_bulk(
    *,
    user_ids: list[int] | None = None,
    limit_users: int | None = 200,
    limit_tracks: int = 500,
    refresh_cover: bool = True,
    force_cover: bool = True,
) -> dict[str, object]:
    ids: list[int] = []
    if user_ids:
        for v in user_ids:
            try:
                n = int(v)
            except Exception:
                continue
            if n > 0 and n not in ids:
                ids.append(n)
    else:
        col = db_handler.userplayback_collection.collection
        if limit_users is None:
            cur = await col.aggregate([{"$group": {"_id": "$user_id"}}])
            async for r in cur:
                try:
                    n = int(r.get("_id"))
                except Exception:
                    continue
                if n > 0 and n not in ids:
                    ids.append(n)
        else:
            limit_users = int(limit_users)
            if limit_users <= 0:
                limit_users = 50
            if limit_users > 2000:
                limit_users = 2000
            cur = await col.aggregate([{"$group": {"_id": "$user_id"}}, {"$limit": int(limit_users)}])
            rows = await cur.to_list(length=limit_users)
            for r in rows or []:
                try:
                    n = int(r.get("_id"))
                except Exception:
                    continue
                if n > 0 and n not in ids:
                    ids.append(n)

    ok = 0
    failed = 0
    results: list[dict[str, object]] = []
    for uid in ids:
        try:
            res = await refresh_user_top_played_cache(
                user_id=int(uid),
                limit=int(limit_tracks),
                refresh_cover=bool(refresh_cover),
                force_cover=bool(force_cover),
            )
            results.append(res)
            ok += 1
        except Exception as e:
            results.append({"user_id": int(uid), "error": str(e) or "failed"})
            failed += 1

    return {"ok": ok, "failed": failed, "results": results}


async def rebuild_global_playback_from_userplayback(*, batch_size: int = 1000) -> dict[str, object]:
    batch_size = int(batch_size)
    if batch_size <= 0:
        batch_size = 1000
    if batch_size > 5000:
        batch_size = 5000

    user_col = db_handler.userplayback_collection.collection
    global_col = db_handler.globalplayback_collection.collection

    await global_col.delete_many({})

    pipeline = [
        {"$group": {"_id": "$track_id", "plays": {"$sum": 1}, "last_played_at": {"$max": "$played_at"}}},
        {"$sort": {"plays": -1}},
    ]
    cur = await user_col.aggregate(pipeline)

    now = float(time.time())
    pending: list[tuple[str, int, float]] = []
    processed = 0

    async def _flush() -> None:
        nonlocal pending
        if not pending:
            return
        ops = [
            UpdateOne(
                {"_id": tid},
                {"$set": {"plays": plays, "last_played_at": last_played_at, "updated_at": now}},
                upsert=True,
            )
            for (tid, plays, last_played_at) in pending
        ]
        try:
            await global_col.bulk_write(ops, ordered=False)
        except Exception:
            for tid, plays, last_played_at in pending:
                try:
                    await global_col.update_one(
                        {"_id": tid},
                        {"$set": {"plays": plays, "last_played_at": last_played_at, "updated_at": now}},
                        upsert=True,
                    )
                except Exception:
                    pass
        pending = []

    async for row in cur:
        tid = str(row.get("_id") or "").strip()
        if not tid:
            continue
        plays = int(row.get("plays") or 0)
        last_played_at = float(row.get("last_played_at") or 0.0)
        pending.append((tid, plays, last_played_at))
        processed += 1
        if len(pending) >= batch_size:
            await _flush()

    if processed == 0:
        audio_col = get_audio_tracks_collection()
        cursor = audio_col.find({"deleted": {"$ne": True}}, {"_id": 1}).limit(500)
        rank = 500
        async for doc in cursor:
            tid = str(doc.get("_id") or "").strip()
            if not tid:
                continue
            pending.append((tid, rank, now))
            processed += 1
            rank -= 1
            if rank <= 0:
                rank = 1
            if len(pending) >= batch_size:
                await _flush()

    await _flush()

    try:
        await global_col.create_index([("plays", -1)])
        await global_col.create_index([("last_played_at", -1)])
    except Exception:
        pass

    return {"ok": True, "tracks": processed, "updated_at": now}

async def user_top_played_tracks(*, user_id: int, page: int, per_page: int) -> BrowseResponse:
    page = int(page)
    if page < 1:
        page = 1
    per_page = int(per_page)
    if per_page <= 0:
        per_page = 20
    if per_page > 100:
        per_page = 100
    skip = (page - 1) * per_page

    cache_col = db_handler.get_collection("user_top_played_cache").collection
    cached = await cache_col.find_one({"_id": str(int(user_id))}, {"_id": 0, "track_ids": 1, "cover_url": 1, "cover_id": 1})
    cover_url: str | None = None
    if isinstance(cached, dict) and isinstance(cached.get("cover_url"), str) and cached["cover_url"].strip():
        cover_url = cached["cover_url"].strip()

    if isinstance(cached, dict) and isinstance(cached.get("track_ids"), list) and cached["track_ids"]:
        all_ids = [str(x) for x in cached["track_ids"] if str(x)]
        total = len(all_ids)
        start = int(skip)
        end = int(skip + per_page)
        items = await get_browse_items_by_ids(all_ids[start:end], user_id=user_id)
        return BrowseResponse(page=page, per_page=per_page, total=total, items=items, cover_url=cover_url)

    col = db_handler.userplayback_collection.collection
    match = {"user_id": int(user_id)}

    total_cur = await col.aggregate(
        [
            {"$match": match},
            {"$group": {"_id": "$track_id"}},
            {"$count": "total"},
        ]
    )
    total_rows = await total_cur.to_list(length=1)
    total = int(total_rows[0]["total"]) if total_rows else 0
    if total <= 0:
        return BrowseResponse(page=page, per_page=per_page, total=0, items=[], cover_url=cover_url)

    rows_cur = await col.aggregate(
        [
            {"$match": match},
            {"$group": {"_id": "$track_id", "plays": {"$sum": 1}, "last_played_at": {"$max": "$played_at"}}},
            {"$sort": {"plays": -1, "last_played_at": -1}},
            {"$skip": int(skip)},
            {"$limit": int(per_page)},
        ]
    )
    rows = await rows_cur.to_list(length=per_page)

    track_ids: list[str] = []
    for r in rows or []:
        tid = (r.get("_id") or "").strip() if isinstance(r.get("_id"), str) else str(r.get("_id") or "").strip()
        if tid:
            track_ids.append(tid)

    items = await get_browse_items_by_ids(track_ids, user_id=user_id)
    return BrowseResponse(page=page, per_page=per_page, total=total, items=items, cover_url=cover_url)


async def get_track_by_id(track_id: str, user_id: Optional[int] = None) -> dict | None:
    col = get_audio_tracks_collection()
    doc = await col.find_one({"_id": track_id, "deleted": {"$ne": True}})
    if not doc:
        return None
    if "_id" in doc:
        doc["_id"] = _as_str_id(doc["_id"])
    spotify = doc.get("spotify")
    if not isinstance(spotify, dict):
        spotify = {}
    audio = doc.get("audio") if isinstance(doc.get("audio"), dict) else {}
    title = (audio.get("title") or "").strip()
    artist = (audio.get("artist") or "").strip() or (audio.get("performer") or "").strip()
    album = (audio.get("album") or "").strip()
    year = audio.get("year")

    url = spotify.get("url") or spotify.get("spotify_url")
    url = _clean_url(url)
    track_spotify_id = spotify.get("track_spotify_id")
    track_spotify_id = track_spotify_id.strip() if isinstance(track_spotify_id, str) else ""
    cover_url = _clean_url(spotify.get("cover_url"))
    if cover_url:
        spotify["cover_url"] = cover_url
    if url:
        spotify["url"] = url

    if "links" in spotify or "spotify_url" in spotify:
        try:
            await col.update_one({"_id": track_id}, {"$unset": {"spotify.links": "", "spotify.spotify_url": ""}})
        except Exception:
            pass
        spotify.pop("links", None)
        spotify.pop("spotify_url", None)

    if (not url or not track_spotify_id) and title and artist:
        try:
            from stream.helpers.cover_search import spotify_best_track

            sp = await spotify_best_track(title=title, artist=artist, album=album, year=year)
        except Exception:
            sp = None

        if isinstance(sp, dict):
            updates: dict[str, Any] = {}
            sp_id = sp.get("id")
            if isinstance(sp_id, str) and sp_id.strip():
                sp_id = sp_id.strip()
                spotify["track_spotify_id"] = sp_id
                updates["spotify.track_spotify_id"] = sp_id
            ext = sp.get("external_urls") if isinstance(sp.get("external_urls"), dict) else {}
            sp_url = ext.get("spotify")
            if isinstance(sp_url, str) and sp_url.strip():
                sp_url = _clean_url(sp_url)
                spotify["url"] = sp_url
                updates["spotify.url"] = sp_url
            if updates:
                try:
                    await col.update_one({"_id": track_id}, {"$set": updates})
                except Exception:
                    pass
            doc["spotify"] = spotify
    _normalize_spotify(doc)
    liked_set = await get_user_liked_track_ids(user_id, [doc["_id"]]) if (user_id and user_id > 0) else set()
    doc["liked"] = doc["_id"] in liked_set
    return doc

async def get_tracks_by_ids(track_ids: list[str], user_id: Optional[int] = None) -> list[dict]:
    ids = [str(x) for x in (track_ids or []) if str(x)]
    if not ids:
        return []

    col = get_audio_tracks_collection()
    projection = {
        "_id": 1,
        "source_chat_id": 1,
        "source_message_id": 1,
        "telegram": 1,
        "audio": 1,
        "spotify": 1,
        "content_hash": 1,
        "fingerprint": 1,
        "created_at": 1,
        "updated_at": 1,
    }
    cursor = col.find({"_id": {"$in": ids}, "deleted": {"$ne": True}}, projection)
    docs: list[dict] = []
    async for doc in cursor:
        if "_id" in doc:
            doc["_id"] = _as_str_id(doc["_id"])
        if not doc.get("created_at") and doc.get("updated_at"):
            doc["created_at"] = doc.get("updated_at")
        _normalize_spotify(doc)
        docs.append(doc)

    liked_set = await get_user_liked_track_ids(user_id, ids) if (user_id and user_id > 0) else set()
    by_id = {str(d.get("_id")): d for d in docs if d.get("_id")}
    res = []
    for t in ids:
        if t in by_id:
            item = by_id[t]
            item["liked"] = t in liked_set
            res.append(item)
    return res

