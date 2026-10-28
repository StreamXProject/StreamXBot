package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"streamgo/internal/api"
	"streamgo/internal/api/middleware"
	"streamgo/internal/config"
	"streamgo/internal/models"
	"streamgo/internal/services"
)

// AuthHandler handles authentication routes for Telegram WebApp, widgets, OAuth, and password login/register.
type AuthHandler struct {
	cfg     *config.Config
	authSvc *services.AuthService
}

// NewAuthHandler creates an AuthHandler instance.
func NewAuthHandler(cfg *config.Config, authSvc *services.AuthService) *AuthHandler {
	return &AuthHandler{
		cfg:     cfg,
		authSvc: authSvc,
	}
}

// Routes registers authentication endpoints on Chi router.
func (h *AuthHandler) Routes(r chi.Router) {
	// Telegram WebApp & Web routes
	r.Post("/auth/telegram", h.TelegramLogin)
	r.Post("/auth/tg/login", h.TgLogin)
	r.Post("/webapp/verify", h.TelegramLogin)

	// Password login & Register
	r.Post("/auth/login", h.UniversalLogin)
	r.Post("/auth/register", h.Register)
	r.Post("/auth/create", h.DirectRegister)
	r.Post("/auth/validate", h.ValidateOTP)

	// Server / Guest password & Setup endpoints
	r.Get("/auth/setup/status", h.SetupStatus)
	r.Post("/auth/setup", h.SetupPassword)
	r.Post("/auth/password", h.ServerPasswordLogin)
	r.Post("/auth/password/change", h.ChangePassword)

	// Cookie & session management
	r.Post("/auth/cookie", h.SetCookie)
	r.Post("/auth/logout", h.Logout)

	// Telegram Login Widget & Token validation
	r.Post("/auth/telegram/widget", h.WidgetLogin)
	r.Post("/auth/telegram/validate-token", h.ValidateToken)

	// Telegram OAuth / OIDC endpoints
	r.Get("/auth/telegram/config", h.Config)
	r.Get("/webapp/config", h.Config)
	r.Get("/auth/telegram/start", h.TelegramStart)
	r.Get("/auth/telegram/callback", h.TelegramCallback)

	// Direct Telegram Bot App Session login
	r.Post("/auth/telegram/bot-session", h.CreateBotSession)
	r.Get("/auth/telegram/bot-session/status", h.CheckBotSessionStatus)

	// Authenticated profile, credentials, and integrations routes
	r.Group(func(sub chi.Router) {
		sub.Use(middleware.RequireAuth(h.authSvc))
		sub.Get("/auth/me", h.Me)
		sub.Post("/auth/credentials", h.SetCredentials)
		sub.Post("/auth/integrations", h.UpdateIntegrations)
		sub.Post("/auth/fcm-token", h.UpdateFCMToken)
	})
}

// UniversalLogin handles POST /auth/login, supporting both password login and WebApp initData.
func (h *AuthHandler) UniversalLogin(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// 1. If init_data is present, process as Telegram WebApp login
	if initData, ok := body["init_data"].(string); ok && strings.TrimSpace(initData) != "" {
		req := models.TgLoginRequest{
			InitData: initData,
		}
		if u, ok := body["username"].(string); ok {
			req.Username = u
		}
		if p, ok := body["password"].(string); ok {
			req.Password = p
		}
		if inv, ok := body["invite_code"].(string); ok {
			req.InviteCode = inv
		}

		resp, err := h.authSvc.TgLogin(r.Context(), req)
		if err != nil {
			api.RespondError(w, http.StatusUnauthorized, err.Error())
			return
		}
		if r.URL.Query().Get("set_cookie") == "true" || r.URL.Query().Get("set_cookie") == "1" {
			h.authSvc.SetAuthCookie(w, resp.Token)
		}
		api.RespondJSON(w, http.StatusOK, resp)
		return
	}

	// 2. Standard username + password login
	username, _ := body["username"].(string)
	password, _ := body["password"].(string)
	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
		api.RespondError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	resp, err := h.authSvc.LoginWithPassword(r.Context(), username, password)
	if err != nil {
		if strings.Contains(err.Error(), "invalid username") {
			api.RespondError(w, http.StatusBadRequest, err.Error())
		} else {
			api.RespondError(w, http.StatusUnauthorized, err.Error())
		}
		return
	}

	if r.URL.Query().Get("set_cookie") == "true" || r.URL.Query().Get("set_cookie") == "1" {
		h.authSvc.SetAuthCookie(w, resp.Token)
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"user_id":     resp.UserID,
		"token":       resp.Token,
		"first_name":  resp.FirstName,
		"username":    resp.Username,
		"profile_url": resp.ProfileURL,
		"photo_url":   resp.PhotoURL,
	})
}

