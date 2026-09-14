import base64
import hashlib
import hmac
import json
import os
import re
import secrets
import time
import random
from urllib.parse import quote_plus, urlencode, urlparse

import httpx
import jwt
from fastapi import APIRouter, Depends, HTTPException, Query, Request, Response
from fastapi.responses import RedirectResponse, HTMLResponse

from Api.routers.webapp import extract_telegram_user
from Api.schemas.auth import (
    ChangeOwnerPasswordRequest,
    OwnerPasswordLoginRequest,
    PasswordLoginRequest,
    SetCookieRequest,
    SetCredentialsRequest,
    SetOwnerPasswordRequest,
    TgLoginRequest,
    FCMTokenRequest,
    RegisterRequest,
    ValidateOTPRequest,
    TelegramWidgetLoginRequest,
    TelegramTokenLoginRequest,
    IntegrationsUpdateRequest,
)
from stream.helpers.logger import LOGGER
from Api.utils.auth import (
    create_auth_token,
    get_optional_user_id,
    require_admin_user_id,
    require_user_id,
    verify_auth_token,
)
from stream.core.config_manager import Config
from stream.database.MongoDb import db_handler
from Api.services import access_control as access
from Api.utils.auth import is_admin_user

logger = LOGGER(__name__)

router = APIRouter(prefix="/auth", tags=["auth"])


_USERNAME_RE = re.compile(r"^[a-z0-9_\.]{3,32}$", flags=re.I)
_TG_USERNAME_RE = re.compile(r"^[a-z0-9_]{5,32}$", flags=re.I)


def _canon_username(value: str) -> str:
    s = (value or "").strip()
    if not s:
        return ""
    s = s.lower()
    if not _USERNAME_RE.match(s):
        return ""
    return s


def _tg_userpic_url(username: str | None) -> str | None:
    u = (username or "").strip().lstrip("@").strip()
    if not u:
        return None
    if not _TG_USERNAME_RE.match(u):
        return None
    return f"https://t.me/i/userpic/320/{u}.jpg"


async def _get_telegram_profile(user_id: int) -> dict[str, str | None]:
    try:
        from stream import bot
    except Exception:
        bot = None

    if bot is None:
        return {"first_name": None, "telegram_username": None, "profile_url": None, "photo_url": None}

    try:
        u = await bot.get_users(int(user_id))
    except Exception:
        return {"first_name": None, "telegram_username": None, "profile_url": None, "photo_url": None}

    first_name = (getattr(u, "first_name", None) or "").strip() or None
    telegram_username = (getattr(u, "username", None) or "").strip() or None
    photo_url = _tg_userpic_url(telegram_username)
    profile_url = photo_url
    return {
        "first_name": first_name,
        "telegram_username": telegram_username,
        "profile_url": profile_url,
        "photo_url": photo_url,
    }


def _b64e(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).decode("ascii").rstrip("=")


def _b64d(data: str) -> bytes:
    s = (data or "").strip()
    pad = "=" * (-len(s) % 4)
    return base64.urlsafe_b64decode(s + pad)


def _hash_password(password: str, *, salt: bytes | None = None, iterations: int = 200_000) -> dict:
    pwd = (password or "").encode("utf-8")
    if not pwd:
        raise HTTPException(status_code=400, detail="password is required")
    salt = os.urandom(16) if salt is None else salt
    dk = hashlib.pbkdf2_hmac("sha256", pwd, salt, int(iterations))
    return {"algo": "pbkdf2_sha256", "salt": _b64e(salt), "iterations": int(iterations), "hash": _b64e(dk)}


def _verify_password(password: str, stored: dict) -> bool:
    if not isinstance(stored, dict):
        return False
    if (stored.get("algo") or "") != "pbkdf2_sha256":
        return False
    try:
        salt = _b64d(str(stored.get("salt") or ""))
        iters = int(stored.get("iterations") or 0)
        expected = _b64d(str(stored.get("hash") or ""))
    except Exception:
        return False
    if not salt or iters <= 0 or not expected:
        return False
    dk = hashlib.pbkdf2_hmac("sha256", (password or "").encode("utf-8"), salt, iters)
    return hmac.compare_digest(dk, expected)


def _is_truthy(value: object) -> bool:
    if isinstance(value, bool):
        return bool(value)
    s = str(value or "").strip().lower()
    return s in {"1", "true", "yes", "on"}

def _normalize_samesite(value: object) -> str | None:
    s = str(value or "").strip().lower()
    if not s:
        return None
    if s in {"lax", "strict", "none"}:
        return s
    return None


def _set_auth_cookie(
    *,
    response: Response,
    token: str,
) -> None:
    debug = _is_truthy(getattr(Config, "DEBUG", False))
    configured_secure = getattr(Config, "COOKIE_SECURE", None)
    configured_samesite = _normalize_samesite(getattr(Config, "COOKIE_SAMESITE", None))
    secure = _is_truthy(configured_secure) if str(configured_secure or "").strip() else (not debug)
    samesite = configured_samesite or ("lax" if debug else "none")
    response.set_cookie(
        key="auth_token",
        value=str(token),
        httponly=True,
        secure=secure,
        samesite=samesite,
        path="/",
        max_age=365 * 24 * 60 * 60,
    )



def _access_http(denied: "access.AccessDenied") -> HTTPException:
    return HTTPException(status_code=denied.status_code, detail=denied.payload())


async def _issue_user_token(user_id: int, **kwargs) -> str:
    """Gate the login (lock / membership) and mint a token carrying the user's token_version."""
    try:
        await access.assert_can_login(int(user_id))
    except access.AccessDenied as denied:
        raise _access_http(denied)
    info = await access.get_user_access(int(user_id), force=True)
    return create_auth_token(user_id=int(user_id), token_version=info["token_version"], **kwargs)


@router.post("/tg/login")
async def tg_login(
    payload: TgLoginRequest,
    response: Response,
    set_cookie: bool = Query(default=False),
):
    tg = extract_telegram_user(payload.init_data)
    tg_user_id = int(tg["user_id"])
    if tg_user_id <= 0:
        raise HTTPException(status_code=401, detail="invalid telegram user")

    now = time.time()
    updates: dict = {
        "first_name": tg.get("first_name"),
        "photo_url": tg.get("photo_url"),
        "profile_url": tg.get("photo_url"),
        "telegram.id": tg_user_id,
        "telegram.username": tg.get("username"),
        "updated_at": now,
    }

    col = db_handler.get_collection("users").collection
    existing_user = await col.find_one({"_id": tg_user_id})
    if not existing_user:
        try:
            await access.assert_can_register(tg_user_id)
        except access.AccessDenied as denied:
            raise _access_http(denied)

    set_on_insert = {
        "created_at": now,
        "token_version": 0,
        "status": "active",
        "registered_via": {"mode": (await access.get_policy())["registration_mode"]},
    }

    canon = _canon_username(payload.username or "")
    if payload.username is not None:
        if not canon:
            raise HTTPException(status_code=400, detail="invalid username")
        existing = await col.find_one({"username": canon, "_id": {"$ne": tg_user_id}}, {"_id": 1})
        if existing:
            raise HTTPException(status_code=409, detail="username already taken")
        updates["username"] = canon
        updates["username_updated_at"] = now

    if payload.password is not None:
        updates["password"] = _hash_password(payload.password)
        updates["password_updated_at"] = now

    await col.update_one({"_id": tg_user_id}, {"$set": updates, "$setOnInsert": set_on_insert}, upsert=True)

    token = await _issue_user_token(tg_user_id, first_name=tg.get("first_name"), profile_url=tg.get("photo_url"))
    if set_cookie:
        _set_auth_cookie(response=response, token=token)
    return {"ok": True, "user_id": tg_user_id, "token": token}


