import asyncio
import json
import re
import time
from typing import Any
import unicodedata
from os import path as ospath

from aiofiles.os import remove as aioremove
from pymongo.errors import DuplicateKeyError
from pyrogram import filters
from pyrogram.types import Message

from stream import bot, get_primary_client_user_id
from stream.core.config_manager import Config
from stream.database.MongoDb import db_handler
from stream.helpers.cover_search import fetch_artist_avatar_info, find_best_cover_url, spotify_best_track
from stream.helpers.dedup import metadata_fingerprint, sha256_prefix_file
from stream.helpers.logger import LOGGER
from stream.plugins.Analyzer.mediaHelper import (
    _is_junk_title,
    download_message_media,
    ensure_media_dir,
    extract_audio_metadata_normalized,
    infer_artist_title,
    run_mediainfo,
    sanitize_filename,
)

LOG = LOGGER(__name__)

_ENRICH_WORKERS: list[asyncio.Task] = []
_INDEX_TASKS: dict[str, asyncio.Task] = {}
_INDEX_TASKS_LOCK = asyncio.Lock()
_ENRICH_RETRY_DELAY_SEC = 60.0


async def _mark_enrichment_retry(
    doc_id: str,
    reason: str,
    *,
    retry_delay: float | None = None,
) -> None:
    now = time.time()
    delay = _ENRICH_RETRY_DELAY_SEC if retry_delay is None else float(retry_delay)
    await db_handler.audio_collection.collection.update_one(
        {"_id": doc_id},
        {
            "$set": {
                "enriched": False,
                "enrichment_error": str(reason or "enrichment failed")[:500],
                "enrichment_error_at": now,
                "enrich_retry_after": now + max(0.0, delay),
                "updated_at": now,
            },
            "$unset": {"enriching": "", "enrichment_started_at": ""},
        },
    )


async def _fetch_message(chat_id: int, message_id: int) -> Message | None:
    from stream.plugins.userBot.service import _USERBOT_INSTANCE
    channel_id = _coerce_int(getattr(Config, "CHANNEL_ID", None))

    clients = [bot]
    if _USERBOT_INSTANCE:
        if chat_id != channel_id:
            clients = [_USERBOT_INSTANCE, bot]
        else:
            clients = [bot, _USERBOT_INSTANCE]

    for c in clients:
        if not c:
            continue
        try:
            msg = await c.get_messages(chat_id, message_id)
            if msg and not getattr(msg, "empty", False):
                return msg
        except Exception:
            pass

    return None


async def _enrichment_loop(worker_id: int):
    while True:
        try:
            col = db_handler.audio_collection.collection
            now = time.time()
            stale_threshold = now - 300
            doc = await col.find_one_and_update(
                {
                    "enriched": False,
                    "deleted": {"$ne": True},
                    "$and": [
                        {
                            "$or": [
                                {"enriching": {"$ne": True}},
                                {"enrichment_started_at": {"$lt": stale_threshold}},
                                {"enrichment_started_at": {"$exists": False}},
                            ]
                        },
                        {
                            "$or": [
                                {"enrich_retry_after": {"$exists": False}},
                                {"enrich_retry_after": {"$lte": now}},
                            ]
                        },
                    ],
                },
                {"$set": {"enriching": True, "enrichment_started_at": now}},
                sort=[("source_message_id", -1)],
            )
            if not doc:
                await asyncio.sleep(2)
                continue

            source_chat_id = doc.get("source_chat_id")
            source_msg_id = doc.get("source_message_id")
            chat_id = doc.get("cache_chat_id") or source_chat_id
            msg_id = doc.get("cache_message_id") or source_msg_id

            if chat_id is None or msg_id is None:
                await _mark_enrichment_retry(
                    doc["_id"],
                    "missing cache/source chat or message id",
                    retry_delay=300,
                )
                continue

            msg = await _fetch_message(chat_id, msg_id)
            media = _pick_audio_media(msg) if msg else None

            if not msg or not media:
                await _mark_enrichment_retry(
                    doc["_id"],
                    "source message unavailable or has no audio media",
                    retry_delay=300,
                )
                continue

            # We use the existing _enrich_audio_doc logic which now supports partial download
            try:
                await _enrich_audio_doc(msg, media, existing_doc_id=str(doc["_id"]))
                # _enrich_audio_doc marks enriched only after all enrichment steps finish.
            except Exception as e:
                LOG.warning(
                    f"Enrichment worker {worker_id} failed on {chat_id}:{msg_id}: {e}"
                )
                await _mark_enrichment_retry(doc["_id"], str(e))

        except asyncio.CancelledError:
            break
        except Exception as e:
            LOG.error(f"Enrichment worker {worker_id} error: {e}")
            await asyncio.sleep(2)


def start_enrichment_workers():
    _ENRICH_WORKERS[:] = [task for task in _ENRICH_WORKERS if not task.done()]
    if _ENRICH_WORKERS:
        LOG.info(f"Enrichment workers already running: {len(_ENRICH_WORKERS)}")
        return

    workers = int(getattr(Config, "ENRICHMENT_WORKERS", getattr(Config, "PROCESSING_CONTENT", 16)))
    if workers <= 0:
        workers = 16
    for i in range(workers):
        task = asyncio.create_task(_enrichment_loop(i))
        _ENRICH_WORKERS.append(task)
    LOG.info(f"Started {workers} enrichment workers.")


async def stop_enrichment_workers(timeout: float = 3.0):
    for task in _ENRICH_WORKERS:
        task.cancel()
    if _ENRICH_WORKERS:
        try:
            await asyncio.wait_for(
                asyncio.gather(*_ENRICH_WORKERS, return_exceptions=True), timeout=timeout
            )
        except (asyncio.TimeoutError, asyncio.CancelledError):
            pass
    _ENRICH_WORKERS.clear()


def _extract_message_file_id(message: Message) -> str | None:
    if not message:
        return None
    media = getattr(message, "audio", None) or getattr(message, "document", None)
    if not media:
        return None
    fid = getattr(media, "file_id", None)
    if not fid:
        return None
    return str(fid)


