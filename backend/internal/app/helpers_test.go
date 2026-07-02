package app

import (
	"database/sql"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Vestige/pkg/config"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

func TestReadContactsCSVWithoutHeaderUsesFallbackColumns(t *testing.T) {
	data := []byte("Acme Inc,hello@example.com,unused,unused,021-12345678\n")

	contacts, err := readContactsCSV(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(contacts))
	}
	contact := contacts[0]
	if contact.Name != "Acme Inc" || contact.Company != "Acme Inc" {
		t.Fatalf("unexpected company mapping: %#v", contact)
	}
	if contact.Email != "hello@example.com" {
		t.Fatalf("unexpected email: %s", contact.Email)
	}
	if contact.Phone != "021-12345678" {
		t.Fatalf("unexpected phone: %s", contact.Phone)
	}
}

func TestTemplateMarkers(t *testing.T) {
	if !templateUsesQRCode(`<p>{{.QRCode}}</p>`) {
		t.Fatal("expected QRCode marker to be detected")
	}
	if templateUsesQRCode(`<p>{{QRCode}}</p>`) {
		t.Fatal("unexpected QRCode marker detection")
	}
	if !templateUsesTrackingImage(`{{TrackingImage "asset.png"}}`) {
		t.Fatal("expected TrackingImage marker to be detected")
	}
}

func TestQRCodeTargetURLUsesEnvAndChoosesSeparator(t *testing.T) {
	t.Setenv("QR_CODE_TARGET_URL", "https://example.com/survey?src=email")
	server := &Server{cfg: config.Default()}

	got := server.qrcodeTargetURL("token-1")
	if got != "https://example.com/survey?src=email&t=token-1" {
		t.Fatalf("unexpected target URL: %s", got)
	}
}

func TestTrackingBaseURLIgnoresAppBaseURLHeader(t *testing.T) {
	cfg := config.Default()
	cfg.Client.TrackingBaseURL = "https://track.example.com/jianji"
	server := &Server{cfg: cfg}
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest("POST", "/api/campaigns/1/send", nil)
	ctx.Request.Header.Set("X-Base-URL", "http://localhost:8080")

	got := server.trackingBaseURL(ctx)
	if got != "https://track.example.com/jianji" {
		t.Fatalf("tracking base URL used app base URL header: %s", got)
	}
}

func TestLocalDataPathFollowsAbsoluteDBPath(t *testing.T) {
	cfg := config.Default()
	server := &Server{cfg: cfg}
	if got := server.localDataPath("campaign-attachments"); got != filepath.Join("data", "campaign-attachments") {
		t.Fatalf("relative DB path should use data dir, got %s", got)
	}

	cfg.Client.DBPath = filepath.Join(t.TempDir(), "app.db")
	server = &Server{cfg: cfg}
	if got, want := server.localDataPath("campaign-attachments"), filepath.Join(filepath.Dir(cfg.Client.DBPath), "campaign-attachments"); got != want {
		t.Fatalf("absolute DB path should use DB dir: got %s want %s", got, want)
	}
}

func TestCampaignSendDelayUsesConfiguredRateAndJitter(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	settings := campaignSendSettings{RatePerMinute: 8, JitterPercent: 0}
	if delay := campaignSendDelay(time.Time{}, now, settings, 0.5); delay != 0 {
		t.Fatalf("first send should not wait, got %s", delay)
	}
	if delay := campaignSendDelay(now, now.Add(2*time.Second), settings, 0.5); delay != 5500*time.Millisecond {
		t.Fatalf("expected 5.5s remaining delay, got %s", delay)
	}
	if delay := campaignSendDelay(now, now.Add(7500*time.Millisecond), settings, 0.5); delay != 0 {
		t.Fatalf("send at interval boundary should not wait, got %s", delay)
	}

	settings.JitterPercent = 20
	if delay := campaignSendDelay(now, now, settings, 0); delay != 6*time.Second {
		t.Fatalf("expected low jitter interval of 6s, got %s", delay)
	}
	if delay := campaignSendDelay(now, now, settings, 1); delay != 9*time.Second {
		t.Fatalf("expected high jitter interval of 9s, got %s", delay)
	}
}

func TestCampaignSendSettingsReadsDatabaseValues(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := conn.Exec(`CREATE TABLE app_settings(key TEXT PRIMARY KEY,value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO app_settings(key,value) VALUES('campaign_send_rate_per_minute','8'),('campaign_send_jitter_percent','35')`); err != nil {
		t.Fatal(err)
	}
	server := &Server{db: conn}
	settings := server.campaignSendSettings()
	if settings.RatePerMinute != 8 || settings.JitterPercent != 35 {
		t.Fatalf("unexpected settings: %#v", settings)
	}

	if _, err := conn.Exec(`UPDATE app_settings SET value='999' WHERE key='campaign_send_rate_per_minute'`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`UPDATE app_settings SET value='bad' WHERE key='campaign_send_jitter_percent'`); err != nil {
		t.Fatal(err)
	}
	settings = server.campaignSendSettings()
	if settings.RatePerMinute != 120 || settings.JitterPercent != defaultCampaignSendJitterPercent {
		t.Fatalf("expected clamped/fallback settings, got %#v", settings)
	}
}

func TestImageExtensionHelpers(t *testing.T) {
	if got := imageExt(" image/png "); got != ".png" {
		t.Fatalf("imageExt returned %q", got)
	}
	if !allowedTrackingImageExt(".WEBP") {
		t.Fatal("expected .WEBP to be allowed")
	}
	if allowedTrackingImageExt(".svg") {
		t.Fatal("expected .svg to be rejected")
	}
}

func TestCompactNotesIncludesNonEmptyFields(t *testing.T) {
	got := compactNotes(map[string]string{
		"官网":   " https://example.com ",
		"数据来源": "",
	})
	if !strings.Contains(got, "官网：https://example.com") {
		t.Fatalf("unexpected notes: %s", got)
	}
	if strings.Contains(got, "数据来源") {
		t.Fatalf("empty fields should be omitted: %s", got)
	}
}

func TestHourlyTrendUsesLocalTimezone(t *testing.T) {
	previousLocal := time.Local
	time.Local = time.FixedZone("CST", 8*60*60)
	t.Cleanup(func() { time.Local = previousLocal })

	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	server := &Server{db: conn}
	trend := server.hourlyTrend(`SELECT ? UNION ALL SELECT ?`, "2026-06-09T16:30:00Z", "2026-06-09T17:10:00Z")
	if len(trend) != 2 {
		t.Fatalf("expected 2 hourly buckets, got %#v", trend)
	}
	if trend[0]["hour"] != "2026-06-10T00:00:00+08:00" || trend[1]["hour"] != "2026-06-10T01:00:00+08:00" {
		t.Fatalf("expected local +08:00 buckets, got %#v", trend)
	}
}

func TestTimezoneDataIncludesShanghai(t *testing.T) {
	if _, err := time.LoadLocation("Asia/Shanghai"); err != nil {
		t.Fatalf("expected embedded timezone data to include Asia/Shanghai: %v", err)
	}
}
