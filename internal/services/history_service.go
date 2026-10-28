package services

import (
	"context"

	"streamgo/internal/models"
	"streamgo/internal/repository"
)

// HistoryService handles playback history and telemetry processing.
type HistoryService struct {
	histRepo  repository.HistoryRepository
	trackRepo repository.TrackRepository
}

// NewHistoryService creates a new HistoryService.
func NewHistoryService(histRepo repository.HistoryRepository, trackRepo repository.TrackRepository) *HistoryService {
	return &HistoryService{
		histRepo:  histRepo,
		trackRepo: trackRepo,
	}
}

// GetHistory returns user playback history as browse items.
func (s *HistoryService) GetHistory(ctx context.Context, userID int64, limit int) (*models.BrowseResponse, error) {
	if limit <= 0 {
		limit = 50
	}

	trackIDs, err := s.histRepo.GetUserHistory(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	if len(trackIDs) == 0 {
		return &models.BrowseResponse{
			Page:    1,
			PerPage: limit,
			Total:   0,
			Items:   []*models.BrowseItem{},
		}, nil
	}

	tracks, err := s.trackRepo.GetByIDs(ctx, trackIDs)
	if err != nil {
		return nil, err
	}

	trackMap := make(map[string]*models.Track, len(tracks))
	for _, t := range tracks {
		trackMap[t.ID] = t
	}

	var items []*models.BrowseItem
	for _, tid := range trackIDs {
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

// GetTopPlayed returns user's most played tracks as browse items.
func (s *HistoryService) GetTopPlayed(ctx context.Context, userID int64, limit int) (*models.BrowseResponse, error) {
	if limit <= 0 {
		limit = 50
	}

	trackIDs, err := s.histRepo.GetUserTopPlayed(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	if len(trackIDs) == 0 {
		return &models.BrowseResponse{
			Page:    1,
			PerPage: limit,
			Total:   0,
			Items:   []*models.BrowseItem{},
		}, nil
	}

	tracks, err := s.trackRepo.GetByIDs(ctx, trackIDs)
	if err != nil {
		return nil, err
	}

	trackMap := make(map[string]*models.Track, len(tracks))
	for _, t := range tracks {
		trackMap[t.ID] = t
	}

	var items []*models.BrowseItem
	for _, tid := range trackIDs {
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

// RecordPlay logs playback directly (e.g. from stream).
func (s *HistoryService) RecordPlay(ctx context.Context, userID int64, trackID string, source string) error {
	return s.histRepo.RecordPlay(ctx, userID, trackID, source)
}

// RecordEvents records batch client listening events.
func (s *HistoryService) RecordEvents(ctx context.Context, userID int64, events []models.ListeningEventItem) error {
	return s.histRepo.RecordListeningEvents(ctx, userID, events)
}
