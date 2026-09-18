from __future__ import annotations

import asyncio
import difflib
import json
import re
import time
from typing import Any
from urllib.parse import quote

from aiohttp import ClientSession, ClientTimeout

from stream.core.config_manager import Config
from stream.helpers.hoaders import hoaders_big_cover_url, hoaders_cover_info

_SPOTIFY_TOKEN: str | None = None
_SPOTIFY_TOKEN_EXPIRES_AT: float = 0.0
_SPOTIFY_SEM = asyncio.Semaphore(3)


def _dbg(msg: str) -> None:
    if bool(getattr(Config, "DEBUG", False)):
        print(msg)


async def _json_or_none(resp, *, label: str) -> dict | None:
    text = await resp.text()
    if not (text or "").strip():
        _dbg(f"[cover] {label} empty_response status={resp.status}")
        return None
    try:
        payload = json.loads(text)
    except Exception as e:
        _dbg(
            f"[cover] {label} bad_json status={resp.status} err={e!r} body={text[:300]!r}"
        )
        return None
    return payload if isinstance(payload, dict) else None


def _strip_query_noise(text: str) -> str:
    s = (text or "").strip()
    if not s:
        return ""
    # Strip disc/cd/volume markers only
    s = re.sub(r"[\(\[]\s*(?:disc|cd|tape|vol|volume)\s*\d+[\)\]]", " ", s, flags=re.I)
    # Strip media descriptors in parentheses or brackets (e.g. [Official Video], (Remastered))
    s = re.sub(
        r"[\(\[]\s*(?:official\s+(?:audio|video|music\s+video)|lyric(?:s)?\s+video|official|audio|video|mv|visualizer|remaster(?:ed)?|hd|4k|explicit|clean)\s*[\)\]]",
        " ",
        s,
        flags=re.I,
    )
    # If there are unclosed brackets (e.g. from truncation), replace the opening bracket with space instead of wiping the text
    if ("(" in s and ")" not in s) or ("[" in s and "]" not in s):
        s = re.sub(r"[\[\(]", " ", s)
    s = re.sub(r"[\.…\s]+$", "", s)
    s = re.sub(
        r"\b(?:official\s+(?:audio|video|music\s+video)|lyric(?:s)?\s+video|visualizer|remaster(?:ed)?|explicit|clean)\b",
        " ",
        s,
        flags=re.I,
    )
    s = re.sub(r"\s+", " ", s).strip()
    return s


def _norm_cmp(text: str) -> str:
    s = (text or "").casefold()
    s = re.sub(r"[\[\(].*?[\]\)]", " ", s)
    s = re.sub(r"[^\w\s]+", " ", s, flags=re.UNICODE)
    s = re.sub(r"\s+", " ", s).strip()
    return s


def is_spotify_configured() -> bool:
    client_id = (getattr(Config, "SPOTIFY_CLIENT_ID", "") or "").strip()
    client_secret = (getattr(Config, "SPOTIFY_CLIENT_SECRET", "") or "").strip()
    return bool(client_id and client_secret)


async def _spotify_get_access_token() -> str:
    global _SPOTIFY_TOKEN, _SPOTIFY_TOKEN_EXPIRES_AT

    if not is_spotify_configured():
        raise RuntimeError("Spotify credentials missing")

    now = time.time()
    if _SPOTIFY_TOKEN and now < (_SPOTIFY_TOKEN_EXPIRES_AT - 60):
        return _SPOTIFY_TOKEN

    client_id = (getattr(Config, "SPOTIFY_CLIENT_ID", "") or "").strip()
    client_secret = (getattr(Config, "SPOTIFY_CLIENT_SECRET", "") or "").strip()

    data = {
        "grant_type": "client_credentials",
        "client_id": client_id,
        "client_secret": client_secret,
    }

    async with _SPOTIFY_SEM:
        async with ClientSession() as session:
            async with session.post("https://accounts.spotify.com/api/token", data=data) as resp:
                payload = await _json_or_none(resp, label="spotify_token")
                if resp.status != 200:
                    raise RuntimeError(f"Spotify token error: {payload}")
                if payload is None:
                    raise RuntimeError("Spotify token response was not JSON")

    token = (payload.get("access_token") or "").strip()
    expires_in = int(payload.get("expires_in") or 3600)
    if not token:
        raise RuntimeError("Spotify token missing")

    _SPOTIFY_TOKEN = token
    _SPOTIFY_TOKEN_EXPIRES_AT = time.time() + expires_in
    return token


def _spotify_cover_from_track(track: dict) -> str | None:
    album = track.get("album") if isinstance(track.get("album"), dict) else {}
    images = album.get("images") if isinstance(album.get("images"), list) else []
    for img in images:
        if isinstance(img, dict) and img.get("url"):
            return str(img.get("url")).strip() or None
    return None


def _spotify_cover_from_album(album: dict) -> str | None:
    images = album.get("images") if isinstance(album.get("images"), list) else []
    for img in images:
        if isinstance(img, dict) and img.get("url"):
            return str(img.get("url")).strip() or None
    return None


