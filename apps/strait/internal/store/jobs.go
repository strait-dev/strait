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
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
)

const defaultJobQueueName = "default"

func (q *Queries) CreateJob(ctx context.Context, job *domain.Job) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateJob")
	defer span.End()

	if job.ID == "" {
		job.ID = uuid.Must(uuid.NewV7()).String()
	}
	job.Version = 1

	if job.VersionID == "" {
		job.VersionID = domain.NewVersionID()
	}
	if job.VersionPolicy == "" {
		job.VersionPolicy = domain.VersionPolicyPin
	}
	if job.ExecutionMode == "" {
		job.ExecutionMode = domain.ExecutionModeHTTP
	}
	if job.CronOverlapPolicy == "" {
		job.CronOverlapPolicy = domain.OverlapPolicyAllow
	}
	if job.Queue == "" {
		job.Queue = defaultJobQueueName
	}

	query := `
		INSERT INTO jobs (
			id, project_id, group_id, name, slug, description, cron, payload_schema,
			tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
			rate_limit_max, rate_limit_window_secs, dedup_window_secs, enabled,
			webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id, version,
			version_id, version_policy, backwards_compatible, created_by, updated_by,
			max_concurrency_per_key, rate_limit_keys, default_run_metadata, retry_priority_boost, dlq_alert_threshold, queue_depth_alert_threshold, poison_pill_threshold, cron_overlap_policy, result_schema,
			debounce_window_secs, batch_window_secs, batch_max_size, execution_mode, preferred_regions, queue_name,
			on_complete_trigger_workflow, on_complete_trigger_job, on_complete_payload_mapping,
			on_failure_trigger_job, on_failure_trigger_workflow, on_failure_payload_mapping,
			paused, paused_at, pause_reason, endpoint_signing_secret
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, 1,
			$27, $28, $29, $30, $31,
			$32, $33::jsonb, $34::jsonb, $35, $36, $37, $38, $39,
			$40, $41, $42, $43, $44, $45,
			$46, $47, $48, $49, $50, $51,
			$52, $53, $54, $55, $56)
			RETURNING created_at, updated_at, version, cache_version`

	tagsJSON, err := marshalTags(job.Tags)
	if err != nil {
		return fmt.Errorf("create job: %w", err)
	}

	var runTTL *int
	if job.RunTTLSecs > 0 {
		runTTL = &job.RunTTLSecs
	}

	err = q.db.QueryRow(
		ctx,
		query,
		job.ID,
		job.ProjectID,
		dbscan.NilIfEmptyString(job.GroupID),
		job.Name,
		job.Slug,
		dbscan.NilIfEmptyString(job.Description),
		dbscan.NilIfEmptyString(job.Cron),
		dbscan.NilIfEmptyRawMessage(job.PayloadSchema),
		tagsJSON,
		job.EndpointURL,
		dbscan.NilIfEmptyString(job.FallbackEndpointURL),
		job.MaxAttempts,
		job.TimeoutSecs,
		dbscan.NilIfZeroInt(job.MaxConcurrency),
		dbscan.NilIfEmptyString(job.ExecutionWindowCron),
		dbscan.NilIfEmptyString(job.Timezone),
		dbscan.NilIfZeroInt(job.RateLimitMax),
		dbscan.NilIfZeroInt(job.RateLimitWindowSecs),
		dbscan.NilIfZeroInt(job.DedupWindowSecs),
		job.Enabled,
		dbscan.NilIfEmptyString(job.WebhookURL),
		dbscan.NilIfEmptyString(job.WebhookSecret),
		runTTL,
		dbscan.NilIfEmptyString(job.RetryStrategy),
		dbscan.NilIfEmptyIntSlice(job.RetryDelaysSecs),
		dbscan.NilIfEmptyString(job.EnvironmentID),
		job.VersionID,
		string(job.VersionPolicy),
		job.BackwardsCompatible,
		dbscan.NilIfEmptyString(job.CreatedBy),
		dbscan.NilIfEmptyString(job.UpdatedBy),
		dbscan.NilIfZeroInt(job.MaxConcurrencyPerKey),
		marshalJSONBOrDefault(job.RateLimitKeys, "[]"),
		marshalJSONBOrDefault(job.DefaultRunMetadata, "{}"),
		job.RetryPriorityBoost,
		job.DLQAlertThreshold,
		job.QueueDepthAlertThreshold,
		job.PoisonPillThreshold,
		string(job.CronOverlapPolicy),
		dbscan.NilIfEmptyRawMessage(job.ResultSchema),
		job.DebounceWindowSecs,
		job.BatchWindowSecs,
		job.BatchMaxSize,
		string(job.ExecutionMode),
		job.PreferredRegions,
		job.Queue,
		dbscan.NilIfEmptyString(job.OnCompleteTriggerWorkflow),
		dbscan.NilIfEmptyString(job.OnCompleteTriggerJob),
		dbscan.NilIfEmptyRawMessage(job.OnCompletePayloadMapping),
		dbscan.NilIfEmptyString(job.OnFailureTriggerJob),
		dbscan.NilIfEmptyString(job.OnFailureTriggerWorkflow),
		dbscan.NilIfEmptyRawMessage(job.OnFailurePayloadMapping),
		job.Paused,
		job.PausedAt,
		dbscan.NilIfEmptyString(job.PauseReason),
		job.EndpointSigningSecret,
	).Scan(&job.CreatedAt, &job.UpdatedAt, &job.Version, &job.CacheVersion)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("job with slug %q already exists: %w", job.Slug, ErrJobSlugConflict)
		}
		return fmt.Errorf("create job: %w", err)
	}

	return nil
}

// CreateJobWithCronScheduleLimit serializes org-wide cron schedule quota
// enforcement with job creation.
func (q *Queries) CreateJobWithCronScheduleLimit(ctx context.Context, job *domain.Job, orgID string, maxSchedules int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateJobWithCronScheduleLimit")
	defer span.End()

	if job.Cron == "" || orgID == "" || maxSchedules < 0 {
		return q.CreateJob(ctx, job)
	}

	if _, ok := TxFromContext(ctx); ok {
		return q.createJobWithCronScheduleLimitLocked(ctx, job, orgID, maxSchedules)
	}
	if _, ok := q.db.(pgx.Tx); ok {
		return q.createJobWithCronScheduleLimitLocked(ctx, job, orgID, maxSchedules)
	}
	if _, ok := q.db.(TxBeginner); !ok {
		return q.createJobWithCronScheduleLimitLocked(ctx, job, orgID, maxSchedules)
	}

	return q.withTx(ctx, func(txq *Queries) error {
		return txq.createJobWithCronScheduleLimitLocked(ctx, job, orgID, maxSchedules)
	})
}

