package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"Vestige/tracker/internal/model"

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

func TestTCBStoreListEventsUsesNativeFindCommand(t *testing.T) {
	client := &fakeRunCommandsClient{results: []string{`{"cursor":{"firstBatch":[{"id":7,"source":"creator","campaign":"1","kind":"open","token":"rid","triggered_at":"2026-06-10T12:00:00Z"}]}}`}}
	store := NewTCBStoreWithClient(client, TCBConfig{EnvID: "env-1", EventsCollection: "tracking_events"})

	items, err := store.ListEvents(context.Background(), model.EventFilter{Source: "creator", Campaign: "1", AfterID: 6})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != 7 {
		t.Fatalf("unexpected items: %#v", items)
	}

	command := client.requests[0].MgoCommands[0]
	if *command.CommandType != "COMMAND" || *command.TableName != "tracking_events" {
		t.Fatalf("unexpected request: %#v", client.requests[0])
	}
	if !strings.HasPrefix(*command.Command, `{"find":"tracking_events"`) {
		t.Fatalf("find must be the first command field, got %s", *command.Command)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(*command.Command), &body); err != nil {
		t.Fatal(err)
	}
	if body["find"] != "tracking_events" {
		t.Fatalf("unexpected command body: %#v", body)
	}
	filter := body["filter"].(map[string]any)
	if filter["source"] != "creator" || filter["campaign"] != "1" {
		t.Fatalf("unexpected filter: %#v", filter)
	}
}

func TestTCBStoreListEventsParsesStringEncodedDocs(t *testing.T) {
	client := &fakeRunCommandsClient{results: []string{`{"cursor":{"firstBatch":["{\"id\":7,\"source\":\"creator\",\"campaign\":\"1\",\"kind\":\"open\",\"token\":\"rid\",\"triggered_at\":\"2026-06-10T12:00:00Z\"}"]}}`}}
	store := NewTCBStoreWithClient(client, TCBConfig{EnvID: "env-1", EventsCollection: "tracking_events"})

	items, err := store.ListEvents(context.Background(), model.EventFilter{Source: "creator"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != 7 || items[0].Source != "creator" {
		t.Fatalf("unexpected items: %#v", items)
	}
}

func TestTCBStoreListEventsParsesExtendedJSONID(t *testing.T) {
	client := &fakeRunCommandsClient{results: []string{`{"cursor":{"firstBatch":[{"id":{"$numberLong":"1800000000000000000"},"source":"creator","campaign":"1","kind":"open","token":"rid","triggered_at":"2026-06-10T12:00:00Z"}]}}`}}
	store := NewTCBStoreWithClient(client, TCBConfig{EnvID: "env-1", EventsCollection: "tracking_events"})

	items, err := store.ListEvents(context.Background(), model.EventFilter{Source: "creator"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != 1800000000000000000 {
		t.Fatalf("unexpected items: %#v", items)
	}
}

func TestTCBStoreRecordEventUsesNativeCommands(t *testing.T) {
	client := &fakeRunCommandsClient{results: []string{`{"ok":1}`}}
	store := NewTCBStoreWithClient(client, TCBConfig{EnvID: "env-1", EventsCollection: "tracking_events"})

	id, err := store.RecordEvent(context.Background(), model.Event{
		Source:      "creator",
		Campaign:    "1",
		Token:       "rid",
		Kind:        "open",
		TriggeredAt: "2026-06-10T12:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC).UnixNano() {
		t.Fatalf("unexpected id: %d", id)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected only insert command, got %d", len(client.requests))
	}
	for _, req := range client.requests {
		if got := *req.MgoCommands[0].CommandType; got != "COMMAND" {
			t.Fatalf("unexpected command type %q in %#v", got, req)
		}
	}
	insert := *client.requests[0].MgoCommands[0].Command
	if !strings.HasPrefix(insert, `{"insert":"tracking_events"`) {
		t.Fatalf("insert must be the first command field, got %s", insert)
	}
}

func TestTCBStoreRecordEventPreservesNanosecondID(t *testing.T) {
	client := &fakeRunCommandsClient{results: []string{`{"ok":1}`}}
	store := NewTCBStoreWithClient(client, TCBConfig{EnvID: "env-1", EventsCollection: "tracking_events"})
	eventTime := time.Date(2026, 6, 10, 12, 0, 0, 123456789, time.UTC)

	id, err := store.RecordEvent(context.Background(), model.Event{Campaign: "1", Kind: "open", TriggeredAt: eventTime.Format(time.RFC3339Nano)})
	if err != nil {
		t.Fatal(err)
	}
	if id != eventTime.UnixNano() {
		t.Fatalf("unexpected id: got %d want %d", id, eventTime.UnixNano())
	}
}
