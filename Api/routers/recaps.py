"""Recaps — listening events, period snapshots and public share links."""
from __future__ import annotations

from typing import Any, Literal, Optional

from fastapi import APIRouter, Depends, HTTPException, Query
from pydantic import BaseModel, Field

from Api.services import recap_service as svc
from Api.utils.auth import require_user_id

router = APIRouter(tags=["recaps"])

PeriodType = Literal["weekly", "monthly", "yearly"]


class ListeningEventIn(BaseModel):
    id: str = Field(min_length=1, max_length=64)
    track_id: str = Field(min_length=1, max_length=128)
    played_at: float
    started_at: Optional[float] = None
    played_ms: int = Field(ge=0)
    duration_ms: int = Field(default=0, ge=0)
    completed: bool = False
    skipped: bool = False
    source: str = "server"
    session_id: Optional[str] = None


class ListeningEventsBatch(BaseModel):
    events: list[ListeningEventIn] = Field(max_length=200)


@router.post("/me/listening-events")
async def post_listening_events(payload: ListeningEventsBatch, user_id: int = Depends(require_user_id)):
    stored = await svc.record_events(user_id, [e.model_dump() for e in payload.events])
    return {"ok": True, "received": len(payload.events), "stored": stored}


@router.get("/me/recaps")
async def list_recaps(tz: int = Query(default=0, ge=-840, le=840), user_id: int = Depends(require_user_id)):
    return {"items": await svc.list_available(user_id, tz)}


@router.get("/me/recaps/shares")
async def my_shares(user_id: int = Depends(require_user_id)):
    return {"items": await svc.list_shares(user_id)}


@router.delete("/me/recaps/shares/{token}")
async def revoke(token: str, user_id: int = Depends(require_user_id)):
    ok = await svc.revoke_share(user_id, token)
    if not ok:
        raise HTTPException(status_code=404, detail="Share link not found")
    return {"ok": True}


@router.delete("/me/recaps/data")
async def delete_data(user_id: int = Depends(require_user_id)):
    return {"ok": True, **(await svc.delete_user_recap_data(user_id))}


@router.get("/me/recaps/{ptype}/{period}")
async def get_recap(
    ptype: PeriodType,
    period: str,
    tz: int = Query(default=0, ge=-840, le=840),
    refresh: bool = Query(default=False),
    user_id: int = Depends(require_user_id),
):
    if not svc.is_period_available(ptype, period, tz):
        raise HTTPException(
            status_code=400,
            detail="This recap is not ready yet. Weekly recaps unlock on the weekend (Sunday), monthly on the last day of the month, and yearly on Dec 31st.",
        )
    try:
        return await svc.get_snapshot(user_id, ptype, period, tz, force=refresh)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc


@router.post("/me/recaps/{ptype}/{period}/share")
async def share_recap(ptype: PeriodType, period: str, tz: int = Query(default=0, ge=-840, le=840), user_id: int = Depends(require_user_id)):
    if not svc.is_period_available(ptype, period, tz):
        raise HTTPException(status_code=400, detail="This recap is not ready to share yet.")
    try:
        snap = await svc.get_snapshot(user_id, ptype, period, tz)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    if snap["stats"]["totalPlays"] == 0:
        raise HTTPException(status_code=400, detail="Nothing to share for this period yet")
    return await svc.create_share(user_id, snap)


@router.get("/recaps/share/{token}")
async def public_share(token: str) -> dict[str, Any]:
    summary = await svc.get_public_share(token)
    if not summary:
        raise HTTPException(status_code=404, detail="This recap link is no longer available")
    return summary
