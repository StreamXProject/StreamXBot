from pyrogram import filters
from pyrogram.errors.exceptions.bad_request_400 import MessageNotModified
from pyrogram.types import (
    CallbackQuery,
    InlineKeyboardButton,
    InlineKeyboardMarkup,
    Message,
)

from stream import bot

# Navigation Keyboards
_MAIN_KEYBOARD = InlineKeyboardMarkup(
    [
        [
            InlineKeyboardButton("General", callback_data="help_user"),
            InlineKeyboardButton("Sources & Filter", callback_data="help_sources"),
        ],
        [
            InlineKeyboardButton("Access & Invites", callback_data="help_access"),
            InlineKeyboardButton("Config", callback_data="help_config_1"),
        ],
        [
            InlineKeyboardButton("Admin & Dev", callback_data="help_admin"),
            InlineKeyboardButton("All Commands", callback_data="help_all"),
        ],
        [
            InlineKeyboardButton("Close", callback_data="help_close"),
        ],
    ]
)

_USER_KEYBOARD = InlineKeyboardMarkup(
    [
        [
            InlineKeyboardButton("« Back", callback_data="help_main"),
            InlineKeyboardButton("Sources", callback_data="help_sources"),
        ],
        [
            InlineKeyboardButton("Close", callback_data="help_close"),
        ],
    ]
)

_SOURCES_KEYBOARD = InlineKeyboardMarkup(
    [
        [
            InlineKeyboardButton("« Back", callback_data="help_main"),
            InlineKeyboardButton("Config", callback_data="help_config_1"),
        ],
        [
            InlineKeyboardButton("Close", callback_data="help_close"),
        ],
    ]
)

_CONFIG_KEYBOARD_1 = InlineKeyboardMarkup(
    [
        [
            InlineKeyboardButton("1 / 3", callback_data="help_noop"),
            InlineKeyboardButton("Next »", callback_data="help_config_2"),
        ],
        [
            InlineKeyboardButton("« Back", callback_data="help_main"),
            InlineKeyboardButton("Close", callback_data="help_close"),
        ],
    ]
)

_CONFIG_KEYBOARD_2 = InlineKeyboardMarkup(
    [
        [
            InlineKeyboardButton("« Prev", callback_data="help_config_1"),
            InlineKeyboardButton("2 / 3", callback_data="help_noop"),
            InlineKeyboardButton("Next »", callback_data="help_config_3"),
        ],
        [
            InlineKeyboardButton("« Back", callback_data="help_main"),
            InlineKeyboardButton("Close", callback_data="help_close"),
        ],
    ]
)

_CONFIG_KEYBOARD_3 = InlineKeyboardMarkup(
    [
        [
            InlineKeyboardButton("« Prev", callback_data="help_config_2"),
            InlineKeyboardButton("3 / 3", callback_data="help_noop"),
        ],
        [
            InlineKeyboardButton("« Back", callback_data="help_main"),
            InlineKeyboardButton("Close", callback_data="help_close"),
        ],
    ]
)

_ACCESS_KEYBOARD = InlineKeyboardMarkup(
    [
        [
            InlineKeyboardButton("« Back", callback_data="help_main"),
            InlineKeyboardButton("Admin & Dev", callback_data="help_admin"),
        ],
        [
            InlineKeyboardButton("Close", callback_data="help_close"),
        ],
    ]
)

_ADMIN_KEYBOARD = InlineKeyboardMarkup(
    [
        [
            InlineKeyboardButton("« Back", callback_data="help_main"),
            InlineKeyboardButton("Access & Invites", callback_data="help_access"),
        ],
        [
            InlineKeyboardButton("Close", callback_data="help_close"),
        ],
    ]
)

_ALL_KEYBOARD = InlineKeyboardMarkup(
    [
        [
            InlineKeyboardButton("« Back", callback_data="help_main"),
        ],
        [
            InlineKeyboardButton("Close", callback_data="help_close"),
        ],
    ]
)


