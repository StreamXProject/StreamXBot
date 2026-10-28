package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/entity"
	tgHtml "github.com/gotd/td/telegram/message/html"
	"github.com/gotd/td/tg"

	"streamgo/internal/config"
	"streamgo/internal/database"
	"streamgo/internal/logger"
	"streamgo/internal/models"
	"streamgo/internal/telegram"
)

var log = logger.New("telegramcmd")

// AuthHelper defines authentication methods required by bot commands.
type AuthHelper interface {
	AuthenticateOrRegisterTgUser(ctx context.Context, tgUserID int64, name, username, photoURL, inviteCode string) (*models.User, string, error)
	ConfirmBotSession(ctx context.Context, sessionID, token string, user *models.User) error
}

// AccessControlHelper defines access policy and administration methods required by bot commands.
type AccessControlHelper interface {
	GetPolicy(ctx context.Context) (*models.AccessPolicy, error)
	UpdatePolicy(ctx context.Context, patch models.PolicyPatch, adminID int64) (*models.AccessPolicy, error)
	AddRequiredChat(ctx context.Context, chat models.RequiredChat) (*models.AccessPolicy, error)
	RemoveRequiredChat(ctx context.Context, chatID int64) (*models.AccessPolicy, error)
	CreateInvite(ctx context.Context, adminID int64, maxUses int, ttlDays int, note string) (*models.InviteCode, error)
	ListInvites(ctx context.Context, includeDead bool) ([]*models.InviteCode, error)
	LockUser(ctx context.Context, userID int64, reason string, adminID int64, revokeSessions bool) error
	UnlockUser(ctx context.Context, userID int64) error
	RevokeSessions(ctx context.Context, userID int64) (int, error)
	AddToBypass(ctx context.Context, userID int64, adminID int64, note string) error
	RemoveFromBypass(ctx context.Context, userID int64) bool
	ListBypass(ctx context.Context) ([]*models.AllowlistEntry, error)
}

// AccessFilterHelper defines source allow/ban and filter mode methods required by bot commands.
type AccessFilterHelper interface {
	GetStats() (mode int, modeName string, allowedCount int, bannedCount int, channelID int64)
	GetMode() int
	SetMode(mode int)
	AllowChat(ctx context.Context, chatID int64) error
	DisallowChat(ctx context.Context, chatID int64) error
	BanChat(ctx context.Context, id int64, reason string) error
	UnbanChat(ctx context.Context, id int64) error
}

// TrackSearchHelper defines track search queries for /search command.
type TrackSearchHelper interface {
	Search(ctx context.Context, query string, limit int) ([]*models.BrowseItem, error)
}

// AdminEditState tracks in-flight interactive setting modifications.
type AdminEditState struct {
	Key       string
	MsgID     int
	Page      int
	EditMode  bool
	Action    string // "add", "remove", or ""
	Peer      tg.PeerClass
	InputPeer tg.InputPeerClass
	UpdatedAt time.Time
}

// CookieWaitState tracks admins who clicked the Cookies button and are expected to upload a .txt file.
type CookieWaitState struct {
	MsgID     int
	Peer      tg.PeerClass
	UpdatedAt time.Time
}

// Handler coordinates Telegram bot command routing and interactive inline queries.
type Handler struct {
	cfg          *config.Config
	cfgMgr       *config.ConfigManager
	db           *database.Client
	tgService    *telegram.Service
	listener     *telegram.IngestionListener
	authSvc      AuthHelper
	accessCtrl   AccessControlHelper
	accessFilter AccessFilterHelper
	trackSvc     TrackSearchHelper
	startTime    time.Time
	editMu       sync.Mutex
	editStates   map[int64]*AdminEditState
	lastPages    map[int64]int
	cookieWait   map[int64]*CookieWaitState
}

