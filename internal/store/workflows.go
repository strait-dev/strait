package store

import (
	"context"
	"errors"
	"fmt"

	"orchestrator/internal/dbscan"
	"orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

func (q *Queries) CreateWorkflow(ctx context.Context, w *domain.Workflow) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.CreateWorkflow")
	defer span.End()

	if w.ID == "" {
		w.ID = uuid.Must(uuid.NewV7()).String()
	}
	w.Version = 1

	query := `
		INSERT INTO workflows (id, project_id, name, slug, description, enabled, version, timeout_secs, max_concurrent_runs)
		VALUES ($1, $2, $3, $4, $5, $6, 1, $7, $8)
		RETURNING created_at, updated_at, version`

	err := q.db.QueryRow(
		ctx,
		query,
		w.ID,
		w.ProjectID,
		w.Name,
		w.Slug,
		dbscan.NilIfEmptyString(w.Description),
		w.Enabled,
		w.TimeoutSecs,
		w.MaxConcurrentRuns,
	).Scan(&w.CreatedAt, &w.UpdatedAt, &w.Version)
	if err != nil {
		return fmt.Errorf("create workflow: %w", err)
	}

	return nil
}

func (q *Queries) GetWorkflow(ctx context.Context, id string) (*domain.Workflow, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.GetWorkflow")
	defer span.End()

	query := `
		SELECT id, project_id, name, slug, description, enabled, version, timeout_secs, max_concurrent_runs, created_at, updated_at
		FROM workflows
		WHERE id = $1`

	w, err := scanWorkflow(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWorkflowNotFound
		}
		return nil, fmt.Errorf("get workflow: %w", err)
	}

	return w, nil
}

func (q *Queries) GetWorkflowBySlug(ctx context.Context, projectID, slug string) (*domain.Workflow, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.GetWorkflowBySlug")
	defer span.End()

	query := `
		SELECT id, project_id, name, slug, description, enabled, version, timeout_secs, max_concurrent_runs, created_at, updated_at
		FROM workflows
		WHERE project_id = $1 AND slug = $2`

	w, err := scanWorkflow(q.db.QueryRow(ctx, query, projectID, slug))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWorkflowNotFound
		}
		return nil, fmt.Errorf("get workflow by slug: %w", err)
	}

	return w, nil
}

func (q *Queries) ListWorkflows(ctx context.Context, projectID string) ([]domain.Workflow, error) {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.ListWorkflows")
	defer span.End()

	query := `
		SELECT id, project_id, name, slug, description, enabled, version, timeout_secs, max_concurrent_runs, created_at, updated_at
		FROM workflows
		WHERE project_id = $1
		ORDER BY created_at DESC`

	rows, err := q.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}
	defer rows.Close()

	workflows := make([]domain.Workflow, 0)
	for rows.Next() {
		workflow, scanErr := scanWorkflow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list workflows scan: %w", scanErr)
		}
		workflows = append(workflows, *workflow)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflows rows: %w", err)
	}

	return workflows, nil
}

func (q *Queries) UpdateWorkflow(ctx context.Context, w *domain.Workflow) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.UpdateWorkflow")
	defer span.End()

	query := `
		UPDATE workflows
		SET name = $1,
		    slug = $2,
		    description = $3,
		    enabled = $4,
		    timeout_secs = $5,
		    max_concurrent_runs = $6,
		    version = version + 1,
		    updated_at = NOW()
		WHERE id = $7
		RETURNING updated_at, version`

	err := q.db.QueryRow(
		ctx,
		query,
		w.Name,
		w.Slug,
		dbscan.NilIfEmptyString(w.Description),
		w.Enabled,
		w.TimeoutSecs,
		w.MaxConcurrentRuns,
		w.ID,
	).Scan(&w.UpdatedAt, &w.Version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrWorkflowNotFound
		}
		return fmt.Errorf("update workflow: %w", err)
	}

	return nil
}

func (q *Queries) DeleteWorkflow(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("orchestrator").Start(ctx, "store.DeleteWorkflow")
	defer span.End()

	query := `DELETE FROM workflows WHERE id = $1`

	if _, err := q.db.Exec(ctx, query, id); err != nil {
		return fmt.Errorf("delete workflow: %w", err)
	}

	return nil
}

func scanWorkflow(scanner scanTarget) (*domain.Workflow, error) {
	var workflow domain.Workflow
	var description *string

	err := scanner.Scan(
		&workflow.ID,
		&workflow.ProjectID,
		&workflow.Name,
		&workflow.Slug,
		&description,
		&workflow.Enabled,
		&workflow.Version,
		&workflow.TimeoutSecs,
		&workflow.MaxConcurrentRuns,
		&workflow.CreatedAt,
		&workflow.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if description != nil {
		workflow.Description = *description
	}

	return &workflow, nil
}
