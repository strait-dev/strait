package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateJobVersion(ctx context.Context, v *domain.JobVersion) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateJobVersion")
	defer span.End()

	query := `
		INSERT INTO job_versions (id, job_id, version, version_id, backwards_compatible, name, slug, description, cron, payload_schema,
			tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, webhook_url, webhook_secret, run_ttl_secs)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, $12, $13, $14, $15, $16, $17, $18)
		RETURNING created_at`

	var desc, cronStr, webhookURL, webhookSecret *string
	var payloadSchema []byte
	var runTTL *int
	if v.Description != "" {
		desc = &v.Description
	}
	if v.Cron != "" {
		cronStr = &v.Cron
	}
	if len(v.PayloadSchema) > 0 {
		payloadSchema = v.PayloadSchema
	}
	if v.WebhookURL != "" {
		webhookURL = &v.WebhookURL
	}
	if v.WebhookSecret != "" {
		webhookSecret = &v.WebhookSecret
	}
	if v.RunTTLSecs > 0 {
		runTTL = &v.RunTTLSecs
	}

	tagsJSON, err := marshalTags(v.Tags)
	if err != nil {
		return fmt.Errorf("create job version: %w", err)
	}

	return q.db.QueryRow(ctx, query,
		v.ID, v.JobID, v.Version, dbscan.NilIfEmptyString(v.VersionID), v.BackwardsCompatible,
		v.Name, v.Slug, desc, cronStr, payloadSchema,
		tagsJSON, v.EndpointURL, dbscan.NilIfEmptyString(v.FallbackEndpointURL), v.MaxAttempts, v.TimeoutSecs, webhookURL, webhookSecret, runTTL,
	).Scan(&v.CreatedAt)
}

func (q *Queries) ListJobVersionsByJob(ctx context.Context, jobID string, limit int, cursor *time.Time) ([]domain.JobVersion, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListJobVersionsByJob")
	defer span.End()

	query := `
		SELECT id, job_id, version, version_id, backwards_compatible,
		       name, slug, description, cron, payload_schema,
		       tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, webhook_url, webhook_secret, run_ttl_secs, created_at
		FROM job_versions
		WHERE job_id = $1`

	args := []any{jobID}
	param := 2

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY version DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list job versions: %w", err)
	}
	defer rows.Close()

	versions := make([]domain.JobVersion, 0, 16)
	for rows.Next() {
		v, err := scanJobVersion(rows)
		if err != nil {
			return nil, fmt.Errorf("list job versions scan: %w", err)
		}
		versions = append(versions, *v)
	}
	return versions, rows.Err()
}

func (q *Queries) GetJobVersion(ctx context.Context, jobID string, version int) (*domain.JobVersion, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJobVersion")
	defer span.End()

	query := `
		SELECT id, job_id, version, version_id, backwards_compatible,
		       name, slug, description, cron, payload_schema,
		       tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, webhook_url, webhook_secret, run_ttl_secs, created_at
		FROM job_versions
		WHERE job_id = $1 AND version = $2`

	v, err := scanJobVersion(q.db.QueryRow(ctx, query, jobID, version))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("job version not found")
		}
		return nil, fmt.Errorf("get job version: %w", err)
	}
	return v, nil
}

