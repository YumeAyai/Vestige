package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	localdb "Vestige/pkg/db"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestSyncCloudTrackingEventsImportsRemoteEventsByToken(t *testing.T) {
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
		"Campaign", "Hello", "<p>Hello</p>", mailboxID, "draft")
	if err != nil {
		t.Fatal(err)
	}
	campaignID, _ := res.LastInsertId()
	res, err = conn.Exec(`INSERT INTO campaign_recipients(campaign_id,contact_id,email,name,tracking_id) VALUES(?,?,?,?,?)`,
		campaignID, contactID, "receiver@example.com", "Receiver", "tracking-1")
	if err != nil {
		t.Fatal(err)
	}
	recipientID, _ := res.LastInsertId()
	if _, err := conn.Exec(`INSERT INTO tracking_marks(campaign_recipient_id,token,kind,label,target_url) VALUES(?,?,?,?,?)`,
		recipientID, "mark-1", "image", "Image", ""); err != nil {
		t.Fatal(err)
	}

	events := []cloudTrackingEventInput{
		{ID: 1, Token: "tracking-1", Kind: "open", EventIndex: "variant:1:open", TriggeredAt: "2026-06-09 10:00:00", IP: "203.0.113.1"},
		{ID: 2, Token: "mark-1", Kind: "image", EventIndex: "variant:1:image:qr.png", TriggeredAt: "2026-06-09 10:01:00", IP: "203.0.113.2"},
		{ID: 3, Token: "unknown", Kind: "open", TriggeredAt: "2026-06-09 10:02:00", IP: "203.0.113.3"},
	}
	previousClient := cloudHTTPClient
	cloudHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.URL.Query().Get("source"); got != "creator-token" {
			t.Fatalf("expected source filter creator-token, got %q", got)
		}
		afterID, _ := strconv.ParseInt(req.URL.Query().Get("after_id"), 10, 64)
		filtered := []cloudTrackingEventInput{}
		for _, event := range events {
			if event.ID > afterID {
				filtered = append(filtered, event)
			}
		}
		var body bytes.Buffer
		_ = json.NewEncoder(&body).Encode(cloudTrackingEventsResponse{Events: filtered})
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(&body),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}
	t.Cleanup(func() { cloudHTTPClient = previousClient })

	server := &Server{db: conn}
	result, err := server.syncCloudTrackingEvents(t.Context(), "https://tracking.test", "creator-token")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 3 || result.Imported != 2 || result.Skipped != 1 || result.LastEventID != 3 {
		t.Fatalf("unexpected sync result: %#v", result)
	}

	var openCount, markEventCount int
	if err := conn.QueryRow(`SELECT open_count FROM campaign_recipients WHERE id=?`, recipientID).Scan(&openCount); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT COUNT(*) FROM tracking_mark_events WHERE token='mark-1'`).Scan(&markEventCount); err != nil {
		t.Fatal(err)
	}
	if openCount != 1 || markEventCount != 1 {
		t.Fatalf("expected imported open and image events, got open_count=%d mark_events=%d", openCount, markEventCount)
	}
	var openIndex, markIndex string
	if err := conn.QueryRow(`SELECT event_index FROM open_events WHERE tracking_id='tracking-1'`).Scan(&openIndex); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT event_index FROM tracking_mark_events WHERE token='mark-1'`).Scan(&markIndex); err != nil {
		t.Fatal(err)
	}
	if openIndex != "variant:1:open" || markIndex != "variant:1:image:qr.png" {
		t.Fatalf("unexpected event indexes: open=%q mark=%q", openIndex, markIndex)
	}

	result, err = server.syncCloudTrackingEvents(t.Context(), "https://tracking.test", "creator-token")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 0 || result.LastEventID != 3 {
		t.Fatalf("expected cursor to prevent duplicate fetch, got %#v", result)
	}
}

