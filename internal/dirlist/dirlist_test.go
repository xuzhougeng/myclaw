package dirlist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExecuteListsDirectoryEntries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "beta"), 0o755); err != nil {
		t.Fatalf("mkdir beta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write alpha: %v", err)
	}

	result, err := Execute(ToolInput{Path: root})
	if err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}
	if result.Tool != ToolName {
		t.Fatalf("unexpected tool name: %#v", result)
	}
	if result.Count != 2 {
		t.Fatalf("expected 2 items, got %#v", result)
	}
	if result.Items[0].Name != "beta" || !result.Items[0].IsDir {
		t.Fatalf("expected directory first, got %#v", result.Items)
	}
	if result.Items[1].Name != "alpha.txt" || result.Items[1].IsDir {
		t.Fatalf("expected file second, got %#v", result.Items)
	}
	if result.Items[1].SizeBytes != 5 {
		t.Fatalf("unexpected file size: %#v", result.Items[1])
	}
}

func TestExecuteFiltersHiddenAndDirectoriesOnly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".hidden-dir"), 0o755); err != nil {
		t.Fatalf("mkdir hidden: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "visible-dir"), 0o755); err != nil {
		t.Fatalf("mkdir visible dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hidden-file"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write hidden: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "visible.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write visible: %v", err)
	}

	result, err := Execute(ToolInput{Path: root, DirectoriesOnly: true})
	if err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}
	if result.Count != 1 {
		t.Fatalf("expected only one visible directory, got %#v", result.Items)
	}
	if result.Items[0].Name != "visible-dir" {
		t.Fatalf("unexpected items: %#v", result.Items)
	}
}

func TestExecuteHonorsLimitAndDefaultPath(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"b.txt", "a.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir() failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	result, err := Execute(ToolInput{Limit: 2})
	if err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		t.Fatalf("Stat(root) failed: %v", err)
	}
	resultInfo, err := os.Stat(result.Path)
	if err != nil {
		t.Fatalf("Stat(result.Path) failed: %v", err)
	}
	if !filepath.IsAbs(result.Path) {
		t.Fatalf("expected absolute result path, got %q", result.Path)
	}
	if !os.SameFile(rootInfo, resultInfo) {
		t.Fatalf("expected result path to reference %q, got %q", root, result.Path)
	}
	if !result.Truncated || result.Count != 2 {
		t.Fatalf("expected truncated result of 2 items, got %#v", result)
	}
	if result.Items[0].Name != "a.txt" || result.Items[1].Name != "b.txt" {
		t.Fatalf("unexpected sort order: %#v", result.Items)
	}
}

func TestAllowedForInterface(t *testing.T) {
	t.Parallel()

	if AllowedForInterface("weixin") {
		t.Fatal("expected weixin to be blocked")
	}
	if !AllowedForInterface("desktop") {
		t.Fatal("expected desktop to be allowed")
	}
}
