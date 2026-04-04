package store

import (
	"context"
	"errors"
	"fmt"

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
