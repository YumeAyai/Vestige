package handler

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultIPPortraitURL = "https://qifu.baidu.com/api/v1/ip-portrait/brief-info"

type ipPortraitBriefResponse struct {
	Code int `json:"code"`
	Data struct {
		Scene         string                              `json:"scene"`
		Company       string                              `json:"company"`
		RiskScore     string                              `json:"risk_score"`
		SecurityRisks map[string][]ipPortraitRiskCategory `json:"security_risks"`
		HitRiskNum    int                                 `json:"hit_risk_num"`
	} `json:"data"`
}

type ipPortraitRiskCategory struct {
	Label    string   `json:"label"`
	SubItems []string `json:"subItems"`
}

func (h *Handler) queueIPPortraitLookup(eventID int64, ip string) {
	ip = strings.TrimSpace(ip)
	if eventID <= 0 || ip == "" || ipPortraitIsLocal(ip) || strings.TrimSpace(h.IPPortraitURL) == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		summary := h.lookupIPPortrait(ctx, ip)
		if summary == "" {
			return
		}
		_ = h.Store.UpdateEventIPRisk(ctx, eventID, summary)
	}()
}

func (h *Handler) lookupIPPortrait(ctx context.Context, ip string) string {
	endpoint := strings.TrimSpace(h.IPPortraitURL)
	if endpoint == "" {
		endpoint = defaultIPPortraitURL
	}
	requestURL, err := ipPortraitURL(endpoint, ip)
	if err != nil {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Referer", ipPortraitReferer(ip))
	client := h.IPHTTPClient
	if client == nil {
		client = &http.Client{Timeout: 1200 * time.Millisecond}
	}
	res, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return ""
	}
	var payload ipPortraitBriefResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil || payload.Code != http.StatusOK {
		return ""
	}
	return ipPortraitSummary(payload)
}

func ipPortraitURL(endpoint, ip string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", err
	}
	values := parsed.Query()
	values.Set("ip", ip)
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func ipPortraitReferer(ip string) string {
	values := url.Values{}
	values.Set("activeKey", "SEARCH_IP")
	values.Set("trace", "apistore_ip_aladdin")
	values.Set("activeId", "SEARCH_IP_ADDRESS")
	values.Set("ip", ip)
	return "https://qifu.baidu.com/?" + values.Encode()
}

func ipPortraitIsLocal(value string) bool {
	parsed := net.ParseIP(value)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsUnspecified()
}

func ipPortraitSummary(payload ipPortraitBriefResponse) string {
	parts := []string{}
	if value := strings.TrimSpace(payload.Data.RiskScore); value != "" {
		parts = append(parts, "风险"+value)
	}
	if value := strings.TrimSpace(payload.Data.Scene); value != "" {
		parts = append(parts, value)
	}
	if value := strings.TrimSpace(payload.Data.Company); value != "" {
		parts = append(parts, value)
	}
	labels := []string{}
	for _, items := range payload.Data.SecurityRisks {
		for _, item := range items {
			if label := strings.TrimSpace(item.Label); label != "" {
				labels = append(labels, label)
			}
			for _, subItem := range item.SubItems {
				if label := strings.TrimSpace(subItem); label != "" {
					labels = append(labels, label)
				}
			}
		}
	}
	parts = append(parts, uniqueStrings(labels)...)
	return strings.Join(uniqueStrings(parts), " / ")
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
