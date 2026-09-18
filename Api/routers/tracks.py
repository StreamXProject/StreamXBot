import datetime
import hashlib
import time
import re
import json
import asyncio
import difflib
from typing import Any, Optional
from urllib.parse import urlparse
import unicodedata

from fastapi import APIRouter, Body, Depends, HTTPException, Query, Request
from fastapi.responses import PlainTextResponse
from pydantic import BaseModel
import httpx
from bs4 import BeautifulSoup

from Api.deps.db import get_audio_tracks_collection
from Api.schemas.browse import BrowseResponse
from Api.schemas.playlists import AvailablePlaylistItem, AvailablePlaylistsResponse
from Api.schemas.track import TrackResponse
from Api.services.lyrics_service import get_track_lyrics
from Api.services.stream_service import download_track, stream_track, warm_track_cached
from Api.services.track_service import (
    attach_liked_to_dicts,
    get_daily_playlist,
    get_daily_playlist_thumbnail_info,
    get_track_by_id,
    get_user_top_played_thumbnail_info,
    random_tracks,
    search_tracks,
)
from Api.utils.auth import get_optional_user_id, require_admin_user_id, require_user_id, verify_auth_token
from stream.core.config_manager import Config
from stream.database.MongoDb import db_handler

router = APIRouter()

from pymongo import UpdateOne

_ALBUM_SLUG_RE = re.compile(r"[^a-z0-9]+", flags=re.I)
_ALBUMS_REFRESH_LOCK = asyncio.Lock()
_ARTISTS_REFRESH_LOCK = asyncio.Lock()
_ARTISTS_LAST_REFRESH_AT: float = 0.0
_ARTISTS_REFRESH_COOLDOWN_SEC: float = 300.0


def _normalize_album_id_part(text: str) -> str:
    raw = (text or "").strip().lower()
    if not raw:
        return ""
    s = raw.replace("÷", " divide ").replace("&", " and ").replace("+", " plus ")
    s = unicodedata.normalize("NFKD", s)
    s = s.encode("ascii", "ignore").decode("ascii")
    s = re.sub(r"[^a-z0-9 ]", " ", s)
    s = re.sub(r"\s+", "_", s.strip())
    s = re.sub(r"_+", "_", s).strip("_")
    if s:
        return s
    h = hashlib.sha1(raw.encode("utf-8")).hexdigest()[:16]
    return f"u_{h}"


def _slugify(value: str) -> str:
    return _normalize_album_id_part(value)


def _coerce_year(value: Any) -> int | None:
    if value is None or isinstance(value, bool):
        return None
    try:
        y = int(value)
    except Exception:
        return None
    if y < 1000 or y > 2100:
        return None
    return y


def _album_id(*, album: str, year: Any = None) -> str:
    b = _normalize_album_id_part(album)
    if not b:
        return ""
    y = _coerce_year(year)
    if y is not None:
        return f"album_{b}_{y}"
    return f"album_{b}"


def _artist_id(name: str) -> str:
    s = _slugify(name)
    if not s:
        return ""
    return f"artist_{s}"


_ARTIST_SPLIT_RE = re.compile(r"\s*(?:,|/|&| and | x | feat\. | feat | ft\. | ft )\s*", flags=re.I)


def _split_artists(value: str) -> list[str]:
    raw = (value or "").strip()
    if not raw:
        return []
    raw = raw.replace("(", " ").replace(")", " ").replace("[", " ").replace("]", " ").strip()
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


def _clean_url(value: Any) -> str:
    if not isinstance(value, str):
        return ""
    s = value.strip()
    if len(s) >= 2 and s[0] == "`" and s[-1] == "`":
        s = s[1:-1].strip()
    return s


class ChannelItem(BaseModel):
    id: int
    title: str | None = None
    username: str | None = None
    type: str | None = None


class ChannelIdsResponse(BaseModel):
    ok: bool = True
    items: list[ChannelItem]


_APPLE_SIZE_RE = re.compile(r"/(\d+)x(\d+)(bb)?\.(jpg|jpeg|png|webp)$", flags=re.I)
_ITUNES_M3U8_RE = re.compile(r"(https://mvod\.itunes\.apple\.com/itunes-assets/[^\\s\"']+?/)(P\\d+)_default\\.m3u8", flags=re.I)


def _resize_apple_image_url(url: Any, *, size: int = 618) -> str | None:
    s = _clean_url(url)
    if not s:
        return None
    try:
        size = int(size)
    except Exception:
        size = 618
    if size <= 0:
        size = 618

    base = s
    suffix = ""
    for sep in ("?", "#"):
        if sep in base:
            base, rest = base.split(sep, 1)
            suffix = sep + rest
            break

    m = _APPLE_SIZE_RE.search(base)
    if not m:
        return s
    bb = m.group(3) or ""
    ext = m.group(4)
    replaced = _APPLE_SIZE_RE.sub(f"/{size}x{size}{bb}.{ext}", base)
    return replaced + suffix


def _itunes_mp4_from_m3u8(url: Any) -> str | None:
    s = _clean_url(url)
    if not s:
        return None
    m = _ITUNES_M3U8_RE.search(s)
    if not m:
        return None
    base = m.group(1)
    pid = m.group(2)
    return f"{base}{pid}_Anull_video_gr697_sdr_3840x2160-.mp4"


def _am_artwork_url(artwork: Any, *, size: int = 1000) -> str | None:
    if not isinstance(artwork, dict):
        return None
    u = artwork.get("url")
    if not isinstance(u, str) or not u.strip():
        return None
    url = u.strip()
    url = url.replace("{w}", str(int(size))).replace("{h}", str(int(size)))
    return url


def _pick_first_str(value: Any) -> str | None:
    if isinstance(value, str) and value.strip():
        return value.strip()
    if isinstance(value, dict):
        for k in ("name", "value", "text", "title", "label", "displayName", "display_name"):
            v = value.get(k)
            if isinstance(v, str) and v.strip():
                return v.strip()
    if isinstance(value, list):
        for v in value:
            if isinstance(v, str) and v.strip():
                return v.strip()
            if isinstance(v, dict):
                picked = _pick_first_str(v)
                if picked:
                    return picked
    return None


def _shazam_artist_slug(name: str) -> str:
    return _slugify(name).replace("_", "-")


def _norm_artist_query(value: str) -> str:
    s = (value or "").strip().lower()
    s = re.sub(r"[^a-z0-9\s]+", " ", s)
    s = re.sub(r"\s+", " ", s).strip()
    return s


def _artist_query_score(*, q: str, name: str) -> float:
    qn = _norm_artist_query(q)
    nn = _norm_artist_query(name)
    if not qn or not nn:
        return 0.0
    if qn == nn:
        return 1000.0
    score = difflib.SequenceMatcher(None, qn, nn).ratio() * 100.0
    if nn.startswith(qn):
        score += 50.0
    if qn in nn:
        score += 25.0
    q_tokens = set(qn.split())
    n_tokens = set(nn.split())
    if q_tokens and n_tokens:
        inter = len(q_tokens & n_tokens)
        union = len(q_tokens | n_tokens)
        score += (inter / union) * 40.0
    return score


def _infer_formed(value: str | None) -> str | None:
    s = (value or "").strip()
    if not s:
        return None
    s2 = s.replace("\u2014", "-").replace("\u2013", "-")
    for pat in (
        r"\bformed\s+(?:in\s+)?(\d{4})\b",
        r"\bbrought together in\s+(\d{4})\b",
        r"\bestablished\s+(?:in\s+)?(\d{4})\b",
        r"\bfounded\s+(?:in\s+)?(\d{4})\b",
        r"\bformed\s+around\s+(\d{4})\b",
    ):
        m = re.search(pat, s2, flags=re.I)
        if m:
            return m.group(1)
    return None


