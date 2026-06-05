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

	// project_id is derived at insert time from whichever FK is populated.
	// The COALESCE mirrors the backfill precedence used by migration 000183.
	// Rows with no resolvable parent get the '__orphaned__' sentinel so the
	// RLS policy never sees a NULL and the reaper can find them later.
	payload := webhookDeliveryPayload(d)
	var payloadArg any
	if len(payload) > 0 {
		payloadArg = payload
	}
	query := `
		INSERT INTO webhook_deliveries (id, run_id, job_id, webhook_url, webhook_retry_policy, status, attempts, max_attempts, last_status_code, last_error, next_retry_at, delivered_at, event_trigger_id, subscription_id, webhook_secret, payload, payload_size_bytes, dedupe_key, project_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16::jsonb, COALESCE(octet_length($16::jsonb::text), 0), NULLIF($17, ''),
			COALESCE(
				(SELECT jr.project_id FROM job_runs jr WHERE jr.id = $2),
				(SELECT et.project_id FROM event_triggers et WHERE et.id = $13),
				(SELECT j.project_id  FROM jobs j           WHERE j.id  = $3),
				(SELECT ws.project_id FROM webhook_subscriptions ws WHERE ws.id = $14),
				'__orphaned__'
			))
		ON CONFLICT (dedupe_key) WHERE dedupe_key IS NOT NULL AND dedupe_key <> '' DO NOTHING
		RETURNING created_at, updated_at`

	err := q.db.QueryRow(ctx, query,
		d.ID,
		dbscan.NilIfEmptyString(d.RunID),
		dbscan.NilIfEmptyString(d.JobID),
		d.WebhookURL,
		dbscan.NilIfEmptyString(d.RetryPolicy),
		d.Status, d.Attempts, d.MaxAttempts,
		d.LastStatusCode, dbscan.NilIfEmptyString(d.LastError), d.NextRetryAt, d.DeliveredAt,
		dbscan.NilIfEmptyString(d.EventTriggerID),
		dbscan.NilIfEmptyString(d.SubscriptionID),
		dbscan.NilIfEmptyString(d.WebhookSecret),
		payloadArg,
		d.DedupeKey,
	).Scan(&d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) && d.DedupeKey != "" {
			return nil
		}
		return fmt.Errorf("create webhook delivery: %w", err)
	}
	return nil
}

