package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"streamgo/internal/database"
	"streamgo/internal/models"
)

// AccessControlRepository manages access control policies, invites, and allowlists.
type AccessControlRepository interface {
	GetPolicy(ctx context.Context) (*models.AccessPolicy, error)
	SavePolicy(ctx context.Context, policy *models.AccessPolicy) error

	CreateInvite(ctx context.Context, invite *models.InviteCode) error
	GetInvite(ctx context.Context, code string) (*models.InviteCode, error)
	ConsumeInvite(ctx context.Context, code string) error
	RevokeInvite(ctx context.Context, code string) bool
	ListInvites(ctx context.Context, includeDead bool) ([]*models.InviteCode, error)

	AddToAllowlist(ctx context.Context, entry *models.AllowlistEntry) error
	RemoveFromAllowlist(ctx context.Context, userID int64) bool
	IsInAllowlist(ctx context.Context, userID int64) bool
	ListAllowlist(ctx context.Context) ([]*models.AllowlistEntry, error)

	AddToBypass(ctx context.Context, entry *models.AllowlistEntry) error
	RemoveFromBypass(ctx context.Context, userID int64) bool
	IsInBypass(ctx context.Context, userID int64) bool
	ListBypass(ctx context.Context) ([]*models.AllowlistEntry, error)

	ListUsers(ctx context.Context, query string, status string, limit int, skip int) ([]*models.User, int64, error)
	SetUserStatus(ctx context.Context, userID int64, status string, reason string) error
	RevokeUserSessions(ctx context.Context, userID int64) (int, error)
}

type mongoAccessControlRepository struct {
	authConfigCol *mongo.Collection
	invitesCol    *mongo.Collection
	allowlistCol  *mongo.Collection
	bypassCol     *mongo.Collection
	usersCol      *mongo.Collection
}

// NewAccessControlRepository creates a MongoDB AccessControlRepository.
func NewAccessControlRepository(db *database.Client) AccessControlRepository {
	return &mongoAccessControlRepository{
		authConfigCol: db.Collection("auth_config"),
		invitesCol:    db.Collection("invites"),
		allowlistCol:  db.Collection("allowlist"),
		bypassCol:     db.Collection("bypass"),
		usersCol:      db.Collection("users"),
	}
}

// GetPolicy retrieves the access policy document, or returns default open policy if not found.
func (r *mongoAccessControlRepository) GetPolicy(ctx context.Context) (*models.AccessPolicy, error) {
	filter := bson.M{"_id": "access_policy"}
	var doc struct {
		Policy models.AccessPolicy `bson:"policy"`
	}
	err := r.authConfigCol.FindOne(ctx, filter).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return &models.AccessPolicy{
				RegistrationMode:  "open",
				EnforceMembership: false,
				RequiredChats:     []models.RequiredChat{},
			}, nil
		}
		return nil, err
	}
	if doc.Policy.RegistrationMode == "" {
		doc.Policy.RegistrationMode = "open"
	}
	return &doc.Policy, nil
}

// SavePolicy saves the access policy.
func (r *mongoAccessControlRepository) SavePolicy(ctx context.Context, policy *models.AccessPolicy) error {
	filter := bson.M{"_id": "access_policy"}
	update := bson.M{
		"$set": bson.M{
			"policy":     policy,
			"updated_at": float64(time.Now().Unix()),
		},
	}
	opts := options.UpdateOne().SetUpsert(true)
	_, err := r.authConfigCol.UpdateOne(ctx, filter, update, opts)
	return err
}

// CreateInvite inserts a new invite code.
func (r *mongoAccessControlRepository) CreateInvite(ctx context.Context, invite *models.InviteCode) error {
	_, err := r.invitesCol.InsertOne(ctx, invite)
	return err
}

// GetInvite finds an invite code.
func (r *mongoAccessControlRepository) GetInvite(ctx context.Context, code string) (*models.InviteCode, error) {
	filter := bson.M{"_id": code}
	var inv models.InviteCode
	err := r.invitesCol.FindOne(ctx, filter).Decode(&inv)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &inv, nil
}

// ConsumeInvite increments the used count on an invite code.
func (r *mongoAccessControlRepository) ConsumeInvite(ctx context.Context, code string) error {
	filter := bson.M{"_id": code}
	update := bson.M{"$inc": bson.M{"used_count": 1}}
	_, err := r.invitesCol.UpdateOne(ctx, filter, update)
	return err
}

// RevokeInvite marks an invite as revoked.
func (r *mongoAccessControlRepository) RevokeInvite(ctx context.Context, code string) bool {
	res, err := r.invitesCol.UpdateOne(ctx, bson.M{"_id": code}, bson.M{"$set": bson.M{"revoked": true}})
	return err == nil && res.MatchedCount > 0
}

