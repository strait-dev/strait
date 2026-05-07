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

var (
	ErrRoleNotFound           = errors.New("role not found")
	ErrMemberNotFound         = errors.New("member not found")
	ErrResourcePolicyNotFound = errors.New("resource policy not found")
	ErrTagPolicyNotFound      = errors.New("tag policy not found")
)

func (q *Queries) CreateProjectRole(ctx context.Context, role *domain.ProjectRole) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateProjectRole")
	defer span.End()

	if role.ID == "" {
		role.ID = uuid.Must(uuid.NewV7()).String()
	}

	query := `
		INSERT INTO project_roles (id, project_id, name, description, permissions, parent_role_id, is_system)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at`

	err := q.db.QueryRow(ctx, query,
		role.ID, role.ProjectID, role.Name, role.Description, role.Permissions, dbscan.NilIfEmptyString(role.ParentRoleID), role.IsSystem,
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
		SELECT id, project_id, name, description, permissions, parent_role_id, is_system, created_at, updated_at
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

func (q *Queries) ListProjectRoles(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.ProjectRole, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListProjectRoles")
	defer span.End()

	query := `
		SELECT id, project_id, name, description, permissions, parent_role_id, is_system, created_at, updated_at
		FROM project_roles
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

func (q *Queries) UpdateProjectRole(ctx context.Context, role *domain.ProjectRole) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UpdateProjectRole")
	defer span.End()

	if role.ParentRoleID == role.ID {
		return fmt.Errorf("parent role cannot reference itself")
	}
	if role.ParentRoleID != "" {
		cycle, err := q.wouldCreateRoleCycle(ctx, role.ID, role.ParentRoleID)
		if err != nil {
			return err
		}
		if cycle {
			return fmt.Errorf("parent role would create a cycle")
		}
	}

	query := `
		UPDATE project_roles
		SET name = $1, description = $2, permissions = $3, parent_role_id = $4, updated_at = NOW()
		WHERE id = $5 AND is_system = FALSE
		RETURNING updated_at`

	err := q.db.QueryRow(ctx, query,
		role.Name, role.Description, role.Permissions, dbscan.NilIfEmptyString(role.ParentRoleID), role.ID,
	).Scan(&role.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRoleNotFound
		}
		return fmt.Errorf("update project role: %w", err)
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

func (q *Queries) ListProjectMembers(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.ProjectMemberRole, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListProjectMembers")
	defer span.End()

	query := `
		SELECT id, project_id, user_id, role_id, granted_by, created_at
		FROM project_member_roles
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

// UserHasProjectAccess checks whether a user has any role in the given project.
func (q *Queries) UserHasProjectAccess(ctx context.Context, userID, projectID string) (bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.UserHasProjectAccess")
	defer span.End()

	var exists bool
	err := q.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM project_member_roles
			WHERE user_id = $1 AND project_id = $2
		)`, userID, projectID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check user project access: %w", err)
	}
	return exists, nil
}

// GetUserPermissions returns the role-based permissions for a user in a project.
// Returns nil if the user has no role assigned. Resource-level policies are
// queried separately via GetResourcePolicies.
func (q *Queries) GetUserPermissions(ctx context.Context, projectID, userID string) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetUserPermissions")
	defer span.End()

	query := `
		WITH RECURSIVE role_tree AS (
			SELECT pr.id, pr.parent_role_id, pr.permissions
			FROM project_member_roles pmr
			JOIN project_roles pr ON pr.id = pmr.role_id
			WHERE pmr.project_id = $1 AND pmr.user_id = $2
			UNION ALL
			SELECT parent.id, parent.parent_role_id, parent.permissions
			FROM project_roles parent
			JOIN role_tree rt ON rt.parent_role_id = parent.id
		)
		SELECT permissions FROM role_tree`

	rows, err := q.db.Query(ctx, query, projectID, userID)
	if err != nil {
		return nil, fmt.Errorf("get user permissions: %w", err)
	}
	defer rows.Close()

	seen := make(map[string]struct{}, 16)
	permissions := make([]string, 0, 16)
	for rows.Next() {
		var perms []string
		if err := rows.Scan(&perms); err != nil {
			return nil, fmt.Errorf("get user permissions scan: %w", err)
		}
		for _, p := range perms {
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			permissions = append(permissions, p)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get user permissions rows: %w", err)
	}
	if len(permissions) == 0 {
		return nil, nil
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
			ON CONFLICT (project_id, resource_type, resource_id, user_id) DO UPDATE SET actions = EXCLUDED.actions
		RETURNING created_at`

	err := q.db.QueryRow(ctx, query, p.ID, p.ProjectID, p.ResourceType, p.ResourceID, p.UserID, p.Actions).Scan(&p.CreatedAt)
	if err != nil {
		return fmt.Errorf("create resource policy: %w", err)
	}

	return nil
}

func (q *Queries) GetResourcePolicies(ctx context.Context, projectID, resourceType, resourceID, userID string) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetResourcePolicies")
	defer span.End()

	query := `
		SELECT actions
		FROM resource_policies
		WHERE project_id = $1 AND resource_type = $2 AND resource_id = $3 AND user_id = $4`

	var actions []string
	err := q.db.QueryRow(ctx, query, projectID, resourceType, resourceID, userID).Scan(&actions)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get resource policies: %w", err)
	}

	return actions, nil
}

