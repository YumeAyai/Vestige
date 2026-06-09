package tracker

import (
	"database/sql"
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

var PixelGIF []byte

func init() {
	PixelGIF, _ = base64.StdEncoding.DecodeString("R0lGODlhAQABAPAAAP///wAAACH5BAAAAAAALAAAAAABAAEAAAICRAEAOw==")
}

type Event struct {
	TrackingID string
	Kind       string
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

func (r SQLiteRecorder) RecordMark(event MarkEvent) error {
	var markID int64
	var kind string
	if err := r.db.QueryRow(`SELECT id,kind FROM tracking_marks WHERE token=?`, event.Token).Scan(&markID, &kind); err != nil {
		markID = 0
		kind = event.Kind
	}
	source := event.Source
	if source == "" {
		source = "local"
	}
	_, err := r.db.Exec(
		`INSERT INTO tracking_mark_events(mark_id,token,kind,source,ip,user_agent,referer,accept_language,forwarded_for,is_prefetch,raw_payload) VALUES(NULLIF(?,0),?,?,?,?,?,?,?,?,?,?)`,
		markID,
		event.Token,
		firstNonEmpty(kind, event.Kind),
		source,
		event.IP,
		event.UserAgent,
		event.Referer,
		event.AcceptLanguage,
		event.ForwardedFor,
		LooksLikePrefetch(event.UserAgent),
		event.Raw,
	)
	return err
}

func PixelURL(baseURL, trackingID string, campaignIDs ...string) string {
	return PixelURLWithSource(baseURL, "", trackingID, campaignIDs...)
}

func PixelURLWithSource(baseURL, source, trackingID string, campaignIDs ...string) string {
	values := url.Values{}
	values.Set("rid", trackingID)
	if source != "" {
		values.Set("s", source)
	}
	if len(campaignIDs) > 0 && campaignIDs[0] != "" {
		values.Set("c", campaignIDs[0])
	}
	return strings.TrimRight(baseURL, "/") + "/p?" + values.Encode()
}

func RedirectURL(baseURL, campaignID, linkID, trackingID, dest string) string {
	return RedirectURLWithSource(baseURL, "", campaignID, linkID, trackingID, dest)
}

func RedirectURLWithSource(baseURL, source, campaignID, linkID, trackingID, dest string) string {
	values := url.Values{}
	if source != "" {
		values.Set("s", source)
	}
	values.Set("c", campaignID)
	values.Set("l", linkID)
	values.Set("rid", trackingID)
	values.Set("dest", dest)
	return strings.TrimRight(baseURL, "/") + "/r?" + values.Encode()
}

func MarkImageURL(baseURL, token string, targets ...string) string {
	return MarkImageURLWithSource(baseURL, "", token, targets...)
}

func MarkImageURLWithSource(baseURL, source, token string, targets ...string) string {
	values := url.Values{}
	values.Set("token", token)
	if source != "" {
		values.Set("s", source)
	}
	if len(targets) > 0 && targets[0] != "" {
		values.Set("target", targets[0])
	}
	return strings.TrimRight(baseURL, "/") + "/qrcode.png?" + values.Encode()
}

func AssetImageURL(baseURL, token, asset string) string {
	return AssetImageURLWithSource(baseURL, "", token, asset)
}

func AssetImageURLWithSource(baseURL, source, token, asset string) string {
	values := url.Values{}
	values.Set("token", token)
	values.Set("asset", asset)
	if source != "" {
		values.Set("s", source)
	}
	return strings.TrimRight(baseURL, "/") + "/qrcode.png?" + values.Encode()
}

func QRCodeHTML(baseURL, token string, targets ...string) string {
	return QRCodeHTMLWithSource(baseURL, "", token, targets...)
}

func QRCodeHTMLWithSource(baseURL, source, token string, targets ...string) string {
	return `<img src="` + MarkImageURLWithSource(baseURL, source, token, targets...) + `" width="132" height="132" alt="二维码" style="width:132px;height:132px;border:0" />`
}

func TrackingImageHTML(baseURL, token, asset, alt string, width int) string {
	return TrackingImageHTMLWithSource(baseURL, "", token, asset, alt, width)
}

func TrackingImageHTMLWithSource(baseURL, source, token, asset, alt string, width int) string {
	if width <= 0 {
		width = 176
	}
	if alt == "" {
		alt = "联系二维码"
	}
	return `<img src="` + AssetImageURLWithSource(baseURL, source, token, asset) + `" width="` + intString(width) + `" alt="` + templateEscape(alt) + `" style="width:` + intString(width) + `px;height:auto;border:0;display:block" />`
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
	if strings.Contains(strings.ToLower(body), "</body>") {
		return strings.Replace(body, "</body>", pixel+"</body>", 1)
	}
	return body + pixel
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