def _spotify_album_score(album: dict, *, artist: str, album_name: str, year: int | None = None) -> int:
    want_artist = _norm_cmp(artist)
    want_album = _norm_cmp(album_name)

    name = _norm_cmp(album.get("name") or "")
    artists = album.get("artists") if isinstance(album.get("artists"), list) else []
    first_artist = ""
    for a in artists:
        if isinstance(a, dict) and a.get("name"):
            first_artist = str(a.get("name"))
            break
    got_artist = _norm_cmp(first_artist)

    score = 0
    if want_album:
        score += 8 if name == want_album else (4 if want_album in name or name in want_album else 0)
    if want_artist:
        score += 4 if got_artist == want_artist else (2 if want_artist in got_artist or got_artist in want_artist else 0)
    if year is not None:
        release_date = str(album.get("release_date") or "").strip()
        if len(release_date) >= 4 and release_date[:4].isdigit():
            try:
                score += 2 if int(release_date[:4]) == int(year) else 0
            except Exception:
                pass
    return score


async def spotify_album_cover_url(*, artist: str, album: str, year: int | None = None) -> str | None:
    a = _strip_query_noise(artist)
    al = _strip_query_noise(album)
    if not a or not al:
        return None

    parts = [
        f'album:"{al}" artist:"{a}"',
        f'album:"{al}"',
        f"{al} {a}",
    ]
    if year:
        parts.insert(1, f'album:"{al}" year:{int(year)}')

    try:
        token = await _spotify_get_access_token()
    except Exception as e:
        _dbg(f"[cover] spotify_album token_failed err={e!r}")
        return None
    headers = {"Authorization": f"Bearer {token}"}

    seen: set[str] = set()
    for q in parts:
        k = (q or "").strip().casefold()
        if not k or k in seen:
            continue
        seen.add(k)
        _dbg(f"[cover] spotify_album query={q!r}")
        url = f"https://api.spotify.com/v1/search?q={quote(q)}&type=album&limit=5"
        async with _SPOTIFY_SEM:
            async with ClientSession(timeout=ClientTimeout(total=5.0)) as session:
                async with session.get(url, headers=headers) as resp:
                    payload = await _json_or_none(resp, label="spotify_album")
                    if payload is None:
                        continue
                    if resp.status != 200:
                        _dbg(f"[cover] spotify_album error status={resp.status} body={str(payload)[:300]!r}")
                        continue

        items = (((payload or {}).get("albums") or {}).get("items") or [])
        if not items:
            continue

        best = None
        best_score = -1
        for it in items:
            if not isinstance(it, dict):
                continue
            s = _spotify_album_score(it, artist=a, album_name=al, year=year)
            if s > best_score:
                best = it
                best_score = s

        picked = best or (items[0] if items else None)
        if not isinstance(picked, dict):
            continue

        cover = _spotify_cover_from_album(picked)
        if cover:
            _dbg(f"[cover] spotify_album match_found url={cover!r}")
            return cover

    return None


def _spotify_track_score(track: dict, *, title: str, artist: str, album: str) -> int:
    want_title = _norm_cmp(title)
    want_artist = _norm_cmp(artist)
    want_album = _norm_cmp(album)

    name = _norm_cmp(track.get("name") or "")
    artists = track.get("artists") if isinstance(track.get("artists"), list) else []
    first_artist = ""
    for a in artists:
        if isinstance(a, dict) and a.get("name"):
            first_artist = str(a.get("name"))
            break
    got_artist = _norm_cmp(first_artist)

    alb = track.get("album") if isinstance(track.get("album"), dict) else {}
    got_album = _norm_cmp(alb.get("name") or "")

    score = 0
    if want_title:
        score += 6 if name == want_title else (3 if want_title in name else 0)
    if want_artist:
        score += 6 if got_artist == want_artist else (3 if want_artist in got_artist or got_artist in want_artist else 0)
    if want_album:
        score += 3 if got_album == want_album else (1 if want_album and want_album in got_album else 0)
    return score


