package main

import (
	"log"
	"nousmail/internal/app"
	"nousmail/internal/db"
	"nousmail/internal/webui"
)

func main() {
	store, err := db.Open("data/app.db")
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	if err := db.Migrate(store); err != nil {
		log.Fatal(err)
	}

	server := app.New(store, webui.FS)
	if err := server.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
