package scftracking

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"nousmail/tracking-server/internal/trackingcloud"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	tcb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/tcb/v20180608"
)

type runCommandsClient interface {
	RunCommandsWithContext(context.Context, *tcb.RunCommandsRequest) (*tcb.RunCommandsResponse, error)
}

type TCBStore struct {
	client runCommandsClient
	envID  string
	events string
	assets string
}

type TCBConfig struct {
	EnvID            string
	Region           string
	EventsCollection string
	AssetsCollection string
}

type tcbEventDoc struct {
	ID             int64  `json:"id"`
	Source         string `json:"source"`
	Campaign       string `json:"campaign"`
	Link           string `json:"link"`
	EventIndex     string `json:"event_index"`
	Token          string `json:"token"`
	Kind           string `json:"kind"`
	IP             string `json:"ip"`
	UserAgent      string `json:"user_agent"`
	Referer        string `json:"referer"`
	AcceptLanguage string `json:"accept_language"`
	ForwardedFor   string `json:"forwarded_for"`
	RawPayload     string `json:"raw_payload"`
	TriggeredAt    string `json:"triggered_at"`
}

type tcbAssetDoc struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	ContentType string `json:"content_type"`
	DataBase64  string `json:"data_base64"`
	Width       int    `json:"width"`
	CreatedAt   string `json:"created_at"`
}

func NewTCBStore(ctx context.Context, cfg TCBConfig) (*TCBStore, error) {
	if strings.TrimSpace(cfg.EnvID) == "" {
		return nil, errors.New("TCB_ENV_ID is required")
	}
	if strings.TrimSpace(cfg.Region) == "" {
		cfg.Region = "ap-shanghai"
	}
	if strings.TrimSpace(cfg.EventsCollection) == "" {
		cfg.EventsCollection = "tracking_events"
	}
	if strings.TrimSpace(cfg.AssetsCollection) == "" {
		cfg.AssetsCollection = "tracking_assets"
	}
	credential, err := common.DefaultProviderChain().GetCredential()
	if err != nil {
		return nil, err
	}
	client, err := tcb.NewClient(credential, cfg.Region, profile.NewClientProfile())
	if err != nil {
		return nil, err
	}
	store := NewTCBStoreWithClient(client, cfg)
	return store, store.EnsureIndexes(ctx)
}

func NewTCBStoreWithClient(client runCommandsClient, cfg TCBConfig) *TCBStore {
	return &TCBStore{
		client: client,
		envID:  strings.TrimSpace(cfg.EnvID),
		events: tcbValueOr(cfg.EventsCollection, "tracking_events"),
		assets: tcbValueOr(cfg.AssetsCollection, "tracking_assets"),
	}
}

