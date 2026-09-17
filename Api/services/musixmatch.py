import asyncio
import json
import os
from pathlib import Path
import time
from typing import Any

import httpx

from stream.core.config_manager import Config

_BASE_URL = "https://apic.musixmatch.com/ws/1.1"
_APP_ID = "mac-ios-v2.0"
_TOKEN_FILE = Path(__file__).resolve().parent.parent.parent / "cookies" / ".musixmatch_token"

_token_lock = asyncio.Lock()
_cached_user_token: str | None = None
_cached_user_token_ts: float = 0.0

_DEFAULT_HEADERS = {
    "Accept": "application/json, text/plain;q=0.9, */*;q=0.8",
    "User-Agent": "Mozilla/5.0",
}

def _safe_json(resp: httpx.Response) -> Any:
    try:
        return resp.json()
    except Exception:
        pass
    try:
        text = resp.content.decode("utf-8-sig", errors="replace")
        return json.loads(text)
    except Exception:
        return {}


def _sync_get_json(*, url: str, params: dict[str, Any]) -> tuple[int, Any]:
    with httpx.Client(timeout=10, follow_redirects=True, headers=_DEFAULT_HEADERS) as client:
        r = client.get(url, params=params)
    return r.status_code, _safe_json(r)


def _extract_header_status(payload: Any) -> int | None:
    try:
        v = (((payload or {}).get("message") or {}).get("header") or {}).get("status_code")
        return int(v)
    except Exception:
        return None


def _pick_spotify_track_id(track: dict) -> str | None:
    spotify = track.get("spotify") if isinstance(track.get("spotify"), dict) else {}
    for k in ("track_spotify_id", "spotify_track_id", "spotify_id", "track_id", "id"):
        v = spotify.get(k)
        if isinstance(v, str) and v.strip():
            return v.strip()

    url = spotify.get("url") or spotify.get("spotify_url")
    if isinstance(url, str) and url.strip():
        s = url.strip()
        marker = "/track/"
        if marker in s:
            tail = s.split(marker, 1)[1]
            tid = tail.split("?", 1)[0].split("/", 1)[0].strip()
            if tid:
                return tid
    return None


