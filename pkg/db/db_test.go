package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenCreatesParentDirectoryAndEnablesForeignKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "app.db")
	conn, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	var enabled int
	if err := conn.QueryRow(`PRAGMA foreign_keys`).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if enabled != 1 {
		t.Fatalf("foreign keys disabled, got %d", enabled)
	}
}

func TestMigrateCreatesCoreTablesAndIsIdempotent(t *testing.T) {
	conn, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	for range 2 {
		if err := Migrate(conn); err != nil {
			t.Fatal(err)
		}
	}

	for _, table := range []string{"mailboxes", "contacts", "campaigns", "campaign_recipients", "tracking_marks", "tracking_mark_events", "tracking_cloud_sync_state", "app_settings"} {
		t.Run(table, func(t *testing.T) {
			var name string
			err := conn.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
			if err != nil {
				t.Fatalf("expected table %s: %v", table, err)
			}
		})
	}

	rows, err := conn.Query(`PRAGMA table_info(campaign_recipients)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	hasVariantID := false
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		if name == "variant_id" {
			hasVariantID = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !hasVariantID {
		t.Fatal("expected campaign_recipients.variant_id to exist")
	}
}
