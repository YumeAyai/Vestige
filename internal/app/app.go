package app

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nousmail/internal/mailer"
	"nousmail/internal/models"
	"nousmail/internal/tracker"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

type Server struct {
	db       *sql.DB
	frontend fs.FS
	tracker  tracker.SQLiteRecorder
}

func New(db *sql.DB, frontend fs.FS) *gin.Engine {
	s := &Server{db: db, frontend: frontend, tracker: tracker.NewSQLiteRecorder(db)}
	r := gin.Default()

	api := r.Group("/api")
	api.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	api.GET("/mailboxes", s.listMailboxes)
	api.POST("/mailboxes", s.createMailbox)
	api.DELETE("/mailboxes/:id", s.deleteMailbox)
	api.GET("/contacts", s.listContacts)
	api.POST("/contacts", s.createContact)
	api.POST("/contacts/import", s.importContacts)
	api.GET("/templates", s.listTemplates)
	api.POST("/templates", s.createTemplate)
	api.POST("/templates/preview", s.previewTemplate)
	api.GET("/campaigns", s.listCampaigns)
	api.POST("/campaigns", s.createCampaign)
	api.GET("/campaigns/:id", s.getCampaign)
	api.POST("/campaigns/:id/send", s.sendCampaign)
	api.GET("/campaigns/:id/stats", s.campaignStats)
	api.GET("/campaigns/:id/recipients", s.listRecipients)
	api.GET("/campaigns/:id/export.csv", s.exportCampaignCSV)
	api.GET("/track/open.gif", s.trackOpen)

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

func (s *Server) createContact(c *gin.Context) {
	var input models.Contact
	if bind(c, &input) != nil {
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

func (s *Server) importContacts(c *gin.Context) {
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

	ext := strings.ToLower(filepath.Ext(file.Filename))
	var contacts []models.Contact
	switch ext {
	case ".xlsx":
		contacts, err = readContactsXLSX(buf.Bytes())
	case ".csv":
		contacts, err = readContactsCSV(buf.Bytes())
	default:
		err = errors.New("仅支持 .xlsx 或 .csv 联系人文件")
	}
	if err != nil {
		fail(c, err)
		return
	}

	imported, skipped := 0, 0
	for _, contact := range contacts {
		if contact.Email == "" {
			skipped++
			continue
		}
		if contact.Name == "" {
			contact.Name = contact.Company
		}
		if contact.Name == "" {
			contact.Name = contact.Email
		}
		res, err := s.db.Exec(`INSERT OR IGNORE INTO contacts(name,email,company,department,phone,tags,notes) VALUES(?,?,?,?,?,?,?)`,
			contact.Name, contact.Email, contact.Company, contact.Department, contact.Phone, contact.Tags, contact.Notes)
		if err != nil {
			fail(c, err)
			return
		}
		affected, _ := res.RowsAffected()
		if affected > 0 {
			imported++
		} else {
			skipped++
		}
	}
	c.JSON(http.StatusOK, gin.H{"imported": imported, "skipped": skipped})
}

func (s *Server) listTemplates(c *gin.Context) {
	rows, err := s.db.Query(`SELECT id,name,subject,body_html FROM templates ORDER BY id DESC`)
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	items := []models.Template{}
	for rows.Next() {
		var item models.Template
		if err := rows.Scan(&item.ID, &item.Name, &item.Subject, &item.BodyHTML); err != nil {
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

type previewTemplateInput struct {
	Subject  string         `json:"subject"`
	BodyHTML string         `json:"body_html"`
	Contact  models.Contact `json:"contact"`
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
	body, err := mailer.RenderBody(input.BodyHTML, mailer.Personalization{Contact: input.Contact})
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

type createCampaignInput struct {
	Name            string  `json:"name"`
	Subject         string  `json:"subject"`
	BodyHTML        string  `json:"body_html"`
	MailboxID       int64   `json:"mailbox_id"`
	TrackingEnabled bool    `json:"tracking_enabled"`
	ContactIDs      []int64 `json:"contact_ids"`
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
		input.Name, input.Subject, input.BodyHTML, input.MailboxID, input.TrackingEnabled)
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
	var campaign models.Campaign
	if err := s.db.QueryRow(`SELECT id,name,subject,body_html,mailbox_id,status,tracking_enabled,created_at,COALESCE(sent_at,'') FROM campaigns WHERE id=?`, id).
		Scan(&campaign.ID, &campaign.Name, &campaign.Subject, &campaign.BodyHTML, &campaign.MailboxID, &campaign.Status, &campaign.TrackingEnabled, &campaign.CreatedAt, &campaign.SentAt); err != nil {
		fail(c, err)
		return
	}
	var mb models.Mailbox
	if err := s.db.QueryRow(`SELECT id,name,host,port,username,password,from_email,from_name,use_tls FROM mailboxes WHERE id=?`, campaign.MailboxID).
		Scan(&mb.ID, &mb.Name, &mb.Host, &mb.Port, &mb.Username, &mb.Password, &mb.FromEmail, &mb.FromName, &mb.UseTLS); err != nil {
		fail(c, err)
		return
	}
	baseURL := trackingBaseURL(c)
	rows, err := s.db.Query(`SELECT cr.id,cr.contact_id,cr.email,cr.name,cr.tracking_id,c.company,c.department,c.phone,c.tags,c.notes FROM campaign_recipients cr JOIN contacts c ON c.id=cr.contact_id WHERE cr.campaign_id=? AND cr.send_status IN ('pending','failed')`, id)
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	sent, failed := 0, 0
	for rows.Next() {
		var rec models.Recipient
		var contact models.Contact
		if err := rows.Scan(&rec.ID, &rec.ContactID, &rec.Email, &rec.Name, &rec.TrackingID, &contact.Company, &contact.Department, &contact.Phone, &contact.Tags, &contact.Notes); err != nil {
			fail(c, err)
			return
		}
		contact.ID, contact.Name, contact.Email = rec.ContactID, rec.Name, rec.Email
		data := mailer.Personalization{BaseURL: baseURL, Contact: contact, Recipient: rec, Campaign: campaign}
		subject, err := mailer.RenderBody(campaign.Subject, data)
		if err != nil {
			_, _ = s.db.Exec(`UPDATE campaign_recipients SET send_status='failed',failure_reason=? WHERE id=?`, err.Error(), rec.ID)
			failed++
			continue
		}
		body, err := mailer.RenderBody(campaign.BodyHTML, data)
		if err == nil && campaign.TrackingEnabled {
			body = tracker.InjectPixel(body, baseURL, rec.TrackingID)
		}
		if err == nil {
			err = mailer.Send(mb, rec.Email, rec.Name, subject, body)
		}
		if err != nil {
			failed++
			_, _ = s.db.Exec(`UPDATE campaign_recipients SET send_status='failed',failure_reason=? WHERE id=?`, err.Error(), rec.ID)
			continue
		}
		sent++
		_, _ = s.db.Exec(`UPDATE campaign_recipients SET send_status='sent',failure_reason='',sent_at=CURRENT_TIMESTAMP WHERE id=?`, rec.ID)
		time.Sleep(300 * time.Millisecond)
	}
	status := "completed"
	if failed > 0 {
		status = "partial_failed"
	}
	_, _ = s.db.Exec(`UPDATE campaigns SET status=?,sent_at=COALESCE(sent_at,CURRENT_TIMESTAMP) WHERE id=?`, status, id)
	c.JSON(http.StatusOK, gin.H{"sent": sent, "failed": failed})
}

func (s *Server) campaignStats(c *gin.Context) {
	id := c.Param("id")
	var stats struct {
		Total    int `json:"total"`
		Sent     int `json:"sent"`
		Failed   int `json:"failed"`
		Opened   int `json:"opened"`
		Unopened int `json:"unopened"`
	}
	_ = s.db.QueryRow(`SELECT COUNT(*), SUM(send_status='sent'), SUM(send_status='failed'), SUM(open_count>0), SUM(open_count=0) FROM campaign_recipients WHERE campaign_id=?`, id).
		Scan(&stats.Total, &stats.Sent, &stats.Failed, &stats.Opened, &stats.Unopened)

	rows, _ := s.db.Query(`SELECT strftime('%Y-%m-%d %H:00', opened_at) hour, COUNT(*) FROM open_events oe JOIN campaign_recipients cr ON cr.id=oe.campaign_recipient_id WHERE cr.campaign_id=? GROUP BY hour ORDER BY hour`, id)
	defer closeRows(rows)
	trend := []gin.H{}
	if rows != nil {
		for rows.Next() {
			var hour string
			var count int
			_ = rows.Scan(&hour, &count)
			trend = append(trend, gin.H{"hour": hour, "count": count})
		}
	}
	c.JSON(http.StatusOK, gin.H{"summary": stats, "trend": trend})
}

func (s *Server) listRecipients(c *gin.Context) {
	rows, err := s.db.Query(`SELECT id,campaign_id,contact_id,email,name,tracking_id,send_status,failure_reason,COALESCE(sent_at,''),COALESCE(first_opened_at,''),COALESCE(last_opened_at,''),open_count FROM campaign_recipients WHERE campaign_id=? ORDER BY id DESC`, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	items := []models.Recipient{}
	for rows.Next() {
		var item models.Recipient
		if err := rows.Scan(&item.ID, &item.CampaignID, &item.ContactID, &item.Email, &item.Name, &item.TrackingID, &item.SendStatus, &item.FailureReason, &item.SentAt, &item.FirstOpenedAt, &item.LastOpenedAt, &item.OpenCount); err != nil {
			fail(c, err)
			return
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, items)
}

func (s *Server) trackOpen(c *gin.Context) {
	tid := c.Query("tid")
	if tid != "" {
		_ = s.tracker.RecordOpen(tracker.Event{
			TrackingID: tid,
			IP:         c.ClientIP(),
			UserAgent:  c.GetHeader("User-Agent"),
		})
	}
	c.Header("Content-Type", "image/gif")
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Data(http.StatusOK, "image/gif", tracker.PixelGIF)
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
	if index >= len(record) {
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
	email := firstNonEmpty(value("邮箱", 4), value("email", 1), value("Email", 1))
	phone := firstNonEmpty(value("联系电话", 3), value("手机号", 4), value("电话", 4))
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

func closeRows(rows *sql.Rows) {
	if rows != nil {
		_ = rows.Close()
	}
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

func trackingBaseURL(c *gin.Context) string {
	if value := strings.TrimSpace(os.Getenv("TRACKING_BASE_URL")); value != "" {
		return value
	}
	if value := strings.TrimSpace(c.GetHeader("X-Tracking-Base-URL")); value != "" {
		return value
	}
	if value := strings.TrimSpace(c.GetHeader("X-Base-URL")); value != "" {
		return value
	}
	return "http://" + c.Request.Host
}