async def spotify_best_track(*, title: str, artist: str, album: str = "", year: int | None = None) -> dict | None:
    if not is_spotify_configured():
        return None
    t = _strip_query_noise(title)
    a = _strip_query_noise(artist)
    al = _strip_query_noise(album)

    if not t:
        return None

    parts: list[str] = []
    if al:
        parts.append(f'track:"{t}" album:"{al}"')
        parts.append(f"{t} {al}")
    if year:
        parts.append(f'track:"{t}" year:{int(year)}')
    if a:
        parts.append(f'track:"{t}" artist:"{a}"')
    parts.append(f'track:"{t}"')

    try:
        token = await _spotify_get_access_token()
    except Exception as e:
        _dbg(f"[cover] spotify_track token_failed err={e!r}")
        return None
    headers = {"Authorization": f"Bearer {token}"}

    seen: set[str] = set()
    for q in parts:
        k = (q or "").strip().casefold()
        if not k or k in seen:
            continue
        seen.add(k)
        _dbg(f"[cover] spotify_track query={q!r}")
        url = f"https://api.spotify.com/v1/search?q={quote(q)}&type=track&limit=5"
        async with _SPOTIFY_SEM:
            async with ClientSession(timeout=ClientTimeout(total=5.0)) as session:
                async with session.get(url, headers=headers) as resp:
                    payload = await _json_or_none(resp, label="spotify_track")
                    if payload is None:
                        continue
                    if resp.status != 200:
                        _dbg(f"[cover] spotify_track error status={resp.status} body={str(payload)[:300]!r}")
                        continue

        items = (((payload or {}).get("tracks") or {}).get("items") or [])
        if not items:
            continue

        best = None
        best_score = -1
        for tr in items:
            if not isinstance(tr, dict):
                continue
            s = _spotify_track_score(tr, title=t, artist=a, album=al)
            if s > best_score:
                best = tr
                best_score = s

        picked = best or (items[0] if items else None)
        if isinstance(picked, dict):
            return picked

    return None


async def spotify_cover_url(*, title: str, artist: str, album: str = "", year: int | None = None) -> str | None:
    t = _strip_query_noise(title)
    a = _strip_query_noise(artist)
    al = _strip_query_noise(album)

    if not t:
        return None

    parts: list[str] = []
    if al:
        parts.append(f'track:"{t}" album:"{al}"')
        parts.append(f"{t} {al}")
    if year:
        parts.append(f'track:"{t}" year:{int(year)}')
    parts.append(f'track:"{t}"')

    tr = await spotify_best_track(title=title, artist=artist, album=album, year=year)
    if not isinstance(tr, dict):
        return None
    cover = _spotify_cover_from_track(tr)
    if cover:
        _dbg(f"[cover] spotify match_found url={cover!r}")
        return cover
    return None


def _apple_artwork_upgrade(url: str) -> str:
    u = (url or "").strip()
    if not u:
        return ""
    u = re.sub(r"/\d+x\d+bb\.", "/3000x3000bb.", u)
    u = re.sub(r"/\d+x\d+\.", "/3000x3000.", u)
    return u


def _apple_score(item: dict, *, title: str, artist: str) -> int:
    want_t = _norm_cmp(title)
    want_a = _norm_cmp(artist)
    got_t = _norm_cmp(item.get("trackName") or "")
    got_a = _norm_cmp(item.get("artistName") or "")

    score = 0
    if want_t:
        score += 6 if got_t == want_t else (3 if want_t in got_t else 0)
    if want_a:
        score += 6 if got_a == want_a else (3 if want_a in got_a or got_a in want_a else 0)
    return score


_APPLE_SEM = asyncio.Semaphore(2)
_APPLE_BACKOFF_UNTIL: float = 0.0


async def apple_cover_url(*, title: str, artist: str, album: str = "", year: int | None = None) -> str | None:
    global _APPLE_BACKOFF_UNTIL
    if time.time() < _APPLE_BACKOFF_UNTIL:
        return None

    t = _strip_query_noise(title)
    a = _strip_query_noise(artist)
    al = _strip_query_noise(album)
    if not a:
        return None

    terms: list[str] = []
    if al:
        terms.append(f"{al} {a}".strip())
        terms.append(al)
        if t:
            terms.append(f"{t} {al}".strip())
    if t:
        terms.append(t)
    if year:
        if t:
            terms.append(f"{t} {int(year)}".strip())

    best_art = None
    best_score = -1

    terms = terms[:2]
    seen: set[str] = set()
    for term in terms:
        k = (term or "").strip().casefold()
        if not k or k in seen:
            continue
        seen.add(k)

        if time.time() < _APPLE_BACKOFF_UNTIL:
            return None

        url = f"https://itunes.apple.com/search?term={quote(term)}&entity=song&limit=5"
        _dbg(f"[cover] apple query={term!r}")

        async with _APPLE_SEM:
            try:
                headers = {"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"}
                async with ClientSession(headers=headers, timeout=ClientTimeout(total=5.0)) as session:
                    async with session.get(url) as resp:
                        if resp.status in (403, 429):
                            _dbg(f"[cover] apple rate limited/blocked status={resp.status}, backing off for 60s")
                            _APPLE_BACKOFF_UNTIL = time.time() + 60.0
                            return None
                        payload = await _json_or_none(resp, label="apple")
                        if payload is None:
                            continue
                        if resp.status != 200:
                            _dbg(f"[cover] apple error status={resp.status} body={str(payload)[:300]!r}")
                            continue
            except Exception as e:
                _dbg(f"[cover] apple request failed: {e}")
                continue
            await asyncio.sleep(0.15)

        results = (payload or {}).get("results") or []
        if not isinstance(results, list) or not results:
            continue

        for it in results:
            if not isinstance(it, dict):
                continue
            s = _apple_score(it, title=t, artist=a)
            if s <= best_score:
                continue
            art = it.get("artworkUrl100") or it.get("artworkUrl60") or ""
            art = _apple_artwork_upgrade(str(art))
            if not art:
                continue
            best_art = art
            best_score = s

        if best_art and best_score >= 9:
            break

    if best_art:
        _dbg(f"[cover] apple match_found url={best_art!r}")
        return best_art
    return None


