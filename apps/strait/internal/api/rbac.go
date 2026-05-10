package api

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// validateCallerCanGrantPermissions checks that the caller's effective
// permissions are a superset of the requested permissions. This prevents
// privilege escalation where a user with limited scopes creates a role
// with broader permissions (e.g. wildcard). Internal secret auth (scopes
// nil) bypasses this check since those requests are fully trusted.
func (s *Server) validateCallerCanGrantPermissions(ctx context.Context, requested []string) error {
	callerScopes := scopesFromContext(ctx)
	if callerScopes == nil {
		// Internal secret auth -- fully trusted, no restriction.
		return nil
	}

	// Determine the caller's effective permissions.
	// For OIDC users with empty token scopes, load from the database.
	// For API keys with empty scopes (legacy backwards compat = full access),
	// we still load from the scopes directly.
	effectivePerms := callerScopes
	actorType, _ := ctx.Value(ctxActorTypeKey).(string)

	if len(callerScopes) == 0 && actorType == "user" {
		// OIDC user with empty scopes: load effective permissions from DB.
		projectID := projectIDFromContext(ctx)
		actorID := actorFromContext(ctx)
		if projectID != "" && actorID != "" {
			perms, err := s.store.GetUserPermissions(ctx, projectID, actorID)
			if err != nil {
				slog.Warn("failed to load caller permissions for escalation check", "error", err)
				return huma.Error403Forbidden("unable to verify caller permissions")
			}
			effectivePerms = perms
		}
	} else if len(callerScopes) == 0 {
		// Legacy API key with empty scopes = full access (backwards compat).
		return nil
	}

	// Check if effective permissions include wildcard.
	if slices.Contains(effectivePerms, domain.ScopeAll) {
		return nil
	}

	// Build a set of the caller's effective permissions for fast lookup.
	permSet := make(map[string]bool, len(effectivePerms))
	for _, p := range effectivePerms {
		permSet[p] = true
	}

	for _, req := range requested {
		if req == domain.ScopeAll {
			return huma.Error403Forbidden("cannot grant wildcard permission: caller does not have wildcard scope")
		}
		if !permSet[req] {
			return huma.Error403Forbidden("cannot grant permission " + req + ": caller does not have it")
		}
	}
	return nil
}

// Roles.

type createRoleRequest struct {
	Name         string   `json:"name" validate:"required,max=255"`
	Description  string   `json:"description" validate:"max=2000"`
	Permissions  []string `json:"permissions" validate:"required,min=1"`
	ParentRoleID string   `json:"parent_role_id,omitempty"`
}

// errAuditDetailsMarshal flags a programmer/serialization failure that
// callers running inside a transaction must surface so the surrounding
// mutation rolls back instead of committing without an audit row. Plain
// "skip the event" decisions (config nil, validation rejected the
// actor, etc.) return (nil, nil) so fire-and-forget callers can keep
// going.
var errAuditDetailsMarshal = errors.New("audit event details marshal failed")

// buildAuditEvent runs the validation, actor resolution, and details
// marshaling steps that emitAuditEvent performs but stops short of
// persisting.
//
// Return shape:
//   - (event, nil)     — caller should persist the event.
//   - (nil,   nil)     — intentional skip (config disabled, unknown
//     audit action, validateActorForEmit declined).
//     Fire-and-forget callers ignore; tx callers
//     simply do not insert.
//   - (nil,   err)     — internal failure (currently only details
//     marshal). tx callers MUST abort so the
//     surrounding mutation does not commit without
//     an audit row. fire-and-forget callers log and
//     drop because there is nothing actionable.
//
// This is split out so callers that need atomic-with-transaction audit
// inserts can construct the event up front, pass it into the tx via
// txStore.CreateAuditEvent, and have the whole unit roll back together.
func (s *Server) buildAuditEvent(ctx context.Context, action, resourceType, resourceID string, details map[string]any) (*domain.AuditEvent, error) {
	if s.config == nil {
		return nil, nil
	}
	if !domain.IsKnownAuditAction(action) {
		slog.Error("emitAuditEvent: unknown action rejected",
			"action", action, "resource_type", resourceType, "resource_id", resourceID)
		return nil, nil
	}
	actorID, actorType, ok := s.validateActorForEmit(ctx, action)
	if !ok {
		return nil, nil
	}
	detailsJSON, err := s.marshalAndCapDetails(ctx, action, details)
	if err != nil {
		slog.Warn("failed to marshal audit event details", "action", action, "error", err)
		return nil, errAuditDetailsMarshal
	}
	return &domain.AuditEvent{
		ProjectID:     projectIDFromContext(ctx),
		ActorID:       actorID,
		ActorType:     actorType,
		Action:        action,
		ResourceType:  resourceType,
		ResourceID:    resourceID,
		Details:       detailsJSON,
		RemoteIP:      remoteIPFromContext(ctx),
		UserAgent:     userAgentFromContext(ctx),
		RequestID:     requestIDFromContext(ctx),
		TraceID:       traceIDFromContext(ctx),
		SchemaVersion: domain.AuditEventSchemaVersionCurrent,
	}, nil
}

