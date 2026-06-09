package scftracking

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"nousmail/tracking-server/internal/trackingcloud"

	"github.com/tencentyun/scf-go-lib/events"
)

type memoryStore struct {
	mu     sync.Mutex
	nextID int64
	events []trackingcloud.Event
	assets map[string]Asset
}

func newMemoryStore() *memoryStore {
	return &memoryStore{assets: map[string]Asset{}}
}

func (s *memoryStore) RecordEvent(_ context.Context, event trackingcloud.Event) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	event.ID = s.nextID
	s.events = append(s.events, event)
	return event.ID, nil
}

func (s *memoryStore) ListEvents(_ context.Context, filter EventFilter) ([]trackingcloud.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := []trackingcloud.Event{}
	for _, event := range s.events {
		if filter.Source != "" && event.Source != filter.Source {
			continue
		}
		if filter.Campaign != "" && event.Campaign != filter.Campaign {
			continue
		}
		if filter.Kind != "" && event.Kind != filter.Kind {
			continue
		}
		if filter.AfterID > 0 && event.ID <= filter.AfterID {
			continue
		}
		items = append(items, event)
		if filter.Limit > 0 && int64(len(items)) >= filter.Limit {
			break
		}
	}
	return items, nil
}

func (s *memoryStore) Stats(_ context.Context, filter EventFilter) (StatsResult, error) {
	items, _ := s.ListEvents(context.Background(), filter)
	byKind := map[string]int{}
	for _, event := range items {
		byKind[event.Kind]++
	}
	summary := []map[string]any{}
	for kind, count := range byKind {
		summary = append(summary, map[string]any{"kind": kind, "count": count, "unique_tokens": count})
	}
	return StatsResult{Summary: summary, Trend: []map[string]any{}}, nil
}

func (s *memoryStore) SaveAsset(_ context.Context, asset Asset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assets[asset.Name] = asset
	return nil
}

func (s *memoryStore) GetAsset(_ context.Context, name string) (Asset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	asset, ok := s.assets[name]
	if !ok {
		return Asset{}, ErrNotFound
	}
	return asset, nil
}

func TestPixelRecordsEventAndReturnsGIF(t *testing.T) {
	store := newMemoryStore()
	h := NewHandler(store)
	h.Now = func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }
	resp, err := h.Handle(context.Background(), events.APIGatewayRequest{
		Method: "GET",
		Path:   "/p",
		QueryString: events.APIGatewayQueryString{
			"s":   {"tenant"},
			"c":   {"1"},
			"rid": {"abc"},
		},
		Headers: map[string]string{"User-Agent": "Mail", "X-Forwarded-For": "203.0.113.10"},
		Context: events.APIGatewayRequestContext{SourceIP: "198.51.100.8"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || !resp.IsBase64Encoded || resp.Headers["Content-Type"] != "image/gif" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if len(store.events) != 1 {
		t.Fatalf("expected one event, got %#v", store.events)
	}
	event := store.events[0]
	if event.Kind != "open" || event.Source != "tenant" || event.Campaign != "1" || event.Token != "abc" {
		t.Fatalf("unexpected event: %#v", event)
	}
	if event.IP != "198.51.100.8" || event.ForwardedFor != "203.0.113.10" {
		t.Fatalf("unexpected ip fields: %#v", event)
	}
}

func TestRedirectRecordsClick(t *testing.T) {
	store := newMemoryStore()
	h := NewHandler(store)
	resp, err := h.Handle(context.Background(), events.APIGatewayRequest{
		Method: "GET",
		Path:   "/r",
		QueryString: events.APIGatewayQueryString{
			"c":    {"2"},
			"l":    {"hero"},
			"rid":  {"abc"},
			"dest": {"https://example.com/landing"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusFound || resp.Headers["Location"] != "https://example.com/landing" {
		t.Fatalf("unexpected redirect: %#v", resp)
	}
	if len(store.events) != 1 || store.events[0].Kind != "click" || store.events[0].Link != "hero" {
		t.Fatalf("unexpected events: %#v", store.events)
	}
}

func TestEventsEndpointFiltersWithCursor(t *testing.T) {
	store := newMemoryStore()
	_, _ = store.RecordEvent(context.Background(), trackingcloud.Event{Source: "tenant", Campaign: "1", Token: "a", Kind: "open"})
	_, _ = store.RecordEvent(context.Background(), trackingcloud.Event{Source: "tenant", Campaign: "1", Token: "b", Kind: "click"})
	h := NewHandler(store)
	resp, err := h.Handle(context.Background(), events.APIGatewayRequest{
		Method: "GET",
		Path:   "/api/events",
		QueryString: events.APIGatewayQueryString{
			"source":   {"tenant"},
			"campaign": {"1"},
			"after_id": {"1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Body, `"kind":"click"`) || strings.Contains(resp.Body, `"kind":"open"`) {
		t.Fatalf("unexpected events response: %#v", resp)
	}
}

func TestAssetUploadAndLoad(t *testing.T) {
	store := newMemoryStore()
	h := NewHandler(store)
	contentType, body, err := MultipartBody("file", "qr.png", "image/png", []byte{0x89, 'P', 'N', 'G'}, map[string]string{"label": "联系图片", "width": "160"})
	if err != nil {
		t.Fatal(err)
	}
	upload, err := h.Handle(context.Background(), events.APIGatewayRequest{
		Method:  "POST",
		Path:    "/api/assets",
		Headers: map[string]string{"Content-Type": contentType, "Host": "track.example.com", "X-Forwarded-Proto": "https"},
		Body:    body,
	})
	if err != nil {
		t.Fatal(err)
	}
	if upload.StatusCode != http.StatusOK || !strings.Contains(upload.Body, `"asset"`) {
		t.Fatalf("unexpected upload response: %#v", upload)
	}
	var payload struct {
		Asset string `json:"asset"`
	}
	if err := jsonUnmarshal([]byte(upload.Body), &payload); err != nil {
		t.Fatal(err)
	}
	image, err := h.Handle(context.Background(), events.APIGatewayRequest{
		Method:      "GET",
		Path:        "/qrcode.png",
		QueryString: events.APIGatewayQueryString{"asset": []string{payload.Asset}, "token": []string{"abc"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if image.StatusCode != http.StatusOK || image.Headers["Content-Type"] != "image/png" {
		t.Fatalf("unexpected image response: %#v", image)
	}
	data, err := base64.StdEncoding.DecodeString(image.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("unexpected image data: %#v", data)
	}
}

func jsonUnmarshal(data []byte, value any) error {
	return json.Unmarshal(data, value)
}
