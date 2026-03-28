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
			return filepath.Join(localAppData, "myclaw", "data")
		}
	}

	configDir, err := os.UserConfigDir()
	if err == nil && configDir != "" {
		return filepath.Join(configDir, "myclaw", "data")
	}

	return "data"
}

var migratableDataFiles = []string{
	filepath.Join("knowledge", "entries.json"),
	filepath.Join("prompts", "items.json"),
	filepath.Join("projects", "active.json"),
	filepath.Join("model", "config.json"),
	filepath.Join("model", "profiles.db"),
	filepath.Join("model", "secret.key"),
	filepath.Join("reminders", "items.json"),
	filepath.Join("weixin-bridge", "account.json"),
	filepath.Join("weixin-bridge", "sync_buf"),
}

func prepareDataDir(rawPath string) (string, error) {
	dataDir, err := filepath.Abs(rawPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", err
	}
	if err := migrateLegacyData(dataDir); err != nil {
		log.Printf("migrate legacy data: %v", err)
	}
	return dataDir, nil
}

func migrateLegacyData(targetDataDir string) error {
	legacyRoots, err := legacyDataRoots(targetDataDir)
	if err != nil {
		return err
	}
	for _, legacyRoot := range legacyRoots {
		if err := migrateLegacyDataFiles(legacyRoot, targetDataDir); err != nil {
			return err
		}
	}
	return nil
}

func legacyDataRoots(targetDataDir string) ([]string, error) {
	paths := make([]string, 0, 2)

	executable, err := os.Executable()
	if err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(executable), "data"))
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	paths = append(paths, filepath.Join(workingDir, "data"))

	targetAbs, err := filepath.Abs(targetDataDir)
	if err != nil {
		return nil, err
	}

	seen := map[string]struct{}{}
	roots := make([]string, 0, len(paths))
	for _, path := range paths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		if absPath == targetAbs {
			continue
		}
		if _, ok := seen[absPath]; ok {
			continue
		}
		seen[absPath] = struct{}{}
		roots = append(roots, absPath)
	}
	return roots, nil
}

func migrateLegacyDataFiles(sourceRoot, targetRoot string) error {
	for _, relativePath := range migratableDataFiles {
		sourcePath := filepath.Join(sourceRoot, relativePath)
		targetPath := filepath.Join(targetRoot, relativePath)

		if _, err := os.Stat(sourcePath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if _, err := os.Stat(targetPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}

		if err := moveFile(sourcePath, targetPath); err != nil {
			return err
		}
		log.Printf("migrated legacy data: %s -> %s", sourcePath, targetPath)
	}
	return nil
}

func moveFile(sourcePath, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(sourcePath, targetPath); err == nil {
		return nil
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	mode := os.FileMode(0o644)
	if info, err := os.Stat(sourcePath); err == nil {
		mode = info.Mode().Perm()
	}
	targetFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(targetFile, sourceFile)
	closeErr := targetFile.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Remove(sourcePath); err != nil {
		return err
	}
	return nil
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
