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

	query := `
		INSERT INTO jobs (
			id, project_id, group_id, name, slug, description, cron, payload_schema,
			tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
			rate_limit_max, rate_limit_window_secs, dedup_window_secs, enabled,
			webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id, version,
			version_id, version_policy, backwards_compatible, created_by, updated_by,
			max_concurrency_per_key, rate_limit_keys, default_run_metadata, retry_priority_boost, dlq_alert_threshold, queue_depth_alert_threshold, skip_if_running, result_schema,
			debounce_window_secs, batch_window_secs, batch_max_size, execution_mode, machine_preset, image_uri, region, preferred_regions,
			on_complete_trigger_workflow, on_complete_payload_mapping
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, 1,
			$27, $28, $29, $30, $31,
			$32, $33::jsonb, $34::jsonb, $35, $36, $37, $38, $39,
			$40, $41, $42, $43, $44, $45, $46, $47,
			$48, $49)
		RETURNING created_at, updated_at, version`

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
		job.SkipIfRunning,
		dbscan.NilIfEmptyRawMessage(job.ResultSchema),
		job.DebounceWindowSecs,
		job.BatchWindowSecs,
		job.BatchMaxSize,
		string(job.ExecutionMode),
		dbscan.NilIfEmptyString(string(job.MachinePreset)),
		dbscan.NilIfEmptyString(job.ImageURI),
		dbscan.NilIfEmptyString(job.Region),
		job.PreferredRegions,
		dbscan.NilIfEmptyString(job.OnCompleteTriggerWorkflow),
		dbscan.NilIfEmptyRawMessage(job.OnCompletePayloadMapping),
	).Scan(&job.CreatedAt, &job.UpdatedAt, &job.Version)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("job with slug %q already exists: %w", job.Slug, ErrJobSlugConflict)
		}
		return fmt.Errorf("create job: %w", err)
	}

	return nil
}

