package handlers

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"streamgo/internal/api"
	"streamgo/internal/config"
	"streamgo/internal/models"
	"streamgo/internal/repository"
)

// ShareHandler handles rich embeds, OpenGraph/Twitter card previews, and share redirects.
type ShareHandler struct {
	trackRepo    repository.TrackRepository
	playlistRepo repository.FavouritePlaylistRepository
	cfg          *config.Config
}

// NewShareHandler creates a new ShareHandler.
func NewShareHandler(trackRepo repository.TrackRepository, playlistRepo repository.FavouritePlaylistRepository, cfg *config.Config) *ShareHandler {
	return &ShareHandler{
		trackRepo:    trackRepo,
		playlistRepo: playlistRepo,
		cfg:          cfg,
	}
}

// Routes mounts /share endpoints.
func (h *ShareHandler) Routes(r chi.Router) {
	r.Get("/share/{id}", h.HandleShare)
	r.Get("/share/tracks/{id}", h.HandleTrackShare)
	r.Get("/share/playlists/{id}", h.HandlePlaylistShare)
}

// HandleShare detects whether id is a track or playlist and renders the preview.
func (h *ShareHandler) HandleShare(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		http.Error(w, "Share ID required", http.StatusBadRequest)
		return
	}

	// 1. Try track first
	track, err := h.trackRepo.GetByID(r.Context(), id)
	if err == nil && track != nil {
		h.renderTrack(w, r, track)
		return
	}

	// 2. Try playlist
	playlist, err := h.playlistRepo.GetPublicPlaylistByID(r.Context(), id)
	if err == nil && playlist != nil {
		h.renderPlaylist(w, r, playlist)
		return
	}

	http.Error(w, "Track or playlist not found", http.StatusNotFound)
}

// HandleTrackShare renders track preview.
func (h *ShareHandler) HandleTrackShare(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	track, err := h.trackRepo.GetByID(r.Context(), id)
	if err != nil || track == nil {
		http.Error(w, "Track not found", http.StatusNotFound)
		return
	}
	h.renderTrack(w, r, track)
}

// HandlePlaylistShare renders playlist preview.
func (h *ShareHandler) HandlePlaylistShare(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	playlist, err := h.playlistRepo.GetPublicPlaylistByID(r.Context(), id)
	if err != nil || playlist == nil {
		http.Error(w, "Playlist not found", http.StatusNotFound)
		return
	}
	h.renderPlaylist(w, r, playlist)
}

