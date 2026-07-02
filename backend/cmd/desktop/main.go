package main

import (
	"context"
	"database/sql"
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
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const appName = "Jianji"

type DesktopApp struct {
	ctx   context.Context
	cfg   config.Config
	store *sql.DB
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
	desktop := &DesktopApp{cfg: cfg, store: store}

	if err := wails.Run(&options.App{
		Title:     "见迹",
		Width:     1440,
		Height:    900,
		MinWidth:  1280,
		MinHeight: 820,
		AssetServer: &assetserver.Options{
			Handler: handler,
		},
		OnStartup: desktop.startup,
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

func (a *DesktopApp) startup(ctx context.Context) {
	a.ctx = ctx
	runtime.WindowMaximise(ctx)
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

func (a *DesktopApp) SelectContactImportFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择联系人名单",
		Filters: []runtime.FileFilter{
			{DisplayName: "联系人名单 (*.xlsx;*.csv)", Pattern: "*.xlsx;*.csv"},
		},
	})
}

func (a *DesktopApp) ImportContactsFromFileDialog() (app.ContactImportResult, error) {
	path, err := a.SelectContactImportFile()
	if err != nil || path == "" {
		return app.ContactImportResult{}, err
	}
	return app.ImportContactsFromPath(a.store, path)
}
