package modelconfig

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"baize/internal/sqliteutil"
)

const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"

	APITypeResponses       = "responses"
	APITypeChatCompletions = "chat_completions"
	APITypeMessages        = "messages"

	DefaultProvider              = ProviderOpenAI
	DefaultAPIType               = APITypeResponses
	DefaultBaseURL               = "https://api.openai.com/v1"
	DefaultAnthropicBaseURL      = "https://api.anthropic.com/v1"
	DefaultRequestTimeoutSeconds = 90

	currentDatabaseVersion = 2
)

type Config struct {
	ID                    string   `json:"id,omitempty"`
	Name                  string   `json:"name,omitempty"`
	Provider              string   `json:"provider"`
	APIType               string   `json:"api_type"`
	BaseURL               string   `json:"base_url"`
	APIKey                string   `json:"api_key,omitempty"`
	Model                 string   `json:"model"`
	RequestTimeoutSeconds *int     `json:"request_timeout_seconds,omitempty"`
	MaxOutputTokensText   *int     `json:"max_output_tokens_text,omitempty"`
	MaxOutputTokensJSON   *int     `json:"max_output_tokens_json,omitempty"`
	MaxOutputTokens       *int     `json:"max_output_tokens,omitempty"`
	Temperature           *float64 `json:"temperature,omitempty"`
	TopP                  *float64 `json:"top_p,omitempty"`
	FrequencyPenalty      *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty       *float64 `json:"presence_penalty,omitempty"`
}

type Summary struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	Provider              string    `json:"provider"`
	APIType               string    `json:"apiType"`
	BaseURL               string    `json:"baseUrl"`
	Model                 string    `json:"model"`
	RequestTimeoutSeconds *int      `json:"requestTimeoutSeconds,omitempty"`
	HasAPIKey             bool      `json:"hasApiKey"`
	APIKeyMasked          string    `json:"apiKeyMasked"`
	Active                bool      `json:"active"`
	UpdatedAt             time.Time `json:"updatedAt"`
	MaxOutputTokensText   *int      `json:"maxOutputTokensText,omitempty"`
	MaxOutputTokensJSON   *int      `json:"maxOutputTokensJSON,omitempty"`
	MaxOutputTokens       *int      `json:"maxOutputTokens,omitempty"`
	Temperature           *float64  `json:"temperature,omitempty"`
	TopP                  *float64  `json:"topP,omitempty"`
	FrequencyPenalty      *float64  `json:"frequencyPenalty,omitempty"`
	PresencePenalty       *float64  `json:"presencePenalty,omitempty"`
}

type Snapshot struct {
	ActiveProfileID string    `json:"activeProfileId"`
	Profiles        []Summary `json:"profiles"`
}

type SaveOptions struct {
	SetActive      bool
	PreserveAPIKey bool
}

type Store struct {
	path     string
	keyPath  string
	db       *sql.DB
	mu       sync.Mutex
	initOnce sync.Once
	initErr  error
}

type databaseFile struct {
	Version         int             `json:"version"`
	ActiveProfileID string          `json:"active_profile_id,omitempty"`
	Profiles        []storedProfile `json:"profiles"`
}

