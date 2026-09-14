"""Admin access control — registration policy, invites, allow list, user locking, membership sweeps."""
from __future__ import annotations

from typing import Any, Literal, Optional

from fastapi import APIRouter, Depends, HTTPException, Query
from pydantic import BaseModel, Field

from Api.services import access_control as access
from Api.utils.auth import require_admin_user_id

router = APIRouter(prefix="/admin/access", tags=["access"])


class RequiredChatIn(BaseModel):
    chat_id: int
    title: Optional[str] = None
    invite_link: Optional[str] = None
    is_private: bool = False


class PolicyPatch(BaseModel):
    registration_mode: Optional[Literal["open", "invite", "allowlist", "closed"]] = None
    enforce_membership: Optional[bool] = None
    lock_message: Optional[str] = Field(default=None, max_length=400)
    required_chats: Optional[list[RequiredChatIn]] = None


class InviteCreate(BaseModel):
    max_uses: int = Field(default=1, ge=0, le=10000)
    ttl_days: Optional[int] = Field(default=7, ge=0, le=3650)
    note: Optional[str] = Field(default=None, max_length=120)


class LockBody(BaseModel):
    reason: Optional[str] = Field(default=None, max_length=300)
    revoke_sessions: bool = True


class AllowlistBody(BaseModel):
    user_id: int
    note: Optional[str] = Field(default=None, max_length=120)


class BypassBody(BaseModel):
    user_id: int
    note: Optional[str] = Field(default=None, max_length=120)


@router.get("/policy")
async def get_policy(_: int = Depends(require_admin_user_id)):
    return {"ok": True, "policy": await access.get_policy(force=True)}


@router.patch("/policy")
async def patch_policy(body: PolicyPatch, admin: int = Depends(require_admin_user_id)):
    patch: dict[str, Any] = {k: v for k, v in body.model_dump().items() if v is not None}
    try:
        policy = await access.update_policy(patch, by=admin)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc))
    return {"ok": True, "policy": policy}


@router.post("/required-chats")
async def add_required_chat(body: RequiredChatIn, _: int = Depends(require_admin_user_id)):
    policy = await access.add_required_chat(body.chat_id, body.title, body.invite_link, is_private=body.is_private)
    return {"ok": True, "policy": policy}


@router.delete("/required-chats/{chat_id}")
async def remove_required_chat(chat_id: int, _: int = Depends(require_admin_user_id)):
    return {"ok": True, "policy": await access.remove_required_chat(chat_id)}


@router.get("/invites")
async def invites(include_dead: bool = Query(default=False), _: int = Depends(require_admin_user_id)):
    return {"ok": True, "items": await access.list_invites(include_dead)}


@router.post("/invites")
async def create_invite(body: InviteCreate, admin: int = Depends(require_admin_user_id)):
    return {"ok": True, "invite": await access.create_invite(admin, body.max_uses, body.ttl_days or None, body.note)}


@router.delete("/invites/{code}")
async def revoke_invite(code: str, _: int = Depends(require_admin_user_id)):
    if not await access.revoke_invite(code):
        raise HTTPException(status_code=404, detail="invite not found")
    return {"ok": True}


@router.get("/allowlist")
async def allowlist(_: int = Depends(require_admin_user_id)):
    return {"ok": True, "items": await access.allowlist_list()}


@router.post("/allowlist")
async def allowlist_add(body: AllowlistBody, admin: int = Depends(require_admin_user_id)):
    await access.allowlist_add(body.user_id, admin, body.note)
    return {"ok": True}


@router.delete("/allowlist/{user_id}")
async def allowlist_remove(user_id: int, _: int = Depends(require_admin_user_id)):
    return {"ok": await access.allowlist_remove(user_id)}


@router.get("/bypass")
async def bypass_list(_: int = Depends(require_admin_user_id)):
    return {"ok": True, "items": await access.bypass_list()}


@router.post("/bypass")
async def bypass_add(body: BypassBody, admin: int = Depends(require_admin_user_id)):
    await access.bypass_add(body.user_id, admin, body.note)
    return {"ok": True}


@router.delete("/bypass/{user_id}")
async def bypass_remove(user_id: int, _: int = Depends(require_admin_user_id)):
    return {"ok": await access.bypass_remove(user_id)}


@router.get("/users")
async def users(
    q: Optional[str] = Query(default=None, max_length=64),
    status: Optional[Literal["active", "locked", "restricted"]] = None,
    limit: int = Query(default=50, ge=1, le=200),
    skip: int = Query(default=0, ge=0),
    _: int = Depends(require_admin_user_id),
):
    return {"ok": True, **(await access.list_users(q, status, limit, skip))}


@router.post("/users/{user_id}/lock")
async def lock_user(user_id: int, body: LockBody, admin: int = Depends(require_admin_user_id)):
    if int(user_id) == int(admin):
        raise HTTPException(status_code=400, detail="You can't lock your own account")
    return {"ok": True, "access": await access.set_user_status(user_id, "locked", body.reason, by=admin, revoke_sessions=body.revoke_sessions)}


@router.post("/users/{user_id}/unlock")
async def unlock_user(user_id: int, _: int = Depends(require_admin_user_id)):
    return {"ok": True, "access": await access.set_user_status(user_id, "active")}


@router.post("/users/{user_id}/revoke-sessions")
async def revoke_sessions(user_id: int, _: int = Depends(require_admin_user_id)):
    return {"ok": True, "token_version": await access.revoke_sessions(user_id)}


@router.post("/reverify")
async def reverify(_: int = Depends(require_admin_user_id)):
    return {"ok": True, **(await access.reverify_all())}
