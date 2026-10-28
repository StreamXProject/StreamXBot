package handlers

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"streamgo/internal/api"
	"streamgo/internal/repository"
	"streamgo/internal/services"
)

var (
	preRegex     = regexp.MustCompile(`(?s)<pre[^>]*>(.*?)</pre>`)
	articleRegex = regexp.MustCompile(`(?s)<article[^>]*id="_tl_editor"[^>]*>(.*?)</article>`)
	tagRegex     = regexp.MustCompile(`<[^>]+>`)
)

func isURL(s string) bool {
	trimmed := strings.TrimSpace(s)
	return strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://")
}

func fetchTelegraphLyrics(ctx context.Context, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	htmlStr := string(body)
	if m := preRegex.FindStringSubmatch(htmlStr); len(m) > 1 {
		decoded := html.UnescapeString(m[1])
		return strings.TrimSpace(decoded), nil
	}

	if m := articleRegex.FindStringSubmatch(htmlStr); len(m) > 1 {
		content := m[1]
		content = strings.ReplaceAll(content, "<br/>", "\n")
		content = strings.ReplaceAll(content, "<br>", "\n")
		content = strings.ReplaceAll(content, "</p>", "\n")
		content = tagRegex.ReplaceAllString(content, "")
		decoded := html.UnescapeString(content)
		return strings.TrimSpace(decoded), nil
	}

	return "", fmt.Errorf("no lyrics content found in telegraph page")
}

// MediaExtraHandler handles static cover images and track lyrics.
type MediaExtraHandler struct {
	trackSvc  *services.TrackService
	lyricsSvc *services.LyricsEnrichmentService
	trackRepo repository.TrackRepository
}

// NewMediaExtraHandler creates a new MediaExtraHandler.
func NewMediaExtraHandler(trackSvc *services.TrackService, lyricsSvc *services.LyricsEnrichmentService, trackRepo repository.TrackRepository) *MediaExtraHandler {
	return &MediaExtraHandler{
		trackSvc:  trackSvc,
		lyricsSvc: lyricsSvc,
		trackRepo: trackRepo,
	}
}

// Routes mounts cover and lyrics endpoints on Chi router.
func (h *MediaExtraHandler) Routes(r chi.Router) {
	r.Get("/covers/file/{file_key}.png", h.GetCoverFile)
	r.Head("/covers/file/{file_key}.png", h.GetCoverFile)
	r.Get("/cover/{id}", h.GetCoverRedirect)
	r.Head("/cover/{id}", h.GetCoverRedirect)
	r.Get("/lyrics/{id}", h.GetLyrics)
	r.Get("/tracks/{id}/lyrics", h.GetLyrics)
}


// GetCoverFile serves cached generated cover PNG files.
func (h *MediaExtraHandler) GetCoverFile(w http.ResponseWriter, r *http.Request) {
	fileKey := strings.TrimSpace(chi.URLParam(r, "file_key"))
	if fileKey == "" {
		api.RespondError(w, http.StatusBadRequest, "file_key is required")
		return
	}

	// Look in GenCovers in current or parent dirs
	searchDirs := []string{
		"GenCovers",
		"../GenCovers",
		"../../GenCovers",
		"/home/misfit/Work/StreamXBot/GenCovers",
	}

	var foundPath string
	for _, dir := range searchDirs {
		candidate := filepath.Join(dir, fmt.Sprintf("%s.png", fileKey))
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			foundPath = candidate
			break
		}
	}

	if foundPath == "" {
		api.RespondError(w, http.StatusNotFound, "cover not found")
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, foundPath)
}

// GetCoverRedirect resolves cover URL for a track and redirects.
func (h *MediaExtraHandler) GetCoverRedirect(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		api.RespondError(w, http.StatusBadRequest, "id is required")
		return
	}

	track, err := h.trackSvc.GetTrack(r.Context(), id)
	if err != nil || track == nil {
		api.RespondError(w, http.StatusNotFound, "track not found")
		return
	}

	item := track.ToBrowseItem()
	coverURL := item.CoverURL
	if coverURL == "" {
		coverURL = track.EffectiveCoverURL()
	}

	if coverURL == "" {
		api.RespondError(w, http.StatusNotFound, "cover not available")
		return
	}

	http.Redirect(w, r, coverURL, http.StatusFound)
}

