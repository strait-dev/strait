package store

import (
	"context"
	"errors"
	"fmt"

	"strait/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

var (
	ErrRoleNotFound           = errors.New("role not found")
	ErrMemberNotFound         = errors.New("member not found")
	ErrResourcePolicyNotFound = errors.New("resource policy not found")
)

func (q *Queries) CreateProjectRole(ctx context.Context, role *domain.ProjectRole) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateProjectRole")
	defer span.End()

	if role.ID == "" {
		role.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO project_roles (id, project_id, name, description, permissions, is_system)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, updated_at`

	err := q.db.QueryRow(ctx, query,
		role.ID, role.ProjectID, role.Name, role.Description, role.Permissions, role.IsSystem,
	).Scan(&role.CreatedAt, &role.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create project role: %w", err)
	}

	return nil
}

func (q *Queries) GetProjectRole(ctx context.Context, id string) (*domain.ProjectRole, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetProjectRole")
	defer span.End()

	query := `
		SELECT id, project_id, name, description, permissions, is_system, created_at, updated_at
		FROM project_roles
		WHERE id = $1`

	role, err := scanProjectRole(q.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoleNotFound
		}
		return nil, fmt.Errorf("get project role: %w", err)
	}

	return role, nil
}

func (q *Queries) ListProjectRoles(ctx context.Context, projectID string) ([]domain.ProjectRole, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListProjectRoles")
	defer span.End()

	query := `
		SELECT id, project_id, name, description, permissions, is_system, created_at, updated_at
		FROM project_roles
		WHERE project_id = $1
		ORDER BY is_system DESC, name ASC`

	rows, err := q.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project roles: %w", err)
	}
	defer rows.Close()

	roles := make([]domain.ProjectRole, 0, 8)
	for rows.Next() {
		role, scanErr := scanProjectRole(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list project roles scan: %w", scanErr)
		}
		roles = append(roles, *role)
	}

	return roles, rows.Err()
}

func (q *Queries) DeleteProjectRole(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteProjectRole")
	defer span.End()

	query := `DELETE FROM project_roles WHERE id = $1 AND is_system = FALSE`
	tag, err := q.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete project role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRoleNotFound
	}
	return nil
}

// AssignMemberRole assigns (or reassigns) a user to a role in a project.
// On conflict, the existing role is updated. Note: created_at is not
// updated on reassignment; the table lacks an updated_at column.
func (q *Queries) AssignMemberRole(ctx context.Context, m *domain.ProjectMemberRole) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.AssignMemberRole")
	defer span.End()

	if m.ID == "" {
		m.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO project_member_roles (id, project_id, user_id, role_id, granted_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (project_id, user_id) DO UPDATE SET role_id = EXCLUDED.role_id, granted_by = EXCLUDED.granted_by
		RETURNING created_at`

	err := q.db.QueryRow(ctx, query, m.ID, m.ProjectID, m.UserID, m.RoleID, m.GrantedBy).Scan(&m.CreatedAt)
	if err != nil {
		return fmt.Errorf("assign member role: %w", err)
	}

	return nil
}

func (q *Queries) GetMemberRole(ctx context.Context, projectID, userID string) (*domain.ProjectMemberRole, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetMemberRole")
	defer span.End()

	query := `
		SELECT id, project_id, user_id, role_id, granted_by, created_at
		FROM project_member_roles
		WHERE project_id = $1 AND user_id = $2`

	var m domain.ProjectMemberRole
	var grantedBy *string
	err := q.db.QueryRow(ctx, query, projectID, userID).Scan(
		&m.ID, &m.ProjectID, &m.UserID, &m.RoleID, &grantedBy, &m.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get member role: %w", err)
	}
	if grantedBy != nil {
		m.GrantedBy = *grantedBy
	}

	return &m, nil
}

func (q *Queries) RemoveMemberRole(ctx context.Context, projectID, userID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.RemoveMemberRole")
	defer span.End()

	query := `DELETE FROM project_member_roles WHERE project_id = $1 AND user_id = $2`
	tag, err := q.db.Exec(ctx, query, projectID, userID)
	if err != nil {
		return fmt.Errorf("remove member role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMemberNotFound
	}
	return nil
}

func (q *Queries) ListProjectMembers(ctx context.Context, projectID string) ([]domain.ProjectMemberRole, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListProjectMembers")
	defer span.End()

	query := `
		SELECT id, project_id, user_id, role_id, granted_by, created_at
		FROM project_member_roles
		WHERE project_id = $1
		ORDER BY created_at ASC`

	rows, err := q.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project members: %w", err)
	}
	defer rows.Close()

	members := make([]domain.ProjectMemberRole, 0, 16)
	for rows.Next() {
		var m domain.ProjectMemberRole
		var grantedBy *string
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.UserID, &m.RoleID, &grantedBy, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("list project members scan: %w", err)
		}
		if grantedBy != nil {
			m.GrantedBy = *grantedBy
		}
		members = append(members, m)
	}

	return members, rows.Err()
}