// TelegramLogin handles POST /auth/telegram and /webapp/verify.
func (h *AuthHandler) TelegramLogin(w http.ResponseWriter, r *http.Request) {
	var initData string

	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/x-www-form-urlencoded") || strings.Contains(contentType, "multipart/form-data") {
		_ = r.ParseForm()
		initData = r.FormValue("init_data")
		if initData == "" {
			initData = r.FormValue("initData")
		}
	} else {
		var payload models.TelegramWebAppRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
			initData = payload.InitData
		}
	}

	initData = strings.TrimSpace(initData)
	if initData == "" {
		api.RespondError(w, http.StatusBadRequest, "init_data is required")
		return
	}

	resp, err := h.authSvc.AuthenticateTelegram(r.Context(), initData)
	if err != nil {
		api.RespondError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if r.URL.Query().Get("set_cookie") == "true" || r.URL.Query().Get("set_cookie") == "1" {
		h.authSvc.SetAuthCookie(w, resp.Token)
	}

	api.RespondJSON(w, http.StatusOK, resp)
}

// TgLogin handles POST /auth/tg/login.
func (h *AuthHandler) TgLogin(w http.ResponseWriter, r *http.Request) {
	var payload models.TgLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request payload")
		return
	}

	resp, err := h.authSvc.TgLogin(r.Context(), payload)
	if err != nil {
		if strings.Contains(err.Error(), "username already taken") {
			api.RespondError(w, http.StatusConflict, err.Error())
		} else if strings.Contains(err.Error(), "invalid username") {
			api.RespondError(w, http.StatusBadRequest, err.Error())
		} else {
			api.RespondError(w, http.StatusUnauthorized, err.Error())
		}
		return
	}

	if r.URL.Query().Get("set_cookie") == "true" || r.URL.Query().Get("set_cookie") == "1" {
		h.authSvc.SetAuthCookie(w, resp.Token)
	}

	api.RespondJSON(w, http.StatusOK, resp)
}

// Register handles POST /auth/register, supporting both direct account creation and Telegram OTP.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var payload models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	h.doRegister(w, r, payload)
}

// DirectRegister handles POST /auth/create as an explicit route for direct user creation.
func (h *AuthHandler) DirectRegister(w http.ResponseWriter, r *http.Request) {
	var payload models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	payload.UserID = 0
	h.doRegister(w, r, payload)
}

func (h *AuthHandler) doRegister(w http.ResponseWriter, r *http.Request, payload models.RegisterRequest) {
	resp, err := h.authSvc.RegisterAccount(r.Context(), payload)
	if err != nil {
		msg := err.Error()
		if strings.Contains(strings.ToLower(msg), "already registered") || strings.Contains(strings.ToLower(msg), "already taken") {
			api.RespondError(w, http.StatusConflict, msg)
		} else if strings.Contains(msg, "invalid username") || strings.Contains(msg, "invalid user id") || strings.Contains(msg, "invite_") {
			api.RespondError(w, http.StatusBadRequest, msg)
		} else if strings.Contains(msg, "registration_closed") || strings.Contains(msg, "not_allowlisted") || strings.Contains(msg, "membership_required") {
			api.RespondError(w, http.StatusForbidden, msg)
		} else {
			api.RespondError(w, http.StatusInternalServerError, msg)
		}
		return
	}

	// If direct registration created the account, an auth token is issued immediately
	if resp.Token != "" {
		if r.URL.Query().Get("set_cookie") == "true" || r.URL.Query().Get("set_cookie") == "1" {
			h.authSvc.SetAuthCookie(w, resp.Token)
		}
		api.RespondJSON(w, http.StatusOK, map[string]any{
			"ok":          true,
			"user_id":     resp.UserID,
			"token":       resp.Token,
			"username":    resp.Username,
			"first_name":  resp.FirstName,
			"profile_url": resp.ProfileURL,
			"photo_url":   resp.PhotoURL,
		})
		return
	}

	// Telegram OTP registration pending
	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"message":     "OTP sent",
		"user_id":     resp.UserID,
		"username":    resp.Username,
		"first_name":  resp.FirstName,
		"profile_url": resp.ProfileURL,
		"photo_url":   resp.PhotoURL,
	})
}