func (q *Queries) createJobWithCronScheduleLimitLocked(ctx context.Context, job *domain.Job, orgID string, maxSchedules int) error {
	if err := q.EnforceCronScheduleLimit(ctx, orgID, maxSchedules); err != nil {
		return err
	}
	return q.CreateJob(ctx, job)
}

func (q *Queries) GetJob(ctx context.Context, id string) (*domain.Job, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJob")
	defer span.End()

	query := `
		SELECT id, project_id, group_id, name, slug, description, cron, payload_schema,
		       tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
		       rate_limit_max, rate_limit_window_secs, dedup_window_secs,
		       enabled, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id, version, version_id, version_policy, backwards_compatible, created_by, updated_by, created_at, updated_at,
		       max_concurrency_per_key, rate_limit_keys, default_run_metadata, retry_priority_boost, dlq_alert_threshold, queue_depth_alert_threshold, poison_pill_threshold, cron_overlap_policy, result_schema, debounce_window_secs, batch_window_secs, batch_max_size, execution_mode, preferred_regions, queue_name, on_complete_trigger_workflow, on_complete_trigger_job, on_complete_payload_mapping, on_failure_trigger_job, on_failure_trigger_workflow, on_failure_payload_mapping,
			       paused, paused_at, pause_reason, endpoint_signing_secret, cache_version
		FROM jobs
		WHERE id = $1`

	job, err := scanJob(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrJobNotFound
		}
		return nil, fmt.Errorf("get job: %w", err)
	}

	return job, nil
}

func (q *Queries) GetJobBySlug(ctx context.Context, projectID, slug string) (*domain.Job, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJobBySlug")
	defer span.End()

	query := `
		SELECT id, project_id, group_id, name, slug, description, cron, payload_schema,
		       tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
		       rate_limit_max, rate_limit_window_secs, dedup_window_secs,
		       enabled, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id, version, version_id, version_policy, backwards_compatible, created_by, updated_by, created_at, updated_at,
		       max_concurrency_per_key, rate_limit_keys, default_run_metadata, retry_priority_boost, dlq_alert_threshold, queue_depth_alert_threshold, poison_pill_threshold, cron_overlap_policy, result_schema, debounce_window_secs, batch_window_secs, batch_max_size, execution_mode, preferred_regions, queue_name, on_complete_trigger_workflow, on_complete_trigger_job, on_complete_payload_mapping, on_failure_trigger_job, on_failure_trigger_workflow, on_failure_payload_mapping,
			       paused, paused_at, pause_reason, endpoint_signing_secret, cache_version
		FROM jobs
		WHERE project_id = $1 AND slug = $2`

	job, err := scanJob(q.db.QueryRow(ctx, query, projectID, slug))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrJobNotFound
		}
		return nil, fmt.Errorf("get job by slug: %w", err)
	}

	return job, nil
}

func (q *Queries) ListJobs(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Job, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListJobs")
	defer span.End()

	query := `
		SELECT id, project_id, group_id, name, slug, description, cron, payload_schema,
		       tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
		       rate_limit_max, rate_limit_window_secs, dedup_window_secs,
		       enabled, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id, version, version_id, version_policy, backwards_compatible, created_by, updated_by, created_at, updated_at,
		       max_concurrency_per_key, rate_limit_keys, default_run_metadata, retry_priority_boost, dlq_alert_threshold, queue_depth_alert_threshold, poison_pill_threshold, cron_overlap_policy, result_schema, debounce_window_secs, batch_window_secs, batch_max_size, execution_mode, preferred_regions, queue_name, on_complete_trigger_workflow, on_complete_trigger_job, on_complete_payload_mapping, on_failure_trigger_job, on_failure_trigger_workflow, on_failure_payload_mapping,
			       paused, paused_at, pause_reason, endpoint_signing_secret, cache_version
		FROM jobs
		WHERE project_id = $1`

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
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]domain.Job, 0, limit)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("list jobs scan: %w", err)
		}
		jobs = append(jobs, *job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list jobs rows: %w", err)
	}

	return jobs, nil
}