func (q *Queries) GetJob(ctx context.Context, id string) (*domain.Job, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJob")
	defer span.End()

	query := `
		SELECT id, project_id, group_id, name, slug, description, cron, payload_schema,
		       tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
		       rate_limit_max, rate_limit_window_secs, dedup_window_secs,
		       enabled, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id, version, version_id, version_policy, backwards_compatible, created_by, updated_by, created_at, updated_at,
		       max_concurrency_per_key, rate_limit_keys, default_run_metadata, retry_priority_boost, dlq_alert_threshold, queue_depth_alert_threshold, skip_if_running, result_schema, debounce_window_secs, batch_window_secs, batch_max_size, execution_mode, machine_preset, image_uri, region, preferred_regions, on_complete_trigger_workflow, on_complete_payload_mapping
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
		       max_concurrency_per_key, rate_limit_keys, default_run_metadata, retry_priority_boost, dlq_alert_threshold, queue_depth_alert_threshold, skip_if_running, result_schema, debounce_window_secs, batch_window_secs, batch_max_size, execution_mode, machine_preset, image_uri, region, preferred_regions, on_complete_trigger_workflow, on_complete_payload_mapping
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
		       max_concurrency_per_key, rate_limit_keys, default_run_metadata, retry_priority_boost, dlq_alert_threshold, queue_depth_alert_threshold, skip_if_running, result_schema, debounce_window_secs, batch_window_secs, batch_max_size, execution_mode, machine_preset, image_uri, region, preferred_regions, on_complete_trigger_workflow, on_complete_payload_mapping
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

	query := `
		WITH snapshot AS (
			INSERT INTO job_versions (id, job_id, version, version_id, name, slug, description, cron, payload_schema,
				tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
				rate_limit_max, rate_limit_window_secs, dedup_window_secs, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id,
				group_id, project_id, enabled, backwards_compatible, created_by, updated_by, skip_if_running, result_schema)
			SELECT $29, id, version, version_id, name, slug, description, cron, payload_schema,
				tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
				rate_limit_max, rate_limit_window_secs, dedup_window_secs, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id,
				group_id, project_id, enabled, backwards_compatible, created_by, updated_by, skip_if_running, result_schema
			FROM jobs WHERE id = $25
		)
		UPDATE jobs
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
		    version = version + 1,
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
		    skip_if_running = $37,
		    result_schema = $38,
		    debounce_window_secs = $39,
		    batch_window_secs = $40,
		    batch_max_size = $41,
		    execution_mode = $42,
		    machine_preset = $43,
		    image_uri = $44,
		    region = $45,
		    preferred_regions = $46,
		    on_complete_trigger_workflow = $47,
		    on_complete_payload_mapping = $48,
		    updated_at = NOW()
		WHERE id = $25
		RETURNING updated_at, version, version_id`

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
		job.SkipIfRunning,
		dbscan.NilIfEmptyRawMessage(job.ResultSchema),
		job.DebounceWindowSecs,
		job.BatchWindowSecs,
		job.BatchMaxSize,
		string(job.ExecutionMode),
		dbscan.NilIfEmptyString(string(job.MachinePreset)),
		dbscan.NilIfEmptyString(job.ImageURI),
		dbscan.NilIfEmptyString(job.Region),
		job.PreferredRegions,
		dbscan.NilIfEmptyString(job.OnCompleteTriggerWorkflow),
		dbscan.NilIfEmptyRawMessage(job.OnCompletePayloadMapping),
	).Scan(&job.UpdatedAt, &job.Version, &job.VersionID)
	if err != nil {
		return fmt.Errorf("update job: %w", err)
	}

	return nil
}

// ErrJobHasActiveRuns is returned when attempting to delete a job that has
// queued, dequeued, or executing runs.
var ErrJobHasActiveRuns = errors.New("job has active runs")

func (q *Queries) DeleteJob(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteJob")
	defer span.End()

	// If the underlying connection supports transactions, wrap the whole
	// delete in one so a crash mid-way doesn't leave orphaned data.
	if txb, ok := q.db.(TxBeginner); ok {
		return WithTx(ctx, txb, func(tx *Queries) error {
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
		`SELECT COUNT(*) FROM job_runs WHERE job_id = $1 AND status IN ('queued','delayed','dequeued','executing','waiting')`,
		id,
	).Scan(&activeCount)
	if err != nil {
		return fmt.Errorf("delete job check active runs: %w", err)
	}
	if activeCount > 0 {
		return ErrJobHasActiveRuns
	}

	// Delete related data before removing the job (FK constraints).
	if _, err := q.db.Exec(ctx, `DELETE FROM job_runs WHERE job_id = $1`, id); err != nil {
		return fmt.Errorf("delete job runs: %w", err)
	}
	if _, err := q.db.Exec(ctx, `DELETE FROM job_versions WHERE job_id = $1`, id); err != nil {
		return fmt.Errorf("delete job versions: %w", err)
	}
	if _, err := q.db.Exec(ctx, `DELETE FROM job_dependencies WHERE job_id = $1 OR depends_on_job_id = $1`, id); err != nil {
		return fmt.Errorf("delete job dependencies: %w", err)
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

func (q *Queries) BatchUpdateJobsEnabled(ctx context.Context, ids []string, enabled bool) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.BatchUpdateJobsEnabled")
	defer span.End()

	if len(ids) == 0 {
		return 0, nil
	}

	query := `UPDATE jobs SET enabled = $1, updated_at = NOW() WHERE id = ANY($2)`
	tag, err := q.db.Exec(ctx, query, enabled, ids)
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
		       max_concurrency_per_key, rate_limit_keys, default_run_metadata, retry_priority_boost, dlq_alert_threshold, queue_depth_alert_threshold, skip_if_running, result_schema, debounce_window_secs, batch_window_secs, batch_max_size, execution_mode, machine_preset, image_uri, region, preferred_regions, on_complete_trigger_workflow, on_complete_payload_mapping
		FROM jobs
		WHERE enabled = TRUE AND cron IS NOT NULL AND cron <> ''
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
		       rate_limit_requests, rate_limit_window_secs, compute_daily_cost_limit_microusd, default_region, plan_tier
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
	var computeDailyCostLimit *int64
	var defaultRegion *string
	var planTier *string
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
		&computeDailyCostLimit,
		&defaultRegion,
		&planTier,
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
	if computeDailyCostLimit != nil {
		quota.ComputeDailyCostLimitMicrousd = *computeDailyCostLimit
	}
	if defaultRegion != nil {
		quota.DefaultRegion = *defaultRegion
	}
	if planTier != nil {
		quota.PlanTier = *planTier
	}

	return quota, nil
}

func (q *Queries) CountProjectQueuedRuns(ctx context.Context, projectID string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountProjectQueuedRuns")
	defer span.End()

	query := `
		SELECT COUNT(*)
		FROM job_runs
		WHERE project_id = $1 AND status IN ('queued', 'delayed')`

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
		FROM job_runs
		WHERE project_id = $1 AND status IN ('dequeued', 'executing')`

	var count int
	if err := q.db.QueryRow(ctx, query, projectID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count project active runs: %w", err)
	}

	return count, nil
}