func (s *TCBStore) EnsureIndexes(ctx context.Context) error {
	commands := []struct {
		table string
		body  map[string]any
	}{
		{s.events, createIndexesCommand(s.events, []map[string]any{
			{"key": map[string]any{"id": 1}, "name": "id_1", "unique": true},
			{"key": map[string]any{"source": 1, "id": 1}, "name": "source_1_id_1"},
			{"key": map[string]any{"source": 1, "campaign": 1, "kind": 1, "triggered_at": 1}, "name": "source_campaign_kind_time"},
			{"key": map[string]any{"source": 1, "campaign": 1, "event_index": 1, "triggered_at": 1}, "name": "source_campaign_event_index_time"},
			{"key": map[string]any{"token": 1, "kind": 1, "triggered_at": 1}, "name": "token_kind_time"},
		})},
		{s.assets, createIndexesCommand(s.assets, []map[string]any{
			{"key": map[string]any{"name": 1}, "name": "name_1", "unique": true},
		})},
		{"tracking_counters", createIndexesCommand("tracking_counters", []map[string]any{
			{"key": map[string]any{"_id": 1}, "name": "_id_1", "unique": true},
		})},
	}
	for _, command := range commands {
		if _, err := s.runCommand(ctx, command.table, "COMMAND", command.body); err != nil && !indexAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func (s *TCBStore) RecordEvent(ctx context.Context, event trackingcloud.Event) (int64, error) {
	if event.Token == "" && event.Campaign == "" {
		return 0, nil
	}
	id, err := s.nextID(ctx, "tracking_events")
	if err != nil {
		return 0, err
	}
	triggeredAt := parseEventTime(event.TriggeredAt)
	event.ID = id
	event.TriggeredAt = triggeredAt.UTC().Format(time.RFC3339)
	raw, _ := json.Marshal(event)
	doc := tcbEventDoc{
		ID:             id,
		Source:         event.Source,
		Campaign:       event.Campaign,
		Link:           event.Link,
		EventIndex:     event.EventIndex,
		Token:          event.Token,
		Kind:           event.Kind,
		IP:             event.IP,
		UserAgent:      event.UserAgent,
		Referer:        event.Referer,
		AcceptLanguage: event.AcceptLanguage,
		ForwardedFor:   event.ForwardedFor,
		RawPayload:     string(raw),
		TriggeredAt:    event.TriggeredAt,
	}
	_, err = s.runCommand(ctx, s.events, "INSERT", map[string]any{
		"insert":    s.events,
		"documents": []any{doc},
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *TCBStore) ListEvents(ctx context.Context, filter EventFilter) ([]trackingcloud.Event, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	docs, err := s.find(ctx, s.events, tcbFilter(filter), map[string]any{"id": 1}, limit)
	if err != nil {
		return nil, err
	}
	items := make([]trackingcloud.Event, 0, len(docs))
	for _, raw := range docs {
		var doc tcbEventDoc
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, err
		}
		items = append(items, eventFromTCBDoc(doc))
	}
	return items, nil
}

func (s *TCBStore) Stats(ctx context.Context, filter EventFilter) (StatsResult, error) {
	filter.Limit = 5000
	items, err := s.ListEvents(ctx, filter)
	if err != nil {
		return StatsResult{}, err
	}
	byKind := map[string]struct {
		Count  int
		Tokens map[string]struct{}
	}{}
	trend := map[string]map[string]int{}
	for _, event := range items {
		row := byKind[event.Kind]
		if row.Tokens == nil {
			row.Tokens = map[string]struct{}{}
		}
		row.Count++
		if event.Token != "" {
			row.Tokens[event.Token] = struct{}{}
		}
		byKind[event.Kind] = row

		hour := parseEventTime(event.TriggeredAt).UTC().Format("2006-01-02 15:00")
		if trend[hour] == nil {
			trend[hour] = map[string]int{}
		}
		trend[hour][event.Kind]++
	}
	summary := make([]map[string]any, 0, len(byKind))
	for kind, row := range byKind {
		summary = append(summary, map[string]any{"kind": kind, "count": row.Count, "unique_tokens": len(row.Tokens)})
	}
	sort.Slice(summary, func(i, j int) bool {
		return fmt.Sprint(summary[i]["kind"]) < fmt.Sprint(summary[j]["kind"])
	})
	trendRows := make([]map[string]any, 0)
	hours := make([]string, 0, len(trend))
	for hour := range trend {
		hours = append(hours, hour)
	}
	sort.Strings(hours)
	for _, hour := range hours {
		kinds := make([]string, 0, len(trend[hour]))
		for kind := range trend[hour] {
			kinds = append(kinds, kind)
		}
		sort.Strings(kinds)
		for _, kind := range kinds {
			trendRows = append(trendRows, map[string]any{"hour": hour, "kind": kind, "count": trend[hour][kind]})
		}
	}
	return StatsResult{Summary: summary, Trend: trendRows}, nil
}

func (s *TCBStore) SaveAsset(ctx context.Context, asset Asset) error {
	if asset.CreatedAt.IsZero() {
		asset.CreatedAt = time.Now().UTC()
	}
	doc := tcbAssetDoc{
		Name:        asset.Name,
		Label:       asset.Label,
		ContentType: asset.ContentType,
		DataBase64:  base64.StdEncoding.EncodeToString(asset.Data),
		Width:       asset.Width,
		CreatedAt:   asset.CreatedAt.UTC().Format(time.RFC3339),
	}
	_, err := s.runCommand(ctx, s.assets, "INSERT", map[string]any{
		"insert":    s.assets,
		"documents": []any{doc},
	})
	return err
}

func (s *TCBStore) GetAsset(ctx context.Context, name string) (Asset, error) {
	docs, err := s.find(ctx, s.assets, map[string]any{"name": name}, map[string]any{}, 1)
	if err != nil {
		return Asset{}, err
	}
	if len(docs) == 0 {
		return Asset{}, ErrNotFound
	}
	var doc tcbAssetDoc
	if err := json.Unmarshal(docs[0], &doc); err != nil {
		return Asset{}, err
	}
	data, err := base64.StdEncoding.DecodeString(doc.DataBase64)
	if err != nil {
		return Asset{}, err
	}
	return Asset{
		Name:        doc.Name,
		Label:       doc.Label,
		ContentType: doc.ContentType,
		Data:        data,
		Width:       doc.Width,
		CreatedAt:   parseEventTime(doc.CreatedAt),
	}, nil
}

func (s *TCBStore) nextID(ctx context.Context, name string) (int64, error) {
	results, err := s.runCommand(ctx, "tracking_counters", "UPDATE", map[string]any{
		"findAndModify": "tracking_counters",
		"query":         map[string]any{"_id": name},
		"update":        map[string]any{"$inc": map[string]any{"seq": 1}},
		"upsert":        true,
		"new":           true,
	})
	if err != nil {
		return 0, err
	}
	for _, raw := range results {
		var payload struct {
			Value map[string]any `json:"value"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			continue
		}
		if seq, ok := int64Value(payload.Value["seq"]); ok {
			return seq, nil
		}
	}
	return 0, errors.New("tcb counter response missing value.seq")
}

func (s *TCBStore) find(ctx context.Context, table string, filter map[string]any, sortFields map[string]any, limit int64) ([]json.RawMessage, error) {
	command := map[string]any{
		"find":   table,
		"filter": filter,
		"limit":  limit,
	}
	if len(sortFields) > 0 {
		command["sort"] = sortFields
	}
	results, err := s.runCommand(ctx, table, "QUERY", command)
	if err != nil {
		return nil, err
	}
	return commandDocuments(results)
}

func (s *TCBStore) runCommand(ctx context.Context, table, commandType string, command any) ([]json.RawMessage, error) {
	data, err := json.Marshal(command)
	if err != nil {
		return nil, err
	}
	req := tcb.NewRunCommandsRequest()
	req.EnvId = strPtr(s.envID)
	req.MgoCommands = []*tcb.MgoCommandParam{{
		TableName:   strPtr(table),
		CommandType: strPtr(commandType),
		Command:     strPtr(string(data)),
	}}
	resp, err := s.client.RunCommandsWithContext(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Response == nil {
		return nil, errors.New("empty tcb RunCommands response")
	}
	results := make([]json.RawMessage, 0, len(resp.Response.Data))
	for _, item := range resp.Response.Data {
		if item == nil || strings.TrimSpace(*item) == "" {
			continue
		}
		results = append(results, json.RawMessage(*item))
	}
	return results, nil
}

func tcbFilter(filter EventFilter) map[string]any {
	result := map[string]any{}
	if filter.Source != "" {
		result["source"] = filter.Source
	}
	if filter.Campaign != "" {
		result["campaign"] = filter.Campaign
	}
	if filter.Kind != "" {
		result["kind"] = filter.Kind
	}
	if filter.Since != "" {
		result["triggered_at"] = map[string]any{"$gte": filter.Since}
	}
	if filter.AfterID > 0 {
		result["id"] = map[string]any{"$gt": filter.AfterID}
	}
	return result
}

func commandDocuments(results []json.RawMessage) ([]json.RawMessage, error) {
	docs := []json.RawMessage{}
	for _, raw := range results {
		var direct []json.RawMessage
		if err := json.Unmarshal(raw, &direct); err == nil {
			docs = append(docs, direct...)
			continue
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, err
		}
		if cursorRaw, ok := payload["cursor"]; ok {
			var cursor map[string]json.RawMessage
			if err := json.Unmarshal(cursorRaw, &cursor); err != nil {
				return nil, err
			}
			for _, key := range []string{"firstBatch", "nextBatch", "batch"} {
				if batchRaw, ok := cursor[key]; ok {
					var batch []json.RawMessage
					if err := json.Unmarshal(batchRaw, &batch); err != nil {
						return nil, err
					}
					docs = append(docs, batch...)
				}
			}
			continue
		}
		for _, key := range []string{"documents", "data", "items"} {
			if batchRaw, ok := payload[key]; ok {
				var batch []json.RawMessage
				if err := json.Unmarshal(batchRaw, &batch); err == nil {
					docs = append(docs, batch...)
				}
			}
		}
	}
	return docs, nil
}

func eventFromTCBDoc(doc tcbEventDoc) trackingcloud.Event {
	return trackingcloud.Event{
		ID:             doc.ID,
		Source:         doc.Source,
		Campaign:       doc.Campaign,
		Link:           doc.Link,
		EventIndex:     doc.EventIndex,
		Token:          doc.Token,
		Kind:           doc.Kind,
		TriggeredAt:    doc.TriggeredAt,
		IP:             doc.IP,
		UserAgent:      doc.UserAgent,
		Referer:        doc.Referer,
		AcceptLanguage: doc.AcceptLanguage,
		ForwardedFor:   doc.ForwardedFor,
	}
}

func createIndexesCommand(table string, indexes []map[string]any) map[string]any {
	return map[string]any{"createIndexes": table, "indexes": indexes}
}

func indexAlreadyExists(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "already") || strings.Contains(message, "exists")
}

func int64Value(value any) (int64, bool) {
	switch v := value.(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	case json.Number:
		parsed, err := v.Int64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func strPtr(value string) *string {
	return &value
}

func tcbValueOr(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
