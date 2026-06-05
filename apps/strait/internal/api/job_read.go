package api

import (
	"context"
	"errors"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/domain"
	"strait/internal/store"
)

// GetJobInput is the typed input for getting a single job.
type GetJobInput struct {
	JobID string `path:"jobID"`
}

// GetJobOutput is the typed output for getting a single job.
type GetJobOutput struct {
	Body *domain.Job
}

func (s *Server) handleGetJob(ctx context.Context, input *GetJobInput) (*GetJobOutput, error) {
	job, err := s.store.GetJob(ctx, input.JobID)
	if err != nil {
		if errors.Is(err, store.ErrJobNotFound) {
			return nil, huma.Error404NotFound("job not found")
		}
		return nil, huma.Error500InternalServerError("failed to get job")
	}
	if job == nil {
		return nil, huma.Error404NotFound("job not found")
	}

	if err := requireProjectMatch(ctx, job.ProjectID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}
	if err := requireEnvironmentMatch(ctx, job.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("job not found")
	}

	return &GetJobOutput{Body: job}, nil
}

// ListJobsInput is the typed input for listing jobs.
type ListJobsInput struct {
	Slug     string `query:"slug"`
	TagKey   string `query:"tag_key"`
	TagValue string `query:"tag_value"`
	Limit    string `query:"limit"`
	Cursor   string `query:"cursor"`
}

// ListJobsOutput is the typed output for listing jobs.
type ListJobsOutput struct {
	Body PaginatedResponse
}

func (s *Server) handleListJobs(ctx context.Context, input *ListJobsInput) (*ListJobsOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if input.TagValue != "" && input.TagKey == "" {
		return nil, huma.Error400BadRequest("tag_key is required when tag_value is provided")
	}

	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	// Slug lookup: return a single-item list when ?slug= is provided.
	if input.Slug != "" {
		emptyPage := func() *ListJobsOutput {
			return &ListJobsOutput{Body: paginatedResult([]domain.Job{}, limit, func(j domain.Job) string {
				return j.CreatedAt.Format(time.RFC3339Nano)
			})}
		}
		job, jobErr := s.store.GetJobBySlug(ctx, projectID, input.Slug)
		if jobErr != nil {
			if errors.Is(jobErr, store.ErrJobNotFound) {
				return emptyPage(), nil
			}
			return nil, huma.Error500InternalServerError("failed to look up job by slug")
		}
		if callerEnv := environmentIDFromContext(ctx); callerEnv != "" && job.EnvironmentID != callerEnv {
			return emptyPage(), nil
		}
		return &ListJobsOutput{Body: paginatedResult([]domain.Job{*job}, limit, func(j domain.Job) string {
			return j.CreatedAt.Format(time.RFC3339Nano)
		})}, nil
	}

	var (
		jobs    []domain.Job
		listErr error
	)
	if input.TagKey != "" {
		jobs, listErr = s.store.ListJobsByTag(ctx, projectID, input.TagKey, input.TagValue, limit+1, cursor)
	} else {
		jobs, listErr = s.store.ListJobs(ctx, projectID, limit+1, cursor)
	}
	if listErr != nil {
		return nil, huma.Error500InternalServerError("failed to list jobs")
	}
	jobs = filterJobsForEnvironment(ctx, jobs)

	return &ListJobsOutput{Body: paginatedResult(jobs, limit, func(j domain.Job) string {
		return j.CreatedAt.Format(time.RFC3339Nano)
	})}, nil
}

func filterJobsForEnvironment(ctx context.Context, jobs []domain.Job) []domain.Job {
	callerEnv := environmentIDFromContext(ctx)
	if callerEnv == "" {
		return jobs
	}
	filtered := jobs[:0]
	for _, job := range jobs {
		if job.EnvironmentID == callerEnv {
			filtered = append(filtered, job)
		}
	}
	return filtered
}
