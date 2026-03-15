package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

type runWebhookPayload struct {
	RunID     string          `json:"run_id"`
	JobID     string          `json:"job_id"`
	ProjectID string          `json:"project_id"`
	Status    string          `json:"status"`
	Attempt   int             `json:"attempt"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

func (q *Queries) CreateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateWebhookDelivery")
	defer span.End()

	if d.ID == "" {
		d.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO webhook_deliveries (id, run_id, job_id, webhook_url, webhook_retry_policy, status, attempts, max_attempts, last_status_code, last_error, next_retry_at, delivered_at, event_trigger_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING created_at, updated_at`

	return q.db.QueryRow(ctx, query,
		d.ID,
		dbscan.NilIfEmptyString(d.RunID),
		dbscan.NilIfEmptyString(d.JobID),
		d.WebhookURL,
		dbscan.NilIfEmptyString(d.RetryPolicy),
		d.Status, d.Attempts, d.MaxAttempts,
		d.LastStatusCode, dbscan.NilIfEmptyString(d.LastError), d.NextRetryAt, d.DeliveredAt,
		dbscan.NilIfEmptyString(d.EventTriggerID),
	).Scan(&d.CreatedAt, &d.UpdatedAt)
}

func (q *Queries) EnqueueRunWebhook(ctx context.Context, job *domain.Job, run *domain.JobRun, maxAttempts int) (*domain.WebhookDelivery, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.EnqueueRunWebhook")
	defer span.End()

	if job == nil {
		return nil, fmt.Errorf("enqueue run webhook: nil job")
	}
	if run == nil {
		return nil, fmt.Errorf("enqueue run webhook: nil run")
	}
	if run.ID == "" {
		return nil, fmt.Errorf("enqueue run webhook: missing run id")
	}
	if run.JobID == "" {
		return nil, fmt.Errorf("enqueue run webhook: missing job id")
	}
	if job.WebhookURL == "" {
		return nil, fmt.Errorf("enqueue run webhook: missing webhook url")
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	payload, err := json.Marshal(runWebhookPayload{
		RunID:     run.ID,
		JobID:     run.JobID,
		ProjectID: run.ProjectID,
		Status:    string(run.Status),
		Attempt:   run.Attempt,
		Result:    run.Result,
		Error:     run.Error,
		Timestamp: time.Now().UTC(),
	})
	if err != nil {
		return nil, fmt.Errorf("enqueue run webhook: marshal payload: %w", err)
	}

	now := time.Now().UTC()
	d := &domain.WebhookDelivery{
		ID:          uuid.Must(uuid.NewV7()).String(),
		RunID:       run.ID,
		JobID:       run.JobID,
		WebhookURL:  job.WebhookURL,
		RetryPolicy: domain.WebhookRetryPolicyExponential,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: maxAttempts,
		NextRetryAt: &now,
	}

	query := `
		INSERT INTO webhook_deliveries (
			id, run_id, job_id, webhook_url, webhook_retry_policy, status, attempts, max_attempts, next_retry_at,
			webhook_secret, payload, payload_size_bytes, event_type
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, octet_length($11::jsonb::text), $12)
		RETURNING created_at, updated_at`

	err = q.db.QueryRow(
		ctx,
		query,
		d.ID,
		d.RunID,
		d.JobID,
		d.WebhookURL,
		d.RetryPolicy,
		d.Status,
		d.Attempts,
		d.MaxAttempts,
		d.NextRetryAt,
		dbscan.NilIfEmptyString(job.WebhookSecret),
		payload,
		fmt.Sprintf("run.%s", run.Status),
	).Scan(&d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("enqueue run webhook: %w", err)
	}

	return d, nil
}

func (q *Queries) UpdateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateWebhookDelivery")
	defer span.End()

	query := `
		UPDATE webhook_deliveries
		SET webhook_retry_policy = $1, status = $2, attempts = $3, last_status_code = $4, last_error = $5,
			next_retry_at = $6, delivered_at = $7, updated_at = NOW()
		WHERE id = $8
		RETURNING updated_at`

	return q.db.QueryRow(ctx, query,
		dbscan.NilIfEmptyString(d.RetryPolicy),
		d.Status, d.Attempts, d.LastStatusCode, dbscan.NilIfEmptyString(d.LastError),
		d.NextRetryAt, d.DeliveredAt, d.ID,
	).Scan(&d.UpdatedAt)
}

