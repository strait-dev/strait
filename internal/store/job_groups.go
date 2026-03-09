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

func (q *Queries) CreateJobGroup(ctx context.Context, group *domain.JobGroup) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateJobGroup")
	defer span.End()

	if group.ID == "" {
		group.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO job_groups (id, project_id, name, slug, description)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at`

	err := q.db.QueryRow(
		ctx,
		query,
		group.ID,
		group.ProjectID,
		group.Name,
		group.Slug,
		dbscan.NilIfEmptyString(group.Description),
	).Scan(&group.CreatedAt, &group.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create job group: %w", err)
	}

	return nil
}

func (q *Queries) GetJobGroup(ctx context.Context, id string) (*domain.JobGroup, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetJobGroup")
	defer span.End()

	query := `
		SELECT id, project_id, name, slug, description, created_at, updated_at
		FROM job_groups
		WHERE id = $1`

	group, err := scanJobGroup(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrJobGroupNotFound
		}
		return nil, fmt.Errorf("get job group: %w", err)
	}

	return group, nil
}

func (q *Queries) ListJobGroups(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobGroup, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListJobGroups")
	defer span.End()

	query := `
		SELECT id, project_id, name, slug, description, created_at, updated_at
		FROM job_groups
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
		return nil, fmt.Errorf("list job groups: %w", err)
	}
	defer rows.Close()

	groups := make([]domain.JobGroup, 0)
	for rows.Next() {
		group, scanErr := scanJobGroup(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list job groups scan: %w", scanErr)
		}
		groups = append(groups, *group)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list job groups rows: %w", err)
	}

	return groups, nil
}

func (q *Queries) UpdateJobGroup(ctx context.Context, group *domain.JobGroup) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateJobGroup")
	defer span.End()

	query := `
		UPDATE job_groups
		SET name = $1,
		    slug = $2,
		    description = $3,
		    updated_at = NOW()
		WHERE id = $4
		RETURNING updated_at`

	err := q.db.QueryRow(
		ctx,
		query,
		group.Name,
		group.Slug,
		dbscan.NilIfEmptyString(group.Description),
		group.ID,
	).Scan(&group.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrJobGroupNotFound
		}
		return fmt.Errorf("update job group: %w", err)
	}

	return nil
}

func (q *Queries) DeleteJobGroup(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteJobGroup")
	defer span.End()

	query := `WITH nullify AS (
		UPDATE jobs SET group_id = NULL WHERE group_id = $1
	)
	DELETE FROM job_groups WHERE id = $1`
	tag, err := q.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete job group: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrJobGroupNotFound
	}

	return nil
}

func (q *Queries) ListJobsByGroup(ctx context.Context, groupID string, limit int, cursor *time.Time) ([]domain.Job, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListJobsByGroup")
	defer span.End()

	query := `
		SELECT id, project_id, group_id, name, slug, description, cron, payload_schema,
		       tags, endpoint_url, fallback_endpoint_url, max_attempts, timeout_secs, max_concurrency, execution_window_cron, timezone,
		       rate_limit_max, rate_limit_window_secs, dedup_window_secs,
		       enabled, webhook_url, webhook_secret, run_ttl_secs, retry_strategy, retry_delays_secs, environment_id, version, version_id, version_policy, backwards_compatible, created_by, updated_by, created_at, updated_at
		FROM jobs
		WHERE group_id = $1`

	args := []any{groupID}
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
		return nil, fmt.Errorf("list jobs by group: %w", err)
	}
	defer rows.Close()

	jobs := make([]domain.Job, 0)
	for rows.Next() {
		job, scanErr := scanJob(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list jobs by group scan: %w", scanErr)
		}
		jobs = append(jobs, *job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list jobs by group rows: %w", err)
	}

	return jobs, nil
}

func scanJobGroup(scanner scanTarget) (*domain.JobGroup, error) {
	var group domain.JobGroup
	var description *string

	err := scanner.Scan(
		&group.ID,
		&group.ProjectID,
		&group.Name,
		&group.Slug,
		&description,
		&group.CreatedAt,
		&group.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if description != nil {
		group.Description = *description
	}

	return &group, nil
}