async def deezer_cover_url(*, title: str, artist: str, album: str = "", year: int | None = None) -> str | None:
    t = _strip_query_noise(title)
    a = _strip_query_noise(artist)
    al = _strip_query_noise(album)
    if not a:
        return None

    queries: list[str] = []
    if al:
        queries.append(f"{al} {a}".strip())
        queries.append(al)
        if t:
            queries.append(f"{t} {al}".strip())
    if t:
        queries.append(t)
    if year:
        if t:
            queries.append(f"{t} {int(year)}".strip())

    queries = queries[:2]
    seen: set[str] = set()
    for q in queries:
        k = (q or "").strip().casefold()
        if not k or k in seen:
            continue
        seen.add(k)

        url = f"https://api.deezer.com/search?q={quote(q)}&limit=3"
        _dbg(f"[cover] deezer query={q!r}")
        try:
            async with ClientSession(timeout=ClientTimeout(total=5.0)) as session:
                async with session.get(url) as resp:
                    payload = await _json_or_none(resp, label="deezer")
                    if payload is None:
                        continue
                    if resp.status != 200:
                        _dbg(f"[cover] deezer error status={resp.status} body={str(payload)[:300]!r}")
                        continue
        except Exception as e:
            _dbg(f"[cover] deezer request failed: {e}")
            continue

        data = (payload or {}).get("data") or []
        if not isinstance(data, list) or not data:
            continue

        first = data[0] if isinstance(data[0], dict) else None
        if not isinstance(first, dict):
            continue
        alb = first.get("album") if isinstance(first.get("album"), dict) else {}
        cover = alb.get("cover_xl") or alb.get("cover_big") or alb.get("cover_medium") or ""
        cover = str(cover).strip()
        if cover:
            _dbg(f"[cover] deezer match_found url={cover!r}")
            return cover

    return None


async def fetch_artist_avatar_info(artist_name: str) -> dict | None:
    if not artist_name or not artist_name.strip():
        return None

    name = artist_name.strip()
    slug = re.sub(r'[^a-z0-9]+', '_', name.lower()).strip('_')
    if not slug:
        return None

    try:
        from stream.database.MongoDb import db_handler
        col = db_handler.get_collection("artist_profiles").collection
        existing = await col.find_one({"_id": slug})
        if existing and existing.get("avatar_url"):
            return existing
    except Exception:
        col = None

    avatar_url = None
    artist_id = None
    link = None

    try:
        url = f"https://api.deezer.com/search/artist?q={quote(name)}"
        async with ClientSession() as session:
            async with session.get(url, timeout=10) as resp:
                if resp.status == 200:
                    payload = await resp.json()
                    data = (payload or {}).get("data") or []
                    if data and isinstance(data[0], dict):
                        first = data[0]
                        avatar_url = first.get("picture_xl") or first.get("picture_big") or first.get("picture_medium")
                        # Reject the default/empty Deezer avatar
                        if avatar_url and "cdn-images.dzcdn.net/images/artist//" in avatar_url:
                            avatar_url = None
                        artist_id = first.get("id")
                        link = first.get("link")
    except Exception as e:
        _dbg(f"[artist] deezer search failed for {name!r}: {e}")

    if not avatar_url and is_spotify_configured():
        try:
            sp_token = await _spotify_get_access_token()
            if sp_token:
                sp_url = f"https://api.spotify.com/v1/search?q={quote(name)}&type=artist&limit=1"
                sp_headers = {"Authorization": f"Bearer {sp_token}"}
                async with ClientSession(headers=sp_headers, timeout=ClientTimeout(total=4.0)) as session:
                    async with session.get(sp_url) as resp:
                        if resp.status == 200:
                            sp_data = await _json_or_none(resp, label="spotify_artist")
                            sp_items = (sp_data or {}).get("artists", {}).get("items") or []
                            if sp_items and isinstance(sp_items[0], dict):
                                sp_imgs = sp_items[0].get("images") or []
                                if sp_imgs and isinstance(sp_imgs[0], dict) and sp_imgs[0].get("url"):
                                    avatar_url = str(sp_imgs[0]["url"]).strip()
                                    artist_id = sp_items[0].get("id")
                                    link = (sp_items[0].get("external_urls") or {}).get("spotify")
        except Exception as e:
            _dbg(f"[artist] spotify artist search failed for {name!r}: {e}")

    if not avatar_url:
        try:
            yt_avatar = await youtube_artist_avatar(name)
            if yt_avatar:
                avatar_url = yt_avatar
        except Exception as e:
            _dbg(f"[artist] youtube avatar search failed for {name!r}: {e}")

    doc = {
        "_id": slug,
        "name": name,
        "artist_id": artist_id,
        "avatar_url": avatar_url,
        "link": link,
        "updated_at": time.time()
    }
    if avatar_url and col is not None:
        try:
            await col.update_one({"_id": slug}, {"$set": doc}, upsert=True)
        except Exception:
            pass

    return doc


