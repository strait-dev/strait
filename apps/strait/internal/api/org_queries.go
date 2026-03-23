package api

import (
	"context"
	"time"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

type ListOrgRunsInput struct {
	OrgID  string `path:"orgID"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}
type ListOrgRunsOutput struct{ Body PaginatedResponse }

func (s *Server) handleListOrgRuns(ctx context.Context, input *ListOrgRunsInput) (*ListOrgRunsOutput, error) {
	orgID := input.OrgID
	if orgID == "" {
		return nil, huma.Error400BadRequest("org_id is required")
	}
	callerOrgID := orgIDFromContext(ctx)
	if callerOrgID == "" {
		if scopesFromContext(ctx) != nil {
			return nil, huma.Error403Forbidden("org-scoped api key required for cross-project queries")
		}
	} else if callerOrgID != orgID {
		return nil, huma.Error403Forbidden("api key does not belong to this organization")
	}
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	runs, err := s.store.ListRunsByOrg(ctx, orgID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list runs")
	}
	return &ListOrgRunsOutput{Body: paginatedResult(runs, limit, func(run domain.JobRun) string { return run.CreatedAt.Format(time.RFC3339Nano) })}, nil
}

type ListOrgJobsInput struct {
	OrgID  string `path:"orgID"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}
type ListOrgJobsOutput struct{ Body PaginatedResponse }

func (s *Server) handleListOrgJobs(ctx context.Context, input *ListOrgJobsInput) (*ListOrgJobsOutput, error) {
	orgID := input.OrgID
	if orgID == "" {
		return nil, huma.Error400BadRequest("org_id is required")
	}
	callerOrgID := orgIDFromContext(ctx)
	if callerOrgID == "" {
		if scopesFromContext(ctx) != nil {
			return nil, huma.Error403Forbidden("org-scoped api key required for cross-project queries")
		}
	} else if callerOrgID != orgID {
		return nil, huma.Error403Forbidden("api key does not belong to this organization")
	}
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	jobs, err := s.store.ListJobsByOrg(ctx, orgID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list jobs")
	}
	return &ListOrgJobsOutput{Body: paginatedResult(jobs, limit, func(job domain.Job) string { return job.CreatedAt.Format(time.RFC3339Nano) })}, nil
}
