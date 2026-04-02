package osascripttool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"baize/internal/toolcontract"
)

const (
	ToolName             = "osascript_tool"
	defaultTimeout       = 5 * time.Second
	maxTimeout           = 10 * time.Second
	maxOutputPreviewRune = 12000
	ToolFamilyKey        = "macos"
	ToolFamilyTitle      = "macOS"
)

var (
	currentGOOS     = func() string { return runtime.GOOS }
	runCommand      = execCommand
	osascriptBinary = func() string { return "osascript" }
)

type ToolInput struct {
	Action         string `json:"action"`
	AppName        string `json:"app_name,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

type ToolResult struct {
	Tool      string `json:"tool"`
	Shell     string `json:"shell"`
	Action    string `json:"action"`
	ExitCode  int    `json:"exit_code"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

type actionSpec struct {
	Name        string
	Description string
	BuildScript func(ToolInput) (string, error)
}

func Definition() toolcontract.Spec {
	return toolcontract.Spec{
		Name:              ToolName,
		FamilyKey:         ToolFamilyKey,
		FamilyTitle:       ToolFamilyTitle,
		DisplayTitle:      "AppleScript Tool",
		DisplayOrder:      33,
		Purpose:           "Run a small allowlisted set of macOS desktop inspection and activation actions through osascript.",
		Description:       "Inspect the current macOS desktop state with approved AppleScript actions such as the frontmost app or visible apps, and optionally activate or open then activate a named application without exposing arbitrary script execution.",
		InputContract:     `Provide {"action":"frontmost_app"} or one of {"action":"visible_apps"}, {"action":"front_window_title"}, {"action":"activate_app","app_name":"Safari"}, {"action":"open_or_activate_app","app_name":"Visual Studio Code"}. Optionally include {"timeout_seconds":5}. This tool is only exposed on macOS.`,
		OutputContract:    "Returns JSON with tool, shell, action, exit_code, stdout, stderr, and truncated. stdout contains the plain-text result produced by the selected action.",
		InputJSONExample:  `{"action":"activate_app","app_name":"Safari"}`,
		OutputJSONExample: `{"tool":"osascript_tool","shell":"osascript","action":"activate_app","exit_code":0,"stdout":"activated:Safari\n","stderr":""}`,
		Usage:             UsageText(),
	}
}

func UsageText() string {
	names := availableActionNames(currentGOOS())
	if len(names) == 0 {
		return "AppleScript Tool is only enabled on macOS."
	}
	return strings.TrimSpace(fmt.Sprintf(`
Tool: %s
Purpose: inspect current macOS desktop state or activate a target app with one allowlisted AppleScript action.

Input:
- action: required allowlisted action name
- app_name: required for activate_app and open_or_activate_app
- timeout_seconds: optional timeout in seconds, clamped to 1-10 and defaulting to 5

Current action allowlist:
- %s

Use this when the user needs frontmost macOS app/window state, or needs to bring a target app to the foreground before another step such as screenshot capture.
`, ToolName, strings.Join(names, "\n- ")))
}

func AllowedForInterface(name string) bool {
	return true
}

func SupportedForCurrentPlatform() bool {
	return strings.EqualFold(strings.TrimSpace(currentGOOS()), "darwin")
}

func NormalizeInput(raw ToolInput) ToolInput {
	return ToolInput{
		Action:         strings.ToLower(strings.TrimSpace(raw.Action)),
		AppName:        strings.TrimSpace(raw.AppName),
		TimeoutSeconds: raw.TimeoutSeconds,
	}
}

func Execute(ctx context.Context, input ToolInput) (ToolResult, error) {
	input = NormalizeInput(input)

	if !SupportedForCurrentPlatform() {
		return ToolResult{}, fmt.Errorf("%s is not supported on %s", ToolName, currentGOOS())
	}

	spec, ok := actionCatalog(currentGOOS())[input.Action]
	if !ok {
		return ToolResult{}, fmt.Errorf("action %q is not allowed on %s", input.Action, currentGOOS())
	}
	script, err := spec.BuildScript(input)
	if err != nil {
		return ToolResult{}, err
	}

	timeout := effectiveTimeout(input.TimeoutSeconds)
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	stdout, stderr, exitCode, err := runCommand(execCtx, osascriptBinary(), "-e", script)
	result := ToolResult{
		Tool:     ToolName,
		Shell:    "osascript",
		Action:   spec.Name,
		ExitCode: exitCode,
	}
	result.Stdout, result.Truncated = truncateText(stdout, maxOutputPreviewRune)
	stderrText, stderrTruncated := truncateText(stderr, maxOutputPreviewRune)
	result.Stderr, result.Truncated = combineTruncation(result.Truncated, stderrText, stderrTruncated)

	switch {
	case err == nil:
		return result, nil
	case errors.Is(execCtx.Err(), context.DeadlineExceeded):
		return result, fmt.Errorf("action %q timed out after %s", spec.Name, timeout)
	case errors.Is(err, exec.ErrNotFound):
		return result, fmt.Errorf("osascript is not available on this machine")
	default:
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return result, nil
		}
		return result, fmt.Errorf("run osascript action %q: %w", spec.Name, err)
	}
}

