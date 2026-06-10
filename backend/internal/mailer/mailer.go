package mailer

import (
	"bytes"
	"crypto/tls"
	"errors"
	"html/template"
	"net"
	"net/textproto"
	"strings"

	"nousmail/pkg/models"

	"gopkg.in/gomail.v2"
)

type Personalization struct {
	BaseURL       string
	Contact       models.Contact
	Recipient     models.Recipient
	Campaign      models.Campaign
	QRCode        template.HTML
	TrackingImage func(asset string) template.HTML
}

func RenderBody(body string, data Personalization) (string, error) {
	tpl, err := template.New("mail").Funcs(template.FuncMap{
		"TrackingImage": func(asset string) template.HTML {
			if data.TrackingImage == nil {
				return ""
			}
			return data.TrackingImage(asset)
		},
	}).Parse(body)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, map[string]any{
		"Name":       data.Contact.Name,
		"Email":      data.Contact.Email,
		"Company":    data.Contact.Company,
		"Department": data.Contact.Department,
		"Phone":      data.Contact.Phone,
		"Tags":       data.Contact.Tags,
		"QRCode":     data.QRCode,
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func Send(mailbox models.Mailbox, toEmail, toName, subject, html string) error {
	mailbox = NormalizeMailbox(mailbox)
	if err := ValidateMailbox(mailbox); err != nil {
		return err
	}
	msg := gomail.NewMessage()
	msg.SetAddressHeader("From", mailbox.FromEmail, mailbox.FromName)
	msg.SetAddressHeader("To", toEmail, toName)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/html", html)

	dialer := gomail.NewDialer(mailbox.Host, mailbox.Port, mailbox.Username, mailbox.Password)
	dialer.SSL = mailbox.UseTLS && mailbox.Port == 465
	dialer.TLSConfig = &tls.Config{ServerName: mailbox.Host, MinVersion: tls.VersionTLS12}
	if !mailbox.UseTLS {
		dialer.SSL = false
	}
	return explainSMTPError(dialer.DialAndSend(msg))
}

func ValidateMailbox(mailbox models.Mailbox) error {
	mailbox = NormalizeMailbox(mailbox)
	missing := []string{}
	if strings.TrimSpace(mailbox.Host) == "" {
		missing = append(missing, "SMTP Host")
	}
	if mailbox.Port <= 0 {
		missing = append(missing, "端口")
	}
	if strings.TrimSpace(mailbox.Username) == "" {
		missing = append(missing, "账号")
	}
	if strings.TrimSpace(mailbox.Password) == "" {
		missing = append(missing, "授权码/密码")
	}
	if strings.TrimSpace(mailbox.FromEmail) == "" {
		missing = append(missing, "发件邮箱")
	}
	if len(missing) > 0 {
		return errors.New("SMTP 配置缺少：" + strings.Join(missing, "、"))
	}
	return nil
}

func Test(mailbox models.Mailbox, toEmail string) error {
	mailbox = NormalizeMailbox(mailbox)
	toEmail = strings.TrimSpace(toEmail)
	if toEmail == "" {
		toEmail = firstNonEmpty(mailbox.FromEmail, mailbox.Username)
	}
	return Send(
		mailbox,
		toEmail,
		"SMTP 测试",
		"见迹 SMTP 测试邮件",
		`<p>这是一封 SMTP 配置测试邮件。</p><p>如果你收到它，说明发件邮箱配置可以正常连接、认证并发送。</p>`,
	)
}

func NormalizeMailbox(mailbox models.Mailbox) models.Mailbox {
	mailbox.Name = strings.TrimSpace(mailbox.Name)
	mailbox.Host = strings.TrimSpace(mailbox.Host)
	mailbox.Username = strings.TrimSpace(mailbox.Username)
	mailbox.FromEmail = strings.TrimSpace(mailbox.FromEmail)
	mailbox.FromName = strings.TrimSpace(mailbox.FromName)
	return mailbox
}

func explainSMTPError(err error) error {
	if err == nil {
		return nil
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Timeout() {
			return errors.New("SMTP 连接超时：请检查 Host、端口、网络连通性，或云服务器安全组/防火墙是否允许 SMTP 出站")
		}
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return errors.New("SMTP Host 无法解析：请检查服务器地址是否正确")
	}
	var smtpErr *textproto.Error
	if errors.As(err, &smtpErr) {
		code := smtpErr.Code
		msg := strings.TrimSpace(smtpErr.Msg)
		switch code {
		case 535, 534:
			return errors.New("SMTP 认证失败：请确认账号使用完整邮箱地址，并使用授权码/应用专用密码，而不是登录密码")
		case 550, 553, 554:
			return errors.New("SMTP 服务器拒绝发件人或收件人：" + msg)
		default:
			return errors.New("SMTP 错误：" + smtpErr.Error())
		}
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "certificate"):
		return errors.New("SMTP TLS 证书校验失败：请检查 Host 是否和证书域名匹配")
	case strings.Contains(msg, "authentication failed"), strings.Contains(msg, "auth"):
		return errors.New("SMTP 认证失败：请确认账号、授权码/应用专用密码和 SMTP 服务开关")
	case strings.Contains(msg, "connection refused"):
		return errors.New("SMTP 连接被拒绝：请检查端口是否正确，465 通常为 SSL/TLS，587 通常为 STARTTLS")
	case strings.Contains(msg, "timeout"):
		return errors.New("SMTP 连接超时：请检查网络、防火墙或服务商是否限制 SMTP 出站")
	default:
		return err
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