# Text Contents
_MAIN_TEXT = (
    "**StreamX Bot Help & Commands**\n\n"
    "Select a category below to explore commands and configuration options:\n\n"
    "• **General**: Basic user utilities, ID lookups and media inspection.\n"
    "• **Sources & Filter**: Contributor allowlists, bans and hybrid mode.\n"
    "• **Access & Invites**: Who can sign up, invite codes, required channels & user locks.\n"
    "• **Config**: Detailed configuration variables with paging.\n"
    "• **Admin & Dev**: Dashboard, log files, updater and maintenance.\n"
    "• **All Commands**: Complete command reference cheat sheet."
)

_USER_TEXT = (
    "**General Commands**\n\n"
    "• `/ping` or `/alive`\n"
    "  Check bot latency, response time, and system uptime.\n\n"
    "• `/id`\n"
    "  Get your user ID, current chat ID, or reply to a message to inspect sender and forwarded entity IDs.\n\n"
    "• `/search <query>`\n"
    "  Search for music tracks in the database library.\n\n"
    "• `/mediainfo` or `/mi`\n"
    "  Inspect technical metadata, audio codec, bitrate, and tags (send with media or in reply).\n\n"
    "• `/help`\n"
    "  Display this interactive help menu."
)

_SOURCES_TEXT = (
    "**Sources & Filter Commands**\n\n"
    "Manage trusted contributor sources, ban lists, and hybrid filter modes:\n\n"
    "• `/filter_mode [0|1|2|hybrid]`\n"
    "  View or switch the audio ingestion mode:\n"
    "  - `0` (`group_only`): Primary channel only\n"
    "  - `1` (`anyone`): Accept from any source\n"
    "  - `2` (`hybrid`): Accept from allowed sources collection only\n\n"
    "• `/sources`\n"
    "  Overview of current filter mode, primary channel, allowed sources, and banned sources.\n\n"
    "• `/allow <peer> [custom name]`\n"
    "  Add a channel, group, or user to allowed contributors (or reply to a message). Automatically unbans if previously banned.\n\n"
    "• `/disallow <peer>`\n"
    "  Remove a source from allowed contributors (or reply to a message).\n\n"
    "• `/ban <peer> [reason]`\n"
    "  Strictly block a channel, group, or user from adding tracks and using the service (or reply to a message). Automatically removes from allowlist.\n\n"
    "• `/unban <peer>`\n"
    "  Unban a source or user (or reply to a message)."
)

_ACCESS_TEXT = (
    "**Access & Invites**\n\n"
    "_(For Bot Owner & Admins)_\n\n"
    "**Sign-ups & Invites:**\n"
    "• `/access`\n"
    "  See your signup rules, required channels, and user counts.\n\n"
    "• `/registration [open|invite|allowlist|closed]`\n"
    "  Choose who gets to sign up:\n"
    "  - `open`: Anyone can sign in with Telegram.\n"
    "  - `invite`: Need an invite code to join.\n"
    "  - `allowlist`: Only people you approved can join.\n"
    "  - `closed`: Nobody new can sign up.\n\n"
    "• `/invite [uses] [days] [note]`\n"
    "  Make an invite code (default: 1 use, lasts 7 days).\n\n"
    "• `/invites`\n"
    "  See all your invite codes and who used them.\n\n"
    "• `/allow <user_id> [note]`\n"
    "  Let a user sign up without an invite code.\n\n"
    "• `/disallow <user_id>`\n"
    "  Remove someone from the approved sign-up list.\n\n"
    "**Channel Requirements:**\n"
    "• `/membership [on|off]`\n"
    "  Turn the required channel check on or off.\n\n"
    "• `/requirechat [chat_id] [public|private]`\n"
    "  Make people join a channel or group before using the app (add `private` to hide the link on the site).\n\n"
    "• `/unrequirechat [chat_id]`\n"
    "  Stop requiring this channel.\n\n"
    "• `/reverify`\n"
    "  Check if everyone is still in the channel right now.\n\n"
    "• `/bypass [user_id|on|off] [note]`\n"
    "  Let someone in without joining the channel (or `/bypass on` to skip channel checks for everyone, or reply to someone).\n\n"
    "• `/unbypass <user_id>`\n"
    "  Make someone follow channel rules again (or reply to them).\n\n"
    "• `/bypasses`\n"
    "  See everyone who gets to skip channel checks.\n\n"
    "**Managing Users:**\n"
    "• `/lock <user_id> [reason]`\n"
    "  Block someone from using the app and log them out everywhere.\n\n"
    "• `/unlock <user_id>`\n"
    "  Unblock someone and let them back in.\n\n"
    "• `/revoke <user_id>`\n"
    "  Log someone out of all their devices (keeps their account)."
)

