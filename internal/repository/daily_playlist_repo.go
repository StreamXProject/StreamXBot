package repository

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"streamgo/internal/database"
	"streamgo/internal/models"
)

// DailyPlaylistRepository manages generated daily playlist documents.
type DailyPlaylistRepository interface {
	GetCachedDailyPlaylist(ctx context.Context, docID string) (*models.DailyPlaylistDoc, error)
	SaveDailyPlaylist(ctx context.Context, doc *models.DailyPlaylistDoc) error
}

type mongoDailyPlaylistRepository struct {
	col *mongo.Collection
}

// NewDailyPlaylistRepository creates a new DailyPlaylistRepository.
func NewDailyPlaylistRepository(db *database.Client) DailyPlaylistRepository {
	return &mongoDailyPlaylistRepository{
		col: db.Collection("daily_playlists"),
	}
}

// GetCachedDailyPlaylist retrieves a cached daily playlist by ID.
func (r *mongoDailyPlaylistRepository) GetCachedDailyPlaylist(ctx context.Context, docID string) (*models.DailyPlaylistDoc, error) {
	filter := bson.M{"_id": docID}
	var doc models.DailyPlaylistDoc
	err := r.col.FindOne(ctx, filter).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &doc, nil
}

// SaveDailyPlaylist upserts a generated daily playlist document.
func (r *mongoDailyPlaylistRepository) SaveDailyPlaylist(ctx context.Context, doc *models.DailyPlaylistDoc) error {
	filter := bson.M{"_id": doc.ID}
	update := bson.M{"$set": doc}
	opts := options.UpdateOne().SetUpsert(true)
	_, err := r.col.UpdateOne(ctx, filter, update, opts)
	return err
}