func (q *Queries) UpdateJob(ctx context.Context, job *domain.Job) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateJob")
	defer span.End()

	newVersionID := domain.NewVersionID()
	if job.Queue == "" {
		job.Queue = defaultJobQueueName
	}

	query := `
		WITH current_job AS (
			SELECT *
			FROM jobs
			WHERE id = $25 AND version = $52
			FOR UPDATE
		), snapshot AS (
			INSERT INTO job_versions (id, job_id, version, version_id, name, slug, description, cron, payload_schema,
				tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
				rate_limit_max, rate_limit_window_secs, dedup_window_secs, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id,
				group_id, project_id, enabled, backwards_compatible, created_by, updated_by,
				max_concurrency_per_key, rate_limit_keys, default_run_metadata, retry_priority_boost, dlq_alert_threshold, queue_depth_alert_threshold,
				poison_pill_threshold, cron_overlap_policy, result_schema, debounce_window_secs, batch_window_secs, batch_max_size,
				execution_mode, preferred_regions, queue_name,
				on_complete_trigger_workflow, on_complete_trigger_job, on_complete_payload_mapping,
				on_failure_trigger_job, on_failure_trigger_workflow, on_failure_payload_mapping,
				paused, paused_at, pause_reason, endpoint_signing_secret)
			SELECT $29, id, version, version_id, name, slug, description, cron, payload_schema,
				tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
				rate_limit_max, rate_limit_window_secs, dedup_window_secs, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id,
				group_id, project_id, enabled, backwards_compatible, created_by, updated_by,
				max_concurrency_per_key, rate_limit_keys, default_run_metadata, retry_priority_boost, dlq_alert_threshold, queue_depth_alert_threshold,
				poison_pill_threshold, cron_overlap_policy, result_schema, debounce_window_secs, batch_window_secs, batch_max_size,
				COALESCE(execution_mode, 'http'), COALESCE(preferred_regions, '{}'), COALESCE(NULLIF(queue_name, ''), 'default'),
				on_complete_trigger_workflow, on_complete_trigger_job, on_complete_payload_mapping,
				on_failure_trigger_job, on_failure_trigger_workflow, on_failure_payload_mapping,
				COALESCE(paused, false), paused_at, pause_reason, COALESCE(endpoint_signing_secret, '')
			FROM current_job
		)
		UPDATE jobs AS j
		SET group_id = $1,
		    name = $2,
		    slug = $3,
		    description = $4,
		    cron = $5,
		    payload_schema = $6,
		    tags = $7::jsonb,
		    endpoint_url = $8,
		    fallback_endpoint_url = $9,
		    max_attempts = $10,
		    timeout_secs = $11,
		    max_concurrency = $12,
		    execution_window_cron = $13,
		    timezone = $14,
		    rate_limit_max = $15,
		    rate_limit_window_secs = $16,
		    dedup_window_secs = $17,
		    enabled = $18,
		    webhook_url = $19,
		    webhook_secret = $20,
		    run_ttl_secs = $21,
		    retry_strategy = $22,
		    retry_delays_secs = $23,
		    environment_id = $24,
		    version = j.version + 1,
		    version_id = $26,
		    updated_by = $27,
		    version_policy = $28,
		    backwards_compatible = $30,
		    max_concurrency_per_key = $31,
		    rate_limit_keys = $32::jsonb,
		    default_run_metadata = $33::jsonb,
		    retry_priority_boost = $34,
		    dlq_alert_threshold = $35,
		    queue_depth_alert_threshold = $36,
		    poison_pill_threshold = $37,
		    cron_overlap_policy = $38,
		    result_schema = $39,
		    debounce_window_secs = $40,
		    batch_window_secs = $41,
		    batch_max_size = $42,
		    execution_mode = $43,
		    preferred_regions = $44,
		    queue_name = $45,
		    on_complete_trigger_workflow = $46,
		    on_complete_trigger_job = $47,
		    on_complete_payload_mapping = $48,
		    on_failure_trigger_job = $49,
		    on_failure_trigger_workflow = $50,
		    on_failure_payload_mapping = $51,
		    endpoint_signing_secret = $53,
		    updated_at = NOW()
		FROM current_job
		WHERE j.id = current_job.id
			RETURNING j.updated_at, j.version, j.version_id, j.cache_version`

	tagsJSON, err := marshalTags(job.Tags)
	if err != nil {
		return fmt.Errorf("update job: %w", err)
	}

	var runTTL *int
	if job.RunTTLSecs > 0 {
		runTTL = &job.RunTTLSecs
	}

	err = q.db.QueryRow(
		ctx,
		query,
		dbscan.NilIfEmptyString(job.GroupID),
		job.Name,
		job.Slug,
		dbscan.NilIfEmptyString(job.Description),
		dbscan.NilIfEmptyString(job.Cron),
		dbscan.NilIfEmptyRawMessage(job.PayloadSchema),
		tagsJSON,
		job.EndpointURL,
		dbscan.NilIfEmptyString(job.FallbackEndpointURL),
		job.MaxAttempts,
		job.TimeoutSecs,
		dbscan.NilIfZeroInt(job.MaxConcurrency),
		dbscan.NilIfEmptyString(job.ExecutionWindowCron),
		dbscan.NilIfEmptyString(job.Timezone),
		dbscan.NilIfZeroInt(job.RateLimitMax),
		dbscan.NilIfZeroInt(job.RateLimitWindowSecs),
		dbscan.NilIfZeroInt(job.DedupWindowSecs),
		job.Enabled,
		dbscan.NilIfEmptyString(job.WebhookURL),
		dbscan.NilIfEmptyString(job.WebhookSecret),
		runTTL,
		dbscan.NilIfEmptyString(job.RetryStrategy),
		dbscan.NilIfEmptyIntSlice(job.RetryDelaysSecs),
		dbscan.NilIfEmptyString(job.EnvironmentID),
		job.ID,
		newVersionID,
		dbscan.NilIfEmptyString(job.UpdatedBy),
		string(job.VersionPolicy),
		uuid.Must(uuid.NewV7()).String(),
		job.BackwardsCompatible,
		dbscan.NilIfZeroInt(job.MaxConcurrencyPerKey),
		marshalJSONBOrDefault(job.RateLimitKeys, "[]"),
		marshalJSONBOrDefault(job.DefaultRunMetadata, "{}"),
		job.RetryPriorityBoost,
		job.DLQAlertThreshold,
		job.QueueDepthAlertThreshold,
		job.PoisonPillThreshold,
		string(job.CronOverlapPolicy),
		dbscan.NilIfEmptyRawMessage(job.ResultSchema),
		job.DebounceWindowSecs,
		job.BatchWindowSecs,
		job.BatchMaxSize,
		string(job.ExecutionMode),
		job.PreferredRegions,
		job.Queue,
		dbscan.NilIfEmptyString(job.OnCompleteTriggerWorkflow),
		dbscan.NilIfEmptyString(job.OnCompleteTriggerJob),
		dbscan.NilIfEmptyRawMessage(job.OnCompletePayloadMapping),
		dbscan.NilIfEmptyString(job.OnFailureTriggerJob),
		dbscan.NilIfEmptyString(job.OnFailureTriggerWorkflow),
		dbscan.NilIfEmptyRawMessage(job.OnFailurePayloadMapping),
		job.Version,
		job.EndpointSigningSecret,
	).Scan(&job.UpdatedAt, &job.Version, &job.VersionID, &job.CacheVersion)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrJobVersionConflict
		}
		return fmt.Errorf("update job: %w", err)
	}

	return nil
}

// UpdateJobWithCronScheduleLimit serializes org-wide cron schedule quota
// enforcement with a job update that adds a cron expression.
func (q *Queries) UpdateJobWithCronScheduleLimit(ctx context.Context, job *domain.Job, orgID string, maxSchedules int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateJobWithCronScheduleLimit")
	defer span.End()

	if job.Cron == "" || orgID == "" || maxSchedules < 0 {
		return q.UpdateJob(ctx, job)
	}

	if _, ok := TxFromContext(ctx); ok {
		return q.updateJobWithCronScheduleLimitLocked(ctx, job, orgID, maxSchedules)
	}
	if _, ok := q.db.(pgx.Tx); ok {
		return q.updateJobWithCronScheduleLimitLocked(ctx, job, orgID, maxSchedules)
	}
	if _, ok := q.db.(TxBeginner); !ok {
		return q.updateJobWithCronScheduleLimitLocked(ctx, job, orgID, maxSchedules)
	}

	return q.withTx(ctx, func(txq *Queries) error {
		return txq.updateJobWithCronScheduleLimitLocked(ctx, job, orgID, maxSchedules)
	})
}