async def _fetch_shazam_json(client: httpx.AsyncClient, url: str, *, params: dict[str, object] | None = None) -> Any:
    r = await client.get(url, params=params)
    r.raise_for_status()
    return r.json()


async def _fetch_shazam_html(client: httpx.AsyncClient, url: str) -> str:
    r = await client.get(url)
    r.raise_for_status()
    return r.text


def _parse_shazam_artist_html(html: str) -> dict[str, Any]:
    out: dict[str, Any] = {}
    try:
        soup = BeautifulSoup(html or "", "html.parser")
    except Exception:
        return out

    scripts = soup.find_all("script", attrs={"type": "application/ld+json"})
    found_ld = False
    for s in scripts:
        try:
            raw = s.string or s.text or ""
            if not raw.strip():
                continue
            data = json.loads(raw)
        except Exception:
            continue

        candidates: list[dict] = []
        if isinstance(data, dict):
            candidates = [data]
        elif isinstance(data, list):
            candidates = [x for x in data if isinstance(x, dict)]

        for obj in candidates:
            t = str(obj.get("@type") or "").strip()
            if t not in ("MusicGroup", "Person", "MusicArtist"):
                continue
            name = _pick_first_str(obj.get("name"))
            if name:
                out["name"] = name
            genres = obj.get("genre")
            if isinstance(genres, list):
                out["genres"] = [str(x).strip() for x in genres if isinstance(x, str) and x.strip()]
            elif isinstance(genres, str) and genres.strip():
                out["genres"] = [genres.strip()]
            desc = _pick_first_str(obj.get("description"))
            if desc:
                out["description"] = desc
            img = obj.get("image")
            if isinstance(img, dict):
                u = _pick_first_str(img.get("url"))
                if u:
                    out["image_url"] = u
            elif isinstance(img, str) and img.strip():
                out["image_url"] = img.strip()
            found_ld = True
            break
        if found_ld:
            break

    try:
        video_block = soup.find(attrs={"data-test-id": "artist_impression_artistVideo"})
        video = None
        if video_block is not None:
            video = video_block.find("video")
        if video is None:
            video = soup.find("video")
        if video is not None:
            poster = video.get("poster")
            if isinstance(poster, str) and poster.strip():
                out["video_poster_url"] = poster.strip()

        search_html = str(video_block) if video_block is not None else (html or "")
        idx = search_html.lower().find(".m3u8")
        if idx != -1:
            start = search_html.rfind("https://", 0, idx)
            if start != -1:
                end = search_html.find(".m3u8", start) + 5
                if end > start:
                    out["video_hls_url"] = search_html[start:end]
        if not out.get("video_hls_url"):
            full_html = html or ""
            idx2 = full_html.lower().find(".m3u8")
            if idx2 != -1:
                start2 = full_html.rfind("https://", 0, idx2)
                if start2 != -1:
                    end2 = full_html.find(".m3u8", start2) + 5
                    if end2 > start2:
                        out["video_hls_url"] = full_html[start2:end2]
        if out.get("video_hls_url"):
            mp4 = _itunes_mp4_from_m3u8(out.get("video_hls_url"))
            if mp4:
                out["video_mp4_url"] = mp4
    except Exception:
        pass

    try:
        members: list[dict[str, str]] = []
        seen: set[str] = set()
        for a in soup.find_all("a", attrs={"data-test-id": re.compile(r"artist_userevent_artistBandMembers", flags=re.I)}):
            name = a.get_text(" ", strip=True)
            href = a.get("href")
            if not isinstance(name, str) or not name.strip():
                continue
            if not isinstance(href, str) or not href.strip():
                continue
            key = f"{name.casefold()}|{href}"
            if key in seen:
                continue
            seen.add(key)
            mid = None
            try:
                m = re.search(r"/(\\d+)$", href.strip())
                if m:
                    mid = m.group(1)
            except Exception:
                mid = None
            item = {"name": name.strip(), "href": href.strip()}
            if mid:
                item["id"] = str(mid)
            members.append(item)
        if members:
            out["members"] = members
    except Exception:
        pass

    try:
        member_of: list[dict[str, str]] = []
        seen2: set[str] = set()
        for a in soup.find_all("a", attrs={"data-test-id": re.compile(r"artist_userevent_memberOfArtistItem", flags=re.I)}):
            name = a.get_text(" ", strip=True)
            href = a.get("href")
            if not isinstance(name, str) or not name.strip():
                continue
            if not isinstance(href, str) or not href.strip():
                continue
            key = f"{name.casefold()}|{href}"
            if key in seen2:
                continue
            seen2.add(key)
            member_of.append({"name": name.strip(), "href": href.strip()})
        if member_of:
            out["member_of"] = member_of
    except Exception:
        pass

    try:
        links: list[dict[str, str]] = []
        seen3: set[str] = set()
        for a in soup.find_all("a", href=True):
            dt = a.get("data-test-id")
            if not isinstance(dt, str) or "artistsociallink" not in dt.casefold():
                continue

            href = a.get("href")
            if not isinstance(href, str) or not href.strip():
                continue
            href = _clean_url(href)
            if not href:
                continue

            if href in seen3:
                continue
            seen3.add(href)

            platform = ""
            try:
                host = (urlparse(href).netloc or "").lower()
                if host.startswith("www."):
                    host = host[4:]
                if host:
                    if "instagram" in host:
                        platform = "instagram"
                    elif host in {"x.com"} or "twitter" in host:
                        platform = "x"
                    elif "facebook" in host:
                        platform = "facebook"
                    elif "youtube" in host or host in {"youtu.be"}:
                        platform = "youtube"
                    elif "tiktok" in host:
                        platform = "tiktok"
                    elif "soundcloud" in host:
                        platform = "soundcloud"
                    elif "spotify" in host:
                        platform = "spotify"
                    else:
                        platform = host
            except Exception:
                platform = ""

            item: dict[str, str] = {"href": href}
            if platform:
                item["platform"] = platform
            links.append(item)
            if len(links) >= 25:
                break
        if links:
            out["links"] = links
    except Exception:
        pass

    try:
        lines = [ln.strip() for ln in soup.get_text("\n", strip=True).splitlines() if ln and ln.strip()]
        for i, ln in enumerate(lines):
            if ln.casefold() == "hometown":
                if i + 1 < len(lines):
                    out.setdefault("hometown", lines[i + 1])
            if ln.casefold() == "born":
                if i + 1 < len(lines):
                    out.setdefault("born", lines[i + 1])
            if ln.casefold() == "formed":
                if i + 1 < len(lines):
                    out.setdefault("formed", lines[i + 1])
    except Exception:
        pass

    return out


def _parse_shazam_artist_detail_json(data: Any) -> dict[str, Any]:
    out: dict[str, Any] = {}
    if isinstance(data, dict) and isinstance(data.get("data"), list) and data["data"]:
        node = data["data"][0]
    elif isinstance(data, dict) and isinstance(data.get("data"), dict):
        node = data["data"]
    else:
        node = None

    if not isinstance(node, dict):
        return out

    out["id"] = _pick_first_str(node.get("id"))
    out["href"] = _pick_first_str(node.get("href"))
    attrs = node.get("attributes") if isinstance(node.get("attributes"), dict) else {}
    if isinstance(attrs, dict):
        name = _pick_first_str(attrs.get("name"))
        if name:
            out["name"] = name
        genres = attrs.get("genreNames")
        if isinstance(genres, list):
            out["genres"] = [str(x).strip() for x in genres if isinstance(x, str) and x.strip()]
        desc = None
        notes = attrs.get("editorialNotes") if isinstance(attrs.get("editorialNotes"), dict) else {}
        if isinstance(notes, dict):
            desc = _pick_first_str(notes.get("standard")) or _pick_first_str(notes.get("short"))
        if desc:
            out["description"] = desc
        born = _pick_first_str(attrs.get("dateOfBirth")) or _pick_first_str(attrs.get("born")) or _pick_first_str(attrs.get("birthDate"))
        if born:
            out["born"] = born
        formed = (
            _pick_first_str(attrs.get("bornOrFormed"))
            or _pick_first_str(attrs.get("born_or_formed"))
            or _pick_first_str(attrs.get("formed"))
            or _pick_first_str(attrs.get("formedOn"))
        )
        if formed:
            out["formed"] = formed
        hometown = (
            _pick_first_str(attrs.get("origin"))
            or _pick_first_str(attrs.get("homeTown"))
            or _pick_first_str(attrs.get("hometown"))
            or _pick_first_str(attrs.get("birthPlace"))
            or _pick_first_str(attrs.get("location"))
        )
        if hometown:
            out["hometown"] = hometown
        img = _am_artwork_url(attrs.get("artwork")) or _am_artwork_url(attrs.get("editorialArtwork"))
        if img:
            out["image_url"] = img

    return out


