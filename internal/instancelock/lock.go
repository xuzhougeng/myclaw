package instancelock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const fileName = "instance.lock"

var ErrAlreadyRunning = errors.New("baize instance already running")

type AlreadyRunningError struct {
	Path string
}

func (e *AlreadyRunningError) Error() string {
	if strings.TrimSpace(e.Path) == "" {
		return ErrAlreadyRunning.Error()
	}
	return fmt.Sprintf("%s: %s", ErrAlreadyRunning, e.Path)
}

func (e *AlreadyRunningError) Unwrap() error {
	return ErrAlreadyRunning
}

type Guard struct {
	path string
	file *os.File
}

func Acquire(dataDir string) (*Guard, error) {
	lockRoot, err := defaultLockRoot(dataDir)
	if err != nil {
		return nil, err
	}
	return acquireAt(lockRoot, dataDir)
}

func acquireAt(lockRoot, dataDir string) (*Guard, error) {
	lockRoot = strings.TrimSpace(lockRoot)
	if lockRoot == "" {
		return nil, fmt.Errorf("lock root is empty")
	}

	absLockRoot, err := filepath.Abs(lockRoot)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absLockRoot, 0o755); err != nil {
		return nil, err
	}

	lockPath := filepath.Join(absLockRoot, fileName)
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	if err := lockFile(file); err != nil {
		_ = file.Close()
		if isAlreadyLocked(err) {
			return nil, &AlreadyRunningError{Path: lockPath}
		}
		return nil, err
	}

	if err := writeMetadata(file, dataDir); err != nil {
		_ = unlockFile(file)
		_ = file.Close()
		return nil, err
	}

	return &Guard{
		path: lockPath,
		file: file,
	}, nil
}

func (g *Guard) Path() string {
	if g == nil {
		return ""
	}
	return g.path
}

func (g *Guard) Release() error {
	if g == nil || g.file == nil {
		return nil
	}

	file := g.file
	g.file = nil

	unlockErr := unlockFile(file)
	closeErr := file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}

func defaultLockRoot(dataDir string) (string, error) {
	if configDir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(configDir) != "" {
		return filepath.Join(configDir, "baize"), nil
	}

	dataDir = strings.TrimSpace(dataDir)
	if dataDir != "" {
		return filepath.Abs(dataDir)
	}

	return filepath.Join(os.TempDir(), "baize"), nil
}

func writeMetadata(file *os.File, dataDir string) error {
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return err
	}

	payload := fmt.Sprintf(
		"pid=%d\nstarted_at=%s\ndata_dir=%s\n",
		os.Getpid(),
		time.Now().UTC().Format(time.RFC3339),
		strings.TrimSpace(dataDir),
	)
	if _, err := file.WriteString(payload); err != nil {
		return err
	}
	return file.Sync()
}
