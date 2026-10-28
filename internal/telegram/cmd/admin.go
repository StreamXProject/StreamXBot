package cmd

import (
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/markup"
	"github.com/gotd/td/tg"
	"go.mongodb.org/mongo-driver/v2/bson"

	"streamgo/internal/telegram"
)

// handleSudo displays an administrative overview with interactive buttons.
func (h *Handler) handleSudo(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64) {
	if !h.isAdmin(senderID) {
		_ = h.replyHTML(ctx, e, upd, "Access denied.", nil)
		return
	}

	text, kb := h.buildAdminPanelContent(ctx)
	_ = h.replyHTML(ctx, e, upd, text, kb)
}

// handleBS opens the configuration variables browser directly.
func (h *Handler) handleBS(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64) {
	if !h.isAdmin(senderID) {
		_ = h.replyHTML(ctx, e, upd, "Access denied.", nil)
		return
	}

	h.setLastPage(senderID, 0)
	kb := h.getSettingsKeyboard(0, false)
	_ = h.replyHTML(ctx, e, upd, "Config Variables | Page: 0 | State: view", kb)
}

// buildAdminPanelContent prepares the text and keyboard markup for the main Admin Panel.
func (h *Handler) buildAdminPanelContent(ctx context.Context) (string, tg.ReplyMarkupClass) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	uptime := time.Since(h.startTime)

	var trackCount, userCount int64
	if h.db != nil {
		trackCount, _ = h.db.Collection("audioTracks").CountDocuments(ctx, bson.M{})
		userCount, _ = h.db.Collection("users").CountDocuments(ctx, bson.M{})
	}

	readyWorkers := 0
	primaryBot := "none"
	if h.tgService != nil {
		readyWorkers = h.tgService.ReadyWorkerCount()
		if self := h.tgService.Self(); self != nil {
			primaryBot = "@" + self.Username
		}
	}

	text := fmt.Sprintf(
		"<b>Admin Panel:</b>\n\n"+
			"• <b>Go Version:</b> <code>%s</code>\n"+
			"• <b>OS/Arch:</b> <code>%s/%s</code>\n"+
			"• <b>Goroutines:</b> %d\n"+
			"• <b>RAM:</b> %.2f MB\n"+
			"• <b>Uptime:</b> %s\n\n"+
			"• <b>Bot:</b> %s (Workers: %d ready)\n"+
			"• <b>Tracks:</b> %d\n"+
			"• <b>Users:</b> %d",
		runtime.Version(), runtime.GOOS, runtime.GOARCH,
		runtime.NumGoroutine(), float64(m.Alloc)/(1024*1024),
		formatDuration(uptime),
		primaryBot, readyWorkers,
		trackCount, userCount,
	)

	kb := markup.InlineKeyboard(
		markup.InlineButtonRow(
			markup.Callback("Config", []byte("config")),
			markup.Callback("Cookies", []byte("cookies")),
		),
		markup.InlineButtonRow(
			markup.Callback("Stats", []byte("stats")),
			markup.Callback("Database", []byte("database")),
		),
	)

	return text, kb
}

// renderAdminPanel updates an existing message with the main Admin Panel view.
func (h *Handler) renderAdminPanel(ctx context.Context, e tg.Entities, peer tg.PeerClass, msgID int) {
	text, kb := h.buildAdminPanelContent(ctx)
	_ = h.editMessage(ctx, peer, e, msgID, text, kb)
}

