package modelconfig

import (
	"context"
	"testing"
)

func TestLoadAppliesEnvOverrides(t *testing.T) {
	store := NewStore()

	t.Setenv("MYCLAW_MODEL_PROVIDER", "openai")
	t.Setenv("MYCLAW_MODEL_BASE_URL", "https://example.com/v1/")
	t.Setenv("MYCLAW_MODEL_API_KEY", "env-secret")
	t.Setenv("MYCLAW_MODEL_NAME", "env-model")
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load with env: %v", err)
	}
	if loaded.BaseURL != "https://example.com/v1" {
		t.Fatalf("expected normalized env base url, got %q", loaded.BaseURL)
	}
	if loaded.APIKey != "env-secret" {
		t.Fatalf("expected env api key, got %q", loaded.APIKey)
	}
	if loaded.Model != "env-model" {
		t.Fatalf("expected env model, got %q", loaded.Model)
	}
}

func TestMaskSecret(t *testing.T) {
	t.Parallel()

	if got := MaskSecret(""); got != "(empty)" {
		t.Fatalf("unexpected empty mask: %q", got)
	}
	if got := MaskSecret("12345678"); got != "********" {
		t.Fatalf("unexpected short mask: %q", got)
	}
	if got := MaskSecret("abcdefgh12345678"); got != "abcd********5678" {
		t.Fatalf("unexpected long mask: %q", got)
	}
}

func TestDefaultConfigUsesOpenAIDefaults(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if cfg.Provider != "openai" {
		t.Fatalf("unexpected provider: %q", cfg.Provider)
	}
	if cfg.BaseURL == "" {
		t.Fatal("expected base url")
	}
	store := NewStore()
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}
	if loaded.Provider != "openai" {
		t.Fatalf("unexpected loaded provider: %q", loaded.Provider)
	}
}
