package main

import (
	"log"
	"nousmail/local-backend/internal/app"
	"nousmail/local-backend/internal/webui"
	"nousmail/pkg/config"
	"nousmail/pkg/db"
)

func main() {
	cfg, err := config.LoadDefault()
	if err != nil {
		log.Fatal(err)
	}
	store, err := db.Open(cfg.LocalBackend.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	if err := db.Migrate(store); err != nil {
		log.Fatal(err)
	}

	server := app.NewWithConfig(store, webui.FS, cfg)
	if err := server.Run(cfg.LocalBackend.Addr); err != nil {
		log.Fatal(err)
	}
}
