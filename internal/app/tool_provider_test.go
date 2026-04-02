package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"baize/internal/bashtool"
	"baize/internal/dirlist"
	"baize/internal/filesearch"
	"baize/internal/knowledge"
	"baize/internal/osascripttool"
	"baize/internal/powershelltool"
	"baize/internal/reminder"
	"baize/internal/screencapture"
	"baize/internal/windowsautomationtool"
)

func TestLocalToolSideEffectLabels(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	service := NewService(store, nil, nil)
	ctx := context.Background()
	mc := MessageContext{}

	defs, err := service.toolProviders.Definitions(ctx, mc)
	if err != nil {
		t.Fatalf("Definitions() failed: %v", err)
	}

	// Strip provider prefix and build a lookup map.
	levelByTool := make(map[string]string, len(defs))
	for _, def := range defs {
		_, name, ok := strings.Cut(def.Name, "::")
		if !ok {
			name = def.Name
		}
		levelByTool[name] = def.SideEffectLevel
	}

	want := map[string]string{
		"knowledge_search":  string(ToolSideEffectReadOnly),
		dirlist.ToolName:    string(ToolSideEffectReadOnly),
		filesearch.ToolName: string(ToolSideEffectReadOnly),
		"reminder_list":     string(ToolSideEffectReadOnly),
		"remember":          string(ToolSideEffectSoftWrite),
		"append_knowledge":  string(ToolSideEffectSoftWrite),
		"reminder_add":      string(ToolSideEffectSoftWrite),
		"forget_knowledge":  string(ToolSideEffectDestructive),
		"reminder_remove":   string(ToolSideEffectDestructive),
	}
	if bashtool.SupportedForCurrentPlatform() {
		want[bashtool.ToolName] = string(ToolSideEffectReadOnly)
	}
	if powershelltool.SupportedForCurrentPlatform() {
		want[powershelltool.ToolName] = string(ToolSideEffectReadOnly)
	}
	if screencapture.SupportedForCurrentPlatform() {
		want[screencapture.ToolName] = string(ToolSideEffectReadOnly)
	}
	if osascripttool.SupportedForCurrentPlatform() {
		want[osascripttool.ToolName] = string(ToolSideEffectSoftWrite)
	}
	if windowsautomationtool.SupportedForCurrentPlatform() {
		want[windowsautomationtool.ToolName] = string(ToolSideEffectSoftWrite)
	}

	for tool, wantLevel := range want {
		got, ok := levelByTool[tool]
		if !ok {
			t.Errorf("tool %q not found in definitions", tool)
			continue
		}
		if got != wantLevel {
			t.Errorf("tool %q SideEffectLevel = %q, want %q", tool, got, wantLevel)
		}
	}
}

func TestLocalToolFamiliesAreExposed(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	service := NewService(store, nil, nil)

	defs, err := service.toolProviders.Definitions(context.Background(), MessageContext{})
	if err != nil {
		t.Fatalf("Definitions() failed: %v", err)
	}

	familyByTool := make(map[string]string, len(defs))
	titleByTool := make(map[string]string, len(defs))
	for _, def := range defs {
		_, name, ok := strings.Cut(def.Name, "::")
		if !ok {
			name = def.Name
		}
		familyByTool[name] = def.FamilyKey
		titleByTool[name] = def.DisplayTitle
	}

	wantFamilies := map[string]string{
		knowledge.SearchToolName:   knowledge.ToolFamilyKey,
		knowledge.RememberToolName: knowledge.ToolFamilyKey,
		knowledge.AppendToolName:   knowledge.ToolFamilyKey,
		knowledge.ForgetToolName:   knowledge.ToolFamilyKey,
		reminder.ListToolName:      reminder.ToolFamilyKey,
		reminder.AddToolName:       reminder.ToolFamilyKey,
		reminder.RemoveToolName:    reminder.ToolFamilyKey,
	}
	if bashtool.SupportedForCurrentPlatform() {
		wantFamilies[bashtool.ToolName] = bashtool.ToolFamilyKey
	}
	if powershelltool.SupportedForCurrentPlatform() {
		wantFamilies[powershelltool.ToolName] = powershelltool.ToolFamilyKey
	}
	if screencapture.SupportedForCurrentPlatform() {
		wantFamilies[screencapture.ToolName] = screencapture.ToolFamilyKey
	}
	if osascripttool.SupportedForCurrentPlatform() {
		wantFamilies[osascripttool.ToolName] = osascripttool.ToolFamilyKey
	}
	if windowsautomationtool.SupportedForCurrentPlatform() {
		wantFamilies[windowsautomationtool.ToolName] = windowsautomationtool.ToolFamilyKey
	}
	for tool, wantFamily := range wantFamilies {
		if got := familyByTool[tool]; got != wantFamily {
			t.Errorf("tool %q FamilyKey = %q, want %q", tool, got, wantFamily)
		}
		if strings.TrimSpace(titleByTool[tool]) == "" {
			t.Errorf("tool %q missing DisplayTitle", tool)
		}
	}
}

