import asyncio
import hashlib
import logging
import os
import re
import subprocess
import time
from collections import deque
from dataclasses import dataclass, field
from typing import Any, AsyncIterator, Optional
from urllib.parse import quote

import aiofiles
from fastapi import HTTPException, Request
from starlette.responses import Response, StreamingResponse

from Api.deps.db import get_audio_tracks_collection
from Api.utils.auth import verify_auth_token
from stream import bot, get_primary_client_user_id
from stream.core.config_manager import Config
from stream.database.MongoDb import db_handler
from stream.helpers.logger import LOGGER

MEDIA_DIR = "stream_media"
ALAC_CACHE_DIR = os.path.join(MEDIA_DIR, "alac_cache")
_ALAC_TRANSCODE_LOCKS: dict[str, asyncio.Lock] = {}
_ALAC_TRANSCODE_LOCKS_LOCK = asyncio.Lock()

_CHUNK_SIZE = 1024 * 1024
_MAX_STREAM_BUFFER_BYTES = 25_000_000
_STREAM_HUBS: dict[str, "_StreamHub"] = {}
_STREAM_HUBS_LOCK = asyncio.Lock()
_FILE_ID_LOCKS: dict[str, asyncio.Lock] = {}
_FILE_ID_LOCKS_LOCK = asyncio.Lock()
_TRACK_AFFINITY: dict[str, tuple[int, float]] = {}
_TRACK_AFFINITY_LOCK = asyncio.Lock()
_TRACK_AFFINITY_TTL_SEC = 300.0

_PLAY_COUNT_THRESHOLD_SEC = 30.0
_PLAY_PROGRESS_TTL_SEC = 1800.0
_PLAY_PROGRESS_MAX = 5000
_PLAY_PROGRESS_UPDATE_EVERY_BYTES = 512 * 1024
_PLAY_PROGRESS_LOCK = asyncio.Lock()
_RECENTLY_RECORDED_HISTORY: dict[tuple[int, str], float] = {}

LOG = LOGGER(__name__)


@dataclass(slots=True)
class _PlayProgress:
    file_size: int | None = None
    duration_sec: float | None = None
    bitrate_kbps: int | None = None
    ranges: list[tuple[int, int]] = field(default_factory=list)
    counted: bool = False
    last_seen: float = 0.0


_PLAY_PROGRESS: dict[str, _PlayProgress] = {}


class _StreamBehindError(RuntimeError):
    pass


class _StreamHub:
    def __init__(
        self,
        *,
        track_id: str,
        file_id: str,
        source_chat_id: int | None,
        source_message_id: int | None,
    ):
        self.track_id = track_id
        self.file_id = file_id
        self.source_chat_id = source_chat_id
        self.source_message_id = source_message_id
        self._cond = asyncio.Condition()
        self._chunks: deque[tuple[int, bytes]] = deque()
        self._total_written = 0
        self._done = False
        self._error: BaseException | None = None
        self._producer: asyncio.Task | None = None
        self._consumers: dict[str, int] = {}
        self._lagged: set[str] = set()
        self._last_access = time.monotonic()

    def start(self) -> None:
        if self._producer is None or self._producer.done():
            self._producer = asyncio.create_task(self._run())

    async def close(self) -> None:
        producer = self._producer
        self._producer = None
        if producer and not producer.done():
            producer.cancel()
            try:
                await producer
            except Exception:
                pass

    def _buffer_start(self) -> int:
        if not self._chunks:
            return self._total_written
        return self._chunks[0][0]

    async def _run(self) -> None:
        from stream import acquire_stream_client, release_stream_client

        client_id = 0
        refreshed = False
        try:
            client_id, client = await acquire_stream_client()
            LOG.debug(f"Streaming started using client {client_id}")

            while True:
                # IMPORTANT: Ensure we have the file_id for THIS specific client
                self.file_id = await _ensure_client_file_id(
                    track_id=self.track_id,
                    client_user_id=int(client_id),
                    client=client,
                    source_chat_id=self.source_chat_id,
                    source_message_id=self.source_message_id,
                )

                target = self.file_id
                if (
                    self.source_chat_id is not None
                    and self.source_message_id is not None
                ):
                    try:
                        msg = await client.get_messages(
                            int(self.source_chat_id), int(self.source_message_id)
                        )
                        if msg:
                            target = msg
                    except Exception as e:
                        LOG.warning(f"Hub failed to fetch source message: {e}")

                try:
                    start_chunk = self._total_written // _CHUNK_SIZE
                    stream_kwargs: dict[str, int] = {}
                    if start_chunk > 0:
                        stream_kwargs["offset"] = int(start_chunk)

                    remaining_skip = max(
                        0, self._total_written - (start_chunk * _CHUNK_SIZE)
                    )

                    async for chunk in client.stream_media(target, **stream_kwargs):
                        if not chunk:
                            continue

                        if remaining_skip > 0:
                            if len(chunk) <= remaining_skip:
                                remaining_skip -= len(chunk)
                                continue
                            chunk = chunk[remaining_skip:]
                            remaining_skip = 0

                        async with self._cond:
                            start = self._total_written
                            self._total_written += len(chunk)
                            self._chunks.append((start, chunk))
                            self._last_access = time.monotonic()
                            self._gc_locked()
                            self._cond.notify_all()
                    return
                except Exception as e:
                    if refreshed:
                        raise
                    msg_str = str(e).upper()
                    if (
                        "FILE_REFERENCE" not in msg_str
                        and "FILE_REFERENCE_EXPIRED" not in msg_str
                    ):
                        raise

                    LOG.debug(
                        f"Hub encountered expired reference, attempting internal refresh"
                    )

                    # Refresh file_id for THIS client and retry
                    try:
                        self.file_id = await _ensure_client_file_id(
                            track_id=self.track_id,
                            client_user_id=int(client_id),
                            client=client,
                            source_chat_id=self.source_chat_id,
                            source_message_id=self.source_message_id,
                            force=True,
                        )
                    except Exception as refresh_err:
                        LOG.error(f"Hub failed to refresh file_id: {refresh_err}")
                        if (
                            self.source_chat_id is None
                            or self.source_message_id is None
                        ):
                            raise e

                    refreshed = True
                    continue
        except asyncio.CancelledError:
            raise
        except BaseException as e:
            async with self._cond:
                self._error = e
                self._cond.notify_all()
        finally:
            await release_stream_client(client_id)
            async with self._cond:
                self._done = True
                self._cond.notify_all()

    def _gc_locked(self) -> None:
        min_offset = min(self._consumers.values(), default=self._total_written)
        while self._chunks:
            start, data = self._chunks[0]
            end = start + len(data)
            if end <= min_offset:
                self._chunks.popleft()
                continue
            break

        buffer_start = self._buffer_start()
        buffer_bytes = self._total_written - buffer_start
        if buffer_bytes <= _MAX_STREAM_BUFFER_BYTES:
            return

        keep_from = self._total_written - _MAX_STREAM_BUFFER_BYTES
        while self._chunks:
            start, data = self._chunks[0]
            end = start + len(data)
            if end <= keep_from:
                self._chunks.popleft()
                continue
            break

        new_start = self._buffer_start()
        if new_start <= buffer_start:
            return

        for cid, off in list(self._consumers.items()):
            if off < new_start:
                self._lagged.add(cid)

    async def iter_bytes(self, start_byte: int) -> AsyncIterator[bytes]:
        cid = str(time.time_ns())
        start_byte = max(0, int(start_byte))

        self.start()
        async with self._cond:
            self._consumers[cid] = start_byte
            self._last_access = time.monotonic()
            self._cond.notify_all()

        try:
            while True:
                async with self._cond:
                    if cid in self._lagged:
                        raise _StreamBehindError()
                    if self._error:
                        raise self._error

                    offset = self._consumers.get(cid, start_byte)
                    buffer_start = self._buffer_start()
                    if offset < buffer_start and self._total_written > 0:
                        raise _StreamBehindError()

                    if self._done and offset >= self._total_written:
                        return

                    chunk = None
                    for start, data in self._chunks:
                        end = start + len(data)
                        if end <= offset:
                            continue
                        if start > offset:
                            break
                        chunk = (start, data)
                        break

                    if chunk is None:
                        await self._cond.wait()
                        continue

                    start, data = chunk
                    rel = offset - start
                    out = data[rel:] if rel > 0 else data
                    self._consumers[cid] = offset + len(out)
                    self._last_access = time.monotonic()
                    self._gc_locked()

                if out:
                    yield out
        finally:
            async with self._cond:
                self._consumers.pop(cid, None)
                self._lagged.discard(cid)
                self._last_access = time.monotonic()
                self._gc_locked()


