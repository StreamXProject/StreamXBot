package repository

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"streamgo/internal/database"
	"streamgo/internal/models"
)

const RecapSchemaVersion = 1

// RecapRepository defines MongoDB persistence operations for recaps and telemetry.
type RecapRepository interface {
	RecordEvents(ctx context.Context, userID int64, events []models.ListeningEventItem) (int, error)
	LoadEvents(ctx context.Context, userID int64, start float64, end float64) ([]*models.ListeningEventDoc, error)
	FirstEventTimestamp(ctx context.Context, userID int64) (float64, bool, error)
	HasPlays(ctx context.Context, userID int64, start float64, end float64) (bool, error)
	FirstSeenBefore(ctx context.Context, userID int64, trackIDs []string, before float64) (map[string]bool, error)
	GetSnapshot(ctx context.Context, userID int64, ptype models.RecapPeriodType, period string, tz int) (*models.RecapSnapshot, float64, error)
	SaveSnapshot(ctx context.Context, userID int64, ptype models.RecapPeriodType, period string, tz int, snap *models.RecapSnapshot) error
	CreateShare(ctx context.Context, userID int64, ptype models.RecapPeriodType, period string, summary *models.RecapPublicSummary) (*models.RecapShareItem, error)
	ListShares(ctx context.Context, userID int64) ([]*models.RecapShareItem, error)
	RevokeShare(ctx context.Context, userID int64, token string) (bool, error)
	GetPublicShare(ctx context.Context, token string) (*models.RecapPublicSummary, error)
	DeleteUserData(ctx context.Context, userID int64) (eventsDeleted int64, snapsDeleted int64, sharesDeleted int64, err error)
}

type mongoRecapRepository struct {
	eventsCol    *mongo.Collection
	historyCol   *mongo.Collection
	snapshotsCol *mongo.Collection
	sharesCol    *mongo.Collection
	tracksCol    *mongo.Collection
	userPbCol    *mongo.Collection
	globPbCol    *mongo.Collection
}

// NewRecapRepository creates a new RecapRepository instance.
func NewRecapRepository(db *database.Client) RecapRepository {
	return &mongoRecapRepository{
		eventsCol:    db.Collection("listening_events"),
		historyCol:   db.Collection("userHistory"),
		snapshotsCol: db.Collection("recap_snapshots"),
		sharesCol:    db.Collection("recap_shares"),
		tracksCol:    db.Collection("audioTracks"),
		userPbCol:    db.Collection("userPlayback"),
		globPbCol:    db.Collection("globalPlayback"),
	}
}

// RecordEvents saves a batch of client listening events idempotently with key user_id:event_id.
func (r *mongoRecapRepository) RecordEvents(ctx context.Context, userID int64, events []models.ListeningEventItem) (int, error) {
	now := float64(time.Now().Unix())
	stored := 0

	for _, ev := range events {
		eid := strings.TrimSpace(ev.ID)
		tid := strings.TrimSpace(ev.TrackID)
		if eid == "" || tid == "" {
			continue
		}

		docID := fmt.Sprintf("%d:%s", userID, eid)
		playedAt := ev.PlayedAt
		if playedAt <= 0 {
			playedAt = now
		}
		startedAt := ev.StartedAt
		if startedAt <= 0 {
			startedAt = playedAt
		}

		playedMs := ev.PlayedMs
		if playedMs < 0 {
			playedMs = 0
		}
		durationMs := ev.DurationMs
		if durationMs < 0 {
			durationMs = 0
		}

		source := strings.TrimSpace(ev.Source)
		if source == "" {
			source = "server"
		}
		if len(source) > 32 {
			source = source[:32]
		}

		var sessionID *string
		if s := strings.TrimSpace(ev.SessionID); s != "" {
			if len(s) > 64 {
				s = s[:64]
			}
			sessionID = &s
		}

		doc := bson.M{
			"_id":         docID,
			"user_id":     userID,
			"track_id":    tid,
			"played_at":   playedAt,
			"started_at":  startedAt,
			"played_ms":   playedMs,
			"duration_ms": durationMs,
			"completed":   ev.Completed,
			"skipped":     ev.Skipped,
			"source":      source,
			"session_id":  sessionID,
			"created_at":  now,
		}

		res, err := r.eventsCol.UpdateOne(
			ctx,
			bson.M{"_id": docID},
			bson.M{"$setOnInsert": doc},
			options.UpdateOne().SetUpsert(true),
		)
		if err == nil && res.UpsertedCount > 0 {
			stored++
		}

		// Also record in legacy playback metrics
		r.recordPlaybackMetrics(ctx, userID, tid, source, playedAt)
	}

	return stored, nil
}

