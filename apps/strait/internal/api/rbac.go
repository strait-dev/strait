package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

// Roles.

type createRoleRequest struct {
	Name         string   `json:"name" validate:"required,max=255"`
	Description  string   `json:"description" validate:"max=2000"`
	Permissions  []string `json:"permissions" validate:"required,min=1"`
	ParentRoleID string   `json:"parent_role_id,omitempty"`
}

func (s *Server) emitAuditEvent(ctx context.Context, action, resourceType, resourceID string, details map[string]any) {
	if s.config == nil {
		return
	}
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		slog.Warn("failed to marshal audit event details", "action", action, "error", err)
		return
	}
	actorType, _ := ctx.Value(ctxActorTypeKey).(string)
	ev := &domain.AuditEvent{
		ProjectID: projectIDFromContext(ctx), ActorID: actorFromContext(ctx), ActorType: actorType,
		Action: action, ResourceType: resourceType, ResourceID: resourceID, Details: detailsJSON,
	}
	if err := s.store.CreateAuditEvent(ctx, ev); err != nil {
		slog.Warn("failed to create audit event", "action", action, "resource_type", resourceType, "resource_id", resourceID, "error", err)
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
	role := &domain.ProjectRole{ProjectID: projectIDFromContext(ctx), Name: req.Name, Description: req.Description, Permissions: req.Permissions, ParentRoleID: req.ParentRoleID}
	if err := s.store.CreateProjectRole(ctx, role); err != nil {
		return nil, huma.Error500InternalServerError("failed to create role")
	}
	s.emitAuditEvent(ctx, "role.created", "role", role.ID, map[string]any{"name": role.Name, "description": role.Description, "permissions": role.Permissions, "parent_role": role.ParentRoleID, "project_id": role.ProjectID, "is_system": role.IsSystem, "change_source": "rbac_api"})
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
	roleID := input.RoleID
	previousRole, _ := s.store.GetProjectRole(ctx, roleID)
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
	s.emitAuditEvent(ctx, "role.updated", "role", roleID, map[string]any{"changes": map[string]any{"before": previousRole, "after": updated}})
	return &UpdateRoleOutput{Body: updated}, nil
}

type DeleteRoleInput struct {
	RoleID string `path:"roleID"`
}

func (s *Server) handleDeleteRole(ctx context.Context, input *DeleteRoleInput) (*struct{}, error) {
	if err := s.checkFeatureAllowed(ctx, projectIDFromContext(ctx), billing.FeatureRBAC, "Role management"); err != nil {
		return nil, err
	}

	if err := s.store.DeleteProjectRole(ctx, input.RoleID); err != nil {
		if errors.Is(err, store.ErrRoleNotFound) {
			return nil, huma.Error404NotFound("role not found or is a system role")
		}
		return nil, huma.Error500InternalServerError("failed to delete role")
	}
	slog.Info("role deleted", "role_id", input.RoleID, "actor", actorFromContext(ctx), "project_id", projectIDFromContext(ctx))
	s.emitAuditEvent(ctx, "role.deleted", "role", input.RoleID, nil)
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
	if _, err := s.store.GetProjectRole(ctx, req.RoleID); err != nil {
		if errors.Is(err, store.ErrRoleNotFound) {
			return nil, huma.Error400BadRequest("role not found")
		}
		return nil, huma.Error500InternalServerError("failed to verify role")
	}
	m := &domain.ProjectMemberRole{ProjectID: projectIDFromContext(ctx), UserID: req.UserID, RoleID: req.RoleID, GrantedBy: actorFromContext(ctx)}
	if err := s.store.AssignMemberRole(ctx, m); err != nil {
		return nil, huma.Error500InternalServerError("failed to assign role")
	}
	s.permCache.Invalidate(m.ProjectID, m.UserID)
	s.emitAuditEvent(ctx, "permission.granted", "role", m.RoleID, map[string]any{"user_id": m.UserID, "project_id": m.ProjectID})
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
		if _, err := s.store.GetProjectRole(ctx, item.RoleID); err != nil {
			if errors.Is(err, store.ErrRoleNotFound) {
				results = append(results, bulkAssignMemberResult{UserID: item.UserID, RoleID: item.RoleID, Status: "error", Error: "role not found"})
				continue
			}
			return nil, huma.Error500InternalServerError("failed to verify role")
		}
		m := &domain.ProjectMemberRole{ProjectID: projectID, UserID: item.UserID, RoleID: item.RoleID, GrantedBy: actor}
		if err := s.store.AssignMemberRole(ctx, m); err != nil {
			results = append(results, bulkAssignMemberResult{UserID: item.UserID, RoleID: item.RoleID, Status: "error", Error: "failed to assign role"})
			continue
		}
		s.permCache.Invalidate(projectID, item.UserID)
		s.emitAuditEvent(ctx, "permission.granted", "role", item.RoleID, map[string]any{"user_id": item.UserID, "project_id": projectID, "bulk": true})
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
	s.emitAuditEvent(ctx, "permission.revoked", "role", resourceID, map[string]any{"user_id": userID, "project_id": projectID, "role_id": roleID})
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
	for _, action := range req.Actions {
		if !domain.ValidScopes[action] {
			return nil, huma.Error400BadRequest("invalid action: " + action)
		}
	}
	policy := &domain.ResourcePolicy{ProjectID: req.ProjectID, ResourceType: req.ResourceType, ResourceID: req.ResourceID, UserID: req.UserID, Actions: req.Actions}
	if err := s.store.CreateResourcePolicy(ctx, policy); err != nil {
		return nil, huma.Error500InternalServerError("failed to create resource policy")
	}
	s.permCache.Invalidate(req.ProjectID, req.UserID)
	s.emitAuditEvent(ctx, "resource_policy.created", "resource_policy", policy.ID, map[string]any{"resource_type": req.ResourceType, "resource_id": req.ResourceID, "user_id": req.UserID, "actions": req.Actions})
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
	policies, err := s.store.ListResourcePolicies(ctx, input.ResourceType, input.ResourceID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list resource policies")
	}
	return &ListResourcePoliciesOutput{Body: paginatedResult(policies, limit, func(p domain.ResourcePolicy) string { return p.CreatedAt.Format(time.RFC3339Nano) })}, nil
}