_CONFIG_TEXT_1 = (
    "**Configuration Variables**\n\n"
    "**Core Credentials:**\n"
    "• `BOT_TOKEN` (str): Telegram bot token from @BotFather.\n"
    "• `API_ID` (int): Telegram API ID from my.telegram.org.\n"
    "• `API_HASH` (str): Telegram API Hash from my.telegram.org.\n"
    "• `OWNER_ID` (int): Telegram user ID of primary bot owner.\n"
    "• `SUDO_USERS` (list[int]): User IDs granted sudo/admin privileges.\n"
    "• `ONLY_API` (bool): Run only the FastAPI server without starting Telegram bot client.\n\n"
    "**Database & Security:**\n"
    "• `MONGO_URI` (str): MongoDB connection string URI.\n"
    "• `DATABASE_NAME` (str): Database name (default: `StreamX`).\n"
    "• `SECRET_KEY` (str): HMAC secret used for signing web auth tokens.\n"
    "• `FIREBASE_CREDENTIALS` (str): Encrypted Firebase service credentials.\n\n"
    "**How to use & update:**\n"
    "Variables can be set in `config.py`, passed as environment variables, or updated live in database via the `/sudo` control panel."
)

_CONFIG_TEXT_2 = (
    "**Configuration Variables**\n\n"
    "**Channels & Filtering:**\n"
    "• `CHANNEL_ID` (int): Primary channel ID for track indexing and streaming.\n"
    "• `DUMP_CHANNEL_ID` (int): Channel ID for dumping/mirroring tracks.\n"
    "• `FILTER_MODE` (int/str):\n"
    "  - `0` / `group_only`: Ingest tracks only from `CHANNEL_ID`.\n"
    "  - `1` / `anyone`: Ingest tracks from any chat, group, or user.\n"
    "  - `2` / `hybrid`: Ingest tracks only from allowed contributors.\n"
    "• `COLLABORATOR_ID` (list[int]): Channel/user IDs seeded into allowed contributors on startup.\n\n"
    "**Userbot History Ingestion:**\n"
    "• `SESSION_STRING` (str): Pyrogram session string for Userbot account.\n"
    "• `SOURCE_CHANNEL_IDS` (list[int]): Channel IDs to monitor/index.\n"
    "• `USERBOT_INDEX` (str): `INDEX` (metadata only) or `DUMP` (copy file to dump channel).\n"
    "• `USERBOT_BATCH_SIZE` (int): Messages per batch during history indexing (default: `50`).\n"
    "• `USERBOT_COOLDOWN_SEC` (int): Delay between message copies (default: `2`s)."
)