_YOUTUBE_SEM = asyncio.Semaphore(5)


def _is_junk_artist(artist: str) -> bool:
    if not artist:
        return True
    a = artist.strip().lower()
    if a in {
        "unknown",
        "unknown artist",
        "various artists",
        "various",
        "artist",
        "none",
        "null",
        "soundtrack",
        "va",
        "ost",
    }:
        return True
    if a.startswith("unknown") or a.endswith("unknown"):
        return True
    if re.fullmatch(r"\d+", a):
        return True
    return False


def _clean_match_text(text: str) -> str:
    s = (text or "").lower()
    # Strip disc/cd markers only, preserving parenthetical song titles and subtitles
    s = re.sub(r"[\(\[]\s*(?:disc|cd|tape|vol|volume)\s*\d+[\)\]]", " ", s, flags=re.I)
    s = re.sub(r"[^\w\s]", " ", s)
    return re.sub(r"\s+", " ", s).strip()


def _words_match(w1: str, w2: str) -> bool:
    if w1 == w2:
        return True
    if len(w1) >= 3 and len(w2) >= 3:
        # Common transliteration suffix (eyeballs vs eyeballz)
        if (w1.endswith("s") and w2.endswith("z") and w1[:-1] == w2[:-1]) or (
            w1.endswith("z") and w2.endswith("s") and w1[:-1] == w2[:-1]
        ):
            return True
        # Sequence similarity >= 0.75 for transliterations and typos
        if difflib.SequenceMatcher(None, w1, w2).ratio() >= 0.75:
            return True
    return False


def _match_youtube_candidate(
    target_title: str,
    target_artist: str,
    cand_title: str,
    cand_channel: str,
    target_album: str = "",
) -> float:
    """Score how well a YouTube search result matches the track title, artist, or channel."""
    c_target_t = _clean_match_text(target_title)
    c_target_a = _clean_match_text(target_artist) if not _is_junk_artist(target_artist) else ""
    c_cand_t = _clean_match_text(cand_title)
    c_cand_ch = _clean_match_text(cand_channel)

    if not c_target_t or not c_cand_t:
        return 0.0

    raw_cand_t = cand_title.lower()

    # Reject non-music / instructional / reaction videos unless the target itself is that
    for bad in ["reaction", "reacts to", "guitar lesson", "how to play", "tutorial", "cover by", "interview", "review"]:
        if bad in raw_cand_t and bad not in c_target_t:
            return 0.0

    # Collect title variants to test (full title, track part after ' - ', leading-digits-stripped title)
    title_variants = [c_target_t]
    if " - " in target_title:
        parts = [p.strip() for p in target_title.split(" - ") if p.strip()]
        if len(parts) >= 2:
            title_variants.append(_clean_match_text(parts[-1]))

    clean_t = re.sub(r"^\d{1,3}\s*[-.]?\s*", "", target_title).strip()
    if clean_t and clean_t != target_title:
        title_variants.append(_clean_match_text(clean_t))

    best_title_score = 0.0
    for var in title_variants:
        if not var:
            continue
        if re.search(r"\b" + re.escape(var) + r"\b", c_cand_t):
            best_title_score = max(best_title_score, 1.0)
            continue

        var_words = [w for w in var.split() if len(w) > 1] or var.split()
        cand_words = c_cand_t.split()
        if not var_words:
            continue

        matched_words = sum(1 for tw in var_words if any(_words_match(tw, cw) for cw in cand_words))
        var_score = matched_words / len(var_words)
        best_title_score = max(best_title_score, var_score)

    if best_title_score < 0.6:
        return 0.0

    score = best_title_score * 50.0

    # Boost exact phrase match
    if best_title_score >= 0.95:
        score += 10.0

    for boost in [
        "official music video",
        "official audio",
        "official video",
        "full song",
        "lyric video",
        "original track",
        "visualizer",
        "feat.",
        "featuring",
        "lyrics",
    ]:
        if boost in raw_cand_t:
            score += 15.0
            break

    # Match track > artists or channel
    trusted_labels = [
        "vevo", "records", "music", "t series", "t-series", "sony", "zee",
        "yrf", "tips", "speed", "kalamkaar", "mass appeal", "metallica",
        "red ribbon", "uproxx", "pop chartbusters", "the listener", "listener",
    ]

    if c_target_a:
        artist_words = [w for w in c_target_a.split() if len(w) > 1] or c_target_a.split()
        cand_ch_words = c_cand_ch.split()
        cand_t_words = c_cand_t.split()
        matched_artist = False

        # Channel match
        if re.search(r"\b" + re.escape(c_target_a) + r"\b", c_cand_ch):
            score += 45.0
            matched_artist = True
        elif artist_words and any(any(_words_match(aw, cw) for cw in cand_ch_words) for aw in artist_words):
            score += 35.0
            matched_artist = True
        elif any(lbl in c_cand_ch for lbl in trusted_labels):
            score += 20.0

        # Video title match
        if re.search(r"\b" + re.escape(c_target_a) + r"\b", c_cand_t):
            score += 40.0
            matched_artist = True
        elif artist_words and any(any(_words_match(aw, cw) for cw in cand_t_words) for aw in artist_words):
            score += 30.0
            matched_artist = True

        if not matched_artist and not any(lbl in c_cand_ch for lbl in trusted_labels):
            score -= 35.0
    else:
        if "topic" in c_cand_ch or any(lbl in c_cand_ch for lbl in trusted_labels):
            score += 25.0

    return score