// GetLyrics returns lyrics for a track either in plain text or JSON matching Python source.
func (h *MediaExtraHandler) GetLyrics(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		api.RespondError(w, http.StatusBadRequest, "track id is required")
		return
	}

	track, err := h.trackSvc.GetTrack(r.Context(), id)
	if err != nil || track == nil {
		api.RespondError(w, http.StatusNotFound, "track not found")
		return
	}

	requestedProvider := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("provider")))

	var (
		lyricsText   string
		lyricsKind   string = "plain"
		lyricsSource string = "unknown"
		telegraphURL string
	)

	// 1. Identify if track has a telegraph link
	rawLyrics := strings.TrimSpace(track.Lyrics)
	if rawLyrics == "" {
		rawLyrics = strings.TrimSpace(track.Audio.Lyrics)
	}
	if isURL(rawLyrics) {
		telegraphURL = rawLyrics
	}

	// 2. Check if lyrics_cache has valid lyrics text
	if track.LyricsCache != nil {
		if txt, ok := track.LyricsCache["text"].(string); ok && strings.TrimSpace(txt) != "" && !isURL(strings.TrimSpace(txt)) {
			cachedSrc, _ := track.LyricsCache["source"].(string)
			// If no specific provider was requested, or the cached source matches
			if requestedProvider == "" || requestedProvider == "auto" || strings.EqualFold(cachedSrc, requestedProvider) {
				lyricsText = strings.TrimSpace(txt)
				if k, ok := track.LyricsCache["kind"].(string); ok && k != "" {
					lyricsKind = k
				}
				if cachedSrc != "" {
					lyricsSource = cachedSrc
				}
			}
		}
	}

	// 3. If rawLyrics is NOT a URL and not empty, and we don't have cached lyrics yet
	if lyricsText == "" && rawLyrics != "" && !isURL(rawLyrics) {
		lyricsText = rawLyrics
	}

	// 4. If we still don't have lyrics, but we have a Telegraph / Web URL, fetch & parse the page
	if lyricsText == "" && telegraphURL != "" {
		scraped, err := fetchTelegraphLyrics(r.Context(), telegraphURL)
		if err == nil && strings.TrimSpace(scraped) != "" {
			lyricsText = strings.TrimSpace(scraped)
			lyricsKind = "synced"
			if strings.Contains(lyricsText, "<") && strings.Contains(lyricsText, ">") {
				lyricsKind = "richsync"
			}
			lyricsSource = "telegraph"
			// Persist to MongoDB cache so future calls are instant
			if h.trackRepo != nil {
				_ = h.trackRepo.UpdateLyricsCache(r.Context(), id, lyricsText, lyricsKind, lyricsSource, telegraphURL)
			}
		}
	}

	// 5. If we still don't have lyrics (or a specific provider was requested), query lyrics providers live
	if (lyricsText == "" || (requestedProvider != "" && requestedProvider != "auto" && !strings.EqualFold(lyricsSource, requestedProvider))) && h.lyricsSvc != nil {
		title := track.Audio.Title
		artist := track.Audio.Artist
		if artist == "" {
			artist = track.EffectiveArtist()
		}
		album := track.Audio.Album

		res, err := h.lyricsSvc.FetchLyricsWithProvider(r.Context(), title, artist, album, requestedProvider)
		if err == nil && res != nil && strings.TrimSpace(res.Lyrics) != "" {
			lyricsText = strings.TrimSpace(res.Lyrics)
			if res.Kind != "" {
				lyricsKind = res.Kind
			}
			if res.Source != "" {
				lyricsSource = res.Source
			}
			if h.trackRepo != nil {
				_ = h.trackRepo.UpdateLyricsCache(r.Context(), id, lyricsText, lyricsKind, lyricsSource, telegraphURL)
			}
		}
	}

	if lyricsText == "" {
		api.RespondError(w, http.StatusNotFound, "lyrics not found")
		return
	}

	format := strings.ToLower(r.URL.Query().Get("format"))
	accept := r.Header.Get("Accept")
	if format == "plain" || (format != "json" && strings.Contains(accept, "text/plain")) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(lyricsText))
		return
	}

	resp := map[string]any{
		"ok":       true,
		"track_id": id,
		"lyrics":   lyricsText,
		"kind":     lyricsKind,
		"source":   lyricsSource,
	}
	if telegraphURL != "" {
		resp["telegraph_url"] = telegraphURL
	}

	api.RespondJSON(w, http.StatusOK, resp)
}