// getSettingsKeyboard constructs a paginated inline keyboard for configuration settings matching StreamXBot.
func (h *Handler) getSettingsKeyboard(page int, editMode bool) tg.ReplyMarkupClass {
	var keys []string
	if h.cfgMgr != nil {
		keys = h.cfgMgr.GetAllKeys()
	}
	total := len(keys)
	itemsPerPage := 12

	if total == 0 {
		return markup.InlineKeyboard(
			markup.InlineButtonRow(
				markup.Callback("Close", []byte("close_settings")),
			),
		)
	}

	totalPages := (total + itemsPerPage - 1) / itemsPerPage
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

	start := page * itemsPerPage
	end := start + itemsPerPage
	if end > total {
		end = total
	}
	keysPage := keys[start:end]

	var rows []tg.KeyboardInlineButtonRow
	var currentRow []tg.KeyboardInlineButton

	for _, k := range keysPage {
		cb := fmt.Sprintf("setting_%s", k)
		if editMode {
			cb = fmt.Sprintf("edit_%s", k)
		}
		currentRow = append(currentRow, markup.Callback(k, []byte(cb)))
		if len(currentRow) == 2 {
			rows = append(rows, markup.InlineButtonRow(currentRow...))
			currentRow = nil
		}
	}
	if len(currentRow) > 0 {
		rows = append(rows, markup.InlineButtonRow(currentRow...))
	}

	// Mode toggle & Back button
	modeLabel := "Edit"
	modeCb := fmt.Sprintf("edit_mode_%d", page)
	if editMode {
		modeLabel = "View"
		modeCb = fmt.Sprintf("view_mode_%d", page)
	}

	rows = append(rows, markup.InlineButtonRow(
		markup.Callback(modeLabel, []byte(modeCb)),
		markup.Callback("Back", []byte(fmt.Sprintf("back_main_%d_%t", page, editMode))),
	))

	// Close button
	rows = append(rows, markup.InlineButtonRow(
		markup.Callback("Close", []byte("close_settings")),
	))

	// Page navigation row
	var pageBtns []tg.KeyboardInlineButton
	for i := 0; i < totalPages; i++ {
		label := fmt.Sprintf("%d", i)
		if i == page {
			label = "•"
		}
		pageBtns = append(pageBtns, markup.Callback(label, []byte(fmt.Sprintf("page_%d_%t", i, editMode))))
	}
	rows = append(rows, markup.InlineButtonRow(pageBtns...))

	return markup.InlineKeyboard(rows...)
}