async def _refresh_albums_cache(*, limit_albums: int = 2000) -> dict[str, int]:
    if _ALBUMS_REFRESH_LOCK.locked():
        return {"processed": 0, "upserted": 0}
    async with _ALBUMS_REFRESH_LOCK:
        limit_albums = int(limit_albums)
        if limit_albums <= 0:
            limit_albums = 2000
        if limit_albums > 5000:
            limit_albums = 5000

        tracks_col = get_audio_tracks_collection()
        pipeline = [
            {
                "$match": {
                    "deleted": {"$ne": True},
                    "audio.album_id": {"$exists": True, "$ne": ""},
                    "$or": [{"audio.album": {"$exists": True, "$ne": ""}}, {"audio.title": {"$exists": True, "$ne": ""}}],
                }
            },
            {
                "$addFields": {
                    "_aid": "$audio.album_id",
                    "_album_title": {"$ifNull": ["$audio.album", "$audio.title"]},
                    "_album_artist": {
                        "$cond": [
                            {"$and": [{"$isArray": "$audio.artists"}, {"$gt": [{"$size": "$audio.artists"}, 0]}]},
                            {"$arrayElemAt": ["$audio.artists", 0]},
                            {"$ifNull": ["$audio.artist", "$audio.performer"]},
                        ]
                    },
                }
            },
            {
                "$addFields": {
                    "_album_norm": {"$toLower": {"$trim": {"input": "$_album_title"}}},
                    "_artist_norm": {"$toLower": {"$trim": {"input": "$_album_artist"}}},
                }
            },
            {"$sort": {"updated_at": -1}},
            {
                "$group": {
                    "_id": "$_aid",
                    "title": {"$first": "$_album_title"},
                    "artist": {"$first": "$_album_artist"},
                    "cover_url": {"$first": {"$ifNull": ["$spotify.big_cover_url", "$spotify.cover_url"]}},
                    "tracks_count": {"$sum": 1},
                    "duration_total": {"$sum": {"$ifNull": ["$audio.duration_sec", 0]}},
                    "updated_at": {"$max": "$updated_at"},
                    "match_album": {"$first": "$_album_norm"},
                    "match_artist": {"$first": "$_artist_norm"},
                }
            },
            {"$sort": {"updated_at": -1}},
            {"$limit": int(limit_albums)},
        ]
        cur = await tracks_col.aggregate(pipeline, allowDiskUse=True)

        albums_col = db_handler.get_collection("albums").collection
        now = time.time()
        upserted = 0
        processed = 0
        async for row in cur:
            processed += 1
            if not isinstance(row, dict):
                continue
            aid = (row.get("_id") or "").strip()
            title = (row.get("title") or "").strip()
            artist = (row.get("artist") or "").strip()
            artists = _split_artists(artist) if artist else []
            cover_url = _clean_url(row.get("cover_url"))
            tracks_count = int(row.get("tracks_count") or 0)
            duration_total = int(row.get("duration_total") or 0)
            updated_at = float(row.get("updated_at") or 0.0)
            match_album = (row.get("match_album") or "").strip()
            match_artist = (row.get("match_artist") or "").strip()

            if not aid or not title or not match_album:
                continue

            res = await albums_col.update_one(
                {"_id": aid},
                {
                    "$setOnInsert": {"created_at": now},
                    "$set": {
                        "title": title,
                        "artist": artist or None,
                        "artists": artists if artists else None,
                        "cover_url": cover_url or None,
                        "tracks_count": tracks_count,
                        "duration_total": duration_total,
                        "match_album": match_album,
                        "match_artist": match_artist or None,
                        "updated_at": updated_at or now,
                    },
                },
                upsert=True,
            )
            if getattr(res, "upserted_id", None) is not None:
                upserted += 1

        return {"processed": processed, "upserted": upserted}


def _is_default_deezer_avatar(url: str | None) -> bool:
    """Return True if the URL is the empty/default Deezer artist avatar."""
    if not url or not isinstance(url, str):
        return False
    return "cdn-images.dzcdn.net/images/artist//" in url


async def _fetch_artist_avatar(client: httpx.AsyncClient, name: str, track_title: str | None = None) -> str | None:
    if not name or not name.strip():
        return None
    clean_name = name.strip()

    # --- 1. Try Deezer direct artist search ---
    try:
        r = await client.get("https://api.deezer.com/search/artist", params={"q": clean_name}, timeout=2.5)
        if r.status_code == 200:
            data = r.json().get("data", [])
            if data and isinstance(data[0], dict):
                art = data[0]
                pic = art.get("picture_xl") or art.get("picture_big") or art.get("picture_medium")
                if pic and not _is_default_deezer_avatar(pic):
                    return pic
    except Exception:
        pass

    # --- 2. Try Spotify artist search if configured ---
    try:
        from stream.helpers.cover_search import is_spotify_configured, _spotify_get_access_token
        if is_spotify_configured():
            token = await _spotify_get_access_token()
            if token:
                sr = await client.get(
                    "https://api.spotify.com/v1/search",
                    params={"q": clean_name, "type": "artist", "limit": 1},
                    headers={"Authorization": f"Bearer {token}"},
                    timeout=2.5,
                )
                if sr.status_code == 200:
                    artists_res = sr.json().get("artists", {}).get("items", [])
                    if artists_res and isinstance(artists_res[0], dict):
                        images = artists_res[0].get("images", [])
                        if images and isinstance(images[0], dict) and images[0].get("url"):
                            return str(images[0]["url"]).strip()
    except Exception:
        pass

    # --- 3. Try Deezer with sub-parts if compound artist name (e.g. "A & B") ---
    sub_parts = _split_artists(clean_name)
    if len(sub_parts) > 1:
        for sub_a in sub_parts:
            try:
                dr = await client.get("https://api.deezer.com/search/artist", params={"q": sub_a}, timeout=2.0)
                if dr.status_code == 200:
                    ddata = dr.json().get("data", [])
                    if ddata and isinstance(ddata[0], dict):
                        art = ddata[0]
                        pic = art.get("picture_xl") or art.get("picture_big") or art.get("picture_medium")
                        if pic and not _is_default_deezer_avatar(pic):
                            return pic
            except Exception:
                pass

    return None


