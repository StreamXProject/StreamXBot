from __future__ import annotations

import asyncio
import base64
import io
import json
import os
import re
import subprocess
import tempfile
import time
from os import path as ospath
from typing import Any
from urllib.parse import quote, urlparse

from aiofiles import open as aiopen
from aiofiles.os import mkdir
from aiofiles.os import path as aiopath
from aiofiles.os import remove as aioremove
from aiohttp import ClientSession
from pyrogram import filters
from pyrogram.types import Message

from stream import bot
from stream.core.config_manager import Config
from stream.helpers.functions import get_readable_bytes
from stream.helpers.hoaders import hoaders_big_cover_url
from stream.helpers.logger import LOGGER

_SPOTIFY_SEARCH_SEMAPHORE = asyncio.Semaphore(3)
_SPOTIFY_SEARCH_CACHE: dict[str, tuple[float, dict | None]] = {}
_SPOTIFY_SEARCH_CACHE_GUARD = asyncio.Lock()
_COV_SEARCH_SEMAPHORE = asyncio.Semaphore(3)
_COV_SEARCH_CACHE: dict[str, tuple[float, dict | None]] = {}
_COV_SEARCH_CACHE_GUARD = asyncio.Lock()
_DOWNLOAD_LOCKS: dict[str, asyncio.Lock] = {}
_DOWNLOAD_LOCKS_GUARD = asyncio.Lock()

MEDIA_DIR = "stream_media"
COVER_NAME = "cover.jpg"

HEADERS = {
    "user-agent": (
        "Mozilla/5.0 (Linux; Android 12) "
        "AppleWebKit/537.36 (KHTML, like Gecko) "
        "Chrome/107.0.0.0 Mobile Safari/537.36"
    )
}

SECTION_EMOJI = {
    "General": "🗒",
    "Video": "🎞",
    "Audio": "🔊",
    "Text": "🔠",
    "Menu": "🗃",
}

LOG = LOGGER(__name__)


async def _get_download_lock(key: str) -> asyncio.Lock:
    async with _DOWNLOAD_LOCKS_GUARD:
        lock = _DOWNLOAD_LOCKS.get(key)
        if lock is None:
            lock = asyncio.Lock()
            _DOWNLOAD_LOCKS[key] = lock
        return lock


def _stream_media_client(message: Message):
    client = getattr(message, "_client", None) or getattr(message, "client", None)
    if client is not None and hasattr(client, "stream_media"):
        return client
    if bot is not None and hasattr(bot, "stream_media"):
        return bot
    raise RuntimeError("No Pyrogram client available for stream_media")


def _sanitize_filename(value: str) -> str:
    value = (value or "").strip().replace("`", "'")
    value = re.sub(r'[\\/:*?"<>|]+', "_", value)
    value = re.sub(r"\s+", " ", value).strip()
    return value[:120] if value else "file"


def _extract_filename_from_url(url: str) -> str:
    try:
        parsed = urlparse(url)
        name = ospath.basename(parsed.path.rstrip("/"))
        return _sanitize_filename(name) if name else "download"
    except Exception:
        return "download"


def _pick_media(msg: Message):
    if not msg:
        return None
    return next(
        (
            m
            for m in (
                msg.document,
                msg.video,
                msg.audio,
                msg.voice,
                msg.animation,
                msg.video_note,
                msg.photo,
            )
            if m is not None
        ),
        None,
    )


def _md_clean(text: str) -> str:
    if not text:
        return ""
    return str(text).replace("`", "'").replace("[", "(").replace("]", ")").strip()


def _parse_mediainfo(output: str) -> dict[str, dict[str, str]]:
    sections: dict[str, dict[str, str]] = {}
    current = ""
    for raw in (output or "").splitlines():
        line = raw.rstrip("\n")
        header = line.strip()
        if header and header.lower() in {
            "general",
            "audio",
            "video",
            "text",
            "image",
            "menu",
        }:
            current = header.lower()
            sections.setdefault(current, {})
            continue
        if not current:
            continue
        if " : " not in line:
            continue
        key, value = line.split(" : ", 1)
        k = key.strip().lower()
        v = value.strip()
        if not k or not v:
            continue
        sections.setdefault(current, {})
        if k not in sections[current]:
            sections[current][k] = v
    return sections


def _is_junk_title(title: str) -> bool:
    t = (title or "").strip().casefold()
    if not t:
        return True
    junk = {
        "core media audio",
        "mpeg audio",
        "iso media file produced by google inc.",
        "lavf",
    }
    for j in junk:
        if j in t:
            return True
    return False


def _pick_best_title(general: dict, audio: dict) -> str:
    t1 = (general.get("title") or "").strip()
    t2 = (audio.get("title") or "").strip()

    if t1 and not _is_junk_title(t1):
        return t1
    if t2 and not _is_junk_title(t2):
        return t2
    return t1 or t2 or ""


