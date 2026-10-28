package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"streamgo/internal/api"
	"streamgo/internal/config"
	"streamgo/internal/services"
)

// SourcesHandler manages admin source filtering and ingestion controls.
type SourcesHandler struct {
	cfg     *config.Config
	filter  *services.AccessFilter
	authSvc *services.AuthService

	mu      sync.RWMutex
	allowed []map[string]any
	banned  []map[string]any
}

// NewSourcesHandler creates a new SourcesHandler instance.
func NewSourcesHandler(cfg *config.Config, filter *services.AccessFilter, authSvc *services.AuthService) *SourcesHandler {
	return &SourcesHandler{
		cfg:     cfg,
		filter:  filter,
		authSvc: authSvc,
		allowed: make([]map[string]any, 0),
		banned:  make([]map[string]any, 0),
	}
}

// Routes mounts the admin sources endpoints.
func (h *SourcesHandler) Routes(r chi.Router) {
	r.Route("/admin/sources", func(sub chi.Router) {
		sub.Use(h.requireAdminAuth)

		sub.Get("/", h.GetOverview)

		sub.Get("/mode", h.GetMode)
		sub.Post("/mode", h.SetMode)

		sub.Get("/allowed", h.ListAllowed)
		sub.Post("/allowed", h.AddAllowed)
		sub.Delete("/allowed/{source_id}", h.RemoveAllowed)

		sub.Get("/banned", h.ListBanned)
		sub.Post("/banned", h.AddBanned)
		sub.Delete("/banned/{source_id}", h.RemoveBanned)
	})
}

func (h *SourcesHandler) requireAdminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret := r.Header.Get("X-Secret-Key")
		if h.cfg != nil && h.cfg.SecretKey != "" && secret == h.cfg.SecretKey {
			next.ServeHTTP(w, r)
			return
		}
		api.RespondError(w, http.StatusUnauthorized, "Admin authentication required")
	})
}

// GetOverview returns summary of filter mode and allowed/banned sources.
func (h *SourcesHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	mode := services.FilterModeGroupOnly
	if h.cfg != nil {
		mode = h.cfg.FilterMode
	}

	var channelID int64
	collaborators := []int64{}
	if h.cfg != nil {
		channelID = h.cfg.ChannelID
		collaborators = h.cfg.CollaboratorIDs
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":                   true,
		"filter_mode":          mode,
		"filter_mode_name":     services.FilterModeToString(mode),
		"channel_id":           channelID,
		"collaborator_ids":     collaborators,
		"allowed_sources_count": len(h.allowed),
		"banned_sources_count":  len(h.banned),
		"allowed_sources":      h.allowed,
		"banned_sources":       h.banned,
	})
}

// GetMode returns the current filter mode.
func (h *SourcesHandler) GetMode(w http.ResponseWriter, r *http.Request) {
	mode := services.FilterModeGroupOnly
	if h.cfg != nil {
		mode = h.cfg.FilterMode
	}
	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"filter_mode":      mode,
		"filter_mode_name": services.FilterModeToString(mode),
	})
}

// SetMode updates the active filter mode.
func (h *SourcesHandler) SetMode(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Mode any `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	parsed := services.ParseFilterMode(payload.Mode)
	if h.cfg != nil {
		h.cfg.FilterMode = parsed
	}

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"filter_mode":      parsed,
		"filter_mode_name": services.FilterModeToString(parsed),
	})
}

// ListAllowed returns all allowed sources.
func (h *SourcesHandler) ListAllowed(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"count":   len(h.allowed),
		"sources": h.allowed,
	})
}

// AddAllowed adds a source to allowed list.
func (h *SourcesHandler) AddAllowed(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		SourceID   any    `json:"source_id"`
		SourceType string `json:"source_type"`
		Name       string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var sid int64
	switch v := payload.SourceID.(type) {
	case float64:
		sid = int64(v)
	case int64:
		sid = v
	case int:
		sid = int64(v)
	case string:
		sid, _ = strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	}

	doc := map[string]any{
		"source_id":   sid,
		"source_type": payload.SourceType,
		"name":        payload.Name,
		"created_at":  time.Now().Unix(),
	}

	h.mu.Lock()
	h.allowed = append(h.allowed, doc)
	h.mu.Unlock()

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"source": doc,
	})
}

// RemoveAllowed removes a source from allowed list.
func (h *SourcesHandler) RemoveAllowed(w http.ResponseWriter, r *http.Request) {
	sidStr := chi.URLParam(r, "source_id")
	sid, err := strconv.ParseInt(sidStr, 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid source_id")
		return
	}

	h.mu.Lock()
	newAllowed := make([]map[string]any, 0, len(h.allowed))
	removed := false
	for _, item := range h.allowed {
		if id, ok := item["source_id"].(int64); ok && id == sid {
			removed = true
			continue
		}
		newAllowed = append(newAllowed, item)
	}
	h.allowed = newAllowed
	h.mu.Unlock()

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"source_id": sid,
		"removed":   removed,
	})
}

// ListBanned returns all banned sources.
func (h *SourcesHandler) ListBanned(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"count":   len(h.banned),
		"sources": h.banned,
	})
}

// AddBanned bans a source.
func (h *SourcesHandler) AddBanned(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		SourceID   any    `json:"source_id"`
		SourceType string `json:"source_type"`
		Name       string `json:"name"`
		Reason     string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var sid int64
	switch v := payload.SourceID.(type) {
	case float64:
		sid = int64(v)
	case int64:
		sid = v
	case int:
		sid = int64(v)
	case string:
		sid, _ = strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	}

	doc := map[string]any{
		"source_id":   sid,
		"source_type": payload.SourceType,
		"name":        payload.Name,
		"reason":      payload.Reason,
		"created_at":  time.Now().Unix(),
	}

	h.mu.Lock()
	h.banned = append(h.banned, doc)
	h.mu.Unlock()

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"source": doc,
	})
}

// RemoveBanned unbans a source.
func (h *SourcesHandler) RemoveBanned(w http.ResponseWriter, r *http.Request) {
	sidStr := chi.URLParam(r, "source_id")
	sid, err := strconv.ParseInt(sidStr, 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid source_id")
		return
	}

	h.mu.Lock()
	newBanned := make([]map[string]any, 0, len(h.banned))
	removed := false
	for _, item := range h.banned {
		if id, ok := item["source_id"].(int64); ok && id == sid {
			removed = true
			continue
		}
		newBanned = append(newBanned, item)
	}
	h.banned = newBanned
	h.mu.Unlock()

	api.RespondJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"source_id": sid,
		"removed":   removed,
	})
}
