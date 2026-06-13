package api

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"
)

// CreateProjectRequest is the request body for creating a project.
type CreateProjectRequest struct {
	ID    string `json:"id" validate:"required"`
	OrgID string `json:"org_id" validate:"required"`
	Name  string `json:"name" validate:"required,min=2"`
}

// CreateProjectInput is the typed input for creating a project.
type CreateProjectInput struct {
	Body CreateProjectRequest
}

// CreateProjectOutput is the typed output for creating a project.
type CreateProjectOutput struct {
	Body *domain.Project
}

func (s *Server) handleCreateProject(ctx context.Context, input *CreateProjectInput) (*CreateProjectOutput, error) {
	// Project creation is an org-level operation; API keys are project-scoped
	// and have no org context, so restrict to internal-secret callers.
	if scopesFromContext(ctx) != nil {
		return nil, huma.Error403Forbidden("project creation requires internal secret")
	}

	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}

	project := &domain.Project{
		ID:    req.ID,
		OrgID: req.OrgID,
		Name:  req.Name,
	}

	if err := s.createProjectWithBillingGuard(ctx, project); err != nil {
		return nil, err
	}

	s.emitAuditEvent(auditContextWithProject(ctx, project.ID), domain.AuditActionProjectCreated, "project", project.ID, map[string]any{
		"name":   project.Name,
		"org_id": project.OrgID,
	})

	return &CreateProjectOutput{Body: project}, nil
}

func (s *Server) createProjectWithBillingGuard(ctx context.Context, project *domain.Project) error {
	if s.txPool != nil && s.billingEnforcer != nil {
		return s.createProjectWithBillingGuardInTx(ctx, project)
	}
	if s.billingEnforcer != nil {
		if err := s.checkProjectBillingAdmission(ctx, project.OrgID); err != nil {
			return err
		}
	}
	if err := s.store.CreateProject(ctx, project); err != nil {
		return projectCreateAPIError(err)
	}
	s.seedProjectSystemRoles(ctx, project.ID)
	return nil
}

func (s *Server) createProjectWithBillingGuardInTx(ctx context.Context, project *domain.Project) error {
	err := store.WithTx(ctx, s.txPool, func(q *store.Queries) error {
		if err := q.AdvisoryXactLock(ctx, orgAdvisoryLockID(project.OrgID)); err != nil {
			return fmt.Errorf("advisory lock: %w", err)
		}
		if err := s.checkProjectBillingAdmission(ctx, project.OrgID); err != nil {
			return err
		}
		if err := q.CreateProject(ctx, project); err != nil {
			return fmt.Errorf("create project: %w", err)
		}
		if err := q.SeedProjectSystemRoles(ctx, project.ID); err != nil {
			slog.Error("failed to seed system roles for project", "project_id", project.ID, "error", err)
		}
		return nil
	})
	if err != nil {
		return projectCreateAPIError(err)
	}
	return nil
}

func (s *Server) checkProjectBillingAdmission(ctx context.Context, orgID string) error {
	if err := s.billingEnforcer.CheckProjectLimit(ctx, orgID); err != nil {
		var le *billing.LimitError
		if errors.As(err, &le) {
			return le
		}
		slog.Error("failed to check project limit", "error", err)
		return huma.Error500InternalServerError("failed to check project limit")
	}
	if err := s.billingEnforcer.EnsureOrgSubscription(ctx, orgID); err != nil {
		slog.Warn("failed to ensure org subscription", "org_id", orgID, "error", err)
	}
	return nil
}

func (s *Server) seedProjectSystemRoles(ctx context.Context, projectID string) {
	if err := s.store.SeedProjectSystemRoles(ctx, projectID); err != nil {
		slog.Error("failed to seed system roles for project", "project_id", projectID, "error", err)
	}
}

func projectCreateAPIError(err error) error {
	var le *billing.LimitError
	if errors.As(err, &le) {
		return le
	}
	if errors.Is(err, store.ErrProjectOrgMismatch) {
		return huma.Error409Conflict("project already exists in a different organization")
	}
	slog.Error("failed to create project", "error", err)
	return huma.Error500InternalServerError("failed to create project")
}

