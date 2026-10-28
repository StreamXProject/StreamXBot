package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// SecretKeys identifies sensitive configuration parameters that should not be exposed or modified lightly.
var SecretKeys = map[string]bool{
	"BOT_TOKEN":                   true,
	"API_ID":                      true,
	"API_HASH":                    true,
	"SESSION_STRING":              true,
	"MONGO_URI":                   true,
	"MULTI_CLIENTS":               true,
	"ONLY_API":                    true,
	"CORS_ORIGINS":                true,
	"FIREBASE_CREDENTIALS":        true,
	"TELEGRAM_OIDC_CLIENT_SECRET": true,
	"SECRET_KEY":                  true,
	"MULTI_CLIENT_TOKENS":         true,
}

// ConfigManager handles dynamic configuration loading, database synchronization,
// type casting, and runtime updates matching StreamXBot's config_manager.py.
type ConfigManager struct {
	cfg           *Config
	db            *mongo.Database
	col           *mongo.Collection
	mu            sync.RWMutex
	overrideTypes map[string]string
	extra         map[string]any
	listeners     []func(key string, val any)
}

// NewManager creates a new ConfigManager wrapping the provided Config.
func NewManager(cfg *Config) *ConfigManager {
	return &ConfigManager{
		cfg:           cfg,
		overrideTypes: make(map[string]string),
		extra:         make(map[string]any),
	}
}

// SetDatabase binds the MongoDB database for persistence.
func (m *ConfigManager) SetDatabase(db *mongo.Database) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.db = db
	if db != nil {
		m.col = db.Collection("botsettings")
	}
}

// Config returns the underlying Config pointer.
func (m *ConfigManager) Config() *Config {
	return m.cfg
}

// OnUpdate registers a listener callback invoked when any config key is updated.
func (m *ConfigManager) OnUpdate(fn func(key string, val any)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, fn)
}

// IsSecretKey checks if the specified key contains sensitive credentials.
func (m *ConfigManager) IsSecretKey(key string) bool {
	k := strings.ToUpper(strings.TrimSpace(key))
	if SecretKeys[k] {
		return true
	}
	if strings.HasPrefix(k, "MULTI_CLIENTS_") {
		return true
	}
	return false
}

// LoadFromDB synchronizes the configuration with the MongoDB botsettings collection (doc: bot_config).
// If the document does not exist, it persists the current defaults.
// If it exists, it backfills missing keys and applies database overrides to runtime state.
func (m *ConfigManager) LoadFromDB(ctx context.Context, db *mongo.Database) error {
	m.SetDatabase(db)
	if m.col == nil {
		return errors.New("database not initialized")
	}

	var doc bson.M
	err := m.col.FindOne(ctx, bson.M{"_id": "bot_config"}).Decode(&doc)
	defaults := m.GetAllConfig()

	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			log.Info("Creating bot_config in botsettings collection with defaults")
			docData := bson.M{
				"_id":        "bot_config",
				"created_at": float64(time.Now().Unix()),
			}
			for k, v := range defaults {
				docData[k] = v
			}
			_, insertErr := m.col.InsertOne(ctx, docData)
			return insertErr
		}
		return fmt.Errorf("read bot_config: %w", err)
	}

	// Backfill missing or empty keys without clobbering existing DB values
	updates := bson.M{}
	for k, v := range defaults {
		dbVal, exists := doc[k]
		if !exists || (isEmptyValue(dbVal) && !isEmptyValue(v)) {
			updates[k] = v
			doc[k] = v
		}
	}
	if len(updates) > 0 {
		_, _ = m.col.UpdateOne(ctx, bson.M{"_id": "bot_config"}, bson.M{"$set": updates})
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Load _types map if present
	if rawTypes, ok := doc["_types"].(bson.M); ok {
		m.overrideTypes = make(map[string]string)
		for k, v := range rawTypes {
			if s, ok := v.(string); ok {
				m.overrideTypes[k] = s
			}
		}
	}

	// Apply DB overrides for all known uppercase keys (skipping sensitive secret keys if env differs)
	for key, dbVal := range doc {
		if key == "_id" || key == "_types" || key == "created_at" || !isAllUpper(key) {
			continue
		}
		if m.IsSecretKey(key) {
			continue
		}
		m.applyValueLocked(key, dbVal)
	}

	log.Infof("Successfully synchronized bot_config from MongoDB (OWNER_ID=%v, SUDO_USERS=%v)", m.cfg.OwnerIDs, m.cfg.SudoUsers)
	return nil
}

