package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
)

// Roles.

type createRoleRequest struct {
	Name         string   `json:"name" validate:"required"`
	Description  string   `json:"description"`
	Permissions  []string `json:"permissions" validate:"required,min=1"`
	ParentRoleID string   `json:"parent_role_id,omitempty"`
}

func (s *Server) emitAuditEvent(ctx context.Context, action, resourceType, resourceID string, details map[string]any) {
	if s.config == nil || !s.config.FFAuditLog {
		return
	}

	detailsJSON, err := json.Marshal(details)
	if err != nil {
		slog.Warn("failed to marshal audit event details", "action", action, "error", err)
		return
	}

	actorType, _ := ctx.Value(ctxActorTypeKey).(string)
	ev := &domain.AuditEvent{
		ProjectID:    projectIDFromContext(ctx),
		ActorID:      actorFromContext(ctx),
		ActorType:    actorType,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      detailsJSON,
	}
	if err := s.store.CreateAuditEvent(ctx, ev); err != nil {
		slog.Warn("failed to create audit event", "action", action, "resource_type", resourceType, "resource_id", resourceID, "error", err)
	}
}

func (s *Server) handleCreateRole(w http.ResponseWriter, r *http.Request) {
	var req createRoleRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, s.maxRequestBodySize)).Decode(&req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}
	if err := domain.ValidateScopes(req.Permissions); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	role := &domain.ProjectRole{
		ProjectID:    projectIDFromContext(r.Context()),
		Name:         req.Name,
		Description:  req.Description,
		Permissions:  req.Permissions,
		ParentRoleID: req.ParentRoleID,
	}

	if err := s.store.CreateProjectRole(r.Context(), role); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create role")
		return
	}

	s.emitAuditEvent(r.Context(), "role.created", "role", role.ID, map[string]any{
		"name":          role.Name,
		"description":   role.Description,
		"permissions":   role.Permissions,
		"parent_role":   role.ParentRoleID,
		"project_id":    role.ProjectID,
		"is_system":     role.IsSystem,
		"change_source": "rbac_api",
	})
	respondJSON(w, http.StatusCreated, role)
}