async def _search_youtube_innertube(query: str, limit: int = 8) -> list[dict[str, Any]]:
    if not query or not query.strip():
        return []
    url = "https://www.youtube.com/youtubei/v1/search"
    payload = {
        "context": {
            "client": {
                "clientName": "WEB",
                "clientVersion": "2.20240101.00.00",
                "hl": "en",
                "gl": "US",
            }
        },
        "query": query.strip(),
    }
    headers = {
        "User-Agent": (
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
            "(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
        ),
    }
    results: list[dict[str, Any]] = []
    async with _YOUTUBE_SEM:
        try:
            async with ClientSession() as session:
                async with session.post(url, json=payload, headers=headers, timeout=6.0) as resp:
                    if resp.status != 200:
                        return []
                    data = await resp.json()
                    sections = (
                        data.get("contents", {})
                        .get("twoColumnSearchResultsRenderer", {})
                        .get("primaryContents", {})
                        .get("sectionListRenderer", {})
                        .get("contents", [])
                    )
                    for sec in sections:
                        contents = sec.get("itemSectionRenderer", {}).get("contents", [])
                        for it in contents:
                            v = it.get("videoRenderer")
                            if not v:
                                continue
                            vid = v.get("videoId")
                            cand_title = "".join(r.get("text", "") for r in v.get("title", {}).get("runs", []))
                            cand_ch = "".join(r.get("text", "") for r in v.get("ownerText", {}).get("runs", []))
                            thumbs = v.get("thumbnail", {}).get("thumbnails", [])
                            big_thumb = thumbs[-1].get("url") if thumbs else None
                            small_thumb = thumbs[0].get("url") if thumbs else None
                            ch_thumbs = (
                                v.get("channelThumbnailSupportedRenderers", {})
                                .get("channelThumbnailWithLinkRenderer", {})
                                .get("thumbnail", {})
                                .get("thumbnails", [])
                            )
                            avatar = ch_thumbs[-1].get("url") if ch_thumbs else None
                            if avatar:
                                if avatar.startswith("//"):
                                    avatar = f"https:{avatar}"
                                avatar = re.sub(r"=s\d+", "=s800", avatar)

                            results.append({
                                "video_id": vid,
                                "title": cand_title,
                                "channel": cand_ch,
                                "big_thumb": big_thumb,
                                "small_thumb": small_thumb,
                                "avatar": avatar,
                            })
                            if len(results) >= limit:
                                return results
        except Exception as e:
            _dbg(f"[cover] innertube search failed query={query!r}: {e}")
    return results


def _search_youtube_ytdlp_sync(query: str, limit: int = 5) -> list[dict[str, Any]]:
    try:
        import yt_dlp
    except Exception:
        return []
    opts = {
        "quiet": True,
        "no_warnings": True,
        "extract_flat": True,
        "noplaylist": True,
    }
    try:
        with yt_dlp.YoutubeDL(opts) as ydl:
            res = ydl.extract_info(f"ytsearch{limit}:{query}", download=False)
        entries = (res or {}).get("entries") or []
        out: list[dict[str, Any]] = []
        for e in entries:
            vid = e.get("id")
            thumbs = e.get("thumbnails") or []
            big_thumb = thumbs[-1].get("url") if thumbs else e.get("thumbnail")
            out.append({
                "video_id": vid,
                "title": e.get("title") or "",
                "channel": e.get("uploader") or e.get("channel") or "",
                "big_thumb": big_thumb,
                "small_thumb": f"https://i.ytimg.com/vi/{vid}/mqdefault.jpg" if vid else None,
                "avatar": None,
            })
        return out
    except Exception as e:
        _dbg(f"[cover] yt_dlp search failed query={query!r}: {e}")
        return []