func (r *mongoRecapRepository) recordPlaybackMetrics(ctx context.Context, userID int64, trackID string, source string, playedAt float64) {
	if trackID == "" {
		return
	}
	now := playedAt
	if now <= 0 {
		now = float64(time.Now().Unix())
	}

	// userHistory log
	_, _ = r.historyCol.InsertOne(ctx, bson.M{
		"user_id":   userID,
		"track_id":  trackID,
		"source":    source,
		"played_at": now,
	})

	// userPlayback counter
	if userID > 0 {
		userDocID := fmt.Sprintf("%d_%s", userID, trackID)
		inc := bson.M{"count": 1}
		if source != "" {
			inc["sources."+source] = 1
		}
		_, _ = r.userPbCol.UpdateOne(
			ctx,
			bson.M{"_id": userDocID},
			bson.M{
				"$setOnInsert": bson.M{
					"user_id":  userID,
					"track_id": trackID,
				},
				"$inc": inc,
				"$set": bson.M{
					"last_played_at": now,
					"updated_at":     now,
				},
			},
			options.UpdateOne().SetUpsert(true),
		)
	}

	// globalPlayback counter
	incGlob := bson.M{"count": 1}
	if source != "" {
		incGlob["sources."+source] = 1
	}
	_, _ = r.globPbCol.UpdateOne(
		ctx,
		bson.M{"_id": trackID},
		bson.M{
			"$inc": incGlob,
			"$set": bson.M{
				"last_played_at": now,
				"updated_at":     now,
			},
		},
		options.UpdateOne().SetUpsert(true),
	)

	// Increment track play_count in tracks collection
	_, _ = r.tracksCol.UpdateOne(
		ctx,
		bson.M{"_id": trackID},
		bson.M{"$inc": bson.M{"audio.play_count": 1, "play_count": 1}},
	)
}

// LoadEvents retrieves listening events for a user within [start, end), falling back to userHistory if needed.
func (r *mongoRecapRepository) LoadEvents(ctx context.Context, userID int64, start float64, end float64) ([]*models.ListeningEventDoc, error) {
	filter := bson.M{
		"user_id": userID,
		"played_at": bson.M{
			"$gte": start,
			"$lt":  end,
		},
	}

	cursor, err := r.eventsCol.Find(ctx, filter)
	if err == nil {
		defer cursor.Close(ctx)
		var events []*models.ListeningEventDoc
		for cursor.Next(ctx) {
			var ev models.ListeningEventDoc
			if err := cursor.Decode(&ev); err == nil && ev.TrackID != "" {
				events = append(events, &ev)
			}
		}
		if len(events) > 0 {
			return events, nil
		}
	}

	// Fallback to legacy userHistory rows (where one row = one stream start)
	histFilter := bson.M{
		"$or": []bson.M{
			{"user_id": userID},
			{"user_id": fmt.Sprintf("%d", userID)},
		},
		"played_at": bson.M{
			"$gte": start,
			"$lt":  end,
		},
	}

	hCursor, err := r.historyCol.Find(ctx, histFilter)
	if err != nil {
		return nil, err
	}
	defer hCursor.Close(ctx)

	var legacyEvents []*models.ListeningEventDoc
	for hCursor.Next(ctx) {
		var row struct {
			TrackID  string  `bson:"track_id"`
			PlayedAt float64 `bson:"played_at"`
			Source   string  `bson:"source"`
		}
		if err := hCursor.Decode(&row); err == nil && row.TrackID != "" {
			legacyEvents = append(legacyEvents, &models.ListeningEventDoc{
				TrackID:    row.TrackID,
				PlayedAt:   row.PlayedAt,
				StartedAt:  row.PlayedAt,
				PlayedMs:   nil, // indicates legacy play: assume full playback
				DurationMs: 0,
				Completed:  false,
				Skipped:    false,
				Source:     row.Source,
				Legacy:     true,
			})
		}
	}

	return legacyEvents, nil
}

