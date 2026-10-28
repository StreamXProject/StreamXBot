package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"streamgo/internal/api"
	"streamgo/internal/api/middleware"
	"streamgo/internal/config"
	"streamgo/internal/models"
	"streamgo/internal/services"
)

// AccessControlHandler handles administrative access policy, Telegram membership, invites, and user control.
type AccessControlHandler struct {
	accessSvc  *services.AccessControlService
	authSvc    *services.AuthService
	cfg        *config.Config
	httpClient *http.Client
}

// NewAccessControlHandler creates an AccessControlHandler.
func NewAccessControlHandler(accessSvc *services.AccessControlService, authSvc *services.AuthService, cfg *config.Config) *AccessControlHandler {
	return &AccessControlHandler{
		accessSvc:  accessSvc,
		authSvc:    authSvc,
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 8 * time.Second},
	}
}

// Routes mounts /admin/access endpoints.
func (h *AccessControlHandler) Routes(r chi.Router) {
	optionalAuth := middleware.OptionalAuth(h.authSvc)

	r.Route("/admin/access", func(sub chi.Router) {
		sub.Use(optionalAuth)

		// Policy
		sub.Get("/policy", h.GetPolicy)
		sub.Patch("/policy", h.PatchPolicy)

		// Required Telegram chats
		sub.Post("/required-chats", h.AddRequiredChat)
		sub.Delete("/required-chats/{chat_id}", h.RemoveRequiredChat)
		sub.Delete("/required-chats", h.RemoveRequiredChat)

		// Invite codes
		sub.Get("/invites", h.ListInvites)
		sub.Post("/invites", h.CreateInvite)
		sub.Delete("/invites/{code}", h.RevokeInvite)
		sub.Delete("/invites", h.RevokeInvite)

		// Allowlist
		sub.Get("/allowlist", h.ListAllowlist)
		sub.Post("/allowlist", h.AddAllowlist)
		sub.Delete("/allowlist/{user_id}", h.RemoveAllowlist)
		sub.Delete("/allowlist", h.RemoveAllowlist)

		// Bypass
		sub.Get("/bypass", h.ListBypass)
		sub.Post("/bypass", h.AddBypass)
		sub.Delete("/bypass/{user_id}", h.RemoveBypass)
		sub.Delete("/bypass", h.RemoveBypass)

		// User accounts
		sub.Get("/users", h.ListUsers)
		sub.Post("/users/{id}/lock", h.LockUser)
		sub.Post("/users/{id}/unlock", h.UnlockUser)
		sub.Post("/users/{id}/revoke-sessions", h.RevokeSessions)

		// Reverify Telegram membership
		sub.Post("/reverify", h.Reverify)
	})
}

// checkAdmin verifies that the caller has admin permissions (owner, sudo, or valid guest password).
func (h *AccessControlHandler) checkAdmin(r *http.Request) (int64, error) {
	if middleware.IsGuest(r.Context()) {
		return 0, nil
	}

	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID <= 0 {
		return 0, fmt.Errorf("authentication required")
	}

	if len(h.cfg.OwnerIDs) == 0 && len(h.cfg.SudoUsers) == 0 {
		return userID, nil
	}

	for _, oid := range h.cfg.OwnerIDs {
		if oid == userID {
			return userID, nil
		}
	}
	for _, sid := range h.cfg.SudoUsers {
		if sid == userID {
			return userID, nil
		}
	}

	return 0, fmt.Errorf("forbidden: administrator privileges required")
}

// GetPolicy handles GET /admin/access/policy.
func (h *AccessControlHandler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	if _, err := h.checkAdmin(r); err != nil {
		api.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	policy, err := h.accessSvc.GetPolicy(r.Context())
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"policy": policy,
	})
}

// PatchPolicy handles PATCH /admin/access/policy.
func (h *AccessControlHandler) PatchPolicy(w http.ResponseWriter, r *http.Request) {
	adminID, err := h.checkAdmin(r)
	if err != nil {
		api.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	var patch models.PolicyPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	updated, err := h.accessSvc.UpdatePolicy(r.Context(), patch, adminID)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"policy": updated,
	})
}

// AddRequiredChat handles POST /admin/access/required-chats.
func (h *AccessControlHandler) AddRequiredChat(w http.ResponseWriter, r *http.Request) {
	if _, err := h.checkAdmin(r); err != nil {
		api.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	var chat models.RequiredChat
	if err := json.NewDecoder(r.Body).Decode(&chat); err != nil || chat.ChatID == 0 {
		api.RespondError(w, http.StatusBadRequest, "chat_id is required")
		return
	}

	policy, err := h.accessSvc.AddRequiredChat(r.Context(), chat)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"required_chats": policy.RequiredChats,
	})
}

// RemoveRequiredChat handles DELETE /admin/access/required-chats/{chat_id}.
func (h *AccessControlHandler) RemoveRequiredChat(w http.ResponseWriter, r *http.Request) {
	if _, err := h.checkAdmin(r); err != nil {
		api.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	chatIDStr := chi.URLParam(r, "chat_id")
	if chatIDStr == "" {
		chatIDStr = r.URL.Query().Get("chat_id")
	}

	chatID, _ := strconv.ParseInt(chatIDStr, 10, 64)
	if chatID == 0 {
		api.RespondError(w, http.StatusBadRequest, "chat_id is required")
		return
	}

	policy, err := h.accessSvc.RemoveRequiredChat(r.Context(), chatID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"required_chats": policy.RequiredChats,
	})
}