_CONFIG_TEXT_3 = (
    "**Configuration Variables**\n\n"
    "**Lyrics & Artwork:**\n"
    "• `MUSIXMATCH` (bool): Enable Musixmatch synced/plain lyrics provider.\n"
    "• `LRCLIB` (bool): Enable LrcLib open-source lyrics provider.\n"
    "• `SPOTIFY_CLIENT_ID` (str): Spotify API Client ID for album artwork & metadata.\n"
    "• `SPOTIFY_CLIENT_SECRET` (str): Spotify API Client Secret.\n"
    "• `COLLEGE` (bool): Generate a custom cover collage if track has no artwork.\n"
    "• `TEXT_COLOR` (str): Hex color code for thumbnail text overlay (e.g. `#FFFFFF`).\n\n"
    "**Multi-Clients & Web API:**\n"
    "• `MULTI_CLIENTS` (bool): Enable multiple worker clients for parallel downloads.\n"
    "• `MULTI_CLIENTS_1..4` (str): Secondary bot tokens to distribute Telegram bandwidth.\n"
    "• `CORS_ORIGIN` / `CORS_ORIGINS` (str): Allowed web origins for API (or `*`).\n"
    "• `COOKIE_SECURE` (bool): Require HTTPS for session cookies.\n"
    "• `COOKIE_SAMESITE` (str): Cookie SameSite policy (`none`, `lax`, `strict`).\n"
    "• `DEBUG` (bool): Enable verbose debug logging in console and log files."
)

_ADMIN_TEXT = (
    "**Admin & Developer Commands**\n\n"
    "_(Accessible to Bot Owner & Sudo users)_\n\n"
    "• `/sudo`\n"
    "  Interactive dashboard for Config variables, Cookies, System stats & Database info.\n\n"
    "• `/logs` or `/log`\n"
    "  Download current application logs.\n\n"
    "• `/update`\n"
    "  Check for git updates and pull latest changes.\n\n"
    "• `/restart`\n"
    "  Restart the bot process and background workers.\n\n"
    "• `/index`\n"
    "  Trigger Userbot channel/topic history indexing.\n\n"
    "• `/reverify_tracks` [force|status|cancel]\n"
    "  Scan library to re-verify metadata & add missing Romanized titles/lyrics.\n\n"
    "• `/fileid`\n"
    "  Retrieve Pyrogram file_id for replied media.\n\n"
    "• `/bs`\n"
    "  Update bot settings panel banner image."
)

_ALL_TEXT = (
    "**Complete Command Reference**\n\n"
    "**User Utilities:**\n"
    "• `/ping`, `/alive` - Latency & uptime\n"
    "• `/id` - Chat/User/Message ID lookup\n"
    "• `/search <query>` - Search library tracks\n"
    "• `/mediainfo`, `/mi` - Media metadata analyzer\n"
    "• `/help` - Interactive help menu\n\n"
    "**Access & Invites:**\n"
    "• `/access` - See access settings & stats\n"
    "• `/registration [mode]` - Who can sign up (open/invite/allowlist/closed)\n"
    "• `/invite [uses] [days]` - Make an invite code\n"
    "• `/invites` - See invite codes & who used them\n"
    "• `/allow <user_id>` - Let someone join without code\n"
    "• `/disallow <user_id>` - Remove someone from approved list\n"
    "• `/membership [on|off]` - Turn channel requirement on or off\n"
    "• `/requirechat [chat_id] [pub|priv]` - Add required channel\n"
    "• `/unrequirechat [chat_id]` - Remove required channel\n"
    "• `/reverify` - Check if everyone is still in channel\n"
    "• `/bypass [user_id|on|off]` - Skip channel check for a user or everyone\n"
    "• `/unbypass <user_id>` - Remove skip for a user\n"
    "• `/bypasses` - See who gets to skip channel checks\n"
    "• `/lock <user_id> [reason]` - Block user & log them out\n"
    "• `/unlock <user_id>` - Unblock user\n"
    "• `/revoke <user_id>` - Log user out on all devices\n\n"
    "**Sources & Hybrid Filter:**\n"
    "• `/sources` - Summary & mode status\n"
    "• `/filter_mode [0|1|2]` - Switch ingestion mode\n"
    "• `/allow <peer>` or `/allow_source` - Add allowed contributor\n"
    "• `/disallow <peer>` or `/disallow_source` - Remove allowed contributor\n"
    "• `/ban <peer> [reason]` - Ban channel/group/user\n"
    "• `/unban <peer>` - Unban source/user\n\n"
    "**Admin & Maintenance:**\n"
    "• `/sudo` - Control panel (Config, Stats, DB, Cookies)\n"
    "• `/logs`, `/log` - Download application logs\n"
    "• `/update` - Pull git updates\n"
    "• `/restart` - Restart bot process\n"
    "• `/index` - Userbot history indexing\n"
    "• `/reverify_tracks` - Re-verify tracks & missing Romanized titles\n"
    "• `/fileid` - Media file_id extractor\n"
    "• `/bs` - Set panel banner image"
)


