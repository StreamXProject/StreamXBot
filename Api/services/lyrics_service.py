import asyncio
import base64
import re
import xml.etree.ElementTree as ET

import aiohttp
from fastapi import HTTPException

from Api.deps.db import get_audio_tracks_collection
from Api.services.musixmatch import fetch_track_lyrics_from_musixmatch
from stream.core.config_manager import Config
from stream.helpers.logger import LOGGER
from stream.plugins.db.lyrics import fetch_best_lyrics
from stream.plugins.db.telegraph import publish_lyrics_text_to_graph

_TELEGRAPH_TASKS: dict[str, asyncio.Task] = {}
_TELEGRAPH_TASKS_LOCK = asyncio.Lock()

LOG = LOGGER(__name__)


def _parse_time_ms(time_str: str) -> int:
    if not time_str:
        return 0
    if ":" in time_str:
        parts = time_str.split(":")
        if len(parts) == 3:
            h, m, s = float(parts[0]), float(parts[1]), float(parts[2])
            return int((h * 3600 + m * 60 + s) * 1000)
        elif len(parts) == 2:
            m, s = float(parts[0]), float(parts[1])
            return int((m * 60 + s) * 1000)
    try:
        return int(float(time_str) * 1000)
    except Exception:
        return 0


def _format_tag(ms: int, open_ch: str = "[") -> str:
    total_sec = ms // 1000
    m = total_sec // 60
    s = total_sec % 60
    millis = ms % 1000
    close_ch = ">" if open_ch == "<" else "]"
    return f"{open_ch}{m:02d}:{s:02d}.{millis:03d}{close_ch}"


def _ttml_to_enhanced_lrc(ttml: str) -> str | None:
    """Ported from piTube TtmlParser: converts Apple Music TTML to enhanced LRC with syllable merging."""
    if not ttml or "<tt" not in ttml:
        return None
    try:
        clean = re.sub(r"<(/)?([a-zA-Z0-9_-]+):", r"<\1", ttml)
        clean = re.sub(r"\s+([a-zA-Z0-9_-]+):([a-zA-Z0-9_-]+)=", r" \2=", clean)
        clean = re.sub(r'\sxmlns(:\w+)?=\"[^\"]+\"', "", clean)
        root = ET.fromstring(clean)

        lines = []
        for p in root.iter("p"):
            begin = p.attrib.get("begin", "")
            if not begin:
                continue
            line_ms = _parse_time_ms(begin)

            spans = []
            for span in p.iter("span"):
                role = span.attrib.get("role", "")
                if role in ("x-bg", "x-translation", "x-roman"):
                    continue
                wb = span.attrib.get("begin", "")
                we = span.attrib.get("end", "")
                txt = (span.text or "").strip()
                if txt and wb:
                    has_trailing_space = bool(span.tail and re.search(r"\s", span.tail))
                    spans.append({
                        "text": txt,
                        "start": _parse_time_ms(wb),
                        "end": _parse_time_ms(we),
                        "space": has_trailing_space,
                    })

            # Merge syllables into words (matching piTube's TtmlParser.mergeSpans)
            words = []
            curr_text = ""
            curr_start = 0
            for i, s in enumerate(spans):
                if i == 0:
                    curr_text = s["text"]
                    curr_start = s["start"]
                elif spans[i - 1]["space"]:
                    if curr_text:
                        words.append((curr_start, curr_text))
                    curr_text = s["text"]
                    curr_start = s["start"]
                else:
                    curr_text += s["text"]
            if curr_text:
                words.append((curr_start, curr_text))

            line_tag = _format_tag(line_ms, "[")
            if words:
                word_str = " ".join(f"{_format_tag(w_ms, '<')}{w_txt}" for w_ms, w_txt in words)
                lines.append(f"{line_tag} {word_str}")
            else:
                direct = "".join(p.itertext()).strip()
                if direct:
                    lines.append(f"{line_tag} {direct}")
        return "\n".join(lines).strip() if lines else None
    except Exception as e:
        if bool(getattr(Config, "DEBUG", False)):
            LOG.debug(f"[lyrics] ttml parsing error: {e}")
        return None

def _clean_telegraph_url(value: str) -> str:
    s = (value or "").strip()
    if s.startswith("`") and s.endswith("`") and len(s) >= 2:
        s = s[1:-1].strip()
    if s.startswith("'") and s.endswith("'") and len(s) >= 2:
        s = s[1:-1].strip()
    if s.startswith('"') and s.endswith('"') and len(s) >= 2:
        s = s[1:-1].strip()
    return s


