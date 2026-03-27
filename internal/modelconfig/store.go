package modelconfig

import (
	"context"
	"os"
	"strings"
)

const (
	DefaultProvider = "openai"
	DefaultBaseURL  = "https://api.openai.com/v1"
)

type Config struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
}

type Store struct{}

func NewStore() *Store {
	return &Store{}
}

func DefaultConfig() Config {
	return Config{
		Provider: DefaultProvider,
		BaseURL:  DefaultBaseURL,
	}
}

func (s *Store) Load(_ context.Context) (Config, error) {
	cfg := DefaultConfig()
	applyEnvOverrides(&cfg)
	return cfg.Normalize(), nil
}

func (c Config) Normalize() Config {
	c.Provider = strings.ToLower(strings.TrimSpace(c.Provider))
	if c.Provider == "" {
		c.Provider = DefaultProvider
	}
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	c.APIKey = strings.TrimSpace(c.APIKey)
	c.Model = strings.TrimSpace(c.Model)
	return c
}

func (c Config) MissingFields() []string {
	var missing []string
	if strings.TrimSpace(c.Provider) == "" {
		missing = append(missing, "provider")
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

func applyEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}
	if value := strings.TrimSpace(os.Getenv("MYCLAW_MODEL_PROVIDER")); value != "" {
		cfg.Provider = value
	}
	if value := strings.TrimSpace(os.Getenv("MYCLAW_MODEL_BASE_URL")); value != "" {
		cfg.BaseURL = value
	}
	if value := strings.TrimSpace(os.Getenv("MYCLAW_MODEL_API_KEY")); value != "" {
		cfg.APIKey = value
	}
	if value := strings.TrimSpace(os.Getenv("MYCLAW_MODEL_NAME")); value != "" {
		cfg.Model = value
	}
}

func MaskSecret(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "(empty)"
	}
	runes := []rune(secret)
	if len(runes) <= 8 {
		return strings.Repeat("*", len(runes))
	}
	return string(runes[:4]) + strings.Repeat("*", len(runes)-8) + string(runes[len(runes)-4:])
}
