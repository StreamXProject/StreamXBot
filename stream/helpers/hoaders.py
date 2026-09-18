import asyncio
import json
import re

from aiohttp import ClientSession, ClientTimeout

from stream.core.config_manager import Config
from stream.helpers.logger import LOGGER

URL = "https://covers.musichoarders.xyz/api/search"

HEADERS = {
    "Accept": "*/*",
    "Accept-Encoding": "gzip, deflate",
    "Accept-Language": "en-US,en;q=0.5",
    "Content-Type": "application/json",
    "Origin": "https://covers.musichoarders.xyz",
    "Referer": "https://covers.musichoarders.xyz/",
    "User-Agent": "Mozilla/5.0",
}

_SEM = asyncio.Semaphore(3)


logger = LOGGER(__name__)

def _dbg(msg: str) -> None:
    logger.debug(msg)



def _parse_ndjson(text: str) -> list[dict]:
    items: list[dict] = []
    for line in (text or "").splitlines():
        s = (line or "").strip()
        if not s:
            continue
        try:
            obj = json.loads(s)
        except Exception:
            continue
        if isinstance(obj, dict):
            items.append(obj)
    return items


def _norm_cmp(text: str) -> str:
    s = (text or "").casefold()
    s = re.sub(r"[\[\(].*?[\]\)]", " ", s)
    s = re.sub(r"[^\w\s]+", " ", s, flags=re.UNICODE)
    s = re.sub(r"\s+", " ", s).strip()
    return s


def _match_points(want: str, got: str) -> int:
    if not want or not got:
        return 0
    if got == want:
        return 4
    if want in got or got in want:
        return 2
    return 0


def _confidence_score(confidence: str) -> int:
    c = (confidence or "").strip().casefold()
    if c == "high":
        return 3
    if c == "medium":
        return 2
    if c == "low":
        return 1
    return 0


def _release_year(release_info: dict) -> int | None:
    if not isinstance(release_info, dict):
        return None
    y = release_info.get("releaseYear")
    if y is not None:
        try:
            return int(y)
        except Exception:
            return None
    date = (release_info.get("date") or release_info.get("releaseDate") or "").strip()
    if len(date) >= 4 and date[:4].isdigit():
        try:
            return int(date[:4])
        except Exception:
            return None
    return None


def _select_best_cover(*, covers: list[dict], artist: str, album: str, year: int | None = None) -> dict | None:
    if not covers:
        return None

    want_album = _norm_cmp(album)
    want_artist = _norm_cmp(artist)
    want_year = None
    if year is not None:
        try:
            want_year = int(year)
        except Exception:
            want_year = None

    priority = {
        "spotify": 4,
        "applemusic": 3,
        "amazonmusic": 2,
        "lastfm": 1,
    }

    best = None
    best_score = -1
    for c in covers:
        if not isinstance(c, dict):
            continue
        rel = c.get("releaseInfo") if isinstance(c.get("releaseInfo"), dict) else {}
        rel_title = _norm_cmp(rel.get("title") or "")
        rel_artist = _norm_cmp(rel.get("artist") or "")

        title_match = _match_points(want_album, rel_title)
        artist_match = _match_points(want_artist, rel_artist)
        if want_album and title_match == 0:
            continue

        src = str(c.get("source") or "").strip().casefold()
        src_score = priority.get(src, 0)
        conf = _confidence_score(c.get("confidence") or "")
        is_original = 2 if bool(c.get("isOriginal")) else 0
        year_score = 0
        if want_year is not None:
            ry = _release_year(rel)
            if ry is not None and ry == want_year:
                year_score = 2

        url = (c.get("bigCoverUrl") or c.get("smallCoverUrl") or "").strip()
        if url.lower().endswith(".mp4"):
            continue

        score = 0
        score += title_match * 5
        score += artist_match * 3
        score += src_score * 2
        score += conf
        score += is_original
        score += year_score
        if c.get("bigCoverUrl"):
            score += 1

        if score > best_score:
            best = c
            best_score = score

    return best


async def hoaders_search(*, artist: str, album: str, country: str = "in", sources: list[str] | None = None) -> tuple[list[dict], int | None]:
    a = (artist or "").strip()
    al = (album or "").strip()
    if not a or not al:
        return [], None

    payload = {
        "artist": a,
        "album": al,
        "country": (country or "in").strip() or "in",
        "sources": sources or ["amazonmusic", "applemusic", "lastfm", "spotify"],
    }

    async with _SEM:
        async with ClientSession(headers=HEADERS, timeout=ClientTimeout(total=6.0)) as session:
            try:
                async with session.post(URL, json=payload) as resp:
                    text = await resp.text()
                    status = int(resp.status)
            except Exception as e:
                _dbg(f"[hoaders] request failed: {e}")
                return [], None

    items = _parse_ndjson(text)
    covers = [i for i in items if isinstance(i, dict) and i.get("type") == "cover"]
    _dbg(f"[hoaders] status={status} parsed={len(items)} covers={len(covers)} artist={a!r} album={al!r}")
    if covers and bool(getattr(Config, "DEBUG", False)):
        _dbg(json.dumps(covers, indent=10, ensure_ascii=False))
    return covers, status


async def hoaders_cover_info(*, artist: str, album: str, year: int | None = None, country: str = "in", sources: list[str] | None = None) -> tuple[str | None, str | None]:
    covers, status = await hoaders_search(artist=artist, album=album, country=country, sources=sources)
    if not covers:
        _dbg(f"[hoaders] no_covers status={status}")
        return None, None
    best = _select_best_cover(covers=covers, artist=artist, album=album, year=year)
    if not best:
        return None, None
    big = (best.get("bigCoverUrl") or best.get("smallCoverUrl") or "").strip()
    small = (best.get("smallCoverUrl") or best.get("bigCoverUrl") or "").strip()
    if big.lower().endswith(".mp4"):
        big = ""
    if small.lower().endswith(".mp4"):
        small = ""
    return big or None, small or None


async def hoaders_big_cover_url(*, artist: str, album: str, year: int | None = None, country: str = "in", sources: list[str] | None = None) -> str | None:
    big, _ = await hoaders_cover_info(artist=artist, album=album, year=year, country=country, sources=sources)
    return big