func (q *Queries) updateJobWithCronScheduleLimitLocked(ctx context.Context, job *domain.Job, orgID string, maxSchedules int) error {
	if err := q.EnforceCronScheduleLimit(ctx, orgID, maxSchedules); err != nil {
		return err
	}
	return q.UpdateJob(ctx, job)
}

// ErrJobVersionConflict is returned when an UpdateJob call fails because the
// job was modified concurrently (optimistic locking via version column).
var ErrJobVersionConflict = errors.New("job version conflict")

// ErrJobHasActiveRuns is returned when attempting to delete a job that has
// queued, dequeued, or executing runs.
var ErrJobHasActiveRuns = errors.New("job has active runs")

func (q *Queries) DeleteJob(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteJob")
	defer span.End()

	// If the underlying connection supports transactions, wrap the whole
	// delete in one so a crash mid-way doesn't leave orphaned data.
	if _, ok := q.db.(TxBeginner); ok {
		return q.withTx(ctx, func(tx *Queries) error {
			return tx.deleteJobTx(ctx, id)
		})
	}
	// Already inside a transaction — execute directly.
	return q.deleteJobTx(ctx, id)
}

func (q *Queries) deleteJobTx(ctx context.Context, id string) error {
	// Lock the job row first to prevent concurrent enqueues while we check and delete.
	var exists bool
	err := q.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE id = $1 FOR UPDATE)`, id).Scan(&exists)
	if err != nil {
		return fmt.Errorf("delete job lock: %w", err)
	}
	if !exists {
		return ErrJobNotFound
	}

	// Now check for active runs under the job-row lock.
	var activeCount int
	err = q.db.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM job_runs jr
		 LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		 WHERE jr.job_id = $1
		   AND COALESCE(s.status, jr.status) IN ('queued','delayed','dequeued','executing','waiting')`,
		id,
	).Scan(&activeCount)
	if err != nil {
		return fmt.Errorf("delete job check active runs: %w", err)
	}
	if activeCount > 0 {
		return ErrJobHasActiveRuns
	}

	// Delete related data before removing the job (FK constraints).
	if _, err := q.db.Exec(ctx, `
		WITH target_runs AS MATERIALIZED (
			SELECT id
			FROM job_runs
			WHERE job_id = $1
		),
		deleted_active_claims AS (
			DELETE FROM job_run_active_claims
			WHERE run_id IN (SELECT id FROM target_runs)
		),
		deleted_lifecycle_events AS (
			DELETE FROM job_run_lifecycle_events
			WHERE run_id IN (SELECT id FROM target_runs)
		),
		deleted_ready_events AS (
			DELETE FROM job_run_ready_events
			WHERE run_id IN (SELECT id FROM target_runs)
		),
		deleted_retries AS (
			DELETE FROM job_retries
			WHERE run_id IN (SELECT id FROM target_runs)
		),
		deleted_priority_events AS (
			DELETE FROM job_run_priority_events
			WHERE run_id IN (SELECT id FROM target_runs)
		),
		deleted_visibility_events AS (
			DELETE FROM job_run_visibility_events
			WHERE run_id IN (SELECT id FROM target_runs)
		),
		deleted_cache_versions AS (
			DELETE FROM job_run_cache_versions
			WHERE run_id IN (SELECT id FROM target_runs)
		),
		deleted_heartbeats AS (
			DELETE FROM job_run_heartbeats
			WHERE run_id IN (SELECT id FROM target_runs)
		),
		deleted_terminal_state AS (
			DELETE FROM job_run_terminal_state
			WHERE run_id IN (SELECT id FROM target_runs)
		)
		SELECT 1`, id); err != nil {
		return fmt.Errorf("delete job run side rows: %w", err)
	}
	if _, err := q.db.Exec(ctx, `DELETE FROM job_runs WHERE job_id = $1`, id); err != nil {
		return fmt.Errorf("delete job runs: %w", err)
	}
	if _, err := q.db.Exec(ctx, `DELETE FROM job_versions WHERE job_id = $1`, id); err != nil {
		return fmt.Errorf("delete job versions: %w", err)
	}
	if _, err := q.db.Exec(ctx, `DELETE FROM job_dependencies WHERE job_id = $1 OR depends_on_job_id = $1`, id); err != nil {
		return fmt.Errorf("delete job dependencies: %w", err)
	}
	if _, err := q.db.Exec(ctx, `DELETE FROM job_memory WHERE job_id = $1`, id); err != nil {
		return fmt.Errorf("delete job memory: %w", err)
	}

	tag, err := q.db.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete job: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrJobNotFound
	}

	return nil
}

func (q *Queries) BatchUpdateJobsEnabled(ctx context.Context, ids []string, enabled bool, projectID string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.BatchUpdateJobsEnabled")
	defer span.End()

	if len(ids) == 0 {
		return 0, nil
	}

	var (
		tag pgconn.CommandTag
		err error
	)
	if projectID == "" {
		query := `UPDATE jobs SET enabled = $1, updated_at = NOW() WHERE id = ANY($2)`
		tag, err = q.db.Exec(ctx, query, enabled, ids)
	} else {
		query := `UPDATE jobs SET enabled = $1, updated_at = NOW() WHERE id = ANY($2) AND project_id = $3`
		tag, err = q.db.Exec(ctx, query, enabled, ids, projectID)
	}
	if err != nil {
		return 0, fmt.Errorf("batch update jobs enabled: %w", err)
	}

	return tag.RowsAffected(), nil
}

func (q *Queries) ListCronJobs(ctx context.Context) ([]domain.Job, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListCronJobs")
	defer span.End()

	query := `
		SELECT id, project_id, group_id, name, slug, description, cron, payload_schema,
		       tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
		       rate_limit_max, rate_limit_window_secs, dedup_window_secs,
		       enabled, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id, version, version_id, version_policy, backwards_compatible, created_by, updated_by, created_at, updated_at,
		       max_concurrency_per_key, rate_limit_keys, default_run_metadata, retry_priority_boost, dlq_alert_threshold, queue_depth_alert_threshold, poison_pill_threshold, cron_overlap_policy, result_schema, debounce_window_secs, batch_window_secs, batch_max_size, execution_mode, preferred_regions, queue_name, on_complete_trigger_workflow, on_complete_trigger_job, on_complete_payload_mapping, on_failure_trigger_job, on_failure_trigger_workflow, on_failure_payload_mapping,
			       paused, paused_at, pause_reason, endpoint_signing_secret, cache_version
		FROM jobs
		WHERE enabled = TRUE AND NOT paused AND cron IS NOT NULL AND cron <> ''
		ORDER BY created_at DESC`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list cron jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]domain.Job, 0, 8)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("list cron jobs scan: %w", err)
		}
		jobs = append(jobs, *job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list cron jobs rows: %w", err)
	}

	return jobs, nil
}

