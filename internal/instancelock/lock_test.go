package instancelock

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const helperEnv = "BAIZE_INSTANCE_LOCK_HELPER"

func TestAcquireReleaseAllowsReacquire(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	lockRoot := filepath.Join(t.TempDir(), "lock-root")
	guard, err := acquireAt(lockRoot, dataDir)
	if err != nil {
		t.Fatalf("acquire instance lock: %v", err)
	}
	if guard.Path() != filepath.Join(lockRoot, fileName) {
		t.Fatalf("unexpected lock path: %q", guard.Path())
	}

	if err := guard.Release(); err != nil {
		t.Fatalf("release instance lock: %v", err)
	}

	reacquired, err := acquireAt(lockRoot, dataDir)
	if err != nil {
		t.Fatalf("reacquire instance lock: %v", err)
	}
	t.Cleanup(func() {
		if err := reacquired.Release(); err != nil {
			t.Fatalf("cleanup instance lock: %v", err)
		}
	})
}

func TestAcquireRejectsSecondProcess(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	lockRoot := filepath.Join(t.TempDir(), "lock-root")
	guard, err := acquireAt(lockRoot, dataDir)
	if err != nil {
		t.Fatalf("acquire instance lock: %v", err)
	}
	t.Cleanup(func() {
		if err := guard.Release(); err != nil {
			t.Fatalf("cleanup instance lock: %v", err)
		}
	})

	cmd := exec.Command(os.Args[0], "-test.run=^TestLockHelperProcess$")
	cmd.Env = append(
		os.Environ(),
		helperEnv+"=1",
		"BAIZE_INSTANCE_LOCK_DATA_DIR="+dataDir,
		"BAIZE_INSTANCE_LOCK_ROOT="+lockRoot,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper process failed: %v\n%s", err, output)
	}
	if string(output) != "already-running\n" {
		t.Fatalf("unexpected helper output: %q", output)
	}
}

func TestLockHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		return
	}

	dataDir := os.Getenv("BAIZE_INSTANCE_LOCK_DATA_DIR")
	lockRoot := os.Getenv("BAIZE_INSTANCE_LOCK_ROOT")
	guard, err := acquireAt(lockRoot, dataDir)
	switch {
	case err == nil:
		_ = guard.Release()
		os.Stdout.WriteString("acquired\n")
		os.Exit(0)
	case errors.Is(err, ErrAlreadyRunning):
		os.Stdout.WriteString("already-running\n")
		os.Exit(0)
	default:
		t.Fatalf("acquire instance lock in helper: %v", err)
	}
}
