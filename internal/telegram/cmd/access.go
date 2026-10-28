package cmd

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
	"go.mongodb.org/mongo-driver/v2/bson"

	"streamgo/internal/models"
)

// handleAccess displays current registration mode, required channels, and user counts.
func (h *Handler) handleAccess(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64) {
	if !h.isAdmin(senderID) {
		return
	}

	if h.accessCtrl == nil {
		_ = h.replyHTML(ctx, e, upd, "Access control service not initialized.", nil)
		return
	}

	policy, err := h.accessCtrl.GetPolicy(ctx)
	if err != nil {
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Failed to load policy: %s", html.EscapeString(err.Error())), nil)
		return
	}

	var userCount, lockedCount, restrictedCount int64
	if h.db != nil {
		userCount, _ = h.db.Collection("users").CountDocuments(ctx, bson.M{})
		lockedCount, _ = h.db.Collection("user_access").CountDocuments(ctx, bson.M{"status": "locked"})
		restrictedCount, _ = h.db.Collection("user_access").CountDocuments(ctx, bson.M{"status": "restricted"})
	}

	channelCheck := "off (skipped for everyone)"
	if policy.EnforceMembership {
		channelCheck = "on"
	}

	bypasses, _ := h.accessCtrl.ListBypass(ctx)
	skippingText := ""
	if len(bypasses) > 0 {
		skippingText = fmt.Sprintf(" · <code>%d</code> can skip", len(bypasses))
	}

	chatsStr := "none"
	if len(policy.RequiredChats) > 0 {
		var chatParts []string
		for _, c := range policy.RequiredChats {
			vis := "public"
			if c.IsPrivate {
				vis = "private"
			}
			title := c.Title
			if title == "" {
				title = fmt.Sprintf("%d", c.ChatID)
			}
			chatParts = append(chatParts, fmt.Sprintf("%s (<code>%d</code>, %s)", html.EscapeString(title), c.ChatID, vis))
		}
		chatsStr = strings.Join(chatParts, ", ")
	}

	lines := []string{
		"<b>Access & Signups Overview</b>",
		fmt.Sprintf("• Who can sign up: <code>%s</code>", html.EscapeString(policy.RegistrationMode)),
		fmt.Sprintf("• Must join channels: <code>%s</code>%s", channelCheck, skippingText),
		fmt.Sprintf("• Required channels: %s", chatsStr),
		fmt.Sprintf("• Users: %d total · %d blocked · %d waiting to join channel", userCount, lockedCount, restrictedCount),
	}

	_ = h.replyHTML(ctx, e, upd, strings.Join(lines, "\n"), nil)
}

// handleRegistration updates the registration mode (open, invite, allowlist, closed).
func (h *Handler) handleRegistration(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64, args []string) {
	if !h.isAdmin(senderID) {
		return
	}

	if len(args) == 0 {
		_ = h.replyHTML(ctx, e, upd, "Usage: /registration &lt;open|invite|allowlist|closed&gt;", nil)
		return
	}

	mode := strings.ToLower(strings.TrimSpace(args[0]))
	notes := map[string]string{
		"open":      "anyone can sign up",
		"invite":    "requires an invite code",
		"allowlist": "only approved users can sign up",
		"closed":    "no new signups",
	}

	note, ok := notes[mode]
	if !ok {
		_ = h.replyHTML(ctx, e, upd, "Usage: /registration &lt;open|invite|allowlist|closed&gt;", nil)
		return
	}

	patch := models.PolicyPatch{RegistrationMode: &mode}
	if _, err := h.accessCtrl.UpdatePolicy(ctx, patch, senderID); err != nil {
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Failed to update policy: %s", html.EscapeString(err.Error())), nil)
		return
	}

	_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Sign-up mode is now <code>%s</code> (%s).", mode, note), nil)
}

// handleMembership toggles channel membership check enforcement.
func (h *Handler) handleMembership(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64, args []string) {
	if !h.isAdmin(senderID) {
		return
	}

	if len(args) == 0 {
		_ = h.replyHTML(ctx, e, upd, "Usage: /membership &lt;on|off&gt;", nil)
		return
	}

	val := strings.ToLower(strings.TrimSpace(args[0])) == "on"
	patch := models.PolicyPatch{EnforceMembership: &val}
	if _, err := h.accessCtrl.UpdatePolicy(ctx, patch, senderID); err != nil {
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Failed to update policy: %s", html.EscapeString(err.Error())), nil)
		return
	}

	if val {
		_ = h.replyHTML(ctx, e, upd, "Channel check is now <b>on</b>. Users have to join your required channels to use the app.", nil)
	} else {
		_ = h.replyHTML(ctx, e, upd, "Channel check is now <b>off</b>. Anyone can use the app without joining channels.", nil)
	}
}

