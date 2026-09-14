import asyncio
from os import environ
import time
import datetime

_pyrogram_error = None
try:
    from pyrogram import Client, enums
except Exception as e:
    Client = None
    enums = None
    _pyrogram_error = e
from .helpers.logger import LOGGER
try:
    from apscheduler.schedulers.asyncio import AsyncIOScheduler
except Exception:
    AsyncIOScheduler = None
try:
    from apscheduler.triggers.cron import CronTrigger
except Exception:
    CronTrigger = None
from stream.core.config_manager import Config


Config.load()

BotStartTime = time.time()
LOGGER(__name__).info("Initializing...")

_only_api = bool(getattr(Config, "ONLY_API", False))

bot = None
scheduler = None
if not _only_api:
    if Client is None or enums is None:
        raise SystemExit(f"pyrogram is required when ONLY_API is False (import error: {_pyrogram_error})")
    bot = Client(
        name="Stream",
        api_id=Config.API_ID,
        api_hash=Config.API_HASH,
        bot_token=Config.BOT_TOKEN,
        plugins=dict(root="stream.plugins"),
        workers=8,
        parse_mode=enums.ParseMode.MARKDOWN,
        in_memory=False,
    )
    if AsyncIOScheduler is not None:
        scheduler = AsyncIOScheduler(event_loop=bot.loop)
elif AsyncIOScheduler is not None:
    scheduler = AsyncIOScheduler()


def _as_int_list(value: object) -> list[int]:
    if value is None:
        return []
    if isinstance(value, (list, tuple, set)):
        raw = list(value)
    else:
        s = str(value).strip()
        if not s:
            return []
        raw = [p for p in s.replace(",", " ").split() if p.strip()]
    out: list[int] = []
    seen: set[int] = set()
    for v in raw:
        try:
            n = int(v)
        except Exception:
            continue
        if n in seen:
            continue
        seen.add(n)
        out.append(n)
    return out


async def refresh_daily_playlists(date: str | None = None) -> None:
    if not date:
        date = datetime.datetime.utcnow().date().isoformat()

    from Api.services.track_service import generate_daily_playlist

    keys = ["random", "top-played"]
    channels: list[int | None] = [None]

    ch = getattr(Config, "CHANNEL_ID", None)
    try:
        if ch is not None:
            channels.append(int(ch))
    except Exception:
        pass

    for cid in _as_int_list(getattr(Config, "SOURCE_CHANNEL_IDS", None)):
        channels.append(int(cid))

    deduped: list[int | None] = []
    seen2: set[str] = set()
    for c in channels:
        k = str(c) if c is not None else ""
        if k in seen2:
            continue
        seen2.add(k)
        deduped.append(c)

    for cid in deduped:
        for key in keys:
            try:
                await generate_daily_playlist(key=key, date=str(date), channel_id=cid, limit=75)
                await ensure_daily_playlist_cover(key=key, date=str(date), channel_id=cid)
            except Exception:
                continue


def _tg_userpic_url(username: str | None) -> str | None:
    u = (username or "").strip().lstrip("@").strip()
    if not u:
        return None
    if not u.replace("_", "").isalnum():
        return None
    if len(u) < 5 or len(u) > 32:
        return None
    return f"https://t.me/i/userpic/320/{u}.jpg"


async def refresh_user_profiles(*, limit_users: int = 2000) -> dict[str, int | bool | str]:
    if bot is None:
        return {"ok": False, "skipped": True, "reason": "bot disabled"}

    from stream.database.MongoDb import db_handler

    now = time.time()
    cutoff = now - (24 * 60 * 60)

    col = db_handler.get_collection("users").collection
    cursor = col.find(
        {"$or": [{"profile_refreshed_at": {"$exists": False}}, {"profile_refreshed_at": {"$lt": cutoff}}]},
        {"_id": 1},
    ).limit(int(limit_users))

    scanned = 0
    updated = 0
    failed = 0

    async for doc in cursor:
        scanned += 1
        try:
            uid = int(doc.get("_id") or 0)
        except Exception:
            uid = 0
        if uid <= 0:
            continue

        try:
            u = await bot.get_users(uid)
        except Exception:
            failed += 1
            continue

        first_name = (getattr(u, "first_name", None) or "").strip() or None
        tg_username = (getattr(u, "username", None) or "").strip() or None
        profile_url = _tg_userpic_url(tg_username)
        photo_url = profile_url

        set_fields: dict[str, object] = {"profile_refreshed_at": now, "updated_at": now}
        if first_name:
            set_fields["first_name"] = first_name
        if tg_username:
            set_fields["telegram"] = {"id": uid, "username": tg_username}
        else:
            set_fields["telegram"] = {"id": uid}
        if profile_url:
            set_fields["profile_url"] = profile_url
            set_fields["photo_url"] = photo_url

        try:
            await col.update_one({"_id": uid}, {"$set": set_fields}, upsert=True)
            updated += 1
        except Exception:
            failed += 1
            continue

    return {"ok": True, "scanned": scanned, "updated": updated, "failed": failed}


