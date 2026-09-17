import asyncio
import copy
import hashlib
import os
import re
import time
import unicodedata
from typing import Any
from urllib.parse import parse_qs, urlparse

try:
    import yt_dlp
except Exception:  # pragma: no cover - runtime fallback when dependency is absent
    yt_dlp = None

from stream.helpers.logger import LOGGER


LOG = LOGGER(__name__)

_YT_TRACK_ID_RE = re.compile(r"^yt_([A-Za-z0-9_-]{11})$")
_YT_VIDEO_ID_RE = re.compile(r"^[A-Za-z0-9_-]{11}$")
_YT_CACHE_TTL_SEC = 15 * 60
_YOUTUBE_ALBUM = "YouTube"
_YOUTUBE_ALBUM_ID = "album_youtube"
_GENERIC_YOUTUBE_ALBUM_TITLES = {"youtube", "youtube music"}

_YT_META_CACHE: dict[str, tuple[float, dict[str, Any]]] = {}
_YT_META_INFLIGHT: dict[str, asyncio.Task[dict[str, Any]]] = {}
_YT_META_LOCK = asyncio.Lock()

_ROOT_DIR = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
_COOKIES_PATH = os.path.join(_ROOT_DIR, "cookies", "yt.txt")
_YDL_HEADERS = {
    "User-Agent": (
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
        "(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
    ),
    "Accept-Language": "en-US,en;q=0.5",
    "Referer": "https://www.google.com/",
}


def _coerce_text(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, str):
        return value.strip()
    return str(value).strip()


def _normalize_album_id_part(text: Any) -> str:
    raw = _coerce_text(text).strip().lower()
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


def _is_generic_youtube_album(value: Any) -> bool:
    normalized = _coerce_text(value).lower()
    return normalized in _GENERIC_YOUTUBE_ALBUM_TITLES


def resolve_youtube_album_metadata(info: dict[str, Any] | None, fallback_title: Any) -> tuple[str, str]:
    payload = info if isinstance(info, dict) else {}
    raw_album = _pick_first_nonempty(payload.get("album"))
    resolved_album = raw_album if raw_album and not _is_generic_youtube_album(raw_album) else _pick_first_nonempty(fallback_title, raw_album, _YOUTUBE_ALBUM)

    album_slug = _normalize_album_id_part(resolved_album)
    album_id = f"album_{album_slug}" if album_slug else _YOUTUBE_ALBUM_ID
    return resolved_album, album_id


def youtube_track_id(video_id: str) -> str:
    vid = _coerce_text(video_id)
    if not _YT_VIDEO_ID_RE.fullmatch(vid):
        return ""
    return f"yt_{vid}"


def is_youtube_track_id(value: Any) -> bool:
    return bool(_YT_TRACK_ID_RE.fullmatch(_coerce_text(value)))


def youtube_track_url(video_id: str) -> str:
    return f"https://www.youtube.com/watch?v={video_id}"


def extract_youtube_video_id(value: Any) -> str | None:
    raw = _coerce_text(value)
    if not raw:
        return None

    match = _YT_TRACK_ID_RE.fullmatch(raw)
    if match:
        return match.group(1)

    raw_lower = raw.lower()
    if "youtube.com" not in raw_lower and "youtu.be/" not in raw_lower:
        return None

    candidate = raw
    if "://" not in candidate:
        candidate = f"https://{candidate.lstrip('/')}"

    try:
        parsed = urlparse(candidate)
    except Exception:
        return None

    host = (parsed.netloc or "").strip().lower()
    if host.startswith("www."):
        host = host[4:]
    if host.startswith("m."):
        host = host[2:]

    video_id: str | None = None
    if host == "youtu.be":
        parts = [p for p in (parsed.path or "").split("/") if p]
        if parts:
            video_id = parts[0]
    elif host.endswith("youtube.com"):
        qs = parse_qs(parsed.query or "")
        raw_v = qs.get("v")
        if raw_v and isinstance(raw_v, list):
            candidate_v = str(raw_v[0] or "").strip()
            if candidate_v:
                video_id = candidate_v
        if not video_id:
            parts = [p for p in (parsed.path or "").split("/") if p]
            if len(parts) >= 2 and parts[0] in {"embed", "shorts", "live", "v"}:
                video_id = parts[1]

    if not video_id or not _YT_VIDEO_ID_RE.fullmatch(video_id):
        return None
    return video_id


def normalize_track_reference(value: Any) -> str:
    raw = _coerce_text(value)
    if not raw:
        return ""
    video_id = extract_youtube_video_id(raw)
    if video_id:
        return youtube_track_id(video_id)
    return raw


def _ydl_opts() -> dict[str, Any]:
    opts: dict[str, Any] = {
        "quiet": True,
        "no_warnings": True,
        "noplaylist": True,
        "skip_download": True,
        "nocheckcertificate": True,
        "extract_flat": False,
        "http_headers": dict(_YDL_HEADERS),
    }
    if os.path.exists(_COOKIES_PATH):
        opts["cookiefile"] = _COOKIES_PATH
    return opts