// handleRequireChat adds a channel that users must join before using the service.
func (h *Handler) handleRequireChat(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64, args []string) {
	if !h.isAdmin(senderID) {
		return
	}

	var targetChatID int64
	visibility := "public"

	for _, a := range args {
		al := strings.ToLower(strings.TrimSpace(a))
		if al == "private" || al == "priv" || al == "secret" || al == "hidden" {
			visibility = "private"
		} else if al == "public" || al == "pub" || al == "visible" {
			visibility = "public"
		} else if id, err := strconv.ParseInt(al, 10, 64); err == nil {
			targetChatID = id
		} else {
			usage := "Usage: <code>/requirechat [chat_id] [public|private]</code>\n\n" +
				"Examples:\n" +
				"• <code>/requirechat</code> (run in group, public)\n" +
				"• <code>/requirechat private</code> (run in group, hide link on WebX)\n" +
				"• <code>/requirechat -1001234567890 private</code>\n" +
				"• <code>/requirechat -1001234567890 public</code>"
			_ = h.replyHTML(ctx, e, upd, usage, nil)
			return
		}
	}

	if targetChatID == 0 {
		peerChatID, _, _, _ := h.extractPeerInfo(msg, e)
		targetChatID = peerChatID
	}

	if targetChatID > 0 {
		_ = h.replyHTML(ctx, e, upd, "That looks like a user id — required chats must be groups or channels.", nil)
		return
	}

	isPrivate := visibility == "private"
	reqChat := models.RequiredChat{
		ChatID:    targetChatID,
		IsPrivate: isPrivate,
	}

	if _, err := h.accessCtrl.AddRequiredChat(ctx, reqChat); err != nil {
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Failed to add required chat: %s", html.EscapeString(err.Error())), nil)
		return
	}

	var inviteInfo string
	if isPrivate {
		inviteInfo = "🔒 <b>Private / Secret Group</b>: The invite link will <b>never</b> be shown on WebX."
	} else {
		inviteInfo = "🌐 <b>Public Group</b>: Invite link shown on WebX."
	}

	reply := fmt.Sprintf(
		"Now required: <b>%d</b> (<code>%d</code>)\n"+
			"Visibility: <code>%s</code>\n\n"+
			"%s",
		targetChatID, targetChatID, visibility, inviteInfo,
	)
	_ = h.replyHTML(ctx, e, upd, reply, nil)
}

// handleUnrequireChat removes a required channel.
func (h *Handler) handleUnrequireChat(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64, args []string) {
	if !h.isAdmin(senderID) {
		return
	}

	var targetChatID int64
	if len(args) > 0 {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			_ = h.replyHTML(ctx, e, upd, "Usage: /unrequirechat [chat_id]", nil)
			return
		}
		targetChatID = id
	} else {
		peerChatID, _, _, _ := h.extractPeerInfo(msg, e)
		targetChatID = peerChatID
	}

	policy, err := h.accessCtrl.RemoveRequiredChat(ctx, targetChatID)
	if err != nil {
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Failed to remove required chat: %s", html.EscapeString(err.Error())), nil)
		return
	}

	_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Removed <code>%d</code>. Required chats: %d", targetChatID, len(policy.RequiredChats)), nil)
}

// handleInvite generates a new registration invite code.
func (h *Handler) handleInvite(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64, args []string) {
	if !h.isAdmin(senderID) {
		return
	}

	maxUses := 1
	ttlDays := 7
	note := ""

	if len(args) > 0 {
		if u, err := strconv.Atoi(args[0]); err == nil && u > 0 {
			maxUses = u
		}
	}
	if len(args) > 1 {
		if d, err := strconv.Atoi(args[1]); err == nil && d >= 0 {
			ttlDays = d
		}
	}
	if len(args) > 2 {
		note = strings.Join(args[2:], " ")
	}

	inv, err := h.accessCtrl.CreateInvite(ctx, senderID, maxUses, ttlDays, note)
	if err != nil {
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Failed to create invite: %s", html.EscapeString(err.Error())), nil)
		return
	}

	usesStr := fmt.Sprintf("%d", inv.MaxUses)
	if inv.MaxUses == 0 {
		usesStr = "unlimited"
	}
	expiry := fmt.Sprintf("lasts %d days", ttlDays)
	if ttlDays == 0 {
		expiry = "never expires"
	}

	_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Invite code: <code>%s</code>\nUses: %s · %s", inv.Code, usesStr, expiry), nil)
}

