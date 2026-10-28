package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"streamgo/internal/database"
	"streamgo/internal/models"
)

// FavouritePlaylistRepository defines data operations for favourites, playlists, albums, and history.
type FavouritePlaylistRepository interface {
	// Track Favourites
	AddFavourite(ctx context.Context, userID int64, trackID string) (alreadyExists bool, err error)
	RemoveFavourite(ctx context.Context, userID int64, trackID string) (deleted bool, err error)
	GetFavouriteTrackIDs(ctx context.Context, userID int64, page, limit int) ([]string, int64, *float64, error)
	GetAllFavouriteTrackIDs(ctx context.Context, userID int64, page, limit int) ([]string, int64, *float64, error)

	// Artist Favourites
	AddFavouriteArtist(ctx context.Context, userID int64, artistID string) (alreadyExists bool, err error)
	RemoveFavouriteArtist(ctx context.Context, userID int64, artistID string) (deleted bool, err error)
	GetFavouriteArtistIDs(ctx context.Context, userID int64) ([]string, error)

	// Playlists
	CreatePlaylist(ctx context.Context, p *models.UserPlaylist) error
	GetUserPlaylists(ctx context.Context, userID int64) ([]*models.UserPlaylist, error)
	GetPlaylistByID(ctx context.Context, playlistID string, userID int64) (*models.UserPlaylist, error)
	GetPublicPlaylistByID(ctx context.Context, playlistID string) (*models.UserPlaylist, error)
	RenamePlaylist(ctx context.Context, playlistID string, userID int64, name, coverID, coverURL, normalThumbnail string) (*models.UserPlaylist, error)
	DeletePlaylist(ctx context.Context, playlistID string, userID int64) error
	AddTracksToPlaylist(ctx context.Context, playlistID string, trackIDs []string) (int, error)
	RemoveTrackFromPlaylist(ctx context.Context, playlistID string, trackID string) error
	GetPlaylistTrackIDs(ctx context.Context, playlistID string, page, limit int) ([]string, int64, error)
	GetPlaylistFirstTrackIDs(ctx context.Context, playlistID string, limit int) ([]string, error)

	// Albums
	SaveAlbum(ctx context.Context, userID int64, albumID string) (alreadyExists bool, err error)
	GetSavedAlbumIDs(ctx context.Context, userID int64, page, limit int) (albumIDs []string, savedAts []float64, total int64, err error)

	// History & Listening Events
	RecordListeningEvents(ctx context.Context, userID *int64, events []models.ListeningEventItem) (int, error)
	GetUserHistoryTrackIDs(ctx context.Context, userID int64, limit int) ([]string, error)
	GetUserTopPlayedTrackIDs(ctx context.Context, userID int64, page, limit int) ([]string, int64, error)
}

type mongoFavouritePlaylistRepository struct {
	favCol         *mongo.Collection
	favArtistCol   *mongo.Collection
	artistsCol     *mongo.Collection
	playlistsCol   *mongo.Collection
	playlistTrkCol *mongo.Collection
	userAlbumsCol  *mongo.Collection
	historyCol     *mongo.Collection
	eventsCol      *mongo.Collection
}

// NewFavouritePlaylistRepository returns an instance of FavouritePlaylistRepository.
func NewFavouritePlaylistRepository(db *database.Client) FavouritePlaylistRepository {
	return &mongoFavouritePlaylistRepository{
		favCol:         db.Collection("user_favourites"),
		favArtistCol:   db.Collection("user_favourite_artists"),
		artistsCol:     db.Collection("artists"),
		playlistsCol:   db.Collection("user_playlists"),
		playlistTrkCol: db.Collection("playlist_tracks"),
		userAlbumsCol:  db.Collection("user_albums"),
		historyCol:     db.Collection("userHistory"),
		eventsCol:      db.Collection("listening_events"),
	}
}

// AddFavourite adds a track to user_favourites.
func (r *mongoFavouritePlaylistRepository) AddFavourite(ctx context.Context, userID int64, trackID string) (bool, error) {
	now := float64(time.Now().Unix())
	filter := bson.M{"user_id": userID, "track_id": trackID}
	update := bson.M{
		"$setOnInsert": bson.M{"created_at": now},
		"$set":         bson.M{"user_id": userID, "track_id": trackID, "updated_at": now},
	}
	opts := options.UpdateOne().SetUpsert(true)
	res, err := r.favCol.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return false, fmt.Errorf("failed to add favourite: %w", err)
	}
	alreadyExists := res.UpsertedID == nil
	return alreadyExists, nil
}