def _extract_info_sync(video_id: str) -> dict[str, Any] | None:
    if yt_dlp is None:
        return None

    url = youtube_track_url(video_id)
    opts = _ydl_opts()
    try:
        with yt_dlp.YoutubeDL(opts) as ydl:
            info = ydl.extract_info(url, download=False)
        return info if isinstance(info, dict) else None
    except Exception as exc:
        if opts.get("cookiefile"):
            try:
                retry_opts = dict(opts)
                retry_opts.pop("cookiefile", None)
                with yt_dlp.YoutubeDL(retry_opts) as ydl:
                    info = ydl.extract_info(url, download=False)
                return info if isinstance(info, dict) else None
            except Exception as retry_exc:
                LOG.warning(f"[youtube_meta] failed video_id={video_id} err={retry_exc}")
                return None
        LOG.warning(f"[youtube_meta] failed video_id={video_id} err={exc}")
        return None


async def _fetch_youtube_info(video_id: str) -> dict[str, Any] | None:
    loop = asyncio.get_running_loop()
    return await loop.run_in_executor(None, _extract_info_sync, video_id)


def _pick_first_nonempty(*values: Any) -> str:
    for value in values:
        if isinstance(value, str) and value.strip():
            return value.strip()
    return ""


def _pick_artist(info: dict[str, Any]) -> str:
    if isinstance(info.get("artists"), list):
        for item in info.get("artists") or []:
            if isinstance(item, str) and item.strip():
                return item.strip()
            if isinstance(item, dict):
                name = item.get("name")
                if isinstance(name, str) and name.strip():
                    return name.strip()

    for key in ("artist", "creator", "uploader", "channel", "uploader_id", "channel_id"):
        picked = _pick_first_nonempty(info.get(key))
        if picked:
            return picked
    return "YouTube"


def _pick_thumbnail(info: dict[str, Any], video_id: str) -> str | None:
    thumbs = info.get("thumbnails")
    if isinstance(thumbs, list):
        for item in reversed(thumbs):
            if not isinstance(item, dict):
                continue
            url = item.get("url")
            if isinstance(url, str) and url.strip():
                return url.strip()

    direct = info.get("thumbnail")
    if isinstance(direct, str) and direct.strip():
        return direct.strip()
    if video_id:
        return f"https://i.ytimg.com/vi/{video_id}/hqdefault.jpg"
    return None


def build_virtual_youtube_track(track_id: str, info: dict[str, Any] | None = None) -> dict[str, Any] | None:
    video_id = extract_youtube_video_id(track_id)
    if not video_id:
        return None

    normalized_track_id = youtube_track_id(video_id)
    payload = info if isinstance(info, dict) else {}

    title = _pick_first_nonempty(payload.get("track"), payload.get("title"), f"YouTube Video {video_id}")
    artist = _pick_artist(payload)
    album, album_id = resolve_youtube_album_metadata(payload, title)
    cover_url = _pick_thumbnail(payload, video_id)

    raw_duration = payload.get("duration")
    try:
        duration_sec = float(raw_duration) if raw_duration is not None else 0.0
    except Exception:
        duration_sec = 0.0
    if duration_sec < 0:
        duration_sec = 0.0

    yt_url = youtube_track_url(video_id)
    updated_at = time.time()

    audio: dict[str, Any] = {
        "title": title,
        "artist": artist,
        "performer": artist,
        "album": album,
        "album_id": album_id,
        "duration_sec": duration_sec,
        "type": "youtube",
        "yt_id": video_id,
        "yt_url": yt_url,
    }

    track: dict[str, Any] = {
        "_id": normalized_track_id,
        "title": title,
        "artist": artist,
        "album": album,
        "album_id": album_id,
        "cover_url": cover_url,
        "duration_sec": duration_sec,
        "type": "youtube",
        "audio": audio,
        "spotify": {
            "cover_url": cover_url,
            "cover_source": "youtube",
        },
        "updated_at": updated_at,
    }

    return track


async def get_virtual_youtube_track(track_id: str) -> dict[str, Any] | None:
    video_id = extract_youtube_video_id(track_id)
    if not video_id:
        return None
    normalized_track_id = youtube_track_id(video_id)
    now = time.time()

    async with _YT_META_LOCK:
        cached = _YT_META_CACHE.get(normalized_track_id)
        if cached and now - float(cached[0]) < _YT_CACHE_TTL_SEC:
            return copy.deepcopy(cached[1])

        task = _YT_META_INFLIGHT.get(normalized_track_id)
        if task is None:
            task = asyncio.create_task(_fetch_youtube_info(video_id))
            _YT_META_INFLIGHT[normalized_track_id] = task

    try:
        info = await task
    finally:
        async with _YT_META_LOCK:
            if _YT_META_INFLIGHT.get(normalized_track_id) is task:
                _YT_META_INFLIGHT.pop(normalized_track_id, None)

    doc = build_virtual_youtube_track(normalized_track_id, info)
    if not doc:
        return None

    async with _YT_META_LOCK:
        _YT_META_CACHE[normalized_track_id] = (time.time(), copy.deepcopy(doc))
    return doc