// handleAdminCallback routes all sudo and settings button actions. Returns true if handled.
func (h *Handler) handleAdminCallback(ctx context.Context, e tg.Entities, update *tg.UpdateBotCallbackQuery) bool {
	if update == nil {
		return false
	}
	data := string(update.Data)
	w := h.tgService.PrimaryWorker()
	if w == nil || w.API == nil {
		return false
	}

	// Identify admin callback query prefixes
	isAdminCb := false
	switch {
	case data == "config" || data == "cookies" || data == "stats" || data == "database" || data == "refresh" || data == "back_main":
		isAdminCb = true
	case strings.HasPrefix(data, "back_main_") || strings.HasPrefix(data, "setting_") || strings.HasPrefix(data, "edit_") ||
		strings.HasPrefix(data, "page_") || strings.HasPrefix(data, "manage_add_") || strings.HasPrefix(data, "manage_remove_") ||
		strings.HasPrefix(data, "clear_setting_") || strings.HasPrefix(data, "type_menu_") || strings.HasPrefix(data, "set_type_") ||
		strings.HasPrefix(data, "back_settings_") || data == "close_settings" ||
		strings.HasPrefix(data, "edit_mode") || strings.HasPrefix(data, "view_mode"):
		isAdminCb = true
	}

	if !isAdminCb {
		return false
	}

	// Verify authorization
	if !h.isAdmin(update.UserID) {
		_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{
			QueryID: update.QueryID,
			Message: "Access denied.",
			Alert:   true,
		})
		return true
	}

	switch {
	case data == "config":
		_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: update.QueryID})
		h.setLastPage(update.UserID, 0)
		kb := h.getSettingsKeyboard(0, false)
		_ = h.editMessage(ctx, update.Peer, e, update.MsgID, "Config Variables | Page: 0 | State: view", kb)
		return true

	case strings.HasPrefix(data, "setting_"):
		key := strings.TrimPrefix(data, "setting_")
		val := any("not set")
		if h.cfgMgr != nil {
			val = h.cfgMgr.Get(key)
		}
		_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{
			QueryID: update.QueryID,
			Message: fmt.Sprintf("%s: %v", key, val),
			Alert:   true,
		})
		return true

	case strings.HasPrefix(data, "edit_mode"):
		_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: update.QueryID})
		page := h.getLastPage(update.UserID)
		parts := strings.Split(data, "_")
		if len(parts) >= 3 {
			if p, err := strconv.Atoi(parts[2]); err == nil {
				page = p
				h.setLastPage(update.UserID, page)
			}
		}
		kb := h.getSettingsKeyboard(page, true)
		_ = h.editMessage(ctx, update.Peer, e, update.MsgID, fmt.Sprintf("Config Variables | Page: %d | State: edit", page), kb)
		return true

	case strings.HasPrefix(data, "view_mode"):
		_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: update.QueryID})
		page := h.getLastPage(update.UserID)
		parts := strings.Split(data, "_")
		if len(parts) >= 3 {
			if p, err := strconv.Atoi(parts[2]); err == nil {
				page = p
				h.setLastPage(update.UserID, page)
			}
		}
		kb := h.getSettingsKeyboard(page, false)
		_ = h.editMessage(ctx, update.Peer, e, update.MsgID, fmt.Sprintf("Config Variables | Page: %d | State: view", page), kb)
		return true

	case strings.HasPrefix(data, "page_"):
		_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: update.QueryID})
		parts := strings.Split(data, "_")
		page := 0
		editMode := false
		if len(parts) >= 2 {
			if p, err := strconv.Atoi(parts[1]); err == nil {
				page = p
			}
		}
		if len(parts) >= 3 {
			editMode = strings.ToLower(parts[2]) == "true"
		}
		h.setLastPage(update.UserID, page)
		stateStr := "view"
		if editMode {
			stateStr = "edit"
		}
		kb := h.getSettingsKeyboard(page, editMode)
		_ = h.editMessage(ctx, update.Peer, e, update.MsgID, fmt.Sprintf("Config Variables | Page: %d | State: %s", page, stateStr), kb)
		return true

	case strings.HasPrefix(data, "manage_add_") || strings.HasPrefix(data, "manage_remove_"):
		action := "add"
		key := strings.TrimPrefix(data, "manage_add_")
		if strings.HasPrefix(data, "manage_remove_") {
			action = "remove"
			key = strings.TrimPrefix(data, "manage_remove_")
		}
		_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{
			QueryID: update.QueryID,
			Message: fmt.Sprintf("Send the user ID to %s.", action),
		})
		page := h.getLastPage(update.UserID)
		inputPeer := extractInputPeer(update.Peer, e, w)
		h.setEditState(update.UserID, &AdminEditState{
			Key:       key,
			MsgID:     update.MsgID,
			Page:      page,
			EditMode:  true,
			Action:    action,
			Peer:      update.Peer,
			InputPeer: inputPeer,
			UpdatedAt: time.Now(),
		})
		kb := markup.InlineKeyboard(
			markup.InlineButtonRow(
				markup.Callback("Back", []byte(fmt.Sprintf("back_settings_%d_true", page))),
				markup.Callback("Close", []byte("close_settings")),
			),
		)
		text := fmt.Sprintf("Send the Telegram user ID to %s %s:", action, key)
		_ = h.editMessage(ctx, update.Peer, e, update.MsgID, text, kb)
		return true

	case strings.HasPrefix(data, "clear_setting_"):
		key := strings.TrimPrefix(data, "clear_setting_")
		page := h.getLastPage(update.UserID)
		if h.cfgMgr == nil {
			_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{
				QueryID: update.QueryID,
				Message: "Config manager not available",
				Alert:   true,
			})
			return true
		}
		_, err := h.cfgMgr.ClearSetting(ctx, key)
		if err != nil {
			_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{
				QueryID: update.QueryID,
				Message: fmt.Sprintf("Failed to clear %s: %v", key, err),
				Alert:   true,
			})
		} else {
			h.clearEditState(update.UserID)
			_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{
				QueryID: update.QueryID,
				Message: fmt.Sprintf("Cleared %s!", key),
				Alert:   true,
			})
			kb := h.getSettingsKeyboard(page, true)
			text := fmt.Sprintf("Config Variables | Page: %d | State: edit\n\nCleared %s.", page, key)
			_ = h.editMessage(ctx, update.Peer, e, update.MsgID, text, kb)
		}
		return true

	case strings.HasPrefix(data, "type_menu_"):
		key := strings.TrimPrefix(data, "type_menu_")
		_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: update.QueryID})
		h.renderTypeMenu(ctx, e, update.Peer, update.MsgID, key)
		return true

	case strings.HasPrefix(data, "set_type_"):
		parts := strings.Split(data, "_")
		if len(parts) >= 4 {
			targetType := parts[len(parts)-1]
			key := strings.Join(parts[2:len(parts)-1], "_")
			if h.cfgMgr != nil {
				_, err := h.cfgMgr.UpdateConfigType(ctx, key, targetType)
				if err != nil {
					_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{
						QueryID: update.QueryID,
						Message: fmt.Sprintf("Failed to update type: %v", err),
						Alert:   true,
					})
				} else {
					_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{
						QueryID: update.QueryID,
						Message: fmt.Sprintf("Type of %s updated to %s!", key, targetType),
						Alert:   true,
					})
					h.renderTypeMenu(ctx, e, update.Peer, update.MsgID, key)
				}
			}
		}
		return true

	case strings.HasPrefix(data, "edit_"):
		key := strings.TrimPrefix(data, "edit_")
		_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: update.QueryID})
		page := h.getLastPage(update.UserID)
		inputPeer := extractInputPeer(update.Peer, e, w)
		h.setEditState(update.UserID, &AdminEditState{
			Key:       key,
			MsgID:     update.MsgID,
			Page:      page,
			EditMode:  true,
			Peer:      update.Peer,
			InputPeer: inputPeer,
			UpdatedAt: time.Now(),
		})

		if key == "OWNER_ID" || key == "SUDO_USERS" || key == "COLLABORATOR_ID" || key == "COLLABORATOR_IDS" {
			kb := markup.InlineKeyboard(
				markup.InlineButtonRow(
					markup.Callback("Add", []byte(fmt.Sprintf("manage_add_%s", key))),
					markup.Callback("Remove", []byte(fmt.Sprintf("manage_remove_%s", key))),
				),
				markup.InlineButtonRow(
					markup.Callback("Type", []byte(fmt.Sprintf("type_menu_%s", key))),
					markup.Callback("Clear", []byte(fmt.Sprintf("clear_setting_%s", key))),
				),
				markup.InlineButtonRow(
					markup.Callback("Back", []byte(fmt.Sprintf("back_settings_%d_true", page))),
					markup.Callback("Close", []byte("close_settings")),
				),
			)
			_ = h.editMessage(ctx, update.Peer, e, update.MsgID, fmt.Sprintf("Manage %s: Choose action.", key), kb)
			return true
		}

		kb := markup.InlineKeyboard(
			markup.InlineButtonRow(
				markup.Callback("Type", []byte(fmt.Sprintf("type_menu_%s", key))),
				markup.Callback("Clear", []byte(fmt.Sprintf("clear_setting_%s", key))),
			),
			markup.InlineButtonRow(
				markup.Callback("Back", []byte(fmt.Sprintf("back_settings_%d_true", page))),
				markup.Callback("Close", []byte("close_settings")),
			),
		)
		_ = h.editMessage(ctx, update.Peer, e, update.MsgID, fmt.Sprintf("Send new value for %s:", key), kb)
		return true

	case strings.HasPrefix(data, "back_settings_"):
		_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: update.QueryID})
		parts := strings.Split(data, "_")
		page := 0
		editMode := true
		if len(parts) >= 3 {
			if p, err := strconv.Atoi(parts[2]); err == nil {
				page = p
			}
		}
		if len(parts) >= 4 {
			editMode = strings.ToLower(parts[3]) == "true"
		}
		h.clearEditState(update.UserID)
		h.setLastPage(update.UserID, page)
		kb := h.getSettingsKeyboard(page, editMode)
		stateStr := "view"
		if editMode {
			stateStr = "edit"
		}
		_ = h.editMessage(ctx, update.Peer, e, update.MsgID, fmt.Sprintf("Config Variables | Page: %d | State: %s", page, stateStr), kb)
		return true

	case data == "close_settings":
		_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: update.QueryID})
		h.clearEditState(update.UserID)
		_, _ = w.API.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
			ID:     []int{update.MsgID},
			Revoke: true,
		})
		return true

	case data == "cookies":
		_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: update.QueryID})
		h.setCookieWait(update.UserID, &CookieWaitState{MsgID: update.MsgID, Peer: update.Peer, UpdatedAt: time.Now()})
		text := "Send .txt file for cookies storage\nExample: yt.txt"
		kb := markup.InlineKeyboard(
			markup.InlineButtonRow(
				markup.Callback("Back", []byte("refresh")),
			),
		)
		_ = h.editMessage(ctx, update.Peer, e, update.MsgID, text, kb)
		return true

	case data == "database":
		_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: update.QueryID})
		var usersCount, tracksCount, sourcesCount int64
		if h.db != nil {
			usersCount, _ = h.db.Collection("users").CountDocuments(ctx, bson.M{})
			tracksCount, _ = h.db.Collection("audioTracks").CountDocuments(ctx, bson.M{})
			sourcesCount, _ = h.db.Collection("allowedSources").CountDocuments(ctx, bson.M{})
		}
		text := fmt.Sprintf(
			"<b>Database Statistics</b>\n\n"+
				"• <b>Users:</b> %d\n"+
				"• <b>Tracks:</b> %d\n"+
				"• <b>Allowed Sources:</b> %d",
			usersCount, tracksCount, sourcesCount,
		)
		kb := markup.InlineKeyboard(
			markup.InlineButtonRow(
				markup.Callback("Back", []byte("refresh")),
			),
		)
		_ = h.editMessage(ctx, update.Peer, e, update.MsgID, text, kb)
		return true

	case data == "stats":
		_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: update.QueryID})
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		uptime := time.Since(h.startTime)
		readyWorkers := 0
		if h.tgService != nil {
			readyWorkers = h.tgService.ReadyWorkerCount()
		}
		text := fmt.Sprintf(
			"<b>System Statistics</b>\n\n"+
				"• <b>OS Uptime:</b> %s\n"+
				"• <b>Bot RAM:</b> %.2f MB\n"+
				"• <b>CPU Cores:</b> %d\n"+
				"• <b>Goroutines:</b> %d\n"+
				"• <b>Workers Ready:</b> %d\n"+
				"• <b>Go Version:</b> %s",
			formatDuration(uptime),
			float64(m.Alloc)/(1024*1024),
			runtime.NumCPU(),
			runtime.NumGoroutine(),
			readyWorkers,
			runtime.Version(),
		)
		kb := markup.InlineKeyboard(
			markup.InlineButtonRow(
				markup.Callback("Refresh", []byte("stats")),
				markup.Callback("Back", []byte("refresh")),
			),
		)
		_ = h.editMessage(ctx, update.Peer, e, update.MsgID, text, kb)
		return true

	case data == "refresh" || data == "back_main" || strings.HasPrefix(data, "back_main_"):
		_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: update.QueryID})
		h.clearEditState(update.UserID)
		h.renderAdminPanel(ctx, e, update.Peer, update.MsgID)
		return true
	}

	return false
}