// GetAllConfig returns a key-value map of all uppercase configuration parameters.
func (m *ConfigManager) GetAllConfig() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getAllConfigLocked()
}

// GetAllKeys returns a sorted slice of all uppercase configuration keys.
func (m *ConfigManager) GetAllKeys() []string {
	all := m.GetAllConfig()
	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Get returns the typed value of the specified configuration key.
func (m *ConfigManager) Get(key string) any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	all := m.getAllConfigLocked()
	return all[strings.ToUpper(strings.TrimSpace(key))]
}

// UpdateConfig casts, updates in-memory state, and persists the setting to MongoDB.
func (m *ConfigManager) UpdateConfig(ctx context.Context, key string, rawVal any) (any, error) {
	key = strings.ToUpper(strings.TrimSpace(key))
	if !isAllUpper(key) {
		return nil, fmt.Errorf("invalid config key: %s", key)
	}

	m.mu.Lock()
	processed := m.processValueLocked(key, rawVal)
	m.applyValueLocked(key, processed)
	m.mu.Unlock()

	// Persist to MongoDB
	if m.col != nil {
		opts := options.UpdateOne().SetUpsert(true)
		_, err := m.col.UpdateOne(ctx, bson.M{"_id": "bot_config"}, bson.M{"$set": bson.M{key: processed}}, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to persist %s: %w", key, err)
		}
	}

	// Trigger callbacks
	for _, fn := range m.listeners {
		fn(key, processed)
	}

	return processed, nil
}

// UpdateConfigType updates the type override for a key (e.g. str, int, bool, list) in _types and reprocesses the current value.
func (m *ConfigManager) UpdateConfigType(ctx context.Context, key string, typeStr string) (any, error) {
	key = strings.ToUpper(strings.TrimSpace(key))
	typeStr = strings.ToLower(strings.TrimSpace(typeStr))

	m.mu.Lock()
	if typeStr == "" || typeStr == "default" {
		delete(m.overrideTypes, key)
	} else {
		m.overrideTypes[key] = typeStr
	}
	typesCopy := make(map[string]string)
	for k, v := range m.overrideTypes {
		typesCopy[k] = v
	}
	currentVal := m.getAllConfigLocked()[key]
	processed := m.processValueLocked(key, currentVal)
	m.applyValueLocked(key, processed)
	m.mu.Unlock()

	if m.col != nil {
		update := bson.M{
			"_types": typesCopy,
			key:      processed,
		}
		_, err := m.col.UpdateOne(ctx, bson.M{"_id": "bot_config"}, bson.M{"$set": update}, options.UpdateOne().SetUpsert(true))
		if err != nil {
			return nil, err
		}
	}

	return processed, nil
}

// ClearSetting resets the key to its type-specific empty/zero value.
func (m *ConfigManager) ClearSetting(ctx context.Context, key string) (any, error) {
	key = strings.ToUpper(strings.TrimSpace(key))
	defType := m.GetTargetType(key)

	var emptyVal any
	switch defType {
	case "bool":
		emptyVal = false
	case "int":
		emptyVal = 0
	case "int64":
		emptyVal = int64(0)
	case "list":
		if key == "OWNER_ID" || key == "SUDO_USERS" || key == "COLLABORATOR_ID" || key == "COLLABORATOR_IDS" {
			emptyVal = []int64{}
		} else {
			emptyVal = []string{}
		}
	default:
		emptyVal = ""
	}

	return m.UpdateConfig(ctx, key, emptyVal)
}

// AddIDs adds user/chat IDs to an ID list key (OWNER_ID, SUDO_USERS, COLLABORATOR_IDS).
func (m *ConfigManager) AddIDs(ctx context.Context, key string, ids []int64) error {
	key = strings.ToUpper(strings.TrimSpace(key))
	curr, ok := m.Get(key).([]int64)
	if !ok {
		curr = []int64{}
	}

	seen := make(map[int64]bool)
	var merged []int64
	for _, id := range append(curr, ids...) {
		if id != 0 && !seen[id] {
			seen[id] = true
			merged = append(merged, id)
		}
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i] < merged[j] })

	_, err := m.UpdateConfig(ctx, key, merged)
	return err
}