// FirstEventTimestamp returns the earliest play timestamp across listening_events and userHistory.
func (r *mongoRecapRepository) FirstEventTimestamp(ctx context.Context, userID int64) (float64, bool, error) {
	var earliest float64
	found := false

	// Check listening_events
	evOpt := options.FindOne().SetSort(bson.D{{Key: "played_at", Value: 1}})
	var evDoc struct {
		PlayedAt float64 `bson:"played_at"`
	}
	if err := r.eventsCol.FindOne(ctx, bson.M{"user_id": userID}, evOpt).Decode(&evDoc); err == nil && evDoc.PlayedAt > 0 {
		earliest = evDoc.PlayedAt
		found = true
	}

	// Check userHistory
	histFilter := bson.M{
		"$or": []bson.M{
			{"user_id": userID},
			{"user_id": fmt.Sprintf("%d", userID)},
		},
	}
	var hDoc struct {
		PlayedAt float64 `bson:"played_at"`
	}
	if err := r.historyCol.FindOne(ctx, histFilter, evOpt).Decode(&hDoc); err == nil && hDoc.PlayedAt > 0 {
		if !found || hDoc.PlayedAt < earliest {
			earliest = hDoc.PlayedAt
			found = true
		}
	}

	return earliest, found, nil
}

// HasPlays returns whether any listening events exist for the user in [start, end).
func (r *mongoRecapRepository) HasPlays(ctx context.Context, userID int64, start float64, end float64) (bool, error) {
	filter := bson.M{
		"user_id": userID,
		"played_at": bson.M{
			"$gte": start,
			"$lt":  end,
		},
	}
	count, err := r.eventsCol.CountDocuments(ctx, filter, options.Count().SetLimit(1))
	if err == nil && count > 0 {
		return true, nil
	}

	histFilter := bson.M{
		"$or": []bson.M{
			{"user_id": userID},
			{"user_id": fmt.Sprintf("%d", userID)},
		},
		"played_at": bson.M{
			"$gte": start,
			"$lt":  end,
		},
	}
	hCount, err := r.historyCol.CountDocuments(ctx, histFilter, options.Count().SetLimit(1))
	if err == nil && hCount > 0 {
		return true, nil
	}

	return false, nil
}

// FirstSeenBefore returns the subset of trackIDs that were listened to before the specified timestamp.
func (r *mongoRecapRepository) FirstSeenBefore(ctx context.Context, userID int64, trackIDs []string, before float64) (map[string]bool, error) {
	seen := make(map[string]bool)
	if len(trackIDs) == 0 {
		return seen, nil
	}

	evFilter := bson.M{
		"user_id":   userID,
		"track_id":  bson.M{"$in": trackIDs},
		"played_at": bson.M{"$lt": before},
	}
	evRes := r.eventsCol.Distinct(ctx, "track_id", evFilter)
	if evRes.Err() == nil {
		var rawValues []interface{}
		if err := evRes.Decode(&rawValues); err == nil {
			for _, raw := range rawValues {
				if s, ok := raw.(string); ok && s != "" {
					seen[s] = true
				}
			}
		}
	}

	histFilter := bson.M{
		"$or": []bson.M{
			{"user_id": userID},
			{"user_id": fmt.Sprintf("%d", userID)},
		},
		"track_id":  bson.M{"$in": trackIDs},
		"played_at": bson.M{"$lt": before},
	}
	hRes := r.historyCol.Distinct(ctx, "track_id", histFilter)
	if hRes.Err() == nil {
		var rawValues []interface{}
		if err := hRes.Decode(&rawValues); err == nil {
			for _, raw := range rawValues {
				if s, ok := raw.(string); ok && s != "" {
					seen[s] = true
				}
			}
		}
	}

	return seen, nil
}

// GetSnapshot retrieves a cached recap snapshot if available.
func (r *mongoRecapRepository) GetSnapshot(ctx context.Context, userID int64, ptype models.RecapPeriodType, period string, tz int) (*models.RecapSnapshot, float64, error) {
	filter := bson.M{
		"user_id": userID,
		"type":    ptype,
		"period":  period,
		"tz":      tz,
	}

	var doc struct {
		SchemaVersion int                   `bson:"schemaVersion"`
		GeneratedAt   float64               `bson:"generatedAt"`
		Snapshot      *models.RecapSnapshot `bson:"snapshot"`
	}

	err := r.snapshotsCol.FindOne(ctx, filter).Decode(&doc)
	if err != nil {
		return nil, 0, err
	}
	if doc.SchemaVersion != RecapSchemaVersion || doc.Snapshot == nil {
		return nil, 0, mongo.ErrNoDocuments
	}

	return doc.Snapshot, doc.GeneratedAt, nil
}

// SaveSnapshot caches a generated recap snapshot.
func (r *mongoRecapRepository) SaveSnapshot(ctx context.Context, userID int64, ptype models.RecapPeriodType, period string, tz int, snap *models.RecapSnapshot) error {
	filter := bson.M{
		"user_id": userID,
		"type":    ptype,
		"period":  period,
		"tz":      tz,
	}

	now := float64(time.Now().Unix())
	update := bson.M{
		"$set": bson.M{
			"user_id":       userID,
			"type":          ptype,
			"period":        period,
			"tz":            tz,
			"schemaVersion": RecapSchemaVersion,
			"generatedAt":   now,
			"snapshot":      snap,
		},
	}

	_, err := r.snapshotsCol.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))
	return err
}

