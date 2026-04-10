package store

import (
	"context"
	"errors"
	"fmt"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
)

var ErrProjectNotFound = errors.New("project not found")

// CreateProject upserts a project row. On conflict (same ID), it updates
// name and updated_at, clears any prior tombstone, and preserves org_id when
// the incoming value is empty.
func (q *Queries) CreateProject(ctx context.Context, project *domain.Project) error {
	err := q.db.QueryRow(ctx, `
		INSERT INTO projects (id, org_id, name, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP, NOW(), NULL)
		ON CONFLICT (id) DO UPDATE SET
			org_id     = COALESCE(NULLIF(EXCLUDED.org_id, ''), projects.org_id),
			name       = EXCLUDED.name,
			updated_at = NOW(),
			deleted_at = NULL
		RETURNING created_at, updated_at`,
		project.ID, project.OrgID, project.Name,
	).Scan(&project.CreatedAt, &project.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	return nil
}

// GetProject returns a project by ID or ErrProjectNotFound.
func (q *Queries) GetProject(ctx context.Context, id string) (*domain.Project, error) {
	var p domain.Project
	err := q.db.QueryRow(ctx, `
		SELECT id, org_id, name, created_at, updated_at
		FROM projects
		WHERE id = $1
		  AND deleted_at IS NULL`, id,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("get project: %w", err)
	}
	return &p, nil
}

// ListProjectsByOrg returns all projects for an organization ordered by created_at.
func (q *Queries) ListProjectsByOrg(ctx context.Context, orgID string) ([]domain.Project, error) {
	rows, err := q.db.Query(ctx, `
		SELECT id, org_id, name, created_at, updated_at
		FROM projects
		WHERE org_id = $1
		  AND deleted_at IS NULL
		ORDER BY created_at ASC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list projects by org: %w", err)
	}
	defer rows.Close()

	var projects []domain.Project
	for rows.Next() {
		var p domain.Project
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// DeleteProject tombstones a project and cleans up active child records within
// a transaction. Jobs are soft-disabled (not deleted) to avoid FK violations
// from existing runs while preserving historical org mapping for billing.
func (q *Queries) DeleteProject(ctx context.Context, id string) error {
	beginner, ok := q.db.(TxBeginner)
	if !ok {
		// Fallback for non-transactional callers (e.g. within an existing tx).
		return q.deleteProjectRows(ctx, id)
	}

	tx, err := beginner.Begin(ctx)
	if err != nil {
		return fmt.Errorf("delete project begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tq := New(tx)
	if err := tq.deleteProjectRows(ctx, id); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (q *Queries) deleteProjectRows(ctx context.Context, id string) error {
	// Verify the project is still active first.
	var exists bool
	err := q.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM projects
			WHERE id = $1
			  AND deleted_at IS NULL
		)
	`, id).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check project exists: %w", err)
	}
	if !exists {
		return ErrProjectNotFound
	}

	// Clean up active child records before tombstoning the project row.
	// Delete from both the legacy project_quotas and the three per-concern
	// tables introduced in phase C so projects stay tidy during cutover.
	cleanupQueries := []string{
		`DELETE FROM project_member_roles WHERE project_id = $1`,
		`DELETE FROM project_roles WHERE project_id = $1`,
		`DELETE FROM api_keys WHERE project_id = $1`,
		`DELETE FROM project_quotas WHERE project_id = $1`,
		`DELETE FROM project_job_quotas WHERE project_id = $1`,
		`DELETE FROM project_agent_quotas WHERE project_id = $1`,
		`DELETE FROM project_platform_settings WHERE project_id = $1`,
		`DELETE FROM usage_records WHERE project_id = $1`,
		`UPDATE jobs SET enabled = false WHERE project_id = $1`,
		`UPDATE projects SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1`,
	}

	for _, query := range cleanupQueries {
		if _, err := q.db.Exec(ctx, query, id); err != nil {
			return fmt.Errorf("delete project cleanup: %w", err)
		}
	}

	return nil
}

// CountProjectsByOrg returns the number of projects belonging to an organization.
func (q *Queries) CountProjectsByOrg(ctx context.Context, orgID string) (int, error) {
	var count int
	err := q.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM projects WHERE org_id = $1 AND deleted_at IS NULL`, orgID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count projects by org: %w", err)
	}
	return count, nil
}
