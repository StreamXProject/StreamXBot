package repository

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"streamgo/internal/database"
	"streamgo/internal/metadata"
	"streamgo/internal/models"
)

var (
	artistSplitRegex = regexp.MustCompile(`(?i)\s*(?:,|/|&|\s+and\s+|\s+x\s+|\s+feat\.\s+|\s+feat\s+|\s+ft\.\s+|\s+ft\s+)\s*`)
)

func splitArtists(value string) []string {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return []string{}
	}
	raw = strings.ReplaceAll(raw, "(", " ")
	raw = strings.ReplaceAll(raw, ")", " ")
	raw = strings.ReplaceAll(raw, "[", " ")
	raw = strings.ReplaceAll(raw, "]", " ")

	parts := artistSplitRegex.Split(raw, -1)
	out := make([]string, 0, len(parts))
	seen := make(map[string]bool)

	for _, p := range parts {
		token := strings.Trim(strings.TrimSpace(p), " .")
		if token == "" {
			continue
		}
		key := strings.ToLower(token)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, token)
	}
	return out
}

func slugify(val string) string {
	return metadata.Slugify(val)
}

// ArtistAlbumRepository manages data access for artists and albums.
type ArtistAlbumRepository interface {
	ListArtists(ctx context.Context, page, perPage int, refresh bool) ([]*models.Artist, int64, error)
	GetArtistByID(ctx context.Context, id string) (*models.Artist, error)
	GetArtistTracks(ctx context.Context, artistName string, limit int) ([]*models.Track, error)
	GetArtistTracksPaginated(ctx context.Context, artistName string, page, perPage int) ([]*models.Track, int64, error)
	GetArtistAlbums(ctx context.Context, artistName string) ([]*models.Album, error)

	ListAlbums(ctx context.Context, page, perPage int, artistFilter string, refresh bool) ([]*models.Album, int64, error)
	GetAlbumByID(ctx context.Context, id string) (*models.Album, error)
	GetAlbumTracks(ctx context.Context, albumID string) ([]*models.Track, error)
	GetAlbumTracksPaginated(ctx context.Context, albumID string, page, perPage int) ([]*models.Track, int64, error)
	RefreshAlbumsCache(ctx context.Context, limit int) (int, error)
	RefreshArtistsCache(ctx context.Context, limitTracks, limitArtists int) (int, error)
}

type mongoArtistAlbumRepository struct {
	artistsCol      *mongo.Collection
	albumsCol       *mongo.Collection
	tracksCol       *mongo.Collection
	albumRefreshMu  sync.Mutex
	artistRefreshMu sync.Mutex
}

// NewArtistAlbumRepository creates an ArtistAlbumRepository backed by MongoDB.
func NewArtistAlbumRepository(db *database.Client) ArtistAlbumRepository {
	return &mongoArtistAlbumRepository{
		artistsCol: db.Collection("artists"),
		albumsCol:  db.Collection("albums"),
		tracksCol:  db.Collection("audioTracks"),
	}
}