// renderTypeMenu displays the type configuration keyboard for a setting.
func (h *Handler) renderTypeMenu(ctx context.Context, e tg.Entities, peer tg.PeerClass, msgID int, key string) {
	overrideType := "default"
	defaultType := "str"
	if h.cfgMgr != nil {
		overrideType = h.cfgMgr.GetOverrideType(key)
		defaultType = h.cfgMgr.GetDefaultType(key)
	}

	activeTypeStr := defaultType + " (default)"
	if overrideType != "default" {
		activeTypeStr = overrideType
	}

	text := fmt.Sprintf(
		"<b>Type Configuration for %s</b>\n\n"+
			"• Default type: <code>%s</code>\n"+
			"• Active type: <code>%s</code>\n\n"+
			"Choose a new type for this setting:",
		key, defaultType, activeTypeStr,
	)

	checkStr := func(t string) string {
		if overrideType == t {
			return " ✓"
		}
		return ""
	}

	kb := markup.InlineKeyboard(
		markup.InlineButtonRow(
			markup.Callback("str"+checkStr("str"), []byte(fmt.Sprintf("set_type_%s_str", key))),
			markup.Callback("int"+checkStr("int"), []byte(fmt.Sprintf("set_type_%s_int", key))),
		),
		markup.InlineButtonRow(
			markup.Callback("bool"+checkStr("bool"), []byte(fmt.Sprintf("set_type_%s_bool", key))),
			markup.Callback("list"+checkStr("list"), []byte(fmt.Sprintf("set_type_%s_list", key))),
		),
		markup.InlineButtonRow(
			markup.Callback("Default"+checkStr("default"), []byte(fmt.Sprintf("set_type_%s_default", key))),
		),
		markup.InlineButtonRow(
			markup.Callback("Back", []byte(fmt.Sprintf("edit_%s", key))),
			markup.Callback("Close", []byte("close_settings")),
		),
	)

	_ = h.editMessage(ctx, peer, e, msgID, text, kb)
}