async def _ensure_dump_message_id(
    *, source_chat_id: int, source_message_id: int
) -> int | None:
    dump_channel_id = getattr(Config, "DUMP_CHANNEL_ID", None)
    try:
        dump_channel_id = int(dump_channel_id)
    except Exception:
        dump_channel_id = 0
    if not dump_channel_id:
        return None

    doc = await db_handler.audio_collection.find_one(
        {
            "source_chat_id": int(source_chat_id),
            "source_message_id": int(source_message_id),
        },
        projection={"telegram": 1},
    )
    telegram = (doc or {}).get("telegram") or {}
    dump_message_id = telegram.get("dump_message_id")
    try:
        dump_message_id = int(dump_message_id) if dump_message_id is not None else None
    except Exception:
        dump_message_id = None
    if dump_message_id:
        return dump_message_id

    try:
        sent = await bot.copy_message(
            chat_id=int(dump_channel_id),
            from_chat_id=int(source_chat_id),
            message_id=int(source_message_id),
        )
    except Exception:
        return None

    dump_message_id = int(getattr(sent, "id"))
    await db_handler.audio_collection.update_one(
        {
            "source_chat_id": int(source_chat_id),
            "source_message_id": int(source_message_id),
        },
        {
            "$set": {
                "telegram.dump_message_id": dump_message_id,
                "updated_at": time.time(),
            }
        },
        upsert=False,
    )
    return dump_message_id


async def _sync_file_ids_for_all_clients(
    *, source_chat_id: int, source_message_id: int
) -> None:
    try:
        from stream import _multi_lock, multi_clients
    except Exception:
        return

    try:
        source_chat_id = int(source_chat_id)
        source_message_id = int(source_message_id)
    except Exception:
        return

    track_filter = {
        "$or": [
            {
                "source_chat_id": int(source_chat_id),
                "source_message_id": int(source_message_id),
            },
            {
                "cache_chat_id": int(source_chat_id),
                "cache_message_id": int(source_message_id),
            },
        ]
    }

    async with _multi_lock:
        clients = list(multi_clients.items())

    if not clients:
        return

    dump_channel_id = getattr(Config, "DUMP_CHANNEL_ID", None)
    try:
        dump_channel_id = int(dump_channel_id)
    except Exception:
        dump_channel_id = 0

    dump_message_id: int | None = None
    for uid, client in clients:
        fid = None
        try:
            msg = await client.get_messages(int(source_chat_id), int(source_message_id))
            fid = _extract_message_file_id(msg)
        except Exception:
            fid = None

        if not fid and dump_channel_id:
            if dump_message_id is None:
                dump_message_id = await _ensure_dump_message_id(
                    source_chat_id=int(source_chat_id),
                    source_message_id=int(source_message_id),
                )
            if dump_message_id:
                try:
                    msg = await client.get_messages(
                        int(dump_channel_id), int(dump_message_id)
                    )
                    fid = _extract_message_file_id(msg)
                except Exception:
                    fid = None

        if not fid:
            continue

        key = str(int(uid))
        await db_handler.audio_collection.update_one(
            track_filter,
            {"$set": {f"telegram.file_ids.{key}": str(fid), "updated_at": time.time()}},
            upsert=False,
        )
        LOG.debug(
            f"index stored file_id source={source_chat_id}:{source_message_id} client={key} file_id={str(fid)}"
        )


def _dbg(msg: str) -> None:
    if bool(getattr(Config, "DEBUG", False)):
        LOG.debug(msg)


_AUDIO_EXTENSIONS = (
    ".mp3",
    ".flac",
    ".wav",
    ".wave",
    ".m4a",
    ".aac",
    ".ogg",
    ".opus",
    ".alac",
    ".aif",
    ".aiff",
    ".wma",
)


def _pick_audio_media(message: Message):
    media = message.audio
    if not media and message.document:
        mime = (message.document.mime_type or "").lower()
        fname = (message.document.file_name or "").lower()
        if mime.startswith("audio/") or fname.endswith(_AUDIO_EXTENSIONS):
            media = message.document
    if not media:
        return None
    return media


def _coerce_int(value):
    if value is None or isinstance(value, bool):
        return None
    try:
        return int(value)
    except Exception:
        return None


def _normalize_mime_type(mime: Any, file_name: str | None = None) -> str | None:
    if file_name:
        fn = file_name.lower().strip()
        if fn.endswith((".wav", ".wave")):
            return "audio/wav"
        if fn.endswith(".flac"):
            return "audio/flac"
        if fn.endswith(".mp3"):
            return "audio/mpeg"
        if fn.endswith(".m4a"):
            return "audio/mp4"
        if fn.endswith((".ogg", ".opus")):
            return "audio/ogg"
        if fn.endswith(".aac"):
            return "audio/aac"
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


_META_KEYS = {"source_chat_id", "source_message_id", "topic_id", "topic_name"}


def _parse_meta_caption(caption) -> dict:
    text = str(caption or "")
    if not text.strip():
        return {}
    lines = text.replace("\r\n", "\n").replace("\r", "\n").split("\n")
    if not lines or lines[0].strip() != "#META":
        return {}

    meta: dict[str, str] = {}
    for raw in lines[1:]:
        line = raw.strip()
        if not line:
            break
        if "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        if key in _META_KEYS:
            meta[key] = value.strip()
    return meta


_FORWARD_TOPIC_CACHE: dict[tuple[int, int], tuple[int, str]] = {}


