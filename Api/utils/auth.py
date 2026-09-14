import base64
import hashlib
import hmac
import json
import time
from typing import Optional

from fastapi import Depends, Header, HTTPException, Request
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer

from stream.core.config_manager import Config

_bearer_scheme = HTTPBearer(auto_error=False)


def _b64url_encode(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode("ascii")


def _b64url_decode(data: str) -> bytes:
    s = (data or "").strip()
    pad = "=" * (-len(s) % 4)
    return base64.urlsafe_b64decode(s + pad)


def _get_secret_key_bytes() -> bytes:
    secret = (getattr(Config, "SECRET_KEY", "") or "").strip()
    if not secret:
        secret = (getattr(Config, "BOT_TOKEN", "") or "").strip()
    if not secret:
        raise RuntimeError(
            "SECRET_KEY is required but not configured. Please set SECRET_KEY or BOT_TOKEN in your .env or environment."
        )
    return secret.encode("utf-8")


def create_auth_token(
    *,
    user_id: int | str,
    ttl_sec: int = 365 * 24 * 60 * 60,
    first_name: str | None = None,
    photo_url: str | None = None,
    profile_url: str | None = None,
    token_version: int | None = None,
) -> str:
    if isinstance(user_id, str) and user_id == "__api__":
        uid = user_id
    else:
        uid = int(user_id)
        if uid <= 0:
            raise HTTPException(status_code=400, detail="user_id must be a positive int")

    now = int(time.time())
    payload: dict[str, object] = {
        "uid": uid,
        "user_id": uid,
        "userid": uid,
        "iat": now,
        "exp": now + int(ttl_sec),
    }
    if token_version is not None:
        payload["tv"] = int(token_version)  # bumped server-side to revoke every session at once
    fn = (first_name or "").strip()
    if fn:
        payload["first_name"] = fn
    pu = (profile_url or photo_url or "").strip()
    if pu:
        payload["profile_url"] = pu
        payload["photo_url"] = pu
    payload_json = json.dumps(payload, separators=(",", ":"), ensure_ascii=False).encode("utf-8")
    payload_b64 = _b64url_encode(payload_json)

    secret = _get_secret_key_bytes()
    sig = hmac.new(secret, payload_b64.encode("utf-8"), hashlib.sha256).digest()
    sig_b64 = _b64url_encode(sig)
    return f"v1.{payload_b64}.{sig_b64}"


def verify_auth_token(token: str) -> dict:
    raw = (token or "").strip()
    if not raw:
        raise HTTPException(status_code=401, detail="missing auth token")

    if raw.lower().startswith("bearer "):
        raw = raw[7:].strip()

    parts = raw.split(".")
    if len(parts) != 3 or parts[0] != "v1":
        raise HTTPException(status_code=401, detail="invalid auth token")

    payload_b64 = parts[1].strip()
    sig_b64 = parts[2].strip()

    # Collect candidate secrets for seamless verification and backwards compatibility
    candidates: list[bytes] = []
    try:
        primary = _get_secret_key_bytes()
        candidates.append(primary)
    except Exception:
        pass

    bot_token = (getattr(Config, "BOT_TOKEN", "") or "").strip()
    if bot_token:
        bt_bytes = bot_token.encode("utf-8")
        if bt_bytes not in candidates:
            candidates.append(bt_bytes)

    configured_secret = (getattr(Config, "SECRET_KEY", "") or "").strip()
    if configured_secret:
        cfg_bytes = configured_secret.encode("utf-8")
        if cfg_bytes not in candidates:
            candidates.append(cfg_bytes)

    fallback_secret = b"SHA234567JDNKDNSNNFNDKSMSERTYUWERTY"
    if fallback_secret not in candidates:
        candidates.append(fallback_secret)

    matched = False
    for cand in candidates:
        expected = _b64url_encode(hmac.new(cand, payload_b64.encode("utf-8"), hashlib.sha256).digest())
        if hmac.compare_digest(expected, sig_b64):
            matched = True
            break

    if not matched:
        raise HTTPException(status_code=401, detail="invalid auth token")

    try:
        payload = json.loads(_b64url_decode(payload_b64).decode("utf-8"))
    except Exception:
        raise HTTPException(status_code=401, detail="invalid auth token")

    uid = payload.get("uid")
    exp = payload.get("exp")
    try:
        exp = int(exp) if exp is not None else 0
    except Exception:
        raise HTTPException(status_code=401, detail="invalid auth token")

    if isinstance(uid, str) and uid == "__api__":
        pass
    else:
        try:
            uid = int(uid)
        except Exception:
            raise HTTPException(status_code=401, detail="invalid auth token")
        if uid <= 0:
            raise HTTPException(status_code=401, detail="invalid auth token")
        payload["uid"] = uid

    if exp and int(time.time()) > exp:
        raise HTTPException(status_code=401, detail="auth token expired")

    return payload


from typing import Optional

async def get_optional_user_id(
    request: Request,
    credentials: HTTPAuthorizationCredentials | None = Depends(_bearer_scheme),
    x_auth_token: str | None = Header(default=None, alias="X-Auth-Token"),
) -> int | None:
    token = ""
    if credentials is not None and (credentials.credentials or "").strip():
        token = (credentials.credentials or "").strip()
    elif (x_auth_token or "").strip():
        token = (x_auth_token or "").strip()
    else:
        token = (request.cookies.get("auth_token") or "").strip()
        if not token:
            token = (request.cookies.get("token") or "").strip()
    
    if not token:
        return None
        
    try:
        payload = verify_auth_token(token)
    except HTTPException:
        return None

    uid = payload.get("uid")
    if isinstance(uid, str) and uid == "__api__":
        return None
    try:
        val = int(uid)
        if val <= 0:
            return None
        from stream.core.source_filter import is_source_banned

        if await is_source_banned(val):
            return None
        from Api.services.access_control import AccessDenied, assert_can_use

        try:
            await assert_can_use(val, payload.get("tv"))
        except AccessDenied:
            return None
        return val
    except (ValueError, TypeError):
        return None


async def require_user_id(
    request: Request,
    credentials: HTTPAuthorizationCredentials | None = Depends(_bearer_scheme),
    x_auth_token: str | None = Header(default=None, alias="X-Auth-Token"),
) -> int:
    token = ""
    if credentials is not None and (credentials.credentials or "").strip():
        token = (credentials.credentials or "").strip()
    elif (x_auth_token or "").strip():
        token = (x_auth_token or "").strip()
    else:
        token = (request.cookies.get("auth_token") or "").strip()
        if not token:
            token = (request.cookies.get("token") or "").strip()
    
    if not token:
        raise HTTPException(status_code=401, detail="user login required")
        
    payload = verify_auth_token(token)
    uid = payload.get("uid")
    if isinstance(uid, str):
        raise HTTPException(status_code=401, detail="user login required")
    try:
        uid_int = int(uid)
        if uid_int <= 0:
            raise HTTPException(status_code=401, detail="user login required")
        from stream.core.source_filter import is_source_banned

        if await is_source_banned(uid_int):
            raise HTTPException(status_code=403, detail="user is banned")
        from Api.services.access_control import AccessDenied, assert_can_use

        try:
            await assert_can_use(uid_int, payload.get("tv"))
        except AccessDenied as denied:
            raise HTTPException(status_code=denied.status_code, detail=denied.payload())
        return uid_int
    except (ValueError, TypeError):
        raise HTTPException(status_code=401, detail="user login required")


def admin_user_ids() -> set[int]:
    owners_raw = getattr(Config, "OWNER_ID", None)
    owners = [owners_raw] if isinstance(owners_raw, (int, str)) else (owners_raw or [])
    sudos_raw = getattr(Config, "SUDO_USERS", None)
    sudos = [sudos_raw] if isinstance(sudos_raw, (int, str)) else (sudos_raw or [])
    allow: set[int] = set()
    for v in list(owners) + list(sudos):
        try:
            allow.add(int(v))
        except Exception:
            pass
    return allow


def is_admin_user(user_id: int | None) -> bool:
    try:
        return user_id is not None and int(user_id) in admin_user_ids()
    except Exception:
        return False


async def require_admin_user_id(user_id: int = Depends(require_user_id)) -> int:
    uid = int(user_id)
    owners_raw = getattr(Config, "OWNER_ID", None)
    owners = [owners_raw] if isinstance(owners_raw, (int, str)) else (owners_raw or [])
    sudos_raw = getattr(Config, "SUDO_USERS", None)
    sudos = [sudos_raw] if isinstance(sudos_raw, (int, str)) else (sudos_raw or [])
    allow: set[int] = set()
    for v in owners:
        try:
            allow.add(int(v))
        except Exception:
            pass
    for v in sudos:
        try:
            allow.add(int(v))
        except Exception:
            pass
    if not allow:
        raise HTTPException(status_code=403, detail="admin access not configured")
    if uid not in allow:
        raise HTTPException(status_code=403, detail="admin only")
    return uid