// handleAdminTextInput captures message text sent by admins during interactive setting updates.
func (h *Handler) handleAdminTextInput(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64, text string) {
	state := h.getEditState(senderID)
	if state == nil {
		return
	}

	w := h.tgService.PrimaryWorker()
	if w == nil || w.API == nil {
		return
	}

	// Try to clean up admin input message
	_, _ = w.API.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
		ID:     []int{msg.ID},
		Revoke: true,
	})

	inputPeer := state.InputPeer
	if u, ok := e.Users[senderID]; ok && u != nil && u.AccessHash != 0 {
		inputPeer = u.AsInputPeer()
		w.SetUserAccessHash(senderID, u.AccessHash)
	} else if hash := w.GetUserAccessHash(senderID); hash != 0 {
		inputPeer = &tg.InputPeerUser{UserID: senderID, AccessHash: hash}
	}

	if state.Action == "add" || state.Action == "remove" {
		tokens := strings.FieldsFunc(text, func(r rune) bool {
			return r == ',' || r == ' ' || r == ';'
		})
		var ids []int64
		for _, t := range tokens {
			if n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64); err == nil {
				ids = append(ids, n)
			}
		}

		if len(ids) == 0 {
			kb := markup.InlineKeyboard(
				markup.InlineButtonRow(
					markup.Callback("Back", []byte(fmt.Sprintf("back_settings_%d_true", state.Page))),
					markup.Callback("Close", []byte("close_settings")),
				),
			)
			_ = h.editMessage(ctx, state.Peer, e, state.MsgID, "Invalid ID. Please try again.", kb)
			return
		}

		var err error
		if state.Action == "add" {
			err = h.cfgMgr.AddIDs(ctx, state.Key, ids)
		} else {
			err = h.cfgMgr.RemoveIDs(ctx, state.Key, ids)
		}

		if err != nil {
			kb := markup.InlineKeyboard(
				markup.InlineButtonRow(
					markup.Callback("Back", []byte(fmt.Sprintf("back_settings_%d_true", state.Page))),
					markup.Callback("Close", []byte("close_settings")),
				),
			)
			_ = h.editMessage(ctx, state.Peer, e, state.MsgID, fmt.Sprintf("Error: %s", html.EscapeString(err.Error())), kb)
			h.clearEditState(senderID)
			return
		}

		kb := h.getSettingsKeyboard(state.Page, true)
		var editErr error
		if inputPeer != nil {
			editErr = h.editMessageWithInputPeer(ctx, inputPeer, state.MsgID, fmt.Sprintf("Config Variables | Page: %d | State: edit", state.Page), kb)
		} else {
			editErr = h.editMessage(ctx, state.Peer, e, state.MsgID, fmt.Sprintf("Config Variables | Page: %d | State: edit", state.Page), kb)
		}
		if editErr != nil {
			_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Config Variables | Page: %d | State: edit", state.Page), kb)
		}
		h.clearEditState(senderID)
		return
	}

	// Direct setting update
	currentVal := h.cfgMgr.Get(state.Key)
	newProcessedVal, err := h.cfgMgr.UpdateConfig(ctx, state.Key, text)
	if err != nil {
		log.Warnf("UpdateConfig failed for %s with val %q: %v", state.Key, text, err)
		kb := markup.InlineKeyboard(
			markup.InlineButtonRow(
				markup.Callback("Back", []byte(fmt.Sprintf("back_settings_%d_%t", state.Page, state.EditMode))),
				markup.Callback("Close", []byte("close_settings")),
			),
		)
		errText := fmt.Sprintf("Error updating setting: %s", html.EscapeString(err.Error()))
		var editErr error
		if inputPeer != nil {
			editErr = h.editMessageWithInputPeer(ctx, inputPeer, state.MsgID, errText, kb)
		} else {
			editErr = h.editMessage(ctx, state.Peer, e, state.MsgID, errText, kb)
		}
		if editErr != nil {
			_ = h.replyHTML(ctx, e, upd, errText, kb)
		}
		h.clearEditState(senderID)
		return
	}

	currentValStr := html.EscapeString(fmt.Sprintf("%v", currentVal))
	newValStr := html.EscapeString(fmt.Sprintf("%v", newProcessedVal))
	feedback := fmt.Sprintf("Updated %s:\nOld value: %s\nNew value: %s", state.Key, currentValStr, newValStr)
	if reflect.TypeOf(currentVal) != reflect.TypeOf(newProcessedVal) {
		oldType := "nil"
		if currentVal != nil {
			oldType = reflect.TypeOf(currentVal).String()
		}
		newType := "nil"
		if newProcessedVal != nil {
			newType = reflect.TypeOf(newProcessedVal).String()
		}
		feedback += fmt.Sprintf("\n\nNote: Value type changed from %s to %s", oldType, newType)
	}

	stateStr := "view"
	if state.EditMode {
		stateStr = "edit"
	}

	panelText := fmt.Sprintf("Config Variables | Page: %d | State: %s\n\n%s", state.Page, stateStr, feedback)
	kb := h.getSettingsKeyboard(state.Page, state.EditMode)

	var editErr error
	if inputPeer != nil {
		editErr = h.editMessageWithInputPeer(ctx, inputPeer, state.MsgID, panelText, kb)
	} else {
		editErr = h.editMessage(ctx, state.Peer, e, state.MsgID, panelText, kb)
	}
	if editErr != nil {
		log.Warnf("Failed to edit panel message %d, falling back to replyHTML: %v", state.MsgID, editErr)
		_ = h.replyHTML(ctx, e, upd, panelText, kb)
	}
	h.clearEditState(senderID)
}