func generateShareToken() string {
	b := make([]byte, 9)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// CreateShare creates or updates a public share token for a recap snapshot.
func (r *mongoRecapRepository) CreateShare(ctx context.Context, userID int64, ptype models.RecapPeriodType, period string, summary *models.RecapPublicSummary) (*models.RecapShareItem, error) {
	existingFilter := bson.M{
		"user_id":    userID,
		"type":       ptype,
		"period":     period,
		"revoked_at": nil,
	}

	var existing models.RecapShareDoc
	if err := r.sharesCol.FindOne(ctx, existingFilter).Decode(&existing); err == nil && existing.Token != "" {
		_, _ = r.sharesCol.UpdateOne(ctx, bson.M{"_id": existing.Token}, bson.M{"$set": bson.M{"summary": summary}})
		label := ""
		if summary != nil {
			label = summary.Label
		}
		return &models.RecapShareItem{
			Token:     existing.Token,
			Type:      existing.Type,
			Period:    existing.Period,
			Label:     label,
			CreatedAt: existing.CreatedAt,
		}, nil
	}

	token := generateShareToken()
	now := float64(time.Now().Unix())

	_, err := r.sharesCol.InsertOne(ctx, bson.M{
		"_id":        token,
		"token":      token,
		"user_id":    userID,
		"type":       ptype,
		"period":     period,
		"created_at": now,
		"revoked_at": nil,
		"summary":    summary,
	})
	if err != nil {
		return nil, err
	}

	label := ""
	if summary != nil {
		label = summary.Label
	}
	return &models.RecapShareItem{
		Token:     token,
		Type:      ptype,
		Period:    period,
		Label:     label,
		CreatedAt: now,
	}, nil
}

// ListShares lists active share links for a user.
func (r *mongoRecapRepository) ListShares(ctx context.Context, userID int64) ([]*models.RecapShareItem, error) {
	filter := bson.M{
		"user_id":    userID,
		"revoked_at": nil,
	}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})

	cursor, err := r.sharesCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var items []*models.RecapShareItem
	for cursor.Next(ctx) {
		var doc models.RecapShareDoc
		if err := cursor.Decode(&doc); err == nil {
			label := ""
			if doc.Summary != nil {
				label = doc.Summary.Label
			}
			items = append(items, &models.RecapShareItem{
				Token:     doc.Token,
				Type:      doc.Type,
				Period:    doc.Period,
				Label:     label,
				CreatedAt: doc.CreatedAt,
			})
		}
	}

	return items, nil
}

// RevokeShare marks a share token as revoked.
func (r *mongoRecapRepository) RevokeShare(ctx context.Context, userID int64, token string) (bool, error) {
	filter := bson.M{
		"user_id":    userID,
		"token":      token,
		"revoked_at": nil,
	}
	update := bson.M{
		"$set": bson.M{
			"revoked_at": float64(time.Now().Unix()),
		},
	}

	res, err := r.sharesCol.UpdateOne(ctx, filter, update)
	if err != nil {
		return false, err
	}
	return res.ModifiedCount > 0, nil
}

// GetPublicShare retrieves the public recap summary for a valid, unrevoked share token.
func (r *mongoRecapRepository) GetPublicShare(ctx context.Context, token string) (*models.RecapPublicSummary, error) {
	filter := bson.M{
		"token":      token,
		"revoked_at": nil,
	}

	var doc models.RecapShareDoc
	if err := r.sharesCol.FindOne(ctx, filter).Decode(&doc); err != nil {
		return nil, err
	}
	return doc.Summary, nil
}

// DeleteUserData deletes all listening events, cached snapshots, and shares for a user.
func (r *mongoRecapRepository) DeleteUserData(ctx context.Context, userID int64) (int64, int64, int64, error) {
	filter := bson.M{"user_id": userID}

	resE, err := r.eventsCol.DeleteMany(ctx, filter)
	if err != nil {
		return 0, 0, 0, err
	}
	resS, err := r.snapshotsCol.DeleteMany(ctx, filter)
	if err != nil {
		return resE.DeletedCount, 0, 0, err
	}
	resSh, err := r.sharesCol.DeleteMany(ctx, filter)
	if err != nil {
		return resE.DeletedCount, resS.DeletedCount, 0, err
	}

	return resE.DeletedCount, resS.DeletedCount, resSh.DeletedCount, nil
}
