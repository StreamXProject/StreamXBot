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

// FavouriteHandler manages user favourites, history, and telemetry listening events.
type FavouriteHandler struct {
	favSvc  *services.FavouritePlaylistService
	authSvc *services.AuthService
}

// NewFavouriteHandler creates a new FavouriteHandler instance.
func NewFavouriteHandler(favSvc *services.FavouritePlaylistService, authSvc *services.AuthService) *FavouriteHandler {
	return &FavouriteHandler{
		favSvc:  favSvc,
		authSvc: authSvc,
	}
}

// Routes mounts favourite endpoints under Chi router.
func (h *FavouriteHandler) Routes(r chi.Router) {
	requireAuth := middleware.RequireAuth(h.authSvc)
	optAuth := middleware.OptionalAuth(h.authSvc)

	// Favourites (with and without /me prefix)
	r.Group(func(sub chi.Router) {
		sub.Use(requireAuth)
		sub.Post("/favourites", h.AddFavourite)
		sub.Post("/me/favourites", h.AddFavourite)
		sub.Delete("/favourites/{id}", h.RemoveFavourite)
		sub.Delete("/me/favourites/{id}", h.RemoveFavourite)
		sub.Get("/favourites", h.ListFavourites)
		sub.Get("/me/favourites", h.ListFavourites)
		sub.Get("/history", h.GetHistory)
		sub.Get("/me/history", h.GetHistory)
		sub.Get("/top-played", h.GetTopPlayed)
		sub.Get("/me/top-played", h.GetTopPlayed)

		// Artist follows
		sub.Post("/artists/favourites", h.AddArtistFavourite)
		sub.Post("/me/artists/favourites", h.AddArtistFavourite)
		sub.Post("/me/artists/favorite", h.AddArtistFavourite)
		sub.Post("/me/artist/favorite", h.AddArtistFavourite)
		sub.Delete("/artists/favourites/{id}", h.RemoveArtistFavourite)
		sub.Delete("/me/artists/favourites/{id}", h.RemoveArtistFavourite)
		sub.Delete("/me/artists/favorite/{id}", h.RemoveArtistFavourite)
		sub.Delete("/me/artist/favorite/{id}", h.RemoveArtistFavourite)
	})

	r.Group(func(sub chi.Router) {
		sub.Use(optAuth)
		sub.Get("/favourites/ids", h.ListFavouriteIDs)
		sub.Get("/me/favourites/ids", h.ListFavouriteIDs)
		sub.Get("/artists/favourites/ids", h.ListFavouriteArtistIDs)
		sub.Get("/me/artists/favourites/ids", h.ListFavouriteArtistIDs)
		sub.Post("/listening-events", h.RecordListeningEvents)
		sub.Post("/me/listening-events", h.RecordListeningEvents)
	})
}

// AddFavourite handles POST /favourites.
func (h *FavouriteHandler) AddFavourite(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		api.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var payload struct {
		TrackID string `json:"track_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	trackID := strings.TrimSpace(payload.TrackID)
	if trackID == "" {
		api.RespondError(w, http.StatusBadRequest, "track_id is required")
		return
	}

	alreadyExists, err := h.favSvc.AddFavourite(r.Context(), userID, trackID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"already_exists": alreadyExists,
	})
}

// RemoveFavourite handles DELETE /favourites/{id}.
func (h *FavouriteHandler) RemoveFavourite(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		api.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		api.RespondError(w, http.StatusBadRequest, "track_id is required")
		return
	}

	deleted, err := h.favSvc.RemoveFavourite(r.Context(), userID, id)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"deleted": deleted,
	})
}

// ListFavouriteIDs handles GET /favourites/ids.
func (h *FavouriteHandler) ListFavouriteIDs(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		api.RespondJSON(w, http.StatusOK, models.FavouriteIdsResponse{
			Page:    1,
			PerPage: 200,
			Total:   0,
			IDs:     []string{},
			Exists:  false,
		})
		return
	}

	page := api.ParseQueryInt(r, "page", 1)
	limit := api.ParseQueryInt(r, "limit", 200)

	resp, err := h.favSvc.GetFavouriteIDs(r.Context(), userID, page, limit)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, resp)
}

// ListFavourites handles GET /favourites.
func (h *FavouriteHandler) ListFavourites(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		api.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	page := api.ParseQueryInt(r, "page", 1)
	limit := api.ParseQueryInt(r, "limit", 20)

	resp, err := h.favSvc.GetFavourites(r.Context(), userID, page, limit)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, resp)
}

// GetHistory handles GET /history and GET /me/history.
func (h *FavouriteHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		api.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	limit := api.ParseQueryInt(r, "limit", 100)
	items, err := h.favSvc.GetUserHistory(r.Context(), userID, limit)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, &models.BrowseResponse{
		Page:    1,
		PerPage: limit,
		Total:   int64(len(items)),
		Items:   items,
	})
}

// GetTopPlayed handles GET /top-played and GET /me/top-played.
func (h *FavouriteHandler) GetTopPlayed(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		api.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	page := api.ParseQueryInt(r, "page", 1)
	limit := api.ParseQueryInt(r, "limit", 50)
	items, total, err := h.favSvc.GetUserTopPlayed(r.Context(), userID, page, limit)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, &models.BrowseResponse{
		Page:    page,
		PerPage: limit,
		Total:   total,
		Items:   items,
	})
}

// AddArtistFavourite handles POST /artists/favourites.
func (h *FavouriteHandler) AddArtistFavourite(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		api.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var aid string
	var payload struct {
		ArtistID string `json:"artist_id"`
		ID       string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
		aid = payload.ArtistID
		if aid == "" {
			aid = payload.ID
		}
	}
	if aid == "" {
		aid = r.URL.Query().Get("artist_id")
		if aid == "" {
			aid = r.URL.Query().Get("id")
		}
	}
	aid = strings.TrimSpace(aid)
	if aid == "" {
		api.RespondError(w, http.StatusBadRequest, "artist_id is required")
		return
	}

	alreadyExists, err := h.favSvc.AddFavouriteArtist(r.Context(), userID, aid)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"already_exists": alreadyExists,
	})
}

// RemoveArtistFavourite handles DELETE /artists/favourites/{id}.
func (h *FavouriteHandler) RemoveArtistFavourite(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		api.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		api.RespondError(w, http.StatusBadRequest, "artist_id is required")
		return
	}

	deleted, err := h.favSvc.RemoveFavouriteArtist(r.Context(), userID, id)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"deleted": deleted,
	})
}

// ListFavouriteArtistIDs handles GET /artists/favourites/ids.
func (h *FavouriteHandler) ListFavouriteArtistIDs(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		api.RespondJSON(w, http.StatusOK, models.FavouriteArtistsResponse{
			OK:  true,
			IDs: []string{},
		})
		return
	}

	ids, err := h.favSvc.GetFavouriteArtistIDs(r.Context(), userID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, models.FavouriteArtistsResponse{
		OK:  true,
		IDs: ids,
	})
}

// RecordListeningEvents handles POST /listening-events.
func (h *FavouriteHandler) RecordListeningEvents(w http.ResponseWriter, r *http.Request) {
	userID, hasUser := middleware.GetUserID(r.Context())
	var uIDPtr *int64
	if hasUser {
		uIDPtr = &userID
	}

	var payload models.ListeningEventsPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid listening events payload")
		return
	}

	count := len(payload.Events)
	if count > 0 {
		count, _ = h.favSvc.RecordListeningEvents(r.Context(), uIDPtr, payload.Events)
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"count": count,
	})
}
