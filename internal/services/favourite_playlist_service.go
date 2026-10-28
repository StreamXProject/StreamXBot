package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"streamgo/internal/models"
	"streamgo/internal/repository"
)

// FavouritePlaylistService orchestrates user favourites, custom playlists, albums, and history.
type FavouritePlaylistService struct {
	favRepo         repository.FavouritePlaylistRepository
	trackRepo       repository.TrackRepository
	artistAlbumRepo repository.ArtistAlbumRepository
}

// NewFavouritePlaylistService creates a new FavouritePlaylistService.
func NewFavouritePlaylistService(
	favRepo repository.FavouritePlaylistRepository,
	trackRepo repository.TrackRepository,
	artistAlbumRepo repository.ArtistAlbumRepository,
) *FavouritePlaylistService {
	return &FavouritePlaylistService{
		favRepo:         favRepo,
		trackRepo:       trackRepo,
		artistAlbumRepo: artistAlbumRepo,
	}
}

// Helper to extract track cover URL
func extractTrackThumbnail(t *models.Track) string {
	if t == nil {
		return ""
	}
	return t.EffectiveCoverURL()
}

// Helper to assemble up to 4 thumbnail URLs for playlist collage preview
func buildPlaylistThumbnails(coverURL string, trackThumbs []string, limit int) []string {
	var out []string
	seen := make(map[string]bool)

	for _, u := range trackThumbs {
		if len(out) >= limit {
			break
		}
		u = strings.TrimSpace(u)
		if u != "" && !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}

	if coverURL != "" && !seen[coverURL] && len(out) < limit {
		seen[coverURL] = true
		out = append(out, coverURL)
	}

	return out
}

