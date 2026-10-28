package cmd

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
)

// handleSources displays the current source filtering status.
func (h *Handler) handleSources(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64) {
	if !h.isAdmin(senderID) {
		return
	}

	if h.accessFilter == nil {
		_ = h.replyHTML(ctx, e, upd, "Source filter service not active.", nil)
		return
	}

	mode, modeName, allowedCount, bannedCount, channelID := h.accessFilter.GetStats()
	text := fmt.Sprintf(
		"<b>Sources & Filter Summary</b>\n"+
			"• <b>Filter Mode:</b> <code>%d</code> (<code>%s</code>)\n"+
			"• <b>Main Channel ID:</b> <code>%d</code>\n"+
			"• <b>Allowed Sources:</b> %d\n"+
			"• <b>Banned Sources:</b> %d",
		mode, html.EscapeString(modeName), channelID, allowedCount, bannedCount,
	)

	_ = h.replyHTML(ctx, e, upd, text, nil)
}

// handleFilterMode views or updates the live ingestion filter mode.
func (h *Handler) handleFilterMode(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64, args []string) {
	if !h.isAdmin(senderID) {
		return
	}

	if len(args) == 0 {
		mode := h.accessFilter.GetMode()
		text := fmt.Sprintf(
			"<b>Current Filter Mode:</b> <code>%d</code> (<code>%s</code>)\n\n"+
				"<b>Modes:</b>\n"+
				"• <code>0</code> / <code>group_only</code>: Preferred channel only\n"+
				"• <code>1</code> / <code>anyone</code>: Accept files from any source\n"+
				"• <code>2</code> / <code>hybrid</code>: Accept files from configured allowlist only\n\n"+
				"<b>Usage:</b> <code>/filter_mode &lt;0|1|2|hybrid&gt;</code>",
			mode, filterModeToString(mode),
		)
		_ = h.replyHTML(ctx, e, upd, text, nil)
		return
	}

	newMode := parseFilterMode(args[0])
	h.accessFilter.SetMode(newMode)
	if h.cfgMgr != nil {
		_, _ = h.cfgMgr.UpdateConfig(ctx, "FILTER_MODE", newMode)
	}
	_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("ꪜ Filter mode updated to <code>%d</code> (<code>%s</code>).", newMode, filterModeToString(newMode)), nil)
}

// handleAllow adds a channel, group, or user to allowed contributors.
func (h *Handler) handleAllow(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64, args []string) {
	if !h.isAdmin(senderID) {
		return
	}

	if len(args) == 0 {
		_ = h.replyHTML(ctx, e, upd, "<b>Usage:</b> <code>/allow &lt;source_id|@username&gt; [custom name]</code> (or reply to a message)\nExample: <code>/allow -100123456789 PZP Music</code>", nil)
		return
	}

	targetID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		_ = h.replyHTML(ctx, e, upd, "Invalid chat ID.", nil)
		return
	}

	name := "N/A"
	if len(args) > 1 {
		name = strings.Join(args[1:], " ")
	}

	if err := h.accessFilter.AllowChat(ctx, targetID); err != nil {
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("ㄨ Error adding allowed source: %s", html.EscapeString(err.Error())), nil)
		return
	}

	sourceType := "channel"
	if targetID > 0 {
		sourceType = "user"
	}

	_ = h.replyHTML(ctx, e, upd, fmt.Sprintf(
		"ꪜ <b>Added to Allowed Sources:</b>\n"+
			"• <b>ID:</b> <code>%d</code>\n"+
			"• <b>Name:</b> <code>%s</code>\n"+
			"• <b>Type:</b> <code>%s</code>",
		targetID, html.EscapeString(name), sourceType,
	), nil)
}

