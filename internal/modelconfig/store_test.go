package modelconfig

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaskSecret(t *testing.T) {
	t.Parallel()

	if got := MaskSecret(""); got != "(empty)" {
		t.Fatalf("unexpected empty mask: %q", got)
	}
	if got := MaskSecret("secret-value"); got != "********" {
		t.Fatalf("unexpected mask: %q", got)
	}
}

func TestDefaultConfigUsesOpenAIDefaults(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if cfg.Provider != ProviderOpenAI {
		t.Fatalf("unexpected provider: %q", cfg.Provider)
	}
	if cfg.APIType != APITypeResponses {
		t.Fatalf("unexpected api type: %q", cfg.APIType)
	}
	if cfg.BaseURL != DefaultBaseURL {
		t.Fatalf("unexpected base url: %q", cfg.BaseURL)
	}
}

func TestStoreSaveLoadListAndSwitchActiveProfile(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "model", "profiles.db"))
	ctx := context.Background()

	first, err := store.Save(ctx, Config{
		Name:     "OpenAI New API",
		Provider: ProviderOpenAI,
		APIType:  APITypeResponses,
		BaseURL:  "https://example.com/v1/",
		APIKey:   "openai-secret",
		Model:    "gpt-4.1-mini",
	}, SaveOptions{SetActive: true})
	if err != nil {
		t.Fatalf("save first profile: %v", err)
	}
	if first.BaseURL != "https://example.com/v1" {
		t.Fatalf("expected normalized base url, got %q", first.BaseURL)
	}
	if first.APIKey != "openai-secret" {
		t.Fatalf("expected decrypted api key, got %q", first.APIKey)
	}

	second, err := store.Save(ctx, Config{
		Name:     "Claude",
		Provider: "antrophic",
		APIType:  "",
		BaseURL:  "",
		APIKey:   "anthropic-secret",
		Model:    "claude-3-7-sonnet-latest",
	}, SaveOptions{})
	if err != nil {
		t.Fatalf("save second profile: %v", err)
	}
	if second.Provider != ProviderAnthropic {
		t.Fatalf("expected anthropic alias normalization, got %q", second.Provider)
	}
	if second.APIType != APITypeMessages {
		t.Fatalf("expected anthropic messages api type, got %q", second.APIType)
	}
	if second.BaseURL != DefaultAnthropicBaseURL {
		t.Fatalf("expected anthropic default base url, got %q", second.BaseURL)
	}

	snapshot, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}
	if snapshot.ActiveProfileID != first.ID {
		t.Fatalf("unexpected active profile: %q", snapshot.ActiveProfileID)
	}
	if len(snapshot.Profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(snapshot.Profiles))
	}
	if !snapshot.Profiles[0].Active {
		t.Fatalf("expected active profile first in list: %#v", snapshot.Profiles)
	}
	if snapshot.Profiles[0].APIKeyMasked != "********" {
		t.Fatalf("expected masked api key, got %q", snapshot.Profiles[0].APIKeyMasked)
	}

	if err := store.SetActive(ctx, second.ID); err != nil {
		t.Fatalf("set active: %v", err)
	}
	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("load active profile: %v", err)
	}
	if loaded.ID != second.ID || loaded.APIKey != "anthropic-secret" {
		t.Fatalf("unexpected loaded active profile: %#v", loaded)
	}
}

func TestSavePreservesExistingAPIKeyWhenRequested(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "model", "profiles.db"))
	ctx := context.Background()

	saved, err := store.Save(ctx, Config{
		Name:     "Primary",
		Provider: ProviderOpenAI,
		APIType:  APITypeResponses,
		BaseURL:  DefaultBaseURL,
		APIKey:   "secret-1",
		Model:    "gpt-4.1-mini",
	}, SaveOptions{SetActive: true})
	if err != nil {
		t.Fatalf("save initial profile: %v", err)
	}

	updated, err := store.Save(ctx, Config{
		ID:       saved.ID,
		Name:     "Primary Updated",
		Provider: ProviderOpenAI,
		APIType:  APITypeChatCompletions,
		BaseURL:  DefaultBaseURL,
		APIKey:   "",
		Model:    "gpt-4o-mini",
	}, SaveOptions{PreserveAPIKey: true, SetActive: true})
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if updated.APIKey != "secret-1" {
		t.Fatalf("expected api key to be preserved, got %q", updated.APIKey)
	}
	if updated.APIType != APITypeChatCompletions || updated.Model != "gpt-4o-mini" {
		t.Fatalf("unexpected updated profile: %#v", updated)
	}
}