async def youtube_artist_avatar(artist_name: str) -> str | None:
    """Find a high-res artist channel avatar on YouTube."""
    if not artist_name or _is_junk_artist(artist_name):
        return None
    name = artist_name.strip()
    query = f"{name} official channel"
    url = "https://www.youtube.com/youtubei/v1/search"
    payload = {
        "context": {
            "client": {
                "clientName": "WEB",
                "clientVersion": "2.20240101.00.00",
                "hl": "en",
                "gl": "US",
            }
        },
        "query": query,
    }
    headers = {
        "User-Agent": (
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
            "(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
        ),
    }
    async with _YOUTUBE_SEM:
        try:
            async with ClientSession() as session:
                async with session.post(url, json=payload, headers=headers, timeout=5.0) as resp:
                    if resp.status != 200:
                        return None
                    data = await resp.json()
                    sections = (
                        data.get("contents", {})
                        .get("twoColumnSearchResultsRenderer", {})
                        .get("primaryContents", {})
                        .get("sectionListRenderer", {})
                        .get("contents", [])
                    )
                    for sec in sections:
                        contents = sec.get("itemSectionRenderer", {}).get("contents", [])
                        for it in contents:
                            ch = it.get("channelRenderer")
                            if ch:
                                thumbs = ch.get("thumbnail", {}).get("thumbnails", [])
                                if thumbs:
                                    avatar = thumbs[-1].get("url")
                                    if avatar:
                                        if avatar.startswith("//"):
                                            avatar = f"https:{avatar}"
                                        return re.sub(r"=s\d+", "=s800", avatar)
                            v = it.get("videoRenderer")
                            if v:
                                ch_name = "".join(r.get("text", "") for r in v.get("ownerText", {}).get("runs", []))
                                if re.search(r"\b" + re.escape(name.lower()) + r"\b", ch_name.lower()):
                                    ch_thumbs = (
                                        v.get("channelThumbnailSupportedRenderers", {})
                                        .get("channelThumbnailWithLinkRenderer", {})
                                        .get("thumbnail", {})
                                        .get("thumbnails", [])
                                    )
                                    if ch_thumbs:
                                        avatar = ch_thumbs[-1].get("url")
                                        if avatar:
                                            if avatar.startswith("//"):
                                                avatar = f"https:{avatar}"
                                            return re.sub(r"=s\d+", "=s800", avatar)
        except Exception as e:
            _dbg(f"[artist] youtube avatar search failed: {e}")
    return None