@router.post("/login")
async def password_login(
    payload: PasswordLoginRequest,
    response: Response,
    set_cookie: bool = Query(default=False),
):
    canon = _canon_username(payload.username)
    if not canon:
        raise HTTPException(status_code=400, detail="invalid username")

    col = db_handler.get_collection("users").collection
    doc = await col.find_one(
        {"username": canon},
        {"_id": 1, "password": 1, "first_name": 1, "profile_url": 1, "photo_url": 1, "telegram": 1},
    )
    if not doc:
        raise HTTPException(status_code=401, detail="invalid credentials")
    if not _verify_password(payload.password, doc.get("password") if isinstance(doc.get("password"), dict) else {}):
        raise HTTPException(status_code=401, detail="invalid credentials")

    uid = int(doc["_id"])
    first_name = doc.get("first_name") if isinstance(doc.get("first_name"), str) and doc.get("first_name").strip() else None
    if not first_name:
        first_name = doc.get("username") if isinstance(doc.get("username"), str) and doc.get("username").strip() else None
    profile_url = doc.get("profile_url") if isinstance(doc.get("profile_url"), str) else None
    if not profile_url:
        profile_url = doc.get("photo_url") if isinstance(doc.get("photo_url"), str) else None
    token = await _issue_user_token(uid, first_name=first_name, profile_url=profile_url)
    if set_cookie:
        _set_auth_cookie(response=response, token=token)
    return {"ok": True, "user_id": uid, "token": token, "first_name": first_name, "username": doc.get("username"), "profile_url": profile_url, "photo_url": profile_url}


def _get_primary_owner_id() -> int:
    owners = getattr(Config, "OWNER_ID", None)
    if isinstance(owners, (int, str)):
        try:
            val = int(owners)
            return val if val > 0 else 0
        except Exception:
            return 0
    for v in (owners or []):
        try:
            uid = int(v)
            if uid > 0:
                return uid
        except Exception:
            continue
    return 0


async def _owner_password_exists() -> bool:
    try:
        col = db_handler.get_collection("auth_config").collection
        doc = await col.find_one({"_id": "owner_password"}, {"password": 1})
        stored = doc.get("password") if isinstance(doc, dict) else None
        return isinstance(stored, dict) and bool(stored)
    except Exception:
        return False


@router.get("/setup/status")
async def setup_status():
    exists = await _owner_password_exists()
    owner_uid = _get_primary_owner_id()
    return {"ok": True, "configured": exists, "needs_setup": not exists, "owner_id": owner_uid if owner_uid > 0 else None}


@router.post("/setup")
async def setup_owner_password(
    payload: SetOwnerPasswordRequest,
    response: Response,
):
    """One-time first-run setup. Creates the hashed owner password and auto-logs in."""
    if await _owner_password_exists():
        raise HTTPException(status_code=403, detail="setup already completed")

    pwd = (payload.password or "").strip()
    if len(pwd) < 6:
        raise HTTPException(status_code=400, detail="password must be at least 6 characters")

    owner_uid = _get_primary_owner_id()
    if owner_uid <= 0:
        raise HTTPException(status_code=500, detail="owner is not configured")

    now = time.time()
    col = db_handler.get_collection("auth_config").collection
    await col.update_one(
        {"_id": "owner_password"},
        {
            "$set": {
                "password": _hash_password(pwd),
                "updated_at": now,
                "created_at": now,
            }
        },
        upsert=True,
    )

    token = create_auth_token(user_id="__api__")
    _set_auth_cookie(response=response, token=token)
    return {"ok": True, "token": token}


@router.post("/password")
async def owner_password_login(
    payload: OwnerPasswordLoginRequest,
    response: Response,
    set_cookie: bool = Query(default=False),
):
    """Single-owner password login. Verifies the submitted password against the
    hashed owner password stored in MongoDB and returns a signed access token."""
    pwd = (payload.password or "").strip()
    if not pwd:
        raise HTTPException(status_code=400, detail="password is required")

    col = db_handler.get_collection("auth_config").collection
    doc = await col.find_one({"_id": "owner_password"}, {"password": 1})
    stored = doc.get("password") if isinstance(doc, dict) else None
    if not isinstance(stored, dict) or not stored:
        raise HTTPException(status_code=503, detail="owner password is not set")

    if not _verify_password(pwd, stored):
        raise HTTPException(status_code=401, detail="invalid credentials")

    token = create_auth_token(user_id="__api__")
    if set_cookie:
        _set_auth_cookie(response=response, token=token)
    return {"ok": True, "token": token}


@router.post("/password/change")
async def change_owner_password(
    payload: ChangeOwnerPasswordRequest,
    user_id: int = Depends(require_user_id),
):
    """Change the owner password. Requires a valid auth token."""
    pwd = (payload.password or "").strip()
    if len(pwd) < 6:
        raise HTTPException(status_code=400, detail="password must be at least 6 characters")

    now = time.time()
    col = db_handler.get_collection("auth_config").collection
    await col.update_one(
        {"_id": "owner_password"},
        {
            "$set": {
                "password": _hash_password(pwd),
                "updated_at": now,
                "updated_by": int(user_id),
            },
            "$setOnInsert": {"created_at": now},
        },
        upsert=True,
    )
    return {"ok": True}


@router.post("/cookie")
async def set_auth_cookie(
    payload: SetCookieRequest,
    response: Response,
):
    token = (payload.token or "").strip()
    if not token:
        raise HTTPException(status_code=400, detail="token is required")
    verified = verify_auth_token(token)
    uid_raw = verified.get("uid")
    if isinstance(uid_raw, str) and uid_raw == "__api__":
        uid = 0
    else:
        try:
            uid = int(uid_raw or 0)
        except (ValueError, TypeError):
            uid = 0
    
    if uid < 0:
        raise HTTPException(status_code=401, detail="invalid auth token")
    if uid == 0 and uid_raw != "__api__":
        raise HTTPException(status_code=401, detail="invalid auth token")
        
    _set_auth_cookie(response=response, token=token)
    return {"ok": True, "user_id": uid}


@router.post("/logout")
async def logout(response: Response):
    debug = _is_truthy(getattr(Config, "DEBUG", False))
    configured_secure = getattr(Config, "COOKIE_SECURE", None)
    configured_samesite = _normalize_samesite(getattr(Config, "COOKIE_SAMESITE", None))
    secure = _is_truthy(configured_secure) if str(configured_secure or "").strip() else (not debug)
    samesite = configured_samesite or ("lax" if debug else "none")
    response.delete_cookie(
        key="auth_token",
        path="/",
        secure=secure,
        samesite=samesite,
    )
    return {"ok": True}