// ValidateOTP handles POST /auth/validate by verifying OTP code and activating account.
func (h *AuthHandler) ValidateOTP(w http.ResponseWriter, r *http.Request) {
	var payload models.ValidateOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := h.authSvc.ValidateOTP(r.Context(), payload)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "no pending registration") || strings.Contains(msg, "invite_") {
			api.RespondError(w, http.StatusBadRequest, msg)
		} else if strings.Contains(msg, "invalid OTP") || strings.Contains(msg, "session_revoked") {
			api.RespondError(w, http.StatusUnauthorized, msg)
		} else if strings.Contains(msg, "registration_closed") || strings.Contains(msg, "not_allowlisted") || strings.Contains(msg, "membership_required") {
			api.RespondError(w, http.StatusForbidden, msg)
		} else {
			api.RespondError(w, http.StatusInternalServerError, msg)
		}
		return
	}

	if r.URL.Query().Get("set_cookie") == "true" || r.URL.Query().Get("set_cookie") == "1" {
		h.authSvc.SetAuthCookie(w, resp.Token)
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"user_id":     resp.UserID,
		"username":    resp.Username,
		"token":       resp.Token,
		"first_name":  resp.FirstName,
		"profile_url": resp.ProfileURL,
		"photo_url":   resp.PhotoURL,
	})
}

// SetCredentials handles POST /auth/credentials for updating credentials.
func (h *AuthHandler) SetCredentials(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID <= 0 {
		api.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var payload models.SetCredentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.authSvc.UpdateCredentials(r.Context(), userID, payload.Username, payload.Password); err != nil {
		if strings.Contains(err.Error(), "username already taken") {
			api.RespondError(w, http.StatusConflict, err.Error())
		} else {
			api.RespondError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// UpdateIntegrations handles POST /auth/integrations.
func (h *AuthHandler) UpdateIntegrations(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID <= 0 {
		api.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req models.IntegrationsUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	integrations, err := h.authSvc.UpdateIntegrations(r.Context(), userID, req)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"integrations": integrations,
	})
}

// SetCookie handles POST /auth/cookie to set the auth_token cookie.
func (h *AuthHandler) SetCookie(w http.ResponseWriter, r *http.Request) {
	var payload models.SetCookieRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	token := strings.TrimSpace(payload.Token)
	if token == "" {
		api.RespondError(w, http.StatusBadRequest, "token is required")
		return
	}

	uid, err := h.authSvc.VerifyToken(token)
	if err != nil {
		api.RespondError(w, http.StatusUnauthorized, "invalid auth token")
		return
	}

	h.authSvc.SetAuthCookie(w, token)
	api.RespondJSON(w, http.StatusOK, map[string]any{"ok": true, "user_id": uid})
}

// Logout handles POST /auth/logout.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.authSvc.ClearAuthCookie(w)
	api.RespondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// WidgetLogin handles POST /auth/telegram/widget.
func (h *AuthHandler) WidgetLogin(w http.ResponseWriter, r *http.Request) {
	var payload models.TelegramWidgetRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid widget payload")
		return
	}

	resp, err := h.authSvc.AuthenticateWidget(r.Context(), payload)
	if err != nil {
		api.RespondError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if r.URL.Query().Get("set_cookie") == "true" || r.URL.Query().Get("set_cookie") == "1" {
		h.authSvc.SetAuthCookie(w, resp.Token)
	}

	api.RespondJSON(w, http.StatusOK, resp)
}

// ValidateToken handles POST /auth/telegram/validate-token.
func (h *AuthHandler) ValidateToken(w http.ResponseWriter, r *http.Request) {
	var payload models.TelegramTokenLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tokenVal := strings.TrimSpace(payload.IDToken)
	if tokenVal == "" {
		tokenVal = strings.TrimSpace(payload.Token)
	}
	if tokenVal == "" {
		api.RespondError(w, http.StatusBadRequest, "id_token is required")
		return
	}

	user, token, err := h.authSvc.ValidateOIDCIDToken(r.Context(), tokenVal, payload.InviteCode)
	if err != nil {
		api.RespondError(w, http.StatusUnauthorized, fmt.Sprintf("invalid telegram ID token: %v", err))
		return
	}

	if r.URL.Query().Get("set_cookie") != "false" {
		h.authSvc.SetAuthCookie(w, token)
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"user_id":     user.ID,
		"token":       token,
		"first_name":  user.FirstName,
		"username":    user.Username,
		"profile_url": user.ProfileURL,
		"photo_url":   user.PhotoURL,
	})
}

// Me handles GET /auth/me.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	if middleware.IsGuest(r.Context()) {
		api.RespondJSON(w, http.StatusOK, map[string]any{
			"ok":    true,
			"guest": true,
			"user":  nil,
		})
		return
	}

	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID <= 0 {
		api.RespondJSON(w, http.StatusOK, map[string]any{
			"ok":    true,
			"guest": true,
			"user":  nil,
		})
		return
	}

	user, err := h.authSvc.GetUserByID(r.Context(), userID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if user == nil {
		api.RespondError(w, http.StatusNotFound, "user not found")
		return
	}

	role := "user"
	for _, oid := range h.cfg.OwnerIDs {
		if oid == userID {
			role = "owner"
			break
		}
	}
	if role == "user" {
		for _, sid := range h.cfg.SudoUsers {
			if sid == userID {
				role = "sudo"
				break
			}
		}
	}

	isAdmin := role == "owner" || role == "sudo"

	userMap := map[string]any{
		"_id":          user.ID,
		"id":           user.ID,
		"user_id":      user.ID,
		"userid":       user.ID,
		"username":     user.Username,
		"first_name":   user.FirstName,
		"profile_url":  user.ProfileURL,
		"photo_url":    user.PhotoURL,
		"telegram":     user.Telegram,
		"status":       user.Status,
		"role":         role,
		"is_admin":     isAdmin,
		"created_at":   user.CreatedAt,
		"updated_at":   user.UpdatedAt,
		"integrations": user.Integrations,
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"user": userMap,
	})
}

