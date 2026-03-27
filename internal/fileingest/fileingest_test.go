package fileingest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveImageFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.png")
	if err := os.WriteFile(path, []byte("png-bytes"), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	input, ok, err := Resolve(path)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !ok {
		t.Fatalf("expected supported file")
	}
	if input.Kind != KindImage {
		t.Fatalf("unexpected kind: %v", input.Kind)
	}
	if !strings.HasPrefix(input.DataURL, "data:image/png;base64,") {
		t.Fatalf("unexpected data url: %q", input.DataURL)
	}
}

func TestResolveUnsupportedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write text: %v", err)
	}

	_, ok, err := Resolve(path)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ok {
		t.Fatalf("expected unsupported file")
	}
}
