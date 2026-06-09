package main

import (
	"log"
	"os"
	"strings"

	"nousmail/pkg/db"
	"nousmail/tracking-server/internal/trackingcloud"
)

func main() {
	dbPath := strings.TrimSpace(os.Getenv("TRACKING_DB_PATH"))
	if dbPath == "" {
		dbPath = "data/tracking.db"
	}
	addr := strings.TrimSpace(os.Getenv("TRACKING_ADDR"))
	if addr == "" {
		addr = ":8081"
	}

	store, err := db.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	if err := trackingcloud.Migrate(store); err != nil {
		log.Fatal(err)
	}

	server := trackingcloud.New(store)
	if err := server.Run(addr); err != nil {
		log.Fatal(err)
	}
}
