package app

import (
	"strings"
	"testing"

	"Vestige/pkg/config"
)

func TestReadContactsCSVWithoutHeaderUsesFallbackColumns(t *testing.T) {
	data := []byte("Acme Inc,hello@example.com,unused,unused,021-12345678\n")

	contacts, err := readContactsCSV(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(contacts))
	}
	contact := contacts[0]
	if contact.Name != "Acme Inc" || contact.Company != "Acme Inc" {
		t.Fatalf("unexpected company mapping: %#v", contact)
	}
	if contact.Email != "hello@example.com" {
		t.Fatalf("unexpected email: %s", contact.Email)
	}
	if contact.Phone != "021-12345678" {
		t.Fatalf("unexpected phone: %s", contact.Phone)
	}
}

func TestTemplateMarkers(t *testing.T) {
	if !templateUsesQRCode(`<p>{{.QRCode}}</p>`) {
		t.Fatal("expected QRCode marker to be detected")
	}
	if templateUsesQRCode(`<p>{{QRCode}}</p>`) {
		t.Fatal("unexpected QRCode marker detection")
	}
	if !templateUsesTrackingImage(`{{TrackingImage "asset.png"}}`) {
		t.Fatal("expected TrackingImage marker to be detected")
	}
}

func TestQRCodeTargetURLUsesEnvAndChoosesSeparator(t *testing.T) {
	t.Setenv("QR_CODE_TARGET_URL", "https://example.com/survey?src=email")
	server := &Server{cfg: config.Default()}

	got := server.qrcodeTargetURL("token-1")
	if got != "https://example.com/survey?src=email&t=token-1" {
		t.Fatalf("unexpected target URL: %s", got)
	}
}

func TestImageExtensionHelpers(t *testing.T) {
	if got := imageExt(" image/png "); got != ".png" {
		t.Fatalf("imageExt returned %q", got)
	}
	if !allowedTrackingImageExt(".WEBP") {
		t.Fatal("expected .WEBP to be allowed")
	}
	if allowedTrackingImageExt(".svg") {
		t.Fatal("expected .svg to be rejected")
	}
}

func TestCompactNotesIncludesNonEmptyFields(t *testing.T) {
	got := compactNotes(map[string]string{
		"官网":   " https://example.com ",
		"数据来源": "",
	})
	if !strings.Contains(got, "官网：https://example.com") {
		t.Fatalf("unexpected notes: %s", got)
	}
	if strings.Contains(got, "数据来源") {
		t.Fatalf("empty fields should be omitted: %s", got)
	}
}
