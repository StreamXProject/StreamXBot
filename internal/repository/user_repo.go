package repository

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"streamgo/internal/database"
	"streamgo/internal/models"
)

// UserRepository manages database operations for users.
type UserRepository interface {
	UpsertUser(ctx context.Context, user *models.User) (*models.User, error)
	GetUserByID(ctx context.Context, id int64) (*models.User, error)
	GetUserByUsername(ctx context.Context, username string) (*models.User, error)
	UpdateUserCredentials(ctx context.Context, userID int64, username string, pwd *models.PasswordHash) error
	UpdateIntegrations(ctx context.Context, userID int64, req models.IntegrationsUpdateRequest) (*models.UserIntegrations, error)

	SaveRegistrationOTP(ctx context.Context, otp *models.RegistrationOTP) error
	GetRegistrationOTP(ctx context.Context, userID int64) (*models.RegistrationOTP, error)
	DeleteRegistrationOTP(ctx context.Context, userID int64) error

	SaveOIDCSession(ctx context.Context, session *models.OIDCSession) error
	GetAndDeleteOIDCSession(ctx context.Context, state string) (*models.OIDCSession, error)

	GetOwnerPassword(ctx context.Context) (*models.PasswordHash, error)
	SetOwnerPassword(ctx context.Context, pwd *models.PasswordHash, updatedBy int64) error

	UpdateFCMToken(ctx context.Context, userID int64, fcmToken string) error
	SaveBotAuthSession(ctx context.Context, session *models.BotAuthSession) error
	GetBotAuthSession(ctx context.Context, sessionID string) (*models.BotAuthSession, error)
	ConfirmBotAuthSession(ctx context.Context, sessionID, token string, user *models.User) error
}

type mongoUserRepository struct {
	usersCol           *mongo.Collection
	otpsCol            *mongo.Collection
	oidcCol            *mongo.Collection
	authConfigCol      *mongo.Collection
	botAuthSessionsCol *mongo.Collection
}

// NewUserRepository creates a UserRepository instance.
func NewUserRepository(db *database.Client) UserRepository {
	return &mongoUserRepository{
		usersCol:           db.Collection("users"),
		otpsCol:            db.Collection("registration_otps"),
		oidcCol:            db.Collection("oidc_sessions"),
		authConfigCol:      db.Collection("auth_config"),
		botAuthSessionsCol: db.Collection("bot_auth_sessions"),
	}
}

// UpsertUser inserts or updates a user document based on user ID.
func (r *mongoUserRepository) UpsertUser(ctx context.Context, u *models.User) (*models.User, error) {
	now := float64(time.Now().Unix())
	filter := bson.M{"_id": u.ID}

	setMap := bson.M{
		"updated_at": now,
	}
	if u.FirstName != "" {
		setMap["first_name"] = u.FirstName
	}
	if u.Username != "" {
		setMap["username"] = u.Username
	}
	if u.PhotoURL != "" {
		setMap["photo_url"] = u.PhotoURL
	}
	if u.ProfileURL != "" {
		setMap["profile_url"] = u.ProfileURL
	}
	if u.Telegram != nil {
		setMap["telegram"] = u.Telegram
	}
	if u.Status != "" {
		setMap["status"] = u.Status
	}
	if u.Password != nil {
		setMap["password"] = u.Password
		setMap["password_updated_at"] = now
	}
	if u.RegisteredVia != nil {
		setMap["registered_via"] = u.RegisteredVia
	}

	update := bson.M{
		"$set": setMap,
		"$setOnInsert": bson.M{
			"created_at":    now,
			"token_version": 0,
		},
	}

	opts := options.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(options.After)

	var result models.User
	err := r.usersCol.FindOneAndUpdate(ctx, filter, update, opts).Decode(&result)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert user %d: %w", u.ID, err)
	}

	return &result, nil
}

