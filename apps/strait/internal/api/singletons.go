package api

import (
	"context"
	"errors"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

// SingletonHolderView is one row of a singleton inspection listing: the run that
// currently holds a resolved key for the owner, plus how many runs are parked
// behind it. Waiters are job runs in "waiting" (jobs) or workflow runs in
// "queued" (workflows) for the same key.
type SingletonHolderView struct {
	LockKey     string     `json:"lock_key"`
	HolderRunID string     `json:"holder_run_id"`
	AcquiredAt  time.Time  `json:"acquired_at"`
	LeaseUntil  *time.Time `json:"lease_until,omitempty"`
	Waiters     int        `json:"waiters"`
}

// singletonHolderViews maps lock rows to inspection views, attaching the live
// waiter count for each held key.
func (s *Server) singletonHolderViews(ctx context.Context, kind domain.SingletonKind, ownerID string, locks []domain.SingletonLock) ([]SingletonHolderView, error) {
	views := make([]SingletonHolderView, 0, len(locks))
	for i := range locks {
		lock := locks[i]
		waiters, err := s.store.CountSingletonWaiters(ctx, kind, ownerID, lock.LockKey)
		if err != nil {
			return nil, err
		}
		views = append(views, SingletonHolderView{
			LockKey:     lock.LockKey,
			HolderRunID: lock.HolderRunID,
			AcquiredAt:  lock.AcquiredAt,
			LeaseUntil:  lock.LeaseUntil,
			Waiters:     waiters,
		})
	}
	return views, nil
}

type ListJobSingletonsInput struct {
	JobID  string `path:"jobID"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}

type ListJobSingletonsOutput struct{ Body PaginatedResponse }

// handleListJobSingletons lists the currently held singleton keys for a job,
// paginated by acquisition time, with the waiter count behind each holder.
func (s *Server) handleListJobSingletons(ctx context.Context, input *ListJobSingletonsInput) (*ListJobSingletonsOutput, error) {
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
	locks, err := s.store.ListSingletonLocksPage(ctx, job.ProjectID, domain.SingletonKindJob, input.JobID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list singleton holders")
	}
	views, err := s.singletonHolderViews(ctx, domain.SingletonKindJob, input.JobID, locks)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to count singleton waiters")
	}
	return &ListJobSingletonsOutput{Body: paginatedResult(views, limit, func(v SingletonHolderView) string {
		return v.AcquiredAt.Format(time.RFC3339Nano)
	})}, nil
}

type ListWorkflowSingletonsInput struct {
	WorkflowID string `path:"workflowID"`
	Limit      string `query:"limit"`
	Cursor     string `query:"cursor"`
}

type ListWorkflowSingletonsOutput struct{ Body PaginatedResponse }

// handleListWorkflowSingletons lists the currently held singleton keys for a
// workflow, paginated by acquisition time, with the waiter count behind each
// holder.
func (s *Server) handleListWorkflowSingletons(ctx context.Context, input *ListWorkflowSingletonsInput) (*ListWorkflowSingletonsOutput, error) {
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	wf, err := s.store.GetWorkflow(ctx, input.WorkflowID)
	if err != nil {
		if errors.Is(err, store.ErrWorkflowNotFound) {
			return nil, huma.Error404NotFound("workflow not found")
		}
		return nil, huma.Error500InternalServerError("failed to get workflow")
	}
	if err := requireProjectMatch(ctx, wf.ProjectID); err != nil {
		return nil, huma.Error404NotFound("workflow not found")
	}
	locks, err := s.store.ListSingletonLocksPage(ctx, wf.ProjectID, domain.SingletonKindWorkflow, input.WorkflowID, limit+1, cursor)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list singleton holders")
	}
	views, err := s.singletonHolderViews(ctx, domain.SingletonKindWorkflow, input.WorkflowID, locks)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to count singleton waiters")
	}
	return &ListWorkflowSingletonsOutput{Body: paginatedResult(views, limit, func(v SingletonHolderView) string {
		return v.AcquiredAt.Format(time.RFC3339Nano)
	})}, nil
}
