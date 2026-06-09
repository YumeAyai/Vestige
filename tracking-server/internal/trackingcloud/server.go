package trackingcloud

import (
	"database/sql"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"nousmail/pkg/tracker"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Server struct {
	db       *sql.DB
	assetDir string
}

type Event struct {
	ID             int64  `json:"id"`
	Source         string `json:"source"`
	Campaign       string `json:"campaign"`
	Link           string `json:"link"`
	Token          string `json:"token"`
	Kind           string `json:"kind"`
	TriggeredAt    string `json:"triggered_at"`
	IP             string `json:"ip,omitempty"`
	UserAgent      string `json:"user_agent,omitempty"`
	Referer        string `json:"referer,omitempty"`
	AcceptLanguage string `json:"accept_language,omitempty"`
	ForwardedFor   string `json:"forwarded_for,omitempty"`
}

func Migrate(conn *sql.DB) error {
	_, err := conn.Exec(`
CREATE TABLE IF NOT EXISTS tracking_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  source TEXT NOT NULL DEFAULT '',
  campaign TEXT NOT NULL DEFAULT '',
  link TEXT NOT NULL DEFAULT '',
  token TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL,
  ip TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  referer TEXT NOT NULL DEFAULT '',
  accept_language TEXT NOT NULL DEFAULT '',
  forwarded_for TEXT NOT NULL DEFAULT '',
  raw_payload TEXT NOT NULL DEFAULT '',
  triggered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_tracking_events_lookup ON tracking_events(source,campaign,kind,triggered_at);
CREATE INDEX IF NOT EXISTS idx_tracking_events_token ON tracking_events(token,kind,triggered_at);
CREATE INDEX IF NOT EXISTS idx_tracking_events_source_cursor ON tracking_events(source,id);
`)
	if err != nil {
		return err
	}
	return addColumns(conn, "tracking_events", map[string]string{
		"forwarded_for": "TEXT NOT NULL DEFAULT ''",
	})
}

func addColumns(conn *sql.DB, table string, columns map[string]string) error {
	rows, err := conn.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	existing := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for name, definition := range columns {
		if existing[name] {
			continue
		}
		if _, err := conn.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + name + ` ` + definition); err != nil {
			return err
		}
	}
	return nil
}

func New(db *sql.DB) *gin.Engine {
	return NewWithOptions(db, Options{})
}

type Options struct {
	AssetDir string
}

func NewWithOptions(db *sql.DB, opts Options) *gin.Engine {
	s := &Server{db: db, assetDir: strings.TrimSpace(opts.AssetDir)}
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true, "service": "tracking-cloud"}) })
	r.GET("/p", s.pixel)
	r.GET("/r", s.redirect)
	r.GET("/qrcode.png", s.qrcode)
	r.GET("/api/stats", s.stats)
	r.GET("/api/events", s.events)
	r.POST("/api/assets", s.uploadAsset)
	return r
}

func (s *Server) pixel(c *gin.Context) {
	_ = s.record(c, eventFromRequest(c, "open"))
	c.Header("Content-Type", "image/gif")
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Data(http.StatusOK, "image/gif", tracker.PixelGIF)
}

func (s *Server) redirect(c *gin.Context) {
	dest := strings.TrimSpace(c.Query("dest"))
	if !validRedirect(dest) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dest must be an http or https URL"})
		return
	}
	_ = s.record(c, eventFromRequest(c, "click"))
	c.Redirect(http.StatusFound, dest)
}

func (s *Server) qrcode(c *gin.Context) {
	token := strings.TrimSpace(firstNonEmpty(c.Query("token"), c.Query("rid")))
	if token != "" && token != "preview" {
		event := eventFromRequest(c, "qrcode")
		event.Token = token
		_ = s.record(c, event)
	}
	asset := strings.TrimSpace(c.Query("asset"))
	if asset != "" {
		name := filepath.Base(asset)
		if name != asset {
			c.Status(http.StatusNotFound)
			return
		}
		path := filepath.Join(s.trackingAssetDir(), name)
		if _, err := os.Stat(path); err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		contentType := mime.TypeByExtension(filepath.Ext(name))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		c.Header("Content-Type", contentType)
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		c.File(path)
		return
	}
	target := strings.TrimSpace(c.Query("target"))
	if target == "" || !validRedirect(target) {
		target = "https://example.com/survey"
	}
	png, err := tracker.QRCodePNG(target, queryInt(c, "size", 176))
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Content-Type", "image/png")
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Data(http.StatusOK, "image/png", png)
}

func (s *Server) uploadAsset(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请上传企业微信二维码图片"})
		return
	}
	contentType := file.Header.Get("Content-Type")
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext == "" {
		ext = imageExt(contentType)
	}
	if !allowedTrackingImageExt(ext) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "仅支持 PNG、JPG、GIF 或 WebP 图片"})
		return
	}
	width, _ := strconv.Atoi(strings.TrimSpace(c.PostForm("width")))
	if width <= 0 {
		width = 176
	}
	if width < 96 {
		width = 96
	}
	if width > 480 {
		width = 480
	}
	label := strings.TrimSpace(c.PostForm("label"))
	if label == "" {
		label = "企业微信二维码"
	}
	dir := s.trackingAssetDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(c, err)
		return
	}
	name := uuid.NewString() + ext
	if err := c.SaveUploadedFile(file, filepath.Join(dir, name)); err != nil {
		fail(c, err)
		return
	}
	baseURL := requestBaseURL(c)
	c.JSON(http.StatusOK, gin.H{
		"label":       label,
		"asset":       name,
		"placeholder": `{{TrackingImage "` + name + `"}}`,
		"image_url":   tracker.AssetImageURL(baseURL, "preview", name),
		"html":        tracker.TrackingImageHTML(baseURL, "preview", name, label, width),
		"collects":    []string{"ip", "user_agent", "referer", "accept_language", "forwarded_for", "triggered_at", "is_prefetch"},
	})
}

