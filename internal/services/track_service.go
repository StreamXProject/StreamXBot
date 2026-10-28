package services

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"streamgo/internal/models"
	"streamgo/internal/repository"
)

// TrackService coordinates track-related business operations.
type TrackService struct {
	repo      repository.TrackRepository
	r2Storage *R2StorageService
}

// NewTrackService creates a new TrackService with the given repository.
func NewTrackService(repo repository.TrackRepository) *TrackService {
	return &TrackService{repo: repo}
}

// SetR2StorageService injects optional R2StorageService for artwork lifecycle management.
func (s *TrackService) SetR2StorageService(r2 *R2StorageService) {
	s.r2Storage = r2
}

// GetTrack retrieves a single track by its ID.
func (s *TrackService) GetTrack(ctx context.Context, id string) (*models.Track, error) {
	return s.repo.GetByID(ctx, id)
}

// Browse retrieves a paginated feed of tracks converted to lightweight BrowseItems.
func (s *TrackService) Browse(
	ctx context.Context,
	page, perPage int,
	sortField, topicName string,
	channelID int64,
) (*models.BrowseResponse, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	tracks, total, err := s.repo.List(ctx, page, perPage, sortField, topicName, channelID)
	if err != nil {
		return nil, err
	}

	items := make([]*models.BrowseItem, 0, len(tracks))
	for _, t := range tracks {
		items = append(items, t.ToBrowseItem())
	}

	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(perPage)))
	}

	coverURL := ""
	for _, item := range items {
		if item.CoverURL != "" {
			coverURL = item.CoverURL
			break
		}
	}

	return &models.BrowseResponse{
		Items:      items,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
		CoverURL:   coverURL,
	}, nil
}

// Search queries tracks by artist, title, or album matching.
func (s *TrackService) Search(ctx context.Context, query string, limit int) ([]*models.BrowseItem, error) {
	tracks, err := s.repo.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	items := make([]*models.BrowseItem, 0, len(tracks))
	for _, t := range tracks {
		items = append(items, t.ToBrowseItem())
	}

	return items, nil
}

// GetRandom retrieves a randomized selection of tracks.
func (s *TrackService) GetRandom(ctx context.Context, limit int, channelID int64) ([]*models.BrowseItem, error) {
	tracks, err := s.repo.Random(ctx, limit, channelID)
	if err != nil {
		return nil, err
	}

	items := make([]*models.BrowseItem, 0, len(tracks))
	for _, t := range tracks {
		items = append(items, t.ToBrowseItem())
	}

	return items, nil
}

// GetTopics returns aggregated topic lists with counts matching Api/schemas/topics.py.
func (s *TrackService) GetTopics(ctx context.Context, channelID int64, limit int) (*models.TopicsResponse, error) {
	topics, err := s.repo.GetTopics(ctx, channelID, limit)
	if err != nil {
		return nil, err
	}

	topicNames := make([]string, 0, len(topics))
	for _, t := range topics {
		topicNames = append(topicNames, t.TopicName)
	}

	return &models.TopicsResponse{
		OK:     true,
		Total:  len(topics),
		Items:  topics,
		Topics: topicNames,
	}, nil
}

// GetChannelIDs returns all unique indexed Telegram source channel IDs.
func (s *TrackService) GetChannelIDs(ctx context.Context) (*models.ChannelIDsResponse, error) {
	cids, err := s.repo.GetChannelIDs(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]models.ChannelItem, 0, len(cids))
	for _, id := range cids {
		items = append(items, models.ChannelItem{
			ID: id,
		})
	}

	return &models.ChannelIDsResponse{
		OK:    true,
		Items: items,
	}, nil
}

// DeleteTrackOptions configures deletion behavior.
type DeleteTrackOptions struct {
	Hard         bool
	PurgeCache   bool
	PurgeArtwork bool
}

// DeleteTrackResult summarizes the outcome of track deletion.
type DeleteTrackResult struct {
	TrackID        string   `json:"track_id"`
	Mode           string   `json:"mode"`
	CachePurged    bool     `json:"cache_purged"`
	PurgedArtworks []string `json:"purged_artworks,omitempty"`
	Message        string   `json:"message"`
}

// DeleteTrack deletes a track (soft by default, or hard) and purges associated transcode cache and orphaned R2 artwork.
func (s *TrackService) DeleteTrack(ctx context.Context, id string, opts DeleteTrackOptions) (*DeleteTrackResult, error) {
	track, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("track lookup failed: %w", err)
	}
	if track == nil {
		return nil, fmt.Errorf("track not found")
	}

	mode := "soft"
	if opts.Hard {
		if err := s.repo.HardDelete(ctx, id); err != nil {
			return nil, fmt.Errorf("failed to hard delete track: %w", err)
		}
		mode = "hard"
	} else {
		if err := s.repo.SoftDelete(ctx, id); err != nil {
			return nil, fmt.Errorf("failed to soft delete track: %w", err)
		}
	}

	res := &DeleteTrackResult{
		TrackID: id,
		Mode:    mode,
		Message: "Track successfully deleted",
	}

	// 1. Purge local transcode cache if requested
	if opts.PurgeCache {
		pattern := filepath.Join("stream_media", "alac_cache", fmt.Sprintf("%s*", id))
		if files, err := filepath.Glob(pattern); err == nil {
			for _, file := range files {
				if info, err := os.Stat(file); err == nil && !info.IsDir() {
					if rmErr := os.Remove(file); rmErr == nil {
						res.CachePurged = true
					}
				}
			}
		}
	}

	// 2. Safely purge orphaned artwork from Cloudflare R2 if requested and R2 is configured
	if opts.PurgeArtwork && s.r2Storage != nil && s.r2Storage.IsConfigured() {
		var candidates []string
		cfCover := track.Spotify.CloudflareCoverURL
		if cfCover == "" {
			cfCover = track.CloudflareCoverURL
		}
		if cfCover != "" {
			candidates = append(candidates, cfCover)
		}

		cfBigCover := track.Spotify.CloudflareBigCoverURL
		if cfBigCover == "" {
			cfBigCover = track.CloudflareBigCoverURL
		}
		if cfBigCover != "" && cfBigCover != cfCover {
			candidates = append(candidates, cfBigCover)
		}

		for _, coverURL := range candidates {
			refs, countErr := s.repo.CountArtworkReferences(ctx, id, coverURL)
			if countErr == nil && refs == 0 {
				if delErr := s.r2Storage.DeleteCoverByURL(ctx, coverURL); delErr == nil {
					res.PurgedArtworks = append(res.PurgedArtworks, coverURL)
				}
			}
		}
	}

	return res, nil
}
