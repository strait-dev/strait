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

func (q *Queries) CreateAPIKey(ctx context.Context, key *domain.APIKey) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateAPIKey")
	defer span.End()

	if key.ID == "" {
		key.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO api_keys (id, project_id, org_id, name, key_hash, key_prefix, scopes, expires_at,
		                      environment_id, rotation_interval_days, next_rotation_at, rotation_webhook_url, rotation_webhook_secret)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING created_at`

	var rotationWebhookSecret any
	if len(key.RotationWebhookSecret) > 0 {
		rotationWebhookSecret = key.RotationWebhookSecret
	}

	err := q.db.QueryRow(ctx, query,
		key.ID, key.ProjectID, dbscan.NilIfEmptyString(key.OrgID), key.Name, key.KeyHash, key.KeyPrefix, key.Scopes, key.ExpiresAt,
		dbscan.NilIfEmptyString(key.EnvironmentID), key.RotationIntervalDays, key.NextRotationAt, dbscan.NilIfEmptyString(key.RotationWebhookURL),
		rotationWebhookSecret,
	).Scan(&key.CreatedAt)
	if err != nil {
		return fmt.Errorf("create api key: %w", err)
	}

	return nil
}

func (q *Queries) GetAPIKeyByHash(ctx context.Context, keyHash string) (*domain.APIKey, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAPIKeyByHash")
	defer span.End()

	query := `SELECT id, project_id, org_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at, revoked_at, replaced_by_key_id, grace_expires_at,
	                 rate_limit_requests, rate_limit_window_secs,
	                 environment_id, rotation_interval_days, next_rotation_at, rotation_webhook_url, rotation_webhook_secret
			  FROM api_keys WHERE key_hash = $1`

	key, err := scanAPIKey(q.db.QueryRow(ctx, query, keyHash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("api key not found")
		}
		return nil, fmt.Errorf("get api key by hash: %w", err)
	}

	return key, nil
}

func (q *Queries) ListAPIKeysByProject(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.APIKey, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAPIKeysByProject")
	defer span.End()

	query := `SELECT id, project_id, org_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at, revoked_at, replaced_by_key_id, grace_expires_at,
	                 rate_limit_requests, rate_limit_window_secs,
	                 environment_id, rotation_interval_days, next_rotation_at, rotation_webhook_url, rotation_webhook_secret
			  FROM api_keys WHERE project_id = $1 AND revoked_at IS NULL`

	args := []any{projectID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	keys := make([]domain.APIKey, 0, 8)
	for rows.Next() {
		key, scanErr := scanAPIKey(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list api keys scan: %w", scanErr)
		}
		keys = append(keys, *key)
	}

	return keys, rows.Err()
}

func (q *Queries) ListAPIKeysByOrg(ctx context.Context, orgID string, limit int, cursor *time.Time) ([]domain.APIKey, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAPIKeysByOrg")
	defer span.End()

	query := `SELECT id, project_id, org_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at, revoked_at, replaced_by_key_id, grace_expires_at,
	                 rate_limit_requests, rate_limit_window_secs,
	                 environment_id, rotation_interval_days, next_rotation_at, rotation_webhook_url, rotation_webhook_secret
			  FROM api_keys WHERE org_id = $1 AND revoked_at IS NULL`

	args := []any{orgID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list api keys by org: %w", err)
	}
	defer rows.Close()

	keys := make([]domain.APIKey, 0, 8)
	for rows.Next() {
		key, scanErr := scanAPIKey(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list api keys by org scan: %w", scanErr)
		}
		keys = append(keys, *key)
	}

	return keys, rows.Err()
}

func (q *Queries) RevokeAPIKey(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RevokeAPIKey")
	defer span.End()

	query := `UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL`
	tag, err := q.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("api key not found or already revoked")
	}

	return nil
}

func (q *Queries) TouchAPIKeyLastUsed(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.TouchAPIKeyLastUsed")
	defer span.End()

	query := `UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`
	_, err := q.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("touch api key last used: %w", err)
	}

	return nil
}

func (q *Queries) GetAPIKeyByID(ctx context.Context, id string) (*domain.APIKey, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetAPIKeyByID")
	defer span.End()

	query := `SELECT id, project_id, org_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at, revoked_at, replaced_by_key_id, grace_expires_at,
	                 rate_limit_requests, rate_limit_window_secs,
	                 environment_id, rotation_interval_days, next_rotation_at, rotation_webhook_url, rotation_webhook_secret
			  FROM api_keys WHERE id = $1`

	key, err := scanAPIKey(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("api key not found")
		}
		return nil, fmt.Errorf("get api key by id: %w", err)
	}
	return key, nil
}

func scanAPIKey(scanner scanTarget) (*domain.APIKey, error) {
	var key domain.APIKey
	var orgID *string
	var replacedBy *string
	var rateLimitRequests *int
	var rateLimitWindowSecs *int
	var environmentID *string
	var rotationWebhookURL *string
	var rotationWebhookSecret []byte
	err := scanner.Scan(
		&key.ID, &key.ProjectID, &orgID, &key.Name, &key.KeyHash, &key.KeyPrefix,
		&key.Scopes, &key.ExpiresAt, &key.LastUsedAt, &key.CreatedAt, &key.RevokedAt, &replacedBy, &key.GraceExpiresAt,
		&rateLimitRequests, &rateLimitWindowSecs,
		&environmentID, &key.RotationIntervalDays, &key.NextRotationAt, &rotationWebhookURL, &rotationWebhookSecret,
	)
	if err != nil {
		return nil, err
	}
	if orgID != nil {
		key.OrgID = *orgID
	}
	if replacedBy != nil {
		key.ReplacedByKeyID = *replacedBy
	}
	if rateLimitRequests != nil {
		key.RateLimitRequests = *rateLimitRequests
	}
	if rateLimitWindowSecs != nil {
		key.RateLimitWindowSecs = *rateLimitWindowSecs
	}
	if environmentID != nil {
		key.EnvironmentID = *environmentID
	}
	if rotationWebhookURL != nil {
		key.RotationWebhookURL = *rotationWebhookURL
	}
	key.RotationWebhookSecret = rotationWebhookSecret
	return &key, nil
}

func (q *Queries) ListAPIKeysDueRotation(ctx context.Context) ([]domain.APIKey, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAPIKeysDueRotation")
	defer span.End()

	query := `SELECT id, project_id, org_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at, revoked_at, replaced_by_key_id, grace_expires_at,
	                 rate_limit_requests, rate_limit_window_secs,
	                 environment_id, rotation_interval_days, next_rotation_at, rotation_webhook_url, rotation_webhook_secret
			  FROM api_keys
			  WHERE rotation_interval_days IS NOT NULL
			    AND next_rotation_at <= NOW()
			    AND revoked_at IS NULL
			    AND replaced_by_key_id IS NULL
			  LIMIT 100`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list api keys due rotation: %w", err)
	}
	defer rows.Close()

	keys := make([]domain.APIKey, 0, 8)
	for rows.Next() {
		key, scanErr := scanAPIKey(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list api keys due rotation scan: %w", scanErr)
		}
		keys = append(keys, *key)
	}

	return keys, rows.Err()
}

func (q *Queries) ListAPIKeysExpiringSoon(ctx context.Context, projectID string, withinDays int) ([]domain.APIKey, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAPIKeysExpiringSoon")
	defer span.End()

	query := `SELECT id, project_id, org_id, name, key_hash, key_prefix, scopes, expires_at, last_used_at, created_at, revoked_at, replaced_by_key_id, grace_expires_at,
	                 rate_limit_requests, rate_limit_window_secs,
	                 environment_id, rotation_interval_days, next_rotation_at, rotation_webhook_url, rotation_webhook_secret
			  FROM api_keys
			  WHERE project_id = $1
			    AND revoked_at IS NULL
			    AND (expires_at IS NULL OR expires_at <= NOW() + INTERVAL '1 day' * $2)
			  ORDER BY expires_at ASC NULLS FIRST
			  LIMIT 100`

	rows, err := q.db.Query(ctx, query, projectID, withinDays)
	if err != nil {
		return nil, fmt.Errorf("list api keys expiring soon: %w", err)
	}
	defer rows.Close()

	keys := make([]domain.APIKey, 0, 8)
	for rows.Next() {
		key, scanErr := scanAPIKey(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list api keys expiring soon scan: %w", scanErr)
		}
		keys = append(keys, *key)
	}

	return keys, rows.Err()
}

func (q *Queries) ListRunsByOrg(ctx context.Context, orgID string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListRunsByOrg")
	defer span.End()

	query := `
		SELECT id, job_id, project_id, status, attempt, payload, result, metadata, error, error_class,
		       triggered_by, scheduled_at, started_at, finished_at, heartbeat_at,
		       next_retry_at, expires_at, parent_run_id, priority, idempotency_key, job_version, created_at, workflow_step_run_id, execution_trace, debug_mode, continuation_of, lineage_depth, tags, job_version_id, created_by, batch_id, concurrency_key, execution_mode, is_rollback, replayed_run_id
		FROM job_runs
		WHERE project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)`

	args := []any{orgID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list runs by org: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.JobRun, 0, 8)
	for rows.Next() {
		run, scanErr := dbscan.ScanRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list runs by org scan: %w", scanErr)
		}
		runs = append(runs, *run)
	}

	return runs, rows.Err()
}

func (q *Queries) ListJobsByOrg(ctx context.Context, orgID string, limit int, cursor *time.Time) ([]domain.Job, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListJobsByOrg")
	defer span.End()

	query := `
		SELECT id, project_id, group_id, name, slug, description, cron, payload_schema,
		       tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
		       rate_limit_max, rate_limit_window_secs, dedup_window_secs,
		       enabled, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id, version, version_id, version_policy, backwards_compatible, created_by, updated_by, created_at, updated_at,
		       max_concurrency_per_key, rate_limit_keys, default_run_metadata, retry_priority_boost, dlq_alert_threshold, queue_depth_alert_threshold, poison_pill_threshold, cron_overlap_policy, result_schema, debounce_window_secs, batch_window_secs, batch_max_size, execution_mode, preferred_regions, queue_name, on_complete_trigger_workflow, on_complete_trigger_job, on_complete_payload_mapping, on_failure_trigger_job, on_failure_trigger_workflow, on_failure_payload_mapping, max_tokens_per_run, max_tool_calls_per_run, max_iterations_per_run, allowed_tools, blocked_tools,
		       paused, paused_at, pause_reason, endpoint_signing_secret
		FROM jobs
		WHERE project_id IN (SELECT id FROM projects WHERE org_id = $1 AND deleted_at IS NULL)`

	args := []any{orgID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs by org: %w", err)
	}
	defer rows.Close()

	jobs := make([]domain.Job, 0, 8)
	for rows.Next() {
		job, scanErr := scanJob(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list jobs by org scan: %w", scanErr)
		}
		jobs = append(jobs, *job)
	}

	return jobs, rows.Err()
}

func (q *Queries) MarkAPIKeyRotated(ctx context.Context, oldKeyID, newKeyID string, graceExpiresAt time.Time) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.MarkAPIKeyRotated")
	defer span.End()

	query := `
		UPDATE api_keys
		SET replaced_by_key_id = $2, grace_expires_at = $3
		WHERE id = $1 AND revoked_at IS NULL AND replaced_by_key_id IS NULL`
	tag, err := q.db.Exec(ctx, query, oldKeyID, newKeyID, graceExpiresAt)
	if err != nil {
		return fmt.Errorf("mark api key rotated: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("api key not found, already revoked, or already rotated")
	}
	return nil
}
