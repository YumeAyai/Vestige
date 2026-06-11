package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"Vestige/pkg/tracker"
	"Vestige/tracker/internal/model"
	"Vestige/tracker/internal/store"

	"github.com/google/uuid"
)

type Handler struct {
	Store         store.Store
	Now           func() time.Time
	PublicBaseURL string
}

var serviceVersion = "event-dev"

func NewHandler(s store.Store) *Handler {
	return &Handler{Store: s, Now: time.Now}
}

func (h *Handler) Handle(ctx context.Context, event model.SCFEvent) (model.SCFResponse, error) {
	path := normalizePath(eventPath(event))
	method := strings.ToUpper(eventMethod(event))

	switch {
	case method == "GET" && path == "/health":
		return jsonResponse(200, map[string]any{"ok": true, "service": "tracker-scf", "version": serviceVersion}), nil
	case h.Store == nil:
		return jsonResponse(500, map[string]any{"error": "tracking store is not configured"}), nil
	case method == "GET" && path == "/p":
		return h.pixel(ctx, event)
	case method == "GET" && path == "/r":
		return h.redirect(ctx, event)
	case method == "GET" && path == "/img":
		return h.trackingImage(ctx, event)
	case method == "GET" && path == "/api/events":
		return h.events(ctx, event)
	case method == "GET" && path == "/api/stats":
		return h.stats(ctx, event)
	case method == "GET" && path == "/api/assets":
		return jsonResponse(200, map[string]any{"ok": true, "method": "POST", "content_type": "multipart/form-data"}), nil
	case method == "POST" && path == "/api/assets":
		return h.uploadAsset(ctx, event)
	default:
		return jsonResponse(404, map[string]any{"error": "not found"}), nil
	}
}

func (h *Handler) pixel(ctx context.Context, event model.SCFEvent) (model.SCFResponse, error) {
	if debugRequest(event) {
		if err := h.record(ctx, event, "open"); err != nil {
			return jsonResponse(500, map[string]any{"ok": false, "error": err.Error()}), nil
		}
		return jsonResponse(200, map[string]any{"ok": true, "kind": "open", "version": serviceVersion}), nil
	}
	_ = h.record(ctx, event, "open")
	return imageResponse("image/gif", tracker.PixelGIF), nil
}

func (h *Handler) redirect(ctx context.Context, event model.SCFEvent) (model.SCFResponse, error) {
	query := eventQuery(event)
	dest := strings.TrimSpace(query.Get("dest"))
	if !validRedirect(dest) {
		return jsonResponse(400, map[string]any{"error": "dest must be an http or https URL"}), nil
	}
	_ = h.record(ctx, event, "click")
	return redirectResponse(dest), nil
}

func (h *Handler) trackingImage(ctx context.Context, event model.SCFEvent) (model.SCFResponse, error) {
	query := eventQuery(event)
	token := strings.TrimSpace(firstNonEmpty(query.Get("token"), query.Get("rid")))
	eventKind := strings.TrimSpace(query.Get("kind"))
	if eventKind == "" {
		eventKind = "image"
	}

	if assetName := strings.TrimSpace(query.Get("asset")); assetName != "" {
		name := filepath.Base(assetName)
		if name != assetName {
			return model.SCFResponse{StatusCode: 404}, nil
		}
		asset, err := h.Store.GetAsset(ctx, name)
		if err != nil {
			return model.SCFResponse{StatusCode: 404}, nil
		}
		contentType := firstNonEmpty(asset.ContentType, mime.TypeByExtension(filepath.Ext(asset.Name)), "application/octet-stream")
		_ = h.recordImageEvent(ctx, event, query, token, eventKind)
		return imageResponse(contentType, asset.Data), nil
	}

	genType := strings.TrimSpace(query.Get("type"))
	switch genType {
	case "qr", "qrcode", "qr-code":
		target := strings.TrimSpace(query.Get("target"))
		if target == "" || !validRedirect(target) {
			return jsonResponse(400, map[string]any{"error": "target must be a valid http/https URL"}), nil
		}
		png, err := tracker.QRCodePNG(target, queryInt(query, "size", 176))
		if err != nil {
			return jsonResponse(400, map[string]any{"error": err.Error()}), nil
		}
		if debugRequest(event) {
			if err := h.recordImageEvent(ctx, event, query, token, eventKind); err != nil {
				return jsonResponse(500, map[string]any{"ok": false, "error": err.Error()}), nil
			}
			return jsonResponse(200, map[string]any{"ok": true, "kind": eventKind, "type": genType, "version": serviceVersion}), nil
		}
		_ = h.recordImageEvent(ctx, event, query, token, eventKind)
		return imageResponse("image/png", png), nil
	case "pixel":
		if debugRequest(event) {
			if err := h.recordImageEvent(ctx, event, query, token, eventKind); err != nil {
				return jsonResponse(500, map[string]any{"ok": false, "error": err.Error()}), nil
			}
			return jsonResponse(200, map[string]any{"ok": true, "kind": eventKind, "type": genType, "version": serviceVersion}), nil
		}
		_ = h.recordImageEvent(ctx, event, query, token, eventKind)
		return imageResponse("image/gif", tracker.PixelGIF), nil
	default:
		return jsonResponse(400, map[string]any{"error": "missing or invalid type parameter (use: qr, pixel)"}), nil
	}
}