func (q *Queries) GetProjectQuota(ctx context.Context, projectID string) (*ProjectQuota, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetProjectQuota")
	defer span.End()

	query := `
		SELECT project_id, max_queued_runs, max_executing_runs, max_jobs, timezone, max_cost_per_run_microusd, max_daily_cost_microusd,
		       rate_limit_requests, rate_limit_window_secs, default_region, plan_tier,
		       max_memory_per_key_bytes, max_memory_per_job_bytes, max_key_lifetime_days, cache_version
		FROM project_quotas
		WHERE project_id = $1`

	quota := &ProjectQuota{}
	var maxQueued *int
	var maxExecuting *int
	var maxJobs *int
	var timezone *string
	var maxCostPerRun *int64
	var maxDailyCost *int64
	var rateLimitRequests *int
	var rateLimitWindowSecs *int
	var defaultRegion *string
	var planTier *string
	var maxMemoryPerKeyBytes *int
	var maxMemoryPerJobBytes *int
	var maxKeyLifetimeDays *int
	err := q.db.QueryRow(ctx, query, projectID).Scan(
		&quota.ProjectID,
		&maxQueued,
		&maxExecuting,
		&maxJobs,
		&timezone,
		&maxCostPerRun,
		&maxDailyCost,
		&rateLimitRequests,
		&rateLimitWindowSecs,
		&defaultRegion,
		&planTier,
		&maxMemoryPerKeyBytes,
		&maxMemoryPerJobBytes,
		&maxKeyLifetimeDays,
		&quota.CacheVersion,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get project quota: %w", err)
	}

	if maxQueued != nil {
		quota.MaxQueuedRuns = *maxQueued
	}
	if maxExecuting != nil {
		quota.MaxExecutingRuns = *maxExecuting
	}
	if maxJobs != nil {
		quota.MaxJobs = *maxJobs
	}
	if timezone != nil {
		quota.Timezone = *timezone
	}
	if maxCostPerRun != nil {
		quota.MaxCostPerRunMicrousd = *maxCostPerRun
	}
	if maxDailyCost != nil {
		quota.MaxDailyCostMicrousd = *maxDailyCost
	}
	if rateLimitRequests != nil {
		quota.RateLimitRequests = *rateLimitRequests
	}
	if rateLimitWindowSecs != nil {
		quota.RateLimitWindowSecs = *rateLimitWindowSecs
	}
	if defaultRegion != nil {
		quota.DefaultRegion = *defaultRegion
	}
	if planTier != nil {
		quota.PlanTier = *planTier
	}
	if maxMemoryPerKeyBytes != nil {
		quota.MaxMemoryPerKeyBytes = *maxMemoryPerKeyBytes
	}
	if maxMemoryPerJobBytes != nil {
		quota.MaxMemoryPerJobBytes = *maxMemoryPerJobBytes
	}
	if maxKeyLifetimeDays != nil {
		quota.MaxKeyLifetimeDays = *maxKeyLifetimeDays
	}

	return quota, nil
}

func (q *Queries) CountProjectQueuedRuns(ctx context.Context, projectID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountProjectQueuedRuns")
	defer span.End()

	query := `
		SELECT COUNT(*)
		FROM job_runs jr
		LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE jr.project_id = $1
		  AND COALESCE(s.status, jr.status) IN ('queued', 'delayed')`

	var count int
	if err := q.db.QueryRow(ctx, query, projectID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count project queued runs: %w", err)
	}

	return count, nil
}

func (q *Queries) CountProjectActiveRuns(ctx context.Context, projectID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountProjectActiveRuns")
	defer span.End()

	query := `
		SELECT COUNT(*)
		FROM job_runs jr
		LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE jr.project_id = $1
		  AND COALESCE(s.status, jr.status) IN ('dequeued', 'executing')`

	var count int
	if err := q.db.QueryRow(ctx, query, projectID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count project active runs: %w", err)
	}

	return count, nil
}

// CountExecutingRunsByOrg counts runs in executing status across all projects in an org.
func (q *Queries) CountExecutingRunsByOrg(ctx context.Context, orgID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountExecutingRunsByOrg")
	defer span.End()

	query := `
		SELECT COUNT(*)
		FROM job_runs jr
		LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE jr.project_id IN (SELECT id FROM projects WHERE org_id = $1)
		  AND COALESCE(s.status, jr.status) = 'executing'`

	var count int
	if err := q.db.QueryRow(ctx, query, orgID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count executing runs by org: %w", err)
	}

	return count, nil
}

// BulkCountExecutingRunsByOrg counts executing runs for multiple orgs in a single
// query, returning a map of orgID -> count.
func (q *Queries) BulkCountExecutingRunsByOrg(ctx context.Context, orgIDs []string) (map[string]int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.BulkCountExecutingRunsByOrg")
	defer span.End()

	if len(orgIDs) == 0 {
		return map[string]int{}, nil
	}

	rows, err := q.db.Query(ctx, `
		SELECT p.org_id, COUNT(jr.id)::int
		FROM job_runs jr
		JOIN projects p ON p.id = jr.project_id
		LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE p.org_id = ANY($1)
		  AND COALESCE(s.status, jr.status) = 'executing'
		GROUP BY p.org_id
	`, orgIDs)
	if err != nil {
		return nil, fmt.Errorf("bulk count executing runs by org: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int, len(orgIDs))
	for rows.Next() {
		var orgID string
		var count int
		if err := rows.Scan(&orgID, &count); err != nil {
			return nil, fmt.Errorf("scanning bulk executing run count: %w", err)
		}
		result[orgID] = count
	}
	return result, rows.Err()
}

// ListOrgsWithExecutingRuns returns distinct org IDs that have at least one executing run.
func (q *Queries) ListOrgsWithExecutingRuns(ctx context.Context) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListOrgsWithExecutingRuns")
	defer span.End()

	query := `
		SELECT DISTINCT p.org_id
		FROM job_runs jr
		JOIN projects p ON p.id = jr.project_id
		LEFT JOIN job_run_read_state s ON s.run_id = jr.id
		WHERE COALESCE(s.status, jr.status) = 'executing'
		  AND p.org_id IS NOT NULL`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing orgs with executing runs: %w", err)
	}
	defer rows.Close()

	var orgIDs []string
	for rows.Next() {
		var orgID string
		if err := rows.Scan(&orgID); err != nil {
			return nil, fmt.Errorf("scanning org_id: %w", err)
		}
		orgIDs = append(orgIDs, orgID)
	}
	return orgIDs, rows.Err()
}

