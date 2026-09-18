import os
import sys
import datetime
import asyncio
import uuid
import glob

from pyrogram import filters
from pyrogram.types import Message, InlineKeyboardMarkup, InlineKeyboardButton

from stream import bot
from stream.core.config_manager import Config
from stream.helpers.logger import LOGGER

PROJECT_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "../../.."))
RESTART_PREFIX = os.path.join(PROJECT_ROOT, ".restartmsg.")

async def _run_cmd(*args):
    try:
        proc = await asyncio.create_subprocess_exec(
            *args,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        out, err = await proc.communicate()
        return proc.returncode, (out or b"").decode(errors="ignore"), (err or b"").decode(errors="ignore")
    except Exception as e:
        return -1, "", str(e)

def _origin_path_from_remote_url(url: str) -> str:
    raw = (url or "").strip()
    if not raw:
        return ""

    if raw.startswith("git@github.com:"):
        return raw[len("git@github.com:") :].strip()

    if raw.startswith("https://github.com/") or raw.startswith("http://github.com/"):
        return raw.split("github.com/", 1)[1].strip()

    if raw.startswith("https://") or raw.startswith("http://"):
        try:
            after_scheme = raw.split("://", 1)[1]
        except Exception:
            return ""
        if "github.com/" not in after_scheme:
            return ""
        host_and_rest = after_scheme.split("github.com/", 1)[1]
        if "@" in after_scheme.split("/", 1)[0]:
            return host_and_rest.strip()
        return host_and_rest.strip()

    return ""

async def _ensure_pat_remote() -> None:
    token = (os.getenv("GITHUB_TOKEN") or "").strip()
    if not token:
        return
    try:
        _, origin_url, _ = await _run_cmd("git", "remote", "get-url", "origin")
        origin_path = _origin_path_from_remote_url(origin_url)
        if not origin_path:
            return
        await _run_cmd("git", "remote", "set-url", "origin", f"https://{token}@github.com/{origin_path}")
    except Exception:
        return

async def _has_config_access(user_id: int) -> bool:
    try:
        uid = int(user_id)
    except Exception:
        return False
    owners = getattr(Config, "OWNER_ID", None) or []
    sudos = getattr(Config, "SUDO_USERS", None) or []
    return uid in owners or uid in sudos

async def graceful_restart():
    try:
        LOGGER(__name__).info("Stopping client...")
        if bot is not None:
            await bot.stop()
            LOGGER(__name__).info("Client stopped.")
    except Exception as e:
        LOGGER(__name__).error(f"Error during graceful stop: {e}")
    finally:
        os.chdir(PROJECT_ROOT)
        os.execl(sys.executable, sys.executable, "-m", "stream")

def now_ist():
    return (datetime.datetime.utcnow() + datetime.timedelta(hours=5, minutes=30)).strftime("%Y-%m-%d %H:%M:%S")

def get_relative_time(date_str):
    try:
        formats = [
            "%a %b %d %H:%M:%S %Y",
            "%a, %d %b %Y %H:%M:%S %z",
            "%Y-%m-%d %H:%M:%S"
        ]
        commit_time = None
        for fmt in formats:
            try:
                commit_time = datetime.datetime.strptime(date_str, fmt)
                break
            except ValueError:
                continue
        if not commit_time:
            return date_str
        now = datetime.datetime.now()
        diff = now - commit_time
        seconds = diff.total_seconds()
        if seconds < 60:
            return f"{int(seconds)} seconds ago"
        if seconds < 3600:
            minutes = int(seconds / 60)
            return f"{minutes} minute{'s' if minutes != 1 else ''} ago"
        if seconds < 86400:
            hours = int(seconds / 3600)
            return f"{hours} hour{'s' if hours != 1 else ''} ago"
        if seconds < 604800:
            days = int(seconds / 86400)
            return f"{days} day{'s' if days != 1 else ''} ago"
        if seconds < 2592000:
            weeks = int(seconds / 604800)
            return f"{weeks} week{'s' if weeks != 1 else ''} ago"
        months = int(seconds / 2592000)
        if months < 12:
            return f"{months} month{'s' if months != 1 else ''} ago"
        years = int(seconds / 31536000)
        return f"{years} year{'s' if years != 1 else ''} ago"
    except Exception as e:
        LOGGER(__name__).error(f"Error calculating relative time: {e}")
        return date_str

async def check_for_updates():
    try:
        await _ensure_pat_remote()
        rc, _, err = await _run_cmd("git", "remote", "update")
        if rc != 0:
            LOGGER(__name__).error(f"git remote update failed: {err}")
            return None
        rc, status_out, _ = await _run_cmd("git", "status", "-uno")
        if rc != 0:
            LOGGER(__name__).error("git status failed")
            return None
        if "Your branch is behind" in status_out:
            rc, count_out, _ = await _run_cmd("git", "rev-list", "--count", "HEAD..origin/main")
            rc2, latest_msg, _ = await _run_cmd("git", "log", "-1", "--pretty=format:%s", "origin/main")
            return {
                "updates_available": True,
                "commits_behind": (count_out or "0").strip(),
                "latest_commit_msg": (latest_msg or "").strip(),
            }
        return {"updates_available": False}
    except Exception as e:
        LOGGER(__name__).error(f"Failed to check for updates: {e}")
        return None

async def pull_updates(msg=None):
    try:
        await _ensure_pat_remote()
        rc, out, err = await _run_cmd("git", "pull", "--ff-only", "origin", "main")
        if rc != 0:
            error = (err or out or "").strip()
            if msg:
                await msg.edit(f"Failed to pull updates: `{error}`")
            LOGGER(__name__).error(f"Git pull failed: {error}")
            return None
        _, commit_hash, _ = await _run_cmd("git", "rev-parse", "HEAD")
        _, commit_date, _ = await _run_cmd("git", "log", "-1", "--format=%cd", "--date=local")
        _, commit_timestamp, _ = await _run_cmd("git", "log", "-1", "--format=%at")
        _, commit_msg, _ = await _run_cmd("git", "log", "-1", "--format=%s")
        LOGGER(__name__).info(f"Bot updated. Commit: {commit_hash.strip()}, Date: {commit_date.strip()}, Message: {commit_msg.strip()}")
        if msg:
            await msg.edit(f"Pulled updates.\nPull: {commit_msg.strip()}\nCommit: `{commit_hash.strip()[:7]}`\nRestarting...")
        return {
            "commit_hash": commit_hash.strip(),
            "commit_date": commit_date.strip(),
            "commit_timestamp": commit_timestamp.strip(),
            "commit_msg": commit_msg.strip(),
        }
    except Exception as e:
        if msg:
            await msg.edit(f"An error occurred: `{e}`")
        LOGGER(__name__).exception("Exception during update")
        return None

async def update(_, message: Message):
    msg = await message.reply_text("Checking for updates...")
    info = await check_for_updates()
    if not info or not info.get("updates_available"):
        await msg.edit("No updates available. Already on the latest version.")
        return
    await msg.edit(f"Updates available! ({info['commits_behind']} new commits)\nLatest: {info['latest_commit_msg']}\nPulling changes...")
    commit_info = await pull_updates(msg)
    if not commit_info:
        return
    restart_id = str(uuid.uuid4())
    fpath = f"{RESTART_PREFIX}{restart_id}"
    try:
        with open(fpath, "w", encoding="utf-8") as f:
            f.write(f"{msg.chat.id} {msg.id} {commit_info['commit_hash']} | {commit_info['commit_date']} | {commit_info['commit_timestamp']} | {commit_info['commit_msg']} | {info['commits_behind']} | {info['latest_commit_msg']}")
    except Exception as e:
        LOGGER(__name__).error(f"Failed writing restart file: {e}")
    
    asyncio.create_task(graceful_restart())

async def restart_notification():
    if bot is None:
        return
    try:
        files = sorted(glob.glob(f"{RESTART_PREFIX}*"), key=lambda p: os.path.getmtime(p))
        if not files:
            return
        
        for fpath in files:
            try:
                content = open(fpath, encoding="utf-8").read().strip()
            except Exception as e:
                LOGGER(__name__).error(f"restart_notification read failed: {e}")
                continue
            parts = content.split(" | ")
            head = parts[0].split()
            chat_id = int(head[0]) if len(head) > 0 else None
            msg_id = int(head[1]) if len(head) > 1 else None
            commit_hash = head[2] if len(head) > 2 else ""
            commit_date = parts[1] if len(parts) > 1 else ""
            commit_timestamp = parts[2] if len(parts) > 2 else ""
            commit_msg = parts[3] if len(parts) > 3 else ""
            commits_behind = parts[4] if len(parts) > 4 else "0"
            latest_msg = parts[5] if len(parts) > 5 else ""

            if not chat_id or not msg_id:
                continue

            restart_time = now_ist()
            text = f"Restarted successfully at {restart_time} IST\n"
            if int(commits_behind) > 0:
                text += f"Updates: {commits_behind} new commits\n"
                text += f"Latest: {latest_msg}\n"
            if commit_hash:
                text += f"Commit: `{commit_hash}`\n"
            if commit_date:
                rel = get_relative_time(commit_date)
                text += f"Committed: {rel}"

            try:
                await bot.edit_message_text(chat_id=chat_id, message_id=msg_id, text=text)
            except Exception as e:
                LOGGER(__name__).error(f"restart_notification edit failed: {e}")
            finally:
                try:
                    os.remove(fpath)
                except OSError:
                    pass
    except Exception as e:
        LOGGER(__name__).error(f"restart_notification failed: {e}")

async def restart_bot(_, message: Message):
    user_id = message.from_user.id if message.from_user else None
    if not user_id or not await _has_config_access(user_id):
        await message.reply_text("Access denied.")
        return
    buttons = [
        [InlineKeyboardButton("Restart Only", callback_data="confirm_restart restart"), InlineKeyboardButton("Update & Restart", callback_data="confirm_restart update")],
        [InlineKeyboardButton("Cancel", callback_data="confirm_restart cancel")]
    ]
    await message.reply("How would you like to proceed?", reply_markup=InlineKeyboardMarkup(buttons))

async def handle_restart_confirmation(_, callback_query):
    user_id = callback_query.from_user.id if callback_query.from_user else None
    if not user_id or not await _has_config_access(user_id):
        await callback_query.answer("Access denied.", show_alert=True)
        return
    choice = callback_query.data.split()[1]
    msg = callback_query.message

    if choice == "cancel":
        await callback_query.answer("Cancelled", show_alert=False)
        try:
            await msg.delete()
        except Exception:
            pass
        return

    if choice == "update":
        await callback_query.answer("Checking for updates...", show_alert=False)
        await msg.edit("Checking for updates...")

        update_info = await check_for_updates()

        if not update_info or not update_info.get("updates_available"):
            await msg.edit("No updates available. Restarting without updating...")
        else:
            await msg.edit(f"Updates available!\n({update_info['commits_behind']} new commits)\nLatest: {update_info['latest_commit_msg']}\nPulling changes...")
            commit_info = await pull_updates(msg)
            if not commit_info:
                return
            restart_id = str(uuid.uuid4())
            fpath = f"{RESTART_PREFIX}{restart_id}"
            try:
                with open(fpath, "w", encoding="utf-8") as f:
                    f.write(f"{msg.chat.id} {msg.id} {commit_info['commit_hash']} | {commit_info['commit_date']} | {commit_info['commit_timestamp']} | {commit_info['commit_msg']} | {update_info['commits_behind']} | {update_info['latest_commit_msg']}")
            except Exception as e:
                LOGGER(__name__).error(f"Failed writing restart file: {e}")
            
            asyncio.create_task(graceful_restart())
            return

    await callback_query.answer("Restarting...", show_alert=False)
    await msg.edit("Restarting...")

    restart_id = str(uuid.uuid4())
    fpath = f"{RESTART_PREFIX}{restart_id}"
    try:
        with open(fpath, "w", encoding="utf-8") as f:
            f.write(f"{msg.chat.id} {msg.id}")
    except Exception as e:
        LOGGER(__name__).error(f"Failed writing restart file: {e}")
    
    asyncio.create_task(graceful_restart())

if bot is not None:
    bot.on_message(filters.command("update"))(update)
    bot.on_message(filters.command("restart"))(restart_bot)
    bot.on_callback_query(filters.regex("^confirm_restart"))(handle_restart_confirmation)