func (s *Server) emitAuditEvent(ctx context.Context, action, resourceType, resourceID string, details map[string]any) {
	ev, err := s.buildAuditEvent(ctx, action, resourceType, resourceID, details)
	if err != nil {
		// Marshal failure is unrecoverable for a fire-and-forget caller;
		// count it as a drop with a distinct reason so dashboards can
		// distinguish marshal bugs from store-write outages.
		if s.metrics != nil && s.metrics.AuditEventsDropped != nil {
			s.metrics.AuditEventsDropped.Add(ctx, 1,
				metric.WithAttributes(attribute.String("reason", "details_marshal_failed")))
		}
		return
	}
	if ev == nil {
		return
	}
	if err := s.store.CreateAuditEvent(ctx, ev); err != nil {
		slog.Warn("failed to create audit event", "action", action, "resource_type", resourceType, "resource_id", resourceID, "error", err)
		if s.metrics != nil && s.metrics.AuditEventsDropped != nil {
			s.metrics.AuditEventsDropped.Add(ctx, 1,
				metric.WithAttributes(attribute.String("reason", "sync_write_failed")))
		}
	}
}

type CreateRoleInput struct{ Body createRoleRequest }
type CreateRoleOutput struct{ Body *domain.ProjectRole }

func (s *Server) handleCreateRole(ctx context.Context, input *CreateRoleInput) (*CreateRoleOutput, error) {
	if err := s.checkFeatureAllowed(ctx, projectIDFromContext(ctx), billing.FeatureRBAC, "Role management"); err != nil {
		return nil, err
	}

	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := domain.ValidateScopes(req.Permissions); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if err := s.validateCallerCanGrantPermissions(ctx, req.Permissions); err != nil {
		return nil, err
	}
	role := &domain.ProjectRole{ProjectID: projectIDFromContext(ctx), Name: req.Name, Description: req.Description, Permissions: req.Permissions, ParentRoleID: req.ParentRoleID}
	if err := s.store.CreateProjectRole(ctx, role); err != nil {
		return nil, huma.Error500InternalServerError("failed to create role")
	}
	s.emitAuditEvent(ctx, domain.AuditActionRoleCreated, "role", role.ID, map[string]any{"name": role.Name, "description": role.Description, "permissions": role.Permissions, "parent_role": role.ParentRoleID, "project_id": role.ProjectID, "is_system": role.IsSystem, "change_source": "rbac_api"})
	return &CreateRoleOutput{Body: role}, nil
}

type ListRolesInput struct {
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}
type ListRolesOutput struct{ Body PaginatedResponse }

func (s *Server) handleListRoles(ctx context.Context, input *ListRolesInput) (*ListRolesOutput, error) {
	projectID := projectIDFromContext(ctx)
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	roles, err := s.store.ListProjectRoles(ctx, projectID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list roles")
	}
	return &ListRolesOutput{Body: paginatedResult(roles, limit, func(role domain.ProjectRole) string { return role.CreatedAt.Format(time.RFC3339Nano) })}, nil
}

type GetRoleInput struct {
	RoleID         string `path:"roleID"`
	IncludeLineage string `query:"include_lineage"`
}
type GetRoleOutput struct{ Body any }