type storedProfile struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	Provider              string    `json:"provider"`
	APIType               string    `json:"api_type"`
	BaseURL               string    `json:"base_url"`
	EncryptedAPIKey       string    `json:"encrypted_api_key,omitempty"`
	Model                 string    `json:"model"`
	RequestTimeoutSeconds *int      `json:"request_timeout_seconds,omitempty"`
	MaxOutputTokensText   *int      `json:"max_output_tokens_text,omitempty"`
	MaxOutputTokensJSON   *int      `json:"max_output_tokens_json,omitempty"`
	MaxOutputTokens       *int      `json:"max_output_tokens,omitempty"`
	Temperature           *float64  `json:"temperature,omitempty"`
	TopP                  *float64  `json:"top_p,omitempty"`
	FrequencyPenalty      *float64  `json:"frequency_penalty,omitempty"`
	PresencePenalty       *float64  `json:"presence_penalty,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

func NewStore(path ...string) *Store {
	store := &Store{}
	if len(path) > 0 {
		store.path = strings.TrimSpace(path[0])
	}
	if len(path) > 1 {
		store.keyPath = strings.TrimSpace(path[1])
	}
	if store.path != "" {
		dir := filepath.Dir(store.path)
		if store.keyPath == "" {
			store.keyPath = filepath.Join(dir, "secret.key")
		}
	}
	return store
}

func DefaultConfig() Config {
	return Config{
		Provider: DefaultProvider,
		APIType:  DefaultAPIType,
		BaseURL:  DefaultBaseURL,
	}
}

func (s *Store) Load(_ context.Context) (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	db, err := s.readDatabaseLocked()
	if err != nil {
		return Config{}, err
	}
	index := indexOfProfile(db.Profiles, db.ActiveProfileID)
	if index == -1 {
		return DefaultConfig(), nil
	}
	return s.profileConfigLocked(db.Profiles[index])
}

func (s *Store) Get(_ context.Context, id string) (Config, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	db, err := s.readDatabaseLocked()
	if err != nil {
		return Config{}, false, err
	}
	index := indexOfProfile(db.Profiles, id)
	if index == -1 {
		return Config{}, false, nil
	}
	cfg, err := s.profileConfigLocked(db.Profiles[index])
	if err != nil {
		return Config{}, false, err
	}
	return cfg, true, nil
}

func (s *Store) List(_ context.Context) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	db, err := s.readDatabaseLocked()
	if err != nil {
		return Snapshot{}, err
	}

	summaries := make([]Summary, 0, len(db.Profiles))
	for _, profile := range db.Profiles {
		cfg := profile.config()
		summaries = append(summaries, Summary{
			ID:                    cfg.ID,
			Name:                  cfg.Name,
			Provider:              cfg.Provider,
			APIType:               cfg.APIType,
			BaseURL:               cfg.BaseURL,
			Model:                 cfg.Model,
			RequestTimeoutSeconds: cfg.RequestTimeoutSeconds,
			HasAPIKey:             strings.TrimSpace(profile.EncryptedAPIKey) != "",
			APIKeyMasked:          MaskSecret(profile.EncryptedAPIKey),
			Active:                profile.ID == db.ActiveProfileID,
			UpdatedAt:             profile.UpdatedAt,
			MaxOutputTokensText:   cfg.MaxOutputTokensText,
			MaxOutputTokensJSON:   cfg.MaxOutputTokensJSON,
			MaxOutputTokens:       SharedMaxOutputTokens(cfg.MaxOutputTokensText, cfg.MaxOutputTokensJSON),
			Temperature:           cfg.Temperature,
			TopP:                  cfg.TopP,
			FrequencyPenalty:      cfg.FrequencyPenalty,
			PresencePenalty:       cfg.PresencePenalty,
		})
	}

	slices.SortFunc(summaries, func(a, b Summary) int {
		switch {
		case a.Active && !b.Active:
			return -1
		case !a.Active && b.Active:
			return 1
		case a.UpdatedAt.After(b.UpdatedAt):
			return -1
		case a.UpdatedAt.Before(b.UpdatedAt):
			return 1
		case a.Name < b.Name:
			return -1
		case a.Name > b.Name:
			return 1
		default:
			return 0
		}
	})

	return Snapshot{
		ActiveProfileID: db.ActiveProfileID,
		Profiles:        summaries,
	}, nil
}

func (s *Store) Save(_ context.Context, cfg Config, opts SaveOptions) (Config, error) {
	if strings.TrimSpace(s.path) == "" {
		return Config{}, errors.New("model config store is read-only")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	db, err := s.readDatabaseLocked()
	if err != nil {
		return Config{}, err
	}

	key, err := s.loadMasterKeyLocked()
	if err != nil {
		return Config{}, err
	}

	now := time.Now().UTC()
	cfg = cfg.Normalize()
	index := indexOfProfile(db.Profiles, cfg.ID)
	if index == -1 {
		if cfg.ID == "" {
			cfg.ID = newID()
		}
		profile, err := newStoredProfile(cfg, key, now)
		if err != nil {
			return Config{}, err
		}
		db.Profiles = append(db.Profiles, profile)
	} else {
		profile := db.Profiles[index]
		profileCfg := cfg
		profileCfg.ID = profile.ID
		if opts.PreserveAPIKey && strings.TrimSpace(profileCfg.APIKey) == "" {
			profileCfg.APIKey = noChangeSecretSentinel
		}
		updated, err := updateStoredProfile(profile, profileCfg, key, now)
		if err != nil {
			return Config{}, err
		}
		db.Profiles[index] = updated
		cfg.ID = updated.ID
	}

	if opts.SetActive || strings.TrimSpace(db.ActiveProfileID) == "" {
		db.ActiveProfileID = cfg.ID
	}

	if err := s.writeDatabaseLocked(db); err != nil {
		return Config{}, err
	}

	index = indexOfProfile(db.Profiles, cfg.ID)
	if index == -1 {
		return Config{}, fmt.Errorf("saved profile %q not found", cfg.ID)
	}
	return s.profileConfigLocked(db.Profiles[index])
}

func (s *Store) Delete(_ context.Context, id string) (bool, error) {
	if strings.TrimSpace(s.path) == "" {
		return false, errors.New("model config store is read-only")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	db, err := s.readDatabaseLocked()
	if err != nil {
		return false, err
	}

	index := indexOfProfile(db.Profiles, id)
	if index == -1 {
		return false, nil
	}

	db.Profiles = append(db.Profiles[:index], db.Profiles[index+1:]...)
	repairActiveProfile(&db)
	if err := s.writeDatabaseLocked(db); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) SetActive(_ context.Context, id string) error {
	if strings.TrimSpace(s.path) == "" {
		return errors.New("model config store is read-only")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	db, err := s.readDatabaseLocked()
	if err != nil {
		return err
	}
	if indexOfProfile(db.Profiles, id) == -1 {
		return fmt.Errorf("model profile %q not found", strings.TrimSpace(id))
	}
	db.ActiveProfileID = strings.TrimSpace(id)
	return s.writeDatabaseLocked(db)
}

func (s *Store) Clear(_ context.Context) error {
	if strings.TrimSpace(s.path) == "" {
		return errors.New("model config store is read-only")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	db := databaseFile{
		Version:         currentDatabaseVersion,
		ActiveProfileID: "",
		Profiles:        nil,
	}
	return s.writeDatabaseLocked(db)
}

func (s *Store) Path() string {
	return s.path
}

func (c Config) Normalize() Config {
	c.ID = strings.TrimSpace(c.ID)
	c.Provider = normalizeProvider(c.Provider)
	if c.Provider == "" {
		c.Provider = DefaultProvider
	}
	c.APIType = normalizeAPIType(c.Provider, c.APIType)
	if c.APIType == "" {
		c.APIType = defaultAPIType(c.Provider)
	}
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if c.BaseURL == "" {
		c.BaseURL = defaultBaseURL(c.Provider)
	}
	c.APIKey = strings.TrimSpace(c.APIKey)
	c.Model = strings.TrimSpace(c.Model)
	c.RequestTimeoutSeconds = normalizeOptionalPositiveInt(c.RequestTimeoutSeconds)
	c.MaxOutputTokensText = normalizeOptionalPositiveInt(c.MaxOutputTokensText)
	c.MaxOutputTokensJSON = normalizeOptionalPositiveInt(c.MaxOutputTokensJSON)
	c.MaxOutputTokens = normalizeOptionalPositiveInt(c.MaxOutputTokens)
	if c.MaxOutputTokens != nil {
		if c.MaxOutputTokensText == nil {
			c.MaxOutputTokensText = c.MaxOutputTokens
		}
		if c.MaxOutputTokensJSON == nil {
			c.MaxOutputTokensJSON = c.MaxOutputTokens
		}
		c.MaxOutputTokens = nil
	}
	c.Name = normalizeProfileName(c.Name, c.Provider, c.APIType, c.Model)
	return c
}

func (c Config) MissingFields() []string {
	var missing []string
	if strings.TrimSpace(c.Provider) == "" {
		missing = append(missing, "provider")
	}
	if strings.TrimSpace(c.APIType) == "" {
		missing = append(missing, "api_type")
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		missing = append(missing, "base_url")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		missing = append(missing, "api_key")
	}
	if strings.TrimSpace(c.Model) == "" {
		missing = append(missing, "model")
	}
	return missing
}

func (c Config) SupportsStructuredOutput() bool {
	switch c.Provider {
	case ProviderOpenAI:
		return c.APIType == APITypeResponses || c.APIType == APITypeChatCompletions
	case ProviderAnthropic:
		return true
	default:
		return false
	}
}

func MaskSecret(secret string) string {
	if strings.TrimSpace(secret) == "" {
		return "(empty)"
	}
	return "********"
}

func SharedMaxOutputTokens(text, json *int) *int {
	if text == nil || json == nil {
		return nil
	}
	if *text != *json {
		return nil
	}
	return text
}

func (s *Store) ensureReadyLocked() error {
	s.initOnce.Do(func() {
		if strings.TrimSpace(s.path) == "" {
			return
		}
		s.db, s.initErr = sqliteutil.Open(s.path)
		if s.initErr != nil {
			return
		}
		_, s.initErr = s.db.Exec(`
			CREATE TABLE IF NOT EXISTS modelconfig_meta (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL
			);
			CREATE TABLE IF NOT EXISTS modelconfig_profiles (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				provider TEXT NOT NULL,
				api_type TEXT NOT NULL,
				base_url TEXT NOT NULL,
				encrypted_api_key TEXT NOT NULL DEFAULT '',
				model TEXT NOT NULL,
				request_timeout_seconds INTEGER,
				max_output_tokens_text INTEGER,
				max_output_tokens_json INTEGER,
				temperature REAL,
				top_p REAL,
				frequency_penalty REAL,
				presence_penalty REAL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)
		`)
	})
	return s.initErr
}

func (s *Store) readDatabaseLocked() (databaseFile, error) {
	if strings.TrimSpace(s.path) == "" {
		return databaseFile{
			Version: currentDatabaseVersion,
		}, nil
	}
	if err := s.ensureReadyLocked(); err != nil {
		return databaseFile{}, err
	}

	db := databaseFile{
		Version: currentDatabaseVersion,
	}

	metaRows, err := s.db.Query(`SELECT key, value FROM modelconfig_meta`)
	if err != nil {
		return databaseFile{}, err
	}
	defer metaRows.Close()
	meta := map[string]string{}
	for metaRows.Next() {
		var key, value string
		if err := metaRows.Scan(&key, &value); err != nil {
			return databaseFile{}, err
		}
		meta[key] = value
	}
	if err := metaRows.Err(); err != nil {
		return databaseFile{}, err
	}

	if value := strings.TrimSpace(meta["version"]); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			db.Version = parsed
		}
	}
	db.ActiveProfileID = strings.TrimSpace(meta["active_profile_id"])

	rows, err := s.db.Query(`
		SELECT
			id, name, provider, api_type, base_url, encrypted_api_key, model,
			request_timeout_seconds, max_output_tokens_text, max_output_tokens_json,
			temperature, top_p, frequency_penalty, presence_penalty,
			created_at, updated_at
		FROM modelconfig_profiles
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return databaseFile{}, err
	}
	defer rows.Close()

	for rows.Next() {
		profile, err := scanStoredProfile(rows)
		if err != nil {
			return databaseFile{}, err
		}
		db.Profiles = append(db.Profiles, profile)
	}
	if err := rows.Err(); err != nil {
		return databaseFile{}, err
	}

	changed := normalizeDatabase(&db)
	if repairActiveProfile(&db) {
		changed = true
	}
	if changed {
		if err := s.writeDatabaseLocked(db); err != nil {
			return databaseFile{}, err
		}
	}
	return db, nil
}

