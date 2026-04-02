package windowsautomationtool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"baize/internal/toolcontract"
)

const (
	ToolName             = "windows_automation_tool"
	defaultTimeout       = 5 * time.Second
	maxTimeout           = 10 * time.Second
	maxOutputPreviewRune = 12000
	defaultWindowLimit   = 20
	maxWindowLimit       = 100
	ToolFamilyKey        = "windows"
	ToolFamilyTitle      = "Windows"
)

var (
	currentGOOS       = func() string { return runtime.GOOS }
	runCommand        = execCommand
	powerShellProgram = func() string { return "powershell" }
)

type ToolInput struct {
	Action         string `json:"action"`
	TitleContains  string `json:"title_contains,omitempty"`
	ProcessName    string `json:"process_name,omitempty"`
	AppName        string `json:"app_name,omitempty"`
	Limit          int    `json:"limit,omitempty"`
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
		DisplayTitle:      "Windows Automation Tool",
		DisplayOrder:      34,
		Purpose:           "Run a small allowlisted set of Windows desktop automation actions through PowerShell.",
		Description:       "Inspect and control Windows desktop focus with approved actions such as listing windows, reading the frontmost window, focusing an app, or launching then focusing an app. This tool does not expose arbitrary PowerShell execution.",
		InputContract:     `Provide {"action":"frontmost_window"} or one of {"action":"list_windows","limit":20}, {"action":"focus_window","title_contains":"..."}, {"action":"focus_app","process_name":"Code.exe"}, {"action":"launch_or_focus_app","app_name":"notepad.exe"}. Optionally include {"timeout_seconds":5}. This tool is only exposed on Windows.`,
		OutputContract:    "Returns JSON with tool, shell, action, exit_code, stdout, stderr, and truncated. stdout contains compact JSON or plain text produced by the selected action.",
		InputJSONExample:  `{"action":"focus_window","title_contains":"Visual Studio Code"}`,
		OutputJSONExample: `{"tool":"windows_automation_tool","shell":"powershell","action":"focus_window","exit_code":0,"stdout":"{\"process_name\":\"Code\",\"process_id\":12345,\"title\":\"main.go - baize - Visual Studio Code\",\"success\":true}","stderr":""}`,
		Usage:             UsageText(),
	}
}

func UsageText() string {
	names := availableActionNames(currentGOOS())
	if len(names) == 0 {
		return "Windows Automation Tool is only enabled on Windows."
	}
	return strings.TrimSpace(fmt.Sprintf(`
Tool: %s
Purpose: inspect and control Windows desktop focus with one allowlisted action.

Input:
- action: required allowlisted action name
- title_contains: required for focus_window
- process_name: required for focus_app
- app_name: required for launch_or_focus_app
- limit: optional for list_windows, clamped to 1-100 and defaulting to 20
- timeout_seconds: optional timeout in seconds, clamped to 1-10 and defaulting to 5

Current action allowlist:
- %s

Use this when the user needs to inspect top-level Windows desktop windows or bring a target app/window to the foreground before another step such as screenshot capture.
`, ToolName, strings.Join(names, "\n- ")))
}

func AllowedForInterface(name string) bool {
	return true
}

func SupportedForCurrentPlatform() bool {
	return strings.EqualFold(strings.TrimSpace(currentGOOS()), "windows")
}

