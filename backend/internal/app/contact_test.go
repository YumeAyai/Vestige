package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	localdb "Vestige/pkg/db"
)

func TestCreateContactRejectsInvalidEmail(t *testing.T) {
	conn, err := localdb.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := localdb.Migrate(conn); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]string{"name": "Bad", "email": "bad-address"})
	req := httptest.NewRequest(http.MethodPost, "/api/contacts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(conn, os.DirFS(t.TempDir())).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
	}
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM contacts`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected no contact to be inserted, got %d", count)
	}
}