func (s *Store) writeDatabaseLocked(db databaseFile) error {
	if strings.TrimSpace(s.path) == "" {
		return errors.New("model config store is read-only")
	}
	if err := s.ensureReadyLocked(); err != nil {
		return err
	}

	return sqliteutil.WithTx(context.Background(), s.db, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM modelconfig_meta`); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM modelconfig_profiles`); err != nil {
			return err
		}

		meta := map[string]string{
			"version":           strconv.Itoa(db.Version),
			"active_profile_id": strings.TrimSpace(db.ActiveProfileID),
		}
		for key, value := range meta {
			if _, err := tx.Exec(`INSERT INTO modelconfig_meta (key, value) VALUES (?, ?)`, key, value); err != nil {
				return err
			}
		}

		for _, profile := range db.Profiles {
			if _, err := tx.Exec(`
				INSERT INTO modelconfig_profiles (
					id, name, provider, api_type, base_url, encrypted_api_key, model,
					request_timeout_seconds, max_output_tokens_text, max_output_tokens_json,
					temperature, top_p, frequency_penalty, presence_penalty,
					created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`,
				profile.ID,
				profile.Name,
				profile.Provider,
				profile.APIType,
				profile.BaseURL,
				profile.EncryptedAPIKey,
				profile.Model,
				nullableInt(profile.RequestTimeoutSeconds),
				nullableInt(profile.MaxOutputTokensText),
				nullableInt(profile.MaxOutputTokensJSON),
				nullableFloat(profile.Temperature),
				nullableFloat(profile.TopP),
				nullableFloat(profile.FrequencyPenalty),
				nullableFloat(profile.PresencePenalty),
				formatSQLiteTime(profile.CreatedAt),
				formatSQLiteTime(profile.UpdatedAt),
			); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) profileConfigLocked(profile storedProfile) (Config, error) {
	cfg := profile.config()
	if strings.TrimSpace(profile.EncryptedAPIKey) == "" {
		return cfg, nil
	}
	key, err := s.loadMasterKeyLocked()
	if err != nil {
		return Config{}, err
	}
	apiKey, err := decryptSecret(key, profile.EncryptedAPIKey)
	if err != nil {
		return Config{}, err
	}
	cfg.APIKey = apiKey
	return cfg, nil
}

func newStoredProfile(cfg Config, key []byte, now time.Time) (storedProfile, error) {
	cfg = cfg.Normalize()
	if cfg.ID == "" {
		cfg.ID = newID()
	}
	encrypted, err := encryptSecret(key, cfg.APIKey)
	if err != nil {
		return storedProfile{}, err
	}
	return storedProfile{
		ID:                    cfg.ID,
		Name:                  cfg.Name,
		Provider:              cfg.Provider,
		APIType:               cfg.APIType,
		BaseURL:               cfg.BaseURL,
		EncryptedAPIKey:       encrypted,
		Model:                 cfg.Model,
		RequestTimeoutSeconds: cfg.RequestTimeoutSeconds,
		MaxOutputTokensText:   cfg.MaxOutputTokensText,
		MaxOutputTokensJSON:   cfg.MaxOutputTokensJSON,
		Temperature:           cfg.Temperature,
		TopP:                  cfg.TopP,
		FrequencyPenalty:      cfg.FrequencyPenalty,
		PresencePenalty:       cfg.PresencePenalty,
		CreatedAt:             now,
		UpdatedAt:             now,
	}, nil
}

func updateStoredProfile(profile storedProfile, cfg Config, key []byte, now time.Time) (storedProfile, error) {
	cfg = cfg.Normalize()
	profile.Name = cfg.Name
	profile.Provider = cfg.Provider
	profile.APIType = cfg.APIType
	profile.BaseURL = cfg.BaseURL
	profile.Model = cfg.Model
	profile.RequestTimeoutSeconds = cfg.RequestTimeoutSeconds
	profile.MaxOutputTokensText = cfg.MaxOutputTokensText
	profile.MaxOutputTokensJSON = cfg.MaxOutputTokensJSON
	profile.MaxOutputTokens = nil
	profile.Temperature = cfg.Temperature
	profile.TopP = cfg.TopP
	profile.FrequencyPenalty = cfg.FrequencyPenalty
	profile.PresencePenalty = cfg.PresencePenalty
	switch cfg.APIKey {
	case noChangeSecretSentinel:
	case "":
		profile.EncryptedAPIKey = ""
	default:
		encrypted, err := encryptSecret(key, cfg.APIKey)
		if err != nil {
			return storedProfile{}, err
		}
		profile.EncryptedAPIKey = encrypted
	}
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = now
	}
	profile.UpdatedAt = now
	return profile, nil
}

