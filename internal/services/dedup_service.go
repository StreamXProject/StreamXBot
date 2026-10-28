package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"streamgo/internal/database"
	"streamgo/internal/models"
)

// DedupService calculates audio fingerprints and detects duplicate track uploads.
type DedupService struct {
	col *mongo.Collection
}

// NewDedupService creates a new DedupService instance.
func NewDedupService(db *database.Client) *DedupService {
	var col *mongo.Collection
	if db != nil {
		col = db.Collection("audioTracks")
	}
	return &DedupService{col: col}
}

// NormalizeText cleans and strips non-alphanumeric characters for fingerprinting (supports multilingual Unicode).
func NormalizeText(text string) string {
	s := strings.ToLower(text)
	reNonAlpha := regexp.MustCompile(`[^\p{L}\p{N}]+`)
	s = reNonAlpha.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// MetadataFingerprint generates a fuzzy bucketed fingerprint: title|artist|album|duration.
func MetadataFingerprint(title, artist, album string, durationSec float64) string {
	t := NormalizeText(title)
	a := NormalizeText(artist)
	al := NormalizeText(album)

	durKey := ""
	if durationSec > 0 {
		// Round to nearest 2-second bucket to tolerate minor encoding duration differences
		bucket := int64(math.Round(durationSec/2.0) * 2)
		durKey = fmt.Sprintf("%d", bucket)
	}

	parts := []string{t, a, al, durKey}
	return strings.Trim(strings.Join(parts, "|"), "|")
}

// CheckDuplicate queries MongoDB to find if a track already exists by file_unique_id or fingerprint.
func (s *DedupService) CheckDuplicate(ctx context.Context, fileUniqueID, fingerprint string) (*models.Track, bool, error) {
	if s.col == nil {
		return nil, false, nil
	}

	var orConditions []bson.M
	if fileUniqueID != "" {
		orConditions = append(orConditions, bson.M{"telegram.file_unique_id": fileUniqueID})
		orConditions = append(orConditions, bson.M{"_id": fileUniqueID})
	}
	if fingerprint != "" {
		orConditions = append(orConditions, bson.M{"fingerprint": fingerprint})
	}

	if len(orConditions) == 0 {
		return nil, false, nil
	}

	filter := bson.M{
		"deleted": bson.M{"$ne": true},
		"$or":     orConditions,
	}

	var existing models.Track
	err := s.col.FindOne(ctx, filter).Decode(&existing)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, false, nil
		}
		return nil, false, err
	}

	return &existing, true, nil
}

// IsDuplicate checks whether a track is already present in the database.
func (s *DedupService) IsDuplicate(ctx context.Context, fileUniqueID, title, artist, album string, durationSec float64) bool {
	fp := MetadataFingerprint(title, artist, album, durationSec)
	_, found, _ := s.CheckDuplicate(ctx, fileUniqueID, fp)
	return found
}