// ListInvites lists invite codes.
func (r *mongoAccessControlRepository) ListInvites(ctx context.Context, includeDead bool) ([]*models.InviteCode, error) {
	filter := bson.M{}
	if !includeDead {
		now := float64(time.Now().Unix())
		filter["revoked"] = bson.M{"$ne": true}
		filter["$or"] = []bson.M{
			{"expires_at": nil},
			{"expires_at": bson.M{"$gt": now}},
		}
	}

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(100)
	cursor, err := r.invitesCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var items []*models.InviteCode
	for cursor.Next(ctx) {
		var inv models.InviteCode
		if err := cursor.Decode(&inv); err == nil {
			items = append(items, &inv)
		}
	}
	return items, nil
}

// AddToAllowlist adds a user to allowlist.
func (r *mongoAccessControlRepository) AddToAllowlist(ctx context.Context, entry *models.AllowlistEntry) error {
	opts := options.UpdateOne().SetUpsert(true)
	_, err := r.allowlistCol.UpdateOne(ctx, bson.M{"_id": entry.UserID}, bson.M{"$set": entry}, opts)
	return err
}

// RemoveFromAllowlist removes user from allowlist.
func (r *mongoAccessControlRepository) RemoveFromAllowlist(ctx context.Context, userID int64) bool {
	res, err := r.allowlistCol.DeleteOne(ctx, bson.M{"_id": userID})
	return err == nil && res.DeletedCount > 0
}

// IsInAllowlist checks if a user is in allowlist.
func (r *mongoAccessControlRepository) IsInAllowlist(ctx context.Context, userID int64) bool {
	err := r.allowlistCol.FindOne(ctx, bson.M{"_id": userID}).Err()
	return err == nil
}

// ListAllowlist lists all allowlisted users.
func (r *mongoAccessControlRepository) ListAllowlist(ctx context.Context) ([]*models.AllowlistEntry, error) {
	cursor, err := r.allowlistCol.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "added_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var items []*models.AllowlistEntry
	for cursor.Next(ctx) {
		var e models.AllowlistEntry
		if err := cursor.Decode(&e); err == nil {
			items = append(items, &e)
		}
	}
	return items, nil
}

// AddToBypass adds a user to bypass list.
func (r *mongoAccessControlRepository) AddToBypass(ctx context.Context, entry *models.AllowlistEntry) error {
	opts := options.UpdateOne().SetUpsert(true)
	_, err := r.bypassCol.UpdateOne(ctx, bson.M{"_id": entry.UserID}, bson.M{"$set": entry}, opts)
	return err
}

// RemoveFromBypass removes user from bypass list.
func (r *mongoAccessControlRepository) RemoveFromBypass(ctx context.Context, userID int64) bool {
	res, err := r.bypassCol.DeleteOne(ctx, bson.M{"_id": userID})
	return err == nil && res.DeletedCount > 0
}

// IsInBypass checks if a user is in the membership bypass list.
func (r *mongoAccessControlRepository) IsInBypass(ctx context.Context, userID int64) bool {
	err := r.bypassCol.FindOne(ctx, bson.M{"_id": userID}).Err()
	return err == nil
}

// ListBypass lists all bypassed users.
func (r *mongoAccessControlRepository) ListBypass(ctx context.Context) ([]*models.AllowlistEntry, error) {
	cursor, err := r.bypassCol.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "added_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var items []*models.AllowlistEntry
	for cursor.Next(ctx) {
		var e models.AllowlistEntry
		if err := cursor.Decode(&e); err == nil {
			items = append(items, &e)
		}
	}
	return items, nil
}

// ListUsers lists users with optional search and status filter.
func (r *mongoAccessControlRepository) ListUsers(ctx context.Context, query string, status string, limit int, skip int) ([]*models.User, int64, error) {
	filter := bson.M{}
	if s := strings.TrimSpace(status); s != "" {
		filter["status"] = s
	}
	if q := strings.TrimSpace(query); q != "" {
		filter["$or"] = []bson.M{
			{"username": bson.M{"$regex": q, "$options": "i"}},
			{"first_name": bson.M{"$regex": q, "$options": "i"}},
		}
	}

	total, err := r.usersCol.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(skip))

	cursor, err := r.usersCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var users []*models.User
	for cursor.Next(ctx) {
		var u models.User
		if err := cursor.Decode(&u); err == nil {
			users = append(users, &u)
		}
	}
	return users, total, nil
}

// SetUserStatus updates user status (active, locked, restricted).
func (r *mongoAccessControlRepository) SetUserStatus(ctx context.Context, userID int64, status string, reason string) error {
	update := bson.M{
		"$set": bson.M{
			"status":      status,
			"updated_at":  float64(time.Now().Unix()),
			"lock_reason": reason,
		},
	}
	_, err := r.usersCol.UpdateOne(ctx, bson.M{"_id": userID}, update)
	return err
}

// RevokeUserSessions increments token_version to invalidate all active tokens.
func (r *mongoAccessControlRepository) RevokeUserSessions(ctx context.Context, userID int64) (int, error) {
	update := bson.M{
		"$inc": bson.M{"token_version": 1},
		"$set": bson.M{"updated_at": float64(time.Now().Unix())},
	}
	var res models.User
	err := r.usersCol.FindOneAndUpdate(ctx, bson.M{"_id": userID}, update, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&res)
	if err != nil {
		return 0, err
	}
	return res.TokenVersion, nil
}
