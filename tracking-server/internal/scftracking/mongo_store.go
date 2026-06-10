package scftracking

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"nousmail/tracking-server/internal/trackingcloud"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoStore struct {
	client   *mongo.Client
	db       *mongo.Database
	events   *mongo.Collection
	assets   *mongo.Collection
	counters *mongo.Collection
}

type MongoConfig struct {
	URI              string
	Database         string
	EventsCollection string
	AssetsCollection string
}

type eventDoc struct {
	ID             int64     `bson:"id"`
	Source         string    `bson:"source"`
	Campaign       string    `bson:"campaign"`
	Link           string    `bson:"link"`
	EventIndex     string    `bson:"event_index"`
	Token          string    `bson:"token"`
	Kind           string    `bson:"kind"`
	IP             string    `bson:"ip"`
	UserAgent      string    `bson:"user_agent"`
	Referer        string    `bson:"referer"`
	AcceptLanguage string    `bson:"accept_language"`
	ForwardedFor   string    `bson:"forwarded_for"`
	RawPayload     string    `bson:"raw_payload"`
	TriggeredAt    time.Time `bson:"triggered_at"`
}

type assetDoc struct {
	Name        string    `bson:"name"`
	Label       string    `bson:"label"`
	ContentType string    `bson:"content_type"`
	Data        []byte    `bson:"data"`
	Width       int       `bson:"width"`
	CreatedAt   time.Time `bson:"created_at"`
}

func NewMongoStore(ctx context.Context, cfg MongoConfig) (*MongoStore, error) {
	if cfg.Database == "" {
		cfg.Database = "nousmail_tracking"
	}
	if cfg.EventsCollection == "" {
		cfg.EventsCollection = "tracking_events"
	}
	if cfg.AssetsCollection == "" {
		cfg.AssetsCollection = "tracking_assets"
	}
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.URI).SetRetryWrites(true))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, err
	}
	db := client.Database(cfg.Database)
	store := &MongoStore{
		client:   client,
		db:       db,
		events:   db.Collection(cfg.EventsCollection),
		assets:   db.Collection(cfg.AssetsCollection),
		counters: db.Collection("tracking_counters"),
	}
	return store, store.EnsureIndexes(ctx)
}

func (s *MongoStore) Close(ctx context.Context) error {
	return s.client.Disconnect(ctx)
}

func (s *MongoStore) EnsureIndexes(ctx context.Context) error {
	_, err := s.events.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "id", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "source", Value: 1}, {Key: "id", Value: 1}}},
		{Keys: bson.D{{Key: "source", Value: 1}, {Key: "campaign", Value: 1}, {Key: "kind", Value: 1}, {Key: "triggered_at", Value: 1}}},
		{Keys: bson.D{{Key: "source", Value: 1}, {Key: "campaign", Value: 1}, {Key: "event_index", Value: 1}, {Key: "triggered_at", Value: 1}}},
		{Keys: bson.D{{Key: "token", Value: 1}, {Key: "kind", Value: 1}, {Key: "triggered_at", Value: 1}}},
	})
	if err != nil {
		return err
	}
	_, err = s.assets.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return err
}

func (s *MongoStore) RecordEvent(ctx context.Context, event trackingcloud.Event) (int64, error) {
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
	doc := eventDoc{
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
		TriggeredAt:    triggeredAt,
	}
	_, err = s.events.InsertOne(ctx, doc)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *MongoStore) ListEvents(ctx context.Context, filter EventFilter) ([]trackingcloud.Event, error) {
	mongoFilter := mongoFilter(filter, true)
	limit := filter.Limit
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	cursor, err := s.events.Find(ctx, mongoFilter, options.Find().SetSort(bson.D{{Key: "id", Value: 1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	items := []trackingcloud.Event{}
	for cursor.Next(ctx) {
		var doc eventDoc
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		items = append(items, eventFromDoc(doc))
	}
	return items, cursor.Err()
}

func (s *MongoStore) Stats(ctx context.Context, filter EventFilter) (StatsResult, error) {
	mongoFilter := mongoFilter(filter, false)
	summaryCursor, err := s.events.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: mongoFilter}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$kind"},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "tokens", Value: bson.D{{Key: "$addToSet", Value: "$token"}}},
		}}},
		{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "kind", Value: "$_id"},
			{Key: "count", Value: "$count"},
			{Key: "unique_tokens", Value: bson.D{{Key: "$size", Value: bson.D{{Key: "$filter", Value: bson.D{
				{Key: "input", Value: "$tokens"},
				{Key: "as", Value: "token"},
				{Key: "cond", Value: bson.D{{Key: "$ne", Value: bson.A{"$$token", ""}}}},
			}}}}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "kind", Value: 1}}}},
	})
	if err != nil {
		return StatsResult{}, err
	}
	defer summaryCursor.Close(ctx)
	summary := []map[string]any{}
	for summaryCursor.Next(ctx) {
		var row map[string]any
		if err := summaryCursor.Decode(&row); err != nil {
			return StatsResult{}, err
		}
		summary = append(summary, row)
	}
	trendCursor, err := s.events.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: mongoFilter}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{
				{Key: "hour", Value: bson.D{{Key: "$dateToString", Value: bson.D{
					{Key: "format", Value: "%Y-%m-%d %H:00"},
					{Key: "date", Value: "$triggered_at"},
					{Key: "timezone", Value: "UTC"},
				}}}},
				{Key: "kind", Value: "$kind"},
			}},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "hour", Value: "$_id.hour"},
			{Key: "kind", Value: "$_id.kind"},
			{Key: "count", Value: "$count"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "hour", Value: 1}, {Key: "kind", Value: 1}}}},
	})
	if err != nil {
		return StatsResult{}, err
	}
	defer trendCursor.Close(ctx)
	trend := []map[string]any{}
	for trendCursor.Next(ctx) {
		var row map[string]any
		if err := trendCursor.Decode(&row); err != nil {
			return StatsResult{}, err
		}
		trend = append(trend, row)
	}
	return StatsResult{Summary: summary, Trend: trend}, nil
}