// RemoveFavourite deletes a track from user_favourites.
func (r *mongoFavouritePlaylistRepository) RemoveFavourite(ctx context.Context, userID int64, trackID string) (bool, error) {
	filter := bson.M{"user_id": userID, "track_id": trackID}
	res, err := r.favCol.DeleteOne(ctx, filter)
	if err != nil {
		return false, fmt.Errorf("failed to remove favourite: %w", err)
	}
	return res.DeletedCount > 0, nil
}

// GetFavouriteTrackIDs returns paginated track IDs liked by the user.
func (r *mongoFavouritePlaylistRepository) GetFavouriteTrackIDs(ctx context.Context, userID int64, page, limit int) ([]string, int64, *float64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	skip := int64((page - 1) * limit)

	filter := bson.M{"user_id": userID}
	total, err := r.favCol.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to count favourites: %w", err)
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(skip).
		SetLimit(int64(limit)).
		SetProjection(bson.M{"track_id": 1, "created_at": 1, "updated_at": 1, "_id": 0})

	cur, err := r.favCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to query favourites: %w", err)
	}
	defer cur.Close(ctx)

	var ids []string
	var lastTS *float64

	for cur.Next(ctx) {
		var doc struct {
			TrackID   string   `bson:"track_id"`
			CreatedAt *float64 `bson:"created_at"`
			UpdatedAt *float64 `bson:"updated_at"`
		}
		if err := cur.Decode(&doc); err == nil && doc.TrackID != "" {
			ids = append(ids, doc.TrackID)
			ts := doc.UpdatedAt
			if ts == nil {
				ts = doc.CreatedAt
			}
			if ts != nil && (lastTS == nil || *ts > *lastTS) {
				lastTS = ts
			}
		}
	}

	return ids, total, lastTS, nil
}

// GetAllFavouriteTrackIDs returns IDs for the /favourites/ids endpoint.
func (r *mongoFavouritePlaylistRepository) GetAllFavouriteTrackIDs(ctx context.Context, userID int64, page, limit int) ([]string, int64, *float64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 200
	}
	skip := int64((page - 1) * limit)

	filter := bson.M{"user_id": userID}
	total, err := r.favCol.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, nil, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(skip).
		SetLimit(int64(limit)).
		SetProjection(bson.M{"track_id": 1, "created_at": 1, "updated_at": 1, "_id": 0})

	cur, err := r.favCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, nil, err
	}
	defer cur.Close(ctx)

	var ids []string
	var lastTS *float64

	for cur.Next(ctx) {
		var doc struct {
			TrackID   string   `bson:"track_id"`
			CreatedAt *float64 `bson:"created_at"`
			UpdatedAt *float64 `bson:"updated_at"`
		}
		if err := cur.Decode(&doc); err == nil && doc.TrackID != "" {
			ids = append(ids, doc.TrackID)
			ts := doc.UpdatedAt
			if ts == nil {
				ts = doc.CreatedAt
			}
			if ts != nil && (lastTS == nil || *ts > *lastTS) {
				lastTS = ts
			}
		}
	}
	return ids, total, lastTS, nil
}

// AddFavouriteArtist adds an artist to user_favourite_artists and increments followers on artists collection.
func (r *mongoFavouritePlaylistRepository) AddFavouriteArtist(ctx context.Context, userID int64, artistID string) (bool, error) {
	now := float64(time.Now().Unix())
	filter := bson.M{"user_id": userID, "artist_id": artistID}
	update := bson.M{
		"$setOnInsert": bson.M{"created_at": now},
		"$set":         bson.M{"user_id": userID, "artist_id": artistID, "updated_at": now},
	}
	opts := options.UpdateOne().SetUpsert(true)
	res, err := r.favArtistCol.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return false, fmt.Errorf("failed to add artist favourite: %w", err)
	}

	alreadyExists := res.UpsertedID == nil
	if !alreadyExists {
		_, _ = r.artistsCol.UpdateOne(ctx, bson.M{"_id": artistID}, bson.M{"$inc": bson.M{"followers": 1}})
	}
	return alreadyExists, nil
}

