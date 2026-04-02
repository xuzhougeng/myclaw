package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

func appendDesktopBackendDebugEntry(dataDir string, lines []string) {
	path := desktopBackendDebugLogPath(dataDir)
	if strings.TrimSpace(path) == "" {
		return
	}

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

	payload := strings.Join(lines, "\n") + "\n---\n"
	if _, err := file.WriteString(payload); err != nil {
		log.Printf("[desktop-debug] write debug log: %v", err)
	}
}

func reportDesktopBackendStartup(dataDir string) {
	if !desktopDebugBuildEnabled() {
		return
	}

	path := desktopBackendDebugLogPath(dataDir)
	appendDesktopBackendDebugEntry(dataDir, []string{
		"timestamp=" + time.Now().UTC().Format(time.RFC3339Nano),
		"buildMode=" + strings.TrimSpace(desktopBuildMode),
		"scope=startup",
		"pid=" + strconv.Itoa(os.Getpid()),
		"dataDir=" + strings.TrimSpace(dataDir),
		"logPath=" + path,
		"event=backend-debug-enabled",
	})
	log.Printf("[desktop-debug] backend debug enabled: %s", path)
}

func reportDesktopBackendEvent(dataDir, scope string, fields map[string]string) {
	if !desktopDebugBuildEnabled() {
		return
	}

	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "event"
	}

	lines := []string{
		"timestamp=" + time.Now().UTC().Format(time.RFC3339Nano),
		"buildMode=" + strings.TrimSpace(desktopBuildMode),
		"scope=" + scope,
	}

	if len(fields) > 0 {
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			field := strings.TrimSpace(key)
			if field == "" {
				continue
			}
			lines = append(lines, field+"="+strings.TrimSpace(fields[key]))
		}
	}

	appendDesktopBackendDebugEntry(dataDir, lines)
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
	appendDesktopBackendDebugEntry(dataDir, strings.Split(strings.TrimSuffix(entry.String(), "---\n"), "\n"))
}
