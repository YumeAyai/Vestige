package scftracking

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"nousmail/tracking-server/internal/trackingcloud"

	tcb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/tcb/v20180608"
)

type fakeRunCommandsClient struct {
	requests []*tcb.RunCommandsRequest
	results  []string
}

func (c *fakeRunCommandsClient) RunCommandsWithContext(_ context.Context, req *tcb.RunCommandsRequest) (*tcb.RunCommandsResponse, error) {
	c.requests = append(c.requests, req)
	data := []*string{}
	if len(c.results) > 0 {
		result := c.results[0]
		c.results = c.results[1:]
		data = append(data, &result)
	}
	return &tcb.RunCommandsResponse{Response: &tcb.RunCommandsResponseParams{Data: data}}, nil
}

func TestTCBStoreRecordEventUsesRunCommands(t *testing.T) {
	client := &fakeRunCommandsClient{results: []string{`{"value":{"seq":42}}`, `{"ok":1}`}}
	store := NewTCBStoreWithClient(client, TCBConfig{EnvID: "env-1", EventsCollection: "tracking_events", AssetsCollection: "tracking_assets"})
	id, err := store.RecordEvent(context.Background(), trackingcloud.Event{
		Source:      "creator",
		Campaign:    "1",
		Token:       "rid",
		Kind:        "open",
		EventIndex:  "variant:2:open",
		TriggeredAt: "2026-06-10T12:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != 42 {
		t.Fatalf("unexpected id: %d", id)
	}
	if len(client.requests) != 2 {
		t.Fatalf("expected counter and insert commands, got %d", len(client.requests))
	}
	insert := client.requests[1].MgoCommands[0]
	if *insert.CommandType != "INSERT" || *insert.TableName != "tracking_events" || *client.requests[1].EnvId != "env-1" {
		t.Fatalf("unexpected insert request: %#v", client.requests[1])
	}
	var command struct {
		Documents []tcbEventDoc `json:"documents"`
	}
	if err := json.Unmarshal([]byte(*insert.Command), &command); err != nil {
		t.Fatal(err)
	}
	if len(command.Documents) != 1 || command.Documents[0].EventIndex != "variant:2:open" {
		t.Fatalf("unexpected insert command: %#v", command)
	}
}

func TestTCBStoreListEventsParsesCursorBatch(t *testing.T) {
	client := &fakeRunCommandsClient{results: []string{`{"cursor":{"firstBatch":[{"id":7,"source":"creator","campaign":"1","kind":"open","token":"rid","event_index":"variant:1:open","triggered_at":"2026-06-10T12:00:00Z"}]}}`}}
	store := NewTCBStoreWithClient(client, TCBConfig{EnvID: "env-1", EventsCollection: "tracking_events"})
	items, err := store.ListEvents(context.Background(), EventFilter{Source: "creator", Campaign: "1", AfterID: 6})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != 7 || items[0].EventIndex != "variant:1:open" {
		t.Fatalf("unexpected items: %#v", items)
	}
	var command map[string]any
	if err := json.Unmarshal([]byte(*client.requests[0].MgoCommands[0].Command), &command); err != nil {
		t.Fatal(err)
	}
	filter := command["filter"].(map[string]any)
	if filter["source"] != "creator" || filter["campaign"] != "1" {
		t.Fatalf("unexpected filter: %#v", filter)
	}
}

func TestTCBStoreAssetRoundTripCommands(t *testing.T) {
	created := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	client := &fakeRunCommandsClient{results: []string{
		`{"ok":1}`,
		`{"cursor":{"firstBatch":[{"name":"qr.png","label":"QR","content_type":"image/png","data_base64":"AQID","width":176,"created_at":"2026-06-10T12:00:00Z"}]}}`,
	}}
	store := NewTCBStoreWithClient(client, TCBConfig{EnvID: "env-1", AssetsCollection: "tracking_assets"})
	if err := store.SaveAsset(context.Background(), Asset{Name: "qr.png", Label: "QR", ContentType: "image/png", Data: []byte{1, 2, 3}, Width: 176, CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
	asset, err := store.GetAsset(context.Background(), "qr.png")
	if err != nil {
		t.Fatal(err)
	}
	if asset.Name != "qr.png" || string(asset.Data) != string([]byte{1, 2, 3}) {
		t.Fatalf("unexpected asset: %#v", asset)
	}
}