async def register_forward_topics(
    source_chat_id: int | str,
    message_ids: list[int],
    topic_id: int,
    topic_name: str,
) -> None:
    """Register topic mapping for forwarded messages so audioIndex can resolve forum topic metadata."""
    t_id = int(topic_id)
    t_name = str(topic_name or "").strip() or ("main" if t_id == 0 else f"topic_{t_id}")
    try:
        s_cid = int(source_chat_id)
    except Exception:
        s_cid = 0

    for mid in message_ids:
        _FORWARD_TOPIC_CACHE[(s_cid, int(mid))] = (t_id, t_name)

    if len(_FORWARD_TOPIC_CACHE) > 20000:
        keys_to_remove = list(_FORWARD_TOPIC_CACHE.keys())[:5000]
        for k in keys_to_remove:
            _FORWARD_TOPIC_CACHE.pop(k, None)

    try:
        col = db_handler.get_collection("forward_topics").collection
        now = time.time()
        docs = [
            {
                "_id": f"{s_cid}:{int(mid)}",
                "source_chat_id": s_cid,
                "source_message_id": int(mid),
                "topic_id": t_id,
                "topic_name": t_name,
                "created_at": now,
            }
            for mid in message_ids
        ]
        if docs:
            from pymongo import UpdateOne

            ops = [
                UpdateOne({"_id": d["_id"]}, {"$set": d}, upsert=True)
                for d in docs
            ]
            await col.bulk_write(ops, ordered=False)
    except Exception:
        pass


_FORWARD_CACHE: dict[tuple[int, int], tuple[int, int, int, str]] = {}


async def register_forwarded_batch(
    cache_chat_id: int | str,
    forwarded_pairs: list[tuple[int, int]],
    source_chat_id: int | str,
    topic_id: int,
    topic_name: str,
) -> None:
    """Register mapping of forwarded messages in CHANNEL_ID back to their source message IDs and topic."""
    t_id = int(topic_id)
    t_name = str(topic_name or "").strip() or ("main" if t_id == 0 else f"topic_{t_id}")
    try:
        s_cid = int(source_chat_id)
        c_cid = int(cache_chat_id)
    except Exception:
        return

    for c_mid, s_mid in forwarded_pairs:
        _FORWARD_CACHE[(c_cid, int(c_mid))] = (s_cid, int(s_mid), t_id, t_name)

    if len(_FORWARD_CACHE) > 20000:
        keys_to_remove = list(_FORWARD_CACHE.keys())[:5000]
        for k in keys_to_remove:
            _FORWARD_CACHE.pop(k, None)

    try:
        col = db_handler.get_collection("forward_cache").collection
        now = time.time()
        docs = [
            {
                "_id": f"{c_cid}:{int(c_mid)}",
                "cache_chat_id": c_cid,
                "cache_message_id": int(c_mid),
                "source_chat_id": s_cid,
                "source_message_id": int(s_mid),
                "topic_id": t_id,
                "topic_name": t_name,
                "created_at": now,
            }
            for c_mid, s_mid in forwarded_pairs
        ]
        if docs:
            from pymongo import UpdateOne

            ops = [
                UpdateOne({"_id": d["_id"]}, {"$set": d}, upsert=True)
                for d in docs
            ]
            await col.bulk_write(ops, ordered=False)
    except Exception:
        pass


async def _lookup_forward_cache(cache_chat_id: int | str, cache_message_id: int) -> tuple[int, int, int, str] | None:
    try:
        c_cid = int(cache_chat_id)
        c_mid = int(cache_message_id)
    except Exception:
        return None

    cached = _FORWARD_CACHE.get((c_cid, c_mid))
    if cached:
        return cached

    try:
        col = db_handler.get_collection("forward_cache").collection
        doc = await col.find_one({"_id": f"{c_cid}:{c_mid}"})
        if doc and "source_chat_id" in doc and "source_message_id" in doc:
            res = (
                int(doc["source_chat_id"]),
                int(doc["source_message_id"]),
                int(doc.get("topic_id", 0)),
                str(doc.get("topic_name", "main")),
            )
            _FORWARD_CACHE[(c_cid, c_mid)] = res
            return res
    except Exception:
        pass

    return None


def _lookup_forward_topic(source_chat_id: int | str, message_id: int) -> tuple[int, str] | None:
    try:
        s_cid = int(source_chat_id)
        m_id = int(message_id)
        return _FORWARD_TOPIC_CACHE.get((s_cid, m_id))
    except Exception:
        return None


async def _source_metadata_from_message(message: Message) -> dict:
    meta = _parse_meta_caption(getattr(message, "caption", None))
    cache_chat_id = _coerce_int(getattr(getattr(message, "chat", None), "id", None))
    cache_message_id = _coerce_int(getattr(message, "id", None))

    forward_chat_id = None
    forward_msg_id = None
    if getattr(message, "forward_from_chat", None):
        forward_chat_id = _coerce_int(getattr(message.forward_from_chat, "id", None))
        forward_msg_id = _coerce_int(getattr(message, "forward_from_message_id", None))
    elif getattr(message, "forward_origin", None):
        orig = message.forward_origin
        orig_chat = getattr(orig, "chat", None)
        if orig_chat:
            forward_chat_id = _coerce_int(getattr(orig_chat, "id", None))
            forward_msg_id = _coerce_int(getattr(orig, "message_id", None))

    # Check forward cache if message arrived without forward header or #META
    cached_fwd = None
    if cache_chat_id and cache_message_id:
        cached_fwd = await _lookup_forward_cache(cache_chat_id, cache_message_id)
        # If forwarded without name (hide_sender_name=True), forward headers are absent.
        # Wait up to 3 seconds for register_forwarded_batch in case the Telegram message arrived
        # before the forward_messages() RPC response completed.
        if (
            not cached_fwd
            and not forward_chat_id
            and not meta.get("source_chat_id")
            and cache_chat_id == _coerce_int(getattr(Config, "CHANNEL_ID", None))
        ):
            for _ in range(30):
                await asyncio.sleep(0.1)
                cached_fwd = await _lookup_forward_cache(cache_chat_id, cache_message_id)
                if cached_fwd:
                    break

    if cached_fwd:
        f_src_chat, f_src_msg, f_topic_id, f_topic_name = cached_fwd
        source_chat_id = _coerce_int(meta.get("source_chat_id")) or forward_chat_id or f_src_chat or cache_chat_id
        source_message_id = _coerce_int(meta.get("source_message_id")) or forward_msg_id or f_src_msg or cache_message_id
        topic_id = _coerce_int(meta.get("topic_id")) or f_topic_id
        topic_name = str(meta.get("topic_name") or f_topic_name or "").strip()
    else:
        source_chat_id = _coerce_int(meta.get("source_chat_id")) or forward_chat_id or cache_chat_id
        source_message_id = _coerce_int(meta.get("source_message_id")) or forward_msg_id or cache_message_id
        topic_id = _coerce_int(meta.get("topic_id"))
        topic_name = str(meta.get("topic_name") or "").strip()

    if (not topic_id or topic_id == 0) and source_chat_id and source_message_id:
        cached = _lookup_forward_topic(source_chat_id, source_message_id)
        if cached:
            topic_id, topic_name = cached

    if topic_id is None:
        topic_id = _coerce_int(getattr(message, "message_thread_id", None))
    if topic_id is None:
        topic_id = 0

    if not topic_name:
        topic_name = "main" if int(topic_id) == 0 else f"topic_{int(topic_id)}"

    return {
        "source_chat_id": source_chat_id,
        "source_message_id": source_message_id,
        "topic_id": int(topic_id),
        "topic_name": topic_name,
        "cache_chat_id": cache_chat_id,
        "cache_message_id": cache_message_id,
    }


