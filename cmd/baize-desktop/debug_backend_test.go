package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportDesktopBackendPanicWritesDebugLog(t *testing.T) {
	previousMode := desktopBuildMode
	desktopBuildMode = "debug"
	defer func() {
		desktopBuildMode = previousMode
	}()

	dataDir := t.TempDir()
	reportDesktopBackendPanic(dataDir, "weixin.handleMessage", "boom", []byte("stack line 1\nstack line 2\n"))

	path := desktopBackendDebugLogPath(dataDir)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read backend debug log: %v", err)
	}
	text := string(content)
	for _, want := range []string{
		"buildMode=debug",
		"scope=weixin.handleMessage",
		"panic=boom",
		"stack line 1",
		"stack line 2",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in backend debug log, got %q", want, text)
		}
	}
}

func TestReportDesktopBackendStartupWritesMarker(t *testing.T) {
	previousMode := desktopBuildMode
	desktopBuildMode = "debug"
	defer func() {
		desktopBuildMode = previousMode
	}()

	dataDir := t.TempDir()
	reportDesktopBackendStartup(dataDir)

	path := desktopBackendDebugLogPath(dataDir)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read backend debug log: %v", err)
	}
	text := string(content)
	for _, want := range []string{
		"buildMode=debug",
		"scope=startup",
		"event=backend-debug-enabled",
		"dataDir=" + dataDir,
		"logPath=" + path,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in backend debug log, got %q", want, text)
		}
	}
}

func TestDesktopBackendDebugLogPath(t *testing.T) {
	t.Parallel()

	path := desktopBackendDebugLogPath(filepath.Join("tmp", "baize-data"))
	if !strings.Contains(filepath.ToSlash(path), "tmp/baize-data/debug/desktop-backend-debug.log") {
		t.Fatalf("unexpected backend debug log path: %q", path)
	}
}
