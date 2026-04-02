package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"baize/internal/knowledge"
	"baize/internal/windowsautomationtool"
)

func TestExecuteWindowsAutomationToolUsesSharedProvider(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	service := NewService(store, nil, nil)

	original := executeWindowsAutomationTool
	t.Cleanup(func() { executeWindowsAutomationTool = original })

	executeWindowsAutomationTool = func(_ context.Context, input windowsautomationtool.ToolInput) (windowsautomationtool.ToolResult, error) {
		if input.Action != "focus_window" {
			t.Fatalf("input.Action = %q, want focus_window", input.Action)
		}
		if input.TitleContains != "Visual Studio Code" {
			t.Fatalf("input.TitleContains = %q, want Visual Studio Code", input.TitleContains)
		}
		return windowsautomationtool.ToolResult{
			Tool:     windowsautomationtool.ToolName,
			Shell:    "powershell",
			Action:   "focus_window",
			ExitCode: 0,
			Stdout:   `{"success":true}`,
		}, nil
	}

	provider := newLocalAgentToolProvider(service).(*localAgentToolProvider)
	output, err := provider.executeWindowsAutomationTool(context.Background(), MessageContext{Interface: "weixin"}, `{"action":"focus_window","title_contains":"Visual Studio Code"}`)
	if err != nil {
		t.Fatalf("executeWindowsAutomationTool() error = %v", err)
	}
	if !strings.Contains(output, `"tool": "windows_automation_tool"`) {
		t.Fatalf("executeWindowsAutomationTool() output missing tool name: %s", output)
	}
	if !strings.Contains(output, `"action": "focus_window"`) {
		t.Fatalf("executeWindowsAutomationTool() output missing action: %s", output)
	}
}
