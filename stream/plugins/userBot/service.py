import asyncio
import time

from pyrogram import Client, enums, filters
from pyrogram.errors import FloodWait, MessageNotModified, RPCError
from pyrogram.handlers import MessageHandler
from pyrogram.types import InlineKeyboardButton, InlineKeyboardMarkup

from stream import bot
from stream.core.config_manager import Config
from stream.database.MongoDb import db_handler

_USERBOT_INSTANCE: Client | None = None
_INDEX_TASKS: dict[int, asyncio.Event] = {}
_MAX_META_CAPTION_LEN = 1024


_AUDIO_EXTENSIONS = (
    ".mp3",
    ".flac",
    ".wav",
    ".wave",
    ".m4a",
    ".aac",
    ".ogg",
    ".opus",
    ".alac",
    ".aif",
    ".aiff",
    ".wma",
)


def _has_audio_media(message) -> bool:
    if getattr(message, "audio", None):
        return True
    doc = getattr(message, "document", None)
    if doc:
        mime = (getattr(doc, "mime_type", "") or "").lower()
        if mime.startswith("audio/"):
            return True
        file_name = (getattr(doc, "file_name", "") or "").lower()
        if file_name.endswith(_AUDIO_EXTENSIONS):
            return True
    return False


def _normalize_mime_type(mime: str | None, file_name: str | None = None) -> str:
    if file_name:
        fn = file_name.lower().strip()
        if fn.endswith((".wav", ".wave")):
            return "audio/wav"
        if fn.endswith(".flac"):
            return "audio/flac"
        if fn.endswith(".mp3"):
            return "audio/mpeg"
        if fn.endswith(".m4a"):
            return "audio/mp4"
        if fn.endswith((".ogg", ".opus")):
            return "audio/ogg"
        if fn.endswith(".aac"):
            return "audio/aac"
    if not mime:
        return "audio/mpeg"
    raw = str(mime).split(";")[0].strip().lower()
    if raw in {"audio/flac", "audio/x-flac"} or raw.endswith("/x-flac"):
        return "audio/flac"
    if raw in {"audio/wav", "audio/x-wav", "audio/wave"}:
        return "audio/wav"
    if raw in {"audio/mp3", "audio/mpeg"}:
        return "audio/mpeg"
    if raw in {"audio/m4a", "audio/x-m4a", "audio/mp4"}:
        return "audio/mp4"
    if raw in {"audio/ogg", "application/ogg"}:
        return "audio/ogg"
    if raw in {"audio/aac"}:
        return "audio/aac"
    return raw or "audio/mpeg"


def _chat_topic_setting() -> int | str:
    value = getattr(Config, "CHAT_TOPIC", 0)
    if isinstance(value, str):
        s = value.strip()
        if s.lower() == "all":
            return "all"
        try:
            return int(s)
        except Exception:
            return 0
    try:
        return int(value)
    except Exception:
        return 0


def _message_topic_id(message) -> int:
    chat = getattr(message, "chat", None)
    if chat:
        if getattr(chat, "type", None) == enums.ChatType.CHANNEL:
            return 0
        if hasattr(chat, "is_forum") and chat.is_forum is False:
            return 0
    thread_id = getattr(message, "message_thread_id", None)
    if thread_id is None:
        return 0
    try:
        return int(thread_id)
    except Exception:
        return 0


def _topic_allowed(topic_id: int, setting: int | str) -> bool:
    if setting == "all":
        return True
    try:
        target = int(setting)
    except Exception:
        target = 0
    if target == 0:
        return int(topic_id) == 0
    return int(topic_id) == target


def _uses_forum_topic_mode(setting: int | str) -> bool:
    if setting == "all":
        return True
    try:
        return int(setting) > 0
    except Exception:
        return False


def _clean_meta_value(value) -> str:
    return (
        str(value if value is not None else "")
        .replace("\r", " ")
        .replace("\n", " ")
        .strip()
    )


def _message_caption(message) -> str:
    caption = getattr(message, "caption", None)
    return str(caption or "").strip()


def _chat_ref_keys(value) -> set[str]:
    keys: set[str] = set()
    if value is None:
        return keys
    try:
        keys.add(str(int(value)))
        return keys
    except Exception:
        pass

    s = str(value or "").strip()
    if not s:
        return keys
    if s.startswith(("http://", "https://")):
        s = s.split("://", 1)[1]
        if "/" in s:
            s = s.split("/", 1)[1]
    s = s.strip().strip("/")
    if not s:
        return keys
    keys.add(s.casefold())
    if s.startswith("@"):
        keys.add(s[1:].casefold())
    else:
        keys.add(f"@{s.casefold()}")
    return keys


def _message_chat_keys(message) -> set[str]:
    chat = getattr(message, "chat", None)
    keys = _chat_ref_keys(getattr(chat, "id", None))
    username = getattr(chat, "username", None)
    if username:
        keys.update(_chat_ref_keys(str(username)))
    return keys