func (q *Queries) DeleteResourcePolicy(ctx context.Context, projectID, id string) (deletedProjectID, userID string, err error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteResourcePolicy")
	defer span.End()

	query := `DELETE FROM resource_policies WHERE project_id = $1 AND id = $2 RETURNING project_id, user_id`
	err = q.db.QueryRow(ctx, query, projectID, id).Scan(&deletedProjectID, &userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrResourcePolicyNotFound
		}
		return "", "", fmt.Errorf("delete resource policy: %w", err)
	}
	return deletedProjectID, userID, nil
}

func (q *Queries) ListResourcePolicies(ctx context.Context, projectID, resourceType, resourceID string, limit int, cursor *time.Time) ([]domain.ResourcePolicy, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListResourcePolicies")
	defer span.End()

	query := `
		SELECT id, project_id, resource_type, resource_id, user_id, actions, created_at
		FROM resource_policies
		WHERE project_id = $1 AND resource_type = $2 AND resource_id = $3`
	args := []any{projectID, resourceType, resourceID}
	param := 4

	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list resource policies: %w", err)
	}
	defer rows.Close()

	policies := make([]domain.ResourcePolicy, 0, limit)
	for rows.Next() {
		var p domain.ResourcePolicy
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.ResourceType, &p.ResourceID, &p.UserID, &p.Actions, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("list resource policies scan: %w", err)
		}
		policies = append(policies, p)
	}

	return policies, rows.Err()
}

func (q *Queries) CreateTagPolicy(ctx context.Context, p *domain.TagPolicy) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CreateTagPolicy")
	defer span.End()

	if p.ID == "" {
		p.ID = uuid.Must(uuid.NewV7()).String()
	}
	query := `
		INSERT INTO tag_policies (id, project_id, resource_type, user_id, tag_key, tag_value, actions)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (project_id, resource_type, user_id, tag_key, tag_value)
		DO UPDATE SET actions = EXCLUDED.actions
		RETURNING created_at`
	if err := q.db.QueryRow(ctx, query, p.ID, p.ProjectID, p.ResourceType, p.UserID, p.TagKey, dbscan.NilIfEmptyString(p.TagValue), p.Actions).Scan(&p.CreatedAt); err != nil {
		return fmt.Errorf("create tag policy: %w", err)
	}
	return nil
}

