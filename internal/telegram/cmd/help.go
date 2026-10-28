package cmd

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/markup"
	"github.com/gotd/td/tg"
	"go.mongodb.org/mongo-driver/v2/bson"

	"streamgo/internal/models"
)

// handleStartOrHelp processes /start and /help commands, including WebX browser session authentication.
func (h *Handler) handleStartOrHelp(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, args []string) {
	// WebX Browser Session Authorization: /start auth_<session_id> or /start login_<session_id>
	if len(args) > 0 && (strings.HasPrefix(args[0], "auth_") || strings.HasPrefix(args[0], "login_")) {
		sessionID := strings.TrimSpace(args[0])
		if h.db == nil || h.authSvc == nil {
			_ = h.replyHTML(ctx, e, upd, "<b>Database or Auth service not available.</b>", nil)
			return
		}

		col := h.db.Collection("bot_auth_sessions")
		var session models.BotAuthSession
		err := col.FindOne(ctx, bson.M{"_id": sessionID}).Decode(&session)
		now := float64(time.Now().Unix())
		if err != nil || session.Status != "pending" || session.ExpiresAt < now {
			_ = h.replyHTML(ctx, e, upd, "⚠️ <b>This sign-in request has expired or is invalid.</b>\n\nPlease return to your browser and click <b>Open Telegram App</b> again.", nil)
			return
		}

		_, senderID, senderName, senderUsername := h.extractPeerInfo(msg, e)
		if senderName == "" {
			senderName = senderUsername
		}
		if senderName == "" {
			senderName = "Telegram User"
		}

		user, token, err := h.authSvc.AuthenticateOrRegisterTgUser(ctx, senderID, senderName, senderUsername, "", session.InviteCode)
		if err != nil {
			_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("<b>Authorization failed:</b> %s", html.EscapeString(err.Error())), nil)
			return
		}

		_ = h.authSvc.ConfirmBotSession(ctx, sessionID, token, user)
		welcomeName := user.FirstName
		if welcomeName == "" {
			welcomeName = "User"
		}
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("<b>Authorized Successfully WebX!</b>\n\nWelcome, <b>%s</b>! Your browser session is ready.\nYou can now return to your browser and enjoy your music!", html.EscapeString(welcomeName)), nil)
		return
	}

	// Interactive Help Menu
	_ = h.replyHTML(ctx, e, upd, helpMainText, helpMainKeyboard())
}

// handleCallbackQuery handles inline button navigation in the interactive help menu.
func (h *Handler) handleCallbackQuery(ctx context.Context, e tg.Entities, update *tg.UpdateBotCallbackQuery) {
	if update == nil {
		return
	}

	// Cache access hashes from entities
	if w := h.tgService.PrimaryWorker(); w != nil {
		for uid, u := range e.Users {
			if u != nil && u.AccessHash != 0 {
				w.SetUserAccessHash(uid, u.AccessHash)
			}
		}
		for cid, ch := range e.Channels {
			if ch != nil && ch.AccessHash != 0 {
				w.SetChannelAccessHash(cid, ch.AccessHash)
			}
		}
	}

	data := string(update.Data)
	log.Infof("[telegram] Callback query from user %d: %q", update.UserID, data)

	w := h.tgService.PrimaryWorker()
	if w == nil || w.API == nil {
		return
	}

	if strings.HasPrefix(data, "confirm_restart") {
		h.handleRestartConfirmation(ctx, e, update)
		return
	}

	if h.handleAdminCallback(ctx, e, update) {
		return
	}

	// Always answer the query to dismiss button loading spinner
	_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{
		QueryID: update.QueryID,
	})

	if data == "help_noop" {
		return
	}

	if data == "help_close" {
		_, _ = w.API.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
			ID:     []int{update.MsgID},
			Revoke: true,
		})
		return
	}

	menu, exists := helpMenuMap[data]
	if !exists {
		return
	}

	_ = h.editMessage(ctx, update.Peer, e, update.MsgID, menu.text, menu.markup)
}

type helpMenuEntry struct {
	text   string
	markup tg.ReplyMarkupClass
}

