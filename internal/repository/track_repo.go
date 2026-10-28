package repository

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"streamgo/internal/database"
	"streamgo/internal/models"
)

// TrackRepository defines the data access contract for tracks and related metadata.
type TrackRepository interface {
	GetByID(ctx context.Context, id string) (*models.Track, error)
	GetByIDs(ctx context.Context, ids []string) ([]*models.Track, error)
	List(ctx context.Context, page, perPage int, sortField, topicName string, channelID int64) ([]*models.Track, int64, error)
	Search(ctx context.Context, query string, limit int) ([]*models.Track, error)
	Random(ctx context.Context, limit int, channelID int64) ([]*models.Track, error)
	GetTopics(ctx context.Context, channelID int64, limit int) ([]*models.TopicItem, error)
	GetChannelIDs(ctx context.Context) ([]int64, error)
	IncrementPlayCount(ctx context.Context, id string) error
	UpdateWorkerFileID(ctx context.Context, trackID, workerID, fileID string) error
	UpdateLyricsCache(ctx context.Context, id string, text, kind, source string, telegraphURL string) error
	SoftDelete(ctx context.Context, id string) error
	HardDelete(ctx context.Context, id string) error
	CountArtworkReferences(ctx context.Context, excludeTrackID, coverURL string) (int64, error)
}


type mongoTrackRepository struct {
	db  *database.Client
	col *mongo.Collection
}

// NewTrackRepository creates a MongoDB-backed TrackRepository.
func NewTrackRepository(db *database.Client) TrackRepository {
	return &mongoTrackRepository{
		db:  db,
		col: db.Collection("audioTracks"),
	}
}

func (r *mongoTrackRepository) GetByID(ctx context.Context, id string) (*models.Track, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("track id is required")
	}

	filter := bson.M{
		"_id":     id,
		"deleted": bson.M{"$ne": true},
	}

	var track models.Track
	err := r.col.FindOne(ctx, filter).Decode(&track)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to fetch track %s: %w", id, err)
	}

	return &track, nil
}

func (r *mongoTrackRepository) GetByIDs(ctx context.Context, ids []string) ([]*models.Track, error) {
	if len(ids) == 0 {
		return []*models.Track{}, nil
	}

	filter := bson.M{
		"_id":     bson.M{"$in": ids},
		"deleted": bson.M{"$ne": true},
	}

	cursor, err := r.col.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to query tracks by IDs: %w", err)
	}
	defer cursor.Close(ctx)

	var tracks []*models.Track
	if err := cursor.All(ctx, &tracks); err != nil {
		return nil, fmt.Errorf("failed to decode tracks: %w", err)
	}

	// Preserve the requested order of IDs
	trackMap := make(map[string]*models.Track, len(tracks))
	for _, t := range tracks {
		trackMap[t.ID] = t
	}

	ordered := make([]*models.Track, 0, len(ids))
	for _, id := range ids {
		if t, ok := trackMap[id]; ok {
			ordered = append(ordered, t)
		}
	}

	return ordered, nil
}


func (r *mongoTrackRepository) List(
	ctx context.Context,
	page, perPage int,
	sortField, topicName string,
	channelID int64,
) ([]*models.Track, int64, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	filter := bson.M{"deleted": bson.M{"$ne": true}}

	if topicName != "" {
		filter["topic_name"] = topicName
	}
	if channelID != 0 {
		filter["source_chat_id"] = channelID
	}

	total, err := r.col.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count tracks: %w", err)
	}

	// Sort specification matching Python source (Api/services/track_service.py line 167)
	var sortDoc bson.D
	sField := strings.ToLower(strings.TrimSpace(sortField))
	switch sField {
	case "play_count", "plays", "popular":
		sortDoc = bson.D{
			{Key: "play_count", Value: -1},
			{Key: "created_at", Value: -1},
			{Key: "_id", Value: -1},
		}
	case "oldest":
		sortDoc = bson.D{
			{Key: "created_at", Value: 1},
			{Key: "source_message_id", Value: 1},
			{Key: "_id", Value: 1},
		}
	case "title", "title_asc", "name":
		sortDoc = bson.D{
			{Key: "audio.title", Value: 1},
			{Key: "created_at", Value: -1},
			{Key: "_id", Value: -1},
		}
	case "title_desc":
		sortDoc = bson.D{
			{Key: "audio.title", Value: -1},
			{Key: "created_at", Value: -1},
			{Key: "_id", Value: -1},
		}
	case "artist", "artist_asc":
		sortDoc = bson.D{
			{Key: "audio.artist", Value: 1},
			{Key: "audio.title", Value: 1},
			{Key: "_id", Value: -1},
		}
	case "updated", "updated_at":
		sortDoc = bson.D{
			{Key: "updated_at", Value: -1},
			{Key: "_id", Value: -1},
		}
	case "recent", "recently_added", "newest", "latest", "created_at":
		sortDoc = bson.D{
			{Key: "created_at", Value: -1},
			{Key: "source_message_id", Value: -1},
			{Key: "_id", Value: -1},
		}
	default:
		// Default matches Python: [("source_message_id", -1)] if channel_id is not None else [("created_at", -1), ("source_message_id", -1), ("_id", -1)]
		if channelID != 0 {
			sortDoc = bson.D{
				{Key: "source_message_id", Value: -1},
				{Key: "created_at", Value: -1},
				{Key: "_id", Value: -1},
			}
		} else {
			sortDoc = bson.D{
				{Key: "created_at", Value: -1},
				{Key: "source_message_id", Value: -1},
				{Key: "_id", Value: -1},
			}
		}
	}

	skip := int64((page - 1) * perPage)
	opts := options.Find().
		SetSort(sortDoc).
		SetSkip(skip).
		SetLimit(int64(perPage))

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list tracks: %w", err)
	}
	defer cursor.Close(ctx)

	var tracks []*models.Track
	if err := cursor.All(ctx, &tracks); err != nil {
		return nil, 0, fmt.Errorf("failed to decode tracks: %w", err)
	}

	return tracks, total, nil
}