def _source_matches_message(message, source_ids: list[int | str]) -> bool:
    message_keys = _message_chat_keys(message)
    if not message_keys:
        return False
    for source_id in source_ids:
        if message_keys & _chat_ref_keys(source_id):
            return True
    return False


def _build_meta_caption(
    *,
    source_chat_id: int,
    source_message_id: int,
    topic_id: int,
    topic_name: str,
    original_caption: str,
) -> str:
    meta = "\n".join(
        [
            "#META",
            f"source_chat_id={int(source_chat_id)}",
            f"source_message_id={int(source_message_id)}",
            f"topic_id={int(topic_id)}",
            f"topic_name={_clean_meta_value(topic_name)}",
        ]
    )
    original = str(original_caption or "").strip()
    if not original:
        return meta[:_MAX_META_CAPTION_LEN]

    available = _MAX_META_CAPTION_LEN - len(meta) - 2
    if available <= 0:
        return meta[:_MAX_META_CAPTION_LEN]
    return f"{meta}\n\n{original[:available].rstrip()}"


def _topic_name_from_message(message, topic_id: int) -> str:
    if int(topic_id) == 0:
        return "main"
    for attr in ("topic", "forum_topic_created", "forum_topic_edited"):
        obj = getattr(message, attr, None)
        title = getattr(obj, "title", None) if obj is not None else None
        if isinstance(title, str) and title.strip():
            return title.strip()
    return f"topic_{int(topic_id)}"


def _coerce_int(value):
    if value is None or isinstance(value, bool):
        return None
    try:
        return int(value)
    except Exception:
        return None


async def _store_topic_metadata(
    *, source_chat_id: int | str, topic_id: int, topic_name: str
) -> None:
    chat_id = _coerce_int(source_chat_id)
    if chat_id is None:
        return
    cleaned_name = str(topic_name or "").strip()
    if not cleaned_name:
        return
    if cleaned_name.startswith("topic_"):
        try:
            existing = await db_handler.get_collection("forum_topics").collection.find_one(
                {"_id": f"{int(chat_id)}:{int(topic_id)}"},
                projection={"topic_name": 1},
            )
            if existing and existing.get("topic_name") and not str(existing["topic_name"]).startswith("topic_"):
                return
        except Exception:
            pass
    try:
        await db_handler.get_collection("forum_topics").update_one(
            {"_id": f"{int(chat_id)}:{int(topic_id)}"},
            {
                "$set": {
                    "source_chat_id": int(chat_id),
                    "topic_id": int(topic_id),
                    "topic_name": cleaned_name,
                    "updated_at": time.time(),
                }
            },
            upsert=True,
        )
    except Exception:
        pass


async def _resolve_topic_name(
    userbot: Client,
    source_chat_id: int | str,
    topic_id: int,
    message=None,
    log=None,
) -> str:
    if message:
        topic_name = _topic_name_from_message(message, topic_id)
    elif int(topic_id) == 0:
        topic_name = "main"
    else:
        topic_name = f"topic_{int(topic_id)}"
    if int(topic_id) != 0 and topic_name == f"topic_{int(topic_id)}":
        try:
            chat = None
            if message:
                chat = getattr(message, "chat", None)
            if not chat:
                chat = await userbot.get_chat(source_chat_id)
            if chat and getattr(chat, "type", None) != enums.ChatType.CHANNEL and getattr(chat, "is_forum", False):
                topic = await userbot.get_forum_topics_by_id(
                    source_chat_id, int(topic_id)
                )
                title = getattr(topic, "title", None)
                if isinstance(title, str) and title.strip():
                    topic_name = title.strip()
        except Exception as e:
            if log:
                log.debug(
                    f"userbot could not resolve topic name for {source_chat_id}:{topic_id}: {e}"
                )
    await _store_topic_metadata(
        source_chat_id=source_chat_id,
        topic_id=int(topic_id),
        topic_name=topic_name,
    )
    return topic_name


async def _get_source_channels() -> list[int | str]:
    ids: list[int | str] = []

    try:
        async for doc in db_handler.channels_collection.find_all(
            {"enabled": True}, projection={"_id": 1}
        ):
            v = doc.get("_id")
            if v is None:
                continue
            try:
                ids.append(int(v))
            except Exception:
                s = str(v).strip()
                if s:
                    ids.append(s)
    except Exception:
        pass

    cfg_ids = getattr(Config, "SOURCE_CHANNEL_IDS", None) or []
    for v in cfg_ids:
        if v is None:
            continue
        try:
            ids.append(int(v))
        except Exception:
            s = str(v).strip()
            if s:
                ids.append(s)

    out: list[int | str] = []
    seen: set[str] = set()
    for cid in ids:
        key = str(cid)
        if key not in seen:
            seen.add(key)
            out.append(cid)
    return out