def _extract_cached_lyrics(doc: dict) -> dict | None:
    cache = doc.get("lyrics_cache") if isinstance(doc.get("lyrics_cache"), dict) else {}
    text = cache.get("text")
    if not isinstance(text, str) or not text.strip():
        return None
    kind = cache.get("kind")
    source = cache.get("source")
    out = {
        "lyrics": text,
        "kind": str(kind) if isinstance(kind, str) and kind.strip() else None,
        "source": str(source) if isinstance(source, str) and source.strip() else None,
    }
    return out


async def _publish_telegraph_and_store(
    *,
    track_id: str,
    title: str,
    artist: str,
    album: str | None,
    lyrics: str,
) -> None:
    try:
        tg = await publish_lyrics_text_to_graph(
            track_id=track_id,
            title=title,
            artist=artist,
            album=album,
            lyrics=lyrics,
        )
    except Exception:
        tg = None

    url = ""
    if isinstance(tg, dict):
        url = _clean_telegraph_url(str(tg.get("url") or ""))
    if not url:
        return

    try:
        col = get_audio_tracks_collection()
        await col.update_one({"_id": track_id}, {"$set": {"lyrics": url}}, upsert=False)
    except Exception:
        return


async def _ensure_telegraph_background(
    *,
    track_id: str,
    title: str,
    artist: str,
    album: str | None,
    lyrics: str,
) -> None:
    track_id = (track_id or "").strip()
    if not track_id:
        return

    async with _TELEGRAPH_TASKS_LOCK:
        t = _TELEGRAPH_TASKS.get(track_id)
        if t and not t.done():
            return

        task = asyncio.create_task(
            _publish_telegraph_and_store(
                track_id=track_id,
                title=title,
                artist=artist,
                album=album,
                lyrics=lyrics,
            )
        )
        _TELEGRAPH_TASKS[track_id] = task

        def _done(_t: asyncio.Task):
            async def _cleanup():
                async with _TELEGRAPH_TASKS_LOCK:
                    cur = _TELEGRAPH_TASKS.get(track_id)
                    if cur is _t:
                        _TELEGRAPH_TASKS.pop(track_id, None)

            asyncio.create_task(_cleanup())

        task.add_done_callback(_done)