func (r *mongoArtistAlbumRepository) RefreshArtistsCache(ctx context.Context, limitTracks, limitArtists int) (int, error) {
	if !r.artistRefreshMu.TryLock() {
		return 0, nil
	}
	defer r.artistRefreshMu.Unlock()

	if limitTracks <= 0 {
		limitTracks = 20000
	}
	if limitTracks > 100000 {
		limitTracks = 100000
	}
	if limitArtists <= 0 {
		limitArtists = 5000
	}
	if limitArtists > 20000 {
		limitArtists = 20000
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "updated_at", Value: -1}}).
		SetLimit(int64(limitTracks)).
		SetProjection(bson.M{
			"audio.artist":          1,
			"audio.performer":       1,
			"audio.artists":         1,
			"spotify.artist_avatar": 1,
			"updated_at":            1,
		})

	filter := bson.M{
		"deleted": bson.M{"$ne": true},
		"$or": []bson.M{
			{"audio.artist": bson.M{"$exists": true, "$ne": ""}},
			{"audio.performer": bson.M{"$exists": true, "$ne": ""}},
		},
	}

	cursor, err := r.tracksCol.Find(ctx, filter, opts)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	type artistEntry struct {
		Name        string
		MatchArtist string
		TracksCount int64
		AvatarURL   string
		UpdatedAt   float64
	}

	byKey := make(map[string]*artistEntry)
	for cursor.Next(ctx) {
		var doc struct {
			Audio struct {
				Artist    string   `bson:"artist"`
				Performer string   `bson:"performer"`
				Artists   []string `bson:"artists"`
			} `bson:"audio"`
			Spotify struct {
				ArtistAvatar string `bson:"artist_avatar"`
			} `bson:"spotify"`
			UpdatedAt float64 `bson:"updated_at"`
		}
		if err := cursor.Decode(&doc); err != nil {
			continue
		}

		var names []string
		if len(doc.Audio.Artists) > 0 {
			for _, a := range doc.Audio.Artists {
				names = append(names, splitArtists(a)...)
			}
		}
		if len(names) == 0 {
			raw := doc.Audio.Artist
			if raw == "" {
				raw = doc.Audio.Performer
			}
			if raw != "" {
				names = splitArtists(raw)
			}
		}

		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			k := strings.ToLower(name)
			if entry, exists := byKey[k]; exists {
				entry.TracksCount++
				if doc.UpdatedAt > entry.UpdatedAt {
					entry.UpdatedAt = doc.UpdatedAt
				}
				if entry.AvatarURL == "" && doc.Spotify.ArtistAvatar != "" {
					entry.AvatarURL = doc.Spotify.ArtistAvatar
				}
			} else {
				byKey[k] = &artistEntry{
					Name:        name,
					MatchArtist: k,
					TracksCount: 1,
					AvatarURL:   doc.Spotify.ArtistAvatar,
					UpdatedAt:   doc.UpdatedAt,
				}
			}
		}
	}

	// Clean up legacy documents with spaces in _id or collaborative names
	_, _ = r.artistsCol.DeleteMany(ctx, bson.M{
		"$or": []bson.M{
			{"_id": bson.M{"$regex": " "}},
			{"name": bson.M{"$regex": `(?i)/|,|&|\s+feat|\s+ft\s+|\s+featuring\s+`}},
			{"match_artist": bson.M{"$regex": `(?i)/|,|&|\s+feat|\s+ft\s+|\s+featuring\s+`}},
		},
	})

	now := float64(time.Now().Unix())
	upserted := 0
	var writeModels []mongo.WriteModel
	var validAIDs []string

	for _, entry := range byKey {
		aid := metadata.ArtistID(entry.Name)
		if aid == "" {
			continue
		}
		validAIDs = append(validAIDs, aid)
		updateDoc := bson.M{
			"name":         entry.Name,
			"match_artist": entry.MatchArtist,
			"tracks_count": entry.TracksCount,
			"updated_at":   entry.UpdatedAt,
		}
		if entry.AvatarURL != "" {
			updateDoc["cover_url"] = entry.AvatarURL
		}
		if entry.UpdatedAt == 0 {
			updateDoc["updated_at"] = now
		}

		model := mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": aid}).
			SetUpdate(bson.M{
				"$setOnInsert": bson.M{"created_at": now, "followers": 0},
				"$set":         updateDoc,
			}).
			SetUpsert(true)

		writeModels = append(writeModels, model)
		if len(writeModels) >= 500 {
			_, _ = r.artistsCol.BulkWrite(ctx, writeModels, options.BulkWrite().SetOrdered(false))
			upserted += len(writeModels)
			writeModels = writeModels[:0]
		}
		if upserted >= limitArtists {
			break
		}
	}

	if len(writeModels) > 0 {
		_, _ = r.artistsCol.BulkWrite(ctx, writeModels, options.BulkWrite().SetOrdered(false))
		upserted += len(writeModels)
	}

	// Purge orphan artists whose canonical IDs are no longer in active scan
	if len(validAIDs) > 0 {
		_, _ = r.artistsCol.DeleteMany(ctx, bson.M{
			"_id": bson.M{"$nin": validAIDs},
		})
	}

	return upserted, nil
}