var (
	helpMainText = "<b>StreamX Bot Help & Commands</b>\n\n" +
		"Select a category below to explore commands and configuration options:\n\n" +
		"• <b>General</b>: Basic user utilities, ID lookups and media inspection.\n" +
		"• <b>Sources & Filter</b>: Contributor allowlists, bans and hybrid mode.\n" +
		"• <b>Access & Invites</b>: Who can sign up, invite codes, required channels & user locks.\n" +
		"• <b>Config</b>: Detailed configuration variables with paging.\n" +
		"• <b>Admin & Dev</b>: Dashboard, log files, updater and maintenance.\n" +
		"• <b>All Commands</b>: Complete command reference cheat sheet."

	helpUserText = "<b>General Commands</b>\n\n" +
		"• <code>/ping</code> or <code>/alive</code>\n" +
		"  Check bot latency, response time, and system uptime.\n\n" +
		"• <code>/id</code>\n" +
		"  Get your user ID, current chat ID, or reply to a message to inspect sender and forwarded entity IDs.\n\n" +
		"• <code>/search &lt;query&gt;</code>\n" +
		"  Search for music tracks in the database library.\n\n" +
		"• <code>/mediainfo</code> or <code>/mi</code>\n" +
		"  Inspect technical metadata, audio codec, bitrate, and tags (send with media or in reply).\n\n" +
		"• <code>/help</code>\n" +
		"  Display this interactive help menu."

	helpSourcesText = "<b>Sources & Filter Commands</b>\n\n" +
		"Manage trusted contributor sources, ban lists, and hybrid filter modes:\n\n" +
		"• <code>/filter_mode [0|1|2|hybrid]</code>\n" +
		"  View or switch the audio ingestion mode:\n" +
		"  - <code>0</code> (<code>group_only</code>): Primary channel only\n" +
		"  - <code>1</code> (<code>anyone</code>): Accept from any source\n" +
		"  - <code>2</code> (<code>hybrid</code>): Accept from allowed sources collection only\n\n" +
		"• <code>/sources</code>\n" +
		"  Overview of current filter mode, primary channel, allowed sources, and banned sources.\n\n" +
		"• <code>/allow &lt;peer&gt;</code>\n" +
		"  Add a channel, group, or user to allowed contributors. Automatically unbans if previously banned.\n\n" +
		"• <code>/disallow &lt;peer&gt;</code>\n" +
		"  Remove a source from allowed contributors.\n\n" +
		"• <code>/ban &lt;peer&gt; [reason]</code>\n" +
		"  Strictly block a channel, group, or user from adding tracks. Automatically removes from allowlist.\n\n" +
		"• <code>/unban &lt;peer&gt;</code>\n" +
		"  Unban a source or user."

	helpAccessText = "<b>Access & Invites</b>\n\n" +
		"<i>(For Bot Owner & Admins)</i>\n\n" +
		"<b>Sign-ups & Invites:</b>\n" +
		"• <code>/access</code>\n" +
		"  See your signup rules, required channels, and user counts.\n\n" +
		"• <code>/registration [open|invite|allowlist|closed]</code>\n" +
		"  Choose who gets to sign up:\n" +
		"  - <code>open</code>: Anyone can sign in with Telegram.\n" +
		"  - <code>invite</code>: Need an invite code to join.\n" +
		"  - <code>allowlist</code>: Only people you approved can join.\n" +
		"  - <code>closed</code>: Nobody new can sign up.\n\n" +
		"• <code>/invite [uses] [days] [note]</code>\n" +
		"  Make an invite code (default: 1 use, lasts 7 days).\n\n" +
		"• <code>/invites</code>\n" +
		"  See all your invite codes and who used them.\n\n" +
		"<b>Channel Requirements:</b>\n" +
		"• <code>/membership [on|off]</code>\n" +
		"  Turn the required channel check on or off.\n\n" +
		"• <code>/requirechat [chat_id] [public|private]</code>\n" +
		"  Make people join a channel or group before using the app.\n\n" +
		"• <code>/unrequirechat [chat_id]</code>\n" +
		"  Stop requiring this channel.\n\n" +
		"• <code>/bypass &lt;user_id&gt; [note]</code>\n" +
		"  Let someone in without joining the channel.\n\n" +
		"• <code>/unbypass &lt;user_id&gt;</code>\n" +
		"  Make someone follow channel rules again.\n\n" +
		"• <code>/bypasses</code>\n" +
		"  See everyone who gets to skip channel checks.\n\n" +
		"<b>Managing Users:</b>\n" +
		"• <code>/lock &lt;user_id&gt; [reason]</code>\n" +
		"  Block someone from using the app and log them out everywhere.\n\n" +
		"• <code>/unlock &lt;user_id&gt;</code>\n" +
		"  Unblock someone and let them back in.\n\n" +
		"• <code>/revoke &lt;user_id&gt;</code>\n" +
		"  Log someone out of all their devices."

	helpConfig1Text = "<b>Configuration Variables (1 / 3)</b>\n\n" +
		"<b>Core Credentials:</b>\n" +
		"• <code>BOT_TOKEN</code>: Telegram bot token from @BotFather.\n" +
		"• <code>API_ID</code>: Telegram API ID from my.telegram.org.\n" +
		"• <code>API_HASH</code>: Telegram API Hash from my.telegram.org.\n" +
		"• <code>OWNER_ID</code>: Telegram user ID of primary bot owner.\n" +
		"• <code>SUDO_USERS</code>: User IDs granted sudo/admin privileges.\n\n" +
		"<b>Database & Security:</b>\n" +
		"• <code>MONGO_URI</code>: MongoDB connection string URI.\n" +
		"• <code>DATABASE_NAME</code>: Database name (default: <code>Stream</code>).\n" +
		"• <code>SECRET_KEY</code>: Secret used for signing web auth tokens."

	helpConfig2Text = "<b>Configuration Variables (2 / 3)</b>\n\n" +
		"<b>Channels & Filtering:</b>\n" +
		"• <code>CHANNEL_ID</code>: Primary channel ID for track indexing and streaming.\n" +
		"• <code>DUMP_CHANNEL_ID</code>: Channel ID for dumping/mirroring tracks.\n" +
		"• <code>FILTER_MODE</code>:\n" +
		"  - <code>0</code> / <code>group_only</code>: Ingest tracks only from <code>CHANNEL_ID</code>.\n" +
		"  - <code>1</code> / <code>anyone</code>: Ingest tracks from any chat, group, or user.\n" +
		"  - <code>2</code> / <code>hybrid</code>: Ingest tracks only from allowed contributors.\n" +
		"• <code>COLLABORATOR_IDS</code>: Channel/user IDs seeded into allowed contributors on startup."

	helpConfig3Text = "<b>Configuration Variables (3 / 3)</b>\n\n" +
		"<b>Multi-Clients & Web API:</b>\n" +
		"• <code>MULTI_CLIENTS</code>: Enable multiple worker clients for parallel downloads.\n" +
		"• <code>MULTI_CLIENT_TOKENS</code>: Secondary bot tokens to distribute Telegram bandwidth.\n" +
		"• <code>CORS_ORIGIN</code>: Allowed web origins for API (or <code>*</code>).\n" +
		"• <code>DEBUG</code>: Enable verbose debug logging in console."

	helpAdminText = "<b>Admin & Developer Commands</b>\n\n" +
		"<i>(Accessible to Bot Owner & Sudo users)</i>\n\n" +
		"• <code>/sudo</code>\n" +
		"  Dashboard for Config, System stats & Database info.\n\n" +
		"• <code>/logs</code> or <code>/log</code>\n" +
		"  View recent application logs.\n\n" +
		"• <code>/restart</code>\n" +
		"  Restart the bot service.\n\n" +
		"• <code>/fileid</code>\n" +
		"  Retrieve Telegram file_id for replied media."

	helpAllText = "<b>Complete Command Reference</b>\n\n" +
		"<b>User Utilities:</b>\n" +
		"• <code>/ping</code>, <code>/alive</code> - Latency & uptime\n" +
		"• <code>/id</code> - Chat/User/Message ID lookup\n" +
		"• <code>/search &lt;query&gt;</code> - Search library tracks\n" +
		"• <code>/mediainfo</code>, <code>/mi</code> - Media metadata analyzer\n" +
		"• <code>/help</code> - Interactive help menu\n\n" +
		"<b>Sources & Hybrid Filter:</b>\n" +
		"• <code>/sources</code> - Summary & mode status\n" +
		"• <code>/filter_mode [0|1|2]</code> - Switch ingestion mode\n" +
		"• <code>/allow &lt;peer&gt;</code> - Add allowed contributor\n" +
		"• <code>/disallow &lt;peer&gt;</code> - Remove allowed contributor\n" +
		"• <code>/ban &lt;peer&gt;</code> - Ban channel/group/user\n" +
		"• <code>/unban &lt;peer&gt;</code> - Unban source/user\n\n" +
		"<b>Access & Invites:</b>\n" +
		"• <code>/access</code> - See access settings & stats\n" +
		"• <code>/registration [mode]</code> - Who can sign up\n" +
		"• <code>/invite [uses] [days]</code> - Make an invite code\n" +
		"• <code>/invites</code> - See invite codes\n" +
		"• <code>/lock &lt;user_id&gt;</code> - Block user & log them out\n" +
		"• <code>/unlock &lt;user_id&gt;</code> - Unblock user\n" +
		"• <code>/revoke &lt;user_id&gt;</code> - Log user out on all devices\n\n" +
		"<b>Admin & Maintenance:</b>\n" +
		"• <code>/sudo</code> - Control panel & stats\n" +
		"• <code>/logs</code>, <code>/log</code> - Application logs\n" +
		"• <code>/restart</code> - Restart bot service\n" +
		"• <code>/fileid</code> - Media file_id extractor"
)