async def _copy_with_backoff(
    userbot: Client,
    message,
    *,
    from_chat_id: int | str,
    topic_id: int,
    topic_name: str,
    log,
) -> None:
    db_channel_id = int(Config.CHANNEL_ID)
    source_chat_id = getattr(getattr(message, "chat", None), "id", None)
    if source_chat_id is None:
        source_chat_id = from_chat_id
    source_chat_id = int(source_chat_id)
    message_id = int(message.id)
    caption = _build_meta_caption(
        source_chat_id=source_chat_id,
        source_message_id=message_id,
        topic_id=int(topic_id),
        topic_name=topic_name,
        original_caption=_message_caption(message),
    )
    while True:
        try:
            if hasattr(message, "copy"):
                await message.copy(
                    chat_id=db_channel_id,
                    caption=caption,
                    parse_mode=enums.ParseMode.DISABLED,
                )
            else:
                await userbot.copy_message(
                    chat_id=db_channel_id,
                    from_chat_id=from_chat_id,
                    message_id=message_id,
                    caption=caption,
                    parse_mode=enums.ParseMode.DISABLED,
                )
            return
        except FloodWait as e:
            delay = int(getattr(e, "value", None) or getattr(e, "x", None) or 0)
            if delay <= 0:
                delay = 5
            log.warning(
                f"userbot floodwait {delay}s (from={from_chat_id} msg={message_id})"
            )
            await asyncio.sleep(delay)
        except RPCError as e:
            log.warning(
                f"userbot copy failed (from={from_chat_id} msg={message_id}): {e}"
            )
            raise


async def _forward_batch_with_backoff(
    userbot: Client,
    messages: list,
    *,
    source_chat_id: int | str,
    topic_id: int,
    topic_name: str,
    log,
) -> int:
    """Forward a batch of messages to db_channel_id with flood wait backoff.
    Forwards without sender name (hide_sender_name=True) and attaches metadata caption
    to the last track of the batch.
    Registers topic and forward metadata so audioIndex can resolve topic_id/topic_name.
    """
    if not messages:
        return 0

    db_channel_id = int(Config.CHANNEL_ID)
    from stream.plugins.db.audioIndex import (
        register_forward_topics,
        register_forwarded_batch,
    )

    msg_ids = [int(m.id) for m in messages]
    await register_forward_topics(source_chat_id, msg_ids, int(topic_id), topic_name)

    forwarded = None
    while True:
        try:
            forwarded = await userbot.forward_messages(
                chat_id=db_channel_id,
                from_chat_id=source_chat_id,
                message_ids=msg_ids,
                hide_sender_name=True,
            )
            break
        except FloodWait as e:
            delay = int(getattr(e, "value", None) or getattr(e, "x", None) or 5)
            log.warning(f"userbot floodwait {delay}s during batch forward, sleeping...")
            await asyncio.sleep(delay)
        except RPCError as e:
            log.warning(f"userbot batch forward failed: {e}")
            raise

    f_list = forwarded if isinstance(forwarded, list) else ([forwarded] if forwarded else [])
    if not f_list:
        return len(msg_ids)

    # Register destination-to-source mapping for audioIndex to resolve source chat and topic
    pairs = [(int(f.id), int(m.id)) for f, m in zip(f_list, messages)]
    await register_forwarded_batch(
        cache_chat_id=db_channel_id,
        forwarded_pairs=pairs,
        source_chat_id=source_chat_id,
        topic_id=int(topic_id),
        topic_name=topic_name,
    )

    # Attach caption based on USERBOT_CAPTION_MODE ("ALL", "LAST", "NONE")
    caption_mode = str(getattr(Config, "USERBOT_CAPTION_MODE", "LAST") or "LAST").strip().upper()
    targets = []
    if caption_mode == "ALL":
        targets = list(zip(f_list, messages))
    elif caption_mode == "LAST":
        last_src_idx = min(len(f_list) - 1, len(messages) - 1)
        targets = [(f_list[-1], messages[last_src_idx])]

    for f_msg, src_msg in targets:
        try:
            caption = _build_meta_caption(
                source_chat_id=int(source_chat_id),
                source_message_id=int(src_msg.id),
                topic_id=int(topic_id),
                topic_name=topic_name,
                original_caption=_message_caption(src_msg),
            )
            while True:
                try:
                    await userbot.edit_message_caption(
                        chat_id=db_channel_id,
                        message_id=int(f_msg.id),
                        caption=caption,
                        parse_mode=enums.ParseMode.DISABLED,
                    )
                    break
                except MessageNotModified:
                    break
                except FloodWait as fe:
                    delay = int(getattr(fe, "value", None) or getattr(fe, "x", None) or 5)
                    log.warning(f"userbot floodwait {delay}s editing caption for msg {f_msg.id}, sleeping...")
                    await asyncio.sleep(delay)
                except RPCError as re:
                    try:
                        await bot.edit_message_caption(
                            chat_id=db_channel_id,
                            message_id=int(f_msg.id),
                            caption=caption,
                            parse_mode=enums.ParseMode.DISABLED,
                        )
                    except Exception as be:
                        log.warning(f"failed to edit caption on msg {f_msg.id} via bot: {be}")
                    break
            if len(targets) > 1:
                await asyncio.sleep(0.05)
        except Exception as e:
            log.warning(f"failed to edit caption on msg {getattr(f_msg, 'id', None)}: {e}")

    return len(f_list)


