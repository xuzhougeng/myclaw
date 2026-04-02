package bashtool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"baize/internal/toolcontract"
)

const (
	ToolName             = "bash_tool"
	defaultTimeout       = 5 * time.Second
	maxTimeout           = 10 * time.Second
	maxOutputPreviewRune = 12000
	ToolFamilyKey        = "shell"
	ToolFamilyTitle      = "Shell"
)

var (
	currentGOOS = func() string { return runtime.GOOS }
	runCommand  = execCommand
)

type ToolInput struct {
	Command        string   `json:"command"`
	Args           []string `json:"args,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
}

type ToolResult struct {
	Tool      string   `json:"tool"`
	Shell     string   `json:"shell"`
	Command   string   `json:"command"`
	Args      []string `json:"args,omitempty"`
	ExitCode  int      `json:"exit_code"`
	Stdout    string   `json:"stdout,omitempty"`
	Stderr    string   `json:"stderr,omitempty"`
	Truncated bool     `json:"truncated,omitempty"`
}

type commandSpec struct {
	Name         string
	ArgVariants  [][]string
	UsageExample string
}

func Definition() toolcontract.Spec {
	return toolcontract.Spec{
		Name:              ToolName,
		FamilyKey:         ToolFamilyKey,
		FamilyTitle:       ToolFamilyTitle,
		DisplayTitle:      "Bash Tool",
		DisplayOrder:      31,
		Purpose:           "Run a small allowlisted set of read-only Bash-oriented commands on Linux and macOS.",
		Description:       "Inspect the local machine on Linux or macOS with one approved read-only command, routed through Bash semantics but still validated against a strict allowlist.",
		InputContract:     `Provide {"command":"..."} and optionally {"args":[...],"timeout_seconds":5}. Only allowlisted commands and exact argument variants are accepted. This tool is only exposed on Linux and macOS.`,
		OutputContract:    "Returns JSON with tool, shell, command, args, exit_code, stdout, stderr, and truncated.",
		InputJSONExample:  `{"command":"uname","args":["-a"]}`,
		OutputJSONExample: `{"tool":"bash_tool","shell":"bash","command":"uname","args":["-a"],"exit_code":0,"stdout":"Linux demo-host 6.8.0\n","stderr":""}`,
		Usage:             UsageText(),
	}
}

func UsageText() string {
	names := availableCommandNames(currentGOOS())
	if len(names) == 0 {
		return "Bash Tool is only enabled on Linux and macOS."
	}
	return strings.TrimSpace(fmt.Sprintf(`
Tool: %s
Purpose: inspect a Linux or macOS machine with one allowlisted read-only command.

Input:
- command: required allowlisted command name
- args: optional exact argument variant allowed for that command
- timeout_seconds: optional timeout in seconds, clamped to 1-10 and defaulting to 5

Current command allowlist:
- %s

Use this when the user needs current host/process/disk/basic shell-visible state on Linux or macOS.
`, ToolName, strings.Join(names, "\n- ")))
}

func AllowedForInterface(name string) bool {
	return true
}

func SupportedForCurrentPlatform() bool {
	goos := strings.ToLower(strings.TrimSpace(currentGOOS()))
	return goos == "linux" || goos == "darwin"
}

func Execute(ctx context.Context, input ToolInput) (ToolResult, error) {
	input = NormalizeInput(input)

	if !SupportedForCurrentPlatform() {
		return ToolResult{}, fmt.Errorf("%s is not supported on %s", ToolName, currentGOOS())
	}

	spec, ok := commandCatalog(currentGOOS())[input.Command]
	if !ok {
		return ToolResult{}, fmt.Errorf("command %q is not allowed on %s", input.Command, currentGOOS())
	}
	if err := validateArgs(spec, input.Args); err != nil {
		return ToolResult{}, err
	}

	timeout := effectiveTimeout(input.TimeoutSeconds)
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	commandLine := buildCommandLine(spec.Name, input.Args)
	stdout, stderr, exitCode, err := runCommand(execCtx, "bash", "-lc", commandLine)
	result := ToolResult{
		Tool:     ToolName,
		Shell:    "bash",
		Command:  spec.Name,
		Args:     append([]string(nil), input.Args...),
		ExitCode: exitCode,
	}
	result.Stdout, result.Truncated = truncateText(stdout, maxOutputPreviewRune)
	stderrText, stderrTruncated := truncateText(stderr, maxOutputPreviewRune)
	result.Stderr, result.Truncated = combineTruncation(result.Truncated, stderrText, stderrTruncated)

	switch {
	case err == nil:
		return result, nil
	case errors.Is(execCtx.Err(), context.DeadlineExceeded):
		return result, fmt.Errorf("command %q timed out after %s", spec.Name, timeout)
	case errors.Is(err, exec.ErrNotFound):
		return result, fmt.Errorf("bash is not available on this machine")
	default:
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return result, nil
		}
		return result, fmt.Errorf("run bash command %q: %w", spec.Name, err)
	}
}

func FormatResult(result ToolResult) (string, error) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func NormalizeInput(raw ToolInput) ToolInput {
	out := ToolInput{
		Command:        strings.ToLower(strings.TrimSpace(raw.Command)),
		TimeoutSeconds: raw.TimeoutSeconds,
	}
	for _, arg := range raw.Args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		out.Args = append(out.Args, arg)
	}
	return out
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

func buildCommandLine(command string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, command)
	parts = append(parts, args...)
	return strings.Join(parts, " ")
}

func availableCommandNames(goos string) []string {
	catalog := commandCatalog(goos)
	out := make([]string, 0, len(catalog))
	for _, spec := range catalog {
		label := spec.Name
		if usage := strings.TrimSpace(spec.UsageExample); usage != "" {
			label += " " + usage
		}
		out = append(out, label)
	}
	sort.Strings(out)
	return out
}

func commandCatalog(goos string) map[string]commandSpec {
	switch strings.ToLower(strings.TrimSpace(goos)) {
	case "darwin", "linux":
		return map[string]commandSpec{
			"date":     {Name: "date", ArgVariants: [][]string{{}}},
			"df":       {Name: "df", ArgVariants: [][]string{{}, {"-h"}}, UsageExample: "-h"},
			"hostname": {Name: "hostname", ArgVariants: [][]string{{}}},
			"id":       {Name: "id", ArgVariants: [][]string{{}}},
			"ls":       {Name: "ls", ArgVariants: [][]string{{}, {"-la"}}, UsageExample: "-la"},
			"ps":       {Name: "ps", ArgVariants: [][]string{{"-ef"}, {"aux"}}, UsageExample: "-ef"},
			"pwd":      {Name: "pwd", ArgVariants: [][]string{{}}},
			"uname":    {Name: "uname", ArgVariants: [][]string{{}, {"-a"}}, UsageExample: "-a"},
			"uptime":   {Name: "uptime", ArgVariants: [][]string{{}}},
			"whoami":   {Name: "whoami", ArgVariants: [][]string{{}}},
		}
	default:
		return map[string]commandSpec{}
	}
}

func validateArgs(spec commandSpec, args []string) error {
	if len(spec.ArgVariants) == 0 && len(args) == 0 {
		return nil
	}
	for _, allowed := range spec.ArgVariants {
		if slices.Equal(args, allowed) {
			return nil
		}
	}
	var variants []string
	for _, allowed := range spec.ArgVariants {
		if len(allowed) == 0 {
			variants = append(variants, "(no args)")
			continue
		}
		variants = append(variants, strings.Join(allowed, " "))
	}
	return fmt.Errorf("command %q only allows these args: %s", spec.Name, strings.Join(variants, ", "))
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
		}
	} else if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	return stdout.String(), stderr.String(), exitCode, err
}
