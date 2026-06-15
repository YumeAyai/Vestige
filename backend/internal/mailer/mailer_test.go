package mailer

import (
	"html/template"
	"strings"
	"testing"

	"Vestige/pkg/models"
)

func TestValidateMailboxReportsMissingFields(t *testing.T) {
	err := ValidateMailbox(models.Mailbox{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	message := err.Error()
	for _, field := range []string{"SMTP Host", "端口", "账号", "授权码/密码", "发件邮箱"} {
		if !strings.Contains(message, field) {
			t.Fatalf("expected %q in %q", field, message)
		}
	}
}

func TestNormalizeMailboxTrimsAddressFields(t *testing.T) {
	got := NormalizeMailbox(models.Mailbox{
		Name:      " 企业邮箱 ",
		Host:      " smtp.example.com ",
		Username:  " user@example.com ",
		FromEmail: " user@example.com ",
		FromName:  " 见迹 ",
	})

	if got.Name != "企业邮箱" {
		t.Fatalf("unexpected name: %q", got.Name)
	}
	if got.Host != "smtp.example.com" {
		t.Fatalf("unexpected host: %q", got.Host)
	}
	if got.Username != "user@example.com" {
		t.Fatalf("unexpected username: %q", got.Username)
	}
	if got.FromEmail != "user@example.com" {
		t.Fatalf("unexpected from email: %q", got.FromEmail)
	}
	if got.FromName != "见迹" {
		t.Fatalf("unexpected from name: %q", got.FromName)
	}
}

func TestRenderBodySupportsTrackingLink(t *testing.T) {
	body, err := RenderBody(
		`<a href="{{TrackingLink "官网" "https://example.com/path?a=1&b=2"}}">查看详情</a>`,
		Personalization{
			TrackingLink: func(label, targetURL string) template.URL {
				if label != "官网" {
					t.Fatalf("unexpected label: %q", label)
				}
				if targetURL != "https://example.com/path?a=1&b=2" {
					t.Fatalf("unexpected target URL: %q", targetURL)
				}
				return template.URL("https://track.example/r?rid=abc&dest=https%3A%2F%2Fexample.com%2Fpath")
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `href="https://track.example/r?rid=abc&amp;dest=https%3A%2F%2Fexample.com%2Fpath"`) {
		t.Fatalf("tracking link was not rendered in href: %s", body)
	}
}