func helpMainKeyboard() tg.ReplyMarkupClass {
	return markup.InlineKeyboard(
		markup.InlineButtonRow(
			markup.Callback("General", []byte("help_user")),
			markup.Callback("Sources & Filter", []byte("help_sources")),
		),
		markup.InlineButtonRow(
			markup.Callback("Access & Invites", []byte("help_access")),
			markup.Callback("Config", []byte("help_config_1")),
		),
		markup.InlineButtonRow(
			markup.Callback("Admin & Dev", []byte("help_admin")),
			markup.Callback("All Commands", []byte("help_all")),
		),
		markup.InlineButtonRow(
			markup.Callback("Close", []byte("help_close")),
		),
	)
}

func helpUserKeyboard() tg.ReplyMarkupClass {
	return markup.InlineKeyboard(
		markup.InlineButtonRow(
			markup.Callback("« Back", []byte("help_main")),
			markup.Callback("Sources", []byte("help_sources")),
		),
		markup.InlineButtonRow(
			markup.Callback("Close", []byte("help_close")),
		),
	)
}

func helpSourcesKeyboard() tg.ReplyMarkupClass {
	return markup.InlineKeyboard(
		markup.InlineButtonRow(
			markup.Callback("« Back", []byte("help_main")),
			markup.Callback("Config", []byte("help_config_1")),
		),
		markup.InlineButtonRow(
			markup.Callback("Close", []byte("help_close")),
		),
	)
}