async def _fill_missing_artist_avatars(items: list[dict], artists_col) -> None:
    needs: list[dict] = []
    for doc in items:
        c_url = doc.get("cover_url")
        if not (isinstance(c_url, str) and c_url.strip() and not _is_default_deezer_avatar(c_url)):
            needs.append(doc)
    if not needs:
        return

    sem = asyncio.Semaphore(6)

    async def _fetch_one(doc: dict, client: httpx.AsyncClient):
        name = doc.get("name") or ""
        if not name:
            return
        try:
            av = await asyncio.wait_for(_fetch_artist_avatar(client, name), timeout=2.5)
        except Exception:
            av = None
        if av:
            doc["cover_url"] = _clean_url(av)
            try:
                await artists_col.update_one({"_id": doc["_id"]}, {"$set": {"cover_url": av}})
            except Exception:
                pass
        else:
            c_url = doc.get("cover_url")
            if isinstance(c_url, str) and c_url.strip():
                doc["cover_url"] = _clean_url(c_url)

    async def _bounded(d: dict, client: httpx.AsyncClient):
        async with sem:
            await _fetch_one(d, client)

    try:
        async with httpx.AsyncClient(
            headers={"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"},
            timeout=4.0,
        ) as http_client:
            await asyncio.gather(*[_bounded(d, http_client) for d in needs], return_exceptions=True)
    except Exception:
        pass


async def _refresh_artists_cache(*, limit_tracks: int = 20000, limit_artists: int = 5000) -> dict[str, int]:
    global _ARTISTS_LAST_REFRESH_AT
    async with _ARTISTS_REFRESH_LOCK:
        now = time.time()
        if (now - _ARTISTS_LAST_REFRESH_AT) < 30.0:
            return {"scanned_tracks": 0, "processed_artists": 0, "upserted": 0}

        limit_tracks = int(limit_tracks)
        if limit_tracks <= 0:
            limit_tracks = 20000
        if limit_tracks > 100_000:
            limit_tracks = 100_000

        limit_artists = int(limit_artists)
        if limit_artists <= 0:
            limit_artists = 5000
        if limit_artists > 20_000:
            limit_artists = 20_000

        tracks_col = get_audio_tracks_collection()
        cursor = (
            tracks_col.find(
                {
                    "deleted": {"$ne": True},
                    "$or": [{"audio.artist": {"$exists": True, "$ne": ""}}, {"audio.performer": {"$exists": True, "$ne": ""}}],
                },
                {"audio.artist": 1, "audio.performer": 1, "audio.artists": 1, "updated_at": 1},
                allow_disk_use=True,
            )
            .sort([("updated_at", -1)])
            .limit(int(limit_tracks))
        )

        by_key: dict[str, dict[str, object]] = {}
        scanned = 0
        for_limit = 0
        async for doc in cursor:
            scanned += 1
            audio = doc.get("audio") if isinstance(doc.get("audio"), dict) else {}
            raw_artists = audio.get("artists")
            artists: list[str] = []
            if isinstance(raw_artists, list):
                for a in raw_artists:
                    if isinstance(a, str) and a.strip():
                        artists.extend(_split_artists(a.strip()))
            if not artists:
                raw_artist = audio.get("artist") or audio.get("performer") or ""
                raw_artist = raw_artist.strip() if isinstance(raw_artist, str) else ""
                if not raw_artist:
                    continue
                artists = _split_artists(raw_artist)
            updated_at = float(doc.get("updated_at") or 0.0)
            for name in artists:
                name = name.strip()
                if not name:
                    continue
                sub_parts = _split_artists(name) if ("/" in name or "," in name or " & " in name or " feat " in name.lower()) else [name]
                for sub_name in sub_parts:
                    k = sub_name.casefold().strip()
                    if not k:
                        continue
                    entry = by_key.get(k)
                    if entry is None:
                        by_key[k] = {
                            "name": sub_name,
                            "match_artist": k,
                            "tracks_count": 1,
                            "updated_at": updated_at,
                        }
                    else:
                        entry["tracks_count"] = int(entry.get("tracks_count") or 0) + 1
                        prev_updated = float(entry.get("updated_at") or 0.0)
                        if updated_at > prev_updated:
                            entry["updated_at"] = updated_at
            if len(by_key) >= limit_artists:
                for_limit += 1
                if for_limit >= 250:
                    break

        artists_col = db_handler.get_collection("artists").collection
        try:
            await artists_col.delete_many({
                "$or": [
                    {"name": {"$regex": "/|,|&| feat| ft | featuring ", "$options": "i"}},
                    {"_id": {"$regex": "/|,|&| feat| ft | featuring ", "$options": "i"}},
                    {"match_artist": {"$regex": "/|,|&| feat| ft | featuring ", "$options": "i"}},
                ]
            })
        except Exception:
            pass

        now = time.time()
        upserted = 0
        processed = 0
        bulk_ops = []

        for k, entry in sorted(by_key.items(), key=lambda kv: float(kv[1].get("updated_at") or 0.0), reverse=True)[:limit_artists]:
            processed += 1
            name = (entry.get("name") or "").strip()
            match_artist = (entry.get("match_artist") or "").strip()
            if not name or not match_artist:
                continue
            aid = _artist_id(name)
            if not aid:
                continue

            bulk_ops.append(
                UpdateOne(
                    {"_id": aid},
                    {
                        "$setOnInsert": {"created_at": now, "followers": 0},
                        "$set": {
                            "name": name,
                            "tracks_count": int(entry.get("tracks_count") or 0),
                            "match_artist": match_artist,
                            "updated_at": float(entry.get("updated_at") or now),
                        },
                    },
                    upsert=True,
                )
            )
            if len(bulk_ops) >= 500:
                try:
                    res = await artists_col.bulk_write(bulk_ops, ordered=False)
                    upserted += getattr(res, "upserted_count", 0)
                except Exception:
                    pass
                bulk_ops.clear()

        if bulk_ops:
            try:
                res = await artists_col.bulk_write(bulk_ops, ordered=False)
                upserted += getattr(res, "upserted_count", 0)
            except Exception:
                pass

        _ARTISTS_LAST_REFRESH_AT = time.time()
        return {"scanned_tracks": scanned, "processed_artists": processed, "upserted": upserted}



def _optional_user_id(request: Request) -> int | None:
    token = ""
    auth = (request.headers.get("authorization") or request.headers.get("Authorization") or "").strip()
    if auth.lower().startswith("bearer "):
        token = auth[7:].strip()
    if not token:
        token = (request.headers.get("X-Auth-Token") or request.headers.get("x-auth-token") or "").strip()
    if not token:
        token = (request.cookies.get("auth_token") or "").strip()
    if not token:
        token = (request.cookies.get("token") or "").strip()
    if not token:
        token = (request.query_params.get("auth_token") or "").strip()
    if not token:
        token = (request.query_params.get("token") or "").strip()
    if not token:
        return None
    try:
        payload = verify_auth_token(token)
        uid = int(payload.get("uid") or 0)
        return uid if uid > 0 else None
    except Exception:
        return None


async def _has_daily_playlist_tracks(*, key: str) -> bool:
    k = (key or "").strip().lower()
    now = float(time.time())

    if k in {"random", "mix", "daily", "daily-playlist"}:
        col = get_audio_tracks_collection()
        return bool(await col.find_one({}, {"_id": 1}))

    if k in {"top", "top-played", "top-playlist"}:
        col = db_handler.globalplayback_collection.collection
        return bool(await col.find_one({}, {"_id": 1}))

    if k in {"surprise", "surprise-me"}:
        col = db_handler.globalplayback_collection.collection
        return bool(await col.find_one({}, {"_id": 1}))

    if k in {"rediscover"}:
        cutoff = now - 30 * 24 * 3600
        col = db_handler.globalplayback_collection.collection
        return bool(await col.find_one({"last_played_at": {"$lt": cutoff}}, {"_id": 1}))

    if k in {"trending", "trending-today"}:
        since = now - 24 * 3600
        col = db_handler.userplayback_collection.collection
        return bool(await col.find_one({"played_at": {"$gte": since}}, {"_id": 1}))

    if k in {"rising", "rising-tracks"}:
        since = now - 3 * 24 * 3600
        col = db_handler.userplayback_collection.collection
        return bool(await col.find_one({"played_at": {"$gte": since}}, {"_id": 1}))

    if k in {"late-night", "late-night-mix", "night"}:
        since = now - 30 * 24 * 3600
        col = db_handler.userplayback_collection.collection
        cur = await col.aggregate(
            [
                {"$match": {"played_at": {"$gte": since}}},
                {"$addFields": {"_dt": {"$toDate": {"$multiply": ["$played_at", 1000]}}}},
                {"$addFields": {"_hour": {"$hour": "$_dt"}}},
                {"$match": {"$or": [{"_hour": {"$gte": 22}}, {"_hour": {"$lte": 3}}]}},
                {"$limit": 1},
                {"$project": {"_id": 1}},
            ]
        )
        rows = await cur.to_list(length=1)
        return bool(rows)

    return False