func FormatResult(result ToolResult) (string, error) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func effectiveTimeout(seconds int) time.Duration {
	timeout := defaultTimeout
	if seconds > 0 {
		timeout = time.Duration(seconds) * time.Second
	}
	if timeout > maxTimeout {
		timeout = maxTimeout
	}
	return timeout
}

func availableActionNames(goos string) []string {
	catalog := actionCatalog(goos)
	out := make([]string, 0, len(catalog))
	for _, spec := range catalog {
		label := spec.Name
		if desc := strings.TrimSpace(spec.Description); desc != "" {
			label += ": " + desc
		}
		out = append(out, label)
	}
	sort.Strings(out)
	return out
}

func actionCatalog(goos string) map[string]actionSpec {
	if !strings.EqualFold(strings.TrimSpace(goos), "darwin") {
		return map[string]actionSpec{}
	}
	return map[string]actionSpec{
		"activate_app": {
			Name:        "activate_app",
			Description: "bring the named application to the foreground",
			BuildScript: buildActivateAppScript,
		},
		"front_window_title": {
			Name:        "front_window_title",
			Description: "return the front window title for the frontmost app when available",
			BuildScript: buildFrontWindowTitleScript,
		},
		"frontmost_app": {
			Name:        "frontmost_app",
			Description: "return the name of the currently frontmost application",
			BuildScript: buildFrontmostAppScript,
		},
		"open_or_activate_app": {
			Name:        "open_or_activate_app",
			Description: "launch the named application if needed, then activate it",
			BuildScript: buildOpenOrActivateAppScript,
		},
		"visible_apps": {
			Name:        "visible_apps",
			Description: "return the currently visible macOS application processes",
			BuildScript: buildVisibleAppsScript,
		},
	}
}

func buildFrontmostAppScript(ToolInput) (string, error) {
	return `tell application "System Events" to get name of first application process whose frontmost is true`, nil
}

func buildVisibleAppsScript(ToolInput) (string, error) {
	return `tell application "System Events" to get name of every application process whose visible is true`, nil
}

func buildFrontWindowTitleScript(ToolInput) (string, error) {
	return strings.TrimSpace(`
tell application "System Events"
	set frontApp to first application process whose frontmost is true
	try
		return name of front window of frontApp
	on error
		return ""
	end try
end tell`), nil
}

func buildActivateAppScript(input ToolInput) (string, error) {
	appName := strings.TrimSpace(input.AppName)
	if appName == "" {
		return "", fmt.Errorf("activate_app requires app_name")
	}
	return strings.TrimSpace(`
set appName to ` + appleScriptStringLiteral(appName) + `
tell application appName to activate
return "activated:" & appName`), nil
}

func buildOpenOrActivateAppScript(input ToolInput) (string, error) {
	appName := strings.TrimSpace(input.AppName)
	if appName == "" {
		return "", fmt.Errorf("open_or_activate_app requires app_name")
	}
	return strings.TrimSpace(`
set appName to ` + appleScriptStringLiteral(appName) + `
tell application appName to launch
tell application appName to activate
return "opened_or_activated:" & appName`), nil
}

func appleScriptStringLiteral(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(value) + `"`
}

func truncateText(value string, limit int) (string, bool) {
	runes := []rune(value)
	if len(runes) <= limit {
		return value, false
	}
	return string(runes[:limit]) + "\n...[truncated]", true
}

func combineTruncation(current bool, value string, truncated bool) (string, bool) {
	return value, current || truncated
}

func execCommand(ctx context.Context, name string, args ...string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return stdout.String(), stderr.String(), exitCode, err
}
