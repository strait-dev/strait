package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
		// 23505 here can only come from the partial unique index on
		// (project_id, webhook_url) WHERE active=TRUE (migration 000302).
		// Surface a typed sentinel so the API handler can return 409 without
		// replaying the one-shot signing secret in the create response.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrWebhookSubscriptionDuplicate
		}
		return fmt.Errorf("create webhook subscription: %w", err)
	}

	return nil
}

// CreateWebhookSubscriptionWithOrgLimit serializes quota enforcement and row
// creation for active webhook endpoints across all projects in an org. The
// per-org advisory lock prevents concurrent creates from all observing the same
// pre-insert count and overshooting the plan limit.
func (q *Queries) CreateWebhookSubscriptionWithOrgLimit(ctx context.Context, sub *domain.WebhookSubscription, orgID string, maxEndpoints int) error {
	return q.CreateWebhookSubscriptionWithLimits(ctx, sub, orgID, maxEndpoints, -1)
}

// CreateWebhookSubscriptionWithLimits serializes quota enforcement and row
// creation for active webhook endpoints across the org and subscriptions within
// the target project.
func (q *Queries) CreateWebhookSubscriptionWithLimits(ctx context.Context, sub *domain.WebhookSubscription, orgID string, maxEndpoints, maxProjectSubscriptions int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWebhookSubscriptionWithOrgLimit")
	defer span.End()

	if _, ok := TxFromContext(ctx); ok {
		return q.createWebhookSubscriptionWithLimitsLocked(ctx, sub, orgID, maxEndpoints, maxProjectSubscriptions)
	}
	if _, ok := q.db.(pgx.Tx); ok {
		return q.createWebhookSubscriptionWithLimitsLocked(ctx, sub, orgID, maxEndpoints, maxProjectSubscriptions)
	}

	_, ok := q.db.(TxBeginner)
	if !ok {
		return q.createWebhookSubscriptionWithLimitsLocked(ctx, sub, orgID, maxEndpoints, maxProjectSubscriptions)
	}

	return q.withTx(ctx, func(txq *Queries) error {
		return txq.createWebhookSubscriptionWithLimitsLocked(ctx, sub, orgID, maxEndpoints, maxProjectSubscriptions)
	})
}

func (q *Queries) createWebhookSubscriptionWithLimitsLocked(ctx context.Context, sub *domain.WebhookSubscription, orgID string, maxEndpoints, maxProjectSubscriptions int) error {
	if maxEndpoints < 0 && maxProjectSubscriptions < 0 {
		return q.CreateWebhookSubscription(ctx, sub)
	}

	if orgID != "" && maxEndpoints >= 0 {
		if err := q.acquireWebhookEndpointLimitLock(ctx, orgID); err != nil {
			return fmt.Errorf("lock webhook endpoint limit: %w", err)
		}
	}
	if maxProjectSubscriptions >= 0 {
		if err := q.acquireWebhookProjectLimitLock(ctx, sub.ProjectID); err != nil {
			return fmt.Errorf("lock project webhook subscription limit: %w", err)
		}
	}

	if orgID != "" && maxEndpoints >= 0 {
		count, err := q.countWebhookSubscriptionsByOrgIgnoringProjectRLS(ctx, orgID)
		if err != nil {
			return fmt.Errorf("count webhook subscriptions before create: %w", err)
		}
		if count >= maxEndpoints {
			return ErrWebhookEndpointLimitExceeded
		}
	}

	if maxProjectSubscriptions >= 0 {
		count, err := q.CountWebhookSubscriptionsByProject(ctx, sub.ProjectID)
		if err != nil {
			return fmt.Errorf("count project webhook subscriptions before create: %w", err)
		}
		if count >= maxProjectSubscriptions {
			return ErrWebhookProjectLimitExceeded
		}
	}

	return q.CreateWebhookSubscription(ctx, sub)
}

func (q *Queries) acquireWebhookProjectLimitLock(ctx context.Context, projectID string) error {
	lockKey := "webhook_project_limit:" + projectID
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		var locked bool
		if err := q.db.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtext($1))`, lockKey).Scan(&locked); err != nil {
			return err
		}
		if locked {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (q *Queries) acquireWebhookEndpointLimitLock(ctx context.Context, orgID string) error {
	lockKey := "webhook_endpoint_limit:" + orgID
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		var locked bool
		if err := q.db.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtext($1))`, lockKey).Scan(&locked); err != nil {
			return err
		}
		if locked {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (q *Queries) countWebhookSubscriptionsByOrgIgnoringProjectRLS(ctx context.Context, orgID string) (count int, err error) {
	var currentProjectID string
	if err := q.db.QueryRow(ctx, `SELECT COALESCE(current_setting('app.current_project_id', true), '')`).Scan(&currentProjectID); err != nil {
		return 0, fmt.Errorf("read project context before webhook subscription count: %w", err)
	}

	if currentProjectID != "" {
		if _, err := q.db.Exec(ctx, `SELECT set_config('app.current_project_id', '', true)`); err != nil {
			return 0, fmt.Errorf("clear project context for org webhook subscription count: %w", err)
		}
		defer func() {
			if restoreErr := q.SetProjectContext(ctx, currentProjectID); restoreErr != nil {
				restoreErr = fmt.Errorf("restore project context after org webhook subscription count: %w", restoreErr)
				if err != nil {
					err = errors.Join(err, restoreErr)
					return
				}
				err = restoreErr
			}
		}()
	}

	err = q.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM webhook_subscriptions ws
		WHERE ws.project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)
		  AND ws.active = TRUE
	`, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count webhook subscriptions by org: %w", err)
	}
	return count, nil
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

	if _, ok := TxFromContext(ctx); ok {
		return q.deleteWebhookSubscriptionLocked(ctx, id)
	}
	if _, ok := q.db.(pgx.Tx); ok {
		return q.deleteWebhookSubscriptionLocked(ctx, id)
	}

	_, ok := q.db.(TxBeginner)
	if !ok {
		return q.deleteWebhookSubscriptionLocked(ctx, id)
	}

	return q.withTx(ctx, func(txq *Queries) error {
		return txq.deleteWebhookSubscriptionLocked(ctx, id)
	})
}

func (q *Queries) deleteWebhookSubscriptionLocked(ctx context.Context, id string) error {
	if _, err := q.db.Exec(ctx, `UPDATE webhook_deliveries SET subscription_id = NULL WHERE subscription_id = $1`, id); err != nil {
		return fmt.Errorf("detach webhook deliveries from subscription: %w", err)
	}

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