async def reconcile_deleted_tracks(*, limit_tracks: int = 500) -> dict[str, int | bool | str]:
    if bot is None:
        return {"ok": False, "skipped": True, "reason": "bot disabled"}

    from stream.database.MongoDb import db_handler

    now = time.time()
    col = db_handler.audio_collection.collection

    cursor = (
        col.find(
            {
                "deleted": {"$ne": True},
                "source_chat_id": {"$exists": True},
                "source_message_id": {"$exists": True},
            },
            {"_id": 1, "source_chat_id": 1, "source_message_id": 1},
        )
        .sort([("updated_at", -1)])
        .limit(int(limit_tracks))
    )

    scanned = 0
    marked = 0
    failed = 0

    async for doc in cursor:
        scanned += 1
        try:
            chat_id = int(doc.get("source_chat_id") or 0)
            message_id = int(doc.get("source_message_id") or 0)
        except Exception:
            continue
        if chat_id == 0 or message_id <= 0:
            continue

        ok = False
        try:
            msg = await bot.get_messages(chat_id, message_id)
            media = getattr(msg, "audio", None) or getattr(msg, "document", None)
            ok = bool(media)
        except Exception as e:
            msg_str = str(e).upper()
            if "MESSAGE_ID_INVALID" in msg_str or "MSG_ID_INVALID" in msg_str or "MESSAGE_NOT_FOUND" in msg_str:
                ok = False
            else:
                failed += 1
                continue

        if ok:
            continue

        try:
            await col.update_one(
                {"_id": doc.get("_id")},
                {"$set": {"deleted": True, "deleted_at": now, "updated_at": now}},
            )
            marked += 1
        except Exception:
            failed += 1
            continue

    return {"ok": True, "scanned": scanned, "marked": marked, "failed": failed}


def add_daily_playlist_jobs(log=None) -> None:
    log = log or LOGGER(__name__)
    if scheduler is None or CronTrigger is None:
        log.info("Daily playlist scheduler disabled (missing scheduler)")
        return

    scheduler.add_job(
        refresh_daily_playlists,
        trigger=CronTrigger(hour=0, minute=0, timezone="UTC"),
        id="daily_playlists_refresh",
        name="Daily Playlists Refresh",
        misfire_grace_time=300,
        max_instances=1,
        next_run_time=datetime.datetime.utcnow() + datetime.timedelta(seconds=20),
        replace_existing=True,
    )


def add_user_profile_refresh_jobs(log=None) -> None:
    log = log or LOGGER(__name__)
    if scheduler is None or CronTrigger is None:
        log.info("User profile refresh scheduler disabled (missing scheduler)")
        return
    scheduler.add_job(
        refresh_user_profiles,
        trigger=CronTrigger(hour=3, minute=0, timezone="UTC"),
        id="user_profiles_refresh",
        name="User Profiles Refresh",
        misfire_grace_time=300,
        max_instances=1,
        next_run_time=datetime.datetime.utcnow() + datetime.timedelta(seconds=60),
        replace_existing=True,
    )