func (s *Server) handleGetRole(ctx context.Context, input *GetRoleInput) (*GetRoleOutput, error) {
	role, err := s.store.GetProjectRole(ctx, input.RoleID)
	if err != nil {
		if errors.Is(err, store.ErrRoleNotFound) {
			return nil, huma.Error404NotFound("role not found")
		}
		return nil, huma.Error500InternalServerError("failed to get role")
	}
	if err := requireProjectMatch(ctx, role.ProjectID); err != nil {
		return nil, huma.Error404NotFound("role not found")
	}
	if input.IncludeLineage != "true" {
		return &GetRoleOutput{Body: role}, nil
	}
	lineage := make([]domain.ProjectRole, 0, 4)
	currentParent := role.ParentRoleID
	for depth := 0; depth < 20 && currentParent != ""; depth++ {
		parent, parentErr := s.store.GetProjectRole(ctx, currentParent)
		if parentErr != nil {
			if errors.Is(parentErr, store.ErrRoleNotFound) {
				break
			}
			return nil, huma.Error500InternalServerError("failed to load role lineage")
		}
		// Stop the lineage walk at the first parent that does not belong to
		// the caller's project (system roles have empty ProjectID and are
		// always visible). Without this guard, a misconfigured role chain
		// could leak the existence and contents of cross-project roles.
		if parent.ProjectID != "" {
			if callerProjectID := projectIDFromContext(ctx); callerProjectID != "" && parent.ProjectID != callerProjectID {
				break
			}
		}
		lineage = append(lineage, *parent)
		currentParent = parent.ParentRoleID
	}
	return &GetRoleOutput{Body: map[string]any{"role": role, "lineage": lineage}}, nil
}

type updateRoleRequest struct {
	Name         string   `json:"name" validate:"required,max=255"`
	Description  string   `json:"description" validate:"max=2000"`
	Permissions  []string `json:"permissions" validate:"required,min=1"`
	ParentRoleID string   `json:"parent_role_id,omitempty"`
}
type UpdateRoleInput struct {
	RoleID string `path:"roleID"`
	Body   updateRoleRequest
}
type UpdateRoleOutput struct{ Body *domain.ProjectRole }

func (s *Server) handleUpdateRole(ctx context.Context, input *UpdateRoleInput) (*UpdateRoleOutput, error) {
	if err := s.checkFeatureAllowed(ctx, projectIDFromContext(ctx), billing.FeatureRBAC, "Role management"); err != nil {
		return nil, err
	}

	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := domain.ValidateScopes(req.Permissions); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if err := s.validateCallerCanGrantPermissions(ctx, req.Permissions); err != nil {
		return nil, err
	}
	roleID := input.RoleID
	previousRole, err := s.store.GetProjectRole(ctx, roleID)
	if err != nil {
		if errors.Is(err, store.ErrRoleNotFound) {
			return nil, huma.Error404NotFound("role not found")
		}
		return nil, huma.Error500InternalServerError("failed to get role")
	}
	if err := requireProjectMatch(ctx, previousRole.ProjectID); err != nil {
		return nil, huma.Error404NotFound("role not found")
	}
	role := &domain.ProjectRole{ID: roleID, Name: req.Name, Description: req.Description, Permissions: req.Permissions, ParentRoleID: req.ParentRoleID}
	if err := s.store.UpdateProjectRole(ctx, role); err != nil {
		if errors.Is(err, store.ErrRoleNotFound) {
			return nil, huma.Error404NotFound("role not found or is a system role")
		}
		return nil, huma.Error500InternalServerError("failed to update role")
	}
	updated, err := s.store.GetProjectRole(ctx, roleID)
	if err != nil {
		if errors.Is(err, store.ErrRoleNotFound) {
			return nil, huma.Error404NotFound("role not found")
		}
		return nil, huma.Error500InternalServerError("failed to load updated role")
	}
	if updated == nil {
		updated = role
	}
	s.emitAuditEvent(ctx, domain.AuditActionRoleUpdated, "role", roleID, map[string]any{"changes": map[string]any{"before": previousRole, "after": updated}})
	return &UpdateRoleOutput{Body: updated}, nil
}

type DeleteRoleInput struct {
	RoleID string `path:"roleID"`
}