// ListInvites handles GET /admin/access/invites.
func (h *AccessControlHandler) ListInvites(w http.ResponseWriter, r *http.Request) {
	if _, err := h.checkAdmin(r); err != nil {
		api.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	includeDead := api.ParseQueryBool(r, "include_dead", false)
	invites, err := h.accessSvc.ListInvites(r.Context(), includeDead)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"invites": invites,
	})
}

// CreateInvite handles POST /admin/access/invites.
func (h *AccessControlHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	adminID, err := h.checkAdmin(r)
	if err != nil {
		api.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	var req models.InviteCreateRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	inv, err := h.accessSvc.CreateInvite(r.Context(), adminID, req.MaxUses, req.TTLDays, req.Note)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"invite": inv,
	})
}

// RevokeInvite handles DELETE /admin/access/invites/{code}.
func (h *AccessControlHandler) RevokeInvite(w http.ResponseWriter, r *http.Request) {
	if _, err := h.checkAdmin(r); err != nil {
		api.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	code := chi.URLParam(r, "code")
	if code == "" {
		code = r.URL.Query().Get("code")
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		api.RespondError(w, http.StatusBadRequest, "invite code is required")
		return
	}

	revoked := h.accessSvc.RevokeInvite(r.Context(), code)
	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"revoked": revoked,
		"code":    code,
	})
}

// ListAllowlist handles GET /admin/access/allowlist.
func (h *AccessControlHandler) ListAllowlist(w http.ResponseWriter, r *http.Request) {
	if _, err := h.checkAdmin(r); err != nil {
		api.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	list, err := h.accessSvc.ListAllowlist(r.Context())
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"allowlist": list,
	})
}

// AddAllowlist handles POST /admin/access/allowlist.
func (h *AccessControlHandler) AddAllowlist(w http.ResponseWriter, r *http.Request) {
	adminID, err := h.checkAdmin(r)
	if err != nil {
		api.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	var req models.AllowlistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID <= 0 {
		api.RespondError(w, http.StatusBadRequest, "valid user_id is required")
		return
	}

	if err := h.accessSvc.AddToAllowlist(r.Context(), req.UserID, adminID, req.Note); err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"user_id": req.UserID,
		"message": "User added to allowlist",
	})
}

// RemoveAllowlist handles DELETE /admin/access/allowlist/{user_id}.
func (h *AccessControlHandler) RemoveAllowlist(w http.ResponseWriter, r *http.Request) {
	if _, err := h.checkAdmin(r); err != nil {
		api.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	uidStr := chi.URLParam(r, "user_id")
	if uidStr == "" {
		uidStr = r.URL.Query().Get("user_id")
	}
	uid, _ := strconv.ParseInt(uidStr, 10, 64)
	if uid <= 0 {
		api.RespondError(w, http.StatusBadRequest, "valid user_id is required")
		return
	}

	removed := h.accessSvc.RemoveFromAllowlist(r.Context(), uid)
	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"removed": removed,
		"user_id": uid,
	})
}

// ListBypass handles GET /admin/access/bypass.
func (h *AccessControlHandler) ListBypass(w http.ResponseWriter, r *http.Request) {
	if _, err := h.checkAdmin(r); err != nil {
		api.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	list, err := h.accessSvc.ListBypass(r.Context())
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"bypass": list,
	})
}

// AddBypass handles POST /admin/access/bypass.
func (h *AccessControlHandler) AddBypass(w http.ResponseWriter, r *http.Request) {
	adminID, err := h.checkAdmin(r)
	if err != nil {
		api.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	var req models.AllowlistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID <= 0 {
		api.RespondError(w, http.StatusBadRequest, "valid user_id is required")
		return
	}

	if err := h.accessSvc.AddToBypass(r.Context(), req.UserID, adminID, req.Note); err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"user_id": req.UserID,
		"message": "User added to bypass list",
	})
}

// RemoveBypass handles DELETE /admin/access/bypass/{user_id}.
func (h *AccessControlHandler) RemoveBypass(w http.ResponseWriter, r *http.Request) {
	if _, err := h.checkAdmin(r); err != nil {
		api.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	uidStr := chi.URLParam(r, "user_id")
	if uidStr == "" {
		uidStr = r.URL.Query().Get("user_id")
	}
	uid, _ := strconv.ParseInt(uidStr, 10, 64)
	if uid <= 0 {
		api.RespondError(w, http.StatusBadRequest, "valid user_id is required")
		return
	}

	removed := h.accessSvc.RemoveFromBypass(r.Context(), uid)
	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"removed": removed,
		"user_id": uid,
	})
}

