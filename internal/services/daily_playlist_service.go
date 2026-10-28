package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"streamgo/internal/models"
	"streamgo/internal/repository"
)

// DailyPlaylistService coordinates daily dynamic playlist generation.
type DailyPlaylistService struct {
	dailyRepo repository.DailyPlaylistRepository
	trackRepo repository.TrackRepository
}

// NewDailyPlaylistService creates a new DailyPlaylistService.
func NewDailyPlaylistService(dailyRepo repository.DailyPlaylistRepository, trackRepo repository.TrackRepository) *DailyPlaylistService {
	return &DailyPlaylistService{
		dailyRepo: dailyRepo,
		trackRepo: trackRepo,
	}
}

// CanonDailyKey normalizes daily playlist key names.
func CanonDailyKey(raw string) string {
	k := strings.ToLower(strings.TrimSpace(raw))
	switch k {
	case "random", "mix", "daily-playlist":
		return "random"
	case "trending", "trending-today", "top", "top-played", "top-playlist":
		return "trending"
	case "rediscover":
		return "rediscover"
	case "late-night", "late-night-mix":
		return "late-night"
	case "rising", "rising-tracks":
		return "rising"
	case "surprise", "surprise-me":
		return "surprise"
	default:
		return k
	}
}

// GetAvailablePlaylists returns the list of daily playlists for the home page.
func (s *DailyPlaylistService) GetAvailablePlaylists(ctx context.Context) (*models.AvailablePlaylistsResponse, error) {
	dailyDefs := []struct {
		Key  string
		Name string
	}{
		{"random", "Daily Mix"},
		{"trending", "Trending Today"},
		{"rediscover", "Rediscover"},
		{"late-night", "Late Night Mix"},
		{"rising", "Rising Tracks"},
		{"surprise", "Surprise Me"},
	}

	today := time.Now().UTC().Format("2006-01-02")
	var items []*models.AvailablePlaylistItem

	for _, d := range dailyDefs {
		docID := fmt.Sprintf("%s:%s:0", d.Key, today)
		var thumbs []string
		var coverURL string

		// If cached, grab cover from first tracks
		if cached, _ := s.dailyRepo.GetCachedDailyPlaylist(ctx, docID); cached != nil && len(cached.TrackIDs) > 0 {
			sampleIDs := cached.TrackIDs
			if len(sampleIDs) > 4 {
				sampleIDs = sampleIDs[:4]
			}
			if sampleTracks, err := s.trackRepo.GetByIDs(ctx, sampleIDs); err == nil {
				for _, t := range sampleTracks {
					u := t.EffectiveCoverURL()
					if u != "" {
						thumbs = append(thumbs, u)
					}
				}
				if len(thumbs) > 0 {
					coverURL = thumbs[0]
				}
			}
		}

		items = append(items, &models.AvailablePlaylistItem{
			ID:              fmt.Sprintf("daily:%s", d.Key),
			Kind:            "daily",
			Name:            d.Name,
			Thumbnails:      thumbs,
			ThumbnailURL:    coverURL,
			NormalThumbnail: coverURL,
			Endpoint:        fmt.Sprintf("/daily-playlist/%s", d.Key),
			RequiresAuth:    false,
		})
	}

	return &models.AvailablePlaylistsResponse{Items: items}, nil
}

// GetDailyPlaylist returns the track list for a daily playlist key.
func (s *DailyPlaylistService) GetDailyPlaylist(ctx context.Context, rawKey string, limit int, channelID int64, userID int64) (*models.BrowseResponse, error) {
	key := CanonDailyKey(rawKey)
	if limit <= 0 {
		limit = 75
	}
	if limit > 100 {
		limit = 100
	}

	today := time.Now().UTC().Format("2006-01-02")
	docID := fmt.Sprintf("%s:%s:%d", key, today, channelID)

	// 1. Check cache
	cached, err := s.dailyRepo.GetCachedDailyPlaylist(ctx, docID)
	if err == nil && cached != nil && len(cached.TrackIDs) > 0 {
		tracks, err := s.trackRepo.GetByIDs(ctx, cached.TrackIDs)
		if err == nil && len(tracks) > 0 {
			trackMap := make(map[string]*models.Track, len(tracks))
			for _, t := range tracks {
				trackMap[t.ID] = t
			}

			var items []*models.BrowseItem
			for _, tid := range cached.TrackIDs {
				if t, ok := trackMap[tid]; ok {
					items = append(items, t.ToBrowseItem())
				}
			}

			return &models.BrowseResponse{
				Page:    1,
				PerPage: limit,
				Total:   int64(len(items)),
				Items:   items,
			}, nil
		}
	}

	// 2. Generate playlist
	var tracks []*models.Track
	switch key {
	case "trending":
		// Highest play count
		tracks, _, err = s.trackRepo.List(ctx, 1, limit, "play_count", "", channelID)
	case "rising":
		// Recently added tracks
		tracks, _, err = s.trackRepo.List(ctx, 1, limit, "created_at", "", channelID)
	case "late-night":
		// Low tempo / chill
		tracks, _, err = s.trackRepo.List(ctx, 1, limit, "updated_at", "", channelID)
	default:
		// Random / Mix / Surprise
		tracks, err = s.trackRepo.Random(ctx, limit, channelID)
	}

	if err != nil {
		return nil, err
	}

	var trackIDs []string
	var items []*models.BrowseItem
	for _, t := range tracks {
		trackIDs = append(trackIDs, t.ID)
		items = append(items, t.ToBrowseItem())
	}

	// 3. Cache the generated playlist
	if len(trackIDs) > 0 {
		var firstCover string
		if len(items) > 0 {
			firstCover = items[0].CoverURL
		}
		doc := &models.DailyPlaylistDoc{
			ID:              docID,
			Key:             key,
			Date:            today,
			ChannelID:       channelID,
			TrackIDs:        trackIDs,
			ThumbnailURL:    firstCover,
			NormalThumbnail: firstCover,
			GeneratedAt:     float64(time.Now().Unix()),
		}
		_ = s.dailyRepo.SaveDailyPlaylist(ctx, doc)
	}

	return &models.BrowseResponse{
		Page:    1,
		PerPage: limit,
		Total:   int64(len(items)),
		Items:   items,
	}, nil
}
