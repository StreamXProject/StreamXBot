package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"streamgo/internal/api"
	"streamgo/internal/api/middleware"
	"streamgo/internal/models"
	"streamgo/internal/services"
)

// HistoryHandler handles playback history, top tracks, and listening telemetry.
type HistoryHandler struct {
	histSvc *services.HistoryService
	authSvc *services.AuthService
}

// NewHistoryHandler creates a new HistoryHandler.
func NewHistoryHandler(histSvc *services.HistoryService, authSvc *services.AuthService) *HistoryHandler {
	return &HistoryHandler{
		histSvc: histSvc,
		authSvc: authSvc,
	}
}

// Routes mounts history endpoints.
func (h *HistoryHandler) Routes(r chi.Router) {
	requireAuth := middleware.RequireAuth(h.authSvc)

	// History endpoints
	r.With(requireAuth).Get("/history", h.GetHistory)
	r.With(requireAuth).Get("/me/history", h.GetHistory)
	r.With(requireAuth).Get("/me/top-played", h.GetTopPlayed)
}

// GetHistory handles GET /history and GET /me/history.
func (h *HistoryHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID <= 0 {
		api.RespondError(w, http.StatusUnauthorized, "user login required")
		return
	}

	limit := api.ParseQueryInt(r, "limit", 50)
	resp, err := h.histSvc.GetHistory(r.Context(), userID, limit)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, resp)
}

// GetTopPlayed handles GET /me/top-played.
func (h *HistoryHandler) GetTopPlayed(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID <= 0 {
		api.RespondError(w, http.StatusUnauthorized, "user login required")
		return
	}

	limit := api.ParseQueryInt(r, "limit", 50)
	resp, err := h.histSvc.GetTopPlayed(r.Context(), userID, limit)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, resp)
}

// RecordEvents handles POST /me/listening-events and POST /listening-events.
func (h *HistoryHandler) RecordEvents(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	var payload models.ListeningEventsPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(payload.Events) == 0 {
		api.RespondJSON(w, http.StatusOK, map[string]any{"ok": true, "count": 0})
		return
	}

	if err := h.histSvc.RecordEvents(r.Context(), userID, payload.Events); err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"count": len(payload.Events),
	})
}