func (r *mongoTrackRepository) Search(ctx context.Context, query string, limit int) ([]*models.Track, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []*models.Track{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	escaped := regexp.QuoteMeta(query)
	regexPattern := bson.M{"$regex": escaped, "$options": "i"}

	filter := bson.M{
		"deleted": bson.M{"$ne": true},
		"$or": []bson.M{
			{"audio.title": regexPattern},
			{"audio.artist": regexPattern},
			{"audio.performer": regexPattern},
			{"audio.album": regexPattern},
			{"titles.romanized": regexPattern},
			{"titles.original": regexPattern},
			{"titles.translations.en": regexPattern},
			{"audio.titles.romanized": regexPattern},
			{"audio.titles.original": regexPattern},
		},
	}


	opts := options.Find().
		SetSort(bson.D{{Key: "play_count", Value: -1}, {Key: "updated_at", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer cursor.Close(ctx)

	var tracks []*models.Track
	if err := cursor.All(ctx, &tracks); err != nil {
		return nil, fmt.Errorf("failed to decode search results: %w", err)
	}

	return tracks, nil
}

func (r *mongoTrackRepository) Random(ctx context.Context, limit int, channelID int64) ([]*models.Track, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	match := bson.M{"deleted": bson.M{"$ne": true}}
	if channelID != 0 {
		match["source_chat_id"] = channelID
	}

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: match}},
		bson.D{{Key: "$sample", Value: bson.M{"size": limit}}},
	}

	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("random aggregation failed: %w", err)
	}
	defer cursor.Close(ctx)

	var tracks []*models.Track
	if err := cursor.All(ctx, &tracks); err != nil {
		return nil, fmt.Errorf("failed to decode random tracks: %w", err)
	}

	return tracks, nil
}

