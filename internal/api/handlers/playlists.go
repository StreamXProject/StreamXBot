package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"streamgo/internal/api"
	"streamgo/internal/api/middleware"
	"streamgo/internal/models"
	"streamgo/internal/services"
)

// PlaylistHandler handles custom user playlist and album endpoints.
type PlaylistHandler struct {
	favSvc  *services.FavouritePlaylistService
	authSvc *services.AuthService
}

// NewPlaylistHandler creates a PlaylistHandler instance.
func NewPlaylistHandler(favSvc *services.FavouritePlaylistService, authSvc *services.AuthService) *PlaylistHandler {
	return &PlaylistHandler{
		favSvc:  favSvc,
		authSvc: authSvc,
	}
}

// Routes registers all playlist and album routes on Chi router.
func (h *PlaylistHandler) Routes(r chi.Router) {
	requireAuth := middleware.RequireAuth(h.authSvc)
	optAuth := middleware.OptionalAuth(h.authSvc)

	// Protected routes (creation, modification, deletion)
	r.Group(func(sub chi.Router) {
		sub.Use(requireAuth)

		// Playlists
		sub.Post("/playlists", h.Create)
		sub.Post("/me/playlists", h.Create)

		sub.Patch("/playlists/{id}", h.Rename)
		sub.Patch("/me/playlists/{id}", h.Rename)
		sub.Patch("/me/playlists/{playlist_id}", h.Rename)
		sub.Put("/playlists/{id}", h.Rename)
		sub.Put("/me/playlists/{id}", h.Rename)

		sub.Delete("/playlists/{id}", h.Delete)
		sub.Delete("/me/playlists/{id}", h.Delete)
		sub.Delete("/me/playlists/{playlist_id}", h.Delete)

		sub.Post("/playlists/{id}/tracks", h.AddTracks)
		sub.Post("/me/playlists/{id}/tracks", h.AddTracks)
		sub.Post("/me/playlists/{playlist_id}/tracks", h.AddTracks)

		sub.Delete("/playlists/{id}/tracks/{track_id}", h.RemoveTrack)
		sub.Delete("/me/playlists/{id}/tracks/{track_id}", h.RemoveTrack)
		sub.Delete("/me/playlists/{playlist_id}/tracks/{track_id}", h.RemoveTrack)

		// Tracks listing for playlist (requires auth in StreamXBot)
		sub.Get("/me/playlists/{playlist_id}/tracks", h.GetTracks)
		sub.Get("/me/playlists/{id}/tracks", h.GetTracks)
		sub.Get("/playlists/{id}/tracks", h.GetTracks)

		// Saved Albums
		sub.Post("/albums", h.SaveAlbum)
		sub.Post("/me/albums", h.SaveAlbum)
		sub.Get("/me/albums", h.ListAlbums)
	})

	// Optional / Public routes
	r.Group(func(sub chi.Router) {
		sub.Use(optAuth)
		sub.Get("/playlists", h.ListMine)
		sub.Get("/me/playlists", h.ListMine)

		// Shared & public playlist access
		sub.Get("/share/playlists/{id}", h.GetSharedPlaylist)
		sub.Get("/share/playlists/{playlist_id}", h.GetSharedPlaylist)
		sub.Get("/playlists/{id}", h.GetSharedPlaylist)
		sub.Get("/me/playlists/{id}", h.GetSharedPlaylist)
	})
}

// Create handles POST /playlists and POST /me/playlists.
func (h *PlaylistHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		api.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var payload struct {
		Name  string `json:"name"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = strings.TrimSpace(payload.Title)
	}
	if name == "" {
		api.RespondError(w, http.StatusBadRequest, "name is required")
		return
	}

	p, err := h.favSvc.CreatePlaylist(r.Context(), userID, name)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, p)
}

// ListMine handles GET /playlists and GET /me/playlists.
func (h *PlaylistHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		api.RespondJSON(w, http.StatusOK, models.PlaylistsResponse{Items: []models.PlaylistItem{}})
		return
	}

	resp, err := h.favSvc.GetUserPlaylists(r.Context(), userID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, resp)
}

// Rename handles PATCH /playlists/{id}, PUT /playlists/{id}, PATCH /me/playlists/{id}.
func (h *PlaylistHandler) Rename(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		api.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		id = strings.TrimSpace(chi.URLParam(r, "playlist_id"))
	}
	if id == "" {
		api.RespondError(w, http.StatusBadRequest, "playlist id is required")
		return
	}

	var payload struct {
		Name  string `json:"name"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = strings.TrimSpace(payload.Title)
	}
	if name == "" {
		api.RespondError(w, http.StatusBadRequest, "name is required")
		return
	}

	updated, err := h.favSvc.RenamePlaylist(r.Context(), id, userID, name)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if updated == nil {
		api.RespondError(w, http.StatusNotFound, "playlist not found")
		return
	}

	api.RespondJSON(w, http.StatusOK, updated)
}