// handleInvites lists currently active invite codes.
func (h *Handler) handleInvites(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64) {
	if !h.isAdmin(senderID) {
		return
	}

	items, err := h.accessCtrl.ListInvites(ctx, false)
	if err != nil || len(items) == 0 {
		_ = h.replyHTML(ctx, e, upd, "No invite codes yet. Make one with /invite [uses] [days].", nil)
		return
	}

	lines := []string{"<b>Invite codes</b>", ""}
	for _, i := range items {
		maxU := fmt.Sprintf("%d", i.MaxUses)
		if i.MaxUses == 0 {
			maxU = "unlimited"
		}
		status := ""
		if i.Revoked {
			status = " · <i>revoked</i>"
		} else if i.ExpiresAt != nil && float64(*i.ExpiresAt) < float64(time.Now().Unix()) {
			status = " · <i>expired</i>"
		} else if i.MaxUses > 0 && i.UsedCount >= i.MaxUses {
			status = " · <i>used</i>"
		}
		noteStr := ""
		if i.Note != "" {
			noteStr = fmt.Sprintf(" · %s", html.EscapeString(i.Note))
		}
		lines = append(lines, fmt.Sprintf("• <code>%s</code> · %d/%s%s%s", i.Code, i.UsedCount, maxU, status, noteStr))
	}

	_ = h.replyHTML(ctx, e, upd, strings.Join(lines, "\n"), nil)
}

// handleLock blocks a user from using the service and revokes active sessions.
func (h *Handler) handleLock(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64, args []string) {
	if !h.isAdmin(senderID) {
		return
	}

	if len(args) == 0 {
		_ = h.replyHTML(ctx, e, upd, "Usage: /lock &lt;user_id&gt; [reason]", nil)
		return
	}

	targetID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		_ = h.replyHTML(ctx, e, upd, "Usage: /lock &lt;user_id&gt; [reason]", nil)
		return
	}

	if h.isAdmin(targetID) {
		_ = h.replyHTML(ctx, e, upd, "You can't lock an admin account.", nil)
		return
	}

	reason := ""
	if len(args) > 1 {
		reason = strings.Join(args[1:], " ")
	}

	if err := h.accessCtrl.LockUser(ctx, targetID, reason, senderID, true); err != nil {
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Failed to lock user: %s", html.EscapeString(err.Error())), nil)
		return
	}

	reasonStr := ""
	if reason != "" {
		reasonStr = fmt.Sprintf("\nReason: %s", html.EscapeString(reason))
	}
	_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Blocked <code>%d</code> and logged them out.%s", targetID, reasonStr), nil)
}

// handleUnlock unblocks a user.
func (h *Handler) handleUnlock(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64, args []string) {
	if !h.isAdmin(senderID) {
		return
	}

	if len(args) == 0 {
		_ = h.replyHTML(ctx, e, upd, "Usage: /unlock &lt;user_id&gt;", nil)
		return
	}

	targetID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		_ = h.replyHTML(ctx, e, upd, "Usage: /unlock &lt;user_id&gt;", nil)
		return
	}

	if err := h.accessCtrl.UnlockUser(ctx, targetID); err != nil {
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Failed to unlock user: %s", html.EscapeString(err.Error())), nil)
		return
	}

	_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Unblocked <code>%d</code>. They can log in again.", targetID), nil)
}

// handleRevoke invalidates all active sessions for a user.
func (h *Handler) handleRevoke(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64, args []string) {
	if !h.isAdmin(senderID) {
		return
	}

	if len(args) == 0 {
		_ = h.replyHTML(ctx, e, upd, "Usage: /revoke &lt;user_id&gt;", nil)
		return
	}

	targetID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		_ = h.replyHTML(ctx, e, upd, "Usage: /revoke &lt;user_id&gt;", nil)
		return
	}

	if _, err := h.accessCtrl.RevokeSessions(ctx, targetID); err != nil {
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Failed to revoke sessions: %s", html.EscapeString(err.Error())), nil)
		return
	}

	_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Logged <code>%d</code> out of all devices.", targetID), nil)
}

