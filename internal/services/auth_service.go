package services

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	randv2 "math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/pbkdf2"

	"streamgo/internal/config"
	"streamgo/internal/models"
	"streamgo/internal/repository"
)

var (
	usernameRegex   = regexp.MustCompile(`^[a-z0-9_.]{3,32}$`)
	tgUsernameRegex = regexp.MustCompile(`(?i)^[a-z0-9_]{5,32}$`)
)

// AuthService handles Telegram WebApp/Widget/OAuth authentication and user management.
type AuthService struct {
	cfg        *config.Config
	userRepo   repository.UserRepository
	accessRepo repository.AccessControlRepository

	botUsernameMu      sync.RWMutex
	cachedBotUsername  string
	botUsernameFetched time.Time

	jwksMu      sync.RWMutex
	jwksKeys    map[string]crypto.PublicKey
	jwksFetched time.Time
}

// NewAuthService creates a new AuthService instance.
func NewAuthService(cfg *config.Config, userRepo repository.UserRepository) *AuthService {
	return &AuthService{
		cfg:      cfg,
		userRepo: userRepo,
		jwksKeys: make(map[string]crypto.PublicKey),
	}
}

// SetAccessControlRepository assigns an access control repository for registration policy enforcement.
func (s *AuthService) SetAccessControlRepository(repo repository.AccessControlRepository) {
	s.accessRepo = repo
}

func (s *AuthService) getSecretCandidates() [][]byte {
	candidates := make([][]byte, 0, 3)
	if s.cfg.SecretKey != "" {
		candidates = append(candidates, []byte(s.cfg.SecretKey))
	}
	if s.cfg.BotToken != "" {
		candidates = append(candidates, []byte(s.cfg.BotToken))
	}
	candidates = append(candidates, []byte("SHA234567JDNKDNSNNFNDKSMSERTYUWERTY"))
	return candidates
}

func (s *AuthService) primarySecret() []byte {
	if s.cfg.SecretKey != "" {
		return []byte(s.cfg.SecretKey)
	}
	if s.cfg.BotToken != "" {
		return []byte(s.cfg.BotToken)
	}
	return []byte("SHA234567JDNKDNSNNFNDKSMSERTYUWERTY")
}

// CanonUsername validates and returns lowercase username if valid.
func CanonUsername(value string) string {
	s := strings.TrimSpace(strings.ToLower(value))
	if !usernameRegex.MatchString(s) {
		return ""
	}
	return s
}

// TgUserpicURL returns userpic URL if username matches Telegram format.
func TgUserpicURL(username string) string {
	u := strings.TrimSpace(strings.TrimPrefix(username, "@"))
	if !tgUsernameRegex.MatchString(u) {
		return ""
	}
	return fmt.Sprintf("https://t.me/i/userpic/320/%s.jpg", u)
}

// HashPassword hashes a password using PBKDF2-SHA256 matching Python StreamXBot.
func (s *AuthService) HashPassword(password string) (*models.PasswordHash, error) {
	if strings.TrimSpace(password) == "" {
		return nil, errors.New("password is required")
	}

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	const iterations = 200000
	dk := pbkdf2.Key([]byte(password), salt, iterations, 32, sha256.New)

	return &models.PasswordHash{
		Algo:       "pbkdf2_sha256",
		Salt:       base64.RawURLEncoding.EncodeToString(salt),
		Iterations: iterations,
		Hash:       base64.RawURLEncoding.EncodeToString(dk),
	}, nil
}

// VerifyPassword verifies a password against the stored PBKDF2 hash.
func (s *AuthService) VerifyPassword(password string, stored *models.PasswordHash) bool {
	if stored == nil || stored.Algo != "pbkdf2_sha256" {
		return false
	}

	salt, err := base64.RawURLEncoding.DecodeString(stored.Salt)
	if err != nil {
		if s2, err2 := base64.URLEncoding.DecodeString(stored.Salt); err2 == nil {
			salt = s2
		} else {
			return false
		}
	}

	expected, err := base64.RawURLEncoding.DecodeString(stored.Hash)
	if err != nil {
		if e2, err2 := base64.URLEncoding.DecodeString(stored.Hash); err2 == nil {
			expected = e2
		} else {
			return false
		}
	}

	if len(salt) == 0 || stored.Iterations <= 0 || len(expected) == 0 {
		return false
	}

	dk := pbkdf2.Key([]byte(password), salt, stored.Iterations, len(expected), sha256.New)
	return subtle.ConstantTimeCompare(dk, expected) == 1
}

