package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"streamgo/internal/api"
	"streamgo/internal/api/middleware"
	"streamgo/internal/config"
	"streamgo/internal/services"
)

// TrackHandler handles HTTP routes for track operations.
type TrackHandler struct {
	trackService *services.TrackService
	authSvc      *services.AuthService
	cfg          *config.Config
}

// NewTrackHandler creates a new TrackHandler.
func NewTrackHandler(svc *services.TrackService, authSvc *services.AuthService, cfg *config.Config) *TrackHandler {
	return &TrackHandler{
		trackService: svc,
		authSvc:      authSvc,
		cfg:          cfg,
	}
}

// Routes mounts track endpoints on the given Chi router.
func (h *TrackHandler) Routes(r chi.Router) {
	r.Get("/tracks", h.List)
	r.Get("/browse", h.List)
	r.Get("/search", h.Search)
	r.Get("/tracks/search", h.Search)
	r.Get("/tracks/random", h.Random)
	r.Get("/tracks/shuffle", h.Random)
	r.Get("/library/shuffle", h.Random)
	r.Get("/tracks/{id}", h.GetByID)

	r.Group(func(sub chi.Router) {
		if h.authSvc != nil {
			sub.Use(middleware.RequireAuth(h.authSvc))
		}
		sub.Delete("/tracks/{id}", h.Delete)
		sub.Delete("/admin/tracks/delete", h.AdminDelete)
		sub.Post("/admin/tracks/delete", h.AdminDelete)
	})
}

// List handles GET /tracks and GET /browse with pagination, sorting, and filters.
func (h *TrackHandler) List(w http.ResponseWriter, r *http.Request) {
	page := api.ParseQueryInt(r, "page", 1)
	perPage := api.ParseQueryInt(r, "per_page", 20)
	if limit := api.ParseQueryInt(r, "limit", 0); limit > 0 {
		perPage = limit
	}

	sortField := strings.TrimSpace(r.URL.Query().Get("sort"))
	topicName := strings.TrimSpace(r.URL.Query().Get("topic"))
	channelID := api.ParseQueryInt64(r, "channel_id", 0)

	resp, err := h.trackService.Browse(r.Context(), page, perPage, sortField, topicName, channelID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, resp)
}

// GetByID handles GET /tracks/{id}.
func (h *TrackHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		api.RespondError(w, http.StatusBadRequest, "track id is required")
		return
	}

	track, err := h.trackService.GetTrack(r.Context(), id)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if track == nil {
		api.RespondError(w, http.StatusNotFound, "track not found")
		return
	}

	if effType := track.EffectiveType(); effType != "" {
		track.Audio.Type = effType
	}

	api.RespondJSON(w, http.StatusOK, track)
}

// Search handles GET /search with text queries.
func (h *TrackHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		query = strings.TrimSpace(r.URL.Query().Get("query"))
	}
	limit := api.ParseQueryInt(r, "limit", 50)

	items, err := h.trackService.Search(r.Context(), query, limit)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"query": query,
		"total": len(items),
		"items": items,
	})
}

// Random handles GET /tracks/random.
func (h *TrackHandler) Random(w http.ResponseWriter, r *http.Request) {
	limit := api.ParseQueryInt(r, "limit", 20)
	channelID := api.ParseQueryInt64(r, "channel_id", 0)

	items, err := h.trackService.GetRandom(r.Context(), limit, channelID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"total": len(items),
		"items": items,
	})
}

// checkAdmin verifies caller has administrator privileges (consistent with access_control.go).
func (h *TrackHandler) checkAdmin(r *http.Request) error {
	if middleware.IsGuest(r.Context()) {
		return nil
	}

	userID, ok := middleware.GetUserID(r.Context())
	if !ok || userID <= 0 {
		return fmt.Errorf("authentication required")
	}

	if h.cfg == nil || (len(h.cfg.OwnerIDs) == 0 && len(h.cfg.SudoUsers) == 0) {
		return nil
	}

	for _, oid := range h.cfg.OwnerIDs {
		if oid == userID {
			return nil
		}
	}
	for _, sid := range h.cfg.SudoUsers {
		if sid == userID {
			return nil
		}
	}

	return fmt.Errorf("forbidden: administrator privileges required")
}