func (s *Server) stats(c *gin.Context) {
	where, args := filters(c, false)
	rows, err := s.db.Query(`
		SELECT kind, COUNT(*), COUNT(DISTINCT NULLIF(token,'')) unique_tokens
		FROM tracking_events `+where+`
		GROUP BY kind
		ORDER BY kind`, args...)
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	summary := []gin.H{}
	for rows.Next() {
		var kind string
		var count, uniqueTokens int
		_ = rows.Scan(&kind, &count, &uniqueTokens)
		summary = append(summary, gin.H{"kind": kind, "count": count, "unique_tokens": uniqueTokens})
	}

	trendRows, err := s.db.Query(`
		SELECT strftime('%Y-%m-%d %H:00', triggered_at) hour, kind, COUNT(*)
		FROM tracking_events `+where+`
		GROUP BY hour, kind
		ORDER BY hour, kind`, args...)
	if err != nil {
		fail(c, err)
		return
	}
	defer trendRows.Close()
	trend := []gin.H{}
	for trendRows.Next() {
		var hour, kind string
		var count int
		_ = trendRows.Scan(&hour, &kind, &count)
		trend = append(trend, gin.H{"hour": hour, "kind": kind, "count": count})
	}
	c.JSON(http.StatusOK, gin.H{"summary": summary, "trend": trend})
}

func (s *Server) events(c *gin.Context) {
	where, args := filters(c, true)
	limit := queryInt(c, "limit", 500)
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	args = append(args, limit)
	rows, err := s.db.Query(`
		SELECT id,source,campaign,link,token,kind,triggered_at,ip,user_agent,referer,accept_language,forwarded_for
		FROM tracking_events `+where+`
		ORDER BY id ASC
		LIMIT ?`, args...)
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	items := []Event{}
	for rows.Next() {
		var item Event
		if err := rows.Scan(&item.ID, &item.Source, &item.Campaign, &item.Link, &item.Token, &item.Kind, &item.TriggeredAt, &item.IP, &item.UserAgent, &item.Referer, &item.AcceptLanguage, &item.ForwardedFor); err != nil {
			fail(c, err)
			return
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, gin.H{"events": items})
}

func (s *Server) record(c *gin.Context, event Event) error {
	if event.Token == "" && event.Campaign == "" {
		return nil
	}
	raw, _ := json.Marshal(gin.H{
		"source":          event.Source,
		"campaign":        event.Campaign,
		"link":            event.Link,
		"token":           event.Token,
		"kind":            event.Kind,
		"ip":              event.IP,
		"user_agent":      event.UserAgent,
		"referer":         event.Referer,
		"accept_language": event.AcceptLanguage,
		"forwarded_for":   event.ForwardedFor,
	})
	_, err := s.db.Exec(
		`INSERT INTO tracking_events(source,campaign,link,token,kind,ip,user_agent,referer,accept_language,forwarded_for,raw_payload) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		event.Source,
		event.Campaign,
		event.Link,
		event.Token,
		event.Kind,
		event.IP,
		event.UserAgent,
		event.Referer,
		event.AcceptLanguage,
		event.ForwardedFor,
		string(raw),
	)
	return err
}

func eventFromRequest(c *gin.Context, kind string) Event {
	return Event{
		Source:         firstNonEmpty(c.Query("s"), c.Query("source")),
		Campaign:       firstNonEmpty(c.Query("c"), c.Query("campaign")),
		Link:           firstNonEmpty(c.Query("l"), c.Query("link")),
		Token:          firstNonEmpty(c.Query("rid"), c.Query("tid"), c.Query("token")),
		Kind:           kind,
		IP:             c.ClientIP(),
		UserAgent:      c.GetHeader("User-Agent"),
		Referer:        c.GetHeader("Referer"),
		AcceptLanguage: c.GetHeader("Accept-Language"),
		ForwardedFor:   c.GetHeader("X-Forwarded-For"),
	}
}

func filters(c *gin.Context, includeCursor bool) (string, []any) {
	clauses := []string{}
	args := []any{}
	if value := strings.TrimSpace(firstNonEmpty(c.Query("s"), c.Query("source"))); value != "" {
		clauses = append(clauses, "source=?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(firstNonEmpty(c.Query("c"), c.Query("campaign"))); value != "" {
		clauses = append(clauses, "campaign=?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(firstNonEmpty(c.Query("event"), c.Query("kind"))); value != "" {
		clauses = append(clauses, "kind=?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(c.Query("since")); value != "" {
		clauses = append(clauses, "triggered_at>=?")
		args = append(args, value)
	}
	if includeCursor {
		if after := queryInt(c, "after_id", 0); after > 0 {
			clauses = append(clauses, "id>?")
			args = append(args, after)
		}
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func validRedirect(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func queryInt(c *gin.Context, key string, fallback int) int {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *Server) trackingAssetDir() string {
	if value := strings.TrimSpace(os.Getenv("TRACKING_ASSET_DIR")); value != "" {
		return value
	}
	if s.assetDir != "" {
		return s.assetDir
	}
	return filepath.Join("data", "tracking-assets")
}

func requestBaseURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}

func imageExt(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}

func allowedTrackingImageExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	default:
		return false
	}
}

func fail(c *gin.Context, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}
