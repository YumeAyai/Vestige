package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	localdb "Vestige/pkg/db"
	"Vestige/pkg/models"
)

func TestUpdateContactChangesAllFields(t *testing.T) {
	conn, err := localdb.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := localdb.Migrate(conn); err != nil {
		t.Fatal(err)
	}
	res, err := conn.Exec(`INSERT INTO contacts(name,email,company,department,phone,tags,notes) VALUES(?,?,?,?,?,?,?)`,
		"Old", "old@example.com", "Old Co", "Sales", "10086", "old", "old notes")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()

	input := models.Contact{
		Name:       "New",
		Email:      "new@example.com; second@example.com",
		Company:    "",
		Department: "Ops",
		Phone:      "021-12345678",
		Tags:       "vip",
		Notes:      "new notes",
	}
	body, _ := json.Marshal(input)
	req := httptest.NewRequest(http.MethodPatch, "/api/contacts/"+strconvInt(id), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(conn, os.DirFS(t.TempDir())).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	var got models.Contact
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != id || got.Name != "New" || got.Email != "new@example.com;second@example.com" || got.Company != "" || got.Department != "Ops" || got.Phone != "021-12345678" || got.Tags != "vip" || got.Notes != "new notes" {
		t.Fatalf("unexpected updated contact: %#v", got)
	}
}

func strconvInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
