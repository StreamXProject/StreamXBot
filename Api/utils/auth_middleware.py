from __future__ import annotations

import time
from urllib.parse import parse_qs

from fastapi import HTTPException
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from Api.utils.auth import verify_auth_token

_SETUP_CACHE: dict[str, object] = {"value": None, "expires": 0.0}
_SETUP_CACHE_TTL_SEC = 10.0

# Path prefixes that REQUIRE a valid auth token.
# Anything not matching this list is allowed through (SPA routes, static
# files, public endpoints, the /auth/* login flow, /health, etc.).
_PROTECTED_PREFIXES: tuple[str, ...] = (
    "/tracks",
    "/albums",
    "/artists",
    "/topics",
    "/search",
    "/browse",
    "/library",
    "/api/v1/library",
    "/playlists",
    "/favourites",
    "/top-played",
    "/me",
    "/jam",
    "/friends",
    "/notifications",
    "/admin",
    "/logs",
    "/youtube",
    "/soundcloud",
    "/test",
    "/daily-playlist",
    "/channelids",
    "/covers/user-playlist",
)


def _path_is_protected(path: str) -> bool:
    p = (path or "/").rstrip("/") or "/"
    for prefix in _PROTECTED_PREFIXES:
        if p == prefix or p.startswith(prefix + "/"):
            return True
    return False


_OPTIONAL_PREFIXES: tuple[str, ...] = (
    "/browse",
    "/tracks",
    "/albums",
    "/artists",
    "/topics",
    "/search",
    "/playlists/available",
    "/daily-playlist",
    "/channelids",
    "/covers",
    "/share",
    "/recaps/share",
    "/library/shuffle",
    "/api/v1/library/shuffle",
)


def _is_optional_route(path: str, method: str) -> bool:
    if method not in ("GET", "HEAD"):
        return False

    p = (path or "/").rstrip("/") or "/"
    for prefix in _OPTIONAL_PREFIXES:
        if p == prefix or p.startswith(prefix + "/"):
            return True

    # Allow public playlist listing (returns empty for guests) and shared playlist view
    if p == "/playlists" or (p.startswith("/playlists/") and not p.endswith("/tracks")):
        return True

    return False


def _extract_tokens(request: Request) -> list[str]:
    tokens = []
    auth_header = (request.headers.get("authorization") or "").strip()
    if auth_header:
        if auth_header.lower().startswith("bearer "):
            tokens.append(auth_header[7:].strip())
        else:
            tokens.append(auth_header)
    x_auth = (request.headers.get("x-auth-token") or "").strip()
    if x_auth:
        tokens.append(x_auth)
    query_token = (request.query_params.get("token") or "").strip()
    if query_token:
        tokens.append(query_token)
    raw_query = str(request.url.query or "")
    if raw_query:
        parsed = parse_qs(raw_query)
        vals = parsed.get("token")
        if vals:
            t = str(vals[0]).strip()
            if t and t not in tokens:
                tokens.append(t)
    for c_name in ("auth_token", "token"):
        c = (request.cookies.get(c_name) or "").strip()
        if c and c not in tokens:
            tokens.append(c)
    return tokens


async def _owner_password_exists() -> bool:
    now = time.time()
    if _SETUP_CACHE["expires"] > now and _SETUP_CACHE["value"] is not None:
        return bool(_SETUP_CACHE["value"])

    from stream.database.MongoDb import db_handler

    try:
        col = db_handler.get_collection("auth_config").collection
        doc = await col.find_one({"_id": "owner_password"}, {"password": 1})
        stored = doc.get("password") if isinstance(doc, dict) else None
        exists = isinstance(stored, dict) and bool(stored)
        _SETUP_CACHE["value"] = exists
        _SETUP_CACHE["expires"] = now + _SETUP_CACHE_TTL_SEC
        return exists
    except Exception:
        return False


class AuthMiddleware(BaseHTTPMiddleware):
    """Enforces bearer/cookie auth on protected API routes.

    During first-run (owner password not yet configured), protected API
    routes return 503 so the frontend can redirect to the setup screen.
    Public/static/auth routes and browser HTML navigation are excluded.
    Optional browsing routes allow unauthenticated/guest requests.
    CORS preflight (OPTIONS) is always allowed through so the CORSMiddleware
    can handle it.
    """

    async def dispatch(self, request: Request, call_next) -> Response:
        if request.method == "OPTIONS":
            return await call_next(request)

        # Allow browser HTML / SPA navigation directly through to serve frontend
        from Api.utils.spa_middleware import is_html_navigation
        if is_html_navigation(request):
            return await call_next(request)

        path = request.url.path or "/"
        if not _path_is_protected(path):
            return await call_next(request)

        if not await _owner_password_exists():
            return JSONResponse(
                status_code=503,
                content={"ok": False, "detail": "setup required"},
            )

        tokens = _extract_tokens(request)
        is_optional = _is_optional_route(path, request.method)

        if not tokens:
            if is_optional:
                return await call_next(request)
            return JSONResponse(
                status_code=401,
                content={"ok": False, "detail": "auth token required"},
            )

        authenticated = False
        valid_payload = None
        for tok in tokens:
            try:
                valid_payload = verify_auth_token(tok)
                authenticated = True
                break
            except Exception:
                continue

        if not authenticated:
            if is_optional:
                return await call_next(request)
            return JSONResponse(
                status_code=401,
                content={"ok": False, "detail": "invalid auth token"},
            )

        if valid_payload:
            uid = valid_payload.get("uid")
            if uid and uid != "__api__":
                from stream.core.source_filter import is_source_banned

                if await is_source_banned(uid):
                    return JSONResponse(
                        status_code=403,
                        content={"ok": False, "detail": "user is banned"},
                    )
                # Account lock / membership restriction / revoked session (30 s memoised per user)
                from Api.services.access_control import AccessDenied, assert_can_use

                try:
                    await assert_can_use(int(uid), valid_payload.get("tv"))
                except AccessDenied as denied:
                    return JSONResponse(status_code=denied.status_code, content=denied.payload())
                except (TypeError, ValueError):
                    pass

        return await call_next(request)
