package scftracking

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nousmail/tracking-server/internal/trackingcloud"
)

type SQLiteStore struct {
	db       *sql.DB
	assetDir string
}

type SQLiteConfig struct {
	AssetDir string
}

func NewSQLiteStore(db *sql.DB, cfg SQLiteConfig) *SQLiteStore {
	return &SQLiteStore{db: db, assetDir: strings.TrimSpace(cfg.AssetDir)}
}

func (s *SQLiteStore) RecordEvent(ctx context.Context, event trackingcloud.Event) (int64, error) {
	if event.Token == "" && event.Campaign == "" {
		return 0, nil
	}
	triggeredAt := parseEventTime(event.TriggeredAt)
	raw, _ := json.Marshal(event)
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO tracking_events(source,campaign,link,event_index,token,kind,ip,user_agent,referer,accept_language,forwarded_for,raw_payload,triggered_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		event.Source,
		event.Campaign,
		event.Link,
		event.EventIndex,
		event.Token,
		event.Kind,
		event.IP,
		event.UserAgent,
		event.Referer,
		event.AcceptLanguage,
		event.ForwardedFor,
		string(raw),
		triggeredAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *SQLiteStore) ListEvents(ctx context.Context, filter EventFilter) ([]trackingcloud.Event, error) {
	where, args := sqliteFilter(filter, true)
	limit := filter.Limit
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,source,campaign,link,event_index,token,kind,triggered_at,ip,user_agent,referer,accept_language,forwarded_for
		FROM tracking_events `+where+`
		ORDER BY id ASC
		LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []trackingcloud.Event{}
	for rows.Next() {
		var item trackingcloud.Event
		if err := rows.Scan(&item.ID, &item.Source, &item.Campaign, &item.Link, &item.EventIndex, &item.Token, &item.Kind, &item.TriggeredAt, &item.IP, &item.UserAgent, &item.Referer, &item.AcceptLanguage, &item.ForwardedFor); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLiteStore) Stats(ctx context.Context, filter EventFilter) (StatsResult, error) {
	where, args := sqliteFilter(filter, false)
	rows, err := s.db.QueryContext(ctx, `
		SELECT kind, COUNT(*), COUNT(DISTINCT NULLIF(token,'')) unique_tokens
		FROM tracking_events `+where+`
		GROUP BY kind
		ORDER BY kind`, args...)
	if err != nil {
		return StatsResult{}, err
	}
	defer rows.Close()
	summary := []map[string]any{}
	for rows.Next() {
		var kind string
		var count, uniqueTokens int
		if err := rows.Scan(&kind, &count, &uniqueTokens); err != nil {
			return StatsResult{}, err
		}
		summary = append(summary, map[string]any{"kind": kind, "count": count, "unique_tokens": uniqueTokens})
	}
	if err := rows.Err(); err != nil {
		return StatsResult{}, err
	}

	trendRows, err := s.db.QueryContext(ctx, `
		SELECT strftime('%Y-%m-%d %H:00', triggered_at) hour, kind, COUNT(*)
		FROM tracking_events `+where+`
		GROUP BY hour, kind
		ORDER BY hour, kind`, args...)
	if err != nil {
		return StatsResult{}, err
	}
	defer trendRows.Close()
	trend := []map[string]any{}
	for trendRows.Next() {
		var hour, kind string
		var count int
		if err := trendRows.Scan(&hour, &kind, &count); err != nil {
			return StatsResult{}, err
		}
		trend = append(trend, map[string]any{"hour": hour, "kind": kind, "count": count})
	}
	return StatsResult{Summary: summary, Trend: trend}, trendRows.Err()
}

func (s *SQLiteStore) SaveAsset(_ context.Context, asset Asset) error {
	if err := os.MkdirAll(s.trackingAssetDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.trackingAssetDir(), asset.Name), asset.Data, 0o644)
}

func (s *SQLiteStore) GetAsset(_ context.Context, name string) (Asset, error) {
	base := filepath.Base(name)
	if base != name {
		return Asset{}, ErrNotFound
	}
	path := filepath.Join(s.trackingAssetDir(), name)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Asset{}, ErrNotFound
		}
		return Asset{}, err
	}
	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	info, _ := os.Stat(path)
	createdAt := time.Time{}
	if info != nil {
		createdAt = info.ModTime()
	}
	return Asset{Name: name, ContentType: contentType, Data: data, CreatedAt: createdAt}, nil
}

func (s *SQLiteStore) trackingAssetDir() string {
	if value := strings.TrimSpace(os.Getenv("TRACKING_ASSET_DIR")); value != "" {
		return value
	}
	if s.assetDir != "" {
		return s.assetDir
	}
	return filepath.Join("data", "tracking-assets")
}

func sqliteFilter(filter EventFilter, includeCursor bool) (string, []any) {
	clauses := []string{}
	args := []any{}
	if filter.Source != "" {
		clauses = append(clauses, "source=?")
		args = append(args, filter.Source)
	}
	if filter.Campaign != "" {
		clauses = append(clauses, "campaign=?")
		args = append(args, filter.Campaign)
	}
	if filter.Kind != "" {
		clauses = append(clauses, "kind=?")
		args = append(args, filter.Kind)
	}
	if filter.Since != "" {
		clauses = append(clauses, "triggered_at>=?")
		args = append(args, filter.Since)
	}
	if includeCursor && filter.AfterID > 0 {
		clauses = append(clauses, "id>?")
		args = append(args, filter.AfterID)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}
