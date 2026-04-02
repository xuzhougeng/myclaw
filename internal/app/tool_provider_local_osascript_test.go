package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"baize/internal/knowledge"
	"baize/internal/osascripttool"
)

func TestExecuteOsaScriptToolUsesSharedProvider(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	service := NewService(store, nil, nil)

	original := executeOsaScriptTool
	t.Cleanup(func() { executeOsaScriptTool = original })

	executeOsaScriptTool = func(_ context.Context, input osascripttool.ToolInput) (osascripttool.ToolResult, error) {
		if input.Action != "activate_app" {
			t.Fatalf("input.Action = %q, want activate_app", input.Action)
		}
		if input.AppName != "Safari" {
			t.Fatalf("input.AppName = %q, want Safari", input.AppName)
		}
		return osascripttool.ToolResult{
			Tool:     osascripttool.ToolName,
			Shell:    "osascript",
			Action:   "activate_app",
			ExitCode: 0,
			Stdout:   "activated:Safari\n",
		}, nil
	}

	provider := newLocalAgentToolProvider(service).(*localAgentToolProvider)
	output, err := provider.executeOsaScriptTool(context.Background(), MessageContext{Interface: "weixin"}, `{"action":"activate_app","app_name":"Safari"}`)
	if err != nil {
		t.Fatalf("executeOsaScriptTool() error = %v", err)
	}
	if !strings.Contains(output, `"tool": "osascript_tool"`) {
		t.Fatalf("executeOsaScriptTool() output missing tool name: %s", output)
	}
	if !strings.Contains(output, `"action": "activate_app"`) {
		t.Fatalf("executeOsaScriptTool() output missing action: %s", output)
	}
}