// Delete handles DELETE /tracks/{id}.
func (h *TrackHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.checkAdmin(r); err != nil {
		if strings.Contains(err.Error(), "forbidden") {
			api.RespondError(w, http.StatusForbidden, err.Error())
		} else {
			api.RespondError(w, http.StatusUnauthorized, err.Error())
		}
		return
	}

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		api.RespondError(w, http.StatusBadRequest, "track id is required")
		return
	}

	hard := api.ParseQueryBool(r, "hard", true)
	purgeCache := api.ParseQueryBool(r, "purge_cache", true)
	purgeArtwork := api.ParseQueryBool(r, "purge_artwork", true)

	opts := services.DeleteTrackOptions{
		Hard:         hard,
		PurgeCache:   purgeCache,
		PurgeArtwork: purgeArtwork,
	}

	res, err := h.trackService.DeleteTrack(r.Context(), id, opts)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			api.RespondError(w, http.StatusNotFound, "track not found")
			return
		}
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"ok":              true,
		"status":          http.StatusOK,
		"track_id":        res.TrackID,
		"mode":            res.Mode,
		"cache_purged":    res.CachePurged,
		"purged_artworks": res.PurgedArtworks,
		"message":         res.Message,
	})
}

// AdminDeleteTracksRequest is the payload for /admin/tracks/delete.
type AdminDeleteTracksRequest struct {
	TrackID  any      `json:"track_id"`
	TrackIDs []string `json:"track_ids"`
}

// AdminDelete handles DELETE and POST /admin/tracks/delete (compatible with StreamXBot).
func (h *TrackHandler) AdminDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.checkAdmin(r); err != nil {
		if strings.Contains(err.Error(), "forbidden") {
			api.RespondError(w, http.StatusForbidden, err.Error())
		} else {
			api.RespondError(w, http.StatusUnauthorized, err.Error())
		}
		return
	}

	var trackIDs []string
	seen := make(map[string]bool)

	// 1. Try reading JSON body if present
	if r.Body != nil {
		var req AdminDeleteTracksRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			if s, ok := req.TrackID.(string); ok && strings.TrimSpace(s) != "" {
				s = strings.TrimSpace(s)
				if !seen[s] {
					seen[s] = true
					trackIDs = append(trackIDs, s)
				}
			} else if slice, ok := req.TrackID.([]any); ok {
				for _, item := range slice {
					s := strings.TrimSpace(fmt.Sprint(item))
					if s != "" && !seen[s] {
						seen[s] = true
						trackIDs = append(trackIDs, s)
					}
				}
			}
			for _, tid := range req.TrackIDs {
				tid = strings.TrimSpace(tid)
				if tid != "" && !seen[tid] {
					seen[tid] = true
					trackIDs = append(trackIDs, tid)
				}
			}
		}
	}

	// 2. Fallback or merge query parameters
	if qID := strings.TrimSpace(r.URL.Query().Get("track_id")); qID != "" && !seen[qID] {
		seen[qID] = true
		trackIDs = append(trackIDs, qID)
	}
	for _, qIDs := range r.URL.Query()["track_ids"] {
		for _, tid := range strings.Split(qIDs, ",") {
			tid = strings.TrimSpace(tid)
			if tid != "" && !seen[tid] {
				seen[tid] = true
				trackIDs = append(trackIDs, tid)
			}
		}
	}

	if len(trackIDs) == 0 {
		api.RespondError(w, http.StatusBadRequest, "track_id or track_ids is required")
		return
	}

	hard := api.ParseQueryBool(r, "hard", true)
	purgeCache := api.ParseQueryBool(r, "purge_cache", true)
	purgeArtwork := api.ParseQueryBool(r, "purge_artwork", true)

	opts := services.DeleteTrackOptions{
		Hard:         hard,
		PurgeCache:   purgeCache,
		PurgeArtwork: purgeArtwork,
	}

	deletedCount := 0
	for _, tid := range trackIDs {
		if _, err := h.trackService.DeleteTrack(r.Context(), tid, opts); err == nil {
			deletedCount++
		}
	}

	api.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"track_ids": trackIDs,
		"matched":   len(trackIDs),
		"deleted":   deletedCount,
	})
}
