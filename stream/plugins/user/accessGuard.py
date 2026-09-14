"""
Access guard — live Telegram channel checks + admin commands for signups & access.

* on_chat_member_updated: when a user leaves / is kicked from a required channel,
  their access is restricted automatically; rejoining restores them automatically.
* Commands (owner / sudo only):
    /access                         show sign-up rules, required channels & user counts
    /registration <open|invite|allowlist|closed>  choose who gets to sign up
    /requirechat [chat_id]          add a channel or group that users must join
    /unrequirechat [chat_id]        remove a required channel
    /membership <on|off>            turn required channel checks on or off
    /bypass <user_id|on|off>        let someone skip channel checks (or turn off for everyone)
    /unbypass <user_id>             make someone follow channel rules again
    /bypasses                       see who gets to skip channel checks
    /invite [uses] [days] [note]    create an invite code
    /invites                        see invite codes and who used them
    /lock <user_id> [reason]        block a user and log them out everywhere
    /unlock <user_id>               unblock a user
    /revoke <user_id>               log a user out of all devices
    /allow <user_id> [note]         approve a user to sign up without a code
    /disallow <user_id>             remove someone from approved sign-up list
    /reverify                       check if everyone is still in the required channels
"""
from __future__ import annotations

import html

from pyrogram import enums, filters
from pyrogram.types import ChatMemberUpdated, Message

from stream import bot
from stream.core.config_manager import Config
from stream.helpers.logger import LOGGER

LOG = LOGGER(__name__)


def _admin_ids() -> list[int]:
    ids: list[int] = []
    for key in ("OWNER_ID", "SUDO_USERS"):
        raw = getattr(Config, key, None)
        vals = [raw] if isinstance(raw, (int, str)) else (raw or [])
        for v in vals:
            try:
                ids.append(int(v))
            except Exception:
                pass
    return ids


def _is_admin(_, __, m: Message) -> bool:
    return bool(m.from_user and m.from_user.id in _admin_ids())


admin_only = filters.create(_is_admin)


def _access():
    from Api.services import access_control

    return access_control


_LEFT = {"left", "banned", "kicked", "restricted"}


# --------------------------------------------------------------------------- live monitor


@bot.on_chat_member_updated()
async def on_member_change(_, update: ChatMemberUpdated):
    access = _access()
    try:
        policy = await access.get_policy()
    except Exception:
        return
    if not policy.get("enforce_membership"):
        return
    required = {int(c["chat_id"]) for c in policy.get("required_chats", [])}
    if not required or update.chat.id not in required:
        return

    new = update.new_chat_member
    old = update.old_chat_member
    user = (new.user if new else None) or (old.user if old else None) or update.from_user
    if not user or user.is_bot:
        return
    uid = int(user.id)

    def _st(member) -> str:
        st = getattr(member, "status", None)
        return (getattr(st, "name", None) or str(st or "")).lower()

    new_status = _st(new) if new else "left"
    gone = new_status in {"left", "banned", "kicked"} or (new_status == "restricted" and new is not None and getattr(new, "is_member", True) is False)

    access.invalidate_membership(uid)
    try:
        info = await access.get_user_access(uid, force=True)
        if gone and info["status"] == "active":
            ok, _missing = await access.check_membership(uid, force=True)
            if not ok:
                title = update.chat.title or str(update.chat.id)
                await access.set_user_status(uid, "restricted", f"Left required chat: {title}", revoke_sessions=False)
                LOG.info(f"[access] restricted {uid} (left {title})")
        elif not gone and info["status"] == "restricted":
            ok, _missing = await access.check_membership(uid, force=True)
            if ok:
                await access.set_user_status(uid, "active")
                LOG.info(f"[access] restored {uid} (rejoined)")
    except Exception as exc:
        LOG.warning(f"[access] membership update for {uid} failed: {exc}")


# --------------------------------------------------------------------------- admin commands


def _args(m: Message) -> list[str]:
    return (m.text or "").split()[1:]