// GetUserByID fetches a user by Telegram numeric ID.
func (r *mongoUserRepository) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	filter := bson.M{"_id": id}
	var u models.User
	err := r.usersCol.FindOne(ctx, filter).Decode(&u)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to fetch user %d: %w", id, err)
	}
	return &u, nil
}

// GetUserByUsername fetches a user by canonical username.
func (r *mongoUserRepository) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, nil
	}
	pattern := fmt.Sprintf("^%s$", regexp.QuoteMeta(username))
	filter := bson.M{
		"$or": []bson.M{
			{"username": bson.M{"$regex": pattern, "$options": "i"}},
			{"telegram.username": bson.M{"$regex": pattern, "$options": "i"}},
		},
	}
	var u models.User
	err := r.usersCol.FindOne(ctx, filter).Decode(&u)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to fetch user by username %s: %w", username, err)
	}
	return &u, nil
}

// UpdateUserCredentials updates the username and password for an existing user.
func (r *mongoUserRepository) UpdateUserCredentials(ctx context.Context, userID int64, username string, pwd *models.PasswordHash) error {
	now := float64(time.Now().Unix())
	filter := bson.M{"_id": userID}
	update := bson.M{
		"$set": bson.M{
			"username":            username,
			"password":            pwd,
			"username_updated_at": now,
			"password_updated_at": now,
			"updated_at":          now,
		},
	}
	_, err := r.usersCol.UpdateOne(ctx, filter, update)
	return err
}

// UpdateIntegrations updates integrations.discord and integrations.lastfm.
func (r *mongoUserRepository) UpdateIntegrations(ctx context.Context, userID int64, req models.IntegrationsUpdateRequest) (*models.UserIntegrations, error) {
	now := float64(time.Now().Unix())
	filter := bson.M{"_id": userID}
	setFields := bson.M{
		"updated_at": now,
	}

	if req.Discord != nil {
		req.Discord.UpdatedAt = now
		setFields["integrations.discord"] = req.Discord
	}
	if req.Lastfm != nil {
		req.Lastfm.UpdatedAt = now
		setFields["integrations.lastfm"] = req.Lastfm
	}

	update := bson.M{"$set": setFields}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After).SetUpsert(true)
	var updated models.User
	err := r.usersCol.FindOneAndUpdate(ctx, filter, update, opts).Decode(&updated)
	if err != nil {
		return nil, err
	}
	if updated.Integrations != nil {
		return updated.Integrations, nil
	}
	return &models.UserIntegrations{}, nil
}

// SaveRegistrationOTP inserts or updates a registration OTP record.
func (r *mongoUserRepository) SaveRegistrationOTP(ctx context.Context, otp *models.RegistrationOTP) error {
	filter := bson.M{"_id": otp.ID}
	update := bson.M{"$set": otp}
	opts := options.UpdateOne().SetUpsert(true)
	_, err := r.otpsCol.UpdateOne(ctx, filter, update, opts)
	return err
}

// GetRegistrationOTP retrieves a pending registration OTP for a user ID.
func (r *mongoUserRepository) GetRegistrationOTP(ctx context.Context, userID int64) (*models.RegistrationOTP, error) {
	filter := bson.M{"_id": userID}
	var otp models.RegistrationOTP
	err := r.otpsCol.FindOne(ctx, filter).Decode(&otp)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &otp, nil
}

// DeleteRegistrationOTP deletes a pending registration OTP.
func (r *mongoUserRepository) DeleteRegistrationOTP(ctx context.Context, userID int64) error {
	_, err := r.otpsCol.DeleteOne(ctx, bson.M{"_id": userID})
	return err
}

// SaveOIDCSession saves an authorization state session.
func (r *mongoUserRepository) SaveOIDCSession(ctx context.Context, session *models.OIDCSession) error {
	filter := bson.M{"_id": session.ID}
	update := bson.M{"$set": session}
	opts := options.UpdateOne().SetUpsert(true)
	_, err := r.oidcCol.UpdateOne(ctx, filter, update, opts)
	return err
}

