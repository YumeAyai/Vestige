package scftracking

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nousmail/pkg/tracker"
	"nousmail/tracking-server/internal/trackingcloud"

	"github.com/google/uuid"
	"github.com/tencentyun/scf-go-lib/events"
)

type Store interface {
	RecordEvent(ctx context.Context, event trackingcloud.Event) (int64, error)
	ListEvents(ctx context.Context, filter EventFilter) ([]trackingcloud.Event, error)
	Stats(ctx context.Context, filter EventFilter) (StatsResult, error)
	SaveAsset(ctx context.Context, asset Asset) error
	GetAsset(ctx context.Context, name string) (Asset, error)
}

type EventFilter struct {
	Source   string
	Campaign string
	Kind     string
	Since    string
	AfterID  int64
	Limit    int64
}

type StatsResult struct {
	Summary []map[string]any `json:"summary"`
	Trend   []map[string]any `json:"trend"`
}

type Asset struct {
	Name        string
	Label       string
	ContentType string
	Data        []byte
	Width       int
	CreatedAt   time.Time
}

type Handler struct {
	Store Store
	Now   func() time.Time
}

func NewHandler(store Store) *Handler {
	return &Handler{Store: store, Now: time.Now}
}

func (h *Handler) Handle(ctx context.Context, req events.APIGatewayRequest) (events.APIGatewayResponse, error) {
	if h.Store == nil {
		return jsonResponse(http.StatusInternalServerError, map[string]string{"error": "tracking store is not configured"}), nil
	}
	path := normalizePath(req.Path, req.Context.Path)
	method := strings.ToUpper(firstNonEmpty(req.Method, req.Context.Method, http.MethodGet))
	switch {
	case method == http.MethodGet && path == "/health":
		return jsonResponse(http.StatusOK, map[string]any{"ok": true, "service": "tracking-scf"}), nil
	case method == http.MethodGet && path == "/p":
		return h.pixel(ctx, req), nil
	case method == http.MethodGet && path == "/r":
		return h.redirect(ctx, req), nil
	case method == http.MethodGet && path == "/qrcode.png":
		return h.trackingImage(ctx, req), nil
	case method == http.MethodGet && path == "/api/events":
		return h.events(ctx, req), nil
	case method == http.MethodGet && path == "/api/stats":
		return h.stats(ctx, req), nil
	case method == http.MethodPost && path == "/api/assets":
		return h.uploadAsset(ctx, req), nil
	default:
		return jsonResponse(http.StatusNotFound, map[string]string{"error": "not found"}), nil
	}
}

func (h *Handler) pixel(ctx context.Context, req events.APIGatewayRequest) events.APIGatewayResponse {
	_ = h.record(ctx, req, "open")
	return binaryResponse(http.StatusOK, "image/gif", tracker.PixelGIF, noStoreHeaders())
}

func (h *Handler) redirect(ctx context.Context, req events.APIGatewayRequest) events.APIGatewayResponse {
	dest := strings.TrimSpace(query(req, "dest"))
	if !validRedirect(dest) {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "dest must be an http or https URL"})
	}
	_ = h.record(ctx, req, "click")
	headers := map[string]string{"Location": dest}
	for key, value := range noStoreHeaders() {
		headers[key] = value
	}
	return events.APIGatewayResponse{StatusCode: http.StatusFound, Headers: headers}
}

func (h *Handler) trackingImage(ctx context.Context, req events.APIGatewayRequest) events.APIGatewayResponse {
	token := strings.TrimSpace(firstNonEmpty(query(req, "token"), query(req, "rid")))
	if token != "" && token != "preview" {
		event := eventFromRequest(req, "qrcode", h.now())
		event.Token = token
		_, _ = h.Store.RecordEvent(ctx, event)
	}
	if assetName := strings.TrimSpace(query(req, "asset")); assetName != "" {
		name := filepath.Base(assetName)
		if name != assetName {
			return emptyResponse(http.StatusNotFound)
		}
		asset, err := h.Store.GetAsset(ctx, name)
		if err != nil {
			return emptyResponse(http.StatusNotFound)
		}
		contentType := firstNonEmpty(asset.ContentType, mime.TypeByExtension(filepath.Ext(asset.Name)), "application/octet-stream")
		return binaryResponse(http.StatusOK, contentType, asset.Data, noStoreHeaders())
	}
	target := strings.TrimSpace(query(req, "target"))
	if target == "" || !validRedirect(target) {
		target = "https://example.com/survey"
	}
	png, err := tracker.QRCodePNG(target, queryInt(req, "size", 176))
	if err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return binaryResponse(http.StatusOK, "image/png", png, noStoreHeaders())
}

