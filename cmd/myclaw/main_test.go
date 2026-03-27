package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyDataFilesMovesWechatAccount(t *testing.T) {
	t.Parallel()

	sourceRoot := t.TempDir()
	targetRoot := t.TempDir()

	sourcePath := filepath.Join(sourceRoot, "weixin-bridge", "account.json")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte(`{"token":"legacy-token"}`), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	if err := migrateLegacyDataFiles(sourceRoot, targetRoot); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	targetPath := filepath.Join(targetRoot, "weixin-bridge", "account.json")
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != `{"token":"legacy-token"}` {
		t.Fatalf("unexpected target content: %s", string(data))
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("expected source file to be moved, got err=%v", err)
	}
}

func TestMigrateLegacyDataFilesKeepsExistingTarget(t *testing.T) {
	t.Parallel()

	sourceRoot := t.TempDir()
	targetRoot := t.TempDir()

	sourcePath := filepath.Join(sourceRoot, "weixin-bridge", "account.json")
	targetPath := filepath.Join(targetRoot, "weixin-bridge", "account.json")

	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte(`{"token":"legacy-token"}`), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte(`{"token":"current-token"}`), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	if err := migrateLegacyDataFiles(sourceRoot, targetRoot); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != `{"token":"current-token"}` {
		t.Fatalf("target should not be overwritten: %s", string(data))
	}
	if _, err := os.Stat(sourcePath); err != nil {
		t.Fatalf("expected source file to remain in place, got err=%v", err)
	}
}