func (r *mongoArtistAlbumRepository) ListArtists(ctx context.Context, page, perPage int, refresh bool) ([]*models.Artist, int64, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}
	if perPage > 200 {
		perPage = 200
	}

	total, err := r.artistsCol.EstimatedDocumentCount(ctx)
	if err != nil {
		total, _ = r.artistsCol.CountDocuments(ctx, bson.M{})
	}

	// Trigger asynchronous background cache refresh if completely empty or explicitly asked
	if refresh || total <= 0 {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			_, _ = r.RefreshArtistsCache(bgCtx, 100000, 20000)
		}()
	}

	filter := bson.M{}
	skip := int64((page - 1) * perPage)
	opts := options.Find().
		SetSort(bson.D{
			{Key: "tracks_count", Value: -1},
			{Key: "updated_at", Value: -1},
		}).
		SetSkip(skip).
		SetLimit(int64(perPage))

	cursor, err := r.artistsCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list artists: %w", err)
	}
	defer cursor.Close(ctx)

	var artists []*models.Artist
	if err := cursor.All(ctx, &artists); err != nil {
		return nil, 0, fmt.Errorf("failed to decode artists: %w", err)
	}

	var uniqueArtists []*models.Artist
	seenNames := make(map[string]bool)
	seenIDs := make(map[string]bool)

	for _, a := range artists {
		if a.Id == "" {
			a.Id = a.ID
		}
		if a.AvatarURL == "" {
			a.AvatarURL = a.CoverURL
		}
		if a.TrackCount == 0 {
			a.TrackCount = a.TracksCount
		}
		if a.TracksCount <= 0 {
			continue
		}
		nameNorm := strings.ToLower(strings.TrimSpace(a.Name))
		if nameNorm == "" || seenNames[nameNorm] || seenIDs[a.Id] {
			continue
		}
		seenNames[nameNorm] = true
		seenIDs[a.Id] = true
		uniqueArtists = append(uniqueArtists, a)
	}

	return uniqueArtists, total, nil
}

func (r *mongoArtistAlbumRepository) GetArtistByID(ctx context.Context, id string) (*models.Artist, error) {
	id = strings.TrimSpace(id)
	filter := bson.M{"$or": []bson.M{{"_id": id}, {"match_artist": strings.ToLower(id)}}}

	var artist models.Artist
	err := r.artistsCol.FindOne(ctx, filter).Decode(&artist)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to fetch artist %s: %w", id, err)
	}

	if artist.Id == "" {
		artist.Id = artist.ID
	}
	if artist.AvatarURL == "" {
		artist.AvatarURL = artist.CoverURL
	}
	if artist.TrackCount == 0 {
		artist.TrackCount = artist.TracksCount
	}

	return &artist, nil
}

func (r *mongoArtistAlbumRepository) GetArtistTracks(ctx context.Context, artistName string, limit int) ([]*models.Track, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	escaped := regexp.QuoteMeta(artistName)
	regex := bson.M{"$regex": "^" + escaped + "$", "$options": "i"}

	filter := bson.M{
		"deleted": bson.M{"$ne": true},
		"$or": []bson.M{
			{"audio.artist": regex},
			{"audio.artists": regex},
			{"audio.performer": regex},
		},
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "play_count", Value: -1}, {Key: "updated_at", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := r.tracksCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var tracks []*models.Track
	if err := cursor.All(ctx, &tracks); err != nil {
		return nil, err
	}
	return tracks, nil
}

func (r *mongoArtistAlbumRepository) GetArtistTracksPaginated(ctx context.Context, artistName string, page, perPage int) ([]*models.Track, int64, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}
	if perPage > 200 {
		perPage = 200
	}

	escaped := regexp.QuoteMeta(artistName)
	regex := bson.M{"$regex": "^" + escaped + "$", "$options": "i"}

	filter := bson.M{
		"deleted": bson.M{"$ne": true},
		"$or": []bson.M{
			{"audio.artist": regex},
			{"audio.artists": regex},
			{"audio.performer": regex},
		},
	}

	total, err := r.tracksCol.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	skip := int64((page - 1) * perPage)
	opts := options.Find().
		SetSort(bson.D{{Key: "play_count", Value: -1}, {Key: "updated_at", Value: -1}}).
		SetSkip(skip).
		SetLimit(int64(perPage))

	cursor, err := r.tracksCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var tracks []*models.Track
	if err := cursor.All(ctx, &tracks); err != nil {
		return nil, 0, err
	}
	return tracks, total, nil
}