async def _warm_up_dialogs(userbot: Client, log) -> None:
    try:
        async for _ in userbot.get_dialogs():
            pass
    except Exception as e:
        log.warning(f"userbot dialogs warmup failed: {e}")


async def _ensure_peer(userbot: Client, chat_id: int | str, log) -> bool:
    try:
        if isinstance(chat_id, str) and chat_id.startswith(("http://", "https://")):
            v = chat_id.split("://", 1)[1]
            v = v.split("/", 1)[1] if "/" in v else v
            chat_id = v
        await userbot.resolve_peer(chat_id)
        return True
    except Exception as e:
        log.warning(f"userbot cannot resolve peer {chat_id}: {e}")
        return False


async def _index_or_dump_audio_message(
    userbot: Client,
    source_chat_id: int | str,
    message,
    log,
    *,
    topic_id: int,
    topic_name: str | None = None,
    progress: dict | None = None,
) -> bool:
    if not _has_audio_media(message):
        return False

    from stream.core.source_filter import is_message_allowed

    allowed, reason = await is_message_allowed(message)
    if not allowed:
        log.debug(
            f"[userbot] Message {getattr(message, 'id', None)} rejected by source filter: {reason}"
        )
        return False

    source_chat_id_for_meta = getattr(getattr(message, "chat", None), "id", None)
    if source_chat_id_for_meta is None:
        source_chat_id_for_meta = source_chat_id

    resolved_topic_name = topic_name or await _resolve_topic_name(
        userbot,
        source_chat_id_for_meta,
        int(topic_id),
        message=message,
        log=log,
    )

    idx_mode = str(getattr(Config, "USERBOT_INDEX", "DUMP") or "DUMP").upper()
    force_dump = int(topic_id) != 0 or _uses_forum_topic_mode(_chat_topic_setting())
    if idx_mode == "INDEX" and not force_dump:
        from stream.plugins.db.audioIndex import (
            _pick_audio_media,
            _upsert_minimal,
        )

        media = _pick_audio_media(message)
        if not media:
            return False
        try:
            await _upsert_minimal(message, media)
        except Exception as e:
            log.warning(
                f"userbot local index failed for {source_chat_id}:{message.id}: {e}"
            )
            return False
    else:
        await _copy_with_backoff(
            userbot,
            message,
            from_chat_id=source_chat_id,
            topic_id=int(topic_id),
            topic_name=resolved_topic_name,
            log=log,
        )

    if progress is not None:
        progress["copied"] = int(progress.get("copied", 0)) + 1
    return True


async def _ingest_main_history(
    userbot: Client,
    source_chat_id: int | str,
    log,
    cancel_event: asyncio.Event | None = None,
    progress: dict | None = None,
) -> int:
    from stream.core.source_filter import is_source_banned

    if await is_source_banned(source_chat_id):
        log.warning(
            f"[userbot] Source chat {source_chat_id} is banned, skipping main history ingestion"
        )
        return 0

    state = db_handler.get_collection("userbot_state")
    state_id = f"history:{source_chat_id}:topic:0"
    doc = await state.read_document(state_id)
    if not doc:
        doc = await state.read_document(f"history:{source_chat_id}") or {}
    checkpoint_id = int(doc.get("last_message_id") or 0)
    max_seen_id = checkpoint_id

    mode = str(getattr(Config, "USERBOT_DUMP_MODE", "FORWARD") or "FORWARD").upper()
    cooldown = float(getattr(Config, "USERBOT_COOLDOWN_SEC", 0.2) or 0.2)
    batch_cooldown = float(getattr(Config, "USERBOT_BATCH_COOLDOWN_SEC", 1.0) or 1.0)
    batch_size = int(getattr(Config, "USERBOT_BATCH_SIZE", 50) or 50)
    if batch_size <= 0:
        batch_size = 50
    if cooldown < 0:
        cooldown = 0
    if batch_cooldown < 0:
        batch_cooldown = 0

    offset_id = 0
    copied = 0

    from stream.core.source_filter import is_message_allowed

    while True:
        if cancel_event and cancel_event.is_set():
            break

        batch = []
        async for m in userbot.get_chat_history(
            source_chat_id, offset_id=offset_id, limit=batch_size
        ):
            batch.append(m)
        if not batch:
            break

        stop_after = False
        if batch[-1].id <= checkpoint_id:
            stop_after = True

        valid_chunk = []
        for m in reversed(batch):
            if m.id <= checkpoint_id:
                continue
            max_seen_id = max(max_seen_id, int(m.id))
            if _message_topic_id(m) == 0 and _has_audio_media(m):
                allowed, _ = await is_message_allowed(m)
                if allowed:
                    valid_chunk.append(m)

        if valid_chunk:
            forward_success = False
            if mode == "FORWARD":
                try:
                    count = await _forward_batch_with_backoff(
                        userbot,
                        valid_chunk,
                        source_chat_id=source_chat_id,
                        topic_id=0,
                        topic_name="main",
                        log=log,
                    )
                    copied += count
                    if progress is not None:
                        progress["copied"] = int(progress.get("copied", 0)) + count
                    forward_success = True
                    if batch_cooldown:
                        await asyncio.sleep(batch_cooldown)
                except Exception as e:
                    log.warning(f"userbot main history forward failed: {e}")

            if not forward_success:
                for m in valid_chunk:
                    if cancel_event and cancel_event.is_set():
                        break
                    ok = await _index_or_dump_audio_message(
                        userbot,
                        source_chat_id,
                        m,
                        log,
                        topic_id=0,
                        topic_name="main",
                        progress=progress,
                    )
                    if ok:
                        copied += 1
                    if cooldown:
                        await asyncio.sleep(cooldown)

        await state.update_document(
            state_id,
            {
                "last_message_id": max_seen_id,
                "updated_at": asyncio.get_event_loop().time(),
            },
        )

        if stop_after:
            break

        offset_id = batch[-1].id

    return copied