// ListUsers handles GET /admin/access/users.
func (h *AccessControlHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if _, err := h.checkAdmin(r); err != nil {
		api.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	query := r.URL.Query().Get("q")
	status := r.URL.Query().Get("status")
	limit := api.ParseQueryInt(r, "limit", 50)
	skip := api.ParseQueryInt(r, "skip", 0)

	resp, err := h.accessSvc.ListUsers(r.Context(), query, status, limit, skip)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"data": resp,
	})
}

// LockUser handles POST /admin/access/users/{id}/lock.
func (h *AccessControlHandler) LockUser(w http.ResponseWriter, r *http.Request) {
	adminID, err := h.checkAdmin(r)
	if err != nil {
		api.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	idStr := chi.URLParam(r, "id")
	userID, _ := strconv.ParseInt(idStr, 10, 64)
	if userID <= 0 {
		api.RespondError(w, http.StatusBadRequest, "valid user id is required")
		return
	}

	var req models.LockUserRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Reason == "" {
		req.Reason = "Account locked by administrator"
	}

	if err := h.accessSvc.LockUser(r.Context(), userID, req.Reason, adminID, req.RevokeSessions); err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"user_id": userID,
		"status":  "locked",
		"reason":  req.Reason,
	})
}

// UnlockUser handles POST /admin/access/users/{id}/unlock.
func (h *AccessControlHandler) UnlockUser(w http.ResponseWriter, r *http.Request) {
	if _, err := h.checkAdmin(r); err != nil {
		api.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	idStr := chi.URLParam(r, "id")
	userID, _ := strconv.ParseInt(idStr, 10, 64)
	if userID <= 0 {
		api.RespondError(w, http.StatusBadRequest, "valid user id is required")
		return
	}

	if err := h.accessSvc.UnlockUser(r.Context(), userID); err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"user_id": userID,
		"status":  "active",
	})
}

// RevokeSessions handles POST /admin/access/users/{id}/revoke-sessions.
func (h *AccessControlHandler) RevokeSessions(w http.ResponseWriter, r *http.Request) {
	if _, err := h.checkAdmin(r); err != nil {
		api.RespondError(w, http.StatusForbidden, err.Error())
		return
	}

	idStr := chi.URLParam(r, "id")
	userID, _ := strconv.ParseInt(idStr, 10, 64)
	if userID <= 0 {
		api.RespondError(w, http.StatusBadRequest, "valid user id is required")
		return
	}

	version, err := h.accessSvc.RevokeSessions(r.Context(), userID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"user_id":       userID,
		"token_version": version,
		"message":       "All active sessions revoked successfully",
	})
}

// Reverify handles POST /admin/access/reverify.
// Checks membership status of a given user (or caller) in configured required Telegram chats.
func (h *AccessControlHandler) Reverify(w http.ResponseWriter, r *http.Request) {
	var targetUserID int64
	var req struct {
		UserID int64 `json:"user_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.UserID > 0 {
		targetUserID = req.UserID
	} else if uidStr := r.URL.Query().Get("user_id"); uidStr != "" {
		targetUserID, _ = strconv.ParseInt(uidStr, 10, 64)
	} else if uid, ok := middleware.GetUserID(r.Context()); ok {
		targetUserID = uid
	}

	policy, err := h.accessSvc.GetPolicy(r.Context())
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type ChatStatus struct {
		ChatID     int64  `json:"chat_id"`
		Title      string `json:"title,omitempty"`
		IsMember   bool   `json:"is_member"`
		Status     string `json:"status"`
		InviteLink string `json:"invite_link,omitempty"`
	}

	results := make([]ChatStatus, 0, len(policy.RequiredChats))
	allPassed := true

	for _, chat := range policy.RequiredChats {
		st := ChatStatus{
			ChatID:     chat.ChatID,
			Title:      chat.Title,
			InviteLink: chat.InviteLink,
			Status:     "unknown",
		}

		if h.cfg.BotToken != "" && targetUserID > 0 {
			url := fmt.Sprintf("https://api.telegram.org/bot%s/getChatMember?chat_id=%d&user_id=%d",
				h.cfg.BotToken, chat.ChatID, targetUserID)
			resp, err := h.httpClient.Get(url)
			if err == nil && resp != nil {
				defer resp.Body.Close()
				bodyBytes, _ := io.ReadAll(resp.Body)
				var tgResp struct {
					OK     bool `json:"ok"`
					Result struct {
						Status string `json:"status"`
					} `json:"result"`
				}
				if json.Unmarshal(bodyBytes, &tgResp) == nil && tgResp.OK {
					st.Status = tgResp.Result.Status
					if st.Status == "member" || st.Status == "administrator" || st.Status == "creator" {
						st.IsMember = true
					} else {
						st.IsMember = false
						allPassed = false
					}
				} else {
					st.Status = "error_or_not_found"
					allPassed = false
				}
			} else {
				st.Status = "check_unreachable"
				allPassed = false
			}
		} else {
			// No bot token configured or user ID 0, mark as bypass/mock passed
			st.IsMember = true
			st.Status = "bypassed_no_bot_token"
		}

		results = append(results, st)
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"user_id":    targetUserID,
		"verified":   allPassed,
		"enforced":   policy.EnforceMembership,
		"chats":      results,
		"chat_count": len(results),
	})
}
