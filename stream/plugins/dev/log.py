import os
import aiofiles
from pyrogram import filters, enums
from pyrogram.types import Message, CallbackQuery, InlineKeyboardMarkup, InlineKeyboardButton

from stream import bot
from stream.helpers.paste import katbin_paste

@bot.on_message(filters.command(["log", "logs"]))
async def log(_, message: Message):
    
    log_path = "log.txt"
    
    if not os.path.exists(log_path):
        return await message.reply_text("Log file not found!")

    processing_msg = await message.reply_text("Processing logs...")

    try:
        buttons = InlineKeyboardMarkup([
            [InlineKeyboardButton("Show Log", callback_data="show_log"),
             InlineKeyboardButton("Web", callback_data="web_paste")]
        ])

        await message.reply_document(
            log_path,
            caption="Log File",
            reply_markup=buttons,
        )
        
        await processing_msg.delete()

    except Exception as e:
        await processing_msg.edit_text(f"Error: {str(e)}")


@bot.on_callback_query(filters.regex("^show_log$"))
async def show_log_callback(_, query: CallbackQuery):
    """Edit existing message with truncated log (prioritize recent content)"""
    await query.answer()
    try:
        log_path = "log.txt"
        async with aiofiles.open(log_path, "r") as f:
            content = await f.read()
            lines = content.splitlines()[-100:]
            truncated = "\n".join(lines)
            
            max_length = 990
            
            if len(truncated) > max_length:
                truncated = truncated[-(max_length):]
            
            formatted = f"```\n{truncated}\n```"
        
        await query.message.edit_text(
            f"**Recent Logs (1,024 char limit):**\n{formatted}",
            reply_markup=InlineKeyboardMarkup([
                [InlineKeyboardButton("Full Log", callback_data="web_paste"),
                 InlineKeyboardButton("Back", callback_data="main_menu")]
            ]),
            parse_mode=enums.ParseMode.MARKDOWN
        )
    except Exception as e:
        await query.message.edit_text(f"Error: {str(e)}")

@bot.on_callback_query(filters.regex("^main_menu$"))
async def main_menu_callback(_, query: CallbackQuery):
    """Return to main menu"""
    await query.answer()
    try:
        await query.message.edit_text(
            "Stream's Log:",
            reply_markup=InlineKeyboardMarkup([
                [InlineKeyboardButton("Show Log", callback_data="show_log"),
                 InlineKeyboardButton("Web", callback_data="web_paste")]
            ])
        )
    except Exception as e:
        await query.message.edit_text(f"Error: {str(e)}")


@bot.on_callback_query(filters.regex("^web_paste$"))
async def web_paste_callback(_, query: CallbackQuery):
    """Handle web paste button"""
    await query.answer()
    try:
        log_path = "log.txt"
        async with aiofiles.open(log_path, "r") as f:
            content = await f.read()
            lines = content.splitlines()[-100:]
            truncated = "\n".join(lines)
        
        paste_url = await katbin_paste(truncated)
        await query.message.edit_text(
            f"**Web Paste:** {paste_url}",
            reply_markup=InlineKeyboardMarkup([
                [InlineKeyboardButton("Show Log", callback_data="show_log"),
                 InlineKeyboardButton("Back", callback_data="main_menu")]
            ]),
            disable_web_page_preview=True
        )
    except Exception as e:
        await query.message.edit_text(f"Error: {str(e)}")