@bot.on_message(filters.command(["help", "start"]))
async def help_command_handler(client, message: Message):
    """Handle /help and /start with interactive inline buttons, plus WebX bot-session auth."""
    if len(message.command) > 1 and (message.command[1].startswith("auth_") or message.command[1].startswith("login_")):
        session_id = message.command[1].strip()
        from stream.database.MongoDb import db_handler
        import time

        col = db_handler.get_collection("bot_auth_sessions").collection
        session = await col.find_one({"_id": session_id})
        if not session or session.get("status") != "pending" or session.get("expires_at", 0) < time.time():
            await message.reply_text(
                "⚠️ **This sign-in request has expired or is invalid.**\n\nPlease return to your browser and click **Open Telegram App** again.",
            )
            return

        from Api.routers.auth import _authenticate_or_register_tg_user
        tg_user = message.from_user
        name = tg_user.first_name or ""
        if tg_user.last_name:
            name = f"{name} {tg_user.last_name}".strip()
        username = tg_user.username or None
        invite_code = session.get("invite_code")

        try:
            user_info, token = await _authenticate_or_register_tg_user(
                tg_user_id=tg_user.id,
                name=name or username or "Telegram User",
                username=username,
                photo_url=None,
                invite_code=invite_code,
            )
            await col.update_one(
                {"_id": session_id},
                {
                    "$set": {
                        "status": "confirmed",
                        "token": token,
                        "user_id": user_info["user_id"],
                        "first_name": user_info["first_name"],
                        "username": user_info["username"],
                        "photo_url": user_info.get("photo_url"),
                        "profile_url": user_info.get("profile_url"),
                        "confirmed_at": time.time(),
                    }
                },
            )
            await message.reply_text(
                f"**Authorized Successfully WebX!**\n\nWelcome, **{user_info.get('first_name') or 'User'}**! Your browser session is ready.\nYou can now return to your browser and enjoy your music!",
            )
            return
        except Exception as e:
            await message.reply_text(
                f"**Authorization failed:** {str(e)}",
            )
            return

    await message.reply_text(
        _MAIN_TEXT,
        reply_markup=_MAIN_KEYBOARD,
    )


@bot.on_callback_query(filters.regex(r"^help_"))
async def help_callback_handler(_, query: CallbackQuery):
    """Handle inline button navigation in help menu."""
    data = query.data

    if data == "help_noop":
        await query.answer()
        return

    await query.answer()

    if data == "help_close":
        try:
            await query.message.delete()
        except Exception:
            pass
        return

    text_map = {
        "help_main": (_MAIN_TEXT, _MAIN_KEYBOARD),
        "help_user": (_USER_TEXT, _USER_KEYBOARD),
        "help_sources": (_SOURCES_TEXT, _SOURCES_KEYBOARD),
        "help_access": (_ACCESS_TEXT, _ACCESS_KEYBOARD),
        "help_config_1": (_CONFIG_TEXT_1, _CONFIG_KEYBOARD_1),
        "help_config_2": (_CONFIG_TEXT_2, _CONFIG_KEYBOARD_2),
        "help_config_3": (_CONFIG_TEXT_3, _CONFIG_KEYBOARD_3),
        "help_admin": (_ADMIN_TEXT, _ADMIN_KEYBOARD),
        "help_all": (_ALL_TEXT, _ALL_KEYBOARD),
    }

    entry = text_map.get(data)
    if not entry:
        return

    text, markup = entry
    try:
        await query.edit_message_text(
            text,
            reply_markup=markup,
        )
    except MessageNotModified:
        pass
    except Exception:
        pass