func (r *mongoArtistAlbumRepository) GetArtistAlbums(ctx context.Context, artistName string) ([]*models.Album, error) {
	escaped := regexp.QuoteMeta(artistName)
	regex := bson.M{"$regex": "^" + escaped + "$", "$options": "i"}

	filter := bson.M{
		"$or": []bson.M{
			{"artist": regex},
			{"artists": regex},
			{"match_artist": strings.ToLower(artistName)},
		},
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "updated_at", Value: -1}}).
		SetLimit(50)

	cursor, err := r.albumsCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var albums []*models.Album
	if err := cursor.All(ctx, &albums); err != nil {
		return nil, err
	}

	for _, a := range albums {
		if a.Id == "" {
			a.Id = a.ID
		}
		if a.TrackCount == 0 {
			a.TrackCount = a.TracksCount
		}
	}

	var uniqueAlbums []*models.Album
	seenTitles := make(map[string]bool)
	seenIDs := make(map[string]bool)
	for _, a := range albums {
		if a.TracksCount <= 0 {
			continue
		}
		titleNorm := strings.ToLower(strings.TrimSpace(a.Title))
		if titleNorm == "" || seenTitles[titleNorm] || seenIDs[a.Id] {
			continue
		}
		seenTitles[titleNorm] = true
		seenIDs[a.Id] = true
		uniqueAlbums = append(uniqueAlbums, a)
	}

	return uniqueAlbums, nil
}