_ARTIST_SPLIT_RE = re.compile(
    r"\s*(?:,|/|&| and | x | feat\. | feat | ft\. | ft )\s*", flags=re.I
)
_ALBUM_SLUG_RE = re.compile(r"[^a-z0-9 ]+", flags=re.I)


def _split_artists(value: str) -> list[str]:
    raw = (value or "").strip()
    if not raw:
        return []
    raw = (
        raw.replace("(", " ")
        .replace(")", " ")
        .replace("[", " ")
        .replace("]", " ")
        .strip()
    )
    parts = [p.strip() for p in _ARTIST_SPLIT_RE.split(raw) if p and p.strip()]
    out: list[str] = []
    seen: set[str] = set()
    for p in parts:
        key = p.casefold()
        if key in seen:
            continue
        seen.add(key)
        out.append(p)
    return out


def _slugify(value: str) -> str:
    s = (value or "").strip().lower()
    if not s:
        return ""
    s = s.replace("÷", " divide ").replace("&", " and ").replace("+", " plus ")
    s = unicodedata.normalize("NFKD", s)
    s = s.encode("ascii", "ignore").decode("ascii")
    s = _ALBUM_SLUG_RE.sub(" ", s)
    s = re.sub(r"\s+", "_", s.strip())
    s = re.sub(r"_+", "_", s).strip("_")
    return s


def _coerce_year(value) -> int | None:
    if value is None or isinstance(value, bool):
        return None
    try:
        y = int(value)
    except Exception:
        return None
    if y < 1000 or y > 2100:
        return None
    return y


def _album_id(*, album: str, year: int | None) -> str:
    b = _slugify(album)
    if not b:
        return ""
    y = _coerce_year(year)
    if y is not None:
        return f"album_{b}_{y}"
    return f"album_{b}"


def _best_title_artist_album(
    *, audio_doc: dict, media, inferred_title: str, inferred_artist: str
) -> tuple[str, str, str]:
    title = ""
    for cand in (audio_doc.get("title"), getattr(media, "title", ""), inferred_title):
        c = (cand or "").strip()
        if c and not _is_junk_title(c):
            title = c
            break
    artist = (
        audio_doc.get("artist")
        or getattr(media, "performer", "")
        or inferred_artist
        or ""
    ).strip()
    album = (audio_doc.get("album") or "").strip()

    file_name = getattr(media, "file_name", "") or ""
    if not title:
        base = ospath.splitext(ospath.basename(file_name))[0].strip()
        title = base or str(getattr(media, "file_unique_id", "") or "").strip()

    if not title:
        title = str(getattr(media, "file_id", "") or "").strip()

    return title, artist, album


async def _upsert_minimal(message: Message, media, enriching: bool = False) -> str:
    file_unique_id = (
        getattr(media, "file_unique_id", None) or f"{message.chat.id}:{message.id}"
    )
    source_meta = await _source_metadata_from_message(message)
    if int(source_meta.get("topic_id") or 0) == 0:
        s_cid = source_meta.get("source_chat_id")
        s_mid = source_meta.get("source_message_id")
        if s_cid and s_mid and s_cid != source_meta.get("cache_chat_id"):
            try:
                fcol = db_handler.get_collection("forward_topics").collection
                fdoc = await fcol.find_one({"_id": f"{int(s_cid)}:{int(s_mid)}"})
                if fdoc and fdoc.get("topic_id"):
                    source_meta["topic_id"] = int(fdoc["topic_id"])
                    source_meta["topic_name"] = str(fdoc.get("topic_name") or "main")
            except Exception:
                pass
    file_id = getattr(media, "file_id", None)
    primary_uid = get_primary_client_user_id()
    file_ids = None
    if primary_uid is not None and file_id:
        file_ids = {str(int(primary_uid)): file_id}
    file_size = _coerce_int(getattr(media, "file_size", None))
    duration_sec = _coerce_int(getattr(media, "duration", None))

    file_name = getattr(media, "file_name", "") or ""
    inferred_artist, inferred_title = infer_artist_title(file_name)

    title = (getattr(media, "title", "") or inferred_title or "").strip()
    artist = (getattr(media, "performer", "") or inferred_artist or "").strip()
    artists = _split_artists(artist) if artist else []

    if not title:
        base = ospath.splitext(ospath.basename(file_name))[0].strip()
        title = base or str(message.id)

    payload = {
        "telegram": {
            "file_id": file_id,
            "mime_type": _normalize_mime_type(getattr(media, "mime_type", None), file_name),
            "file_size": file_size,
        },
        "audio": {
            "title": title,
            "artist": artist,
            "artists": artists if artists else None,
            "duration_sec": duration_sec,
        },
        "source_chat_id": source_meta.get("source_chat_id"),
        "source_message_id": source_meta.get("source_message_id"),
        "topic_id": source_meta.get("topic_id"),
        "topic_name": source_meta.get("topic_name"),
        "cache_chat_id": source_meta.get("cache_chat_id"),
        "cache_message_id": source_meta.get("cache_message_id"),
        "indexed": True,
        "enriched": False,
    }
    if enriching:
        payload["enriching"] = True

    now_ts = time.time()
    payload["updated_at"] = now_ts

    if file_ids:
        payload["telegram"]["file_ids"] = file_ids

    update = {
        "$set": {k: v for k, v in payload.items() if v is not None},
        "$setOnInsert": {"created_at": now_ts},
    }
    if not enriching:
        update["$unset"] = {
            "enriching": "",
            "enrichment_started_at": "",
            "enrichment_error": "",
            "enrichment_error_at": "",
            "enrich_retry_after": "",
        }

    await db_handler.audio_collection.update_one(
        {"_id": file_unique_id},
        update,
        upsert=True,
    )
    return file_unique_id


