"""
Discord Remote Auth (QR Code Login) router.

Provides:
- WebSocket /ws/discord/remote-auth:
    Real-time bidirectional event stream for the Remote Auth lifecycle.
- REST /api/discord/remote-auth/start:
    Start a background session and receive QR url & fingerprint.
- REST /api/discord/remote-auth/status/{session_id}:
    Poll the session state for completion or scanned events.
- REST /api/discord/remote-auth/cancel/{session_id}:
    Cancel an active session.
"""
from __future__ import annotations

import asyncio
import uuid
import time
from typing import Dict, Optional, Any

from fastapi import APIRouter, WebSocket, WebSocketDisconnect, HTTPException
from pydantic import BaseModel

import httpx
from Api.services.discord_remote_auth import DiscordRemoteAuthSession
from stream.helpers.logger import LOGGER

router = APIRouter(tags=["discord"])

# In-memory store for active sessions (auto-cleaned after TTL)
_sessions: Dict[str, DiscordRemoteAuthSession] = {}
_session_tasks: Dict[str, asyncio.Task] = {}
SESSION_TTL_SEC = 240


def _cleanup_old_sessions() -> None:
    now = time.time()
    stale = [sid for sid, s in _sessions.items() if now - s.created_at > SESSION_TTL_SEC]
    for sid in stale:
        session = _sessions.pop(sid, None)
        if session:
            session.cancel()
        task = _session_tasks.pop(sid, None)
        if task and not task.done():
            task.cancel()


@router.websocket("/ws/discord/remote-auth")
async def discord_remote_auth_ws(websocket: WebSocket) -> None:
    """Stream Discord QR remote-auth events directly over WebSocket."""
    await websocket.accept()
    session_id = str(uuid.uuid4())
    session = DiscordRemoteAuthSession(session_id)
    _sessions[session_id] = session
    _cleanup_old_sessions()

    LOGGER(__name__).info(f"[DiscordRA WS] New connection established: {session_id}")

    try:
        async for event in session.stream_events():
            await websocket.send_json(event)
            if event.get("type") in ("success", "error", "cancelled"):
                break
    except WebSocketDisconnect:
        LOGGER(__name__).info(f"[DiscordRA WS] Client disconnected early: {session_id}")
    except Exception as exc:
        LOGGER(__name__).error(f"[DiscordRA WS] Unexpected error for {session_id}: {exc}")
    finally:
        session.cancel()
        _sessions.pop(session_id, None)


class StartRemoteAuthResponse(BaseModel):
    session_id: str
    status: str
    url: Optional[str] = None
    fingerprint: Optional[str] = None


class RemoteAuthStatusResponse(BaseModel):
    session_id: str
    status: str
    url: Optional[str] = None
    fingerprint: Optional[str] = None
    user: Optional[dict] = None
    token: Optional[str] = None
    error: Optional[str] = None


async def _run_session_background(session: DiscordRemoteAuthSession) -> None:
    try:
        async for _ in session.stream_events():
            if session.status in ("success", "error", "cancelled"):
                break
    except Exception as exc:
        session.error = str(exc)
        session.status = "error"


@router.post("/api/discord/remote-auth/start", response_model=StartRemoteAuthResponse)
async def start_remote_auth() -> StartRemoteAuthResponse:
    """Initialize a Discord Remote Auth session and wait up to 5s for the initial QR code."""
    _cleanup_old_sessions()
    session_id = str(uuid.uuid4())
    session = DiscordRemoteAuthSession(session_id)
    _sessions[session_id] = session

    # Run the stream in background
    task = asyncio.create_task(_run_session_background(session))
    _session_tasks[session_id] = task

    # Wait for the fingerprint to appear (typically within 1-2 seconds)
    for _ in range(50):
        if session.fingerprint:
            break
        if session.status in ("error", "cancelled"):
            break
        await asyncio.sleep(0.1)

    if session.error:
        raise HTTPException(status_code=502, detail=session.error)

    return StartRemoteAuthResponse(
        session_id=session_id,
        status=session.status,
        url=session.qr_url,
        fingerprint=session.fingerprint,
    )


@router.get("/api/discord/remote-auth/status/{session_id}", response_model=RemoteAuthStatusResponse)
async def get_remote_auth_status(session_id: str) -> RemoteAuthStatusResponse:
    """Check the status of a running Remote Auth session."""
    session = _sessions.get(session_id)
    if not session:
        raise HTTPException(status_code=404, detail="Session not found or expired")

    return RemoteAuthStatusResponse(
        session_id=session.session_id,
        status=session.status,
        url=session.qr_url,
        fingerprint=session.fingerprint,
        user=session.user_info,
        token=session.token,
        error=session.error,
    )


@router.post("/api/discord/remote-auth/cancel/{session_id}")
async def cancel_remote_auth(session_id: str) -> dict:
    """Cancel an active Remote Auth session."""
    session = _sessions.pop(session_id, None)
    if session:
        session.cancel()
    task = _session_tasks.pop(session_id, None)
    if task and not task.done():
        task.cancel()
    return {"status": "cancelled", "session_id": session_id}


class ExternalAssetsRequest(BaseModel):
    urls: list[str]
    application_id: Optional[str] = "1547543416143876167"
    token: str


@router.post("/api/discord/external-assets")
async def resolve_external_assets(req: ExternalAssetsRequest) -> list[dict]:
    """Proxy external assets resolution to Discord's API with user's token.
    Returns list of {'url': ..., 'external_asset_path': ...}.
    """
    app_id = req.application_id or "1547543416143876167"
    token = req.token.strip()
    if not token or not req.urls:
        return []

    discord_url = f"https://discord.com/api/v9/applications/{app_id}/external-assets"
    headers = {
        "Authorization": token,
        "Content-Type": "application/json",
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
    }
    try:
        async with httpx.AsyncClient(timeout=8.0) as client:
            resp = await client.post(discord_url, headers=headers, json={"urls": req.urls})
            if 200 <= resp.status_code < 300:
                data = resp.json()
                if isinstance(data, list):
                    return data
            LOGGER(__name__).warning(
                f"[Discord external-assets] Discord API returned {resp.status_code}: {resp.text[:200]}"
            )
    except Exception as exc:
        LOGGER(__name__).error(f"[Discord external-assets] Error proxying: {exc}")
    return []

