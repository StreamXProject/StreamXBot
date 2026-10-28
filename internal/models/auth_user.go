package models

// TelegramInfo stores nested Telegram account information.
type TelegramInfo struct {
	ID       int64  `bson:"id,omitempty" json:"id,omitempty"`
	Username string `bson:"username,omitempty" json:"username,omitempty"`
}

// PasswordHash stores PBKDF2 password parameters matching StreamXBot.
type PasswordHash struct {
	Algo       string `bson:"algo" json:"algo"`
	Salt       string `bson:"salt" json:"salt"`
	Iterations int    `bson:"iterations" json:"iterations"`
	Hash       string `bson:"hash" json:"hash"`
}

// DiscordIntegrationSchema represents Discord Rich Presence / RPC configuration.
type DiscordIntegrationSchema struct {
	Enabled     bool    `bson:"enabled" json:"enabled"`
	Token       string  `bson:"token" json:"token"`
	Mode        string  `bson:"mode,omitempty" json:"mode,omitempty"`
	ClientID    string  `bson:"client_id,omitempty" json:"client_id,omitempty"`
	DaemonURL   string  `bson:"daemon_url,omitempty" json:"daemon_url,omitempty"`
	ShowArtwork bool    `bson:"show_artwork" json:"show_artwork"`
	UpdatedAt   float64 `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// LastfmIntegrationSchema represents Last.fm scrobbler integration configuration.
type LastfmIntegrationSchema struct {
	Enabled    bool    `bson:"enabled" json:"enabled"`
	APIKey     string  `bson:"api_key" json:"api_key"`
	APISecret  string  `bson:"api_secret" json:"api_secret"`
	SessionKey string  `bson:"session_key" json:"session_key"`
	Username   string  `bson:"username" json:"username"`
	ScrobbleAt float64 `bson:"scrobble_at" json:"scrobble_at"`
	NowPlaying bool    `bson:"now_playing" json:"now_playing"`
	UpdatedAt  float64 `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// UserIntegrations aggregates all third-party integrations for a user.
type UserIntegrations struct {
	Discord *DiscordIntegrationSchema `bson:"discord,omitempty" json:"discord,omitempty"`
	Lastfm  *LastfmIntegrationSchema  `bson:"lastfm,omitempty" json:"lastfm,omitempty"`
}

// IntegrationsUpdateRequest payload for updating Discord and Lastfm integrations.
type IntegrationsUpdateRequest struct {
	Discord *DiscordIntegrationSchema `json:"discord,omitempty"`
	Lastfm  *LastfmIntegrationSchema  `json:"lastfm,omitempty"`
}

// User represents a user record in the users collection.
type User struct {
	ID           int64             `bson:"_id" json:"id"`
	UserID       int64             `bson:"user_id,omitempty" json:"user_id,omitempty"`
	FirstName    string            `bson:"first_name,omitempty" json:"first_name,omitempty"`
	Username     string            `bson:"username,omitempty" json:"username,omitempty"`
	PhotoURL     string            `bson:"photo_url,omitempty" json:"photo_url,omitempty"`
	ProfileURL   string            `bson:"profile_url,omitempty" json:"profile_url,omitempty"`
	Status       string            `bson:"status,omitempty" json:"status,omitempty"`
	Password     *PasswordHash     `bson:"password,omitempty" json:"-"`
	TokenVersion int               `bson:"token_version,omitempty" json:"token_version,omitempty"`
	Telegram     *TelegramInfo     `bson:"telegram,omitempty" json:"telegram,omitempty"`
	Integrations *UserIntegrations `bson:"integrations,omitempty" json:"integrations,omitempty"`
	RegisteredVia any              `bson:"registered_via,omitempty" json:"registered_via,omitempty"`
	CreatedAt    float64           `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt    float64           `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// RegistrationOTP stores a pending user registration and OTP code.
type RegistrationOTP struct {
	ID               int64         `bson:"_id" json:"user_id"`
	OTP              string        `bson:"otp" json:"otp"`
	Username         string        `bson:"username" json:"username"`
	Password         *PasswordHash `bson:"password" json:"password"`
	FirstName        string        `bson:"first_name,omitempty" json:"first_name,omitempty"`
	TelegramUsername string        `bson:"telegram_username,omitempty" json:"telegram_username,omitempty"`
	ProfileURL       string        `bson:"profile_url,omitempty" json:"profile_url,omitempty"`
	PhotoURL         string        `bson:"photo_url,omitempty" json:"photo_url,omitempty"`
	InviteCode       string        `bson:"invite_code,omitempty" json:"invite_code,omitempty"`
	CreatedAt        float64       `bson:"created_at" json:"created_at"`
}

// OIDCSession stores a pending Telegram OAuth / OpenID Connect authorization session.
type OIDCSession struct {
	ID           string  `bson:"_id" json:"state"`
	CodeVerifier string  `bson:"code_verifier" json:"code_verifier"`
	RedirectURI  string  `bson:"redirect_uri" json:"redirect_uri"`
	FrontendBase string  `bson:"frontend_base" json:"frontend_base"`
	DestPath     string  `bson:"dest_path" json:"dest_path"`
	InviteCode   string  `bson:"invite_code,omitempty" json:"invite_code,omitempty"`
	CreatedAt    float64 `bson:"created_at" json:"created_at"`
}

// AuthResponse returns token and user payload upon successful authentication.
type AuthResponse struct {
	OK         bool   `json:"ok"`
	Token      string `json:"token"`
	UserID     int64  `json:"user_id,omitempty"`
	FirstName  string `json:"first_name,omitempty"`
	Username   string `json:"username,omitempty"`
	PhotoURL   string `json:"photo_url,omitempty"`
	ProfileURL string `json:"profile_url,omitempty"`
	User       *User  `json:"user,omitempty"`
}

// TelegramWebAppRequest contains raw initData string sent from Telegram WebApp.
type TelegramWebAppRequest struct {
	InitData string `json:"init_data"`
}

// TgLoginRequest contains initData and optional credentials for /auth/tg/login.
type TgLoginRequest struct {
	InitData   string `json:"init_data"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	InviteCode string `json:"invite_code,omitempty"`
}

// PasswordLoginRequest contains username and password for /auth/login.
type PasswordLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// RegisterRequest contains registration fields for /auth/register matching Python StreamXBot.
type RegisterRequest struct {
	UserID     int64  `json:"userid"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	InviteCode string `json:"invite_code,omitempty"`
}

// TokenClaims contains verified information from an auth token.
type TokenClaims struct {
	UserID       int64  `json:"user_id"`
	IsGuest      bool   `json:"is_guest"`
	FirstName    string `json:"first_name,omitempty"`
	Username     string `json:"username,omitempty"`
	PhotoURL     string `json:"photo_url,omitempty"`
	ProfileURL   string `json:"profile_url,omitempty"`
	TokenVersion int    `json:"token_version,omitempty"`
	Exp          int64  `json:"exp,omitempty"`
}

// SetupStatusResponse represents response for GET /auth/setup/status.
type SetupStatusResponse struct {
	OK         bool   `json:"ok"`
	Configured bool   `json:"configured"`
	NeedsSetup bool   `json:"needs_setup"`
	OwnerID    *int64 `json:"owner_id,omitempty"`
}

// SetupPasswordRequest represents payload for POST /auth/setup, POST /auth/password, POST /auth/password/change.
type SetupPasswordRequest struct {
	Password string `json:"password"`
}

// Password login / change request aliases matching Api/schemas/auth.py.
type OwnerPasswordLoginRequest = SetupPasswordRequest
type SetOwnerPasswordRequest = SetupPasswordRequest
type ChangeOwnerPasswordRequest = SetupPasswordRequest

// ValidateOTPRequest contains verification fields for /auth/validate.
type ValidateOTPRequest struct {
	UserID int64  `json:"userid"`
	OTP    string `json:"otp"`
}

// TelegramTokenLoginRequest contains token validation payload for /auth/telegram/validate-token.
type TelegramTokenLoginRequest struct {
	IDToken    string `json:"id_token"`
	Token      string `json:"token,omitempty"`
	InviteCode string `json:"invite_code,omitempty"`
}

// SetCookieRequest contains the token to store in an HTTP-only cookie.
type SetCookieRequest struct {
	Token string `json:"token"`
}

// SetCredentialsRequest contains updated username and password for the active user.
type SetCredentialsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// TelegramWidgetRequest contains callback parameters from Telegram Login Widget.
type TelegramWidgetRequest struct {
	ID         int64  `json:"id"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name,omitempty"`
	Username   string `json:"username,omitempty"`
	PhotoURL   string `json:"photo_url,omitempty"`
	AuthDate   int64  `json:"auth_date"`
	Hash       string `json:"hash"`
	InviteCode string `json:"invite_code,omitempty"`
}

// TelegramWidgetLoginRequest alias matching Api/schemas/auth.py.
type TelegramWidgetLoginRequest = TelegramWidgetRequest

// FCMTokenRequest payload for POST /auth/fcm-token matching Api/schemas/auth.py.
type FCMTokenRequest struct {
	FCMToken string `json:"fcm_token"`
}

// TelegramBotSessionRequest payload for POST /auth/telegram/bot-session matching Api/schemas/auth.py.
type TelegramBotSessionRequest struct {
	InviteCode string `json:"invite_code,omitempty"`
}

// BotAuthSession represents a temporary session for Telegram bot login.
type BotAuthSession struct {
	ID         string  `bson:"_id" json:"session_id"`
	Status     string  `bson:"status" json:"status"`
	InviteCode string  `bson:"invite_code,omitempty" json:"invite_code,omitempty"`
	Token      string  `bson:"token,omitempty" json:"token,omitempty"`
	UserID     int64   `bson:"user_id,omitempty" json:"user_id,omitempty"`
	FirstName  string  `bson:"first_name,omitempty" json:"first_name,omitempty"`
	Username   string  `bson:"username,omitempty" json:"username,omitempty"`
	PhotoURL   string  `bson:"photo_url,omitempty" json:"photo_url,omitempty"`
	ProfileURL string  `bson:"profile_url,omitempty" json:"profile_url,omitempty"`
	CreatedAt  float64 `bson:"created_at" json:"created_at"`
	ExpiresAt  float64 `bson:"expires_at" json:"expires_at"`
}
