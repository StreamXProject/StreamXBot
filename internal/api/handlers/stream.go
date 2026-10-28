package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"streamgo/internal/api"
	"streamgo/internal/api/middleware"
	"streamgo/internal/services"
)

// StreamHandler handles audio streaming and download endpoints.
type StreamHandler struct {
	streamService    *services.StreamService
	authSvc          *services.AuthService
	histSvc          *services.HistoryService
	recentlyRecorded sync.Map
}

// NewStreamHandler creates a new StreamHandler.
func NewStreamHandler(svc *services.StreamService, authSvc *services.AuthService, histSvc *services.HistoryService) *StreamHandler {
	return &StreamHandler{
		streamService: svc,
		authSvc:       authSvc,
		histSvc:       histSvc,
	}
}

// Routes mounts the streaming and download endpoints.
func (h *StreamHandler) Routes(r chi.Router) {
	optionalAuth := middleware.OptionalAuth(h.authSvc)

	r.With(optionalAuth).Get("/tracks/{id}/stream", h.Stream)
	r.With(optionalAuth).Head("/tracks/{id}/stream", h.Stream)
	r.With(optionalAuth).Get("/stream/{id}", h.Stream)
	r.With(optionalAuth).Head("/stream/{id}", h.Stream)

	r.With(optionalAuth).Get("/tracks/{id}/download", h.Download)
	r.With(optionalAuth).Head("/tracks/{id}/download", h.Download)
	r.With(optionalAuth).Get("/download/{id}", h.Download)
	r.With(optionalAuth).Head("/download/{id}", h.Download)

	r.Get("/tracks/{id}/warm", h.Warm)
}

// Stream handles GET and HEAD /tracks/{id}/stream with HTTP Range support.
func (h *StreamHandler) Stream(w http.ResponseWriter, r *http.Request) {
	trackID := chi.URLParam(r, "id")
	trackID = strings.TrimSpace(trackID)
	if trackID == "" {
		api.RespondError(w, http.StatusBadRequest, "track id is required")
		return
	}

	// Record playback in userHistory and counters upon stream start
	if h.histSvc != nil && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		userID := h.extractUserID(r)
		if userID > 0 {
			now := float64(time.Now().Unix())
			cacheKey := fmt.Sprintf("%d:%s", userID, trackID)
			shouldRecord := true
			if val, ok := h.recentlyRecorded.Load(cacheKey); ok {
				if lastTs, ok := val.(float64); ok && (now-lastTs < 15.0) {
					shouldRecord = false
				}
			}

			if shouldRecord {
				h.recentlyRecorded.Store(cacheKey, now)
				go func(uid int64, tid string) {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_ = h.histSvc.RecordPlay(ctx, uid, tid, "direct")
				}(userID, trackID)
			}
		}
	}

	h.streamService.StreamTrack(w, r, trackID)
}

func (h *StreamHandler) extractUserID(r *http.Request) int64 {
	if uid, ok := middleware.GetUserID(r.Context()); ok && uid > 0 {
		return uid
	}
	if h.authSvc == nil {
		return 0
	}
	token := middleware.ExtractToken(r)
	if token == "" {
		return 0
	}
	claims, err := h.authSvc.VerifyTokenClaims(token)
	if err != nil || claims == nil || claims.UserID <= 0 {
		return 0
	}
	return claims.UserID
}

// Download handles GET and HEAD /tracks/{id}/download with Content-Disposition attachment.
func (h *StreamHandler) Download(w http.ResponseWriter, r *http.Request) {
	trackID := chi.URLParam(r, "id")
	trackID = strings.TrimSpace(trackID)
	if trackID == "" {
		api.RespondError(w, http.StatusBadRequest, "track id is required")
		return
	}

	track, _ := h.streamService.GetTrack(r.Context(), trackID)
	if track != nil {
		filename := track.Audio.Title
		if track.Audio.Artist != "" {
			filename = fmt.Sprintf("%s - %s", track.Audio.Artist, track.Audio.Title)
		}
		if filename == "" {
			filename = track.Telegram.FileName
		}
		if filename == "" {
			filename = fmt.Sprintf("track-%s", track.ID)
		}
		ext := track.Audio.Type
		if (h.streamService.ALACService() != nil && h.streamService.ALACService().ShouldDecodeALAC(r, track)) || strings.ToLower(r.URL.Query().Get("format")) == "flac" {
			ext = "flac"
		}
		if ext == "alac" {
			ext = "m4a"
		}
		if ext == "" {
			ext = "mp3"
		}
		cleanFilename := strings.TrimSuffix(filename, filepath.Ext(filename))
		filename = fmt.Sprintf("%s.%s", cleanFilename, ext)

		fallback := strings.ReplaceAll(filename, `"`, `_`)
		encoded := url.QueryEscape(filename)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, fallback, encoded))
	} else {
		w.Header().Set("Content-Disposition", `attachment; filename="track.mp3"`)
	}

	h.streamService.StreamTrack(w, r, trackID)
}

// Warm handles GET /tracks/{id}/warm to prewarm playback cache.
func (h *StreamHandler) Warm(w http.ResponseWriter, r *http.Request) {
	trackID := chi.URLParam(r, "id")
	trackID = strings.TrimSpace(trackID)
	if trackID == "" {
		api.RespondError(w, http.StatusBadRequest, "track id is required")
		return
	}

	ready := h.streamService.IsTelegramReady()
	go h.streamService.WarmTrack(context.Background(), trackID)
	api.RespondJSON(w, http.StatusOK, map[string]any{"ok": true, "ready": ready})
}