// handleCookieDocument processes cookie files uploaded by admins.
func (h *Handler) handleCookieDocument(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64, doc *tg.Document) bool {
	h.editMu.Lock()
	waitState, ok := h.cookieWait[senderID]
	h.editMu.Unlock()
	if !ok || waitState == nil {
		return false
	}

	var fileName string
	for _, attr := range doc.Attributes {
		if fn, ok := attr.(*tg.DocumentAttributeFilename); ok {
			fileName = fn.FileName
			break
		}
	}
	if !strings.HasSuffix(strings.ToLower(fileName), ".txt") {
		_ = h.replyHTML(ctx, e, upd, "Only .txt files are allowed for cookies storage.", nil)
		return true
	}

	_ = os.MkdirAll("cookies", 0755)
	destPath := filepath.Join("cookies", filepath.Base(fileName))
	f, err := os.Create(destPath)
	if err != nil {
		_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Failed to save cookies: %v", err), nil)
		return true
	}
	defer f.Close()

	loc := &tg.InputDocumentFileLocation{
		ID:            doc.ID,
		AccessHash:    doc.AccessHash,
		FileReference: doc.FileReference,
	}
	_ = h.tgService.DownloadPartial(ctx, loc, 0, f)

	h.clearEditState(senderID)
	_ = h.replyHTML(ctx, e, upd, fmt.Sprintf("Cookies file saved: <code>%s</code>", html.EscapeString(destPath)), nil)
	h.renderAdminPanel(ctx, e, waitState.Peer, waitState.MsgID)
	return true
}

