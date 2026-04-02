package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

type bundleSpec struct {
	output  string
	sources []string
	header  []byte
}

func main() {
	rootDir, err := repoRoot()
	if err != nil {
		exitf("resolve repo root: %v", err)
	}

	frontendDir := filepath.Join(rootDir, "cmd", "baize-desktop", "frontend")
	srcDir := filepath.Join(frontendDir, "src")
	distDir := filepath.Join(frontendDir, "dist")

	bundles := []bundleSpec{
		{
			output: "app.css",
			sources: []string{
				filepath.Join("css", "tokens.css"),
				filepath.Join("css", "base-layout.css"),
				filepath.Join("css", "views", "dashboard.css"),
				filepath.Join("css", "views", "memory.css"),
				filepath.Join("css", "views", "prompts.css"),
				filepath.Join("css", "views", "skills.css"),
				filepath.Join("css", "views", "tools.css"),
				filepath.Join("css", "views", "chat-shell.css"),
				filepath.Join("css", "views", "model-settings.css"),
				filepath.Join("css", "views", "weixin.css"),
				filepath.Join("css", "views", "chat-content.css"),
				filepath.Join("css", "components.css"),
				filepath.Join("css", "responsive.css"),
			},
		},
		{
			output: "app.js",
			header: buildJSBundleHeader(),
			sources: []string{
				filepath.Join("js", "core", "navigation.js"),
				filepath.Join("js", "shared", "state-models.js"),
				filepath.Join("js", "shared", "utils.js"),
				filepath.Join("js", "core", "state.js"),
				filepath.Join("js", "views", "chat.js"),
				filepath.Join("js", "views", "library.js"),
				filepath.Join("js", "ui", "chat-session-ui.js"),
				filepath.Join("js", "core", "events.js"),
				filepath.Join("js", "core", "backend.js"),
				filepath.Join("js", "features", "library-actions.js"),
				filepath.Join("js", "features", "chat-composer.js"),
				filepath.Join("js", "core", "init.js"),
			},
		},
	}

	for _, bundle := range bundles {
		content, err := concatBundle(srcDir, bundle.header, bundle.sources)
		if err != nil {
			exitf("build %s: %v", bundle.output, err)
		}
		targetPath := filepath.Join(distDir, bundle.output)
		if err := writeIfChanged(targetPath, content); err != nil {
			exitf("write %s: %v", targetPath, err)
		}
		fmt.Printf("built %s\n", filepath.ToSlash(targetPath))
	}
}

func repoRoot() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..")), nil
}

func buildJSBundleHeader() []byte {
	debugEnabled := os.Getenv("BAIZE_DESKTOP_DEBUG_DIAGNOSTICS") == "1"
	buildMode := "release"
	if debugEnabled {
		buildMode = "debug"
	}
	return []byte(fmt.Sprintf(
		"window.__BAIZE_DESKTOP_BUILD_MODE__ = %q;\nwindow.__BAIZE_DESKTOP_DEBUG_DIAGNOSTICS__ = %s;\n",
		buildMode,
		strconv.FormatBool(debugEnabled),
	))
}

func concatBundle(baseDir string, header []byte, sources []string) ([]byte, error) {
	var out bytes.Buffer
	trimmedHeader := bytes.TrimRight(normalizeLF(header), "\n")
	if len(trimmedHeader) > 0 {
		out.Write(trimmedHeader)
		out.WriteString("\n\n")
	}
	for index, source := range sources {
		sourcePath := filepath.Join(baseDir, source)
		content, err := os.ReadFile(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", sourcePath, err)
		}
		content = normalizeLF(content)
		content = bytes.TrimRight(content, "\n")

		if index > 0 {
			out.WriteString("\n\n")
		}

		out.WriteString("/* Source: ")
		out.WriteString(filepath.ToSlash(source))
		out.WriteString(" */\n")
		out.Write(content)
		out.WriteString("\n")
	}
	return out.Bytes(), nil
}

func writeIfChanged(targetPath string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}

	existing, err := os.ReadFile(targetPath)
	output := content
	if err == nil {
		output = applyExistingEOL(content, existing)
		if bytes.Equal(existing, output) {
			return nil
		}
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return os.WriteFile(targetPath, output, 0o644)
}

func normalizeLF(content []byte) []byte {
	content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	content = bytes.ReplaceAll(content, []byte("\r"), []byte("\n"))
	return content
}

func applyExistingEOL(content, existing []byte) []byte {
	if bytes.Contains(existing, []byte("\r\n")) {
		return bytes.ReplaceAll(content, []byte("\n"), []byte("\r\n"))
	}
	return content
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
