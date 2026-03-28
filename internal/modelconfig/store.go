package modelconfig

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"

	APITypeResponses       = "responses"
	APITypeChatCompletions = "chat_completions"
	APITypeMessages        = "messages"

	DefaultProvider         = ProviderOpenAI
	DefaultAPIType          = APITypeResponses
	DefaultBaseURL          = "https://api.openai.com/v1"
	DefaultAnthropicBaseURL = "https://api.anthropic.com/v1"

	currentDatabaseVersion = 1
	masterKeySize          = 32
)

type Config struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Provider string `json:"provider"`
	APIType  string `json:"api_type"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key,omitempty"`
	Model    string `json:"model"`
}

type Summary struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Provider     string    `json:"provider"`
	APIType      string    `json:"apiType"`
	BaseURL      string    `json:"baseUrl"`
	Model        string    `json:"model"`
	HasAPIKey    bool      `json:"hasApiKey"`
	APIKeyMasked string    `json:"apiKeyMasked"`
	Active       bool      `json:"active"`
	UpdatedAt    time.Time `json:"updatedAt"`
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
	path       string
	keyPath    string
	legacyPath string
	mu         sync.Mutex
}

type databaseFile struct {
	Version         int             `json:"version"`
	LegacyImported  bool            `json:"legacy_imported"`
	ActiveProfileID string          `json:"active_profile_id,omitempty"`
	Profiles        []storedProfile `json:"profiles"`
}

type storedProfile struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Provider        string    `json:"provider"`
	APIType         string    `json:"api_type"`
	BaseURL         string    `json:"base_url"`
	EncryptedAPIKey string    `json:"encrypted_api_key,omitempty"`
	Model           string    `json:"model"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func NewStore(path ...string) *Store {
	store := &Store{}
	if len(path) > 0 {
		store.path = strings.TrimSpace(path[0])
	}
	if len(path) > 1 {
		store.keyPath = strings.TrimSpace(path[1])
	}
	if len(path) > 2 {
		store.legacyPath = strings.TrimSpace(path[2])
	}
	if store.path != "" {
		dir := filepath.Dir(store.path)
		if store.keyPath == "" {
			store.keyPath = filepath.Join(dir, "secret.key")
		}
		if store.legacyPath == "" {
			store.legacyPath = filepath.Join(dir, "config.json")
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
			ID:           cfg.ID,
			Name:         cfg.Name,
			Provider:     cfg.Provider,
			APIType:      cfg.APIType,
			BaseURL:      cfg.BaseURL,
			Model:        cfg.Model,
			HasAPIKey:    strings.TrimSpace(profile.EncryptedAPIKey) != "",
			APIKeyMasked: maskedStoredSecret(profile.EncryptedAPIKey),
			Active:       profile.ID == db.ActiveProfileID,
			UpdatedAt:    profile.UpdatedAt,
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
		LegacyImported:  true,
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

const noChangeSecretSentinel = "\x00myclaw:keep-secret\x00"

func (s *Store) readDatabaseLocked() (databaseFile, error) {
	if strings.TrimSpace(s.path) == "" {
		return databaseFile{
			Version:        currentDatabaseVersion,
			LegacyImported: true,
		}, nil
	}

	db := databaseFile{
		Version: currentDatabaseVersion,
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return databaseFile{}, err
		}
	} else if len(data) > 0 {
		if err := json.Unmarshal(data, &db); err != nil {
			return databaseFile{}, err
		}
	}

	changed := normalizeDatabase(&db)
	if !db.LegacyImported {
		imported, err := s.importLegacyConfigLocked(&db)
		if err != nil {
			return databaseFile{}, err
		}
		changed = changed || imported
		db.LegacyImported = true
		changed = true
	}
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

func normalizeDatabase(db *databaseFile) bool {
	if db == nil {
		return false
	}
	changed := false
	if db.Version != currentDatabaseVersion {
		db.Version = currentDatabaseVersion
		changed = true
	}
	for index := range db.Profiles {
		if normalizeStoredProfile(&db.Profiles[index]) {
			changed = true
		}
	}
	return changed
}

func normalizeStoredProfile(profile *storedProfile) bool {
	if profile == nil {
		return false
	}
	before := *profile
	cfg := profile.config().Normalize()
	profile.ID = strings.TrimSpace(profile.ID)
	if profile.ID == "" {
		profile.ID = newID()
	}
	profile.Name = cfg.Name
	profile.Provider = cfg.Provider
	profile.APIType = cfg.APIType
	profile.BaseURL = cfg.BaseURL
	profile.Model = cfg.Model
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = time.Now().UTC()
	}
	if profile.UpdatedAt.IsZero() {
		profile.UpdatedAt = profile.CreatedAt
	}
	return before != *profile
}

func repairActiveProfile(db *databaseFile) bool {
	if db == nil {
		return false
	}
	if len(db.Profiles) == 0 {
		if db.ActiveProfileID != "" {
			db.ActiveProfileID = ""
			return true
		}
		return false
	}
	if indexOfProfile(db.Profiles, db.ActiveProfileID) != -1 {
		return false
	}
	db.ActiveProfileID = db.Profiles[0].ID
	return true
}

func (s *Store) importLegacyConfigLocked(db *databaseFile) (bool, error) {
	if strings.TrimSpace(s.legacyPath) == "" {
		return false, nil
	}
	data, err := os.ReadFile(s.legacyPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if len(data) == 0 {
		return false, retireLegacyConfig(s.legacyPath)
	}

	var legacy struct {
		Provider string `json:"provider"`
		BaseURL  string `json:"base_url"`
		APIKey   string `json:"api_key"`
		Model    string `json:"model"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return false, err
	}

	if len(db.Profiles) == 0 {
		key, err := s.loadMasterKeyLocked()
		if err != nil {
			return false, err
		}
		cfg := Config{
			Name:     strings.TrimSpace(legacy.Model),
			Provider: legacy.Provider,
			APIType:  DefaultAPIType,
			BaseURL:  legacy.BaseURL,
			APIKey:   legacy.APIKey,
			Model:    legacy.Model,
		}.Normalize()
		if cfg.ID == "" {
			cfg.ID = newID()
		}
		profile, err := newStoredProfile(cfg, key, time.Now().UTC())
		if err != nil {
			return false, err
		}
		db.Profiles = append(db.Profiles, profile)
		if strings.TrimSpace(db.ActiveProfileID) == "" {
			db.ActiveProfileID = profile.ID
		}
	}

	return true, retireLegacyConfig(s.legacyPath)
}