// GetAndDeleteOIDCSession retrieves and deletes an authorization state session in one operation.
func (r *mongoUserRepository) GetAndDeleteOIDCSession(ctx context.Context, state string) (*models.OIDCSession, error) {
	filter := bson.M{"_id": state}
	var session models.OIDCSession
	err := r.oidcCol.FindOneAndDelete(ctx, filter).Decode(&session)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

// GetOwnerPassword retrieves the hashed owner/server password from auth_config.
func (r *mongoUserRepository) GetOwnerPassword(ctx context.Context) (*models.PasswordHash, error) {
	filter := bson.M{"_id": "owner_password"}
	var doc struct {
		Password *models.PasswordHash `bson:"password"`
	}
	err := r.authConfigCol.FindOne(ctx, filter).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return doc.Password, nil
}

// SetOwnerPassword saves or updates the hashed owner/server password in auth_config.
func (r *mongoUserRepository) SetOwnerPassword(ctx context.Context, pwd *models.PasswordHash, updatedBy int64) error {
	now := float64(time.Now().Unix())
	filter := bson.M{"_id": "owner_password"}

	setMap := bson.M{
		"password":   pwd,
		"updated_at": now,
	}
	if updatedBy > 0 {
		setMap["updated_by"] = updatedBy
	}

	update := bson.M{
		"$set": setMap,
		"$setOnInsert": bson.M{
			"created_at": now,
		},
	}
	opts := options.UpdateOne().SetUpsert(true)
	_, err := r.authConfigCol.UpdateOne(ctx, filter, update, opts)
	return err
}

// UpdateFCMToken updates the user's FCM push token in the users collection.
func (r *mongoUserRepository) UpdateFCMToken(ctx context.Context, userID int64, fcmToken string) error {
	now := float64(time.Now().Unix())
	filter := bson.M{"_id": userID}
	update := bson.M{
		"$set": bson.M{
			"fcm_token":  fcmToken,
			"updated_at": now,
		},
	}
	opts := options.UpdateOne().SetUpsert(true)
	_, err := r.usersCol.UpdateOne(ctx, filter, update, opts)
	return err
}

// SaveBotAuthSession saves or updates a temporary Telegram bot auth session.
func (r *mongoUserRepository) SaveBotAuthSession(ctx context.Context, session *models.BotAuthSession) error {
	filter := bson.M{"_id": session.ID}
	update := bson.M{
		"$set": bson.M{
			"_id":         session.ID,
			"status":      session.Status,
			"invite_code": session.InviteCode,
			"created_at":  session.CreatedAt,
			"expires_at":  session.ExpiresAt,
		},
	}
	opts := options.UpdateOne().SetUpsert(true)
	_, err := r.botAuthSessionsCol.UpdateOne(ctx, filter, update, opts)
	return err
}

// GetBotAuthSession retrieves a temporary bot auth session by ID.
func (r *mongoUserRepository) GetBotAuthSession(ctx context.Context, sessionID string) (*models.BotAuthSession, error) {
	filter := bson.M{"_id": sessionID}
	var s models.BotAuthSession
	err := r.botAuthSessionsCol.FindOne(ctx, filter).Decode(&s)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

// ConfirmBotAuthSession marks a temporary bot auth session as confirmed with token and user details.
func (r *mongoUserRepository) ConfirmBotAuthSession(ctx context.Context, sessionID, token string, user *models.User) error {
	now := float64(time.Now().Unix())
	filter := bson.M{"_id": sessionID}
	update := bson.M{
		"$set": bson.M{
			"status":       "confirmed",
			"token":        token,
			"user_id":      user.ID,
			"first_name":   user.FirstName,
			"username":     user.Username,
			"photo_url":    user.PhotoURL,
			"profile_url":  user.ProfileURL,
			"confirmed_at": now,
		},
	}
	_, err := r.botAuthSessionsCol.UpdateOne(ctx, filter, update)
	return err
}
