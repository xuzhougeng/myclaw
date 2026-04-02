package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"baize/internal/knowledge"
	"baize/internal/screencapture"
)

func TestExecuteScreenCaptureUsesSharedProviderAndAnalyzer(t *testing.T) {
	t.Parallel()

	store := knowledge.NewStore(filepath.Join(t.TempDir(), "app.db"))
	service := NewService(store, fakeAI{
		configured:   true,
		imageSummary: "屏幕上显示编辑器。",
	}, nil)

	original := executeScreenCapture
	t.Cleanup(func() { executeScreenCapture = original })

	executeScreenCapture = func(ctx context.Context, input screencapture.ToolInput, opts screencapture.ExecuteOptions) (screencapture.ToolResult, error) {
		if input.MaxDimension != 1200 {
			t.Fatalf("input.MaxDimension = %d, want 1200", input.MaxDimension)
		}
		if opts.Analyzer == nil {
			t.Fatal("expected analyzer to be wired when ai service exists")
		}
		summary, err := opts.Analyzer(ctx, "screen.jpg", "data:image/jpeg;base64,AAAA")
		if err != nil {
			t.Fatalf("Analyzer() error = %v", err)
		}
		return screencapture.ToolResult{
			Tool:           screencapture.ToolName,
			Path:           "/tmp/screen.jpg",
			MIMEType:       "image/jpeg",
			Width:          1200,
			Height:         750,
			DisplayIndex:   0,
			CapturedAt:     "2026-04-02T12:00:00Z",
			Analyze:        true,
			AnalysisStatus: "summarized",
			Summary:        summary,
		}, nil
	}

	provider := newLocalAgentToolProvider(service).(*localAgentToolProvider)
	output, err := provider.executeScreenCapture(context.Background(), MessageContext{Interface: "weixin"}, `{"analyze":true,"max_dimension":1200}`)
	if err != nil {
		t.Fatalf("executeScreenCapture() error = %v", err)
	}
	if !strings.Contains(output, `"tool": "screen_capture"`) {
		t.Fatalf("executeScreenCapture() output missing tool name: %s", output)
	}
	if !strings.Contains(output, "屏幕上显示编辑器。") {
		t.Fatalf("executeScreenCapture() output missing summary: %s", output)
	}
}
