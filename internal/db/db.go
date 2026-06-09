package db

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	conn, err := sql.Open("sqlite3", path+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	return conn, nil
}

func Migrate(conn *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS mailboxes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  host TEXT NOT NULL,
  port INTEGER NOT NULL,
  username TEXT NOT NULL,
  password TEXT NOT NULL,
  from_email TEXT NOT NULL,
  from_name TEXT NOT NULL,
  use_tls INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS contacts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  email TEXT NOT NULL UNIQUE,
  company TEXT NOT NULL DEFAULT '',
  department TEXT NOT NULL DEFAULT '',
  phone TEXT NOT NULL DEFAULT '',
  tags TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS templates (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  subject TEXT NOT NULL,
  body_html TEXT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS campaigns (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  subject TEXT NOT NULL,
  body_html TEXT NOT NULL,
  mailbox_id INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft',
  tracking_enabled INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  sent_at DATETIME,
  FOREIGN KEY(mailbox_id) REFERENCES mailboxes(id)
);

CREATE TABLE IF NOT EXISTS campaign_recipients (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  campaign_id INTEGER NOT NULL,
  contact_id INTEGER NOT NULL,
  email TEXT NOT NULL,
  name TEXT NOT NULL,
  tracking_id TEXT NOT NULL UNIQUE,
  send_status TEXT NOT NULL DEFAULT 'pending',
  failure_reason TEXT NOT NULL DEFAULT '',
  sent_at DATETIME,
  first_opened_at DATETIME,
  last_opened_at DATETIME,
  open_count INTEGER NOT NULL DEFAULT 0,
  FOREIGN KEY(campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
  FOREIGN KEY(contact_id) REFERENCES contacts(id)
);

CREATE TABLE IF NOT EXISTS open_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  campaign_recipient_id INTEGER NOT NULL,
  tracking_id TEXT NOT NULL,
  ip TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  is_prefetch INTEGER NOT NULL DEFAULT 0,
  opened_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY(campaign_recipient_id) REFERENCES campaign_recipients(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS tracking_marks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  campaign_recipient_id INTEGER NOT NULL,
  token TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  target_url TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY(campaign_recipient_id) REFERENCES campaign_recipients(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS tracking_mark_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  mark_id INTEGER,
  token TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'local',
  ip TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  referer TEXT NOT NULL DEFAULT '',
  accept_language TEXT NOT NULL DEFAULT '',
  is_prefetch INTEGER NOT NULL DEFAULT 0,
  raw_payload TEXT NOT NULL DEFAULT '',
  triggered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY(mark_id) REFERENCES tracking_marks(id) ON DELETE SET NULL
);`
	if _, err := conn.Exec(schema); err != nil {
		return err
	}
	return addColumns(conn, "tracking_mark_events", map[string]string{
		"referer":         "TEXT NOT NULL DEFAULT ''",
		"accept_language": "TEXT NOT NULL DEFAULT ''",
		"is_prefetch":     "INTEGER NOT NULL DEFAULT 0",
	})
}

func addColumns(conn *sql.DB, table string, columns map[string]string) error {
	rows, err := conn.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	existing := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for name, definition := range columns {
		if existing[name] {
			continue
		}
		if _, err := conn.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + name + ` ` + definition); err != nil {
			return err
		}
	}
	return nil
}
