package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"Vestige/tracker/internal/model"
	"Vestige/tracker/internal/store"
)

type memoryStore struct {
	mu     sync.Mutex
	nextID int64
	events []model.Event
	assets map[string]model.Asset
}

func newMemoryStore() *memoryStore {
	return &memoryStore{assets: map[string]model.Asset{}}
}

func (s *memoryStore) RecordEvent(_ context.Context, event model.Event) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	event.ID = s.nextID
	s.events = append(s.events, event)
	return event.ID, nil
}

func (s *memoryStore) ListEvents(_ context.Context, filter model.EventFilter) ([]model.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := []model.Event{}
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

func (s *memoryStore) Stats(ctx context.Context, filter model.EventFilter) (model.StatsResult, error) {
	items, _ := s.ListEvents(ctx, filter)
	byKind := map[string]int{}
	for _, event := range items {
		byKind[event.Kind]++
	}
	summary := []map[string]any{}
	for kind, count := range byKind {
		summary = append(summary, map[string]any{"kind": kind, "count": count, "unique_tokens": count})
	}
	return model.StatsResult{Summary: summary, Trend: []map[string]any{}}, nil
}

func (s *memoryStore) SaveAsset(_ context.Context, asset model.Asset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assets[asset.Name] = asset
	return nil
}

func (s *memoryStore) GetAsset(_ context.Context, name string) (model.Asset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	asset, ok := s.assets[name]
	if !ok {
		return model.Asset{}, store.ErrNotFound
	}
	return asset, nil
}

func TestPixelRecordsOpenEvent(t *testing.T) {
	mem := newMemoryStore()
	h := NewHandler(mem)
	h.Now = func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }

	resp, err := h.Handle(context.Background(), model.SCFEvent{
		Method:      http.MethodGet,
		Path:        "/jianji/p",
		QueryString: "s=tenant&c=campaign&rid=token&i=variant%3A1%3Aopen",
		Headers: map[string]string{
			"user-agent":      "Mail",
			"x-forwarded-for": "203.0.113.1, 10.0.0.1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || !resp.IsBase64Encoded || resp.Headers["Content-Type"] != "image/gif" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if len(mem.events) != 1 {
		t.Fatalf("expected one event, got %#v", mem.events)
	}
	event := mem.events[0]
	if event.Kind != "open" || event.Source != "tenant" || event.Campaign != "campaign" || event.Token != "token" {
		t.Fatalf("unexpected event: %#v", event)
	}
	if event.EventIndex != "variant:1:open" || event.TriggeredAt != "2026-06-10T12:00:00Z" {
		t.Fatalf("unexpected event metadata: %#v", event)
	}
	if event.IP != "203.0.113.1" || event.ForwardedFor != "203.0.113.1, 10.0.0.1" {
		t.Fatalf("unexpected request environment: %#v", event)
	}
}

func TestPixelDebugReturnsJSON(t *testing.T) {
	mem := newMemoryStore()
	resp, err := NewHandler(mem).Handle(context.Background(), model.SCFEvent{
		Method:      http.MethodGet,
		Path:        "/p",
		QueryString: "s=tenant&c=campaign&rid=token&debug=1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || resp.Headers["Content-Type"] != "application/json" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if !strings.Contains(resp.Body, `"ok":true`) || len(mem.events) != 1 {
		t.Fatalf("unexpected debug result: body=%s events=%#v", resp.Body, mem.events)
	}
}

func TestImageDoesNotRecordInvalidRequest(t *testing.T) {
	mem := newMemoryStore()
	h := NewHandler(mem)

	resp, err := h.Handle(context.Background(), model.SCFEvent{
		Method:      http.MethodGet,
		Path:        "/img",
		QueryString: "token=mark-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %#v", resp)
	}
	if len(mem.events) != 0 {
		t.Fatalf("invalid image request should not record events: %#v", mem.events)
	}
}

func TestImageAssetRecordsImageAfterLoad(t *testing.T) {
	mem := newMemoryStore()
	mem.assets["asset.png"] = model.Asset{Name: "asset.png", ContentType: "image/png", Data: []byte{0x89, 'P', 'N', 'G'}}
	h := NewHandler(mem)

	resp, err := h.Handle(context.Background(), model.SCFEvent{
		Method:      http.MethodGet,
		Path:        "/img",
		QueryString: "asset=asset.png&token=mark-1&s=tenant&c=campaign&i=variant%3A1%3Aimage%3Aasset.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || resp.Headers["Content-Type"] != "image/png" {
		t.Fatalf("unexpected image response: %#v", resp)
	}
	data, err := base64.StdEncoding.DecodeString(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("unexpected image body: %#v", data)
	}
	if len(mem.events) != 1 || mem.events[0].Kind != "image" || mem.events[0].Token != "mark-1" {
		t.Fatalf("unexpected image event: %#v", mem.events)
	}
}

func TestUploadAssetReturnsImgURL(t *testing.T) {
	mem := newMemoryStore()
	h := NewHandler(mem)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "qr.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte{0x89, 'P', 'N', 'G'}); err != nil {
		t.Fatal(err)
	}
	_ = writer.WriteField("label", "联系图片")
	_ = writer.WriteField("width", "160")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	resp, err := h.Handle(context.Background(), model.SCFEvent{
		Method: http.MethodPost,
		Path:   "/jianji/api/assets",
		Headers: map[string]string{
			"Content-Type":      writer.FormDataContentType(),
			"Host":              "track.example.com",
			"X-Forwarded-Proto": "https",
		},
		Body: body.String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected upload response: %#v", resp)
	}
	var payload struct {
		Asset    string `json:"asset"`
		ImageURL string `json:"image_url"`
		HTML     string `json:"html"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Asset == "" || !strings.Contains(payload.ImageURL, "https://track.example.com/jianji/img?") {
		t.Fatalf("unexpected upload payload: %#v", payload)
	}
	if !strings.Contains(payload.HTML, `/jianji/img?`) {
		t.Fatalf("html should use mounted /jianji/img: %s", payload.HTML)
	}
}

func TestAssetsGetReturnsEndpointInfo(t *testing.T) {
	resp, err := NewHandler(newMemoryStore()).Handle(context.Background(), model.SCFEvent{
		Method: http.MethodGet,
		Path:   "/api/assets",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected response: %#v", resp)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(resp.Body), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != true || payload["method"] != "POST" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestFilterFromQueryDoesNotAcceptMongoObjects(t *testing.T) {
	query := parseQuery(`s=%7B%22%24ne%22%3Anull%7D&kind=open&since=%7B%22%24gt%22%3Anull%7D&after_id[$gt]=1&after_id=42`)
	filter := filterFromQuery(query, true)
	if filter.Source != `{"$ne":null}` || filter.Kind != "open" {
		t.Fatalf("unexpected scalar filter values: %#v", filter)
	}
	if filter.Since != "" {
		t.Fatalf("invalid since should be ignored: %#v", filter)
	}
	if filter.AfterID != 42 {
		t.Fatalf("after_id should only come from parsed scalar integer: %#v", filter)
	}
}

func TestEventFromRequestSanitizesScalars(t *testing.T) {
	query := parseQuery("s=tenant%00x&rid=token&i=index")
	event := eventFromRequest(query, model.SCFEvent{}, "open", time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC))
	if event.Source != "tenantx" || event.Token != "token" {
		t.Fatalf("unexpected event: %#v", event)
	}
}
