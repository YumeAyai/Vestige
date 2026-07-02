package store

import (
	"context"
	"errors"

	"Vestige/tracker/internal/model"
)

type Store interface {
	RecordEvent(ctx context.Context, event model.Event) (int64, error)
	UpdateEventIPRisk(ctx context.Context, id int64, ipRisk string) error
	ListEvents(ctx context.Context, filter model.EventFilter) ([]model.Event, error)
	Stats(ctx context.Context, filter model.EventFilter) (model.StatsResult, error)
	SaveAsset(ctx context.Context, asset model.Asset) error
	GetAsset(ctx context.Context, name string) (model.Asset, error)
}

var ErrNotFound = errors.New("not found")