func (s *Server) handleListRoles(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	roles, err := s.store.ListProjectRoles(r.Context(), projectID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list roles")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(roles, limit, func(role domain.ProjectRole) string {
		return role.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleGetRole(w http.ResponseWriter, r *http.Request) {
	roleID := chi.URLParam(r, "roleID")
	role, err := s.store.GetProjectRole(r.Context(), roleID)
	if err != nil {
		if errors.Is(err, store.ErrRoleNotFound) {
			respondError(w, r, http.StatusNotFound, "role not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get role")
		return
	}

	if r.URL.Query().Get("include_lineage") != "true" {
		respondJSON(w, http.StatusOK, role)
		return
	}

	lineage := make([]domain.ProjectRole, 0, 4)
	currentParent := role.ParentRoleID
	for depth := 0; depth < 20 && currentParent != ""; depth++ {
		parent, parentErr := s.store.GetProjectRole(r.Context(), currentParent)
		if parentErr != nil {
			if errors.Is(parentErr, store.ErrRoleNotFound) {
				break
			}
			respondError(w, r, http.StatusInternalServerError, "failed to load role lineage")
			return
		}
		lineage = append(lineage, *parent)
		currentParent = parent.ParentRoleID
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"role":    role,
		"lineage": lineage,
	})
}

type updateRoleRequest struct {
	Name         string   `json:"name" validate:"required"`
	Description  string   `json:"description"`
	Permissions  []string `json:"permissions" validate:"required,min=1"`
	ParentRoleID string   `json:"parent_role_id,omitempty"`
}

func (s *Server) handleUpdateRole(w http.ResponseWriter, r *http.Request) {
	var req updateRoleRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, s.maxRequestBodySize)).Decode(&req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}
	if err := domain.ValidateScopes(req.Permissions); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	roleID := chi.URLParam(r, "roleID")
	previousRole, _ := s.store.GetProjectRole(r.Context(), roleID)
	role := &domain.ProjectRole{
		ID:           roleID,
		Name:         req.Name,
		Description:  req.Description,
		Permissions:  req.Permissions,
		ParentRoleID: req.ParentRoleID,
	}

	if err := s.store.UpdateProjectRole(r.Context(), role); err != nil {
		if errors.Is(err, store.ErrRoleNotFound) {
			respondError(w, r, http.StatusNotFound, "role not found or is a system role")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to update role")
		return
	}

	updated, err := s.store.GetProjectRole(r.Context(), roleID)
	if err != nil {
		if errors.Is(err, store.ErrRoleNotFound) {
			respondError(w, r, http.StatusNotFound, "role not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to load updated role")
		return
	}
	if updated == nil {
		updated = role
	}

	s.emitAuditEvent(r.Context(), "role.updated", "role", roleID, map[string]any{
		"changes": map[string]any{
			"before": previousRole,
			"after":  updated,
		},
	})
	respondJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteRole(w http.ResponseWriter, r *http.Request) {
	roleID := chi.URLParam(r, "roleID")
	if err := s.store.DeleteProjectRole(r.Context(), roleID); err != nil {
		if errors.Is(err, store.ErrRoleNotFound) {
			respondError(w, r, http.StatusNotFound, "role not found or is a system role")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to delete role")
		return
	}
	slog.Info("role deleted", "role_id", roleID, "actor", actorFromContext(r.Context()),
		"project_id", projectIDFromContext(r.Context()))
	s.emitAuditEvent(r.Context(), "role.deleted", "role", roleID, nil)
	w.WriteHeader(http.StatusNoContent)
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

func (s *Server) handleAssignMember(w http.ResponseWriter, r *http.Request) {
	var req assignMemberRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, s.maxRequestBodySize)).Decode(&req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}

	// Verify the role exists.
	if _, err := s.store.GetProjectRole(r.Context(), req.RoleID); err != nil {
		if errors.Is(err, store.ErrRoleNotFound) {
			respondError(w, r, http.StatusBadRequest, "role not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to verify role")
		return
	}

	m := &domain.ProjectMemberRole{
		ProjectID: projectIDFromContext(r.Context()),
		UserID:    req.UserID,
		RoleID:    req.RoleID,
		GrantedBy: actorFromContext(r.Context()),
	}

	if err := s.store.AssignMemberRole(r.Context(), m); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to assign role")
		return
	}

	s.permCache.Invalidate(m.ProjectID, m.UserID)
	s.emitAuditEvent(r.Context(), "permission.granted", "role", m.RoleID, map[string]any{
		"user_id":    m.UserID,
		"project_id": m.ProjectID,
	})
	respondJSON(w, http.StatusCreated, m)
}

func (s *Server) handleBulkAssignMembers(w http.ResponseWriter, r *http.Request) {
	var req bulkAssignMembersRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}

	projectID := projectIDFromContext(r.Context())
	actor := actorFromContext(r.Context())
	results := make([]bulkAssignMemberResult, 0, len(req.Items))

	for _, item := range req.Items {
		if item.UserID == "" || item.RoleID == "" {
			results = append(results, bulkAssignMemberResult{UserID: item.UserID, RoleID: item.RoleID, Status: "error", Error: "user_id and role_id are required"})
			continue
		}

		if _, err := s.store.GetProjectRole(r.Context(), item.RoleID); err != nil {
			if errors.Is(err, store.ErrRoleNotFound) {
				results = append(results, bulkAssignMemberResult{UserID: item.UserID, RoleID: item.RoleID, Status: "error", Error: "role not found"})
				continue
			}
			respondError(w, r, http.StatusInternalServerError, "failed to verify role")
			return
		}

		m := &domain.ProjectMemberRole{
			ProjectID: projectID,
			UserID:    item.UserID,
			RoleID:    item.RoleID,
			GrantedBy: actor,
		}
		if err := s.store.AssignMemberRole(r.Context(), m); err != nil {
			results = append(results, bulkAssignMemberResult{UserID: item.UserID, RoleID: item.RoleID, Status: "error", Error: "failed to assign role"})
			continue
		}

		s.permCache.Invalidate(projectID, item.UserID)
		s.emitAuditEvent(r.Context(), "permission.granted", "role", item.RoleID, map[string]any{
			"user_id":    item.UserID,
			"project_id": projectID,
			"bulk":       true,
		})
		results = append(results, bulkAssignMemberResult{UserID: item.UserID, RoleID: item.RoleID, Status: "assigned"})
	}

	respondJSON(w, http.StatusOK, map[string]any{"results": results, "total": len(results)})
}

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	members, err := s.store.ListProjectMembers(r.Context(), projectID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list members")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(members, limit, func(m domain.ProjectMemberRole) string {
		return m.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	projectID := projectIDFromContext(r.Context())
	memberRole, _ := s.store.GetMemberRole(r.Context(), projectID, userID)

	if err := s.store.RemoveMemberRole(r.Context(), projectID, userID); err != nil {
		if errors.Is(err, store.ErrMemberNotFound) {
			respondError(w, r, http.StatusNotFound, "member not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to remove member")
		return
	}
	s.permCache.Invalidate(projectID, userID)
	slog.Info("member removed", "user_id", userID, "actor", actorFromContext(r.Context()),
		"project_id", projectID)
	resourceID := userID
	roleID := ""
	if memberRole != nil && memberRole.RoleID != "" {
		roleID = memberRole.RoleID
		resourceID = memberRole.RoleID
	}
	s.emitAuditEvent(r.Context(), "permission.revoked", "role", resourceID, map[string]any{
		"user_id":    userID,
		"project_id": projectID,
		"role_id":    roleID,
	})
	w.WriteHeader(http.StatusNoContent)
}

// System Roles.

func (s *Server) handleSeedSystemRoles(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	if err := s.store.SeedProjectSystemRoles(r.Context(), projectID); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to seed system roles")
		return
	}

	roles, err := s.store.ListProjectRoles(r.Context(), projectID, 100, nil)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list roles after seeding")
		return
	}

	respondJSON(w, http.StatusOK, roles)
}

// Resource Policies.

type createResourcePolicyRequest struct {
	ProjectID    string   `json:"project_id" validate:"required"`
	ResourceType string   `json:"resource_type" validate:"required"`
	ResourceID   string   `json:"resource_id" validate:"required"`
	UserID       string   `json:"user_id" validate:"required"`
	Actions      []string `json:"actions" validate:"required,min=1"`
}

func (s *Server) handleCreateResourcePolicy(w http.ResponseWriter, r *http.Request) {
	var req createResourcePolicyRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}

	for _, action := range req.Actions {
		if !domain.ValidScopes[action] {
			respondError(w, r, http.StatusBadRequest, "invalid action: "+action)
			return
		}
	}

	policy := &domain.ResourcePolicy{
		ProjectID:    req.ProjectID,
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
		UserID:       req.UserID,
		Actions:      req.Actions,
	}

	if err := s.store.CreateResourcePolicy(r.Context(), policy); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create resource policy")
		return
	}

	// Invalidate cache for the affected user.
	s.permCache.Invalidate(req.ProjectID, req.UserID)
	s.emitAuditEvent(r.Context(), "resource_policy.created", "resource_policy", policy.ID, map[string]any{
		"resource_type": req.ResourceType,
		"resource_id":   req.ResourceID,
		"user_id":       req.UserID,
		"actions":       req.Actions,
	})

	respondJSON(w, http.StatusCreated, policy)
}

