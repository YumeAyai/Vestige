package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"Vestige/pkg/config"
	localdb "Vestige/pkg/db"
)

func TestImportContactsFromJSONPath(t *testing.T) {
	conn, err := localdb.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := localdb.Migrate(conn); err != nil {
		t.Fatal(err)
	}

	filePath := t.TempDir() + "/contacts.csv"
	if err := os.WriteFile(filePath, []byte("Acme Inc,hello@example.com,unused,unused,021-12345678\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(importContactsInput{Path: filePath})
	req := httptest.NewRequest(http.MethodPost, "/api/contacts/import", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(conn, os.DirFS(t.TempDir())).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM contacts WHERE email='hello@example.com'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected imported contact, got %d", count)
	}
}

func TestContactsPageAllDoesNotClampAtTwoHundred(t *testing.T) {
	conn, err := localdb.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := localdb.Migrate(conn); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 205; i++ {
		if _, err := conn.Exec(`INSERT INTO contacts(name,email,company) VALUES(?,?,?)`, "Contact", "person"+strconv.Itoa(i)+"@example.com", "Acme"); err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/contacts/page?limit=all", nil)
	rec := httptest.NewRecorder()
	New(conn, os.DirFS(t.TempDir())).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
		Limit int              `json:"limit"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Total != 205 || payload.Limit != 205 || len(payload.Items) != 205 {
		t.Fatalf("expected all 205 contacts, got total=%d limit=%d items=%d", payload.Total, payload.Limit, len(payload.Items))
	}
}

func TestUploadCampaignAttachmentFromJSONPath(t *testing.T) {
	dataDir := t.TempDir()
	cfg := config.Default()
	cfg.Client.DBPath = filepath.Join(dataDir, "test.db")
	conn, err := localdb.Open(cfg.Client.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := localdb.Migrate(conn); err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(t.TempDir(), "quote.pdf")
	if err := os.WriteFile(filePath, []byte("attachment body"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(campaignAttachmentPathInput{Path: filePath})
	req := httptest.NewRequest(http.MethodPost, "/api/campaign-attachments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	NewWithConfig(conn, os.DirFS(t.TempDir()), cfg).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}
	var storedName string
	if err := conn.QueryRow(`SELECT stored_name FROM campaign_attachments WHERE original_name='quote.pdf'`).Scan(&storedName); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "campaign-attachments", storedName)); err != nil {
		t.Fatalf("expected attachment copy in local data dir: %v", err)
	}
}