async def _collect_topic_audio_messages(
    userbot: Client,
    source_chat_id: int | str,
    *,
    topic_id: int,
    last_id: int,
    cancel_event: asyncio.Event | None = None,
) -> list:
    messages: dict[int, object] = {}
    for media_filter in (enums.MessagesFilter.AUDIO, enums.MessagesFilter.DOCUMENT):
        if cancel_event and cancel_event.is_set():
            break
        async for msg in userbot.search_messages(
            source_chat_id,
            query="",
            filter=media_filter,
            min_id=int(last_id),
            message_thread_id=int(topic_id),
        ):
            if cancel_event and cancel_event.is_set():
                break
            msg_id = _coerce_int(getattr(msg, "id", None))
            if msg_id is None or msg_id <= int(last_id):
                continue
            if (
                getattr(msg, "message_thread_id", None) is not None
                and _message_topic_id(msg) != int(topic_id)
            ):
                continue
            if not _has_audio_media(msg):
                continue
            messages[int(msg_id)] = msg
    return [messages[k] for k in sorted(messages)]


async def _ingest_topic_history(
    userbot: Client,
    source_chat_id: int | str,
    log,
    *,
    topic_id: int,
    topic_name: str | None = None,
    cancel_event: asyncio.Event | None = None,
    progress: dict | None = None,
) -> int:
    from stream.core.source_filter import is_source_banned

    if await is_source_banned(source_chat_id):
        log.warning(
            f"[userbot] Source chat {source_chat_id} is banned, skipping topic history ingestion"
        )
        return 0

    state = db_handler.get_collection("userbot_state")
    state_id = f"history:{source_chat_id}:topic:{int(topic_id)}"
    doc = await state.read_document(state_id) or {}
    last_id = int(doc.get("last_message_id") or 0)

    mode = str(getattr(Config, "USERBOT_DUMP_MODE", "FORWARD") or "FORWARD").upper()
    cooldown = float(getattr(Config, "USERBOT_COOLDOWN_SEC", 0.2) or 0.2)
    batch_cooldown = float(getattr(Config, "USERBOT_BATCH_COOLDOWN_SEC", 1.0) or 1.0)
    batch_size = int(getattr(Config, "USERBOT_BATCH_SIZE", 50) or 50)
    if batch_size <= 0:
        batch_size = 50
    if cooldown < 0:
        cooldown = 0
    if batch_cooldown < 0:
        batch_cooldown = 0

    if not topic_name:
        topic_name = await _resolve_topic_name(
            userbot, source_chat_id, int(topic_id), log=log
        )

    copied = 0
    messages = await _collect_topic_audio_messages(
        userbot,
        source_chat_id,
        topic_id=int(topic_id),
        last_id=last_id,
        cancel_event=cancel_event,
    )
    if not messages:
        return 0

    from stream.core.source_filter import is_message_allowed

    # Process in chunks of batch_size (default 50)
    for i in range(0, len(messages), batch_size):
        if cancel_event and cancel_event.is_set():
            break
        chunk = messages[i : i + batch_size]
        valid_chunk = []
        for m in chunk:
            allowed, _ = await is_message_allowed(m)
            if allowed and _has_audio_media(m):
                valid_chunk.append(m)

        if not valid_chunk:
            last_id = int(chunk[-1].id)
            await state.update_document(
                state_id,
                {
                    "last_message_id": last_id,
                    "topic_id": int(topic_id),
                    "topic_name": topic_name,
                    "updated_at": asyncio.get_event_loop().time(),
                },
            )
            continue

        forward_success = False
        if mode == "FORWARD":
            try:
                count = await _forward_batch_with_backoff(
                    userbot,
                    valid_chunk,
                    source_chat_id=source_chat_id,
                    topic_id=int(topic_id),
                    topic_name=topic_name,
                    log=log,
                )
                copied += count
                if progress is not None:
                    progress["copied"] = int(progress.get("copied", 0)) + count
                forward_success = True
                if batch_cooldown:
                    await asyncio.sleep(batch_cooldown)
            except Exception as e:
                log.warning(
                    f"userbot forward batch failed for {source_chat_id} topic {topic_id} ({e}), falling back to copy"
                )

        if not forward_success:
            for msg in valid_chunk:
                if cancel_event and cancel_event.is_set():
                    break
                ok = await _index_or_dump_audio_message(
                    userbot,
                    source_chat_id,
                    msg,
                    log,
                    topic_id=int(topic_id),
                    topic_name=topic_name,
                    progress=progress,
                )
                if ok:
                    copied += 1
                if cooldown:
                    await asyncio.sleep(cooldown)

        last_id = int(chunk[-1].id)
        await state.update_document(
            state_id,
            {
                "last_message_id": last_id,
                "topic_id": int(topic_id),
                "topic_name": topic_name,
                "updated_at": asyncio.get_event_loop().time(),
            },
        )
    return copied


