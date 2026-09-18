from time import time
from datetime import datetime

from pyrogram import filters
from pyrogram.types import Message

from stream import bot, BotStartTime
from stream.helpers.functions import get_readable_time


@bot.on_message(filters.command(["ping", "alive"]))
async def ping(_, message: Message):

    pong_reply = await message.reply_text("ping!")

    start = datetime.now()
    await pong_reply.edit("pong!")   
    end = datetime.now()

    botuptime = get_readable_time(time() - BotStartTime)
    pong = (end - start).microseconds / 1000

    return await pong_reply.edit(f"`{pong}`ms \n| `{botuptime}`")
