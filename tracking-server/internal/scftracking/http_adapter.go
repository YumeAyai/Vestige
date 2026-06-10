package scftracking

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/tencentyun/scf-go-lib/events"
)

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req, err := APIGatewayRequestFromHTTP(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := h.Handle(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	WriteAPIGatewayResponse(w, resp)
}

func APIGatewayRequestFromHTTP(r *http.Request) (events.APIGatewayRequest, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return events.APIGatewayRequest{}, err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	query := events.APIGatewayQueryString{}
	for key, values := range r.URL.Query() {
		query[key] = values
	}
	headers := map[string]string{}
	for key, values := range r.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	if r.Host != "" {
		headers["Host"] = r.Host
	}
	if r.TLS != nil {
		headers["X-Forwarded-Proto"] = "https"
	} else if headers["X-Forwarded-Proto"] == "" {
		headers["X-Forwarded-Proto"] = "http"
	}

	return events.APIGatewayRequest{
		Method:      r.Method,
		Path:        r.URL.Path,
		Headers:     headers,
		QueryString: query,
		Body:        string(body),
		Context: events.APIGatewayRequestContext{
			Method:   r.Method,
			Path:     r.URL.Path,
			SourceIP: remoteIP(r.RemoteAddr),
		},
	}, nil
}

func WriteAPIGatewayResponse(w http.ResponseWriter, resp events.APIGatewayResponse) {
	for key, value := range resp.Headers {
		w.Header().Set(key, value)
	}
	status := resp.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	body := []byte(resp.Body)
	if resp.IsBase64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(resp.Body)
		if err == nil {
			body = decoded
		}
	}
	_, _ = w.Write(body)
}

func remoteIP(addr string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(addr)
}

func NewHTTPHandler(store Store) http.Handler {
	return NewHandler(store)
}

func RunHTTP(ctx context.Context, addr string, store Store) error {
	server := &http.Server{Addr: addr, Handler: NewHTTPHandler(store)}
	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()
	err := server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