async def _list_forum_topics(
    userbot: Client, source_chat_id: int | str, log
) -> list[tuple[int, str]]:
    topics: list[tuple[int, str]] = []
    try:
        chat = await userbot.get_chat(source_chat_id)
        if getattr(chat, "type", None) == enums.ChatType.CHANNEL or not getattr(chat, "is_forum", False):
            return topics
    except Exception as e:
        log.debug(f"userbot could not get chat info for {source_chat_id}: {e}")

    try:
        async for topic in userbot.get_forum_topics(source_chat_id):
            if bool(getattr(topic, "is_deleted", False)):
                continue
            topic_id = _coerce_int(getattr(topic, "id", None))
            if topic_id is None or topic_id <= 0:
                continue
            topic_name = str(getattr(topic, "title", "") or "").strip()
            if not topic_name:
                topic_name = f"topic_{int(topic_id)}"
            await _store_topic_metadata(
                source_chat_id=source_chat_id,
                topic_id=int(topic_id),
                topic_name=topic_name,
            )
            topics.append((int(topic_id), topic_name))
    except Exception as e:
        log.debug(f"userbot forum topic listing skipped for {source_chat_id}: {e}")
    return topics


async def ingest_channel_history(
    userbot: Client,
    source_chat_id: int | str,
    log,
    cancel_event: asyncio.Event | None = None,
    progress: dict | None = None,
    topic_setting: int | str | None = None,
) -> int:
    if not await _ensure_peer(userbot, source_chat_id, log):
        return 0

    if topic_setting is None:
        topic_setting = _chat_topic_setting()
    copied = 0

    if topic_setting == "all" or _topic_allowed(0, topic_setting):
        copied += await _ingest_main_history(
            userbot,
            source_chat_id,
            log,
            cancel_event=cancel_event,
            progress=progress,
        )

    if cancel_event and cancel_event.is_set():
        return copied

    if topic_setting == "all":
        for topic_id, topic_name in await _list_forum_topics(
            userbot, source_chat_id, log
        ):
            if cancel_event and cancel_event.is_set():
                break
            copied += await _ingest_topic_history(
                userbot,
                source_chat_id,
                log,
                topic_id=topic_id,
                topic_name=topic_name,
                cancel_event=cancel_event,
                progress=progress,
            )
    else:
        try:
            configured_topic = int(topic_setting)
        except Exception:
            configured_topic = 0
        if configured_topic > 0:
            copied += await _ingest_topic_history(
                userbot,
                source_chat_id,
                log,
                topic_id=configured_topic,
                cancel_event=cancel_event,
                progress=progress,
            )

    return copied


async def _remember_live_message(
    source_chat_id: int | str, topic_id: int, msg_id: int
) -> None:
    state = db_handler.get_collection("userbot_state")
    state_id = f"history:{source_chat_id}:topic:{int(topic_id)}"
    try:
        existing = await state.read_document(state_id)
        if not existing:
            return
        last_id = int(existing.get("last_message_id") or 0)
    except Exception:
        return
    if int(msg_id) <= last_id:
        return
    await state.update_document(
        state_id,
        {
            "last_message_id": int(msg_id),
            "topic_id": int(topic_id),
            "updated_at": asyncio.get_event_loop().time(),
        },
    )


async def _live_audio_handler(client: Client, message) -> None:
    from stream.helpers.logger import LOGGER

    log = LOGGER(__name__)
    if not _has_audio_media(message):
        return

    source_ids = await _get_source_channels()
    if not _source_matches_message(message, source_ids):
        return

    topic_id = _message_topic_id(message)
    topic_setting = _chat_topic_setting()
    if not _topic_allowed(topic_id, topic_setting):
        return

    source_chat_id = getattr(getattr(message, "chat", None), "id", None)
    if source_chat_id is None:
        return
    if _coerce_int(source_chat_id) == _coerce_int(
        getattr(Config, "CHANNEL_ID", None)
    ):
        return

    try:
        ok = await _index_or_dump_audio_message(
            client,
            int(source_chat_id),
            message,
            log,
            topic_id=int(topic_id),
            progress=None,
        )
        if ok:
            await _remember_live_message(
                int(source_chat_id), int(topic_id), int(message.id)
            )
    except Exception as e:
        log.warning(
            f"userbot live dump failed for {source_chat_id}:{getattr(message, 'id', None)}: {e}"
        )