func (r *mongoTrackRepository) GetTopics(ctx context.Context, channelID int64, limit int) ([]*models.TopicItem, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	matchFilter := bson.M{
		"deleted":    bson.M{"$ne": true},
		"topic_name": bson.M{"$exists": true, "$nin": []interface{}{"", "null", "None", nil}},
	}
	if channelID != 0 {
		matchFilter["source_chat_id"] = channelID
	}

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: matchFilter}},
		bson.D{{Key: "$sort", Value: bson.D{
			{Key: "updated_at", Value: -1},
			{Key: "source_message_id", Value: -1},
		}}},
		bson.D{{Key: "$group", Value: bson.M{
			"_id":            "$topic_name",
			"topic_id":       bson.M{"$first": "$topic_id"},
			"source_chat_id": bson.M{"$first": "$source_chat_id"},
			"cover_url":      bson.M{"$first": bson.M{"$ifNull": []any{"$spotify.cloudflare_cover_url", "$spotify.cover_url"}}},
			"big_cover_url":  bson.M{"$first": bson.M{"$ifNull": []any{"$spotify.cloudflare_big_cover_url", "$spotify.big_cover_url", "$spotify.cover_url"}}},
			"raw_thumbnails": bson.M{"$push": bson.M{"$ifNull": []any{"$spotify.cloudflare_cover_url", "$spotify.cover_url"}}},
			"tracks_count":   bson.M{"$sum": 1},
		}}},
		bson.D{{Key: "$project", Value: bson.M{
			"topic_id":       1,
			"source_chat_id": 1,
			"cover_url":      1,
			"big_cover_url":  1,
			"thumbnails":     bson.M{"$slice": []interface{}{"$raw_thumbnails", 8}},
			"tracks_count":   1,
		}}},
		bson.D{{Key: "$sort", Value: bson.M{"tracks_count": -1, "_id": 1}}},
		bson.D{{Key: "$limit", Value: limit}},
	}

	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("topics aggregation failed: %w", err)
	}
	defer cursor.Close(ctx)

	var results []struct {
		ID           string   `bson:"_id"`
		TopicID      *int64   `bson:"topic_id"`
		SourceChatID *int64   `bson:"source_chat_id"`
		TracksCount  int64    `bson:"tracks_count"`
		CoverURL     string   `bson:"cover_url"`
		BigCoverURL  string   `bson:"big_cover_url"`
		Thumbnails   []string `bson:"thumbnails"`
	}

	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode topics: %w", err)
	}

	topics := make([]*models.TopicItem, 0, len(results))
	for _, res := range results {
		name := strings.TrimSpace(res.ID)
		if name == "" {
			continue
		}

		cover := strings.TrimSpace(res.BigCoverURL)
		if cover == "" {
			cover = strings.TrimSpace(res.CoverURL)
		}

		uniqueThumbs := make([]string, 0, 4)
		seen := make(map[string]struct{})
		for _, th := range res.Thumbnails {
			th = strings.TrimSpace(th)
			if th != "" {
				if _, ok := seen[th]; !ok {
					seen[th] = struct{}{}
					uniqueThumbs = append(uniqueThumbs, th)
					if len(uniqueThumbs) >= 4 {
						break
					}
				}
			}
		}

		if cover == "" && len(uniqueThumbs) > 0 {
			cover = uniqueThumbs[0]
		}

		endpoint := fmt.Sprintf("/topics/%s/tracks", url.PathEscape(name))

		topics = append(topics, &models.TopicItem{
			Name:            name,
			TopicName:       name,
			TopicID:         res.TopicID,
			Count:           res.TracksCount,
			TracksCount:     res.TracksCount,
			CoverURL:        cover,
			ThumbnailURL:    cover,
			NormalThumbnail: cover,
			Thumbnails:      uniqueThumbs,
			SourceChatID:    res.SourceChatID,
			Endpoint:        endpoint,
		})
	}

	return topics, nil
}

func (r *mongoTrackRepository) GetChannelIDs(ctx context.Context) ([]int64, error) {
	filter := bson.M{
		"deleted":        bson.M{"$ne": true},
		"source_chat_id": bson.M{"$exists": true, "$ne": 0},
	}

	distinctRes := r.col.Distinct(ctx, "source_chat_id", filter)
	if err := distinctRes.Err(); err != nil {
		return nil, fmt.Errorf("failed to get channel ids: %w", err)
	}

	var rawValues []interface{}
	if err := distinctRes.Decode(&rawValues); err != nil {
		return nil, fmt.Errorf("failed to decode channel ids: %w", err)
	}

	var channelIDs []int64
	for _, v := range rawValues {
		switch n := v.(type) {
		case int64:
			channelIDs = append(channelIDs, n)
		case int32:
			channelIDs = append(channelIDs, int64(n))
		case int:
			channelIDs = append(channelIDs, int64(n))
		case float64:
			channelIDs = append(channelIDs, int64(n))
		}
	}

	return channelIDs, nil
}

func (r *mongoTrackRepository) IncrementPlayCount(ctx context.Context, id string) error {
	filter := bson.M{"_id": id}
	update := bson.M{
		"$inc": bson.M{"play_count": 1},
	}
	_, err := r.col.UpdateOne(ctx, filter, update)
	return err
}

func (r *mongoTrackRepository) UpdateWorkerFileID(ctx context.Context, trackID, workerID, fileID string) error {
	filter := bson.M{"_id": trackID}
	update := bson.M{
		"$set": bson.M{
			"telegram.file_ids." + workerID: fileID,
			"updated_at":                    float64(time.Now().Unix()),
		},
	}
	_, err := r.col.UpdateOne(ctx, filter, update)
	return err
}

func (r *mongoTrackRepository) UpdateLyricsCache(ctx context.Context, id string, text, kind, source string, telegraphURL string) error {
	filter := bson.M{"_id": id}
	set := bson.M{
		"lyrics_cache.text":       text,
		"lyrics_cache.kind":       kind,
		"lyrics_cache.source":     source,
		"lyrics_cache.updated_at": float64(time.Now().Unix()),
		"updated_at":              float64(time.Now().Unix()),
	}
	if telegraphURL != "" {
		set["lyrics"] = telegraphURL
	}
	_, err := r.col.UpdateOne(ctx, filter, bson.M{"$set": set})
	return err
}

