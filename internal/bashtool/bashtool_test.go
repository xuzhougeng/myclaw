package bashtool

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
	currentGOOS = func() string { return "linux" }
	runCommand = func(_ context.Context, name string, args ...string) (string, string, int, error) {
		if name != "bash" {
			t.Fatalf("unexpected runner: %q", name)
		}
		if len(args) != 2 || args[0] != "-lc" || args[1] != "uname -a" {
			t.Fatalf("unexpected args: %#v", args)
		}
		return "Linux test-host\n", "", 0, nil
	}
	t.Cleanup(func() {
		currentGOOS = oldGOOS
		runCommand = oldRun
	})

	result, err := Execute(context.Background(), ToolInput{
		Command: "uname",
		Args:    []string{"-a"},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Tool != ToolName || result.Command != "uname" || result.Shell != "bash" || result.ExitCode != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Stdout != "Linux test-host\n" {
		t.Fatalf("unexpected stdout: %#v", result)
	}
}

func TestExecuteRejectsDisallowedArgs(t *testing.T) {
	oldGOOS := currentGOOS
	currentGOOS = func() string { return "linux" }
	t.Cleanup(func() {
		currentGOOS = oldGOOS
	})

	_, err := Execute(context.Background(), ToolInput{
		Command: "uname",
		Args:    []string{"-r"},
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

	currentGOOS = func() string { return "linux" }
	if !SupportedForCurrentPlatform() {
		t.Fatal("expected linux platform to be supported")
	}

	currentGOOS = func() string { return "windows" }
	if SupportedForCurrentPlatform() {
		t.Fatal("expected windows to be unsupported")
	}
}
