package tracker

import (
	"database/sql"
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

var PixelGIF []byte

const DeliverySecurityScanWindow = 5 * time.Second

func init() {
	PixelGIF, _ = base64.StdEncoding.DecodeString("R0lGODlhAQABAPAAAP///wAAACH5BAAAAAAALAAAAAABAAEAAAICRAEAOw==")
}

type Event struct {
	TrackingID string
	Kind       string
	EventIndex string
	Source     string
	Campaign   string
	Link       string
	IP         string
	UserAgent  string
	Referer    string
}

type MarkEvent struct {
	Token          string
	Kind           string
	EventIndex     string
	Source         string
	IP             string
	UserAgent      string
	Referer        string
	AcceptLanguage string
	ForwardedFor   string
	Raw            string
}

type Recorder interface {
	RecordOpen(event Event) error
	RecordMark(event MarkEvent) error
}

type SQLiteRecorder struct {
	db *sql.DB
}

func NewSQLiteRecorder(db *sql.DB) SQLiteRecorder {
	return SQLiteRecorder{db: db}
}

func (r SQLiteRecorder) RecordOpen(event Event) error {
	var recipientID int64
	var sentAt string
	if err := r.db.QueryRow(`SELECT id,COALESCE(sent_at,'') FROM campaign_recipients WHERE tracking_id=?`, event.TrackingID).Scan(&recipientID, &sentAt); err != nil {
		return err
	}
	isPrefetch := LooksLikePrefetch(event.UserAgent) || LooksLikeDeliverySecurityScan(sentAt, time.Now())
	_, err := r.db.Exec(
		`INSERT INTO open_events(campaign_recipient_id,tracking_id,event_index,ip,user_agent,is_prefetch) VALUES(?,?,?,?,?,?)`,
		recipientID,
		event.TrackingID,
		event.EventIndex,
		event.IP,
		event.UserAgent,
		isPrefetch,
	)
	if err != nil {
		return err
	}
	if isPrefetch {
		return nil
	}
	_, err = r.db.Exec(
		`UPDATE campaign_recipients SET open_count=open_count+1, first_opened_at=COALESCE(first_opened_at,CURRENT_TIMESTAMP), last_opened_at=CURRENT_TIMESTAMP WHERE id=?`,
		recipientID,
	)
	return err
}

func (r SQLiteRecorder) RecordMark(event MarkEvent) error {
	var markID int64
	var kind string
	var sentAt string
	if err := r.db.QueryRow(`
		SELECT tm.id,tm.kind,COALESCE(cr.sent_at,'')
		FROM tracking_marks tm
		JOIN campaign_recipients cr ON cr.id=tm.campaign_recipient_id
		WHERE tm.token=?`, event.Token).Scan(&markID, &kind, &sentAt); err != nil {
		markID = 0
		kind = event.Kind
	}
	source := event.Source
	if source == "" {
		source = "local"
	}
	_, err := r.db.Exec(
		`INSERT INTO tracking_mark_events(mark_id,token,kind,event_index,source,ip,user_agent,referer,accept_language,forwarded_for,is_prefetch,raw_payload) VALUES(NULLIF(?,0),?,?,?,?,?,?,?,?,?,?,?)`,
		markID,
		event.Token,
		firstNonEmpty(kind, event.Kind),
		event.EventIndex,
		source,
		event.IP,
		event.UserAgent,
		event.Referer,
		event.AcceptLanguage,
		event.ForwardedFor,
		LooksLikePrefetch(event.UserAgent) || LooksLikeDeliverySecurityScan(sentAt, time.Now()),
		event.Raw,
	)
	return err
}

func PixelURL(baseURL, trackingID string, campaignIDs ...string) string {
	return PixelURLWithSource(baseURL, "", trackingID, campaignIDs...)
}

func PixelURLWithSource(baseURL, source, trackingID string, campaignIDs ...string) string {
	eventIndex := ""
	if len(campaignIDs) > 1 {
		eventIndex = campaignIDs[1]
	}
	return PixelURLWithSourceAndIndex(baseURL, source, trackingID, firstVariadic(campaignIDs), eventIndex)
}

func PixelURLWithSourceAndIndex(baseURL, source, trackingID, campaignID, eventIndex string) string {
	values := url.Values{}
	values.Set("rid", trackingID)
	if source != "" {
		values.Set("s", source)
	}
	if campaignID != "" {
		values.Set("c", campaignID)
	}
	if eventIndex != "" {
		values.Set("i", eventIndex)
	}
	return strings.TrimRight(baseURL, "/") + "/p?" + values.Encode()
}

func RedirectURL(baseURL, campaignID, linkID, trackingID, dest string) string {
	return RedirectURLWithSource(baseURL, "", campaignID, linkID, trackingID, dest)
}

func RedirectURLWithSource(baseURL, source, campaignID, linkID, trackingID, dest string) string {
	return RedirectURLWithSourceAndIndex(baseURL, source, campaignID, linkID, trackingID, dest, linkID)
}

func RedirectURLWithSourceAndIndex(baseURL, source, campaignID, linkID, trackingID, dest, eventIndex string) string {
	values := url.Values{}
	if source != "" {
		values.Set("s", source)
	}
	values.Set("c", campaignID)
	values.Set("l", linkID)
	values.Set("rid", trackingID)
	if eventIndex != "" {
		values.Set("i", eventIndex)
	}
	values.Set("dest", dest)
	return strings.TrimRight(baseURL, "/") + "/r?" + values.Encode()
}

func MarkImageURL(baseURL, token string, targets ...string) string {
	return MarkImageURLWithSource(baseURL, "", token, targets...)
}

func MarkImageURLWithSource(baseURL, source, token string, targets ...string) string {
	return MarkImageURLWithSourceAndIndex(baseURL, source, token, "", targets...)
}

func MarkImageURLWithSourceAndIndex(baseURL, source, token, eventIndex string, targets ...string) string {
	values := url.Values{}
	values.Set("token", token)
	values.Set("type", "qr")
	if source != "" {
		values.Set("s", source)
	}
	if eventIndex != "" {
		values.Set("i", eventIndex)
	}
	if len(targets) > 0 && targets[0] != "" {
		values.Set("target", targets[0])
	}
	return strings.TrimRight(baseURL, "/") + "/img?" + values.Encode()
}

func AssetImageURL(baseURL, token, asset string) string {
	return AssetImageURLWithSource(baseURL, "", token, asset)
}

func AssetImageURLWithSource(baseURL, source, token, asset string) string {
	return AssetImageURLWithSourceAndIndex(baseURL, source, token, asset, asset)
}

func AssetImageURLWithSourceAndIndex(baseURL, source, token, asset, eventIndex string) string {
	values := url.Values{}
	values.Set("token", token)
	values.Set("asset", asset)
	if source != "" {
		values.Set("s", source)
	}
	if eventIndex != "" {
		values.Set("i", eventIndex)
	}
	return strings.TrimRight(baseURL, "/") + "/img?" + values.Encode()
}

func QRCodeHTML(baseURL, token string, targets ...string) string {
	return QRCodeHTMLWithSource(baseURL, "", token, targets...)
}

func QRCodeHTMLWithSource(baseURL, source, token string, targets ...string) string {
	return QRCodeHTMLWithSourceAndIndex(baseURL, source, token, "", targets...)
}

func QRCodeHTMLWithSourceAndIndex(baseURL, source, token, eventIndex string, targets ...string) string {
	return `<img src="` + MarkImageURLWithSourceAndIndex(baseURL, source, token, eventIndex, targets...) + `" width="132" height="132" alt="二维码" style="width:132px;height:132px;border:0" />`
}

func TrackingImageHTML(baseURL, token, asset, alt string, width int) string {
	return TrackingImageHTMLWithSource(baseURL, "", token, asset, alt, width)
}

func TrackingImageHTMLWithSource(baseURL, source, token, asset, alt string, width int) string {
	return TrackingImageHTMLWithSourceAndIndex(baseURL, source, token, asset, alt, width, asset)
}

func TrackingImageHTMLWithSourceAndIndex(baseURL, source, token, asset, alt string, width int, eventIndex string) string {
	if width <= 0 {
		width = 176
	}
	if alt == "" {
		alt = "联系二维码"
	}
	return `<img src="` + AssetImageURLWithSourceAndIndex(baseURL, source, token, asset, eventIndex) + `" width="` + intString(width) + `" alt="` + templateEscape(alt) + `" style="width:` + intString(width) + `px;height:auto;border:0;display:block" />`
}

func QRCodePNG(target string, size int) ([]byte, error) {
	if size <= 0 {
		size = 160
	}
	return qrcode.Encode(target, qrcode.Medium, size)
}

func InjectPixel(body, baseURL, trackingID string, campaignIDs ...string) string {
	return InjectPixelWithSource(body, baseURL, "", trackingID, campaignIDs...)
}

func InjectPixelWithSource(body, baseURL, source, trackingID string, campaignIDs ...string) string {
	pixel := `<img src="` + PixelURLWithSource(baseURL, source, trackingID, campaignIDs...) + `" width="1" height="1" alt="" style="display:none;width:1px;height:1px;border:0" />`
	return injectPixel(body, pixel)
}

func InjectPixelWithSourceAndIndex(body, baseURL, source, trackingID, campaignID, eventIndex string) string {
	pixel := `<img src="` + PixelURLWithSourceAndIndex(baseURL, source, trackingID, campaignID, eventIndex) + `" width="1" height="1" alt="" style="display:none;width:1px;height:1px;border:0" />`
	return injectPixel(body, pixel)
}

func injectPixel(body, pixel string) string {
	if strings.Contains(strings.ToLower(body), "</body>") {
		return strings.Replace(body, "</body>", pixel+"</body>", 1)
	}
	return body + pixel
}

func firstVariadic(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func LooksLikePrefetch(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	return strings.Contains(ua, "googleimageproxy") || strings.Contains(ua, "apple") && strings.Contains(ua, "mail")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func LooksLikeDeliverySecurityScan(sentAt string, eventAt time.Time) bool {
	sent, ok := parseTime(sentAt)
	if !ok || eventAt.IsZero() {
		return false
	}
	diff := eventAt.Sub(sent)
	return diff >= 0 && diff <= DeliverySecurityScanWindow
}

func ParseEventTime(value string) (time.Time, bool) {
	return parseTime(value)
}

func parseTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func intString(value int) string {
	return strconv.Itoa(value)
}

func templateEscape(value string) string {
	value = strings.ReplaceAll(value, `&`, `&amp;`)
	value = strings.ReplaceAll(value, `"`, `&quot;`)
	value = strings.ReplaceAll(value, `<`, `&lt;`)
	value = strings.ReplaceAll(value, `>`, `&gt;`)
	return value
}
