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

func (q *Queries) UpsertNotifySubscriber(ctx context.Context, sub *domain.NotifySubscriber) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpsertNotifySubscriber")
	defer span.End()

	if sub.ID == "" {
		sub.ID = uuid.Must(uuid.NewV7()).String()
	}
	if sub.Locale == "" {
		sub.Locale = "en"
	}
	if sub.Timezone == "" {
		sub.Timezone = "UTC"
	}
	if sub.Status == "" {
		sub.Status = domain.NotifySubscriberStatusActive
	}
	if len(sub.PushTokens) == 0 {
		sub.PushTokens = []byte("[]")
	}
	if len(sub.Attributes) == 0 {
		sub.Attributes = []byte("{}")
	}

	query := `
		INSERT INTO subscribers (
			id, project_id, external_id, email, phone, locale, timezone, push_tokens, attributes, tenant_id, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (project_id, external_id)
		DO UPDATE SET
			email = EXCLUDED.email,
			phone = EXCLUDED.phone,
			locale = EXCLUDED.locale,
			timezone = EXCLUDED.timezone,
			push_tokens = EXCLUDED.push_tokens,
			attributes = EXCLUDED.attributes,
			tenant_id = EXCLUDED.tenant_id,
			status = EXCLUDED.status,
			updated_at = NOW()
		RETURNING id, created_at, updated_at`

	err := q.db.QueryRow(
		ctx,
		query,
		sub.ID,
		sub.ProjectID,
		sub.ExternalID,
		dbscan.NilIfEmptyString(sub.Email),
		dbscan.NilIfEmptyString(sub.Phone),
		sub.Locale,
		sub.Timezone,
		sub.PushTokens,
		sub.Attributes,
		dbscan.NilIfEmptyString(sub.TenantID),
		sub.Status,
	).Scan(&sub.ID, &sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert notify subscriber: %w", err)
	}

	return nil
}

func (q *Queries) GetNotifySubscriber(ctx context.Context, id, projectID string) (*domain.NotifySubscriber, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetNotifySubscriber")
	defer span.End()

	query := `
		SELECT id, project_id, external_id, email, phone, locale, timezone, push_tokens, attributes, tenant_id, status, created_at, updated_at
		FROM subscribers
		WHERE id = $1 AND project_id = $2`

	sub, err := scanNotifySubscriber(q.db.QueryRow(ctx, query, id, projectID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotifySubscriberNotFound
		}
		return nil, fmt.Errorf("get notify subscriber: %w", err)
	}

	return sub, nil
}

func (q *Queries) GetNotifySubscriberByExternalID(ctx context.Context, projectID, externalID string) (*domain.NotifySubscriber, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetNotifySubscriberByExternalID")
	defer span.End()

	query := `
		SELECT id, project_id, external_id, email, phone, locale, timezone, push_tokens, attributes, tenant_id, status, created_at, updated_at
		FROM subscribers
		WHERE project_id = $1 AND external_id = $2`

	sub, err := scanNotifySubscriber(q.db.QueryRow(ctx, query, projectID, externalID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotifySubscriberNotFound
		}
		return nil, fmt.Errorf("get notify subscriber by external ID: %w", err)
	}

	return sub, nil
}

func (q *Queries) ListNotifySubscribers(ctx context.Context, projectID string, tenantID, status *string, limit int, cursor *time.Time) ([]domain.NotifySubscriber, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListNotifySubscribers")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, project_id, external_id, email, phone, locale, timezone, push_tokens, attributes, tenant_id, status, created_at, updated_at
		FROM subscribers
		WHERE project_id = $1
		  AND ($2::text IS NULL OR tenant_id = $2)
		  AND ($3::text IS NULL OR status = $3)
		  AND ($4::timestamptz IS NULL OR created_at < $4)
		ORDER BY created_at DESC
		LIMIT $5`

	var tenantValue any
	if tenantID != nil {
		tenantValue = *tenantID
	}
	var statusValue any
	if status != nil {
		statusValue = *status
	}
	var cursorValue any
	if cursor != nil {
		cursorValue = *cursor
	}

	rows, err := q.db.Query(ctx, query, projectID, tenantValue, statusValue, cursorValue, limit)
	if err != nil {
		return nil, fmt.Errorf("list notify subscribers: %w", err)
	}
	defer rows.Close()

	subs := make([]domain.NotifySubscriber, 0, limit)
	for rows.Next() {
		sub, scanErr := scanNotifySubscriber(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list notify subscribers scan: %w", scanErr)
		}
		subs = append(subs, *sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notify subscribers rows: %w", err)
	}

	return subs, nil
}

func (q *Queries) UpdateNotifySubscriber(ctx context.Context, sub *domain.NotifySubscriber) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateNotifySubscriber")
	defer span.End()

	if len(sub.PushTokens) == 0 {
		sub.PushTokens = []byte("[]")
	}
	if len(sub.Attributes) == 0 {
		sub.Attributes = []byte("{}")
	}
	if sub.Locale == "" {
		sub.Locale = "en"
	}
	if sub.Timezone == "" {
		sub.Timezone = "UTC"
	}
	if sub.Status == "" {
		sub.Status = domain.NotifySubscriberStatusActive
	}

	query := `
		UPDATE subscribers
		SET email = $3,
			phone = $4,
			locale = $5,
			timezone = $6,
			push_tokens = $7,
			attributes = $8,
			tenant_id = $9,
			status = $10,
			updated_at = NOW()
		WHERE id = $1 AND project_id = $2
		RETURNING created_at, updated_at`

	err := q.db.QueryRow(
		ctx,
		query,
		sub.ID,
		sub.ProjectID,
		dbscan.NilIfEmptyString(sub.Email),
		dbscan.NilIfEmptyString(sub.Phone),
		sub.Locale,
		sub.Timezone,
		sub.PushTokens,
		sub.Attributes,
		dbscan.NilIfEmptyString(sub.TenantID),
		sub.Status,
	).Scan(&sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotifySubscriberNotFound
		}
		return fmt.Errorf("update notify subscriber: %w", err)
	}

	return nil
}

func scanNotifySubscriber(scanner scanTarget) (*domain.NotifySubscriber, error) {
	var sub domain.NotifySubscriber
	var email *string
	var phone *string
	var tenantID *string
	var pushTokens []byte
	var attributes []byte

	err := scanner.Scan(
		&sub.ID,
		&sub.ProjectID,
		&sub.ExternalID,
		&email,
		&phone,
		&sub.Locale,
		&sub.Timezone,
		&pushTokens,
		&attributes,
		&tenantID,
		&sub.Status,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if email != nil {
		sub.Email = *email
	}
	if phone != nil {
		sub.Phone = *phone
	}
	if tenantID != nil {
		sub.TenantID = *tenantID
	}
	if len(pushTokens) == 0 {
		sub.PushTokens = []byte("[]")
	} else {
		sub.PushTokens = pushTokens
	}
	if len(attributes) == 0 {
		sub.Attributes = []byte("{}")
	} else {
		sub.Attributes = attributes
	}

	return &sub, nil
}
