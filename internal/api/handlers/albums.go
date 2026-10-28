package handlers

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"streamgo/internal/api"
	"streamgo/internal/services"
)

// AlbumHandler handles HTTP endpoints for albums.
type AlbumHandler struct {
	svc *services.ArtistAlbumService
}

// NewAlbumHandler creates an AlbumHandler instance.
func NewAlbumHandler(svc *services.ArtistAlbumService) *AlbumHandler {
	return &AlbumHandler{svc: svc}
}

// Routes mounts album endpoints on Chi router.
func (h *AlbumHandler) Routes(r chi.Router) {
	r.Get("/albums", h.List)
	r.Get("/albums/{id}", h.GetByID)
	r.Get("/albums/{id}/tracks", h.GetTracks)
}

// List handles GET /albums with pagination and optional artist filter.
func (h *AlbumHandler) List(w http.ResponseWriter, r *http.Request) {
	page := api.ParseQueryInt(r, "page", 1)
	perPage := api.ParseQueryInt(r, "per_page", 50)
	if limit := api.ParseQueryInt(r, "limit", 0); limit > 0 {
		perPage = limit
	}
	refresh := api.ParseQueryBool(r, "refresh", false)
	artistFilter := strings.TrimSpace(r.URL.Query().Get("artist"))

	resp, err := h.svc.ListAlbums(r.Context(), page, perPage, artistFilter, refresh)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, resp)
}

// GetByID handles GET /albums/{id}.
func (h *AlbumHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		api.RespondError(w, http.StatusBadRequest, "album id is required")
		return
	}

	detail, err := h.svc.GetAlbumDetail(r.Context(), id)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if detail == nil {
		api.RespondError(w, http.StatusNotFound, "album not found")
		return
	}

	api.RespondJSON(w, http.StatusOK, detail)
}

// GetTracks handles GET /albums/{id}/tracks.
func (h *AlbumHandler) GetTracks(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		api.RespondError(w, http.StatusBadRequest, "album id is required")
		return
	}

	page := api.ParseQueryInt(r, "page", 1)
	perPage := api.ParseQueryInt(r, "per_page", 50)
	if limit := api.ParseQueryInt(r, "limit", 0); limit > 0 {
		perPage = limit
	}

	tracks, total, err := h.svc.GetAlbumTracksPaginated(r.Context(), id, page, perPage)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"page":     page,
		"per_page": perPage,
		"total":    total,
		"items":    tracks,
	})
}
