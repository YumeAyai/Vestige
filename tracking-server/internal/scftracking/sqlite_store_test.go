package scftracking

import (
	"context"
	"testing"

	"nousmail/pkg/db"
	"nousmail/tracking-server/internal/trackingcloud"
)

func TestSQLiteStoreRecordsAndListsEvents(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := trackingcloud.Migrate(conn); err != nil {
		t.Fatal(err)
	}
	store := NewSQLiteStore(conn, SQLiteConfig{AssetDir: t.TempDir()})
	id, err := store.RecordEvent(context.Background(), trackingcloud.Event{
		Source:      "creator",
		Campaign:    "1",
		Token:       "abc",
		Kind:        "open",
		EventIndex:  "variant:1:open",
		TriggeredAt: "2026-06-10T12:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != 1 {
		t.Fatalf("unexpected id: %d", id)
	}
	items, err := store.ListEvents(context.Background(), EventFilter{Source: "creator", Campaign: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].EventIndex != "variant:1:open" {
		t.Fatalf("unexpected items: %#v", items)
	}
	stats, err := store.Stats(context.Background(), EventFilter{Campaign: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Summary) != 1 || stats.Summary[0]["kind"] != "open" {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}