func (q *Queries) GetWebhookDelivery(ctx context.Context, id string) (*domain.WebhookDelivery, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWebhookDelivery")
	defer span.End()

	query := `SELECT id, run_id, job_id, webhook_url, webhook_retry_policy, status, attempts, max_attempts,
					 last_status_code, last_error, next_retry_at, delivered_at, created_at, updated_at,
					 event_trigger_id
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

func (q *Queries) RetryWebhookDelivery(ctx context.Context, id string) (*domain.WebhookDelivery, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RetryWebhookDelivery")
	defer span.End()

	now := time.Now().UTC()
	query := `
		UPDATE webhook_deliveries
		SET status = 'pending', attempts = 0, last_status_code = NULL, last_error = NULL,
			next_retry_at = $1, delivered_at = NULL, updated_at = NOW()
		WHERE id = $2 AND status IN ('failed', 'dead')
		RETURNING id, run_id, job_id, webhook_url, webhook_retry_policy, status, attempts, max_attempts,
			last_status_code, last_error, next_retry_at, delivered_at, created_at, updated_at, event_trigger_id`

	d, err := scanWebhookDelivery(q.db.QueryRow(ctx, query, now, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("webhook delivery not retriable")
		}
		return nil, fmt.Errorf("retry webhook delivery: %w", err)
	}

	return d, nil
}

func (q *Queries) ListWebhookDeliveries(ctx context.Context, projectID, status string, limit int, cursor *time.Time) ([]domain.WebhookDelivery, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListWebhookDeliveries")
	defer span.End()

	baseQuery := `SELECT wd.id, wd.run_id, wd.job_id, wd.webhook_url, wd.webhook_retry_policy, wd.status, wd.attempts, wd.max_attempts,
					 wd.last_status_code, wd.last_error, wd.next_retry_at, wd.delivered_at, wd.created_at, wd.updated_at,
					 wd.event_trigger_id
				  FROM webhook_deliveries wd
				  LEFT JOIN jobs j ON wd.job_id = j.id
				  WHERE (j.project_id = $1 OR wd.job_id IS NULL)`
	args := []any{projectID}
	param := 2

	if status != "" {
		baseQuery += fmt.Sprintf(" AND wd.status = $%d", param)
		args = append(args, status)
		param++
	}

	if cursor != nil {
		baseQuery += fmt.Sprintf(" AND wd.created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	baseQuery += fmt.Sprintf(" ORDER BY wd.created_at DESC LIMIT $%d", param)
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
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListPendingWebhookRetries")
	defer span.End()

	query := `SELECT id, run_id, job_id, webhook_url, webhook_retry_policy, status, attempts, max_attempts,
					 last_status_code, last_error, next_retry_at, delivered_at, created_at, updated_at,
					 event_trigger_id
			  FROM webhook_deliveries
			  WHERE status = 'pending' AND next_retry_at IS NOT NULL AND next_retry_at <= NOW()
			  ORDER BY next_retry_at ASC
			  LIMIT 100`

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

func (q *Queries) ListPendingRunWebhookDeliveries(ctx context.Context) ([]domain.WebhookDelivery, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListPendingRunWebhookDeliveries")
	defer span.End()

	query := `SELECT id, run_id, job_id, webhook_url, webhook_retry_policy, status, attempts, max_attempts,
					 last_status_code, last_error, next_retry_at, delivered_at, created_at, updated_at,
					 event_trigger_id
			  FROM webhook_deliveries
			  WHERE status = 'pending'
			    AND next_retry_at IS NOT NULL
			    AND next_retry_at <= NOW()
			    AND run_id IS NOT NULL
			    AND event_trigger_id IS NULL
			  ORDER BY next_retry_at ASC
			  LIMIT 100`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list pending run webhook deliveries: %w", err)
	}
	defer rows.Close()

	deliveries := make([]domain.WebhookDelivery, 0, 16)
	for rows.Next() {
		d, err := scanWebhookDelivery(rows)
		if err != nil {
			return nil, fmt.Errorf("list pending run webhook deliveries scan: %w", err)
		}
		deliveries = append(deliveries, *d)
	}
	return deliveries, rows.Err()
}

// DeleteOldWebhookDeliveries removes delivered/dead deliveries older than the given time.
func (q *Queries) DeleteOldWebhookDeliveries(ctx context.Context, before time.Time, limit int) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteOldWebhookDeliveries")
	defer span.End()

	if limit <= 0 {
		limit = 1000
	}

	query := `
		DELETE FROM webhook_deliveries
		WHERE id IN (
			SELECT id FROM webhook_deliveries
			WHERE status IN ('delivered', 'dead') AND created_at < $1
			ORDER BY created_at ASC
			LIMIT $2
		)`

	tag, err := q.db.Exec(ctx, query, before, limit)
	if err != nil {
		return 0, fmt.Errorf("delete old webhook deliveries: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func scanWebhookDelivery(scanner scanTarget) (*domain.WebhookDelivery, error) {
	var d domain.WebhookDelivery
	var lastError *string
	var runID *string
	var jobID *string
	var retryPolicy *string
	var eventTriggerID *string

	err := scanner.Scan(
		&d.ID, &runID, &jobID, &d.WebhookURL, &retryPolicy, &d.Status,
		&d.Attempts, &d.MaxAttempts, &d.LastStatusCode, &lastError,
		&d.NextRetryAt, &d.DeliveredAt, &d.CreatedAt, &d.UpdatedAt,
		&eventTriggerID,
	)
	if err != nil {
		return nil, err
	}
	if lastError != nil {
		d.LastError = *lastError
	}
	if runID != nil {
		d.RunID = *runID
	}
	if jobID != nil {
		d.JobID = *jobID
	}
	if retryPolicy != nil {
		d.RetryPolicy = *retryPolicy
	}
	if eventTriggerID != nil {
		d.EventTriggerID = *eventTriggerID
	}
	return &d, nil
}

// ReplayWebhookDelivery creates a new delivery with the same payload as the original.
func (q *Queries) ReplayWebhookDelivery(ctx context.Context, id string) (*domain.WebhookDelivery, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ReplayWebhookDelivery")
	defer span.End()

	newID := uuid.Must(uuid.NewV7()).String()
	query := `
		INSERT INTO webhook_deliveries (
			id, run_id, job_id, webhook_url, webhook_retry_policy, status, attempts, max_attempts,
			next_retry_at, webhook_secret, payload, payload_size_bytes, event_type, event_trigger_id
		)
		SELECT $1, run_id, job_id, webhook_url, webhook_retry_policy, 'pending', 0, max_attempts,
		       NOW(), webhook_secret, payload, payload_size_bytes, event_type, event_trigger_id
		FROM webhook_deliveries
		WHERE id = $2
		RETURNING id, run_id, job_id, webhook_url, webhook_retry_policy, status, attempts, max_attempts,
		          last_status_code, last_error, next_retry_at, delivered_at, created_at, updated_at,
		          event_trigger_id`

	d, err := scanWebhookDelivery(q.db.QueryRow(ctx, query, newID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("webhook delivery not found")
		}
		return nil, fmt.Errorf("replay webhook delivery: %w", err)
	}
	return d, nil
}

func (q *Queries) ResetStuckWebhookDeliveries(ctx context.Context) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ResetStuckWebhookDeliveries")
	defer span.End()

	query := `
		UPDATE webhook_deliveries SET next_retry_at = NOW()
		WHERE status = 'pending' AND next_retry_at < NOW() - interval '5 minutes'`

	tag, err := q.db.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("reset stuck webhook deliveries: %w", err)
	}
	return tag.RowsAffected(), nil
}