func (r *mongoArtistAlbumRepository) RefreshAlbumsCache(ctx context.Context, limit int) (int, error) {
	r.albumRefreshMu.Lock()
	defer r.albumRefreshMu.Unlock()

	if limit <= 0 {
		limit = 2000
	}
	if limit > 5000 {
		limit = 5000
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"deleted":        bson.M{"$ne": true},
			"audio.album_id": bson.M{"$exists": true, "$ne": ""},
			"$or": []bson.M{
				{"audio.album": bson.M{"$exists": true, "$ne": ""}},
				{"audio.title": bson.M{"$exists": true, "$ne": ""}},
			},
		}}},
		{{Key: "$addFields", Value: bson.M{
			"_aid":         "$audio.album_id",
			"_album_title": bson.M{"$ifNull": []any{"$audio.album", "$audio.title"}},
			"_album_artist": bson.M{
				"$cond": []any{
					bson.M{"$and": []any{
						bson.M{"$isArray": "$audio.artists"},
						bson.M{"$gt": []any{bson.M{"$size": "$audio.artists"}, 0}},
					}},
					bson.M{"$arrayElemAt": []any{"$audio.artists", 0}},
					bson.M{"$ifNull": []any{"$audio.artist", "$audio.performer"}},
				},
			},
			"_year": "$audio.year",
		}}},
		{{Key: "$addFields", Value: bson.M{
			"_album_norm":  bson.M{"$toLower": bson.M{"$trim": bson.M{"input": "$_album_title"}}},
			"_artist_norm": bson.M{"$toLower": bson.M{"$trim": bson.M{"input": "$_album_artist"}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "updated_at", Value: -1}}}},
		{{Key: "$group", Value: bson.M{
			"_id":            "$_aid",
			"title":          bson.M{"$first": "$_album_title"},
			"artist":         bson.M{"$first": "$_album_artist"},
			"cover_url":      bson.M{"$first": bson.M{"$ifNull": []any{"$spotify.cloudflare_big_cover_url", "$spotify.cloudflare_cover_url", "$spotify.big_cover_url", "$spotify.cover_url"}}},
			"year":           bson.M{"$first": "$_year"},
			"tracks_count":   bson.M{"$sum": 1},
			"duration_total": bson.M{"$sum": bson.M{"$ifNull": []any{"$audio.duration_sec", 0}}},
			"updated_at":     bson.M{"$max": "$updated_at"},
			"match_album":    bson.M{"$first": "$_album_norm"},
			"match_artist":   bson.M{"$first": "$_artist_norm"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "updated_at", Value: -1}}}},
		{{Key: "$limit", Value: limit}},
	}

	cursor, err := r.tracksCol.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	now := float64(time.Now().Unix())
	upserted := 0
	var writeModels []mongo.WriteModel

	for cursor.Next(ctx) {
		var row struct {
			ID            string  `bson:"_id"`
			Title         string  `bson:"title"`
			Artist        string  `bson:"artist"`
			CoverURL      string  `bson:"cover_url"`
			Year          int     `bson:"year"`
			TracksCount   int64   `bson:"tracks_count"`
			DurationTotal float64 `bson:"duration_total"`
			UpdatedAt     float64 `bson:"updated_at"`
			MatchAlbum    string  `bson:"match_album"`
			MatchArtist   string  `bson:"match_artist"`
		}
		if err := cursor.Decode(&row); err != nil {
			continue
		}

		aid := strings.TrimSpace(row.ID)
		title := strings.TrimSpace(row.Title)
		matchAlbum := strings.TrimSpace(row.MatchAlbum)
		if aid == "" || title == "" || matchAlbum == "" {
			continue
		}

		artist := strings.TrimSpace(row.Artist)
		var artists []string
		if artist != "" {
			artists = splitArtists(artist)
		}

		updateDoc := bson.M{
			"title":          title,
			"artist":         artist,
			"cover_url":      row.CoverURL,
			"tracks_count":   row.TracksCount,
			"duration_total": row.DurationTotal,
			"match_album":    matchAlbum,
			"match_artist":   strings.TrimSpace(row.MatchArtist),
			"updated_at":     row.UpdatedAt,
		}
		if len(artists) > 0 {
			updateDoc["artists"] = artists
		}
		if row.Year > 0 {
			updateDoc["year"] = row.Year
		}
		if row.UpdatedAt == 0 {
			updateDoc["updated_at"] = now
		}

		model := mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": aid}).
			SetUpdate(bson.M{
				"$setOnInsert": bson.M{"created_at": now},
				"$set":         updateDoc,
			}).
			SetUpsert(true)

		writeModels = append(writeModels, model)
		if len(writeModels) >= 500 {
			_, _ = r.albumsCol.BulkWrite(ctx, writeModels, options.BulkWrite().SetOrdered(false))
			upserted += len(writeModels)
			writeModels = writeModels[:0]
		}
	}

	if len(writeModels) > 0 {
		_, _ = r.albumsCol.BulkWrite(ctx, writeModels, options.BulkWrite().SetOrdered(false))
		upserted += len(writeModels)
	}

	// Purge orphan albums that no longer have any active tracks in audioTracks
	distinctRes := r.tracksCol.Distinct(ctx, "audio.album_id", bson.M{
		"deleted":        bson.M{"$ne": true},
		"audio.album_id": bson.M{"$exists": true, "$ne": ""},
	})
	if distinctRes.Err() == nil {
		var distinctAIDs []string
		if err := distinctRes.Decode(&distinctAIDs); err == nil && len(distinctAIDs) > 0 {
			_, _ = r.albumsCol.DeleteMany(ctx, bson.M{
				"_id": bson.M{"$nin": distinctAIDs},
			})
		}
	}

	return upserted, nil
}

