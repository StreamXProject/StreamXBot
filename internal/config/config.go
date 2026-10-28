package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"streamgo/internal/logger"
)

var log = logger.New("config")

// Config holds all configuration parameters for StreamGO.
type Config struct {
	Port                     string
	Debug                    bool
	APILogs                  bool
	CorsOrigin               string
	MongoURI                 string
	DatabaseName             string
	ApiID                    int
	ApiHash                  string
	BotToken                 string
	SecretKey                string
	SessionString            string
	ChannelID                int64
	DumpChannelID            int64
	MultiClients             bool
	MultiClientTokens        []string
	FilterMode               int
	ChatTopic                string
	CollaboratorIDs          []int64
	Lyrics                   bool
	LRCLIB                   bool
	Musixmatch               bool
	TelegramOIDCClientID     string
	TelegramOIDCClientSecret string
	TelegramOIDCOrigin       string
	TelegramOIDCRedirectURI  string
	CookieSecure             bool
	CookieSameSite           string
	OwnerIDs                 []int64
	SudoUsers                []int64
	EnrichmentWorkers        int
	GuestPassword            string
	AlacCacheMaxBytes        int64
	AlacCacheMaxFiles        int

	// Cloudflare R2 / S3 Blob Storage (Optional)
	R2AccountID       string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2BucketName      string
	R2PublicURL       string
}

// Load reads configuration from .env file (if present) and environment variables.
func Load() *Config {
	// Try loading .env; ignore error if not found (e.g., in container/production)
	if err := godotenv.Load(); err != nil {
		log.Info("No .env file found, using system environment variables")
	} else {
		log.Info("Loaded configuration from .env")
	}

	debug := getEnvBool("DEBUG", false)
	logger.SetDebug(debug)

	mongoURI := strings.TrimSpace(getEnv("MONGO_URI", "mongodb://localhost:27017"))
	// Sanitize any accidental leading equals sign (e.g. MONGO_URI==...)
	mongoURI = strings.TrimPrefix(mongoURI, "=")

	multiClients := getEnvBool("MULTI_CLIENTS", false)
	var multiTokens []string
	for _, key := range []string{"MULTI_CLIENTS_1", "MULTI_CLIENTS_2", "MULTI_CLIENTS_3", "MULTI_CLIENTS_4"} {
		if val := strings.TrimSpace(os.Getenv(key)); val != "" {
			multiTokens = append(multiTokens, val)
		}
	}
	if extra := strings.TrimSpace(os.Getenv("MULTI_CLIENT_TOKENS")); extra != "" {
		for _, part := range strings.Split(extra, ",") {
			if p := strings.TrimSpace(part); p != "" {
				multiTokens = append(multiTokens, p)
			}
		}
	}
	if len(multiTokens) > 0 {
		multiClients = true
	}

	// Parse collaborator IDs from COLLABORATOR_ID and COLLABORATOR_IDS
	collabList := parseIDList(os.Getenv("COLLABORATOR_IDS"))
	if singleCollab := getEnvInt64("COLLABORATOR_ID", 0); singleCollab != 0 {
		collabList = append(collabList, singleCollab)
	}

	// Owner and Sudo users
	ownerIDs := parseIDList(os.Getenv("OWNER_ID"))
	if len(ownerIDs) == 0 {
		if oid := getEnvInt64("OWNER_ID", 0); oid != 0 {
			ownerIDs = []int64{oid}
		}
	}
	sudoUsers := parseIDList(os.Getenv("SUDO_USERS"))

	return &Config{
		Port:                     getEnv("PORT", "8000"),
		Debug:                    debug,
		APILogs:                  getEnvBool("API_LOGS", getEnvBool("HTTP_LOGS", false)),
		CorsOrigin:               getEnv("CORS_ORIGIN", "*"),
		MongoURI:                 mongoURI,
		DatabaseName:             getEnv("DATABASE_NAME", "Stream"),
		ApiID:                    getEnvInt("API_ID", 0),
		ApiHash:                  strings.TrimSpace(os.Getenv("API_HASH")),
		BotToken:                 strings.TrimSpace(os.Getenv("BOT_TOKEN")),
		SecretKey:                strings.TrimSpace(os.Getenv("SECRET_KEY")),
		SessionString:            strings.TrimSpace(os.Getenv("SESSION_STRING")),
		ChannelID:                getEnvInt64("CHANNEL_ID", 0),
		DumpChannelID:            getEnvInt64("DUMP_CHANNEL_ID", 0),
		MultiClients:             multiClients,
		MultiClientTokens:        multiTokens,
		FilterMode:               getEnvInt("FILTER_MODE", 0),
		ChatTopic:                strings.TrimSpace(getEnv("CHAT_TOPIC", "all")),
		CollaboratorIDs:          collabList,
		Lyrics:                   getEnvBool("LYRICS", true),
		LRCLIB:                   getEnvBool("LRCLIB", false),
		Musixmatch:               getEnvBool("MUSIXMATCH", true),
		TelegramOIDCClientID:     strings.TrimSpace(os.Getenv("TELEGRAM_OIDC_CLIENT_ID")),
		TelegramOIDCClientSecret: strings.TrimSpace(os.Getenv("TELEGRAM_OIDC_CLIENT_SECRET")),
		TelegramOIDCOrigin:       strings.TrimSpace(os.Getenv("TELEGRAM_OIDC_ORIGIN")),
		TelegramOIDCRedirectURI:  strings.TrimSpace(os.Getenv("TELEGRAM_OIDC_REDIRECT_URI")),
		CookieSecure:             getEnvBool("COOKIE_SECURE", false),
		CookieSameSite:           getEnv("COOKIE_SAMESITE", "lax"),
		OwnerIDs:                 ownerIDs,
		SudoUsers:                sudoUsers,
		EnrichmentWorkers:        getEnvInt("ENRICHMENT_WORKERS", 2),
		GuestPassword:            getEnv("GUEST_PASSWORD", ""),
		AlacCacheMaxBytes:        getEnvInt64("ALAC_CACHE_MAX_BYTES", 5*1024*1024*1024),
		AlacCacheMaxFiles:        getEnvInt("ALAC_CACHE_MAX_FILES", 200),
		R2AccountID:              getEnv("R2_ACCOUNT_ID", ""),
		R2AccessKeyID:            getEnv("R2_ACCESS_KEY_ID", ""),
		R2SecretAccessKey:        getEnv("R2_SECRET_ACCESS_KEY", ""),
		R2BucketName:             getEnv("R2_BUCKET_NAME", ""),
		R2PublicURL:              strings.TrimRight(getEnv("R2_PUBLIC_URL", ""), "/"),
	}
}

