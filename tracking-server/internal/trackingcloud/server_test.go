package trackingcloud

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

func newTestRouter(t *testing.T) (*gin.Engine, *sql.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	conn, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(conn); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return New(conn), conn
}

func TestPixelRecordsOpenEventAndReturnsGIF(t *testing.T) {
	router, conn := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/p?s=tenant&c=campaign&rid=token&i=variant%3A1%3Aopen", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/gif" {
		t.Fatalf("content type = %q", got)
	}

	var kind, source, campaign, token, eventIndex, forwardedFor string
	if err := conn.QueryRow(`SELECT kind,source,campaign,token,event_index,forwarded_for FROM tracking_events`).Scan(&kind, &source, &campaign, &token, &eventIndex, &forwardedFor); err != nil {
		t.Fatal(err)
	}
	if kind != "open" || source != "tenant" || campaign != "campaign" || token != "token" {
		t.Fatalf("unexpected event: kind=%s source=%s campaign=%s token=%s", kind, source, campaign, token)
	}
	if forwardedFor != "203.0.113.9, 10.0.0.1" {
		t.Fatalf("unexpected forwarded_for: %q", forwardedFor)
	}
	if eventIndex != "variant:1:open" {
		t.Fatalf("unexpected event_index: %q", eventIndex)
	}
}

func TestRedirectRejectsUnsafeDestAndDoesNotRecord(t *testing.T) {
	router, conn := newTestRouter(t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/r?rid=token&dest=javascript:alert(1)", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM tracking_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected no events, got %d", count)
	}
}

func TestRedirectRecordsClickAndRedirects(t *testing.T) {
	router, conn := newTestRouter(t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/r?c=42&l=hero&rid=token&dest=https%3A%2F%2Fexample.com%2Fnext", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "https://example.com/next" {
		t.Fatalf("location = %q", got)
	}

	var kind, campaign, link, eventIndex string
	if err := conn.QueryRow(`SELECT kind,campaign,link,event_index FROM tracking_events`).Scan(&kind, &campaign, &link, &eventIndex); err != nil {
		t.Fatal(err)
	}
	if kind != "click" || campaign != "42" || link != "hero" {
		t.Fatalf("unexpected event: kind=%s campaign=%s link=%s", kind, campaign, link)
	}
	if eventIndex != "hero" {
		t.Fatalf("unexpected event_index: %s", eventIndex)
	}
}

func TestEventsEndpointFiltersAndUsesCursor(t *testing.T) {
	router, conn := newTestRouter(t)
	_, err := conn.Exec(`
		INSERT INTO tracking_events(source,campaign,token,kind,forwarded_for) VALUES
		('tenant','1','a','open',''),
		('tenant','1','b','click','198.51.100.8'),
		('tenant','2','c','open','')`)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/events?source=tenant&campaign=1&after_id=1&limit=10", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Events []Event `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Events) != 1 {
		t.Fatalf("expected 1 event, got %#v", body.Events)
	}
	if body.Events[0].Kind != "click" || body.Events[0].Token != "b" {
		t.Fatalf("unexpected event: %#v", body.Events[0])
	}
	if body.Events[0].ForwardedFor != "198.51.100.8" {
		t.Fatalf("unexpected forwarded_for: %#v", body.Events[0])
	}
}