// UpdateProjectDefaultRegion sets the default_region for a project's quota row.
// It upserts the row if it does not exist.
func (q *Queries) UpdateProjectDefaultRegion(ctx context.Context, projectID, defaultRegion string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateProjectDefaultRegion")
	defer span.End()

	query := `
		INSERT INTO project_quotas (project_id, default_region)
		VALUES ($1, $2)
		ON CONFLICT (project_id) DO UPDATE SET default_region = EXCLUDED.default_region`

	_, err := q.db.Exec(ctx, query, projectID, defaultRegion)
	if err != nil {
		return fmt.Errorf("update project default region: %w", err)
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
	var resultSchema []byte
	var debounceWindowSecs *int
	var batchWindowSecs *int
	var batchMaxSize *int
	var executionMode *string
	var machinePreset *string
	var imageURI *string
	var region *string
	var preferredRegions []string
	var onCompleteTriggerWorkflow *string
	var onCompletePayloadMapping []byte

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
		&job.SkipIfRunning,
		&resultSchema,
		&debounceWindowSecs,
		&batchWindowSecs,
		&batchMaxSize,
		&executionMode,
		&machinePreset,
		&imageURI,
		&region,
		&preferredRegions,
		&onCompleteTriggerWorkflow,
		&onCompletePayloadMapping,
	)
	if err != nil {
		return nil, err
	}

	if description != nil {
		job.Description = *description
	}
	if groupID != nil {
		job.GroupID = *groupID
	}
	if cron != nil {
		job.Cron = *cron
	}
	if payloadSchema != nil {
		job.PayloadSchema = json.RawMessage(payloadSchema)
	}
	if len(tagsJSON) > 0 {
		tags, err := unmarshalTags(tagsJSON)
		if err != nil {
			return nil, err
		}
		job.Tags = tags
	}
	if fallbackEndpointURL != nil {
		job.FallbackEndpointURL = *fallbackEndpointURL
	}
	if webhookURL != nil {
		job.WebhookURL = *webhookURL
	}
	if webhookSecret != nil {
		job.WebhookSecret = *webhookSecret
	}
	if runTTLSecs != nil {
		job.RunTTLSecs = *runTTLSecs
	}
	if maxConcurrency != nil {
		job.MaxConcurrency = *maxConcurrency
	}
	if executionWindowCron != nil {
		job.ExecutionWindowCron = *executionWindowCron
	}
	if timezone != nil {
		job.Timezone = *timezone
	}
	if rateLimitMax != nil {
		job.RateLimitMax = *rateLimitMax
	}
	if rateLimitWindowSecs != nil {
		job.RateLimitWindowSecs = *rateLimitWindowSecs
	}
	if dedupWindowSecs != nil {
		job.DedupWindowSecs = *dedupWindowSecs
	}
	if retryStrategy != nil {
		job.RetryStrategy = *retryStrategy
	}
	if len(retryDelaysSecs) > 0 {
		job.RetryDelaysSecs = retryDelaysSecs
	}
	if environmentID != nil {
		job.EnvironmentID = *environmentID
	}
	if versionID != nil {
		job.VersionID = *versionID
	}
	if versionPolicy != nil {
		job.VersionPolicy = domain.VersionPolicy(*versionPolicy)
	}
	if createdBy != nil {
		job.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		job.UpdatedBy = *updatedBy
	}
	if maxConcurrencyPerKey != nil {
		job.MaxConcurrencyPerKey = *maxConcurrencyPerKey
	}
	if len(rateLimitKeysJSON) > 0 && string(rateLimitKeysJSON) != "[]" && string(rateLimitKeysJSON) != "null" {
		if err := json.Unmarshal(rateLimitKeysJSON, &job.RateLimitKeys); err != nil {
			return nil, fmt.Errorf("unmarshal rate_limit_keys: %w", err)
		}
	}
	if len(defaultRunMetadataJSON) > 0 && string(defaultRunMetadataJSON) != "{}" && string(defaultRunMetadataJSON) != "null" {
		if err := json.Unmarshal(defaultRunMetadataJSON, &job.DefaultRunMetadata); err != nil {
			return nil, fmt.Errorf("unmarshal default_run_metadata: %w", err)
		}
	}
	if resultSchema != nil {
		job.ResultSchema = json.RawMessage(resultSchema)
	}
	if debounceWindowSecs != nil {
		job.DebounceWindowSecs = *debounceWindowSecs
	}
	if batchWindowSecs != nil {
		job.BatchWindowSecs = *batchWindowSecs
	}
	if batchMaxSize != nil {
		job.BatchMaxSize = *batchMaxSize
	}
	if executionMode != nil && *executionMode != "" {
		job.ExecutionMode = domain.ExecutionMode(*executionMode)
	}
	if job.ExecutionMode == "" {
		job.ExecutionMode = domain.ExecutionModeHTTP
	}
	if machinePreset != nil {
		job.MachinePreset = domain.MachinePreset(*machinePreset)
	}
	if imageURI != nil {
		job.ImageURI = *imageURI
	}
	if region != nil {
		job.Region = *region
	}
	if len(preferredRegions) > 0 {
		job.PreferredRegions = preferredRegions
	}
	if onCompleteTriggerWorkflow != nil {
		job.OnCompleteTriggerWorkflow = *onCompleteTriggerWorkflow
	}
	if onCompletePayloadMapping != nil {
		job.OnCompletePayloadMapping = json.RawMessage(onCompletePayloadMapping)
	}

	return &job, nil
}

func (q *Queries) ListJobsByTag(ctx context.Context, projectID, tagKey, tagValue string, limit int, cursor *time.Time) ([]domain.Job, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListJobsByTag")
	defer span.End()

	base := `
		SELECT id, project_id, group_id, name, slug, description, cron, payload_schema,
		       tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
		       rate_limit_max, rate_limit_window_secs, dedup_window_secs,
		       enabled, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id, version, version_id, version_policy, backwards_compatible, created_by, updated_by, created_at, updated_at,
		       max_concurrency_per_key, rate_limit_keys, default_run_metadata, retry_priority_boost, dlq_alert_threshold, queue_depth_alert_threshold, skip_if_running, result_schema, debounce_window_secs, batch_window_secs, batch_max_size, execution_mode, machine_preset, image_uri, region, preferred_regions, on_complete_trigger_workflow, on_complete_payload_mapping
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
