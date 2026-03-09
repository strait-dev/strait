package api

import (
	"encoding/json"
	"errors"
	"net/http"

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
	roles, err := s.store.ListProjectRoles(r.Context(), projectID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list roles")
		return
	}
	respondJSON(w, http.StatusOK, roles)
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

	respondJSON(w, http.StatusCreated, m)
}

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	members, err := s.store.ListProjectMembers(r.Context(), projectID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list members")
		return
	}
	respondJSON(w, http.StatusOK, members)
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
	w.WriteHeader(http.StatusNoContent)
}