@router.post("/credentials")
async def set_credentials(payload: SetCredentialsRequest, user_id: int = Depends(require_user_id)):
    canon = _canon_username(payload.username)
    if not canon:
        raise HTTPException(status_code=400, detail="invalid username")

    now = time.time()
    col = db_handler.get_collection("users").collection
    existing = await col.find_one({"username": canon, "_id": {"$ne": int(user_id)}}, {"_id": 1})
    if existing:
        raise HTTPException(status_code=409, detail="username already taken")

    await col.update_one(
        {"_id": int(user_id)},
        {
            "$set": {
                "username": canon,
                "password": _hash_password(payload.password),
                "username_updated_at": now,
                "password_updated_at": now,
                "updated_at": now,
            }
        },
        upsert=True,
    )
    return {"ok": True}


@router.get("/me")
async def auth_me(user_id: int | None = Depends(get_optional_user_id)):
    if user_id is None:
        return {"ok": True, "guest": True, "user": None}

    col = db_handler.get_collection("users").collection
    doc = await col.find_one(
        {"_id": int(user_id)},
        {
            "_id": 1,
            "username": 1,
            "first_name": 1,
            "profile_url": 1,
            "photo_url": 1,
            "telegram": 1,
            "integrations": 1,
            "created_at": 1,
            "updated_at": 1,
        },
    )
    if not doc:
        return {"ok": True, "user_id": int(user_id)}
    doc["_id"] = int(doc["_id"])
    doc["user_id"] = int(doc["_id"])
    doc["userid"] = int(doc["_id"])
    doc["id"] = int(doc["_id"])
    if not isinstance(doc.get("profile_url"), str) or not doc.get("profile_url"):
        pu = doc.get("photo_url")
        if isinstance(pu, str) and pu:
            doc["profile_url"] = pu

    if "integrations" not in doc or not isinstance(doc["integrations"], dict):
        doc["integrations"] = {}

    uid = int(user_id)
    owner_set = set()
    owners = getattr(Config, "OWNER_ID", None)
    if isinstance(owners, (int, str)):
        owners = [owners]
    for v in (owners or []):
        try:
            owner_set.add(int(v))
        except Exception:
            pass
            
    sudo_set = set()
    sudos = getattr(Config, "SUDO_USERS", None)
    if isinstance(sudos, (int, str)):
        sudos = [sudos]
    for v in (sudos or []):
        try:
            sudo_set.add(int(v))
        except Exception:
            pass
            
    if uid in owner_set:
        doc["role"] = "owner"
    elif uid in sudo_set:
        doc["role"] = "sudo"
    doc["is_admin"] = is_admin_user(uid)
    info = await access.get_user_access(uid)
    doc["status"] = info["status"]
    doc["lock_reason"] = info.get("lock_reason")

    return {"ok": True, "user": doc}


@router.get("/access/status")
async def access_status():
    """Public: what the signup / login screens need to know about the access policy."""
    policy = await access.get_policy()
    return {"ok": True, **access.public_policy(policy)}


@router.post("/access/verify-membership")
async def verify_membership(request: Request):
    """User-triggered re-check after joining the required chat(s). Works even while restricted."""
    tokens = [t for t in [(request.headers.get("authorization") or "").replace("Bearer ", "").strip(), (request.headers.get("x-auth-token") or "").strip(), (request.cookies.get("auth_token") or "").strip()] if t]
    uid: int | None = None
    for tok in tokens:
        try:
            payload = verify_auth_token(tok)
            if isinstance(payload.get("uid"), int):
                uid = int(payload["uid"])
                break
        except Exception:
            continue
    if not uid:
        raise HTTPException(status_code=401, detail="user login required")
    return await access.reverify_user(uid)


@router.post("/integrations")
async def update_integrations(
    payload: IntegrationsUpdateRequest,
    user_id: int = Depends(require_user_id),
):
    col = db_handler.get_collection("users").collection
    now = time.time()
    updates: dict = {"updated_at": now}

    if payload.discord is not None:
        discord_data = payload.discord.model_dump()
        discord_data["updated_at"] = now
        updates["integrations.discord"] = discord_data

    if payload.lastfm is not None:
        lastfm_data = payload.lastfm.model_dump()
        lastfm_data["updated_at"] = now
        updates["integrations.lastfm"] = lastfm_data

    await col.update_one({"_id": int(user_id)}, {"$set": updates}, upsert=True)
    updated_user = await col.find_one({"_id": int(user_id)}, {"integrations": 1})
    return {
        "ok": True,
        "integrations": (updated_user or {}).get("integrations", {}),
    }


@router.post("/fcm-token")
async def update_fcm_token(payload: FCMTokenRequest, user_id: int = Depends(require_user_id)):
    col = db_handler.get_collection("users").collection
    await col.update_one(
        {"_id": int(user_id)},
        {"$set": {"fcm_token": payload.fcm_token, "updated_at": time.time()}},
        upsert=True,
    )
    return {"ok": True}

@router.post("/register")
async def register_account(payload: RegisterRequest):
    from stream import bot
    if not bot:
        raise HTTPException(status_code=500, detail="Bot is not running or ONLY_API is true")

    canon = _canon_username(payload.username)
    if not canon:
        raise HTTPException(status_code=400, detail="invalid username")

    col = db_handler.get_collection("users").collection
    existing_user = await col.find_one({"_id": payload.userid})
    if existing_user and existing_user.get("password"):
        raise HTTPException(status_code=409, detail="User already registered")

    existing_username = await col.find_one({"username": canon, "_id": {"$ne": payload.userid}}, {"_id": 1})
    if existing_username:
        raise HTTPException(status_code=409, detail="username already taken")

    # Registration policy: closed / invite / allowlist / required chat membership
    try:
        reg_ctx = await access.assert_can_register(int(payload.userid), payload.invite_code)
    except access.AccessDenied as denied:
        raise _access_http(denied)

    otp = str(random.randint(100000, 999999))
    now = time.time()
    otp_col = db_handler.get_collection("registration_otps").collection
    tg_profile = await _get_telegram_profile(int(payload.userid))
    
    await otp_col.update_one(
        {"_id": payload.userid},
        {
            "$set": {
                "otp": otp,
                "username": canon,
                "password": _hash_password(payload.password),
                "first_name": tg_profile.get("first_name"),
                "telegram_username": tg_profile.get("telegram_username"),
                "profile_url": tg_profile.get("profile_url"),
                "photo_url": tg_profile.get("photo_url"),
                "invite_code": reg_ctx.get("invite_code"),
                "created_at": now
            }
        },
        upsert=True
    )
    
    try:
        await bot.send_message(
            chat_id=payload.userid,
            text=f"Your StreamX registration OTP is: `{otp}`\n\nThis OTP is valid for registration."
        )
    except Exception as e:
        import logging
        logging.getLogger(__name__).error(f"Failed to send OTP to {payload.userid}: {e}")
        raise HTTPException(status_code=500, detail="Failed to send OTP via Telegram bot. Have you started the bot?")

    return {
        "ok": True,
        "message": "OTP sent",
        "user_id": int(payload.userid),
        "username": canon,
        "first_name": tg_profile.get("first_name"),
        "profile_url": tg_profile.get("profile_url"),
        "photo_url": tg_profile.get("photo_url"),
    }