def _get_wav_duration_from_file(file_path: str, file_size: int | None = None) -> int | None:
    import wave
    import io
    import struct

    try:
        with open(file_path, "rb") as f:
            header = f.read(65536)
        try:
            with wave.open(io.BytesIO(header), "rb") as w:
                rate = w.getframerate()
                frames = w.getnframes()
                if rate > 0 and frames > 0:
                    return int(round(frames / rate))
        except Exception:
            pass
        if len(header) >= 44 and header[:4] == b"RIFF" and header[8:12] == b"WAVE":
            pos = 12
            byte_rate = None
            data_size = None
            while pos + 8 <= len(header):
                chunk_id = header[pos:pos + 4]
                chunk_size = struct.unpack("<I", header[pos + 4:pos + 8])[0]
                if chunk_id == b"fmt " and pos + 8 + min(chunk_size, 16) <= len(header):
                    fmt_data = header[pos + 8:pos + 24]
                    if len(fmt_data) >= 16:
                        _, _, _, byte_rate, _, _ = struct.unpack("<HHIIHH", fmt_data[:16])
                elif chunk_id == b"data":
                    data_size = chunk_size
                pos += 8 + chunk_size
            if byte_rate and byte_rate > 0:
                if data_size and data_size > 0:
                    return int(round(data_size / byte_rate))
                if file_size and file_size > 0:
                    return int(round((file_size - 44) / byte_rate))
    except Exception:
        pass
    return None


