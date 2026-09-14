import asyncio
from asyncio import sleep
import html
import time
from secrets import token_hex
try:
    from telegraph.aio import Telegraph
    from telegraph.exceptions import RetryAfterError
except Exception:
    Telegraph = None
    RetryAfterError = Exception

from ... import LOGGER
from ...core.config_manager import Config
from .lyrics import fetch_best_lyrics, score_match

LOG = LOGGER(__name__)

_TELEGRAPH_HELPER = None
_TELEGRAPH_INIT_LOCK = asyncio.Lock()

class TelegraphHelper:
    _cooldown_until: float = 0.0

    def __init__(self, author_name=None, author_url=None):
        if Telegraph is None:
            raise RuntimeError("telegraph package not installed")
        self._telegraph = Telegraph(domain="graph.org")
        self._author_name = author_name
        self._author_url = author_url

    async def create_account(self):
        if time.time() < TelegraphHelper._cooldown_until:
            return None
        LOG.debug("Creating Telegraph Account")
        try:
            out = await self._telegraph.create_account(
                short_name=token_hex(5),
                author_name=self._author_name,
                author_url=self._author_url,
            )
            LOG.debug("Telegraph account created")
            return out
        except RetryAfterError as st:
            delay = float(getattr(st, "retry_after", 60) or 60)
            TelegraphHelper._cooldown_until = time.time() + delay
            LOG.warning(
                f"Telegraph create_account flood limit exceeded. Cooling down for {int(delay)} seconds."
            )
            return None
        except Exception as e:
            LOG.error(f"Failed to create Telegraph Account: {e}", exc_info=True)
            raise

    async def create_page(self, title, content):
        if time.time() < TelegraphHelper._cooldown_until:
            LOG.debug("Telegraph in flood cooldown, skipping page creation")
            return None
        try:
            out = await self._telegraph.create_page(
                title=title,
                author_name=self._author_name,
                author_url=self._author_url,
                html_content=content,
            )
            return out
        except RetryAfterError as st:
            delay = float(getattr(st, "retry_after", 60) or 60)
            TelegraphHelper._cooldown_until = time.time() + delay
            LOG.warning(
                f"Telegraph rate limit exceeded (RetryAfter {int(delay)}s). Skipping Telegraph export until cooldown expires."
            )
            return None
        except Exception as e:
            LOG.error(f"Telegraph create_page failed: {e}", exc_info=True)
            return None

    async def edit_page(self, path, title, content):
        if time.time() < TelegraphHelper._cooldown_until:
            return None
        try:
            return await self._telegraph.edit_page(
                path=path,
                title=title,
                author_name=self._author_name,
                author_url=self._author_url,
                html_content=content,
            )
        except RetryAfterError as st:
            delay = float(getattr(st, "retry_after", 60) or 60)
            TelegraphHelper._cooldown_until = time.time() + delay
            LOG.warning(
                f"Telegraph edit_page flood limit exceeded. Cooling down for {int(delay)} seconds."
            )
            return None
        except Exception as e:
            LOG.error(f"Telegraph edit_page failed: {e}", exc_info=True)
            return None

    async def edit_telegraph(self, path, telegraph_content):
        nxt_page = 1
        prev_page = 0
        num_of_path = len(path)
        for content in telegraph_content:
            if nxt_page == 1:
                content += (
                    f'<b><a href="https://telegra.ph/{path[nxt_page]}">Next</a></b>'
                )
                nxt_page += 1
            else:
                if prev_page <= num_of_path:
                    content += f'<b><a href="https://telegra.ph/{path[prev_page]}">Prev</a></b>'
                    prev_page += 1
                if nxt_page < num_of_path:
                    content += f'<b> | <a href="https://telegra.ph/{path[nxt_page]}">Next</a></b>'
                    nxt_page += 1
            await self.edit_page(
                path=path[prev_page],
                title="Lyrics",
                content=content,
            )
        return