@bot.on_message(filters.command("access") & admin_only)
async def cmd_access(_, m: Message):
    access = _access()
    p = await access.get_policy(force=True)
    chats = p.get("required_chats") or []
    users = await access.list_users(limit=1)
    locked = await access.list_users(status="locked", limit=1)
    restricted = await access.list_users(status="restricted", limit=1)
    bypasses = await access.bypass_list()
    channel_check = "on" if p.get("enforce_membership") else "off (skipped for everyone)"
    skipping_text = f" · <code>{len(bypasses)}</code> can skip" if bypasses else ""
    lines = [
        "<b>Access & Signups Overview</b>",
        f"• Who can sign up: <code>{p['registration_mode']}</code>",
        f"• Must join channels: <code>{channel_check}</code>{skipping_text}",
        "• Required channels: " + (", ".join(f"{html.escape(str(c.get('title') or c['chat_id']))} (<code>{c['chat_id']}</code>, {'private' if c.get('is_private') else 'public'})" for c in chats) if chats else "none"),
        f"• Users: {users['total']} total · {locked['total']} blocked · {restricted['total']} waiting to join channel",
    ]
    await m.reply_text(parse_mode=enums.ParseMode.HTML, text="\n".join(lines))


@bot.on_message(filters.command("registration") & admin_only)
async def cmd_registration(_, m: Message):
    args = _args(m)
    if not args or args[0].lower() not in ("open", "invite", "allowlist", "closed"):
        return await m.reply_text(parse_mode=enums.ParseMode.HTML, text="Usage: /registration <open|invite|allowlist|closed>")
    mode = args[0].lower()
    p = await _access().update_policy({"registration_mode": mode}, by=m.from_user.id)
    notes = {
        "open": "anyone can sign up",
        "invite": "requires an invite code",
        "allowlist": "only approved users can sign up",
        "closed": "no new signups",
    }
    await m.reply_text(parse_mode=enums.ParseMode.HTML, text=f"Sign-up mode is now <code>{mode}</code> ({notes.get(mode, '')}).")


@bot.on_message(filters.command("membership") & admin_only)
async def cmd_membership(_, m: Message):
    args = _args(m)
    if not args or args[0].lower() not in ("on", "off"):
        return await m.reply_text(parse_mode=enums.ParseMode.HTML, text="Usage: /membership <on|off>")
    is_on = args[0].lower() == "on"
    p = await _access().update_policy({"enforce_membership": is_on}, by=m.from_user.id)
    msg = "Channel check is now <b>on</b>. Users have to join your required channels to use the app." if is_on else "Channel check is now <b>off</b>. Anyone can use the app without joining channels."
    await m.reply_text(parse_mode=enums.ParseMode.HTML, text=msg)


@bot.on_message(filters.command("requirechat") & admin_only)
async def cmd_requirechat(_, m: Message):
    args = _args(m)
    chat_id: int | None = None
    visibility = "public"
    for a in args:
        al = a.strip().lower()
        if al in ("private", "priv", "secret", "hidden"):
            visibility = "private"
        elif al in ("public", "pub", "visible"):
            visibility = "public"
        elif al.lstrip("-").isdigit():
            chat_id = int(al)
        else:
            return await m.reply_text(
                parse_mode=enums.ParseMode.HTML,
                text=(
                    "Usage: <code>/requirechat [chat_id] [public|private]</code>\n\n"
                    "Examples:\n"
                    "• <code>/requirechat</code> (run in group, public)\n"
                    "• <code>/requirechat private</code> (run in group, hide link on WebX)\n"
                    "• <code>/requirechat -1001234567890 private</code>\n"
                    "• <code>/requirechat -1001234567890 public</code>"
                ),
            )
    if chat_id is None:
        chat_id = int(m.chat.id)
    if chat_id > 0:
        return await m.reply_text(parse_mode=enums.ParseMode.HTML, text="That looks like a user id — required chats must be groups or channels.")

    is_private = (visibility == "private")
    access = _access()
    p = await access.add_required_chat(chat_id, is_private=is_private)
    if not p.get("enforce_membership"):
        p = await access.update_policy({"enforce_membership": True}, by=m.from_user.id)
    c = next((c for c in p["required_chats"] if int(c["chat_id"]) == chat_id), None)
    title = html.escape(str((c or {}).get("title") or chat_id))

    if is_private:
        invite_info = "🔒 <b>Private / Secret Group</b>: The invite link will <b>never</b> be shown on WebX."
    else:
        link = (c or {}).get("invite_link")
        if link:
            invite_info = f"🌐 <b>Public Group</b>: Invite link shown on WebX: {link}"
        else:
            invite_info = (
                "⚠️ <b>Invite link unavailable</b>: Bot lacks admin rights to generate/export invite link.\n"
                "WebX will show <i>\"No invite link · Ask admin\"</i>."
            )

    await m.reply_text(
        parse_mode=enums.ParseMode.HTML,
        text=(
            f"Now required: <b>{title}</b> (<code>{chat_id}</code>)\n"
            f"Visibility: <code>{visibility}</code>\n\n"
            f"{invite_info}"
        ),
    )