// SetupStatus handles GET /auth/setup/status.
func (h *AuthHandler) SetupStatus(w http.ResponseWriter, r *http.Request) {
	resp, err := h.authSvc.GetSetupStatus(r.Context())
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.RespondJSON(w, http.StatusOK, resp)
}

// SetupPassword handles POST /auth/setup.
func (h *AuthHandler) SetupPassword(w http.ResponseWriter, r *http.Request) {
	var payload models.SetupPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	token, err := h.authSvc.SetupOwnerPassword(r.Context(), payload.Password)
	if err != nil {
		if strings.Contains(err.Error(), "already completed") {
			api.RespondError(w, http.StatusForbidden, err.Error())
		} else {
			api.RespondError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	if r.URL.Query().Get("set_cookie") == "true" || r.URL.Query().Get("set_cookie") == "1" {
		h.authSvc.SetAuthCookie(w, token)
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"token": token,
	})
}

// ServerPasswordLogin handles POST /auth/password.
func (h *AuthHandler) ServerPasswordLogin(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	username, _ := body["username"].(string)
	password, _ := body["password"].(string)

	// If username is provided and not "guest", treat as user login with password (matching Swagger schema)
	if strings.TrimSpace(username) != "" && !strings.EqualFold(strings.TrimSpace(username), "guest") {
		if strings.TrimSpace(password) == "" {
			api.RespondError(w, http.StatusBadRequest, "password is required")
			return
		}

		resp, err := h.authSvc.LoginWithPassword(r.Context(), username, password)
		if err != nil {
			if strings.Contains(err.Error(), "invalid username") {
				api.RespondError(w, http.StatusBadRequest, err.Error())
			} else {
				api.RespondError(w, http.StatusUnauthorized, err.Error())
			}
			return
		}

		if r.URL.Query().Get("set_cookie") == "true" || r.URL.Query().Get("set_cookie") == "1" {
			h.authSvc.SetAuthCookie(w, resp.Token)
		}

		api.RespondJSON(w, http.StatusOK, map[string]any{
			"ok":          true,
			"user_id":     resp.UserID,
			"token":       resp.Token,
			"first_name":  resp.FirstName,
			"username":    resp.Username,
			"profile_url": resp.ProfileURL,
			"photo_url":   resp.PhotoURL,
		})
		return
	}

	token, err := h.authSvc.LoginWithServerPassword(r.Context(), password)
	if err != nil {
		api.RespondError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if r.URL.Query().Get("set_cookie") == "true" || r.URL.Query().Get("set_cookie") == "1" {
		h.authSvc.SetAuthCookie(w, token)
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"token": token,
	})
}

// ChangePassword handles POST /auth/password/change.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var payload models.SetupPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.authSvc.ChangeOwnerPassword(r.Context(), payload.Password, userID); err != nil {
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Config handles GET /auth/telegram/config and GET /webapp/config.
func (h *AuthHandler) Config(w http.ResponseWriter, r *http.Request) {
	clientID := h.authSvc.GetTelegramClientID()
	botUsername := h.authSvc.GetBotUsername(r.Context())
	enabled := clientID != "" || botUsername != ""

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"enabled":      enabled,
		"client_id":    clientID,
		"bot_id":       clientID,
		"bot_username": botUsername,
		"has_auth":     true,
	})
}

