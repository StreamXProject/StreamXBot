import os
import asyncio
import time
from typing import Any
import uuid
import yt_dlp
import requests
from PIL import Image
from fastapi import APIRouter, Depends, HTTPException, BackgroundTasks
from pydantic import BaseModel
from Api.utils.auth import require_user_id
from stream.helpers.logger import LOGGER
from Api.deps.db import get_audio_tracks_collection
from stream import bot
from stream.core.config_manager import Config
from Api.services.lyrics_service import get_track_lyrics

LOG = LOGGER(__name__)
router = APIRouter(prefix="/youtube", tags=["YouTube"])

ROOT_DIR = os.getcwd()
COOKIES_PATH = os.path.join(ROOT_DIR, "cookies", "yt.txt")
MAX_FILE_SIZE_MB = 200
MAX_FILE_SIZE_BYTES = MAX_FILE_SIZE_MB * 1024 * 1024

def _normalize_mime_type(mime: Any) -> str:
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


class YTDownloadRequest(BaseModel):
    url: str

def progress_hook(d):
    """Hook to cancel download if file size exceeds limit."""
    if d['status'] == 'downloading':
        downloaded = d.get('downloaded_bytes', 0)
        total = d.get('total_bytes') or d.get('total_bytes_estimate', 0)
        if total > MAX_FILE_SIZE_BYTES or downloaded > MAX_FILE_SIZE_BYTES:
            raise Exception(f"File size exceeds {MAX_FILE_SIZE_MB}MB limit.")

def download_thumbnail(url: str, video_id: str) -> str | None:
    """Download thumbnail and return local path for Telegram."""
    try:
        tmp_dir = "stream_media"
        os.makedirs(tmp_dir, exist_ok=True)
        resp = requests.get(url, timeout=10)
        if resp.status_code == 200:
            thumb_path = os.path.join(tmp_dir, f"thumb_{video_id}.jpg")
            with open(thumb_path, "wb") as f:
                f.write(resp.content)
            
            with Image.open(thumb_path) as img:
                img.thumbnail((320, 320))
                img.save(thumb_path, "JPEG")
            return thumb_path
    except Exception as e:
        LOG.warning(f"Failed to download thumbnail: {e}")
    return None