// New creates a new command Handler.
func New(
	cfgMgr *config.ConfigManager,
	db *database.Client,
	tgService *telegram.Service,
	listener *telegram.IngestionListener,
	authSvc AuthHelper,
	accessCtrl AccessControlHelper,
	accessFilter AccessFilterHelper,
	trackSvc TrackSearchHelper,
) *Handler {
	var cfg *config.Config
	if cfgMgr != nil {
		cfg = cfgMgr.Config()
	}
	return &Handler{
		cfg:          cfg,
		cfgMgr:       cfgMgr,
		db:           db,
		tgService:    tgService,
		listener:     listener,
		authSvc:      authSvc,
		accessCtrl:   accessCtrl,
		accessFilter: accessFilter,
		trackSvc:     trackSvc,
		startTime:    time.Now(),
		editStates:   make(map[int64]*AdminEditState),
		lastPages:    make(map[int64]int),
		cookieWait:   make(map[int64]*CookieWaitState),
	}
}

// SetupDispatcher binds message routers and inline callback query handlers to the update dispatcher.
func (h *Handler) SetupDispatcher(d *tg.UpdateDispatcher) {
	d.OnNewMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		if msg, ok := u.Message.(*tg.Message); ok {
			h.routeMessage(ctx, e, u, msg)
		}
		return nil
	})

	d.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
		if msg, ok := u.Message.(*tg.Message); ok {
			h.routeMessage(ctx, e, u, msg)
		}
		return nil
	})

	d.OnBotCallbackQuery(func(ctx context.Context, e tg.Entities, u *tg.UpdateBotCallbackQuery) error {
		h.handleCallbackQuery(ctx, e, u)
		return nil
	})
}