// UpdateProjectDefaultRegion sets the default_region for a project's quota row.
// It upserts the row if it does not exist.
func (q *Queries) UpdateProjectDefaultRegion(ctx context.Context, projectID, defaultRegion string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateProjectDefaultRegion")
	defer span.End()

	query := `
		INSERT INTO project_quotas (project_id, default_region)
		VALUES ($1, $2)
		ON CONFLICT (project_id) DO UPDATE SET default_region = EXCLUDED.default_region
		WHERE project_quotas.default_region IS DISTINCT FROM EXCLUDED.default_region`

	_, err := q.db.Exec(ctx, query, projectID, defaultRegion)
	if err != nil {
		return fmt.Errorf("update project default region: %w", err)
	}
	return nil
}

func (q *Queries) UpdateProjectMaxKeyLifetimeDays(ctx context.Context, projectID string, days int) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateProjectMaxKeyLifetimeDays")
	defer span.End()

	query := `
		INSERT INTO project_quotas (project_id, max_key_lifetime_days)
		VALUES ($1, $2)
		ON CONFLICT (project_id) DO UPDATE SET max_key_lifetime_days = EXCLUDED.max_key_lifetime_days
		WHERE project_quotas.max_key_lifetime_days IS DISTINCT FROM EXCLUDED.max_key_lifetime_days`

	_, err := q.db.Exec(ctx, query, projectID, days)
	if err != nil {
		return fmt.Errorf("update project max key lifetime days: %w", err)
	}
	return nil
}

type scanTarget interface {
	Scan(dest ...any) error
}

func scanJob(scanner scanTarget) (*domain.Job, error) {
	var job domain.Job
	var description *string
	var groupID *string
	var cron *string
	var payloadSchema []byte
	var tagsJSON []byte
	var fallbackEndpointURL *string
	var maxConcurrency *int
	var executionWindowCron *string
	var timezone *string
	var rateLimitMax *int
	var rateLimitWindowSecs *int
	var dedupWindowSecs *int
	var webhookURL *string
	var webhookSecret *string
	var runTTLSecs *int
	var retryStrategy *string
	var retryDelaysSecs []int
	var environmentID *string
	var versionID *string
	var versionPolicy *string
	var createdBy *string
	var updatedBy *string
	var maxConcurrencyPerKey *int
	var rateLimitKeysJSON []byte
	var defaultRunMetadataJSON []byte
	var cronOverlapPolicy *string
	var resultSchema []byte
	var debounceWindowSecs *int
	var batchWindowSecs *int
	var batchMaxSize *int
	var executionMode *string
	var preferredRegions []string
	var queueName *string
	var onCompleteTriggerWorkflow *string
	var onCompleteTriggerJob *string
	var onCompletePayloadMapping []byte
	var onFailureTriggerJob *string
	var onFailureTriggerWorkflow *string
	var onFailurePayloadMapping []byte
	var pausedAt *time.Time
	var pauseReason *string

	err := scanner.Scan(
		&job.ID,
		&job.ProjectID,
		&groupID,
		&job.Name,
		&job.Slug,
		&description,
		&cron,
		&payloadSchema,
		&tagsJSON,
		&job.EndpointURL,
		&fallbackEndpointURL,
		&job.MaxAttempts,
		&job.TimeoutSecs,
		&maxConcurrency,
		&executionWindowCron,
		&timezone,
		&rateLimitMax,
		&rateLimitWindowSecs,
		&dedupWindowSecs,
		&job.Enabled,
		&webhookURL,
		&webhookSecret,
		&runTTLSecs,
		&retryStrategy,
		&retryDelaysSecs,
		&environmentID,
		&job.Version,
		&versionID,
		&versionPolicy,
		&job.BackwardsCompatible,
		&createdBy,
		&updatedBy,
		&job.CreatedAt,
		&job.UpdatedAt,
		&maxConcurrencyPerKey,
		&rateLimitKeysJSON,
		&defaultRunMetadataJSON,
		&job.RetryPriorityBoost,
		&job.DLQAlertThreshold,
		&job.QueueDepthAlertThreshold,
		&job.PoisonPillThreshold,
		&cronOverlapPolicy,
		&resultSchema,
		&debounceWindowSecs,
		&batchWindowSecs,
		&batchMaxSize,
		&executionMode,
		&preferredRegions,
		&queueName,
		&onCompleteTriggerWorkflow,
		&onCompleteTriggerJob,
		&onCompletePayloadMapping,
		&onFailureTriggerJob,
		&onFailureTriggerWorkflow,
		&onFailurePayloadMapping,
		&job.Paused,
		&pausedAt,
		&pauseReason,
		&job.EndpointSigningSecret,
		&job.CacheVersion,
	)
	if err != nil {
		return nil, err
	}

	return applyScannedJobNullables(&job, scannedJobNullables{
		pausedAt: pausedAt, pauseReason: pauseReason,
		description: description, groupID: groupID, cron: cron,
		payloadSchema: payloadSchema, tagsJSON: tagsJSON,
		fallbackEndpointURL: fallbackEndpointURL,
		webhookURL:          webhookURL, webhookSecret: webhookSecret,
		runTTLSecs: runTTLSecs, maxConcurrency: maxConcurrency,
		executionWindowCron: executionWindowCron, timezone: timezone,
		rateLimitMax: rateLimitMax, rateLimitWindowSecs: rateLimitWindowSecs,
		dedupWindowSecs: dedupWindowSecs, retryStrategy: retryStrategy,
		retryDelaysSecs: retryDelaysSecs, environmentID: environmentID,
		versionID: versionID, versionPolicy: versionPolicy,
		createdBy: createdBy, updatedBy: updatedBy,
		maxConcurrencyPerKey: maxConcurrencyPerKey,
		rateLimitKeysJSON:    rateLimitKeysJSON, defaultRunMetadataJSON: defaultRunMetadataJSON,
		cronOverlapPolicy: cronOverlapPolicy, resultSchema: resultSchema,
		debounceWindowSecs: debounceWindowSecs, batchWindowSecs: batchWindowSecs,
		batchMaxSize: batchMaxSize, executionMode: executionMode,
		preferredRegions: preferredRegions, queueName: queueName,
		onCompleteTriggerWorkflow: onCompleteTriggerWorkflow,
		onCompleteTriggerJob:      onCompleteTriggerJob,
		onCompletePayloadMapping:  onCompletePayloadMapping,
		onFailureTriggerJob:       onFailureTriggerJob,
		onFailureTriggerWorkflow:  onFailureTriggerWorkflow,
		onFailurePayloadMapping:   onFailurePayloadMapping,
	})
}