async def process_yt_download(url: str, user_id: int):
    """Background task to download audio from YouTube, upload to Telegram, and save to DB."""
    ydl_opts = {
        'format': 'bestaudio/ba/best[ext=m4a]/best[ext=mp3]/best',
        'noplaylist': True,
        'quiet': True,
        'no_warnings': True,
        'nocheckcertificate': True,
        'cookiefile': COOKIES_PATH if os.path.exists(COOKIES_PATH) else None,
        'progress_hooks': [progress_hook],
        'http_headers': {
            'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
            'Accept-Language': 'en-US,en;q=0.5',
            'Referer': 'https://www.google.com/',
        }
    }
    
    final_filepath = None
    thumbnail_path = None
    try:
        loop = asyncio.get_event_loop()
        
        # 1. Extract metadata
        info = None
        try:
            with yt_dlp.YoutubeDL(ydl_opts) as ydl:
                info = await loop.run_in_executor(None, lambda: ydl.extract_info(url, download=False))
        except Exception as e:
            msg_upper = str(e).upper()
            if 'COOKIE' in msg_upper or 'FORMAT' in msg_upper:
                LOG.warning(f"YouTube extraction failed (e={e}), retrying without cookies...")
                ydl_opts_no_cookies = ydl_opts.copy()
                ydl_opts_no_cookies.pop('cookiefile', None)
                with yt_dlp.YoutubeDL(ydl_opts_no_cookies) as ydl:
                    info = await loop.run_in_executor(None, lambda: ydl.extract_info(url, download=False))
                    ydl_opts = ydl_opts_no_cookies
            else:
                raise
            
        if not info:
            LOG.error(f"Could not extract info for YouTube URL: {url}")
            return
            
        video_id = info.get('id')
        title = info.get('title', 'Unknown Title')
        performer = info.get('uploader', 'Unknown Artist')
        duration = info.get('duration', 0)
        thumbnail_url = info.get('thumbnail')
        ext = info.get('ext', 'mp3')
        
        # --- DUPLICATE CHECK ---
        col = get_audio_tracks_collection()
        existing = await col.find_one({"audio.yt_id": video_id})
        if existing:
            LOG.info(f"Track with YouTube ID {video_id} already exists in database. Skipping download.")
            return
        # ------------------------

        # 2. Download thumbnail for Telegram
        if thumbnail_url:
            thumbnail_path = await loop.run_in_executor(None, download_thumbnail, thumbnail_url, video_id)

        # 3. Prepare download path
        tmp_dir = "stream_media"
        os.makedirs(tmp_dir, exist_ok=True)
        
        # 4. Download audio directly (no conversion to force MP3)
        ydl_opts_dl = ydl_opts.copy()
        download_template = os.path.join(tmp_dir, f"{uuid.uuid4().hex[:16]}.%(ext)s")
        ydl_opts_dl['outtmpl'] = download_template
        
        with yt_dlp.YoutubeDL(ydl_opts_dl) as ydl:
            info_dl = await loop.run_in_executor(None, lambda: ydl.extract_info(url, download=True))
            final_filepath = ydl.prepare_filename(info_dl)

        if not os.path.exists(final_filepath):
            LOG.error(f"File download failed for YouTube URL: {url}. Path {final_filepath} not found.")
            return

        # 5. Upload to Telegram
        if bot is None:
            LOG.error("Bot is not initialized. Cannot upload track.")
            return

        channel_id = int(Config.CHANNEL_ID)
        if not channel_id:
            LOG.error("CHANNEL_ID is not configured.")
            return
        
        try:
            file_size = os.path.getsize(final_filepath)
            LOG.info(f"Uploading file: {os.path.basename(final_filepath)} ({file_size / (1024*1024):.2f} MB)")
            
            msg = await bot.send_audio(
                chat_id=channel_id,
                audio=final_filepath,
                title=title,
                performer=performer,
                duration=int(duration),
                thumb=thumbnail_path if thumbnail_path else None,
                caption=f"<blockquote>{title}</blockquote>"
            )
        except Exception as e:
            LOG.error(f"Pyrogram upload failed: {e}")
            return
        
        if not msg:
            LOG.error("Failed to upload to Telegram (msg is null).")
            return
        
        media = msg.audio or msg.document
        if not media:
            LOG.error("Uploaded message has no audio or document attribute.")
            return

        # 6. Save to MongoDB
        track_id = media.file_unique_id
        
        try:
            me = await bot.get_me()
            bot_id = str(me.id)
        except Exception:
            bot_id = "default"
        
        # Match the provided sample document structure
        doc = {
            "_id": track_id,
            "source_chat_id": channel_id,
            "source_message_id": msg.id,
            "audio": {
                "title": title,
                "artist": performer,
                "performer": performer,
                "duration_sec": float(duration),
                "type": ext,
                "yt_id": video_id,
                "yt_url": url,
                "album": "YouTube",
                "album_id": "album_youtube"
            },
            "telegram": {
                "file_id": media.file_id,
                "mime_type": _normalize_mime_type(getattr(media, "mime_type", "audio/mpeg")),
                "file_size": media.file_size,
                "file_ids": {
                    bot_id: media.file_id
                }
            },
            "spotify": {
                "cover_url": thumbnail_url,
                "cover_source": "youtube"
            },
            "fingerprint": f"{title.lower()}|{performer.lower()}|youtube|{int(duration)}",
            "created_at": time.time(),
            "updated_at": time.time(),
            "deleted": False
        }
        
        await col.insert_one(doc)
        LOG.info(f"Successfully processed YouTube audio: {title} (Track ID: {track_id}, Msg ID: {msg.id})")
        
        # 7. Look for lyrics in background
        try:
            asyncio.create_task(get_track_lyrics(track_id))
        except Exception as e:
            LOG.warning(f"Failed to trigger lyrics fetch: {e}")
            
    except Exception as e:
        LOG.error(f"Error in background YouTube process: {e}")
    finally:
        # Cleanup local files
        for p in [final_filepath, thumbnail_path]:
            if p and os.path.exists(p):
                try:
                    os.remove(p)
                except Exception as e:
                    LOG.debug(f"Failed to cleanup {p}: {e}")

@router.post("/download")
async def download_youtube_track(
    req: YTDownloadRequest, 
    background_tasks: BackgroundTasks, 
    user_id: int = Depends(require_user_id)
):
    """
    Trigger a YouTube audio download with cookie support and thumbnail optimization.
    The process runs in the background.
    """
    if not req.url:
        raise HTTPException(status_code=400, detail="YouTube URL is required")

    from stream.core.source_filter import (
        FilterMode,
        get_filter_mode,
        is_source_allowed,
        is_source_banned,
    )

    if await is_source_banned(user_id):
        raise HTTPException(status_code=403, detail="User is banned from adding tracks")

    mode = get_filter_mode()
    if mode == FilterMode.HYBRID:
        if not await is_source_allowed(user_id):
            raise HTTPException(
                status_code=403,
                detail="User is not in allowed contributors collection (hybrid mode)",
            )

    background_tasks.add_task(process_yt_download, req.url, user_id)
    return {
        "ok": True,
        "message": "YouTube audio download started in background",
        "url": req.url,
    }
