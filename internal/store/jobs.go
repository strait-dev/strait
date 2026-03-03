package store

import (
	"context"
	"encoding/json"
	"fmt"

	"orchestrator/internal/dbscan"
	"orchestrator/internal/domain"

	"github.com/google/uuid"
)

func (q *Queries) CreateJob(ctx context.Context, job *domain.Job) error {
	if job.ID == "" {
		job.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO jobs (
			id, project_id, name, slug, description, cron, payload_schema,
			endpoint_url, max_attempts, timeout_secs, enabled,
			webhook_url, webhook_secret
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING created_at, updated_at`

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
	).Scan(&job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create job: %w", err)
	}

	return nil
}

func (q *Queries) GetJob(ctx context.Context, id string) (*domain.Job, error) {
	query := `
		SELECT id, project_id, name, slug, description, cron, payload_schema,
		       endpoint_url, max_attempts, timeout_secs, enabled, webhook_url, webhook_secret, created_at, updated_at
		FROM jobs
		WHERE id = $1`

	job, err := scanJob(q.db.QueryRow(ctx, query, id))
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}

	return job, nil
}

func (q *Queries) GetJobBySlug(ctx context.Context, projectID, slug string) (*domain.Job, error) {
	query := `
		SELECT id, project_id, name, slug, description, cron, payload_schema,
		       endpoint_url, max_attempts, timeout_secs, enabled, webhook_url, webhook_secret, created_at, updated_at
		FROM jobs
		WHERE project_id = $1 AND slug = $2`

	job, err := scanJob(q.db.QueryRow(ctx, query, projectID, slug))
	if err != nil {
		return nil, fmt.Errorf("get job by slug: %w", err)
	}

	return job, nil
}

func (q *Queries) ListJobs(ctx context.Context, projectID string) ([]domain.Job, error) {
	query := `
		SELECT id, project_id, name, slug, description, cron, payload_schema,
		       endpoint_url, max_attempts, timeout_secs, enabled, webhook_url, webhook_secret, created_at, updated_at
		FROM jobs
		WHERE project_id = $1
		ORDER BY created_at DESC`

	rows, err := q.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]domain.Job, 0)
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
	query := `
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
		    updated_at = NOW()
		WHERE id = $12
		RETURNING updated_at`

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
		job.ID,
	).Scan(&job.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update job: %w", err)
	}

	return nil
}

func (q *Queries) DeleteJob(ctx context.Context, id string) error {
	query := `DELETE FROM jobs WHERE id = $1`

	if _, err := q.db.Exec(ctx, query, id); err != nil {
		return fmt.Errorf("delete job: %w", err)
	}

	return nil
}

func (q *Queries) ListCronJobs(ctx context.Context) ([]domain.Job, error) {
	query := `
		SELECT id, project_id, name, slug, description, cron, payload_schema,
		       endpoint_url, max_attempts, timeout_secs, enabled, webhook_url, webhook_secret, created_at, updated_at
		FROM jobs
		WHERE enabled = TRUE AND cron IS NOT NULL AND cron <> ''
		ORDER BY created_at DESC`

	rows, err := q.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list cron jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]domain.Job, 0)
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

	return &job, nil
}