def extract_audio_metadata(output: str) -> dict[str, str]:
    sections = _parse_mediainfo(output)
    general = sections.get("general", {})
    audio = sections.get("audio", {})

    title = _pick_best_title(general, audio)
    duration = general.get("duration") or audio.get("duration") or ""
    file_type = audio.get("format") or general.get("format") or ""
    compression = audio.get("compression mode") or general.get("compression mode") or ""
    album = general.get("album") or ""
    performer = general.get("performer") or general.get("album/performer") or ""
    composer = general.get("composer") or ""
    producer = general.get("producer") or ""
    label = general.get("label") or ""
    genre = general.get("genre") or ""
    recorded_date = general.get("recorded date") or general.get("recorded date ") or ""
    bit_depth = audio.get("bit depth") or ""
    bitrate = audio.get("bit rate") or general.get("overall bit rate") or ""
    sampling_rate = audio.get("sampling rate") or ""

    if file_type:
        file_type = file_type.lower()

    return {
        "Title": title,
        "Duration": duration,
        "Type": file_type,
        "Compression": compression,
        "Album": album,
        "Performer": performer,
        "Composer": composer,
        "Producer": producer,
        "Label": label,
        "Genre": genre,
        "Recorded Date": recorded_date,
        "Bit Depth": bit_depth,
        "Bitrate": bitrate,
        "Sampling Rate": sampling_rate,
    }


def _drop_empty(d: dict) -> dict:
    out = {}
    for k, v in (d or {}).items():
        if v is None:
            continue
        if isinstance(v, str) and v.strip() == "":
            continue
        if isinstance(v, dict):
            nested = _drop_empty(v)
            if nested:
                out[k] = nested
            continue
        out[k] = v
    return out


def _digits_compact(value: str) -> str:
    s = (value or "").strip()
    s = re.sub(r"(?<=\d)\s+(?=\d)", "", s)
    return s


def _parse_number(value: str) -> float | None:
    s = _digits_compact(value)
    m = re.search(r"(\d+(?:\.\d+)?)", s)
    if not m:
        return None
    try:
        return float(m.group(1))
    except Exception:
        return None


def _parse_bitrate_kbps(value: str) -> int | None:
    if not value:
        return None
    s = _digits_compact(value).lower()
    n = _parse_number(s)
    if n is None:
        return None
    if "mb/s" in s:
        return int(round(n * 1000))
    if "kb/s" in s:
        return int(round(n))
    if "b/s" in s:
        return int(round(n / 1000))
    return int(round(n))


def _parse_sampling_rate_hz(value: str) -> int | None:
    if not value:
        return None
    s = _digits_compact(value).lower()
    n = _parse_number(s)
    if n is None:
        return None
    if "khz" in s:
        return int(round(n * 1000))
    if "hz" in s:
        return int(round(n))
    return int(round(n))


def _parse_bit_depth(value: str) -> int | None:
    if not value:
        return None
    n = _parse_number(value)
    if n is None:
        return None
    return int(round(n))


def _parse_year(value: str) -> int | None:
    if not value:
        return None
    m = re.search(r"(19\d{2}|20\d{2})", value)
    if not m:
        return None
    try:
        return int(m.group(1))
    except Exception:
        return None


def _parse_duration_seconds(value: str) -> int | None:
    if not value:
        return None
    s = value.lower()
    total = 0
    m = re.search(r"(\d+)\s*h", s)
    if m:
        total += int(m.group(1)) * 3600
    m = re.search(r"(\d+)\s*min", s)
    if m:
        total += int(m.group(1)) * 60
    m = re.search(r"(\d+)\s*s", s)
    if m:
        total += int(m.group(1))
    if total > 0:
        return total
    m = re.search(r"(\d+(?:\.\d+)?)\s*ms", s)
    if m:
        try:
            return int(round(float(m.group(1)) / 1000))
        except Exception:
            return None
    return None


def extract_audio_metadata_normalized(
    output: str,
    duration_sec: int | None = None,
    file_size: int | None = None,
) -> dict:
    sections = _parse_mediainfo(output)
    general = sections.get("general", {})
    audio = sections.get("audio", {})

    title = _pick_best_title(general, audio)
    album = general.get("album") or general.get("original source form/name") or ""
    artist = (
        general.get("performer")
        or general.get("album/performer")
        or general.get("director")
        or general.get("artist")
        or audio.get("performer")
        or ""
    )
    composer = general.get("composer") or ""
    label = general.get("label") or ""
    genre = general.get("genre") or ""
    recorded_date = general.get("recorded date") or ""

    file_type = (audio.get("format") or general.get("format") or "").lower()
    bit_depth = _parse_bit_depth(audio.get("bit depth") or "")
    bitrate_kbps = _parse_bitrate_kbps(
        audio.get("bit rate") or general.get("overall bit rate") or ""
    )
    sampling_rate_hz = _parse_sampling_rate_hz(audio.get("sampling rate") or "")
    year = _parse_year(recorded_date)

    parsed_dur = _parse_duration_seconds(
        general.get("duration") or audio.get("duration") or ""
    )
    if duration_sec is None:
        duration_sec = parsed_dur

    # Check if duration_sec is suspiciously truncated due to partial chunk download
    # (e.g. 2MB partial chunk of a large WAV file or CBR audio)
    if file_size and file_size > 3_000_000:
        calc_dur = None
        if file_type in ("pcm", "wave", "wav") or (sampling_rate_hz and bit_depth):
            sr = sampling_rate_hz or 44100
            bd = bit_depth or 16
            ch_str = (audio.get("channel(s)") or general.get("channel(s)") or "").lower()
            ch = 1 if "1 channel" in ch_str else 2
            byte_rate = (sr * ch * bd) / 8
            if byte_rate > 0:
                calc_dur = int(round((file_size - 44) / byte_rate))
        if not calc_dur and bitrate_kbps and bitrate_kbps > 0:
            calc_dur = int(round((file_size * 8) / (bitrate_kbps * 1000)))

        if calc_dur and (
            duration_sec is None
            or (calc_dur > duration_sec and (duration_sec <= 20 or (calc_dur / max(duration_sec, 1) >= 2)))
        ):
            duration_sec = calc_dur

    return _drop_empty(
        {
            "title": title,
            "album": album,
            "artist": artist,
            "composer": composer,
            "label": label,
            "genre": genre,
            "year": year,
            "duration_sec": duration_sec,
            "type": file_type,
            "bit_depth": bit_depth,
            "bitrate_kbps": bitrate_kbps,
            "sampling_rate_hz": sampling_rate_hz,
        }
    )