@bot.on_message(filters.command("unrequirechat") & admin_only)
async def cmd_unrequirechat(_, m: Message):
    args = _args(m)
    try:
        chat_id = int(args[0]) if args else int(m.chat.id)
    except ValueError:
        return await m.reply_text(parse_mode=enums.ParseMode.HTML, text="Usage: /unrequirechat [chat_id]")
    p = await _access().remove_required_chat(chat_id)
    await m.reply_text(parse_mode=enums.ParseMode.HTML, text=f"Removed <code>{chat_id}</code>. Required chats: {len(p['required_chats'])}")


@bot.on_message(filters.command("invite") & admin_only)
async def cmd_invite(_, m: Message):
    args = _args(m)
    uses = int(args[0]) if len(args) > 0 and args[0].isdigit() else 1
    days = int(args[1]) if len(args) > 1 and args[1].isdigit() else 7
    note = " ".join(args[2:]) if len(args) > 2 else None
    inv = await _access().create_invite(m.from_user.id, uses, days or None, note)
    expiry = f"lasts {days} days" if days else "never expires"
    await m.reply_text(parse_mode=enums.ParseMode.HTML, text=f"Invite code: <code>{inv['code']}</code>\nUses: {inv['max_uses'] or 'unlimited'} · {expiry}")


@bot.on_message(filters.command("invites") & admin_only)
async def cmd_invites(_, m: Message):
    items = await _access().list_invites(include_dead=True)
    if not items:
        return await m.reply_text(parse_mode=enums.ParseMode.HTML, text="No invite codes yet. Make one with /invite [uses] [days].")

    lines = []
    now = _access()._now()
    for i in items[:30]:
        max_u = i['max_uses'] or 'unlimited'
        used_count = i['uses']
        status = ""
        if i.get("revoked_at"):
            status = " · <i>revoked</i>"
        elif i.get("expires_at") and i["expires_at"] < now:
            status = " · <i>expired</i>"
        elif i['max_uses'] and used_count >= i['max_uses']:
            used_by = i.get('used_by') or []
            if used_by:
                status = f" · <i>used by {', '.join(str(u) for u in used_by[:2])}</i>"
            else:
                status = " · <i>used</i>"

        note_str = f" · {html.escape(i['note'])}" if i.get("note") else ""
        lines.append(f"• <code>{i['code']}</code> · {used_count}/{max_u}{status}{note_str}")

    await m.reply_text(parse_mode=enums.ParseMode.HTML, text="<b>Invite codes</b>\n\n" + "\n".join(lines))


@bot.on_message(filters.command("lock") & admin_only)
async def cmd_lock(_, m: Message):
    args = _args(m)
    if not args or not args[0].lstrip("-").isdigit():
        return await m.reply_text(parse_mode=enums.ParseMode.HTML, text="Usage: /lock <user_id> [reason]")
    uid = int(args[0])
    if uid in _admin_ids():
        return await m.reply_text(parse_mode=enums.ParseMode.HTML, text="You can't lock an admin account.")
    reason = " ".join(args[1:]) or None
    await _access().set_user_status(uid, "locked", reason, by=m.from_user.id)
    await m.reply_text(parse_mode=enums.ParseMode.HTML, text=f"Blocked <code>{uid}</code> and logged them out." + (f"\nReason: {html.escape(reason)}" if reason else ""))


