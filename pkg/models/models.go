package models

type Mailbox struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	Password  string `json:"password,omitempty"`
	FromEmail string `json:"from_email"`
	FromName  string `json:"from_name"`
	UseTLS    bool   `json:"use_tls"`
}

type Contact struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	Company    string `json:"company"`
	Department string `json:"department"`
	Phone      string `json:"phone"`
	Tags       string `json:"tags"`
	Notes      string `json:"notes"`
}

type Template struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Subject   string `json:"subject"`
	BodyHTML  string `json:"body_html"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Campaign struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Subject         string `json:"subject"`
	BodyHTML        string `json:"body_html"`
	MailboxID       int64  `json:"mailbox_id"`
	Status          string `json:"status"`
	TrackingEnabled bool   `json:"tracking_enabled"`
	CreatedAt       string `json:"created_at"`
	SentAt          string `json:"sent_at"`
}

type CampaignAttachment struct {
	ID           int64  `json:"id"`
	CampaignID   int64  `json:"campaign_id"`
	OriginalName string `json:"original_name"`
	StoredName   string `json:"stored_name"`
	ContentType  string `json:"content_type"`
	Size         int64  `json:"size"`
	LinkBackup   bool   `json:"link_backup"`
	CloudAsset   string `json:"cloud_asset"`
	CloudURL     string `json:"cloud_url"`
}

type Recipient struct {
	ID             int64  `json:"id"`
	CampaignID     int64  `json:"campaign_id"`
	ContactID      int64  `json:"contact_id"`
	Email          string `json:"email"`
	Name           string `json:"name"`
	TrackingID     string `json:"tracking_id"`
	SendStatus     string `json:"send_status"`
	FailureReason  string `json:"failure_reason"`
	SentAt         string `json:"sent_at"`
	FirstOpenedAt  string `json:"first_opened_at"`
	LastOpenedAt   string `json:"last_opened_at"`
	OpenCount      int    `json:"open_count"`
	QRLoadCount    int    `json:"qr_load_count"`
	FirstQRLoadAt  string `json:"first_qr_load_at"`
	LastQRLoadAt   string `json:"last_qr_load_at"`
	LastQRIP       string `json:"last_qr_ip"`
	LastQRUA       string `json:"last_qr_user_agent"`
	LastQRXFF      string `json:"last_qr_forwarded_for"`
	LastQRSource   string `json:"last_qr_source"`
	LastQRReferer  string `json:"last_qr_referer"`
	LastQRLang     string `json:"last_qr_accept_language"`
	LastQRPrefetch bool   `json:"last_qr_is_prefetch"`
	LastQRIPRisk   string `json:"last_qr_ip_risk"`
}