func TestHostToolsExposedOnWeixin(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	service := NewService(store, nil, nil)

	defs, err := service.toolProviders.Definitions(context.Background(), MessageContext{Interface: "weixin"})
	if err != nil {
		t.Fatalf("Definitions() failed: %v", err)
	}
	var (
		hasDirlist    bool
		hasBash       bool
		hasPowerShell bool
		hasScreen     bool
		hasOsaScript  bool
		hasWinAuto    bool
	)
	for _, def := range defs {
		if strings.HasSuffix(def.Name, "::bash_tool") {
			hasBash = true
		}
		if strings.HasSuffix(def.Name, "::powershell_tool") {
			hasPowerShell = true
		}
		if strings.HasSuffix(def.Name, "::list_directory") {
			hasDirlist = true
		}
		if strings.HasSuffix(def.Name, "::screen_capture") {
			hasScreen = true
		}
		if strings.HasSuffix(def.Name, "::osascript_tool") {
			hasOsaScript = true
		}
		if strings.HasSuffix(def.Name, "::windows_automation_tool") {
			hasWinAuto = true
		}
	}
	if !hasDirlist {
		t.Fatalf("expected directory listing tool in weixin definitions: %#v", defs)
	}
	if bashtool.SupportedForCurrentPlatform() && !hasBash {
		t.Fatalf("expected bash tool in weixin definitions: %#v", defs)
	}
	if powershelltool.SupportedForCurrentPlatform() && !hasPowerShell {
		t.Fatalf("expected powershell tool in weixin definitions: %#v", defs)
	}
	if screencapture.SupportedForCurrentPlatform() && !hasScreen {
		t.Fatalf("expected screen capture tool in weixin definitions: %#v", defs)
	}
	if osascripttool.SupportedForCurrentPlatform() && !hasOsaScript {
		t.Fatalf("expected osascript tool in weixin definitions: %#v", defs)
	}
	if windowsautomationtool.SupportedForCurrentPlatform() && !hasWinAuto {
		t.Fatalf("expected windows automation tool in weixin definitions: %#v", defs)
	}
}

func TestDisabledAgentToolIsFilteredFromDefinitions(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	service := NewService(store, nil, nil)
	service.SetDisabledAgentTools([]string{"local::everything_file_search"})

	allDefs, err := service.ListAllAgentToolDefinitions(context.Background(), MessageContext{})
	if err != nil {
		t.Fatalf("ListAllAgentToolDefinitions() failed: %v", err)
	}
	filteredDefs, err := service.ListAgentToolDefinitions(context.Background(), MessageContext{})
	if err != nil {
		t.Fatalf("ListAgentToolDefinitions() failed: %v", err)
	}

	var allHasFileSearch bool
	for _, def := range allDefs {
		if def.Name == "local::everything_file_search" {
			allHasFileSearch = true
			break
		}
	}
	if !allHasFileSearch {
		t.Fatalf("expected disabled tool to remain visible in all definitions: %#v", allDefs)
	}

	for _, def := range filteredDefs {
		if def.Name == "local::everything_file_search" {
			t.Fatalf("expected disabled tool to be filtered out: %#v", filteredDefs)
		}
	}
}

func TestExecuteAgentToolRejectsDisabledTool(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	service := NewService(store, nil, nil)
	service.SetDisabledAgentTools([]string{"local::everything_file_search"})

	_, err := service.ExecuteAgentTool(context.Background(), MessageContext{}, "local::everything_file_search", `{"query":"report.csv"}`)
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected disabled tool error, got %v", err)
	}
}