func (h *Handler) recordImageEvent(ctx context.Context, event model.SCFEvent, query url.Values, token, kind string) error {
	if token == "preview" {
		return nil
	}
	evt := eventFromRequest(query, event, kind, h.now())
	if token != "" {
		evt.Token = token
	}
	if evt.Token == "" && evt.Campaign == "" {
		return nil
	}
	_, err := h.Store.RecordEvent(ctx, evt)
	return err
}

func (h *Handler) events(ctx context.Context, event model.SCFEvent) (model.SCFResponse, error) {
	query := eventQuery(event)
	filter := filterFromQuery(query, true)
	items, err := h.Store.ListEvents(ctx, filter)
	if err != nil {
		return jsonResponse(400, map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(200, map[string]any{"events": items}), nil
}

func (h *Handler) stats(ctx context.Context, event model.SCFEvent) (model.SCFResponse, error) {
	query := eventQuery(event)
	filter := filterFromQuery(query, false)
	result, err := h.Store.Stats(ctx, filter)
	if err != nil {
		return jsonResponse(400, map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(200, result), nil
}

func (h *Handler) uploadAsset(ctx context.Context, event model.SCFEvent) (model.SCFResponse, error) {
	contentType := header(event, "Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		return jsonResponse(400, map[string]any{"error": "请上传企业微信二维码图片"}), nil
	}

	var buf bytes.Buffer
	if event.IsBase64Encoded {
		data, err := base64.StdEncoding.DecodeString(event.Body)
		if err != nil {
			return jsonResponse(400, map[string]any{"error": "invalid base64 body"}), nil
		}
		buf.Write(data)
	} else {
		buf.WriteString(event.Body)
	}

	reader := multipart.NewReader(&buf, params["boundary"])
	form, err := reader.ReadForm(8 << 20)
	if err != nil {
		return jsonResponse(400, map[string]any{"error": "请上传企业微信二维码图片"}), nil
	}
	defer form.RemoveAll()

	files := form.File["file"]
	if len(files) == 0 {
		return jsonResponse(400, map[string]any{"error": "请上传企业微信二维码图片"}), nil
	}
	fileHeader := files[0]
	file, err := fileHeader.Open()
	if err != nil {
		return jsonResponse(400, map[string]any{"error": err.Error()}), nil
	}
	defer file.Close()

	var data bytes.Buffer
	if _, err := data.ReadFrom(file); err != nil {
		return jsonResponse(400, map[string]any{"error": err.Error()}), nil
	}

	contentType = firstNonEmpty(fileHeader.Header.Get("Content-Type"), http.DetectContentType(data.Bytes()))
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext == "" {
		ext = imageExt(contentType)
	}
	if !allowedTrackingImageExt(ext) {
		return jsonResponse(400, map[string]any{"error": "仅支持 PNG、JPG、GIF 或 WebP 图片"}), nil
	}

	width := formValueInt(formValue(form.Value, "width"), 176)
	if width < 96 {
		width = 96
	}
	if width > 480 {
		width = 480
	}

	label := strings.TrimSpace(formValue(form.Value, "label"))
	if label == "" {
		label = "企业微信二维码"
	}

	name := uuid.NewString() + ext
	asset := model.Asset{
		Name:        name,
		Label:       label,
		ContentType: contentType,
		Data:        data.Bytes(),
		Width:       width,
		CreatedAt:   h.now(),
	}

	if err := h.Store.SaveAsset(ctx, asset); err != nil {
		return jsonResponse(400, map[string]any{"error": err.Error()}), nil
	}

	baseURL := h.trackingBaseURL(event)
	return jsonResponse(200, map[string]any{
		"label":       label,
		"asset":       name,
		"placeholder": `{{TrackingImage "` + name + `"}}`,
		"image_url":   tracker.AssetImageURL(baseURL, "preview", name),
		"html":        tracker.TrackingImageHTML(baseURL, "preview", name, label, width),
		"collects":    []string{"ip", "user_agent", "referer", "accept_language", "forwarded_for", "triggered_at", "is_prefetch"},
	}), nil
}

func (h *Handler) record(ctx context.Context, event model.SCFEvent, kind string) error {
	query := eventQuery(event)
	evt := eventFromRequest(query, event, kind, h.now())
	if evt.Token == "" && evt.Campaign == "" {
		return nil
	}
	_, err := h.Store.RecordEvent(ctx, evt)
	return err
}

func (h *Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h *Handler) trackingBaseURL(event model.SCFEvent) string {
	if value := strings.TrimSpace(h.PublicBaseURL); value != "" {
		return value
	}
	return requestBaseURL(event)
}

func eventFromRequest(query url.Values, event model.SCFEvent, kind string, now time.Time) model.Event {
	return model.Event{
		Source:         safeScalar(firstNonEmpty(query.Get("s"), query.Get("source")), 128),
		Campaign:       safeScalar(firstNonEmpty(query.Get("c"), query.Get("campaign")), 128),
		Link:           safeScalar(firstNonEmpty(query.Get("l"), query.Get("link")), 512),
		EventIndex:     safeScalar(firstNonEmpty(query.Get("i"), query.Get("idx"), query.Get("index"), query.Get("event_index"), query.Get("l"), query.Get("link"), query.Get("asset")), 512),
		Token:          safeScalar(firstNonEmpty(query.Get("rid"), query.Get("tid"), query.Get("token")), 256),
		Kind:           kind,
		TriggeredAt:    now.UTC().Format(time.RFC3339Nano),
		IP:             safeScalar(firstNonEmpty(event.RequestContext.SourceIP, strings.Split(header(event, "X-Forwarded-For"), ",")[0], header(event, "X-Real-IP")), 128),
		UserAgent:      safeScalar(header(event, "User-Agent"), 512),
		Referer:        safeScalar(header(event, "Referer"), 512),
		AcceptLanguage: safeScalar(header(event, "Accept-Language"), 128),
		ForwardedFor:   safeScalar(header(event, "X-Forwarded-For"), 512),
	}
}

func filterFromQuery(query url.Values, includeCursor bool) model.EventFilter {
	limit := int64(queryInt(query, "limit", 500))
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	filter := model.EventFilter{
		Source:   safeScalar(firstNonEmpty(query.Get("s"), query.Get("source")), 128),
		Campaign: safeScalar(firstNonEmpty(query.Get("c"), query.Get("campaign")), 128),
		Kind:     safeScalar(firstNonEmpty(query.Get("event"), query.Get("kind")), 64),
		Limit:    limit,
	}
	if since := safeRFC3339(query.Get("since")); since != "" {
		filter.Since = since
	}
	if includeCursor {
		filter.AfterID = int64(queryInt(query, "after_id", 0))
	}
	return filter
}

func safeScalar(value string, maxLen int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, value)
	if maxLen > 0 && len(value) > maxLen {
		value = value[:maxLen]
	}
	return value
}

func safeRFC3339(value string) string {
	value = safeScalar(value, 64)
	if value == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return ""
	}
	return parsed.UTC().Format(time.RFC3339)
}

func debugRequest(event model.SCFEvent) bool {
	value := strings.ToLower(strings.TrimSpace(eventQuery(event).Get("debug")))
	return value == "1" || value == "true" || value == "yes"
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return mountedRoute(path)
}

func eventMethod(event model.SCFEvent) string {
	return firstNonEmpty(event.Method, event.HTTPMethod, event.RequestContext.Method, event.RequestContext.HTTPMethod, http.MethodGet)
}

func eventPath(event model.SCFEvent) string {
	return firstNonEmpty(event.Path, event.RequestContext.Path, "/")
}

func eventQuery(event model.SCFEvent) url.Values {
	values := parseQuery(event.QueryString)
	for key, value := range event.QueryStringParameters {
		if _, ok := values[key]; !ok {
			values.Set(key, value)
		}
	}
	for key, list := range event.MultiValueQueryStringParameters {
		if _, ok := values[key]; ok {
			continue
		}
		for _, value := range list {
			values.Add(key, value)
		}
	}
	return values
}

func mountedRoute(value string) string {
	routes := []string{"/health", "/p", "/r", "/img", "/api/events", "/api/stats", "/api/assets"}
	for _, route := range routes {
		if value == route || strings.HasSuffix(value, route) {
			return route
		}
	}
	return value
}

func requestBaseURL(event model.SCFEvent) string {
	scheme := "http"
	if proto := header(event, "X-Forwarded-Proto"); proto == "http" || proto == "https" {
		scheme = proto
	}
	host := header(event, "Host")
	if host == "" {
		host = "localhost"
	}
	return scheme + "://" + host + requestMountPrefix(eventPath(event))
}

func requestMountPrefix(path string) string {
	path = strings.TrimSpace(path)
	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	for _, route := range []string{"/api/assets", "/api/events", "/api/stats", "/health", "/img", "/p", "/r"} {
		if path == route {
			return ""
		}
		if strings.HasSuffix(path, route) {
			prefix := strings.TrimRight(strings.TrimSuffix(path, route), "/")
			if prefix != "/" {
				return prefix
			}
			return ""
		}
	}
	return ""
}

func header(event model.SCFEvent, key string) string {
	for name, value := range event.Headers {
		if strings.EqualFold(name, key) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func queryInt(query url.Values, key string, fallback int) int {
	value := strings.TrimSpace(query.Get(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseQuery(queryString string) url.Values {
	query, _ := url.ParseQuery(queryString)
	return query
}

func validRedirect(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func imageExt(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}

func allowedTrackingImageExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	default:
		return false
	}
}

func formValueInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func formValue(values map[string][]string, key string) string {
	list := values[key]
	if len(list) == 0 {
		return ""
	}
	return list[0]
}

func jsonResponse(statusCode int, data any) model.SCFResponse {
	body, _ := json.Marshal(data)
	return model.SCFResponse{
		StatusCode: statusCode,
		Headers:    responseHeaders("application/json"),
		Body:       string(body),
	}
}

func imageResponse(contentType string, data []byte) model.SCFResponse {
	return model.SCFResponse{
		StatusCode:      200,
		Headers:         responseHeaders(contentType),
		Body:            base64.StdEncoding.EncodeToString(data),
		IsBase64Encoded: true,
	}
}

func responseHeaders(contentType string) map[string]string {
	return map[string]string{
		"Content-Type":  contentType,
		"content-type":  contentType,
		"Cache-Control": "no-store, no-cache, must-revalidate, max-age=0",
	}
}

func redirectResponse(location string) model.SCFResponse {
	return model.SCFResponse{
		StatusCode: 302,
		Headers:    map[string]string{"Location": location},
	}
}