// RemoveFavouriteArtist removes an artist from user_favourite_artists and decrements followers.
func (r *mongoFavouritePlaylistRepository) RemoveFavouriteArtist(ctx context.Context, userID int64, artistID string) (bool, error) {
	filter := bson.M{"user_id": userID, "artist_id": artistID}
	res, err := r.favArtistCol.DeleteOne(ctx, filter)
	if err != nil {
		return false, fmt.Errorf("failed to remove artist favourite: %w", err)
	}
	if res.DeletedCount > 0 {
		_, _ = r.artistsCol.UpdateOne(ctx, bson.M{"_id": artistID}, bson.M{"$inc": bson.M{"followers": -1}})
	}
	return res.DeletedCount > 0, nil
}

// GetFavouriteArtistIDs returns list of followed artist IDs.
func (r *mongoFavouritePlaylistRepository) GetFavouriteArtistIDs(ctx context.Context, userID int64) ([]string, error) {
	filter := bson.M{"user_id": userID}
	cur, err := r.favArtistCol.Find(ctx, filter, options.Find().SetProjection(bson.M{"artist_id": 1, "_id": 0}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var ids []string
	for cur.Next(ctx) {
		var doc struct {
			ArtistID string `bson:"artist_id"`
		}
		if err := cur.Decode(&doc); err == nil && doc.ArtistID != "" {
			ids = append(ids, doc.ArtistID)
		}
	}
	return ids, nil
}

// CreatePlaylist inserts a new playlist into user_playlists.
func (r *mongoFavouritePlaylistRepository) CreatePlaylist(ctx context.Context, p *models.UserPlaylist) error {
	_, err := r.playlistsCol.InsertOne(ctx, p)
	return err
}

// GetUserPlaylists returns all playlists owned by a user sorted by creation date descending.
func (r *mongoFavouritePlaylistRepository) GetUserPlaylists(ctx context.Context, userID int64) ([]*models.UserPlaylist, error) {
	filter := bson.M{"user_id": userID}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	cur, err := r.playlistsCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var list []*models.UserPlaylist
	for cur.Next(ctx) {
		var p models.UserPlaylist
		if err := cur.Decode(&p); err == nil {
			list = append(list, &p)
		}
	}
	return list, nil
}

// GetPlaylistByID returns playlist owned by a specific user.
func (r *mongoFavouritePlaylistRepository) GetPlaylistByID(ctx context.Context, playlistID string, userID int64) (*models.UserPlaylist, error) {
	filter := bson.M{"_id": playlistID, "user_id": userID}
	var p models.UserPlaylist
	err := r.playlistsCol.FindOne(ctx, filter).Decode(&p)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// GetPublicPlaylistByID returns playlist by ID for sharing.
func (r *mongoFavouritePlaylistRepository) GetPublicPlaylistByID(ctx context.Context, playlistID string) (*models.UserPlaylist, error) {
	filter := bson.M{"_id": playlistID}
	var p models.UserPlaylist
	err := r.playlistsCol.FindOne(ctx, filter).Decode(&p)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// RenamePlaylist renames a playlist.
func (r *mongoFavouritePlaylistRepository) RenamePlaylist(ctx context.Context, playlistID string, userID int64, name, coverID, coverURL, normalThumbnail string) (*models.UserPlaylist, error) {
	now := float64(time.Now().Unix())
	filter := bson.M{"_id": playlistID, "user_id": userID}
	update := bson.M{
		"$set": bson.M{
			"name":             name,
			"cover_id":         coverID,
			"cover_url":        coverURL,
			"normal_thumbnail": normalThumbnail,
			"updated_at":       now,
		},
	}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var p models.UserPlaylist
	err := r.playlistsCol.FindOneAndUpdate(ctx, filter, update, opts).Decode(&p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DeletePlaylist removes a playlist and its associated tracks.
func (r *mongoFavouritePlaylistRepository) DeletePlaylist(ctx context.Context, playlistID string, userID int64) error {
	filter := bson.M{"_id": playlistID, "user_id": userID}
	res, err := r.playlistsCol.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return errors.New("playlist not found")
	}
	_, _ = r.playlistTrkCol.DeleteMany(ctx, bson.M{"playlist_id": playlistID})
	return nil
}

// AddTracksToPlaylist adds tracks with incremental position numbers.
func (r *mongoFavouritePlaylistRepository) AddTracksToPlaylist(ctx context.Context, playlistID string, trackIDs []string) (int, error) {
	var last models.PlaylistTrack
	lastOpt := options.FindOne().SetSort(bson.D{{Key: "position", Value: -1}})
	nextPos := 1
	if err := r.playlistTrkCol.FindOne(ctx, bson.M{"playlist_id": playlistID}, lastOpt).Decode(&last); err == nil {
		nextPos = last.Position + 1
	}

	now := float64(time.Now().Unix())
	addedCount := 0

	for _, tid := range trackIDs {
		filter := bson.M{"playlist_id": playlistID, "track_id": tid}
		update := bson.M{
			"$setOnInsert": bson.M{"position": nextPos, "added_at": now},
			"$set":         bson.M{"playlist_id": playlistID, "track_id": tid},
		}
		opts := options.UpdateOne().SetUpsert(true)
		res, err := r.playlistTrkCol.UpdateOne(ctx, filter, update, opts)
		if err != nil {
			return addedCount, err
		}
		if res.UpsertedID != nil {
			addedCount++
			nextPos++
		}
	}

	return addedCount, nil
}

// RemoveTrackFromPlaylist deletes a track from playlist_tracks.
func (r *mongoFavouritePlaylistRepository) RemoveTrackFromPlaylist(ctx context.Context, playlistID string, trackID string) error {
	filter := bson.M{"playlist_id": playlistID, "track_id": trackID}
	res, err := r.playlistTrkCol.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return errors.New("track not found in playlist")
	}
	return nil
}

// GetPlaylistTrackIDs returns paginated track IDs in a playlist ordered by position.
func (r *mongoFavouritePlaylistRepository) GetPlaylistTrackIDs(ctx context.Context, playlistID string, page, limit int) ([]string, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	skip := int64((page - 1) * limit)

	filter := bson.M{"playlist_id": playlistID}
	total, err := r.playlistTrkCol.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "position", Value: 1}}).
		SetSkip(skip).
		SetLimit(int64(limit)).
		SetProjection(bson.M{"track_id": 1, "_id": 0})

	cur, err := r.playlistTrkCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)

	var ids []string
	for cur.Next(ctx) {
		var doc struct {
			TrackID string `bson:"track_id"`
		}
		if err := cur.Decode(&doc); err == nil && doc.TrackID != "" {
			ids = append(ids, doc.TrackID)
		}
	}
	return ids, total, nil
}