func (h *Handler) events(ctx context.Context, req events.APIGatewayRequest) events.APIGatewayResponse {
	filter := filterFromRequest(req, true)
	items, err := h.Store.ListEvents(ctx, filter)
	if err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return jsonResponse(http.StatusOK, map[string]any{"events": items})
}

func (h *Handler) stats(ctx context.Context, req events.APIGatewayRequest) events.APIGatewayResponse {
	result, err := h.Store.Stats(ctx, filterFromRequest(req, false))
	if err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return jsonResponse(http.StatusOK, result)
}

func (h *Handler) uploadAsset(ctx context.Context, req events.APIGatewayRequest) events.APIGatewayResponse {
	contentType := header(req, "Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "multipart form file is required"})
	}
	body, err := requestBodyBytes(req)
	if err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	form, err := reader.ReadForm(8 << 20)
	if err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	defer form.RemoveAll()
	files := form.File["file"]
	if len(files) == 0 {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "请上传企业微信二维码图片"})
	}
	file := files[0]
	opened, err := file.Open()
	if err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	defer opened.Close()
	var data bytes.Buffer
	if _, err := data.ReadFrom(opened); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	contentType = firstNonEmpty(file.Header.Get("Content-Type"), http.DetectContentType(data.Bytes()))
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext == "" {
		ext = imageExt(contentType)
	}
	if !allowedTrackingImageExt(ext) {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "仅支持 PNG、JPG、GIF 或 WebP 图片"})
	}
	width := formValueInt(form.Value, "width", 176)
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
	asset := Asset{
		Name:        name,
		Label:       label,
		ContentType: contentType,
		Data:        data.Bytes(),
		Width:       width,
		CreatedAt:   h.now(),
	}
	if err := h.Store.SaveAsset(ctx, asset); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	baseURL := requestBaseURL(req)
	return jsonResponse(http.StatusOK, map[string]any{
		"label":       label,
		"asset":       name,
		"placeholder": `{{TrackingImage "` + name + `"}}`,
		"image_url":   tracker.AssetImageURL(baseURL, "preview", name),
		"html":        tracker.TrackingImageHTML(baseURL, "preview", name, label, width),
		"collects":    []string{"ip", "user_agent", "referer", "accept_language", "forwarded_for", "triggered_at", "is_prefetch"},
	})
}

func (h *Handler) record(ctx context.Context, req events.APIGatewayRequest, kind string) error {
	event := eventFromRequest(req, kind, h.now())
	if event.Token == "" && event.Campaign == "" {
		return nil
	}
	_, err := h.Store.RecordEvent(ctx, event)
	return err
}

func (h *Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func eventFromRequest(req events.APIGatewayRequest, kind string, now time.Time) trackingcloud.Event {
	return trackingcloud.Event{
		Source:         firstNonEmpty(query(req, "s"), query(req, "source")),
		Campaign:       firstNonEmpty(query(req, "c"), query(req, "campaign")),
		Link:           firstNonEmpty(query(req, "l"), query(req, "link")),
		EventIndex:     firstNonEmpty(query(req, "i"), query(req, "idx"), query(req, "index"), query(req, "event_index"), query(req, "l"), query(req, "link"), query(req, "asset")),
		Token:          firstNonEmpty(query(req, "rid"), query(req, "tid"), query(req, "token")),
		Kind:           kind,
		TriggeredAt:    now.UTC().Format(time.RFC3339),
		IP:             clientIP(req),
		UserAgent:      header(req, "User-Agent"),
		Referer:        header(req, "Referer"),
		AcceptLanguage: header(req, "Accept-Language"),
		ForwardedFor:   header(req, "X-Forwarded-For"),
	}
}

func filterFromRequest(req events.APIGatewayRequest, includeCursor bool) EventFilter {
	limit := int64(queryInt(req, "limit", 500))
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	filter := EventFilter{
		Source:   strings.TrimSpace(firstNonEmpty(query(req, "s"), query(req, "source"))),
		Campaign: strings.TrimSpace(firstNonEmpty(query(req, "c"), query(req, "campaign"))),
		Kind:     strings.TrimSpace(firstNonEmpty(query(req, "event"), query(req, "kind"))),
		Since:    strings.TrimSpace(query(req, "since")),
		Limit:    limit,
	}
	if includeCursor {
		filter.AfterID = int64(queryInt(req, "after_id", 0))
	}
	return filter
}

func normalizePath(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if idx := strings.Index(value, "?"); idx >= 0 {
			value = value[:idx]
		}
		if !strings.HasPrefix(value, "/") {
			value = "/" + value
		}
		return mountedRoute(value)
	}
	return "/"
}

