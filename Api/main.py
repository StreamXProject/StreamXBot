import os
from contextlib import asynccontextmanager
from fastapi import FastAPI
from fastapi.staticfiles import StaticFiles
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import FileResponse

from Api.deps.db import init_db
from Api.utils.auth_middleware import AuthMiddleware
from Api.routers.recaps import router as recaps_router
from Api.routers.access import router as access_router
from Api.routers.browse import router as browse_router
from Api.routers.share import router as share_router
from Api.routers.auth import router as auth_router
from Api.routers.jam import router as jam_router
from Api.routers.webapp import router as webapp_router
from Api.routers.favourites import router as favourites_router
from Api.routers.playlists import router as playlists_router
from Api.routers.health import router as health_router
from Api.routers.test import router as test_router
from Api.routers.tracks import router as tracks_router
from Api.routers.cover import router as cover_router
from Api.routers.admin_refresh import router as admin_refresh_router
from Api.routers.friends import router as friends_router
from Api.routers.notifications import router as notifications_router
from Api.routers.presence import router as presence_router
from Api.routers.soundcloud import router as soundcloud_router
from Api.routers.logs import router as logs_router
from Api.routers.yt_dlp import router as yt_dlp_router
from Api.routers.sources import router as sources_router
from Api.routers.topics import router as topics_router
from Api.routers.discord import router as discord_router

from stream.core.config_manager import Config


BASE_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
_webx_dist = os.path.join(BASE_DIR, "WebX", "dist")
if os.path.exists(_webx_dist):
    DIST_DIR = _webx_dist
else:
    DIST_DIR = os.path.join(BASE_DIR, "dist")
    if not os.path.exists(DIST_DIR):
        _web_dist = os.path.join(BASE_DIR, "StreamXWeb", "dist")
        if os.path.exists(_web_dist):
            DIST_DIR = _web_dist
ASSETS_DIR = os.path.join(DIST_DIR, "assets")


@asynccontextmanager
async def lifespan(app: FastAPI):
    await init_db()
    await Config.load_from_db()
    yield


app = FastAPI(lifespan=lifespan)


if os.path.exists(ASSETS_DIR):
    app.mount("/assets", StaticFiles(directory=ASSETS_DIR), name="assets")

def _get_cors_origins():
    cors = getattr(Config, "CORS_ORIGIN", None) or getattr(Config, "CORS_ORIGINS", None)
    if not cors:
        return ["*"]
    if isinstance(cors, list):
        origins = [str(c).strip() for c in cors if str(c).strip()]
        return origins or ["*"]
    if isinstance(cors, str):
        s = cors.strip()
        if not s or s == "*":
            return ["*"]
        if "," in s:
            origins = [x.strip() for x in s.split(",") if x.strip()]
            return origins or ["*"]
        return [s]
    return ["*"]

app.add_middleware(AuthMiddleware)

_cors_origins = _get_cors_origins()
_allow_all_origins = "*" in _cors_origins

app.add_middleware(
    CORSMiddleware,
    allow_origins=[] if _allow_all_origins else _cors_origins,
    allow_origin_regex=r"^(https?://|capacitor://|tauri://|ionic://).*" if _allow_all_origins else None,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
    expose_headers=["Accept-Ranges", "Content-Range", "Content-Length"],
)


# API routers
app.include_router(health_router)
app.include_router(auth_router)
app.include_router(jam_router)
app.include_router(webapp_router)
app.include_router(topics_router)
app.include_router(browse_router)
app.include_router(tracks_router)
app.include_router(playlists_router)
app.include_router(favourites_router)
app.include_router(cover_router)
app.include_router(admin_refresh_router)
app.include_router(test_router)
app.include_router(friends_router)
app.include_router(notifications_router)
app.include_router(presence_router)
app.include_router(share_router)
app.include_router(soundcloud_router)
app.include_router(logs_router)
app.include_router(yt_dlp_router)
app.include_router(sources_router)
app.include_router(discord_router)
app.include_router(recaps_router)
app.include_router(access_router)



@app.get("/")
async def serve_root():
    index_file = os.path.join(DIST_DIR, "index.html")
    if os.path.exists(index_file):
        return FileResponse(index_file, headers={"Cache-Control": "no-cache, no-store, must-revalidate"})
    return {"status": "frontend not built"}


@app.get("/{full_path:path}")
async def serve_spa(full_path: str):
    file_path = os.path.join(DIST_DIR, full_path)

    if os.path.exists(file_path):
        if full_path in ("sw.js", "manifest.json"):
            return FileResponse(file_path, headers={"Cache-Control": "no-cache, no-store, must-revalidate"})
        return FileResponse(file_path)

    index_file = os.path.join(DIST_DIR, "index.html")
    if os.path.exists(index_file):
        return FileResponse(index_file, headers={"Cache-Control": "no-cache, no-store, must-revalidate"})

    return {"status": "frontend not built"}