// RemoveIDs removes user/chat IDs from an ID list key.
func (m *ConfigManager) RemoveIDs(ctx context.Context, key string, ids []int64) error {
	key = strings.ToUpper(strings.TrimSpace(key))
	curr, ok := m.Get(key).([]int64)
	if !ok {
		return nil
	}

	toRemove := make(map[int64]bool)
	for _, id := range ids {
		toRemove[id] = true
	}

	var filtered []int64
	for _, id := range curr {
		if !toRemove[id] {
			filtered = append(filtered, id)
		}
	}

	_, err := m.UpdateConfig(ctx, key, filtered)
	return err
}

func (m *ConfigManager) getOverrideTypeLocked(key string) string {
	key = strings.ToUpper(strings.TrimSpace(key))
	if t, ok := m.overrideTypes[key]; ok && t != "" {
		return t
	}
	return "default"
}

// GetOverrideType returns the configured override type for the key or "default".
func (m *ConfigManager) GetOverrideType(key string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getOverrideTypeLocked(key)
}

func (m *ConfigManager) getDefaultTypeLocked(key string) string {
	val := m.getAllConfigLocked()[key]
	if val == nil {
		return "str"
	}
	switch val.(type) {
	case bool:
		return "bool"
	case int, int32:
		return "int"
	case int64:
		return "int"
	case []string, []int64, []any:
		return "list"
	default:
		return "str"
	}
}

// GetDefaultType returns the base Go type name ("str", "int", "bool", "list") for the key.
func (m *ConfigManager) GetDefaultType(key string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getDefaultTypeLocked(key)
}

func (m *ConfigManager) getTargetTypeLocked(key string) string {
	ov := m.getOverrideTypeLocked(key)
	if ov != "default" {
		return ov
	}
	return m.getDefaultTypeLocked(key)
}

// GetTargetType returns the effective type ("str", "int", "bool", "list") for the key.
func (m *ConfigManager) GetTargetType(key string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getTargetTypeLocked(key)
}

func (m *ConfigManager) getAllConfigLocked() map[string]any {
	c := m.cfg
	res := map[string]any{
		"PORT":                        c.Port,
		"DEBUG":                       c.Debug,
		"API_LOGS":                    c.APILogs,
		"CORS_ORIGIN":                 c.CorsOrigin,
		"MONGO_URI":                   c.MongoURI,
		"DATABASE_NAME":               c.DatabaseName,
		"API_ID":                      c.ApiID,
		"API_HASH":                    c.ApiHash,
		"BOT_TOKEN":                   c.BotToken,
		"SECRET_KEY":                  c.SecretKey,
		"SESSION_STRING":              c.SessionString,
		"CHANNEL_ID":                  c.ChannelID,
		"DUMP_CHANNEL_ID":             c.DumpChannelID,
		"MULTI_CLIENTS":               c.MultiClients,
		"MULTI_CLIENT_TOKENS":         c.MultiClientTokens,
		"FILTER_MODE":                 c.FilterMode,
		"CHAT_TOPIC":                  c.ChatTopic,
		"COLLABORATOR_ID":             c.CollaboratorIDs,
		"COLLABORATOR_IDS":            c.CollaboratorIDs,
		"LYRICS":                      c.Lyrics,
		"LRCLIB":                      c.LRCLIB,
		"MUSIXMATCH":                  c.Musixmatch,
		"TELEGRAM_OIDC_CLIENT_ID":     c.TelegramOIDCClientID,
		"TELEGRAM_OIDC_CLIENT_SECRET": c.TelegramOIDCClientSecret,
		"TELEGRAM_OIDC_ORIGIN":        c.TelegramOIDCOrigin,
		"TELEGRAM_OIDC_REDIRECT_URI":  c.TelegramOIDCRedirectURI,
		"COOKIE_SECURE":               c.CookieSecure,
		"COOKIE_SAMESITE":             c.CookieSameSite,
		"OWNER_ID":                    c.OwnerIDs,
		"SUDO_USERS":                  c.SudoUsers,
		"ENRICHMENT_WORKERS":          c.EnrichmentWorkers,
		"GUEST_PASSWORD":              c.GuestPassword,
		"ALAC_CACHE_MAX_BYTES":        c.AlacCacheMaxBytes,
		"ALAC_CACHE_MAX_FILES":        c.AlacCacheMaxFiles,
		"R2_ACCOUNT_ID":               c.R2AccountID,
		"R2_ACCESS_KEY_ID":            c.R2AccessKeyID,
		"R2_SECRET_ACCESS_KEY":        c.R2SecretAccessKey,
		"R2_BUCKET_NAME":              c.R2BucketName,
		"R2_PUBLIC_URL":               c.R2PublicURL,
	}
	for k, v := range m.extra {
		res[k] = v
	}
	return res
}