@bot.on_message(filters.command("unlock") & admin_only)
async def cmd_unlock(_, m: Message):
    args = _args(m)
    if not args or not args[0].lstrip("-").isdigit():
        return await m.reply_text(parse_mode=enums.ParseMode.HTML, text="Usage: /unlock <user_id>")
    await _access().set_user_status(int(args[0]), "active")
    await m.reply_text(parse_mode=enums.ParseMode.HTML, text=f"Unblocked <code>{args[0]}</code>. They can log in again.")


@bot.on_message(filters.command("revoke") & admin_only)
async def cmd_revoke(_, m: Message):
    args = _args(m)
    if not args or not args[0].lstrip("-").isdigit():
        return await m.reply_text(parse_mode=enums.ParseMode.HTML, text="Usage: /revoke <user_id>")
    tv = await _access().revoke_sessions(int(args[0]))
    await m.reply_text(parse_mode=enums.ParseMode.HTML, text=f"Logged <code>{args[0]}</code> out of all devices.")


@bot.on_message(filters.command("allow") & admin_only)
async def cmd_allow(_, m: Message):
    args = _args(m)
    if not args or not args[0].isdigit():
        return await m.reply_text(parse_mode=enums.ParseMode.HTML, text="Usage: /allow <user_id> [note]")
    await _access().allowlist_add(int(args[0]), m.from_user.id, " ".join(args[1:]) or None)
    await m.reply_text(parse_mode=enums.ParseMode.HTML, text=f"<code>{args[0]}</code> is approved and can sign up without a code.")


@bot.on_message(filters.command("disallow") & admin_only)
async def cmd_disallow(_, m: Message):
    args = _args(m)
    if not args or not args[0].isdigit():
        return await m.reply_text(parse_mode=enums.ParseMode.HTML, text="Usage: /disallow <user_id>")
    ok = await _access().allowlist_remove(int(args[0]))
    await m.reply_text(parse_mode=enums.ParseMode.HTML, text="Removed from approved sign-up list." if ok else "User was not on the list.")


@bot.on_message(filters.command("reverify") & admin_only)
async def cmd_reverify(_, m: Message):
    note = await m.reply_text(parse_mode=enums.ParseMode.HTML, text="Checking if users are still in the required channels…")
    res = await _access().reverify_all()
    await note.edit_text(parse_mode=enums.ParseMode.HTML, text=f"Done! Checked {res['checked']} users · {res['restricted']} left the channel · {res['restored']} rejoined")