// CreatePlaylist creates a new user playlist with a generated UUID.
func (s *FavouritePlaylistService) CreatePlaylist(ctx context.Context, userID int64, name string) (*models.PlaylistItem, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("name is required")
	}

	idBytes := make([]byte, 16)
	_, _ = rand.Read(idBytes)
	playlistID := hex.EncodeToString(idBytes)

	now := float64(time.Now().Unix())
	coverID := fmt.Sprintf("cover_%s", playlistID)
	coverURL := fmt.Sprintf("/covers/user-playlist/%s.png", playlistID)
	normalThumbnail := coverURL

	p := &models.UserPlaylist{
		ID:              playlistID,
		UserID:          userID,
		Name:            name,
		CoverID:         coverID,
		CoverURL:        coverURL,
		NormalThumbnail: normalThumbnail,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.favRepo.CreatePlaylist(ctx, p); err != nil {
		return nil, err
	}

	return &models.PlaylistItem{
		PlaylistID:      playlistID,
		Name:            name,
		Thumbnails:      buildPlaylistThumbnails(coverURL, nil, 4),
		CoverID:         coverID,
		CoverURL:        coverURL,
		NormalThumbnail: normalThumbnail,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

// GetUserPlaylists returns all playlists owned by user with up to 4 track cover thumbnails for collages.
func (s *FavouritePlaylistService) GetUserPlaylists(ctx context.Context, userID int64) (*models.PlaylistsResponse, error) {
	rawPlaylists, err := s.favRepo.GetUserPlaylists(ctx, userID)
	if err != nil {
		return nil, err
	}

	items := make([]models.PlaylistItem, 0, len(rawPlaylists))
	for _, p := range rawPlaylists {
		firstIDs, _ := s.favRepo.GetPlaylistFirstTrackIDs(ctx, p.ID, 4)
		var trackThumbs []string
		if len(firstIDs) > 0 {
			tracks, _ := s.trackRepo.GetByIDs(ctx, firstIDs)
			for _, t := range tracks {
				if thumb := extractTrackThumbnail(t); thumb != "" {
					trackThumbs = append(trackThumbs, thumb)
				}
			}
		}

		items = append(items, models.PlaylistItem{
			PlaylistID:      p.ID,
			Name:            p.Name,
			Thumbnails:      buildPlaylistThumbnails(p.CoverURL, trackThumbs, 4),
			CoverURL:        p.CoverURL,
			NormalThumbnail: p.NormalThumbnail,
			CoverID:         p.CoverID,
			CreatedAt:       p.CreatedAt,
			UpdatedAt:       p.UpdatedAt,
		})
	}

	return &models.PlaylistsResponse{Items: items}, nil
}

// RenamePlaylist renames an existing user playlist.
func (s *FavouritePlaylistService) RenamePlaylist(ctx context.Context, playlistID string, userID int64, name string) (*models.PlaylistItem, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("name is required")
	}

	coverID := fmt.Sprintf("cover_%s", playlistID)
	coverURL := fmt.Sprintf("/covers/user-playlist/%s.png", playlistID)
	normalThumbnail := coverURL

	p, err := s.favRepo.RenamePlaylist(ctx, playlistID, userID, name, coverID, coverURL, normalThumbnail)
	if err != nil {
		return nil, err
	}

	firstIDs, _ := s.favRepo.GetPlaylistFirstTrackIDs(ctx, playlistID, 4)
	var trackThumbs []string
	if len(firstIDs) > 0 {
		tracks, _ := s.trackRepo.GetByIDs(ctx, firstIDs)
		for _, t := range tracks {
			if thumb := extractTrackThumbnail(t); thumb != "" {
				trackThumbs = append(trackThumbs, thumb)
			}
		}
	}

	return &models.PlaylistItem{
		PlaylistID:      p.ID,
		Name:            p.Name,
		Thumbnails:      buildPlaylistThumbnails(p.CoverURL, trackThumbs, 4),
		CoverURL:        p.CoverURL,
		NormalThumbnail: p.NormalThumbnail,
		CoverID:         p.CoverID,
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
	}, nil
}

// DeletePlaylist deletes a playlist and cleans up its tracks.
func (s *FavouritePlaylistService) DeletePlaylist(ctx context.Context, playlistID string, userID int64) error {
	return s.favRepo.DeletePlaylist(ctx, playlistID, userID)
}

// AddTracksToPlaylist appends tracks to a playlist.
func (s *FavouritePlaylistService) AddTracksToPlaylist(ctx context.Context, playlistID string, userID int64, trackIDs []string) (int, error) {
	existing, err := s.favRepo.GetPlaylistByID(ctx, playlistID, userID)
	if err != nil {
		return 0, err
	}
	if existing == nil {
		return 0, errors.New("playlist not found")
	}

	// Verify tracks exist
	tracks, err := s.trackRepo.GetByIDs(ctx, trackIDs)
	if err != nil || len(tracks) == 0 {
		return 0, errors.New("tracks not found")
	}

	var validIDs []string
	for _, t := range tracks {
		validIDs = append(validIDs, t.ID)
	}

	return s.favRepo.AddTracksToPlaylist(ctx, playlistID, validIDs)
}

// RemoveTrackFromPlaylist removes a track from a playlist.
func (s *FavouritePlaylistService) RemoveTrackFromPlaylist(ctx context.Context, playlistID string, userID int64, trackID string) error {
	existing, err := s.favRepo.GetPlaylistByID(ctx, playlistID, userID)
	if err != nil {
		return err
	}
	if existing == nil {
		return errors.New("playlist not found")
	}

	return s.favRepo.RemoveTrackFromPlaylist(ctx, playlistID, trackID)
}

// GetPlaylistTracks returns paginated tracks inside a playlist.
func (s *FavouritePlaylistService) GetPlaylistTracks(ctx context.Context, playlistID string, userID int64, page, limit int) (*models.PlaylistTracksResponse, error) {
	existing, err := s.favRepo.GetPlaylistByID(ctx, playlistID, userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errors.New("playlist not found")
	}

	ids, total, err := s.favRepo.GetPlaylistTrackIDs(ctx, playlistID, page, limit)
	if err != nil {
		return nil, err
	}

	tracks, err := s.trackRepo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	items := make([]*models.BrowseItem, 0, len(tracks))
	for _, t := range tracks {
		items = append(items, t.ToBrowseItem())
	}

	return &models.PlaylistTracksResponse{
		Page:    page,
		PerPage: limit,
		Total:   total,
		Items:   items,
	}, nil
}

// GetSharedPlaylist returns public read-only playlist with tracks for sharing.
func (s *FavouritePlaylistService) GetSharedPlaylist(ctx context.Context, playlistID string) (*models.PlaylistShareResponse, error) {
	p, err := s.favRepo.GetPublicPlaylistByID(ctx, playlistID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errors.New("playlist not found")
	}

	ids, _, err := s.favRepo.GetPlaylistTrackIDs(ctx, playlistID, 1, 200)
	if err != nil {
		return nil, err
	}

	tracks, err := s.trackRepo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	items := make([]*models.BrowseItem, 0, len(tracks))
	for _, t := range tracks {
		items = append(items, t.ToBrowseItem())
	}

	return &models.PlaylistShareResponse{
		PlaylistID:      p.ID,
		Name:            p.Name,
		CoverURL:        p.CoverURL,
		NormalThumbnail: p.NormalThumbnail,
		Tracks:          items,
		CreatedAt:       p.CreatedAt,
	}, nil
}

// AddFavourite likes a track.
func (s *FavouritePlaylistService) AddFavourite(ctx context.Context, userID int64, trackID string) (bool, error) {
	trackID = strings.TrimSpace(trackID)
	if trackID == "" {
		return false, errors.New("track_id is required")
	}
	track, err := s.trackRepo.GetByID(ctx, trackID)
	if err != nil {
		return false, err
	}
	if track == nil {
		return false, errors.New("track not found")
	}
	return s.favRepo.AddFavourite(ctx, userID, trackID)
}

// RemoveFavourite unlikes a track.
func (s *FavouritePlaylistService) RemoveFavourite(ctx context.Context, userID int64, trackID string) (bool, error) {
	trackID = strings.TrimSpace(trackID)
	if trackID == "" {
		return false, errors.New("track_id is required")
	}
	return s.favRepo.RemoveFavourite(ctx, userID, trackID)
}

// GetFavourites returns paginated favorites with track details.
func (s *FavouritePlaylistService) GetFavourites(ctx context.Context, userID int64, page, limit int) (*models.FavouritesResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	ids, total, lastTS, err := s.favRepo.GetFavouriteTrackIDs(ctx, userID, page, limit)
	if err != nil {
		return nil, err
	}

	tracks, err := s.trackRepo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	byID := make(map[string]*models.Track)
	for _, t := range tracks {
		byID[t.ID] = t
	}

	items := make([]models.FavouriteItem, 0, len(ids))
	for _, id := range ids {
		if t, ok := byID[id]; ok {
			items = append(items, models.FavouriteItem{
				Track: t.ToBrowseItem(),
			})
		}
	}

	return &models.FavouritesResponse{
		Page:          page,
		PerPage:       limit,
		Total:         total,
		Items:         items,
		LastUpdatedAt: lastTS,
	}, nil
}

// GetFavouriteIDs returns lightweight list of liked track IDs.
func (s *FavouritePlaylistService) GetFavouriteIDs(ctx context.Context, userID int64, page, limit int) (*models.FavouriteIdsResponse, error) {
	ids, total, lastTS, err := s.favRepo.GetAllFavouriteTrackIDs(ctx, userID, page, limit)
	if err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []string{}
	}
	return &models.FavouriteIdsResponse{
		Page:          page,
		PerPage:       limit,
		Total:         total,
		IDs:           ids,
		Exists:        total > 0,
		LastUpdatedAt: lastTS,
	}, nil
}