async def publish_track_lyrics_to_graph(
    *,
    track_id: str | None = None,
    title: str,
    artist: str,
    album: str | None = None,
) -> dict:
    track_id = (track_id or "").strip() or None
    title = (title or "").strip()
    artist = (artist or "").strip()
    album = (album or "").strip() or None
    if not title or not artist:
        return {"ok": False, "error": "title_and_artist_required"}

    LOG.info(f"lyrics publish start track_id={track_id} title={title!r} artist={artist!r} album={album!r}")

    try:
        lr = await fetch_best_lyrics(title=title, artist=artist, album=album)
    except Exception as e:
        q = " ".join([p for p in [title, artist] if p]).strip() or title
        LOG.warning(f"LRCLIB request failed q={q!r}: {e}", exc_info=True)
        lr = {"ok": False, "error": "lrclib_failed"}

    if not lr.get("ok"):
        LOG.info(f"lyrics publish no_match track_id={track_id} title={title!r} artist={artist!r} album={album!r}")
        if lr.get("error") == "no_lyrics":
            match = lr.get("match") if isinstance(lr.get("match"), dict) else {}
            LOG.info(f"lyrics publish empty_lyrics track_id={track_id} lrclib_id={match.get('id')}")
            return {"ok": False, "error": "no_lyrics"}
        return {"ok": False, "error": "no_accurate_match"}

    match = lr.get("match") if isinstance(lr.get("match"), dict) else {}
    lyrics = (lr.get("lyrics") or "").strip()
    kind = lr.get("kind") or "plain"

    try:
        q = " ".join([p for p in [title, artist] if p]).strip() or title
        s = score_match(match, title, artist, album)
        LOG.info(
            "LRCLIB match "
            + str(
                {
                    "q": q,
                    "score": s,
                    "want": {"title": title, "artist": artist, "album": album},
                    "got": {
                        "title": match.get("trackName"),
                        "artist": match.get("artistName"),
                        "album": match.get("albumName"),
                        "id": match.get("id"),
                        "synced": bool(match.get("syncedLyrics")),
                        "plain": bool(match.get("plainLyrics")),
                    },
                }
            )
        )
    except Exception:
        pass

    content = "<pre>" + html.escape(lyrics) + "</pre>"
    if album:
        content = "<b>" + html.escape(artist) + "</b><br/><i>" + html.escape(album) + "</i><br/>" + content
    else:
        content = "<b>" + html.escape(artist) + "</b><br/>" + content

    global _TELEGRAPH_HELPER, _TELEGRAPH_INIT_LOCK
    async with _TELEGRAPH_INIT_LOCK:
        if _TELEGRAPH_HELPER is None:
            author_url = getattr(Config, "AUTHOR_URL", None)
            helper = TelegraphHelper(author_name=artist, author_url=author_url)
            LOG.info(f"Telegraph init start author_url={author_url!r}")
            await helper.create_account()
            _TELEGRAPH_HELPER = helper
            LOG.info("Telegraph init done")

    try:
        _TELEGRAPH_HELPER._author_name = artist
    except Exception:
        pass

    try:
        LOG.info(f"Telegraph create_page start title={title!r} author={artist!r} bytes={len(content)}")
        page = await _TELEGRAPH_HELPER.create_page(title=title, content=content)
    except Exception as e:
        LOG.error(f"lyrics publish telegraph_failed track_id={track_id}: {e}", exc_info=True)
        return {"ok": False, "error": "telegraph_create_failed"}
    url = None
    if isinstance(page, dict):
        url = page.get("url")
    if not url:
        LOG.error(f"lyrics publish telegraph_no_url track_id={track_id} page={page!r}")
        return {"ok": False, "error": "telegraph_no_url"}

    db_updated = False
    if url and track_id:
        try:
            from stream.database.MongoDb import db_handler

            await db_handler.audio_collection.update_one(
                {"_id": str(track_id)},
                {"$set": {"lyrics": str(url), "updated_at": time.time()}},
                upsert=False,
            )
            db_updated = True
            LOG.info(f"lyrics db updated track_id={track_id} url={url}")
        except Exception as e:
            LOG.warning(f"lyrics db update failed track_id={track_id}: {e}", exc_info=True)

    return {
        "ok": bool(url),
        "url": url,
        "db_updated": db_updated,
        "lrclib_id": match.get("id"),
        "lrclib_title": match.get("trackName"),
        "lrclib_artist": match.get("artistName"),
        "lrclib_album": match.get("albumName"),
        "lyrics_kind": kind,
    }


async def publish_lyrics_text_to_graph(
    *,
    track_id: str | None = None,
    title: str,
    artist: str,
    album: str | None = None,
    lyrics: str,
) -> dict:
    track_id = (track_id or "").strip() or None
    title = (title or "").strip()
    artist = (artist or "").strip()
    album = (album or "").strip() or None
    lyrics = (lyrics or "").strip()

    if not title or not artist:
        return {"ok": False, "error": "title_and_artist_required"}
    if not lyrics:
        return {"ok": False, "error": "no_lyrics"}

    content = "<pre>" + html.escape(lyrics) + "</pre>"
    if album:
        content = "<b>" + html.escape(artist) + "</b><br/><i>" + html.escape(album) + "</i><br/>" + content
    else:
        content = "<b>" + html.escape(artist) + "</b><br/>" + content

    global _TELEGRAPH_HELPER, _TELEGRAPH_INIT_LOCK
    async with _TELEGRAPH_INIT_LOCK:
        if _TELEGRAPH_HELPER is None:
            author_url = getattr(Config, "AUTHOR_URL", None)
            helper = TelegraphHelper(author_name=artist, author_url=author_url)
            await helper.create_account()
            _TELEGRAPH_HELPER = helper

    try:
        _TELEGRAPH_HELPER._author_name = artist
    except Exception:
        pass

    try:
        page = await _TELEGRAPH_HELPER.create_page(title=title, content=content)
    except Exception:
        return {"ok": False, "error": "telegraph_create_failed"}

    url = page.get("url") if isinstance(page, dict) else None
    if not url:
        return {"ok": False, "error": "telegraph_no_url"}

    db_updated = False
    if track_id:
        try:
            from stream.database.MongoDb import db_handler

            await db_handler.audio_collection.update_one(
                {"_id": str(track_id)},
                {"$set": {"lyrics": str(url), "updated_at": time.time()}},
                upsert=False,
            )
            db_updated = True
        except Exception:
            db_updated = False

    return {"ok": True, "url": url, "db_updated": db_updated}