class ArtistSearchItem(BaseModel):
    id: str
    name: str
    href: str | None = None
    image_url: str | None = None
    video_poster_url: str | None = None
    video_hls_url: str | None = None
    video_mp4_url: str | None = None
    genres: list[str] | None = None
    description: str | None = None
    hometown: str | None = None
    born: str | None = None
    formed: str | None = None
    links: list[dict[str, str]] | None = None
    members: list[dict[str, str]] | None = None
    member_of: list[dict[str, str]] | None = None
    source: str = "shazam"


class ArtistSearchResponse(BaseModel):
    ok: bool = True
    q: str
    country: str
    items: list[ArtistSearchItem]


class ArtistLookupResponse(BaseModel):
    ok: bool = True
    id: str
    country: str
    item: ArtistSearchItem | None = None


def _extract_artist_search_items(payload: Any) -> list[dict]:
    if isinstance(payload, dict):
        results = payload.get("results")
        if isinstance(results, dict):
            artists = results.get("artists")
            if isinstance(artists, dict) and isinstance(artists.get("data"), list):
                return [x for x in artists.get("data") if isinstance(x, dict)]

        if isinstance(payload.get("data"), list):
            return [x for x in payload.get("data") if isinstance(x, dict)]

        for v in payload.values():
            items = _extract_artist_search_items(v)
            if items:
                return items
    elif isinstance(payload, list):
        for v in payload:
            items = _extract_artist_search_items(v)
            if items:
                return items
    return []


@router.get("/search", response_model=BrowseResponse)
async def search(
    q: str | None = Query(default=None, min_length=0, max_length=80),
    query: str | None = Query(default=None, min_length=0, max_length=80),
    channel_id: int | None = Query(default=None),
    page: int = Query(default=1, ge=1),
    limit: int = Query(default=20, ge=1, le=50),
    user_id: Optional[int] = Depends(get_optional_user_id),
):
    raw = (q or "").strip()
    if not raw:
        raw = (query or "").strip()
    if not raw:
        return BrowseResponse(page=int(page), per_page=int(limit), total=0, items=[])
    return await search_tracks(raw, channel_id=channel_id, page=int(page), per_page=int(limit), user_id=user_id)

@router.get("/tracks/search", response_model=BrowseResponse)
async def track_search(
    q: str = Query(default="", min_length=0, max_length=80),
    channel_id: int | None = Query(default=None),
    page: int = Query(default=1, ge=1),
    limit: int = Query(default=20, ge=1, le=50),
    user_id: Optional[int] = Depends(get_optional_user_id),
):
    q = (q or "").strip()
    if not q:
        return BrowseResponse(page=int(page), per_page=int(limit), total=0, items=[])
    return await search_tracks(q, channel_id=channel_id, page=int(page), per_page=int(limit), user_id=user_id)


@router.get("/search/artist/{artist_name}")
@router.get("/search/artist")
@router.get("/search/artists")
async def search_artists_db(
    artist_name: str = "",
    q: str = "",
    page: int = Query(default=1, ge=1),
    limit: int = Query(default=50, ge=1, le=200),
    user_id: Optional[int] = Depends(get_optional_user_id),
):
    query_str = (artist_name or q or "").strip()
    artists_col = db_handler.get_collection("artists").collection
    filter_query: dict[str, Any] = {}

    if query_str:
        escaped_query = re.escape(query_str)
        filter_query = {
            "$or": [
                {"_id": {"$regex": escaped_query, "$options": "i"}},
                {"name": {"$regex": escaped_query, "$options": "i"}},
                {"match_artist": {"$regex": escaped_query, "$options": "i"}},
            ]
        }

    total = await artists_col.count_documents(filter_query)
    skip = (int(page) - 1) * int(limit)
    cursor = (
        artists_col.find(filter_query, {"match_artist": 0})
        .sort([("followers", -1), ("updated_at", -1)])
        .skip(int(skip))
        .limit(int(limit))
    )

    followed_ids: set[str] = set()
    if user_id is not None:
        try:
            fav_col = db_handler.get_collection("user_favourite_artists").collection
            async for fdoc in fav_col.find({"user_id": int(user_id)}, {"_id": 0, "artist_id": 1}):
                aid = fdoc.get("artist_id")
                if aid:
                    followed_ids.add(aid)
        except Exception:
            pass

    items: list[dict] = []
    async for doc in cursor:
        if "_id" in doc:
            doc["_id"] = str(doc["_id"])
        c_url = doc.get("cover_url")
        if isinstance(c_url, str) and c_url.strip() and not _is_default_deezer_avatar(c_url):
            doc["cover_url"] = _clean_url(c_url)
        doc["is_following"] = doc.get("_id") in followed_ids
        items.append(doc)

    await _fill_missing_artist_avatars(items, artists_col)

    return {
        "ok": True,
        "page": int(page),
        "per_page": int(limit),
        "total": int(total),
        "query": query_str,
        "items": items,
    }

@router.get("/tracks/shuffle", response_model=BrowseResponse)
@router.get("/tracks/random", response_model=BrowseResponse)
@router.get("/library/shuffle", response_model=BrowseResponse)
@router.get("/api/v1/library/shuffle", response_model=BrowseResponse)
async def track_shuffle(
    limit: int = Query(default=100, ge=1, le=200),
    seed: int | None = Query(default=None),
    channel_id: int | None = Query(default=None),
    liked: bool | None = Query(default=None),
    genre: str | None = Query(default=None),
    artist: str | None = Query(default=None),
    source: str | None = Query(default=None),
    lossless: bool | None = Query(default=None),
    user_id: Optional[int] = Depends(get_optional_user_id),
):
    return await random_tracks(
        limit=int(limit),
        seed=seed,
        channel_id=channel_id,
        user_id=user_id,
        liked=liked,
        genre=genre,
        artist=artist,
        source=source,
        lossless=lossless,
    )


@router.get("/history", response_model=BrowseResponse)
async def user_history_direct(
    limit: int = Query(default=100, ge=1, le=200),
    user_id: int = Depends(require_user_id),
):
    from Api.routers.playlists import get_user_history
    return await get_user_history(limit=limit, user_id=user_id)

@router.get("/playlists/available", response_model=AvailablePlaylistsResponse)
async def available_playlists(request: Request):
    today = datetime.datetime.utcnow().date().isoformat()
    items: list[AvailablePlaylistItem] = []

    daily_defs = [
        ("random", "Daily Mix"),
        ("trending", "Trending Today"),
        ("rediscover", "Rediscover"),
        ("late-night", "Late Night Mix"),
        ("rising", "Rising Tracks"),
        ("surprise", "Surprise Me"),
    ]
    for key, name in daily_defs:
        if not await _has_daily_playlist_tracks(key=key):
            continue
        info = await get_daily_playlist_thumbnail_info(key=key, date=today, channel_id=None, limit=4)
        url = info.get("cover_url") if isinstance(info, dict) else None
        normal = info.get("normal_thumbnail") if isinstance(info, dict) else None
        thumbs = info.get("thumbnails") if isinstance(info, dict) and isinstance(info.get("thumbnails"), list) else []
        items.append(
            AvailablePlaylistItem(
                id=f"daily:{key}",
                kind="daily",
                name=name,
                thumbnails=[str(x) for x in thumbs if isinstance(x, str)],
                thumbnail_url=url,
                normal_thumbnail=normal,
                endpoint=f"/daily-playlist/{key}",
                requires_auth=False,
            )
        )

    return AvailablePlaylistsResponse(items=items)