// HasR2 returns true if all necessary Cloudflare R2 credentials are provided.
func (c *Config) HasR2() bool {
	return c.R2AccountID != "" && c.R2AccessKeyID != "" && c.R2SecretAccessKey != "" && c.R2BucketName != ""
}

func parseIDList(raw string) []int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	// Try JSON array first
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		var arr []any
		if err := json.Unmarshal([]byte(raw), &arr); err == nil {
			var ids []int64
			for _, item := range arr {
				switch v := item.(type) {
				case float64:
					ids = append(ids, int64(v))
				case string:
					if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
						ids = append(ids, n)
					}
				}
			}
			return ids
		}
	}

	// Delimiter split (comma, space)
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == ';'
	})
	var ids []int64
	for _, f := range fields {
		if n, err := strconv.ParseInt(strings.TrimSpace(f), 10, 64); err == nil {
			ids = append(ids, n)
		}
	}
	return ids
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		v := strings.TrimSpace(val)
		v = strings.Trim(v, "\"'")
		return v
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		val = strings.ToLower(strings.TrimSpace(val))
		return val == "true" || val == "1" || val == "yes"
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			return n
		}
	}
	return defaultVal
}

func getEnvInt64(key string, defaultVal int64) int64 {
	if val, ok := os.LookupEnv(key); ok {
		if n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64); err == nil {
			return n
		}
	}
	return defaultVal
}
