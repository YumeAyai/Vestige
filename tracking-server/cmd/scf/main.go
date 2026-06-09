package main

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"nousmail/pkg/config"
	"nousmail/tracking-server/internal/scftracking"

	"github.com/tencentyun/scf-go-lib/cloudfunction"
	"github.com/tencentyun/scf-go-lib/events"
)

var handler *scftracking.Handler

func init() {
	cfg, err := config.LoadDefault()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := scftracking.NewMongoStore(ctx, scftracking.MongoConfig{
		URI:              requiredValue("MONGODB_URI", cfg.SCF.MongoDB.URI),
		Database:         valueOr(cfg.SCF.MongoDB.Database, "nousmail_tracking"),
		EventsCollection: valueOr(cfg.SCF.MongoDB.EventsCollection, "tracking_events"),
		AssetsCollection: valueOr(cfg.SCF.MongoDB.AssetsCollection, "tracking_assets"),
	})
	if err != nil {
		log.Fatalf("init mongo store: %v", err)
	}
	handler = scftracking.NewHandler(store)
}

func main() {
	cloudfunction.Start(handle)
}

func handle(ctx context.Context, req events.APIGatewayRequest) (events.APIGatewayResponse, error) {
	if handler == nil {
		return events.APIGatewayResponse{StatusCode: 500, Body: `{"error":"handler is not initialized"}`}, nil
	}
	return handler.Handle(ctx, req)
}

func requiredValue(key string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		log.Fatal(errors.New(key + " is required"))
	}
	return value
}

func valueOr(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
