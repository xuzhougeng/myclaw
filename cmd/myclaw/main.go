package main

import (
	"context"
	"flag"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"myclaw/internal/ai"
	"myclaw/internal/app"
	"myclaw/internal/knowledge"
	"myclaw/internal/modelconfig"
	"myclaw/internal/promptlib"
	"myclaw/internal/reminder"
	"myclaw/internal/sessionstate"
	"myclaw/internal/skilllib"
	"myclaw/internal/terminal"
	"myclaw/internal/weixin"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	dataDirFlag := flag.String("data-dir", envOrDefault("MYCLAW_DATA_DIR", "data"), "directory used to persist data")
	logFileFlag := flag.String("log-file", envOrDefault("MYCLAW_LOG_FILE", ""), "optional log file path")
	weixinEnabled := flag.Bool("weixin", envOrDefault("MYCLAW_WEIXIN_ENABLED", "0") == "1", "enable WeChat bridge")
	weixinLogin := flag.Bool("weixin-login", false, "run WeChat QR login and exit")
	weixinLogout := flag.Bool("weixin-logout", false, "remove saved WeChat credentials and exit")
	terminalEnabled := flag.Bool("terminal", false, "run interactive terminal")
	flag.Parse()

	if err := configureLogging(*logFileFlag); err != nil {
		log.Fatalf("configure logging: %v", err)
	}

	dataDir, err := filepath.Abs(*dataDirFlag)
	if err != nil {
		log.Fatalf("resolve data dir: %v", err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}
	if err := migrateLegacyData(dataDir); err != nil {
		log.Printf("migrate legacy data: %v", err)
	}

	store := knowledge.NewStore(filepath.Join(dataDir, "knowledge", "entries.json"))
	promptStore := promptlib.NewStore(filepath.Join(dataDir, "prompts", "items.json"))
	if err := promptlib.SeedDefaultPrompts(context.Background(), promptStore, promptlib.DefaultPromptSeedMarker(dataDir)); err != nil {
		log.Fatalf("seed default prompts: %v", err)
	}
	modelStore := modelconfig.NewStore(filepath.Join(dataDir, "model", "profiles.db"))
	aiService := ai.NewService(modelStore)
	reminderStore := reminder.NewStore(filepath.Join(dataDir, "reminders", "items.json"))
	reminderManager := reminder.NewManager(reminderStore)
	sessionStore := sessionstate.NewStore(filepath.Join(dataDir, "sessions", "items.json"))
	skillLoader := skilllib.NewLoader(skilllib.DefaultDirs(dataDir)...)
	service := app.NewServiceWithRuntime(store, aiService, reminderManager, skillLoader, sessionStore, promptStore)
	service.SetFileSearchEverythingPath(envOrDefault("MYCLAW_WEIXIN_EVERYTHING_PATH", ""))
	bridge := weixin.NewBridge(weixin.NewClient("", ""), service, reminderManager, weixin.BridgeConfig{
		DataDir:        dataDir,
		EverythingPath: envOrDefault("MYCLAW_WEIXIN_EVERYTHING_PATH", ""),
	})
	repl := terminal.NewREPL(service, os.Stdin, os.Stdout)

	if *weixinLogout {
		if err := bridge.Logout(); err != nil {
			log.Fatalf("weixin logout: %v", err)
		}
		log.Printf("weixin credentials removed")
		return
	}

	if *weixinLogin {
		if err := bridge.Login(); err != nil {
			log.Fatalf("weixin login: %v", err)
		}
		log.Printf("weixin login complete")
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		if err := reminderManager.Run(ctx); err != nil && err != context.Canceled {
			log.Printf("reminder scheduler stopped: %v", err)
		}
	}()

	runTerminal := *terminalEnabled || !*weixinEnabled

	errCh := make(chan error, 1)
	if *weixinEnabled {
		if !bridge.LoadAccount() {
			log.Printf("no saved weixin account found, starting login flow")
			if err := bridge.Login(); err != nil {
				log.Fatalf("weixin login: %v", err)
			}
		}
		log.Printf("myclaw started: data_dir=%s interface=weixin", dataDir)
		go func() {
			errCh <- bridge.Run(ctx)
		}()
	}

	if runTerminal {
		reminderManager.RegisterNotifier(reminder.Target{Interface: "terminal", UserID: "terminal"}, terminal.NewNotifier(os.Stdout))
		log.Printf("myclaw started: data_dir=%s interface=terminal", dataDir)
		if err := repl.Run(ctx); err != nil && err != context.Canceled {
			log.Fatalf("terminal stopped: %v", err)
		}
		cancel()
	}

	if *weixinEnabled {
		if err := <-errCh; err != nil && err != context.Canceled {
			log.Fatalf("weixin bridge stopped: %v", err)
		}
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
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