// TelegramStart handles GET /auth/telegram/start.
func (h *AuthHandler) TelegramStart(w http.ResponseWriter, r *http.Request) {
	redirect := r.URL.Query().Get("redirect")
	if redirect == "" || !strings.HasPrefix(redirect, "/") || strings.HasPrefix(redirect, "//") {
		redirect = "/"
	}

	frontendURL := r.URL.Query().Get("frontend_url")
	frontendBase := getFrontendBaseURL(r, frontendURL, h.cfg)
	callbackURI := getTelegramRedirectURI(r, h.cfg, frontendBase)
	inviteCode := strings.TrimSpace(r.URL.Query().Get("invite_code"))

	authURL, err := h.authSvc.StartTelegramOIDC(r.Context(), callbackURI, frontendBase, redirect, inviteCode)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

// TelegramCallback handles GET /auth/telegram/callback.
func (h *AuthHandler) TelegramCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	frontendBase := getFrontendBaseURL(r, "", h.cfg)

	// 1. Provider error
	if errVal := q.Get("error"); errVal != "" {
		errDesc := q.Get("error_description")
		if errDesc == "" {
			errDesc = errVal
		}
		http.Redirect(w, r, fmt.Sprintf("%s/login?error=%s", frontendBase, url.QueryEscape(errVal)), http.StatusFound)
		return
	}

	// 2. Direct Widget Query Callback (?hash=...&id=...)
	if q.Get("hash") != "" && q.Get("id") != "" {
		id, err := strconv.ParseInt(q.Get("id"), 10, 64)
		if err == nil && id > 0 {
			authDate, _ := strconv.ParseInt(q.Get("auth_date"), 10, 64)
			widgetReq := models.TelegramWidgetRequest{
				ID:        id,
				FirstName: q.Get("first_name"),
				LastName:  q.Get("last_name"),
				Username:  q.Get("username"),
				PhotoURL:  q.Get("photo_url"),
				AuthDate:  authDate,
				Hash:      q.Get("hash"),
			}
			resp, err := h.authSvc.AuthenticateWidget(r.Context(), widgetReq)
			if err == nil {
				h.authSvc.SetAuthCookie(w, resp.Token)
				renderSuccessPopup(w, resp.Token, resp.UserID, resp.FirstName, resp.Username, resp.PhotoURL, frontendBase+"/")
				return
			}
		}
		http.Redirect(w, r, fmt.Sprintf("%s/login?error=invalid_widget_signature", frontendBase), http.StatusFound)
		return
	}

	// 3. OIDC code exchange (?code=...&state=...)
	code := q.Get("code")
	state := q.Get("state")
	if code != "" && state != "" {
		user, token, destPath, err := h.authSvc.ExchangeOIDCCode(r.Context(), code, state)
		if err != nil {
			http.Redirect(w, r, fmt.Sprintf("%s/login?error=%s", frontendBase, url.QueryEscape(err.Error())), http.StatusFound)
			return
		}

		h.authSvc.SetAuthCookie(w, token)
		redirectTarget := fmt.Sprintf("%s/login?token=%s&redirect=%s", frontendBase, url.QueryEscape(token), url.QueryEscape(destPath))
		renderSuccessPopup(w, token, user.ID, user.FirstName, user.Username, user.PhotoURL, redirectTarget)
		return
	}

	// 4. Hash fragment fallback (for clients where Telegram returned fragment #tgAuthResult or #id_token)
	renderFragmentFallbackHTML(w)
}

