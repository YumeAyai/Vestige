package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	localdb "Vestige/pkg/db"
)

func TestGlobalStatsIncludesCampaignRows(t *testing.T) {
	conn, err := localdb.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := localdb.Migrate(conn); err != nil {
		t.Fatal(err)
	}

	mailbox, _ := conn.Exec(`INSERT INTO mailboxes(name,host,port,username,password,from_email,from_name,use_tls) VALUES('m','localhost',25,'u','p','from@example.com','From',0)`)
	mailboxID, _ := mailbox.LastInsertId()
	campaign, _ := conn.Exec(`INSERT INTO campaigns(name,subject,body_html,mailbox_id,status) VALUES('任务 A','s','b',?,'sent')`, mailboxID)
	campaignID, _ := campaign.LastInsertId()
	contact, _ := conn.Exec(`INSERT INTO contacts(name,email) VALUES('A','a@example.com')`)
	contactID, _ := contact.LastInsertId()
	recipient, err := conn.Exec(`INSERT INTO campaign_recipients(campaign_id,contact_id,email,name,tracking_id,send_status,sent_at,open_count) VALUES(?,?,?,?,?,'sent',CURRENT_TIMESTAMP,1)`, campaignID, contactID, "a@example.com", "A", "track-a")
	if err != nil {
		t.Fatal(err)
	}
	recipientID, _ := recipient.LastInsertId()
	mark, _ := conn.Exec(`INSERT INTO tracking_marks(campaign_recipient_id,token,kind,label,target_url) VALUES(?,?,'click','link','https://example.com')`, recipientID, "click-a")
	markID, _ := mark.LastInsertId()
	if _, err := conn.Exec(`INSERT INTO tracking_mark_events(mark_id,token,kind,event_index) VALUES(?,?,'click','1')`, markID, "click-a"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	rec := httptest.NewRecorder()
	New(conn, os.DirFS(t.TempDir())).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Campaigns []struct {
			ID        int64  `json:"id"`
			Name      string `json:"name"`
			CreatedAt string `json:"created_at"`
			Sent      int    `json:"sent"`
			Opened    int    `json:"opened"`
			Clicked   int    `json:"clicked"`
		} `json:"campaigns"`
		RecentEvents []struct {
			Type         string `json:"type"`
			CampaignName string `json:"campaign_name"`
			Email        string `json:"email"`
		} `json:"recent_events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Campaigns) != 1 {
		t.Fatalf("expected one campaign row, got %#v", payload.Campaigns)
	}
	got := payload.Campaigns[0]
	if got.ID != campaignID || got.Name != "任务 A" || got.CreatedAt == "" || got.Sent != 1 || got.Opened != 1 || got.Clicked != 1 {
		t.Fatalf("unexpected campaign stats: %#v", got)
	}
	if len(payload.RecentEvents) != 1 || payload.RecentEvents[0].Type != "click" || payload.RecentEvents[0].CampaignName != "任务 A" || payload.RecentEvents[0].Email != "a@example.com" {
		t.Fatalf("unexpected recent events: %#v", payload.RecentEvents)
	}
}
