package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/tencentyun/scf-go-lib/cloudfunction"

	"Vestige/pkg/config"
	"Vestige/tracker/internal/handler"
	"Vestige/tracker/internal/model"
	"Vestige/tracker/internal/store"
)

var (
	h    *handler.Handler
	hErr error
	hMu  sync.Mutex
)

func init() {
	if err := initHandler(context.Background()); err != nil {
		log.Printf("tracker init deferred: %v", err)
	}
}

func initHandler(ctx context.Context) error {
	hMu.Lock()
	defer hMu.Unlock()
	if h != nil {
		return nil
	}

	cfg, err := config.LoadDefault()
	if err != nil {
		hErr = err
		return err
	}

	tcbConfig := store.TCBConfig{
		EnvID:            envIDFromConfig(cfg),
		Region:           valueOr(cfg.SCF.TCB.Region, "ap-shanghai"),
		EventsCollection: valueOr(cfg.SCF.TCB.EventsCollection, "tracking_events"),
		AssetsCollection: valueOr(cfg.SCF.TCB.AssetsCollection, "tracking_assets"),
	}
	tcbStore, err := store.NewTCBStore(ctx, tcbConfig)
	if err != nil {
		hErr = err
		return err
	}

	if err := tcbStore.EnsureIndexes(ctx); err != nil {
		hErr = err
		return err
	}

	h = handler.NewHandler(tcbStore)
	hErr = nil
	log.Println("tracker scf initialized")
	return nil
}

func main() {
	cloudfunction.Start(track)
}

func track(ctx context.Context, event model.SCFEvent) (model.SCFResponse, error) {
	if err := initHandler(ctx); err != nil {
		return initErrorResponse(err), nil
	}
	return h.Handle(ctx, event)
}

func initErrorResponse(err error) model.SCFResponse {
	message := "handler not initialized"
	if err != nil {
		message = err.Error()
	} else if hErr != nil {
		message = hErr.Error()
	}
	body, _ := json.Marshal(map[string]string{
		"error":  "handler not initialized",
		"reason": message,
	})
	return model.SCFResponse{
		StatusCode: 500,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(body),
	}
}

func valueOr(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func envIDFromConfig(cfg config.Config) string {
	return firstNonEmpty(
		cfg.SCF.TCB.EnvID,
		os.Getenv("TCB_ENV_ID"),
		os.Getenv("TCB_ENV"),
		os.Getenv("SCF_NAMESPACE"),
	)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
