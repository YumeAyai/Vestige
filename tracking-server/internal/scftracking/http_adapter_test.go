package scftracking

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPAdapterRecordsPixel(t *testing.T) {
	store := newMemoryStore()
	handler := NewHTTPHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/jianji/p?c=1&rid=abc&i=variant:1:open", nil)
	req.Header.Set("User-Agent", "Mail")
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK || resp.Header().Get("Content-Type") != "image/gif" {
		t.Fatalf("unexpected response: code=%d headers=%v", resp.Code, resp.Header())
	}
	if len(store.events) != 1 || store.events[0].Kind != "open" || store.events[0].EventIndex != "variant:1:open" {
		t.Fatalf("unexpected events: %#v", store.events)
	}
}