// AddFavouriteArtist follows an artist.
func (s *FavouritePlaylistService) AddFavouriteArtist(ctx context.Context, userID int64, artistID string) (bool, error) {
	artistID = strings.TrimSpace(artistID)
	if artistID == "" {
		return false, errors.New("artist_id is required")
	}
	return s.favRepo.AddFavouriteArtist(ctx, userID, artistID)
}

// RemoveFavouriteArtist unfollows an artist.
func (s *FavouritePlaylistService) RemoveFavouriteArtist(ctx context.Context, userID int64, artistID string) (bool, error) {
	artistID = strings.TrimSpace(artistID)
	if artistID == "" {
		return false, errors.New("artist_id is required")
	}
	return s.favRepo.RemoveFavouriteArtist(ctx, userID, artistID)
}

// GetFavouriteArtistIDs returns list of followed artist IDs.
func (s *FavouritePlaylistService) GetFavouriteArtistIDs(ctx context.Context, userID int64) ([]string, error) {
	return s.favRepo.GetFavouriteArtistIDs(ctx, userID)
}

// SaveAlbums saves one or more albums for user.
func (s *FavouritePlaylistService) SaveAlbums(ctx context.Context, userID int64, albumIDs []string) (int, error) {
	count := 0
	for _, aid := range albumIDs {
		aid = strings.TrimSpace(aid)
		if aid == "" {
			continue
		}
		already, err := s.favRepo.SaveAlbum(ctx, userID, aid)
		if err == nil && !already {
			count++
		}
	}
	return count, nil
}