def _hub_key(
    file_id: str, source_chat_id: int | None, source_message_id: int | None
) -> str:
    if source_chat_id is not None and source_message_id is not None:
        return f"m:{source_chat_id}:{source_message_id}"
    return f"f:{file_id}"


async def _get_or_create_hub(
    *,
    track_id: str,
    file_id: str,
    source_chat_id: int | None,
    source_message_id: int | None,
) -> _StreamHub:
    key = _hub_key(file_id, source_chat_id, source_message_id)
    async with _STREAM_HUBS_LOCK:
        hub = _STREAM_HUBS.get(key)
        if hub is None:
            hub = _StreamHub(
                track_id=track_id,
                file_id=file_id,
                source_chat_id=source_chat_id,
                source_message_id=source_message_id,
            )
            _STREAM_HUBS[key] = hub
        hub.start()
        return hub


async def close_stream_hubs() -> None:
    async with _STREAM_HUBS_LOCK:
        hubs = list(_STREAM_HUBS.values())
        _STREAM_HUBS.clear()
    for hub in hubs:
        try:
            await hub.close()
        except Exception:
            pass


def _parse_range_header(value: str) -> tuple[int | None, int | None]:
    if not value:
        return None, None
    m = re.match(r"^bytes=(\d+)-(\d+)?$", value.strip(), flags=re.I)
    if not m:
        return None, None
    try:
        start = int(m.group(1))
    except Exception:
        return None, None
    if start < 0:
        return None, None
    end = None
    if m.group(2) is not None and m.group(2) != "":
        try:
            end = int(m.group(2))
        except Exception:
            end = None
    if end is not None and end < start:
        return None, None
    return start, end


def _guess_extension(mime_type: str) -> str:
    mt = (mime_type or "").strip().lower()
    if mt in {"audio/mpeg", "audio/mp3"}:
        return ".mp3"
    if mt in {"audio/flac", "audio/x-flac"}:
        return ".flac"
    if mt in {"audio/wav", "audio/x-wav", "audio/wave"}:
        return ".wav"
    if mt in {"audio/ogg", "application/ogg"}:
        return ".ogg"
    if mt in {"audio/aac"}:
        return ".aac"
    if mt in {"audio/mp4", "audio/m4a", "audio/x-m4a"}:
        return ".m4a"
    return ".mp3"


def _normalize_mime_type(mime_type: Any, doc: dict | None = None) -> str:
    raw = (str(mime_type) if mime_type is not None else "").split(";")[0].strip().lower()
    if raw in {"audio/flac", "audio/x-flac"} or raw.endswith("/x-flac"):
        return "audio/flac"
    if raw in {"audio/wav", "audio/x-wav", "audio/wave"}:
        return "audio/wav"
    if raw in {"audio/mpeg", "audio/mp3"}:
        return "audio/mpeg"
    if raw in {"audio/ogg", "application/ogg"}:
        return "audio/ogg"
    if raw in {"audio/aac"}:
        return "audio/aac"
    if raw in {"audio/mp4", "audio/m4a", "audio/x-m4a"}:
        return "audio/mp4"

    if doc:
        audio = doc.get("audio") if isinstance(doc.get("audio"), dict) else {}
        atype = str(audio.get("type") or audio.get("format") or "").lower().strip()
        if "flac" in atype:
            return "audio/flac"
        if "wav" in atype:
            return "audio/wav"
        if "mp3" in atype or "mpeg" in atype:
            return "audio/mpeg"
        if "aac" in atype:
            return "audio/aac"
        telegram = doc.get("telegram") if isinstance(doc.get("telegram"), dict) else {}
        fname = str(telegram.get("file_name") or "").lower().strip()
        if fname.endswith(".flac"):
            return "audio/flac"
        if fname.endswith(".wav"):
            return "audio/wav"
        if fname.endswith(".mp3"):
            return "audio/mpeg"

    return raw or "audio/mpeg"


def _build_download_filename(
    *, track_id: str, audio: dict, telegram: dict, mime_type: str
) -> str:
    title = (audio.get("title") or telegram.get("title") or "").strip()
    artist = (
        audio.get("artist") or audio.get("performer") or telegram.get("artist") or ""
    ).strip()
    if artist and title:
        raw = f"{artist} - {title}"
    elif title:
        raw = title
    else:
        raw = f"track-{str(track_id or '').strip()[:12]}"
    raw = re.sub(r"\s+", " ", raw).strip()
    cleaned = re.sub(r'[\\/:*?"<>|\x00-\x1f]+', " ", raw)
    cleaned = re.sub(r"\s+", " ", cleaned).strip(" ._-")
    if not cleaned:
        cleaned = f"track-{str(track_id or '').strip()[:12]}"
    ext = _guess_extension(mime_type)
    lower = cleaned.lower()
    if not lower.endswith(ext):
        cleaned = f"{cleaned}{ext}"
    return cleaned


def _content_disposition_header(*, filename: str, track_id: str, mime_type: str) -> str:
    ext = _guess_extension(mime_type)
    fallback = re.sub(r"[^A-Za-z0-9._ -]+", "", filename or "").strip()
    if not fallback:
        fallback = f"track-{str(track_id or '').strip()[:12]}{ext}"
    if not fallback.lower().endswith(ext):
        fallback = f"{fallback}{ext}"
    encoded = quote(filename or fallback, safe="")
    return f"attachment; filename=\"{fallback}\"; filename*=UTF-8''{encoded}"


def _request_fingerprint(request: Request) -> str:
    ip = ""
    try:
        if request.client and request.client.host:
            ip = str(request.client.host)
    except Exception:
        ip = ""
    ua = (
        request.headers.get("user-agent") or request.headers.get("User-Agent") or ""
    ).strip()
    raw = f"{ip}|{ua}"
    return hashlib.sha1(raw.encode("utf-8", errors="ignore")).hexdigest()[:16]


def _request_user_id(request: Request) -> int | None:
    token = (
        request.headers.get("authorization")
        or request.headers.get("Authorization")
        or ""
    ).strip()
    if not token:
        token = (
            request.headers.get("x-auth-token")
            or request.headers.get("X-Auth-Token")
            or ""
        ).strip()
    if not token:
        token = (request.query_params.get("token") or "").strip()
    if not token:
        token = (request.query_params.get("auth_token") or "").strip()
    if not token:
        token = (request.query_params.get("auth") or "").strip()
    if not token:
        token = (request.cookies.get("auth_token") or "").strip()
    if not token:
        token = (request.cookies.get("token") or "").strip()
    if not token:
        return None
    try:
        payload = verify_auth_token(token)
        uid = payload.get("uid")
        uid = int(uid)
        if uid > 0:
            return int(uid)
    except Exception:
        return None
    return None


def _merge_ranges(
    ranges: list[tuple[int, int]], start: int, end: int
) -> list[tuple[int, int]]:
    start = int(start)
    end = int(end)
    if end < start:
        return ranges
    if not ranges:
        return [(start, end)]
    out: list[tuple[int, int]] = []
    inserted = False
    for a, b in ranges:
        if b + 1 < start:
            out.append((a, b))
            continue
        if end + 1 < a:
            if not inserted:
                out.append((start, end))
                inserted = True
            out.append((a, b))
            continue
        start = min(start, int(a))
        end = max(end, int(b))
    if not inserted:
        out.append((start, end))
    out.sort(key=lambda x: x[0])
    merged: list[tuple[int, int]] = []
    for a, b in out:
        if not merged:
            merged.append((a, b))
            continue
        la, lb = merged[-1]
        if a <= lb + 1:
            merged[-1] = (la, max(lb, b))
        else:
            merged.append((a, b))
    return merged


