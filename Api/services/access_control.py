"""
Access control — registration policy, account locking, session revocation and
Telegram chat-membership gating.

Storage
  auth_config/_id="access_policy"  {registration_mode, enforce_membership, required_chats[], lock_message}
  users.{status, token_version, lock_reason, locked_at, locked_by}
  invite_codes                     {_id: code, created_by, created_at, max_uses, uses, expires_at, used_by[], note}
  registration_allowlist           {_id: user_id, added_by, added_at, note}

Fast path
  Every authenticated request consults ``get_user_access`` which is memoised for 30 s per user,
  so locking a user / bumping token_version cuts every device off within seconds without a DB
  round-trip per request.
"""
from __future__ import annotations

import asyncio
import secrets
import string
import time
from typing import Any, Literal, Optional

from stream.database.MongoDb import db_handler
from stream.helpers.logger import LOGGER

LOG = LOGGER(__name__)

RegistrationMode = Literal["open", "invite", "allowlist", "closed"]
UserStatus = Literal["active", "locked", "restricted"]
REGISTRATION_MODES: tuple[str, ...] = ("open", "invite", "allowlist", "closed")

POLICY_ID = "access_policy"
_POLICY_TTL = 15.0
_ACCESS_TTL = 30.0
_MEMBERSHIP_TTL = 10 * 60.0

_policy_cache: dict[str, Any] = {"value": None, "expires": 0.0}
_access_cache: dict[int, tuple[float, dict[str, Any]]] = {}
_membership_cache: dict[int, tuple[float, bool, list[dict[str, Any]]]] = {}
_lock = asyncio.Lock()


def _col(name: str):
    return db_handler.get_collection(name).collection


def _now() -> float:
    return time.time()


# --------------------------------------------------------------------------- policy


def default_policy() -> dict[str, Any]:
    return {
        "registration_mode": "open",
        "enforce_membership": False,
        "required_chats": [],  # [{chat_id:int, title:str, invite_link:str|None}]
        "lock_message": "Your account has been locked. Contact an administrator if you think this is a mistake.",
        "updated_at": 0,
    }


async def get_policy(force: bool = False) -> dict[str, Any]:
    now = _now()
    if not force and _policy_cache["value"] is not None and _policy_cache["expires"] > now:
        return _policy_cache["value"]
    doc = None
    try:
        doc = await _col("auth_config").find_one({"_id": POLICY_ID})
    except Exception as exc:
        LOG.warning(f"[access] policy read failed: {exc}")
    policy = {**default_policy(), **{k: v for k, v in (doc or {}).items() if k != "_id"}}
    if policy.get("registration_mode") not in REGISTRATION_MODES:
        policy["registration_mode"] = "open"
    _policy_cache["value"] = policy
    _policy_cache["expires"] = now + _POLICY_TTL
    return policy


async def update_policy(patch: dict[str, Any], by: int | None = None) -> dict[str, Any]:
    clean: dict[str, Any] = {}
    if "registration_mode" in patch:
        mode = str(patch["registration_mode"] or "").lower()
        if mode not in REGISTRATION_MODES:
            raise ValueError("registration_mode must be one of open, invite, allowlist, closed")
        clean["registration_mode"] = mode
    if "enforce_membership" in patch:
        clean["enforce_membership"] = bool(patch["enforce_membership"])
    if "lock_message" in patch and patch["lock_message"] is not None:
        clean["lock_message"] = str(patch["lock_message"])[:400]
    if "required_chats" in patch and patch["required_chats"] is not None:
        chats = []
        for c in patch["required_chats"]:
            try:
                cid = int(c["chat_id"]) if isinstance(c, dict) else int(c)
            except Exception:
                continue
            is_priv = bool(c.get("is_private")) if isinstance(c, dict) else False
            chats.append(
                {
                    "chat_id": cid,
                    "title": (str(c.get("title")) if isinstance(c, dict) and c.get("title") else None),
                    "invite_link": None if is_priv else (str(c.get("invite_link")) if isinstance(c, dict) and c.get("invite_link") else None),
                    "is_private": is_priv,
                }
            )
        clean["required_chats"] = chats
    clean["updated_at"] = _now()
    if by:
        clean["updated_by"] = int(by)
    await _col("auth_config").update_one({"_id": POLICY_ID}, {"$set": clean}, upsert=True)
    _policy_cache["expires"] = 0.0
    _membership_cache.clear()
    return await get_policy(force=True)


