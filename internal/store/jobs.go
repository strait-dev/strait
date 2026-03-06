package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"orchestrator/internal/dbscan"
	"orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateJob(ctx context.Context, job *domain.Job) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.CreateJob")
	defer span.End()

	if job.ID == "" {
		job.ID = uuid.Must(uuid.NewV7()).String()
	}
	job.Version = 1

	query := `
		INSERT INTO jobs (
			id, project_id, name, slug, description, cron, payload_schema,
			endpoint_url, max_attempts, timeout_secs, enabled,
			webhook_url, webhook_secret, run_ttl_secs, version
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, 1)
		RETURNING created_at, updated_at, version`

	var runTTL *int
	if job.RunTTLSecs > 0 {
		runTTL = &job.RunTTLSecs
	}

	err := q.db.QueryRow(
		ctx,
		query,
		job.ID,
		job.ProjectID,
		job.Name,
		job.Slug,
		dbscan.NilIfEmptyString(job.Description),
		dbscan.NilIfEmptyString(job.Cron),
		dbscan.NilIfEmptyRawMessage(job.PayloadSchema),
		job.EndpointURL,
		job.MaxAttempts,
		job.TimeoutSecs,
		job.Enabled,
		dbscan.NilIfEmptyString(job.WebhookURL),
		dbscan.NilIfEmptyString(job.WebhookSecret),
		runTTL,
	).Scan(&job.CreatedAt, &job.UpdatedAt, &job.Version)
	if err != nil {
		return fmt.Errorf("create job: %w", err)
	}

	return nil
}

func (q *Queries) GetJob(ctx context.Context, id string) (*domain.Job, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.GetJob")
	defer span.End()

	query := `
		SELECT id, project_id, name, slug, description, cron, payload_schema,
		       endpoint_url, max_attempts, timeout_secs, enabled, webhook_url, webhook_secret, run_ttl_secs, version, created_at, updated_at
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
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.GetJobBySlug")
	defer span.End()

	query := `
		SELECT id, project_id, name, slug, description, cron, payload_schema,
		       endpoint_url, max_attempts, timeout_secs, enabled, webhook_url, webhook_secret, run_ttl_secs, version, created_at, updated_at
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

func (q *Queries) ListJobs(ctx context.Context, projectID string) ([]domain.Job, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListJobs")
	defer span.End()

	query := `
		SELECT id, project_id, name, slug, description, cron, payload_schema,
		       endpoint_url, max_attempts, timeout_secs, enabled, webhook_url, webhook_secret, run_ttl_secs, version, created_at, updated_at
		FROM jobs
		WHERE project_id = $1
		ORDER BY created_at DESC`

	rows, err := q.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]domain.Job, 0, 16)
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
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.UpdateJob")
	defer span.End()

	query := `
		WITH snapshot AS (
			INSERT INTO job_versions (id, job_id, version, name, slug, description, cron, payload_schema,
				endpoint_url, max_attempts, timeout_secs, webhook_url, webhook_secret, run_ttl_secs)
			SELECT $14, id, version, name, slug, description, cron, payload_schema,
				endpoint_url, max_attempts, timeout_secs, webhook_url, webhook_secret, run_ttl_secs
			FROM jobs WHERE id = $13
		)
		UPDATE jobs
		SET name = $1,
		    slug = $2,
		    description = $3,
		    cron = $4,
		    payload_schema = $5,
		    endpoint_url = $6,
		    max_attempts = $7,
		    timeout_secs = $8,
		    enabled = $9,
		    webhook_url = $10,
		    webhook_secret = $11,
		    run_ttl_secs = $12,
		    version = version + 1,
		    updated_at = NOW()
		WHERE id = $13
		RETURNING updated_at, version`

	var runTTL *int
	if job.RunTTLSecs > 0 {
		runTTL = &job.RunTTLSecs
	}

	err := q.db.QueryRow(
		ctx,
		query,
		job.Name,
		job.Slug,
		dbscan.NilIfEmptyString(job.Description),
		dbscan.NilIfEmptyString(job.Cron),
		dbscan.NilIfEmptyRawMessage(job.PayloadSchema),
		job.EndpointURL,
		job.MaxAttempts,
		job.TimeoutSecs,
		job.Enabled,
		dbscan.NilIfEmptyString(job.WebhookURL),
		dbscan.NilIfEmptyString(job.WebhookSecret),
		runTTL,
		job.ID,
		uuid.Must(uuid.NewV7()).String(),
	).Scan(&job.UpdatedAt, &job.Version)
	if err != nil {
		return fmt.Errorf("update job: %w", err)
	}

	return nil
}

func (q *Queries) DeleteJob(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.DeleteJob")
	defer span.End()

	query := `DELETE FROM jobs WHERE id = $1`

	if _, err := q.db.Exec(ctx, query, id); err != nil {
		return fmt.Errorf("delete job: %w", err)
	}

	return nil
}

func (q *Queries) ListCronJobs(ctx context.Context) ([]domain.Job, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListCronJobs")
	defer span.End()

	query := `
		SELECT id, project_id, name, slug, description, cron, payload_schema,
		       endpoint_url, max_attempts, timeout_secs, enabled, webhook_url, webhook_secret, run_ttl_secs, version, created_at, updated_at
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

type scanTarget interface {
	Scan(dest ...any) error
}

func scanJob(scanner scanTarget) (*domain.Job, error) {
	var job domain.Job
	var description *string
	var cron *string
	var payloadSchema []byte
	var webhookURL *string
	var webhookSecret *string
	var runTTLSecs *int

	err := scanner.Scan(
		&job.ID,
		&job.ProjectID,
		&job.Name,
		&job.Slug,
		&description,
		&cron,
		&payloadSchema,
		&job.EndpointURL,
		&job.MaxAttempts,
		&job.TimeoutSecs,
		&job.Enabled,
		&webhookURL,
		&webhookSecret,
		&runTTLSecs,
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
	if cron != nil {
		job.Cron = *cron
	}
	if payloadSchema != nil {
		job.PayloadSchema = json.RawMessage(payloadSchema)
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

	return &job, nil
}
