package api

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

// validateOrgIDForInternalCaller is the gate used by org-query handlers when
// the caller authenticated with the internal management secret (callerOrgID
// empty AND scopes nil). Internal callers can list across any organization,
// so the supplied identifier is structurally validated (non-empty, bounded
// length, no control or whitespace characters) before it reaches the store
// to prevent silent bypasses on malformed input. An audit-style log entry
// is emitted so every cross-org listing exercised through the internal path
// is observable, since no audit_event row can be attributed (no project
// context).
func validateOrgIDForInternalCaller(ctx context.Context, orgID, op string) error {
	if orgID == "" {
		return huma.Error400BadRequest("org_id is required")
	}
	if len(orgID) > 128 {
		return huma.Error400BadRequest("org_id is too long")
	}
	for _, r := range orgID {
		if r <= 0x20 || r == 0x7f {
			return huma.Error400BadRequest("org_id contains invalid characters")
		}
	}
	slog.Info("org_queries internal-secret listing",
		"op", op,
		"org_id", orgID,
		"actor", actorFromContext(ctx),
		"request_id", requestIDFromContext(ctx),
	)
	return nil
}

type ListOrgRunsInput struct {
	OrgID  string `path:"orgID"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}
type ListOrgRunsOutput struct{ Body PaginatedResponse }

func (s *Server) handleListOrgRuns(ctx context.Context, input *ListOrgRunsInput) (*ListOrgRunsOutput, error) {
	orgID := input.OrgID
	callerOrgID := orgIDFromContext(ctx)
	if callerOrgID == "" {
		if scopesFromContext(ctx) != nil {
			return nil, huma.Error403Forbidden("org-scoped api key required for cross-project queries")
		}
		if err := validateOrgIDForInternalCaller(ctx, orgID, "ListOrgRuns"); err != nil {
			return nil, err
		}
	} else {
		if orgID == "" {
			return nil, huma.Error400BadRequest("org_id is required")
		}
		if callerOrgID != orgID {
			return nil, huma.Error403Forbidden("api key does not belong to this organization")
		}
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
	callerOrgID := orgIDFromContext(ctx)
	if callerOrgID == "" {
		if scopesFromContext(ctx) != nil {
			return nil, huma.Error403Forbidden("org-scoped api key required for cross-project queries")
		}
		if err := validateOrgIDForInternalCaller(ctx, orgID, "ListOrgJobs"); err != nil {
			return nil, err
		}
	} else {
		if orgID == "" {
			return nil, huma.Error400BadRequest("org_id is required")
		}
		if callerOrgID != orgID {
			return nil, huma.Error403Forbidden("api key does not belong to this organization")
		}
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