def _format_filtered_metadata(output: str) -> str:
    meta = extract_audio_metadata(output)
    lines: list[str] = []
    for k, v in meta.items():
        value = _md_clean(v) if v else "N/A"
        lines.append(f"{k}: `{value}`")
    return "\n".join(lines)


async def get_access_token() -> str:
    global _SPOTIFY_TOKEN, _SPOTIFY_TOKEN_EXPIRES_AT

    now = time.time()
    if _SPOTIFY_TOKEN and now < (_SPOTIFY_TOKEN_EXPIRES_AT - 30):
        return _SPOTIFY_TOKEN

    client_id = (Config.SPOTIFY_CLIENT_ID or "").strip()
    client_secret = (Config.SPOTIFY_CLIENT_SECRET or "").strip()
    if not client_id or not client_secret:
        raise RuntimeError(
            "Spotify credentials are missing (SPOTIFY_CLIENT_ID / SPOTIFY_CLIENT_SECRET)"
        )

    credentials = base64.b64encode(
        f"{client_id}:{client_secret}".encode("utf-8")
    ).decode("utf-8")
    url = "https://accounts.spotify.com/api/token"
    headers = {
        "Authorization": f"Basic {credentials}",
        "Content-Type": "application/x-www-form-urlencoded",
    }
    data = {"grant_type": "client_credentials"}

    async with ClientSession() as session:
        async with session.post(url, headers=headers, data=data) as response:
            payload = await response.json(content_type=None)
            if response.status != 200:
                error = payload.get("error") if isinstance(payload, dict) else payload
                raise RuntimeError(f"Spotify token error: {error}")

    _SPOTIFY_TOKEN = payload.get("access_token")
    expires_in = payload.get("expires_in", 3600)
    if not _SPOTIFY_TOKEN:
        raise RuntimeError("Spotify token missing in response")
    try:
        _SPOTIFY_TOKEN_EXPIRES_AT = now + int(expires_in)
    except Exception:
        _SPOTIFY_TOKEN_EXPIRES_AT = now + 3600

    return _SPOTIFY_TOKEN


async def spotify_search_track(query: str, limit: int = 1) -> dict | None:
    raw = (query or "").strip()
    if not raw:
        return None

    key = f"track|{raw.lower()}|{int(limit)}"
    now = time.time()
    async with _SPOTIFY_SEARCH_CACHE_GUARD:
        cached = _SPOTIFY_SEARCH_CACHE.get(key)
        if cached and now < cached[0]:
            return cached[1]

    async with _SPOTIFY_SEARCH_SEMAPHORE:
        token = await get_access_token()
        q = quote(raw)
        url = f"https://api.spotify.com/v1/search?q={q}&type=track&limit={int(limit)}"
        headers = {"Authorization": f"Bearer {token}"}

        async with ClientSession() as session:
            async with session.get(url, headers=headers) as response:
                payload = await response.json(content_type=None)
                if response.status != 200:
                    error = (
                        payload.get("error") if isinstance(payload, dict) else payload
                    )
                    raise RuntimeError(f"Spotify search error: {error}")

    items = ((payload or {}).get("tracks") or {}).get("items") or []
    track = items[0] if items else None
    async with _SPOTIFY_SEARCH_CACHE_GUARD:
        _SPOTIFY_SEARCH_CACHE[key] = (time.time() + 6 * 3600, track)
    return track


async def spotify_search_tracks(query: str, limit: int = 5) -> list[dict]:
    raw = (query or "").strip()
    if not raw:
        return []

    key = f"tracks|{raw.lower()}|{int(limit)}"
    now = time.time()
    async with _SPOTIFY_SEARCH_CACHE_GUARD:
        cached = _SPOTIFY_SEARCH_CACHE.get(key)
        if cached and now < cached[0]:
            v = cached[1]
            return v if isinstance(v, list) else ([v] if v else [])

    async with _SPOTIFY_SEARCH_SEMAPHORE:
        token = await get_access_token()
        q = quote(raw)
        url = f"https://api.spotify.com/v1/search?q={q}&type=track&limit={int(limit)}"
        headers = {"Authorization": f"Bearer {token}"}

        async with ClientSession() as session:
            async with session.get(url, headers=headers) as response:
                payload = await response.json(content_type=None)
                if response.status != 200:
                    error = (
                        payload.get("error") if isinstance(payload, dict) else payload
                    )
                    raise RuntimeError(f"Spotify search error: {error}")

    items = ((payload or {}).get("tracks") or {}).get("items") or []
    async with _SPOTIFY_SEARCH_CACHE_GUARD:
        _SPOTIFY_SEARCH_CACHE[key] = (time.time() + 6 * 3600, items)
    return items