func (m *ConfigManager) processValueLocked(key string, val any) any {
	targetType := m.getTargetTypeLocked(key)

	switch targetType {
	case "bool":
		switch v := val.(type) {
		case bool:
			return v
		case string:
			s := strings.ToLower(strings.TrimSpace(v))
			return s == "true" || s == "1" || s == "yes" || s == "y" || s == "on"
		case int, int64, float64:
			return fmt.Sprintf("%v", v) != "0"
		}
		return false

	case "int":
		if key == "CHANNEL_ID" || key == "DUMP_CHANNEL_ID" || key == "ALAC_CACHE_MAX_BYTES" {
			return toInt64(val)
		}
		return int(toInt64(val))

	case "list":
		if key == "OWNER_ID" || key == "SUDO_USERS" || key == "COLLABORATOR_ID" || key == "COLLABORATOR_IDS" {
			return toInt64List(val)
		}
		return toStringList(val)

	case "str":
		if val == nil {
			return ""
		}
		if s, ok := val.(string); ok {
			return strings.TrimSpace(s)
		}
		return fmt.Sprintf("%v", val)

	default:
		return val
	}
}

func (m *ConfigManager) applyValueLocked(key string, val any) {
	c := m.cfg
	switch key {
	case "PORT":
		c.Port = fmt.Sprintf("%v", val)
	case "DEBUG":
		c.Debug = toBool(val)
	case "API_LOGS":
		c.APILogs = toBool(val)
	case "CORS_ORIGIN":
		c.CorsOrigin = fmt.Sprintf("%v", val)
	case "MONGO_URI":
		c.MongoURI = fmt.Sprintf("%v", val)
	case "DATABASE_NAME":
		c.DatabaseName = fmt.Sprintf("%v", val)
	case "API_ID":
		c.ApiID = int(toInt64(val))
	case "API_HASH":
		c.ApiHash = fmt.Sprintf("%v", val)
	case "BOT_TOKEN":
		c.BotToken = fmt.Sprintf("%v", val)
	case "SECRET_KEY":
		c.SecretKey = fmt.Sprintf("%v", val)
	case "SESSION_STRING":
		c.SessionString = fmt.Sprintf("%v", val)
	case "CHANNEL_ID":
		c.ChannelID = toInt64(val)
	case "DUMP_CHANNEL_ID":
		c.DumpChannelID = toInt64(val)
	case "MULTI_CLIENTS":
		c.MultiClients = toBool(val)
	case "MULTI_CLIENT_TOKENS":
		c.MultiClientTokens = toStringList(val)
	case "FILTER_MODE":
		c.FilterMode = int(toInt64(val))
	case "CHAT_TOPIC":
		c.ChatTopic = fmt.Sprintf("%v", val)
	case "COLLABORATOR_ID", "COLLABORATOR_IDS":
		c.CollaboratorIDs = toInt64List(val)
	case "LYRICS":
		c.Lyrics = toBool(val)
	case "LRCLIB":
		c.LRCLIB = toBool(val)
	case "MUSIXMATCH":
		c.Musixmatch = toBool(val)
	case "TELEGRAM_OIDC_CLIENT_ID":
		c.TelegramOIDCClientID = fmt.Sprintf("%v", val)
	case "TELEGRAM_OIDC_CLIENT_SECRET":
		c.TelegramOIDCClientSecret = fmt.Sprintf("%v", val)
	case "TELEGRAM_OIDC_ORIGIN":
		c.TelegramOIDCOrigin = fmt.Sprintf("%v", val)
	case "TELEGRAM_OIDC_REDIRECT_URI":
		c.TelegramOIDCRedirectURI = fmt.Sprintf("%v", val)
	case "COOKIE_SECURE":
		c.CookieSecure = toBool(val)
	case "COOKIE_SAMESITE":
		c.CookieSameSite = fmt.Sprintf("%v", val)
	case "OWNER_ID":
		c.OwnerIDs = toInt64List(val)
	case "SUDO_USERS":
		c.SudoUsers = toInt64List(val)
	case "ENRICHMENT_WORKERS":
		c.EnrichmentWorkers = int(toInt64(val))
	case "GUEST_PASSWORD":
		c.GuestPassword = fmt.Sprintf("%v", val)
	case "ALAC_CACHE_MAX_BYTES":
		c.AlacCacheMaxBytes = toInt64(val)
	case "ALAC_CACHE_MAX_FILES":
		c.AlacCacheMaxFiles = int(toInt64(val))
	case "R2_ACCOUNT_ID":
		c.R2AccountID = fmt.Sprintf("%v", val)
	case "R2_ACCESS_KEY_ID":
		c.R2AccessKeyID = fmt.Sprintf("%v", val)
	case "R2_SECRET_ACCESS_KEY":
		c.R2SecretAccessKey = fmt.Sprintf("%v", val)
	case "R2_BUCKET_NAME":
		c.R2BucketName = fmt.Sprintf("%v", val)
	case "R2_PUBLIC_URL":
		c.R2PublicURL = fmt.Sprintf("%v", val)
	default:
		m.extra[key] = val
	}
}