type DeleteResourcePolicyInput struct {
	PolicyID string `path:"policyID"`
}

func (s *Server) handleDeleteResourcePolicy(ctx context.Context, input *DeleteResourcePolicyInput) (*struct{}, error) {
	projectID, userID, err := s.store.DeleteResourcePolicy(ctx, input.PolicyID)
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
	s.emitAuditEvent(ctx, "resource_policy.deleted", "resource_policy", input.PolicyID, map[string]any{"affected_user": userID})
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
	if err := validateTags(map[string]string{req.TagKey: req.TagValue}); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	for _, action := range req.Actions {
		if !domain.ValidScopes[action] {
			return nil, huma.Error400BadRequest("invalid action: " + action)
		}
	}
	policy := &domain.TagPolicy{ProjectID: req.ProjectID, ResourceType: req.ResourceType, UserID: req.UserID, TagKey: req.TagKey, TagValue: req.TagValue, Actions: req.Actions}
	if err := s.store.CreateTagPolicy(ctx, policy); err != nil {
		return nil, huma.Error500InternalServerError("failed to create tag policy")
	}
	s.permCache.Invalidate(req.ProjectID, req.UserID)
	s.emitAuditEvent(ctx, "tag_policy.created", "tag_policy", policy.ID, map[string]any{"tag_key": req.TagKey, "tag_value": req.TagValue, "resource_type": req.ResourceType, "user_id": req.UserID, "actions": req.Actions})
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
	projectID, userID, err := s.store.DeleteTagPolicy(ctx, input.PolicyID)
	if err != nil {
		if errors.Is(err, store.ErrTagPolicyNotFound) {
			return nil, huma.Error404NotFound("tag policy not found")
		}
		return nil, huma.Error500InternalServerError("failed to delete tag policy")
	}
	if projectID != "" && userID != "" {
		s.permCache.Invalidate(projectID, userID)
	}
	s.emitAuditEvent(ctx, "tag_policy.deleted", "tag_policy", input.PolicyID, nil)
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
	if from != nil && to != nil && from.After(*to) {
		return nil, huma.Error400BadRequest("from must be <= to")
	}
	events, err := s.store.ListAuditEvents(ctx, projectID, input.ActorID, input.ResourceType, input.ResourceID, limit+1, cursor, from, to, ascending)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list audit events")
	}
	return &ListAuditEventsOutput{Body: paginatedResult(events, limit, func(ev domain.AuditEvent) string { return ev.CreatedAt.Format(time.RFC3339Nano) })}, nil
}

type VerifyAuditChainInput struct{}
type VerifyAuditChainOutput struct {
	Body *domain.AuditChainVerification
}

func (s *Server) handleVerifyAuditChain(ctx context.Context, _ *VerifyAuditChainInput) (*VerifyAuditChainOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	if err := s.checkFeatureAllowed(ctx, projectID, billing.FeatureAuditLogs, "Audit logs"); err != nil {
		return nil, err
	}

	result, err := s.store.VerifyAuditChain(ctx, projectID)
	if err != nil {
		slog.Error("failed to verify audit chain", "project_id", projectID, "error", err)
		return nil, huma.Error500InternalServerError("failed to verify audit chain")
	}

	return &VerifyAuditChainOutput{Body: result}, nil
}