// handleDisallow removes a channel, group, or user from allowed contributors.
func (h *Handler) handleDisallow(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64, args []string) {
	if !h.isAdmin(senderID) {
		return
	}

	if len(args) == 0 {
		_ = h.replyHTML(ctx, e, upd, "<b>Usage:</b> <code>/disallow &lt;source_id|@username&gt;</code> (or reply to a message)", nil)
		return
	}

	targetID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		_ = h.replyHTML(ctx, e, upd, "Invalid chat ID.", nil)
		return
	}

	if err := h.accessFilter.DisallowChat(ctx, targetID); err != nil {
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("ⓘ Source <code>%d</code> was not found in allowed sources.", targetID), nil)
		return
	}

	sourceType := "channel"
	if targetID > 0 {
		sourceType = "user"
	}

	_ = h.replyHTML(ctx, e, upd, fmt.Sprintf(
		"ꪜ <b>Removed from Allowed Sources:</b>\n"+
			"• <b>ID:</b> <code>%d</code>\n"+
			"• <b>Name:</b> <code>N/A</code>\n"+
			"• <b>Type:</b> <code>%s</code>",
		targetID, sourceType,
	), nil)
}

// handleBan blocks a channel, group, or user from contributing tracks.
func (h *Handler) handleBan(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64, args []string) {
	if !h.isAdmin(senderID) {
		return
	}

	if len(args) == 0 {
		_ = h.replyHTML(ctx, e, upd, "<b>Usage:</b> <code>/ban &lt;source_id|@username&gt; [reason]</code> (or reply to a message)\nExample: <code>/ban 123456789 Spamming tracks</code>", nil)
		return
	}

	targetID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		_ = h.replyHTML(ctx, e, upd, "Invalid peer ID.", nil)
		return
	}

	reason := "No reason provided"
	if len(args) > 1 {
		reason = strings.Join(args[1:], " ")
	}

	if err := h.accessFilter.BanChat(ctx, targetID, reason); err != nil {
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("ㄨ Error banning source: %s", html.EscapeString(err.Error())), nil)
		return
	}

	sourceType := "channel"
	if targetID > 0 {
		sourceType = "user"
	}

	_ = h.replyHTML(ctx, e, upd, fmt.Sprintf(
		"⊘ <b>Source Banned:</b>\n"+
			"• <b>ID:</b> <code>%d</code>\n"+
			"• <b>Name:</b> <code>N/A</code>\n"+
			"• <b>Type:</b> <code>%s</code>\n"+
			"• <b>Reason:</b> <code>%s</code>\n\n"+
			"<i>This source is strictly blocked from adding tracks and using the service.</i>",
		targetID, sourceType, html.EscapeString(reason),
	), nil)
}

// handleUnban unbans a channel, group, or user.
func (h *Handler) handleUnban(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64, args []string) {
	if !h.isAdmin(senderID) {
		return
	}

	if len(args) == 0 {
		_ = h.replyHTML(ctx, e, upd, "<b>Usage:</b> <code>/unban &lt;source_id|@username&gt;</code> (or reply to a message)", nil)
		return
	}

	targetID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		_ = h.replyHTML(ctx, e, upd, "Invalid peer ID.", nil)
		return
	}

	if err := h.accessFilter.UnbanChat(ctx, targetID); err != nil {
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("ⓘ Source <code>%d</code> was not found in banned sources.", targetID), nil)
		return
	}

	sourceType := "channel"
	if targetID > 0 {
		sourceType = "user"
	}

	_ = h.replyHTML(ctx, e, upd, fmt.Sprintf(
		"ꪜ <b>Source Unbanned:</b>\n"+
			"• <b>ID:</b> <code>%d</code>\n"+
			"• <b>Name:</b> <code>N/A</code>\n"+
			"• <b>Type:</b> <code>%s</code>",
		targetID, sourceType,
	), nil)
}

func parseFilterMode(val string) int {
	s := strings.ToLower(strings.TrimSpace(val))
	switch s {
	case "0", "group_only", "group", "channel_only":
		return 0
	case "1", "anyone", "all", "any":
		return 1
	case "2", "hybrid", "allowlist", "whitelist":
		return 2
	default:
		if n, err := strconv.Atoi(s); err == nil {
			if n == 1 || n == 2 {
				return n
			}
		}
		return 0
	}
}

func filterModeToString(mode int) string {
	switch mode {
	case 1:
		return "anyone"
	case 2:
		return "hybrid"
	default:
		return "group_only"
	}
}