@router.post("/validate")
async def validate_account(
    payload: ValidateOTPRequest,
    response: Response,
    set_cookie: bool = Query(default=False),
):
    otp_col = db_handler.get_collection("registration_otps").collection
    doc = await otp_col.find_one({"_id": payload.userid})
    if not doc:
        raise HTTPException(status_code=400, detail="No pending registration found")

    if doc.get("otp") != payload.otp:
        raise HTTPException(status_code=401, detail="Invalid OTP")

    # Re-check the policy at completion time (mode may have changed while the OTP was pending)
    try:
        await access.assert_can_register(int(payload.userid), doc.get("invite_code"))
        if doc.get("invite_code"):
            await access.consume_invite(str(doc["invite_code"]), int(payload.userid))
    except access.AccessDenied as denied:
        raise _access_http(denied)

    now = time.time()
    col = db_handler.get_collection("users").collection
    tg_profile = await _get_telegram_profile(int(payload.userid))
    first_name = tg_profile.get("first_name") or (doc.get("first_name") if isinstance(doc.get("first_name"), str) else None)
    telegram_username = tg_profile.get("telegram_username") or (
        doc.get("telegram_username") if isinstance(doc.get("telegram_username"), str) else None
    )
    profile_url = tg_profile.get("profile_url") or (doc.get("profile_url") if isinstance(doc.get("profile_url"), str) else None)
    photo_url = tg_profile.get("photo_url") or (doc.get("photo_url") if isinstance(doc.get("photo_url"), str) else None)
    if not profile_url:
        profile_url = photo_url
    if not photo_url:
        photo_url = profile_url

    updates = {
        "username": doc["username"],
        "password": doc["password"],
        "first_name": first_name,
        "photo_url": photo_url,
        "profile_url": profile_url,
        "profile_refreshed_at": now,
        "username_updated_at": now,
        "password_updated_at": now,
        "updated_at": now,
        "telegram.id": int(payload.userid),
        "telegram.username": telegram_username,
        "status": "active",
        "registered_via": {"mode": (await access.get_policy())["registration_mode"], "invite_code": doc.get("invite_code")},
    }
    set_on_insert = {
        "created_at": now,
        "token_version": 0,
    }

    await col.update_one(
        {"_id": payload.userid},
        {"$set": updates, "$setOnInsert": set_on_insert},
        upsert=True
    )

    await otp_col.delete_one({"_id": payload.userid})

    token = await _issue_user_token(int(payload.userid), first_name=first_name, profile_url=profile_url, photo_url=photo_url)
    if set_cookie:
        _set_auth_cookie(response=response, token=token)
        
    return {
        "ok": True,
        "user_id": int(payload.userid),
        "username": doc.get("username"),
        "token": token,
        "first_name": first_name,
        "profile_url": profile_url,
        "photo_url": photo_url,
    }


_telegram_jwks_client = jwt.PyJWKClient(
    "https://oauth.telegram.org/.well-known/jwks.json",
    cache_keys=True,
    max_cached_keys=16,
)


def _get_telegram_client_id() -> str:
    cid = (getattr(Config, "TELEGRAM_OIDC_CLIENT_ID", "") or "").strip()
    if cid:
        return cid
    bot_token = (getattr(Config, "BOT_TOKEN", "") or "").strip()
    if ":" in bot_token:
        return bot_token.split(":")[0].strip()
    return ""


async def _get_bot_username() -> str:
    try:
        from stream import bot
        if bot and getattr(bot, "me", None) and getattr(bot.me, "username", None):
            return str(bot.me.username)
    except Exception:
        pass
    bot_token = (getattr(Config, "BOT_TOKEN", "") or "").strip()
    if bot_token:
        try:
            async with httpx.AsyncClient(timeout=4.0) as client:
                r = await client.get(f"https://api.telegram.org/bot{bot_token}/getMe")
                if r.status_code == 200:
                    data = r.json()
                    if data.get("ok"):
                        return data.get("result", {}).get("username", "")
        except Exception:
            pass
    return ""


def _get_telegram_redirect_uri(request: Request, frontend_base: str | None = None) -> str:
    configured = (getattr(Config, "TELEGRAM_OIDC_REDIRECT_URI", "") or "").strip()
    if configured:
        return configured
    if frontend_base and frontend_base.startswith(("http://", "https://")):
        return f"{frontend_base.rstrip('/')}/auth/telegram/callback"
    proto = request.headers.get("x-forwarded-proto") or request.url.scheme
    host = request.headers.get("x-forwarded-host") or request.headers.get("host") or request.url.netloc
    return f"{proto}://{host}/auth/telegram/callback"


def _get_frontend_base_url(request: Request, client_redirect: str | None = None) -> str:
    if client_redirect and client_redirect.startswith(("http://", "https://")):
        parsed = urlparse(client_redirect)
        if parsed.scheme and parsed.netloc:
            return f"{parsed.scheme}://{parsed.netloc}"

    origin = request.headers.get("origin") or ""
    if not origin:
        referer = request.headers.get("referer") or ""
        if referer:
            parsed = urlparse(referer)
            if parsed.scheme and parsed.netloc:
                origin = f"{parsed.scheme}://{parsed.netloc}"
    if not origin:
        cors = (getattr(Config, "CORS_ORIGIN", "") or "").strip()
        if cors and cors != "*":
            origin = cors.split(",")[0].strip()
    if not origin:
        proto = request.headers.get("x-forwarded-proto") or request.url.scheme
        host = request.headers.get("x-forwarded-host") or request.headers.get("host") or request.url.netloc
        origin = f"{proto}://{host}"
    return origin.rstrip("/")


def _generate_pkce_pair() -> tuple[str, str]:
    code_verifier = secrets.token_urlsafe(64)
    digest = hashlib.sha256(code_verifier.encode("ascii")).digest()
    code_challenge = base64.urlsafe_b64encode(digest).decode("ascii").rstrip("=")
    return code_verifier, code_challenge


def _verify_telegram_widget_data(data: dict, bot_token: str) -> bool:
    received_hash = data.get("hash")
    if not received_hash or not bot_token:
        return False
    try:
        auth_date = int(data.get("auth_date", 0))
    except Exception:
        return False
    if time.time() - auth_date > 86400:
        return False
    # Telegram Login Widget hash is computed ONLY over Telegram's user data fields:
    # auth_date, first_name, id, last_name, photo_url, username.
    # Exclude 'hash' as well as custom application fields like 'invite_code' or query params.
    telegram_fields = {"auth_date", "first_name", "id", "last_name", "photo_url", "username"}
    items = []
    for k in sorted(data.keys()):
        if k in telegram_fields and data[k] is not None:
            items.append(f"{k}={data[k]}")
    data_check_string = "\n".join(items)
    secret_key = hashlib.sha256(bot_token.encode("utf-8")).digest()
    computed_hash = hmac.new(secret_key, data_check_string.encode("utf-8"), hashlib.sha256).hexdigest()
    return hmac.compare_digest(computed_hash, str(received_hash).lower())