async def youtube_cover_search(
    *,
    title: str,
    artist: str = "",
    album: str = "",
    year: int | None = None,
) -> tuple[str | None, str | None, dict[str, Any] | None]:
    """Search YouTube for a track cover matching track > artists or channel."""
    t = _strip_query_noise(title)
    a = _strip_query_noise(artist)
    al = _strip_query_noise(album)

    # Clean leading track numbers (e.g. "16 Canibus" -> "Canibus")
    clean_a = re.sub(r"^\d{1,3}\s*[-.]?\s*", "", a).strip() if a else ""
    if clean_a and not _is_junk_artist(clean_a):
        a = clean_a

    # Detect known Metallica Load/Reload box sets when artist is missing or unknown
    metallica_markers = (
        "shadowcast",
        "loadapalooza",
        "poor norwegian me",
        "poor touring me",
        "escape from the studio",
        "club shows & rehearsals",
    )
    if any(m in title.lower() for m in metallica_markers) and (not a or _is_junk_artist(a)):
        a = "Metallica"

    # If artist is missing or junk, check if embedded in title like "[Artist] Title" or "Artist - Title"
    if _is_junk_artist(a):
        m = re.match(r"^\[(?P<artist>[^\]]+)\]\s*(?P<title>.+)$", title)
        if not m:
            m = re.match(r"^\((?:\d+)\)\s*\[(?P<artist>[^\]]+)\]\s*(?P<title>.+)$", title)
        if not m:
            m = re.match(r"^(?P<artist>.+?)\s*-\s*(?P<title>.+)$", title)
        if m:
            cand_a = (m.group("artist") or "").strip()
            cand_t = (m.group("title") or "").strip()
            if cand_a and not _is_junk_artist(cand_a):
                a = cand_a
            if cand_t:
                t = _strip_query_noise(cand_t)
        if not a or _is_junk_artist(a):
            m_feat = re.search(r"[\(\[](?:performed by|feat\.?|featuring)\s+(.+?)[\)\]\…\.\,]", title, flags=re.I)
            if m_feat:
                cand_feat = m_feat.group(1).strip()
                if cand_feat and not _is_junk_artist(cand_feat):
                    a = cand_feat

    if not t:
        return None, None, None

    # Track part after ' - ' if compound title (e.g. 'Shadowcast - Jack (...)')
    track_part = ""
    if " - " in t:
        parts = [p.strip() for p in t.split(" - ") if p.strip()]
        if len(parts) >= 2:
            track_part = parts[-1]

    # Primary artist if multiple artists listed (e.g. 'Biggsmoke & DRJ Sohail' -> 'Biggsmoke')
    primary_artist = ""
    if a and not _is_junk_artist(a):
        splits = re.split(r"\s*(?:&|,|\/|\bfeat\.?|\bfeaturing)\s*", a, flags=re.I)
        if len(splits) > 1 and splits[0].strip():
            primary_artist = splits[0].strip()

    clean_t = re.sub(r"^\d{1,3}\s*[-.]?\s*", "", t).strip()

    queries: list[str] = []

    if track_part:
        if a and not _is_junk_artist(a):
            queries.append(f"{track_part} {a}")
            queries.append(f"{a} {track_part}")
        queries.append(track_part)
        if a and not _is_junk_artist(a):
            queries.append(f"{t} {a}")
        queries.append(t)
    else:
        if clean_t and clean_t != t:
            if a and not _is_junk_artist(a):
                queries.append(f"{clean_t} {a}")
            queries.append(clean_t)

        if a and not _is_junk_artist(a):
            queries.append(f"{t} {a}")
            if primary_artist and primary_artist.lower() != a.lower():
                queries.append(f"{t} {primary_artist}")
            if al and al.lower() not in t.lower():
                queries.append(f"{t} {a} {al}")
        else:
            queries.append(t)
            if al:
                queries.append(f"{t} {al}")
            queries.append(f"{t} song")

    best_candidate: dict[str, Any] | None = None
    best_score: float = 0.0
    seen_vids: set[str] = set()

    for q in queries:
        candidates = await _search_youtube_innertube(q)

        for cand in candidates:
            vid = cand.get("video_id")
            if not vid or vid in seen_vids:
                continue
            seen_vids.add(vid)

            score = _match_youtube_candidate(
                target_title=title,
                target_artist=a,
                cand_title=cand.get("title", ""),
                cand_channel=cand.get("channel", ""),
                target_album=al,
            )
            if score > best_score:
                best_score = score
                best_candidate = cand

        if best_candidate and best_score >= 85.0:
            break

    if not best_candidate or best_score < 60.0:
        _dbg(f"[cover] youtube no match found for title={t!r} artist={a!r} (best_score={best_score})")
        return None, None, None

    vid = best_candidate.get("video_id")
    big_thumb = best_candidate.get("big_thumb")
    if not big_thumb and vid:
        big_thumb = f"https://i.ytimg.com/vi/{vid}/hqdefault.jpg"

    small_thumb = best_candidate.get("small_thumb") or (
        f"https://i.ytimg.com/vi/{vid}/mqdefault.jpg" if vid else None
    )

    _dbg(
        f"[cover] youtube match_found score={best_score} "
        f"track={t!r} artist={a!r} -> vid={vid} title={best_candidate.get('title')!r} "
        f"channel={best_candidate.get('channel')!r}"
    )

    meta = {
        "video_id": vid,
        "title": best_candidate.get("title"),
        "channel": best_candidate.get("channel"),
        "channel_avatar": best_candidate.get("avatar"),
        "score": best_score,
    }

    return big_thumb, small_thumb, meta


async def find_best_cover_url(*, title: str, artist: str, album: str = "", year: int | None = None) -> tuple[str | None, str | None, str | None]:
    use_spotify = bool(getattr(Config, "SPOTIFY_COVER_SEARCH", False))
    use_fallbacks = bool(getattr(Config, "MUSIC_HOADER_SEARCH", False))

    if not use_spotify and not use_fallbacks:
        use_spotify = True
        use_fallbacks = True

    if use_spotify and is_spotify_configured():
        try:
            if (album or "").strip():
                u = await spotify_album_cover_url(artist=artist, album=album, year=year)
                if u:
                    return u, "spotify_album", None
            u = await spotify_cover_url(title=title, artist=artist, album=album, year=year)
            if u:
                return u, "spotify", None
        except Exception as e:
            _dbg(f"[cover] spotify failed err={e!r}")

    if use_fallbacks:
        try:
            name = (album or "").strip() or (title or "").strip()
            big, small = await hoaders_cover_info(artist=artist, album=name, year=year)
            if big:
                return big, "hoaders", small
        except Exception as e:
            _dbg(f"[cover] hoaders failed err={e!r}")

        try:
            u = await apple_cover_url(title=title, artist=artist, album=album, year=year)
            if u:
                return u, "apple", None
        except Exception as e:
            _dbg(f"[cover] apple failed err={e!r}")

        try:
            u = await deezer_cover_url(title=title, artist=artist, album=album, year=year)
            if u:
                return u, "deezer", None
        except Exception as e:
            _dbg(f"[cover] deezer failed err={e!r}")

        # Fallback to YouTube search matching track > artists or channel
        if getattr(Config, "YOUTUBE_COVER_SEARCH", True):
            try:
                yt_big, yt_small, yt_meta = await youtube_cover_search(
                    title=title,
                    artist=artist,
                    album=album,
                    year=year,
                )
                if yt_big:
                    return yt_big, "youtube", yt_small
            except Exception as e:
                _dbg(f"[cover] youtube search failed err={e!r}")

    return None, None, None