func (s *Server) handleListResourcePolicies(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	resourceType := query.Get("resource_type")
	resourceID := query.Get("resource_id")
	if resourceType == "" || resourceID == "" {
		respondError(w, r, http.StatusBadRequest, "resource_type and resource_id are required")
		return
	}

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	policies, err := s.store.ListResourcePolicies(r.Context(), resourceType, resourceID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list resource policies")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(policies, limit, func(p domain.ResourcePolicy) string {
		return p.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleDeleteResourcePolicy(w http.ResponseWriter, r *http.Request) {
	policyID := chi.URLParam(r, "policyID")

	projectID, userID, err := s.store.DeleteResourcePolicy(r.Context(), policyID)
	if err != nil {
		if errors.Is(err, store.ErrResourcePolicyNotFound) {
			respondError(w, r, http.StatusNotFound, "resource policy not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to delete resource policy")
		return
	}

	// Invalidate cache for the affected user so revoked access takes effect immediately.
	if projectID != "" && userID != "" {
		s.permCache.Invalidate(projectID, userID)
	}

	slog.Info("resource policy deleted", "policy_id", policyID, "actor", actorFromContext(r.Context()),
		"affected_user", userID, "project_id", projectID)
	s.emitAuditEvent(r.Context(), "resource_policy.deleted", "resource_policy", policyID, map[string]any{"affected_user": userID})
	w.WriteHeader(http.StatusNoContent)
}

type createTagPolicyRequest struct {
	ProjectID    string   `json:"project_id" validate:"required"`
	ResourceType string   `json:"resource_type" validate:"required"`
	UserID       string   `json:"user_id" validate:"required"`
	TagKey       string   `json:"tag_key" validate:"required"`
	TagValue     string   `json:"tag_value,omitempty"`
	Actions      []string `json:"actions" validate:"required,min=1"`
}

func (s *Server) handleCreateTagPolicy(w http.ResponseWriter, r *http.Request) {
	var req createTagPolicyRequest
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.validateRequest(w, r, &req) {
		return
	}
	if err := validateTags(map[string]string{req.TagKey: req.TagValue}); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	for _, action := range req.Actions {
		if !domain.ValidScopes[action] {
			respondError(w, r, http.StatusBadRequest, "invalid action: "+action)
			return
		}
	}

	policy := &domain.TagPolicy{
		ProjectID:    req.ProjectID,
		ResourceType: req.ResourceType,
		UserID:       req.UserID,
		TagKey:       req.TagKey,
		TagValue:     req.TagValue,
		Actions:      req.Actions,
	}
	if err := s.store.CreateTagPolicy(r.Context(), policy); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create tag policy")
		return
	}
	s.permCache.Invalidate(req.ProjectID, req.UserID)
	s.emitAuditEvent(r.Context(), "tag_policy.created", "tag_policy", policy.ID, map[string]any{
		"tag_key":       req.TagKey,
		"tag_value":     req.TagValue,
		"resource_type": req.ResourceType,
		"user_id":       req.UserID,
		"actions":       req.Actions,
	})
	respondJSON(w, http.StatusCreated, policy)
}

func (s *Server) handleListTagPolicies(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	projectID := query.Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}
	resourceType := query.Get("resource_type")
	userID := query.Get("user_id")

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	policies, err := s.store.ListTagPolicies(r.Context(), projectID, resourceType, userID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list tag policies")
		return
	}
	respondJSON(w, http.StatusOK, paginatedResult(policies, limit, func(p domain.TagPolicy) string {
		return p.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleDeleteTagPolicy(w http.ResponseWriter, r *http.Request) {
	policyID := chi.URLParam(r, "policyID")
	projectID, userID, err := s.store.DeleteTagPolicy(r.Context(), policyID)
	if err != nil {
		if errors.Is(err, store.ErrTagPolicyNotFound) {
			respondError(w, r, http.StatusNotFound, "tag policy not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to delete tag policy")
		return
	}
	if projectID != "" && userID != "" {
		s.permCache.Invalidate(projectID, userID)
	}
	s.emitAuditEvent(r.Context(), "tag_policy.deleted", "tag_policy", policyID, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListAuditEvents(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	projectID := query.Get("project_id")
	if projectID == "" {
		projectID = projectIDFromContext(r.Context())
	}
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	actorID := query.Get("actor_id")
	resourceType := query.Get("resource_type")
	resourceID := query.Get("resource_id")
	order := query.Get("order")
	ascending := order == "asc"
	if order != "" && order != "asc" && order != "desc" {
		respondError(w, r, http.StatusBadRequest, "order must be one of: asc, desc")
		return
	}

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	var from, to *time.Time
	if raw := query.Get("from"); raw != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, raw)
		if parseErr != nil {
			respondError(w, r, http.StatusBadRequest, "from must be a valid RFC3339 timestamp")
			return
		}
		from = &parsed
	}
	if raw := query.Get("to"); raw != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, raw)
		if parseErr != nil {
			respondError(w, r, http.StatusBadRequest, "to must be a valid RFC3339 timestamp")
			return
		}
		to = &parsed
	}
	if from != nil && to != nil && from.After(*to) {
		respondError(w, r, http.StatusBadRequest, "from must be <= to")
		return
	}

	events, err := s.store.ListAuditEvents(r.Context(), projectID, actorID, resourceType, resourceID, limit+1, cursor, from, to, ascending)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list audit events")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(events, limit, func(ev domain.AuditEvent) string {
		return ev.CreatedAt.Format(time.RFC3339Nano)
	}))
}
