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

func (q *Queries) CreateJob(ctx context.Context, job *domain.Job) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateJob")
	defer span.End()

	if job.ID == "" {
		job.ID = uuid.Must(uuid.NewV7()).String()
	}
	job.Version = 1

	query := `
		INSERT INTO jobs (
			id, project_id, group_id, name, slug, description, cron, payload_schema,
			tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
			rate_limit_max, rate_limit_window_secs, dedup_window_secs, enabled,
			webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id,
			execution_mode, sandbox_code, sandbox_language, cancel_endpoint_url, version
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, 1)
		RETURNING created_at, updated_at, version`

	tagsJSON, err := marshalJobTags(job.Tags)
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
		string(job.ExecutionMode),
		dbscan.NilIfEmptyString(job.SandboxCode),
		dbscan.NilIfEmptyString(job.SandboxLanguage),
		dbscan.NilIfEmptyString(job.CancelEndpointURL),
	).Scan(&job.CreatedAt, &job.UpdatedAt, &job.Version)
	if err != nil {
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
		       enabled, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id, execution_mode, sandbox_code, sandbox_language, cancel_endpoint_url, version, created_at, updated_at
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
		       enabled, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id, execution_mode, sandbox_code, sandbox_language, cancel_endpoint_url, version, created_at, updated_at
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
		       enabled, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id, execution_mode, sandbox_code, sandbox_language, cancel_endpoint_url, version, created_at, updated_at
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

	query := `
		WITH snapshot AS (
			INSERT INTO job_versions (id, job_id, version, name, slug, description, cron, payload_schema,
				tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
				rate_limit_max, rate_limit_window_secs, dedup_window_secs, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id,
				execution_mode, sandbox_code, sandbox_language, cancel_endpoint_url)
			SELECT $30, id, version, name, slug, description, cron, payload_schema,
				tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
				rate_limit_max, rate_limit_window_secs, dedup_window_secs, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id,
				execution_mode, sandbox_code, sandbox_language, cancel_endpoint_url
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
		    execution_mode = $26,
		    sandbox_code = $27,
		    sandbox_language = $28,
		    cancel_endpoint_url = $29,
		    version = version + 1,
		    updated_at = NOW()
		WHERE id = $25
		RETURNING updated_at, version`

	tagsJSON, err := marshalJobTags(job.Tags)
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
		string(job.ExecutionMode),
		dbscan.NilIfEmptyString(job.SandboxCode),
		dbscan.NilIfEmptyString(job.SandboxLanguage),
		dbscan.NilIfEmptyString(job.CancelEndpointURL),
		uuid.Must(uuid.NewV7()).String(),
	).Scan(&job.UpdatedAt, &job.Version)
	if err != nil {
		return fmt.Errorf("update job: %w", err)
	}

	return nil
}

func (q *Queries) DeleteJob(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteJob")
	defer span.End()

	query := `DELETE FROM jobs WHERE id = $1`

	if _, err := q.db.Exec(ctx, query, id); err != nil {
		return fmt.Errorf("delete job: %w", err)
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
		       enabled, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id, execution_mode, sandbox_code, sandbox_language, cancel_endpoint_url, version, created_at, updated_at
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
		SELECT project_id, max_queued_runs, max_executing_runs, max_jobs, timezone, max_cost_per_run_microusd, max_daily_cost_microusd
		FROM project_quotas
		WHERE project_id = $1`

	quota := &ProjectQuota{}
	var maxQueued *int
	var maxExecuting *int
	var maxJobs *int
	var timezone *string
	var maxCostPerRun *int64
	var maxDailyCost *int64
	err := q.db.QueryRow(ctx, query, projectID).Scan(
		&quota.ProjectID,
		&maxQueued,
		&maxExecuting,
		&maxJobs,
		&timezone,
		&maxCostPerRun,
		&maxDailyCost,
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
	var executionMode *string
	var sandboxCode *string
	var sandboxLanguage *string
	var cancelEndpointURL *string

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
		&executionMode,
		&sandboxCode,
		&sandboxLanguage,
		&cancelEndpointURL,
		&job.Version,
		&job.CreatedAt,
		&job.UpdatedAt,
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
		tags, err := unmarshalJobTags(tagsJSON)
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
	if executionMode != nil {
		job.ExecutionMode = domain.ExecutionMode(*executionMode)
	}
	if sandboxCode != nil {
		job.SandboxCode = *sandboxCode
	}
	if sandboxLanguage != nil {
		job.SandboxLanguage = *sandboxLanguage
	}
	if cancelEndpointURL != nil {
		job.CancelEndpointURL = *cancelEndpointURL
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
		       enabled, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id, execution_mode, sandbox_code, sandbox_language, cancel_endpoint_url, version, created_at, updated_at
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

	jobs := make([]domain.Job, 0)
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

func marshalJobTags(tags map[string]string) ([]byte, error) {
	if len(tags) == 0 {
		return []byte(`{}`), nil
	}

	encoded, err := json.Marshal(tags)
	if err != nil {
		return nil, fmt.Errorf("marshal job tags: %w", err)
	}
	return encoded, nil
}

func unmarshalJobTags(raw []byte) (map[string]string, error) {
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