type scannedJobNullables struct {
	pausedAt                  *time.Time
	pauseReason               *string
	description               *string
	groupID                   *string
	cron                      *string
	payloadSchema             []byte
	tagsJSON                  []byte
	fallbackEndpointURL       *string
	webhookURL                *string
	webhookSecret             *string
	runTTLSecs                *int
	maxConcurrency            *int
	executionWindowCron       *string
	timezone                  *string
	rateLimitMax              *int
	rateLimitWindowSecs       *int
	dedupWindowSecs           *int
	retryStrategy             *string
	retryDelaysSecs           []int
	environmentID             *string
	versionID                 *string
	versionPolicy             *string
	createdBy                 *string
	updatedBy                 *string
	maxConcurrencyPerKey      *int
	rateLimitKeysJSON         []byte
	defaultRunMetadataJSON    []byte
	cronOverlapPolicy         *string
	resultSchema              []byte
	debounceWindowSecs        *int
	batchWindowSecs           *int
	batchMaxSize              *int
	executionMode             *string
	preferredRegions          []string
	queueName                 *string
	onCompleteTriggerWorkflow *string
	onCompleteTriggerJob      *string
	onCompletePayloadMapping  []byte
	onFailureTriggerJob       *string
	onFailureTriggerWorkflow  *string
	onFailurePayloadMapping   []byte
}

//nolint:gocyclo,cyclop,funlen // flat nullable-to-field assignments, not meaningfully splittable.
func applyScannedJobNullables(job *domain.Job, n scannedJobNullables) (*domain.Job, error) {
	if n.pausedAt != nil {
		job.PausedAt = n.pausedAt
	}
	if n.pauseReason != nil {
		job.PauseReason = *n.pauseReason
	}
	if n.description != nil {
		job.Description = *n.description
	}
	if n.groupID != nil {
		job.GroupID = *n.groupID
	}
	if n.cron != nil {
		job.Cron = *n.cron
	}
	if n.payloadSchema != nil {
		job.PayloadSchema = json.RawMessage(n.payloadSchema)
	}
	if len(n.tagsJSON) > 0 {
		tags, err := unmarshalTags(n.tagsJSON)
		if err != nil {
			return nil, err
		}
		job.Tags = tags
	}
	if n.fallbackEndpointURL != nil {
		job.FallbackEndpointURL = *n.fallbackEndpointURL
	}
	if n.webhookURL != nil {
		job.WebhookURL = *n.webhookURL
	}
	if n.webhookSecret != nil {
		job.WebhookSecret = *n.webhookSecret
	}
	if n.runTTLSecs != nil {
		job.RunTTLSecs = *n.runTTLSecs
	}
	if n.maxConcurrency != nil {
		job.MaxConcurrency = *n.maxConcurrency
	}
	if n.executionWindowCron != nil {
		job.ExecutionWindowCron = *n.executionWindowCron
	}
	if n.timezone != nil {
		job.Timezone = *n.timezone
	}
	if n.rateLimitMax != nil {
		job.RateLimitMax = *n.rateLimitMax
	}
	if n.rateLimitWindowSecs != nil {
		job.RateLimitWindowSecs = *n.rateLimitWindowSecs
	}
	if n.dedupWindowSecs != nil {
		job.DedupWindowSecs = *n.dedupWindowSecs
	}
	if n.retryStrategy != nil {
		job.RetryStrategy = *n.retryStrategy
	}
	if len(n.retryDelaysSecs) > 0 {
		job.RetryDelaysSecs = n.retryDelaysSecs
	}
	if n.environmentID != nil {
		job.EnvironmentID = *n.environmentID
	}
	if n.versionID != nil {
		job.VersionID = *n.versionID
	}
	if n.versionPolicy != nil {
		job.VersionPolicy = domain.VersionPolicy(*n.versionPolicy)
	}
	if n.createdBy != nil {
		job.CreatedBy = *n.createdBy
	}
	if n.updatedBy != nil {
		job.UpdatedBy = *n.updatedBy
	}
	if n.maxConcurrencyPerKey != nil {
		job.MaxConcurrencyPerKey = *n.maxConcurrencyPerKey
	}
	if hasNonEmptyJSONArray(n.rateLimitKeysJSON) {
		if err := json.Unmarshal(n.rateLimitKeysJSON, &job.RateLimitKeys); err != nil {
			return nil, fmt.Errorf("unmarshal rate_limit_keys: %w", err)
		}
	}
	if hasNonEmptyJSONObject(n.defaultRunMetadataJSON) {
		if err := json.Unmarshal(n.defaultRunMetadataJSON, &job.DefaultRunMetadata); err != nil {
			return nil, fmt.Errorf("unmarshal default_run_metadata: %w", err)
		}
	}
	if n.cronOverlapPolicy != nil {
		job.CronOverlapPolicy = domain.CronOverlapPolicy(*n.cronOverlapPolicy)
	}
	if n.resultSchema != nil {
		job.ResultSchema = json.RawMessage(n.resultSchema)
	}
	if n.debounceWindowSecs != nil {
		job.DebounceWindowSecs = *n.debounceWindowSecs
	}
	if n.batchWindowSecs != nil {
		job.BatchWindowSecs = *n.batchWindowSecs
	}
	if n.batchMaxSize != nil {
		job.BatchMaxSize = *n.batchMaxSize
	}
	if n.executionMode != nil && *n.executionMode != "" {
		job.ExecutionMode = domain.ExecutionMode(*n.executionMode)
	}
	if job.ExecutionMode == "" {
		job.ExecutionMode = domain.ExecutionModeHTTP
	}
	if len(n.preferredRegions) > 0 {
		job.PreferredRegions = n.preferredRegions
	}
	if n.queueName != nil && *n.queueName != "" {
		job.Queue = *n.queueName
	}
	if n.onCompleteTriggerWorkflow != nil {
		job.OnCompleteTriggerWorkflow = *n.onCompleteTriggerWorkflow
	}
	if n.onCompleteTriggerJob != nil {
		job.OnCompleteTriggerJob = *n.onCompleteTriggerJob
	}
	if n.onCompletePayloadMapping != nil {
		job.OnCompletePayloadMapping = json.RawMessage(n.onCompletePayloadMapping)
	}
	if n.onFailureTriggerJob != nil {
		job.OnFailureTriggerJob = *n.onFailureTriggerJob
	}
	if n.onFailureTriggerWorkflow != nil {
		job.OnFailureTriggerWorkflow = *n.onFailureTriggerWorkflow
	}
	if n.onFailurePayloadMapping != nil {
		job.OnFailurePayloadMapping = json.RawMessage(n.onFailurePayloadMapping)
	}

	return job, nil
}