func (h *ShareHandler) wantsJSON(r *http.Request) bool {
	if r.URL.Query().Get("format") == "json" {
		return true
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	return strings.Contains(accept, "application/json") && !strings.Contains(accept, "text/html")
}

func (h *ShareHandler) renderTrack(w http.ResponseWriter, r *http.Request, track *models.Track) {
	title := track.Audio.Title
	if title == "" {
		title = "Unknown Title"
	}
	artist := track.Audio.Artist
	if artist == "" {
		artist = "Unknown Artist"
	}
	album := track.Audio.Album

	baseURL := h.baseURL(r)
	shareURL := fmt.Sprintf("%s/share/%s", baseURL, track.ID)
	streamURL := fmt.Sprintf("%s/stream/%s", baseURL, track.ID)
	downloadURL := fmt.Sprintf("%s/tracks/%s/download", baseURL, track.ID)
	coverURL := fmt.Sprintf("%s/stream/cover/%s", baseURL, track.ID)

	desc := fmt.Sprintf("Listen to %s by %s on StreamX.", title, artist)
	if album != "" {
		desc = fmt.Sprintf("Listen to %s by %s from the album %s on StreamX.", title, artist, album)
	}

	if h.wantsJSON(r) {
		api.RespondJSON(w, http.StatusOK, map[string]any{
			"ok":           true,
			"type":         "track",
			"id":           track.ID,
			"title":        title,
			"artist":       artist,
			"album":        album,
			"duration_sec": track.Audio.DurationSec,
			"audio_type":   track.Audio.Type,
			"cover_url":    coverURL,
			"stream_url":   streamURL,
			"download_url": downloadURL,
			"share_url":    shareURL,
			"open_graph": map[string]string{
				"og:title":       fmt.Sprintf("%s - %s", title, artist),
				"og:description": desc,
				"og:image":       coverURL,
				"og:audio":       streamURL,
				"og:type":        "music.song",
			},
		})
		return
	}

	durationFormatted := formatDuration(int(track.Audio.DurationSec))
	pageTitle := html.EscapeString(fmt.Sprintf("%s – %s | StreamX", title, artist))
	escTitle := html.EscapeString(title)
	escArtist := html.EscapeString(artist)
	escAlbum := html.EscapeString(album)
	escDesc := html.EscapeString(desc)
	escType := strings.ToUpper(track.Audio.Type)
	if escType == "" {
		escType = "AUDIO"
	}

	// Extra metadata
	bitrateInfo := ""
	if track.Audio.BitrateKbps != nil && *track.Audio.BitrateKbps > 0 {
		bitrateInfo = fmt.Sprintf("%d kbps", *track.Audio.BitrateKbps)
	}
	sampleInfo := ""
	if track.Audio.SamplingRateHz != nil && *track.Audio.SamplingRateHz > 0 {
		sampleInfo = fmt.Sprintf("%.1f kHz", float64(*track.Audio.SamplingRateHz)/1000.0)
	}
	bitDepthInfo := ""
	if track.Audio.BitDepth != nil && *track.Audio.BitDepth > 0 {
		bitDepthInfo = fmt.Sprintf("%d-bit", *track.Audio.BitDepth)
	}
	qualityParts := []string{}
	if bitDepthInfo != "" {
		qualityParts = append(qualityParts, bitDepthInfo)
	}
	if sampleInfo != "" {
		qualityParts = append(qualityParts, sampleInfo)
	}
	if bitrateInfo != "" {
		qualityParts = append(qualityParts, bitrateInfo)
	}
	qualityStr := ""
	if len(qualityParts) > 0 {
		qualityStr = html.EscapeString(strings.Join(qualityParts, " · "))
	}

	yearStr := ""
	if track.Audio.Year != nil && *track.Audio.Year > 0 {
		yearStr = fmt.Sprintf("%d", *track.Audio.Year)
	}
	genreStr := html.EscapeString(track.Audio.Genre)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s</title>

  <!-- OpenGraph / Facebook / Telegram -->
  <meta property="og:site_name" content="StreamX">
  <meta property="og:type" content="music.song">
  <meta property="og:url" content="%s">
  <meta property="og:title" content="%s - %s">
  <meta property="og:description" content="%s">
  <meta property="og:image" content="%s">
  <meta property="og:audio" content="%s">
  <meta property="music:musician" content="%s">
  <meta property="music:duration" content="%d">

  <!-- Twitter -->
  <meta name="twitter:card" content="summary_large_image">
  <meta name="twitter:url" content="%s">
  <meta name="twitter:title" content="%s - %s">
  <meta name="twitter:description" content="%s">
  <meta name="twitter:image" content="%s">

  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800&display=swap" rel="stylesheet">
  <style>
    :root {
      --bg-deep: #06080d;
      --surface: rgba(15, 18, 28, 0.75);
      --surface-hover: rgba(25, 30, 45, 0.85);
      --border: rgba(255,255,255,0.06);
      --border-hover: rgba(255,255,255,0.12);
      --accent: #818cf8;
      --accent-bright: #a5b4fc;
      --accent-glow: rgba(129,140,248,0.15);
      --text: #f1f5f9;
      --text-secondary: #94a3b8;
      --text-tertiary: #64748b;
    }

    * { box-sizing: border-box; margin: 0; padding: 0; }

    body {
      font-family: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif;
      background: var(--bg-deep);
      color: var(--text);
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 1.5rem;
      overflow: hidden;
    }

    .bg-mesh {
      position: fixed;
      inset: 0;
      z-index: 0;
      background:
        radial-gradient(ellipse 80%% 50%% at 50%% -20%%, rgba(99,102,241,0.12) 0%%, transparent 60%%),
        radial-gradient(ellipse 60%% 40%% at 80%% 60%%, rgba(139,92,246,0.08) 0%%, transparent 50%%),
        radial-gradient(ellipse 50%% 60%% at 20%% 80%%, rgba(59,130,246,0.06) 0%%, transparent 50%%);
    }

    .card {
      position: relative;
      z-index: 1;
      background: var(--surface);
      backdrop-filter: blur(40px) saturate(1.4);
      -webkit-backdrop-filter: blur(40px) saturate(1.4);
      border: 1px solid var(--border);
      border-radius: 24px;
      max-width: 420px;
      width: 100%%;
      overflow: hidden;
      box-shadow:
        0 0 0 1px rgba(255,255,255,0.03) inset,
        0 30px 60px -12px rgba(0,0,0,0.5),
        0 0 80px -20px var(--accent-glow);
      animation: cardIn 0.6s cubic-bezier(0.16, 1, 0.3, 1) both;
    }

    @keyframes cardIn {
      from { opacity: 0; transform: translateY(20px) scale(0.97); }
      to   { opacity: 1; transform: translateY(0) scale(1); }
    }

    .cover-section {
      position: relative;
      padding: 2rem 2rem 0;
      display: flex;
      justify-content: center;
    }

    .cover-glow {
      position: absolute;
      width: 200px;
      height: 200px;
      top: 50%%;
      left: 50%%;
      transform: translate(-50%%, -50%%);
      background: var(--accent);
      filter: blur(80px);
      opacity: 0.15;
      border-radius: 50%%;
      pointer-events: none;
    }

    .cover-frame {
      position: relative;
      width: 100%%;
      max-width: 280px;
      aspect-ratio: 1;
      border-radius: 16px;
      overflow: hidden;
      box-shadow:
        0 8px 32px rgba(0,0,0,0.4),
        0 0 0 1px rgba(255,255,255,0.06) inset;
    }

    .cover-img {
      width: 100%%;
      height: 100%%;
      object-fit: cover;
      transition: transform 0.4s cubic-bezier(0.16, 1, 0.3, 1);
    }

    .card:hover .cover-img {
      transform: scale(1.03);
    }

    .format-badge {
      position: absolute;
      top: 12px;
      right: 12px;
      background: rgba(0,0,0,0.55);
      backdrop-filter: blur(12px);
      padding: 4px 10px;
      border-radius: 8px;
      font-size: 0.65rem;
      font-weight: 700;
      letter-spacing: 0.08em;
      color: var(--accent-bright);
      border: 1px solid rgba(255,255,255,0.08);
    }

    .content {
      padding: 1.5rem 2rem 2rem;
      display: flex;
      flex-direction: column;
      gap: 1.25rem;
    }

    .track-info {
      display: flex;
      flex-direction: column;
      gap: 0.25rem;
    }

    .track-title {
      font-size: 1.35rem;
      font-weight: 700;
      letter-spacing: -0.02em;
      line-height: 1.3;
      color: #fff;
    }

    .track-artist {
      font-size: 0.95rem;
      color: var(--text-secondary);
      font-weight: 500;
    }

    .track-album {
      font-size: 0.8rem;
      color: var(--text-tertiary);
      margin-top: 2px;
    }

    .track-meta {
      display: flex;
      flex-wrap: wrap;
      gap: 0.5rem;
      margin-top: 0.25rem;
    }

    .meta-chip {
      display: inline-flex;
      align-items: center;
      gap: 4px;
      padding: 3px 10px;
      border-radius: 6px;
      font-size: 0.7rem;
      font-weight: 600;
      letter-spacing: 0.02em;
      background: rgba(255,255,255,0.04);
      color: var(--text-tertiary);
      border: 1px solid rgba(255,255,255,0.04);
    }

    .player-section {
      display: flex;
      flex-direction: column;
      gap: 0.5rem;
    }

    .waveform {
      display: flex;
      align-items: flex-end;
      justify-content: center;
      gap: 2px;
      height: 32px;
      opacity: 0.5;
    }

    .waveform .bar {
      width: 3px;
      border-radius: 2px;
      background: var(--accent);
      animation: wave 1.2s ease-in-out infinite;
    }

    @keyframes wave {
      0%%, 100%% { height: 20%%; }
      50%% { height: 80%%; }
    }

    audio {
      width: 100%%;
      height: 40px;
      border-radius: 12px;
      outline: none;
    }
    audio::-webkit-media-controls-panel {
      background: rgba(255,255,255,0.04);
      border-radius: 12px;
    }

    .actions {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 0.625rem;
    }

    .btn {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 0.4rem;
      padding: 0.7rem 0.75rem;
      font-family: 'Inter', sans-serif;
      font-size: 0.85rem;
      font-weight: 600;
      border-radius: 12px;
      text-decoration: none;
      border: none;
      cursor: pointer;
      transition: all 0.2s cubic-bezier(0.16, 1, 0.3, 1);
    }

    .btn-play {
      background: linear-gradient(135deg, #6366f1, #8b5cf6);
      color: #fff;
      box-shadow: 0 4px 16px rgba(99,102,241,0.3);
    }
    .btn-play:hover {
      box-shadow: 0 6px 24px rgba(99,102,241,0.45);
      transform: translateY(-1px);
    }

    .btn-dl {
      background: rgba(255,255,255,0.05);
      color: var(--text-secondary);
      border: 1px solid var(--border);
    }
    .btn-dl:hover {
      background: rgba(255,255,255,0.08);
      border-color: var(--border-hover);
      color: var(--text);
    }

    .footer {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding-top: 1rem;
      border-top: 1px solid var(--border);
    }

    .brand {
      display: flex;
      align-items: center;
      gap: 6px;
      font-size: 0.7rem;
      font-weight: 600;
      color: var(--text-tertiary);
      letter-spacing: 0.04em;
    }

    .brand-dot {
      width: 6px; height: 6px;
      border-radius: 50%%;
      background: var(--accent);
      box-shadow: 0 0 8px var(--accent);
    }

    .duration {
      font-size: 0.75rem;
      color: var(--text-tertiary);
      font-variant-numeric: tabular-nums;
    }
  </style>
</head>
<body>
  <div class="bg-mesh"></div>
  <div class="card">
    <div class="cover-section">
      <div class="cover-glow"></div>
      <div class="cover-frame">
        <img class="cover-img"
             src="%s"
             alt="%s"
             onerror="this.src='data:image/svg+xml,%%3Csvg xmlns=%%22http://www.w3.org/2000/svg%%22 viewBox=%%220 0 400 400%%22%%3E%%3Crect fill=%%22%%23111827%%22 width=%%22400%%22 height=%%22400%%22/%%3E%%3Ctext x=%%2250%%%%25%%22 y=%%2250%%%%25%%22 dominant-baseline=%%22middle%%22 text-anchor=%%22middle%%22 font-family=%%22Inter,sans-serif%%22 font-size=%%2240%%22 fill=%%22%%23374151%%22%%3E♫%%3C/text%%3E%%3C/svg%%3E'">
        <span class="format-badge">%s</span>
      </div>
    </div>
    <div class="content">
      <div class="track-info">
        <h1 class="track-title">%s</h1>
        <div class="track-artist">%s</div>
        %s
        %s
      </div>

      <div class="player-section">
        <div class="waveform" id="waveform">
          <div class="bar" style="animation-delay:0s;height:30%%"></div>
          <div class="bar" style="animation-delay:0.1s;height:60%%"></div>
          <div class="bar" style="animation-delay:0.15s;height:40%%"></div>
          <div class="bar" style="animation-delay:0.2s;height:80%%"></div>
          <div class="bar" style="animation-delay:0.3s;height:50%%"></div>
          <div class="bar" style="animation-delay:0.05s;height:70%%"></div>
          <div class="bar" style="animation-delay:0.25s;height:35%%"></div>
          <div class="bar" style="animation-delay:0.35s;height:65%%"></div>
          <div class="bar" style="animation-delay:0.12s;height:45%%"></div>
          <div class="bar" style="animation-delay:0.28s;height:55%%"></div>
          <div class="bar" style="animation-delay:0.18s;height:75%%"></div>
          <div class="bar" style="animation-delay:0.08s;height:25%%"></div>
        </div>
        <audio controls src="%s" preload="metadata"></audio>
      </div>

      <div class="actions">
        <a href="%s" class="btn btn-play">
          <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path d="M4 2.5a.5.5 0 0 1 .772-.42l8 5a.5.5 0 0 1 0 .84l-8 5A.5.5 0 0 1 4 12.5V2.5z"/></svg>
          Stream
        </a>
        <a href="%s" class="btn btn-dl">
          <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path d="M8 12l-4-4h2.5V2h3v6H12L8 12z"/><path d="M2 14h12v1H2v-1z"/></svg>
          Download
        </a>
      </div>

      <div class="footer">
        <div class="brand"><span class="brand-dot"></span> StreamX</div>
        <span class="duration">%s</span>
      </div>
    </div>
  </div>
  <script>
    const audio = document.querySelector('audio');
    const bars = document.querySelectorAll('.waveform .bar');
    audio.addEventListener('play', () => {
      bars.forEach(b => b.style.animationPlayState = 'running');
      document.querySelector('.waveform').style.opacity = '1';
    });
    audio.addEventListener('pause', () => {
      bars.forEach(b => b.style.animationPlayState = 'paused');
      document.querySelector('.waveform').style.opacity = '0.5';
    });
  </script>
</body>
</html>`,
		pageTitle,
		shareURL, escTitle, escArtist, escDesc, coverURL, streamURL, escArtist, track.Audio.DurationSec,
		shareURL, escTitle, escArtist, escDesc, coverURL,
		coverURL, escTitle, escType,
		escTitle, escArtist,
		renderAlbumLine(escAlbum, yearStr),
		renderMetaChips(qualityStr, genreStr),
		streamURL,
		streamURL,
		downloadURL,
		durationFormatted,
	)
}

func (h *ShareHandler) renderPlaylist(w http.ResponseWriter, r *http.Request, playlist *models.UserPlaylist) {
	name := playlist.Name
	if name == "" {
		name = "StreamX Playlist"
	}

	baseURL := h.baseURL(r)
	shareURL := fmt.Sprintf("%s/share/playlists/%s", baseURL, playlist.ID)
	coverURL := playlist.CoverURL
	if coverURL == "" {
		coverURL = fmt.Sprintf("%s/share/cover/default.jpg", baseURL)
	}

	_, trackCount, _ := h.playlistRepo.GetPlaylistTrackIDs(r.Context(), playlist.ID, 1, 1)
	desc := fmt.Sprintf("Stream %s featuring %d tracks on StreamX.", name, trackCount)

	if h.wantsJSON(r) {
		api.RespondJSON(w, http.StatusOK, map[string]any{
			"ok":          true,
			"type":        "playlist",
			"id":          playlist.ID,
			"name":        name,
			"track_count": trackCount,
			"cover_url":   coverURL,
			"share_url":   shareURL,
			"open_graph": map[string]string{
				"og:title":       name,
				"og:description": desc,
				"og:image":       coverURL,
				"og:type":        "music.playlist",
			},
		})
		return
	}

	pageTitle := html.EscapeString(fmt.Sprintf("%s | StreamX Playlist", name))
	escName := html.EscapeString(name)
	escDesc := html.EscapeString(desc)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s</title>

  <!-- OpenGraph / Facebook / Telegram -->
  <meta property="og:site_name" content="StreamX">
  <meta property="og:type" content="music.playlist">
  <meta property="og:url" content="%s">
  <meta property="og:title" content="%s">
  <meta property="og:description" content="%s">
  <meta property="og:image" content="%s">

  <!-- Twitter -->
  <meta name="twitter:card" content="summary_large_image">
  <meta name="twitter:url" content="%s">
  <meta name="twitter:title" content="%s">
  <meta name="twitter:description" content="%s">
  <meta name="twitter:image" content="%s">

  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800&display=swap" rel="stylesheet">
  <style>
    :root {
      --bg-deep: #06080d;
      --surface: rgba(15, 18, 28, 0.75);
      --border: rgba(255,255,255,0.06);
      --accent: #818cf8;
      --accent-bright: #a5b4fc;
      --accent-glow: rgba(129,140,248,0.15);
      --text: #f1f5f9;
      --text-secondary: #94a3b8;
      --text-tertiary: #64748b;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif;
      background: var(--bg-deep);
      color: var(--text);
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 1.5rem;
      overflow: hidden;
    }
    .bg-mesh {
      position: fixed; inset: 0; z-index: 0;
      background:
        radial-gradient(ellipse 80%% 50%% at 50%% -20%%, rgba(139,92,246,0.12) 0%%, transparent 60%%),
        radial-gradient(ellipse 60%% 40%% at 80%% 60%%, rgba(59,130,246,0.08) 0%%, transparent 50%%);
    }
    .card {
      position: relative; z-index: 1;
      background: var(--surface);
      backdrop-filter: blur(40px) saturate(1.4);
      -webkit-backdrop-filter: blur(40px) saturate(1.4);
      border: 1px solid var(--border);
      border-radius: 24px;
      max-width: 420px; width: 100%%;
      overflow: hidden;
      box-shadow:
        0 0 0 1px rgba(255,255,255,0.03) inset,
        0 30px 60px -12px rgba(0,0,0,0.5),
        0 0 80px -20px var(--accent-glow);
      animation: cardIn 0.6s cubic-bezier(0.16, 1, 0.3, 1) both;
    }
    @keyframes cardIn {
      from { opacity: 0; transform: translateY(20px) scale(0.97); }
      to   { opacity: 1; transform: translateY(0) scale(1); }
    }
    .cover-section {
      position: relative;
      width: 100%%;
      aspect-ratio: 16/9;
      overflow: hidden;
      background: #0f1219;
    }
    .cover-img {
      width: 100%%; height: 100%%;
      object-fit: cover;
      transition: transform 0.4s cubic-bezier(0.16, 1, 0.3, 1);
    }
    .card:hover .cover-img { transform: scale(1.03); }
    .pl-badge {
      position: absolute; top: 14px; right: 14px;
      background: rgba(0,0,0,0.55);
      backdrop-filter: blur(12px);
      padding: 4px 10px;
      border-radius: 8px;
      font-size: 0.65rem; font-weight: 700; letter-spacing: 0.08em;
      color: var(--accent-bright);
      border: 1px solid rgba(255,255,255,0.08);
    }
    .track-count-overlay {
      position: absolute; bottom: 14px; left: 14px;
      background: rgba(0,0,0,0.6);
      backdrop-filter: blur(12px);
      padding: 5px 12px;
      border-radius: 8px;
      font-size: 0.75rem; font-weight: 600;
      color: #e2e8f0;
      border: 1px solid rgba(255,255,255,0.06);
    }
    .content {
      padding: 1.5rem 2rem 2rem;
      display: flex; flex-direction: column; gap: 1.25rem;
    }
    .pl-info { display: flex; flex-direction: column; gap: 0.25rem; }
    .pl-name {
      font-size: 1.35rem; font-weight: 700;
      letter-spacing: -0.02em; line-height: 1.3; color: #fff;
    }
    .pl-meta { font-size: 0.85rem; color: var(--text-secondary); font-weight: 500; }
    .btn-open {
      display: flex; align-items: center; justify-content: center; gap: 0.4rem;
      padding: 0.8rem; font-family: 'Inter', sans-serif;
      font-size: 0.9rem; font-weight: 600;
      border-radius: 12px; text-decoration: none; border: none; cursor: pointer;
      background: linear-gradient(135deg, #6366f1, #8b5cf6);
      color: #fff;
      box-shadow: 0 4px 16px rgba(99,102,241,0.3);
      transition: all 0.2s cubic-bezier(0.16, 1, 0.3, 1);
    }
    .btn-open:hover {
      box-shadow: 0 6px 24px rgba(99,102,241,0.45);
      transform: translateY(-1px);
    }
    .footer {
      display: flex; justify-content: space-between; align-items: center;
      padding-top: 1rem; border-top: 1px solid var(--border);
    }
    .brand {
      display: flex; align-items: center; gap: 6px;
      font-size: 0.7rem; font-weight: 600; color: var(--text-tertiary); letter-spacing: 0.04em;
    }
    .brand-dot {
      width: 6px; height: 6px; border-radius: 50%%;
      background: var(--accent); box-shadow: 0 0 8px var(--accent);
    }
    .pl-type { font-size: 0.7rem; color: var(--text-tertiary); letter-spacing: 0.04em; }
  </style>
</head>
<body>
  <div class="bg-mesh"></div>
  <div class="card">
    <div class="cover-section">
      <img class="cover-img" src="%s" alt="%s"
           onerror="this.src='data:image/svg+xml,%%3Csvg xmlns=%%22http://www.w3.org/2000/svg%%22 viewBox=%%220 0 640 360%%22%%3E%%3Crect fill=%%22%%23111827%%22 width=%%22640%%22 height=%%22360%%22/%%3E%%3Ctext x=%%2250%%%%25%%22 y=%%2250%%%%25%%22 dominant-baseline=%%22middle%%22 text-anchor=%%22middle%%22 font-family=%%22Inter,sans-serif%%22 font-size=%%2240%%22 fill=%%22%%23374151%%22%%3E♫♫♫%%3C/text%%3E%%3C/svg%%3E'">
      <span class="pl-badge">PLAYLIST</span>
      <span class="track-count-overlay">%d tracks</span>
    </div>
    <div class="content">
      <div class="pl-info">
        <h1 class="pl-name">%s</h1>
        <div class="pl-meta">%d tracks in this playlist</div>
      </div>
      <a href="%s" class="btn-open">
        <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path d="M4 2.5a.5.5 0 0 1 .772-.42l8 5a.5.5 0 0 1 0 .84l-8 5A.5.5 0 0 1 4 12.5V2.5z"/></svg>
        Open in StreamX
      </a>
      <div class="footer">
        <div class="brand"><span class="brand-dot"></span> StreamX</div>
        <span class="pl-type">Public Playlist</span>
      </div>
    </div>
  </div>
</body>
</html>`,
		pageTitle,
		shareURL, escName, escDesc, coverURL,
		shareURL, escName, escDesc, coverURL,
		coverURL, escName,
		trackCount,
		escName, trackCount,
		shareURL,
	)
}

func (h *ShareHandler) baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "localhost:8000"
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func renderAlbumLine(album, year string) string {
	if album == "" && year == "" {
		return ""
	}
	if album != "" && year != "" {
		return fmt.Sprintf(`<div class="track-album">%s · %s</div>`, album, year)
	}
	if album != "" {
		return fmt.Sprintf(`<div class="track-album">%s</div>`, album)
	}
	return fmt.Sprintf(`<div class="track-album">%s</div>`, year)
}

func renderMetaChips(quality, genre string) string {
	if quality == "" && genre == "" {
		return ""
	}
	out := `<div class="track-meta">`
	if quality != "" {
		out += fmt.Sprintf(`<span class="meta-chip">%s</span>`, quality)
	}
	if genre != "" {
		out += fmt.Sprintf(`<span class="meta-chip">%s</span>`, genre)
	}
	out += `</div>`
	return out
}

func formatDuration(sec int) string {
	if sec <= 0 {
		return "0:00"
	}
	m := sec / 60
	s := sec % 60
	return fmt.Sprintf("%d:%02d", m, s)
}
