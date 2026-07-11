package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"log"
	"math/rand"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"Vestige/backend/internal/mailer"
	"Vestige/pkg/config"
	"Vestige/pkg/models"
	"Vestige/pkg/tracker"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

type Server struct {
	db               *sql.DB
	frontend         fs.FS
	cfg              config.Config
	sendingCampaigns sync.Map
	cloudSyncMu      sync.Mutex
}

var cloudHTTPClient = &http.Client{Timeout: 2500 * time.Millisecond}

const (
	defaultCampaignSendRatePerMinute = 8
	defaultCampaignSendJitterPercent = 35
)

type campaignSendSettings struct {
	RatePerMinute int
	JitterPercent int
}

func New(db *sql.DB, frontend fs.FS) *gin.Engine {
	return NewWithConfig(db, frontend, config.MustLoadDefault())
}

func NewWithConfig(db *sql.DB, frontend fs.FS, cfg config.Config) *gin.Engine {
	s := &Server{db: db, frontend: frontend, cfg: cfg}
	r := gin.Default()

	api := r.Group("/api")
	api.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	r.GET("/img", s.trackingImage)

	api.GET("/mailboxes", s.listMailboxes)
	api.POST("/mailboxes", s.createMailbox)
	api.POST("/mailboxes/test", s.testMailbox)
	api.DELETE("/mailboxes/:id", s.deleteMailbox)
	api.GET("/contacts", s.listContacts)
	api.GET("/contacts/page", s.pageContacts)
	api.POST("/contacts", s.createContact)
	api.PATCH("/contacts/:id", s.updateContact)
	api.PATCH("/contacts/batch", s.updateContactsBatch)
	api.DELETE("/contacts/batch", s.deleteContactsBatch)
	api.POST("/contacts/import", s.importContacts)
	api.GET("/templates", s.listTemplates)
	api.POST("/templates", s.createTemplate)
	api.PATCH("/templates/:id", s.updateTemplate)
	api.DELETE("/templates/:id", s.deleteTemplate)
	api.POST("/templates/:id/copy", s.copyTemplate)
	api.POST("/templates/tracking-image-asset", s.createTemplateTrackingImageAsset)
	api.POST("/templates/preview", s.previewTemplate)
	api.GET("/campaigns", s.listCampaigns)
	api.POST("/campaign-attachments", s.uploadCampaignAttachment)
	api.POST("/campaigns", s.createCampaign)
	api.GET("/campaigns/:id", s.getCampaign)
	api.POST("/campaigns/:id/send", s.sendCampaign)
	api.GET("/campaigns/:id/stats", s.campaignStats)
	api.GET("/campaigns/:id/recipients", s.listRecipients)
	api.GET("/campaigns/:id/export.csv", s.exportCampaignCSV)
	api.POST("/campaigns/:id/variants", s.createVariant)
	api.PATCH("/campaigns/:id/variants/:vid", s.updateVariant)
	api.DELETE("/campaigns/:id/variants/:vid", s.deleteVariant)
	api.GET("/campaigns/:id/ab-stats", s.abStats)
	api.GET("/campaigns/:id/links", s.listLinks)
	api.GET("/campaigns/:id/links/:lid/stats", s.linkStats)
	api.GET("/stats", s.globalStats)
	api.POST("/tracking/cloud-events/import", s.importCloudTrackingEvents)
	api.POST("/tracking/cloud-events/sync", s.syncCloudTrackingEventsHandler)

	dist, err := fs.Sub(frontend, "dist")
	if err == nil {
		r.GET("/", func(c *gin.Context) {
			serveEmbedded(c, dist, "index.html")
		})
		r.NoRoute(func(c *gin.Context) {
			path := strings.TrimPrefix(c.Request.URL.Path, "/")
			if path == "" {
				path = "index.html"
			}
			if _, err := fs.Stat(dist, path); err != nil {
				path = "index.html"
			}
			serveEmbedded(c, dist, path)
		})
	}

	return r
}

func (s *Server) localDataPath(parts ...string) string {
	base := "data"
	if dbPath := strings.TrimSpace(s.cfg.Client.DBPath); filepath.IsAbs(dbPath) {
		base = filepath.Dir(dbPath)
	}
	return filepath.Join(append([]string{base}, parts...)...)
}

