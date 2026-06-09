package main

import (
	"log"

	"nousmail/pkg/config"
	"nousmail/pkg/db"
	"nousmail/tracking-server/internal/trackingcloud"
)

func main() {
	cfg, err := config.LoadDefault()
	if err != nil {
		log.Fatal(err)
	}

	store, err := db.Open(cfg.TrackingCloud.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	if err := trackingcloud.Migrate(store); err != nil {
		log.Fatal(err)
	}

	server := trackingcloud.NewWithOptions(store, trackingcloud.Options{AssetDir: cfg.TrackingCloud.AssetDir})
	if err := server.Run(cfg.TrackingCloud.Addr); err != nil {
		log.Fatal(err)
	}
}