func (s *MongoStore) SaveAsset(ctx context.Context, asset Asset) error {
	if asset.CreatedAt.IsZero() {
		asset.CreatedAt = time.Now().UTC()
	}
	_, err := s.assets.InsertOne(ctx, assetDoc{
		Name:        asset.Name,
		Label:       asset.Label,
		ContentType: asset.ContentType,
		Data:        asset.Data,
		Width:       asset.Width,
		CreatedAt:   asset.CreatedAt,
	})
	return err
}

func (s *MongoStore) GetAsset(ctx context.Context, name string) (Asset, error) {
	var doc assetDoc
	err := s.assets.FindOne(ctx, bson.D{{Key: "name", Value: name}}).Decode(&doc)
	if err != nil {
		return Asset{}, err
	}
	return Asset{
		Name:        doc.Name,
		Label:       doc.Label,
		ContentType: doc.ContentType,
		Data:        doc.Data,
		Width:       doc.Width,
		CreatedAt:   doc.CreatedAt,
	}, nil
}

func (s *MongoStore) nextID(ctx context.Context, name string) (int64, error) {
	var result struct {
		Seq int64 `bson:"seq"`
	}
	err := s.counters.FindOneAndUpdate(
		ctx,
		bson.D{{Key: "_id", Value: name}},
		bson.D{{Key: "$inc", Value: bson.D{{Key: "seq", Value: 1}}}},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
	).Decode(&result)
	return result.Seq, err
}

func mongoFilter(filter EventFilter, includeCursor bool) bson.D {
	clauses := bson.D{}
	if filter.Source != "" {
		clauses = append(clauses, bson.E{Key: "source", Value: filter.Source})
	}
	if filter.Campaign != "" {
		clauses = append(clauses, bson.E{Key: "campaign", Value: filter.Campaign})
	}
	if filter.Kind != "" {
		clauses = append(clauses, bson.E{Key: "kind", Value: filter.Kind})
	}
	if filter.Since != "" {
		if since := parseEventTime(filter.Since); !since.IsZero() {
			clauses = append(clauses, bson.E{Key: "triggered_at", Value: bson.D{{Key: "$gte", Value: since}}})
		}
	}
	if includeCursor && filter.AfterID > 0 {
		clauses = append(clauses, bson.E{Key: "id", Value: bson.D{{Key: "$gt", Value: filter.AfterID}}})
	}
	return clauses
}

func eventFromDoc(doc eventDoc) trackingcloud.Event {
	return trackingcloud.Event{
		ID:             doc.ID,
		Source:         doc.Source,
		Campaign:       doc.Campaign,
		Link:           doc.Link,
		EventIndex:     doc.EventIndex,
		Token:          doc.Token,
		Kind:           doc.Kind,
		TriggeredAt:    doc.TriggeredAt.UTC().Format(time.RFC3339),
		IP:             doc.IP,
		UserAgent:      doc.UserAgent,
		Referer:        doc.Referer,
		AcceptLanguage: doc.AcceptLanguage,
		ForwardedFor:   doc.ForwardedFor,
	}
}

func parseEventTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Now().UTC()
	}
	formats := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04"}
	for _, format := range formats {
		if parsed, err := time.Parse(format, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Now().UTC()
}