func (p storedProfile) config() Config {
	return Config{
		ID:                    p.ID,
		Name:                  p.Name,
		Provider:              p.Provider,
		APIType:               p.APIType,
		BaseURL:               p.BaseURL,
		Model:                 p.Model,
		RequestTimeoutSeconds: p.RequestTimeoutSeconds,
		MaxOutputTokensText:   p.MaxOutputTokensText,
		MaxOutputTokensJSON:   p.MaxOutputTokensJSON,
		MaxOutputTokens:       p.MaxOutputTokens,
		Temperature:           p.Temperature,
		TopP:                  p.TopP,
		FrequencyPenalty:      p.FrequencyPenalty,
		PresencePenalty:       p.PresencePenalty,
	}.Normalize()
}

func indexOfProfile(profiles []storedProfile, id string) int {
	target := strings.TrimSpace(id)
	if target == "" {
		return -1
	}
	for index, profile := range profiles {
		if profile.ID == target {
			return index
		}
	}
	return -1
}

func scanStoredProfile(scanner interface{ Scan(...any) error }) (storedProfile, error) {
	var (
		profile          storedProfile
		requestTimeout   sql.NullInt64
		maxText          sql.NullInt64
		maxJSON          sql.NullInt64
		temperature      sql.NullFloat64
		topP             sql.NullFloat64
		frequencyPenalty sql.NullFloat64
		presencePenalty  sql.NullFloat64
		createdAt        string
		updatedAt        string
	)
	if err := scanner.Scan(
		&profile.ID,
		&profile.Name,
		&profile.Provider,
		&profile.APIType,
		&profile.BaseURL,
		&profile.EncryptedAPIKey,
		&profile.Model,
		&requestTimeout,
		&maxText,
		&maxJSON,
		&temperature,
		&topP,
		&frequencyPenalty,
		&presencePenalty,
		&createdAt,
		&updatedAt,
	); err != nil {
		return storedProfile{}, err
	}
	profile.RequestTimeoutSeconds = intPtrFromNull(requestTimeout)
	profile.MaxOutputTokensText = intPtrFromNull(maxText)
	profile.MaxOutputTokensJSON = intPtrFromNull(maxJSON)
	profile.Temperature = floatPtrFromNull(temperature)
	profile.TopP = floatPtrFromNull(topP)
	profile.FrequencyPenalty = floatPtrFromNull(frequencyPenalty)
	profile.PresencePenalty = floatPtrFromNull(presencePenalty)
	var err error
	if profile.CreatedAt, err = parseSQLiteTime(createdAt); err != nil {
		return storedProfile{}, err
	}
	if profile.UpdatedAt, err = parseSQLiteTime(updatedAt); err != nil {
		return storedProfile{}, err
	}
	return profile, nil
}

func nullableInt(value *int) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*value), Valid: true}
}

func nullableFloat(value *float64) sql.NullFloat64 {
	if value == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *value, Valid: true}
}

func intPtrFromNull(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	out := int(value.Int64)
	return &out
}

func floatPtrFromNull(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	out := value.Float64
	return &out
}

func formatSQLiteTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseSQLiteTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value)
}

func newID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return time.Now().Format("20060102150405")
	}
	return hex.EncodeToString(buf[:])
}