func NormalizeInput(raw ToolInput) ToolInput {
	limit := raw.Limit
	switch {
	case limit <= 0:
		limit = defaultWindowLimit
	case limit > maxWindowLimit:
		limit = maxWindowLimit
	}
	return ToolInput{
		Action:         strings.ToLower(strings.TrimSpace(raw.Action)),
		TitleContains:  strings.TrimSpace(raw.TitleContains),
		ProcessName:    strings.TrimSpace(raw.ProcessName),
		AppName:        strings.TrimSpace(raw.AppName),
		Limit:          limit,
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

	stdout, stderr, exitCode, err := runCommand(execCtx, powerShellProgram(), "-NoProfile", "-NonInteractive", "-Command", script)
	result := ToolResult{
		Tool:     ToolName,
		Shell:    "powershell",
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
		return result, fmt.Errorf("powershell is not available on this machine")
	default:
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return result, nil
		}
		return result, fmt.Errorf("run windows automation action %q: %w", spec.Name, err)
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
	if !strings.EqualFold(strings.TrimSpace(goos), "windows") {
		return map[string]actionSpec{}
	}
	return map[string]actionSpec{
		"focus_app": {
			Name:        "focus_app",
			Description: "bring the first visible top-level window for a process name to the foreground",
			BuildScript: buildFocusAppScript,
		},
		"focus_window": {
			Name:        "focus_window",
			Description: "bring the first visible top-level window whose title contains the provided text to the foreground",
			BuildScript: buildFocusWindowScript,
		},
		"frontmost_window": {
			Name:        "frontmost_window",
			Description: "return the current foreground window title and process",
			BuildScript: buildFrontmostWindowScript,
		},
		"launch_or_focus_app": {
			Name:        "launch_or_focus_app",
			Description: "focus a visible app window if present, otherwise launch the app and then focus it",
			BuildScript: buildLaunchOrFocusAppScript,
		},
		"list_windows": {
			Name:        "list_windows",
			Description: "list visible top-level windows with process names and titles",
			BuildScript: buildListWindowsScript,
		},
	}
}

func buildFrontmostWindowScript(ToolInput) (string, error) {
	return commonWindowScriptPrelude() + `
$info = Get-MyClawForegroundWindowInfo
if ($null -eq $info) {
  Write-Output "{}"
  exit 0
}
$info | ConvertTo-Json -Compress
`, nil
}

func buildListWindowsScript(input ToolInput) (string, error) {
	return commonWindowScriptPrelude() + fmt.Sprintf(`
$limit = %d
$items = @(Get-MyClawWindowList | Sort-Object ProcessName, Id | Select-Object -First $limit | ForEach-Object {
  [pscustomobject]@{
    process_name = $_.ProcessName
    process_id = $_.Id
    title = $_.MainWindowTitle
  }
})
$items | ConvertTo-Json -Compress
`, input.Limit), nil
}

func buildFocusWindowScript(input ToolInput) (string, error) {
	title := strings.TrimSpace(input.TitleContains)
	if title == "" {
		return "", fmt.Errorf("focus_window requires title_contains")
	}
	return commonWindowScriptPrelude() + `
$needle = ` + powerShellStringLiteral(title) + `
$candidate = Get-MyClawWindowList | Where-Object {
  $_.MainWindowTitle.IndexOf($needle, [System.StringComparison]::OrdinalIgnoreCase) -ge 0
} | Select-Object -First 1
if ($null -eq $candidate) {
  Write-Error ("no visible window matched title_contains: " + $needle)
  exit 1
}
$success = Invoke-MyClawFocus $candidate
[pscustomobject]@{
  process_name = $candidate.ProcessName
  process_id = $candidate.Id
  title = $candidate.MainWindowTitle
  success = [bool]$success
} | ConvertTo-Json -Compress
`, nil
}

func buildFocusAppScript(input ToolInput) (string, error) {
	processName := normalizedProcessBase(input.ProcessName)
	if processName == "" {
		return "", fmt.Errorf("focus_app requires process_name")
	}
	return commonWindowScriptPrelude() + `
$processName = ` + powerShellStringLiteral(processName) + `
$candidate = Get-MyClawWindowList | Where-Object {
  $_.ProcessName -ieq $processName
} | Select-Object -First 1
if ($null -eq $candidate) {
  Write-Error ("no visible window matched process_name: " + $processName)
  exit 1
}
$success = Invoke-MyClawFocus $candidate
[pscustomobject]@{
  process_name = $candidate.ProcessName
  process_id = $candidate.Id
  title = $candidate.MainWindowTitle
  success = [bool]$success
} | ConvertTo-Json -Compress
`, nil
}

func buildLaunchOrFocusAppScript(input ToolInput) (string, error) {
	appName := strings.TrimSpace(input.AppName)
	if appName == "" {
		return "", fmt.Errorf("launch_or_focus_app requires app_name")
	}
	processBase := normalizedProcessBase(appName)
	if processBase == "" {
		return "", fmt.Errorf("launch_or_focus_app requires app_name")
	}
	return commonWindowScriptPrelude() + `
$appName = ` + powerShellStringLiteral(appName) + `
$processName = ` + powerShellStringLiteral(processBase) + `
$launched = $false
$candidate = Get-MyClawWindowList | Where-Object {
  $_.ProcessName -ieq $processName
} | Select-Object -First 1
if ($null -eq $candidate) {
  try {
    Start-Process -FilePath $appName | Out-Null
    $launched = $true
  } catch {
    Write-Error ("failed to launch app: " + $appName + " :: " + $_.Exception.Message)
    exit 1
  }
  for ($i = 0; $i -lt 20; $i++) {
    Start-Sleep -Milliseconds 500
    $candidate = Get-MyClawWindowList | Where-Object {
      $_.ProcessName -ieq $processName
    } | Select-Object -First 1
    if ($null -ne $candidate) {
      break
    }
  }
}
if ($null -eq $candidate) {
  Write-Error ("app has no visible top-level window: " + $appName)
  exit 1
}
$success = Invoke-MyClawFocus $candidate
[pscustomobject]@{
  launched = [bool]$launched
  process_name = $candidate.ProcessName
  process_id = $candidate.Id
  title = $candidate.MainWindowTitle
  success = [bool]$success
} | ConvertTo-Json -Compress
`, nil
}

func commonWindowScriptPrelude() string {
	return strings.TrimSpace(`
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName Microsoft.VisualBasic
Add-Type @"
using System;
using System.Runtime.InteropServices;
using System.Text;
public static class MyClawUser32 {
  [DllImport("user32.dll")]
  public static extern bool SetForegroundWindow(IntPtr hWnd);
  [DllImport("user32.dll")]
  public static extern bool ShowWindowAsync(IntPtr hWnd, int nCmdShow);
  [DllImport("user32.dll")]
  public static extern IntPtr GetForegroundWindow();
  [DllImport("user32.dll", CharSet = CharSet.Unicode)]
  public static extern int GetWindowText(IntPtr hWnd, StringBuilder text, int count);
  [DllImport("user32.dll", SetLastError = true)]
  public static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint processId);
}
"@
function Get-MyClawWindowList {
  Get-Process | Where-Object {
    $_.MainWindowHandle -ne 0 -and -not [string]::IsNullOrWhiteSpace($_.MainWindowTitle)
  }
}
function Get-MyClawForegroundWindowInfo {
  $hwnd = [MyClawUser32]::GetForegroundWindow()
  if ($hwnd -eq [IntPtr]::Zero) {
    return $null
  }
  $text = New-Object System.Text.StringBuilder 1024
  [void][MyClawUser32]::GetWindowText($hwnd, $text, $text.Capacity)
  $processId = [uint32]0
  [void][MyClawUser32]::GetWindowThreadProcessId($hwnd, [ref]$processId)
  $proc = $null
  if ($processId -gt 0) {
    try {
      $proc = Get-Process -Id ([int]$processId) -ErrorAction Stop
    } catch {}
  }
  [pscustomobject]@{
    process_name = if ($null -ne $proc) { $proc.ProcessName } else { "" }
    process_id = [int]$processId
    title = $text.ToString()
  }
}
function Invoke-MyClawFocus($proc) {
  if ($null -eq $proc -or $proc.MainWindowHandle -eq 0) {
    return $false
  }
  try {
    [void][Microsoft.VisualBasic.Interaction]::AppActivate([int]$proc.Id)
  } catch {}
  [void][MyClawUser32]::ShowWindowAsync($proc.MainWindowHandle, 9)
  Start-Sleep -Milliseconds 150
  return [MyClawUser32]::SetForegroundWindow($proc.MainWindowHandle)
}`) + "\n"
}

func normalizedProcessBase(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	base := filepath.Base(strings.ReplaceAll(value, "\\", "/"))
	base = strings.TrimSuffix(base, filepath.Ext(base))
	return strings.TrimSpace(base)
}

func powerShellStringLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
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