func mountedRoute(value string) string {
	routes := []string{"/health", "/p", "/r", "/qrcode.png", "/api/events", "/api/stats", "/api/assets"}
	for _, route := range routes {
		if value == route || strings.HasSuffix(value, route) {
			return route
		}
	}
	return value
}

func requestBaseURL(req events.APIGatewayRequest) string {
	scheme := "https"
	if proto := header(req, "X-Forwarded-Proto"); proto == "http" || proto == "https" {
		scheme = proto
	}
	host := header(req, "Host")
	if host == "" {
		host = "localhost"
	}
	return scheme + "://" + host
}

func clientIP(req events.APIGatewayRequest) string {
	if ip := strings.TrimSpace(req.Context.SourceIP); ip != "" {
		return ip
	}
	if xff := header(req, "X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	return ""
}

func query(req events.APIGatewayRequest, key string) string {
	values := req.QueryString[key]
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func queryInt(req events.APIGatewayRequest, key string, fallback int) int {
	value := query(req, key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func header(req events.APIGatewayRequest, key string) string {
	for name, value := range req.Headers {
		if strings.EqualFold(name, key) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func requestBodyBytes(req events.APIGatewayRequest) ([]byte, error) {
	body := []byte(req.Body)
	if strings.EqualFold(header(req, "X-Body-Base64"), "true") || strings.EqualFold(header(req, "X-Api-Gateway-Is-Base64-Encoded"), "true") {
		decoded, err := base64.StdEncoding.DecodeString(req.Body)
		if err != nil {
			return nil, err
		}
		body = decoded
	}
	return body, nil
}

func binaryResponse(status int, contentType string, data []byte, headers map[string]string) events.APIGatewayResponse {
	if headers == nil {
		headers = map[string]string{}
	}
	headers["Content-Type"] = contentType
	return events.APIGatewayResponse{
		IsBase64Encoded: true,
		StatusCode:      status,
		Headers:         headers,
		Body:            base64.StdEncoding.EncodeToString(data),
	}
}

func jsonResponse(status int, payload any) events.APIGatewayResponse {
	body, _ := json.Marshal(payload)
	return events.APIGatewayResponse{
		StatusCode: status,
		Headers: map[string]string{
			"Content-Type": "application/json; charset=utf-8",
		},
		Body: string(body),
	}
}

func emptyResponse(status int) events.APIGatewayResponse {
	return events.APIGatewayResponse{StatusCode: status, Headers: noStoreHeaders()}
}

func noStoreHeaders() map[string]string {
	return map[string]string{"Cache-Control": "no-store, no-cache, must-revalidate, max-age=0"}
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

func formValue(values map[string][]string, key string) string {
	list := values[key]
	if len(list) == 0 {
		return ""
	}
	return list[0]
}

func formValueInt(values map[string][]string, key string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(formValue(values, key)))
	if err != nil {
		return fallback
	}
	return parsed
}

func MultipartBody(field, filename, contentType string, data []byte, fields map[string]string) (string, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, filename))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return "", "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", "", err
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return "", "", err
		}
	}
	if err := writer.Close(); err != nil {
		return "", "", err
	}
	return writer.FormDataContentType(), body.String(), nil
}

var ErrNotFound = errors.New("not found")