func webhookDeliveryPayload(d *domain.WebhookDelivery) json.RawMessage {
	if d == nil {
		return nil
	}
	if len(d.Payload) > 0 {
		return d.Payload
	}
	if d.LastError == "" {
		return nil
	}
	var payload json.RawMessage
	if json.Unmarshal([]byte(d.LastError), &payload) == nil {
		return payload
	}
	return nil
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

	d := &domain.WebhookDelivery{
		ID:            uuid.Must(uuid.NewV7()).String(),
		RunID:         run.ID,
		JobID:         run.JobID,
		WebhookURL:    job.WebhookURL,
		WebhookSecret: job.WebhookSecret,
		RetryPolicy:   domain.WebhookRetryPolicyExponential,
		Status:        domain.WebhookStatusPending,
		Attempts:      0,
		MaxAttempts:   maxAttempts,
	}

	// EnqueueRunWebhook has the run in scope so project_id is known
	// upfront — pass it as an explicit parameter rather than using the
	// COALESCE subquery that CreateWebhookDelivery relies on.
	query := `
		INSERT INTO webhook_deliveries (
			id, run_id, job_id, webhook_url, webhook_retry_policy, status, attempts, max_attempts, next_retry_at,
			webhook_secret, payload, payload_size_bytes, event_type, project_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9, $10::jsonb, octet_length($10::jsonb::text), $11, $12)
		RETURNING next_retry_at, created_at, updated_at`

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
		dbscan.NilIfEmptyString(job.WebhookSecret),
		payload,
		fmt.Sprintf("run.%s", run.Status),
		run.ProjectID,
	).Scan(&d.NextRetryAt, &d.CreatedAt, &d.UpdatedAt)
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
			next_retry_at = $6, delivered_at = $7, claim_token = NULL, lease_expires_at = NULL, updated_at = NOW()
		WHERE id = $8
		RETURNING updated_at`

	return q.db.QueryRow(ctx, query,
		dbscan.NilIfEmptyString(d.RetryPolicy),
		d.Status, d.Attempts, d.LastStatusCode, dbscan.NilIfEmptyString(d.LastError),
		d.NextRetryAt, d.DeliveredAt, d.ID,
	).Scan(&d.UpdatedAt)
}

func (q *Queries) ClaimPendingWebhookRetries(ctx context.Context, limit int, leaseDuration time.Duration) ([]domain.WebhookDelivery, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ClaimPendingWebhookRetries")
	defer span.End()

	if limit <= 0 {
		limit = 100
	}
	if leaseDuration <= 0 {
		leaseDuration = 2 * time.Minute
	}

	beginner, ok := q.db.(TxBeginner)
	if !ok {
		return nil, fmt.Errorf("claim pending webhook retries: db does not support transactions")
	}

	tx, err := beginner.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("claim pending webhook retries: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx)

	claimToken := uuid.Must(uuid.NewV7()).String()
	leaseExpiry := time.Now().UTC().Add(leaseDuration)
	//nolint:dupword // Claimed delivery scan currently includes project_id in both base and claim columns.
	query := `
		WITH claimable AS (
			SELECT wd.id
			FROM webhook_deliveries wd
			WHERE wd.status = 'pending'
			  AND wd.next_retry_at IS NOT NULL
			  AND wd.next_retry_at <= NOW()
			  AND (
				wd.claim_token IS NULL
				OR wd.lease_expires_at IS NULL
				OR wd.lease_expires_at <= NOW()
			  )
			ORDER BY wd.next_retry_at ASC, wd.created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		),
		claimed AS (
		UPDATE webhook_deliveries wd
		SET claim_token = $2,
		    lease_expires_at = $3,
		    updated_at = NOW()
		FROM claimable
		WHERE wd.id = claimable.id
		RETURNING wd.id, wd.run_id, wd.job_id, wd.webhook_url, wd.webhook_retry_policy, wd.status, wd.attempts, wd.max_attempts,
		          wd.last_status_code, wd.last_error, wd.next_retry_at, wd.delivered_at, wd.created_at, wd.updated_at,
		          wd.event_trigger_id, wd.subscription_id, wd.payload, wd.webhook_secret, COALESCE(wd.project_id, '') AS project_id,
		          wd.claim_token, wd.lease_expires_at
		)
		SELECT claimed.id, claimed.run_id, claimed.job_id, claimed.webhook_url, claimed.webhook_retry_policy, claimed.status,
		       claimed.attempts, claimed.max_attempts, claimed.last_status_code, claimed.last_error, claimed.next_retry_at,
		       claimed.delivered_at, claimed.created_at, claimed.updated_at, claimed.event_trigger_id, claimed.subscription_id,
		       claimed.payload, claimed.webhook_secret, claimed.project_id, claimed.project_id, COALESCE(p.org_id, ''), claimed.claim_token, claimed.lease_expires_at
		FROM claimed
		LEFT JOIN projects p ON p.id = claimed.project_id AND claimed.project_id != '__orphaned__'`

	rows, err := tx.Query(ctx, query, limit, claimToken, leaseExpiry)
	if err != nil {
		return nil, fmt.Errorf("claim pending webhook retries: %w", err)
	}
	defer rows.Close()

	deliveries := make([]domain.WebhookDelivery, 0, limit)
	for rows.Next() {
		d, err := scanClaimedWebhookDeliveryWithOrg(rows)
		if err != nil {
			return nil, fmt.Errorf("claim pending webhook retries scan: %w", err)
		}
		deliveries = append(deliveries, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim pending webhook retries rows: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("claim pending webhook retries: commit tx: %w", err)
	}

	return deliveries, nil
}

func (q *Queries) UpdateClaimedWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) (bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateClaimedWebhookDelivery")
	defer span.End()

	query := `
		UPDATE webhook_deliveries
		SET webhook_retry_policy = $3, status = $4, attempts = $5, last_status_code = $6, last_error = $7,
		    next_retry_at = $8, delivered_at = $9, claim_token = NULL, lease_expires_at = NULL, updated_at = NOW()
		WHERE id = $1 AND claim_token = $2
		RETURNING updated_at`

	err := q.db.QueryRow(ctx, query,
		d.ID, d.ClaimToken,
		dbscan.NilIfEmptyString(d.RetryPolicy),
		d.Status, d.Attempts, d.LastStatusCode, dbscan.NilIfEmptyString(d.LastError),
		d.NextRetryAt, d.DeliveredAt,
	).Scan(&d.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("update claimed webhook delivery: %w", err)
	}

	d.ClaimToken = ""
	d.LeaseExpiresAt = nil
	return true, nil
}

func (q *Queries) GetWebhookDelivery(ctx context.Context, id string) (*domain.WebhookDelivery, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetWebhookDelivery")
	defer span.End()

	query := `SELECT id, run_id, job_id, webhook_url, webhook_retry_policy, status, attempts, max_attempts,
					 last_status_code, last_error, next_retry_at, delivered_at, created_at, updated_at,
					 event_trigger_id, subscription_id, payload, webhook_secret, COALESCE(project_id, '')
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
			last_status_code, last_error, next_retry_at, delivered_at, created_at, updated_at,
			event_trigger_id, subscription_id, payload, webhook_secret, COALESCE(project_id, '')`

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
					 wd.event_trigger_id, wd.subscription_id, wd.payload, wd.webhook_secret, COALESCE(wd.project_id, '')
				  FROM webhook_deliveries wd
				  LEFT JOIN jobs j ON wd.job_id = j.id
				  WHERE (j.project_id = $1 OR wd.project_id = $1)`
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

	query := `SELECT wd.id, wd.run_id, wd.job_id, wd.webhook_url, wd.webhook_retry_policy, wd.status, wd.attempts, wd.max_attempts,
					 wd.last_status_code, wd.last_error, wd.next_retry_at, wd.delivered_at, wd.created_at, wd.updated_at,
					 wd.event_trigger_id, wd.subscription_id, wd.payload, wd.webhook_secret, COALESCE(wd.project_id, ''),
					 COALESCE(wd.project_id, ''), COALESCE(p.org_id, '')
			  FROM webhook_deliveries wd
			  LEFT JOIN projects p ON p.id = wd.project_id AND wd.project_id != '__orphaned__'
			  WHERE wd.status = 'pending' AND wd.next_retry_at IS NOT NULL AND wd.next_retry_at <= NOW()
			    AND (wd.claim_token IS NULL OR wd.lease_expires_at IS NULL OR wd.lease_expires_at <= NOW())
			  ORDER BY wd.next_retry_at ASC
			  LIMIT 100`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list pending webhook retries: %w", err)
	}
	defer rows.Close()

	deliveries := make([]domain.WebhookDelivery, 0, 16)
	for rows.Next() {
		d, err := scanWebhookDeliveryWithOrg(rows)
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
					 event_trigger_id, subscription_id, payload, webhook_secret, COALESCE(project_id, '')
			  FROM webhook_deliveries
			  WHERE status = 'pending'
			    AND next_retry_at IS NOT NULL
			    AND next_retry_at <= NOW()
			    AND run_id IS NOT NULL
			    AND event_trigger_id IS NULL
			    AND (claim_token IS NULL OR lease_expires_at IS NULL OR lease_expires_at <= NOW())
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
	var subscriptionID *string
	var webhookSecret *string
	var payload []byte

	err := scanner.Scan(
		&d.ID, &runID, &jobID, &d.WebhookURL, &retryPolicy, &d.Status,
		&d.Attempts, &d.MaxAttempts, &d.LastStatusCode, &lastError,
		&d.NextRetryAt, &d.DeliveredAt, &d.CreatedAt, &d.UpdatedAt,
		&eventTriggerID, &subscriptionID, &payload, &webhookSecret, &d.ProjectID,
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
	if subscriptionID != nil {
		d.SubscriptionID = *subscriptionID
	}
	if webhookSecret != nil {
		d.WebhookSecret = *webhookSecret
	}
	if len(payload) > 0 {
		d.Payload = append(json.RawMessage(nil), payload...)
	}
	return &d, nil
}

// scanWebhookDeliveryWithOrg scans a delivery row that includes two extra trailing
// columns: project_id and org_id (populated by ListPendingWebhookRetries for
// billing cost recording).
func scanWebhookDeliveryWithOrg(scanner scanTarget) (*domain.WebhookDelivery, error) {
	var d domain.WebhookDelivery
	var lastError *string
	var runID *string
	var jobID *string
	var retryPolicy *string
	var eventTriggerID *string
	var subscriptionID *string
	var webhookSecret *string
	var payload []byte

	err := scanner.Scan(
		&d.ID, &runID, &jobID, &d.WebhookURL, &retryPolicy, &d.Status,
		&d.Attempts, &d.MaxAttempts, &d.LastStatusCode, &lastError,
		&d.NextRetryAt, &d.DeliveredAt, &d.CreatedAt, &d.UpdatedAt,
		&eventTriggerID, &subscriptionID, &payload, &webhookSecret, &d.ProjectID,
		&d.ProjectID, &d.OrgID,
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
	if subscriptionID != nil {
		d.SubscriptionID = *subscriptionID
	}
	if webhookSecret != nil {
		d.WebhookSecret = *webhookSecret
	}
	if len(payload) > 0 {
		d.Payload = append(json.RawMessage(nil), payload...)
	}
	return &d, nil
}

func scanClaimedWebhookDeliveryWithOrg(scanner scanTarget) (*domain.WebhookDelivery, error) {
	d, err := scanWebhookDeliveryWithOrgAndClaim(scanner)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func scanWebhookDeliveryWithOrgAndClaim(scanner scanTarget) (*domain.WebhookDelivery, error) {
	var d domain.WebhookDelivery
	var lastError *string
	var runID *string
	var jobID *string
	var retryPolicy *string
	var eventTriggerID *string
	var subscriptionID *string
	var webhookSecret *string
	var payload []byte
	var claimToken *string

	err := scanner.Scan(
		&d.ID, &runID, &jobID, &d.WebhookURL, &retryPolicy, &d.Status,
		&d.Attempts, &d.MaxAttempts, &d.LastStatusCode, &lastError,
		&d.NextRetryAt, &d.DeliveredAt, &d.CreatedAt, &d.UpdatedAt,
		&eventTriggerID, &subscriptionID, &payload, &webhookSecret, &d.ProjectID,
		&d.ProjectID, &d.OrgID, &claimToken, &d.LeaseExpiresAt,
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
	if subscriptionID != nil {
		d.SubscriptionID = *subscriptionID
	}
	if webhookSecret != nil {
		d.WebhookSecret = *webhookSecret
	}
	if len(payload) > 0 {
		d.Payload = append(json.RawMessage(nil), payload...)
	}
	if claimToken != nil {
		d.ClaimToken = *claimToken
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
			next_retry_at, webhook_secret, payload, payload_size_bytes, event_type, event_trigger_id, subscription_id, project_id
		)
		SELECT $1, run_id, job_id, webhook_url, webhook_retry_policy, 'pending', 0, max_attempts,
		       NOW(), webhook_secret, payload, payload_size_bytes, event_type, event_trigger_id, subscription_id, project_id
		FROM webhook_deliveries
		WHERE id = $2
		RETURNING id, run_id, job_id, webhook_url, webhook_retry_policy, status, attempts, max_attempts,
		          last_status_code, last_error, next_retry_at, delivered_at, created_at, updated_at,
		          event_trigger_id, subscription_id, payload, webhook_secret, COALESCE(project_id, '')`

	d, err := scanWebhookDelivery(q.db.QueryRow(ctx, query, newID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("webhook delivery not found")
		}
		return nil, fmt.Errorf("replay webhook delivery: %w", err)
	}
	return d, nil
}

// CountPendingWebhookDeliveries returns the number of webhook deliveries with status 'pending'.
func (q *Queries) CountPendingWebhookDeliveries(ctx context.Context) (int64, error) {
	var count int64
	err := q.db.QueryRow(ctx,
		"SELECT COUNT(*) FROM webhook_deliveries WHERE status = 'pending'",
	).Scan(&count)
	return count, err
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