def public_policy(policy: dict[str, Any]) -> dict[str, Any]:
    """What the signup/login screens are allowed to know."""
    return {
        "registration_mode": policy["registration_mode"],
        "enforce_membership": bool(policy.get("enforce_membership")),
        "required_chats": [
            {
                "title": c.get("title") or str(c["chat_id"]),
                "invite_link": None if c.get("is_private") else c.get("invite_link"),
                "is_private": bool(c.get("is_private")),
            }
            for c in policy.get("required_chats", [])
        ],
    }


# --------------------------------------------------------------------------- required chats


async def add_required_chat(
    chat_id: int,
    title: str | None = None,
    invite_link: str | None = None,
    is_private: bool = False,
) -> dict[str, Any]:
    policy = await get_policy(force=True)
    chats = [c for c in policy.get("required_chats", []) if int(c["chat_id"]) != int(chat_id)]
    if not is_private:
        if not title or not invite_link:
            fetched = await describe_chat(chat_id)
            title = title or fetched.get("title")
            invite_link = invite_link or fetched.get("invite_link")
    else:
        if not title:
            fetched = await describe_chat(chat_id)
            title = fetched.get("title")
        invite_link = None
    chats.append({
        "chat_id": int(chat_id),
        "title": title,
        "invite_link": invite_link if not is_private else None,
        "is_private": bool(is_private),
    })
    return await update_policy({"required_chats": chats})


async def remove_required_chat(chat_id: int) -> dict[str, Any]:
    policy = await get_policy(force=True)
    chats = [c for c in policy.get("required_chats", []) if int(c["chat_id"]) != int(chat_id)]
    return await update_policy({"required_chats": chats})


async def describe_chat(chat_id: int) -> dict[str, Any]:
    """Best-effort title + invite link via the bot (needs the bot inside the chat)."""
    out: dict[str, Any] = {"chat_id": int(chat_id), "title": None, "invite_link": None}
    try:
        from stream import bot

        if not bot:
            return out
        chat = await bot.get_chat(int(chat_id))
        out["title"] = getattr(chat, "title", None) or getattr(chat, "first_name", None)
        username = getattr(chat, "username", None)
        if username:
            out["invite_link"] = f"https://t.me/{username}"
        else:
            out["invite_link"] = getattr(chat, "invite_link", None)
            if not out["invite_link"]:
                try:
                    out["invite_link"] = await bot.export_chat_invite_link(int(chat_id))
                except Exception:
                    pass
    except Exception as exc:
        LOG.debug(f"[access] describe_chat({chat_id}) failed: {exc}")
    return out


# --------------------------------------------------------------------------- membership


_NOT_MEMBER = {"left", "kicked", "banned", "restricted_left"}


def _status_name(member) -> str:
    st = getattr(member, "status", None)
    name = getattr(st, "name", None) or str(st or "")
    return name.lower()


def is_admin_id(user_id: int) -> bool:
    try:
        from stream.core.config_manager import Config
        owner = getattr(Config, "OWNER_ID", 0)
        sudos = getattr(Config, "SUDO_USERS", []) or []
        uid = int(user_id)
        if owner and uid == int(owner):
            return True
        for s in sudos:
            try:
                if uid == int(s):
                    return True
            except Exception:
                pass
        return False
    except Exception:
        return False


async def is_membership_bypassed(user_id: int) -> bool:
    """True if user is admin/owner or has been granted an individual membership bypass."""
    if is_admin_id(user_id):
        return True
    doc = await _col("membership_bypass").find_one({"_id": int(user_id)})
    return doc is not None


