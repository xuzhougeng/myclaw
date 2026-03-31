package app

import "testing"

func TestInspectInputPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		input              string
		wantCommand        string
		wantExecution      CommandExecutionKind
		wantKnown          bool
		wantControl        bool
		wantPersistHistory bool
		wantActivateConv   bool
	}{
		{
			name:               "new conversation control",
			input:              "/new",
			wantCommand:        "/new",
			wantExecution:      CommandExecutionControl,
			wantKnown:          true,
			wantControl:        true,
			wantPersistHistory: true,
			wantActivateConv:   true,
		},
		{
			name:               "help alias",
			input:              "／h",
			wantCommand:        "/help",
			wantExecution:      CommandExecutionService,
			wantKnown:          true,
			wantControl:        false,
			wantPersistHistory: false,
			wantActivateConv:   false,
		},
		{
			name:               "find transport tool",
			input:              "/find output.csv",
			wantCommand:        "/find",
			wantExecution:      CommandExecutionTransportTool,
			wantKnown:          true,
			wantControl:        false,
			wantPersistHistory: false,
			wantActivateConv:   false,
		},
		{
			name:               "send transport tool",
			input:              "/send 1",
			wantCommand:        "/send",
			wantExecution:      CommandExecutionTransportTool,
			wantKnown:          true,
			wantControl:        false,
			wantPersistHistory: false,
			wantActivateConv:   false,
		},
		{
			name:      "unknown input",
			input:     "/unknown",
			wantKnown: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := InspectInputPolicy(tc.input)
			if got.Command != tc.wantCommand {
				t.Fatalf("expected command %q, got %#v", tc.wantCommand, got)
			}
			if got.Execution != tc.wantExecution {
				t.Fatalf("expected execution %q, got %#v", tc.wantExecution, got)
			}
			if got.IsKnownCommand != tc.wantKnown {
				t.Fatalf("expected known=%v, got %#v", tc.wantKnown, got)
			}
			if got.IsConversationControl != tc.wantControl {
				t.Fatalf("expected control=%v, got %#v", tc.wantControl, got)
			}
			if got.PersistHistory != tc.wantPersistHistory {
				t.Fatalf("expected persist=%v, got %#v", tc.wantPersistHistory, got)
			}
			if got.ActivateConversation != tc.wantActivateConv {
				t.Fatalf("expected activate=%v, got %#v", tc.wantActivateConv, got)
			}
		})
	}
}

func TestCanonicalizeCommandInput(t *testing.T) {
	t.Parallel()

	if got := CanonicalizeCommandInput("／h"); got != "/help" {
		t.Fatalf("expected /help, got %q", got)
	}
	if got := CanonicalizeCommandInput("/r hello"); got != "/remember hello" {
		t.Fatalf("expected /remember hello, got %q", got)
	}
	if got := CanonicalizeCommandInput("/unknown hello"); got != "/unknown hello" {
		t.Fatalf("expected unknown command to remain unchanged, got %q", got)
	}
}