func TestCloudOpenWithinFiveSecondsAfterDeliveryIsMarkedPrefetch(t *testing.T) {
	conn, err := localdb.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := localdb.Migrate(conn); err != nil {
		t.Fatal(err)
	}

	res, err := conn.Exec(`INSERT INTO contacts(name,email,company) VALUES(?,?,?)`, "Receiver", "receiver@example.com", "Example Co")
	if err != nil {
		t.Fatal(err)
	}
	contactID, _ := res.LastInsertId()
	res, err = conn.Exec(`INSERT INTO mailboxes(name,host,port,username,password,from_email,from_name,use_tls) VALUES(?,?,?,?,?,?,?,?)`,
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
	res, err = conn.Exec(`INSERT INTO campaign_recipients(campaign_id,contact_id,email,name,tracking_id,send_status,sent_at) VALUES(?,?,?,?,?,?,?)`,
		campaignID, contactID, "receiver@example.com", "Receiver", "tracking-early", "sent", "2026-06-09 10:00:00")
	if err != nil {
		t.Fatal(err)
	}
	recipientID, _ := res.LastInsertId()

	server := &Server{db: conn}
	err = server.recordCloudOpenEvent(cloudTrackingEventInput{
		Token:       "tracking-early",
		Kind:        "open",
		TriggeredAt: "2026-06-09 10:00:03",
	}, "{}")
	if err != nil {
		t.Fatal(err)
	}

	var openCount, isPrefetch int
	if err := conn.QueryRow(`SELECT open_count FROM campaign_recipients WHERE id=?`, recipientID).Scan(&openCount); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT is_prefetch FROM open_events WHERE tracking_id='tracking-early'`).Scan(&isPrefetch); err != nil {
		t.Fatal(err)
	}
	if openCount != 1 || isPrefetch != 1 {
		t.Fatalf("expected early open to be counted and marked, got open_count=%d is_prefetch=%d", openCount, isPrefetch)
	}
}

func TestCloudOpenUsesCloudIPRiskAsPrefetchLabel(t *testing.T) {
	conn, err := localdb.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := localdb.Migrate(conn); err != nil {
		t.Fatal(err)
	}

	res, err := conn.Exec(`INSERT INTO contacts(name,email,company) VALUES(?,?,?)`, "Receiver", "receiver@example.com", "Example Co")
	if err != nil {
		t.Fatal(err)
	}
	contactID, _ := res.LastInsertId()
	res, err = conn.Exec(`INSERT INTO mailboxes(name,host,port,username,password,from_email,from_name,use_tls) VALUES(?,?,?,?,?,?,?,?)`,
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
	res, err = conn.Exec(`INSERT INTO campaign_recipients(campaign_id,contact_id,email,name,tracking_id,send_status,sent_at) VALUES(?,?,?,?,?,?,?)`,
		campaignID, contactID, "receiver@example.com", "Receiver", "tracking-risk", "sent", "2026-06-09 10:00:00")
	if err != nil {
		t.Fatal(err)
	}
	recipientID, _ := res.LastInsertId()

	server := &Server{db: conn}
	err = server.recordCloudOpenEvent(cloudTrackingEventInput{
		Token:       "tracking-risk",
		Kind:        "open",
		TriggeredAt: "2026-06-09 10:10:00",
		IP:          "121.32.180.28",
		UserAgent:   "Mozilla/5.0 Chrome/120",
		IPRisk:      "风险高 / 家庭宽带 / 劫持代理IP / 代理IP / 非正常设备IP / 疑似黑ROM设备IP",
	}, "{}")
	if err != nil {
		t.Fatal(err)
	}

	var openCount, isPrefetch int
	var ipRisk string
	if err := conn.QueryRow(`SELECT open_count FROM campaign_recipients WHERE id=?`, recipientID).Scan(&openCount); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT is_prefetch,ip_risk FROM open_events WHERE tracking_id='tracking-risk'`).Scan(&isPrefetch, &ipRisk); err != nil {
		t.Fatal(err)
	}
	if openCount != 1 || isPrefetch != 1 {
		t.Fatalf("expected risky ip portrait to be counted and marked, got open_count=%d is_prefetch=%d", openCount, isPrefetch)
	}
	if !strings.Contains(ipRisk, "代理IP") || !strings.Contains(ipRisk, "风险高") {
		t.Fatalf("expected ip risk summary, got %q", ipRisk)
	}
}
