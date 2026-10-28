package services

import (
	"context"

	"streamgo/internal/models"
	"streamgo/internal/repository"
)

// ArtistAlbumService orchestrates artist and album operations.
type ArtistAlbumService struct {
	repo      repository.ArtistAlbumRepository
	trackRepo repository.TrackRepository
}

// NewArtistAlbumService creates a new ArtistAlbumService.
func NewArtistAlbumService(repo repository.ArtistAlbumRepository, trackRepo repository.TrackRepository) *ArtistAlbumService {
	return &ArtistAlbumService{
		repo:      repo,
		trackRepo: trackRepo,
	}
}

// ListArtists returns a paginated list of artists matching Python schema.
func (s *ArtistAlbumService) ListArtists(ctx context.Context, page, perPage int, refresh bool) (*models.ArtistsResponse, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}
	if perPage > 200 {
		perPage = 200
	}

	artists, total, err := s.repo.ListArtists(ctx, page, perPage, refresh)
	if err != nil {
		return nil, err
	}

	return &models.ArtistsResponse{
		Ok:      true,
		Page:    page,
		PerPage: perPage,
		Total:   total,
		Items:   artists,
	}, nil
}

// GetArtistDetail returns detailed artist information with top tracks and albums.
func (s *ArtistAlbumService) GetArtistDetail(ctx context.Context, id string) (*models.ArtistDetail, error) {
	artist, err := s.repo.GetArtistByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if artist == nil {
		return nil, nil
	}

	tracks, err := s.repo.GetArtistTracks(ctx, artist.Name, 50)
	if err != nil {
		return nil, err
	}

	browseItems := make([]*models.BrowseItem, 0, len(tracks))
	for _, t := range tracks {
		browseItems = append(browseItems, t.ToBrowseItem())
	}

	popularTracks := browseItems
	if len(popularTracks) > 10 {
		popularTracks = popularTracks[:10]
	}

	albums, err := s.repo.GetArtistAlbums(ctx, artist.Name)
	if err != nil {
		return nil, err
	}

	singles := make([]*models.BrowseItem, 0)
	for _, t := range browseItems {
		if t.Album == "" {
			singles = append(singles, t)
		}
	}

	return &models.ArtistDetail{
		Ok:            true,
		Artist:        artist,
		PopularTracks: popularTracks,
		TopTracks:     popularTracks,
		Releases:      albums,
		Albums:        albums,
		Singles:       singles,
		Tracks:        browseItems,
	}, nil
}

// GetArtistTracks returns the top tracks for a specific artist ID.
func (s *ArtistAlbumService) GetArtistTracks(ctx context.Context, id string, limit int) ([]*models.BrowseItem, error) {
	artist, err := s.repo.GetArtistByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if artist == nil {
		return nil, nil
	}

	tracks, err := s.repo.GetArtistTracks(ctx, artist.Name, limit)
	if err != nil {
		return nil, err
	}

	items := make([]*models.BrowseItem, 0, len(tracks))
	for _, t := range tracks {
		items = append(items, t.ToBrowseItem())
	}
	return items, nil
}

// GetArtistTracksPaginated returns paginated tracks for a specific artist ID.
func (s *ArtistAlbumService) GetArtistTracksPaginated(ctx context.Context, id string, page, perPage int) ([]*models.BrowseItem, int64, error) {
	artist, err := s.repo.GetArtistByID(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	if artist == nil {
		return nil, 0, nil
	}

	tracks, total, err := s.repo.GetArtistTracksPaginated(ctx, artist.Name, page, perPage)
	if err != nil {
		return nil, 0, err
	}

	items := make([]*models.BrowseItem, 0, len(tracks))
	for _, t := range tracks {
		items = append(items, t.ToBrowseItem())
	}
	return items, total, nil
}

// ListAlbums returns a paginated list of albums matching Python schema.
func (s *ArtistAlbumService) ListAlbums(ctx context.Context, page, perPage int, artistFilter string, refresh bool) (*models.AlbumsResponse, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}
	if perPage > 200 {
		perPage = 200
	}

	albums, total, err := s.repo.ListAlbums(ctx, page, perPage, artistFilter, refresh)
	if err != nil {
		return nil, err
	}

	return &models.AlbumsResponse{
		Ok:      true,
		Page:    page,
		PerPage: perPage,
		Total:   total,
		Items:   albums,
	}, nil
}

// GetAlbumDetail returns full album metadata along with its tracklist.
func (s *ArtistAlbumService) GetAlbumDetail(ctx context.Context, id string) (*models.AlbumDetail, error) {
	album, err := s.repo.GetAlbumByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if album == nil {
		return nil, nil
	}

	tracks, err := s.repo.GetAlbumTracks(ctx, album.ID)
	if err != nil {
		return nil, err
	}

	browseItems := make([]*models.BrowseItem, 0, len(tracks))
	for _, t := range tracks {
		browseItems = append(browseItems, t.ToBrowseItem())
	}

	return &models.AlbumDetail{
		Ok:     true,
		Album:  album,
		Tracks: browseItems,
	}, nil
}

// GetAlbumTracks returns only the tracks of an album.
func (s *ArtistAlbumService) GetAlbumTracks(ctx context.Context, id string) ([]*models.BrowseItem, error) {
	tracks, err := s.repo.GetAlbumTracks(ctx, id)
	if err != nil {
		return nil, err
	}

	browseItems := make([]*models.BrowseItem, 0, len(tracks))
	for _, t := range tracks {
		browseItems = append(browseItems, t.ToBrowseItem())
	}
	return browseItems, nil
}

// GetAlbumTracksPaginated returns paginated tracks for an album.
func (s *ArtistAlbumService) GetAlbumTracksPaginated(ctx context.Context, id string, page, perPage int) ([]*models.BrowseItem, int64, error) {
	tracks, total, err := s.repo.GetAlbumTracksPaginated(ctx, id, page, perPage)
	if err != nil {
		return nil, 0, err
	}

	browseItems := make([]*models.BrowseItem, 0, len(tracks))
	for _, t := range tracks {
		browseItems = append(browseItems, t.ToBrowseItem())
	}
	return browseItems, total, nil
}
