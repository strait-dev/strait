package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
)

// Roles.

type createRoleRequest struct {
	Name        string   `json:"name" validate:"required"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions" validate:"required,min=1"`
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
		ProjectID:   projectIDFromContext(r.Context()),
		Name:        req.Name,
		Description: req.Description,
		Permissions: req.Permissions,
	}

	if err := s.store.CreateProjectRole(r.Context(), role); err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to create role")
		return
	}

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
	respondJSON(w, http.StatusOK, role)
}

type updateRoleRequest struct {
	Name        string   `json:"name" validate:"required"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions" validate:"required,min=1"`
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
	role := &domain.ProjectRole{
		ID:          roleID,
		Name:        req.Name,
		Description: req.Description,
		Permissions: req.Permissions,
	}

	if err := s.store.UpdateProjectRole(r.Context(), role); err != nil {
		if errors.Is(err, store.ErrRoleNotFound) {
			respondError(w, r, http.StatusNotFound, "role not found or is a system role")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to update role")
		return
	}

	respondJSON(w, http.StatusOK, role)
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
	w.WriteHeader(http.StatusNoContent)
}

// Members.

type assignMemberRequest struct {
	UserID string `json:"user_id" validate:"required"`
	RoleID string `json:"role_id" validate:"required"`
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
	respondJSON(w, http.StatusCreated, m)
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

	if err := s.store.RemoveMemberRole(r.Context(), projectID, userID); err != nil {
		if errors.Is(err, store.ErrMemberNotFound) {
			respondError(w, r, http.StatusNotFound, "member not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to remove member")
		return
	}
	s.permCache.Invalidate(projectID, userID)
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

	policies, err := s.store.ListResourcePolicies(r.Context(), resourceType, resourceID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list resource policies")
		return
	}

	respondJSON(w, http.StatusOK, policies)
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

	w.WriteHeader(http.StatusNoContent)
}
