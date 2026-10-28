package handlers

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"streamgo/internal/api"
	"streamgo/internal/services"
)

// TopicHandler handles routes related to topics and source channels.
type TopicHandler struct {
	trackService *services.TrackService
}

// NewTopicHandler creates a new TopicHandler.
func NewTopicHandler(svc *services.TrackService) *TopicHandler {
	return &TopicHandler{trackService: svc}
}

// Routes mounts topic and channel endpoints.
func (h *TopicHandler) Routes(r chi.Router) {
	r.Get("/topics", h.ListTopics)
	r.Get("/topics/{name}/tracks", h.TopicTracks)
	r.Get("/channelids", h.ChannelIDs)
}

// ListTopics handles GET /topics.
func (h *TopicHandler) ListTopics(w http.ResponseWriter, r *http.Request) {
	limit := api.ParseQueryInt(r, "limit", 100)
	channelID := api.ParseQueryInt64(r, "channel_id", 0)

	resp, err := h.trackService.GetTopics(r.Context(), channelID, limit)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, resp)
}

// TopicTracks handles GET /topics/{name}/tracks with pagination.
func (h *TopicHandler) TopicTracks(w http.ResponseWriter, r *http.Request) {
	rawName := chi.URLParam(r, "name")
	topicName, err := url.PathUnescape(rawName)
	if err != nil || strings.TrimSpace(topicName) == "" {
		topicName = rawName
	}
	topicName = strings.TrimSpace(topicName)
	if topicName == "" {
		api.RespondError(w, http.StatusBadRequest, "topic name is required")
		return
	}

	page := api.ParseQueryInt(r, "page", 1)
	perPage := api.ParseQueryInt(r, "per_page", 20)
	if limit := api.ParseQueryInt(r, "limit", 0); limit > 0 {
		perPage = limit
	}
	channelID := api.ParseQueryInt64(r, "channel_id", 0)

	resp, err := h.trackService.Browse(r.Context(), page, perPage, "", topicName, channelID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, resp)
}

// ChannelIDs handles GET /channelids.
func (h *TopicHandler) ChannelIDs(w http.ResponseWriter, r *http.Request) {
	resp, err := h.trackService.GetChannelIDs(r.Context())
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondJSON(w, http.StatusOK, resp)
}
