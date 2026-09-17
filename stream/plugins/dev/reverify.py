import asyncio
import time

from pyrogram import filters
from pyrogram.errors.exceptions.bad_request_400 import MessageNotModified
from pyrogram.types import Message

from Api.services.reverify_service import (
    cancel_reverify_job,
    get_reverify_status,
    reverify_tracks,
)
from stream import bot
from stream.helpers.filters import sudo_cmd


@bot.on_message(filters.command(["reverify_tracks", "reverifytracks", "reindex_tracks"]) & sudo_cmd)
async def cmd_reverify_tracks(_, message: Message):
    args = message.command[1:] if len(message.command) > 1 else []
    subcmd = args[0].lower() if args else ""

    if subcmd in ("status", "progress"):
        status = get_reverify_status()
        if not status.get("running"):
            await message.reply_text("ℹ️ No track re-verification job is currently running.")
            return
        total = status.get("total", 0)
        processed = status.get("processed", 0)
        pct = f"{round((processed / total) * 100, 1)}%" if total else "0%"
        await message.reply_text(
            f"🔄 **Track Re-verification in Progress**\n\n"
            f"• **Status:** `{status.get('status')}`\n"
            f"• **Progress:** `{processed}/{total}` ({pct})\n"
            f"• **Titles Added:** `{status.get('titles_added', 0)}`\n"
            f"• **Lyrics Added:** `{status.get('lyrics_added', 0)}`\n"
            f"• **Elapsed:** `{status.get('elapsed_sec', 0)}s`"
        )
        return

    if subcmd in ("cancel", "stop"):
        cancelled = cancel_reverify_job()
        if cancelled:
            await message.reply_text("🛑 Re-verification cancellation requested.")
        else:
            await message.reply_text("ℹ️ No running job to cancel.")
        return

    force_all = subcmd in ("force", "all")
    missing_lyrics = subcmd in ("lyrics",)

    curr = get_reverify_status()
    if curr.get("running"):
        await message.reply_text(
            "⚠️ A re-verification job is already running! Use `/reverify_tracks status` or `/reverify_tracks cancel`."
        )
        return

    mode_label = "All tracks (forced)" if force_all else "Tracks missing titles / left out"
    msg = await message.reply_text(
        f"🔄 **Starting Track Re-verification...**\n"
        f"• **Mode:** `{mode_label}`\n"
        f"_Scanning library..._"
    )

    last_edit = 0.0

    async def _progress(info: dict):
        nonlocal last_edit
        now = time.time()
        if now - last_edit < 2.5:
            return
        last_edit = now

        tot = info.get("total", 0)
        cur = info.get("processed", 0)
        pct = f"{round((cur / tot) * 100, 1)}%" if tot else "0%"
        try:
            await msg.edit_text(
                f"🔄 **Re-verifying Tracks...**\n\n"
                f"• **Progress:** `{cur}/{tot}` ({pct})\n"
                f"• **Titles Added:** `{info.get('titles_added', 0)}`\n"
                f"• **Lyrics Added:** `{info.get('lyrics_added', 0)}`\n"
                f"• **Errors:** `{info.get('errors', 0)}`\n"
                f"• **Elapsed:** `{info.get('elapsed_sec', 0)}s`"
            )
        except MessageNotModified:
            pass
        except Exception:
            pass

    try:
        res = await reverify_tracks(
            force_all=force_all,
            missing_titles=True,
            missing_lyrics=missing_lyrics,
            progress_callback=_progress,
        )

        status_text = "Completed" if res.get("status") == "completed" else res.get("status", "Done")
        await msg.edit_text(
            f"✅ **Track Re-verification {status_text}!**\n\n"
            f"• **Total Scanned:** `{res.get('total', 0)}`\n"
            f"• **Processed:** `{res.get('processed', 0)}`\n"
            f"• **Titles Added:** `{res.get('titles_added', 0)}`\n"
            f"• **Lyrics Added:** `{res.get('lyrics_added', 0)}`\n"
            f"• **Errors:** `{res.get('errors', 0)}`\n"
            f"• **Time Taken:** `{res.get('elapsed_sec', 0)}s`"
        )
    except Exception as e:
        await msg.edit_text(f"❌ Track re-verification failed: `{e}`")