def _covered_bytes(ranges: list[tuple[int, int]]) -> int:
    total = 0
    for a, b in ranges:
        total += max(0, int(b) - int(a) + 1)
    return int(total)


def _seconds_from_bytes(
    *,
    covered: int,
    file_size: int | None,
    duration_sec: float | None,
    bitrate_kbps: int | None,
) -> float:
    covered = int(max(0, covered))
    if file_size and duration_sec and file_size > 0 and duration_sec > 0:
        return float(covered) * float(duration_sec) / float(file_size)
    if bitrate_kbps and bitrate_kbps > 0:
        bytes_per_sec = (float(bitrate_kbps) * 1000.0) / 8.0
        if bytes_per_sec > 0:
            return float(covered) / float(bytes_per_sec)
    return 0.0


async def _prune_play_progress_locked(now: float) -> None:
    if not _PLAY_PROGRESS:
        return
    expired: list[str] = []
    for k, v in _PLAY_PROGRESS.items():
        if (now - float(v.last_seen)) > float(_PLAY_PROGRESS_TTL_SEC):
            expired.append(k)
    for k in expired:
        _PLAY_PROGRESS.pop(k, None)
    if len(_PLAY_PROGRESS) <= _PLAY_PROGRESS_MAX:
        return
    items = sorted(_PLAY_PROGRESS.items(), key=lambda kv: float(kv[1].last_seen))
    for k, _ in items[: max(0, len(items) - _PLAY_PROGRESS_MAX)]:
        _PLAY_PROGRESS.pop(k, None)


async def _update_play_progress(
    *,
    key: str,
    start: int,
    end: int,
    file_size: int | None,
    duration_sec: float | None,
    bitrate_kbps: int | None,
) -> bool:
    now = time.monotonic()
    async with _PLAY_PROGRESS_LOCK:
        await _prune_play_progress_locked(now)
        prog = _PLAY_PROGRESS.get(key)
        if prog is None:
            prog = _PlayProgress(
                file_size=int(file_size) if file_size is not None else None,
                duration_sec=float(duration_sec) if duration_sec is not None else None,
                bitrate_kbps=int(bitrate_kbps) if bitrate_kbps is not None else None,
                last_seen=float(now),
            )
            _PLAY_PROGRESS[key] = prog
        prog.last_seen = float(now)
        if file_size is not None:
            try:
                prog.file_size = int(file_size)
            except Exception:
                pass
        if duration_sec is not None:
            try:
                prog.duration_sec = float(duration_sec)
            except Exception:
                pass
        if bitrate_kbps is not None:
            try:
                prog.bitrate_kbps = int(bitrate_kbps)
            except Exception:
                pass
        if prog.counted:
            return False
        prog.ranges = _merge_ranges(prog.ranges, int(start), int(end))
        covered = _covered_bytes(prog.ranges)
        sec = _seconds_from_bytes(
            covered=covered,
            file_size=prog.file_size,
            duration_sec=prog.duration_sec,
            bitrate_kbps=prog.bitrate_kbps,
        )
        if sec >= float(_PLAY_COUNT_THRESHOLD_SEC):
            prog.counted = True
            return True
        return False