@router.get("/daily-playlist/{key}", response_model=BrowseResponse)
async def daily_playlist(
    key: str,
    limit: int = Query(default=75, ge=1, le=75),
    channel_id: int | None = Query(default=None),
    user_id: Optional[int] = Depends(get_optional_user_id),
):
    key = (key or "").strip().lower()
    if not key:
        raise HTTPException(status_code=400, detail="key is required")
    if key not in {
        "random",
        "mix",
        "top",
        "top-played",
        "daily-playlist",
        "top-playlist",
        "trending",
        "trending-today",
        "rediscover",
        "late-night",
        "late-night-mix",
        "rising",
        "rising-tracks",
        "surprise",
        "surprise-me",
    }:
        raise HTTPException(status_code=404, detail="unknown daily playlist")
    return await get_daily_playlist(key=key, date=None, channel_id=channel_id, limit=int(limit), user_id=user_id)


@router.get("/tracks/{track_id}", response_model=TrackResponse)
async def track_details(track_id: str, user_id: Optional[int] = Depends(get_optional_user_id)):
    doc = await get_track_by_id(track_id, user_id=user_id)
    if not doc:
        raise HTTPException(status_code=404, detail="track not found")
    return doc


@router.get("/albums")
async def list_albums(
    page: int = Query(default=1, ge=1),
    limit: int = Query(default=50, ge=1, le=200),
    refresh: bool = Query(default=False),
):
    albums_col = db_handler.get_collection("albums").collection
    try:
        existing = int(await albums_col.estimated_document_count())
    except Exception:
        existing = 0

    tracks_col = get_audio_tracks_collection()
    should_refresh = bool(refresh) or existing <= 0
    if not should_refresh:
        try:
            latest_track = await tracks_col.find_one(
                {"deleted": {"$ne": True}, "audio.album_id": {"$exists": True, "$ne": ""}},
                projection={"updated_at": 1},
                sort=[("updated_at", -1)],
            )
            latest_album = await albums_col.find_one(
                {},
                projection={"updated_at": 1},
                sort=[("updated_at", -1)],
            )
            track_ts = float((latest_track or {}).get("updated_at") or 0.0)
            album_ts = float((latest_album or {}).get("updated_at") or 0.0)
            if track_ts > album_ts:
                should_refresh = True
        except Exception:
            pass

    if should_refresh:
        await _refresh_albums_cache(limit_albums=5000 if refresh else 2000)
        try:
            existing = int(await albums_col.estimated_document_count())
        except Exception:
            existing = 0

    skip = (int(page) - 1) * int(limit)
    cursor = (
        albums_col.find({}, {"match_album": 0, "match_artist": 0})
        .sort([("updated_at", -1)])
        .skip(int(skip))
        .limit(int(limit))
    )
    items: list[dict] = []
    async for doc in cursor:
        if "_id" in doc:
            doc["_id"] = str(doc["_id"])
        if isinstance(doc.get("cover_url"), str):
            doc["cover_url"] = _clean_url(doc.get("cover_url"))
        items.append(doc)
    return {"ok": True, "page": int(page), "per_page": int(limit), "total": int(existing), "items": items}


@router.get("/albums/{album_id}")
async def album_details(album_id: str, user_id: Optional[int] = Depends(get_optional_user_id)):
    aid = (album_id or "").strip()
    if not aid:
        raise HTTPException(status_code=400, detail="album_id is required")

    albums_col = db_handler.get_collection("albums").collection
    album = await albums_col.find_one({"_id": aid})
    if not album:
        await _refresh_albums_cache(limit_albums=5000)
        album = await albums_col.find_one({"_id": aid})
    if not album:
        raise HTTPException(status_code=404, detail="album not found")

    tracks_col = get_audio_tracks_collection()
    pipeline = [
        {"$match": {"deleted": {"$ne": True}, "audio.album_id": str(aid)}},
        {"$sort": {"audio.track_number": 1, "source_message_id": 1, "updated_at": -1}},
        {
            "$project": {
                "_id": 1,
                "source_chat_id": 1,
                "source_message_id": 1,
                "audio": 1,
                "spotify": 1,
                "created_at": 1,
                "updated_at": 1,
            }
        },
    ]
    cur = await tracks_col.aggregate(pipeline)
    tracks: list[dict] = []
    async for doc in cur:
        if "_id" in doc:
            doc["_id"] = str(doc["_id"])
        if not doc.get("created_at") and doc.get("updated_at"):
            doc["created_at"] = doc.get("updated_at")
        spotify = doc.get("spotify") if isinstance(doc.get("spotify"), dict) else {}
        if isinstance(spotify, dict):
            if isinstance(spotify.get("cover_url"), str):
                spotify["cover_url"] = _clean_url(spotify.get("cover_url"))
            if isinstance(spotify.get("url"), str):
                spotify["url"] = _clean_url(spotify.get("url"))
            doc["spotify"] = spotify
        tracks.append(doc)

    await attach_liked_to_dicts(tracks, user_id)
    album["_id"] = str(album.get("_id"))
    if isinstance(album.get("cover_url"), str):
        album["cover_url"] = _clean_url(album.get("cover_url"))
    album.pop("match_album", None)
    album.pop("match_artist", None)
    return {"ok": True, "album": album, "tracks": tracks}


@router.get("/albums/{album_id}/tracks")
async def album_tracks(
    album_id: str,
    page: int = Query(default=1, ge=1),
    limit: int = Query(default=50, ge=1, le=200),
    user_id: Optional[int] = Depends(get_optional_user_id),
):
    aid = (album_id or "").strip()
    if not aid:
        raise HTTPException(status_code=400, detail="album_id is required")

    albums_col = db_handler.get_collection("albums").collection
    album = await albums_col.find_one({"_id": aid})
    if not album:
        raise HTTPException(status_code=404, detail="album not found")

    tracks_col = get_audio_tracks_collection()
    base_pipeline = [
        {"$match": {"deleted": {"$ne": True}, "audio.album_id": str(aid)}},
    ]

    total_cur = await tracks_col.aggregate([*base_pipeline, {"$count": "total"}])
    total_rows = await total_cur.to_list(length=1)
    total = int(total_rows[0]["total"]) if total_rows else 0

    skip = (int(page) - 1) * int(limit)
    data_pipeline = [
        *base_pipeline,
        {"$sort": {"audio.track_number": 1, "source_message_id": 1, "updated_at": -1}},
        {"$skip": int(skip)},
        {"$limit": int(limit)},
        {
            "$project": {
                "_id": 1,
                "source_chat_id": 1,
                "source_message_id": 1,
                "audio": 1,
                "spotify": 1,
                "created_at": 1,
                "updated_at": 1,
            }
        },
    ]
    cur = await tracks_col.aggregate(data_pipeline)
    items: list[dict] = []
    async for doc in cur:
        if "_id" in doc:
            doc["_id"] = str(doc["_id"])
        if not doc.get("created_at") and doc.get("updated_at"):
            doc["created_at"] = doc.get("updated_at")
        spotify = doc.get("spotify") if isinstance(doc.get("spotify"), dict) else {}
        if isinstance(spotify, dict):
            if isinstance(spotify.get("cover_url"), str):
                spotify["cover_url"] = _clean_url(spotify.get("cover_url"))
            if isinstance(spotify.get("url"), str):
                spotify["url"] = _clean_url(spotify.get("url"))
            doc["spotify"] = spotify
        items.append(doc)

    await attach_liked_to_dicts(items, user_id)
    return {"ok": True, "page": int(page), "per_page": int(limit), "total": int(total), "items": items}