func (s *Server) handleDeleteRole(ctx context.Context, input *DeleteRoleInput) (*struct{}, error) {
	if err := s.checkFeatureAllowed(ctx, projectIDFromContext(ctx), billing.FeatureRBAC, "Role management"); err != nil {
		return nil, err
	}

	role, err := s.store.GetProjectRole(ctx, input.RoleID)
	if err != nil {
		if errors.Is(err, store.ErrRoleNotFound) {
			return nil, huma.Error404NotFound("role not found or is a system role")
		}
		return nil, huma.Error500InternalServerError("failed to get role")
	}
	if err := requireProjectMatch(ctx, role.ProjectID); err != nil {
		return nil, huma.Error404NotFound("role not found or is a system role")
	}

	if err := s.store.DeleteProjectRole(ctx, input.RoleID); err != nil {
		if errors.Is(err, store.ErrRoleNotFound) {
			return nil, huma.Error404NotFound("role not found or is a system role")
		}
		return nil, huma.Error500InternalServerError("failed to delete role")
	}
	slog.Info("role deleted", "role_id", input.RoleID, "actor", actorFromContext(ctx), "project_id", projectIDFromContext(ctx))
	s.emitAuditEvent(ctx, domain.AuditActionRoleDeleted, "role", input.RoleID, nil)
	return nil, nil
}

