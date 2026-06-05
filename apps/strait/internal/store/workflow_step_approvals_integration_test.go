//go:build integration

package store_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateWorkflowStepApproval(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-update-approval", domain.StepPending)

	approval := &domain.WorkflowStepApproval{
		ID:                newID(),
		WorkflowRunID:     wfRun.ID,
		WorkflowStepRunID: stepRun.ID,
		Approvers:         []string{"user-a", "user-b"},
		Status:            "pending",
		RequestedAt:       time.Now().UTC().Add(-10 * time.Minute),
	}
	require.NoError(t, q.CreateWorkflowStepApproval(ctx, approval))

	now := time.Now().UTC()
	require.NoError(t, q.UpdateWorkflowStepApproval(ctx, approval.
		ID, "approved",
		"user-a",

		&now, ""))

	got, err := q.GetWorkflowStepApprovalByStepRunID(ctx, stepRun.ID)
	require.NoError(t, err)
	require.Equal(t, "approved",

		got.
			Status)
	require.Equal(t, "user-a",

		got.ApprovedBy,
	)

	// Not found.
	err = q.UpdateWorkflowStepApproval(ctx, newID(), "approved", "user-a", &now, "")
	require.Error(t, err)

}

func TestUpdateWorkflowStepApproval_RejectsStaleDecision(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-update-approval-stale", domain.StepPending)

	approval := &domain.WorkflowStepApproval{
		ID:                newID(),
		WorkflowRunID:     wfRun.ID,
		WorkflowStepRunID: stepRun.ID,
		Approvers:         []string{"user-a", "user-b"},
		Status:            domain.ApprovalStatusPending,
		RequestedAt:       time.Now().UTC().Add(-10 * time.Minute),
	}
	require.NoError(t, q.CreateWorkflowStepApproval(ctx, approval))

	now := time.Now().UTC()
	require.NoError(t, q.UpdateWorkflowStepApproval(ctx, approval.
		ID, domain.
		ApprovalStatusApproved,

		"user-a", &now, "",
	))

	err := q.UpdateWorkflowStepApproval(ctx, approval.ID, domain.ApprovalStatusRejected, "user-b", &now, "late rejection")
	require.True(t, errors.Is(err, store.
		ErrRunConflict,
	))

	got, err := q.GetWorkflowStepApprovalByStepRunID(ctx, stepRun.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		ApprovalStatusApproved,

		got.Status,
	)
	require.Equal(t, "user-a",

		got.ApprovedBy,
	)

}