@router.get("/artists")
async def list_artists(
    page: int = Query(default=1, ge=1),
    limit: int = Query(default=50, ge=1, le=200),
    refresh: bool = Query(default=False),
    user_id: Optional[int] = Depends(get_optional_user_id),
):
    artists_col = db_handler.get_collection("artists").collection
    try:
        existing = int(await artists_col.estimated_document_count())
    except Exception:
        existing = 0

    tracks_col = get_audio_tracks_collection()
    should_refresh = bool(refresh) or existing <= 0
    now = time.time()
    if not should_refresh and (now - _ARTISTS_LAST_REFRESH_AT) > _ARTISTS_REFRESH_COOLDOWN_SEC:
        try:
            latest_track = await tracks_col.find_one(
                {"deleted": {"$ne": True}},
                projection={"updated_at": 1},
                sort=[("updated_at", -1)],
            )
            latest_artist = await artists_col.find_one(
                {},
                projection={"updated_at": 1},
                sort=[("updated_at", -1)],
            )
            track_ts = float((latest_track or {}).get("updated_at") or 0.0)
            artist_ts = float((latest_artist or {}).get("updated_at") or 0.0)
            if track_ts > artist_ts:
                should_refresh = True
        except Exception:
            pass

    if should_refresh:
        if _ARTISTS_REFRESH_LOCK.locked() and existing > 0:
            pass
        else:
            await _refresh_artists_cache(limit_tracks=100_000 if refresh else 20_000, limit_artists=20_000 if refresh else 5_000)
            try:
                existing = int(await artists_col.estimated_document_count())
            except Exception:
                existing = 0

    followed_ids: set[str] = set()
    if user_id is not None:
        try:
            fav_col = db_handler.get_collection("user_favourite_artists").collection
            async for fdoc in fav_col.find({"user_id": int(user_id)}, {"_id": 0, "artist_id": 1}):
                aid = fdoc.get("artist_id")
                if aid:
                    followed_ids.add(aid)
        except Exception:
            pass

    skip = (int(page) - 1) * int(limit)
    cursor = (
        artists_col.find({}, {"match_artist": 0})
        .sort([("updated_at", -1)])
        .skip(int(skip))
        .limit(int(limit))
    )
    items: list[dict] = []
    async for doc in cursor:
        if "_id" in doc:
            doc["_id"] = str(doc["_id"])
        c_url = doc.get("cover_url")
        if isinstance(c_url, str) and c_url.strip() and not _is_default_deezer_avatar(c_url):
            doc["cover_url"] = _clean_url(c_url)
        doc["is_following"] = doc.get("_id") in followed_ids
        items.append(doc)

    await _fill_missing_artist_avatars(items, artists_col)
    return {"ok": True, "page": int(page), "per_page": int(limit), "total": int(existing), "items": items}


@router.get("/artists/{artist_id}")
async def artist_details(artist_id: str, user_id: Optional[int] = Depends(get_optional_user_id)):
    aid = (artist_id or "").strip()
    if not aid:
        raise HTTPException(status_code=400, detail="artist_id is required")

    artists_col = db_handler.get_collection("artists").collection
    artist = await artists_col.find_one({"_id": aid})
    if not artist:
        await _refresh_artists_cache(limit_tracks=100_000, limit_artists=20_000)
        artist = await artists_col.find_one({"_id": aid})
    if not artist:
        raise HTTPException(status_code=404, detail="artist not found")

    match_artist = (artist.get("match_artist") or "").strip()
    if not match_artist:
        raise HTTPException(status_code=404, detail="artist not ready")

    tracks_col = get_audio_tracks_collection()
    pipeline = [
        {
            "$match": {
                "deleted": {"$ne": True},
                "$or": [{"audio.artist": {"$exists": True, "$ne": ""}}, {"audio.performer": {"$exists": True, "$ne": ""}}],
            }
        },
        {"$addFields": {"_artist_raw": {"$ifNull": ["$audio.artist", "$audio.performer"]}}},
        {"$addFields": {"_artist_norm": {"$toLower": {"$trim": {"input": "$_artist_raw"}}}}},
        {"$match": {"_artist_norm": str(match_artist)}},
        {"$sort": {"created_at": -1, "updated_at": -1}},
        {"$limit": 200},
        {
            "$project": {
                "_id": 1,
                "source_chat_id": 1,
                "source_message_id": 1,
                "audio": 1,
                "spotify": 1,
                "created_at": 1,
                "updated_at": 1,
            }
        },
    ]
    cur = await tracks_col.aggregate(pipeline)
    tracks: list[dict] = []
    track_ids: list[str] = []
    async for doc in cur:
        if "_id" in doc:
            doc["_id"] = str(doc["_id"])
        if not doc.get("created_at") and doc.get("updated_at"):
            doc["created_at"] = doc.get("updated_at")
        tid = str(doc.get("_id") or "").strip()
        if not tid:
            continue
        doc["_id"] = tid
        spotify = doc.get("spotify") if isinstance(doc.get("spotify"), dict) else {}
        if isinstance(spotify, dict):
            if isinstance(spotify.get("cover_url"), str):
                spotify["cover_url"] = _clean_url(spotify.get("cover_url"))
            if isinstance(spotify.get("url"), str):
                spotify["url"] = _clean_url(spotify.get("url"))
            doc["spotify"] = spotify
        tracks.append(doc)
        track_ids.append(tid)

    plays_by_id: dict[str, int] = {}
    try:
        if track_ids:
            gcol = db_handler.globalplayback_collection.collection
            pcur = gcol.find({"_id": {"$in": track_ids}}, {"plays": 1})
            async for row in pcur:
                rid = str(row.get("_id") or "").strip()
                if not rid:
                    continue
                plays_by_id[rid] = int(row.get("plays") or 0)
    except Exception:
        plays_by_id = {}

    def _popular_key(d: dict) -> tuple[int, float]:
        tid = str(d.get("_id") or "")
        plays = int(plays_by_id.get(tid) or 0)
        updated_at = float(d.get("updated_at") or 0.0)
        return (plays, updated_at)

    popular_tracks = sorted(tracks, key=_popular_key, reverse=True)[:10]
    singles = [t for t in tracks if not (t.get("audio") or {}).get("album")][:20]

    await attach_liked_to_dicts(tracks, user_id)
    await attach_liked_to_dicts(popular_tracks, user_id)
    await attach_liked_to_dicts(singles, user_id)

    albums_col = db_handler.get_collection("albums").collection
    releases: list[dict] = []
    try:
        acur = albums_col.find({"match_artist": str(match_artist)}, {"match_album": 0, "match_artist": 0}).sort([("updated_at", -1)]).limit(20)
        async for a in acur:
            if "_id" in a:
                a["_id"] = str(a["_id"])
            if isinstance(a.get("cover_url"), str):
                a["cover_url"] = _clean_url(a.get("cover_url"))
            releases.append(a)
    except Exception:
        releases = []

    artist["_id"] = str(artist.get("_id"))
    c_url = artist.get("cover_url")
    if isinstance(c_url, str) and c_url.strip() and not _is_default_deezer_avatar(c_url):
        artist["cover_url"] = _clean_url(c_url)
    else:
        name = artist.get("name") or ""
        if name:
            try:
                async with httpx.AsyncClient(headers={"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"}, timeout=2.5) as http_client:
                    av = await _fetch_artist_avatar(http_client, name)
                    if av:
                        artist["cover_url"] = _clean_url(av)
                        await artists_col.update_one({"_id": aid}, {"$set": {"cover_url": av}})
            except Exception:
                pass
    artist.pop("match_artist", None)

    return {
        "ok": True,
        "artist": artist,
        "popular_tracks": popular_tracks,
        "releases": releases,
        "singles": singles,
        "tracks": tracks,
    }