// handleFileID extracts and encodes Telegram file_id from replied media.
func (h *Handler) handleFileID(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64) {
	if !h.isAdmin(senderID) {
		_ = h.replyHTML(ctx, e, upd, "Access denied.", nil)
		return
	}

	var targetDoc *tg.Document
	if msg.Media != nil {
		if docMedia, ok := msg.Media.(*tg.MessageMediaDocument); ok && docMedia.Document != nil {
			if doc, ok := docMedia.Document.(*tg.Document); ok {
				targetDoc = doc
			}
		}
	}

	if targetDoc == nil {
		_ = h.replyHTML(ctx, e, upd, "Please reply to an image, video, document, or GIF to get its file ID.", nil)
		return
	}

	primaryID := telegram.EncodeFileID(targetDoc.ID, targetDoc.AccessHash, int32(targetDoc.DCID), targetDoc.FileReference)
	reply := fmt.Sprintf("<b>File ID:</b>\n<code>%s</code>", primaryID)
	_ = h.replyHTML(ctx, e, upd, reply, nil)
}

// handleLogs outputs recent server log lines.
func (h *Handler) handleLogs(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64) {
	if !h.isAdmin(senderID) {
		_ = h.replyHTML(ctx, e, upd, "Access denied.", nil)
		return
	}

	logPaths := []string{"log.txt", "serverlogs.txt"}
	var lines []string

	for _, p := range logPaths {
		if data, err := os.ReadFile(p); err == nil {
			all := strings.Split(string(data), "\n")
			if len(all) > 40 {
				lines = all[len(all)-40:]
			} else {
				lines = all
			}
			break
		}
	}

	if len(lines) == 0 {
		_ = h.replyHTML(ctx, e, upd, "Log file not found!", nil)
		return
	}

	snippet := strings.Join(lines, "\n")
	if len(snippet) > 990 {
		snippet = snippet[len(snippet)-990:]
	}

	reply := fmt.Sprintf("<b>Recent Logs (1,024 char limit):</b>\n<pre>%s</pre>", html.EscapeString(snippet))
	_ = h.replyHTML(ctx, e, upd, reply, nil)
}