// Delete handles DELETE /playlists/{id} and DELETE /me/playlists/{id}.
func (h *PlaylistHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		api.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		id = strings.TrimSpace(chi.URLParam(r, "playlist_id"))
	}
	if id == "" {
		api.RespondError(w, http.StatusBadRequest, "playlist id is required")
		return
	}

	if err := h.favSvc.DeletePlaylist(r.Context(), id, userID); err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok": true,
	})
}

// AddTracks handles POST /playlists/{id}/tracks and POST /me/playlists/{id}/tracks.
func (h *PlaylistHandler) AddTracks(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		api.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		id = strings.TrimSpace(chi.URLParam(r, "playlist_id"))
	}
	if id == "" {
		api.RespondError(w, http.StatusBadRequest, "playlist id is required")
		return
	}

	var payload struct {
		TrackID  any      `json:"track_id"`
		TrackIDs []string `json:"track_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var trackIDs []string
	if payload.TrackID != nil {
		switch v := payload.TrackID.(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				trackIDs = append(trackIDs, s)
			}
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					trackIDs = append(trackIDs, strings.TrimSpace(s))
				}
			}
		}
	}
	for _, tid := range payload.TrackIDs {
		if s := strings.TrimSpace(tid); s != "" {
			trackIDs = append(trackIDs, s)
		}
	}

	if len(trackIDs) == 0 {
		api.RespondError(w, http.StatusBadRequest, "track_id or track_ids is required")
		return
	}

	added, err := h.favSvc.AddTracksToPlaylist(r.Context(), id, userID, trackIDs)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if len(trackIDs) == 1 {
		api.RespondJSON(w, http.StatusOK, map[string]any{
			"ok":             true,
			"already_exists": added == 0,
		})
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"added": added,
	})
}

// RemoveTrack handles DELETE /playlists/{id}/tracks/{track_id} and DELETE /me/playlists/{id}/tracks/{track_id}.
func (h *PlaylistHandler) RemoveTrack(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		api.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		id = strings.TrimSpace(chi.URLParam(r, "playlist_id"))
	}
	trackID := strings.TrimSpace(chi.URLParam(r, "track_id"))
	if id == "" || trackID == "" {
		api.RespondError(w, http.StatusBadRequest, "playlist id and track id are required")
		return
	}

	if err := h.favSvc.RemoveTrackFromPlaylist(r.Context(), id, userID, trackID); err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"deleted": true,
	})
}

// GetTracks handles GET /playlists/{id}/tracks and GET /me/playlists/{id}/tracks.
func (h *PlaylistHandler) GetTracks(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		api.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		id = strings.TrimSpace(chi.URLParam(r, "playlist_id"))
	}
	if id == "" {
		api.RespondError(w, http.StatusBadRequest, "playlist id is required")
		return
	}

	page := api.ParseQueryInt(r, "page", 1)
	limit := api.ParseQueryInt(r, "limit", 50)

	resp, err := h.favSvc.GetPlaylistTracks(r.Context(), id, userID, page, limit)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, resp)
}

// GetSharedPlaylist handles GET /share/playlists/{id} and GET /playlists/{id}.
func (h *PlaylistHandler) GetSharedPlaylist(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		id = strings.TrimSpace(chi.URLParam(r, "playlist_id"))
	}
	if id == "" {
		api.RespondError(w, http.StatusBadRequest, "playlist id is required")
		return
	}

	resp, err := h.favSvc.GetSharedPlaylist(r.Context(), id)
	if err != nil {
		api.RespondError(w, http.StatusNotFound, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, resp)
}

// SaveAlbum handles POST /albums and POST /me/albums.
func (h *PlaylistHandler) SaveAlbum(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		api.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var payload struct {
		AlbumID  any      `json:"album_id"`
		AlbumIDs []string `json:"album_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var albumIDs []string
	if payload.AlbumID != nil {
		switch v := payload.AlbumID.(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				albumIDs = append(albumIDs, s)
			}
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					albumIDs = append(albumIDs, strings.TrimSpace(s))
				}
			}
		}
	}
	for _, aid := range payload.AlbumIDs {
		if s := strings.TrimSpace(aid); s != "" {
			albumIDs = append(albumIDs, s)
		}
	}

	if len(albumIDs) == 0 {
		api.RespondError(w, http.StatusBadRequest, "album_id or album_ids is required")
		return
	}

	added, err := h.favSvc.SaveAlbums(r.Context(), userID, albumIDs)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if len(albumIDs) == 1 {
		api.RespondJSON(w, http.StatusOK, map[string]any{
			"ok":             true,
			"already_exists": added == 0,
		})
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"added": added,
	})
}

// ListAlbums handles GET /albums and GET /me/albums.
func (h *PlaylistHandler) ListAlbums(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		api.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	page := api.ParseQueryInt(r, "page", 1)
	limit := api.ParseQueryInt(r, "limit", 50)

	resp, err := h.favSvc.GetSavedAlbums(r.Context(), userID, page, limit)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, resp)
}
