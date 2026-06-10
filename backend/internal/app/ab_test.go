package app

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	localdb "Vestige/pkg/db"
)

func TestCreateVariantRouteStoresVariant(t *testing.T) {
	conn, campaignID := testCampaignDB(t)
	router := New(conn, os.DirFS(t.TempDir()))

	body, _ := json.Marshal(ginH{
		"name":      "B",
		"subject":   "Subject B",
		"body_html": "<p>B</p>",
		"weight":    30,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/"+campaignID+"/variants", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM campaign_variants WHERE campaign_id=? AND name='B'`, campaignID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected variant to be stored, got %d", count)
	}
}

func TestPrepareCampaignVariantsAssignsRecipientsByWeight(t *testing.T) {
	conn, campaignID := testCampaignDB(t)
	if _, err := conn.Exec(`INSERT INTO campaign_variants(campaign_id,name,subject,body_html,weight) VALUES(?,?,?,?,?),(?,?,?,?,?)`,
		campaignID, "A", "A", "<p>A</p>", 1,
		campaignID, "B", "B", "<p>B</p>", 3,
	); err != nil {
		t.Fatal(err)
	}
	var aID, bID int64
	if err := conn.QueryRow(`SELECT id FROM campaign_variants WHERE name='A'`).Scan(&aID); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT id FROM campaign_variants WHERE name='B'`).Scan(&bID); err != nil {
		t.Fatal(err)
	}

	server := &Server{db: conn}
	variants, err := server.prepareCampaignVariants(mustInt64(t, campaignID))
	if err != nil {
		t.Fatal(err)
	}
	if len(variants) != 2 {
		t.Fatalf("expected 2 variants, got %d", len(variants))
	}
	var aCount, bCount int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM campaign_recipients WHERE variant_id=?`, aID).Scan(&aCount); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT COUNT(*) FROM campaign_recipients WHERE variant_id=?`, bID).Scan(&bCount); err != nil {
		t.Fatal(err)
	}
	if aCount != 1 || bCount != 3 {
		t.Fatalf("unexpected weighted assignment: A=%d B=%d", aCount, bCount)
	}
}

type ginH map[string]any

func testCampaignDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	conn, err := localdb.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := localdb.Migrate(conn); err != nil {
		t.Fatal(err)
	}
	res, err := conn.Exec(`INSERT INTO mailboxes(name,host,port,username,password,from_email,from_name,use_tls) VALUES(?,?,?,?,?,?,?,?)`,
		"local", "127.0.0.1", 1, "user", "pass", "sender@example.com", "Sender", 0)
	if err != nil {
		t.Fatal(err)
	}
	mailboxID, _ := res.LastInsertId()
	res, err = conn.Exec(`INSERT INTO campaigns(name,subject,body_html,mailbox_id,status) VALUES(?,?,?,?,?)`,
		"Campaign", "Hello", "<p>Hello</p>", mailboxID, "draft")
	if err != nil {
		t.Fatal(err)
	}
	campaignID, _ := res.LastInsertId()
	for i := 0; i < 4; i++ {
		contactRes, err := conn.Exec(`INSERT INTO contacts(name,email,company) VALUES(?,?,?)`, "Receiver", "receiver"+strconv.Itoa(i)+"@example.com", "Example Co")
		if err != nil {
			t.Fatal(err)
		}
		contactID, _ := contactRes.LastInsertId()
		if _, err := conn.Exec(`INSERT INTO campaign_recipients(campaign_id,contact_id,email,name,tracking_id) VALUES(?,?,?,?,?)`,
			campaignID, contactID, "receiver"+strconv.Itoa(i)+"@example.com", "Receiver", "tracking-"+strconv.Itoa(i)); err != nil {
			t.Fatal(err)
		}
	}
	return conn, strconv.FormatInt(campaignID, 10)
}

func mustInt64(t *testing.T, value string) int64 {
	t.Helper()
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
