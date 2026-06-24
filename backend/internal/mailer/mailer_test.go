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

func TestValidateEmailAcceptsCommonAddress(t *testing.T) {
	if err := ValidateEmail("sales+cn@example.co.uk", "邮箱"); err != nil {
		t.Fatalf("expected valid email, got %v", err)
	}
}

func TestValidateEmailRejectsInvalidFormats(t *testing.T) {
	for _, email := range []string{
		"not-an-email",
		"user@localhost",
		"user@example",
		"user name@example.com",
		"User <user@example.com>",
		"user@example.com,other@example.com",
		"user@example.com;other@example.com",
		"用户@example.com",
		"user@例子.com",
	} {
		if err := ValidateEmail(email, "邮箱"); err == nil {
			t.Fatalf("expected %q to be invalid", email)
		}
	}
}

func TestValidateEmailListAcceptsSemicolonSeparatedAddresses(t *testing.T) {
	if err := ValidateEmailList("sales@example.com; ops@example.com；support@example.co.uk", "邮箱"); err != nil {
		t.Fatalf("expected valid email list, got %v", err)
	}
}

func TestNormalizeEmailListTrimsSeparatedAddresses(t *testing.T) {
	got := NormalizeEmailList(" sales@example.com ; ops@example.com； support@example.com ")
	want := "sales@example.com;ops@example.com;support@example.com"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNormalizeEmailListDropsInvalidAddresses(t *testing.T) {
	got := NormalizeEmailList("sales@example.com; bad-address;用户@example.com; ops@example.com")
	want := "sales@example.com;ops@example.com"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestParseEmailListKeepsValidAddressesWhenSomeAreInvalid(t *testing.T) {
	got, err := ParseEmailList("sales@example.com;bad-address;ops@example.com", "邮箱")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"sales@example.com", "ops@example.com"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestSendRejectsInvalidRecipientBeforeDialing(t *testing.T) {
	err := Send(
		models.Mailbox{
			Host:      "127.0.0.1",
			Port:      1,
			Username:  "sender@example.com",
			Password:  "secret",
			FromEmail: "sender@example.com",
			FromName:  "Sender",
		},
		"bad-address",
		"Bad",
		"Subject",
		"<p>Hello</p>",
	)
	if err == nil || !strings.Contains(err.Error(), "收件邮箱格式不正确") {
		t.Fatalf("expected recipient validation error, got %v", err)
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