async def userbot_ingest_forever(userbot: Client, log):
    poll = int(getattr(Config, "USERBOT_POLL_INTERVAL_SEC", 300) or 300)
    if poll < 10:
        poll = 10

    while True:
        source_ids = await _get_source_channels()
        if not source_ids:
            await asyncio.sleep(poll)
            continue

        for cid in source_ids:
            try:
                copied = await ingest_channel_history(userbot, cid, log)
                if copied:
                    log.info(f"userbot ingested {copied} items from {cid}")
            except Exception as e:
                log.warning(f"userbot ingest failed for {cid}: {e}")

        await asyncio.sleep(poll)


async def start_userbot(log):
    session_string = (getattr(Config, "SESSION_STRING", "") or "").strip()
    if not session_string:
        return None

    userbot = Client(
        name="StreamUser",
        api_id=int(Config.API_ID),
        api_hash=str(Config.API_HASH),
        session_string=session_string,
        in_memory=False,
        workers=4,
    )
    await userbot.start()
    me = await userbot.get_me()
    log.info(f"Userbot started: {me.first_name} (@{me.username}) [ID: {me.id}]")
    return userbot


async def start_userbot_service(log):
    global _USERBOT_INSTANCE
    userbot = await start_userbot(log)
    if not userbot:
        return None, None
    _USERBOT_INSTANCE = userbot
    await _warm_up_dialogs(userbot, log)
    userbot.add_handler(
        MessageHandler(_live_audio_handler, filters.audio | filters.document),
        group=20,
    )

    # We no longer run ingest_forever automatically
    task = None
    return userbot, task


async def stop_userbot_service(userbot: Client | None, task: asyncio.Task | None):
    if task:
        task.cancel()
        try:
            await asyncio.wait_for(task, timeout=2.0)
        except (asyncio.CancelledError, asyncio.TimeoutError):
            pass
        except Exception:
            pass

    if userbot:
        try:
            await asyncio.wait_for(userbot.stop(), timeout=3.0)
        except Exception:
            pass