// ValidateTelegramInitData verifies the HMAC-SHA256 signature of Telegram WebApp initData.
func (s *AuthService) ValidateTelegramInitData(initData string) (*models.User, error) {
	initData = strings.TrimSpace(initData)
	if initData == "" {
		return nil, errors.New("init_data is required")
	}

	values, err := url.ParseQuery(initData)
	if err != nil {
		return nil, fmt.Errorf("invalid init_data query string: %w", err)
	}

	receivedHash := values.Get("hash")
	if receivedHash == "" {
		return nil, errors.New("hash is missing from init_data")
	}
	values.Del("hash")

	// Sort keys alphabetically
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var checkPairs []string
	for _, k := range keys {
		checkPairs = append(checkPairs, fmt.Sprintf("%s=%s", k, values.Get(k)))
	}
	dataCheckString := strings.Join(checkPairs, "\n")

	botToken := strings.TrimSpace(s.cfg.BotToken)
	if botToken == "" {
		return nil, errors.New("BOT_TOKEN is not configured")
	}

	// Telegram WebApp Secret Key = HMAC-SHA256("WebAppData", botToken)
	h := hmac.New(sha256.New, []byte("WebAppData"))
	h.Write([]byte(botToken))
	secretKey := h.Sum(nil)

	// Calculated Hash = HMAC-SHA256(secretKey, dataCheckString)
	hData := hmac.New(sha256.New, secretKey)
	hData.Write([]byte(dataCheckString))
	calculatedHash := hex.EncodeToString(hData.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(calculatedHash), []byte(receivedHash)) != 1 {
		return nil, errors.New("invalid Telegram signature")
	}

	userJSON := values.Get("user")
	if userJSON == "" {
		return nil, errors.New("user payload missing from init_data")
	}

	var tgPayload struct {
		ID        int64  `json:"id"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Username  string `json:"username"`
		PhotoURL  string `json:"photo_url"`
	}
	if err := json.Unmarshal([]byte(userJSON), &tgPayload); err != nil {
		return nil, fmt.Errorf("failed to parse user json: %w", err)
	}
	if tgPayload.ID <= 0 {
		return nil, errors.New("invalid user id in init_data")
	}

	name := strings.TrimSpace(tgPayload.FirstName)
	if tgPayload.LastName != "" {
		name = strings.TrimSpace(name + " " + tgPayload.LastName)
	}

	profileURL := tgPayload.PhotoURL
	if profileURL == "" && tgPayload.Username != "" {
		profileURL = TgUserpicURL(tgPayload.Username)
	}

	return &models.User{
		ID:         tgPayload.ID,
		UserID:     tgPayload.ID,
		FirstName:  name,
		Username:   tgPayload.Username,
		PhotoURL:   tgPayload.PhotoURL,
		ProfileURL: profileURL,
		Telegram: &models.TelegramInfo{
			ID:       tgPayload.ID,
			Username: tgPayload.Username,
		},
		Status: "active",
	}, nil
}

// VerifyTelegramWidgetData verifies HMAC of Telegram Login Widget data.
func (s *AuthService) VerifyTelegramWidgetData(fields map[string]string) bool {
	botToken := strings.TrimSpace(s.cfg.BotToken)
	if botToken == "" {
		return false
	}

	receivedHash := fields["hash"]
	if receivedHash == "" {
		return false
	}

	authDateStr := fields["auth_date"]
	authDate, _ := strconv.ParseInt(authDateStr, 10, 64)
	if authDate > 0 && time.Now().Unix()-authDate > 86400 {
		return false
	}

	allowed := map[string]bool{
		"auth_date":  true,
		"first_name": true,
		"id":         true,
		"last_name":  true,
		"photo_url":  true,
		"username":   true,
	}

	var keys []string
	for k := range fields {
		if allowed[k] && fields[k] != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var items []string
	for _, k := range keys {
		items = append(items, fmt.Sprintf("%s=%s", k, fields[k]))
	}
	dataCheckString := strings.Join(items, "\n")

	secretKey := sha256.Sum256([]byte(botToken))
	h := hmac.New(sha256.New, secretKey[:])
	h.Write([]byte(dataCheckString))
	calculated := hex.EncodeToString(h.Sum(nil))

	return subtle.ConstantTimeCompare([]byte(strings.ToLower(calculated)), []byte(strings.ToLower(receivedHash))) == 1
}

// ValidateTelegramWidget verifies callback data from Telegram Login Widget.
func (s *AuthService) ValidateTelegramWidget(req models.TelegramWidgetRequest) (*models.User, error) {
	if req.ID <= 0 || req.Hash == "" {
		return nil, errors.New("invalid widget payload")
	}

	fields := map[string]string{
		"id":        strconv.FormatInt(req.ID, 10),
		"auth_date": strconv.FormatInt(req.AuthDate, 10),
		"hash":      req.Hash,
	}
	if req.FirstName != "" {
		fields["first_name"] = req.FirstName
	}
	if req.LastName != "" {
		fields["last_name"] = req.LastName
	}
	if req.Username != "" {
		fields["username"] = req.Username
	}
	if req.PhotoURL != "" {
		fields["photo_url"] = req.PhotoURL
	}

	if !s.VerifyTelegramWidgetData(fields) {
		return nil, errors.New("invalid widget signature")
	}

	name := strings.TrimSpace(req.FirstName)
	if req.LastName != "" {
		name = strings.TrimSpace(name + " " + req.LastName)
	}

	profileURL := req.PhotoURL
	if profileURL == "" && req.Username != "" {
		profileURL = TgUserpicURL(req.Username)
	}

	return &models.User{
		ID:         req.ID,
		UserID:     req.ID,
		FirstName:  name,
		Username:   req.Username,
		PhotoURL:   req.PhotoURL,
		ProfileURL: profileURL,
		Telegram: &models.TelegramInfo{
			ID:       req.ID,
			Username: req.Username,
		},
		Status: "active",
	}, nil
}

// CreateToken creates a signed token interoperable with Python StreamXBot.
func (s *AuthService) CreateToken(u *models.User) (string, error) {
	now := time.Now().Unix()
	exp := now + (365 * 24 * 60 * 60) // 1 year

	payload := map[string]any{
		"uid":         u.ID,
		"user_id":     u.ID,
		"userid":      u.ID,
		"iat":         now,
		"exp":         exp,
		"first_name":  u.FirstName,
		"photo_url":   u.PhotoURL,
		"profile_url": u.ProfileURL,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadBytes)
	secret := s.primarySecret()

	h := hmac.New(sha256.New, secret)
	h.Write([]byte(payloadB64))
	sigB64 := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	return fmt.Sprintf("v1.%s.%s", payloadB64, sigB64), nil
}

// CreateGuestToken creates a signed token with uid="__api__" for guest/API sessions.
func (s *AuthService) CreateGuestToken() (string, error) {
	now := time.Now().Unix()
	exp := now + (365 * 24 * 60 * 60) // 1 year

	payload := map[string]any{
		"uid":         "__api__",
		"user_id":     "__api__",
		"userid":      "__api__",
		"iat":         now,
		"exp":         exp,
		"first_name":  "Guest",
		"photo_url":   "",
		"profile_url": "",
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadBytes)
	secret := s.primarySecret()

	h := hmac.New(sha256.New, secret)
	h.Write([]byte(payloadB64))
	sigB64 := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	return fmt.Sprintf("v1.%s.%s", payloadB64, sigB64), nil
}

// VerifyTokenClaims parses and verifies an auth token, returning TokenClaims.
// Supports both regular user tokens and guest tokens (uid="__api__").
func (s *AuthService) VerifyTokenClaims(rawToken string) (*models.TokenClaims, error) {
	token := strings.TrimSpace(rawToken)
	if token == "" {
		return nil, errors.New("missing auth token")
	}

	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid auth token format")
	}

	// 1. Format v1.{payload_b64}.{sig_b64}
	if parts[0] == "v1" {
		payloadB64 := parts[1]
		sigB64 := parts[2]

		matched := false
		candidates := s.getSecretCandidates()
		for _, cand := range candidates {
			h := hmac.New(sha256.New, cand)
			h.Write([]byte(payloadB64))
			expectedSig := base64.RawURLEncoding.EncodeToString(h.Sum(nil))
			if subtle.ConstantTimeCompare([]byte(expectedSig), []byte(sigB64)) == 1 {
				matched = true
				break
			}
		}

		if !matched {
			return nil, errors.New("invalid token signature")
		}

		payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadB64)
		if err != nil {
			return nil, errors.New("malformed token payload")
		}

		var rawMap map[string]any
		if err := json.Unmarshal(payloadBytes, &rawMap); err != nil {
			return nil, errors.New("invalid token payload json")
		}

		claims := &models.TokenClaims{}

		// Check expiry
		if expVal, ok := rawMap["exp"].(float64); ok && expVal > 0 {
			if float64(time.Now().Unix()) > expVal {
				return nil, errors.New("auth token expired")
			}
			claims.Exp = int64(expVal)
		}

		// Check UID / UserID
		uidRaw := rawMap["uid"]
		if uidRaw == nil {
			uidRaw = rawMap["user_id"]
		}

		if uidStr, ok := uidRaw.(string); ok && (uidStr == "__api__" || uidStr == "guest") {
			claims.IsGuest = true
			claims.UserID = 0
			claims.FirstName = "Guest"
			return claims, nil
		}

		var uid int64
		switch v := uidRaw.(type) {
		case float64:
			uid = int64(v)
		case string:
			uid, _ = strconv.ParseInt(v, 10, 64)
		}

		if uid <= 0 {
			return nil, errors.New("invalid user id in token")
		}

		claims.UserID = uid
		if fn, ok := rawMap["first_name"].(string); ok {
			claims.FirstName = fn
		}
		if un, ok := rawMap["username"].(string); ok {
			claims.Username = un
		}
		if pu, ok := rawMap["photo_url"].(string); ok {
			claims.PhotoURL = pu
		}
		if pru, ok := rawMap["profile_url"].(string); ok {
			claims.ProfileURL = pru
		}

		return claims, nil
	}

	// 2. Standard JWT fallback: header.payload.signature
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		if pb, err2 := base64.URLEncoding.DecodeString(parts[1]); err2 == nil {
			payloadBytes = pb
		} else {
			return nil, errors.New("malformed jwt payload")
		}
	}

	var rawMap map[string]any
	if err := json.Unmarshal(payloadBytes, &rawMap); err != nil {
		return nil, errors.New("invalid jwt payload json")
	}

	claims := &models.TokenClaims{}
	if expVal, ok := rawMap["exp"].(float64); ok && expVal > 0 {
		if float64(time.Now().Unix()) > expVal {
			return nil, errors.New("jwt token expired")
		}
		claims.Exp = int64(expVal)
	}

	uidRaw := rawMap["uid"]
	if uidRaw == nil {
		uidRaw = rawMap["user_id"]
	}

	if uidStr, ok := uidRaw.(string); ok && (uidStr == "__api__" || uidStr == "guest") {
		claims.IsGuest = true
		claims.UserID = 0
		claims.FirstName = "Guest"
		return claims, nil
	}

	var uid int64
	switch v := uidRaw.(type) {
	case float64:
		uid = int64(v)
	case string:
		uid, _ = strconv.ParseInt(v, 10, 64)
	}

	if uid <= 0 {
		return nil, errors.New("invalid user id in jwt")
	}

	claims.UserID = uid
	return claims, nil
}

// VerifyToken verifies token signature and returns the user ID (or 0 for guest tokens).
func (s *AuthService) VerifyToken(rawToken string) (int64, error) {
	claims, err := s.VerifyTokenClaims(rawToken)
	if err != nil {
		return 0, err
	}
	return claims.UserID, nil
}


// AuthenticateTelegram validates Telegram initData, saves user, and creates token.
func (s *AuthService) AuthenticateTelegram(ctx context.Context, initData string) (*models.AuthResponse, error) {
	u, err := s.ValidateTelegramInitData(initData)
	if err != nil {
		return nil, err
	}

	savedUser, err := s.userRepo.UpsertUser(ctx, u)
	if err != nil {
		return nil, err
	}

	token, err := s.CreateToken(savedUser)
	if err != nil {
		return nil, err
	}

	return &models.AuthResponse{
		OK:         true,
		Token:      token,
		UserID:     savedUser.ID,
		FirstName:  savedUser.FirstName,
		Username:   savedUser.Username,
		PhotoURL:   savedUser.PhotoURL,
		ProfileURL: savedUser.ProfileURL,
		User:       savedUser,
	}, nil
}

// AuthenticateWidget validates Telegram Widget login, saves user, and creates token.
func (s *AuthService) AuthenticateWidget(ctx context.Context, req models.TelegramWidgetRequest) (*models.AuthResponse, error) {
	u, err := s.ValidateTelegramWidget(req)
	if err != nil {
		return nil, err
	}

	savedUser, err := s.userRepo.UpsertUser(ctx, u)
	if err != nil {
		return nil, err
	}

	token, err := s.CreateToken(savedUser)
	if err != nil {
		return nil, err
	}

	return &models.AuthResponse{
		OK:         true,
		Token:      token,
		UserID:     savedUser.ID,
		FirstName:  savedUser.FirstName,
		Username:   savedUser.Username,
		PhotoURL:   savedUser.PhotoURL,
		ProfileURL: savedUser.ProfileURL,
		User:       savedUser,
	}, nil
}

// VerifyGuestPassword verifies password against GUEST_PASSWORD in config or owner_password in MongoDB auth_config.
func (s *AuthService) VerifyGuestPassword(ctx context.Context, password string) bool {
	pwd := strings.TrimSpace(password)
	if pwd == "" {
		return false
	}

	// 1. Direct env GUEST_PASSWORD check
	if s.cfg.GuestPassword != "" && pwd == s.cfg.GuestPassword {
		return true
	}

	// 2. Database owner_password check
	stored, err := s.userRepo.GetOwnerPassword(ctx)
	if err == nil && stored != nil {
		if s.VerifyPassword(pwd, stored) {
			return true
		}
	}

	return false
}

// GetSetupStatus checks whether owner/server password has been configured.
func (s *AuthService) GetSetupStatus(ctx context.Context) (*models.SetupStatusResponse, error) {
	stored, err := s.userRepo.GetOwnerPassword(ctx)
	if err != nil {
		return nil, err
	}
	configured := (stored != nil) || (s.cfg.GuestPassword != "")
	needsSetup := !configured

	var ownerID *int64
	if len(s.cfg.OwnerIDs) > 0 && s.cfg.OwnerIDs[0] > 0 {
		oid := s.cfg.OwnerIDs[0]
		ownerID = &oid
	}

	return &models.SetupStatusResponse{
		OK:         true,
		Configured: configured,
		NeedsSetup: needsSetup,
		OwnerID:    ownerID,
	}, nil
}

// SetupOwnerPassword creates initial owner password and returns a guest/API token.
func (s *AuthService) SetupOwnerPassword(ctx context.Context, password string) (string, error) {
	status, err := s.GetSetupStatus(ctx)
	if err == nil && status.Configured {
		return "", errors.New("setup already completed")
	}

	pwd := strings.TrimSpace(password)
	if len(pwd) < 6 {
		return "", errors.New("password must be at least 6 characters")
	}

	pwdHash, err := s.HashPassword(pwd)
	if err != nil {
		return "", err
	}

	var updatedBy int64
	if len(s.cfg.OwnerIDs) > 0 {
		updatedBy = s.cfg.OwnerIDs[0]
	}

	if err := s.userRepo.SetOwnerPassword(ctx, pwdHash, updatedBy); err != nil {
		return "", fmt.Errorf("failed to save owner password: %w", err)
	}

	return s.CreateGuestToken()
}

// LoginWithServerPassword authenticates with server/guest password and returns guest token.
func (s *AuthService) LoginWithServerPassword(ctx context.Context, password string) (string, error) {
	if !s.VerifyGuestPassword(ctx, password) {
		return "", errors.New("invalid credentials")
	}
	return s.CreateGuestToken()
}

// ChangeOwnerPassword updates the server password.
func (s *AuthService) ChangeOwnerPassword(ctx context.Context, password string, userID int64) error {
	pwd := strings.TrimSpace(password)
	if len(pwd) < 6 {
		return errors.New("password must be at least 6 characters")
	}

	pwdHash, err := s.HashPassword(pwd)
	if err != nil {
		return err
	}

	return s.userRepo.SetOwnerPassword(ctx, pwdHash, userID)
}

// LoginWithPassword authenticates a user with username and password, or server/guest password.
func (s *AuthService) LoginWithPassword(ctx context.Context, username, password string) (*models.AuthResponse, error) {
	// If logging in as guest with server/guest password
	if (strings.EqualFold(username, "guest") || strings.TrimSpace(username) == "") && s.VerifyGuestPassword(ctx, password) {
		token, err := s.CreateGuestToken()
		if err != nil {
			return nil, err
		}
		return &models.AuthResponse{
			OK:        true,
			Token:     token,
			UserID:    0,
			FirstName: "Guest",
			Username:  "guest",
		}, nil
	}

	canon := CanonUsername(username)
	if canon == "" {
		return nil, errors.New("invalid username")
	}

	user, err := s.userRepo.GetUserByUsername(ctx, canon)
	if err != nil {
		return nil, err
	}
	if user == nil || user.Password == nil {
		return nil, errors.New("invalid credentials")
	}

	if !s.VerifyPassword(password, user.Password) {
		return nil, errors.New("invalid credentials")
	}

	token, err := s.CreateToken(user)
	if err != nil {
		return nil, err
	}

	return &models.AuthResponse{
		OK:         true,
		Token:      token,
		UserID:     user.ID,
		FirstName:  user.FirstName,
		Username:   user.Username,
		PhotoURL:   user.PhotoURL,
		ProfileURL: user.ProfileURL,
		User:       user,
	}, nil
}

// TgLogin authenticates via Telegram WebApp initData and allows optional username/password configuration.
func (s *AuthService) TgLogin(ctx context.Context, req models.TgLoginRequest) (*models.AuthResponse, error) {
	user, err := s.ValidateTelegramInitData(req.InitData)
	if err != nil {
		return nil, err
	}

	if req.Username != "" {
		canon := CanonUsername(req.Username)
		if canon == "" {
			return nil, errors.New("invalid username")
		}
		existing, err := s.userRepo.GetUserByUsername(ctx, canon)
		if err != nil {
			return nil, err
		}
		if existing != nil && existing.ID != user.ID {
			return nil, errors.New("username already taken")
		}
		user.Username = canon
	}

	if req.Password != "" {
		pwdHash, err := s.HashPassword(req.Password)
		if err != nil {
			return nil, err
		}
		user.Password = pwdHash
	}

	savedUser, err := s.userRepo.UpsertUser(ctx, user)
	if err != nil {
		return nil, err
	}

	token, err := s.CreateToken(savedUser)
	if err != nil {
		return nil, err
	}

	return &models.AuthResponse{
		OK:         true,
		Token:      token,
		UserID:     savedUser.ID,
		FirstName:  savedUser.FirstName,
		Username:   savedUser.Username,
		PhotoURL:   savedUser.PhotoURL,
		ProfileURL: savedUser.ProfileURL,
		User:       savedUser,
	}, nil
}

// RegisterAccount initiates registration via Telegram OTP or directly creates user account if UserID <= 0 or Direct is true.
func (s *AuthService) RegisterAccount(ctx context.Context, req models.RegisterRequest) (*models.AuthResponse, error) {
	canon := CanonUsername(req.Username)
	if canon == "" {
		return nil, errors.New("invalid username")
	}

	// Check global registration policy (open, invite, allowlist, closed)
	var regMode string = "open"
	if s.accessRepo != nil {
		policy, err := s.accessRepo.GetPolicy(ctx)
		if err == nil && policy != nil {
			regMode = policy.RegistrationMode
			switch strings.ToLower(policy.RegistrationMode) {
			case "closed":
				return nil, errors.New("registration_closed: New accounts cannot be created at this time")
			case "allowlist":
				if req.UserID > 0 && !s.accessRepo.IsInAllowlist(ctx, req.UserID) {
					return nil, errors.New("not_allowlisted: Your account is not on the registration allowlist")
				}
			case "invite":
				code := strings.ToUpper(strings.TrimSpace(req.InviteCode))
				if code == "" {
					return nil, errors.New("invite_required: An invite code is required to register")
				}
				inv, err := s.accessRepo.GetInvite(ctx, code)
				if err != nil || inv == nil || inv.Revoked {
					return nil, errors.New("invite_invalid: That invite code is invalid or has been revoked")
				}
				if inv.ExpiresAt != nil && float64(time.Now().Unix()) > *inv.ExpiresAt {
					return nil, errors.New("invite_expired: That invite code has expired")
				}
				if inv.MaxUses > 0 && inv.UsedCount >= inv.MaxUses {
					return nil, errors.New("invite_exhausted: That invite code has already reached its maximum uses")
				}
				// Atomic consumption happens immediately for direct registration; for OTP registration, consumed on validate
				if req.UserID <= 0 {
					_ = s.accessRepo.ConsumeInvite(ctx, code)
				}
			}

			// Required chats membership check
			if policy.EnforceMembership && len(policy.RequiredChats) > 0 && req.UserID > 0 && s.cfg.BotToken != "" {
				if !s.accessRepo.IsInBypass(ctx, req.UserID) {
					for _, chat := range policy.RequiredChats {
						ok, err := s.CheckTelegramChatMember(ctx, chat.ChatID, req.UserID)
						if err != nil || !ok {
							return nil, errors.New("membership_required: Join the required Telegram chat before registering")
						}
					}
				}
			}
		}
	}

	// 1. Direct manual user creation (when UserID <= 0)
	if req.UserID <= 0 {
		existingUsername, err := s.userRepo.GetUserByUsername(ctx, canon)
		if err != nil {
			return nil, err
		}
		if existingUsername != nil {
			return nil, errors.New("username already taken")
		}

		pwdHash, err := s.HashPassword(req.Password)
		if err != nil {
			return nil, err
		}

		userID := req.UserID
		if userID <= 0 {
			// Generate unique 64-bit user ID (timestamp millis + random offset)
			userID = time.Now().UnixNano()/1e6 + int64(randv2.IntN(10000))
		}

		firstName := req.Username

		now := float64(time.Now().Unix())
		var regVia map[string]any
		if regMode != "" {
			regVia = map[string]any{
				"mode": regMode,
			}
			if req.InviteCode != "" {
				regVia["invite_code"] = strings.ToUpper(strings.TrimSpace(req.InviteCode))
			}
		}

		newUser := &models.User{
			ID:            userID,
			UserID:        userID,
			Username:      canon,
			FirstName:     firstName,
			Password:      pwdHash,
			Status:        "active",
			RegisteredVia: regVia,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		savedUser, err := s.userRepo.UpsertUser(ctx, newUser)
		if err != nil {
			return nil, fmt.Errorf("failed to create user: %w", err)
		}

		token, err := s.CreateToken(savedUser)
		if err != nil {
			return nil, err
		}

		return &models.AuthResponse{
			OK:         true,
			Token:      token,
			UserID:     savedUser.ID,
			FirstName:  savedUser.FirstName,
			Username:   savedUser.Username,
			ProfileURL: savedUser.ProfileURL,
			PhotoURL:   savedUser.PhotoURL,
			User:       savedUser,
		}, nil
	}

	// 2. Telegram OTP flow (requires existing Telegram account & running bot)
	if strings.TrimSpace(s.cfg.BotToken) == "" {
		return nil, errors.New("Bot is not running or ONLY_API is true")
	}

	existingUser, err := s.userRepo.GetUserByID(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if existingUser != nil && existingUser.Password != nil {
		return nil, errors.New("User already registered")
	}

	existingUsername, err := s.userRepo.GetUserByUsername(ctx, canon)
	if err != nil {
		return nil, err
	}
	if existingUsername != nil && existingUsername.ID != req.UserID {
		return nil, errors.New("username already taken")
	}

	pwdHash, err := s.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	otp := fmt.Sprintf("%06d", randv2.IntN(1000000))
	now := float64(time.Now().Unix())

	firstName, tgUsername, photoURL, profileURL := s.GetTelegramProfile(ctx, req.UserID)
	if profileURL == "" && tgUsername != "" {
		profileURL = TgUserpicURL(tgUsername)
	}
	if photoURL == "" {
		photoURL = profileURL
	}

	otpDoc := &models.RegistrationOTP{
		ID:               req.UserID,
		OTP:              otp,
		Username:         canon,
		Password:         pwdHash,
		FirstName:        firstName,
		TelegramUsername: tgUsername,
		ProfileURL:       profileURL,
		PhotoURL:         photoURL,
		InviteCode:       strings.ToUpper(strings.TrimSpace(req.InviteCode)),
		CreatedAt:        now,
	}

	if err := s.userRepo.SaveRegistrationOTP(ctx, otpDoc); err != nil {
		return nil, fmt.Errorf("failed to save pending registration: %w", err)
	}

	msgText := fmt.Sprintf("Your StreamX registration OTP is: `%s`\n\nThis OTP is valid for registration.", otp)
	if err := s.SendTelegramMessage(ctx, req.UserID, msgText); err != nil {
		return nil, fmt.Errorf("failed to send OTP via Telegram bot. Have you started the bot? (%w)", err)
	}

	return &models.AuthResponse{
		OK:         true,
		UserID:     req.UserID,
		Username:   canon,
		FirstName:  firstName,
		PhotoURL:   photoURL,
		ProfileURL: profileURL,
	}, nil
}

// ValidateOTP completes user registration after verifying the OTP code.
func (s *AuthService) ValidateOTP(ctx context.Context, req models.ValidateOTPRequest) (*models.AuthResponse, error) {
	otpDoc, err := s.userRepo.GetRegistrationOTP(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if otpDoc == nil {
		return nil, errors.New("no pending registration found")
	}

	if otpDoc.OTP != strings.TrimSpace(req.OTP) {
		return nil, errors.New("invalid OTP")
	}

	// Re-check registration policy at completion time (mode may have changed while the OTP was pending)
	var regMode string = "open"
	if s.accessRepo != nil {
		policy, err := s.accessRepo.GetPolicy(ctx)
		if err == nil && policy != nil {
			regMode = policy.RegistrationMode
			switch strings.ToLower(policy.RegistrationMode) {
			case "closed":
				return nil, errors.New("registration_closed: New accounts cannot be created at this time")
			case "allowlist":
				if req.UserID > 0 && !s.accessRepo.IsInAllowlist(ctx, req.UserID) {
					return nil, errors.New("not_allowlisted: Your account is not on the registration allowlist")
				}
			case "invite":
				code := strings.ToUpper(strings.TrimSpace(otpDoc.InviteCode))
				if code == "" {
					return nil, errors.New("invite_required: An invite code is required to register")
				}
				inv, err := s.accessRepo.GetInvite(ctx, code)
				if err != nil || inv == nil || inv.Revoked {
					return nil, errors.New("invite_invalid: That invite code is invalid or has been revoked")
				}
				if inv.ExpiresAt != nil && float64(time.Now().Unix()) > *inv.ExpiresAt {
					return nil, errors.New("invite_expired: That invite code has expired")
				}
				if inv.MaxUses > 0 && inv.UsedCount >= inv.MaxUses {
					return nil, errors.New("invite_exhausted: That invite code has already reached its maximum uses")
				}
			}

			// Required chats membership check
			if policy.EnforceMembership && len(policy.RequiredChats) > 0 && req.UserID > 0 && s.cfg.BotToken != "" {
				if !s.accessRepo.IsInBypass(ctx, req.UserID) {
					for _, chat := range policy.RequiredChats {
						ok, err := s.CheckTelegramChatMember(ctx, chat.ChatID, req.UserID)
						if err != nil || !ok {
							return nil, errors.New("membership_required: Join the required Telegram chat before registering")
						}
					}
				}
			}
		}

		if otpDoc.InviteCode != "" {
			_ = s.accessRepo.ConsumeInvite(ctx, otpDoc.InviteCode)
		}
	}

	firstName := otpDoc.FirstName
	tgUsername := otpDoc.TelegramUsername
	profileURL := otpDoc.ProfileURL
	photoURL := otpDoc.PhotoURL

	// Try refreshing profile from Telegram
	fn, un, ph, pr := s.GetTelegramProfile(ctx, req.UserID)
	if fn != "" {
		firstName = fn
	}
	if un != "" {
		tgUsername = un
	}
	if ph != "" {
		photoURL = ph
	}
	if pr != "" {
		profileURL = pr
	}

	now := float64(time.Now().Unix())
	var regVia map[string]any
	if regMode != "" {
		regVia = map[string]any{
			"mode": regMode,
		}
		if otpDoc.InviteCode != "" {
			regVia["invite_code"] = otpDoc.InviteCode
		}
	}

	user := &models.User{
		ID:            req.UserID,
		UserID:        req.UserID,
		FirstName:     firstName,
		Username:      otpDoc.Username,
		PhotoURL:      photoURL,
		ProfileURL:    profileURL,
		Status:        "active",
		Password:      otpDoc.Password,
		RegisteredVia: regVia,
		Telegram: &models.TelegramInfo{
			ID:       req.UserID,
			Username: tgUsername,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	savedUser, err := s.userRepo.UpsertUser(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("failed to complete registration: %w", err)
	}

	_ = s.userRepo.DeleteRegistrationOTP(ctx, req.UserID)

	token, err := s.CreateToken(savedUser)
	if err != nil {
		return nil, err
	}

	return &models.AuthResponse{
		OK:         true,
		Token:      token,
		UserID:     savedUser.ID,
		FirstName:  savedUser.FirstName,
		Username:   savedUser.Username,
		PhotoURL:   savedUser.PhotoURL,
		ProfileURL: savedUser.ProfileURL,
		User:       savedUser,
	}, nil
}

// UpdateCredentials updates credentials for an authenticated user.
func (s *AuthService) UpdateCredentials(ctx context.Context, userID int64, username, password string) error {
	canon := CanonUsername(username)
	if canon == "" {
		return errors.New("invalid username")
	}

	existing, err := s.userRepo.GetUserByUsername(ctx, canon)
	if err != nil {
		return err
	}
	if existing != nil && existing.ID != userID {
		return errors.New("username already taken")
	}

	pwdHash, err := s.HashPassword(password)
	if err != nil {
		return err
	}

	return s.userRepo.UpdateUserCredentials(ctx, userID, canon, pwdHash)
}

// UpdateIntegrations updates integrations.discord and integrations.lastfm for an authenticated user.
func (s *AuthService) UpdateIntegrations(ctx context.Context, userID int64, req models.IntegrationsUpdateRequest) (*models.UserIntegrations, error) {
	return s.userRepo.UpdateIntegrations(ctx, userID, req)
}

// SendTelegramMessage sends a message to a user via Telegram Bot HTTP API.
func (s *AuthService) SendTelegramMessage(ctx context.Context, chatID int64, text string) error {
	botToken := strings.TrimSpace(s.cfg.BotToken)
	if botToken == "" {
		return errors.New("bot token not configured")
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	payload := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Description string `json:"description"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return fmt.Errorf("telegram API error %d: %s", resp.StatusCode, errResp.Description)
	}

	return nil
}