@router.get("/artists/{artist_id}/tracks")
async def artist_tracks(
    artist_id: str,
    page: int = Query(default=1, ge=1),
    limit: int = Query(default=50, ge=1, le=200),
    user_id: Optional[int] = Depends(get_optional_user_id),
):
    aid = (artist_id or "").strip()
    if not aid:
        raise HTTPException(status_code=400, detail="artist_id is required")

    artists_col = db_handler.get_collection("artists").collection
    artist = await artists_col.find_one({"_id": aid})
    if not artist:
        raise HTTPException(status_code=404, detail="artist not found")

    match_artist = (artist.get("match_artist") or "").strip()
    if not match_artist:
        raise HTTPException(status_code=404, detail="artist not ready")

    tracks_col = get_audio_tracks_collection()
    base_pipeline = [
        {"$match": {"deleted": {"$ne": True}}},
        {
            "$addFields": {
                "_artists_raw": {
                    "$cond": [
                        {"$isArray": "$audio.artists"},
                        "$audio.artists",
                        [
                            {
                                "$ifNull": [
                                    "$audio.artist",
                                    {"$ifNull": ["$audio.performer", ""]},
                                ]
                            }
                        ],
                    ]
                }
            }
        },
        {
            "$addFields": {
                "_artists_norm": {
                    "$map": {
                        "input": "$_artists_raw",
                        "as": "a",
                        "in": {"$toLower": {"$trim": {"input": "$$a"}}},
                    }
                }
            }
        },
        {"$match": {"_artists_norm": str(match_artist)}},
    ]

    total_cur = await tracks_col.aggregate([*base_pipeline, {"$count": "total"}])
    total_rows = await total_cur.to_list(length=1)
    total = int(total_rows[0]["total"]) if total_rows else 0

    skip = (int(page) - 1) * int(limit)
    data_pipeline = [
        *base_pipeline,
        {"$sort": {"created_at": -1, "updated_at": -1}},
        {"$skip": int(skip)},
        {"$limit": int(limit)},
        {
            "$project": {
                "_id": 1,
                "source_chat_id": 1,
                "source_message_id": 1,
                "audio": 1,
                "spotify": 1,
                "created_at": 1,
                "updated_at": 1,
            }
        },
    ]
    cur = await tracks_col.aggregate(data_pipeline)
    items: list[dict] = []
    async for doc in cur:
        if "_id" in doc:
            doc["_id"] = str(doc["_id"])
        if not doc.get("created_at") and doc.get("updated_at"):
            doc["created_at"] = doc.get("updated_at")
        spotify = doc.get("spotify") if isinstance(doc.get("spotify"), dict) else {}
        if isinstance(spotify, dict):
            if isinstance(spotify.get("cover_url"), str):
                spotify["cover_url"] = _clean_url(spotify.get("cover_url"))
            if isinstance(spotify.get("url"), str):
                spotify["url"] = _clean_url(spotify.get("url"))
            doc["spotify"] = spotify
        items.append(doc)

    await attach_liked_to_dicts(items, user_id)
    return {"ok": True, "page": int(page), "per_page": int(limit), "total": int(total), "items": items}


class AdminDeleteTracksRequest(BaseModel):
    track_id: str | list[str] | None = None
    track_ids: list[str] | None = None


@router.delete("/admin/tracks/delete")
@router.post("/admin/tracks/delete", include_in_schema=False)
async def admin_delete_tracks(
    payload: AdminDeleteTracksRequest | None = Body(None),
    track_id: str | None = Query(None),
    track_ids: list[str] | None = Query(None),
    admin_user_id: int = Depends(require_admin_user_id),
):
    t_ids: list[str] = []
    if payload:
        if isinstance(payload.track_id, list):
            t_ids.extend(payload.track_id)
        elif payload.track_id:
            t_ids.append(payload.track_id)
        if payload.track_ids:
            t_ids.extend(payload.track_ids)
    if track_id:
        t_ids.append(track_id)
    if track_ids:
        t_ids.extend(track_ids)

    track_ids: list[str] = []
    seen: set[str] = set()
    for tid in t_ids:
        tid_str = str(tid or "").strip()
        if not tid_str or tid_str in seen:
            continue
        seen.add(tid_str)
        track_ids.append(tid_str)

    if not track_ids:
        raise HTTPException(status_code=400, detail="track_id or track_ids is required")

    now = time.time()
    col = get_audio_tracks_collection()
    res = await col.update_many(
        {"_id": {"$in": track_ids}},
        {"$set": {"deleted": True, "deleted_at": now, "deleted_by": int(admin_user_id), "updated_at": now}},
    )
    return {
        "ok": True,
        "track_ids": track_ids,
        "matched": int(getattr(res, "matched_count", 0) or 0),
        "modified": int(getattr(res, "modified_count", 0) or 0),
    }

@router.get("/tracks/{track_id}/stream")
async def track_stream(track_id: str, request: Request):
    return await stream_track(track_id, request)


@router.head("/tracks/{track_id}/stream")
async def track_stream_head(track_id: str, request: Request):
    return await stream_track(track_id, request)


@router.get("/channelids", response_model=ChannelIdsResponse)
async def available_channel_ids():
    tracks_col = get_audio_tracks_collection()
    unique_ids_raw = await tracks_col.distinct("source_chat_id", {"deleted": {"$ne": True}})
    
    unique_ids = []
    for cid in unique_ids_raw:
        if cid is None:
            continue
        try:
            unique_ids.append(int(cid))
        except (TypeError, ValueError):
            pass

    if not unique_ids:
        return ChannelIdsResponse(ok=True, items=[])

    channels_col = db_handler.channels_collection.collection
    cursor = channels_col.find({"_id": {"$in": unique_ids}}, {"_id": 1, "title": 1, "username": 1, "type": 1})
    
    channel_details = {}
    async for doc in cursor:
        try:
            cid = int(doc.get("_id"))
            channel_details[cid] = doc
        except Exception:
            continue

    try:
        from stream import bot
    except ImportError:
        bot = None

    items: list[ChannelItem] = []
    for cid in set(unique_ids):
        doc = channel_details.get(cid, {})
        title = doc.get("title")
        username = doc.get("username")
        ctype = doc.get("type")
        
        if not title and bot is not None:
            try:
                chat = await bot.get_chat(cid)
                title = getattr(chat, "title", title)
                username = getattr(chat, "username", username)
                ctype_enum = getattr(chat, "type", None)
                ctype = getattr(ctype_enum, "name", str(ctype_enum)) if ctype_enum else ctype
                
                try:
                    await channels_col.update_one(
                        {"_id": cid},
                        {"$set": {"title": title, "username": username, "type": ctype}},
                        upsert=True
                    )
                except Exception:
                    pass
            except Exception:
                pass

        items.append(
            ChannelItem(
                id=cid,
                title=str(title).strip() if isinstance(title, str) and title.strip() else None,
                username=str(username).strip() if isinstance(username, str) and username.strip() else None,
                type=str(ctype).strip() if isinstance(ctype, str) and ctype.strip() else None,
            )
        )
        
    return ChannelIdsResponse(ok=True, items=items)


@router.get("/tracks/{track_id}/download")
async def track_download(track_id: str, request: Request):
    return await download_track(track_id, request)


@router.get("/tracks/{track_id}/warm")
async def track_warm(track_id: str):
    return await warm_track_cached(track_id)


@router.get("/tracks/{track_id}/lyrics")
async def track_lyrics(track_id: str, request: Request, provider: Optional[str] = Query(None)):
    fmt = (request.query_params.get("format") or "").strip().lower()
    res = await get_track_lyrics(track_id, provider=provider)
    want_json = fmt == "json"
    if not want_json:
        if isinstance(res, dict) and res.get("ok") and isinstance(res.get("lyrics"), str):
            return PlainTextResponse(content=res["lyrics"], media_type="text/plain; charset=utf-8")
    return res