def get_track_cover_links(track: dict) -> tuple[str | None, list[str]]:
    images = ((track or {}).get("album") or {}).get("images") or []
    normalized: list[tuple[str, int]] = []
    for img in images:
        if not isinstance(img, dict):
            continue
        url = img.get("url")
        if not url:
            continue
        try:
            h = int(img.get("height") or 0)
        except Exception:
            h = 0
        normalized.append((url, h))

    normalized.sort(key=lambda x: x[1], reverse=True)
    urls = [u for (u, _) in normalized]
    if not urls:
        return None, []
    return urls[0], urls


def sanitize_filename(value: str) -> str:
    return _sanitize_filename(value)


def infer_artist_title(file_name: str) -> tuple[str, str]:
    base = ospath.splitext(ospath.basename(file_name or ""))[0].strip()
    base = re.sub(r"\s+", " ", base)
    m = re.match(r"^(?P<artist>.+?)\s*-\s*(?P<title>.+)$", base)
    if not m:
        return "", base
    return (m.group("artist") or "").strip(), (m.group("title") or "").strip()


async def ensure_media_dir() -> str:
    base_dir = ospath.abspath(MEDIA_DIR)

    try:
        if await aiopath.isfile(base_dir):
            base_dir = ospath.abspath(f"{MEDIA_DIR}_dir")
        await asyncio.to_thread(os.makedirs, base_dir, exist_ok=True)
        return base_dir
    except Exception:
        fallback = ospath.join(tempfile.gettempdir(), "stream_media")
        await asyncio.to_thread(os.makedirs, fallback, exist_ok=True)
        return fallback


async def reset_media_session_for_message(client, message: Any) -> None:
    """Safely stop and remove stale MTProto media sessions from a Pyrogram client.
    Prevents infinite TimeoutError loops when a connection to a specific Telegram DC drops.
    """
    if not client:
        return
    try:
        dc_id = None
        from pyrogram.file_id import FileId

        if hasattr(message, "audio") or hasattr(message, "document"):
            media = getattr(message, "audio", None) or getattr(message, "document", None) or getattr(message, "video", None)
            fid_str = getattr(media, "file_id", None) if media else None
            if fid_str:
                decoded = FileId.decode(fid_str)
                dc_id = getattr(decoded, "dc_id", None)
        elif isinstance(message, str):
            decoded = FileId.decode(message)
            dc_id = getattr(decoded, "dc_id", None)
        elif isinstance(message, int):
            dc_id = message

        sessions = getattr(client, "media_sessions", None)
        if isinstance(sessions, dict):
            if dc_id and dc_id in sessions:
                sess = sessions.pop(dc_id, None)
                if sess:
                    try:
                        await sess.stop()
                    except Exception:
                        pass
            elif not dc_id:
                for d, sess in list(sessions.items()):
                    sessions.pop(d, None)
                    if sess:
                        try:
                            await sess.stop()
                        except Exception:
                            pass
    except Exception:
        pass