func (r *mongoArtistAlbumRepository) ListAlbums(ctx context.Context, page, perPage int, artistFilter string, refresh bool) ([]*models.Album, int64, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}
	if perPage > 200 {
		perPage = 200
	}

	filter := bson.M{}
	if artistFilter != "" {
		escaped := regexp.QuoteMeta(artistFilter)
		filter["$or"] = []bson.M{
			{"artist": bson.M{"$regex": escaped, "$options": "i"}},
			{"artists": bson.M{"$regex": escaped, "$options": "i"}},
			{"match_artist": strings.ToLower(artistFilter)},
		}
	}

	var total int64
	var err error
	if len(filter) == 0 {
		total, err = r.albumsCol.EstimatedDocumentCount(ctx)
	}
	if total <= 0 || len(filter) > 0 {
		total, err = r.albumsCol.CountDocuments(ctx, filter)
	}

	// Trigger asynchronous background cache refresh if completely empty or explicitly asked
	if (refresh || total <= 0) && artistFilter == "" {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			_, _ = r.RefreshAlbumsCache(bgCtx, 5000)
		}()
	}

	skip := int64((page - 1) * perPage)
	opts := options.Find().
		SetSort(bson.D{{Key: "updated_at", Value: -1}}).
		SetSkip(skip).
		SetLimit(int64(perPage))

	cursor, err := r.albumsCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var albums []*models.Album
	if err := cursor.All(ctx, &albums); err != nil {
		return nil, 0, err
	}

	var uniqueAlbums []*models.Album
	seenKeys := make(map[string]bool)
	seenIDs := make(map[string]bool)

	for _, a := range albums {
		if a.Id == "" {
			a.Id = a.ID
		}
		if a.TrackCount == 0 {
			a.TrackCount = a.TracksCount
		}
		if a.TracksCount <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(a.Title)) + "::" + strings.ToLower(strings.TrimSpace(a.Artist))
		if seenKeys[key] || seenIDs[a.Id] {
			continue
		}
		seenKeys[key] = true
		seenIDs[a.Id] = true
		uniqueAlbums = append(uniqueAlbums, a)
	}

	return uniqueAlbums, total, nil
}

func (r *mongoArtistAlbumRepository) GetAlbumByID(ctx context.Context, id string) (*models.Album, error) {
	id = strings.TrimSpace(id)
	var album models.Album
	err := r.albumsCol.FindOne(ctx, bson.M{"_id": id}).Decode(&album)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			// Find track from this album to synthesize album doc instantly without full aggregation
			var t models.Track
			tErr := r.tracksCol.FindOne(ctx, bson.M{"audio.album_id": id, "deleted": bson.M{"$ne": true}}).Decode(&t)
			if tErr == nil && t.Audio.Album != "" {
				album = models.Album{
					ID:         id,
					Title:      t.Audio.Album,
					Artist:     t.Audio.Artist,
					Artists:    t.Audio.Artists,
					CoverURL:   t.EffectiveCoverURL(),
					MatchAlbum: strings.ToLower(t.Audio.Album),
				}
				return &album, nil
			}
			return nil, nil
		}
		return nil, err
	}

	if album.Id == "" {
		album.Id = album.ID
	}
	if album.TrackCount == 0 {
		album.TrackCount = album.TracksCount
	}

	return &album, nil
}

func (r *mongoArtistAlbumRepository) GetAlbumTracks(ctx context.Context, albumID string) ([]*models.Track, error) {
	filter := bson.M{
		"deleted":        bson.M{"$ne": true},
		"audio.album_id": albumID,
	}

	opts := options.Find().
		SetSort(bson.D{
			{Key: "audio.track_number", Value: 1},
			{Key: "source_message_id", Value: 1},
			{Key: "updated_at", Value: -1},
		}).
		SetLimit(200)

	cursor, err := r.tracksCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var tracks []*models.Track
	if err := cursor.All(ctx, &tracks); err != nil {
		return nil, err
	}
	return tracks, nil
}

func (r *mongoArtistAlbumRepository) GetAlbumTracksPaginated(ctx context.Context, albumID string, page, perPage int) ([]*models.Track, int64, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}
	if perPage > 200 {
		perPage = 200
	}

	filter := bson.M{
		"deleted":        bson.M{"$ne": true},
		"audio.album_id": albumID,
	}

	total, err := r.tracksCol.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	skip := int64((page - 1) * perPage)
	opts := options.Find().
		SetSort(bson.D{
			{Key: "audio.track_number", Value: 1},
			{Key: "source_message_id", Value: 1},
			{Key: "updated_at", Value: -1},
		}).
		SetSkip(skip).
		SetLimit(int64(perPage))

	cursor, err := r.tracksCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var tracks []*models.Track
	if err := cursor.All(ctx, &tracks); err != nil {
		return nil, 0, err
	}
	return tracks, total, nil
}