func TestDeleteActiveProfilePromotesNextProfile(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "model", "profiles.db"))
	ctx := context.Background()

	first, err := store.Save(ctx, Config{
		Name:     "One",
		Provider: ProviderOpenAI,
		APIType:  APITypeResponses,
		BaseURL:  DefaultBaseURL,
		APIKey:   "secret-1",
		Model:    "gpt-4.1-mini",
	}, SaveOptions{SetActive: true})
	if err != nil {
		t.Fatalf("save first: %v", err)
	}
	second, err := store.Save(ctx, Config{
		Name:     "Two",
		Provider: ProviderAnthropic,
		APIType:  APITypeMessages,
		BaseURL:  DefaultAnthropicBaseURL,
		APIKey:   "secret-2",
		Model:    "claude-3-5-haiku-latest",
	}, SaveOptions{})
	if err != nil {
		t.Fatalf("save second: %v", err)
	}

	deleted, err := store.Delete(ctx, first.ID)
	if err != nil {
		t.Fatalf("delete active profile: %v", err)
	}
	if !deleted {
		t.Fatal("expected delete to report success")
	}

	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("load after delete: %v", err)
	}
	if loaded.ID != second.ID {
		t.Fatalf("expected second profile to become active, got %#v", loaded)
	}
}

func TestClearRemovesAllProfiles(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "model", "profiles.db"))
	ctx := context.Background()

	if _, err := store.Save(ctx, Config{
		Name:     "One",
		Provider: ProviderOpenAI,
		APIType:  APITypeResponses,
		BaseURL:  DefaultBaseURL,
		APIKey:   "secret",
		Model:    "gpt-4.1-mini",
	}, SaveOptions{SetActive: true}); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	if err := store.Clear(ctx); err != nil {
		t.Fatalf("clear store: %v", err)
	}

	snapshot, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list after clear: %v", err)
	}
	if len(snapshot.Profiles) != 0 || snapshot.ActiveProfileID != "" {
		t.Fatalf("expected empty snapshot after clear, got %#v", snapshot)
	}

	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("load after clear: %v", err)
	}
	if loaded.Provider != ProviderOpenAI || loaded.APIType != APITypeResponses {
		t.Fatalf("expected default config after clear, got %#v", loaded)
	}
	if loaded.APIKey != "" || loaded.Model != "" {
		t.Fatalf("expected empty credentials after clear, got %#v", loaded)
	}
}

func TestStoreMigratesLegacyPlaintextConfigIntoEncryptedDatabase(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(root, "model", "profiles.db")
	keyPath := filepath.Join(root, "model", "secret.key")
	legacyPath := filepath.Join(root, "model", "config.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{
  "provider":"openai",
  "base_url":"https://legacy.example/v1",
  "api_key":"legacy-secret",
  "model":"gpt-legacy"
}`), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	store := NewStore(dbPath, keyPath, legacyPath)
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load migrated config: %v", err)
	}
	if loaded.Model != "gpt-legacy" || loaded.APIKey != "legacy-secret" {
		t.Fatalf("unexpected migrated config: %#v", loaded)
	}

	rawDB, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read db file: %v", err)
	}
	if strings.Contains(string(rawDB), "legacy-secret") {
		t.Fatal("database should not contain plaintext api key")
	}

	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read key file: %v", err)
	}
	if len(keyData) != 32 {
		t.Fatalf("unexpected key length: %d", len(keyData))
	}

	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy plaintext config to be retired, got err=%v", err)
	}
}