func (q *Queries) ListTagPolicies(ctx context.Context, projectID, resourceType, userID string, limit int, cursor *time.Time) ([]domain.TagPolicy, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListTagPolicies")
	defer span.End()

	query := `
		SELECT id, project_id, resource_type, user_id, tag_key, tag_value, actions, created_at
		FROM tag_policies
		WHERE project_id = $1`
	args := []any{projectID}
	param := 2
	if resourceType != "" {
		query += fmt.Sprintf(" AND resource_type = $%d", param)
		args = append(args, resourceType)
		param++
	}
	if userID != "" {
		query += fmt.Sprintf(" AND user_id = $%d", param)
		args = append(args, userID)
		param++
	}
	if cursor != nil {
		query += fmt.Sprintf(" AND created_at < $%d", param)
		args = append(args, *cursor)
		param++
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", param)
	args = append(args, limit)

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tag policies: %w", err)
	}
	defer rows.Close()

	policies := make([]domain.TagPolicy, 0, limit)
	for rows.Next() {
		var p domain.TagPolicy
		var tagValue *string
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.ResourceType, &p.UserID, &p.TagKey, &tagValue, &p.Actions, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("list tag policies scan: %w", err)
		}
		if tagValue != nil {
			p.TagValue = *tagValue
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (q *Queries) DeleteTagPolicy(ctx context.Context, projectID, id string) (deletedProjectID, userID string, err error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteTagPolicy")
	defer span.End()

	query := `DELETE FROM tag_policies WHERE project_id = $1 AND id = $2 RETURNING project_id, user_id`
	err = q.db.QueryRow(ctx, query, projectID, id).Scan(&deletedProjectID, &userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrTagPolicyNotFound
		}
		return "", "", fmt.Errorf("delete tag policy: %w", err)
	}
	return deletedProjectID, userID, nil
}

func (q *Queries) GetTagPolicyActions(ctx context.Context, projectID, resourceType, userID string, tags map[string]string) ([]string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetTagPolicyActions")
	defer span.End()

	query := `
		SELECT tag_key, tag_value, actions
		FROM tag_policies
		WHERE project_id = $1 AND resource_type = $2 AND user_id = $3`
	rows, err := q.db.Query(ctx, query, projectID, resourceType, userID)
	if err != nil {
		return nil, fmt.Errorf("get tag policy actions: %w", err)
	}
	defer rows.Close()

	seen := make(map[string]struct{}, 8)
	actions := make([]string, 0, 8)
	for rows.Next() {
		var key string
		var val *string
		var policyActions []string
		if err := rows.Scan(&key, &val, &policyActions); err != nil {
			return nil, fmt.Errorf("scan tag policy actions: %w", err)
		}
		tagVal, ok := tags[key]
		if !ok {
			continue
		}
		if val != nil && *val != "" && tagVal != *val {
			continue
		}
		for _, action := range policyActions {
			if _, exists := seen[action]; exists {
				continue
			}
			seen[action] = struct{}{}
			actions = append(actions, action)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get tag policy actions rows: %w", err)
	}
	if len(actions) == 0 {
		return nil, nil
	}
	return actions, nil
}

func (q *Queries) wouldCreateRoleCycle(ctx context.Context, roleID, parentRoleID string) (bool, error) {
	query := `
		WITH RECURSIVE ancestors AS (
			SELECT id, parent_role_id
			FROM project_roles
			WHERE id = $1
			UNION ALL
			SELECT pr.id, pr.parent_role_id
			FROM project_roles pr
			JOIN ancestors a ON a.parent_role_id = pr.id
		)
		SELECT EXISTS(SELECT 1 FROM ancestors WHERE id = $2)`
	var cycle bool
	if err := q.db.QueryRow(ctx, query, parentRoleID, roleID).Scan(&cycle); err != nil {
		return false, fmt.Errorf("check role cycle: %w", err)
	}
	return cycle, nil
}

func scanProjectRole(scanner scanTarget) (*domain.ProjectRole, error) {
	var role domain.ProjectRole
	var description *string
	var parentRoleID *string

	err := scanner.Scan(
		&role.ID, &role.ProjectID, &role.Name, &description,
		&role.Permissions, &parentRoleID, &role.IsSystem, &role.CreatedAt, &role.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if description != nil {
		role.Description = *description
	}
	if parentRoleID != nil {
		role.ParentRoleID = *parentRoleID
	}
	return &role, nil
}

// SeedProjectSystemRoles idempotently creates the 4 system roles for a project.
func (q *Queries) SeedProjectSystemRoles(ctx context.Context, projectID string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.SeedProjectSystemRoles")
	defer span.End()

	for name, perms := range domain.SystemRolePermissions {
		role := &domain.ProjectRole{
			ID:          uuid.Must(uuid.NewV7()).String(),
			ProjectID:   projectID,
			Name:        name,
			Description: "System role: " + name,
			Permissions: perms,
			IsSystem:    true,
		}
		query := `
			INSERT INTO project_roles (id, project_id, name, description, permissions, is_system)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (project_id, name) DO NOTHING`
		if _, err := q.db.Exec(ctx, query, role.ID, role.ProjectID, role.Name, role.Description, role.Permissions, role.IsSystem); err != nil {
			return fmt.Errorf("seed system role %s: %w", name, err)
		}
	}

	return nil
}
