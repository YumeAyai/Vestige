package main

import (
	"Vestige/backend/internal/app"
	"Vestige/backend/internal/webui"
	"Vestige/pkg/config"
	"Vestige/pkg/db"
	"log"
	"net"
	"strings"
	"time"

	"github.com/pkg/browser"
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
	openBrowserAfterStart(cfg.Client.Addr)
	if err := server.Run(cfg.Client.Addr); err != nil {
		log.Fatal(err)
	}
}

func openBrowserAfterStart(addr string) {
	go func() {
		time.Sleep(500 * time.Millisecond)
		url := browserURL(addr)
		log.Printf("opening %s", url)
		if err := browser.OpenURL(url); err != nil {
			log.Printf("open browser failed: %v", err)
		}
	}()
}

func browserURL(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "http://localhost:8080"
	}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, ":") {
			return "http://localhost" + addr
		}
		return "http://" + addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port)
}