@router.get("/telegram/config")
async def telegram_config():
    client_id = _get_telegram_client_id()
    bot_username = await _get_bot_username()
    enabled = bool(client_id or bot_username)
    return {
        "ok": True,
        "enabled": enabled,
        "client_id": client_id,
        "bot_username": bot_username,
    }


@router.get("/telegram/start")
async def telegram_start(
    request: Request,
    redirect: str = Query(default="/"),
    frontend_url: str | None = Query(default=None),
    invite_code: str | None = Query(default=None),
):
    client_id = _get_telegram_client_id()
    if not client_id:
        raise HTTPException(status_code=500, detail="Telegram Client ID is not configured")

    state = secrets.token_urlsafe(32)
    code_verifier, code_challenge = _generate_pkce_pair()
    frontend_base = (frontend_url or "").strip().rstrip("/") or _get_frontend_base_url(request, redirect)
    callback_uri = _get_telegram_redirect_uri(request, frontend_base)
    dest_path = redirect if redirect.startswith("/") and not redirect.startswith("//") else "/"
    clean_invite = (invite_code or "").strip().upper() or None

    now = time.time()
    oidc_col = db_handler.get_collection("oidc_sessions").collection
    await oidc_col.update_one(
        {"_id": state},
        {
            "$set": {
                "code_verifier": code_verifier,
                "redirect_uri": callback_uri,
                "frontend_base": frontend_base,
                "dest_path": dest_path,
                "invite_code": clean_invite,
                "created_at": now,
            }
        },
        upsert=True,
    )

    configured_origin = (getattr(Config, "TELEGRAM_OIDC_ORIGIN", "") or "").strip().rstrip("/")
    origin = configured_origin or frontend_base
    if origin and not origin.startswith(("http://", "https://")):
        origin = f"https://{origin}"

    client_secret = (getattr(Config, "TELEGRAM_OIDC_CLIENT_SECRET", "") or "").strip()
    if client_secret:
        auth_params = {
            "client_id": client_id,
            "bot_id": client_id,
            "origin": origin,
            "redirect_uri": callback_uri,
            "response_type": "code",
            "scope": "openid profile",
            "state": state,
            "code_challenge": code_challenge,
            "code_challenge_method": "S256",
        }
    else:
        auth_params = {
            "bot_id": client_id,
            "origin": origin,
            "return_to": callback_uri,
            "request_access": "write",
        }
    auth_url = f"https://oauth.telegram.org/auth?{urlencode(auth_params)}"
    logger.info(f"[Telegram Auth] /telegram/start: redirecting to {auth_url} (mode={'oidc' if client_secret else 'widget'})")
    return RedirectResponse(url=auth_url, status_code=302)