// Members.
type assignMemberRequest struct {
	UserID string `json:"user_id" validate:"required"`
	RoleID string `json:"role_id" validate:"required"`
}
type bulkAssignMembersRequest struct {
	Items []assignMemberRequest `json:"items" validate:"required,min=1"`
}
type bulkAssignMemberResult struct {
	UserID string `json:"user_id"`
	RoleID string `json:"role_id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type AssignMemberInput struct{ Body assignMemberRequest }
type AssignMemberOutput struct{ Body *domain.ProjectMemberRole }

func (s *Server) handleAssignMember(ctx context.Context, input *AssignMemberInput) (*AssignMemberOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if s.billingEnforcer != nil {
		projectID := projectIDFromContext(ctx)
		orgID, err := s.billingEnforcer.GetActiveProjectOrgID(ctx, projectID)
		if err == nil && orgID != "" {
			if err := s.billingEnforcer.CheckMemberLimit(ctx, orgID); err != nil {
				var le *billing.LimitError
				if errors.As(err, &le) {
					return nil, le
				}
			}
		}
	}
	// Prevent self-assignment: callers cannot assign roles to themselves.
	caller := actorFromContext(ctx)
	if caller != "" && caller == req.UserID {
		return nil, huma.Error403Forbidden("cannot assign a role to yourself")
	}
	targetRole, err := s.store.GetProjectRole(ctx, req.RoleID)
	if err != nil {
		if errors.Is(err, store.ErrRoleNotFound) {
			return nil, huma.Error400BadRequest("role not found")
		}
		return nil, huma.Error500InternalServerError("failed to verify role")
	}
	if targetRole.ProjectID != "" && targetRole.ProjectID != projectIDFromContext(ctx) {
		return nil, huma.Error400BadRequest("role not found")
	}
	if err := s.validateCallerCanGrantPermissions(ctx, targetRole.Permissions); err != nil {
		return nil, err
	}
	m := &domain.ProjectMemberRole{ProjectID: projectIDFromContext(ctx), UserID: req.UserID, RoleID: req.RoleID, GrantedBy: caller}
	if err := s.store.AssignMemberRole(ctx, m); err != nil {
		return nil, huma.Error500InternalServerError("failed to assign role")
	}
	s.permCache.Invalidate(m.ProjectID, m.UserID)
	s.emitAuditEvent(ctx, domain.AuditActionPermissionGranted, "role", m.RoleID, map[string]any{"user_id": m.UserID, "project_id": m.ProjectID})
	return &AssignMemberOutput{Body: m}, nil
}

type BulkAssignMembersInput struct{ Body bulkAssignMembersRequest }
type BulkAssignMembersOutput struct{ Body any }

func (s *Server) handleBulkAssignMembers(ctx context.Context, input *BulkAssignMembersInput) (*BulkAssignMembersOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	projectID := projectIDFromContext(ctx)
	actor := actorFromContext(ctx)
	results := make([]bulkAssignMemberResult, 0, len(req.Items))
	for _, item := range req.Items {
		if item.UserID == "" || item.RoleID == "" {
			results = append(results, bulkAssignMemberResult{UserID: item.UserID, RoleID: item.RoleID, Status: "error", Error: "user_id and role_id are required"})
			continue
		}
		if actor != "" && actor == item.UserID {
			results = append(results, bulkAssignMemberResult{UserID: item.UserID, RoleID: item.RoleID, Status: "error", Error: "cannot assign a role to yourself"})
			continue
		}
		targetRole, err := s.store.GetProjectRole(ctx, item.RoleID)
		if err != nil {
			if errors.Is(err, store.ErrRoleNotFound) {
				results = append(results, bulkAssignMemberResult{UserID: item.UserID, RoleID: item.RoleID, Status: "error", Error: "role not found"})
				continue
			}
			return nil, huma.Error500InternalServerError("failed to verify role")
		}
		if targetRole.ProjectID != "" && targetRole.ProjectID != projectID {
			results = append(results, bulkAssignMemberResult{UserID: item.UserID, RoleID: item.RoleID, Status: "error", Error: "role not found"})
			continue
		}
		if err := s.validateCallerCanGrantPermissions(ctx, targetRole.Permissions); err != nil {
			results = append(results, bulkAssignMemberResult{UserID: item.UserID, RoleID: item.RoleID, Status: "error", Error: err.Error()})
			continue
		}
		m := &domain.ProjectMemberRole{ProjectID: projectID, UserID: item.UserID, RoleID: item.RoleID, GrantedBy: actor}
		if err := s.store.AssignMemberRole(ctx, m); err != nil {
			results = append(results, bulkAssignMemberResult{UserID: item.UserID, RoleID: item.RoleID, Status: "error", Error: "failed to assign role"})
			continue
		}
		s.permCache.Invalidate(projectID, item.UserID)
		s.emitAuditEvent(ctx, domain.AuditActionPermissionGranted, "role", item.RoleID, map[string]any{"user_id": item.UserID, "project_id": projectID, "bulk": true})
		results = append(results, bulkAssignMemberResult{UserID: item.UserID, RoleID: item.RoleID, Status: "assigned"})
	}
	return &BulkAssignMembersOutput{Body: map[string]any{"results": results, "total": len(results)}}, nil
}

type ListMembersInput struct {
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}
type ListMembersOutput struct{ Body PaginatedResponse }

func (s *Server) handleListMembers(ctx context.Context, input *ListMembersInput) (*ListMembersOutput, error) {
	projectID := projectIDFromContext(ctx)
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	members, err := s.store.ListProjectMembers(ctx, projectID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list members")
	}
	return &ListMembersOutput{Body: paginatedResult(members, limit, func(m domain.ProjectMemberRole) string { return m.CreatedAt.Format(time.RFC3339Nano) })}, nil
}

type RemoveMemberInput struct {
	UserID string `path:"userID"`
}

func (s *Server) handleRemoveMember(ctx context.Context, input *RemoveMemberInput) (*struct{}, error) {
	userID := input.UserID
	projectID := projectIDFromContext(ctx)
	memberRole, _ := s.store.GetMemberRole(ctx, projectID, userID)
	if err := s.store.RemoveMemberRole(ctx, projectID, userID); err != nil {
		if errors.Is(err, store.ErrMemberNotFound) {
			return nil, huma.Error404NotFound("member not found")
		}
		return nil, huma.Error500InternalServerError("failed to remove member")
	}
	s.permCache.Invalidate(projectID, userID)
	slog.Info("member removed", "user_id", userID, "actor", actorFromContext(ctx), "project_id", projectID)
	resourceID := userID
	roleID := ""
	if memberRole != nil && memberRole.RoleID != "" {
		roleID = memberRole.RoleID
		resourceID = memberRole.RoleID
	}
	s.emitAuditEvent(ctx, domain.AuditActionPermissionRevoked, "role", resourceID, map[string]any{"user_id": userID, "project_id": projectID, "role_id": roleID})
	return nil, nil
}

// System Roles.
type SeedSystemRolesInput struct{}
type SeedSystemRolesOutput struct{ Body []domain.ProjectRole }

func (s *Server) handleSeedSystemRoles(ctx context.Context, _ *SeedSystemRolesInput) (*SeedSystemRolesOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if err := s.store.SeedProjectSystemRoles(ctx, projectID); err != nil {
		return nil, huma.Error500InternalServerError("failed to seed system roles")
	}
	roles, err := s.store.ListProjectRoles(ctx, projectID, 100, nil)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list roles after seeding")
	}
	roleNames := make([]string, 0, len(roles))
	for _, r := range roles {
		if r.IsSystem {
			roleNames = append(roleNames, r.Name)
		}
	}
	s.emitAuditEvent(ctx, domain.AuditActionRoleSystemSeeded, "role", projectID, map[string]any{
		"project_id":     projectID,
		"system_roles":   roleNames,
		"roles_returned": len(roles),
	})
	return &SeedSystemRolesOutput{Body: roles}, nil
}

// Resource Policies.
type createResourcePolicyRequest struct {
	ProjectID    string   `json:"project_id" validate:"required"`
	ResourceType string   `json:"resource_type" validate:"required"`
	ResourceID   string   `json:"resource_id" validate:"required"`
	UserID       string   `json:"user_id" validate:"required"`
	Actions      []string `json:"actions" validate:"required,min=1"`
}
type CreateResourcePolicyInput struct{ Body createResourcePolicyRequest }
type CreateResourcePolicyOutput struct{ Body *domain.ResourcePolicy }

func (s *Server) handleCreateResourcePolicy(ctx context.Context, input *CreateResourcePolicyInput) (*CreateResourcePolicyOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}
	for _, action := range req.Actions {
		if !domain.ValidScopes[action] {
			return nil, huma.Error400BadRequest("invalid action: " + action)
		}
	}
	if err := s.validateCallerCanGrantPermissions(ctx, req.Actions); err != nil {
		return nil, err
	}
	policy := &domain.ResourcePolicy{ProjectID: req.ProjectID, ResourceType: req.ResourceType, ResourceID: req.ResourceID, UserID: req.UserID, Actions: req.Actions}
	if err := s.store.CreateResourcePolicy(ctx, policy); err != nil {
		return nil, huma.Error500InternalServerError("failed to create resource policy")
	}
	s.permCache.Invalidate(req.ProjectID, req.UserID)
	s.emitAuditEvent(ctx, domain.AuditActionResourcePolicyCreated, "resource_policy", policy.ID, map[string]any{"resource_type": req.ResourceType, "resource_id": req.ResourceID, "user_id": req.UserID, "actions": req.Actions})
	return &CreateResourcePolicyOutput{Body: policy}, nil
}

type ListResourcePoliciesInput struct {
	ResourceType string `query:"resource_type"`
	ResourceID   string `query:"resource_id"`
	Limit        string `query:"limit"`
	Cursor       string `query:"cursor"`
}
type ListResourcePoliciesOutput struct{ Body PaginatedResponse }

func (s *Server) handleListResourcePolicies(ctx context.Context, input *ListResourcePoliciesInput) (*ListResourcePoliciesOutput, error) {
	if input.ResourceType == "" || input.ResourceID == "" {
		return nil, huma.Error400BadRequest("resource_type and resource_id are required")
	}
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	policies, err := s.store.ListResourcePolicies(ctx, projectIDFromContext(ctx), input.ResourceType, input.ResourceID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list resource policies")
	}
	return &ListResourcePoliciesOutput{Body: paginatedResult(policies, limit, func(p domain.ResourcePolicy) string { return p.CreatedAt.Format(time.RFC3339Nano) })}, nil
}

type DeleteResourcePolicyInput struct {
	PolicyID string `path:"policyID"`
}

func (s *Server) handleDeleteResourcePolicy(ctx context.Context, input *DeleteResourcePolicyInput) (*struct{}, error) {
	projectID, userID, err := s.store.DeleteResourcePolicy(ctx, projectIDFromContext(ctx), input.PolicyID)
	if err != nil {
		if errors.Is(err, store.ErrResourcePolicyNotFound) {
			return nil, huma.Error404NotFound("resource policy not found")
		}
		return nil, huma.Error500InternalServerError("failed to delete resource policy")
	}
	if projectID != "" && userID != "" {
		s.permCache.Invalidate(projectID, userID)
	}
	slog.Info("resource policy deleted", "policy_id", input.PolicyID, "actor", actorFromContext(ctx), "affected_user", userID, "project_id", projectID)
	s.emitAuditEvent(ctx, domain.AuditActionResourcePolicyDeleted, "resource_policy", input.PolicyID, map[string]any{"affected_user": userID})
	return nil, nil
}

type createTagPolicyRequest struct {
	ProjectID    string   `json:"project_id" validate:"required"`
	ResourceType string   `json:"resource_type" validate:"required"`
	UserID       string   `json:"user_id" validate:"required"`
	TagKey       string   `json:"tag_key" validate:"required"`
	TagValue     string   `json:"tag_value,omitempty"`
	Actions      []string `json:"actions" validate:"required,min=1"`
}
type CreateTagPolicyInput struct{ Body createTagPolicyRequest }
type CreateTagPolicyOutput struct{ Body *domain.TagPolicy }

func (s *Server) handleCreateTagPolicy(ctx context.Context, input *CreateTagPolicyInput) (*CreateTagPolicyOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}
	if err := validateTags(map[string]string{req.TagKey: req.TagValue}); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	for _, action := range req.Actions {
		if !domain.ValidScopes[action] {
			return nil, huma.Error400BadRequest("invalid action: " + action)
		}
	}
	if err := s.validateCallerCanGrantPermissions(ctx, req.Actions); err != nil {
		return nil, err
	}
	policy := &domain.TagPolicy{ProjectID: req.ProjectID, ResourceType: req.ResourceType, UserID: req.UserID, TagKey: req.TagKey, TagValue: req.TagValue, Actions: req.Actions}
	if err := s.store.CreateTagPolicy(ctx, policy); err != nil {
		return nil, huma.Error500InternalServerError("failed to create tag policy")
	}
	s.permCache.Invalidate(req.ProjectID, req.UserID)
	s.emitAuditEvent(ctx, domain.AuditActionTagPolicyCreated, "tag_policy", policy.ID, map[string]any{"tag_key": req.TagKey, "tag_value": req.TagValue, "resource_type": req.ResourceType, "user_id": req.UserID, "actions": req.Actions})
	return &CreateTagPolicyOutput{Body: policy}, nil
}

type ListTagPoliciesInput struct {
	ResourceType string `query:"resource_type"`
	UserID       string `query:"user_id"`
	Limit        string `query:"limit"`
	Cursor       string `query:"cursor"`
}
type ListTagPoliciesOutput struct{ Body PaginatedResponse }

func (s *Server) handleListTagPolicies(ctx context.Context, input *ListTagPoliciesInput) (*ListTagPoliciesOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	policies, err := s.store.ListTagPolicies(ctx, projectID, input.ResourceType, input.UserID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list tag policies")
	}
	return &ListTagPoliciesOutput{Body: paginatedResult(policies, limit, func(p domain.TagPolicy) string { return p.CreatedAt.Format(time.RFC3339Nano) })}, nil
}

type DeleteTagPolicyInput struct {
	PolicyID string `path:"policyID"`
}

func (s *Server) handleDeleteTagPolicy(ctx context.Context, input *DeleteTagPolicyInput) (*struct{}, error) {
	projectID, userID, err := s.store.DeleteTagPolicy(ctx, projectIDFromContext(ctx), input.PolicyID)
	if err != nil {
		if errors.Is(err, store.ErrTagPolicyNotFound) {
			return nil, huma.Error404NotFound("tag policy not found")
		}
		return nil, huma.Error500InternalServerError("failed to delete tag policy")
	}
	if projectID != "" && userID != "" {
		s.permCache.Invalidate(projectID, userID)
	}
	s.emitAuditEvent(ctx, domain.AuditActionTagPolicyDeleted, "tag_policy", input.PolicyID, nil)
	return nil, nil
}

type ListAuditEventsInput struct {
	ActorID      string `query:"actor_id"`
	ResourceType string `query:"resource_type"`
	ResourceID   string `query:"resource_id"`
	Order        string `query:"order"`
	From         string `query:"from"`
	To           string `query:"to"`
	Limit        string `query:"limit"`
	Cursor       string `query:"cursor"`
}
type ListAuditEventsOutput struct{ Body PaginatedResponse }

func (s *Server) handleListAuditEvents(ctx context.Context, input *ListAuditEventsInput) (*ListAuditEventsOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if err := requireProjectWideAuditAccess(ctx); err != nil {
		return nil, err
	}

	if err := s.checkFeatureAllowed(ctx, projectID, billing.FeatureAuditLogs, "Audit logs"); err != nil {
		return nil, err
	}

	ascending := input.Order == "asc"
	if input.Order != "" && input.Order != "asc" && input.Order != "desc" {
		return nil, huma.Error400BadRequest("order must be one of: asc, desc")
	}
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	var from, to *time.Time
	if input.From != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, input.From)
		if parseErr != nil {
			return nil, huma.Error400BadRequest("from must be a valid RFC3339 timestamp")
		}
		from = &parsed
	}
	if input.To != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, input.To)
		if parseErr != nil {
			return nil, huma.Error400BadRequest("to must be a valid RFC3339 timestamp")
		}
		to = &parsed
	}
	const maxListWindow = 90 * 24 * time.Hour
	now := time.Now().UTC()
	if to == nil {
		to = &now
	}
	if from == nil {
		defaultFrom := to.Add(-maxListWindow)
		from = &defaultFrom
	}
	if from.After(*to) {
		return nil, huma.Error400BadRequest("from must be <= to")
	}
	if to.Sub(*from) > maxListWindow {
		return nil, huma.Error400BadRequest("time window must not exceed 90 days")
	}
	events, err := s.store.ListAuditEvents(ctx, projectID, input.ActorID, input.ResourceType, input.ResourceID, limit+1, cursor, from, to, ascending)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list audit events")
	}
	s.emitAuditEvent(ctx, domain.AuditActionAuditListRead, "audit", projectID, map[string]any{
		"count":        len(events),
		"filter_actor": input.ActorID,
		"filter_rtype": input.ResourceType,
		"filter_rid":   input.ResourceID,
	})
	return &ListAuditEventsOutput{Body: paginatedResult(events, limit, func(ev domain.AuditEvent) string { return ev.CreatedAt.Format(time.RFC3339Nano) })}, nil
}

// VerifyAuditChainInput selects between a full scan from the chain head
// and an incremental re-check from the last stored checkpoint. The
// default (incremental=false) preserves the existing endpoint semantics.
type VerifyAuditChainInput struct {
	Incremental bool `query:"incremental"`
}

type VerifyAuditChainOutput struct {
	Body *domain.AuditChainVerification
}

func (s *Server) handleVerifyAuditChain(ctx context.Context, input *VerifyAuditChainInput) (*VerifyAuditChainOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if err := requireProjectWideAuditAccess(ctx); err != nil {
		return nil, err
	}

	if err := s.checkFeatureAllowed(ctx, projectID, billing.FeatureAuditLogs, "Audit logs"); err != nil {
		return nil, err
	}

	var (
		result *domain.AuditChainVerification
		err    error
	)
	if input != nil && input.Incremental {
		result, err = s.store.VerifyAuditChainIncremental(ctx, projectID)
	} else {
		result, err = s.store.VerifyAuditChain(ctx, projectID)
	}
	if s.metrics != nil && s.metrics.AuditChainVerifyTotal != nil {
		s.metrics.AuditChainVerifyTotal.Add(ctx, 1)
	}
	if err != nil {
		slog.Error("failed to verify audit chain", "project_id", projectID, "error", err)
		if s.metrics != nil && s.metrics.AuditChainVerifyFailed != nil {
			s.metrics.AuditChainVerifyFailed.Add(ctx, 1,
				metric.WithAttributes(attribute.String("reason", "verifier_error")))
		}
		return nil, huma.Error500InternalServerError("failed to verify audit chain")
	}
	if !result.Valid && s.metrics != nil && s.metrics.AuditChainVerifyFailed != nil {
		s.metrics.AuditChainVerifyFailed.Add(ctx, 1,
			metric.WithAttributes(attribute.String("reason", "chain_broken")))
	}

	s.emitAuditEvent(ctx, domain.AuditActionAuditChainVerified, "audit", projectID, map[string]any{
		"valid":          result.Valid,
		"events_checked": result.EventsChecked,
		"incremental":    result.Incremental,
	})

	return &VerifyAuditChainOutput{Body: result}, nil
}