async def bypass_add(user_id: int, by: int | None = None, note: str | None = None) -> None:
    """Exempt a user from mandatory channel membership checks."""
    await _col("membership_bypass").update_one(
        {"_id": int(user_id)},
        {"$set": {"added_by": by, "added_at": _now(), "note": note}},
        upsert=True,
    )
    invalidate_membership(user_id)
    try:
        user = await _col("users").find_one({"_id": int(user_id)}, {"status": 1})
        if user and user.get("status") == "restricted":
            await set_user_status(user_id, "active")
    except Exception:
        pass


async def bypass_remove(user_id: int) -> bool:
    """Revoke a user's channel membership exemption."""
    res = await _col("membership_bypass").delete_one({"_id": int(user_id)})
    invalidate_membership(user_id)
    return res.deleted_count > 0


async def bypass_list() -> list[dict[str, Any]]:
    """List all users exempt from channel membership."""
    out = []
    async for d in _col("membership_bypass").find({}).sort("added_at", -1).limit(500):
        out.append({
            "user_id": int(d["_id"]),
            "added_by": d.get("added_by"),
            "added_at": d.get("added_at"),
            "note": d.get("note"),
        })
    return out


async def check_membership(user_id: int, force: bool = False) -> tuple[bool, list[dict[str, Any]]]:
    """(ok, missing_chats). Cached 10 min per user; membership events invalidate it."""
    policy = await get_policy()
    chats = policy.get("required_chats", [])
    if not policy.get("enforce_membership") or not chats:
        return True, []
    if await is_membership_bypassed(user_id):
        return True, []
    now = _now()
    cached = _membership_cache.get(int(user_id))
    if cached and not force and cached[0] > now:
        return cached[1], cached[2]
    missing: list[dict[str, Any]] = []
    try:
        from stream import bot
    except Exception:
        bot = None
    if not bot:
        # can't verify → fail open but log; admins should keep the bot running
        LOG.warning("[access] membership check skipped: bot not running")
        return True, []
    for c in chats:
        try:
            member = await bot.get_chat_member(int(c["chat_id"]), int(user_id))
            if _status_name(member) in _NOT_MEMBER:
                missing.append(c)
        except Exception as exc:
            # USER_NOT_PARTICIPANT etc. → not a member; other errors also count as missing
            LOG.debug(f"[access] get_chat_member({c['chat_id']}, {user_id}): {exc}")
            missing.append(c)
    ok = not missing
    _membership_cache[int(user_id)] = (now + _MEMBERSHIP_TTL, ok, missing)
    return ok, missing


def invalidate_membership(user_id: int) -> None:
    _membership_cache.pop(int(user_id), None)