func helpConfig1Keyboard() tg.ReplyMarkupClass {
	return markup.InlineKeyboard(
		markup.InlineButtonRow(
			markup.Callback("1 / 3", []byte("help_noop")),
			markup.Callback("Next »", []byte("help_config_2")),
		),
		markup.InlineButtonRow(
			markup.Callback("« Back", []byte("help_main")),
			markup.Callback("Close", []byte("help_close")),
		),
	)
}

func helpConfig2Keyboard() tg.ReplyMarkupClass {
	return markup.InlineKeyboard(
		markup.InlineButtonRow(
			markup.Callback("« Prev", []byte("help_config_1")),
			markup.Callback("2 / 3", []byte("help_noop")),
			markup.Callback("Next »", []byte("help_config_3")),
		),
		markup.InlineButtonRow(
			markup.Callback("« Back", []byte("help_main")),
			markup.Callback("Close", []byte("help_close")),
		),
	)
}

func helpConfig3Keyboard() tg.ReplyMarkupClass {
	return markup.InlineKeyboard(
		markup.InlineButtonRow(
			markup.Callback("« Prev", []byte("help_config_2")),
			markup.Callback("3 / 3", []byte("help_noop")),
		),
		markup.InlineButtonRow(
			markup.Callback("« Back", []byte("help_main")),
			markup.Callback("Close", []byte("help_close")),
		),
	)
}

func helpAccessKeyboard() tg.ReplyMarkupClass {
	return markup.InlineKeyboard(
		markup.InlineButtonRow(
			markup.Callback("« Back", []byte("help_main")),
			markup.Callback("Admin & Dev", []byte("help_admin")),
		),
		markup.InlineButtonRow(
			markup.Callback("Close", []byte("help_close")),
		),
	)
}

func helpAdminKeyboard() tg.ReplyMarkupClass {
	return markup.InlineKeyboard(
		markup.InlineButtonRow(
			markup.Callback("« Back", []byte("help_main")),
			markup.Callback("Access & Invites", []byte("help_access")),
		),
		markup.InlineButtonRow(
			markup.Callback("Close", []byte("help_close")),
		),
	)
}

func helpAllKeyboard() tg.ReplyMarkupClass {
	return markup.InlineKeyboard(
		markup.InlineButtonRow(
			markup.Callback("« Back", []byte("help_main")),
		),
		markup.InlineButtonRow(
			markup.Callback("Close", []byte("help_close")),
		),
	)
}

var helpMenuMap = map[string]helpMenuEntry{
	"help_main":     {text: helpMainText, markup: helpMainKeyboard()},
	"help_user":     {text: helpUserText, markup: helpUserKeyboard()},
	"help_sources":  {text: helpSourcesText, markup: helpSourcesKeyboard()},
	"help_access":   {text: helpAccessText, markup: helpAccessKeyboard()},
	"help_config_1": {text: helpConfig1Text, markup: helpConfig1Keyboard()},
	"help_config_2": {text: helpConfig2Text, markup: helpConfig2Keyboard()},
	"help_config_3": {text: helpConfig3Text, markup: helpConfig3Keyboard()},
	"help_admin":    {text: helpAdminText, markup: helpAdminKeyboard()},
	"help_all":      {text: helpAllText, markup: helpAllKeyboard()},
}