func renderSuccessPopup(w http.ResponseWriter, token string, userID int64, firstName, username, photoURL, redirectTarget string) {
	safeFirstName, _ := json.Marshal(firstName)
	safeUsername, _ := json.Marshal(username)
	safePhoto, _ := json.Marshal(photoURL)
	safeTarget, _ := json.Marshal(redirectTarget)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<!DOCTYPE html>
<html><body><script>
  if (window.opener && window.opener !== window) {
    try {
      window.opener.postMessage({
        type: 'tg_auth_success',
        token: '%s',
        user: {
          user_id: %d,
          first_name: %s,
          username: %s,
          photo_url: %s
        }
      }, window.location.origin);
      window.close();
    } catch (e) {
      window.location.replace(%s);
    }
  } else {
    window.location.replace(%s);
  }
</script></body></html>`, token, userID, string(safeFirstName), string(safeUsername), string(safePhoto), string(safeTarget), string(safeTarget))
}

func renderFragmentFallbackHTML(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Verifying Telegram Login...</title>
  <style>
    body {
      margin: 0; min-height: 100vh; display: flex; align-items: center; justify-content: center;
      background-color: #0b0f17; color: #f1f5f9; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    }
    .card {
      text-align: center; padding: 36px 44px; background: rgba(30, 41, 59, 0.8); backdrop-filter: blur(16px);
      border: 1px solid rgba(255, 255, 255, 0.1); border-radius: 20px; max-width: 360px; box-shadow: 0 20px 40px rgba(0,0,0,0.5);
    }
    .spinner {
      width: 44px; height: 44px; border: 4px solid rgba(56, 189, 248, 0.2); border-top-color: #38bdf8;
      border-radius: 50%; animation: spin 0.8s linear infinite; margin: 0 auto 16px;
    }
    @keyframes spin { to { transform: rotate(360deg); } }
    h2 { font-size: 18px; margin: 0 0 8px; font-weight: 600; }
    p { font-size: 13px; color: #94a3b8; margin: 0; }
  </style>
</head>
<body>
  <div class="card">
    <div class="spinner"></div>
    <h2>Verifying Telegram login</h2>
    <p>Connecting your account, please wait...</p>
  </div>
  <script>
    (async function() {
      try {
        const hash = window.location.hash.substring(1);
        const search = window.location.search.substring(1);
        const hashParams = new URLSearchParams(hash);
        const searchParams = new URLSearchParams(search);

        const dest = sessionStorage.getItem('tg_auth_redirect') || '/';
        let inviteCode = searchParams.get('invite_code') || '';
        if (!inviteCode) {
          try {
            inviteCode = window.opener?.sessionStorage?.getItem('webx_invite_code') || sessionStorage.getItem('webx_invite_code') || '';
          } catch (e) {}
        }

        const tgAuthResult = hashParams.get('tgAuthResult');
        if (tgAuthResult) {
          let base64 = tgAuthResult.replace(/-/g, '+').replace(/_/g, '/');
          while (base64.length % 4) { base64 += '='; }
          const binary = atob(base64);
          const bytes = Uint8Array.from(binary, c => c.charCodeAt(0));
          const decodedJson = new TextDecoder().decode(bytes);
          const payload = JSON.parse(decodedJson);
          if (inviteCode) payload.invite_code = inviteCode;

          const res = await fetch('/auth/telegram/widget', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
          });
          const data = await res.json();
          if (data.ok && data.token) {
            sessionStorage.removeItem('tg_auth_redirect');
            sessionStorage.removeItem('webx_invite_code');
            if (window.opener && window.opener !== window) {
              try {
                window.opener.postMessage({ type: 'tg_auth_success', token: data.token, user: data }, window.location.origin);
                window.close();
                return;
              } catch (e) {}
            }
            window.location.replace('/login?token=' + encodeURIComponent(data.token) + '&redirect=' + encodeURIComponent(dest));
            return;
          }
        }

        const idToken = hashParams.get('id_token');
        if (idToken) {
          const res = await fetch('/auth/telegram/validate-token', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ id_token: idToken, invite_code: inviteCode || undefined })
          });
          const data = await res.json();
          if (data.ok && data.token) {
            sessionStorage.removeItem('tg_auth_redirect');
            sessionStorage.removeItem('webx_invite_code');
            if (window.opener && window.opener !== window) {
              try {
                window.opener.postMessage({ type: 'tg_auth_success', token: data.token, user: data }, window.location.origin);
                window.close();
                return;
              } catch (e) {}
            }
            window.location.replace('/login?token=' + encodeURIComponent(data.token) + '&redirect=' + encodeURIComponent(dest));
            return;
          }
        }

        const code = searchParams.get('code') || hashParams.get('code');
        const state = searchParams.get('state') || hashParams.get('state');
        if (code && state) {
          window.location.replace('/auth/telegram/callback?code=' + encodeURIComponent(code) + '&state=' + encodeURIComponent(state));
          return;
        }

        if (search) {
          window.location.replace('/auth/telegram/callback?' + search);
          return;
        }

        window.location.replace('/login?error=no_auth_payload');
      } catch (err) {
        window.location.replace('/login?error=verification_failed');
      }
    })();
  </script>
</body>
</html>`)
}