async def get_track_lyrics(track_id: str, provider: str | None = None) -> dict:
    track_id = (track_id or "").strip()
    if not track_id:
        raise HTTPException(status_code=400, detail="track_id is required")

    col = get_audio_tracks_collection()
    doc = await col.find_one(
        {"_id": track_id},
        projection={"audio": 1, "telegram": 1, "spotify": 1, "lyrics": 1, "lyrics_cache": 1, "titles": 1},
    )
    if not doc:
        raise HTTPException(status_code=404, detail="track not found")

    audio = doc.get("audio") if isinstance(doc.get("audio"), dict) else {}
    telegram = doc.get("telegram") if isinstance(doc.get("telegram"), dict) else {}

    title = (audio.get("title") or "").strip() or (telegram.get("title") or "").strip()
    artist = (audio.get("artist") or "").strip() or (audio.get("performer") or "").strip() or (telegram.get("artist") or "").strip()
    album = (audio.get("album") or "").strip() or (telegram.get("album") or "").strip()
    duration_sec = int(audio.get("duration") or telegram.get("duration") or 0)

    if not title:
        return {"ok": False, "error": "missing_title"}

    # If titles (romanized / translations) not yet indexed, enrich in background via Musixmatch
    if not doc.get("titles") and bool(getattr(Config, "MUSIXMATCH", True)):
        try:
            from Api.services.musixmatch import fetch_and_save_musixmatch_titles
            asyncio.create_task(fetch_and_save_musixmatch_titles(track_id=track_id, track=doc))
        except Exception:
            pass

    provider_clean = (provider or "").strip().lower()
    cached_url = _clean_telegraph_url(doc.get("lyrics") if isinstance(doc.get("lyrics"), str) else "")
    cached = _extract_cached_lyrics(doc)

    # Use cache if no specific provider was forced, or if cached source matches the requested provider
    if cached and (not provider_clean or provider_clean == "auto" or cached.get("source") == provider_clean):
        out = {"ok": True, "track_id": track_id, "lyrics": cached["lyrics"]}
        if cached.get("kind"):
            out["kind"] = cached["kind"]
        if cached.get("source"):
            out["source"] = cached["source"]
        if cached_url:
            out["telegraph_url"] = cached_url
        else:
            try:
                asyncio.create_task(
                    _ensure_telegraph_background(
                        track_id=track_id,
                        title=title,
                        artist=artist or "Unknown",
                        album=album or None,
                        lyrics=str(cached["lyrics"] or ""),
                    )
                )
            except Exception:
                pass
        return out

    async def _save_to_cache(res_dict: dict) -> None:
        try:
            col_t = get_audio_tracks_collection()
            update_fields: dict[str, Any] = {
                "lyrics_cache.text": str(res_dict.get("lyrics") or ""),
                "lyrics_cache.kind": str(res_dict.get("kind") or ""),
                "lyrics_cache.source": str(res_dict.get("source") or "unknown"),
                "lyrics_cache.updated_at": __import__("time").time(),
            }
            if res_dict.get("titles"):
                update_fields["titles"] = res_dict["titles"]
                update_fields["audio.titles"] = res_dict["titles"]
            await col_t.update_one(
                {"_id": track_id},
                {"$set": update_fields},
                upsert=False,
            )
            if cached_url:
                res_dict["telegraph_url"] = cached_url
            else:
                asyncio.create_task(
                    _ensure_telegraph_background(
                        track_id=track_id,
                        title=title,
                        artist=artist or "Unknown",
                        album=album or None,
                        lyrics=str(res_dict.get("lyrics") or ""),
                    )
                )
            res_dict["telegraph_url"] = _clean_telegraph_url(str(res_dict.get("telegraph_url") or ""))
            telegraph_url = (res_dict.get("telegraph_url") or "").strip()
            if telegraph_url and telegraph_url != cached_url:
                await col_t.update_one(
                    {"_id": track_id},
                    {"$set": {"lyrics": telegraph_url}},
                    upsert=False,
                )
        except Exception:
            pass

    async def _try_betterlyrics() -> dict | None:
        """BetterLyrics: Word-timed TTML from lyrics-api.boidu.dev (piTube Apple Music provider)."""
        try:
            if bool(getattr(Config, "DEBUG", False)):
                LOG.debug(f"[lyrics] method=betterlyrics track_id={track_id!r} title={title!r} artist={artist!r}")
            params = {"s": title, "a": artist}
            if album:
                params["al"] = album
            if duration_sec > 0:
                params["d"] = str(duration_sec)

            async with aiohttp.ClientSession(headers={"User-Agent": "piTube/1.0"}) as session:
                async with session.get(
                    "https://lyrics-api.boidu.dev/getLyrics",
                    params=params,
                    timeout=aiohttp.ClientTimeout(total=8),
                ) as resp:
                    if resp.status != 200:
                        return None
                    data = await resp.json(content_type=None)
                    ttml = data.get("ttml") if isinstance(data, dict) else None
                    if not ttml or not isinstance(ttml, str):
                        return None

                    enhanced_lrc = _ttml_to_enhanced_lrc(ttml)
                    if not enhanced_lrc:
                        return None

                    res = {
                        "ok": True,
                        "track_id": track_id,
                        "lyrics": enhanced_lrc,
                        "kind": "richsync",
                        "source": "betterlyrics",
                    }
                    await _save_to_cache(res)
                    return res
        except Exception as e:
            if bool(getattr(Config, "DEBUG", False)):
                LOG.debug(f"[lyrics] betterlyrics failed: {e}")
        return None

    async def _try_musixmatch() -> dict | None:
        """Musixmatch: RichSync word timestamps and line-synced lyrics."""
        try:
            if bool(getattr(Config, "DEBUG", False)):
                LOG.debug(f"[lyrics] method=musixmatch track_id={track_id!r}")
            mxm = await fetch_track_lyrics_from_musixmatch(track=doc)
            if mxm.get("ok"):
                mxm["track_id"] = track_id
                await _save_to_cache(mxm)
                return mxm
        except Exception as e:
            if bool(getattr(Config, "DEBUG", False)):
                LOG.debug(f"[lyrics] musixmatch failed: {e}")
        return None

    async def _try_lrclib() -> dict | None:
        """LRCLIB: Fast open-source synchronized LRC lyrics."""
        try:
            if bool(getattr(Config, "DEBUG", False)):
                LOG.debug(f"[lyrics] method=lrclib track_id={track_id!r}")
            res = await fetch_best_lyrics(title=title, artist=artist or None, album=album or None)
            if res.get("ok"):
                res["track_id"] = track_id
                await _save_to_cache(res)
                return res
        except Exception as e:
            if bool(getattr(Config, "DEBUG", False)):
                LOG.debug(f"[lyrics] lrclib failed: {e}")
        return None

    async def _try_kugou() -> dict | None:
        """KuGou: Large synced lyrics catalog ported from piTube."""
        try:
            if bool(getattr(Config, "DEBUG", False)):
                LOG.debug(f"[lyrics] method=kugou track_id={track_id!r} title={title!r} artist={artist!r}")
            clean_title = re.sub(r"\(.*?\)|\[.*?\]", "", title).strip()
            clean_artist = re.sub(r"\(.*?\)|\[.*?\]", "", artist).strip()
            query = f"{clean_title} - {clean_artist}".strip()
            if not query:
                return None

            async with aiohttp.ClientSession(headers={"User-Agent": "piTube/1.0"}) as session:
                async with session.get(
                    "https://mobileservice.kugou.com/api/v3/search/song",
                    params={"version": "9108", "plat": "0", "pagesize": "6", "showtype": "0", "keyword": query},
                    timeout=aiohttp.ClientTimeout(total=8),
                ) as r1:
                    if r1.status != 200:
                        return None
                    data1 = await r1.json(content_type=None)
                    info = (data1.get("data") or {}).get("info") or []
                    if not info:
                        return None

                    target_hash = None
                    for song in info:
                        if duration_sec > 0 and abs(int(song.get("duration") or 0) - duration_sec) > 8:
                            continue
                        h = (song.get("hash") or "").strip()
                        if h:
                            target_hash = h
                            break
                    if not target_hash and info:
                        target_hash = (info[0].get("hash") or "").strip()
                    if not target_hash:
                        return None

                async with session.get(
                    "https://lyrics.kugou.com/search",
                    params={"ver": "1", "man": "yes", "client": "pc", "hash": target_hash},
                    timeout=aiohttp.ClientTimeout(total=8),
                ) as r2:
                    if r2.status != 200:
                        return None
                    data2 = await r2.json(content_type=None)
                    cands = data2.get("candidates") or []
                    if not cands:
                        return None
                    cand_id = cands[0].get("id")
                    accesskey = cands[0].get("accesskey")
                    if not cand_id or not accesskey:
                        return None

                async with session.get(
                    "https://lyrics.kugou.com/download",
                    params={"fmt": "lrc", "charset": "utf8", "client": "pc", "ver": "1", "id": str(cand_id), "accesskey": str(accesskey)},
                    timeout=aiohttp.ClientTimeout(total=8),
                ) as r3:
                    if r3.status != 200:
                        return None
                    data3 = await r3.json(content_type=None)
                    raw_content = data3.get("content") or ""
                    if not raw_content:
                        return None
                    decoded = base64.b64decode(raw_content).decode("utf-8", errors="ignore").strip()
                    if not decoded:
                        return None

                    res = {
                        "ok": True,
                        "track_id": track_id,
                        "lyrics": decoded,
                        "kind": "synced",
                        "source": "kugou",
                    }
                    await _save_to_cache(res)
                    return res
        except Exception as e:
            if bool(getattr(Config, "DEBUG", False)):
                LOG.debug(f"[lyrics] kugou failed: {e}")
        return None

    # Priority ordering based on requested provider
    providers_map = {
        "betterlyrics": _try_betterlyrics,
        "musixmatch": _try_musixmatch,
        "lrclib": _try_lrclib,
        "kugou": _try_kugou,
    }

    if provider_clean in providers_map:
        # User specified an explicit provider - try that first, then fall back
        chain = [providers_map[provider_clean]] + [fn for k, fn in providers_map.items() if k != provider_clean]
    else:
        # Auto priority order: BetterLyrics (Apple Music word-timed) -> Musixmatch (RichSync) -> LRCLIB -> KuGou
        chain = [_try_betterlyrics, _try_musixmatch, _try_lrclib, _try_kugou]

    for try_fn in chain:
        r = await try_fn()
        if r and r.get("ok"):
            return r

    return {"ok": False, "error": "no_lyrics_found", "track_id": track_id}