// GetSavedAlbums returns saved albums for user.
func (s *FavouritePlaylistService) GetSavedAlbums(ctx context.Context, userID int64, page, limit int) (*models.UserAlbumsResponse, error) {
	albumIDs, savedAts, total, err := s.favRepo.GetSavedAlbumIDs(ctx, userID, page, limit)
	if err != nil {
		return nil, err
	}

	items := make([]models.UserAlbumItem, 0, len(albumIDs))
	for i, aid := range albumIDs {
		var albumDoc any
		if s.artistAlbumRepo != nil {
			if alb, err := s.artistAlbumRepo.GetAlbumByID(ctx, aid); err == nil && alb != nil {
				albumDoc = alb
			}
		}
		var savedAt float64
		if i < len(savedAts) {
			savedAt = savedAts[i]
		}
		items = append(items, models.UserAlbumItem{
			AlbumID: aid,
			Album:   albumDoc,
			SavedAt: savedAt,
		})
	}

	return &models.UserAlbumsResponse{
		Page:    page,
		PerPage: limit,
		Total:   total,
		Items:   items,
	}, nil
}

// RecordListeningEvents records bulk telemetry events.
func (s *FavouritePlaylistService) RecordListeningEvents(ctx context.Context, userID *int64, events []models.ListeningEventItem) (int, error) {
	return s.favRepo.RecordListeningEvents(ctx, userID, events)
}

// GetUserHistory returns recent track items.
func (s *FavouritePlaylistService) GetUserHistory(ctx context.Context, userID int64, limit int) ([]*models.BrowseItem, error) {
	ids, err := s.favRepo.GetUserHistoryTrackIDs(ctx, userID, limit)
	if err != nil {
		return nil, err
	}

	tracks, err := s.trackRepo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	byID := make(map[string]*models.Track)
	for _, t := range tracks {
		byID[t.ID] = t
	}

	items := make([]*models.BrowseItem, 0, len(ids))
	for _, id := range ids {
		if t, ok := byID[id]; ok {
			items = append(items, t.ToBrowseItem())
		}
	}
	return items, nil
}

// GetUserTopPlayed returns user's top played tracks.
func (s *FavouritePlaylistService) GetUserTopPlayed(ctx context.Context, userID int64, page, limit int) ([]*models.BrowseItem, int64, error) {
	ids, total, err := s.favRepo.GetUserTopPlayedTrackIDs(ctx, userID, page, limit)
	if err != nil {
		return nil, 0, err
	}

	tracks, err := s.trackRepo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}

	byID := make(map[string]*models.Track)
	for _, t := range tracks {
		byID[t.ID] = t
	}

	items := make([]*models.BrowseItem, 0, len(ids))
	for _, id := range ids {
		if t, ok := byID[id]; ok {
			items = append(items, t.ToBrowseItem())
		}
	}
	return items, total, nil
}