def add_deleted_track_reconcile_jobs(log=None) -> None:
    log = log or LOGGER(__name__)
    if scheduler is None or CronTrigger is None:
        log.info("Deleted track reconcile scheduler disabled (missing scheduler)")
        return
    scheduler.add_job(
        reconcile_deleted_tracks,
        trigger=CronTrigger(hour="*/6", minute=5, timezone="UTC"),
        id="tracks_deleted_reconcile",
        name="Tracks Deleted Reconcile",
        misfire_grace_time=300,
        max_instances=1,
        next_run_time=datetime.datetime.utcnow() + datetime.timedelta(seconds=90),
        replace_existing=True,
    )

multi_clients: dict[int, object] = {}
work_loads: dict[int, int] = {}
_multi_lock = asyncio.Lock()
_multi_initialized = False
_rr_cursor: int = 0
_primary_user_id: int | None = None


def get_primary_client_user_id() -> int | None:
    return _primary_user_id

async def initialize_multi_clients(log=None, primary_user_id: int | None = None) -> None:
    log = log or LOGGER(__name__)
    if bot is None:
        log.info("Multi-client init skipped (bot disabled)")
        return
    global _multi_initialized
    if _multi_initialized:
        log.info("Multi-client init skipped (already initialized)")
        return
    _multi_initialized = True
    global _primary_user_id

    async with _multi_lock:
        multi_clients.clear()
        work_loads.clear()
        _primary_user_id = None

    try:
        if primary_user_id is None:
            me = await bot.get_me()
            primary_user_id = int(getattr(me, "id"))
        primary_user_id = int(primary_user_id)
    except Exception:
        primary_user_id = None

    if primary_user_id is not None:
        async with _multi_lock:
            _primary_user_id = primary_user_id
            multi_clients[primary_user_id] = bot
            work_loads[primary_user_id] = 0
        log.debug(f"Primary client cached user_id={primary_user_id}")

    enabled = bool(getattr(Config, "MULTI_CLIENTS", False))
    if not enabled:
        log.info("Multi-client mode disabled (Config.MULTI_CLIENTS=false)")
        return

    tokens: list[str] = []
    for key in ("MULTI_CLIENTS_1", "MULTI_CLIENTS_2", "MULTI_CLIENTS_3", "MULTI_CLIENTS_4"):
        v = (getattr(Config, key, "") or "").strip()
        if v:
            tokens.append(v)

    configured = getattr(Config, "MULTI_CLIENT_TOKENS", None)
    if isinstance(configured, (list, tuple, set)):
        for v in configured:
            vv = (str(v or "")).strip()
            if vv:
                tokens.append(vv)

    for k, v in sorted(environ.items()):
        if k.startswith("MULTI_TOKEN"):
            vv = (v or "").strip()
            if vv:
                tokens.append(vv)

    deduped: list[str] = []
    seen: set[str] = set()
    for t in tokens:
        tt = (t or "").strip()
        if not tt or tt in seen:
            continue
        seen.add(tt)
        deduped.append(tt)
    tokens = deduped

    if not tokens:
        log.warning("Multi-client enabled but no tokens found")
        return

    async def start_client(client_id: int, token: str):
        session_string = token if len(token) >= 100 else None
        bot_token = None if session_string else token
        try:
            client = Client(
                name=f"StreamMulti{client_id}",
                api_id=int(Config.API_ID),
                api_hash=str(Config.API_HASH),
                bot_token=bot_token,
                session_string=session_string,
                workers=4,
                in_memory=True,
                no_updates=True,
            )
            await client.start()
            await client.get_me()
            return client_id, client
        except Exception as e:
            log.warning(f"Multi client {client_id} failed to start: {e}")
            return None

    results = await asyncio.gather(
        *[start_client(i + 1, t) for i, t in enumerate(tokens)],
        return_exceptions=False,
    )

    ok: list[tuple[int, Client]] = [r for r in results if isinstance(r, tuple)]
    if not ok:
        log.warning("Multi-client enabled but no additional clients started successfully")
        return

    async with _multi_lock:
        for _, client in ok:
            try:
                me = await client.get_me()
                uid = int(getattr(me, "id"))
            except Exception:
                continue
            if uid not in multi_clients:
                multi_clients[uid] = client
                work_loads[uid] = 0
                log.debug(f"Multi client registered user_id={uid}")

    total = len(multi_clients)
    if total <= 1:
        log.warning("Multi-client enabled but no additional clients registered successfully")
        return

    async with _multi_lock:
        clients = list(multi_clients.items())

    parts: list[str] = []
    for uid, client in sorted(clients, key=lambda x: int(x[0])):
        try:
            me = await client.get_me()
            first = (getattr(me, "first_name", "") or "").strip()
            username = (getattr(me, "username", "") or "").strip()
        except Exception:
            first = ""
            username = ""

        label = ""
        if first and username:
            label = f"{first} (@{username})"
        elif username:
            label = f"@{username}"
        elif first:
            label = first
        else:
            label = "Unknown"

        parts.append(f"{label} [ID: {int(uid)}]")

    log.info("Multi-client mode enabled: " + ", ".join(parts))


