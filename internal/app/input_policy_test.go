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
			name:               "kb stats stays stateless",
			input:              "/kb stats",
			wantCommand:        "/kb",
			wantExecution:      CommandExecutionService,
			wantKnown:          true,
			wantControl:        false,
			wantPersistHistory: false,
			wantActivateConv:   false,
		},
		{
			name:               "kb switch stays stateless",
			input:              "/kb switch Alpha",
			wantCommand:        "/kb",
			wantExecution:      CommandExecutionService,
			wantKnown:          true,
			wantControl:        false,
			wantPersistHistory: false,
			wantActivateConv:   false,
		},
		{
			name:               "kb list activates conversation",
			input:              "/kb list",
			wantCommand:        "/kb",
			wantExecution:      CommandExecutionService,
			wantKnown:          true,
			wantControl:        false,
			wantPersistHistory: true,
			wantActivateConv:   true,
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
	if got := CanonicalizeCommandInput("/kb remember hello"); got != "/kb remember hello" {
		t.Fatalf("expected /kb remember hello, got %q", got)
	}
	if got := CanonicalizeCommandInput("/unknown hello"); got != "/unknown hello" {
		t.Fatalf("expected unknown command to remain unchanged, got %q", got)
	}
}

func TestParseNewConversationMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    Mode
		wantErr string
	}{
		{name: "default agent", input: "/new", want: ModeAgent},
		{name: "ask", input: "/new ask", want: ModeAsk},
		{name: "agent", input: "/new agent", want: ModeAgent},
		{name: "invalid mode", input: "/new kb", wantErr: "用法: /new [ask|agent]"},
		{name: "too many args", input: "/new ask extra", wantErr: "用法: /new [ask|agent]"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseNewConversationMode(tc.input)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("expected error %q, got mode=%q err=%v", tc.wantErr, got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse new conversation mode: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected mode %q, got %q", tc.want, got)
			}
		})
	}
}
