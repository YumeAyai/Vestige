package store

import (
	"bytes"
	"context"
	"encoding/base64"
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
	client := &fakeRunCommandsClient{results: []string{`{"ok":1}`, `{"ok":1}`}}
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
	if len(client.requests) != 2 {
		t.Fatalf("expected insert and counter commands, got %d", len(client.requests))
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
	counter := client.requests[1].MgoCommands[0]
	if *counter.TableName != "tracking_counters" {
		t.Fatalf("unexpected counter table: %s", *counter.TableName)
	}
	if !strings.HasPrefix(*counter.Command, `{"update":"tracking_counters"`) {
		t.Fatalf("counter update must be the first command field, got %s", *counter.Command)
	}
}

func TestTCBStoreRecordEventPreservesNanosecondID(t *testing.T) {
	client := &fakeRunCommandsClient{results: []string{`{"ok":1}`, `{"ok":1}`}}
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

func TestTCBStoreUpdateEventIPRisk(t *testing.T) {
	client := &fakeRunCommandsClient{results: []string{`{"ok":1}`}}
	store := NewTCBStoreWithClient(client, TCBConfig{EnvID: "env-1", EventsCollection: "tracking_events"})

	err := store.UpdateEventIPRisk(context.Background(), 123, "风险高 / 家庭宽带 / 代理IP")
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected one update command, got %d", len(client.requests))
	}
	command := client.requests[0].MgoCommands[0]
	if *command.TableName != "tracking_events" {
		t.Fatalf("unexpected table: %s", *command.TableName)
	}
	var body updateCommandBody
	if err := json.Unmarshal([]byte(*command.Command), &body); err != nil {
		t.Fatal(err)
	}
	if body.Update != "tracking_events" || len(body.Updates) != 1 {
		t.Fatalf("unexpected update body: %#v", body)
	}
	update := body.Updates[0]
	if intValue(update.Query["id"]) != 123 {
		t.Fatalf("unexpected update query: %#v", update.Query)
	}
	set := update.Update["$set"].(map[string]any)
	if set["ip_risk"] != "风险高 / 家庭宽带 / 代理IP" {
		t.Fatalf("unexpected set body: %#v", set)
	}
}

func TestTCBStoreRecordEventUpdatesCounters(t *testing.T) {
	client := &fakeRunCommandsClient{results: []string{`{"ok":1}`, `{"ok":1}`}}
	store := NewTCBStoreWithClient(client, TCBConfig{EnvID: "env-1", EventsCollection: "tracking_events", CountersCollection: "custom_counters"})

	_, err := store.RecordEvent(context.Background(), model.Event{
		Source:      "creator",
		Campaign:    "camp-1",
		Token:       "token-1",
		Kind:        "click",
		TriggeredAt: "2026-06-10T12:34:56Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("expected insert and counter commands, got %d", len(client.requests))
	}
	command := client.requests[1].MgoCommands[0]
	if *command.TableName != "custom_counters" {
		t.Fatalf("unexpected counter table: %s", *command.TableName)
	}
	var body updateCommandBody
	if err := json.Unmarshal([]byte(*command.Command), &body); err != nil {
		t.Fatal(err)
	}
	if body.Update != "custom_counters" || len(body.Updates) != 2 {
		t.Fatalf("unexpected counter update body: %#v", body)
	}
	hourUpdate := body.Updates[0]
	if !hourUpdate.Upsert || hourUpdate.Query["_id"] == "" {
		t.Fatalf("hour counter must be upserted with stable id: %#v", hourUpdate)
	}
	setOnInsert := hourUpdate.Update["$setOnInsert"].(map[string]any)
	if setOnInsert["type"] != "hour" || setOnInsert["hour"] != "2026-06-10 12:00" || setOnInsert["kind"] != "click" {
		t.Fatalf("unexpected hour counter dimensions: %#v", setOnInsert)
	}
	inc := hourUpdate.Update["$inc"].(map[string]any)
	if intValue(inc["count"]) != 1 {
		t.Fatalf("hour counter should increment count by 1: %#v", inc)
	}
	tokenUpdate := body.Updates[1]
	tokenInsert := tokenUpdate.Update["$setOnInsert"].(map[string]any)
	if tokenInsert["type"] != "token_hour" || tokenInsert["token"] != "token-1" || tokenInsert["hour"] != "2026-06-10 12:00" {
		t.Fatalf("unexpected token counter dimensions: %#v", tokenInsert)
	}
}

func TestTCBStoreStatsUsesCounters(t *testing.T) {
	client := &fakeRunCommandsClient{results: []string{
		`{"cursor":{"firstBatch":[{"type":"hour","source":"creator","campaign":"camp-1","kind":"click","hour":"2026-06-10 12:00","count":{"$numberInt":"3"}},{"type":"hour","source":"creator","campaign":"camp-1","kind":"open","hour":"2026-06-10 12:00","count":2}]}}`,
		`{"cursor":{"firstBatch":[{"type":"token_hour","source":"creator","campaign":"camp-1","kind":"click","token":"a","hour":"2026-06-10 12:00"},{"type":"token_hour","source":"creator","campaign":"camp-1","kind":"click","token":"b","hour":"2026-06-10 12:00"},{"type":"token_hour","source":"creator","campaign":"camp-1","kind":"open","token":"a","hour":"2026-06-10 12:00"}]}}`,
	}}
	store := NewTCBStoreWithClient(client, TCBConfig{EnvID: "env-1", EventsCollection: "tracking_events", CountersCollection: "tracking_counters"})

	stats, err := store.Stats(context.Background(), model.EventFilter{
		Source:   "creator",
		Campaign: "camp-1",
		Since:    "2026-06-10T00:05:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("stats should query counters only, got %d requests", len(client.requests))
	}
	for _, req := range client.requests {
		if table := *req.MgoCommands[0].TableName; table != "tracking_counters" {
			t.Fatalf("stats should not scan raw events, got table %s", table)
		}
	}
	if len(stats.Summary) != 2 {
		t.Fatalf("unexpected summary: %#v", stats.Summary)
	}
	if stats.Summary[0]["kind"] != "click" || stats.Summary[0]["count"] != 3 || stats.Summary[0]["unique_tokens"] != 2 {
		t.Fatalf("unexpected click summary: %#v", stats.Summary[0])
	}
	if stats.Summary[1]["kind"] != "open" || stats.Summary[1]["count"] != 2 || stats.Summary[1]["unique_tokens"] != 1 {
		t.Fatalf("unexpected open summary: %#v", stats.Summary[1])
	}
	if len(stats.Trend) != 2 {
		t.Fatalf("unexpected trend: %#v", stats.Trend)
	}

	var firstFind findCommandBody
	if err := json.Unmarshal([]byte(*client.requests[0].MgoCommands[0].Command), &firstFind); err != nil {
		t.Fatal(err)
	}
	if firstFind.Find != "tracking_counters" || firstFind.Filter["type"] != "hour" {
		t.Fatalf("unexpected hour find: %#v", firstFind)
	}
	hourFilter := firstFind.Filter["hour"].(map[string]any)
	if hourFilter["$gte"] != "2026-06-10 00:00" {
		t.Fatalf("unexpected since hour filter: %#v", hourFilter)
	}
}

func TestTCBStoreSaveAssetUsesNameAsID(t *testing.T) {
	client := &fakeRunCommandsClient{results: []string{`{"ok":1}`}}
	store := NewTCBStoreWithClient(client, TCBConfig{EnvID: "env-1", AssetsCollection: "tracking_assets"})

	err := store.SaveAsset(context.Background(), model.Asset{
		Name:        "asset-1.png",
		Label:       "Asset",
		ContentType: "image/png",
		Data:        []byte("png"),
		Width:       132,
	})
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Documents []map[string]any `json:"documents"`
	}
	if err := json.Unmarshal([]byte(*client.requests[0].MgoCommands[0].Command), &body); err != nil {
		t.Fatal(err)
	}
	if body.Documents[0]["_id"] != "asset-1.png" || body.Documents[0]["name"] != "asset-1.png" {
		t.Fatalf("asset should use stable name id, got %#v", body.Documents[0])
	}
}

func TestTCBStoreGetAssetDecodesLegacyBase64Image(t *testing.T) {
	jpeg := []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00, 0x08}
	rawEncoded := base64.StdEncoding.EncodeToString(jpeg)
	encoded := rawEncoded[:6] + `\n` + rawEncoded[6:]
	client := &fakeRunCommandsClient{results: []string{`{"cursor":{"firstBatch":[{"_id":{"$oid":"6a2963fa41d7c754d223e479"},"name":"legacy.jpg","label":"Legacy","content_type":"application/octet-stream","data_base64":"` + encoded + `","width":{"$numberInt":"176"},"created_at":"2026-06-10T13:17:46Z"}]}}`}}
	store := NewTCBStoreWithClient(client, TCBConfig{EnvID: "env-1", AssetsCollection: "tracking_assets"})

	asset, err := store.GetAsset(context.Background(), "legacy.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if asset.Name != "legacy.jpg" || asset.ContentType != "image/jpeg" {
		t.Fatalf("unexpected asset metadata: %#v", asset)
	}
	if asset.Width != 176 {
		t.Fatalf("unexpected asset width: %d", asset.Width)
	}
	if !bytes.Equal(asset.Data, jpeg) {
		t.Fatalf("unexpected asset data: %x", asset.Data)
	}
}

func TestTCBStoreReportsCommandWriteErrors(t *testing.T) {
	client := &fakeRunCommandsClient{results: []string{`{"ok":0,"errmsg":"insert failed"}`}}
	store := NewTCBStoreWithClient(client, TCBConfig{EnvID: "env-1", AssetsCollection: "tracking_assets"})

	err := store.SaveAsset(context.Background(), model.Asset{Name: "asset-1.png", Data: []byte("png")})
	if err == nil || !strings.Contains(err.Error(), "tcb command failed") {
		t.Fatalf("expected command failure, got %v", err)
	}
}