async def _enrich_audio_doc(
    message: Message,
    media,
    existing_doc_id: str | None = None,
):
    source_meta = await _source_metadata_from_message(message)
    if int(source_meta.get("topic_id") or 0) == 0:
        s_cid = source_meta.get("source_chat_id")
        s_mid = source_meta.get("source_message_id")
        if s_cid and s_mid and s_cid != source_meta.get("cache_chat_id"):
            try:
                fcol = db_handler.get_collection("forward_topics").collection
                fdoc = await fcol.find_one({"_id": f"{int(s_cid)}:{int(s_mid)}"})
                if fdoc and fdoc.get("topic_id"):
                    source_meta["topic_id"] = int(fdoc["topic_id"])
                    source_meta["topic_name"] = str(fdoc.get("topic_name") or "main")
            except Exception:
                pass
    media_file_unique_id = getattr(media, "file_unique_id", None)
    file_unique_id = str(
        existing_doc_id or media_file_unique_id or f"{message.chat.id}:{message.id}"
    )
    if bool(getattr(Config, "DEBUG", False)):
        LOG.debug(
            f"[index] start file_unique_id={file_unique_id!r} chat={int(message.chat.id)} msg={int(message.id)} "
            f"file_name={str(getattr(media, 'file_name', '') or '')!r}"
        )

    base_dir = await ensure_media_dir()
    base_name = sanitize_filename(getattr(media, "file_name", "") or f"{message.id}")
    unique_key = (
        getattr(media, "file_unique_id", None) or f"{message.chat.id}_{message.id}"
    )
    unique_prefix = sanitize_filename(str(unique_key))[:48]
    nonce = str(time.time_ns())[-8:]
    filename = f"{unique_prefix}_{nonce}_{base_name}"
    file_path = ospath.join(base_dir, filename)

    file_size = getattr(media, "file_size", None)
    content_hash = None
    output = ""
    wav_dur = None
    partial_path = file_path + ".part"
    from stream.plugins.Analyzer.mediaHelper import download_partial_media

    try:
        max_chunk = int(getattr(Config, "PARTIAL_DOWNLOAD_BYTES", 2_000_000))
        await download_partial_media(message, partial_path, max_bytes=max_chunk)
        output = await run_mediainfo(partial_path)
        wav_dur = _get_wav_duration_from_file(partial_path, file_size=file_size)
        audio_doc_test = extract_audio_metadata_normalized(
            output, duration_sec=wav_dur, file_size=file_size
        )

        # If partial download failed to extract duration, do a full download.
        if not audio_doc_test.get("duration_sec"):
            LOG.debug(
                f"Partial mediainfo insufficient, falling back to full download for {file_unique_id}"
            )
            file_size_dl = await download_message_media(message, file_path)
            if file_size_dl:
                file_size = file_size_dl
            output = await run_mediainfo(file_path)
            wav_dur = _get_wav_duration_from_file(file_path, file_size=file_size)
            try:
                content_hash = await asyncio.to_thread(sha256_prefix_file, file_path)
            except Exception as e:
                LOG.warning(
                    f"Hashing failed chat={message.chat.id} msg={message.id}: {e}"
                )
        else:
            # We got enough info from partial download. Try hashing partial just as prefix
            try:
                content_hash = await asyncio.to_thread(sha256_prefix_file, partial_path)
            except Exception:
                pass

    except Exception as e:
        LOG.warning(
            f"Indexing failed chat={message.chat.id} msg={message.id}: {e}",
            exc_info=True,
        )
        try:
            from stream.plugins.Analyzer.mediaHelper import reset_media_session_for_message, _stream_media_client
            client = _stream_media_client(message)
            await reset_media_session_for_message(client, message)
        except Exception:
            pass
    finally:
        for p in (file_path, partial_path):
            try:
                await aioremove(p)
            except Exception:
                pass

    if not output:
        if bool(getattr(Config, "DEBUG", False)):
            LOG.debug(f"[index] mediainfo empty file_unique_id={file_unique_id!r}")
        await _mark_enrichment_retry(file_unique_id, "mediainfo produced no output")
        return

    duration_sec = _coerce_int(getattr(media, "duration", None)) or wav_dur

    audio_doc = extract_audio_metadata_normalized(
        output, duration_sec=duration_sec, file_size=file_size
    )

    file_name = getattr(media, "file_name", "") or ""
    inferred_performer, inferred_title = infer_artist_title(file_name)

    title, performer, album = _best_title_artist_album(
        audio_doc=audio_doc,
        media=media,
        inferred_title=inferred_title,
        inferred_artist=inferred_performer,
    )
    if bool(getattr(Config, "DEBUG", False)):
        LOG.debug(
            f"[index] metadata picked file_unique_id={file_unique_id!r} title={title!r} artist={performer!r} album={album!r} "
            f"duration_sec={audio_doc.get('duration_sec')!r}"
        )

    audio_doc["title"] = title
    if performer:
        audio_doc["artist"] = performer
        artists = _split_artists(performer)
        if artists:
            audio_doc["artists"] = artists
    if album:
        audio_doc["album"] = album
        aid = _album_id(album=album, year=_coerce_year(audio_doc.get("year")))
        if aid:
            audio_doc["album_id"] = aid

    origin_cover_url = None
    cover_url = None
    cover_source = None
    spotify: dict = {}

    spotify_enabled = bool(getattr(Config, "SPOTIFY_COVER_SEARCH", False))
    fallbacks_enabled = bool(getattr(Config, "MUSIC_HOADER_SEARCH", False))

    _dbg(
        "[cover] start "
        + json.dumps(
            {
                "title": title,
                "artist": performer,
                "album": album,
                "year": audio_doc.get("year"),
                "spotify_enabled": spotify_enabled,
                "fallbacks_enabled": fallbacks_enabled,
                "file_unique_id": file_unique_id,
            },
            ensure_ascii=False,
        )
    )

    small_cover_url = None

    async def _fetch_cover():
        try:
            return await find_best_cover_url(
                title=title,
                artist=performer,
                album=album,
                year=audio_doc.get("year"),
            )
        except Exception as ce:
            LOG.warning(f"Cover lookup failed chat={message.chat.id} msg={message.id}: {ce}")
            return None, None, None

    async def _fetch_avatar():
        if not performer:
            return None
        try:
            return await fetch_artist_avatar_info(performer)
        except Exception as ae:
            _dbg(f"[artist] failed to fetch artist avatar: {ae}")
            return None

    async def _fetch_spotify():
        try:
            return await spotify_best_track(
                title=title,
                artist=performer,
                album=album,
                year=audio_doc.get("year"),
            )
        except Exception as se:
            _dbg(f"[spotify] track lookup failed: {se}")
            return None

    cover_res, art_info, sp_track = await asyncio.gather(
        _fetch_cover(),
        _fetch_avatar(),
        _fetch_spotify(),
    )

    if cover_res:
        origin_cover_url, cover_source, small_cover_url = cover_res

    if origin_cover_url:
        cover_url = str(origin_cover_url).strip()
        if (
            cover_url.startswith("`")
            and cover_url.endswith("`")
            and len(cover_url) >= 2
        ):
            cover_url = cover_url[1:-1].strip()
        _dbg(
            f"[cover] found track={title!r} artist={performer!r} src={cover_source!r} url={origin_cover_url!r}"
        )

    if art_info and art_info.get("avatar_url"):
        spotify["artist_avatar"] = art_info.get("avatar_url")

    if cover_url:
        spotify["cover_url"] = cover_url
        if cover_source:
            spotify["cover_source"] = cover_source

    if cover_source in {"hoaders", "youtube"} and small_cover_url:
        spotify["cover_url"] = small_cover_url
        spotify["big_cover_url"] = cover_url
    elif small_cover_url:
        spotify["small_cover_url"] = small_cover_url

    if isinstance(sp_track, dict):
        sp_id = sp_track.get("id")
        if isinstance(sp_id, str) and sp_id.strip():
            spotify["track_spotify_id"] = sp_id.strip()
        ext = (
            sp_track.get("external_urls")
            if isinstance(sp_track.get("external_urls"), dict)
            else {}
        )
        sp_url = ext.get("spotify")
        if isinstance(sp_url, str) and sp_url.strip():
            s = sp_url.strip()
            spotify["url"] = s

    if bool(getattr(Config, "DEBUG", False)):
        LOG.debug(
            f"[spotify] resolved file_unique_id={file_unique_id!r} "
            f"track_spotify_id={spotify.get('track_spotify_id')!r} url={spotify.get('url')!r} cover_url={spotify.get('cover_url')!r}"
        )
        LOG.debug(
            f"[spotify] resolved file_unique_id={file_unique_id!r} "
            f"track_spotify_id={spotify.get('track_spotify_id')!r} url={spotify.get('url')!r} cover_url={spotify.get('cover_url')!r}"
        )

    _dbg(
        "[cover] done "
        + json.dumps(
            {
                "cover_url": cover_url,
                "cover_source": cover_source,
                "origin_cover_url": origin_cover_url,
            },
            ensure_ascii=False,
        )
    )

    file_id = getattr(media, "file_id", None)
    primary_uid = get_primary_client_user_id()
    primary_uid_key = None
    if primary_uid is not None:
        try:
            primary_uid_key = str(int(primary_uid))
        except Exception:
            primary_uid_key = None
    if primary_uid_key and file_id:
        LOG.debug(
            f"index file_ids set primary={primary_uid_key} file_id={file_id} doc={file_unique_id}"
        )

    fingerprint = metadata_fingerprint(
        title=title,
        artist=performer,
        album=album,
        duration_sec=audio_doc.get("duration_sec"),
    )

    payload = {
        "telegram.file_id": file_id,
        "telegram.file_unique_id": media_file_unique_id,
        "telegram.mime_type": _normalize_mime_type(getattr(media, "mime_type", None), file_name),
        "telegram.file_size": file_size,
        "audio": audio_doc,
        "spotify": spotify,
        "content_hash": content_hash,
        "fingerprint": fingerprint,
        "topic_id": source_meta.get("topic_id"),
        "topic_name": source_meta.get("topic_name"),
        "cache_chat_id": source_meta.get("cache_chat_id"),
        "cache_message_id": source_meta.get("cache_message_id"),
        "updated_at": time.time(),
    }
    if primary_uid_key and file_id:
        payload[f"telegram.file_ids.{primary_uid_key}"] = str(file_id)

    col = db_handler.audio_collection

    duplicate = None
    if not duplicate and content_hash:
        duplicate = await col.find_document(
            {"content_hash": content_hash}, projection={"_id": 1}
        )
    if not duplicate and fingerprint:
        duplicate = await col.find_document(
            {"fingerprint": fingerprint}, projection={"_id": 1}
        )

    target_id = (
        duplicate["_id"] if (duplicate and duplicate.get("_id")) else file_unique_id
    )

    ensure_source = {}
    existing = await col.read_document(
        target_id,
        projection={
            "_id": 1,
            "source_chat_id": 1,
            "source_message_id": 1,
            "topic_id": 1,
            "topic_name": 1,
            "cache_chat_id": 1,
            "cache_message_id": 1,
            "created_at": 1,
            "updated_at": 1,
        },
    )
    if (
        not existing
        or existing.get("source_chat_id") is None
        or existing.get("source_message_id") is None
    ):
        ensure_source = {
            "source_chat_id": source_meta.get("source_chat_id"),
            "source_message_id": source_meta.get("source_message_id"),
        }
    if not existing or existing.get("topic_id") is None:
        ensure_source["topic_id"] = source_meta.get("topic_id")
    if not existing or not existing.get("topic_name"):
        ensure_source["topic_name"] = source_meta.get("topic_name")
    if not existing or existing.get("cache_chat_id") is None:
        ensure_source["cache_chat_id"] = source_meta.get("cache_chat_id")
    if not existing or existing.get("cache_message_id") is None:
        ensure_source["cache_message_id"] = source_meta.get("cache_message_id")

    now_ts = time.time()
    set_fields = {
        **{k: v for k, v in payload.items() if v is not None},
        **ensure_source,
        "enriched": False,
        "enriching": True,
        "updated_at": now_ts,
    }
    if existing and not existing.get("created_at"):
        set_fields["created_at"] = existing.get("updated_at") or now_ts

    try:
        await col.update_one({"_id": target_id}, {"$set": set_fields, "$setOnInsert": {"created_at": now_ts}}, upsert=True)
    except DuplicateKeyError:
        dup = None
        if content_hash:
            try:
                dup = await col.find_document(
                    {"content_hash": content_hash}, projection={"_id": 1}
                )
            except Exception:
                dup = None
        if not dup and fingerprint:
            try:
                dup = await col.find_document(
                    {"fingerprint": fingerprint}, projection={"_id": 1}
                )
            except Exception:
                dup = None
        if dup and dup.get("_id"):
            target_id = dup["_id"]
            ensure_source2 = {}
            try:
                existing3 = await col.read_document(
                    target_id,
                    projection={
                        "_id": 1,
                        "source_chat_id": 1,
                        "source_message_id": 1,
                        "topic_id": 1,
                        "topic_name": 1,
                        "cache_chat_id": 1,
                        "cache_message_id": 1,
                        "created_at": 1,
                        "updated_at": 1,
                    },
                )
            except Exception:
                existing3 = None
            if (
                not existing3
                or existing3.get("source_chat_id") is None
                or existing3.get("source_message_id") is None
            ):
                ensure_source2 = {
                    "source_chat_id": source_meta.get("source_chat_id"),
                    "source_message_id": source_meta.get("source_message_id"),
                }
            if not existing3 or existing3.get("topic_id") is None:
                ensure_source2["topic_id"] = source_meta.get("topic_id")
            if not existing3 or not existing3.get("topic_name"):
                ensure_source2["topic_name"] = source_meta.get("topic_name")
            if not existing3 or existing3.get("cache_chat_id") is None:
                ensure_source2["cache_chat_id"] = source_meta.get("cache_chat_id")
            if not existing3 or existing3.get("cache_message_id") is None:
                ensure_source2["cache_message_id"] = source_meta.get("cache_message_id")
            if existing3 and existing3.get("created_at"):
                set_fields.pop("created_at", None)
            elif existing3 and not existing3.get("created_at"):
                set_fields["created_at"] = existing3.get("updated_at") or now_ts
            await col.update_one(
                {"_id": target_id},
                {"$set": {**set_fields, **ensure_source2}},
                upsert=False,
            )
            if target_id != file_unique_id:
                try:
                    await col.delete_document(file_unique_id)
                except Exception:
                    pass
        else:
            raise

    if target_id != file_unique_id:
        try:
            await col.delete_document(file_unique_id)
        except Exception:
            pass

    if bool(getattr(Config, "DEBUG", False)):
        LOG.debug(f"[mongo] upserted audio doc _id={target_id!r}")

    try:
        lyrics_enabled = bool(getattr(Config, "MUSIXMATCH", False)) or bool(
            getattr(Config, "LRCLIB", True)
        )
        if lyrics_enabled:
            existing2 = None
            try:
                existing2 = await col.read_document(target_id, projection={"lyrics": 1})
            except Exception:
                existing2 = None
            existing_lyrics = (existing2 or {}).get("lyrics")
            has_lyrics = isinstance(existing_lyrics, str) and existing_lyrics.strip()
            if not has_lyrics:
                from Api.services.lyrics_service import get_track_lyrics

                try:
                    lyrics_result = await asyncio.wait_for(get_track_lyrics(target_id), timeout=5.0)
                except Exception as le:
                    lyrics_result = {"ok": False, "error": str(le)}
    except Exception as e:
        LOG.warning(f"[index] lyrics fetch failed for {target_id!r}: {e}")

    now = time.time()
    await col.update_one(
        {"_id": target_id},
        {
            "$set": {"enriched": True, "enriched_at": now, "updated_at": now},
            "$unset": {
                "enriching": "",
                "enrichment_started_at": "",
                "enrichment_error": "",
                "enrichment_error_at": "",
                "enrich_retry_after": "",
            },
        },
        upsert=False,
    )
    if bool(getattr(Config, "DEBUG", False)):
        LOG.debug(
            f"[index] done file_unique_id={file_unique_id!r} target_id={target_id!r}"
        )


