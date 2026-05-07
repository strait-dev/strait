package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateWebhookSubscription(ctx context.Context, sub *domain.WebhookSubscription) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWebhookSubscription")
	defer span.End()

	if sub.ID == "" {
		sub.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO webhook_subscriptions (id, project_id, webhook_url, event_types, secret, active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at`

	err := q.db.QueryRow(
		ctx,
		query,
		sub.ID,
		sub.ProjectID,
		sub.WebhookURL,
		sub.EventTypes,
		sub.Secret,
		sub.Active,
	).Scan(&sub.CreatedAt)
	if err != nil {
		return fmt.Errorf("create webhook subscription: %w", err)
	}

	return nil
}

// CreateWebhookSubscriptionWithOrgLimit serializes quota enforcement and row
// creation for active webhook endpoints across all projects in an org. The
// per-org advisory lock prevents concurrent creates from all observing the same
// pre-insert count and overshooting the plan limit.
func (q *Queries) CreateWebhookSubscriptionWithOrgLimit(ctx context.Context, sub *domain.WebhookSubscription, orgID string, maxEndpoints int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWebhookSubscriptionWithOrgLimit")
	defer span.End()

	if _, ok := TxFromContext(ctx); ok {
		return q.createWebhookSubscriptionWithOrgLimitLocked(ctx, sub, orgID, maxEndpoints)
	}
	if _, ok := q.db.(pgx.Tx); ok {
		return q.createWebhookSubscriptionWithOrgLimitLocked(ctx, sub, orgID, maxEndpoints)
	}

	beginner, ok := q.db.(TxBeginner)
	if !ok {
		return q.createWebhookSubscriptionWithOrgLimitLocked(ctx, sub, orgID, maxEndpoints)
	}

	return WithTx(ctx, beginner, func(txq *Queries) error {
		return txq.createWebhookSubscriptionWithOrgLimitLocked(ctx, sub, orgID, maxEndpoints)
	})
}

func (q *Queries) createWebhookSubscriptionWithOrgLimitLocked(ctx context.Context, sub *domain.WebhookSubscription, orgID string, maxEndpoints int) error {
	if maxEndpoints < 0 {
		return q.CreateWebhookSubscription(ctx, sub)
	}

	if _, err := q.db.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "webhook_endpoint_limit:"+orgID); err != nil {
		return fmt.Errorf("lock webhook endpoint limit: %w", err)
	}

	var count int
	if err := q.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM webhook_subscriptions ws
		WHERE ws.project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
		  AND ws.active = TRUE
	`, orgID).Scan(&count); err != nil {
		return fmt.Errorf("count webhook subscriptions before create: %w", err)
	}
	if count >= maxEndpoints {
		return ErrWebhookEndpointLimitExceeded
	}

	return q.CreateWebhookSubscription(ctx, sub)
}

func (q *Queries) ListWebhookSubscriptions(ctx context.Context, projectID string) ([]domain.WebhookSubscription, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWebhookSubscriptions")
	defer span.End()

	query := `
		SELECT id, project_id, webhook_url, event_types, secret, active, created_at
		FROM webhook_subscriptions
		WHERE project_id = $1 AND active = TRUE
		ORDER BY created_at DESC`

	rows, err := q.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list webhook subscriptions: %w", err)
	}
	defer rows.Close()

	subs := make([]domain.WebhookSubscription, 0, 64)
	for rows.Next() {
		var sub domain.WebhookSubscription
		if err := rows.Scan(&sub.ID, &sub.ProjectID, &sub.WebhookURL, &sub.EventTypes, &sub.Secret, &sub.Active, &sub.CreatedAt); err != nil {
			return nil, fmt.Errorf("list webhook subscriptions scan: %w", err)
		}
		subs = append(subs, sub)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list webhook subscriptions rows: %w", err)
	}

	return subs, nil
}

func (q *Queries) DeleteWebhookSubscription(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteWebhookSubscription")
	defer span.End()

	query := `DELETE FROM webhook_subscriptions WHERE id = $1`
	tag, err := q.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete webhook subscription: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrWebhookSubscriptionNotFound
	}

	return nil
}

func (q *Queries) GetWebhookSubscription(ctx context.Context, id string) (*domain.WebhookSubscription, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWebhookSubscription")
	defer span.End()

	query := `
		SELECT id, project_id, webhook_url, event_types, secret, active, created_at
		FROM webhook_subscriptions
		WHERE id = $1`

	var sub domain.WebhookSubscription
	err := q.db.QueryRow(ctx, query, id).Scan(&sub.ID, &sub.ProjectID, &sub.WebhookURL, &sub.EventTypes, &sub.Secret, &sub.Active, &sub.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWebhookSubscriptionNotFound
		}
		return nil, fmt.Errorf("get webhook subscription: %w", err)
	}

	return &sub, nil
}

// RotateWebhookSecret rotates the signing secret for a webhook subscription.
// The current secret is moved to previous_secret, and the new secret takes effect.
// During the grace period, both secrets are available for signing.
func (q *Queries) RotateWebhookSecret(ctx context.Context, id, newSecret string, graceExpiresAt time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RotateWebhookSecret")
	defer span.End()

	query := `
		UPDATE webhook_subscriptions
		SET previous_secret = secret,
		    secret = $2,
		    secret_grace_expires_at = $3
		WHERE id = $1`

	tag, err := q.db.Exec(ctx, query, id, newSecret, graceExpiresAt)
	if err != nil {
		return fmt.Errorf("rotate webhook secret: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrWebhookSubscriptionNotFound
	}
	return nil
}

// GetWebhookSubscriptionSecrets returns the current and previous signing secrets
// for a webhook subscription. Used by the delivery worker to sign payloads.
func (q *Queries) GetWebhookSubscriptionSecrets(ctx context.Context, subscriptionID string) (string, string, *time.Time, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWebhookSubscriptionSecrets")
	defer span.End()

	query := `
		SELECT secret, previous_secret, secret_grace_expires_at
		FROM webhook_subscriptions
		WHERE id = $1`

	var secret string
	var previousSecret *string
	var graceExpiresAt *time.Time

	err := q.db.QueryRow(ctx, query, subscriptionID).Scan(&secret, &previousSecret, &graceExpiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", nil, nil
		}
		return "", "", nil, fmt.Errorf("get webhook subscription secrets: %w", err)
	}

	prev := ""
	if previousSecret != nil {
		prev = *previousSecret
	}
	return secret, prev, graceExpiresAt, nil
}