func (s *Server) listMailboxes(c *gin.Context) {
	rows, err := s.db.Query(`SELECT id,name,host,port,username,from_email,from_name,use_tls FROM mailboxes ORDER BY id DESC`)
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	items := []models.Mailbox{}
	for rows.Next() {
		var item models.Mailbox
		if err := rows.Scan(&item.ID, &item.Name, &item.Host, &item.Port, &item.Username, &item.FromEmail, &item.FromName, &item.UseTLS); err != nil {
			fail(c, err)
			return
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, items)
}

func (s *Server) createMailbox(c *gin.Context) {
	var input models.Mailbox
	if bind(c, &input) != nil {
		return
	}
	input = mailer.NormalizeMailbox(input)
	if err := mailer.ValidateMailbox(input); err != nil {
		fail(c, err)
		return
	}
	res, err := s.db.Exec(`INSERT INTO mailboxes(name,host,port,username,password,from_email,from_name,use_tls) VALUES(?,?,?,?,?,?,?,?)`,
		input.Name, input.Host, input.Port, input.Username, input.Password, input.FromEmail, input.FromName, input.UseTLS)
	if err != nil {
		fail(c, err)
		return
	}
	input.ID, _ = res.LastInsertId()
	input.Password = ""
	c.JSON(http.StatusCreated, input)
}

type testMailboxInput struct {
	models.Mailbox
	ToEmail string `json:"to_email"`
}

func (s *Server) testMailbox(c *gin.Context) {
	var input testMailboxInput
	if bind(c, &input) != nil {
		return
	}
	if err := mailer.Test(input.Mailbox, input.ToEmail); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) deleteMailbox(c *gin.Context) {
	id := c.Param("id")
	var used int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM campaigns WHERE mailbox_id=?`, id).Scan(&used); err != nil {
		fail(c, err)
		return
	}
	if used > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "该邮箱已被邮件任务使用，不能删除"})
		return
	}
	res, err := s.db.Exec(`DELETE FROM mailboxes WHERE id=?`, id)
	if err != nil {
		fail(c, err)
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) listContacts(c *gin.Context) {
	rows, err := s.db.Query(`SELECT id,name,email,company,department,phone,tags,notes FROM contacts ORDER BY id DESC`)
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	items := []models.Contact{}
	for rows.Next() {
		var item models.Contact
		if err := rows.Scan(&item.ID, &item.Name, &item.Email, &item.Company, &item.Department, &item.Phone, &item.Tags, &item.Notes); err != nil {
			fail(c, err)
			return
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, items)
}

func (s *Server) pageContacts(c *gin.Context) {
	allRows := strings.EqualFold(strings.TrimSpace(c.Query("limit")), "all")
	limit := queryInt(c, "limit", 20)
	if !allRows {
		if limit <= 0 {
			limit = 20
		}
		if limit > 200 {
			limit = 200
		}
	}
	offset := queryInt(c, "offset", 0)
	if offset < 0 {
		offset = 0
	}
	q := strings.TrimSpace(c.Query("q"))

	where := ""
	args := []any{}
	if q != "" {
		where = `WHERE name LIKE ? OR email LIKE ? OR company LIKE ? OR phone LIKE ? OR tags LIKE ? OR notes LIKE ?`
		like := "%" + q + "%"
		args = append(args, like, like, like, like, like, like)
	}

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM contacts `+where, args...).Scan(&total); err != nil {
		fail(c, err)
		return
	}
	if allRows {
		limit = total
		offset = 0
	}

	pageArgs := append([]any{}, args...)
	pageArgs = append(pageArgs, limit, offset)
	rows, err := s.db.Query(`SELECT id,name,email,company,department,phone,tags,notes FROM contacts `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, pageArgs...)
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()

	items := []models.Contact{}
	for rows.Next() {
		var item models.Contact
		if err := rows.Scan(&item.ID, &item.Name, &item.Email, &item.Company, &item.Department, &item.Phone, &item.Tags, &item.Notes); err != nil {
			fail(c, err)
			return
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, gin.H{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

type importContactsInput struct {
	Filename string `json:"filename"`
	Data     string `json:"data"`
	Path     string `json:"path"`
}

type ContactImportResult struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

func (s *Server) createContact(c *gin.Context) {
	var input models.Contact
	if bind(c, &input) != nil {
		return
	}
	input.Email = mailer.NormalizeEmailList(input.Email)
	if err := mailer.ValidateEmailList(input.Email, "邮箱"); err != nil {
		fail(c, err)
		return
	}
	if input.Name == "" {
		input.Name = input.Email
	}
	res, err := s.db.Exec(`INSERT INTO contacts(name,email,company,department,phone,tags,notes) VALUES(?,?,?,?,?,?,?)`,
		input.Name, input.Email, input.Company, input.Department, input.Phone, input.Tags, input.Notes)
	if err != nil {
		fail(c, err)
		return
	}
	input.ID, _ = res.LastInsertId()
	c.JSON(http.StatusCreated, input)
}

func (s *Server) updateContact(c *gin.Context) {
	var input models.Contact
	if bind(c, &input) != nil {
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "联系人不存在"})
		return
	}
	input.Email = mailer.NormalizeEmailList(input.Email)
	if err := mailer.ValidateEmailList(input.Email, "邮箱"); err != nil {
		fail(c, err)
		return
	}
	if input.Name == "" {
		input.Name = firstNonEmpty(input.Company, input.Email)
	}
	res, err := s.db.Exec(`UPDATE contacts SET name=?,email=?,company=?,department=?,phone=?,tags=?,notes=? WHERE id=?`,
		input.Name, input.Email, input.Company, input.Department, input.Phone, input.Tags, input.Notes, id)
	if err != nil {
		fail(c, err)
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	input.ID = id
	c.JSON(http.StatusOK, input)
}

type contactBatchInput struct {
	IDs   []int64 `json:"ids"`
	Tags  string  `json:"tags"`
	Notes string  `json:"notes"`
}

func (s *Server) updateContactsBatch(c *gin.Context) {
	var input contactBatchInput
	if bind(c, &input) != nil {
		return
	}
	if len(input.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择联系人"})
		return
	}
	updated := 0
	for _, id := range input.IDs {
		res, err := s.db.Exec(`UPDATE contacts SET tags=CASE WHEN ?='' THEN tags ELSE ? END, notes=CASE WHEN ?='' THEN notes ELSE ? END WHERE id=?`,
			input.Tags, input.Tags, input.Notes, input.Notes, id)
		if err != nil {
			fail(c, err)
			return
		}
		affected, _ := res.RowsAffected()
		updated += int(affected)
	}
	c.JSON(http.StatusOK, gin.H{"updated": updated})
}

func (s *Server) deleteContactsBatch(c *gin.Context) {
	var input contactBatchInput
	if bind(c, &input) != nil {
		return
	}
	if len(input.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择联系人"})
		return
	}
	deleted := 0
	for _, id := range input.IDs {
		res, err := s.db.Exec(`DELETE FROM contacts WHERE id=? AND NOT EXISTS (SELECT 1 FROM campaign_recipients WHERE contact_id=?)`, id, id)
		if err != nil {
			fail(c, err)
			return
		}
		affected, _ := res.RowsAffected()
		deleted += int(affected)
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

func (s *Server) importContacts(c *gin.Context) {
	if strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		s.importContactsJSON(c)
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		fail(c, err)
		return
	}
	f, err := file.Open()
	if err != nil {
		fail(c, err)
		return
	}
	defer f.Close()

	buf := bytes.Buffer{}
	if _, err := buf.ReadFrom(f); err != nil {
		fail(c, err)
		return
	}

	s.importContactsBytes(c, file.Filename, buf.Bytes())
}

func (s *Server) importContactsJSON(c *gin.Context) {
	var input importContactsInput
	if bind(c, &input) != nil {
		return
	}

	if strings.TrimSpace(input.Path) != "" {
		data, err := os.ReadFile(input.Path)
		if err != nil {
			fail(c, err)
			return
		}
		s.importContactsBytes(c, input.Path, data)
		return
	}

	data, err := base64.StdEncoding.DecodeString(input.Data)
	if err != nil {
		fail(c, errors.New("联系人文件内容无效"))
		return
	}
	s.importContactsBytes(c, input.Filename, data)
}

func (s *Server) importContactsBytes(c *gin.Context, filename string, data []byte) {
	result, err := ImportContactsData(s.db, filename, data)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func ImportContactsFromPath(conn *sql.DB, path string) (ContactImportResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ContactImportResult{}, err
	}
	return ImportContactsData(conn, path, data)
}

func ImportContactsData(conn *sql.DB, filename string, data []byte) (ContactImportResult, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	var contacts []models.Contact
	var err error
	switch ext {
	case ".xlsx":
		contacts, err = readContactsXLSX(data)
	case ".csv":
		contacts, err = readContactsCSV(data)
	default:
		err = errors.New("仅支持 .xlsx 或 .csv 联系人文件")
	}
	if err != nil {
		return ContactImportResult{}, err
	}

	imported, skipped := 0, 0
	for _, contact := range contacts {
		contact.Email = mailer.NormalizeEmailList(contact.Email)
		if err := mailer.ValidateEmailList(contact.Email, "邮箱"); err != nil {
			skipped++
			continue
		}
		if contact.Name == "" {
			contact.Name = contact.Company
		}
		if contact.Name == "" {
			contact.Name = contact.Email
		}
		res, err := conn.Exec(`INSERT OR IGNORE INTO contacts(name,email,company,department,phone,tags,notes) VALUES(?,?,?,?,?,?,?)`,
			contact.Name, contact.Email, contact.Company, contact.Department, contact.Phone, contact.Tags, contact.Notes)
		if err != nil {
			return ContactImportResult{}, err
		}
		affected, _ := res.RowsAffected()
		if affected > 0 {
			imported++
		} else {
			skipped++
		}
	}
	return ContactImportResult{Imported: imported, Skipped: skipped}, nil
}

func (s *Server) listTemplates(c *gin.Context) {
	rows, err := s.db.Query(`SELECT id,name,subject,body_html,created_at,updated_at FROM templates ORDER BY updated_at DESC,id DESC`)
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	items := []models.Template{}
	for rows.Next() {
		var item models.Template
		if err := rows.Scan(&item.ID, &item.Name, &item.Subject, &item.BodyHTML, &item.CreatedAt, &item.UpdatedAt); err != nil {
			fail(c, err)
			return
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, items)
}

func (s *Server) createTemplate(c *gin.Context) {
	var input models.Template
	if bind(c, &input) != nil {
		return
	}
	res, err := s.db.Exec(`INSERT INTO templates(name,subject,body_html) VALUES(?,?,?)`, input.Name, input.Subject, input.BodyHTML)
	if err != nil {
		fail(c, err)
		return
	}
	input.ID, _ = res.LastInsertId()
	c.JSON(http.StatusCreated, input)
}

func (s *Server) updateTemplate(c *gin.Context) {
	id := c.Param("id")
	var input models.Template
	if bind(c, &input) != nil {
		return
	}
	res, err := s.db.Exec(`UPDATE templates SET name=?,subject=?,body_html=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, input.Name, input.Subject, input.BodyHTML, id)
	if err != nil {
		fail(c, err)
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	input.ID, _ = strconv.ParseInt(id, 10, 64)
	c.JSON(http.StatusOK, input)
}

func (s *Server) deleteTemplate(c *gin.Context) {
	res, err := s.db.Exec(`DELETE FROM templates WHERE id=?`, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) copyTemplate(c *gin.Context) {
	var item models.Template
	if err := s.db.QueryRow(`SELECT name,subject,body_html FROM templates WHERE id=?`, c.Param("id")).Scan(&item.Name, &item.Subject, &item.BodyHTML); err != nil {
		fail(c, err)
		return
	}
	item.Name = item.Name + " 副本"
	res, err := s.db.Exec(`INSERT INTO templates(name,subject,body_html) VALUES(?,?,?)`, item.Name, item.Subject, item.BodyHTML)
	if err != nil {
		fail(c, err)
		return
	}
	item.ID, _ = res.LastInsertId()
	c.JSON(http.StatusCreated, item)
}

type previewTemplateInput struct {
	Subject  string         `json:"subject"`
	BodyHTML string         `json:"body_html"`
	Contact  models.Contact `json:"contact"`
}

func (s *Server) createTemplateTrackingImageAsset(c *gin.Context) {
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
	width := queryInt(c, "width", 0)
	if width == 0 {
		width, _ = strconv.Atoi(strings.TrimSpace(c.PostForm("width")))
	}
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
	payload, err := forwardAsset(s.trackingBaseURL(c), file, label, width, "image")
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, payload)
}

func (s *Server) previewTemplate(c *gin.Context) {
	var input previewTemplateInput
	if bind(c, &input) != nil {
		return
	}
	if input.Contact.Name == "" {
		input.Contact.Name = "上海示例企业有限公司"
	}
	if input.Contact.Company == "" {
		input.Contact.Company = input.Contact.Name
	}
	if input.Contact.Email == "" {
		input.Contact.Email = "contact@example.com"
	}
	if input.Contact.Phone == "" {
		input.Contact.Phone = "021-00000000"
	}
	body, err := mailer.RenderBody(input.BodyHTML, mailer.Personalization{
		BaseURL: s.trackingBaseURL(c),
		Contact: input.Contact,
		QRCode:  template.HTML(tracker.QRCodeHTML(s.trackingBaseURL(c), "preview")),
		TrackingImage: func(asset string) template.HTML {
			return template.HTML(tracker.TrackingImageHTML(s.trackingBaseURL(c), "preview", asset, "企业微信二维码", 176))
		},
		TrackingLink: func(_ string, targetURL string) template.URL {
			return template.URL(strings.TrimSpace(targetURL))
		},
	})
	if err != nil {
		fail(c, err)
		return
	}
	subject, err := mailer.RenderBody(input.Subject, mailer.Personalization{Contact: input.Contact})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"subject": subject, "body_html": body})
}

func (s *Server) trackingImage(c *gin.Context) {
	token := strings.TrimSpace(firstNonEmpty(c.Query("token"), c.Query("rid")))
	if token != "" && token != "preview" {
		raw, _ := json.Marshal(gin.H{
			"token":           token,
			"kind":            "image",
			"event_index":     trackingEventIndexFromQuery(c),
			"asset":           c.Query("asset"),
			"ip":              c.ClientIP(),
			"user_agent":      c.GetHeader("User-Agent"),
			"referer":         c.GetHeader("Referer"),
			"accept_language": c.GetHeader("Accept-Language"),
			"forwarded_for":   c.GetHeader("X-Forwarded-For"),
		})
		_ = tracker.NewSQLiteRecorder(s.db).RecordMark(tracker.MarkEvent{
			Token:          token,
			Kind:           "image",
			EventIndex:     trackingEventIndexFromQuery(c),
			Source:         "local",
			IP:             c.ClientIP(),
			UserAgent:      c.GetHeader("User-Agent"),
			Referer:        c.GetHeader("Referer"),
			AcceptLanguage: c.GetHeader("Accept-Language"),
			ForwardedFor:   c.GetHeader("X-Forwarded-For"),
			Raw:            string(raw),
		})
	}

	asset := strings.TrimSpace(c.Query("asset"))
	if asset != "" {
		name := filepath.Base(asset)
		if name != asset {
			c.Status(http.StatusNotFound)
			return
		}
		path := s.localDataPath("tracking-assets", name)
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
	if target == "" || !validHTTPURL(target) {
		target = s.qrcodeTargetURL(token)
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

type createCampaignInput struct {
	Name            string  `json:"name"`
	Subject         string  `json:"subject"`
	BodyHTML        string  `json:"body_html"`
	MailboxID       int64   `json:"mailbox_id"`
	TrackingEnabled bool    `json:"tracking_enabled"`
	ContactIDs      []int64 `json:"contact_ids"`
	AttachmentIDs   []int64 `json:"attachment_ids"`
}

type uploadCampaignAttachmentInput struct {
	OriginalName  string `json:"original_name"`
	ContentType   string `json:"content_type"`
	ContentBase64 string `json:"content_base64"`
	LinkBackup    bool   `json:"link_backup"`
}

func (s *Server) uploadCampaignAttachment(c *gin.Context) {
	if strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		var input uploadCampaignAttachmentInput
		if bind(c, &input) != nil {
			return
		}
		if len(input.ContentBase64) > ((20<<20)+2)/3*4+4 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "附件不能超过 20MB"})
			return
		}
		data, err := base64.StdEncoding.DecodeString(input.ContentBase64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "附件内容编码无效"})
			return
		}
		log.Printf("campaign attachment received as JSON: filename=%q size=%d content_type=%q", input.OriginalName, len(data), input.ContentType)
		s.saveCampaignAttachment(c, input.OriginalName, input.ContentType, data, input.LinkBackup)
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		log.Printf("campaign attachment rejected: form file missing: content_type=%q content_length=%d err=%v", c.GetHeader("Content-Type"), c.Request.ContentLength, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "请上传附件"})
		return
	}
	log.Printf("campaign attachment received: filename=%q size=%d content_type=%q request_length=%d", file.Filename, file.Size, file.Header.Get("Content-Type"), c.Request.ContentLength)
	originalName := filepath.Base(strings.TrimSpace(c.PostForm("original_name")))
	if originalName == "." || originalName == string(filepath.Separator) || strings.TrimSpace(originalName) == "" {
		originalName = filepath.Base(file.Filename)
	}
	if originalName == "." || originalName == string(filepath.Separator) || strings.TrimSpace(originalName) == "" {
		originalName = "attachment"
	}
	src, err := file.Open()
	if err != nil {
		fail(c, err)
		return
	}
	defer src.Close()
	data, err := io.ReadAll(io.LimitReader(src, (20<<20)+1))
	if err != nil {
		fail(c, err)
		return
	}
	contentType := firstNonEmpty(file.Header.Get("Content-Type"), mime.TypeByExtension(filepath.Ext(originalName)), "application/octet-stream")
	linkBackup := strings.EqualFold(c.PostForm("link_backup"), "true") || c.PostForm("link_backup") == "1"
	s.saveCampaignAttachment(c, originalName, contentType, data, linkBackup)
}

func (s *Server) saveCampaignAttachment(c *gin.Context, originalName, contentType string, data []byte, linkBackup bool) {
	originalName = filepath.Base(strings.TrimSpace(originalName))
	if originalName == "." || originalName == string(filepath.Separator) || originalName == "" {
		originalName = "attachment"
	}
	if len(data) == 0 {
		log.Printf("campaign attachment rejected: empty file: filename=%q", originalName)
		c.JSON(http.StatusBadRequest, gin.H{"error": "附件不能为空"})
		return
	}
	if len(data) > 20<<20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "附件不能超过 20MB"})
		return
	}
	ext := strings.ToLower(filepath.Ext(originalName))
	storedName := uuid.NewString() + ext
	dir := s.localDataPath("campaign-attachments")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(c, err)
		return
	}
	path := filepath.Join(dir, storedName)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fail(c, err)
		return
	}
	contentType = firstNonEmpty(contentType, mime.TypeByExtension(ext), "application/octet-stream")
	cloudAsset := ""
	cloudURL := ""
	if linkBackup {
		payload, err := forwardAssetReader(s.trackingBaseURL(c), bytes.NewReader(data), originalName, originalName, 0, "attachment")
		if err != nil {
			fail(c, err)
			return
		}
		cloudAsset, _ = payload["asset"].(string)
		cloudURL, _ = payload["download_url"].(string)
		if strings.TrimSpace(cloudURL) == "" {
			fail(c, errors.New("云端附件未返回下载链接"))
			return
		}
	}
	res, err := s.db.Exec(
		`INSERT INTO campaign_attachments(original_name,stored_name,content_type,size,link_backup,cloud_asset,cloud_url) VALUES(?,?,?,?,?,?,?)`,
		originalName,
		storedName,
		contentType,
		len(data),
		linkBackup,
		cloudAsset,
		cloudURL,
	)
	if err != nil {
		fail(c, err)
		return
	}
	id, _ := res.LastInsertId()
	c.JSON(http.StatusCreated, gin.H{
		"id":            id,
		"original_name": originalName,
		"content_type":  contentType,
		"size":          len(data),
		"link_backup":   linkBackup,
		"cloud_asset":   cloudAsset,
		"cloud_url":     cloudURL,
	})
}

func (s *Server) createCampaign(c *gin.Context) {
	var input createCampaignInput
	if bind(c, &input) != nil {
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		fail(c, err)
		return
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO campaigns(name,subject,body_html,mailbox_id,tracking_enabled,status) VALUES(?,?,?,?,?,'draft')`,
		input.Name, input.Subject, input.BodyHTML, input.MailboxID, true)
	if err != nil {
		fail(c, err)
		return
	}
	campaignID, _ := res.LastInsertId()
	for _, contactID := range input.ContactIDs {
		var ct models.Contact
		err := tx.QueryRow(`SELECT id,name,email FROM contacts WHERE id=?`, contactID).Scan(&ct.ID, &ct.Name, &ct.Email)
		if err != nil {
			continue
		}
		_, err = tx.Exec(`INSERT INTO campaign_recipients(campaign_id,contact_id,email,name,tracking_id) VALUES(?,?,?,?,?)`,
			campaignID, ct.ID, ct.Email, ct.Name, uuid.NewString())
		if err != nil {
			fail(c, err)
			return
		}
	}
	for _, attachmentID := range input.AttachmentIDs {
		if _, err := tx.Exec(`UPDATE campaign_attachments SET campaign_id=? WHERE id=? AND campaign_id IS NULL`, campaignID, attachmentID); err != nil {
			fail(c, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": campaignID})
}

func (s *Server) listCampaigns(c *gin.Context) {
	rows, err := s.db.Query(`SELECT id,name,subject,body_html,mailbox_id,status,tracking_enabled,created_at,COALESCE(sent_at,'') FROM campaigns ORDER BY id DESC`)
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	items := []models.Campaign{}
	for rows.Next() {
		var item models.Campaign
		if err := rows.Scan(&item.ID, &item.Name, &item.Subject, &item.BodyHTML, &item.MailboxID, &item.Status, &item.TrackingEnabled, &item.CreatedAt, &item.SentAt); err != nil {
			fail(c, err)
			return
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, items)
}

func (s *Server) getCampaign(c *gin.Context) {
	id := c.Param("id")
	var item models.Campaign
	err := s.db.QueryRow(`SELECT id,name,subject,body_html,mailbox_id,status,tracking_enabled,created_at,COALESCE(sent_at,'') FROM campaigns WHERE id=?`, id).
		Scan(&item.ID, &item.Name, &item.Subject, &item.BodyHTML, &item.MailboxID, &item.Status, &item.TrackingEnabled, &item.CreatedAt, &item.SentAt)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (s *Server) sendCampaign(c *gin.Context) {
	id := c.Param("id")
	campaignID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid campaign id"})
		return
	}
	var exists int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM campaigns WHERE id=?`, campaignID).Scan(&exists); err != nil {
		fail(c, err)
		return
	}
	if exists == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if _, loaded := s.sendingCampaigns.LoadOrStore(campaignID, struct{}{}); loaded {
		c.Status(http.StatusAccepted)
		return
	}
	baseURL := s.trackingBaseURL(c)
	sourceToken := s.trackingSourceToken()
	_, _ = s.db.Exec(`UPDATE campaigns SET status='sending' WHERE id=?`, campaignID)
	_, _ = s.db.Exec(`UPDATE campaign_recipients SET send_status='waiting',failure_reason='' WHERE campaign_id=? AND send_status IN ('pending','failed')`, campaignID)
	go s.runCampaignSend(campaignID, baseURL, sourceToken)
	c.Status(http.StatusAccepted)
}

func (s *Server) runCampaignSend(campaignID int64, baseURL, sourceToken string) {
	defer s.sendingCampaigns.Delete(campaignID)
	var campaign models.Campaign
	if err := s.db.QueryRow(`SELECT id,name,subject,body_html,mailbox_id,status,tracking_enabled,created_at,COALESCE(sent_at,'') FROM campaigns WHERE id=?`, campaignID).
		Scan(&campaign.ID, &campaign.Name, &campaign.Subject, &campaign.BodyHTML, &campaign.MailboxID, &campaign.Status, &campaign.TrackingEnabled, &campaign.CreatedAt, &campaign.SentAt); err != nil {
		return
	}
	var mb models.Mailbox
	if err := s.db.QueryRow(`SELECT id,name,host,port,username,password,from_email,from_name,use_tls FROM mailboxes WHERE id=?`, campaign.MailboxID).
		Scan(&mb.ID, &mb.Name, &mb.Host, &mb.Port, &mb.Username, &mb.Password, &mb.FromEmail, &mb.FromName, &mb.UseTLS); err != nil {
		_, _ = s.db.Exec(`UPDATE campaigns SET status='partial_failed' WHERE id=?`, campaignID)
		return
	}
	variants, err := s.prepareCampaignVariants(campaignID)
	if err != nil {
		_, _ = s.db.Exec(`UPDATE campaigns SET status='partial_failed' WHERE id=?`, campaignID)
		return
	}
	attachments, err := s.campaignAttachments(campaignID)
	if err != nil {
		_, _ = s.db.Exec(`UPDATE campaigns SET status='partial_failed' WHERE id=?`, campaignID)
		return
	}
	rows, err := s.db.Query(`SELECT cr.id,cr.contact_id,cr.email,cr.name,cr.tracking_id,COALESCE(cr.variant_id,0),c.company,c.department,c.phone,c.tags,c.notes FROM campaign_recipients cr JOIN contacts c ON c.id=cr.contact_id WHERE cr.campaign_id=? AND cr.send_status IN ('waiting','failed')`, campaignID)
	if err != nil {
		_, _ = s.db.Exec(`UPDATE campaigns SET status='partial_failed' WHERE id=?`, campaignID)
		return
	}
	type sendTarget struct {
		recipient models.Recipient
		contact   models.Contact
		variant   campaignVariant
	}
	targets := []sendTarget{}
	for rows.Next() {
		var rec models.Recipient
		var contact models.Contact
		var variantID int64
		if err := rows.Scan(&rec.ID, &rec.ContactID, &rec.Email, &rec.Name, &rec.TrackingID, &variantID, &contact.Company, &contact.Department, &contact.Phone, &contact.Tags, &contact.Notes); err != nil {
			_ = rows.Close()
			_, _ = s.db.Exec(`UPDATE campaigns SET status='partial_failed' WHERE id=?`, campaignID)
			return
		}
		contact.ID, contact.Name, contact.Email = rec.ContactID, rec.Name, rec.Email
		targets = append(targets, sendTarget{recipient: rec, contact: contact, variant: variants[variantID]})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		_, _ = s.db.Exec(`UPDATE campaigns SET status='partial_failed' WHERE id=?`, campaignID)
		return
	}
	if err := rows.Close(); err != nil {
		_, _ = s.db.Exec(`UPDATE campaigns SET status='partial_failed' WHERE id=?`, campaignID)
		return
	}
	failed := 0
	sendSettings := s.campaignSendSettings()
	var lastSendAt time.Time
	for _, target := range targets {
		rec := target.recipient
		contact := target.contact
		rec.Email = mailer.NormalizeEmailList(rec.Email)
		contact.Email = rec.Email
		if err := mailer.ValidateEmailList(rec.Email, "收件邮箱"); err != nil {
			failed++
			_, _ = s.db.Exec(`UPDATE campaign_recipients SET send_status='failed',failure_reason=? WHERE id=?`, err.Error(), rec.ID)
			continue
		}
		subjectTemplate := firstNonEmpty(target.variant.Subject, campaign.Subject)
		bodyTemplate := firstNonEmpty(target.variant.BodyHTML, campaign.BodyHTML)
		eventScope := campaignEventScope(campaign.ID, target.variant.ID)
		qrHTML := template.HTML("")
		var markToken string
		if templateUsesQRCode(bodyTemplate) || templateUsesTrackingImage(bodyTemplate) {
			markToken, err = s.ensureTrackingMark(rec.ID, "image", "联系图片", "")
			if err != nil {
				_, _ = s.db.Exec(`UPDATE campaign_recipients SET send_status='failed',failure_reason=? WHERE id=?`, err.Error(), rec.ID)
				failed++
				continue
			}
			qrHTML = template.HTML(tracker.QRCodeHTMLWithSourceAndIndex(baseURL, sourceToken, markToken, eventScope+":image:qr"))
		}
		data := mailer.Personalization{
			BaseURL:   baseURL,
			Contact:   contact,
			Recipient: rec,
			Campaign:  campaign,
			QRCode:    qrHTML,
			TrackingImage: func(asset string) template.HTML {
				return template.HTML(tracker.TrackingImageHTMLWithSourceAndIndex(baseURL, sourceToken, markToken, asset, "企业微信二维码", 176, eventScope+":image:"+asset))
			},
			TrackingLink: func(label, targetURL string) template.URL {
				linkLabel := strings.TrimSpace(label)
				if linkLabel == "" {
					linkLabel = "链接"
				}
				linkTarget := strings.TrimSpace(targetURL)
				if linkTarget == "" {
					linkTarget = s.qrcodeTargetURL(rec.TrackingID)
				}
				linkToken, err := s.ensureTrackingMarkForTarget(rec.ID, "click", linkLabel, linkTarget)
				if err != nil {
					return template.URL(linkTarget)
				}
				return template.URL(tracker.RedirectURLWithSourceAndIndex(baseURL, sourceToken, strconv.FormatInt(campaign.ID, 10), linkLabel, linkToken, linkTarget, eventScope+":click:"+linkLabel))
			},
		}
		subject, err := mailer.RenderBody(subjectTemplate, data)
		if err != nil {
			_, _ = s.db.Exec(`UPDATE campaign_recipients SET send_status='failed',failure_reason=? WHERE id=?`, err.Error(), rec.ID)
			failed++
			continue
		}
		body, err := mailer.RenderBody(bodyTemplate, data)
		if err == nil {
			body = tracker.InjectPixelWithSourceAndIndex(body, baseURL, sourceToken, rec.TrackingID, strconv.FormatInt(campaign.ID, 10), eventScope+":open")
			body = s.appendAttachmentDownloadLinks(body, campaign.ID, rec.ID, attachments, baseURL, sourceToken, eventScope)
		}
		if err == nil {
			waitForCampaignSendSlot(&lastSendAt, sendSettings)
			err = sendMailWithTimeout(mb, rec.Email, rec.Name, subject, body, attachments, s.localDataPath("campaign-attachments"), 20*time.Second)
		}
		if err != nil {
			failed++
			_, _ = s.db.Exec(`UPDATE campaign_recipients SET send_status='failed',failure_reason=? WHERE id=?`, err.Error(), rec.ID)
			continue
		}
		_, _ = s.db.Exec(`UPDATE campaign_recipients SET send_status='sent',failure_reason='',sent_at=CURRENT_TIMESTAMP WHERE id=?`, rec.ID)
	}
	status := "completed"
	if failed > 0 {
		status = "partial_failed"
	}
	_, _ = s.db.Exec(`UPDATE campaigns SET status=?,sent_at=COALESCE(sent_at,CURRENT_TIMESTAMP) WHERE id=?`, status, campaignID)
}

func (s *Server) campaignSendSettings() campaignSendSettings {
	return campaignSendSettings{
		RatePerMinute: clampInt(s.appSettingInt("campaign_send_rate_per_minute", defaultCampaignSendRatePerMinute), 1, 120),
		JitterPercent: clampInt(s.appSettingInt("campaign_send_jitter_percent", defaultCampaignSendJitterPercent), 0, 80),
	}
}

func (s *Server) appSettingInt(key string, fallback int) int {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM app_settings WHERE key=?`, key).Scan(&raw)
	if err != nil {
		return fallback
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return value
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func waitForCampaignSendSlot(lastSendAt *time.Time, settings campaignSendSettings) {
	now := time.Now()
	if delay := campaignSendDelay(*lastSendAt, now, settings, rand.Float64()); delay > 0 {
		time.Sleep(delay)
		now = time.Now()
	}
	*lastSendAt = now
}

func campaignSendDelay(lastSendAt, now time.Time, settings campaignSendSettings, randomValue float64) time.Duration {
	if lastSendAt.IsZero() {
		return 0
	}
	rate := clampInt(settings.RatePerMinute, 1, 120)
	jitterPercent := clampInt(settings.JitterPercent, 0, 80)
	baseInterval := time.Minute / time.Duration(rate)
	jitterRange := float64(baseInterval) * float64(jitterPercent) / 100
	jitter := time.Duration((clampFloat(randomValue, 0, 1)*2 - 1) * jitterRange)
	interval := baseInterval + jitter
	if interval < time.Second {
		interval = time.Second
	}
	nextSendAt := lastSendAt.Add(interval)
	if now.Before(nextSendAt) {
		return nextSendAt.Sub(now)
	}
	return 0
}

func clampFloat(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

type campaignVariant struct {
	ID       int64
	Name     string
	Subject  string
	BodyHTML string
	Weight   int
}

func (s *Server) prepareCampaignVariants(campaignID int64) (map[int64]campaignVariant, error) {
	rows, err := s.db.Query(`SELECT id,name,subject,body_html,weight FROM campaign_variants WHERE campaign_id=? ORDER BY id`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	variants := []campaignVariant{}
	byID := map[int64]campaignVariant{}
	for rows.Next() {
		var variant campaignVariant
		if err := rows.Scan(&variant.ID, &variant.Name, &variant.Subject, &variant.BodyHTML, &variant.Weight); err != nil {
			return nil, err
		}
		if variant.Weight <= 0 {
			variant.Weight = 1
		}
		variants = append(variants, variant)
		byID[variant.ID] = variant
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(variants) == 0 {
		return byID, nil
	}

	recRows, err := s.db.Query(`SELECT id FROM campaign_recipients WHERE campaign_id=? AND variant_id IS NULL AND send_status IN ('pending','waiting','failed') ORDER BY id`, campaignID)
	if err != nil {
		return nil, err
	}
	recipientIDs := []int64{}
	for recRows.Next() {
		var id int64
		if err := recRows.Scan(&id); err != nil {
			_ = recRows.Close()
			return nil, err
		}
		recipientIDs = append(recipientIDs, id)
	}
	if err := recRows.Err(); err != nil {
		_ = recRows.Close()
		return nil, err
	}
	if err := recRows.Close(); err != nil {
		return nil, err
	}
	for index, recipientID := range recipientIDs {
		variant := weightedVariant(variants, index)
		if _, err := s.db.Exec(`UPDATE campaign_recipients SET variant_id=? WHERE id=?`, variant.ID, recipientID); err != nil {
			return nil, err
		}
	}
	return byID, nil
}

func weightedVariant(variants []campaignVariant, index int) campaignVariant {
	total := 0
	for _, variant := range variants {
		total += variant.Weight
	}
	if total <= 0 {
		return variants[index%len(variants)]
	}
	slot := index % total
	for _, variant := range variants {
		if slot < variant.Weight {
			return variant
		}
		slot -= variant.Weight
	}
	return variants[len(variants)-1]
}

func (s *Server) campaignAttachments(campaignID int64) ([]models.CampaignAttachment, error) {
	rows, err := s.db.Query(`SELECT id,campaign_id,original_name,stored_name,content_type,size,link_backup,cloud_asset,cloud_url FROM campaign_attachments WHERE campaign_id=? ORDER BY id`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	attachments := []models.CampaignAttachment{}
	for rows.Next() {
		var attachment models.CampaignAttachment
		if err := rows.Scan(&attachment.ID, &attachment.CampaignID, &attachment.OriginalName, &attachment.StoredName, &attachment.ContentType, &attachment.Size, &attachment.LinkBackup, &attachment.CloudAsset, &attachment.CloudURL); err != nil {
			return nil, err
		}
		attachments = append(attachments, attachment)
	}
	return attachments, rows.Err()
}

func (s *Server) appendAttachmentDownloadLinks(body string, campaignID, recipientID int64, attachments []models.CampaignAttachment, baseURL, sourceToken, eventScope string) string {
	links := []string{}
	for _, attachment := range attachments {
		if !attachment.LinkBackup || strings.TrimSpace(attachment.CloudURL) == "" {
			continue
		}
		label := "附件下载：" + attachment.OriginalName
		token, err := s.ensureTrackingMarkForTarget(recipientID, "click", label, attachment.CloudURL)
		if err != nil {
			continue
		}
		href := tracker.RedirectURLWithSourceAndIndex(baseURL, sourceToken, strconv.FormatInt(campaignID, 10), label, token, attachment.CloudURL, eventScope+":attachment:"+attachment.OriginalName)
		links = append(links, `<li><a href="`+template.HTMLEscapeString(href)+`">`+template.HTMLEscapeString(attachment.OriginalName)+`</a></li>`)
	}
	if len(links) == 0 {
		return body
	}
	block := `<div style="margin-top:16px"><p>附件备用下载：</p><ul>` + strings.Join(links, "") + `</ul></div>`
	lower := strings.ToLower(body)
	if idx := strings.LastIndex(lower, "</body>"); idx >= 0 {
		return body[:idx] + block + body[idx:]
	}
	return body + block
}

func (s *Server) campaignStats(c *gin.Context) {
	s.syncCloudTrackingEventsBestEffort(c)
	id := c.Param("id")
	var stats struct {
		Total          int `json:"total"`
		Sent           int `json:"sent"`
		Failed         int `json:"failed"`
		Pending        int `json:"pending"`
		Waiting        int `json:"waiting"`
		Opened         int `json:"opened"`
		Unopened       int `json:"unopened"`
		QRLoaded       int `json:"qr_loaded"`
		QRLoadEvents   int `json:"qr_load_events"`
		QRNotLoaded    int `json:"qr_not_loaded"`
		PrefetchEvents int `json:"prefetch_events"`
		Clicked        int `json:"clicked"`
		ClickEvents    int `json:"click_events"`
	}
	_ = s.db.QueryRow(`
		SELECT
			COUNT(*),
			COALESCE(SUM(send_status='sent'),0),
			COALESCE(SUM(send_status='failed'),0),
			COALESCE(SUM(send_status='pending'),0),
			COALESCE(SUM(send_status='waiting'),0),
			COALESCE(SUM(open_count>0),0),
			COALESCE(SUM(open_count=0),0),
			COALESCE(SUM(qr_load_count>0),0),
			COALESCE(SUM(qr_load_count),0),
			COALESCE(SUM(qr_load_count=0),0),
			COALESCE(SUM(prefetch_count),0),
			COALESCE(SUM(click_count>0),0),
			COALESCE(SUM(click_count),0)
		FROM (
			SELECT
				cr.id,
				cr.send_status,
				cr.open_count,
				COUNT(tme.id) qr_load_count,
				COALESCE(SUM(tme.is_prefetch),0) prefetch_count,
				(
					SELECT COUNT(*)
					FROM tracking_mark_events click_events
					JOIN tracking_marks click_marks ON click_marks.id=click_events.mark_id
					WHERE click_marks.campaign_recipient_id=cr.id AND click_events.kind='click'
				) click_count
			FROM campaign_recipients cr
				LEFT JOIN tracking_marks tm ON tm.campaign_recipient_id=cr.id AND tm.kind='image'
				LEFT JOIN tracking_mark_events tme ON tme.mark_id=tm.id AND tme.kind='image'
			WHERE cr.campaign_id=?
			GROUP BY cr.id
		)`, id).
		Scan(&stats.Total, &stats.Sent, &stats.Failed, &stats.Pending, &stats.Waiting, &stats.Opened, &stats.Unopened, &stats.QRLoaded, &stats.QRLoadEvents, &stats.QRNotLoaded, &stats.PrefetchEvents, &stats.Clicked, &stats.ClickEvents)

	trend := s.hourlyTrend(`SELECT opened_at FROM open_events oe JOIN campaign_recipients cr ON cr.id=oe.campaign_recipient_id WHERE cr.campaign_id=?`, id)
	qrTrend := s.hourlyTrend(`SELECT tme.triggered_at FROM tracking_mark_events tme JOIN tracking_marks tm ON tm.id=tme.mark_id JOIN campaign_recipients cr ON cr.id=tm.campaign_recipient_id WHERE cr.campaign_id=? AND tme.kind='image'`, id)
	c.JSON(http.StatusOK, gin.H{"summary": stats, "trend": trend, "qr_trend": qrTrend})
}

func (s *Server) listRecipients(c *gin.Context) {
	s.syncCloudTrackingEventsBestEffort(c)
	rows, err := s.db.Query(`
		SELECT
			cr.id,
			cr.campaign_id,
			cr.contact_id,
			cr.email,
			cr.name,
			cr.tracking_id,
			cr.send_status,
			cr.failure_reason,
			COALESCE(cr.sent_at,''),
			COALESCE(cr.first_opened_at,''),
			COALESCE(cr.last_opened_at,''),
			cr.open_count,
			COUNT(tme.id) qr_load_count,
			COALESCE(MIN(tme.triggered_at),'') first_qr_load_at,
			COALESCE(MAX(tme.triggered_at),'') last_qr_load_at,
			COALESCE(
				(
					SELECT latest.ip
					FROM tracking_mark_events latest
						WHERE latest.mark_id=tm.id AND latest.kind='image'
					ORDER BY latest.triggered_at DESC, latest.id DESC
					LIMIT 1
				),
				''
			) last_qr_ip,
			COALESCE(
				(
					SELECT latest.user_agent
					FROM tracking_mark_events latest
						WHERE latest.mark_id=tm.id AND latest.kind='image'
					ORDER BY latest.triggered_at DESC, latest.id DESC
					LIMIT 1
				),
				''
			) last_qr_user_agent,
			COALESCE(
				(
					SELECT latest.forwarded_for
					FROM tracking_mark_events latest
						WHERE latest.mark_id=tm.id AND latest.kind='image'
					ORDER BY latest.triggered_at DESC, latest.id DESC
					LIMIT 1
				),
				''
			) last_qr_forwarded_for,
			COALESCE(
				(
					SELECT latest.source
					FROM tracking_mark_events latest
						WHERE latest.mark_id=tm.id AND latest.kind='image'
					ORDER BY latest.triggered_at DESC, latest.id DESC
					LIMIT 1
				),
				''
			) last_qr_source,
			COALESCE(
				(
					SELECT latest.referer
					FROM tracking_mark_events latest
						WHERE latest.mark_id=tm.id AND latest.kind='image'
					ORDER BY latest.triggered_at DESC, latest.id DESC
					LIMIT 1
				),
				''
			) last_qr_referer,
			COALESCE(
				(
					SELECT latest.accept_language
					FROM tracking_mark_events latest
						WHERE latest.mark_id=tm.id AND latest.kind='image'
					ORDER BY latest.triggered_at DESC, latest.id DESC
					LIMIT 1
				),
				''
			) last_qr_accept_language,
			COALESCE(
				(
					SELECT latest.is_prefetch
					FROM tracking_mark_events latest
						WHERE latest.mark_id=tm.id AND latest.kind='image'
					ORDER BY latest.triggered_at DESC, latest.id DESC
					LIMIT 1
				),
				0
			) last_qr_is_prefetch,
			COALESCE(
				(
					SELECT latest.ip_risk
					FROM tracking_mark_events latest
						WHERE latest.mark_id=tm.id AND latest.kind='image'
					ORDER BY latest.triggered_at DESC, latest.id DESC
					LIMIT 1
				),
				''
			) last_qr_ip_risk
		FROM campaign_recipients cr
		LEFT JOIN tracking_marks tm ON tm.campaign_recipient_id=cr.id AND tm.kind='image'
		LEFT JOIN tracking_mark_events tme ON tme.mark_id=tm.id AND tme.kind='image'
		WHERE cr.campaign_id=?
		GROUP BY cr.id
		ORDER BY cr.id DESC`, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	items := []models.Recipient{}
	for rows.Next() {
		var item models.Recipient
		if err := rows.Scan(&item.ID, &item.CampaignID, &item.ContactID, &item.Email, &item.Name, &item.TrackingID, &item.SendStatus, &item.FailureReason, &item.SentAt, &item.FirstOpenedAt, &item.LastOpenedAt, &item.OpenCount, &item.QRLoadCount, &item.FirstQRLoadAt, &item.LastQRLoadAt, &item.LastQRIP, &item.LastQRUA, &item.LastQRXFF, &item.LastQRSource, &item.LastQRReferer, &item.LastQRLang, &item.LastQRPrefetch, &item.LastQRIPRisk); err != nil {
			fail(c, err)
			return
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, items)
}

type cloudTrackingEventInput struct {
	ID             int64  `json:"id"`
	Source         string `json:"source"`
	Campaign       string `json:"campaign"`
	Link           string `json:"link"`
	EventIndex     string `json:"event_index"`
	Token          string `json:"token"`
	Kind           string `json:"kind"`
	TriggeredAt    string `json:"triggered_at"`
	IP             string `json:"ip"`
	UserAgent      string `json:"user_agent"`
	Referer        string `json:"referer"`
	AcceptLanguage string `json:"accept_language"`
	ForwardedFor   string `json:"forwarded_for"`
	IPRisk         string `json:"ip_risk"`
}

type importCloudTrackingEventsInput struct {
	Events []cloudTrackingEventInput `json:"events"`
}

func (s *Server) importCloudTrackingEvents(c *gin.Context) {
	var input importCloudTrackingEventsInput
	if bind(c, &input) != nil {
		return
	}
	imported := 0
	skipped := 0
	for _, event := range input.Events {
		if strings.TrimSpace(event.Token) == "" {
			continue
		}
		raw, _ := json.Marshal(event)
		err := s.recordCloudTrackingEvent(event, string(raw))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				skipped++
				continue
			}
			fail(c, err)
			return
		}
		imported++
	}
	c.JSON(http.StatusOK, gin.H{"imported": imported, "skipped": skipped})
}

func (s *Server) syncCloudTrackingEventsHandler(c *gin.Context) {
	result, err := s.syncCloudTrackingEvents(c.Request.Context(), s.trackingBaseURL(c), s.trackingSourceToken())
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

type cloudTrackingSyncResult struct {
	Fetched     int   `json:"fetched"`
	Imported    int   `json:"imported"`
	Skipped     int   `json:"skipped"`
	LastEventID int64 `json:"last_event_id"`
}

type cloudTrackingEventsResponse struct {
	Events []cloudTrackingEventInput `json:"events"`
}

func (s *Server) syncCloudTrackingEvents(ctx context.Context, baseURL, sourceToken string) (cloudTrackingSyncResult, error) {
	s.cloudSyncMu.Lock()
	defer s.cloudSyncMu.Unlock()

	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return cloudTrackingSyncResult{}, nil
	}

	source := baseURL + "|" + sourceToken
	var lastID int64
	err := s.db.QueryRow(`SELECT last_event_id FROM tracking_cloud_sync_state WHERE source=?`, source).Scan(&lastID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return cloudTrackingSyncResult{}, err
	}

	result := cloudTrackingSyncResult{LastEventID: lastID}
	const limit = 1000
	for page := 0; page < 10; page++ {
		endpoint, err := cloudEventsURL(baseURL, sourceToken, lastID, limit)
		if err != nil {
			return result, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return result, err
		}
		res, err := cloudHTTPClient.Do(req)
		if err != nil {
			return result, err
		}
		var payload cloudTrackingEventsResponse
		decodeErr := json.NewDecoder(res.Body).Decode(&payload)
		closeErr := res.Body.Close()
		if decodeErr != nil {
			return result, decodeErr
		}
		if closeErr != nil {
			return result, closeErr
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return result, errors.New(res.Status)
		}
		if len(payload.Events) == 0 {
			break
		}
		for _, event := range payload.Events {
			if event.ID > lastID {
				lastID = event.ID
			}
			result.Fetched++
			if strings.TrimSpace(event.Token) == "" {
				result.Skipped++
				continue
			}
			raw, _ := json.Marshal(event)
			if err := s.recordCloudTrackingEvent(event, string(raw)); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					result.Skipped++
					continue
				}
				return result, err
			}
			result.Imported++
		}
		result.LastEventID = lastID
		if len(payload.Events) < limit {
			break
		}
	}
	if result.LastEventID > 0 {
		_, err = s.db.Exec(`
			INSERT INTO tracking_cloud_sync_state(source,last_event_id,updated_at)
			VALUES(?,?,CURRENT_TIMESTAMP)
			ON CONFLICT(source) DO UPDATE SET last_event_id=excluded.last_event_id, updated_at=CURRENT_TIMESTAMP`,
			source,
			result.LastEventID,
		)
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func cloudEventsURL(baseURL, sourceToken string, afterID int64, limit int) (string, error) {
	endpoint, err := url.Parse(strings.TrimRight(baseURL, "/") + "/api/events")
	if err != nil {
		return "", err
	}
	values := endpoint.Query()
	if sourceToken != "" {
		values.Set("source", sourceToken)
	}
	if afterID > 0 {
		values.Set("after_id", strconv.FormatInt(afterID, 10))
	}
	values.Set("limit", strconv.Itoa(limit))
	endpoint.RawQuery = values.Encode()
	return endpoint.String(), nil
}

func (s *Server) syncCloudTrackingEventsBestEffort(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	_, _ = s.syncCloudTrackingEvents(ctx, s.trackingBaseURL(c), s.trackingSourceToken())
}

func (s *Server) exportCampaignCSV(c *gin.Context) {
	rows, err := s.db.Query(`SELECT c.name,cr.name,cr.email,ct.company,ct.department,ct.tags,cr.send_status,COALESCE(cr.sent_at,''),CASE WHEN cr.open_count>0 THEN '已阅读' ELSE '未阅读' END,COALESCE(cr.first_opened_at,''),COALESCE(cr.last_opened_at,''),cr.open_count,cr.failure_reason FROM campaign_recipients cr JOIN campaigns c ON c.id=cr.campaign_id JOIN contacts ct ON ct.id=cr.contact_id WHERE cr.campaign_id=? ORDER BY cr.id`, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="campaign-`+c.Param("id")+`.csv"`)
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"任务名称", "姓名", "邮箱", "公司", "部门", "标签", "发送状态", "发送时间", "阅读状态", "首次阅读时间", "最近阅读时间", "阅读次数", "失败原因"})
	for rows.Next() {
		record := make([]string, 13)
		ptrs := make([]any, len(record))
		for i := range record {
			ptrs[i] = &record[i]
		}
		_ = rows.Scan(ptrs...)
		_ = w.Write(record)
	}
	w.Flush()
}

func bind(c *gin.Context, v any) error {
	if err := c.ShouldBindJSON(v); err != nil {
		fail(c, err)
		return err
	}
	return nil
}

func fail(c *gin.Context, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

func cell(record []string, index int) string {
	if index < 0 || index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

func readContactsCSV(data []byte) ([]models.Contact, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	header := headerMap(records[0])
	start := 1
	if !hasKnownHeader(header) {
		start = 0
	}
	contacts := []models.Contact{}
	for _, record := range records[start:] {
		contacts = append(contacts, contactFromRow(func(name string, fallback int) string {
			if index, ok := header[name]; ok {
				return cell(record, index)
			}
			return cell(record, fallback)
		}))
	}
	return contacts, nil
}

func readContactsXLSX(data []byte) ([]models.Contact, error) {
	book, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer book.Close()
	sheet := book.GetSheetName(0)
	rows, err := book.GetRows(sheet)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	header := headerMap(rows[0])
	contacts := []models.Contact{}
	for _, row := range rows[1:] {
		contacts = append(contacts, contactFromRow(func(name string, fallback int) string {
			if index, ok := header[name]; ok {
				return cell(row, index)
			}
			return cell(row, fallback)
		}))
	}
	return contacts, nil
}

func contactFromRow(value func(name string, fallback int) string) models.Contact {
	company := firstNonEmpty(value("公司名", 0), value("公司", 0), value("企业名称", 0))
	email := firstNonEmpty(value("邮箱", -1), value("email", -1), value("Email", -1), value("", 1), value("", 4))
	phone := firstNonEmpty(value("联系电话", -1), value("手机号", -1), value("电话", -1), value("", 4), value("", 3))
	industry := value("行业", 8)
	size := value("规模", 9)
	tags := strings.Trim(strings.Join([]string{industry, size}, ","), ",")
	notes := compactNotes(map[string]string{
		"发票金额":  value("发票金额(元)", 1),
		"是否已收集": value("是否已收集", 2),
		"官网":    value("官网", 5),
		"数据来源":  value("数据来源", 6),
		"收集时间":  value("收集时间", 7),
	})
	return models.Contact{
		Name:    company,
		Email:   email,
		Company: company,
		Phone:   phone,
		Tags:    tags,
		Notes:   notes,
	}
}

func headerMap(record []string) map[string]int {
	header := map[string]int{}
	for index, value := range record {
		header[strings.TrimSpace(value)] = index
	}
	return header
}

func hasKnownHeader(header map[string]int) bool {
	_, hasEmailCN := header["邮箱"]
	_, hasEmailEN := header["email"]
	_, hasCompany := header["公司名"]
	return hasEmailCN || hasEmailEN || hasCompany
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func compactNotes(values map[string]string) string {
	parts := []string{}
	for key, value := range values {
		if strings.TrimSpace(value) != "" {
			parts = append(parts, key+"："+strings.TrimSpace(value))
		}
	}
	return strings.Join(parts, "；")
}

func sendMailWithTimeout(mb models.Mailbox, toEmail, toName, subject, body string, attachments []models.CampaignAttachment, attachmentDir string, timeout time.Duration) error {
	mailAttachments := make([]mailer.Attachment, 0, len(attachments))
	for _, attachment := range attachments {
		if strings.TrimSpace(attachment.StoredName) == "" {
			continue
		}
		mailAttachments = append(mailAttachments, mailer.Attachment{
			Path: filepath.Join(attachmentDir, filepath.Base(attachment.StoredName)),
			Name: attachment.OriginalName,
		})
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- mailer.Send(mb, toEmail, toName, subject, body, mailAttachments...)
	}()
	select {
	case err := <-errCh:
		return err
	case <-time.After(timeout):
		return errors.New("发送超时，请检查 SMTP 主机、端口、TLS 设置或网络连通性")
	}
}

func campaignEventScope(campaignID, variantID int64) string {
	if variantID > 0 {
		return "variant:" + strconv.FormatInt(variantID, 10)
	}
	return "campaign:" + strconv.FormatInt(campaignID, 10)
}

func closeRows(rows *sql.Rows) {
	if rows != nil {
		_ = rows.Close()
	}
}

func (s *Server) ensureTrackingMark(recipientID int64, kind, label, targetURL string) (string, error) {
	var token string
	err := s.db.QueryRow(`SELECT token FROM tracking_marks WHERE campaign_recipient_id=? AND kind=?`, recipientID, kind).Scan(&token)
	if err == nil {
		return token, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	token = uuid.NewString()
	_, err = s.db.Exec(
		`INSERT INTO tracking_marks(campaign_recipient_id,token,kind,label,target_url) VALUES(?,?,?,?,?)`,
		recipientID,
		token,
		kind,
		label,
		targetURL,
	)
	return token, err
}

func (s *Server) ensureTrackingMarkForTarget(recipientID int64, kind, label, targetURL string) (string, error) {
	var token string
	err := s.db.QueryRow(`SELECT token FROM tracking_marks WHERE campaign_recipient_id=? AND kind=? AND label=? AND target_url=?`, recipientID, kind, label, targetURL).Scan(&token)
	if err == nil {
		return token, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	token = uuid.NewString()
	_, err = s.db.Exec(
		`INSERT INTO tracking_marks(campaign_recipient_id,token,kind,label,target_url) VALUES(?,?,?,?,?)`,
		recipientID,
		token,
		kind,
		label,
		targetURL,
	)
	return token, err
}

func (s *Server) recordCloudTrackingEvent(event cloudTrackingEventInput, raw string) error {
	if event.Kind == "open" {
		return s.recordCloudOpenEvent(event, raw)
	}
	return s.recordCloudMarkEvent(event, raw)
}

func (s *Server) recordCloudOpenEvent(event cloudTrackingEventInput, _ string) error {
	var recipientID int64
	var sentAt string
	if err := s.db.QueryRow(`SELECT id,COALESCE(sent_at,'') FROM campaign_recipients WHERE tracking_id=?`, event.Token).Scan(&recipientID, &sentAt); err != nil {
		return err
	}
	triggeredAt := strings.TrimSpace(event.TriggeredAt)
	eventAt := time.Now()
	if parsed, ok := tracker.ParseEventTime(triggeredAt); ok {
		eventAt = parsed
	}
	isPrefetch, ipRisk := s.cloudEventPrefetchInsight(event, sentAt, eventAt)
	if triggeredAt == "" {
		_, err := s.db.Exec(
			`INSERT INTO open_events(campaign_recipient_id,tracking_id,event_index,ip,user_agent,is_prefetch,ip_risk) VALUES(?,?,?,?,?,?,?)`,
			recipientID,
			event.Token,
			event.EventIndex,
			event.IP,
			event.UserAgent,
			isPrefetch,
			ipRisk,
		)
		if err != nil {
			return err
		}
	} else {
		_, err := s.db.Exec(
			`INSERT INTO open_events(campaign_recipient_id,tracking_id,event_index,ip,user_agent,is_prefetch,ip_risk,opened_at) VALUES(?,?,?,?,?,?,?,?)`,
			recipientID,
			event.Token,
			event.EventIndex,
			event.IP,
			event.UserAgent,
			isPrefetch,
			ipRisk,
			triggeredAt,
		)
		if err != nil {
			return err
		}
	}
	_, err := s.db.Exec(
		`UPDATE campaign_recipients SET open_count=open_count+1, first_opened_at=COALESCE(first_opened_at,?), last_opened_at=COALESCE(NULLIF(?,''),CURRENT_TIMESTAMP) WHERE id=?`,
		firstNonEmpty(triggeredAt, time.Now().Format(time.RFC3339)),
		triggeredAt,
		recipientID,
	)
	return err
}

func (s *Server) recordCloudMarkEvent(event cloudTrackingEventInput, raw string) error {
	var markID int64
	var kind string
	var sentAt string
	if err := s.db.QueryRow(`
		SELECT tm.id,tm.kind,COALESCE(cr.sent_at,'')
		FROM tracking_marks tm
		JOIN campaign_recipients cr ON cr.id=tm.campaign_recipient_id
		WHERE tm.token=?`, event.Token).Scan(&markID, &kind, &sentAt); err != nil {
		return err
	}
	if event.Kind != "" {
		kind = event.Kind
	}
	if kind == "" {
		kind = "unknown"
	}
	triggeredAt := strings.TrimSpace(event.TriggeredAt)
	eventAt := time.Now()
	if parsed, ok := tracker.ParseEventTime(triggeredAt); ok {
		eventAt = parsed
	}
	isPrefetch, ipRisk := s.cloudEventPrefetchInsight(event, sentAt, eventAt)
	if triggeredAt == "" {
		_, err := s.db.Exec(
			`INSERT INTO tracking_mark_events(mark_id,token,kind,event_index,source,ip,user_agent,referer,accept_language,forwarded_for,is_prefetch,ip_risk,raw_payload) VALUES(NULLIF(?,0),?,?,?,?,?,?,?,?,?,?,?,?)`,
			markID,
			event.Token,
			kind,
			event.EventIndex,
			"cloud",
			event.IP,
			event.UserAgent,
			event.Referer,
			event.AcceptLanguage,
			event.ForwardedFor,
			isPrefetch,
			ipRisk,
			raw,
		)
		return err
	}
	_, err := s.db.Exec(
		`INSERT INTO tracking_mark_events(mark_id,token,kind,event_index,source,ip,user_agent,referer,accept_language,forwarded_for,is_prefetch,ip_risk,raw_payload,triggered_at) VALUES(NULLIF(?,0),?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		markID,
		event.Token,
		kind,
		event.EventIndex,
		"cloud",
		event.IP,
		event.UserAgent,
		event.Referer,
		event.AcceptLanguage,
		event.ForwardedFor,
		isPrefetch,
		ipRisk,
		raw,
		triggeredAt,
	)
	return err
}

func (s *Server) cloudEventPrefetchInsight(event cloudTrackingEventInput, sentAt string, eventAt time.Time) (bool, string) {
	isPrefetch := tracker.LooksLikePrefetch(event.UserAgent) || tracker.LooksLikeDeliverySecurityScan(sentAt, eventAt)
	ipRisk := strings.TrimSpace(event.IPRisk)
	if ipRisk != "" && containsAnyRiskText([]string{ipRisk}, "机房", "idc", "数据中心", "云主机", "云服务", "cdn", "代理", "vpn", "服务器", "托管", "劫持", "非正常设备", "黑rom") {
		isPrefetch = true
	}
	return isPrefetch, ipRisk
}

func containsAnyRiskText(values []string, needles ...string) bool {
	for _, value := range values {
		lower := strings.ToLower(value)
		for _, needle := range needles {
			if strings.Contains(lower, strings.ToLower(needle)) {
				return true
			}
		}
	}
	return false
}

func templateUsesQRCode(body string) bool {
	return strings.Contains(body, ".QRCode")
}

func templateUsesTrackingImage(body string) bool {
	return strings.Contains(body, "TrackingImage")
}

func serveEmbedded(c *gin.Context, dist fs.FS, path string) {
	data, err := fs.ReadFile(dist, path)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = "text/html; charset=utf-8"
	}
	c.Data(http.StatusOK, contentType, data)
}

func (s *Server) trackingBaseURL(c *gin.Context) string {
	if value := strings.TrimSpace(os.Getenv("TRACKING_BASE_URL")); value != "" {
		return value
	}
	if value := strings.TrimSpace(c.GetHeader("X-Tracking-Base-URL")); value != "" {
		return value
	}
	if value := strings.TrimSpace(s.cfg.Client.TrackingBaseURL); value != "" {
		return value
	}
	return config.Default().Client.TrackingBaseURL
}

func (s *Server) trackingSourceToken() string {
	if value := strings.TrimSpace(os.Getenv("TRACKING_SOURCE_TOKEN")); value != "" {
		return value
	}
	if value := strings.TrimSpace(s.cfg.Client.TrackingSourceToken); value != "" {
		return value
	}
	var token string
	err := s.db.QueryRow(`SELECT value FROM app_settings WHERE key='tracking_source_token'`).Scan(&token)
	if err == nil && strings.TrimSpace(token) != "" {
		return token
	}
	token = uuid.NewString()
	_, err = s.db.Exec(`
		INSERT INTO app_settings(key,value,updated_at)
		VALUES('tracking_source_token',?,CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value=app_settings.value`,
		token,
	)
	if err != nil {
		return ""
	}
	return token
}

func appBaseURL(c *gin.Context) string {
	if value := strings.TrimSpace(c.GetHeader("X-Base-URL")); value != "" {
		return value
	}
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
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

func trackingEventIndexFromQuery(c *gin.Context) string {
	return firstNonEmpty(c.Query("i"), c.Query("idx"), c.Query("index"), c.Query("event_index"), c.Query("l"), c.Query("link"), c.Query("asset"))
}

func (s *Server) qrcodeTargetURL(token string) string {
	if value := strings.TrimSpace(os.Getenv("QR_CODE_TARGET_URL")); value != "" {
		separator := "?"
		if strings.Contains(value, "?") {
			separator = "&"
		}
		return value + separator + "t=" + token
	}
	value := strings.TrimSpace(s.cfg.Client.QRCodeTargetURL)
	if value == "" {
		value = "https://example.com/survey"
	}
	separator := "?"
	if strings.Contains(value, "?") {
		separator = "&"
	}
	return value + separator + "t=" + token
}

func validHTTPURL(value string) bool {
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
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

func forwardAsset(baseURL string, file *multipart.FileHeader, label string, width int, kind string) (gin.H, error) {
	src, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	return forwardAssetReader(baseURL, src, file.Filename, label, width, kind)
}

func forwardAssetReader(baseURL string, src io.Reader, filename, label string, width int, kind string) (gin.H, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, src); err != nil {
		return nil, err
	}
	if err := writer.WriteField("label", label); err != nil {
		return nil, err
	}
	if err := writer.WriteField("width", strconv.Itoa(width)); err != nil {
		return nil, err
	}
	if err := writer.WriteField("kind", kind); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/api/assets"
	req, err := http.NewRequest(http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var payload gin.H
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		if message, ok := payload["error"].(string); ok && message != "" {
			return nil, errors.New(message)
		}
		return nil, errors.New(res.Status)
	}
	return payload, nil
}

// globalStats returns global tracking statistics
func (s *Server) globalStats(c *gin.Context) {
	s.syncCloudTrackingEventsBestEffort(c)
	since := c.Query("since")
	campaignID := c.Query("campaign")

	var stats struct {
		TotalCampaigns int `json:"total_campaigns"`
		TotalSent      int `json:"total_sent"`
		TotalOpened    int `json:"total_opened"`
		TotalClicked   int `json:"total_clicked"`
		TotalQRLoaded  int `json:"total_qr_loaded"`
	}
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM campaigns`).Scan(&stats.TotalCampaigns)
	where, args := recipientStatsWhere("cr", since, campaignID, "cr.send_status='sent'")
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM campaign_recipients cr `+where, args...).Scan(&stats.TotalSent)
	where, args = recipientStatsWhere("cr", since, campaignID, "cr.open_count>0")
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM campaign_recipients cr `+where, args...).Scan(&stats.TotalOpened)
	where, args = recipientStatsWhere("cr", since, campaignID, "tme.kind='click'")
	_ = s.db.QueryRow(`
		SELECT COUNT(DISTINCT tme.mark_id)
		FROM tracking_mark_events tme
		JOIN tracking_marks tm ON tm.id=tme.mark_id
		JOIN campaign_recipients cr ON cr.id=tm.campaign_recipient_id `+where, args...).Scan(&stats.TotalClicked)
	where, args = recipientStatsWhere("cr", since, campaignID, "tme.kind='image'", "tme.is_prefetch=0")
	_ = s.db.QueryRow(`
		SELECT COUNT(DISTINCT tm.id)
		FROM tracking_mark_events tme
		JOIN tracking_marks tm ON tm.id=tme.mark_id
		JOIN campaign_recipients cr ON cr.id=tm.campaign_recipient_id `+where, args...).Scan(&stats.TotalQRLoaded)

	// Get daily trend for last 30 days
	rows, _ := s.db.Query(`
		SELECT DATE(cr.sent_at) date,
			COUNT(*) sent,
			COALESCE(SUM(cr.open_count>0), 0) opened,
			COALESCE((SELECT COUNT(*) FROM tracking_mark_events tme JOIN tracking_marks tm ON tm.id=tme.mark_id JOIN campaign_recipients cr2 ON cr2.id=tm.campaign_recipient_id WHERE DATE(cr2.sent_at)=DATE(cr.sent_at) AND tme.kind='click'), 0) clicked
		FROM campaign_recipients cr
		WHERE cr.send_status='sent' AND cr.sent_at >= DATE('now', '-30 days')
		GROUP BY DATE(cr.sent_at)
		ORDER BY date
	`)
	defer closeRows(rows)
	trend := []gin.H{}
	if rows != nil {
		for rows.Next() {
			var date string
			var sent, opened, clicked int
			_ = rows.Scan(&date, &sent, &opened, &clicked)
			trend = append(trend, gin.H{"date": date, "sent": sent, "opened": opened, "clicked": clicked})
		}
	}

	c.JSON(http.StatusOK, gin.H{"summary": stats, "trend": trend})
}

func recipientStatsWhere(alias, since, campaignID string, extra ...string) (string, []any) {
	conditions := []string{}
	args := []any{}
	if since != "" {
		conditions = append(conditions, alias+".sent_at >= ?")
		args = append(args, since)
	}
	if campaignID != "" {
		conditions = append(conditions, alias+".campaign_id = ?")
		args = append(args, campaignID)
	}
	conditions = append(conditions, extra...)
	if len(conditions) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

// createVariant creates a new campaign variant for AB testing
func (s *Server) createVariant(c *gin.Context) {
	campaignID := c.Param("id")
	var input struct {
		Name     string `json:"name"`
		Subject  string `json:"subject"`
		BodyHTML string `json:"body_html"`
		Weight   int    `json:"weight"`
	}
	if bind(c, &input) != nil {
		return
	}
	if input.Weight == 0 {
		input.Weight = 50
	}
	res, err := s.db.Exec(`INSERT INTO campaign_variants(campaign_id,name,subject,body_html,weight) VALUES(?,?,?,?,?)`,
		campaignID, input.Name, input.Subject, input.BodyHTML, input.Weight)
	if err != nil {
		fail(c, err)
		return
	}
	id, _ := res.LastInsertId()
	c.JSON(http.StatusCreated, gin.H{"id": id, "name": input.Name, "subject": input.Subject, "body_html": input.BodyHTML, "weight": input.Weight})
}

// updateVariant updates a campaign variant
func (s *Server) updateVariant(c *gin.Context) {
	campaignID := c.Param("id")
	variantID := c.Param("vid")
	var input struct {
		Name     string `json:"name"`
		Subject  string `json:"subject"`
		BodyHTML string `json:"body_html"`
		Weight   int    `json:"weight"`
	}
	if bind(c, &input) != nil {
		return
	}
	_, err := s.db.Exec(`UPDATE campaign_variants SET name=?, subject=?, body_html=?, weight=? WHERE id=? AND campaign_id=?`,
		input.Name, input.Subject, input.BodyHTML, input.Weight, variantID, campaignID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"updated": true})
}

// deleteVariant deletes a campaign variant
func (s *Server) deleteVariant(c *gin.Context) {
	campaignID := c.Param("id")
	variantID := c.Param("vid")
	_, err := s.db.Exec(`DELETE FROM campaign_variants WHERE id=? AND campaign_id=?`, variantID, campaignID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// abStats returns AB testing statistics
func (s *Server) abStats(c *gin.Context) {
	s.syncCloudTrackingEventsBestEffort(c)
	campaignID := c.Param("id")

	// Get variants
	rows, err := s.db.Query(`SELECT id,name,subject,body_html,weight FROM campaign_variants WHERE campaign_id=? ORDER BY id`, campaignID)
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	variants := []gin.H{}
	for rows.Next() {
		var id int
		var name, subject, bodyHTML string
		var weight int
		_ = rows.Scan(&id, &name, &subject, &bodyHTML, &weight)
		variants = append(variants, gin.H{"id": id, "name": name, "subject": subject, "body_html": bodyHTML, "weight": weight})
	}

	// Get stats for each variant
	results := []gin.H{}
	for _, v := range variants {
		vid := v["id"].(int)
		var stats struct {
			Sent     int `json:"sent"`
			Opened   int `json:"opened"`
			Clicked  int `json:"clicked"`
			QRLoaded int `json:"qr_loaded"`
		}
		_ = s.db.QueryRow(`
			SELECT
				COALESCE(SUM(rg.send_status='sent'), 0),
				COALESCE(SUM(rg.open_count>0), 0),
				COALESCE((SELECT COUNT(*) FROM tracking_mark_events tme JOIN tracking_marks tm ON tm.id=tme.mark_id WHERE tm.campaign_recipient_id=rg.id AND tme.kind='click'), 0),
					COALESCE((SELECT COUNT(*) FROM tracking_mark_events tme JOIN tracking_marks tm ON tm.id=tme.mark_id WHERE tm.campaign_recipient_id=rg.id AND tme.kind='image' AND tme.is_prefetch=0), 0)
			FROM campaign_recipients rg WHERE rg.campaign_id=? AND rg.variant_id=?`, campaignID, vid).
			Scan(&stats.Sent, &stats.Opened, &stats.Clicked, &stats.QRLoaded)
		results = append(results, gin.H{
			"variant":    v,
			"stats":      stats,
			"open_rate":  safeRate(stats.Opened, stats.Sent),
			"click_rate": safeRate(stats.Clicked, stats.Sent),
		})
	}

	c.JSON(http.StatusOK, gin.H{"variants": results})
}

// listLinks lists all tracked links in a campaign
func (s *Server) listLinks(c *gin.Context) {
	campaignID := c.Param("id")
	rows, err := s.db.Query(`
		SELECT DISTINCT tm.id, tm.label, tm.target_url
		FROM tracking_marks tm
		JOIN campaign_recipients cr ON cr.id=tm.campaign_recipient_id
		WHERE cr.campaign_id=? AND tm.kind='click'
		ORDER BY tm.label`, campaignID)
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	links := []gin.H{}
	for rows.Next() {
		var id int
		var label, targetURL string
		_ = rows.Scan(&id, &label, &targetURL)
		links = append(links, gin.H{"id": id, "label": label, "target_url": targetURL})
	}
	c.JSON(http.StatusOK, links)
}

// linkStats returns statistics for a specific link
func (s *Server) linkStats(c *gin.Context) {
	s.syncCloudTrackingEventsBestEffort(c)
	campaignID := c.Param("id")
	linkID := c.Param("lid")

	var stats struct {
		TotalClicks  int `json:"total_clicks"`
		UniqueClicks int `json:"unique_clicks"`
	}
	_ = s.db.QueryRow(`
		SELECT
			COUNT(*),
			COUNT(DISTINCT tme.ip || tme.user_agent)
		FROM tracking_mark_events tme
		JOIN tracking_marks tm ON tm.id=tme.mark_id
		JOIN campaign_recipients cr ON cr.id=tm.campaign_recipient_id
		WHERE cr.campaign_id=? AND tm.id=? AND tme.kind='click'`, campaignID, linkID).
		Scan(&stats.TotalClicks, &stats.UniqueClicks)

	trend := s.hourlyTrend(`
		SELECT tme.triggered_at
		FROM tracking_mark_events tme
		JOIN tracking_marks tm ON tm.id=tme.mark_id
		JOIN campaign_recipients cr ON cr.id=tm.campaign_recipient_id
		WHERE cr.campaign_id=? AND tm.id=? AND tme.kind='click'`, campaignID, linkID)

	c.JSON(http.StatusOK, gin.H{"summary": stats, "trend": trend})
}

func (s *Server) hourlyTrend(query string, args ...any) []gin.H {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return []gin.H{}
	}
	defer rows.Close()
	counts := map[time.Time]int{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			continue
		}
		eventAt, ok := tracker.ParseEventTime(value)
		if !ok {
			continue
		}
		hour := eventAt.Local().Truncate(time.Hour)
		counts[hour]++
	}
	hours := make([]time.Time, 0, len(counts))
	for hour := range counts {
		hours = append(hours, hour)
	}
	sort.Slice(hours, func(i, j int) bool { return hours[i].Before(hours[j]) })
	trend := make([]gin.H, 0, len(hours))
	for _, hour := range hours {
		trend = append(trend, gin.H{
			"hour":  hour.Format(time.RFC3339),
			"count": counts[hour],
		})
	}
	return trend
}

func safeRate(count, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(count) / float64(total) * 100
}
