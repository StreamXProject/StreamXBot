package handlers

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"streamgo/internal/api"
	"streamgo/internal/api/middleware"
	"streamgo/internal/services"
)

// DailyPlaylistHandler handles daily mix and dynamic playlist endpoints.
type DailyPlaylistHandler struct {
	dailySvc *services.DailyPlaylistService
	authSvc  *services.AuthService
}

// NewDailyPlaylistHandler creates a new DailyPlaylistHandler.
func NewDailyPlaylistHandler(dailySvc *services.DailyPlaylistService, authSvc *services.AuthService) *DailyPlaylistHandler {
	return &DailyPlaylistHandler{
		dailySvc: dailySvc,
		authSvc:  authSvc,
	}
}

// Routes mounts the daily playlist routes.
func (h *DailyPlaylistHandler) Routes(r chi.Router) {
	optionalAuth := middleware.OptionalAuth(h.authSvc)

	r.With(optionalAuth).Get("/playlists/available", h.Available)
	r.With(optionalAuth).Get("/daily-playlist/{key}", h.GetDaily)
}

// Available handles GET /playlists/available.
func (h *DailyPlaylistHandler) Available(w http.ResponseWriter, r *http.Request) {
	resp, err := h.dailySvc.GetAvailablePlaylists(r.Context())
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.RespondJSON(w, http.StatusOK, resp)
}

// GetDaily handles GET /daily-playlist/{key}.
func (h *DailyPlaylistHandler) GetDaily(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if strings.TrimSpace(key) == "" {
		api.RespondError(w, http.StatusBadRequest, "key is required")
		return
	}

	limit := api.ParseQueryInt(r, "limit", 75)
	channelID := api.ParseQueryInt64(r, "channel_id", 0)
	userID, _ := middleware.GetUserID(r.Context())

	resp, err := h.dailySvc.GetDailyPlaylist(r.Context(), key, limit, channelID, userID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, resp)
}
