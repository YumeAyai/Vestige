package tracker

import (
	"net/url"
	"strings"
	"testing"
)

func TestTrackingURLsTrimBaseAndEncodeQuery(t *testing.T) {
	pixel := PixelURL("https://track.example.com/", "rid 1", "camp&1")
	parsed, err := url.Parse(pixel)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.String() != pixel {
		t.Fatalf("invalid URL: %s", pixel)
	}
	if parsed.Path != "/p" {
		t.Fatalf("unexpected pixel path: %s", parsed.Path)
	}
	if got := parsed.Query().Get("rid"); got != "rid 1" {
		t.Fatalf("unexpected rid: %q", got)
	}
	if got := parsed.Query().Get("c"); got != "camp&1" {
		t.Fatalf("unexpected campaign: %q", got)
	}

	redirect := RedirectURL("https://track.example.com///", "42", "hero", "rid", "https://example.com/a?x=1&y=2")
	parsed, err = url.Parse(redirect)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/r" {
		t.Fatalf("unexpected redirect path: %s", parsed.Path)
	}
	if got := parsed.Query().Get("dest"); got != "https://example.com/a?x=1&y=2" {
		t.Fatalf("unexpected dest: %q", got)
	}
}

func TestInjectPixelPlacesImageBeforeBodyClose(t *testing.T) {
	body := "<html><body><p>Hello</p></body></html>"
	got := InjectPixel(body, "https://track.example.com", "rid", "campaign")

	if !strings.Contains(got, `src="https://track.example.com/p?c=campaign&amp;rid=rid"`) &&
		!strings.Contains(got, `src="https://track.example.com/p?c=campaign&rid=rid"`) {
		t.Fatalf("pixel URL missing from body: %s", got)
	}
	if strings.Index(got, `<img`) > strings.Index(got, `</body>`) {
		t.Fatalf("pixel should be inserted before body close: %s", got)
	}
}

func TestTrackingImageHTMLEscapesAltAndDefaultsWidth(t *testing.T) {
	got := TrackingImageHTML("https://track.example.com", "preview", "qr.png", `ACME "Team" <tag>`, 0)

	if !strings.Contains(got, `width="176"`) {
		t.Fatalf("expected default width: %s", got)
	}
	if !strings.Contains(got, `alt="ACME &quot;Team&quot; &lt;tag&gt;"`) {
		t.Fatalf("expected escaped alt text: %s", got)
	}
	if strings.Contains(got, `<tag>`) {
		t.Fatalf("alt text was not escaped: %s", got)
	}
}

func TestLooksLikePrefetch(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		want      bool
	}{
		{name: "google image proxy", userAgent: "Mozilla/5.0 GoogleImageProxy", want: true},
		{name: "apple mail", userAgent: "Mozilla/5.0 Apple Mail", want: true},
		{name: "regular browser", userAgent: "Mozilla/5.0 Chrome/120", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LooksLikePrefetch(tt.userAgent); got != tt.want {
				t.Fatalf("LooksLikePrefetch(%q) = %v, want %v", tt.userAgent, got, tt.want)
			}
		})
	}
}