// routeMessage routes an incoming message to the matching command or audio listener.
func (h *Handler) routeMessage(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, msg *tg.Message) {
	if msg == nil {
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

	text := strings.TrimSpace(msg.Message)
	chatID, senderID, _, _ := h.extractPeerInfo(msg, e)

	if text != "" {
		log.Debugf("[telegram] Received message from user %d in chat %d: %q", senderID, chatID, text)
	}

	// Check if this message is an audio upload
	isAudio := false
	if msg.Media != nil {
		if docMedia, ok := msg.Media.(*tg.MessageMediaDocument); ok && docMedia.Document != nil {
			if doc, ok := docMedia.Document.(*tg.Document); ok {
				mime := strings.ToLower(doc.MimeType)
				hasAudioAttr := false
				var fileName string
				for _, attr := range doc.Attributes {
					switch a := attr.(type) {
					case *tg.DocumentAttributeAudio:
						hasAudioAttr = true
					case *tg.DocumentAttributeFilename:
						fileName = strings.ToLower(strings.TrimSpace(a.FileName))
					}
				}

				isAudioExt := strings.HasSuffix(fileName, ".flac") || strings.HasSuffix(fileName, ".mp3") ||
					strings.HasSuffix(fileName, ".wav") || strings.HasSuffix(fileName, ".wave") ||
					strings.HasSuffix(fileName, ".m4a") || strings.HasSuffix(fileName, ".alac") ||
					strings.HasSuffix(fileName, ".aac") || strings.HasSuffix(fileName, ".ogg") ||
					strings.HasSuffix(fileName, ".opus") || strings.HasSuffix(fileName, ".aif") ||
					strings.HasSuffix(fileName, ".aiff") || strings.HasSuffix(fileName, ".wma")

				if strings.HasPrefix(mime, "audio/") || hasAudioAttr || isAudioExt ||
					mime == "application/x-flac" || mime == "application/flac" || mime == "application/ogg" {
					isAudio = true
				}

				// Check if admin is uploading a cookie .txt file
				if h.isAdmin(senderID) && h.handleCookieDocument(ctx, e, upd, msg, senderID, doc) {
					return
				}
			}
		}
	}

	// Intercept admin text responses when in active edit state
	if h.isAdmin(senderID) {
		if text == "/cancel" {
			h.clearEditState(senderID)
			_ = h.replyHTML(ctx, e, upd, "Action cancelled.", nil)
			return
		}
		if !strings.HasPrefix(text, "/") && text != "" {
			if state := h.getEditState(senderID); state != nil {
				h.handleAdminTextInput(ctx, e, upd, msg, senderID, text)
				return
			}
		}
	}

	// Normal text / audio uploads without a leading '/' command are forwarded to ingestion listener
	if !strings.HasPrefix(text, "/") {
		if isAudio && h.listener != nil {
			h.listener.HandleMessage(ctx, msg)
		}
		return
	}

	// If admin entered a new command, clear any pending edit prompt state
	if h.isAdmin(senderID) {
		h.clearEditState(senderID)
	}

	parts := strings.Fields(text)
	if len(parts) == 0 {
		return
	}

	rawCmd := parts[0][1:] // strip '/'
	cmdName := strings.ToLower(strings.Split(rawCmd, "@")[0])
	args := parts[1:]

	// Ingest audio concurrently if command is not a media inspector
	if isAudio && h.listener != nil && cmdName != "mediainfo" && cmdName != "mi" {
		h.listener.HandleMessage(ctx, msg)
	}

	switch cmdName {
	case "start", "help":
		h.handleStartOrHelp(ctx, e, upd, msg, args)
	case "ping", "alive":
		h.handlePing(ctx, e, upd, msg)
	case "id":
		h.handleID(ctx, e, upd, msg, chatID, senderID)
	case "search":
		h.handleSearch(ctx, e, upd, msg, args)
	case "mediainfo", "mi":
		h.handleMediaInfo(ctx, e, upd, msg)
	case "sources":
		h.handleSources(ctx, e, upd, msg, senderID)
	case "filter_mode", "filtermode":
		h.handleFilterMode(ctx, e, upd, msg, senderID, args)
	case "allow", "allow_source", "allowsource":
		h.handleAllow(ctx, e, upd, msg, senderID, args)
	case "disallow", "disallow_source", "disallowsource":
		h.handleDisallow(ctx, e, upd, msg, senderID, args)
	case "ban", "ban_source", "bansources", "bansource":
		h.handleBan(ctx, e, upd, msg, senderID, args)
	case "unban", "unban_source", "unbansources", "unbansource":
		h.handleUnban(ctx, e, upd, msg, senderID, args)
	case "access":
		h.handleAccess(ctx, e, upd, msg, senderID)
	case "registration":
		h.handleRegistration(ctx, e, upd, msg, senderID, args)
	case "membership":
		h.handleMembership(ctx, e, upd, msg, senderID, args)
	case "requirechat":
		h.handleRequireChat(ctx, e, upd, msg, senderID, args)
	case "unrequirechat":
		h.handleUnrequireChat(ctx, e, upd, msg, senderID, args)
	case "invite":
		h.handleInvite(ctx, e, upd, msg, senderID, args)
	case "invites":
		h.handleInvites(ctx, e, upd, msg, senderID)
	case "lock":
		h.handleLock(ctx, e, upd, msg, senderID, args)
	case "unlock":
		h.handleUnlock(ctx, e, upd, msg, senderID, args)
	case "revoke":
		h.handleRevoke(ctx, e, upd, msg, senderID, args)
	case "bypass", "exempt":
		h.handleBypass(ctx, e, upd, msg, senderID, args)
	case "unbypass", "unexempt":
		h.handleUnbypass(ctx, e, upd, msg, senderID, args)
	case "bypasses", "bypassed":
		h.handleBypasses(ctx, e, upd, msg, senderID)
	case "fileid":
		h.handleFileID(ctx, e, upd, msg, senderID)
	case "log", "logs":
		h.handleLogs(ctx, e, upd, msg, senderID)
	case "sudo":
		h.handleSudo(ctx, e, upd, msg, senderID)
	case "bs":
		h.handleBS(ctx, e, upd, msg, senderID)
	case "restart":
		h.handleRestart(ctx, e, upd, msg, senderID)
	}
}

func (h *Handler) isOwner(userID int64) bool {
	if h.cfgMgr != nil {
		if raw := h.cfgMgr.Get("OWNER_ID"); raw != nil {
			switch list := raw.(type) {
			case []int64:
				for _, id := range list {
					if id == userID {
						return true
					}
				}
			case []int:
				for _, id := range list {
					if int64(id) == userID {
						return true
					}
				}
			}
		}
	}
	if h.cfg != nil {
		for _, id := range h.cfg.OwnerIDs {
			if id == userID {
				return true
			}
		}
	}
	return false
}

func (h *Handler) isAdmin(userID int64) bool {
	if h.isOwner(userID) {
		return true
	}
	if h.cfgMgr != nil {
		if raw := h.cfgMgr.Get("SUDO_USERS"); raw != nil {
			switch list := raw.(type) {
			case []int64:
				for _, id := range list {
					if id == userID {
						return true
					}
				}
			case []int:
				for _, id := range list {
					if int64(id) == userID {
						return true
					}
				}
			}
		}
	}
	if h.cfg != nil {
		for _, id := range h.cfg.SudoUsers {
			if id == userID {
				return true
			}
		}
	}
	return false
}

func (h *Handler) getEditState(userID int64) *AdminEditState {
	h.editMu.Lock()
	defer h.editMu.Unlock()
	return h.editStates[userID]
}

func (h *Handler) setEditState(userID int64, st *AdminEditState) {
	h.editMu.Lock()
	defer h.editMu.Unlock()
	h.editStates[userID] = st
}

func (h *Handler) clearEditState(userID int64) {
	h.editMu.Lock()
	defer h.editMu.Unlock()
	delete(h.editStates, userID)
	delete(h.cookieWait, userID)
}

func (h *Handler) getLastPage(userID int64) int {
	h.editMu.Lock()
	defer h.editMu.Unlock()
	return h.lastPages[userID]
}

func (h *Handler) setLastPage(userID int64, page int) {
	h.editMu.Lock()
	defer h.editMu.Unlock()
	h.lastPages[userID] = page
}

func (h *Handler) setCookieWait(userID int64, st *CookieWaitState) {
	h.editMu.Lock()
	defer h.editMu.Unlock()
	h.cookieWait[userID] = st
}

func (h *Handler) extractPeerInfo(msg *tg.Message, e tg.Entities) (chatID int64, senderID int64, senderName string, senderUsername string) {
	switch p := msg.PeerID.(type) {
	case *tg.PeerChannel:
		chatID = -1000000000000 - p.ChannelID
	case *tg.PeerChat:
		chatID = -p.ChatID
	case *tg.PeerUser:
		chatID = p.UserID
	}

	if msg.FromID != nil {
		switch p := msg.FromID.(type) {
		case *tg.PeerUser:
			senderID = p.UserID
		case *tg.PeerChannel:
			senderID = -1000000000000 - p.ChannelID
		}
	} else if chatID > 0 {
		senderID = chatID
	}

	if u, ok := e.Users[senderID]; ok {
		senderName = u.FirstName
		if u.LastName != "" {
			senderName = strings.TrimSpace(senderName + " " + u.LastName)
		}
		senderUsername = u.Username
		if w := h.tgService.PrimaryWorker(); w != nil && u.AccessHash != 0 {
			w.SetUserAccessHash(senderID, u.AccessHash)
		}
	}

	return
}

func (h *Handler) replyHTML(ctx context.Context, e tg.Entities, upd message.AnswerableMessageUpdate, htmlContent string, replyMarkup tg.ReplyMarkupClass) error {
	w := h.tgService.PrimaryWorker()
	if w == nil || w.API == nil {
		return errors.New("no ready Telegram worker")
	}

	sender := message.NewSender(w.API)
	b := sender.Reply(e, upd)
	if replyMarkup != nil {
		b = b.Markup(replyMarkup)
	}

	_, err := b.StyledText(ctx, tgHtml.String(nil, htmlContent))
	if err != nil {
		// Fallback to sending as answer to the chat if reply fails
		b2 := sender.Answer(e, upd).CloneBuilder()
		if replyMarkup != nil {
			b2 = b2.Markup(replyMarkup)
		}
		_, err = b2.StyledText(ctx, tgHtml.String(nil, htmlContent))
	}
	return err
}

func (h *Handler) editMessage(ctx context.Context, peer tg.PeerClass, e tg.Entities, msgID int, htmlContent string, replyMarkup tg.ReplyMarkupClass) error {
	w := h.tgService.PrimaryWorker()
	if w == nil || w.API == nil {
		return errors.New("no ready Telegram worker")
	}

	inputPeer := extractInputPeer(peer, e, w)
	if inputPeer == nil {
		return errors.New("could not resolve input peer")
	}

	return h.editMessageWithInputPeer(ctx, inputPeer, msgID, htmlContent, replyMarkup)
}

func (h *Handler) editMessageWithInputPeer(ctx context.Context, inputPeer tg.InputPeerClass, msgID int, htmlContent string, replyMarkup tg.ReplyMarkupClass) error {
	w := h.tgService.PrimaryWorker()
	if w == nil || w.API == nil {
		return errors.New("no ready Telegram worker")
	}
	if inputPeer == nil {
		return errors.New("input peer is nil")
	}

	text, entities := parseHTML(htmlContent)
	req := &tg.MessagesEditMessageRequest{
		Peer:        inputPeer,
		ID:          msgID,
		Message:     text,
		Entities:    entities,
		ReplyMarkup: replyMarkup,
		NoWebpage:   true,
	}

	_, err := w.API.MessagesEditMessage(ctx, req)
	if err != nil {
		log.Warnf("MessagesEditMessage failed (peer: %v, msgID: %d): %v", inputPeer, msgID, err)
	}
	return err
}

func extractInputPeer(p tg.PeerClass, e tg.Entities, w *telegram.ClientWorker) tg.InputPeerClass {
	if p == nil {
		return nil
	}
	switch peer := p.(type) {
	case *tg.PeerUser:
		if u, ok := e.Users[peer.UserID]; ok {
			if w != nil {
				w.SetUserAccessHash(peer.UserID, u.AccessHash)
			}
			return u.AsInputPeer()
		}
		accessHash := int64(0)
		if w != nil {
			accessHash = w.GetUserAccessHash(peer.UserID)
		}
		return &tg.InputPeerUser{UserID: peer.UserID, AccessHash: accessHash}
	case *tg.PeerChat:
		if c, ok := e.Chats[peer.ChatID]; ok {
			return c.AsInputPeer()
		}
		return &tg.InputPeerChat{ChatID: peer.ChatID}
	case *tg.PeerChannel:
		if ch, ok := e.Channels[peer.ChannelID]; ok {
			return ch.AsInputPeer()
		}
		accessHash := int64(0)
		if w != nil {
			accessHash = w.GetChannelAccessHash(peer.ChannelID)
		}
		return &tg.InputPeerChannel{ChannelID: peer.ChannelID, AccessHash: accessHash}
	}
	return nil
}

func parseHTML(raw string) (string, []tg.MessageEntityClass) {
	var b entity.Builder
	if err := tgHtml.HTML(strings.NewReader(raw), &b, tgHtml.Options{}); err != nil {
		return raw, nil
	}
	return b.Complete()
}

func formatDuration(d time.Duration) string {
	seconds := int(d.Seconds())
	days := seconds / 86400
	remainder := seconds % 86400
	hours := remainder / 3600
	remainder = remainder % 3600
	minutes := remainder / 60
	secs := remainder % 60

	var result string
	if days != 0 {
		result += fmt.Sprintf("%dd ", days)
	}
	if hours != 0 {
		result += fmt.Sprintf("%dh ", hours)
	}
	if minutes != 0 {
		result += fmt.Sprintf("%dm ", minutes)
	}
	result += fmt.Sprintf("%ds ", secs)
	return strings.TrimSpace(result)
}

func formatSeconds(sec int32) string {
	if sec <= 0 {
		return "0:00"
	}
	m := sec / 60
	s := sec % 60
	return fmt.Sprintf("%d:%02d", m, s)
}
