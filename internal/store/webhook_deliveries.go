package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"orchestrator/internal/dbscan"
	"orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.CreateWebhookDelivery")
	defer span.End()

	if d.ID == "" {
		d.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO webhook_deliveries (id, run_id, job_id, webhook_url, status, attempts, max_attempts, last_status_code, last_error, next_retry_at, delivered_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at, updated_at`

	return q.db.QueryRow(ctx, query,
		d.ID, d.RunID, d.JobID, d.WebhookURL, d.Status, d.Attempts, d.MaxAttempts,
		d.LastStatusCode, dbscan.NilIfEmptyString(d.LastError), d.NextRetryAt, d.DeliveredAt,
	).Scan(&d.CreatedAt, &d.UpdatedAt)
}

func (q *Queries) UpdateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.UpdateWebhookDelivery")
	defer span.End()

	query := `
		UPDATE webhook_deliveries
		SET status = $1, attempts = $2, last_status_code = $3, last_error = $4,
			next_retry_at = $5, delivered_at = $6, updated_at = NOW()
		WHERE id = $7
		RETURNING updated_at`

	return q.db.QueryRow(ctx, query,
		d.Status, d.Attempts, d.LastStatusCode, dbscan.NilIfEmptyString(d.LastError),
		d.NextRetryAt, d.DeliveredAt, d.ID,
	).Scan(&d.UpdatedAt)
}

func (q *Queries) GetWebhookDelivery(ctx context.Context, id string) (*domain.WebhookDelivery, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.GetWebhookDelivery")
	defer span.End()

	query := `SELECT id, run_id, job_id, webhook_url, status, attempts, max_attempts,
					 last_status_code, last_error, next_retry_at, delivered_at, created_at, updated_at
			  FROM webhook_deliveries WHERE id = $1`

	d, err := scanWebhookDelivery(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("webhook delivery not found")
		}
		return nil, fmt.Errorf("get webhook delivery: %w", err)
	}
	return d, nil
}

func (q *Queries) ListWebhookDeliveries(ctx context.Context, projectID, status string, limit int, cursor *time.Time) ([]domain.WebhookDelivery, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListWebhookDeliveries")
	defer span.End()

	baseQuery := `SELECT id, run_id, job_id, webhook_url, status, attempts, max_attempts,
					 last_status_code, last_error, next_retry_at, delivered_at, created_at, updated_at
				  FROM webhook_deliveries
				  WHERE job_id IN (SELECT id FROM jobs WHERE project_id = $1)`
	args := []any{projectID}
	param := 2

	if status != "" {
		baseQuery += fmt.Sprintf(" AND status = $%d", param)
		args = append(args, status)
		param++
	}

	if cursor != nil {
		baseQuery += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	baseQuery += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("list webhook deliveries: %w", err)
	}
	defer rows.Close()

	deliveries := make([]domain.WebhookDelivery, 0, limit)
	for rows.Next() {
		d, err := scanWebhookDelivery(rows)
		if err != nil {
			return nil, fmt.Errorf("list webhook deliveries scan: %w", err)
		}
		deliveries = append(deliveries, *d)
	}
	return deliveries, rows.Err()
}

func (q *Queries) ListPendingWebhookRetries(ctx context.Context) ([]domain.WebhookDelivery, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListPendingWebhookRetries")
	defer span.End()

	query := `SELECT id, run_id, job_id, webhook_url, status, attempts, max_attempts,
					 last_status_code, last_error, next_retry_at, delivered_at, created_at, updated_at
			  FROM webhook_deliveries
			  WHERE status = 'pending' AND next_retry_at IS NOT NULL AND next_retry_at <= NOW()
			  ORDER BY next_retry_at ASC`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list pending webhook retries: %w", err)
	}
	defer rows.Close()

	deliveries := make([]domain.WebhookDelivery, 0, 16)
	for rows.Next() {
		d, err := scanWebhookDelivery(rows)
		if err != nil {
			return nil, fmt.Errorf("list pending webhook retries scan: %w", err)
		}
		deliveries = append(deliveries, *d)
	}
	return deliveries, rows.Err()
}

func scanWebhookDelivery(scanner scanTarget) (*domain.WebhookDelivery, error) {
	var d domain.WebhookDelivery
	var lastError *string

	err := scanner.Scan(
		&d.ID, &d.RunID, &d.JobID, &d.WebhookURL, &d.Status,
		&d.Attempts, &d.MaxAttempts, &d.LastStatusCode, &lastError,
		&d.NextRetryAt, &d.DeliveredAt, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if lastError != nil {
		d.LastError = *lastError
	}
	return &d, nil
}