// handleRestart initiates restart flow with confirmation buttons matching StreamXBot.
func (h *Handler) handleRestart(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message, senderID int64) {
	if !h.isAdmin(senderID) {
		_ = h.replyHTML(ctx, e, upd, "Access denied.", nil)
		return
	}

	kb := markup.InlineKeyboard(
		markup.InlineButtonRow(
			markup.Callback("Restart Only", []byte("confirm_restart restart")),
			markup.Callback("Update & Restart", []byte("confirm_restart update")),
		),
		markup.InlineButtonRow(
			markup.Callback("Cancel", []byte("confirm_restart cancel")),
		),
	)

	_ = h.replyHTML(ctx, e, upd, "How would you like to proceed?", kb)
}

// handleRestartConfirmation handles button callbacks for restart confirmation.
func (h *Handler) handleRestartConfirmation(ctx context.Context, e tg.Entities, update *tg.UpdateBotCallbackQuery) {
	if update == nil {
		return
	}

	data := string(update.Data)
	parts := strings.Split(data, " ")
	choice := "restart"
	if len(parts) > 1 {
		choice = parts[1]
	}

	w := h.tgService.PrimaryWorker()
	if w == nil || w.API == nil {
		return
	}

	if choice == "cancel" {
		_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{
			QueryID: update.QueryID,
			Message: "Cancelled",
		})
		_, _ = w.API.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
			ID:     []int{update.MsgID},
			Revoke: true,
		})
		return
	}

	_, _ = w.API.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{
		QueryID: update.QueryID,
		Message: "Restarting...",
	})

	_ = h.editMessage(ctx, update.Peer, e, update.MsgID, "Restarting...", nil)

	go func() {
		time.Sleep(1 * time.Second)
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	}()
}
