package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"Vestige/pkg/config"
	localdb "Vestige/pkg/db"
)

func TestUploadCampaignAttachmentJSONPreservesContentAndName(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	conn, err := localdb.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := localdb.Migrate(conn); err != nil {
		t.Fatal(err)
	}

	want := []byte("pdf-content\x00\x01\x02")
	body, err := json.Marshal(uploadCampaignAttachmentInput{
		OriginalName:  "滴滴企业版.pdf",
		ContentType:   "application/pdf",
		ContentBase64: base64.StdEncoding.EncodeToString(want),
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/campaign-attachments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	cfg := config.Default()
	cfg.Client.DBPath = dbPath
	NewWithConfig(conn, os.DirFS(t.TempDir()), cfg).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}
	var originalName, storedName string
	var size int
	if err := conn.QueryRow(`SELECT original_name,stored_name,size FROM campaign_attachments`).Scan(&originalName, &storedName, &size); err != nil {
		t.Fatal(err)
	}
	if originalName != "滴滴企业版.pdf" || size != len(want) {
		t.Fatalf("unexpected attachment metadata: name=%q size=%d", originalName, size)
	}
	got, err := os.ReadFile(filepath.Join(dir, "campaign-attachments", storedName))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("stored attachment differs: got %q want %q", got, want)
	}
}
