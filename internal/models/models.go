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
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Subject  string `json:"subject"`
	BodyHTML string `json:"body_html"`
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

type Recipient struct {
	ID            int64  `json:"id"`
	CampaignID    int64  `json:"campaign_id"`
	ContactID     int64  `json:"contact_id"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	TrackingID    string `json:"tracking_id"`
	SendStatus    string `json:"send_status"`
	FailureReason string `json:"failure_reason"`
	SentAt        string `json:"sent_at"`
	FirstOpenedAt string `json:"first_opened_at"`
	LastOpenedAt  string `json:"last_opened_at"`
	OpenCount     int    `json:"open_count"`
}
