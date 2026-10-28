package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"streamgo/internal/logger"
)

var logCover = logger.New("cover_search")

// CoverSearchService searches for high-resolution album artwork and artist avatars.
type CoverSearchService struct {
	httpClient *http.Client
}

// NewCoverSearchService creates a new CoverSearchService.
func NewCoverSearchService() *CoverSearchService {
	return &CoverSearchService{
		httpClient: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
}

// StripNoise removes common junk, watermarks, and descriptors from titles/artists for clean search.
func StripNoise(text string) string {
	s := strings.TrimSpace(text)
	if s == "" {
		return ""
	}

	// Remove disc/volume markers
	reDisc := regexp.MustCompile(`(?i)[\(\[]\s*(?:disc|cd|tape|vol|volume)\s*\d+[\)\]]`)
	s = reDisc.ReplaceAllString(s, " ")

	// Remove official video, lyric video, remaster, audio descriptors (with optional year)
	reDescriptors := regexp.MustCompile(`(?i)[\(\[]\s*(?:official\s+(?:audio|video|music\s+video)|lyric(?:s)?\s+video|official|audio|video|mv|visualizer|remaster(?:ed)?(?:\s+\d+)?|hd|4k|explicit|clean|flac|320kbps|24bit|live(?:\s+at[^\]\)]+)?)\s*[\)\]]`)
	s = reDescriptors.ReplaceAllString(s, " ")

	// Remove standalone noise words
	reStandalone := regexp.MustCompile(`(?i)\b(?:official\s+(?:audio|video|music\s+video)|lyric(?:s)?\s+video|visualizer|remaster(?:ed)?(?:\s+\d+)?|explicit|clean)\b`)
	s = reStandalone.ReplaceAllString(s, " ")

	// Remove unclosed brackets
	if (strings.Contains(s, "(") && !strings.Contains(s, ")")) || (strings.Contains(s, "[") && !strings.Contains(s, "]")) {
		s = strings.ReplaceAll(s, "(", " ")
		s = strings.ReplaceAll(s, "[", " ")
	}

	// Remove telegram channel prefixes or watermarks (@channel_name)
	reChannel := regexp.MustCompile(`(?i)@\w+`)
	s = reChannel.ReplaceAllString(s, " ")

	// Normalize spaces
	reSpaces := regexp.MustCompile(`\s+`)
	s = reSpaces.ReplaceAllString(s, " ")

	return strings.TrimSpace(s)
}

// FindBestCover attempts to find the best album artwork using iTunes and Deezer APIs.
func (s *CoverSearchService) FindBestCover(ctx context.Context, title, artist, album string) (coverURL, thumbURL string, err error) {
	cleanTitle := StripNoise(title)
	cleanArtist := StripNoise(artist)
	cleanAlbum := StripNoise(album)

	// 1. Try iTunes Search API
	if cover, thumb, err := s.searchITunes(ctx, cleanTitle, cleanArtist); err == nil && cover != "" {
		return cover, thumb, nil
	}

	// 2. Try Deezer Search API
	if cover, thumb, err := s.searchDeezer(ctx, cleanTitle, cleanArtist); err == nil && cover != "" {
		return cover, thumb, nil
	}

	// 3. Try with Album name if available
	if cleanAlbum != "" {
		if cover, thumb, err := s.searchITunes(ctx, cleanAlbum, cleanArtist); err == nil && cover != "" {
			return cover, thumb, nil
		}
	}

	return "", "", fmt.Errorf("no cover found for %s - %s", artist, title)
}

// searchITunes queries the Apple iTunes Search API for artwork.
func (s *CoverSearchService) searchITunes(ctx context.Context, title, artist string) (coverURL, thumbURL string, err error) {
	query := fmt.Sprintf("%s %s", artist, title)
	apiURL := fmt.Sprintf("https://itunes.apple.com/search?term=%s&entity=song&limit=3", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("itunes api returned status %d", resp.StatusCode)
	}

	var payload struct {
		ResultCount int `json:"resultCount"`
		Results     []struct {
			ArtworkUrl100 string `json:"artworkUrl100"`
			ArtworkUrl60  string `json:"artworkUrl60"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", err
	}

	if payload.ResultCount > 0 && len(payload.Results) > 0 {
		raw := payload.Results[0].ArtworkUrl100
		if raw != "" {
			// Upgrade 100x100 to ultra-high-res 1000x1000bb.jpg
			highRes := strings.Replace(raw, "100x100bb.jpg", "1000x1000bb.jpg", 1)
			return highRes, payload.Results[0].ArtworkUrl60, nil
		}
	}

	return "", "", nil
}

// searchDeezer queries Deezer API for album cover art.
func (s *CoverSearchService) searchDeezer(ctx context.Context, title, artist string) (coverURL, thumbURL string, err error) {
	query := fmt.Sprintf(`artist:"%s" track:"%s"`, artist, title)
	apiURL := fmt.Sprintf("https://api.deezer.com/search?q=%s&limit=3", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", "", err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("deezer api returned status %d", resp.StatusCode)
	}

	var payload struct {
		Total int `json:"total"`
		Data  []struct {
			Album struct {
				CoverBig string `json:"cover_big"`
				CoverXL  string `json:"cover_xl"`
				CoverMed string `json:"cover_medium"`
			} `json:"album"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", err
	}

	if payload.Total > 0 && len(payload.Data) > 0 {
		alb := payload.Data[0].Album
		cover := alb.CoverXL
		if cover == "" {
			cover = alb.CoverBig
		}
		if cover != "" {
			return cover, alb.CoverMed, nil
		}
	}

	return "", "", nil
}

// FindArtistAvatar fetches high-resolution artist profile picture from Deezer API.
func (s *CoverSearchService) FindArtistAvatar(ctx context.Context, artist string) (string, error) {
	cleanArtist := StripNoise(artist)
	apiURL := fmt.Sprintf("https://api.deezer.com/search/artist?q=%s&limit=1", url.QueryEscape(cleanArtist))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var payload struct {
		Data []struct {
			PictureXL  string `json:"picture_xl"`
			PictureBig string `json:"picture_big"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}

	if len(payload.Data) > 0 {
		pic := payload.Data[0].PictureXL
		if pic == "" {
			pic = payload.Data[0].PictureBig
		}
		return pic, nil
	}

	return "", nil
}
