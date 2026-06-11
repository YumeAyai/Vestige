package model

import (
	"time"

	"github.com/tencentyun/scf-go-lib/events"
)

type Event struct {
	ID             int64  `json:"id"`
	Source         string `json:"source"`
	Campaign       string `json:"campaign"`
	Link           string `json:"link"`
	EventIndex     string `json:"event_index,omitempty"`
	Token          string `json:"token"`
	Kind           string `json:"kind"`
	TriggeredAt    string `json:"triggered_at"`
	IP             string `json:"ip,omitempty"`
	UserAgent      string `json:"user_agent,omitempty"`
	Referer        string `json:"referer,omitempty"`
	AcceptLanguage string `json:"accept_language,omitempty"`
	ForwardedFor   string `json:"forwarded_for,omitempty"`
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

type SCFEvent struct {
	Headers                         map[string]string   `json:"headers"`
	QueryString                     string              `json:"queryString"`
	QueryStringParameters           map[string]string   `json:"queryStringParameters"`
	MultiValueQueryStringParameters map[string][]string `json:"multiValueQueryStringParameters"`
	Method                          string              `json:"method"`
	HTTPMethod                      string              `json:"httpMethod"`
	Path                            string              `json:"path"`
	RequestContext                  SCFRequestContext   `json:"requestContext"`
	Body                            string              `json:"body"`
	IsBase64Encoded                 bool                `json:"isBase64Encoded"`
}

type SCFRequestContext struct {
	Method     string `json:"method"`
	HTTPMethod string `json:"httpMethod"`
	Path       string `json:"path"`
	SourceIP   string `json:"sourceIp"`
}

type SCFResponse = events.APIGatewayResponse