// GetPlaylistFirstTrackIDs returns the first N track IDs for playlist cover collage generation.
func (r *mongoFavouritePlaylistRepository) GetPlaylistFirstTrackIDs(ctx context.Context, playlistID string, limit int) ([]string, error) {
	if limit < 1 {
		limit = 4
	}
	filter := bson.M{"playlist_id": playlistID}
	opts := options.Find().
		SetSort(bson.D{{Key: "position", Value: 1}}).
		SetLimit(int64(limit)).
		SetProjection(bson.M{"track_id": 1, "_id": 0})

	cur, err := r.playlistTrkCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var ids []string
	for cur.Next(ctx) {
		var doc struct {
			TrackID string `bson:"track_id"`
		}
		if err := cur.Decode(&doc); err == nil && doc.TrackID != "" {
			ids = append(ids, doc.TrackID)
		}
	}
	return ids, nil
}

// SaveAlbum adds an album to user_albums.
func (r *mongoFavouritePlaylistRepository) SaveAlbum(ctx context.Context, userID int64, albumID string) (bool, error) {
	now := float64(time.Now().Unix())
	filter := bson.M{"user_id": userID, "album_id": albumID}
	update := bson.M{
		"$setOnInsert": bson.M{"user_id": userID, "album_id": albumID, "saved_at": now},
		"$set":         bson.M{"updated_at": now},
	}
	opts := options.UpdateOne().SetUpsert(true)
	res, err := r.userAlbumsCol.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return false, fmt.Errorf("failed to save album: %w", err)
	}
	alreadyExists := res.UpsertedID == nil
	return alreadyExists, nil
}

// GetSavedAlbumIDs returns saved albums for user.
func (r *mongoFavouritePlaylistRepository) GetSavedAlbumIDs(ctx context.Context, userID int64, page, limit int) ([]string, []float64, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	skip := int64((page - 1) * limit)

	filter := bson.M{"user_id": userID}
	total, err := r.userAlbumsCol.CountDocuments(ctx, filter)
	if err != nil {
		return nil, nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "saved_at", Value: -1}}).
		SetSkip(skip).
		SetLimit(int64(limit))

	cur, err := r.userAlbumsCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, nil, 0, err
	}
	defer cur.Close(ctx)

	var albumIDs []string
	var savedAts []float64

	for cur.Next(ctx) {
		var doc models.UserAlbum
		if err := cur.Decode(&doc); err == nil && doc.AlbumID != "" {
			albumIDs = append(albumIDs, doc.AlbumID)
			savedAts = append(savedAts, doc.SavedAt)
		}
	}
	return albumIDs, savedAts, total, nil
}

