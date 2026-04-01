package powershelltool

import (
	"context"
	"testing"
)

func TestDefinitionHasFamilyMetadata(t *testing.T) {
	t.Parallel()

	spec := Definition().Normalized()
	if spec.Name != ToolName {
		t.Fatalf("unexpected tool name: %#v", spec)
	}
	if spec.FamilyKey != ToolFamilyKey || spec.FamilyTitle != ToolFamilyTitle {
		t.Fatalf("unexpected family metadata: %#v", spec)
	}
	if spec.DisplayTitle == "" || spec.OutputJSONExample == "" {
		t.Fatalf("expected display title and output example: %#v", spec)
	}
}

func TestExecuteUsesAllowlistedCommand(t *testing.T) {
	oldGOOS := currentGOOS
	oldRun := runCommand
	oldProgram := powerShellProgram
	currentGOOS = func() string { return "windows" }
	powerShellProgram = func() string { return "powershell" }
	runCommand = func(_ context.Context, name string, args ...string) (string, string, int, error) {
		if name != "powershell" {
			t.Fatalf("unexpected runner: %q", name)
		}
		if len(args) != 3 || args[0] != "-NoProfile" || args[1] != "-Command" || args[2] != "Get-Process" {
			t.Fatalf("unexpected args: %#v", args)
		}
		return "Handles  NPM(K)\n", "", 0, nil
	}
	t.Cleanup(func() {
		currentGOOS = oldGOOS
		runCommand = oldRun
		powerShellProgram = oldProgram
	})

	result, err := Execute(context.Background(), ToolInput{Command: "Get-Process"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Tool != ToolName || result.Command != "Get-Process" || result.Shell != "powershell" || result.ExitCode != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Stdout != "Handles  NPM(K)\n" {
		t.Fatalf("unexpected stdout: %#v", result)
	}
}

func TestExecuteRejectsDisallowedArgs(t *testing.T) {
	oldGOOS := currentGOOS
	currentGOOS = func() string { return "windows" }
	t.Cleanup(func() {
		currentGOOS = oldGOOS
	})

	_, err := Execute(context.Background(), ToolInput{
		Command: "Get-Process",
		Args:    []string{"-Name", "explorer"},
	})
	if err == nil || err.Error() == "" {
		t.Fatal("expected arg validation error")
	}
}

func TestSupportedForCurrentPlatform(t *testing.T) {
	oldGOOS := currentGOOS
	t.Cleanup(func() {
		currentGOOS = oldGOOS
	})

	currentGOOS = func() string { return "windows" }
	if !SupportedForCurrentPlatform() {
		t.Fatal("expected windows platform to be supported")
	}

	currentGOOS = func() string { return "linux" }
	if SupportedForCurrentPlatform() {
		t.Fatal("expected linux to be unsupported")
	}
}