func (r *mongoTrackRepository) SoftDelete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("track id is required")
	}

	filter := bson.M{
		"_id":     id,
		"deleted": bson.M{"$ne": true},
	}
	update := bson.M{
		"$set": bson.M{
			"deleted":    true,
			"deleted_at": float64(time.Now().Unix()),
			"updated_at": float64(time.Now().Unix()),
		},
	}
	res, err := r.col.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return errors.New("track not found")
	}
	return nil
}

func (r *mongoTrackRepository) HardDelete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("track id is required")
	}

	// 1. Fetch track first if possible to find any alternate IDs (file_unique_id, telegram.file_unique_id)
	targetIDsMap := map[string]struct{}{id: {}}
	var track models.Track
	if err := r.col.FindOne(ctx, bson.M{"_id": id}).Decode(&track); err == nil {
		if strings.TrimSpace(track.ID) != "" {
			targetIDsMap[strings.TrimSpace(track.ID)] = struct{}{}
		}
		if strings.TrimSpace(track.Telegram.FileUniqueID) != "" {
			targetIDsMap[strings.TrimSpace(track.Telegram.FileUniqueID)] = struct{}{}
		}
	}

	var targetIDs []string
	for tid := range targetIDsMap {
		targetIDs = append(targetIDs, tid)
	}

	// 2. Cascade delete from audioTracks
	res, err := r.col.DeleteMany(ctx, bson.M{
		"$or": []bson.M{
			{"_id": bson.M{"$in": targetIDs}},
			{"file_unique_id": bson.M{"$in": targetIDs}},
			{"telegram.file_unique_id": bson.M{"$in": targetIDs}},
		},
	})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return errors.New("track not found")
	}

	// 3. Cascade delete from user_favourites
	_, _ = r.db.Collection("user_favourites").DeleteMany(ctx, bson.M{
		"$or": []bson.M{
			{"track_id": bson.M{"$in": targetIDs}},
			{"file_unique_id": bson.M{"$in": targetIDs}},
		},
	})

	// 4. Cascade delete from playlist_tracks
	_, _ = r.db.Collection("playlist_tracks").DeleteMany(ctx, bson.M{
		"$or": []bson.M{
			{"track_id": bson.M{"$in": targetIDs}},
			{"file_unique_id": bson.M{"$in": targetIDs}},
		},
	})

	// 5. Cascade delete from userHistory
	_, _ = r.db.Collection("userHistory").DeleteMany(ctx, bson.M{
		"$or": []bson.M{
			{"track_id": bson.M{"$in": targetIDs}},
			{"file_unique_id": bson.M{"$in": targetIDs}},
		},
	})

	// 6. Cascade delete from userPlayback
	var pbFilters []bson.M
	for _, tid := range targetIDs {
		pbFilters = append(pbFilters,
			bson.M{"track_id": tid},
			bson.M{"file_unique_id": tid},
			bson.M{"_id": bson.M{"$regex": ":" + regexp.QuoteMeta(tid) + ":"}},
		)
	}
	_, _ = r.db.Collection("userPlayback").DeleteMany(ctx, bson.M{"$or": pbFilters})

	// 7. Cascade delete from globalPlayback
	_, _ = r.db.Collection("globalPlayback").DeleteMany(ctx, bson.M{
		"$or": []bson.M{
			{"_id": bson.M{"$in": targetIDs}},
			{"track_id": bson.M{"$in": targetIDs}},
		},
	})

	// 8. Cascade delete from listening_events
	_, _ = r.db.Collection("listening_events").DeleteMany(ctx, bson.M{
		"$or": []bson.M{
			{"track_id": bson.M{"$in": targetIDs}},
			{"file_unique_id": bson.M{"$in": targetIDs}},
		},
	})

	// 9. Cascade clean daily_playlists by pulling all references
	_, _ = r.db.Collection("daily_playlists").UpdateMany(ctx,
		bson.M{"track_ids": bson.M{"$in": targetIDs}},
		bson.M{"$pull": bson.M{"track_ids": bson.M{"$in": targetIDs}}},
	)

	return nil
}

func (r *mongoTrackRepository) CountArtworkReferences(ctx context.Context, excludeTrackID, coverURL string) (int64, error) {
	coverURL = strings.TrimSpace(coverURL)
	if coverURL == "" {
		return 0, nil
	}

	filter := bson.M{
		"_id":     bson.M{"$ne": excludeTrackID},
		"deleted": bson.M{"$ne": true},
		"$or": []bson.M{
			{"spotify.cloudflare_cover_url": coverURL},
			{"spotify.cloudflare_big_cover_url": coverURL},
		},
	}
	return r.col.CountDocuments(ctx, filter)
}
