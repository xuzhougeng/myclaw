package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	appsvc "baize/internal/app"
	"baize/internal/knowledge"
	"baize/internal/modelconfig"
	"baize/internal/projectstate"
	"baize/internal/promptlib"
	"baize/internal/reminder"
	"baize/internal/sessionstate"
	"baize/internal/weixin"
)

func TestDesktopSettingsCanBeSavedAndReloaded(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))

	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	bridge := weixin.NewBridge(weixin.NewClient("", ""), service, reminders, weixin.BridgeConfig{DataDir: root})
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, bridge)

	saved, err := app.SaveSettings(AppSettingsInput{
		WeixinHistoryMessages: 22,
		WeixinHistoryRunes:    888,
		WeixinEverythingPath:  `C:\Tools\Everything\es.exe`,
		DisabledToolNames:     []string{"local::everything_file_search", "mcp.docs::lookup"},
	})
	if err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if saved.WeixinHistoryMessages != 22 || saved.WeixinHistoryRunes != 888 || saved.WeixinEverythingPath != `C:\Tools\Everything\es.exe` {
		t.Fatalf("unexpected saved settings: %#v", saved)
	}
	if strings.Join(saved.DisabledToolNames, ",") != "local::everything_file_search,mcp.docs::lookup" {
		t.Fatalf("unexpected disabled tools: %#v", saved.DisabledToolNames)
	}

	messages, runes := service.WeixinHistoryLimits()
	if messages != 22 || runes != 888 {
		t.Fatalf("expected live service settings to update, got messages=%d runes=%d", messages, runes)
	}
	if bridge.EverythingPath() != `C:\Tools\Everything\es.exe` {
		t.Fatalf("expected live bridge settings to update, got %q", bridge.EverythingPath())
	}

	reloadedService := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	reloadedBridge := weixin.NewBridge(weixin.NewClient("", ""), reloadedService, reminders, weixin.BridgeConfig{DataDir: root})
	reloadedApp := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, reloadedService, sessionStore, reminders, reloadedBridge)

	reloaded, err := reloadedApp.GetSettings()
	if err != nil {
		t.Fatalf("get reloaded settings: %v", err)
	}
	if reloaded.WeixinHistoryMessages != 22 || reloaded.WeixinHistoryRunes != 888 || reloaded.WeixinEverythingPath != `C:\Tools\Everything\es.exe` {
		t.Fatalf("unexpected reloaded settings: %#v", reloaded)
	}
	if strings.Join(reloaded.DisabledToolNames, ",") != "local::everything_file_search,mcp.docs::lookup" {
		t.Fatalf("unexpected reloaded disabled tools: %#v", reloaded.DisabledToolNames)
	}

	messages, runes = reloadedService.WeixinHistoryLimits()
	if messages != 22 || runes != 888 {
		t.Fatalf("expected persisted service settings to load, got messages=%d runes=%d", messages, runes)
	}
	if reloadedBridge.EverythingPath() != `C:\Tools\Everything\es.exe` {
		t.Fatalf("expected persisted bridge settings to load, got %q", reloadedBridge.EverythingPath())
	}
	if strings.Join(reloadedService.DisabledAgentTools(), ",") != "local::everything_file_search,mcp.docs::lookup" {
		t.Fatalf("expected persisted disabled tools on service, got %#v", reloadedService.DisabledAgentTools())
	}
}

func TestDesktopSettingsRejectNegativeValues(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	_, err := app.SaveSettings(AppSettingsInput{
		WeixinHistoryMessages: -1,
		WeixinHistoryRunes:    360,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "不能小于 0") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestDesktopSettingsSavePreservesPersistedChatSessionSelection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := knowledge.NewStore(filepath.Join(root, "app.db"))
	projectStore := projectstate.NewStore(filepath.Join(root, "app.db"))
	promptStore := promptlib.NewStore(filepath.Join(root, "app.db"))
	reminders := reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db")))
	sessionStore := sessionstate.NewStore(filepath.Join(root, "app.db"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	app := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, service, sessionStore, reminders, nil)

	first, err := app.GetChatState()
	if err != nil {
		t.Fatalf("get chat state: %v", err)
	}

	if _, err := app.SaveSettings(AppSettingsInput{
		WeixinHistoryMessages: 12,
		WeixinHistoryRunes:    360,
	}); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	reloadedService := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	reloadedApp := NewDesktopApp(root, store, promptStore, projectStore, nil, nil, reloadedService, sessionStore, reminders, nil)
	state, err := reloadedApp.GetChatState()
	if err != nil {
		t.Fatalf("reloaded get chat state: %v", err)
	}
	if state.SessionID != first.SessionID {
		t.Fatalf("expected settings save to preserve chat selection %q, got %#v", first.SessionID, state)
	}
}

func TestDesktopSettingsPersistScreenTraceOptions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	appDBPath := filepath.Join(root, "app.db")
	store := knowledge.NewStore(appDBPath)
	projectStore := projectstate.NewStore(appDBPath)
	promptStore := promptlib.NewStore(appDBPath)
	reminders := reminder.NewManager(reminder.NewStore(appDBPath))
	sessionStore := sessionstate.NewStore(appDBPath)
	modelStore := modelconfig.NewStore(appDBPath, filepath.Join(root, "model", "secret.key"))
	service := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)

	savedProfile, err := modelStore.Save(context.Background(), modelconfig.Config{
		Name:     "ScreenTrace Mini",
		Provider: modelconfig.ProviderOpenAI,
		APIType:  modelconfig.APITypeResponses,
		BaseURL:  "https://api.openai.com/v1",
		APIKey:   "sk-test",
		Model:    "gpt-4.1-mini",
	}, modelconfig.SaveOptions{})
	if err != nil {
		t.Fatalf("save model profile: %v", err)
	}

	app := NewDesktopApp(root, store, promptStore, projectStore, modelStore, nil, service, sessionStore, reminders, nil)
	result, err := app.SaveSettings(AppSettingsInput{
		WeixinHistoryMessages:       12,
		WeixinHistoryRunes:          360,
		ScreenTraceEnabled:          true,
		ScreenTraceIntervalSeconds:  30,
		ScreenTraceRetentionDays:    14,
		ScreenTraceVisionProfileID:  savedProfile.ID,
		ScreenTraceWriteDigestsToKB: true,
	})
	if err != nil {
		t.Fatalf("save screentrace settings: %v", err)
	}
	if !result.ScreenTraceEnabled || result.ScreenTraceIntervalSeconds != 30 || result.ScreenTraceRetentionDays != 14 || result.ScreenTraceVisionProfileID != savedProfile.ID || !result.ScreenTraceWriteDigestsToKB {
		t.Fatalf("unexpected saved screentrace settings: %#v", result)
	}

	reloadedService := appsvc.NewServiceWithRuntime(store, nil, reminders, nil, sessionStore, promptStore)
	reloadedApp := NewDesktopApp(root, store, promptStore, projectStore, modelStore, nil, reloadedService, sessionStore, reminders, nil)
	reloaded, err := reloadedApp.GetSettings()
	if err != nil {
		t.Fatalf("get settings after reload: %v", err)
	}
	if !reloaded.ScreenTraceEnabled || reloaded.ScreenTraceIntervalSeconds != 30 || reloaded.ScreenTraceRetentionDays != 14 || reloaded.ScreenTraceVisionProfileID != savedProfile.ID || !reloaded.ScreenTraceWriteDigestsToKB {
		t.Fatalf("unexpected reloaded screentrace settings: %#v", reloaded)
	}
}
