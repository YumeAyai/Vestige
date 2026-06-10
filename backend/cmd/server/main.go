package main

import (
	"Vestige/backend/internal/app"
	"Vestige/backend/internal/webui"
	"Vestige/pkg/config"
	"Vestige/pkg/db"
	"log"
)

func main() {
	cfg, err := config.LoadDefault()
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

	server := app.NewWithConfig(store, webui.FS, cfg)
	if err := server.Run(cfg.Client.Addr); err != nil {
		log.Fatal(err)
	}
}