def _audio_ingest_filter():
    """Build the source filter for audio and document messages.
    Dynamic source checks (mode 0 channel only, mode 1 anyone, mode 2 hybrid allowlist,
    and strict ban enforcement) are executed in channel_audio_filter via is_message_allowed().
    """
    return filters.audio | filters.document


@bot.on_message(_audio_ingest_filter())
async def channel_audio_filter(_, message: Message):
    try:
        from stream.core.source_filter import is_message_allowed

        allowed, reason = await is_message_allowed(message)
        if not allowed:
            LOG.debug(
                f"[ingest] Ignored audio message {getattr(message, 'id', None)} "
                f"from chat={getattr(getattr(message, 'chat', None), 'id', None)}: {reason}"
            )
            return

        key = f"{message.chat.id}:{message.id}"
        async with _INDEX_TASKS_LOCK:
            task = _INDEX_TASKS.get(key)
            if task and not task.done():
                return
            media = _pick_audio_media(message)
            if not media:
                return
            await _upsert_minimal(message, media, enriching=True)
            fid_key = f"fid:{message.chat.id}:{message.id}"
            fid_task = _INDEX_TASKS.get(fid_key)
            if not fid_task or fid_task.done():
                _INDEX_TASKS[fid_key] = asyncio.create_task(
                    _sync_file_ids_for_all_clients(
                        source_chat_id=int(message.chat.id),
                        source_message_id=int(message.id),
                    )
                )
            task = asyncio.create_task(_enrich_audio_doc(message, media))
            _INDEX_TASKS[key] = task

        def _done(_t: asyncio.Task):
            try:
                _t.result()
            except Exception as e:
                LOG.warning(
                    f"channel_audio_filter background indexing failed chat={message.chat.id} msg={message.id}: {e}",
                    exc_info=True,
                )

            async def _cleanup():
                async with _INDEX_TASKS_LOCK:
                    current = _INDEX_TASKS.get(key)
                    if current is _t:
                        _INDEX_TASKS.pop(key, None)

            asyncio.create_task(_cleanup())

        task.add_done_callback(_done)
    except Exception as e:
        LOG.warning(
            f"channel_audio_filter failed chat={message.chat.id} msg={message.id}: {e}",
            exc_info=True,
        )


