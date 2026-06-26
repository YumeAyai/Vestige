package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	localdb "Vestige/pkg/db"
)

func TestSendCampaignReturnsAcceptedWithoutWaitingForSMTP(t *testing.T) {
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
	res, err = conn.Exec(`INSERT INTO contacts(name,email,company) VALUES(?,?,?)`, "Receiver", "receiver@example.com", "Example Co")
	if err != nil {
		t.Fatal(err)
	}
	contactID, _ := res.LastInsertId()
	res, err = conn.Exec(`INSERT INTO campaigns(name,subject,body_html,mailbox_id,status) VALUES(?,?,?,?,?)`,
		"Campaign", "Hello {{.Name}}", "<p>Hello</p>", mailboxID, "draft")
	if err != nil {
		t.Fatal(err)
	}
	campaignID, _ := res.LastInsertId()
	if _, err := conn.Exec(`INSERT INTO campaign_recipients(campaign_id,contact_id,email,name,tracking_id) VALUES(?,?,?,?,?)`,
		campaignID, contactID, "receiver@example.com", "Receiver", "tracking-1"); err != nil {
		t.Fatal(err)
	}

	router := New(conn, os.DirFS(t.TempDir()))
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/campaigns/%d/send", campaignID), nil)
	req.Header.Set("X-Base-URL", "http://example.test")
	rec := httptest.NewRecorder()

	start := time.Now()
	router.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d: %s", rec.Code, rec.Body.String())
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("send endpoint waited too long before responding: %s", elapsed)
	}
}

func TestCampaignStatsSeparatesPendingAndWaiting(t *testing.T) {
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
	for _, status := range []string{"pending", "waiting"} {
		res, err := conn.Exec(`INSERT INTO contacts(name,email,company) VALUES(?,?,?)`, status, status+"@example.com", "Example Co")
		if err != nil {
			t.Fatal(err)
		}
		contactID, _ := res.LastInsertId()
		if _, err := conn.Exec(`INSERT INTO campaign_recipients(campaign_id,contact_id,email,name,tracking_id,send_status) VALUES(?,?,?,?,?,?)`,
			campaignID, contactID, status+"@example.com", status, "tracking-"+status, status); err != nil {
			t.Fatal(err)
		}
	}

	router := New(conn, os.DirFS(t.TempDir()))
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/campaigns/%d/stats", campaignID), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Summary struct {
			Pending int `json:"pending"`
			Waiting int `json:"waiting"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Summary.Pending != 1 || payload.Summary.Waiting != 1 {
		t.Fatalf("expected pending=1 waiting=1, got pending=%d waiting=%d", payload.Summary.Pending, payload.Summary.Waiting)
	}
}