// RecordListeningEvents bulk upserts telemetry events into listening_events.
func (r *mongoFavouritePlaylistRepository) RecordListeningEvents(ctx context.Context, userID *int64, events []models.ListeningEventItem) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}

	modelsList := make([]mongo.WriteModel, 0, len(events))
	now := float64(time.Now().Unix())

	for _, ev := range events {
		if ev.ID == "" {
			continue
		}
		filter := bson.M{"event_id": ev.ID}
		if userID != nil {
			filter["user_id"] = *userID
		}
		setFields := bson.M{
			"event_id":    ev.ID,
			"track_id":    ev.TrackID,
			"played_at":   ev.PlayedAt,
			"started_at":  ev.StartedAt,
			"played_ms":   ev.PlayedMs,
			"duration_ms": ev.DurationMs,
			"completed":   ev.Completed,
			"skipped":     ev.Skipped,
			"source":      ev.Source,
		}
		if userID != nil {
			setFields["user_id"] = *userID
		}
		if ev.SessionID != "" {
			setFields["session_id"] = ev.SessionID
		}

		update := bson.M{
			"$setOnInsert": bson.M{"created_at": now},
			"$set":         setFields,
		}
		model := mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(update).SetUpsert(true)
		modelsList = append(modelsList, model)
	}

	if len(modelsList) == 0 {
		return 0, nil
	}

	opts := options.BulkWrite().SetOrdered(false)
	res, err := r.eventsCol.BulkWrite(ctx, modelsList, opts)
	if err != nil {
		return 0, fmt.Errorf("bulk write listening events failed: %w", err)
	}
	return int(res.UpsertedCount + res.ModifiedCount), nil
}

// GetUserHistoryTrackIDs returns recent track IDs from userHistory.
func (r *mongoFavouritePlaylistRepository) GetUserHistoryTrackIDs(ctx context.Context, userID int64, limit int) ([]string, error) {
	if limit < 1 {
		limit = 100
	}
	filter := bson.M{
		"$or": []bson.M{
			{"user_id": userID},
			{"user_id": fmt.Sprintf("%d", userID)},
		},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "played_at", Value: -1}}).
		SetLimit(int64(limit * 2))

	cur, err := r.historyCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	seen := make(map[string]bool)
	var ordered []string

	for cur.Next(ctx) {
		var doc struct {
			TrackID string `bson:"track_id"`
		}
		if err := cur.Decode(&doc); err == nil && doc.TrackID != "" {
			if !seen[doc.TrackID] {
				seen[doc.TrackID] = true
				ordered = append(ordered, doc.TrackID)
				if len(ordered) >= limit {
					break
				}
			}
		}
	}
	return ordered, nil
}

// GetUserTopPlayedTrackIDs returns most frequently played track IDs from listening_events.
func (r *mongoFavouritePlaylistRepository) GetUserTopPlayedTrackIDs(ctx context.Context, userID int64, page, limit int) ([]string, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	skip := int64((page - 1) * limit)

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{"user_id": userID}}},
		bson.D{{Key: "$group", Value: bson.M{"_id": "$track_id", "play_count": bson.M{"$sum": 1}}}},
		bson.D{{Key: "$sort", Value: bson.M{"play_count": -1}}},
	}

	countPipeline := append(pipeline, bson.D{{Key: "$count", Value: "total"}})
	countCur, err := r.eventsCol.Aggregate(ctx, countPipeline)
	var total int64
	if err == nil {
		if countCur.Next(ctx) {
			var cnt struct {
				Total int64 `bson:"total"`
			}
			if err := countCur.Decode(&cnt); err == nil {
				total = cnt.Total
			}
		}
		countCur.Close(ctx)
	}

	pipeline = append(pipeline,
		bson.D{{Key: "$skip", Value: skip}},
		bson.D{{Key: "$limit", Value: int64(limit)}},
	)

	cur, err := r.eventsCol.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)

	var ids []string
	for cur.Next(ctx) {
		var doc struct {
			ID string `bson:"_id"`
		}
		if err := cur.Decode(&doc); err == nil && doc.ID != "" {
			ids = append(ids, doc.ID)
		}
	}
	return ids, total, nil
}
