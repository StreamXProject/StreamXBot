import json
import os
import re
from pathlib import Path

_env_path = Path(__file__).resolve().parent / ".env"
try:
    from dotenv import load_dotenv

    if _env_path.exists():
        load_dotenv(dotenv_path=_env_path, override=False)
    else:
        load_dotenv(override=False)
except ImportError:
    if _env_path.exists():
        with open(_env_path, "r", encoding="utf-8") as _f:
            for _line in _f:
                _line = _line.strip()
                if not _line or _line.startswith("#") or "=" not in _line:
                    continue
                _k, _, _v = _line.partition("=")
                _k = _k.strip()
                _v = _v.strip()
                if (_v.startswith('"') and _v.endswith('"')) or (_v.startswith("'") and _v.endswith("'")):
                    _v = _v[1:-1]
                if _k and _k not in os.environ:
                    os.environ[_k] = _v


def _get_str(key: str, default: str = "") -> str:
    val = os.getenv(key)
    if val is None:
        return default
    return str(val).strip()


def _get_int(key: str, default: int = 0) -> int:
    val = os.getenv(key)
    if val is None or not str(val).strip():
        return default
    try:
        return int(str(val).strip())
    except (TypeError, ValueError):
        return default


def _get_bool(key: str, default: bool = False) -> bool:
    val = os.getenv(key)
    if val is None or not str(val).strip():
        return default
    return str(val).strip().lower() in ("true", "1", "yes", "y", "on")


def _get_list_int(key: str, default: list | None = None) -> list[int]:
    if default is None:
        default = []
    val = os.getenv(key)
    if val is None or not str(val).strip():
        return default
    s = str(val).strip()
    if s.startswith("["):
        try:
            parsed = json.loads(s)
            if isinstance(parsed, list):
                return [int(x) for x in parsed if str(x).strip().lstrip("-").isdigit()]
        except Exception:
            pass
    parts = re.split(r"[\s,]+", s)
    out = [int(p) for p in parts if p.strip().lstrip("-").isdigit()]
    return out if out else default


def _get_id_or_list(key: str, default: int | list = 0) -> int | list:
    val = os.getenv(key)
    if val is None or not str(val).strip():
        return default
    s = str(val).strip()
    if s.startswith("["):
        try:
            parsed = json.loads(s)
            if isinstance(parsed, list):
                return [int(x) for x in parsed if str(x).strip().lstrip("-").isdigit()]
        except Exception:
            pass
    try:
        return int(s)
    except (TypeError, ValueError):
        return default


def _get_filter_mode(key: str, default: int = 1) -> int | str:
    val = os.getenv(key)
    if val is None or not str(val).strip():
        return default
    s = str(val).strip()
    if s.isdigit():
        return int(s)
    return s


def _get_chat_topic(key: str, default: int = 0) -> int | str:
    val = os.getenv(key)
    if val is None or not str(val).strip():
        return default
    s = str(val).strip()
    if s.lower() == "all":
        return "all"
    try:
        return int(s)
    except (TypeError, ValueError):
        return s


# Core
BOT_TOKEN = _get_str("BOT_TOKEN")
API_ID = _get_int("API_ID", 0)
API_HASH = _get_str("API_HASH")
MONGO_URI = _get_str("MONGO_URI")
DATABASE_NAME = _get_str("DATABASE_NAME", "StreamX")
OWNER_ID = _get_id_or_list("OWNER_ID", 0)
SUDO_USERS = _get_list_int("SUDO_USERS", [])
SECRET_KEY = _get_str("SECRET_KEY")
GITHUB_TOKEN = _get_str("GITHUB_TOKEN")
ONLY_API = _get_bool("ONLY_API", False)

# Firebase
FIREBASE_CREDENTIALS = _get_str("FIREBASE_CREDENTIALS")

# Debug
DEBUG = _get_bool("DEBUG", False)

# Thumbnail Generation
COLLEGE = _get_bool("COLLEGE", True)
TEXT_COLOR = _get_str("TEXT_COLOR", "#FFFFFF")
YOUTUBE_COVER_SEARCH = _get_bool("YOUTUBE_COVER_SEARCH", True)

# API & Web
CORS_ORIGIN = _get_str("CORS_ORIGIN", "*")
CORS_ORIGINS = _get_str("CORS_ORIGINS", CORS_ORIGIN)
COOKIE_SECURE = _get_bool("COOKIE_SECURE", True)
COOKIE_SAMESITE = _get_str("COOKIE_SAMESITE", "none")

# Userbot
SESSION_STRING = _get_str("SESSION_STRING")
SOURCE_CHANNEL_IDS = _get_list_int("SOURCE_CHANNEL_IDS", [])
CHAT_TOPIC = _get_chat_topic("CHAT_TOPIC", 0)
USERBOT_COOLDOWN_SEC = float(os.getenv("USERBOT_COOLDOWN_SEC", "0.2"))
USERBOT_BATCH_SIZE = int(os.getenv("USERBOT_BATCH_SIZE", "50"))
USERBOT_BATCH_COOLDOWN_SEC = float(os.getenv("USERBOT_BATCH_COOLDOWN_SEC", "1.0"))
USERBOT_DUMP_MODE = str(os.getenv("USERBOT_DUMP_MODE", "FORWARD") or "FORWARD").strip().upper()
USERBOT_CAPTION_MODE = str(os.getenv("USERBOT_CAPTION_MODE", "LAST") or "LAST").strip().upper()
ENRICHMENT_WORKERS = int(os.getenv("ENRICHMENT_WORKERS", os.getenv("PROCESSING_CONTENT", "16")))

# Misc
CHANNEL_ID = _get_int("CHANNEL_ID", 0)
DUMP_CHANNEL_ID = _get_int("DUMP_CHANNEL_ID", 0)
FILTER_MODE = _get_filter_mode("FILTER_MODE", 1)
COLLABORATOR_ID = _get_list_int("COLLABORATOR_ID", [])
COLLABORATOR_IDS = _get_list_int("COLLABORATOR_IDS", COLLABORATOR_ID)

# Lyrics API
LRCLIB = _get_bool("LRCLIB", False)
MUSIXMATCH = _get_bool("MUSIXMATCH", True)
BETTERLYRICS = _get_bool("BETTERLYRICS", True)
KUGOU = _get_bool("KUGOU", True)

# Spotify
SPOTIFY_CLIENT_ID = _get_str("SPOTIFY_CLIENT_ID")
SPOTIFY_CLIENT_SECRET = _get_str("SPOTIFY_CLIENT_SECRET")

# Multi-Clients
MULTI_CLIENTS = _get_bool("MULTI_CLIENTS", False)
MULTI_CLIENTS_1 = _get_str("MULTI_CLIENTS_1")
MULTI_CLIENTS_2 = _get_str("MULTI_CLIENTS_2")
MULTI_CLIENTS_3 = _get_str("MULTI_CLIENTS_3")
MULTI_CLIENTS_4 = _get_str("MULTI_CLIENTS_4")

for _env_k, _env_v in os.environ.items():
    if _env_k.startswith("MULTI_CLIENTS_") and _env_k not in globals():
        globals()[_env_k] = _env_v