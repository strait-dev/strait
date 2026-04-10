package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateNotifyTopic(ctx context.Context, topic *domain.NotifyTopic) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateNotifyTopic")
	defer span.End()

	if topic.ID == "" {
		topic.ID = uuid.Must(uuid.NewV7()).String()
	}
	if len(topic.Attributes) == 0 {
		topic.Attributes = []byte("{}")
	}

	query := `
		INSERT INTO topics (id, project_id, topic_key, name, description, attributes)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at`

	err := q.db.QueryRow(
		ctx,
		query,
		topic.ID,
		topic.ProjectID,
		topic.TopicKey,
		topic.Name,
		dbscan.NilIfEmptyString(topic.Description),
		topic.Attributes,
	).Scan(&topic.CreatedAt)
	if err != nil {
		return fmt.Errorf("create notify topic: %w", err)
	}

	return nil
}

func (q *Queries) GetNotifyTopicByKey(ctx context.Context, projectID, topicKey string) (*domain.NotifyTopic, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetNotifyTopicByKey")
	defer span.End()

	query := `
		SELECT id, project_id, topic_key, name, description, attributes, created_at
		FROM topics
		WHERE project_id = $1 AND topic_key = $2`

	topic, err := scanNotifyTopic(q.db.QueryRow(ctx, query, projectID, topicKey))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotifyTopicNotFound
		}
		return nil, fmt.Errorf("get notify topic by key: %w", err)
	}

	return topic, nil
}

func (q *Queries) ListNotifyTopics(ctx context.Context, projectID string) ([]domain.NotifyTopic, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListNotifyTopics")
	defer span.End()

	query := `
		SELECT id, project_id, topic_key, name, description, attributes, created_at
		FROM topics
		WHERE project_id = $1
		ORDER BY created_at DESC`

	rows, err := q.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list notify topics: %w", err)
	}
	defer rows.Close()

	topics := make([]domain.NotifyTopic, 0, 16)
	for rows.Next() {
		topic, scanErr := scanNotifyTopic(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list notify topics scan: %w", scanErr)
		}
		topics = append(topics, *topic)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notify topics rows: %w", err)
	}

	return topics, nil
}

func (q *Queries) AddNotifyTopicSubscriber(ctx context.Context, topicID, subscriberID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.AddNotifyTopicSubscriber")
	defer span.End()

	query := `
		INSERT INTO topic_memberships (topic_id, subscriber_id)
		VALUES ($1, $2)
		ON CONFLICT (topic_id, subscriber_id) DO NOTHING`

	if _, err := q.db.Exec(ctx, query, topicID, subscriberID); err != nil {
		return fmt.Errorf("add notify topic subscriber: %w", err)
	}

	return nil
}

func (q *Queries) RemoveNotifyTopicSubscriber(ctx context.Context, topicID, subscriberID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RemoveNotifyTopicSubscriber")
	defer span.End()

	query := `DELETE FROM topic_memberships WHERE topic_id = $1 AND subscriber_id = $2`
	if _, err := q.db.Exec(ctx, query, topicID, subscriberID); err != nil {
		return fmt.Errorf("remove notify topic subscriber: %w", err)
	}

	return nil
}

func (q *Queries) ListNotifySubscribersByTopicKey(ctx context.Context, projectID, topicKey string, tenantID *string, limit int) ([]domain.NotifySubscriber, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListNotifySubscribersByTopicKey")
	defer span.End()

	if limit <= 0 {
		limit = 500
	}

	query := `
		SELECT s.id, s.project_id, s.external_id, s.email, s.phone, s.locale, s.timezone, s.push_tokens, s.attributes, s.tenant_id, s.status, s.created_at, s.updated_at
		FROM topics t
		JOIN topic_memberships tm ON tm.topic_id = t.id
		JOIN subscribers s ON s.id = tm.subscriber_id
		WHERE t.project_id = $1
		  AND t.topic_key = $2
		  AND ($3::text IS NULL OR s.tenant_id = $3)
		  AND s.status = 'active'
		ORDER BY s.created_at DESC
		LIMIT $4`

	var tenantValue any
	if tenantID != nil {
		tenantValue = *tenantID
	}

	rows, err := q.db.Query(ctx, query, projectID, topicKey, tenantValue, limit)
	if err != nil {
		return nil, fmt.Errorf("list notify subscribers by topic key: %w", err)
	}
	defer rows.Close()

	subs := make([]domain.NotifySubscriber, 0, limit)
	for rows.Next() {
		sub, scanErr := scanNotifySubscriber(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list notify subscribers by topic key scan: %w", scanErr)
		}
		subs = append(subs, *sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notify subscribers by topic key rows: %w", err)
	}

	return subs, nil
}

// ListNotifySubscribersByTopicKeyCursor returns active subscribers for a topic
// using keyset (cursor) pagination ordered by created_at DESC. Pass nil cursor
// for the first page; pass the created_at of the last subscriber from the
// previous page to advance. Each call returns at most [limit] rows.
func (q *Queries) ListNotifySubscribersByTopicKeyCursor(ctx context.Context, projectID, topicKey string, tenantID *string, limit int, cursor *time.Time) ([]domain.NotifySubscriber, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListNotifySubscribersByTopicKeyCursor")
	defer span.End()

	if limit <= 0 {
		limit = 500
	}

	query := `
		SELECT s.id, s.project_id, s.external_id, s.email, s.phone, s.locale, s.timezone, s.push_tokens, s.attributes, s.tenant_id, s.status, s.created_at, s.updated_at
		FROM topics t
		JOIN topic_memberships tm ON tm.topic_id = t.id
		JOIN subscribers s ON s.id = tm.subscriber_id
		WHERE t.project_id = $1
		  AND t.topic_key = $2
		  AND ($3::text IS NULL OR s.tenant_id = $3)
		  AND s.status = 'active'
		  AND ($4::timestamptz IS NULL OR s.created_at < $4)
		ORDER BY s.created_at DESC
		LIMIT $5`

	var tenantValue any
	if tenantID != nil {
		tenantValue = *tenantID
	}
	var cursorValue any
	if cursor != nil {
		cursorValue = *cursor
	}

	rows, err := q.db.Query(ctx, query, projectID, topicKey, tenantValue, cursorValue, limit)
	if err != nil {
		return nil, fmt.Errorf("list notify subscribers by topic key cursor: %w", err)
	}
	defer rows.Close()

	subs := make([]domain.NotifySubscriber, 0, limit)
	for rows.Next() {
		sub, scanErr := scanNotifySubscriber(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list notify subscribers by topic key cursor scan: %w", scanErr)
		}
		subs = append(subs, *sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notify subscribers by topic key cursor rows: %w", err)
	}

	return subs, nil
}

func scanNotifyTopic(scanner scanTarget) (*domain.NotifyTopic, error) {
	var topic domain.NotifyTopic
	var description *string
	var attributes []byte

	err := scanner.Scan(
		&topic.ID,
		&topic.ProjectID,
		&topic.TopicKey,
		&topic.Name,
		&description,
		&attributes,
		&topic.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if description != nil {
		topic.Description = *description
	}
	if len(attributes) == 0 {
		topic.Attributes = []byte("{}")
	} else {
		topic.Attributes = attributes
	}

	return &topic, nil
}