def _lrc_timestamp(total_seconds: float) -> str:
    try:
        t = float(total_seconds)
    except Exception:
        t = 0.0
    if t < 0:
        t = 0.0
    mm = int(t // 60)
    sec_f = t - (mm * 60)
    ss = int(sec_f // 1)
    hh = int(round((sec_f - ss) * 100))
    if hh >= 100:
        hh = 0
        ss += 1
    if ss >= 60:
        ss = 0
        mm += 1
    return f"[{mm:02d}:{ss:02d}.{hh:02d}]"


def _word_timestamp(total_seconds: float) -> str:
    try:
        t = float(total_seconds)
    except Exception:
        t = 0.0
    if t < 0:
        t = 0.0
    mm = int(t // 60)
    sec_f = t - (mm * 60)
    ss = int(sec_f // 1)
    hh = int(round((sec_f - ss) * 100))
    if hh >= 100:
        hh = 0
        ss += 1
    if ss >= 60:
        ss = 0
        mm += 1
    return f"<{mm:02d}:{ss:02d}.{hh:02d}>"


def _richsync_json_to_lrc(richsync_body: str) -> str | None:
    s = (richsync_body or "").strip()
    if not s:
        return None
    try:
        items = json.loads(s) if isinstance(s, str) else s
    except Exception:
        return None
    if not isinstance(items, list) or not items:
        return None
    out: list[str] = []
    for it in items:
        if not isinstance(it, dict):
            continue
        line_text = (it.get("x") or "").strip()
        words = it.get("l")
        ts_val = it.get("ts")
        try:
            line_ts = float(ts_val) if ts_val is not None else 0.0
        except Exception:
            line_ts = 0.0
        if not line_text and not words:
            continue
        line_tag = _lrc_timestamp(line_ts)
        if isinstance(words, list) and words:
            parts = [line_tag]
            for w in words:
                if not isinstance(w, dict):
                    continue
                c = w.get("c") or ""
                offset = w.get("o")
                try:
                    word_offset = float(offset) if offset is not None else 0.0
                except Exception:
                    word_offset = 0.0
                if not c.strip():
                    parts.append(c)
                else:
                    parts.append(f"{_word_timestamp(line_ts + word_offset)}{c}")
            out.append("".join(parts).rstrip())
        else:
            out.append(f"{line_tag} {line_text}".rstrip())
    joined = "\n".join(out).strip()
    return joined or None


def _subtitles_json_to_lrc(subtitle_body: str) -> str | None:
    s = (subtitle_body or "").strip()
    if not s:
        return None
    try:
        items = json.loads(s)
    except Exception:
        return None
    if not isinstance(items, list) or not items:
        return None
    out: list[str] = []
    for it in items:
        if not isinstance(it, dict):
            continue
        text = it.get("text")
        if not isinstance(text, str):
            text = ""
        time_obj = it.get("time") if isinstance(it.get("time"), dict) else {}
        total = time_obj.get("total")
        ts = _lrc_timestamp(float(total) if total is not None else 0.0)
        text = text.rstrip()
        if text:
            out.append(f"{ts} {text}")
        else:
            out.append(ts)
    joined = "\n".join(out).strip()
    return joined or None


def _extract_plain_lyrics(payload: Any) -> str | None:
    try:
        macro = (((payload or {}).get("message") or {}).get("body") or {}).get("macro_calls") or {}
        lyr = macro.get("track.lyrics.get") or {}
        body = ((lyr.get("message") or {}).get("body") or {}).get("lyrics") or {}
        text = (body.get("lyrics_body") or "").strip()
        if not text:
            return None
        lines = text.splitlines()
        cleaned: list[str] = []
        for line in lines:
            if line.strip().startswith("***") and "This Lyrics" in line:
                break
            cleaned.append(line.rstrip())
        out = "\n".join(cleaned).strip()
        return out or None
    except Exception:
        return None


def _extract_synced_subtitles(payload: Any) -> str | None:
    try:
        macro = (((payload or {}).get("message") or {}).get("body") or {}).get("macro_calls") or {}
        # 1. Check for richsync in macro if returned
        rich = macro.get("track.richsync.get") or {}
        rich_body = ((rich.get("message") or {}).get("body") or {}).get("richsync") or {}
        rich_text = (rich_body.get("richsync_body") or "").strip()
        if rich_text:
            parsed_rich = _richsync_json_to_lrc(rich_text)
            if parsed_rich:
                return parsed_rich

        # 2. Check standard subtitles
        sub = macro.get("track.subtitles.get") or {}
        body = ((sub.get("message") or {}).get("body") or {})
        subtitle_list = body.get("subtitle_list") or []
        if not isinstance(subtitle_list, list) or not subtitle_list:
            return None
        first = subtitle_list[0] if isinstance(subtitle_list[0], dict) else {}
        subtitle = first.get("subtitle") if isinstance(first.get("subtitle"), dict) else {}
        text = (subtitle.get("subtitle_body") or "").strip()
        return _subtitles_json_to_lrc(text)
    except Exception:
        return None


import re

try:
    import pykakasi

    _KAKASI = pykakasi.kakasi()
except Exception:
    _KAKASI = None


def _is_cjk(text: str) -> bool:
    if not text:
        return False
    return any(
        "\u3040" <= c <= "\u309f"  # Hiragana
        or "\u30a0" <= c <= "\u30ff"  # Katakana
        or "\u4e00" <= c <= "\u9fff"  # CJK Ideographs
        or "\uac00" <= c <= "\ud7af"  # Hangul
        for c in text
    )


def _romanize_japanese(text: str) -> str | None:
    if not text or not _KAKASI or not _is_cjk(text):
        return None
    try:
        conv = _KAKASI.convert(text)
        parts = [
            item.get("hepburn") or item.get("orig") or ""
            for item in conv
            if (item.get("hepburn") or item.get("orig"))
        ]
        res = " ".join(parts)
        res = re.sub(r"(\w)\(", r"\1 (", res)
        res = re.sub(r"([(\[])\s+", r"\1", res)
        res = re.sub(r"\s+([)\]])", r"\1", res)
        res = re.sub(r"\s*([,\-!?\'\"])", r"\1", res)
        res = re.sub(r"\s+", " ", res).strip()
        words = []
        for w in res.split():
            if w.startswith("(") and len(w) > 1:
                words.append("(" + w[1:].capitalize())
            else:
                words.append(w.capitalize() if w.islower() else w)
        return " ".join(words)
    except Exception:
        return None


def parse_mxm_titles(track_info: dict | None, fallback_title: str = "") -> dict | None:
    """Parse Musixmatch track metadata into a normalized titles dict:
    {
        "original": "ふたりの異変",
        "romanized": "Futarino Ihen",
        "translations": {
            "en": "Unusual Changes of Two",
            ...
        }
    }
    Languages that Musixmatch does not return are omitted.
    """
    t_info = track_info if isinstance(track_info, dict) else {}
    fallback = str(fallback_title or "").strip()
    mxm_name = str(t_info.get("track_name") or "").strip()

    original = fallback or mxm_name
    if not original:
        return None

    raw_list = t_info.get("track_name_translation_list")
    romanized: str | None = None
    translations: dict[str, str] = {}

    if isinstance(raw_list, list):
        for entry in raw_list:
            if not isinstance(entry, dict):
                continue
            item = entry.get("track_name_translation")
            if not isinstance(item, dict):
                item = entry

            lang = str(item.get("language") or "").strip().lower()
            val = str(item.get("translation") or "").strip()
            if not lang or not val:
                continue

            # Check romanized identifiers (rj = Romanized Japanese, rk = Romanized Korean, u0/zr = transliteration)
            if lang in ("rj", "rk", "romanized", "romaja"):
                romanized = val
            elif lang in ("u0", "zr"):
                if not romanized and not _is_cjk(val):
                    romanized = val
                else:
                    translations[lang] = val
            else:
                translations[lang] = val

    # If Musixmatch returned an English / alternate release title in track_name that differs from original
    if mxm_name and mxm_name.lower() != original.lower():
        if _is_cjk(original) and not _is_cjk(mxm_name):
            if "en" not in translations:
                translations["en"] = mxm_name
        elif not _is_cjk(original) and _is_cjk(mxm_name):
            if "ja" not in translations:
                translations["ja"] = mxm_name

    # If romanized title is not yet available, fallback to Japanese romanization if original has CJK/Kana
    if not romanized:
        rom = _romanize_japanese(original)
        if rom and rom.lower() != original.lower():
            romanized = rom

    out: dict[str, Any] = {"original": original}
    if romanized:
        out["romanized"] = romanized
    if translations:
        out["translations"] = translations

    return out



async def save_track_titles(track_id: str, titles: dict) -> bool:
    """Save normalized titles into the audioTracks MongoDB collection."""
    if not track_id or not isinstance(titles, dict) or not titles:
        return False
    try:
        from Api.deps.db import get_audio_tracks_collection

        col = get_audio_tracks_collection()
        await col.update_one(
            {"_id": str(track_id)},
            {
                "$set": {
                    "titles": titles,
                    "audio.titles": titles,
                    "updated_at": time.time(),
                }
            },
            upsert=False,
        )
        return True
    except Exception:
        return False


def _read_persisted_token() -> str | None:
    env_tok = (os.getenv("MUSIXMATCH_USER_TOKEN") or getattr(Config, "MUSIXMATCH_USER_TOKEN", None) or "").strip()
    if env_tok:
        return env_tok
    try:
        if _TOKEN_FILE.exists():
            content = _TOKEN_FILE.read_text(encoding="utf-8").strip()
            if content:
                data = json.loads(content) if content.startswith("{") else {"token": content}
                tok = str(data.get("token") or "").strip()
                if tok:
                    return tok
    except Exception:
        pass
    return None


def _persist_token(token: str) -> None:
    if not token or not token.strip():
        return
    try:
        _TOKEN_FILE.parent.mkdir(parents=True, exist_ok=True)
        _TOKEN_FILE.write_text(
            json.dumps({"token": token.strip(), "saved_at": time.time()}, indent=2),
            encoding="utf-8",
        )
    except Exception:
        pass


async def _fetch_user_token(*, force_refresh: bool = False) -> str | None:
    global _cached_user_token, _cached_user_token_ts

    now = time.time()
    if not force_refresh:
        if _cached_user_token and (now - _cached_user_token_ts) < 24 * 3600:
            return _cached_user_token
        disk_tok = _read_persisted_token()
        if disk_tok:
            _cached_user_token = disk_tok
            _cached_user_token_ts = now
            return disk_tok

    async with _token_lock:
        now = time.time()
        if not force_refresh:
            if _cached_user_token and (now - _cached_user_token_ts) < 24 * 3600:
                return _cached_user_token
            disk_tok = _read_persisted_token()
            if disk_tok:
                _cached_user_token = disk_tok
                _cached_user_token_ts = now
                return disk_tok

        http_status, payload = await asyncio.to_thread(
            _sync_get_json,
            url=f"{_BASE_URL}/token.get",
            params={"app_id": _APP_ID},
        )
        if http_status != 200:
            return _read_persisted_token()

        mxm_status = _extract_header_status(payload) or http_status
        if mxm_status != 200:
            return _read_persisted_token()

        token = (
            (((payload or {}).get("message") or {}).get("body") or {}).get("user_token") or ""
        ).strip()
        if not token:
            return _read_persisted_token()

        _cached_user_token = token
        _cached_user_token_ts = time.time()
        _persist_token(token)
        return token


async def fetch_and_save_musixmatch_titles(
    track_id: str,
    *,
    track: dict | None = None,
    title: str = "",
    artist: str = "",
    spotify_track_id: str = "",
) -> dict | None:
    """Query Musixmatch matcher.track.get and save parsed titles in audioTracks collection."""
    if not track_id:
        return None

    t = track or {}
    audio = t.get("audio") if isinstance(t.get("audio"), dict) else {}
    telegram = t.get("telegram") if isinstance(t.get("telegram"), dict) else {}

    track_title = title or (audio.get("title") or "").strip() or (telegram.get("title") or "").strip()
    track_artist = (
        artist
        or (audio.get("artist") or "").strip()
        or (audio.get("performer") or "").strip()
        or (telegram.get("artist") or "").strip()
    )
    sp_id = spotify_track_id or _pick_spotify_track_id(t)

    token = await _fetch_user_token(force_refresh=False)
    if not token:
        return None

    params: dict[str, Any] = {
        "usertoken": token,
        "app_id": _APP_ID,
        "subtitle_format": "mxm",
    }
    if sp_id:
        params["track_spotify_id"] = sp_id
    elif track_title and track_artist:
        params["q_track"] = track_title
        params["q_artist"] = track_artist
    elif track_title:
        params["q_track"] = track_title
    else:
        return None

    http_status, payload = await asyncio.to_thread(
        _sync_get_json,
        url=f"{_BASE_URL}/macro.subtitles.get",
        params=params,
    )
    mxm_status = _extract_header_status(payload) or http_status
    if mxm_status == 400 or http_status in (400, 401, 403):
        token2 = await _fetch_user_token(force_refresh=True)
        if token2:
            params["usertoken"] = token2
            http_status, payload = await asyncio.to_thread(
                _sync_get_json,
                url=f"{_BASE_URL}/macro.subtitles.get",
                params=params,
            )
            mxm_status = _extract_header_status(payload) or http_status

    track_info: dict[str, Any] = {}
    if mxm_status == 200:
        macro = (((payload or {}).get("message") or {}).get("body") or {}).get("macro_calls") or {}
        matcher = (macro.get("matcher.track.get") or {}).get("message") or {}
        if matcher.get("header", {}).get("status_code") == 200:
            track_info = (matcher.get("body") or {}).get("track") or {}

    # Fallback to track.search if matcher.track.get didn't find the track
    if not track_info and (track_title or track_artist):
        search_params: dict[str, Any] = {
            "usertoken": params.get("usertoken") or token,
            "app_id": _APP_ID,
            "page_size": 3,
        }
        if track_title and track_artist:
            search_params["q_track"] = track_title
            search_params["q_artist"] = track_artist
        elif track_title:
            search_params["q_track"] = track_title
        elif track_artist:
            search_params["q_artist"] = track_artist

        s_status, s_payload = await asyncio.to_thread(
            _sync_get_json,
            url=f"{_BASE_URL}/track.search",
            params=search_params,
        )
        s_list = (((s_payload or {}).get("message") or {}).get("body") or {}).get("track_list") or []
        if s_list and isinstance(s_list, list):
            first_match = s_list[0].get("track") or {}
            if first_match:
                track_info = first_match

    titles = parse_mxm_titles(track_info, fallback_title=track_title)
    if titles:
        await save_track_titles(track_id, titles)
    return titles


async def fetch_track_lyrics_from_musixmatch(*, track: dict) -> dict:
    audio = track.get("audio") if isinstance(track.get("audio"), dict) else {}
    telegram = track.get("telegram") if isinstance(track.get("telegram"), dict) else {}
    title = (audio.get("title") or "").strip() or (telegram.get("title") or "").strip()
    artist = (audio.get("artist") or "").strip() or (audio.get("performer") or "").strip() or (telegram.get("artist") or "").strip()
    album = (audio.get("album") or "").strip() or (telegram.get("album") or "").strip()
    year = audio.get("year")
    track_id = str(track.get("_id") or track.get("id") or "").strip()

    spotify_track_id = _pick_spotify_track_id(track)
    if not spotify_track_id:
        try:
            from stream.helpers.cover_search import spotify_best_track

            sp = await spotify_best_track(title=title, artist=artist, album=album, year=year)
        except Exception:
            sp = None

        if isinstance(sp, dict):
            spotify_track_id = (sp.get("id") or "").strip() if isinstance(sp.get("id"), str) else ""
            ext = sp.get("external_urls") if isinstance(sp.get("external_urls"), dict) else {}
            spotify_url = (ext.get("spotify") or "").strip() if isinstance(ext.get("spotify"), str) else ""
            if spotify_track_id:
                spotify = track.get("spotify") if isinstance(track.get("spotify"), dict) else {}
                spotify["track_spotify_id"] = spotify_track_id
                if spotify_url:
                    spotify["url"] = spotify_url
                track["spotify"] = spotify

    token = await _fetch_user_token(force_refresh=False)
    if not token:
        return {"ok": False, "error": "token_failed"}

    params = {
        "usertoken": token,
        "app_id": _APP_ID,
        "subtitle_format": "mxm",
    }
    
    if spotify_track_id:
        params["track_spotify_id"] = spotify_track_id
    elif title and artist:
        params["q_track"] = title
        params["q_artist"] = artist
    else:
        return {"ok": False, "error": "missing_spotify_track_id_and_metadata"}

    async def _call() -> tuple[int, Any]:
        return await asyncio.to_thread(
            _sync_get_json,
            url=f"{_BASE_URL}/macro.subtitles.get",
            params=params,
        )

    http_status, payload = await _call()
    mxm_status = _extract_header_status(payload) or http_status

    if mxm_status == 400 or http_status in (400, 401, 403):
        token2 = await _fetch_user_token(force_refresh=True)
        if token2:
            params["usertoken"] = token2
            http_status, payload = await _call()
            mxm_status = _extract_header_status(payload) or http_status

    debug_payload = None
    if bool(getattr(Config, "DEBUG", False)):
        debug_payload = payload

    if mxm_status != 200:
        return {
            "ok": False,
            "error": "mxm_failed",
            "status_code": int(mxm_status),
            "spotify_track_id": spotify_track_id,
            **({"musixmatch": debug_payload} if debug_payload is not None else {}),
        }

    macro = (((payload or {}).get("message") or {}).get("body") or {}).get("macro_calls") or {}
    track_info = (((macro.get("matcher.track.get") or {}).get("message") or {}).get("body") or {}).get("track") or {}

    # Extract alternate / romanized titles and persist if available
    titles = parse_mxm_titles(track_info, fallback_title=title)
    if titles and track_id:
        asyncio.create_task(save_track_titles(track_id, titles))

    # Try fetching richsync for word-level sync if track_id was matched
    try:
        mxm_track_id = track_info.get("track_id")
        current_token = params.get("usertoken")
        if mxm_track_id and current_token:
            r_status, r_payload = await asyncio.to_thread(
                _sync_get_json,
                url=f"{_BASE_URL}/track.richsync.get",
                params={"app_id": _APP_ID, "usertoken": current_token, "track_id": mxm_track_id},
            )
            if r_status == 200:
                r_body = (((r_payload or {}).get("message") or {}).get("body") or {}).get("richsync") or {}
                r_text = (r_body.get("richsync_body") or "").strip()
                if r_text:
                    parsed_rich = _richsync_json_to_lrc(r_text)
                    if parsed_rich:
                        return {
                            "ok": True,
                            "lyrics": parsed_rich,
                            "kind": "richsync",
                            "source": "musixmatch",
                            "spotify_track_id": spotify_track_id,
                            **({"titles": titles} if titles is not None else {}),
                            **({"musixmatch": debug_payload} if debug_payload is not None else {}),
                        }
    except Exception:
        pass

    synced = _extract_synced_subtitles(payload)
    if synced:
        return {
            "ok": True,
            "lyrics": synced,
            "kind": "synced",
            "source": "musixmatch",
            "spotify_track_id": spotify_track_id,
            **({"titles": titles} if titles is not None else {}),
            **({"musixmatch": debug_payload} if debug_payload is not None else {}),
        }

    plain = _extract_plain_lyrics(payload)
    if plain:
        return {
            "ok": True,
            "lyrics": plain,
            "kind": "plain",
            "source": "musixmatch",
            "spotify_track_id": spotify_track_id,
            **({"titles": titles} if titles is not None else {}),
            **({"musixmatch": debug_payload} if debug_payload is not None else {}),
        }

    return {
        "ok": False,
        "error": "no_lyrics",
        "spotify_track_id": spotify_track_id,
        **({"titles": titles} if titles is not None else {}),
        **({"musixmatch": debug_payload} if debug_payload is not None else {}),
    }

