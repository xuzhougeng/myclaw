package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"baize/internal/ai"
	"baize/internal/app"
	"baize/internal/instancelock"
	"baize/internal/knowledge"
	"baize/internal/modelconfig"
	"baize/internal/projectstate"
	"baize/internal/promptlib"
	"baize/internal/reminder"
	"baize/internal/sessionstate"
	"baize/internal/skilllib"
	"baize/internal/terminal"
	"baize/internal/weixin"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	dataDirFlag := flag.String("data-dir", envOrDefault("BAIZE_DATA_DIR", "data"), "directory used to persist data")
	logFileFlag := flag.String("log-file", envOrDefault("BAIZE_LOG_FILE", ""), "optional log file path")
	weixinEnabled := flag.Bool("weixin", envOrDefault("BAIZE_WEIXIN_ENABLED", "0") == "1", "enable WeChat bridge")
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

	instanceGuard, err := instancelock.Acquire(dataDir)
	if err != nil {
		if errors.Is(err, instancelock.ErrAlreadyRunning) {
			log.Fatalf("baize is already running; only one instance is allowed at a time")
		}
		log.Fatalf("acquire instance lock: %v", err)
	}
	defer func() {
		if err := instanceGuard.Release(); err != nil {
			log.Printf("release instance lock: %v", err)
		}
	}()

	appDBPath := filepath.Join(dataDir, "app.db")
	store := knowledge.NewStore(appDBPath)
	promptStore := promptlib.NewStore(appDBPath)
	projectStore := projectstate.NewStore(appDBPath)
	if err := promptlib.SeedDefaultPrompts(context.Background(), promptStore, promptlib.DefaultPromptSeedMarker(dataDir)); err != nil {
		log.Fatalf("seed default prompts: %v", err)
	}
	modelStore := modelconfig.NewStore(
		appDBPath,
		filepath.Join(dataDir, "model", "secret.key"),
	)
	aiService := ai.NewService(modelStore)
	reminderStore := reminder.NewStore(appDBPath)
	reminderManager := reminder.NewManager(reminderStore)
	sessionStore := sessionstate.NewStore(appDBPath)
	skillLoader := skilllib.NewLoader(skilllib.DefaultDirs(dataDir)...)
	service := app.NewServiceWithRuntime(store, aiService, reminderManager, skillLoader, sessionStore, promptStore)
	service.SetProjectStore(projectStore)
	service.SetFileSearchEverythingPath(envOrDefault("BAIZE_WEIXIN_EVERYTHING_PATH", ""))
	bridge := weixin.NewBridge(weixin.NewClient("", ""), service, reminderManager, weixin.BridgeConfig{
		DataDir:        dataDir,
		EverythingPath: envOrDefault("BAIZE_WEIXIN_EVERYTHING_PATH", ""),
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
		log.Printf("baize started: data_dir=%s interface=weixin", dataDir)
		go func() {
			errCh <- bridge.Run(ctx)
		}()
	}

	if runTerminal {
		reminderManager.RegisterNotifier(reminder.Target{Interface: "terminal", UserID: "terminal"}, terminal.NewNotifier(os.Stdout))
		log.Printf("baize started: data_dir=%s interface=terminal", dataDir)
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