def public_chats(chats: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return [
        {
            "title": c.get("title") or "Required chat",
            "invite_link": None if c.get("is_private") else c.get("invite_link"),
            "is_private": bool(c.get("is_private")),
        }
        for c in chats
    ]


# --------------------------------------------------------------------------- user status


async def get_user_access(user_id: int, force: bool = False) -> dict[str, Any]:
    """{status, token_version, lock_reason} — memoised for 30 s."""
    uid = int(user_id)
    now = _now()
    hit = _access_cache.get(uid)
    if hit and not force and hit[0] > now:
        return hit[1]
    doc = None
    try:
        doc = await _col("users").find_one({"_id": uid}, {"status": 1, "token_version": 1, "lock_reason": 1})
    except Exception as exc:
        LOG.warning(f"[access] user read failed: {exc}")
    info = {
        "status": (doc or {}).get("status") or "active",
        "token_version": int((doc or {}).get("token_version") or 0),
        "lock_reason": (doc or {}).get("lock_reason"),
    }
    _access_cache[uid] = (now + _ACCESS_TTL, info)
    return info


def invalidate_access(user_id: int | None = None) -> None:
    if user_id is None:
        _access_cache.clear()
    else:
        _access_cache.pop(int(user_id), None)


async def set_user_status(user_id: int, status: UserStatus, reason: str | None = None, by: int | None = None, revoke_sessions: bool = True) -> dict[str, Any]:
    uid = int(user_id)
    update: dict[str, Any] = {"status": status, "updated_at": _now()}
    if status == "active":
        update["lock_reason"] = None
        update["locked_at"] = None
        update["locked_by"] = None
    else:
        update["lock_reason"] = (reason or ("Left a required Telegram chat" if status == "restricted" else "Locked by an administrator"))[:300]
        update["locked_at"] = _now()
        update["locked_by"] = int(by) if by else None
    ops: dict[str, Any] = {"$set": update}
    if revoke_sessions and status != "active":
        ops["$inc"] = {"token_version": 1}
    await _col("users").update_one({"_id": uid}, ops, upsert=True)
    invalidate_access(uid)
    invalidate_membership(uid)
    return await get_user_access(uid, force=True)


async def revoke_sessions(user_id: int) -> int:
    """Bump token_version → every existing token for the user becomes invalid."""
    uid = int(user_id)
    await _col("users").update_one({"_id": uid}, {"$inc": {"token_version": 1}, "$set": {"updated_at": _now()}}, upsert=True)
    invalidate_access(uid)
    return (await get_user_access(uid, force=True))["token_version"]


class AccessDenied(Exception):
    def __init__(self, code: str, message: str, extra: dict[str, Any] | None = None, status_code: int = 403):
        super().__init__(message)
        self.code = code
        self.message = message
        self.extra = extra or {}
        self.status_code = status_code

    def payload(self) -> dict[str, Any]:
        return {"ok": False, "detail": self.code, "message": self.message, **self.extra}


async def assert_can_use(user_id: int, token_version: int | None) -> None:
    """Raise AccessDenied if the account is locked/restricted or the token was revoked.

    Status is checked first so a locked user sees *why* (403 + reason) instead of a bare sign-out;
    the token_version check only applies to accounts that are otherwise active.
    """
    info = await get_user_access(user_id)
    status = info["status"]
    if status == "locked":
        policy = await get_policy()
        raise AccessDenied("account_locked", info.get("lock_reason") or policy["lock_message"], {"reason": info.get("lock_reason")})
    if status == "restricted":
        policy = await get_policy()
        raise AccessDenied(
            "membership_required",
            info.get("lock_reason") or "Join the required Telegram chat to continue.",
            {"reason": info.get("lock_reason"), "required_chats": public_chats(policy.get("required_chats", []))},
        )
    if token_version is not None and int(token_version) < int(info["token_version"]):
        raise AccessDenied("session_revoked", "Your session was signed out. Please sign in again.", status_code=401)
    if token_version is None and int(info["token_version"]) > 0:
        # legacy token minted before versioning on an account that has since been revoked/locked
        raise AccessDenied("session_revoked", "Your session was signed out. Please sign in again.", status_code=401)


async def assert_can_login(user_id: int) -> None:
    """Login-time gate: status + live membership check (restores a restricted user who rejoined)."""
    info = await get_user_access(user_id, force=True)
    if info["status"] == "locked":
        policy = await get_policy()
        raise AccessDenied("account_locked", info.get("lock_reason") or policy["lock_message"], {"reason": info.get("lock_reason")})
    ok, missing = await check_membership(user_id, force=True)
    if not ok:
        if info["status"] != "restricted":
            await set_user_status(user_id, "restricted", "Left a required Telegram chat", revoke_sessions=False)
        raise AccessDenied("membership_required", "Join the required Telegram chat to continue.", {"required_chats": public_chats(missing)})
    if info["status"] == "restricted":
        await set_user_status(user_id, "active")


async def reverify_user(user_id: int) -> dict[str, Any]:
    """User-triggered “I've joined — verify”. Lifts a membership restriction if satisfied."""
    ok, missing = await check_membership(user_id, force=True)
    info = await get_user_access(user_id, force=True)
    if ok and info["status"] == "restricted":
        info = await set_user_status(user_id, "active")
    elif not ok and info["status"] == "active":
        info = await set_user_status(user_id, "restricted", "Left a required Telegram chat", revoke_sessions=False)
    return {"ok": ok, "status": info["status"], "required_chats": public_chats(missing)}


# --------------------------------------------------------------------------- registration gate


async def assert_can_register(user_id: int, invite_code: str | None = None) -> dict[str, Any]:
    """Validate the registration policy. Returns context (e.g. consumed invite) for the caller."""
    policy = await get_policy()
    mode = policy["registration_mode"]
    ctx: dict[str, Any] = {"mode": mode}
    if mode == "closed":
        raise AccessDenied("registration_closed", "Registrations are currently closed.")
    if mode == "allowlist":
        allowed = await _col("registration_allowlist").find_one({"_id": int(user_id)})
        if not allowed:
            raise AccessDenied("not_allowlisted", "This Telegram account is not on the allow list.")
    if mode == "invite":
        code = (invite_code or "").strip().upper()
        if not code:
            raise AccessDenied("invite_required", "An invite code is required to register.", status_code=400)
        inv = await _col("invite_codes").find_one({"_id": code})
        if not inv:
            raise AccessDenied("invite_invalid", "That invite code is not valid.", status_code=400)
        if inv.get("revoked_at"):
            raise AccessDenied("invite_invalid", "That invite code was revoked.", status_code=400)
        if inv.get("expires_at") and float(inv["expires_at"]) < _now():
            raise AccessDenied("invite_expired", "That invite code has expired.", status_code=400)
        if int(inv.get("max_uses") or 0) and int(inv.get("uses") or 0) >= int(inv["max_uses"]):
            raise AccessDenied("invite_exhausted", "That invite code has already been used.", status_code=400)
        ctx["invite_code"] = code
    ok, missing = await check_membership(user_id, force=True)
    if not ok:
        raise AccessDenied("membership_required", "Join the required Telegram chat before registering.", {"required_chats": public_chats(missing)})
    return ctx


async def consume_invite(code: str, user_id: int) -> None:
    """Atomic: only succeeds while uses < max_uses (or unlimited)."""
    c = (code or "").strip().upper()
    if not c:
        return
    res = await _col("invite_codes").update_one(
        {"_id": c, "revoked_at": None, "$or": [{"max_uses": 0}, {"max_uses": None}, {"$expr": {"$lt": ["$uses", "$max_uses"]}}]},
        {"$inc": {"uses": 1}, "$addToSet": {"used_by": int(user_id)}, "$set": {"last_used_at": _now()}},
    )
    if res.matched_count == 0:
        raise AccessDenied("invite_exhausted", "That invite code has already been used.", status_code=400)


# --------------------------------------------------------------------------- invites & allowlist


def _gen_code(n: int = 8) -> str:
    alphabet = string.ascii_uppercase + string.digits
    alphabet = alphabet.replace("O", "").replace("0", "").replace("I", "").replace("1", "")
    return "".join(secrets.choice(alphabet) for _ in range(n))


async def create_invite(by: int, max_uses: int = 1, ttl_days: int | None = 7, note: str | None = None) -> dict[str, Any]:
    code = _gen_code()
    doc = {
        "_id": code,
        "created_by": int(by),
        "created_at": _now(),
        "max_uses": max(0, int(max_uses)),
        "uses": 0,
        "used_by": [],
        "expires_at": (_now() + int(ttl_days) * 86400) if ttl_days else None,
        "revoked_at": None,
        "note": (note or "")[:120] or None,
    }
    await _col("invite_codes").insert_one(doc)
    return _invite_public(doc)


def _invite_public(d: dict[str, Any]) -> dict[str, Any]:
    return {
        "code": d["_id"],
        "created_by": d.get("created_by"),
        "created_at": d.get("created_at"),
        "max_uses": d.get("max_uses") or 0,
        "uses": d.get("uses") or 0,
        "used_by": d.get("used_by") or [],
        "expires_at": d.get("expires_at"),
        "revoked_at": d.get("revoked_at"),
        "note": d.get("note"),
    }


async def list_invites(include_dead: bool = False) -> list[dict[str, Any]]:
    out = []
    async for d in _col("invite_codes").find({}).sort("created_at", -1).limit(200):
        pub = _invite_public(d)
        dead = bool(pub["revoked_at"]) or (pub["expires_at"] and pub["expires_at"] < _now()) or (pub["max_uses"] and pub["uses"] >= pub["max_uses"])
        if include_dead or not dead:
            out.append(pub)
    return out


async def revoke_invite(code: str) -> bool:
    res = await _col("invite_codes").update_one({"_id": (code or "").strip().upper(), "revoked_at": None}, {"$set": {"revoked_at": _now()}})
    return res.modified_count > 0


async def allowlist_add(user_id: int, by: int | None = None, note: str | None = None) -> None:
    await _col("registration_allowlist").update_one({"_id": int(user_id)}, {"$set": {"added_by": by, "added_at": _now(), "note": note}}, upsert=True)


async def allowlist_remove(user_id: int) -> bool:
    res = await _col("registration_allowlist").delete_one({"_id": int(user_id)})
    return res.deleted_count > 0


async def allowlist_list() -> list[dict[str, Any]]:
    out = []
    async for d in _col("registration_allowlist").find({}).sort("added_at", -1).limit(500):
        out.append({"user_id": int(d["_id"]), "added_by": d.get("added_by"), "added_at": d.get("added_at"), "note": d.get("note")})
    return out


# --------------------------------------------------------------------------- users (admin)


async def list_users(query: str | None = None, status: str | None = None, limit: int = 100, skip: int = 0) -> dict[str, Any]:
    q: dict[str, Any] = {"password": {"$exists": True}}
    if status in ("active", "locked", "restricted"):
        q["status"] = status if status != "active" else {"$in": ["active", None]}
    if query:
        qs = str(query).strip()
        ors: list[dict[str, Any]] = [{"username": {"$regex": qs, "$options": "i"}}, {"first_name": {"$regex": qs, "$options": "i"}}, {"telegram.username": {"$regex": qs, "$options": "i"}}]
        if qs.isdigit():
            ors.append({"_id": int(qs)})
        q["$or"] = ors
    col = _col("users")
    total = await col.count_documents(q)
    items = []
    cursor = col.find(q, {"username": 1, "first_name": 1, "profile_url": 1, "photo_url": 1, "telegram": 1, "status": 1, "lock_reason": 1, "locked_at": 1, "token_version": 1, "created_at": 1, "updated_at": 1}).sort("created_at", -1).skip(int(skip)).limit(int(limit))
    async for d in cursor:
        items.append(
            {
                "user_id": int(d["_id"]),
                "username": d.get("username"),
                "first_name": d.get("first_name"),
                "profile_url": d.get("profile_url") or d.get("photo_url"),
                "telegram_username": (d.get("telegram") or {}).get("username") if isinstance(d.get("telegram"), dict) else None,
                "status": d.get("status") or "active",
                "lock_reason": d.get("lock_reason"),
                "locked_at": d.get("locked_at"),
                "token_version": int(d.get("token_version") or 0),
                "created_at": d.get("created_at"),
            }
        )
    return {"total": total, "items": items}


async def reverify_all(limit: int = 2000) -> dict[str, int]:
    """Admin sweep: re-check membership for every registered user (rate-friendly, sequential)."""
    policy = await get_policy(force=True)
    if not policy.get("enforce_membership") or not policy.get("required_chats"):
        return {"checked": 0, "restricted": 0, "restored": 0}
    checked = restricted = restored = 0
    async for d in _col("users").find({"password": {"$exists": True}}, {"_id": 1, "status": 1}).limit(limit):
        uid = int(d["_id"])
        checked += 1
        ok, _missing = await check_membership(uid, force=True)
        status = d.get("status") or "active"
        if not ok and status == "active":
            await set_user_status(uid, "restricted", "Left a required Telegram chat", revoke_sessions=False)
            restricted += 1
        elif ok and status == "restricted":
            await set_user_status(uid, "active")
            restored += 1
        await asyncio.sleep(0.05)
    return {"checked": checked, "restricted": restricted, "restored": restored}
