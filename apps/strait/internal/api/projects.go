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

	// Serialize project creation per org using an advisory lock inside a
	// transaction so the limit check and insert are atomic.
	if s.txPool != nil && s.billingEnforcer != nil { //nolint:nestif
		txErr := store.WithTx(ctx, s.txPool, func(q *store.Queries) error {
			// Advisory lock keyed on org_id to serialize concurrent creates.
			if err := q.AdvisoryXactLock(ctx, orgAdvisoryLockID(req.OrgID)); err != nil {
				return fmt.Errorf("advisory lock: %w", err)
			}

			if err := s.billingEnforcer.CheckProjectLimit(ctx, req.OrgID); err != nil {
				return err
			}

			// Ensure the org has a subscription row (lazy init for free tier).
			if subErr := s.billingEnforcer.EnsureOrgSubscription(ctx, req.OrgID); subErr != nil {
				slog.Warn("failed to ensure org subscription", "org_id", req.OrgID, "error", subErr)
			}

			if err := q.CreateProject(ctx, project); err != nil {
				return fmt.Errorf("create project: %w", err)
			}

			if err := q.SeedProjectSystemRoles(ctx, project.ID); err != nil {
				slog.Error("failed to seed system roles for project", "project_id", project.ID, "error", err)
			}

			return nil
		})
		if txErr != nil {
			var le *billing.LimitError
			if errors.As(txErr, &le) {
				return nil, le
			}
			if errors.Is(txErr, store.ErrProjectOrgMismatch) {
				return nil, huma.Error409Conflict("project already exists in a different organization")
			}
			slog.Error("failed to create project", "error", txErr)
			return nil, huma.Error500InternalServerError("failed to create project")
		}
	} else {
		// Fallback: no transaction pool available, run without advisory lock.
		if s.billingEnforcer != nil {
			if err := s.billingEnforcer.CheckProjectLimit(ctx, req.OrgID); err != nil {
				var le *billing.LimitError
				if errors.As(err, &le) {
					return nil, le
				}
				slog.Error("failed to check project limit", "error", err)
				return nil, huma.Error500InternalServerError("failed to check project limit")
			}

			if err := s.billingEnforcer.EnsureOrgSubscription(ctx, req.OrgID); err != nil {
				slog.Warn("failed to ensure org subscription", "org_id", req.OrgID, "error", err)
			}
		}

		if err := s.store.CreateProject(ctx, project); err != nil {
			if errors.Is(err, store.ErrProjectOrgMismatch) {
				return nil, huma.Error409Conflict("project already exists in a different organization")
			}
			slog.Error("failed to create project", "error", err)
			return nil, huma.Error500InternalServerError("failed to create project")
		}

		if err := s.store.SeedProjectSystemRoles(ctx, project.ID); err != nil {
			slog.Error("failed to seed system roles for project", "project_id", project.ID, "error", err)
		}
	}

	s.emitAuditEvent(ctx, domain.AuditActionProjectCreated, "project", project.ID, map[string]any{
		"name":   project.Name,
		"org_id": project.OrgID,
	})

	return &CreateProjectOutput{Body: project}, nil
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