// CheckTelegramChatMember checks whether a user is an active member/admin of a required Telegram chat.
func (s *AuthService) CheckTelegramChatMember(ctx context.Context, chatID int64, userID int64) (bool, error) {
	botToken := strings.TrimSpace(s.cfg.BotToken)
	if botToken == "" {
		return true, nil
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getChatMember?chat_id=%d&user_id=%d", botToken, chatID, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return false, err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var tgResp struct {
		OK     bool `json:"ok"`
		Result struct {
			Status string `json:"status"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tgResp); err != nil {
		return false, err
	}

	if !tgResp.OK {
		return false, nil
	}

	switch tgResp.Result.Status {
	case "member", "administrator", "creator", "restricted":
		return true, nil
	default:
		return false, nil
	}
}

// GetTelegramProfile retrieves user's first name, username, and profile pictures.
func (s *AuthService) GetTelegramProfile(ctx context.Context, userID int64) (firstName, username, photoURL, profileURL string) {
	botToken := strings.TrimSpace(s.cfg.BotToken)
	if botToken == "" {
		return "", "", "", ""
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getChat?chat_id=%d", botToken, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", "", "", ""
	}

	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", ""
	}
	defer resp.Body.Close()

	var tgResp struct {
		OK     bool `json:"ok"`
		Result struct {
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
			Username  string `json:"username"`
			Photo     *struct {
				SmallFileID string `json:"small_file_id"`
				BigFileID   string `json:"big_file_id"`
			} `json:"photo"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tgResp); err == nil && tgResp.OK {
		name := strings.TrimSpace(tgResp.Result.FirstName)
		if tgResp.Result.LastName != "" {
			name = strings.TrimSpace(name + " " + tgResp.Result.LastName)
		}
		firstName = name
		username = tgResp.Result.Username
		if username != "" {
			profileURL = TgUserpicURL(username)
			photoURL = profileURL
		}
		// If username didn't produce a photo URL or user has no username, resolve Telegram photo file
		if (profileURL == "" || photoURL == "") && tgResp.Result.Photo != nil {
			fileID := tgResp.Result.Photo.BigFileID
			if fileID == "" {
				fileID = tgResp.Result.Photo.SmallFileID
			}
			if fileID != "" {
				fileURL := s.getTelegramFilePath(ctx, fileID)
				if fileURL != "" {
					if profileURL == "" {
						profileURL = fileURL
					}
					if photoURL == "" {
						photoURL = fileURL
					}
				}
			}
		}
	}

	// Fallback to database user record if available
	if (profileURL == "" || photoURL == "") && s.userRepo != nil && userID > 0 {
		if existing, err := s.userRepo.GetUserByID(ctx, userID); err == nil && existing != nil {
			if profileURL == "" {
				profileURL = existing.ProfileURL
			}
			if photoURL == "" {
				photoURL = existing.PhotoURL
			}
		}
	}

	if profileURL != "" && photoURL == "" {
		photoURL = profileURL
	}
	if photoURL != "" && profileURL == "" {
		profileURL = photoURL
	}

	return
}

func (s *AuthService) getTelegramFilePath(ctx context.Context, fileID string) string {
	botToken := strings.TrimSpace(s.cfg.BotToken)
	if botToken == "" || fileID == "" {
		return ""
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getFile?file_id=%s", botToken, fileID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return ""
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var getFileResp struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&getFileResp); err == nil && getFileResp.OK && getFileResp.Result.FilePath != "" {
		return fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", botToken, getFileResp.Result.FilePath)
	}
	return ""
}

// GetTelegramClientID returns the configured Telegram OIDC client ID or extracts it from BOT_TOKEN.
func (s *AuthService) GetTelegramClientID() string {
	if s.cfg.TelegramOIDCClientID != "" {
		return s.cfg.TelegramOIDCClientID
	}
	parts := strings.Split(s.cfg.BotToken, ":")
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return ""
}

// GetBotUsername queries Telegram Bot API to get the bot's @username with in-memory caching.
func (s *AuthService) GetBotUsername(ctx context.Context) string {
	s.botUsernameMu.RLock()
	if s.cachedBotUsername != "" && time.Since(s.botUsernameFetched) < time.Hour {
		name := s.cachedBotUsername
		s.botUsernameMu.RUnlock()
		return name
	}
	s.botUsernameMu.RUnlock()

	botToken := strings.TrimSpace(s.cfg.BotToken)
	if botToken == "" {
		return ""
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", botToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return ""
	}

	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var getMeResp struct {
		OK     bool `json:"ok"`
		Result struct {
			Username string `json:"username"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&getMeResp); err == nil && getMeResp.OK {
		s.botUsernameMu.Lock()
		s.cachedBotUsername = getMeResp.Result.Username
		s.botUsernameFetched = time.Now()
		s.botUsernameMu.Unlock()
		return getMeResp.Result.Username
	}

	return ""
}

// UpdateFCMToken sets the user's FCM push token.
func (s *AuthService) UpdateFCMToken(ctx context.Context, userID int64, fcmToken string) error {
	if userID <= 0 {
		return errors.New("unauthorized")
	}
	return s.userRepo.UpdateFCMToken(ctx, userID, fcmToken)
}

// CreateBotSession creates a temporary session for direct Telegram bot login matching Python StreamXBot.
func (s *AuthService) CreateBotSession(ctx context.Context, inviteCode string) (map[string]any, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	sessionID := "auth_" + base64.RawURLEncoding.EncodeToString(b)
	now := float64(time.Now().Unix())
	expiresAt := now + 600

	var invCode string
	if inviteCode != "" {
		invCode = strings.ToUpper(strings.TrimSpace(inviteCode))
	}

	session := &models.BotAuthSession{
		ID:         sessionID,
		Status:     "pending",
		InviteCode: invCode,
		CreatedAt:  now,
		ExpiresAt:  expiresAt,
	}

	if err := s.userRepo.SaveBotAuthSession(ctx, session); err != nil {
		return nil, err
	}

	botUsername := s.GetBotUsername(ctx)
	tgURL := ""
	webURL := ""
	if botUsername != "" {
		tgURL = fmt.Sprintf("tg://resolve?domain=%s&start=%s", botUsername, sessionID)
		webURL = fmt.Sprintf("https://t.me/%s?start=%s", botUsername, sessionID)
	}

	return map[string]any{
		"ok":           true,
		"session_id":   sessionID,
		"bot_username": botUsername,
		"tg_url":       tgURL,
		"web_url":      webURL,
	}, nil
}

// CheckBotSessionStatus checks whether the user has authorized via Telegram bot.
func (s *AuthService) CheckBotSessionStatus(ctx context.Context, sessionID string) (map[string]any, error) {
	session, err := s.userRepo.GetBotAuthSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return map[string]any{
			"ok":     false,
			"status": "not_found",
		}, nil
	}

	now := float64(time.Now().Unix())
	if session.ExpiresAt < now && session.Status == "pending" {
		return map[string]any{
			"ok":     false,
			"status": "expired",
		}, nil
	}

	if session.Status == "confirmed" {
		return map[string]any{
			"ok":          true,
			"status":      "confirmed",
			"token":       session.Token,
			"user_id":     session.UserID,
			"first_name":  session.FirstName,
			"username":    session.Username,
			"photo_url":   session.PhotoURL,
			"profile_url": session.ProfileURL,
		}, nil
	}

	return map[string]any{
		"ok":     true,
		"status": "pending",
	}, nil
}

// GeneratePKCE creates a secure random code_verifier and code_challenge.
func (s *AuthService) GeneratePKCE() (codeVerifier string, codeChallenge string) {
	bytes := make([]byte, 48)
	_, _ = rand.Read(bytes)
	codeVerifier = base64.RawURLEncoding.EncodeToString(bytes)

	h := sha256.Sum256([]byte(codeVerifier))
	codeChallenge = base64.RawURLEncoding.EncodeToString(h[:])
	return
}

// StartTelegramOIDC creates an OIDC authorization session and returns the Telegram redirect URL.
func (s *AuthService) StartTelegramOIDC(ctx context.Context, redirectURI, frontendBase, destPath, inviteCode string) (string, error) {
	clientID := s.GetTelegramClientID()
	if clientID == "" {
		return "", errors.New("Telegram Client ID is not configured")
	}

	stateBytes := make([]byte, 24)
	_, _ = rand.Read(stateBytes)
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	codeVerifier, codeChallenge := s.GeneratePKCE()
	now := float64(time.Now().Unix())

	session := &models.OIDCSession{
		ID:           state,
		CodeVerifier: codeVerifier,
		RedirectURI:  redirectURI,
		FrontendBase: frontendBase,
		DestPath:     destPath,
		InviteCode:   inviteCode,
		CreatedAt:    now,
	}

	if err := s.userRepo.SaveOIDCSession(ctx, session); err != nil {
		return "", fmt.Errorf("failed to save OIDC session: %w", err)
	}

	origin := s.cfg.TelegramOIDCOrigin
	if origin == "" {
		origin = frontendBase
	}
	if origin != "" && !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://") {
		origin = "https://" + origin
	}

	v := url.Values{}
	if s.cfg.TelegramOIDCClientSecret != "" {
		v.Set("client_id", clientID)
		v.Set("bot_id", clientID)
		v.Set("origin", origin)
		v.Set("redirect_uri", redirectURI)
		v.Set("response_type", "code")
		v.Set("scope", "openid profile")
		v.Set("state", state)
		v.Set("code_challenge", codeChallenge)
		v.Set("code_challenge_method", "S256")
	} else {
		v.Set("bot_id", clientID)
		v.Set("origin", origin)
		v.Set("return_to", redirectURI)
		v.Set("request_access", "write")
	}

	authURL := fmt.Sprintf("https://oauth.telegram.org/auth?%s", v.Encode())
	return authURL, nil
}

// fetchJWKS retrieves and parses public keys from Telegram's JWKS endpoint.
func (s *AuthService) fetchJWKS(ctx context.Context) error {
	s.jwksMu.RLock()
	if len(s.jwksKeys) > 0 && time.Since(s.jwksFetched) < time.Hour {
		s.jwksMu.RUnlock()
		return nil
	}
	s.jwksMu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://oauth.telegram.org/.well-known/jwks.json", nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var jwks struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			Alg string `json:"alg"`
			N   string `json:"n"`
			E   string `json:"e"`
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("failed to decode JWKS: %w", err)
	}

	newKeys := make(map[string]crypto.PublicKey)
	for _, k := range jwks.Keys {
		if k.Kty == "RSA" && k.N != "" && k.E != "" {
			nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
			if err != nil {
				continue
			}
			eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
			if err != nil {
				continue
			}
			var eInt int
			for _, b := range eBytes {
				eInt = (eInt << 8) | int(b)
			}
			newKeys[k.Kid] = &rsa.PublicKey{
				N: new(big.Int).SetBytes(nBytes),
				E: eInt,
			}
		} else if k.Kty == "EC" && k.X != "" && k.Y != "" {
			xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
			if err != nil {
				continue
			}
			yBytes, err := base64.RawURLEncoding.DecodeString(k.Y)
			if err != nil {
				continue
			}
			var curve elliptic.Curve
			if k.Crv == "P-256" || k.Crv == "ES256" {
				curve = elliptic.P256()
			} else {
				continue
			}
			newKeys[k.Kid] = &ecdsa.PublicKey{
				Curve: curve,
				X:     new(big.Int).SetBytes(xBytes),
				Y:     new(big.Int).SetBytes(yBytes),
			}
		} else if k.Kty == "OKP" && k.Crv == "Ed25519" && k.X != "" {
			xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
			if err == nil && len(xBytes) == ed25519.PublicKeySize {
				newKeys[k.Kid] = ed25519.PublicKey(xBytes)
			}
		}
	}

	s.jwksMu.Lock()
	s.jwksKeys = newKeys
	s.jwksFetched = time.Now()
	s.jwksMu.Unlock()

	return nil
}

// ValidateTelegramIDToken decodes and verifies a Telegram ID token against Telegram's JWKS.
func (s *AuthService) ValidateTelegramIDToken(ctx context.Context, idToken string) (jwt.MapClaims, error) {
	if err := s.fetchJWKS(ctx); err != nil {
		return nil, fmt.Errorf("failed to fetch Telegram JWKS: %w", err)
	}

	token, err := jwt.Parse(idToken, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		s.jwksMu.RLock()
		pubKey, ok := s.jwksKeys[kid]
		s.jwksMu.RUnlock()
		if !ok {
			// Refresh keys once in case of key rotation
			_ = s.fetchJWKS(ctx)
			s.jwksMu.RLock()
			pubKey, ok = s.jwksKeys[kid]
			s.jwksMu.RUnlock()
		}
		if !ok {
			return nil, fmt.Errorf("JWKS key not found for kid: %s", kid)
		}
		return pubKey, nil
	}, jwt.WithIssuer("https://oauth.telegram.org"))

	if err != nil {
		return nil, fmt.Errorf("failed to verify ID token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid ID token claims")
	}

	return claims, nil
}

// ResolveTelegramUserID resolves numeric Telegram User ID from decoded OIDC claims.
func (s *AuthService) ResolveTelegramUserID(ctx context.Context, claims jwt.MapClaims) (int64, error) {
	var tgUserID int64

	// 1. Raw numeric fields
	for _, field := range []string{"id", "user_id", "telegram_id"} {
		if val, ok := claims[field]; ok {
			switch v := val.(type) {
			case float64:
				tgUserID = int64(v)
			case string:
				tgUserID, _ = strconv.ParseInt(v, 10, 64)
			}
			if tgUserID > 0 {
				break
			}
		}
	}

	prefUsername, _ := claims["preferred_username"].(string)
	prefUsername = strings.TrimSpace(prefUsername)

	// If ID is missing or a pairwise 64-bit hash (> 10^12) and we have a username, resolve from DB
	if (tgUserID <= 0 || tgUserID > 1000000000000) && prefUsername != "" {
		canon := CanonUsername(prefUsername)
		existing, _ := s.userRepo.GetUserByUsername(ctx, canon)
		if existing != nil && existing.ID > 0 && existing.ID < 1000000000000 {
			return existing.ID, nil
		}
	}

	if tgUserID > 0 {
		return tgUserID, nil
	}

	// Fallback to sub
	if subStr, ok := claims["sub"].(string); ok {
		if subID, err := strconv.ParseInt(subStr, 10, 64); err == nil && subID > 0 {
			return subID, nil
		}
	}

	return 0, errors.New("cannot extract valid Telegram user ID from claims")
}

// ExchangeOIDCCode exchanges authorization code for tokens, resolves user, and issues application token.
func (s *AuthService) ExchangeOIDCCode(ctx context.Context, code, state string) (*models.User, string, string, error) {
	session, err := s.userRepo.GetAndDeleteOIDCSession(ctx, state)
	if err != nil {
		return nil, "", "", err
	}
	if session == nil {
		return nil, "", "", errors.New("invalid_state")
	}

	if time.Now().Unix()-int64(session.CreatedAt) > 600 {
		return nil, "", "", errors.New("expired_session")
	}

	clientID := s.GetTelegramClientID()
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {session.RedirectURI},
		"client_id":     {clientID},
		"code_verifier": {session.CodeVerifier},
	}
	if s.cfg.TelegramOIDCClientSecret != "" {
		form.Set("client_secret", s.cfg.TelegramOIDCClientSecret)
	}

	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth.telegram.org/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, "", "", err
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return nil, "", "", fmt.Errorf("token exchange request failed: %w", err)
	}
	defer tokenResp.Body.Close()

	var tokenBody struct {
		IDToken          string `json:"id_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenBody); err != nil {
		return nil, "", "", fmt.Errorf("invalid token exchange response: %w", err)
	}

	if tokenBody.Error != "" {
		return nil, "", "", fmt.Errorf("%s: %s", tokenBody.Error, tokenBody.ErrorDescription)
	}
	if tokenBody.IDToken == "" {
		return nil, "", "", errors.New("missing_id_token")
	}

	claims, err := s.ValidateTelegramIDToken(ctx, tokenBody.IDToken)
	if err != nil {
		return nil, "", "", err
	}

	tgUserID, err := s.ResolveTelegramUserID(ctx, claims)
	if err != nil {
		return nil, "", "", err
	}

	name, _ := claims["name"].(string)
	username, _ := claims["preferred_username"].(string)
	picture, _ := claims["picture"].(string)

	profileURL := picture
	if profileURL == "" && username != "" {
		profileURL = TgUserpicURL(username)
	}

	user := &models.User{
		ID:         tgUserID,
		UserID:     tgUserID,
		FirstName:  strings.TrimSpace(name),
		Username:   username,
		PhotoURL:   picture,
		ProfileURL: profileURL,
		Telegram: &models.TelegramInfo{
			ID:       tgUserID,
			Username: username,
		},
		Status: "active",
	}

	savedUser, err := s.userRepo.UpsertUser(ctx, user)
	if err != nil {
		return nil, "", "", err
	}

	token, err := s.CreateToken(savedUser)
	if err != nil {
		return nil, "", "", err
	}

	destPath := session.DestPath
	if destPath == "" {
		destPath = "/"
	}

	return savedUser, token, destPath, nil
}

// ValidateOIDCIDToken handles token verification for frontend hash fragment login (/auth/telegram/validate-token).
func (s *AuthService) ValidateOIDCIDToken(ctx context.Context, idToken, inviteCode string) (*models.User, string, error) {
	claims, err := s.ValidateTelegramIDToken(ctx, idToken)
	if err != nil {
		return nil, "", err
	}

	tgUserID, err := s.ResolveTelegramUserID(ctx, claims)
	if err != nil {
		return nil, "", err
	}

	name, _ := claims["name"].(string)
	username, _ := claims["preferred_username"].(string)
	picture, _ := claims["picture"].(string)

	profileURL := picture
	if profileURL == "" && username != "" {
		profileURL = TgUserpicURL(username)
	}

	user := &models.User{
		ID:         tgUserID,
		UserID:     tgUserID,
		FirstName:  strings.TrimSpace(name),
		Username:   username,
		PhotoURL:   picture,
		ProfileURL: profileURL,
		Telegram: &models.TelegramInfo{
			ID:       tgUserID,
			Username: username,
		},
		Status: "active",
	}

	savedUser, err := s.userRepo.UpsertUser(ctx, user)
	if err != nil {
		return nil, "", err
	}

	token, err := s.CreateToken(savedUser)
	if err != nil {
		return nil, "", err
	}

	return savedUser, token, nil
}

// SetAuthCookie writes auth_token as an HTTP-only cookie.
func (s *AuthService) SetAuthCookie(w http.ResponseWriter, token string) {
	secure := s.cfg.CookieSecure

	sameSite := http.SameSiteNoneMode
	if strings.ToLower(s.cfg.CookieSameSite) == "lax" {
		sameSite = http.SameSiteLaxMode
	} else if strings.ToLower(s.cfg.CookieSameSite) == "strict" {
		sameSite = http.SameSiteStrictMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		MaxAge:   365 * 24 * 60 * 60,
	})
}

// ClearAuthCookie clears the auth_token cookie.
func (s *AuthService) ClearAuthCookie(w http.ResponseWriter) {
	secure := s.cfg.CookieSecure

	sameSite := http.SameSiteNoneMode
	if strings.ToLower(s.cfg.CookieSameSite) == "lax" {
		sameSite = http.SameSiteLaxMode
	} else if strings.ToLower(s.cfg.CookieSameSite) == "strict" {
		sameSite = http.SameSiteStrictMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		MaxAge:   -1,
	})
}

