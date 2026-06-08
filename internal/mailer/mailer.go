package mailer

import (
	"bytes"
	"crypto/tls"
	"html/template"

	"nousmail/internal/models"

	"gopkg.in/gomail.v2"
)

type Personalization struct {
	BaseURL   string
	Contact   models.Contact
	Recipient models.Recipient
	Campaign  models.Campaign
}

func RenderBody(body string, data Personalization) (string, error) {
	tpl, err := template.New("mail").Parse(body)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, map[string]string{
		"Name":       data.Contact.Name,
		"Email":      data.Contact.Email,
		"Company":    data.Contact.Company,
		"Department": data.Contact.Department,
		"Phone":      data.Contact.Phone,
		"Tags":       data.Contact.Tags,
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func Send(mailbox models.Mailbox, toEmail, toName, subject, html string) error {
	msg := gomail.NewMessage()
	msg.SetAddressHeader("From", mailbox.FromEmail, mailbox.FromName)
	msg.SetAddressHeader("To", toEmail, toName)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/html", html)

	dialer := gomail.NewDialer(mailbox.Host, mailbox.Port, mailbox.Username, mailbox.Password)
	if !mailbox.UseTLS {
		dialer.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return dialer.DialAndSend(msg)
}