@bot.on_deleted_messages(group=4)
async def deleted_messages_handler(_, messages):
    try:
        now = time.time()
        pairs: list[tuple[int, int]] = []
        for m in messages or []:
            chat = getattr(m, "chat", None)
            chat_id = (
                getattr(chat, "id", None)
                if chat is not None
                else getattr(m, "chat_id", None)
            )
            msg_id = getattr(m, "id", None)
            if msg_id is None:
                msg_id = getattr(m, "message_id", None)
            try:
                chat_id = int(chat_id) if chat_id is not None else None
                msg_id = int(msg_id) if msg_id is not None else None
            except Exception:
                chat_id = None
                msg_id = None
            if chat_id is None or msg_id is None:
                continue
            pairs.append((chat_id, msg_id))

        if not pairs:
            return

        res = await db_handler.audio_collection.collection.update_many(
            {
                "$or": [
                    {"source_chat_id": cid, "source_message_id": mid}
                    for (cid, mid) in pairs
                ]
                + [
                    {"cache_chat_id": cid, "cache_message_id": mid}
                    for (cid, mid) in pairs
                ]
            },
            {"$set": {"deleted": True, "deleted_at": now, "updated_at": now}},
        )
        if bool(getattr(Config, "DEBUG", False)):
            try:
                LOG.debug(
                    f"deleted_messages_handler marked deleted count={len(pairs)} matched={int(getattr(res, 'matched_count', 0) or 0)} "
                    f"modified={int(getattr(res, 'modified_count', 0) or 0)}"
                )
            except Exception:
                pass
    except Exception as e:
        LOG.warning(f"deleted_messages_handler failed: {e}", exc_info=True)


@bot.on_raw_update(group=4)
async def deleted_messages_raw_handler(_, update, __, ___):
    try:
        from pyrogram.raw.types import UpdateDeleteChannelMessages
    except Exception:
        return

    if not isinstance(update, UpdateDeleteChannelMessages):
        return

    try:
        channel_id = int(getattr(update, "channel_id", 0) or 0)
    except Exception:
        channel_id = 0
    if channel_id <= 0:
        return

    msg_ids = getattr(update, "messages", None) or []
    ids: list[int] = []
    for x in msg_ids:
        try:
            ids.append(int(x))
        except Exception:
            continue
    if not ids:
        return

    chat_id = -1000000000000 - int(channel_id)
    now = time.time()
    try:
        res = await db_handler.audio_collection.collection.update_many(
            {"source_chat_id": int(chat_id), "source_message_id": {"$in": ids}},
            {"$set": {"deleted": True, "deleted_at": now, "updated_at": now}},
        )
    except Exception as e:
        LOG.warning(
            f"deleted_messages_raw_handler mongo update failed: {e}", exc_info=True
        )
        return

    if bool(getattr(Config, "DEBUG", False)):
        try:
            LOG.debug(
                f"deleted_messages_raw_handler chat={chat_id} ids={len(ids)} matched={int(getattr(res, 'matched_count', 0) or 0)} "
                f"modified={int(getattr(res, 'modified_count', 0) or 0)}"
            )
        except Exception:
            pass