// GetUserByID returns user information by ID.
func (s *AuthService) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	return s.userRepo.GetUserByID(ctx, id)
}

// AuthenticateOrRegisterTgUser finds or creates a user by Telegram user ID and generates an auth token.
func (s *AuthService) AuthenticateOrRegisterTgUser(ctx context.Context, tgUserID int64, name, username, photoURL, inviteCode string) (*models.User, string, error) {
	now := float64(time.Now().Unix())
	user, err := s.userRepo.GetUserByID(ctx, tgUserID)
	if err != nil {
		return nil, "", err
	}

	if user == nil {
		if s.accessRepo != nil {
			policy, pErr := s.accessRepo.GetPolicy(ctx)
			if pErr == nil && policy != nil {
				switch strings.ToLower(policy.RegistrationMode) {
				case "closed":
					return nil, "", errors.New("registration is currently closed")
				case "invite":
					if inviteCode == "" {
						return nil, "", errors.New("an invite code is required to register")
					}
					inv, iErr := s.accessRepo.GetInvite(ctx, strings.ToUpper(strings.TrimSpace(inviteCode)))
					if iErr != nil || inv == nil || inv.Revoked {
						return nil, "", errors.New("invalid or revoked invite code")
					}
					if inv.ExpiresAt != nil && now > *inv.ExpiresAt {
						return nil, "", errors.New("invite code has expired")
					}
					if inv.MaxUses > 0 && inv.UsedCount >= inv.MaxUses {
						return nil, "", errors.New("invite code has reached maximum uses")
					}
					_ = s.accessRepo.ConsumeInvite(ctx, inv.Code)
				case "allowlist":
					if !s.accessRepo.IsInAllowlist(ctx, tgUserID) {
						return nil, "", errors.New("your account is not on the registration allowlist")
					}
				}
			}
		}

		user = &models.User{
			ID:           tgUserID,
			UserID:       tgUserID,
			FirstName:    name,
			Username:     CanonUsername(username),
			PhotoURL:     photoURL,
			ProfileURL:   photoURL,
			Status:       "active",
			TokenVersion: 0,
			Telegram: &models.TelegramInfo{
				ID:       tgUserID,
				Username: username,
			},
			RegisteredVia: map[string]any{
				"mode":        "telegram_bot",
				"invite_code": inviteCode,
			},
			CreatedAt: now,
			UpdatedAt: now,
		}
	} else {
		if user.Status == "locked" {
			return nil, "", errors.New("your account has been suspended")
		}
		if name != "" {
			user.FirstName = name
		}
		if username != "" && user.Username == "" {
			user.Username = CanonUsername(username)
		}
		if photoURL != "" {
			user.PhotoURL = photoURL
			user.ProfileURL = photoURL
		}
		if user.Telegram == nil {
			user.Telegram = &models.TelegramInfo{ID: tgUserID, Username: username}
		}
		user.UpdatedAt = now
	}

	savedUser, err := s.userRepo.UpsertUser(ctx, user)
	if err != nil {
		return nil, "", err
	}

	token, err := s.CreateToken(savedUser)
	if err != nil {
		return nil, "", err
	}

	return savedUser, token, nil
}

// ConfirmBotSession marks a bot auth session confirmed in the repository.
func (s *AuthService) ConfirmBotSession(ctx context.Context, sessionID, token string, user *models.User) error {
	return s.userRepo.ConfirmBotAuthSession(ctx, sessionID, token, user)
}
