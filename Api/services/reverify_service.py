import asyncio
import time
from typing import Any, Callable, Coroutine

from Api.deps.db import get_audio_tracks_collection
from Api.services.musixmatch import fetch_and_save_musixmatch_titles
from stream.core.config_manager import Config
from stream.helpers.logger import LOGGER

LOG = LOGGER(__name__)

_JOB_LOCK = asyncio.Lock()
_CURRENT_JOB: dict[str, Any] | None = None
_CANCEL_EVENT: asyncio.Event | None = None


def get_reverify_status() -> dict[str, Any]:
    """Retrieve status of active or most recently completed reverify job."""
    if _CURRENT_JOB is not None:
        return dict(_CURRENT_JOB)
    return {"running": False, "status": "idle"}


def cancel_reverify_job() -> bool:
    """Request cancellation of currently running reverify job."""
    global _CANCEL_EVENT
    if _CURRENT_JOB and _CURRENT_JOB.get("running") and _CANCEL_EVENT:
        _CANCEL_EVENT.set()
        if _CURRENT_JOB:
            _CURRENT_JOB["status"] = "cancelling"
        return True
    return False


async def reverify_tracks(
    *,
    force_all: bool = False,
    missing_titles: bool = True,
    missing_lyrics: bool = False,
    unenriched_only: bool = False,
    limit: int = 0,
    concurrency: int = 2,
    progress_callback: Callable[[dict[str, Any]], Coroutine[Any, Any, None]] | None = None,
) -> dict[str, Any]:
    """
    Scan audioTracks collection and re-verify metadata from Musixmatch/Lyrics providers.
    - missing_titles: checks tracks missing 'titles' or 'audio.titles'.
    - missing_lyrics: checks tracks missing lyrics.
    - unenriched_only: checks tracks with enriched: False.
    - force_all: re-checks all tracks regardless of current state.
    """
    global _CURRENT_JOB, _CANCEL_EVENT

    async with _JOB_LOCK:
        if _CURRENT_JOB and _CURRENT_JOB.get("running"):
            return {
                "ok": False,
                "error": "already_running",
                "current_job": dict(_CURRENT_JOB),
            }

        _CANCEL_EVENT = asyncio.Event()
        start_ts = time.time()
        _CURRENT_JOB = {
            "running": True,
            "status": "scanning",
            "started_at": start_ts,
            "total": 0,
            "processed": 0,
            "titles_added": 0,
            "lyrics_added": 0,
            "errors": 0,
            "elapsed_sec": 0.0,
        }

    try:
        col = get_audio_tracks_collection()
        query: dict[str, Any] = {"deleted": {"$ne": True}}

        if not force_all:
            or_conditions: list[dict[str, Any]] = []
            if unenriched_only:
                or_conditions.append({"enriched": False})
            else:
                if missing_titles:
                    or_conditions.append({"titles": {"$exists": False}})
                    or_conditions.append({"titles": None})
                    or_conditions.append({"audio.titles": {"$exists": False}})
                if missing_lyrics:
                    or_conditions.append({"lyrics": {"$exists": False}})
                    or_conditions.append({"lyrics": None})
                    or_conditions.append({"lyrics": ""})

            if or_conditions:
                query["$or"] = or_conditions

        total = await col.count_documents(query)
        if limit and limit > 0:
            total = min(total, limit)

        _CURRENT_JOB["total"] = total
        _CURRENT_JOB["status"] = "processing"

        if progress_callback:
            try:
                await progress_callback(dict(_CURRENT_JOB))
            except Exception:
                pass

        if total == 0:
            elapsed = round(time.time() - start_ts, 2)
            _CURRENT_JOB.update({"running": False, "status": "completed", "elapsed_sec": elapsed})
            return {"ok": True, **dict(_CURRENT_JOB)}

        cursor = col.find(
            query,
            projection={
                "_id": 1,
                "audio": 1,
                "telegram": 1,
                "spotify": 1,
                "titles": 1,
                "lyrics": 1,
                "lyrics_cache": 1,
                "enriched": 1,
            },
        ).sort([("updated_at", -1)])

        if limit and limit > 0:
            cursor = cursor.limit(limit)

        sem = asyncio.Semaphore(max(1, concurrency))
        processed = 0
        titles_added = 0
        lyrics_added = 0
        errors = 0
        last_progress_ts = time.time()

        async def _process_track(doc: dict):
            nonlocal processed, titles_added, lyrics_added, errors, last_progress_ts
            if _CANCEL_EVENT and _CANCEL_EVENT.is_set():
                return

            tid = str(doc.get("_id") or "").strip()
            if not tid:
                return

            async with sem:
                if _CANCEL_EVENT and _CANCEL_EVENT.is_set():
                    return

                try:
                    has_titles = bool(doc.get("titles") or (doc.get("audio") or {}).get("titles"))
                    if (not has_titles or force_all) and bool(getattr(Config, "MUSIXMATCH", True)):
                        res_titles = await fetch_and_save_musixmatch_titles(track_id=tid, track=doc)
                        if res_titles:
                            titles_added += 1

                    has_lyrics = bool(
                        (doc.get("lyrics") and str(doc["lyrics"]).strip())
                        or (doc.get("lyrics_cache") or {}).get("text")
                    )
                    if (not has_lyrics or force_all):
                        try:
                            from Api.services.lyrics_service import get_track_lyrics

                            lyr = await get_track_lyrics(tid)
                            if lyr and lyr.get("ok"):
                                lyrics_added += 1
                        except Exception:
                            pass

                    if doc.get("enriched") is False:
                        await col.update_one(
                            {"_id": tid},
                            {
                                "$unset": {
                                    "enrichment_error": "",
                                    "enrichment_error_at": "",
                                    "enrich_retry_after": "",
                                }
                            },
                        )

                    await asyncio.sleep(0.25)

                except Exception as e:
                    errors += 1
                    if bool(getattr(Config, "DEBUG", False)):
                        LOG.debug(f"[reverify] failed track {tid}: {e}")
                finally:
                    processed += 1
                    now = time.time()
                    _CURRENT_JOB.update(
                        {
                            "processed": processed,
                            "titles_added": titles_added,
                            "lyrics_added": lyrics_added,
                            "errors": errors,
                            "elapsed_sec": round(now - start_ts, 2),
                        }
                    )

                    if progress_callback and (now - last_progress_ts >= 3.0 or processed >= total):
                        last_progress_ts = now
                        try:
                            await progress_callback(dict(_CURRENT_JOB))
                        except Exception:
                            pass

        tasks = []
        async for doc in cursor:
            if _CANCEL_EVENT and _CANCEL_EVENT.is_set():
                break
            tasks.append(asyncio.create_task(_process_track(doc)))

        if tasks:
            await asyncio.gather(*tasks, return_exceptions=True)

        is_cancelled = bool(_CANCEL_EVENT and _CANCEL_EVENT.is_set())
        status_str = "cancelled" if is_cancelled else "completed"
        elapsed = round(time.time() - start_ts, 2)

        _CURRENT_JOB.update(
            {
                "running": False,
                "status": status_str,
                "elapsed_sec": elapsed,
            }
        )

        if progress_callback:
            try:
                await progress_callback(dict(_CURRENT_JOB))
            except Exception:
                pass

        return {"ok": True, **dict(_CURRENT_JOB)}

    finally:
        if _CURRENT_JOB:
            _CURRENT_JOB["running"] = False
