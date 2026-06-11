package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"Vestige/backend/internal/app"
	"Vestige/backend/internal/webui"
	"Vestige/pkg/config"
	"Vestige/pkg/db"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

const appName = "Jianji"

type DesktopApp struct {
	cfg config.Config
}

func main() {
	cfg, err := desktopConfig()
	if err != nil {
		log.Fatal(err)
	}

	store, err := db.Open(cfg.Client.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	if err := db.Migrate(store); err != nil {
		log.Fatal(err)
	}

	handler := app.NewWithConfig(store, webui.FS, cfg)
	desktop := &DesktopApp{cfg: cfg}

	if err := wails.Run(&options.App{
		Title:     "见迹",
		Width:     1280,
		Height:    820,
		MinWidth:  1024,
		MinHeight: 680,
		AssetServer: &assetserver.Options{
			Handler: handler,
		},
		Bind: []interface{}{
			desktop,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "jianji.local.desktop",
		},
		BackgroundColour: options.NewRGB(248, 249, 251),
	}); err != nil {
		log.Fatal(err)
	}
}

func desktopConfig() (config.Config, error) {
	cfg, err := config.LoadDefault()
	if err != nil {
		return cfg, err
	}

	if shouldUseDesktopDBPath() {
		dir, err := desktopDataDir()
		if err != nil {
			return cfg, err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return cfg, err
		}
		cfg.Client.DBPath = filepath.Join(dir, "app.db")
	}

	return cfg, nil
}

func shouldUseDesktopDBPath() bool {
	return os.Getenv("CLIENT_DB_PATH") == "" && os.Getenv("VESTIGE_CONFIG") == ""
}

func desktopDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, appName), nil
}

func (a *DesktopApp) Health() map[string]string {
	return map[string]string{
		"mode":    "desktop",
		"db_path": a.cfg.Client.DBPath,
	}
}
