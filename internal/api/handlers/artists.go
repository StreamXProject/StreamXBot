package handlers

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"streamgo/internal/api"
	"streamgo/internal/services"
)

// ArtistHandler handles endpoints for artist discovery and profiles.
type ArtistHandler struct {
	svc *services.ArtistAlbumService
}

// NewArtistHandler creates an ArtistHandler instance.
func NewArtistHandler(svc *services.ArtistAlbumService) *ArtistHandler {
	return &ArtistHandler{svc: svc}
}

// Routes mounts artist endpoints on the Chi router.
func (h *ArtistHandler) Routes(r chi.Router) {
	r.Get("/artists", h.List)
	r.Get("/artists/{id}", h.GetByID)
	r.Get("/artists/{id}/tracks", h.GetTracks)
}

// List handles GET /artists with pagination.
func (h *ArtistHandler) List(w http.ResponseWriter, r *http.Request) {
	page := api.ParseQueryInt(r, "page", 1)
	perPage := api.ParseQueryInt(r, "per_page", 50)
	if limit := api.ParseQueryInt(r, "limit", 0); limit > 0 {
		perPage = limit
	}
	refresh := api.ParseQueryBool(r, "refresh", false)

	resp, err := h.svc.ListArtists(r.Context(), page, perPage, refresh)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, resp)
}

// GetByID handles GET /artists/{id}.
func (h *ArtistHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		api.RespondError(w, http.StatusBadRequest, "artist id is required")
		return
	}

	detail, err := h.svc.GetArtistDetail(r.Context(), id)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if detail == nil {
		api.RespondError(w, http.StatusNotFound, "artist not found")
		return
	}

	api.RespondJSON(w, http.StatusOK, detail)
}

// GetTracks handles GET /artists/{id}/tracks.
func (h *ArtistHandler) GetTracks(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		api.RespondError(w, http.StatusBadRequest, "artist id is required")
		return
	}

	page := api.ParseQueryInt(r, "page", 1)
	perPage := api.ParseQueryInt(r, "per_page", 50)
	if limit := api.ParseQueryInt(r, "limit", 0); limit > 0 {
		perPage = limit
	}

	tracks, total, err := h.svc.GetArtistTracksPaginated(r.Context(), id, page, perPage)
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