async def download_partial_media(
    message: Message,
    file_path: str,
    max_bytes: int = 2_000_000,
) -> int:
    media = message.audio
    if (
        not media
        and message.document
        and (message.document.mime_type or "").startswith("audio/")
    ):
        media = message.document

    lock = await _get_download_lock(file_path)
    async with lock:
        async with aiopen(file_path, "wb") as f:
            written = 0
            stream_client = _stream_media_client(message)
            chunk_limit = max(1, (max_bytes + 1024 * 1024 - 1) // (1024 * 1024))
            try:
                async for chunk in stream_client.stream_media(
                    message,
                    limit=chunk_limit,
                ):
                    if not chunk:
                        continue
                    remaining = max_bytes - written
                    if remaining <= 0:
                        break
                    out = chunk[:remaining]
                    await f.write(out)
                    written += len(out)
                    if written >= max_bytes:
                        break
                return written
            except (TimeoutError, OSError, Exception) as e:
                if "TimeoutError" in type(e).__name__ or "GetFile" in str(e):
                    await reset_media_session_for_message(stream_client, message)
                raise


async def download_message_media(
    message: Message,
    file_path: str,
    max_full_size: int = 50_000_000,
    max_prefix_bytes: int = 10_000_000,
    stream_limit: int = 25,
):
    media = message.audio
    if (
        not media
        and message.document
        and (message.document.mime_type or "").startswith("audio/")
    ):
        media = message.document

    size = getattr(media, "file_size", None) if media else None

    if bool(getattr(Config, "DEBUG", False)):
        try:
            cid = int(getattr(getattr(message, "chat", None), "id", 0) or 0)
            mid = int(getattr(message, "id", 0) or 0)
        except Exception:
            cid = 0
            mid = 0
        LOG.debug(
            f"[download] start chat={cid} msg={mid} file_path={ospath.basename(file_path)!r} size={size} "
            f"full_max={int(max_full_size)} prefix_max={int(max_prefix_bytes)}"
        )

    lock = await _get_download_lock(file_path)
    async with lock:
        if size and size <= max_full_size:
            delays = (0.15, 0.4, 0.9)
            for i, delay in enumerate(delays):
                try:
                    await message.download(file_name=file_path)
                    if bool(getattr(Config, "DEBUG", False)):
                        LOG.debug(
                            f"[download] complete mode=full bytes={int(size)} path={ospath.basename(file_path)!r}"
                        )
                    return size
                except PermissionError:
                    if i == len(delays) - 1:
                        raise
                    await asyncio.sleep(delay)
                except (TimeoutError, OSError, Exception) as e:
                    if "TimeoutError" in type(e).__name__ or "GetFile" in str(e):
                        client = _stream_media_client(message)
                        await reset_media_session_for_message(client, message)
                    if i == len(delays) - 1:
                        raise
                    await asyncio.sleep(delay)

    async with aiopen(file_path, "wb") as f:
        written = 0
        stream_client = _stream_media_client(message)
        try:
            async for chunk in stream_client.stream_media(message, limit=stream_limit):
                if not chunk:
                    break
                remaining = max_prefix_bytes - written
                if remaining <= 0:
                    break
                await f.write(chunk[:remaining])
                written += min(len(chunk), remaining)
        except (TimeoutError, OSError, Exception) as e:
            if "TimeoutError" in type(e).__name__ or "GetFile" in str(e):
                await reset_media_session_for_message(stream_client, message)
            raise

    if bool(getattr(Config, "DEBUG", False)):
        LOG.debug(
            f"[download] complete mode=prefix written={int(written)} path={ospath.basename(file_path)!r} "
            f"limit={int(stream_limit)}"
        )
    return size


def best_cover_url(track: dict) -> str | None:
    cover_url, _ = get_track_cover_links(track)
    return cover_url


def _spotify_track_brief(track: dict | None) -> dict:
    if not isinstance(track, dict):
        return {}
    album = track.get("album") if isinstance(track.get("album"), dict) else {}
    artists = track.get("artists") if isinstance(track.get("artists"), list) else []
    artist_names = []
    for a in artists:
        if isinstance(a, dict) and a.get("name"):
            artist_names.append(a.get("name"))
    ext = (
        track.get("external_urls")
        if isinstance(track.get("external_urls"), dict)
        else {}
    )
    return {
        "id": track.get("id"),
        "name": track.get("name"),
        "artists": artist_names,
        "album": album.get("name") if isinstance(album, dict) else None,
        "release_date": album.get("release_date") if isinstance(album, dict) else None,
        "spotify_url": ext.get("spotify") if isinstance(ext, dict) else None,
    }


def _norm(s: str) -> str:
    return re.sub(r"\s+", " ", (s or "").lower()).strip()


async def spotify_search_best_match(
    title: str, album: str, performer: str, limit: int = 5
) -> dict | None:

    tokens = []
    if title:
        tokens.append(f'track:"{title}"')
    if album:
        tokens.append(f'album:"{album}"')
    if performer:
        tokens.append(f'artist:"{performer}"')
    q = " ".join(tokens) if tokens else title

    cache_key = f"best|{q.lower()}|{int(limit)}"
    now = time.time()
    async with _SPOTIFY_SEARCH_CACHE_GUARD:
        cached = _SPOTIFY_SEARCH_CACHE.get(cache_key)
        if cached and now < cached[0]:
            return cached[1]

    async with _SPOTIFY_SEARCH_SEMAPHORE:
        token = await get_access_token()
        url = f"https://api.spotify.com/v1/search?q={quote(q)}&type=track&limit={int(limit)}"
        headers = {"Authorization": f"Bearer {token}"}

        async with ClientSession() as session:
            async with session.get(url, headers=headers) as response:
                payload = await response.json(content_type=None)
                if response.status != 200:
                    error = (
                        payload.get("error") if isinstance(payload, dict) else payload
                    )
                    raise RuntimeError(f"Spotify search error: {error}")

    items = ((payload or {}).get("tracks") or {}).get("items") or []
    if not items:
        return None

    title_cf = (title or "").casefold()
    album_cf = (album or "").casefold()
    performer_cf = (performer or "").casefold()
    best = None
    best_score = -1

    for tr in items:
        if not isinstance(tr, dict):
            continue
        name_cf = (tr.get("name", "") or "").casefold()
        tr_album_cf = (((tr.get("album") or {}).get("name") or "") or "").casefold()
        artists = tr.get("artists") or []
        tr_artist_cf = (
            ((artists[0] or {}).get("name", "") or "").casefold()
            if artists and isinstance(artists[0], dict)
            else ""
        )

        score = 0
        if title_cf:
            score += 6 if name_cf == title_cf else (3 if title_cf in name_cf else 0)
        if album_cf:
            score += (
                4 if tr_album_cf == album_cf else (2 if album_cf in tr_album_cf else 0)
            )
        if performer_cf:
            score += (
                6
                if tr_artist_cf == performer_cf
                else (3 if performer_cf in tr_artist_cf else 0)
            )

        if score > best_score:
            best = tr
            best_score = score

    picked = best or items[0]
    async with _SPOTIFY_SEARCH_CACHE_GUARD:
        _SPOTIFY_SEARCH_CACHE[cache_key] = (time.time() + 6 * 3600, picked)
    return picked


def _strip_query_noise(text: str) -> str:
    s = (text or "").strip()
    if not s:
        return ""
    s = re.sub(r"[\[\(].*?[\]\)]", " ", s)
    s = re.sub(
        r"\b(?:official|audio|video|lyrics?|lyric|mv|visualizer|remaster(?:ed)?|hd|explicit|clean|version|edit)\b",
        " ",
        s,
        flags=re.I,
    )
    s = re.sub(r"\s+", " ", s).strip()
    return s


async def spotify_find_cover_url(
    title: str, performer: str, album: str = ""
) -> str | None:
    t = _strip_query_noise(title)
    p = _strip_query_noise(performer)
    a = _strip_query_noise(album)

    _dbg(
        f"[cover] spotify start title={title!r} artist={performer!r} album={album!r} cleaned=({t!r}, {p!r}, {a!r})"
    )

    variants: list[str] = []
    base = re.sub(r"\s+", " ", t).strip()
    m = re.search(r"[\(\[]\s*(.+?)\s*[\)\]]", t)
    tag = (m.group(1) or "").strip() if m else ""
    if base:
        variants.append(base)
    if tag:
        variants.append(f"{base} {tag}")
        variants.append(f"{base} - {tag}")
        tag_cf = tag.casefold()
        if "extended" in tag_cf:
            variants.append(f"{base} Extended")
            variants.append(f"{base} Extended Mix")
        if "remix" in tag_cf:
            variants.append(f"{base} Remix")
        if "radio" in tag_cf and "edit" in tag_cf:
            variants.append(f"{base} Radio Edit")
        if "vip" in tag_cf:
            variants.append(f"{base} VIP")

    queries: list[str] = []
    if t and p:
        queries.append(f'track:"{t}" artist:"{p}"')
        if a:
            queries.append(f'track:"{t}" artist:"{p}" album:"{a}"')
    for v in variants:
        if p:
            queries.append(f"{v} {p}")
            queries.append(f'track:"{v}" artist:"{p}"')
        queries.append(v)
    if t and p and a:
        queries.append(f"{t} {p} {a}")
    if t:
        queries.append(t)

    seen: set[str] = set()
    for q in queries:
        k = (q or "").strip().lower()
        if not k or k in seen:
            continue
        seen.add(k)
        _dbg(f"[cover] spotify query={q!r}")
        items = await spotify_search_tracks(q, limit=5)
        if items:
            _dbg(
                f"[cover] spotify results={len(items)} first={_spotify_track_brief(items[0])}"
            )
            best = await spotify_search_best_match(
                title=t or title, album=a or album, performer=p or performer, limit=5
            )
            if best:
                url = best_cover_url(best)
                if url:
                    _dbg(
                        f"[cover] spotify match_found url={url} data={_spotify_track_brief(best)}"
                    )
                    return url
            for tr in items:
                url = best_cover_url(tr)
                if url:
                    _dbg(
                        f"[cover] spotify match_found url={url} data={_spotify_track_brief(tr)}"
                    )
                    return url

    track = await spotify_search_best_match(
        title=t or title, album=a or album, performer=p or performer
    )
    if track:
        url = best_cover_url(track)
        if url:
            _dbg(
                f"[cover] spotify match_found url={url} data={_spotify_track_brief(track)}"
            )
        return url
    return None


def _cov_confidence_score(value: str) -> int:
    s = (value or "").strip().lower()
    if s == "very_high":
        return 4
    if s == "high":
        return 3
    if s == "medium":
        return 2
    if s == "low":
        return 1
    return 0


def _cov_parse_ndjson(text: str) -> list[dict]:
    items: list[dict] = []
    buf = ""
    braces = 0
    for ch in text or "":
        if ch == "{":
            braces += 1
        if ch == "}":
            braces = max(0, braces - 1)
        buf += ch
        if braces == 0 and buf.strip():
            try:
                data = json.loads(buf)
                if isinstance(data, dict):
                    items.append(data)
                elif isinstance(data, list):
                    items.extend([x for x in data if isinstance(x, dict)])
            except Exception:
                pass
            buf = ""
    return items


def _cov_item_brief(item: dict | None) -> dict:
    if not isinstance(item, dict):
        return {}
    rel = item.get("releaseInfo") if isinstance(item.get("releaseInfo"), dict) else {}
    return {
        "source": item.get("source"),
        "confidence": item.get("confidence"),
        "isOriginal": item.get("isOriginal"),
        "release": {
            "title": rel.get("title") if isinstance(rel, dict) else None,
            "artist": rel.get("artist") if isinstance(rel, dict) else None,
            "releaseYear": rel.get("releaseYear") if isinstance(rel, dict) else None,
            "releaseDate": rel.get("releaseDate") if isinstance(rel, dict) else None,
        },
        "bigCoverUrl": item.get("bigCoverUrl"),
        "smallCoverUrl": item.get("smallCoverUrl"),
        "type": item.get("type"),
    }


async def cov_find_cover(
    title: str, artist: str, album: str, year: int | None = None, country: str = "in"
) -> dict | None:
    name = (album or "").strip() or (title or "").strip()
    artist = (artist or "").strip()
    if not name or not artist:
        return None

    _dbg(
        f"[cover] cov start name={name!r} artist={artist!r} year={year!r} country={country!r}"
    )

    key = f"cov|{name.lower()}|{artist.lower()}|{(str(year) if year else '')}|{country.lower()}"
    now = time.time()
    async with _COV_SEARCH_CACHE_GUARD:
        cached = _COV_SEARCH_CACHE.get(key)
        if cached and now < cached[0]:
            return cached[1]

    payload = {
        "artist": artist,
        "album": name,
        "country": country,
        "sources": ["spotify", "applemusic", "amazonmusic", "lastfm"],
    }

    async with _COV_SEARCH_SEMAPHORE:
        async with ClientSession() as session:
            async with session.post(
                "https://covers.musichoarders.xyz/api/search",
                json=payload,
                headers={"Content-Type": "application/json", **HEADERS},
            ) as resp:
                text = await resp.text()
                if resp.status != 200 or not text:
                    _dbg(
                        f"[cover] cov no_results status={resp.status} text_len={len(text or '')}"
                    )
                    async with _COV_SEARCH_CACHE_GUARD:
                        _COV_SEARCH_CACHE[key] = (time.time() + 1800, None)
                    return None

    items = _cov_parse_ndjson(text)
    _dbg(f"[cover] cov parsed_items={len(items)}")
    best = None
    best_score = -1
    wanted_year = None
    if year:
        try:
            wanted_year = int(year)
        except Exception:
            wanted_year = None

    title_cf = (name or "").casefold()
    artist_cf = (artist or "").casefold()
    for it in items:
        if (it.get("type") or "").lower() != "cover":
            continue
        source_bonus = 2 if (it.get("source") or "").strip().lower() == "spotify" else 0
        rel = it.get("releaseInfo") or {}
        rel_title_cf = ((rel.get("title") or "") or "").casefold()
        rel_artist_cf = ((rel.get("artist") or "") or "").casefold()
        conf = _cov_confidence_score(it.get("confidence") or "")
        is_original = 2 if bool(it.get("isOriginal")) else 0
        year_score = 0
        if wanted_year is not None:
            y = rel.get("releaseYear")
            if y is None:
                # try releaseDate "YYYY-MM-DD"
                d = (rel.get("releaseDate") or "").strip()
                try:
                    y = int(d[:4]) if len(d) >= 4 and d[:4].isdigit() else None
                except Exception:
                    y = None
            if y is not None:
                try:
                    year_score = 2 if int(y) == wanted_year else 0
                except Exception:
                    year_score = 0

        match_score = 0
        if title_cf:
            match_score += (
                4
                if rel_title_cf == title_cf
                else (2 if title_cf in rel_title_cf else 0)
            )
        if artist_cf:
            match_score += (
                4
                if rel_artist_cf == artist_cf
                else (2 if artist_cf in rel_artist_cf else 0)
            )

        score = conf + is_original + year_score + match_score + source_bonus
        if score > best_score:
            best = it
            best_score = score

    if best and _cov_confidence_score(best.get("confidence") or "") < 2:
        best = None

    if best:
        _dbg(f"[cover] cov match_found data={_cov_item_brief(best)}")
    else:
        _dbg("[cover] cov no_match")

    async with _COV_SEARCH_CACHE_GUARD:
        _COV_SEARCH_CACHE[key] = (time.time() + 6 * 3600, best)
    return best


async def cov_find_cover_url(
    title: str, artist: str, album: str, year: int | None = None
) -> str | None:
    name = (album or "").strip() or (title or "").strip()
    performer = (artist or "").strip()
    if not name or not performer:
        return None
    try:
        return await hoaders_big_cover_url(artist=performer, album=name, year=year)
    except Exception as e:
        _dbg(f"[cover] hoaders failed err={e!r}")
        return None


async def _fetch_bytes(url: str) -> bytes:
    async with ClientSession() as session:
        async with session.get(url, headers=HEADERS) as resp:
            resp.raise_for_status()
            return await resp.read()


async def fetch_bytes_with_type(url: str) -> tuple[bytes, str]:
    _dbg(f"[cover] downloading url={url}")
    async with ClientSession() as session:
        async with session.get(url, headers=HEADERS) as resp:
            resp.raise_for_status()
            ct = (resp.headers.get("Content-Type") or "").split(";")[0].strip().lower()
            return await resp.read(), ct


def _dbg(msg: str) -> None:
    if bool(getattr(Config, "DEBUG", False)):
        LOG.debug(msg)


_MEDIAINFO_AVAILABLE: bool | None = None  # None = not yet checked


async def _check_mediainfo_available() -> bool:
    """One-time check whether the mediainfo binary exists and runs."""
    global _MEDIAINFO_AVAILABLE
    if _MEDIAINFO_AVAILABLE is not None:
        return _MEDIAINFO_AVAILABLE
    try:
        proc = await asyncio.create_subprocess_exec(
            "mediainfo", "--Version",
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        await asyncio.wait_for(proc.communicate(), timeout=5.0)
        _MEDIAINFO_AVAILABLE = proc.returncode == 0
    except Exception:
        _MEDIAINFO_AVAILABLE = False
    if not _MEDIAINFO_AVAILABLE:
        LOG.warning("mediainfo binary not available — metadata extraction will be limited")
    else:
        LOG.info("mediainfo binary OK")
    return _MEDIAINFO_AVAILABLE


async def run_mediainfo(path: str) -> str:
    if not await _check_mediainfo_available():
        return ""

    proc = None
    try:
        proc = await asyncio.create_subprocess_exec(
            "mediainfo",
            path,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        stdout, stderr = await asyncio.wait_for(proc.communicate(), timeout=8.0)
        data = stdout or stderr or b""
        return data.decode(errors="ignore")
    except asyncio.TimeoutError:
        LOG.warning(f"mediainfo timed out on {path}")
        if proc:
            try:
                proc.kill()
            except ProcessLookupError:
                pass
            try:
                await asyncio.wait_for(proc.wait(), timeout=2.0)
            except asyncio.TimeoutError:
                pass
        return ""
    except Exception as e:
        LOG.warning(f"mediainfo failed on {path}: {e}")
        if proc and proc.returncode is None:
            try:
                proc.kill()
            except ProcessLookupError:
                pass
            try:
                await asyncio.wait_for(proc.wait(), timeout=2.0)
            except asyncio.TimeoutError:
                pass
        return ""


async def _download_partial_http(
    url: str, file_path: str, max_bytes: int = 10_000_000
) -> int | None:
    async with ClientSession() as session:
        async with session.get(url, headers=HEADERS) as resp:
            resp.raise_for_status()
            size = resp.headers.get("Content-Length")
            file_size = int(size) if size and str(size).isdigit() else None

            written = 0
            async with aiopen(file_path, "wb") as f:
                async for chunk in resp.content.iter_chunked(512 * 1024):
                    if not chunk:
                        break
                    remaining = max_bytes - written
                    if remaining <= 0:
                        break
                    await f.write(chunk[:remaining])
                    written += len(chunk)

            return file_size


async def _download_telegram(reply: Message, media, file_path: str) -> int | None:
    size = getattr(media, "file_size", None)

    if size and size <= 50_000_000:
        await reply.download(file_name=file_path)
        return size

    async with aiopen(file_path, "wb") as f:
        written = 0
        max_prefix_bytes = 10_000_000
        stream_client = _stream_media_client(reply)
        async for chunk in stream_client.stream_media(reply, limit=25):
            if not chunk:
                break
            remaining = max_prefix_bytes - written
            if remaining <= 0:
                break
            await f.write(chunk[:remaining])
            written += min(len(chunk), remaining)

    return size


async def generate_mediainfo(
    message: Message, link: str | None = None, reply: Message | None = None, media=None
):
    status = await message.reply_text("Generating MediaInfo...")
    file_path = cover_path = None

    try:
        base_dir = await ensure_media_dir()

        # 🎵 AUDIO COVER EXTRACTION (Telegram thumbnail)
        if reply and reply.audio and reply.audio.thumbs:
            cover_path = ospath.join(base_dir, COVER_NAME)
            thumb = reply.audio.thumbs[-1]
            await bot.download_media(thumb.file_id, file_name=cover_path)
            await message.reply_photo(cover_path, caption="🎵 Audio Cover")

        # FILE DOWNLOAD
        if link:
            filename = _extract_filename_from_url(link)
            file_path = ospath.join(base_dir, filename)
            file_size = await _download_partial_http(link, file_path)
        else:
            filename = _sanitize_filename(getattr(media, "file_name", str(reply.id)))
            file_path = ospath.join(base_dir, filename)
            file_size = await _download_telegram(reply, media, file_path)

        output = await run_mediainfo(file_path)
        text = f"📌 `{ospath.basename(file_path)}`\n"
        if file_size:
            text += f"Size: `{get_readable_bytes(file_size)}`\n\n"
        text += _format_filtered_metadata(output)[:3500]

        await status.edit_text(text, disable_web_page_preview=True)

    except Exception as e:
        await status.edit_text(f"MediaInfo failed: {e}")

    finally:
        for p in (file_path, cover_path):
            if p:
                try:
                    await aioremove(p)
                except Exception:
                    pass


@bot.on_message(filters.command(["mediainfo", "mi"]))
async def mediainfo_handler(_, message: Message):
    reply = message.reply_to_message

    if reply and (reply.text or reply.caption):
        link = (reply.text or reply.caption).strip()
        if re.match(r"^https?://", link, flags=re.I):
            return await generate_mediainfo(message, link=link)

    if reply:
        media = _pick_media(reply)
        if media:
            return await generate_mediainfo(message, reply=reply, media=media)

    if len(message.command) > 1:
        return await generate_mediainfo(message, link=message.command[1])

    await message.reply_text(
        "Usage:\n/mediainfo <link>\nor reply to an audio / media / link",
    )


@bot.on_message(filters.command(["search"]))
async def spotify_search_handler(_, message: Message):
    query = (
        " ".join(message.command[1:]).strip()
        if getattr(message, "command", None)
        else ""
    )
    if (
        not query
        and message.reply_to_message
        and (message.reply_to_message.text or message.reply_to_message.caption)
    ):
        query = (
            message.reply_to_message.text or message.reply_to_message.caption or ""
        ).strip()

    if not query:
        return await message.reply_text("Usage:\n/search <query>")

    status = await message.reply_text("Searching...")

    try:
        track = await spotify_search_track(query)
        if not track:
            return await status.edit_text("No results found.")

        cover_url, cover_urls = get_track_cover_links(track)

        track_name = _md_clean(track.get("name", ""))
        artists = ", ".join(
            _md_clean(a.get("name", ""))
            for a in (track.get("artists") or [])
            if isinstance(a, dict)
        )
        album = _md_clean(((track.get("album") or {}).get("name") or ""))
        spotify_url = (
            ((track.get("external_urls") or {}).get("spotify")) or ""
        ).strip()

        caption_lines = [
            f"🎵 `{track_name}`",
            f"👤 {artists}" if artists else "",
            f"💿 {album}" if album else "",
            "",
        ]

        links = []
        if spotify_url:
            links.append(f"[Spotify]({spotify_url})")
        if cover_url:
            labels = ["Large", "Medium", "Small"]
            for idx, url in enumerate(cover_urls[:3]):
                label = labels[idx] if idx < len(labels) else f"Cover {idx + 1}"
                links.append(f"[{label}]({url})")

        if links:
            caption_lines.append(" | ".join(links))

        caption = "\n".join([l for l in caption_lines if l != ""]).strip()
        if len(caption) > 1024:
            caption = caption[:1020] + "..."

        if not cover_url:
            await status.edit_text(caption or "Found track, but no cover image.")
            return

        try:
            await message.reply_photo(photo=cover_url, caption=caption)
        except Exception:
            data = await _fetch_bytes(cover_url)
            bio = io.BytesIO(data)
            bio.name = "cover.jpg"
            await message.reply_photo(photo=bio, caption=caption)

        await status.delete()

    except Exception as e:
        await status.edit_text(f"Search failed: {e}")