// GetJobAtVersion returns the job configuration as it existed at the given version.
// It reads from the job_versions snapshot table. If no snapshot exists for the
// requested version (e.g., version 1 before snapshotting was enabled), it falls
// back to the live jobs table.
//
// Note: version_policy, created_by, and updated_by are read from the live jobs
// table since they are not stored per-version in job_versions.
func (q *Queries) GetJobAtVersion(ctx context.Context, jobID string, version int) (*domain.Job, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJobAtVersion")
	defer span.End()

	query := `
		SELECT jv.job_id, COALESCE(jv.project_id, j.project_id), COALESCE(jv.group_id, j.group_id),
		       jv.name, jv.slug, jv.description, jv.cron, jv.payload_schema,
		       jv.tags, jv.endpoint_url, jv.fallback_endpoint_url, jv.max_attempts, jv.timeout_secs,
		       COALESCE(jv.max_concurrency, j.max_concurrency), COALESCE(jv.execution_window_cron, j.execution_window_cron),
		       COALESCE(jv.timezone, j.timezone),
		       COALESCE(jv.rate_limit_max, j.rate_limit_max), COALESCE(jv.rate_limit_window_secs, j.rate_limit_window_secs),
		       COALESCE(jv.dedup_window_secs, j.dedup_window_secs),
		       COALESCE(jv.enabled, j.enabled), jv.webhook_url, jv.webhook_secret, jv.run_ttl_secs,
		       COALESCE(jv.retry_strategy, j.retry_strategy), COALESCE(jv.retry_delays_secs, j.retry_delays_secs),
		       COALESCE(jv.environment_id, j.environment_id),
		       jv.version, jv.version_id, j.version_policy, COALESCE(jv.backwards_compatible, j.backwards_compatible),
		       COALESCE(NULLIF(jv.created_by, ''), j.created_by), COALESCE(NULLIF(jv.updated_by, ''), j.updated_by),
		       jv.created_at, j.updated_at,
		       COALESCE(jv.max_concurrency_per_key, j.max_concurrency_per_key),
		       COALESCE(jv.rate_limit_keys, j.rate_limit_keys),
		       COALESCE(jv.default_run_metadata, j.default_run_metadata),
		       COALESCE(jv.retry_priority_boost, j.retry_priority_boost),
		       COALESCE(jv.dlq_alert_threshold, j.dlq_alert_threshold),
		       COALESCE(jv.queue_depth_alert_threshold, j.queue_depth_alert_threshold),
		       jv.poison_pill_threshold,
		       COALESCE(jv.cron_overlap_policy, j.cron_overlap_policy),
		       COALESCE(jv.result_schema, j.result_schema),
		       jv.debounce_window_secs,
		       jv.batch_window_secs,
		       jv.batch_max_size,
		       jv.execution_mode,
		       jv.preferred_regions,
		       jv.queue_name,
		       jv.on_complete_trigger_workflow,
		       jv.on_complete_trigger_job,
		       jv.on_complete_payload_mapping,
		       jv.on_failure_trigger_job,
		       jv.on_failure_trigger_workflow,
		       jv.on_failure_payload_mapping,
		       jv.paused, jv.paused_at, jv.pause_reason,
		       jv.endpoint_signing_secret, jv.version::bigint
		FROM job_versions jv
		JOIN jobs j ON j.id = jv.job_id
		WHERE jv.job_id = $1 AND jv.version = $2`

	job, err := scanJob(q.db.QueryRow(ctx, query, jobID, version))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No snapshot for this version — fall back to live job.
			return q.GetJob(ctx, jobID)
		}
		return nil, fmt.Errorf("get job at version: %w", err)
	}

	return job, nil
}

func scanJobVersion(scanner scanTarget) (*domain.JobVersion, error) {
	var v domain.JobVersion
	var versionID *string
	var description, cronStr, webhookURL, webhookSecret *string
	var fallbackEndpointURL *string
	var payloadSchema []byte
	var tagsJSON []byte
	var runTTLSecs *int

	err := scanner.Scan(
		&v.ID, &v.JobID, &v.Version, &versionID, &v.BackwardsCompatible,
		&v.Name, &v.Slug,
		&description, &cronStr, &payloadSchema,
		&tagsJSON, &v.EndpointURL, &fallbackEndpointURL, &v.MaxAttempts, &v.TimeoutSecs,
		&webhookURL, &webhookSecret, &runTTLSecs,
		&v.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if versionID != nil {
		v.VersionID = *versionID
	}
	if description != nil {
		v.Description = *description
	}
	if cronStr != nil {
		v.Cron = *cronStr
	}
	if payloadSchema != nil {
		v.PayloadSchema = json.RawMessage(payloadSchema)
	}
	if len(tagsJSON) > 0 {
		tags, unmarshalErr := unmarshalTags(tagsJSON)
		if unmarshalErr != nil {
			return nil, unmarshalErr
		}
		v.Tags = tags
	}
	if fallbackEndpointURL != nil {
		v.FallbackEndpointURL = *fallbackEndpointURL
	}
	if webhookURL != nil {
		v.WebhookURL = *webhookURL
	}
	if webhookSecret != nil {
		v.WebhookSecret = *webhookSecret
	}
	if runTTLSecs != nil {
		v.RunTTLSecs = *runTTLSecs
	}
	return &v, nil
}

// GetJobVersionByVersionID looks up a specific version by its nanoid version_id.
func (q *Queries) GetJobVersionByVersionID(ctx context.Context, versionID string) (*domain.JobVersion, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJobVersionByVersionID")
	defer span.End()

	query := `
		SELECT id, job_id, version, version_id, backwards_compatible,
		       name, slug, description, cron, payload_schema,
		       tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, webhook_url, webhook_secret, run_ttl_secs, created_at
		FROM job_versions
		WHERE version_id = $1`

	v, err := scanJobVersion(q.db.QueryRow(ctx, query, versionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrJobNotFound
		}
		return nil, fmt.Errorf("get job version by version_id: %w", err)
	}
	return v, nil
}
