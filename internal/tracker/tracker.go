package tracker

import (
	"database/sql"
	"encoding/base64"
	"net/url"
	"strings"
)

var PixelGIF []byte

func init() {
	PixelGIF, _ = base64.StdEncoding.DecodeString("R0lGODlhAQABAPAAAP///wAAACH5BAAAAAAALAAAAAABAAEAAAICRAEAOw==")
}

type Event struct {
	TrackingID string
	IP         string
	UserAgent  string
}

type Recorder interface {
	RecordOpen(event Event) error
}

type SQLiteRecorder struct {
	db *sql.DB
}

func NewSQLiteRecorder(db *sql.DB) SQLiteRecorder {
	return SQLiteRecorder{db: db}
}

func (r SQLiteRecorder) RecordOpen(event Event) error {
	var recipientID int64
	if err := r.db.QueryRow(`SELECT id FROM campaign_recipients WHERE tracking_id=?`, event.TrackingID).Scan(&recipientID); err != nil {
		return err
	}
	_, err := r.db.Exec(
		`INSERT INTO open_events(campaign_recipient_id,tracking_id,ip,user_agent,is_prefetch) VALUES(?,?,?,?,?)`,
		recipientID,
		event.TrackingID,
		event.IP,
		event.UserAgent,
		LooksLikePrefetch(event.UserAgent),
	)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(
		`UPDATE campaign_recipients SET open_count=open_count+1, first_opened_at=COALESCE(first_opened_at,CURRENT_TIMESTAMP), last_opened_at=CURRENT_TIMESTAMP WHERE id=?`,
		recipientID,
	)
	return err
}

func PixelURL(baseURL, trackingID string) string {
	return strings.TrimRight(baseURL, "/") + "/api/track/open.gif?tid=" + url.QueryEscape(trackingID)
}

func InjectPixel(body, baseURL, trackingID string) string {
	pixel := `<img src="` + PixelURL(baseURL, trackingID) + `" width="1" height="1" alt="" style="display:none;width:1px;height:1px;border:0" />`
	if strings.Contains(strings.ToLower(body), "</body>") {
		return strings.Replace(body, "</body>", pixel+"</body>", 1)
	}
	return body + pixel
}

func LooksLikePrefetch(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	return strings.Contains(ua, "googleimageproxy") || strings.Contains(ua, "apple") && strings.Contains(ua, "mail")
}