func getFrontendBaseURL(r *http.Request, clientRedirect string, cfg *config.Config) string {
	if clientRedirect != "" && (strings.HasPrefix(clientRedirect, "http://") || strings.HasPrefix(clientRedirect, "https://")) {
		if u, err := url.Parse(clientRedirect); err == nil && u.Scheme != "" && u.Host != "" {
			return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
		}
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		referer := r.Header.Get("Referer")
		if referer != "" {
			if u, err := url.Parse(referer); err == nil && u.Scheme != "" && u.Host != "" {
				origin = fmt.Sprintf("%s://%s", u.Scheme, u.Host)
			}
		}
	}
	if origin == "" && cfg.CorsOrigin != "" && cfg.CorsOrigin != "*" {
		origin = strings.Split(cfg.CorsOrigin, ",")[0]
	}
	if origin == "" {
		proto := r.Header.Get("X-Forwarded-Proto")
		if proto == "" {
			if r.TLS != nil {
				proto = "https"
			} else {
				proto = "http"
			}
		}
		host := r.Header.Get("X-Forwarded-Host")
		if host == "" {
			host = r.Host
		}
		origin = fmt.Sprintf("%s://%s", proto, host)
	}
	return strings.TrimRight(origin, "/")
}

func getTelegramRedirectURI(r *http.Request, cfg *config.Config, frontendBase string) string {
	if cfg.TelegramOIDCRedirectURI != "" {
		return cfg.TelegramOIDCRedirectURI
	}
	if frontendBase != "" && (strings.HasPrefix(frontendBase, "http://") || strings.HasPrefix(frontendBase, "https://")) {
		return fmt.Sprintf("%s/auth/telegram/callback", strings.TrimRight(frontendBase, "/"))
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return fmt.Sprintf("%s://%s/auth/telegram/callback", proto, host)
}

// UpdateFCMToken handles POST /auth/fcm-token matching Api/schemas/auth.py.
func (h *AuthHandler) UpdateFCMToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID <= 0 {
		api.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req models.FCMTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if strings.TrimSpace(req.FCMToken) == "" {
		api.RespondError(w, http.StatusBadRequest, "fcm_token is required")
		return
	}

	if err := h.authSvc.UpdateFCMToken(r.Context(), userID, req.FCMToken); err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// CreateBotSession handles POST /auth/telegram/bot-session matching Api/schemas/auth.py.
func (h *AuthHandler) CreateBotSession(w http.ResponseWriter, r *http.Request) {
	var req models.TelegramBotSessionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	res, err := h.authSvc.CreateBotSession(r.Context(), req.InviteCode)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, res)
}

// CheckBotSessionStatus handles GET /auth/telegram/bot-session/status matching Api/routers/auth.py.
func (h *AuthHandler) CheckBotSessionStatus(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		api.RespondError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	res, err := h.authSvc.CheckBotSessionStatus(r.Context(), sessionID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	setCookie := r.URL.Query().Get("set_cookie") != "false"
	if setCookie && res["status"] == "confirmed" {
		if token, ok := res["token"].(string); ok && token != "" {
			h.authSvc.SetAuthCookie(w, token)
		}
	}

	api.RespondJSON(w, http.StatusOK, res)
}