@bot.on_message(filters.command("index") & filters.user(Config.OWNER_ID))
async def index_command(client, message):
    from stream.helpers.logger import LOGGER

    log = LOGGER(__name__)

    try:
        from stream.core.config_manager import Config
        await Config.reload_config()
    except Exception:
        pass

    db_channel_id = getattr(Config, "CHANNEL_ID", None)
    idx_mode = str(getattr(Config, "USERBOT_INDEX", "DUMP") or "DUMP").upper()
    if idx_mode == "EXPORT" and _uses_forum_topic_mode(_chat_topic_setting()):
        idx_mode = "DUMP"

    if idx_mode != "EXPORT":
        userbot = _USERBOT_INSTANCE
        if not userbot:
            await message.reply("Userbot not started.")
            return

    if not db_channel_id and idx_mode not in ("INDEX", "EXPORT"):
        await message.reply("No CHANNEL_ID configured for DUMP mode.")
        return

    if idx_mode == "EXPORT":
        import json
        import os

        export_path = "export/result.json"
        if not os.path.exists(export_path):
            await message.reply(f"Export file not found: `{export_path}`")
            return

        status = await message.reply("Parsing export JSON file...")
        try:
            with open(export_path, "r", encoding="utf-8") as f:
                data = json.load(f)
        except Exception as e:
            await status.edit_text(f"Failed to read export JSON: {e}")
            return

        messages = data.get("messages", [])
        if not messages:
            await status.edit_text("No messages found in export JSON.")
            return

        chat_id = data.get("id")
        if chat_id is not None:
            # Telegram export chat ID starts with positive, convert to supergroup ID
            chat_id = int(f"-100{chat_id}")

        copied = 0
        import time

        from stream.database.MongoDb import db_handler
        from stream.plugins.db.audioIndex import _coerce_int, _split_artists

        await status.edit_text(f"Importing {len(messages)} messages from export...")

        for msg in messages:
            if msg.get("type") != "message" or msg.get("media_type") != "audio_file":
                continue

            msg_id = msg.get("id")
            if not msg_id:
                continue

            # Fallback chat ID if not in root
            c_id = (
                chat_id
                if chat_id
                else int(f"-100{str(msg.get('from_id', '')).replace('channel', '')}")
            )

            file_id = None  # we don't have file_id
            file_unique_id = f"{c_id}:{msg_id}"

            title = (msg.get("title") or msg.get("file") or "").strip()
            artist = (msg.get("performer") or "").strip()
            duration = _coerce_int(msg.get("duration_seconds"))
            mime = _normalize_mime_type(msg.get("mime_type"), msg.get("file"))

            if not title:
                title = str(msg_id)

            artists = _split_artists(artist) if artist else []

            now_ts = time.time()
            payload = {
                "telegram": {
                    "file_id": None,
                    "mime_type": mime,
                    "file_size": None,
                },
                "audio": {
                    "title": title,
                    "artist": artist,
                    "artists": artists if artists else None,
                    "duration_sec": duration,
                },
                "source_chat_id": c_id,
                "source_message_id": msg_id,
                "indexed": True,
                "enriched": False,
                "updated_at": now_ts,
            }

            await db_handler.audio_collection.update_one(
                {"_id": file_unique_id},
                {
                    "$set": payload,
                    "$setOnInsert": {"created_at": now_ts},
                    "$unset": {
                        "enriching": "",
                        "enrichment_started_at": "",
                        "enrichment_error": "",
                        "enrichment_error_at": "",
                        "enrich_retry_after": "",
                    },
                },
                upsert=True,
            )
            copied += 1

            if copied % 500 == 0:
                try:
                    await status.edit_text(
                        f"Imported {copied} audio tracks from export..."
                    )
                except Exception:
                    pass

        await status.edit_text(
            f"Finished exporting JSON.\n✓ Tracks Imported: {copied}"
        )
        return

    args = getattr(message, "command", [])[1:]
    override_source_ids = None
    override_topic = None

    if len(args) == 1:
        arg = str(args[0]).strip()
        if arg.lower() == "all" or (arg.lstrip("-").isdigit() and not arg.startswith("-100")):
            override_topic = "all" if arg.lower() == "all" else int(arg)
        elif arg.startswith("-100") or arg.lstrip("-").isdigit():
            override_source_ids = [int(arg)]
    elif len(args) >= 2:
        try:
            override_source_ids = [int(args[0])]
            arg1 = str(args[1]).strip()
            override_topic = "all" if arg1.lower() == "all" else int(arg1)
        except Exception:
            pass

    source_ids = override_source_ids or await _get_source_channels()
    topic_setting = override_topic if override_topic is not None else _chat_topic_setting()

    if not source_ids:
        await message.reply("No source channels found.")
        return

    status = await message.reply(
        f"Found {len(source_ids)} source channels.\n"
        f"Topic: `{topic_setting}`\n"
        f"Dump Channel: `{db_channel_id}`\n\n"
        f"Starting..."
    )

    cancel_event = asyncio.Event()
    _INDEX_TASKS[status.id] = cancel_event
    progress = {"copied": 0, "failed": 0, "last_error": None}

    async def _update_loop():
        while not cancel_event.is_set():
            await asyncio.sleep(15)
            if cancel_event.is_set():
                break
            try:
                err_line = f"\n⚠️ Error: {progress['last_error']}" if progress.get("last_error") else ""
                await status.edit_text(
                    f"Indexing in progress...\n"
                    f"Topic: `{topic_setting}`\n\n"
                    f"✓ Indexed/Sent: {progress['copied']}\n"
                    f"ㄨ Failed Channels: {progress['failed']}"
                    f"{err_line}",
                    reply_markup=InlineKeyboardMarkup(
                        [
                            [
                                InlineKeyboardButton(
                                    "Cancel", callback_data=f"cancel_index_{status.id}"
                                )
                            ]
                        ]
                    ),
                )
            except Exception:
                pass

    updater_task = asyncio.create_task(_update_loop())

    for cid in source_ids:
        if cancel_event.is_set():
            break
        try:
            await ingest_channel_history(
                userbot, cid, log, cancel_event, progress, topic_setting=topic_setting
            )
        except Exception as e:
            progress["failed"] += 1
            progress["last_error"] = str(e)[:120]
            log.warning(f"Failed indexing {cid}: {e}")

    cancel_event.set()
    await updater_task

    _INDEX_TASKS.pop(status.id, None)

    try:
        err_line = f"\n⚠️ Last Error: {progress['last_error']}" if progress.get("last_error") else ""
        await status.edit_text(
            f"Finished.\n"
            f"Topic: `{topic_setting}`\n\n"
            f"✓ Indexed/Sent: {progress['copied']}\n"
            f"ㄨ Failed Channels: {progress['failed']}"
            f"{err_line}"
        )
    except Exception:
        pass


@bot.on_callback_query(filters.regex(r"^cancel_index_(\d+)"))
async def cancel_index_callback(client, query):
    allowed_users = getattr(Config, "OWNER_ID", [])
    if not isinstance(allowed_users, list):
        allowed_users = [allowed_users]

    if query.from_user.id not in allowed_users:
        await query.answer("Not authorized.", show_alert=True)
        return

    msg_id = int(query.matches[0].group(1))
    cancel_event = _INDEX_TASKS.get(msg_id)
    if cancel_event:
        cancel_event.set()
        await query.answer("Cancellation requested...", show_alert=False)
    else:
        await query.answer("Task not found or already finished.", show_alert=True)