func toBool(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	s := strings.ToLower(fmt.Sprintf("%v", v))
	return s == "true" || s == "1" || s == "yes" || s == "on"
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int8:
		return int64(n)
	case int16:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case uint:
		return int64(n)
	case uint8:
		return int64(n)
	case uint16:
		return int64(n)
	case uint32:
		return int64(n)
	case uint64:
		return int64(n)
	case float32:
		return int64(n)
	case float64:
		return int64(n)
	case string:
		s := strings.TrimSpace(n)
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int64(f)
		}
	}
	return 0
}

func toInt64List(v any) []int64 {
	if v == nil {
		return []int64{}
	}
	if list, ok := v.([]int64); ok {
		return list
	}
	if list, ok := v.([]int); ok {
		out := make([]int64, len(list))
		for i, id := range list {
			out[i] = int64(id)
		}
		return out
	}
	if s, ok := v.(string); ok {
		return parseIDList(s)
	}
	if arr, ok := v.(bson.A); ok {
		var out []int64
		for _, item := range arr {
			if id := toInt64(item); id != 0 {
				out = append(out, id)
			}
		}
		return out
	}
	if arr, ok := v.([]any); ok {
		var out []int64
		for _, item := range arr {
			if id := toInt64(item); id != 0 {
				out = append(out, id)
			}
		}
		return out
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice {
		var out []int64
		for i := 0; i < rv.Len(); i++ {
			if id := toInt64(rv.Index(i).Interface()); id != 0 {
				out = append(out, id)
			}
		}
		return out
	}
	if id := toInt64(v); id != 0 {
		return []int64{id}
	}
	return []int64{}
}

func toStringList(v any) []string {
	if v == nil {
		return []string{}
	}
	if list, ok := v.([]string); ok {
		return list
	}
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
			var arr []string
			if err := json.Unmarshal([]byte(s), &arr); err == nil {
				return arr
			}
		}
		parts := strings.FieldsFunc(s, func(r rune) bool {
			return r == ',' || r == ' ' || r == ';'
		})
		var out []string
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	}
	if arr, ok := v.(bson.A); ok {
		var out []string
		for _, item := range arr {
			out = append(out, fmt.Sprintf("%v", item))
		}
		return out
	}
	if arr, ok := v.([]any); ok {
		var out []string
		for _, item := range arr {
			out = append(out, fmt.Sprintf("%v", item))
		}
		return out
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice {
		var out []string
		for i := 0; i < rv.Len(); i++ {
			out = append(out, fmt.Sprintf("%v", rv.Index(i).Interface()))
		}
		return out
	}
	return []string{}
}

func isEmptyValue(v any) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val) == ""
	case []string:
		return len(val) == 0
	case []int64:
		return len(val) == 0
	case []int:
		return len(val) == 0
	case []any:
		return len(val) == 0
	case bson.A:
		return len(val) == 0
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Map {
		return rv.Len() == 0
	}
	return false
}

func isAllUpper(s string) bool {
	hasLetter := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') {
			return false
		}
		if (r >= 'A' && r <= 'Z') {
			hasLetter = true
		}
	}
	return hasLetter
}