@router.get("/telegram/callback")
async def telegram_callback(
    request: Request,
    response: Response,
    code: str | None = Query(default=None),
    state: str | None = Query(default=None),
    error: str | None = Query(default=None),
    error_description: str | None = Query(default=None),
):
    query_params = dict(request.query_params)
    logger.info(f"Telegram callback invoked: query_keys={list(query_params.keys())}")

    frontend_base = _get_frontend_base_url(request)

    # 1. Error response from provider
    if error:
        err_msg = error_description or error
        logger.warning(f"Telegram callback returned error: {error} ({error_description})")
        return RedirectResponse(
            url=f"{frontend_base}/login?error={quote_plus(error)}",
            status_code=302,
        )

    # 2. Telegram Widget Query flow (if id and hash are present in query params)
    if "hash" in query_params and "id" in query_params:
        bot_token = (getattr(Config, "BOT_TOKEN", "") or "").strip()
        if bot_token and _verify_telegram_widget_data(query_params, bot_token):
            try:
                tg_user_id = int(query_params["id"])
            except Exception:
                return RedirectResponse(url=f"{frontend_base}/login?error=invalid_user_id", status_code=302)

            name = (query_params.get("first_name") or "").strip()
            if query_params.get("last_name"):
                name = f"{name} {query_params['last_name'].strip()}".strip()
            username = (query_params.get("username") or "").strip() or None
            photo_url = (query_params.get("photo_url") or "").strip() or None

            now = time.time()
            col = db_handler.get_collection("users").collection
            updates: dict = {
                "user_id": tg_user_id,
                "userid": tg_user_id,
                "telegram.id": tg_user_id,
                "updated_at": now,
            }
            if name:
                updates["first_name"] = name
            if username:
                updates["telegram.username"] = username
            if photo_url:
                updates["photo_url"] = photo_url
                updates["profile_url"] = photo_url

            existing = await col.find_one({"_id": tg_user_id}, {"username": 1, "first_name": 1, "photo_url": 1, "profile_url": 1})
            if existing:
                if not name and existing.get("first_name"):
                    name = existing.get("first_name")
                if not photo_url:
                    photo_url = existing.get("profile_url") or existing.get("photo_url")
            else:
                invite_code = query_params.get("invite_code")
                try:
                    reg_ctx = await access.assert_can_register(tg_user_id, invite_code)
                    if reg_ctx.get("invite_code"):
                        await access.consume_invite(reg_ctx["invite_code"], tg_user_id)
                except access.AccessDenied as denied:
                    return RedirectResponse(
                        url=f"{frontend_base}/login?error={quote_plus(str(denied.code))}",
                        status_code=302,
                    )
                if username:
                    canon = _canon_username(username)
                    if canon:
                        u_conflict = await col.find_one({"username": canon, "_id": {"$ne": tg_user_id}}, {"_id": 1})
                        if not u_conflict:
                            updates["username"] = canon
                            updates["username_updated_at"] = now

            set_on_insert = {
                "created_at": now,
                "token_version": 0,
                "status": "active",
                "registered_via": {"mode": (await access.get_policy())["registration_mode"]},
            }
            await col.update_one({"_id": tg_user_id}, {"$set": updates, "$setOnInsert": set_on_insert}, upsert=True)

            try:
                token = await _issue_user_token(
                    tg_user_id,
                    first_name=name or username,
                    profile_url=photo_url,
                    photo_url=photo_url,
                )
            except HTTPException as exc:
                d = exc.detail if isinstance(exc.detail, dict) else {"detail": str(exc.detail), "message": str(exc.detail)}
                return RedirectResponse(url=f"{frontend_base}/login?error={quote_plus(str(d.get('detail')))}", status_code=302)

            redirect_target = f"{frontend_base}/login?token={token}&redirect=%2F"
            safe_first_name = json.dumps(name or username or "Telegram User")
            safe_username = json.dumps(username)
            safe_photo = json.dumps(photo_url)
            safe_target = json.dumps(redirect_target)
            popup_html = f"""<!DOCTYPE html>
<html><body><script>
  if (window.opener && window.opener !== window) {{
    try {{
      window.opener.postMessage({{
        type: 'tg_auth_success',
        token: '{token}',
        user: {{
          user_id: {tg_user_id},
          first_name: {safe_first_name},
          username: {safe_username},
          photo_url: {safe_photo}
        }}
      }}, window.location.origin);
      window.close();
    }} catch (e) {{
      window.location.replace({safe_target});
    }}
  }} else {{
    window.location.replace({safe_target});
  }}
</script></body></html>"""
            html_resp = HTMLResponse(content=popup_html, status_code=200)
            _set_auth_cookie(response=html_resp, token=token)
            return html_resp
        else:
            logger.warning("Telegram widget HMAC signature validation failed on callback query")
            return RedirectResponse(url=f"{frontend_base}/login?error=invalid_widget_signature", status_code=302)

    # 3. Standard OIDC Authorization Code Flow
    if code and state:
        oidc_col = db_handler.get_collection("oidc_sessions").collection
        session = await oidc_col.find_one({"_id": state})
        if not session:
            logger.warning(f"Telegram OIDC session not found for state: {state}")
            return RedirectResponse(url=f"{frontend_base}/login?error=invalid_state", status_code=302)

        frontend_base = session.get("frontend_base") or frontend_base
        dest_path = session.get("dest_path") or "/"
        await oidc_col.delete_one({"_id": state})

        if time.time() - float(session.get("created_at", 0)) > 600:
            return RedirectResponse(url=f"{frontend_base}/login?error=expired_session", status_code=302)

        client_id = _get_telegram_client_id()
        client_secret = (getattr(Config, "TELEGRAM_OIDC_CLIENT_SECRET", "") or "").strip()

        token_data = {
            "grant_type": "authorization_code",
            "code": code,
            "redirect_uri": session.get("redirect_uri") or _get_telegram_redirect_uri(request),
            "client_id": client_id,
            "code_verifier": session["code_verifier"],
        }
        if client_secret:
            token_data["client_secret"] = client_secret

        try:
            async with httpx.AsyncClient(timeout=15.0) as http_client:
                token_resp = await http_client.post(
                    "https://oauth.telegram.org/token",
                    data=token_data,
                    auth=(str(client_id), client_secret) if client_secret else None,
                )
            if token_resp.status_code != 200:
                logger.error(f"[Telegram Auth] Token exchange HTTP failed: {token_resp.status_code} {token_resp.text}")
                return RedirectResponse(url=f"{frontend_base}/login?error=token_exchange_failed", status_code=302)
            resp_json = token_resp.json()
            if "error" in resp_json:
                err_code = resp_json.get("error", "token_exchange_failed")
                err_desc = resp_json.get("error_description") or err_code
                logger.error(f"[Telegram Auth] Token exchange returned error: {err_code} ({err_desc})")
                return RedirectResponse(
                    url=f"{frontend_base}/login?error={quote_plus(err_code)}",
                    status_code=302,
                )
        except Exception as e:
            logger.error(f"[Telegram Auth] Error communicating with Telegram token endpoint: {e}")
            return RedirectResponse(url=f"{frontend_base}/login?error=provider_unavailable", status_code=302)

        id_token = resp_json.get("id_token")
        if not id_token:
            logger.error(f"[Telegram Auth] Token response missing id_token: {resp_json}")
            return RedirectResponse(url=f"{frontend_base}/login?error=missing_id_token", status_code=302)
        logger.info("[Telegram Auth] Token exchanged successfully. Validating id_token claims...")

        try:
            signing_key = _telegram_jwks_client.get_signing_key_from_jwt(id_token)
            claims = jwt.decode(
                id_token,
                signing_key.key,
                algorithms=["RS256", "ES256", "EdDSA", "ES256K"],
                issuer="https://oauth.telegram.org",
                audience=str(client_id),
                options={"verify_exp": True},
            )
        except Exception as e:
            logger.error(f"Telegram ID token validation failed: {e}")
            return RedirectResponse(url=f"{frontend_base}/login?error=invalid_id_token", status_code=302)

        sub = claims.get("sub")
        try:
            tg_user_id = int(sub)
            if tg_user_id <= 0:
                raise ValueError()
        except Exception:
            logger.error(f"Telegram claims sub is invalid: {sub}")
            return RedirectResponse(url=f"{frontend_base}/login?error=invalid_user_id", status_code=302)

        name = (claims.get("name") or "").strip()
        preferred_username = (claims.get("preferred_username") or "").strip() or None
        picture = (claims.get("picture") or "").strip() or None

        now = time.time()
        col = db_handler.get_collection("users").collection
        updates = {
            "user_id": tg_user_id,
            "userid": tg_user_id,
            "telegram.id": tg_user_id,
            "updated_at": now,
        }
        if name:
            updates["first_name"] = name
        if preferred_username:
            updates["telegram.username"] = preferred_username
        if picture:
            updates["photo_url"] = picture
            updates["profile_url"] = picture

        existing = await col.find_one({"_id": tg_user_id}, {"username": 1, "first_name": 1, "photo_url": 1, "profile_url": 1})
        if existing:
            if not name and existing.get("first_name"):
                name = existing.get("first_name")
            if not picture:
                picture = existing.get("profile_url") or existing.get("photo_url")
        else:
            invite_code = (session_doc.get("invite_code") if session_doc else None) or request.query_params.get("invite_code")
            try:
                reg_ctx = await access.assert_can_register(tg_user_id, invite_code)
                if reg_ctx.get("invite_code"):
                    await access.consume_invite(reg_ctx["invite_code"], tg_user_id)
            except access.AccessDenied as denied:
                return RedirectResponse(
                    url=f"{frontend_base}/login?error={quote_plus(str(denied.code))}",
                    status_code=302,
                )
            if preferred_username:
                canon = _canon_username(preferred_username)
                if canon:
                    u_conflict = await col.find_one({"username": canon, "_id": {"$ne": tg_user_id}}, {"_id": 1})
                    if not u_conflict:
                        updates["username"] = canon
                        updates["username_updated_at"] = now

        set_on_insert = {
            "created_at": now,
            "token_version": 0,
            "status": "active",
            "registered_via": {"mode": (await access.get_policy())["registration_mode"]},
        }
        await col.update_one({"_id": tg_user_id}, {"$set": updates, "$setOnInsert": set_on_insert}, upsert=True)

        try:
            token = await _issue_user_token(
                tg_user_id,
                first_name=name or preferred_username,
                profile_url=picture,
                photo_url=picture,
            )
        except HTTPException as exc:
            d = exc.detail if isinstance(exc.detail, dict) else {"detail": str(exc.detail), "message": str(exc.detail)}
            return RedirectResponse(url=f"{frontend_base}/login?error={quote_plus(str(d.get('detail')))}", status_code=302)

        dest_encoded = quote_plus(dest_path)
        redirect_target = f"{frontend_base}/login?token={token}&redirect={dest_encoded}"
        safe_first_name = json.dumps(name or preferred_username or "Telegram User")
        safe_username = json.dumps(preferred_username)
        safe_photo = json.dumps(picture)
        safe_target = json.dumps(redirect_target)
        popup_html = f"""<!DOCTYPE html>
<html><body><script>
  if (window.opener && window.opener !== window) {{
    try {{
      window.opener.postMessage({{
        type: 'tg_auth_success',
        token: '{token}',
        user: {{
          user_id: {tg_user_id},
          first_name: {safe_first_name},
          username: {safe_username},
          photo_url: {safe_photo}
        }}
      }}, window.location.origin);
      window.close();
    }} catch (e) {{
      window.location.replace({safe_target});
    }}
  }} else {{
    window.location.replace({safe_target});
  }}
</script></body></html>"""
        html_resp = HTMLResponse(content=popup_html, status_code=200)
        _set_auth_cookie(response=html_resp, token=token)
        return html_resp

    # 4. Hash fragment / tgAuthResult HTML Fallback (when Telegram redirects with URL fragment)
    html_content = """<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Verifying Telegram Login...</title>
  <style>
    body {
      margin: 0;
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      background-color: #0b0f17;
      color: #f1f5f9;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    }
    .card {
      text-align: center;
      padding: 36px 44px;
      background: rgba(30, 41, 59, 0.8);
      backdrop-filter: blur(16px);
      border: 1px solid rgba(255, 255, 255, 0.1);
      border-radius: 20px;
      max-width: 360px;
      box-shadow: 0 20px 40px rgba(0,0,0,0.5);
    }
    .spinner {
      width: 44px;
      height: 44px;
      border: 4px solid rgba(56, 189, 248, 0.2);
      border-top-color: #38bdf8;
      border-radius: 50%;
      animation: spin 0.8s linear infinite;
      margin: 0 auto 16px;
    }
    @keyframes spin { to { transform: rotate(360deg); } }
    h2 { font-size: 18px; margin: 0 0 8px; font-weight: 600; }
    p { font-size: 13px; color: #94a3b8; margin: 0; }
  </style>
</head>
<body>
  <div class="card">
    <div class="spinner"></div>
    <h2>Verifying Telegram login</h2>
    <p>Connecting your account, please wait...</p>
  </div>
  <script>
    (async function() {
      try {
        const hash = window.location.hash.substring(1);
        const search = window.location.search.substring(1);
        const hashParams = new URLSearchParams(hash);
        const searchParams = new URLSearchParams(search);

        const dest = sessionStorage.getItem('tg_auth_redirect') || '/';
        let inviteCode = searchParams.get('invite_code') || '';
        if (!inviteCode) {
          try {
            inviteCode = window.opener?.sessionStorage?.getItem('webx_invite_code') || sessionStorage.getItem('webx_invite_code') || '';
          } catch (e) {}
        }

        // 1. Check for tgAuthResult in hash (standard Telegram Web callback)
        const tgAuthResult = hashParams.get('tgAuthResult');
        if (tgAuthResult) {
          let base64 = tgAuthResult.replace(/-/g, '+').replace(/_/g, '/');
          while (base64.length % 4) { base64 += '='; }
          const binary = atob(base64);
          const bytes = Uint8Array.from(binary, c => c.charCodeAt(0));
          const decodedJson = new TextDecoder().decode(bytes);
          const payload = JSON.parse(decodedJson);
          if (inviteCode) {
            payload.invite_code = inviteCode;
          }

          const res = await fetch('/auth/telegram/widget', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
          });
          const data = await res.json();
          if (data.ok && data.token) {
            sessionStorage.removeItem('tg_auth_redirect');
            sessionStorage.removeItem('webx_invite_code');
            if (window.opener && window.opener !== window) {
              try {
                window.opener.postMessage({ type: 'tg_auth_success', token: data.token, user: data }, window.location.origin);
                window.close();
                return;
              } catch (e) {}
            }
            window.location.replace('/login?token=' + encodeURIComponent(data.token) + '&redirect=' + encodeURIComponent(dest));
            return;
          } else {
            const errObj = (data && typeof data.detail === 'object' && data.detail !== null) ? data.detail : null;
            const errCode = errObj ? (errObj.detail || 'widget_verification_failed') : (data.detail || 'widget_verification_failed');
            const errDesc = errObj ? (errObj.message || '') : (typeof data.detail === 'string' ? data.detail : '');
            if (window.opener && window.opener !== window) {
              try {
                window.opener.postMessage({ type: 'tg_auth_error', error: errCode, error_description: errDesc }, window.location.origin);
                window.close();
                return;
              } catch (e) {}
            }
            window.location.replace('/login?error=' + encodeURIComponent(errCode));
            return;
          }
        }

        // 2. Check for id_token in hash (OIDC fragment response)
        const idToken = hashParams.get('id_token');
        if (idToken) {
          const res = await fetch('/auth/telegram/validate-token', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ id_token: idToken, invite_code: inviteCode || undefined })
          });
          const data = await res.json();
          if (data.ok && data.token) {
            sessionStorage.removeItem('tg_auth_redirect');
            sessionStorage.removeItem('webx_invite_code');
            if (window.opener && window.opener !== window) {
              try {
                window.opener.postMessage({ type: 'tg_auth_success', token: data.token, user: data }, window.location.origin);
                window.close();
                return;
              } catch (e) {}
            }
            window.location.replace('/login?token=' + encodeURIComponent(data.token) + '&redirect=' + encodeURIComponent(dest));
            return;
          } else {
            const errObj = (data && typeof data.detail === 'object' && data.detail !== null) ? data.detail : null;
            const errCode = errObj ? (errObj.detail || 'token_verification_failed') : (data.detail || 'token_verification_failed');
            const errDesc = errObj ? (errObj.message || '') : (typeof data.detail === 'string' ? data.detail : '');
            if (window.opener && window.opener !== window) {
              try {
                window.opener.postMessage({ type: 'tg_auth_error', error: errCode, error_description: errDesc }, window.location.origin);
                window.close();
                return;
              } catch (e) {}
            }
            window.location.replace('/login?error=' + encodeURIComponent(errCode));
            return;
          }
        }

        // 3. Check for code & state in hash
        const code = hashParams.get('code') || searchParams.get('code');
        const state = hashParams.get('state') || searchParams.get('state');
        if (code && state) {
          window.location.replace('/auth/telegram/callback?code=' + encodeURIComponent(code) + '&state=' + encodeURIComponent(state));
          return;
        }

        // 4. Check for error in hash or search
        const error = hashParams.get('error') || searchParams.get('error');
        if (error) {
          const errorDesc = hashParams.get('error_description') || searchParams.get('error_description') || '';
          if (window.opener && window.opener !== window) {
            try {
              window.opener.postMessage({ type: 'tg_auth_error', error: error, error_description: errorDesc }, window.location.origin);
              window.close();
              return;
            } catch (e) {}
          }
          window.location.replace('/login?error=' + encodeURIComponent(error));
          return;
        }

        // 5. Check if query params have widget auth (id and hash)
        if (searchParams.has('id') && searchParams.has('hash')) {
          window.location.replace('/auth/telegram/callback?' + search);
          return;
        }

        // Nothing found
        window.location.replace('/login?error=invalid_state');
      } catch (err) {
        console.error('Telegram auth callback processing error:', err);
        window.location.replace('/login?error=callback_processing_failed');
      }
    })();
  </script>
</body>
</html>"""
    return HTMLResponse(content=html_content, status_code=200)