async def stop_multi_clients(log=None) -> None:
    log = log or LOGGER(__name__)
    async with _multi_lock:
        clients = [(cid, c) for cid, c in multi_clients.items() if cid != _primary_user_id]
        multi_clients.clear()
        work_loads.clear()
        if _primary_user_id is not None and bot is not None:
            multi_clients[_primary_user_id] = bot
            work_loads[_primary_user_id] = 0

    for cid, client in clients:
        try:
            await asyncio.wait_for(client.stop(), timeout=3.0)
        except Exception as e:
            log.warning(f"Multi client {cid} stop failed: {e}")

    if clients:
        log.info(f"Multi-client stopped ({len(clients)} additional clients)")


async def acquire_stream_client() -> tuple[int, Client]:
    if bool(getattr(Config, "ONLY_API", False)) or bot is None:
        raise RuntimeError("Streaming clients are disabled (ONLY_API=true)")
    async with _multi_lock:
        candidates = list(work_loads.keys())
        if bool(getattr(Config, "MULTI_CLIENTS", False)) and len(candidates) > 1 and _primary_user_id is not None:
            candidates = [c for c in candidates if c != _primary_user_id] or candidates
        if not candidates:
            raise RuntimeError("No streaming clients available")

        min_load = min(work_loads.get(c, 0) for c in candidates)
        tied = sorted([c for c in candidates if int(work_loads.get(c) or 0) == int(min_load)])

        global _rr_cursor
        if tied:
            start_idx = 0
            if _rr_cursor in tied:
                start_idx = (tied.index(_rr_cursor) + 1) % len(tied)
            cid = tied[start_idx]
            _rr_cursor = cid
        else:
            cid = min(work_loads, key=work_loads.get)

        work_loads[cid] = int(work_loads.get(cid) or 0) + 1
        return cid, multi_clients[cid]


async def acquire_stream_client_prefer(preferred_user_ids: set[int] | list[int] | None) -> tuple[int, Client]:
    if bool(getattr(Config, "ONLY_API", False)) or bot is None:
        raise RuntimeError("Streaming clients are disabled (ONLY_API=true)")
    async with _multi_lock:
        candidates = list(work_loads.keys())
        if bool(getattr(Config, "MULTI_CLIENTS", False)) and len(candidates) > 1 and _primary_user_id is not None:
            candidates = [c for c in candidates if c != _primary_user_id] or candidates

        preferred: set[int] = set()
        if preferred_user_ids:
            try:
                preferred = {int(x) for x in preferred_user_ids}
            except Exception:
                preferred = set()

        if preferred:
            picked = [c for c in candidates if c in preferred]
            if picked:
                candidates = picked

        if not candidates:
            raise RuntimeError("No streaming clients available")

        min_load = min(work_loads.get(c, 0) for c in candidates)
        tied = sorted([c for c in candidates if int(work_loads.get(c) or 0) == int(min_load)])

        global _rr_cursor
        if tied:
            start_idx = 0
            if _rr_cursor in tied:
                start_idx = (tied.index(_rr_cursor) + 1) % len(tied)
            cid = tied[start_idx]
            _rr_cursor = cid
        else:
            cid = min(work_loads, key=work_loads.get)

        work_loads[cid] = int(work_loads.get(cid) or 0) + 1
        return cid, multi_clients[cid]


async def release_stream_client(client_id: int) -> None:
    async with _multi_lock:
        if client_id not in work_loads:
            return
        v = int(work_loads.get(client_id) or 0) - 1
        work_loads[client_id] = v if v > 0 else 0
