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
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	for range 2 {
		if err := Migrate(conn); err != nil {
			t.Fatal(err)
		}
	}

	for _, table := range []string{"mailboxes", "contacts", "campaigns", "campaign_recipients", "campaign_attachments", "tracking_marks", "tracking_mark_events", "tracking_cloud_sync_state", "app_settings"} {
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

	settings := map[string]string{}
	settingRows, err := conn.Query(`SELECT key,value FROM app_settings WHERE key IN ('campaign_send_rate_per_minute','campaign_send_jitter_percent')`)
	if err != nil {
		t.Fatal(err)
	}
	defer settingRows.Close()
	for settingRows.Next() {
		var key, value string
		if err := settingRows.Scan(&key, &value); err != nil {
			t.Fatal(err)
		}
		settings[key] = value
	}
	if err := settingRows.Err(); err != nil {
		t.Fatal(err)
	}
	if settings["campaign_send_rate_per_minute"] != "8" || settings["campaign_send_jitter_percent"] != "35" {
		t.Fatalf("unexpected send rate defaults: %#v", settings)
	}
}