@router.post("/telegram/widget")
async def telegram_widget_login(
    payload: TelegramWidgetLoginRequest,
    response: Response,
    set_cookie: bool = Query(default=True),
):
    logger.info(f"[Telegram Auth] /telegram/widget: validating widget payload for user_id={payload.id}, username={payload.username}")
    bot_token = (getattr(Config, "BOT_TOKEN", "") or "").strip()
    if not bot_token:
        logger.error("[Telegram Auth] /telegram/widget failed: BOT_TOKEN is not configured")
        raise HTTPException(status_code=500, detail="BOT_TOKEN is not configured")

    widget_dict = payload.model_dump()
    widget_dict.pop("invite_code", None)
    if not _verify_telegram_widget_data(widget_dict, bot_token):
        logger.warning(f"[Telegram Auth] /telegram/widget: signature verification failed for user_id={payload.id}")
        raise HTTPException(status_code=401, detail="invalid telegram widget signature")

    tg_user_id = int(payload.id)
    if tg_user_id <= 0:
        raise HTTPException(status_code=400, detail="invalid user id")

    name = (payload.first_name or "").strip()
    if payload.last_name:
        name = f"{name} {payload.last_name.strip()}".strip()
    username = (payload.username or "").strip() or None
    photo_url = (payload.photo_url or "").strip() or None

    now = time.time()
    col = db_handler.get_collection("users").collection
    updates: dict = {
        "user_id": tg_user_id,
        "userid": tg_user_id,
        "telegram.id": tg_user_id,
        "updated_at": now,
    }
    if name:
        updates["first_name"] = name
    if username:
        updates["telegram.username"] = username
    if photo_url:
        updates["photo_url"] = photo_url
        updates["profile_url"] = photo_url

    existing = await col.find_one({"_id": tg_user_id}, {"username": 1, "first_name": 1, "photo_url": 1, "profile_url": 1})
    if existing:
        if not name and existing.get("first_name"):
            name = existing.get("first_name")
        if not photo_url:
            photo_url = existing.get("profile_url") or existing.get("photo_url")
    else:
        invite_code = getattr(payload, "invite_code", None)
        try:
            reg_ctx = await access.assert_can_register(tg_user_id, invite_code)
            if reg_ctx.get("invite_code"):
                await access.consume_invite(reg_ctx["invite_code"], tg_user_id)
        except access.AccessDenied as denied:
            raise _access_http(denied)
        if username:
            canon = _canon_username(username)
            if canon:
                u_conflict = await col.find_one({"username": canon, "_id": {"$ne": tg_user_id}}, {"_id": 1})
                if not u_conflict:
                    updates["username"] = canon
                    updates["username_updated_at"] = now

    set_on_insert = {
        "created_at": now,
        "token_version": 0,
        "status": "active",
        "registered_via": {"mode": (await access.get_policy())["registration_mode"], "invite_code": getattr(payload, "invite_code", None)},
    }
    await col.update_one({"_id": tg_user_id}, {"$set": updates, "$setOnInsert": set_on_insert}, upsert=True)

    token = await _issue_user_token(
        tg_user_id,
        first_name=name or username,
        profile_url=photo_url,
        photo_url=photo_url,
    )
    if set_cookie:
        _set_auth_cookie(response=response, token=token)

    logger.info(f"[Telegram Auth] /telegram/widget: user {tg_user_id} ({name or username}) successfully authenticated")
    return {
        "ok": True,
        "user_id": tg_user_id,
        "token": token,
        "first_name": name,
        "username": existing.get("username") if existing else (updates.get("username") or username),
        "profile_url": photo_url,
        "photo_url": photo_url,
    }