// ListProjectsInput is the typed input for listing projects.
type ListProjectsInput struct {
	OrgID string `query:"org_id"`
}

// ListProjectsOutput is the typed output for listing projects.
type ListProjectsOutput struct {
	Body []domain.Project
}

func (s *Server) handleListProjects(ctx context.Context, input *ListProjectsInput) (*ListProjectsOutput, error) {
	// Project listing is an org-level operation; API keys are project-scoped.
	if scopesFromContext(ctx) != nil {
		return nil, huma.Error403Forbidden("project listing requires internal secret")
	}

	if input.OrgID == "" {
		return nil, huma.Error400BadRequest("org_id query parameter is required")
	}

	projects, err := s.store.ListProjectsByOrg(ctx, input.OrgID)
	if err != nil {
		slog.Error("failed to list projects", "error", err)
		return nil, huma.Error500InternalServerError("failed to list projects")
	}

	if projects == nil {
		projects = []domain.Project{}
	}

	return &ListProjectsOutput{Body: projects}, nil
}

// GetProjectInput is the typed input for getting a project.
type GetProjectInput struct {
	ProjectID string `path:"projectID"`
}

// GetProjectOutput is the typed output for getting a project.
type GetProjectOutput struct {
	Body *domain.Project
}

func (s *Server) handleGetProject(ctx context.Context, input *GetProjectInput) (*GetProjectOutput, error) {
	if input.ProjectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	// API key callers can only read their own project.
	if scopesFromContext(ctx) != nil {
		if projectIDFromContext(ctx) != input.ProjectID {
			return nil, huma.Error403Forbidden("access denied")
		}
	}

	// Internal-secret callers: verify project belongs to caller's org context.
	if scopesFromContext(ctx) == nil {
		if err := s.validateProjectBelongsToCallerOrg(ctx, input.ProjectID); err != nil {
			// Gracefully allow when no billing enforcer or no project context
			// (backwards compat for callers without X-Project-Id).
			if projectIDFromContext(ctx) != "" {
				return nil, huma.Error403Forbidden("access denied")
			}
		}
	}

	project, err := s.store.GetProject(ctx, input.ProjectID)
	if err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			return nil, huma.Error404NotFound("project not found")
		}
		slog.Error("failed to get project", "error", err)
		return nil, huma.Error500InternalServerError("failed to get project")
	}

	return &GetProjectOutput{Body: project}, nil
}

// DeleteProjectInput is the typed input for deleting a project.
type DeleteProjectInput struct {
	ProjectID string `path:"projectID"`
}

func (s *Server) handleDeleteProject(ctx context.Context, input *DeleteProjectInput) (*struct{}, error) {
	if input.ProjectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	// API key callers can only delete their own project.
	if scopesFromContext(ctx) != nil {
		if projectIDFromContext(ctx) != input.ProjectID {
			return nil, huma.Error403Forbidden("access denied")
		}
	}

	// Internal-secret callers: verify project belongs to caller's org context.
	if scopesFromContext(ctx) == nil {
		if err := s.validateProjectBelongsToCallerOrg(ctx, input.ProjectID); err != nil {
			if projectIDFromContext(ctx) != "" {
				return nil, huma.Error403Forbidden("access denied")
			}
		}
	}

	if err := s.store.DeleteProject(ctx, input.ProjectID); err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			return nil, huma.Error404NotFound("project not found")
		}
		slog.Error("failed to delete project", "error", err)
		return nil, huma.Error500InternalServerError("failed to delete project")
	}

	s.emitAuditEvent(ctx, domain.AuditActionProjectDeleted, "project", input.ProjectID, nil)

	return nil, nil
}

// orgAdvisoryLockID returns a deterministic int64 hash of the org ID for use
// as a pg_advisory_xact_lock key, serializing per-org project creation.
func orgAdvisoryLockID(orgID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(orgID))
	return int64(h.Sum64()) //nolint:gosec // advisory lock IDs can wrap
}