func TestUpdateWorkflowStepApproval_ConcurrentApprovalHasSingleWinner(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-update-approval-race", domain.StepPending)

	approval := &domain.WorkflowStepApproval{
		ID:                newID(),
		WorkflowRunID:     wfRun.ID,
		WorkflowStepRunID: stepRun.ID,
		Approvers:         []string{"user-a", "user-b"},
		Status:            domain.ApprovalStatusPending,
		RequestedAt:       time.Now().UTC().Add(-10 * time.Minute),
	}
	require.NoError(t, q.CreateWorkflowStepApproval(ctx, approval))

	const contenders = 24
	var successes int64
	var conflicts int64
	var wg conc.WaitGroup
	for i := range contenders {
		i := i
		wg.Go(func() {
			now := time.Now().UTC()
			err := q.UpdateWorkflowStepApproval(ctx, approval.ID, domain.ApprovalStatusApproved, "user-"+newID(), &now, "")
			switch {
			case err == nil:
				atomic.AddInt64(&successes, 1)
			case errors.Is(err, store.ErrRunConflict):
				atomic.AddInt64(&conflicts, 1)
			default:
				assert.Failf(t, "test failure", "contender %d UpdateWorkflowStepApproval() error = %v", i, err)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, successes)
	require.EqualValues(t, contenders-
		1,
		conflicts)

	got, err := q.GetWorkflowStepApprovalByStepRunID(ctx, stepRun.ID)
	require.NoError(t, err)
	require.Equal(t, domain.
		ApprovalStatusApproved,

		got.Status,
	)

}

func TestListApprovalsPastReminderPoint(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-approvals-reminder", domain.StepPending)

	// Create an approval that is past the 50% reminder point.
	requestedAt := time.Now().UTC().Add(-30 * time.Minute)
	expiresAt := time.Now().UTC().Add(10 * time.Minute) // 40min total, 30 elapsed = 75% past.
	approval := &domain.WorkflowStepApproval{
		ID:                newID(),
		WorkflowRunID:     wfRun.ID,
		WorkflowStepRunID: stepRun.ID,
		Approvers:         []string{"user-a"},
		Status:            "pending",
		RequestedAt:       requestedAt,
		ExpiresAt:         &expiresAt,
	}
	require.NoError(t, q.CreateWorkflowStepApproval(ctx, approval))

	approvals, err := q.ListApprovalsPastReminderPoint(ctx)
	require.NoError(t, err)
	require.Len(t, approvals,

		1)
	require.Equal(t, approval.
		ID, approvals[0].ID,
	)

}

func TestGetApprovalStats(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-approval-stats"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: new(projectID),
	})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: new(projectID)})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID:   new(stepJob.ID),
		StepRef: new("approval-stats-step"),
	})
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: new(projectID),
	})
	stepRun := testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{
		Status:  testutil.Ptr(domain.StepPending),
		StepRef: new(step.StepRef),
	})

	now := time.Now().UTC()
	approval := &domain.WorkflowStepApproval{
		ID:                newID(),
		WorkflowRunID:     wfRun.ID,
		WorkflowStepRunID: stepRun.ID,
		Approvers:         []string{"user-a"},
		Status:            "pending",
		RequestedAt:       now.Add(-5 * time.Minute),
	}
	require.NoError(t, q.CreateWorkflowStepApproval(ctx, approval))

	from := now.Add(-1 * time.Hour)
	to := now.Add(1 * time.Hour)
	stats, err := q.GetApprovalStats(ctx, projectID, from, to)
	require.NoError(t, err)
	require.EqualValues(t, 1, stats.
		TotalRequested,
	)
	require.EqualValues(t, 1, stats.
		TotalPending,
	)

	// Empty range.
	emptyStats, err := q.GetApprovalStats(ctx, projectID, now.Add(-2*time.Hour), now.Add(-1*time.Hour))
	require.NoError(t, err)
	require.EqualValues(t, 0, emptyStats.
		TotalRequested,
	)

}

func TestGetStepRunByWorkflowRunAndRef_LookupAndNotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-step-run-by-ref", domain.StepPending)

	got, err := q.GetStepRunByWorkflowRunAndRef(ctx, wfRun.ID, stepRun.StepRef)
	require.NoError(t, err)
	require.Equal(t, stepRun.
		ID, got.
		ID)

	// Not found.
	_, err = q.GetStepRunByWorkflowRunAndRef(ctx, wfRun.ID, "nonexistent-ref")
	require.Error(t, err)

}

func TestGetWorkflowStepApprovalByStepRunID_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	got, err := q.GetWorkflowStepApprovalByStepRunID(ctx, newID())
	require.NoError(t, err)
	require.Nil(t, got)

}

func TestListExpiredWorkflowStepApprovals_Integration(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-expired-approvals", domain.StepPending)

	// Already expired.
	expired := time.Now().UTC().Add(-1 * time.Minute)
	approval := &domain.WorkflowStepApproval{
		ID:                newID(),
		WorkflowRunID:     wfRun.ID,
		WorkflowStepRunID: stepRun.ID,
		Approvers:         []string{"user-a"},
		Status:            "pending",
		RequestedAt:       time.Now().UTC().Add(-10 * time.Minute),
		ExpiresAt:         &expired,
	}
	require.NoError(t, q.CreateWorkflowStepApproval(ctx, approval))

	approvals, err := q.ListExpiredWorkflowStepApprovals(ctx)
	require.NoError(t, err)
	require.Len(t, approvals,

		1)
	require.Equal(t, approval.
		ID, approvals[0].ID,
	)

}

// mustCreateWorkflowStepFixtureApproval helper is intentionally not added;
// we reuse mustCreateWorkflowStepFixture from the main integration test file.