@router.post("/telegram/validate-token")
async def telegram_validate_token(
    payload: TelegramTokenLoginRequest,
    response: Response,
    set_cookie: bool = Query(default=True),
):
    id_token = (payload.id_token or "").strip()
    if not id_token:
        raise HTTPException(status_code=400, detail="id_token is required")

    logger.info("[Telegram Auth] /telegram/validate-token: validating received ID token against JWKS...")
    client_id = _get_telegram_client_id()
    try:
        signing_key = _telegram_jwks_client.get_signing_key_from_jwt(id_token)
        claims = jwt.decode(
            id_token,
            signing_key.key,
            algorithms=["RS256", "ES256", "EdDSA", "ES256K"],
            issuer="https://oauth.telegram.org",
            audience=str(client_id) if client_id else None,
            options={"verify_exp": True, "verify_aud": bool(client_id)},
        )
    except Exception as e:
        logger.error(f"[Telegram Auth] Telegram ID token validation failed: {e}")
        raise HTTPException(status_code=401, detail=f"invalid telegram ID token: {e}")

    sub = claims.get("sub")
    try:
        tg_user_id = int(sub)
        if tg_user_id <= 0:
            raise ValueError()
    except Exception:
        raise HTTPException(status_code=400, detail="invalid user identifier in token")

    name = (claims.get("name") or "").strip()
    preferred_username = (claims.get("preferred_username") or "").strip() or None
    picture = (claims.get("picture") or "").strip() or None

    now = time.time()
    col = db_handler.get_collection("users").collection
    updates: dict = {
        "telegram.id": tg_user_id,
        "updated_at": now,
    }
    if name:
        updates["first_name"] = name
    if preferred_username:
        updates["telegram.username"] = preferred_username
    if picture:
        updates["photo_url"] = picture
        updates["profile_url"] = picture

    existing = await col.find_one({"_id": tg_user_id}, {"username": 1, "first_name": 1, "photo_url": 1, "profile_url": 1})
    if existing:
        if not name and existing.get("first_name"):
            name = existing.get("first_name")
        if not picture:
            picture = existing.get("profile_url") or existing.get("photo_url")
    else:
        invite_code = getattr(payload, "invite_code", None)
        try:
            reg_ctx = await access.assert_can_register(tg_user_id, invite_code)
            if reg_ctx.get("invite_code"):
                await access.consume_invite(reg_ctx["invite_code"], tg_user_id)
        except access.AccessDenied as denied:
            raise _access_http(denied)
        if preferred_username:
            canon = _canon_username(preferred_username)
            if canon:
                u_conflict = await col.find_one({"username": canon, "_id": {"$ne": tg_user_id}}, {"_id": 1})
                if not u_conflict:
                    updates["username"] = canon
                    updates["username_updated_at"] = now

    set_on_insert = {
        "created_at": now,
        "token_version": 0,
        "status": "active",
        "registered_via": {"mode": (await access.get_policy())["registration_mode"], "invite_code": getattr(payload, "invite_code", None)},
    }
    await col.update_one({"_id": tg_user_id}, {"$set": updates, "$setOnInsert": set_on_insert}, upsert=True)

    token = await _issue_user_token(
        tg_user_id,
        first_name=name or preferred_username,
        profile_url=picture,
        photo_url=picture,
    )
    if set_cookie:
        _set_auth_cookie(response=response, token=token)

    logger.info(f"[Telegram Auth] /telegram/validate-token: user {tg_user_id} ({name or preferred_username}) successfully authenticated")
    return {
        "ok": True,
        "user_id": tg_user_id,
        "token": token,
        "first_name": name,
        "username": existing.get("username") if existing else (updates.get("username") or preferred_username),
        "profile_url": picture,
        "photo_url": picture,
    }

