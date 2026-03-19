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
// name and updated_at, and preserves org_id when the incoming value is empty.
func (q *Queries) CreateProject(ctx context.Context, project *domain.Project) error {
	err := q.db.QueryRow(ctx, `
		INSERT INTO projects (id, org_id, name, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE SET
			org_id     = COALESCE(NULLIF(EXCLUDED.org_id, ''), projects.org_id),
			name       = EXCLUDED.name,
			updated_at = NOW()
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
		FROM projects WHERE id = $1`, id,
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
		FROM projects WHERE org_id = $1
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

// DeleteProject removes a project and its associated API keys, quotas, roles, and member roles.
func (q *Queries) DeleteProject(ctx context.Context, id string) error {
	tag, err := q.db.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrProjectNotFound
	}
	return nil
}

// CountProjectsByOrg returns the number of projects belonging to an organization.
func (q *Queries) CountProjectsByOrg(ctx context.Context, orgID string) (int, error) {
	var count int
	err := q.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM projects WHERE org_id = $1`, orgID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count projects by org: %w", err)
	}
	return count, nil
}
