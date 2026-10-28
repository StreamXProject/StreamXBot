package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"streamgo/internal/config"
	"streamgo/internal/logger"
)

var logLyrics = logger.New("lyrics")

const (
	musixmatchBaseURL = "https://apic.musixmatch.com/ws/1.1"
	musixmatchAppID   = "mac-ios-v2.0"
)

// LyricsResult bundles resolved lyrics and alternative multilingual titles.
type LyricsResult struct {
	Lyrics       string
	SyncedLyrics string
	Kind         string // "richsync" or "lrc"
	Source       string // "musixmatch" or "lrclib"
	Titles       map[string]any
}

// LyricsEnrichmentService fetches lyrics and alternative titles from Musixmatch and LRCLIB.
type LyricsEnrichmentService struct {
	cfg        *config.Config
	httpClient *http.Client

	tokenMu      sync.RWMutex
	cachedToken  string
	tokenSavedAt float64
}

// NewLyricsEnrichmentService creates a new LyricsEnrichmentService.
func NewLyricsEnrichmentService(cfg *config.Config) *LyricsEnrichmentService {
	return &LyricsEnrichmentService{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// FetchLyrics searches for lyrics using default provider ordering.
func (s *LyricsEnrichmentService) FetchLyrics(ctx context.Context, title, artist, album string) (*LyricsResult, error) {
	return s.FetchLyricsWithProvider(ctx, title, artist, album, "auto")
}

// FetchLyricsWithProvider searches for lyrics respecting user/frontend provider preference.
func (s *LyricsEnrichmentService) FetchLyricsWithProvider(ctx context.Context, title, artist, album, provider string) (*LyricsResult, error) {
	if s.cfg != nil && !s.cfg.Lyrics {
		return nil, nil // Lyrics fetching disabled
	}

	cleanTitle := StripNoise(title)
	cleanArtist := StripNoise(artist)
	cleanAlbum := StripNoise(album)

	if cleanTitle == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}

	p := strings.ToLower(strings.TrimSpace(provider))

	// Helper runners
	runMusixmatch := func() (*LyricsResult, error) {
		res, err := s.queryMusixmatch(ctx, cleanTitle, cleanArtist, cleanAlbum)
		if err == nil && res != nil && (res.Lyrics != "" || len(res.Titles) > 0) {
			return res, nil
		}
		return nil, err
	}

	runLRCLIB := func() (*LyricsResult, error) {
		res, err := s.queryLRCLIB(ctx, cleanTitle, cleanArtist, cleanAlbum)
		if err == nil && res != nil && res.Lyrics != "" {
			return res, nil
		}
		return nil, err
	}

	runKuGou := func() (*LyricsResult, error) {
		res, err := s.queryKuGou(ctx, cleanTitle, cleanArtist, 0)
		if err == nil && res != nil && res.Lyrics != "" {
			return res, nil
		}
		return nil, err
	}

	switch p {
	case "musixmatch":
		if res, err := runMusixmatch(); err == nil {
			return res, nil
		}
		if res, err := runLRCLIB(); err == nil {
			return res, nil
		}
		if res, err := runKuGou(); err == nil {
			return res, nil
		}
	case "lrclib":
		if res, err := runLRCLIB(); err == nil {
			return res, nil
		}
		if res, err := runMusixmatch(); err == nil {
			return res, nil
		}
		if res, err := runKuGou(); err == nil {
			return res, nil
		}
	case "kugou":
		if res, err := runKuGou(); err == nil {
			return res, nil
		}
		if res, err := runMusixmatch(); err == nil {
			return res, nil
		}
		if res, err := runLRCLIB(); err == nil {
			return res, nil
		}
	default:
		// "auto" or unspecified: Musixmatch -> LRCLIB -> KuGou
		if s.cfg == nil || s.cfg.Musixmatch {
			if res, err := runMusixmatch(); err == nil {
				return res, nil
			}
		}
		if s.cfg == nil || s.cfg.LRCLIB {
			if res, err := runLRCLIB(); err == nil {
				return res, nil
			}
		}
		if res, err := runKuGou(); err == nil {
			return res, nil
		}
	}

	return nil, fmt.Errorf("no lyrics found for %s - %s", artist, title)
}

func (s *LyricsEnrichmentService) getUserToken(ctx context.Context, forceRefresh bool) string {
	s.tokenMu.RLock()
	now := float64(time.Now().Unix())
	if !forceRefresh && s.cachedToken != "" && (now-s.tokenSavedAt) < 24*3600 {
		tok := s.cachedToken
		s.tokenMu.RUnlock()
		return tok
	}
	s.tokenMu.RUnlock()

	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()

	if !forceRefresh && s.cachedToken != "" && (now-s.tokenSavedAt) < 24*3600 {
		return s.cachedToken
	}

	// Try reading from token files
	tokenCandidates := []string{
		"/home/misfit/Work/StreamXBot/cookies/.musixmatch_token",
		"./cookies/.musixmatch_token",
		filepath.Join(os.TempDir(), ".musixmatch_token"),
	}

	if !forceRefresh {
		for _, path := range tokenCandidates {
			if data, err := os.ReadFile(path); err == nil {
				var tokenDoc struct {
					Token string `json:"token"`
				}
				if err := json.Unmarshal(data, &tokenDoc); err == nil && tokenDoc.Token != "" {
					s.cachedToken = strings.TrimSpace(tokenDoc.Token)
					s.tokenSavedAt = now
					return s.cachedToken
				}
			}
		}
	}

	// Request new user token from Musixmatch
	tokenURL := fmt.Sprintf("%s/token.get?app_id=%s", musixmatchBaseURL, musixmatchAppID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err == nil {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
		resp, err := s.httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			var doc struct {
				Message struct {
					Header struct {
						StatusCode int `json:"status_code"`
					} `json:"header"`
					Body struct {
						UserToken string `json:"user_token"`
					} `json:"body"`
				} `json:"message"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&doc); err == nil {
				if doc.Message.Body.UserToken != "" {
					s.cachedToken = strings.TrimSpace(doc.Message.Body.UserToken)
					s.tokenSavedAt = now
					return s.cachedToken
				}
			}
		}
	}

	// Fallback hardcoded / known seed token if dynamic token acquisition fails
	fallback := "26091748e0125fcea9641300d8bbf8995de1514987802fc2c70d6f"
	s.cachedToken = fallback
	s.tokenSavedAt = now
	return fallback
}

func (s *LyricsEnrichmentService) queryMusixmatch(ctx context.Context, title, artist, album string) (*LyricsResult, error) {
	token := s.getUserToken(ctx, false)
	if token == "" {
		return nil, fmt.Errorf("failed to acquire musixmatch token")
	}

	endpoint := fmt.Sprintf("%s/macro.subtitles.get?app_id=%s&usertoken=%s&subtitle_format=mxm&q_track=%s&q_artist=%s",
		musixmatchBaseURL,
		musixmatchAppID,
		url.QueryEscape(token),
		url.QueryEscape(title),
		url.QueryEscape(artist),
	)
	if album != "" {
		endpoint += fmt.Sprintf("&q_album=%s", url.QueryEscape(album))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload struct {
		Message struct {
			Header struct {
				StatusCode int `json:"status_code"`
			} `json:"header"`
			Body struct {
				MacroCalls map[string]struct {
					Message struct {
						Header struct {
							StatusCode int `json:"status_code"`
						} `json:"header"`
						Body any `json:"body"`
					} `json:"message"`
				} `json:"macro_calls"`
			} `json:"body"`
		} `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	macro := payload.Message.Body.MacroCalls
	trackCall, hasTrack := macro["matcher.track.get"]
	if !hasTrack {
		return nil, fmt.Errorf("no matcher.track.get in musixmatch response")
	}

	trackBodyBytes, _ := json.Marshal(trackCall.Message.Body)
	var trackData struct {
		Track struct {
			TrackID                  int64  `json:"track_id"`
			TrackName                string `json:"track_name"`
			HasRichsync              int    `json:"has_richsync"`
			HasSubtitles             int    `json:"has_subtitles"`
			TrackNameTranslationList []struct {
				TrackNameTranslation struct {
					Language    string `json:"language"`
					Translation string `json:"translation"`
				} `json:"track_name_translation"`
			} `json:"track_name_translation_list"`
		} `json:"track"`
	}
	_ = json.Unmarshal(trackBodyBytes, &trackData)

	originalTitle := title
	if trackData.Track.TrackName != "" {
		originalTitle = trackData.Track.TrackName
	}

	// Extract romanized title
	romanized := ""
	for _, item := range trackData.Track.TrackNameTranslationList {
		lang := strings.ToLower(strings.TrimSpace(item.TrackNameTranslation.Language))
		val := strings.TrimSpace(item.TrackNameTranslation.Translation)
		if lang == "rj" || lang == "rk" || lang == "romanized" || lang == "romaja" {
			romanized = val
			break
		}
		if (lang == "u0" || lang == "zr") && !isCJK(val) && romanized == "" {
			romanized = val
		}
	}

	// Fallback to Japanese romanizer if title is CJK
	if romanized == "" && isCJK(originalTitle) {
		romanized = romanizeJapanese(originalTitle)
	}

	titles := map[string]any{
		"original": originalTitle,
	}
	if romanized != "" && !strings.EqualFold(romanized, originalTitle) {
		titles["romanized"] = romanized
	}

	res := &LyricsResult{
		Titles: titles,
	}

	// 1. If has_richsync is true and track_id > 0, fetch word-level richsync
	if trackData.Track.TrackID > 0 && trackData.Track.HasRichsync == 1 {
		richsyncURL := fmt.Sprintf("%s/track.richsync.get?app_id=%s&usertoken=%s&track_id=%d",
			musixmatchBaseURL,
			musixmatchAppID,
			url.QueryEscape(token),
			trackData.Track.TrackID,
		)
		rReq, err := http.NewRequestWithContext(ctx, http.MethodGet, richsyncURL, nil)
		if err == nil {
			rReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
			if rResp, err := s.httpClient.Do(rReq); err == nil {
				defer rResp.Body.Close()
				var rDoc struct {
					Message struct {
						Body struct {
							Richsync struct {
								RichsyncBody string `json:"richsync_body"`
							} `json:"richsync"`
						} `json:"body"`
					} `json:"message"`
				}
				if err := json.NewDecoder(rResp.Body).Decode(&rDoc); err == nil {
					richText := strings.TrimSpace(rDoc.Message.Body.Richsync.RichsyncBody)
					if richText != "" {
						if lrc := parseRichsyncJSONToLRC(richText); lrc != "" {
							res.Lyrics = lrc
							res.SyncedLyrics = lrc
							res.Kind = "richsync"
							res.Source = "musixmatch"
							return res, nil
						}
					}
				}
			}
		}
	}

	// 2. Check track.subtitles.get from macro calls
	if subCall, ok := macro["track.subtitles.get"]; ok {
		subBodyBytes, _ := json.Marshal(subCall.Message.Body)
		var subData struct {
			SubtitleList []struct {
				Subtitle struct {
					SubtitleBody string `json:"subtitle_body"`
				} `json:"subtitle"`
			} `json:"subtitle_list"`
		}
		if err := json.Unmarshal(subBodyBytes, &subData); err == nil && len(subData.SubtitleList) > 0 {
			subBody := strings.TrimSpace(subData.SubtitleList[0].Subtitle.SubtitleBody)
			if subBody != "" {
				lrc := parseSubtitlesJSONToLRC(subBody)
				if lrc != "" {
					res.Lyrics = lrc
					res.SyncedLyrics = lrc
					res.Kind = "lrc"
					res.Source = "musixmatch"
					return res, nil
				}
			}
		}
	}

	// 3. Plain lyrics from macro calls
	if lyrCall, ok := macro["track.lyrics.get"]; ok {
		lyrBodyBytes, _ := json.Marshal(lyrCall.Message.Body)
		var lyrData struct {
			Lyrics struct {
				LyricsBody string `json:"lyrics_body"`
			} `json:"lyrics"`
		}
		if err := json.Unmarshal(lyrBodyBytes, &lyrData); err == nil {
			plain := strings.TrimSpace(lyrData.Lyrics.LyricsBody)
			if plain != "" {
				res.Lyrics = plain
				res.Kind = "lrc"
				res.Source = "musixmatch"
				return res, nil
			}
		}
	}

	return res, nil
}

func (s *LyricsEnrichmentService) queryLRCLIB(ctx context.Context, title, artist, album string) (*LyricsResult, error) {
	apiURL := fmt.Sprintf("https://lrclib.net/api/get?track_name=%s&artist_name=%s",
		url.QueryEscape(title),
		url.QueryEscape(artist),
	)
	if album != "" {
		apiURL += fmt.Sprintf("&album_name=%s", url.QueryEscape(album))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "StreamGO/1.0")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lrclib status: %d", resp.StatusCode)
	}

	var payload struct {
		TrackName    string `json:"trackName"`
		ArtistName   string `json:"artistName"`
		PlainLyrics  string `json:"plainLyrics"`
		SyncedLyrics string `json:"syncedLyrics"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	bestLyrics := payload.SyncedLyrics
	if bestLyrics == "" {
		bestLyrics = payload.PlainLyrics
	}

	if bestLyrics == "" {
		return nil, fmt.Errorf("empty lyrics from lrclib")
	}

	titles := map[string]any{
		"original": title,
	}

	return &LyricsResult{
		Lyrics:       bestLyrics,
		SyncedLyrics: payload.SyncedLyrics,
		Kind:         "lrc",
		Source:       "lrclib",
		Titles:       titles,
	}, nil
}

func (s *LyricsEnrichmentService) queryKuGou(ctx context.Context, title, artist string, durationSec int) (*LyricsResult, error) {
	query := fmt.Sprintf("%s - %s", strings.TrimSpace(title), strings.TrimSpace(artist))

	// 1. Search song
	searchURL := fmt.Sprintf("https://mobileservice.kugou.com/api/v3/search/song?version=9108&plat=0&pagesize=6&showtype=0&keyword=%s", url.QueryEscape(query))
	req1, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}
	req1.Header.Set("User-Agent", "piTube/1.0")

	resp1, err := s.httpClient.Do(req1)
	if err != nil {
		return nil, err
	}
	defer resp1.Body.Close()

	var searchRes struct {
		Data struct {
			Info []struct {
				Hash     string  `json:"hash"`
				Duration float64 `json:"duration"`
			} `json:"info"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp1.Body).Decode(&searchRes); err != nil {
		return nil, err
	}

	if len(searchRes.Data.Info) == 0 {
		return nil, fmt.Errorf("no songs found on kugou")
	}

	targetHash := ""
	for _, song := range searchRes.Data.Info {
		if durationSec > 0 && math.Abs(song.Duration-float64(durationSec)) > 8 {
			continue
		}
		if song.Hash != "" {
			targetHash = song.Hash
			break
		}
	}
	if targetHash == "" {
		targetHash = searchRes.Data.Info[0].Hash
	}
	if targetHash == "" {
		return nil, fmt.Errorf("no valid hash found on kugou")
	}

	// 2. Search lyrics candidate
	candURL := fmt.Sprintf("https://lyrics.kugou.com/search?ver=1&man=yes&client=pc&hash=%s", targetHash)
	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, candURL, nil)
	if err != nil {
		return nil, err
	}
	req2.Header.Set("User-Agent", "piTube/1.0")

	resp2, err := s.httpClient.Do(req2)
	if err != nil {
		return nil, err
	}
	defer resp2.Body.Close()

	var candRes struct {
		Candidates []struct {
			ID        string `json:"id"`
			AccessKey string `json:"accesskey"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&candRes); err != nil {
		return nil, err
	}
	if len(candRes.Candidates) == 0 {
		return nil, fmt.Errorf("no lyrics candidates on kugou")
	}

	candID := candRes.Candidates[0].ID
	accessKey := candRes.Candidates[0].AccessKey

	// 3. Download lyrics
	downURL := fmt.Sprintf("https://lyrics.kugou.com/download?fmt=lrc&charset=utf8&client=pc&ver=1&id=%s&accesskey=%s", candID, accessKey)
	req3, err := http.NewRequestWithContext(ctx, http.MethodGet, downURL, nil)
	if err != nil {
		return nil, err
	}
	req3.Header.Set("User-Agent", "piTube/1.0")

	resp3, err := s.httpClient.Do(req3)
	if err != nil {
		return nil, err
	}
	defer resp3.Body.Close()

	var downRes struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(resp3.Body).Decode(&downRes); err != nil {
		return nil, err
	}
	if downRes.Content == "" {
		return nil, fmt.Errorf("empty kugou content")
	}

	decoded, err := base64.StdEncoding.DecodeString(downRes.Content)
	if err != nil {
		return nil, err
	}

	lrcText := string(decoded)
	return &LyricsResult{
		Lyrics:       lrcText,
		SyncedLyrics: lrcText,
		Kind:         "synced",
		Source:       "kugou",
		Titles:       map[string]any{"original": title},
	}, nil
}

func formatLRCTimestamp(totalSeconds float64) string {
	if totalSeconds < 0 {
		totalSeconds = 0
	}
	m := int(totalSeconds / 60)
	secF := totalSeconds - float64(m*60)
	s := int(secF)
	hh := int(math.Round((secF - float64(s)) * 100))
	if hh >= 100 {
		hh = 0
		s++
	}
	if s >= 60 {
		s = 0
		m++
	}
	return fmt.Sprintf("[%02d:%02d.%02d]", m, s, hh)
}

func formatWordTimestamp(totalSeconds float64) string {
	if totalSeconds < 0 {
		totalSeconds = 0
	}
	m := int(totalSeconds / 60)
	secF := totalSeconds - float64(m*60)
	s := int(secF)
	hh := int(math.Round((secF - float64(s)) * 100))
	if hh >= 100 {
		hh = 0
		s++
	}
	if s >= 60 {
		s = 0
		m++
	}
	return fmt.Sprintf("<%02d:%02d.%02d>", m, s, hh)
}

func parseRichsyncJSONToLRC(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}

	var items []struct {
		TS float64 `json:"ts"`
		TE float64 `json:"te"`
		X  string  `json:"x"`
		L  []struct {
			C string  `json:"c"`
			O float64 `json:"o"`
		} `json:"l"`
	}

	if err := json.Unmarshal([]byte(body), &items); err != nil || len(items) == 0 {
		return ""
	}

	var out []string
	for _, it := range items {
		lineText := strings.TrimSpace(it.X)
		if lineText == "" && len(it.L) == 0 {
			continue
		}

		lineTag := formatLRCTimestamp(it.TS)
		if len(it.L) > 0 {
			var parts []string
			parts = append(parts, lineTag)
			for _, w := range it.L {
				c := w.C
				if strings.TrimSpace(c) == "" {
					parts = append(parts, c)
				} else {
					parts = append(parts, fmt.Sprintf("%s%s", formatWordTimestamp(it.TS+w.O), c))
				}
			}
			out = append(out, strings.TrimRight(strings.Join(parts, ""), " "))
		} else {
			out = append(out, strings.TrimRight(fmt.Sprintf("%s %s", lineTag, lineText), " "))
		}
	}

	return strings.Join(out, "\n")
}

func parseSubtitlesJSONToLRC(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}

	// Already LRC format text with [mm:ss timestamps
	if len(body) > 6 && body[0] == '[' && (body[1] >= '0' && body[1] <= '9') && body[3] == ':' {
		return body
	}

	var items []struct {
		Text string `json:"text"`
		Time struct {
			Total float64 `json:"total"`
		} `json:"time"`
	}

	if err := json.Unmarshal([]byte(body), &items); err != nil || len(items) == 0 {
		return body
	}

	var out []string
	for _, it := range items {
		ts := formatLRCTimestamp(it.Time.Total)
		t := strings.TrimRight(it.Text, " ")
		if t != "" {
			out = append(out, fmt.Sprintf("%s %s", ts, t))
		} else {
			out = append(out, ts)
		}
	}

	return strings.Join(out, "\n")
}

func isCJK(text string) bool {
	for _, r := range text {
		if (r >= 0x3040 && r <= 0x309f) || // Hiragana
			(r >= 0x30a0 && r <= 0x30ff) || // Katakana
			(r >= 0x4e00 && r <= 0x9fff) || // CJK Ideographs
			(r >= 0xac00 && r <= 0xd7af) { // Hangul
			return true
		}
	}
	return false
}

func romanizeJapanese(text string) string {
	if text == "" || !isCJK(text) {
		return ""
	}

	pyBins := []string{
		"/home/misfit/Work/StreamXBot/.venv/bin/python3",
		"python3",
	}

	script := `import pykakasi, sys; k = pykakasi.kakasi(); print(' '.join(w.get('hepburn','').capitalize() for w in k.convert(sys.argv[1])))`

	for _, py := range pyBins {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		cmd := exec.CommandContext(ctx, py, "-c", script, text)
		out, err := cmd.Output()
		cancel()
		if err == nil {
			res := strings.TrimSpace(string(out))
			if res != "" {
				// Clean formatting
				re := regexp.MustCompile(`\s+`)
				res = re.ReplaceAllString(res, " ")
				return res
			}
		}
	}

	return ""
}