func retireLegacyConfig(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.WriteFile(path, []byte(`{"migrated":true}`), 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
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

func (s *Store) loadMasterKeyLocked() ([]byte, error) {
	if strings.TrimSpace(s.keyPath) == "" {
		return nil, errors.New("model secret key store is read-only")
	}

	data, err := os.ReadFile(s.keyPath)
	if err == nil {
		if len(data) != masterKeySize {
			return nil, fmt.Errorf("invalid model secret key length %d", len(data))
		}
		return data, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	key := make([]byte, masterKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(s.keyPath), 0o700); err != nil {
		return nil, err
	}
	tmpPath := s.keyPath + ".tmp"
	if err := os.WriteFile(tmpPath, key, 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmpPath, s.keyPath); err != nil {
		return nil, err
	}
	return key, nil
}

func (s *Store) writeDatabaseLocked(db databaseFile) error {
	if strings.TrimSpace(s.path) == "" {
		return errors.New("model config store is read-only")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
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
		ID:              cfg.ID,
		Name:            cfg.Name,
		Provider:        cfg.Provider,
		APIType:         cfg.APIType,
		BaseURL:         cfg.BaseURL,
		EncryptedAPIKey: encrypted,
		Model:           cfg.Model,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

func updateStoredProfile(profile storedProfile, cfg Config, key []byte, now time.Time) (storedProfile, error) {
	cfg = cfg.Normalize()
	profile.ID = profile.ID
	profile.Name = cfg.Name
	profile.Provider = cfg.Provider
	profile.APIType = cfg.APIType
	profile.BaseURL = cfg.BaseURL
	profile.Model = cfg.Model
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
		ID:       p.ID,
		Name:     p.Name,
		Provider: p.Provider,
		APIType:  p.APIType,
		BaseURL:  p.BaseURL,
		Model:    p.Model,
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

func normalizeProvider(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", ProviderOpenAI:
		return ProviderOpenAI
	case ProviderAnthropic, "antrophic":
		return ProviderAnthropic
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeAPIType(provider, value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return defaultAPIType(provider)
	case "response":
		return APITypeResponses
	case "responses":
		return APITypeResponses
	case "chat_completion", "chat-completion", "chat completions", "chat completion", "chat-completions":
		return APITypeChatCompletions
	case APITypeChatCompletions:
		return APITypeChatCompletions
	case "message":
		return APITypeMessages
	case APITypeMessages:
		return APITypeMessages
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func defaultAPIType(provider string) string {
	switch normalizeProvider(provider) {
	case ProviderAnthropic:
		return APITypeMessages
	default:
		return APITypeResponses
	}
}

func defaultBaseURL(provider string) string {
	switch normalizeProvider(provider) {
	case ProviderAnthropic:
		return DefaultAnthropicBaseURL
	default:
		return DefaultBaseURL
	}
}

func normalizeProfileName(name, provider, apiType, model string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	model = strings.TrimSpace(model)
	if model != "" {
		return model
	}
	label := providerLabel(normalizeProvider(provider))
	switch normalizeAPIType(provider, apiType) {
	case APITypeResponses:
		return label + " Responses"
	case APITypeChatCompletions:
		return label + " Chat Completions"
	case APITypeMessages:
		return label + " Messages"
	default:
		return label
	}
}

func providerLabel(provider string) string {
	switch normalizeProvider(provider) {
	case ProviderAnthropic:
		return "Anthropic"
	default:
		return "OpenAI"
	}
}

func maskedStoredSecret(encrypted string) string {
	if strings.TrimSpace(encrypted) == "" {
		return "(empty)"
	}
	return "********"
}

func newID() string {
	var buf [8]byte
	if _, err := io.ReadFull(rand.Reader, buf[:]); err != nil {
		return time.Now().UTC().Format("20060102150405")
	}
	return hex.EncodeToString(buf[:])
}

func encryptSecret(key []byte, secret string) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(secret), nil)
	return "v1:" + base64.StdEncoding.EncodeToString(nonce) + ":" + base64.StdEncoding.EncodeToString(ciphertext), nil
}

func decryptSecret(key []byte, encrypted string) (string, error) {
	encrypted = strings.TrimSpace(encrypted)
	if encrypted == "" {
		return "", nil
	}
	parts := strings.Split(encrypted, ":")
	if len(parts) != 3 || parts[0] != "v1" {
		return "", fmt.Errorf("unsupported encrypted secret format")
	}
	nonce, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
