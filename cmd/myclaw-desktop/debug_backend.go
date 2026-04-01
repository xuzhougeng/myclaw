package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	desktopBuildMode = "release"

	desktopBackendDebugMu sync.Mutex
)

func desktopDebugBuildEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(desktopBuildMode), "debug")
}

func desktopBackendDebugLogPath(dataDir string) string {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return ""
	}
	return filepath.Join(dataDir, "debug", "desktop-backend-debug.log")
}

func reportDesktopBackendPanic(dataDir, scope string, recovered any, stack []byte) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "background"
	}

	trimmedStack := bytes.TrimSpace(stack)
	if len(trimmedStack) == 0 {
		log.Printf("[desktop-debug] panic in %s: %v", scope, recovered)
	} else {
		log.Printf("[desktop-debug] panic in %s: %v\n%s", scope, recovered, trimmedStack)
	}

	if !desktopDebugBuildEnabled() {
		return
	}

	path := desktopBackendDebugLogPath(dataDir)
	if strings.TrimSpace(path) == "" {
		return
	}

	var entry strings.Builder
	entry.WriteString("timestamp=")
	entry.WriteString(time.Now().UTC().Format(time.RFC3339Nano))
	entry.WriteByte('\n')
	entry.WriteString("buildMode=")
	entry.WriteString(strings.TrimSpace(desktopBuildMode))
	entry.WriteByte('\n')
	entry.WriteString("scope=")
	entry.WriteString(scope)
	entry.WriteByte('\n')
	entry.WriteString("panic=")
	entry.WriteString(fmt.Sprint(recovered))
	entry.WriteByte('\n')
	if len(trimmedStack) > 0 {
		entry.WriteString("stack=\n")
		entry.Write(trimmedStack)
		entry.WriteByte('\n')
	}
	entry.WriteString("---\n")

	desktopBackendDebugMu.Lock()
	defer desktopBackendDebugMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("[desktop-debug] mkdir debug log dir: %v", err)
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("[desktop-debug] open debug log: %v", err)
		return
	}
	defer file.Close()

	if _, err := file.WriteString(entry.String()); err != nil {
		log.Printf("[desktop-debug] write debug log: %v", err)
	}
}
