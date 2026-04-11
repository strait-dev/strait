package api

import (
	"context"
	"errors"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

type CreateJobDependencyRequest struct {
	DependsOnJobID string `json:"depends_on_job_id" validate:"required"`
	Condition      string `json:"condition,omitempty"`
}
type CreateJobDependencyInput struct {
	JobID string `path:"jobID"`
	Body  CreateJobDependencyRequest
}
type CreateJobDependencyOutput struct{ Body *domain.JobDependency }

func (s *Server) handleCreateJobDependency(ctx context.Context, input *CreateJobDependencyInput) (*CreateJobDependencyOutput, error) {
	jobID := input.JobID
	job, err := s.store.GetJob(ctx, jobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if req.DependsOnJobID == jobID {
		return nil, huma.Error400BadRequest("job cannot depend on itself")
	}
	depJob, err := s.store.GetJob(ctx, req.DependsOnJobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error400BadRequest("depends_on_job_id does not exist")
		}
		return nil, huma.Error500InternalServerError("failed to get dependency job")
	}
	if depJob.ProjectID != job.ProjectID {
		return nil, huma.Error400BadRequest("dependency jobs must belong to the same project")
	}
	condition := req.Condition
	if condition == "" {
		condition = "completed"
	}
	if !isValidDependencyCondition(condition) {
		return nil, huma.Error400BadRequest("condition must be one of: completed, failed, any")
	}
	dep := &domain.JobDependency{JobID: jobID, DependsOnJobID: req.DependsOnJobID, Condition: condition}
	if err := s.store.CreateJobDependency(ctx, dep); err != nil {
		return nil, huma.Error500InternalServerError("failed to create job dependency")
	}
	s.emitAuditEvent(ctx, "job_dependency.created", "job_dependency", dep.ID, map[string]any{
		"job_id":            jobID,
		"depends_on_job_id": req.DependsOnJobID,
		"condition":         condition,
	})
	return &CreateJobDependencyOutput{Body: dep}, nil
}

type ListJobDependenciesInput struct {
	JobID  string `path:"jobID"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}
type ListJobDependenciesOutput struct{ Body PaginatedResponse }

func (s *Server) handleListJobDependencies(ctx context.Context, input *ListJobDependenciesInput) (*ListJobDependenciesOutput, error) {
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	deps, err := s.store.ListJobDependencies(ctx, input.JobID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list job dependencies")
	}
	return &ListJobDependenciesOutput{Body: paginatedResult(deps, limit, func(d domain.JobDependency) string { return d.CreatedAt.Format(time.RFC3339Nano) })}, nil
}

type DeleteJobDependencyInput struct {
	JobID string `path:"jobID"`
	DepID string `path:"depID"`
}

func (s *Server) handleDeleteJobDependency(ctx context.Context, input *DeleteJobDependencyInput) (*struct{}, error) {
	if _, err := s.store.GetJob(ctx, input.JobID); err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}
	dep, err := s.store.GetJobDependency(ctx, input.DepID)
	if err != nil {
		if errors.Is(err, store.ErrJobDependencyNotFound) {
			return nil, huma.Error404NotFound("job dependency not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job dependency")
	}
	if dep.JobID != input.JobID {
		return nil, huma.Error404NotFound("job dependency not found")
	}
	if err := s.store.DeleteJobDependency(ctx, input.DepID); err != nil {
		return nil, huma.Error500InternalServerError("failed to delete job dependency")
	}
	s.emitAuditEvent(ctx, "job_dependency.deleted", "job_dependency", input.DepID, map[string]any{
		"job_id":            input.JobID,
		"depends_on_job_id": dep.DependsOnJobID,
	})
	return nil, nil
}

func isValidDependencyCondition(condition string) bool {
	switch condition {
	case "completed", "failed", "any":
		return true
	default:
		return false
	}
}
