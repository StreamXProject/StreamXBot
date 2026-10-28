package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"

	"streamgo/internal/api"
	"streamgo/internal/logger"
	"streamgo/internal/models"
	"streamgo/internal/services"
)

var logDiscordHandler = logger.New("discord_handler")

// DiscordHandler handles Discord Remote Auth and external assets endpoints.
type DiscordHandler struct {
	discordSvc *services.DiscordService
}

// NewDiscordHandler creates a new DiscordHandler instance.
func NewDiscordHandler(discordSvc *services.DiscordService) *DiscordHandler {
	return &DiscordHandler{
		discordSvc: discordSvc,
	}
}

// Routes mounts Discord Remote Auth and asset proxy endpoints.
func (h *DiscordHandler) Routes(r chi.Router) {
	// WebSocket endpoint for real-time QR code flow
	r.HandleFunc("/ws/discord/remote-auth", h.WebSocketRemoteAuth)

	// REST API endpoints
	r.Post("/api/discord/remote-auth/start", h.StartRemoteAuth)
	r.Get("/api/discord/remote-auth/status/{session_id}", h.GetRemoteAuthStatus)
	r.Post("/api/discord/remote-auth/cancel/{session_id}", h.CancelRemoteAuth)
	r.Post("/api/discord/external-assets", h.ResolveExternalAssets)
}

// WebSocketRemoteAuth streams Discord QR remote-auth events directly over WebSocket.
func (h *DiscordHandler) WebSocketRemoteAuth(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		logDiscordHandler.Warnf("Failed to accept websocket: %v", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")

	sess, err := h.discordSvc.CreateWSSession(r.Context())
	if err != nil {
		logDiscordHandler.Errorf("Failed to create Discord remote auth session: %v", err)
		_ = conn.Close(websocket.StatusInternalError, err.Error())
		return
	}
	defer func() {
		sess.Cancel()
		_ = h.discordSvc.CancelSession(sess.SessionID)
	}()

	logDiscordHandler.Infof("[DiscordRA WS] New connection established: %s", sess.SessionID)

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-sess.EventsChan:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			if err := conn.Write(r.Context(), websocket.MessageText, data); err != nil {
				logDiscordHandler.Infof("[DiscordRA WS] Client disconnected early: %s", sess.SessionID)
				return
			}
			if typ, _ := ev["type"].(string); typ == "success" || typ == "error" || typ == "cancelled" {
				return
			}
		}
	}
}

// StartRemoteAuth initializes a Discord Remote Auth session and waits up to 4s for initial QR code.
func (h *DiscordHandler) StartRemoteAuth(w http.ResponseWriter, r *http.Request) {
	resp, err := h.discordSvc.StartSession(r.Context())
	if err != nil {
		api.RespondError(w, http.StatusBadGateway, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, resp)
}

// GetRemoteAuthStatus checks the status of a running Remote Auth session.
func (h *DiscordHandler) GetRemoteAuthStatus(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if sessionID == "" {
		api.RespondError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	resp, err := h.discordSvc.GetStatus(sessionID)
	if err != nil {
		api.RespondError(w, http.StatusNotFound, "Session not found or expired")
		return
	}

	api.RespondJSON(w, http.StatusOK, resp)
}

// CancelRemoteAuth cancels an active Remote Auth session.
func (h *DiscordHandler) CancelRemoteAuth(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if sessionID == "" {
		api.RespondError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	_ = h.discordSvc.CancelSession(sessionID)

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"status":     "cancelled",
		"session_id": sessionID,
	})
}

// ResolveExternalAssets proxies external asset resolution to Discord's API with user's token.
func (h *DiscordHandler) ResolveExternalAssets(w http.ResponseWriter, r *http.Request) {
	var req models.ExternalAssetsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.RespondJSON(w, http.StatusOK, []any{})
		return
	}

	results, err := h.discordSvc.ResolveExternalAssets(r.Context(), req)
	if err != nil {
		logDiscordHandler.Warnf("ResolveExternalAssets error: %v", err)
		api.RespondJSON(w, http.StatusOK, []any{})
		return
	}

	api.RespondJSON(w, http.StatusOK, results)
}
