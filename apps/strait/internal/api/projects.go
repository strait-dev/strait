package api

import (
	"context"
	"errors"
	"hash/fnv"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"
)

// orgAdvisoryLockID returns a deterministic int64 hash of the org ID for
// use as a pg_advisory_xact_lock key. Kept at the api package level for
// tests; the platform/projects service has its own internal copy used
// during transactional project creation.
func orgAdvisoryLockID(orgID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(orgID))
	return int64(h.Sum64()) //nolint:gosec // advisory lock IDs can wrap
}

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

	if err := s.platformProjects.Create(ctx, project); err != nil {
		var le *billing.LimitError
		if errors.As(err, &le) {
			return nil, le
		}
		slog.Error("failed to create project", "error", err)
		return nil, huma.Error500InternalServerError("failed to create project")
	}

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

	projects, err := s.platformProjects.ListByOrg(ctx, input.OrgID)
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

	project, err := s.platformProjects.Get(ctx, input.ProjectID)
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

	err := s.platformProjects.Delete(ctx, input.ProjectID)
	if err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			return nil, huma.Error404NotFound("project not found")
		}
		slog.Error("failed to delete project", "error", err)
		return nil, huma.Error500InternalServerError("failed to delete project")
	}

	return nil, nil
}