@bot.on_message(filters.command(["bypass", "exempt"]) & admin_only)
async def cmd_bypass(_, m: Message):
    args = _args(m)
    access = _access()

    reply_user = m.reply_to_message.from_user if (m.reply_to_message and m.reply_to_message.from_user) else None
    if reply_user and not reply_user.is_bot:
        uid = int(reply_user.id)
        note = " ".join(args) or None
        await access.bypass_add(uid, by=m.from_user.id, note=note)
        name = html.escape(reply_user.first_name or str(uid))
        return await m.reply_text(
            parse_mode=enums.ParseMode.HTML,
            text=f"✅ <b>{name}</b> (<code>{uid}</code>) can now skip required channels.",
        )

    if not args:
        policy = await access.get_policy()
        enforced = policy.get("enforce_membership", False)
        bypassed_list = await access.bypass_list()
        return await m.reply_text(
            parse_mode=enums.ParseMode.HTML,
            text=(
                "<b>Skip Channel Check</b>\n\n"
                f"• Check channels: <code>{'ON' if enforced else 'OFF (skipped for everyone)'}</code>\n"
                f"• People skipping: <code>{len(bypassed_list)}</code>\n\n"
                "<b>How to use:</b>\n"
                "• <code>/bypass &lt;user_id&gt; [note]</code> — Let this user skip channel check\n"
                "• <code>/bypass</code> (reply to a message) — Let this user skip channel check\n"
                "• <code>/unbypass &lt;user_id&gt;</code> — Make them follow channel rules again\n"
                "• <code>/bypass on</code> — Turn off channel check for everyone\n"
                "• <code>/bypass off</code> — Turn channel check back on\n"
                "• <code>/bypasses</code> — See who gets to skip"
            ),
        )

    sub = args[0].strip().lower()

    if sub in ("on", "enable", "true", "all"):
        p = await access.update_policy({"enforce_membership": False}, by=m.from_user.id)
        return await m.reply_text(
            parse_mode=enums.ParseMode.HTML,
            text="🔓 Channel check is now <b>off for everyone</b>. Anyone can join or stream without joining channels.",
        )
    if sub in ("off", "disable", "false"):
        p = await access.update_policy({"enforce_membership": True}, by=m.from_user.id)
        return await m.reply_text(
            parse_mode=enums.ParseMode.HTML,
            text="🔒 Channel check is now <b>on</b>. Users must join required channels to use the app.",
        )
    if sub in ("list", "status"):
        items = await access.bypass_list()
        if not items:
            return await m.reply_text(parse_mode=enums.ParseMode.HTML, text="No users are currently skipping channel checks.")
        lines = [f"• <code>{i['user_id']}</code>" + (f" ({html.escape(i['note'])})" if i.get("note") else "") for i in items[:50]]
        return await m.reply_text(
            parse_mode=enums.ParseMode.HTML,
            text=f"<b>People skipping channels ({len(items)})</b>\n\n" + "\n".join(lines),
        )

    if sub.lstrip("-").isdigit():
        uid = int(sub)
        note = " ".join(args[1:]) if len(args) > 1 else None
        await access.bypass_add(uid, by=m.from_user.id, note=note)
        return await m.reply_text(
            parse_mode=enums.ParseMode.HTML,
            text=f"✅ <code>{uid}</code> can now skip required channels.",
        )

    return await m.reply_text(
        parse_mode=enums.ParseMode.HTML,
        text="How to use: <code>/bypass &lt;user_id|on|off|status&gt;</code> (or reply to a user)",
    )


@bot.on_message(filters.command(["unbypass", "unexempt"]) & admin_only)
async def cmd_unbypass(_, m: Message):
    args = _args(m)
    access = _access()

    reply_user = m.reply_to_message.from_user if (m.reply_to_message and m.reply_to_message.from_user) else None
    if reply_user and not reply_user.is_bot:
        uid = int(reply_user.id)
    elif args and args[0].lstrip("-").isdigit():
        uid = int(args[0])
    else:
        return await m.reply_text(parse_mode=enums.ParseMode.HTML, text="How to use: <code>/unbypass &lt;user_id&gt;</code> (or reply to a user)")

    removed = await access.bypass_remove(uid)
    if removed:
        await m.reply_text(
            parse_mode=enums.ParseMode.HTML,
            text=f"Removed channel skip for <code>{uid}</code>. They must follow channel rules again.",
        )
    else:
        await m.reply_text(
            parse_mode=enums.ParseMode.HTML,
            text=f"<code>{uid}</code> was not on the skip list.",
        )


@bot.on_message(filters.command(["bypasses", "bypassed"]) & admin_only)
async def cmd_bypasses(_, m: Message):
    access = _access()
    items = await access.bypass_list()
    if not items:
        return await m.reply_text(parse_mode=enums.ParseMode.HTML, text="No users are currently skipping channel checks.")
    lines = [f"• <code>{i['user_id']}</code>" + (f" ({html.escape(i['note'])})" if i.get("note") else "") for i in items[:50]]
    await m.reply_text(
        parse_mode=enums.ParseMode.HTML,
        text=f"<b>People skipping channels ({len(items)})</b>\n\n" + "\n".join(lines),
    )

