package main

import (
	"log"
	"net/http"

	"nousmail/pkg/config"
	"nousmail/pkg/db"
	"nousmail/tracking-server/internal/scftracking"
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

	server := scftracking.NewHTTPHandler(scftracking.NewSQLiteStore(store, scftracking.SQLiteConfig{AssetDir: cfg.TrackingCloud.AssetDir}))
	if err := http.ListenAndServe(cfg.TrackingCloud.Addr, server); err != nil {
		log.Fatal(err)
	}
}