func hasNonEmptyJSONArray(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	value := string(raw)
	return value != "[]" && value != "null"
}

func hasNonEmptyJSONObject(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	value := string(raw)
	return value != "{}" && value != "null"
}

func (q *Queries) ListJobsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.Job, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListJobsByTag")
	defer span.End()

	base := `
		SELECT id, project_id, group_id, name, slug, description, cron, payload_schema,
		       tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
		       rate_limit_max, rate_limit_window_secs, dedup_window_secs,
		       enabled, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id, version, version_id, version_policy, backwards_compatible, created_by, updated_by, created_at, updated_at,
		       max_concurrency_per_key, rate_limit_keys, default_run_metadata, retry_priority_boost, dlq_alert_threshold, queue_depth_alert_threshold, poison_pill_threshold, cron_overlap_policy, result_schema, debounce_window_secs, batch_window_secs, batch_max_size, execution_mode, preferred_regions, queue_name, on_complete_trigger_workflow, on_complete_trigger_job, on_complete_payload_mapping, on_failure_trigger_job, on_failure_trigger_workflow, on_failure_payload_mapping,
			       paused, paused_at, pause_reason, endpoint_signing_secret, cache_version
		FROM jobs
		WHERE project_id = $1`

	args := []any{projectID, tagKey}
	param := 3
	if tagValue == "" {
		base += ` AND tags ? $2`
	} else {
		base += ` AND tags ->> $2 = $3`
		args = append(args, tagValue)
		param++
	}

	if cursor != nil {
		base += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	base += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, base, args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs by tag: %w", err)
	}
	defer rows.Close()

	jobs := make([]domain.Job, 0, limit)
	for rows.Next() {
		job, scanErr := scanJob(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list jobs by tag scan: %w", scanErr)
		}
		jobs = append(jobs, *job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list jobs by tag rows: %w", err)
	}

	return jobs, nil
}

func marshalTags(tags map[string]string) ([]byte, error) {
	if len(tags) == 0 {
		return []byte(`{}`), nil
	}

	encoded, err := json.Marshal(tags)
	if err != nil {
		return nil, fmt.Errorf("marshal job tags: %w", err)
	}
	return encoded, nil
}

func unmarshalTags(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var tags map[string]string
	if err := json.Unmarshal(raw, &tags); err != nil {
		return nil, fmt.Errorf("unmarshal job tags: %w", err)
	}
	if len(tags) == 0 {
		return nil, nil
	}
	return tags, nil
}

// marshalJSONBOrDefault marshals v as JSON for a JSONB column.
// Returns defaultVal when v is nil or empty.
func marshalJSONBOrDefault(v any, defaultVal string) []byte {
	switch val := v.(type) {
	case nil:
		return []byte(defaultVal)
	case []domain.RateLimitKey:
		if len(val) == 0 {
			return []byte(defaultVal)
		}
	case map[string]string:
		if len(val) == 0 {
			return []byte(defaultVal)
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(defaultVal)
	}
	return b
}

func (q *Queries) PauseJob(ctx context.Context, id, reason string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.PauseJob")
	defer span.End()

	query := `
		WITH updated AS (
			UPDATE jobs
			SET paused = TRUE,
			    paused_at = NOW(),
			    pause_reason = $2,
			    updated_at = NOW()
			WHERE id = $1
			  AND (paused IS DISTINCT FROM TRUE OR pause_reason IS DISTINCT FROM $2)
			RETURNING 1
		),
		existing AS (
			SELECT 1
			FROM jobs
			WHERE id = $1
			  AND NOT EXISTS (SELECT 1 FROM updated)
		)
		SELECT EXISTS (SELECT 1 FROM updated UNION ALL SELECT 1 FROM existing)`
	var found bool
	err := q.db.QueryRow(ctx, query, id, reason).Scan(&found)
	if err != nil {
		return fmt.Errorf("pause job: %w", err)
	}
	if !found {
		return ErrJobNotFound
	}
	return nil
}

func (q *Queries) ResumeJob(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ResumeJob")
	defer span.End()

	query := `
		WITH updated AS (
			UPDATE jobs
			SET paused = FALSE,
			    paused_at = NULL,
			    pause_reason = NULL,
			    updated_at = NOW()
			WHERE id = $1
			  AND (paused IS DISTINCT FROM FALSE OR paused_at IS NOT NULL OR pause_reason IS NOT NULL)
			RETURNING 1
		),
		existing AS (
			SELECT 1
			FROM jobs
			WHERE id = $1
			  AND NOT EXISTS (SELECT 1 FROM updated)
		)
		SELECT EXISTS (SELECT 1 FROM updated UNION ALL SELECT 1 FROM existing)`
	var found bool
	err := q.db.QueryRow(ctx, query, id).Scan(&found)
	if err != nil {
		return fmt.Errorf("resume job: %w", err)
	}
	if !found {
		return ErrJobNotFound
	}
	return nil
}

// UpdateJobEndpoint persists a new endpoint URL, optional fallback URL, and
// signing secret for a job. Callers are responsible for SSRF-validating the
// URLs and generating a fresh signing secret before calling this method.
func (q *Queries) UpdateJobEndpoint(ctx context.Context, jobID, projectID, endpointURL, fallbackURL, signingSecret string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateJobEndpoint")
	defer span.End()

	// project_id is scoped explicitly (in addition to RLS) per the dual-layer
	// tenant-isolation standard, so the row updated is the job the caller already
	// fetched and authorized even if the RLS context is ever absent.
	query := `
		UPDATE jobs
		SET endpoint_url            = $3,
		    fallback_endpoint_url   = $4,
		    endpoint_signing_secret = $5,
		    updated_at              = NOW()
		WHERE id = $1 AND project_id = $2`

	var fallback *string
	if fallbackURL != "" {
		fallback = &fallbackURL
	}

	tag, err := q.db.Exec(ctx, query, jobID, projectID, endpointURL, fallback, signingSecret)
	if err != nil {
		return fmt.Errorf("update job endpoint: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrJobNotFound
	}
	return nil
}