async def _register_play(
    *,
    track_id: str,
    user_id: int | None,
    source: str,
    jam_id: str | None,
) -> None:
    track_id = (track_id or "").strip()
    if not track_id:
        return
    source = (source or "").strip() or "direct"
    if source not in {"direct", "jam"}:
        source = "direct"
    now = time.time()

    should_count_global = True
    if user_id is not None and int(user_id) > 0:
        played_bucket = int(now // _PLAY_PROGRESS_TTL_SEC)
        user_play_id = f"{int(user_id)}:{track_id}:{played_bucket}"
        doc: dict[str, object] = {
            "_id": user_play_id,
            "user_id": int(user_id),
            "track_id": track_id,
            "played_at": float(now),
            "bucket": int(played_bucket),
            "source": source,
        }
        if jam_id:
            doc["jam_id"] = str(jam_id)
        try:
            res = await db_handler.userplayback_collection.collection.update_one(
                {"_id": user_play_id},
                {"$setOnInsert": doc},
                upsert=True,
            )
            should_count_global = bool(getattr(res, "upserted_id", None))
        except Exception:
            should_count_global = False

    if not should_count_global:
        return

    inc: dict[str, int] = {"plays": 1, f"sources.{source}": 1}
    update: dict[str, object] = {
        "$inc": inc,
        "$set": {"last_played_at": float(now), "updated_at": float(now)},
    }
    if jam_id:
        update["$set"]["last_jam_id"] = str(jam_id)
    try:
        await db_handler.globalplayback_collection.collection.update_one(
            {"_id": track_id}, update, upsert=True
        )
    except Exception:
        return


async def _wrap_with_play_count(
    *,
    iterator: AsyncIterator[bytes],
    request: Request,
    track_id: str,
    from_bytes: int,
    file_size: int | None,
    duration_sec: float | None,
    bitrate_kbps: int | None,
) -> AsyncIterator[bytes]:
    if (request.method or "").upper() == "HEAD":
        async for chunk in iterator:
            yield chunk
        return

    user_id = _request_user_id(request)
    source = "jam" if (request.query_params.get("jam_id") or "").strip() else "direct"
    jam_id = (request.query_params.get("jam_id") or "").strip() or None
    fp = _request_fingerprint(request)
    key = f"{int(user_id) if user_id is not None else 0}:{track_id}:{fp}"

    cursor = max(0, int(from_bytes))
    last_flush = int(cursor)

    triggered = False
    try:
        async for chunk in iterator:
            if not chunk:
                continue
            yield chunk
            clen = len(chunk)
            if clen <= 0:
                continue
            cursor += int(clen)
            if triggered:
                continue
            if (cursor - last_flush) < int(_PLAY_PROGRESS_UPDATE_EVERY_BYTES):
                continue
            hit = await _update_play_progress(
                key=key,
                start=last_flush,
                end=cursor - 1,
                file_size=file_size,
                duration_sec=duration_sec,
                bitrate_kbps=bitrate_kbps,
            )
            last_flush = int(cursor)
            if hit:
                triggered = True
                asyncio.create_task(
                    _register_play(
                        track_id=track_id, user_id=user_id, source=source, jam_id=jam_id
                    )
                )
    finally:
        if not triggered and cursor > last_flush:
            try:
                hit2 = await _update_play_progress(
                    key=key,
                    start=last_flush,
                    end=cursor - 1,
                    file_size=file_size,
                    duration_sec=duration_sec,
                    bitrate_kbps=bitrate_kbps,
                )
                if hit2:
                    asyncio.create_task(
                        _register_play(
                            track_id=track_id,
                            user_id=user_id,
                            source=source,
                            jam_id=jam_id,
                        )
                    )
            except Exception:
                pass


def _extract_media_file_id(message) -> str | None:
    if not message:
        return None
    media = getattr(message, "audio", None) or getattr(message, "document", None)
    if not media:
        return None
    fid = getattr(media, "file_id", None)
    if not fid:
        return None
    return str(fid)


def _message_has_downloadable_media(message) -> bool:
    return bool(_extract_media_file_id(message))


def _hydration_chat_id() -> int:
    for attr in ("CHANNEL_ID", "DUMP_CHANNEL_ID"):
        value = getattr(Config, attr, None)
        try:
            chat_id = int(value)
        except Exception:
            chat_id = 0
        if chat_id:
            return chat_id
    return 0


def _coerce_optional_int(value) -> int | None:
    if value is None or isinstance(value, bool):
        return None
    try:
        return int(value)
    except Exception:
        return None


def _playback_source_ids(doc: dict | None) -> tuple[int | None, int | None]:
    if not isinstance(doc, dict):
        return None, None
    cache_chat_id = _coerce_optional_int(doc.get("cache_chat_id"))
    cache_message_id = _coerce_optional_int(doc.get("cache_message_id"))
    if cache_chat_id is not None and cache_message_id is not None:
        return cache_chat_id, cache_message_id
    return (
        _coerce_optional_int(doc.get("source_chat_id")),
        _coerce_optional_int(doc.get("source_message_id")),
    )


async def _get_lock(key: str) -> asyncio.Lock:
    async with _FILE_ID_LOCKS_LOCK:
        lock = _FILE_ID_LOCKS.get(key)
        if lock is None:
            lock = asyncio.Lock()
            _FILE_ID_LOCKS[key] = lock
        return lock


async def _ensure_client_file_id(
    *,
    track_id: str,
    client_user_id: int,
    client,
    source_chat_id: int | None,
    source_message_id: int | None,
    force: bool = False,
) -> str:
    lock = await _get_lock(f"{track_id}:{client_user_id}")
    async with lock:
        col = get_audio_tracks_collection()
        key = str(int(client_user_id))
        doc: dict | None = None
        telegram: dict = {}

        # If force is True, we ignore the cache for the entire function call.
        ignore_cache = force
        if ignore_cache:
            LOG.debug(
                f"stream file_id force refresh track={track_id} client={client_user_id}"
            )

        for attempt in range(6):
            doc = await col.find_one(
                {"_id": track_id},
                projection={
                    "telegram": 1,
                    "source_chat_id": 1,
                    "source_message_id": 1,
                    "cache_chat_id": 1,
                    "cache_message_id": 1,
                },
            )
            telegram = (doc or {}).get("telegram") or {}
            file_ids = (
                telegram.get("file_ids")
                if isinstance(telegram.get("file_ids"), dict)
                else {}
            )
            existing = (file_ids or {}).get(key)
            if not ignore_cache and isinstance(existing, str) and existing.strip():
                LOG.debug(
                    f"stream file_id cache hit track={track_id} client={client_user_id} file_id={existing.strip()}"
                )
                return existing.strip()

            resolved_chat_id = source_chat_id
            if resolved_chat_id is None:
                resolved_chat_id, _ = _playback_source_ids(doc)
            if resolved_chat_id is None:
                resolved_chat_id = getattr(Config, "CHANNEL_ID", None)
                try:
                    resolved_chat_id = (
                        int(resolved_chat_id) if resolved_chat_id is not None else None
                    )
                except Exception:
                    resolved_chat_id = None

            resolved_message_id = source_message_id
            if resolved_message_id is None:
                _, resolved_message_id = _playback_source_ids(doc)

            if resolved_chat_id is not None and resolved_message_id is not None:
                try:
                    msg = await client.get_messages(
                        int(resolved_chat_id), int(resolved_message_id)
                    )
                    fid = _extract_media_file_id(msg)
                except Exception:
                    fid = None

                if fid:
                    LOG.debug(
                        f"stream file_id synced via channel track={track_id} client={client_user_id} file_id={fid}"
                    )
                    await col.update_one(
                        {"_id": track_id},
                        {
                            "$set": {
                                f"telegram.file_ids.{key}": fid,
                                "updated_at": time.time(),
                            }
                        },
                    )
                    return fid

            if attempt < 5:
                await asyncio.sleep(0.35)

        dump_channel_id = _hydration_chat_id()
        if not dump_channel_id:
            raise HTTPException(
                status_code=404, detail="No source message to sync file_id"
            )

        resolved_chat_id = source_chat_id
        if resolved_chat_id is None:
            resolved_chat_id, _ = _playback_source_ids(doc)

        resolved_message_id = source_message_id
        if resolved_message_id is None:
            _, resolved_message_id = _playback_source_ids(doc)

        dump_message_id = telegram.get("dump_message_id")
        try:
            dump_message_id = (
                int(dump_message_id) if dump_message_id is not None else None
            )
        except Exception:
            dump_message_id = None

        if not dump_message_id:
            if resolved_chat_id is None or resolved_message_id is None:
                fallback_file_id = (telegram.get("file_id") or "").strip()
                if not fallback_file_id:
                    raise HTTPException(
                        status_code=404, detail="No source message to sync file_id"
                    )
                LOG.debug(
                    f"stream syncing via dump send_document track={track_id} client={client_user_id}"
                )
                sent = await bot.send_document(int(dump_channel_id), fallback_file_id)
            else:
                LOG.debug(
                    f"stream syncing via dump copy_message track={track_id} client={client_user_id} from={resolved_chat_id}:{resolved_message_id}"
                )
                try:
                    sent = await bot.copy_message(
                        chat_id=int(dump_channel_id),
                        from_chat_id=int(resolved_chat_id),
                        message_id=int(resolved_message_id),
                    )
                except Exception as e:
                    LOG.debug(
                        f"bot copy_message failed: {e}. Trying with userbot if available..."
                    )
                    from stream.plugins.userBot.service import _USERBOT_INSTANCE

                    if _USERBOT_INSTANCE:
                        sent = await _USERBOT_INSTANCE.copy_message(
                            chat_id=int(dump_channel_id),
                            from_chat_id=int(resolved_chat_id),
                            message_id=int(resolved_message_id),
                        )
                    else:
                        raise e
            dump_message_id = int(getattr(sent, "id"))
            await col.update_one(
                {"_id": track_id},
                {
                    "$set": {
                        "telegram.dump_message_id": dump_message_id,
                        "cache_chat_id": int(dump_channel_id),
                        "cache_message_id": dump_message_id,
                        "updated_at": time.time(),
                    }
                },
            )

        msg = await client.get_messages(int(dump_channel_id), int(dump_message_id))
        fid = _extract_media_file_id(msg)
        if not fid:
            raise HTTPException(
                status_code=404, detail="Failed to read file_id from dump message"
            )

        LOG.debug(
            f"stream file_id synced via dump track={track_id} client={client_user_id} file_id={fid}"
        )
        await col.update_one(
            {"_id": track_id},
            {"$set": {f"telegram.file_ids.{key}": fid, "updated_at": time.time()}},
        )
        return fid


async def _stream_range(
    *,
    track_id: str,
    client_user_id: int,
    client,
    file_id: str,
    source_chat_id: int | None,
    source_message_id: int | None,
    from_bytes: int,
    until_bytes: int | None,
) -> AsyncIterator[bytes]:
    cursor = max(0, int(from_bytes))
    refreshed = False
    try:
        if until_bytes is not None:
            until_bytes = max(cursor, int(until_bytes))
        while True:
            # IMPORTANT: Re-sync for THIS client on every retry to avoid client mismatch
            file_id = await _ensure_client_file_id(
                track_id=track_id,
                client_user_id=int(client_user_id),
                client=client,
                source_chat_id=source_chat_id,
                source_message_id=source_message_id,
                force=refreshed,
            )

            target: str | object = file_id
            if source_chat_id is not None and source_message_id is not None:
                try:
                    msg = await client.get_messages(
                        int(source_chat_id), int(source_message_id)
                    )
                    if msg and _message_has_downloadable_media(msg):
                        target = msg
                except Exception:
                    pass

            start_chunk = cursor // _CHUNK_SIZE
            stream_kwargs: dict[str, int] = {}
            if start_chunk:
                stream_kwargs["offset"] = int(start_chunk)

            stream_cursor = int(start_chunk) * int(_CHUNK_SIZE)
            try:
                async for chunk in client.stream_media(target, **stream_kwargs):
                    if not chunk:
                        continue

                    chunk_start = stream_cursor
                    chunk_end = stream_cursor + len(chunk) - 1
                    stream_cursor += len(chunk)

                    if chunk_end < cursor:
                        continue
                    if until_bytes is not None and chunk_start > until_bytes:
                        return

                    out_start = max(cursor, chunk_start)
                    out_end = (
                        chunk_end
                        if until_bytes is None
                        else min(until_bytes, chunk_end)
                    )
                    rel_start = out_start - chunk_start
                    rel_end = out_end - chunk_start
                    out_chunk = chunk[int(rel_start) : int(rel_end) + 1]

                    if out_chunk:
                        yield out_chunk
                        cursor = int(out_end) + 1
                    if until_bytes is not None and out_end >= until_bytes:
                        return
                return
            except Exception as e:
                if refreshed or source_chat_id is None or source_message_id is None:
                    raise
                msg_str = str(e).upper()
                if (
                    "FILE_REFERENCE" not in msg_str
                    and "FILE_REFERENCE_EXPIRED" not in msg_str
                ):
                    raise
                refreshed = True
                continue
    finally:
        from stream import release_stream_client

        await release_stream_client(client_user_id)


async def _direct_stream(
    *,
    track_id: str,
    client_user_id: int,
    client,
    file_id: str,
    source_chat_id: int | None,
    source_message_id: int | None,
    start_byte: int = 0,
) -> AsyncIterator[bytes]:
    cursor = max(0, int(start_byte))
    refreshed = False
    try:
        while True:
            # IMPORTANT: Re-sync for THIS client on every retry to avoid client mismatch
            file_id = await _ensure_client_file_id(
                track_id=track_id,
                client_user_id=int(client_user_id),
                client=client,
                source_chat_id=source_chat_id,
                source_message_id=source_message_id,
                force=refreshed,
            )

            target: str | object = file_id
            if source_chat_id is not None and source_message_id is not None:
                try:
                    msg = await client.get_messages(
                        int(source_chat_id), int(source_message_id)
                    )
                    if msg and _message_has_downloadable_media(msg):
                        target = msg
                except Exception:
                    pass

            remaining_skip = max(0, int(cursor))
            try:
                async for chunk in client.stream_media(target):
                    if not chunk:
                        continue
                    if remaining_skip > 0:
                        if len(chunk) <= remaining_skip:
                            remaining_skip -= len(chunk)
                            continue
                        chunk = chunk[remaining_skip:]
                        remaining_skip = 0
                    if chunk:
                        cursor += len(chunk)
                        yield chunk
                return
            except Exception as e:
                if refreshed or source_chat_id is None or source_message_id is None:
                    raise
                msg_str = str(e).upper()
                if (
                    "FILE_REFERENCE" not in msg_str
                    and "FILE_REFERENCE_EXPIRED" not in msg_str
                ):
                    raise
                refreshed = True
                continue
    finally:
        from stream import release_stream_client

        await release_stream_client(client_user_id)


async def record_user_history(user_id: int, track_id: str, played_at: float) -> None:
    try:
        col = db_handler.get_collection("userHistory").collection
        doc = {
            "user_id": int(user_id),
            "track_id": str(track_id),
            "played_at": float(played_at),
        }
        res = await col.insert_one(doc)
        LOG.debug(f"[userHistory] Inserted doc_id={res.inserted_id} for user_id={user_id} track_id={track_id}")
    except Exception as e:
        LOG.error(f"[userHistory] Failed to insert for user_id={user_id} track_id={track_id}: {e}", exc_info=True)


def _is_alac_track(doc: dict) -> bool:
    if not isinstance(doc, dict):
        return False
    audio = doc.get("audio") if isinstance(doc.get("audio"), dict) else {}
    telegram = doc.get("telegram") if isinstance(doc.get("telegram"), dict) else {}

    audio_type = str(audio.get("type") or "").lower().strip()
    audio_format = str(audio.get("format") or "").lower().strip()
    mime_type = str(telegram.get("mime_type") or "").lower().strip()
    file_name = str(telegram.get("file_name") or "").lower().strip()

    if "alac" in audio_type or "alac" in audio_format or "alac" in mime_type:
        return True
    if "alac" in file_name:
        return True
    return False


def _should_decode_alac(request: Request, doc: dict) -> bool:
    if not _is_alac_track(doc):
        return False

    fmt = (request.query_params.get("format") or "").lower().strip()
    if fmt in ("flac", "decoded", "transcode", "pcm", "wav"):
        return True
    decode_param = (
        request.query_params.get("decode")
        or request.query_params.get("transcode")
        or request.query_params.get("alac_decode")
        or ""
    ).lower().strip()
    if decode_param in ("1", "true", "yes", "flac"):
        return True

    if fmt in ("raw", "alac", "original", "source") or decode_param in ("0", "false", "no"):
        return False

    client_hdr = (
        request.headers.get("x-streamx-client")
        or request.headers.get("X-StreamX-Client")
        or ""
    ).lower().strip()
    if "native" in client_hdr or "android-native" in client_hdr or "exoplayer" in client_hdr:
        return False

    ua = (
        request.headers.get("user-agent") or request.headers.get("User-Agent") or ""
    ).lower().strip()
    sec_dest = (request.headers.get("sec-fetch-dest") or "").lower().strip()
    sec_mode = (request.headers.get("sec-fetch-mode") or "").lower().strip()
    sec_ua = (request.headers.get("sec-ch-ua") or "").lower().strip()
    sec_platform = (request.headers.get("sec-ch-ua-platform") or "").lower().strip()

    is_browser = bool(
        sec_dest
        or sec_mode
        or sec_ua
        or sec_platform
        or "mozilla" in ua
        or "chrome" in ua
        or "firefox" in ua
        or "safari" in ua
        or "edge" in ua
        or "edg" in ua
        or "opera" in ua
    )

    if not is_browser:
        return False

    is_apple_os = (
        ("macintosh" in ua or "mac os x" in ua or "iphone" in ua or "ipad" in ua or "ipod" in ua)
        and "windows" not in ua
        and "android" not in ua
    )
    if is_apple_os and ("safari" in ua or "applewebkit" in ua) and "chrome" not in ua and "edg" not in ua:
        return False

    is_windows = "windows" in ua or "win32" in ua or "win64" in ua or "windows" in sec_platform
    is_android = "android" in ua or "android" in sec_platform
    is_linux = "linux" in ua or "linux" in sec_platform

    if is_windows or is_android or is_linux:
        return True

    return True


def _prune_alac_cache_if_needed(max_files: int = 200, max_size_bytes: int = 5_000_000_000) -> None:
    try:
        if not os.path.isdir(ALAC_CACHE_DIR):
            return
        entries = []
        total_size = 0
        for fname in os.listdir(ALAC_CACHE_DIR):
            if not fname.endswith(".flac"):
                continue
            fpath = os.path.join(ALAC_CACHE_DIR, fname)
            try:
                stat = os.stat(fpath)
                entries.append((fpath, stat.st_mtime, stat.st_size))
                total_size += stat.st_size
            except OSError:
                continue

        if len(entries) <= max_files and total_size <= max_size_bytes:
            return

        entries.sort(key=lambda x: x[1])
        while entries and (len(entries) > max_files or total_size > max_size_bytes):
            oldest_path, _, oldest_size = entries.pop(0)
            try:
                os.remove(oldest_path)
                total_size -= oldest_size
            except OSError:
                pass
    except Exception as e:
        LOG.warning(f"[ALAC-Decode] Cache pruning error: {e}")


async def _get_alac_transcode_lock(track_id: str) -> asyncio.Lock:
    async with _ALAC_TRANSCODE_LOCKS_LOCK:
        lock = _ALAC_TRANSCODE_LOCKS.get(track_id)
        if lock is None:
            lock = asyncio.Lock()
            _ALAC_TRANSCODE_LOCKS[track_id] = lock
        return lock


async def _stream_cached_file(
    file_path: str,
    from_bytes: int,
    until_bytes: int | None,
    chunk_size: int = 256 * 1024,
) -> AsyncIterator[bytes]:
    cursor = int(max(0, from_bytes))
    target_end = None if until_bytes is None else int(until_bytes)
    async with aiofiles.open(file_path, "rb") as f:
        if cursor > 0:
            await f.seek(cursor)
        while True:
            read_size = chunk_size
            if target_end is not None:
                remaining = (target_end - cursor) + 1
                if remaining <= 0:
                    break
                read_size = min(read_size, remaining)
            chunk = await f.read(read_size)
            if not chunk:
                break
            cursor += len(chunk)
            yield chunk
            if target_end is not None and cursor > target_end:
                break


def _run_ffmpeg_transcode(src_file: str, dst_file: str) -> tuple[bool, str]:
    cmd = [
        "ffmpeg",
        "-hide_banner",
        "-loglevel", "error",
        "-y",
        "-i", src_file,
        "-vn",
        "-c:a", "flac",
        "-compression_level", "0",
        "-f", "flac",
        dst_file,
    ]
    res = subprocess.run(cmd, capture_output=True)
    return res.returncode == 0, res.stderr.decode("utf-8", errors="ignore")


async def _ensure_alac_decoded_file(track_id: str, doc: dict) -> str:
    os.makedirs(ALAC_CACHE_DIR, exist_ok=True)
    target_file = os.path.join(ALAC_CACHE_DIR, f"{track_id}.flac")

    if os.path.isfile(target_file) and os.path.getsize(target_file) > 10240:
        return target_file

    lock = await _get_alac_transcode_lock(track_id)
    async with lock:
        if os.path.isfile(target_file) and os.path.getsize(target_file) > 10240:
            return target_file

        temp_flac = os.path.join(
            ALAC_CACHE_DIR, f"{track_id}.tmp_{os.getpid()}_{int(time.time() * 1000)}.flac"
        )
        temp_src = os.path.join(
            ALAC_CACHE_DIR, f"{track_id}.src_{os.getpid()}_{int(time.time() * 1000)}.m4a"
        )

        telegram = doc.get("telegram") or {}
        primary_file_id = (telegram.get("file_id") or "").strip()
        playback_chat_id, playback_message_id = _playback_source_ids(doc)

        from stream import acquire_stream_client, release_stream_client

        client_user_id, client = await acquire_stream_client()
        try:
            client_key = str(int(client_user_id))
            file_ids = (
                telegram.get("file_ids")
                if isinstance(telegram.get("file_ids"), dict)
                else {}
            )
            file_id = (file_ids or {}).get(client_key) or ""
            if not file_id:
                file_id = await _ensure_client_file_id(
                    track_id=track_id,
                    client_user_id=int(client_user_id),
                    client=client,
                    source_chat_id=playback_chat_id,
                    source_message_id=playback_message_id,
                )

            target: str | object = file_id
            if playback_chat_id is not None and playback_message_id is not None:
                try:
                    msg = await client.get_messages(
                        int(playback_chat_id), int(playback_message_id)
                    )
                    if msg and _message_has_downloadable_media(msg):
                        target = msg
                except Exception:
                    pass

            LOG.debug(f"[ALAC-Decode] Fetching source media for track_id={track_id}")
            async with aiofiles.open(temp_src, "wb") as sf:
                async for chunk in client.stream_media(target):
                    if chunk:
                        await sf.write(chunk)

            LOG.debug(f"[ALAC-Decode] Transcoding {temp_src} -> {temp_flac}")
            ok, err_msg = await asyncio.to_thread(_run_ffmpeg_transcode, temp_src, temp_flac)
            if not ok or not os.path.isfile(temp_flac) or os.path.getsize(temp_flac) <= 10240:
                raise RuntimeError(f"FFmpeg transcode failed: {err_msg[:300]}")

            os.replace(temp_flac, target_file)
            LOG.debug(
                f"[ALAC-Decode] Transcode completed successfully: {target_file} ({os.path.getsize(target_file)} bytes)"
            )
            _prune_alac_cache_if_needed()
            return target_file

        finally:
            await release_stream_client(int(client_user_id))
            try:
                if os.path.isfile(temp_flac):
                    os.remove(temp_flac)
            except Exception:
                pass
            try:
                if os.path.isfile(temp_src):
                    os.remove(temp_src)
            except Exception:
                pass


async def stream_track(track_id: str, request: Request):
    if bool(getattr(Config, "ONLY_API", False)) or bot is None:
        raise HTTPException(status_code=503, detail="streaming disabled")
    col = get_audio_tracks_collection()
    doc = await col.find_one(
        {"_id": track_id},
        projection={
            "telegram": 1,
            "audio": 1,
            "source_chat_id": 1,
            "source_message_id": 1,
            "cache_chat_id": 1,
            "cache_message_id": 1,
            "deleted": 1,
        },
    )
    if not doc:
        raise HTTPException(status_code=404, detail="Track not found")
    if bool(doc.get("deleted")):
        raise HTTPException(status_code=404, detail="Track deleted")

    telegram = doc.get("telegram") or {}
    audio = doc.get("audio") if isinstance(doc.get("audio"), dict) else {}
    primary_file_id = (telegram.get("file_id") or "").strip()

    playback_chat_id, playback_message_id = _playback_source_ids(doc)

    mime_type = _normalize_mime_type(telegram.get("mime_type"), doc=doc)

    file_size: Optional[int] = None
    try:
        if telegram.get("file_size") is not None:
            file_size = int(telegram.get("file_size"))
    except Exception:
        file_size = None
    if file_size is None:
        try:
            if doc.get("file_size") is not None:
                file_size = int(doc.get("file_size"))
        except Exception:
            file_size = None
    if file_size is not None and int(file_size) <= 0:
        file_size = None

    duration_sec: float | None = None
    try:
        if audio.get("duration_sec") is not None:
            duration_sec = float(audio.get("duration_sec"))
    except Exception:
        duration_sec = None

    bitrate_kbps: int | None = None
    try:
        if audio.get("bitrate_kbps") is not None:
            bitrate_kbps = int(audio.get("bitrate_kbps"))
    except Exception:
        bitrate_kbps = None

    user_id: Optional[int] = None
    token = request.query_params.get("token") or request.headers.get("X-Auth-Token")
    if not token:
        auth_hdr = request.headers.get("Authorization") or ""
        if auth_hdr.startswith("Bearer "):
            token = auth_hdr[7:].strip()
    if token:
        try:
            payload = verify_auth_token(token)
            if isinstance(payload, dict):
                raw_uid = payload.get("uid") or payload.get("user_id")
                if raw_uid is not None:
                    try:
                        user_id = int(raw_uid)
                    except (ValueError, TypeError):
                        user_id = None
        except Exception:
            user_id = None

    range_header = (
        request.headers.get("range") or request.headers.get("Range") or ""
    ).strip()
    start_byte, end_byte = _parse_range_header(range_header)

    has_range = bool(range_header) and start_byte is not None
    from_bytes = int(start_byte or 0)

    if user_id and (request.method or "").upper() in ("GET", "HEAD"):
        now_ts = time.time()
        cache_key = (int(user_id), str(track_id))
        last_rec = _RECENTLY_RECORDED_HISTORY.get(cache_key, 0.0)
        if now_ts - last_rec > 15.0:
            _RECENTLY_RECORDED_HISTORY[cache_key] = now_ts
            asyncio.create_task(record_user_history(user_id, track_id, now_ts))

    should_decode = _should_decode_alac(request, doc)
    if should_decode:
        flac_path = None
        flac_file_size = None
        try:
            flac_path = await _ensure_alac_decoded_file(track_id, doc)
            if flac_path and os.path.isfile(flac_path):
                flac_file_size = os.path.getsize(flac_path)
        except Exception as e:
            LOG.error(
                f"[ALAC-Decode] Failed to ensure decoded FLAC file for track_id={track_id}: {e}",
                exc_info=True,
            )
            flac_path = None
            flac_file_size = None

        if flac_path and flac_file_size:
            mime_type = "audio/flac"
            file_size = flac_file_size

            until_bytes: int | None = None
            if has_range:
                if end_byte is not None:
                    until_bytes = int(end_byte)
                elif file_size is not None:
                    until_bytes = int(file_size) - 1
            if file_size is not None and until_bytes is not None:
                until_bytes = min(int(until_bytes), int(file_size) - 1)

            if file_size is not None and has_range and from_bytes >= file_size:
                raise HTTPException(status_code=416, detail="range not satisfiable")

            status_code = (
                206
                if (has_range and file_size is not None and until_bytes is not None)
                else 200
            )

            headers = {
                "Accept-Ranges": "bytes",
                "Content-Type": mime_type,
            }
            if status_code == 206 and file_size is not None and until_bytes is not None:
                headers["Content-Range"] = f"bytes {from_bytes}-{until_bytes}/{file_size}"
                headers["Content-Length"] = str((until_bytes - from_bytes) + 1)
            elif file_size is not None:
                headers["Content-Length"] = str(file_size)

            if (request.method or "").upper() == "HEAD":
                return Response(
                    content=b"", status_code=status_code, headers=headers, media_type=mime_type
                )

            iterator = _stream_cached_file(
                file_path=flac_path,
                from_bytes=from_bytes,
                until_bytes=until_bytes,
            )
            wrapped = _wrap_with_play_count(
                iterator=iterator,
                request=request,
                track_id=track_id,
                from_bytes=from_bytes if status_code == 206 else 0,
                file_size=file_size,
                duration_sec=duration_sec,
                bitrate_kbps=bitrate_kbps,
            )
            return StreamingResponse(
                wrapped, status_code=status_code, headers=headers, media_type=mime_type
            )

    until_bytes: int | None = None
    if has_range:
        if end_byte is not None:
            until_bytes = int(end_byte)
        elif file_size is not None:
            until_bytes = int(file_size) - 1
    if file_size is not None and until_bytes is not None:
        until_bytes = min(int(until_bytes), int(file_size) - 1)

    if file_size is not None and has_range and from_bytes >= file_size:
        raise HTTPException(status_code=416, detail="range not satisfiable")

    status_code = (
        206
        if (has_range and file_size is not None and until_bytes is not None)
        else 200
    )

    headers = {
        "Accept-Ranges": "bytes",
        "Content-Type": mime_type,
    }
    if status_code == 206 and file_size is not None and until_bytes is not None:
        headers["Content-Range"] = f"bytes {from_bytes}-{until_bytes}/{file_size}"
        if (request.method or "").upper() == "HEAD":
            headers["Content-Length"] = str((until_bytes - from_bytes) + 1)
    elif file_size is not None:
        if (request.method or "").upper() == "HEAD":
            headers["Content-Length"] = str(file_size)

    if (request.method or "").upper() == "HEAD":
        return Response(
            content=b"", status_code=status_code, headers=headers, media_type=mime_type
        )

    file_ids_for_pick = (
        telegram.get("file_ids") if isinstance(telegram.get("file_ids"), dict) else {}
    )
    preferred_user_ids: list[int] = []
    for k, v in (file_ids_for_pick or {}).items():
        if not isinstance(v, str) or not v.strip():
            continue
        try:
            preferred_user_ids.append(int(k))
        except Exception:
            pass

    from stream import (
        acquire_stream_client,
        acquire_stream_client_prefer,
        release_stream_client,
    )

    affinity_client_id: int | None = None
    now = time.monotonic()
    async with _TRACK_AFFINITY_LOCK:
        entry = _TRACK_AFFINITY.get(track_id)
        if entry:
            cid, ts = entry
            if (now - float(ts)) <= float(_TRACK_AFFINITY_TTL_SEC):
                affinity_client_id = int(cid)
            else:
                _TRACK_AFFINITY.pop(track_id, None)

    if preferred_user_ids:
        if affinity_client_id is not None and affinity_client_id in set(
            preferred_user_ids
        ):
            try:
                client_user_id, client = await acquire_stream_client_prefer(
                    [int(affinity_client_id)]
                )
            except Exception:
                client_user_id, client = await acquire_stream_client_prefer(
                    preferred_user_ids
                )
        else:
            client_user_id, client = await acquire_stream_client_prefer(
                preferred_user_ids
            )
    else:
        client_user_id, client = await acquire_stream_client()

    async with _TRACK_AFFINITY_LOCK:
        _TRACK_AFFINITY[track_id] = (int(client_user_id), time.monotonic())
    try:
        if preferred_user_ids:
            if affinity_client_id is not None and int(client_user_id) == int(
                affinity_client_id
            ):
                LOG.debug(
                    f"stream using affinity client track={track_id} client={client_user_id}"
                )
            else:
                LOG.debug(
                    f"stream picked preferred client track={track_id} client={client_user_id}"
                )
        else:
            LOG.debug(f"stream picked client track={track_id} client={client_user_id}")
        client_key = str(int(client_user_id))
        file_ids = (
            telegram.get("file_ids")
            if isinstance(telegram.get("file_ids"), dict)
            else {}
        )
        file_id = (file_ids or {}).get(client_key)
        if isinstance(file_id, str):
            file_id = file_id.strip()
        else:
            file_id = ""

        primary_uid = get_primary_client_user_id()
        if (
            not file_id
            and primary_uid is not None
            and int(primary_uid) == int(client_user_id)
            and primary_file_id
        ):
            file_id = primary_file_id
            LOG.debug(
                f"stream using primary file_id track={track_id} client={client_user_id} file_id={file_id}"
            )
            await get_audio_tracks_collection().update_one(
                {"_id": track_id},
                {
                    "$set": {
                        f"telegram.file_ids.{client_key}": file_id,
                    }
                },
            )
        elif file_id:
            LOG.debug(
                f"stream using cached file_id track={track_id} client={client_user_id} file_id={file_id}"
            )

        if not file_id:
            file_id = await _ensure_client_file_id(
                track_id=track_id,
                client_user_id=int(client_user_id),
                client=client,
                source_chat_id=playback_chat_id,
                source_message_id=playback_message_id,
            )
    except Exception:
        await release_stream_client(int(client_user_id))
        raise

    if status_code == 206:
        iterator = _stream_range(
            track_id=track_id,
            client_user_id=int(client_user_id),
            client=client,
            file_id=file_id,
            source_chat_id=playback_chat_id,
            source_message_id=playback_message_id,
            from_bytes=from_bytes,
            until_bytes=until_bytes,
        )
    else:
        iterator = _direct_stream(
            track_id=track_id,
            client_user_id=int(client_user_id),
            client=client,
            file_id=file_id,
            source_chat_id=playback_chat_id,
            source_message_id=playback_message_id,
            start_byte=0,
        )
    wrapped = _wrap_with_play_count(
        iterator=iterator,
        request=request,
        track_id=track_id,
        from_bytes=from_bytes if status_code == 206 else 0,
        file_size=file_size,
        duration_sec=duration_sec,
        bitrate_kbps=bitrate_kbps,
    )
    return StreamingResponse(
        wrapped, status_code=status_code, headers=headers, media_type=mime_type
    )


async def download_track(track_id: str, request: Request):
    if bool(getattr(Config, "ONLY_API", False)) or bot is None:
        raise HTTPException(status_code=503, detail="streaming disabled")
    col = get_audio_tracks_collection()
    doc = await col.find_one(
        {"_id": track_id},
        projection={
            "telegram": 1,
            "audio": 1,
            "file_size": 1,
            "source_chat_id": 1,
            "source_message_id": 1,
            "cache_chat_id": 1,
            "cache_message_id": 1,
            "deleted": 1,
        },
    )
    if not doc:
        raise HTTPException(status_code=404, detail="Track not found")
    if bool(doc.get("deleted")):
        raise HTTPException(status_code=404, detail="Track deleted")

    telegram = doc.get("telegram") or {}
    audio = doc.get("audio") if isinstance(doc.get("audio"), dict) else {}
    primary_file_id = (telegram.get("file_id") or "").strip()

    playback_chat_id, playback_message_id = _playback_source_ids(doc)

    if not primary_file_id and (
        playback_chat_id is None or playback_message_id is None
    ):
        raise HTTPException(status_code=404, detail="Track source unavailable")

    file_size: Optional[int] = None
    try:
        if telegram.get("file_size") is not None:
            file_size = int(telegram.get("file_size"))
    except Exception:
        file_size = None
    if file_size is None:
        try:
            if doc.get("file_size") is not None:
                file_size = int(doc.get("file_size"))
        except Exception:
            file_size = None
    if file_size is not None and int(file_size) <= 0:
        file_size = None

    mime_type = _normalize_mime_type(telegram.get("mime_type"), doc=doc)
    filename = _build_download_filename(
        track_id=track_id, audio=audio, telegram=telegram, mime_type=mime_type
    )

    range_header = (
        request.headers.get("range") or request.headers.get("Range") or ""
    ).strip()
    start_byte, end_byte = _parse_range_header(range_header)

    has_range = bool(range_header) and start_byte is not None
    from_bytes = int(start_byte or 0)

    should_decode = _should_decode_alac(request, doc)
    if should_decode:
        flac_path = None
        flac_file_size = None
        try:
            flac_path = await _ensure_alac_decoded_file(track_id, doc)
            if flac_path and os.path.isfile(flac_path):
                flac_file_size = os.path.getsize(flac_path)
        except Exception as e:
            LOG.error(
                f"[ALAC-Decode] Failed to ensure decoded FLAC file for download track_id={track_id}: {e}",
                exc_info=True,
            )
            flac_path = None
            flac_file_size = None

        if flac_path and flac_file_size:
            mime_type = "audio/flac"
            file_size = flac_file_size
            if filename.endswith(".m4a"):
                filename = filename[:-4] + ".flac"
            elif not filename.endswith(".flac"):
                filename = filename + ".flac"

            until_bytes: int | None = None
            if has_range:
                if end_byte is not None:
                    until_bytes = int(end_byte)
                elif file_size is not None:
                    until_bytes = int(file_size) - 1
            if file_size is not None and until_bytes is not None:
                until_bytes = min(int(until_bytes), int(file_size) - 1)

            if file_size is not None and has_range and from_bytes >= file_size:
                raise HTTPException(status_code=416, detail="range not satisfiable")

            status_code = (
                206
                if (has_range and file_size is not None and until_bytes is not None)
                else 200
            )

            cd = _content_disposition(filename)
            headers = {
                "Accept-Ranges": "bytes",
                "Content-Type": mime_type,
                "Content-Disposition": cd,
            }
            if status_code == 206 and file_size is not None and until_bytes is not None:
                headers["Content-Range"] = f"bytes {from_bytes}-{until_bytes}/{file_size}"
                headers["Content-Length"] = str((until_bytes - from_bytes) + 1)
            elif file_size is not None:
                headers["Content-Length"] = str(file_size)

            if (request.method or "").upper() == "HEAD":
                return Response(
                    content=b"", status_code=status_code, headers=headers, media_type=mime_type
                )

            iterator = _stream_cached_file(
                file_path=flac_path,
                from_bytes=from_bytes,
                until_bytes=until_bytes,
            )
            return StreamingResponse(
                iterator, status_code=status_code, headers=headers, media_type=mime_type
            )

    until_bytes: int | None = None
    if has_range:
        if end_byte is not None:
            until_bytes = int(end_byte)
        elif file_size is not None:
            until_bytes = int(file_size) - 1
    if file_size is not None and until_bytes is not None:
        until_bytes = min(int(until_bytes), int(file_size) - 1)

    if file_size is not None and has_range and from_bytes >= file_size:
        raise HTTPException(status_code=416, detail="range not satisfiable")

    status_code = (
        206
        if (has_range and file_size is not None and until_bytes is not None)
        else 200
    )

    headers = {
        "Accept-Ranges": "bytes",
        "Content-Type": mime_type,
        "Content-Disposition": _content_disposition_header(
            filename=filename, track_id=track_id, mime_type=mime_type
        ),
    }

    if status_code == 206 and file_size is not None and until_bytes is not None:
        headers["Content-Range"] = f"bytes {from_bytes}-{until_bytes}/{file_size}"
        headers["Content-Length"] = str((until_bytes - from_bytes) + 1)
    elif file_size is not None:
        headers["Content-Length"] = str(file_size)

    file_ids_for_pick = (
        telegram.get("file_ids") if isinstance(telegram.get("file_ids"), dict) else {}
    )
    preferred_user_ids: list[int] = []
    for k, v in (file_ids_for_pick or {}).items():
        if not isinstance(v, str) or not v.strip():
            continue
        try:
            preferred_user_ids.append(int(k))
        except Exception:
            pass

    from stream import (
        acquire_stream_client,
        acquire_stream_client_prefer,
        release_stream_client,
    )

    if preferred_user_ids:
        client_user_id, client = await acquire_stream_client_prefer(preferred_user_ids)
    else:
        client_user_id, client = await acquire_stream_client()

    try:
        client_key = str(int(client_user_id))
        file_ids = (
            telegram.get("file_ids")
            if isinstance(telegram.get("file_ids"), dict)
            else {}
        )
        file_id = (file_ids or {}).get(client_key)
        if isinstance(file_id, str):
            file_id = file_id.strip()
        else:
            file_id = ""

        primary_uid = get_primary_client_user_id()
        if (
            not file_id
            and primary_uid is not None
            and int(primary_uid) == int(client_user_id)
            and primary_file_id
        ):
            file_id = primary_file_id
            await get_audio_tracks_collection().update_one(
                {"_id": track_id},
                {
                    "$set": {
                        f"telegram.file_ids.{client_key}": file_id,
                    }
                },
            )

        if not file_id:
            file_id = await _ensure_client_file_id(
                track_id=track_id,
                client_user_id=int(client_user_id),
                client=client,
                source_chat_id=playback_chat_id,
                source_message_id=playback_message_id,
            )
    except Exception:
        await release_stream_client(int(client_user_id))
        raise

    if status_code == 206:
        iterator = _stream_range(
            track_id=track_id,
            client_user_id=int(client_user_id),
            client=client,
            file_id=file_id,
            source_chat_id=playback_chat_id,
            source_message_id=playback_message_id,
            from_bytes=from_bytes,
            until_bytes=until_bytes,
        )
    else:
        iterator = _direct_stream(
            track_id=track_id,
            client_user_id=int(client_user_id),
            client=client,
            file_id=file_id,
            source_chat_id=playback_chat_id,
            source_message_id=playback_message_id,
            start_byte=0,
        )

    return StreamingResponse(
        iterator, status_code=status_code, headers=headers, media_type=mime_type
    )


async def warm_track_cached(track_id: str) -> dict:
    if bool(getattr(Config, "ONLY_API", False)) or bot is None:
        raise HTTPException(status_code=503, detail="streaming disabled")
    track_id = (track_id or "").strip()
    if not track_id:
        raise HTTPException(status_code=400, detail="track_id is required")

    col = get_audio_tracks_collection()
    doc = await col.find_one(
        {"_id": track_id},
        projection={
            "telegram": 1,
            "audio": 1,
            "source_chat_id": 1,
            "source_message_id": 1,
            "cache_chat_id": 1,
            "cache_message_id": 1,
        },
    )
    if not doc:
        raise HTTPException(status_code=404, detail="Track not found")

    if _is_alac_track(doc):
        flac_file = os.path.join(ALAC_CACHE_DIR, f"{track_id}.flac")
        is_ready = os.path.isfile(flac_file) and os.path.getsize(flac_file) > 10240
        if not is_ready:
            asyncio.create_task(_ensure_alac_decoded_file(track_id, doc))
        return {"ok": True, "ready": is_ready}

    playback_chat_id, playback_message_id = _playback_source_ids(doc)

    # LIGHTWEIGHT WARMING:
    # Instead of starting a Hub (which starts a producer task and consumes a client),
    # we just ensure the file_id is resolved and cached for the primary client.
    # This makes the eventual stream start much faster without 'Streaming started' noise.
    from stream import acquire_stream_client, release_stream_client

    # We only warm for the primary client to avoid exhausting others.
    client_id, client = await acquire_stream_client()
    try:
        await _ensure_client_file_id(
            track_id=track_id,
            client_user_id=int(client_id),
            client=client,
            source_chat_id=playback_chat_id,
            source_message_id=playback_message_id,
        )
    finally:
        await release_stream_client(client_id)

    return {"ok": True}