// handleBypass exempts a user from required channel checks or toggles enforcement.
func (h *Handler) handleBypass(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64, args []string) {
	if !h.isAdmin(senderID) {
		return
	}

	if len(args) == 0 {
		policy, _ := h.accessCtrl.GetPolicy(ctx)
		bypassedList, _ := h.accessCtrl.ListBypass(ctx)
		channelCheck := "OFF (skipped for everyone)"
		if policy.EnforceMembership {
			channelCheck = "ON"
		}
		text := fmt.Sprintf(
			"<b>Skip Channel Check</b>\n\n"+
				"• Check channels: <code>%s</code>\n"+
				"• People skipping: <code>%d</code>\n\n"+
				"<b>How to use:</b>\n"+
				"• <code>/bypass &lt;user_id&gt; [note]</code> — Let this user skip channel check\n"+
				"• <code>/bypass</code> (reply to a message) — Let this user skip channel check\n"+
				"• <code>/unbypass &lt;user_id&gt;</code> — Make them follow channel rules again\n"+
				"• <code>/bypass on</code> — Turn off channel check for everyone\n"+
				"• <code>/bypass off</code> — Turn channel check back on\n"+
				"• <code>/bypasses</code> — See who gets to skip",
			channelCheck, len(bypassedList),
		)
		_ = h.replyHTML(ctx, e, upd, text, nil)
		return
	}

	sub := strings.ToLower(strings.TrimSpace(args[0]))
	if sub == "on" || sub == "enable" || sub == "true" || sub == "all" {
		val := false
		_, _ = h.accessCtrl.UpdatePolicy(ctx, models.PolicyPatch{EnforceMembership: &val}, senderID)
		_ = h.replyHTML(ctx, e, upd, "🔓 Channel check is now <b>off for everyone</b>. Anyone can join or stream without joining channels.", nil)
		return
	}
	if sub == "off" || sub == "disable" || sub == "false" {
		val := true
		_, _ = h.accessCtrl.UpdatePolicy(ctx, models.PolicyPatch{EnforceMembership: &val}, senderID)
		_ = h.replyHTML(ctx, e, upd, "🔒 Channel check is now <b>on</b>. Users must join required channels to use the app.", nil)
		return
	}

	targetID, err := strconv.ParseInt(sub, 10, 64)
	if err != nil {
		_ = h.replyHTML(ctx, e, upd, "How to use: <code>/bypass &lt;user_id|on|off|status&gt;</code> (or reply to a user)", nil)
		return
	}

	note := ""
	if len(args) > 1 {
		note = strings.Join(args[1:], " ")
	}

	if err := h.accessCtrl.AddToBypass(ctx, targetID, senderID, note); err != nil {
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Failed to add bypass: %s", html.EscapeString(err.Error())), nil)
		return
	}

	_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("✅ <code>%d</code> can now skip required channels.", targetID), nil)
}

// handleUnbypass removes a user from required channel check bypass.
func (h *Handler) handleUnbypass(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64, args []string) {
	if !h.isAdmin(senderID) {
		return
	}

	if len(args) == 0 {
		_ = h.replyHTML(ctx, e, upd, "How to use: <code>/unbypass &lt;user_id&gt;</code> (or reply to a user)", nil)
		return
	}

	targetID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		_ = h.replyHTML(ctx, e, upd, "How to use: <code>/unbypass &lt;user_id&gt;</code> (or reply to a user)", nil)
		return
	}

	bypasses, _ := h.accessCtrl.ListBypass(ctx)
	found := false
	for _, b := range bypasses {
		if b.UserID == targetID {
			found = true
			break
		}
	}

	h.accessCtrl.RemoveFromBypass(ctx, targetID)
	if found {
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Removed channel skip for <code>%d</code>. They must follow channel rules again.", targetID), nil)
	} else {
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("<code>%d</code> was not on the skip list.", targetID), nil)
	}
}

// handleBypasses lists all users exempted from channel checks.
func (h *Handler) handleBypasses(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64) {
	if !h.isAdmin(senderID) {
		return
	}

	list, err := h.accessCtrl.ListBypass(ctx)
	if err != nil || len(list) == 0 {
		_ = h.replyHTML(ctx, e, upd, "No users are currently skipping channel checks.", nil)
		return
	}

	lines := []string{fmt.Sprintf("<b>People skipping channels (%d)</b>", len(list)), ""}
	for _, entry := range list {
		noteStr := ""
		if entry.Note != "" {
			noteStr = fmt.Sprintf(" (%s)", html.EscapeString(entry.Note))
		}
		lines = append(lines, fmt.Sprintf("• <code>%d</code>%s", entry.UserID, noteStr))
	}

	_ = h.replyHTML(ctx, e, upd, strings.Join(lines, "\n"), nil)
}
