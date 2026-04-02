package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
)

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func defaultDesktopDataDir() string {
	if runtime.GOOS == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "baize", "data")
		}
	}

	configDir, err := os.UserConfigDir()
	if err == nil && configDir != "" {
		return filepath.Join(configDir, "baize", "data")
	}

	return "data"
}

func prepareDataDir(rawPath string) (string, error) {
	dataDir, err := filepath.Abs(rawPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", err
	}
	return dataDir, nil
}

func configureLogging(path string) error {
	path = filepath.Clean(path)
	if path == "." || path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	log.SetOutput(io.MultiWriter(os.Stderr, file))
	return nil
}
