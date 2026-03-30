package main

import (
	"embed"
	"flag"
	"log"
	"path/filepath"

	"myclaw/internal/ai"
	appsvc "myclaw/internal/app"
	"myclaw/internal/knowledge"
	"myclaw/internal/modelconfig"
	"myclaw/internal/projectstate"
	"myclaw/internal/promptlib"
	"myclaw/internal/reminder"
	"myclaw/internal/sessionstate"
	"myclaw/internal/skilllib"
	"myclaw/internal/weixin"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	dataDirFlag := flag.String("data-dir", envOrDefault("MYCLAW_DATA_DIR", defaultDesktopDataDir()), "directory used to persist data")
	logFileFlag := flag.String("log-file", envOrDefault("MYCLAW_LOG_FILE", ""), "optional log file path")
	httpDevFlag := flag.Bool("http-dev", envOrDefault("MYCLAW_DESKTOP_HTTP_DEV", "0") == "1", "serve the desktop frontend over HTTP for browser-based development")
	httpListenFlag := flag.String("http-listen", envOrDefault("MYCLAW_DESKTOP_HTTP_LISTEN", "127.0.0.1:3415"), "listen address for HTTP desktop development mode")
	flag.Parse()

	if err := configureLogging(*logFileFlag); err != nil {
		log.Fatalf("configure logging: %v", err)
	}

	dataDir, err := prepareDataDir(*dataDirFlag)
	if err != nil {
		log.Fatalf("prepare data dir: %v", err)
	}

	store := knowledge.NewStore(filepath.Join(dataDir, "knowledge", "entries.json"))
	promptStore := promptlib.NewStore(filepath.Join(dataDir, "prompts", "items.json"))
	projectStore := projectstate.NewStore(filepath.Join(dataDir, "projects", "active.json"))
	modelStore := modelconfig.NewStore(filepath.Join(dataDir, "model", "profiles.db"))
	aiService := ai.NewService(modelStore)
	reminderStore := reminder.NewStore(filepath.Join(dataDir, "reminders", "items.json"))
	reminderManager := reminder.NewManager(reminderStore)
	sessionStore := sessionstate.NewStore(filepath.Join(dataDir, "sessions", "items.json"))
	skillLoader := skilllib.NewLoader(skilllib.DefaultDirs(dataDir)...)
	service := appsvc.NewServiceWithRuntime(store, aiService, reminderManager, skillLoader, sessionStore, promptStore)
	weixinBridge := weixin.NewBridge(weixin.NewClient("", ""), service, reminderManager, weixin.BridgeConfig{
		DataDir: dataDir,
	})
	desktopApp := NewDesktopApp(dataDir, store, promptStore, projectStore, modelStore, aiService, service, sessionStore, reminderManager, weixinBridge)

	if *httpDevFlag {
		if err := runHTTPDevServer(*httpListenFlag, desktopApp); err != nil {
			log.Fatalf("run http dev server: %v", err)
		}
		return
	}

	err = wails.Run(&options.App{
		Title:     "myclaw",
		Width:     1440,
		Height:    960,
		MinWidth:  1120,
		MinHeight: 720,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour:         options.NewRGB(244, 238, 228),
		EnableDefaultContextMenu: true,
		OnStartup:                desktopApp.startup,
		OnBeforeClose:            desktopApp.beforeClose,
		OnShutdown:               desktopApp.shutdown,
		Bind: []interface{}{
			desktopApp,
		},
	})
	if err != nil {
		log.Fatalf("run desktop app: %v", err)
	}
}
