package api

import (
	"context"
	"errors"
	"time"

	straitcache "strait/internal/cache"
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

type jobDependencyListVersionGetter interface {
	GetJobDependencyListVersion(context.Context, string) (int64, error)
}

func (s *Server) handleCreateJobDependency(ctx context.Context, input *CreateJobDependencyInput) (*CreateJobDependencyOutput, error) {
	jobID := input.JobID
	job, err := s.store.GetJob(ctx, jobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}
	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
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
	if err := requireEnvironmentMatch(ctx, depJob.EnvironmentID); err != nil {
		return nil, huma.Error400BadRequest("dependency job does not belong to the authenticated environment")
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
	s.refreshJobDependencyCacheAfterMutation(ctx, jobID)
	s.emitAuditEvent(ctx, domain.AuditActionJobDependencyCreated, "job_dependency", dep.ID, map[string]any{
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
	job, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}
	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	var deps []domain.JobDependency
	if cursor == nil {
		deps, err = s.listCachedJobDependencies(ctx, input.JobID, limit+1)
	} else {
		deps, err = s.store.ListJobDependencies(ctx, input.JobID, limit+1, cursor)
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list job dependencies")
	}
	return &ListJobDependenciesOutput{Body: paginatedResult(deps, limit, func(d domain.JobDependency) string { return d.CreatedAt.Format(time.RFC3339Nano) })}, nil
}

func (s *Server) listCachedJobDependencies(ctx context.Context, jobID string, limit int) ([]domain.JobDependency, error) {
	key := jobDepsCacheKey{JobID: jobID, Limit: limit}
	loader := func(loadCtx context.Context, loadKey jobDepsCacheKey) (straitcache.Versioned[[]domain.JobDependency], error) {
		deps, err := s.store.ListJobDependencies(loadCtx, loadKey.JobID, loadKey.Limit, nil)
		if err != nil {
			return straitcache.Versioned[[]domain.JobDependency]{}, err
		}
		version, err := s.jobDependencyListVersion(loadCtx, loadKey.JobID, deps)
		if err != nil {
			return straitcache.Versioned[[]domain.JobDependency]{}, err
		}
		return straitcache.Versioned[[]domain.JobDependency]{Value: deps, Version: version}, nil
	}
	if s.jobDependencyCache == nil || !jobDependencyCacheableLimit(limit) {
		loaded, err := loader(ctx, key)
		return loaded.Value, err
	}
	return s.jobDependencyCache.List(ctx, key, loader)
}

func (s *Server) refreshJobDependencyCacheAfterMutation(ctx context.Context, jobID string) {
	if s.jobDependencyCache == nil {
		return
	}
	s.jobDependencyCache.RefreshJob(ctx, jobID, func(loadCtx context.Context, loadKey jobDepsCacheKey) (straitcache.Versioned[[]domain.JobDependency], error) {
		deps, err := s.store.ListJobDependencies(loadCtx, loadKey.JobID, loadKey.Limit, nil)
		if err != nil {
			return straitcache.Versioned[[]domain.JobDependency]{}, err
		}
		version, err := s.jobDependencyListVersion(loadCtx, loadKey.JobID, deps)
		if err != nil {
			return straitcache.Versioned[[]domain.JobDependency]{}, err
		}
		return straitcache.Versioned[[]domain.JobDependency]{Value: deps, Version: version}, nil
	})
}

func (s *Server) jobDependencyListVersion(ctx context.Context, jobID string, deps []domain.JobDependency) (int64, error) {
	version := jobDependenciesCacheVersion(deps)
	if version > 0 {
		return version, nil
	}
	if getter, ok := s.store.(jobDependencyListVersionGetter); ok {
		return getter.GetJobDependencyListVersion(ctx, jobID)
	}
	return 0, nil
}

type DeleteJobDependencyInput struct {
	JobID string `path:"jobID"`
	DepID string `path:"depID"`
}

func (s *Server) handleDeleteJobDependency(ctx context.Context, input *DeleteJobDependencyInput) (*struct{}, error) {
	job, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}
	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
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
	s.refreshJobDependencyCacheAfterMutation(ctx, input.JobID)
	s.emitAuditEvent(ctx, domain.AuditActionJobDependencyDeleted, "job_dependency", input.DepID, map[string]any{
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