// GetUserPermissions returns the role-based permissions for a user in a project.
// Returns nil if the user has no role assigned. Resource-level policies are
// queried separately via GetResourcePolicies.
func (q *Queries) GetUserPermissions(ctx context.Context, projectID, userID string) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetUserPermissions")
	defer span.End()

	query := `
		SELECT pr.permissions
		FROM project_member_roles pmr
		JOIN project_roles pr ON pr.id = pmr.role_id
		WHERE pmr.project_id = $1 AND pmr.user_id = $2`

	var permissions []string
	err := q.db.QueryRow(ctx, query, projectID, userID).Scan(&permissions)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get user permissions: %w", err)
	}

	return permissions, nil
}

func (q *Queries) CreateResourcePolicy(ctx context.Context, p *domain.ResourcePolicy) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateResourcePolicy")
	defer span.End()

	if p.ID == "" {
		p.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO resource_policies (id, project_id, resource_type, resource_id, user_id, actions)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (resource_type, resource_id, user_id) DO UPDATE SET actions = EXCLUDED.actions
		RETURNING created_at`

	err := q.db.QueryRow(ctx, query, p.ID, p.ProjectID, p.ResourceType, p.ResourceID, p.UserID, p.Actions).Scan(&p.CreatedAt)
	if err != nil {
		return fmt.Errorf("create resource policy: %w", err)
	}

	return nil
}

func (q *Queries) GetResourcePolicies(ctx context.Context, resourceType, resourceID, userID string) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetResourcePolicies")
	defer span.End()

	query := `
		SELECT actions
		FROM resource_policies
		WHERE resource_type = $1 AND resource_id = $2 AND user_id = $3`

	var actions []string
	err := q.db.QueryRow(ctx, query, resourceType, resourceID, userID).Scan(&actions)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get resource policies: %w", err)
	}

	return actions, nil
}

func (q *Queries) DeleteResourcePolicy(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteResourcePolicy")
	defer span.End()

	query := `DELETE FROM resource_policies WHERE id = $1`
	tag, err := q.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete resource policy: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrResourcePolicyNotFound
	}
	return nil
}

func (q *Queries) ListResourcePolicies(ctx context.Context, resourceType, resourceID string) ([]domain.ResourcePolicy, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListResourcePolicies")
	defer span.End()

	query := `
		SELECT id, project_id, resource_type, resource_id, user_id, actions, created_at
		FROM resource_policies
		WHERE resource_type = $1 AND resource_id = $2
		ORDER BY created_at ASC`

	rows, err := q.db.Query(ctx, query, resourceType, resourceID)
	if err != nil {
		return nil, fmt.Errorf("list resource policies: %w", err)
	}
	defer rows.Close()

	policies := make([]domain.ResourcePolicy, 0, 8)
	for rows.Next() {
		var p domain.ResourcePolicy
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.ResourceType, &p.ResourceID, &p.UserID, &p.Actions, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("list resource policies scan: %w", err)
		}
		policies = append(policies, p)
	}

	return policies, rows.Err()
}

func scanProjectRole(scanner scanTarget) (*domain.ProjectRole, error) {
	var role domain.ProjectRole
	var description *string

	err := scanner.Scan(
		&role.ID, &role.ProjectID, &role.Name, &description,
		&role.Permissions, &role.IsSystem, &role.CreatedAt, &role.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if description != nil {
		role.Description = *description
	}
	return &role, nil
}
