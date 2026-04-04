//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/testutil"
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
	if err := q.CreateWorkflowStepApproval(ctx, approval); err != nil {
		t.Fatalf("CreateWorkflowStepApproval() error = %v", err)
	}

	now := time.Now().UTC()
	if err := q.UpdateWorkflowStepApproval(ctx, approval.ID, "approved", "user-a", &now, ""); err != nil {
		t.Fatalf("UpdateWorkflowStepApproval() error = %v", err)
	}

	got, err := q.GetWorkflowStepApprovalByStepRunID(ctx, stepRun.ID)
	if err != nil {
		t.Fatalf("GetWorkflowStepApprovalByStepRunID() error = %v", err)
	}
	if got.Status != "approved" {
		t.Fatalf("status = %q, want %q", got.Status, "approved")
	}
	if got.ApprovedBy != "user-a" {
		t.Fatalf("approved_by = %q, want %q", got.ApprovedBy, "user-a")
	}

	// Not found.
	err = q.UpdateWorkflowStepApproval(ctx, newID(), "approved", "user-a", &now, "")
	if err == nil {
		t.Fatal("UpdateWorkflowStepApproval(notfound) expected error, got nil")
	}
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
	if err := q.CreateWorkflowStepApproval(ctx, approval); err != nil {
		t.Fatalf("CreateWorkflowStepApproval() error = %v", err)
	}

	approvals, err := q.ListApprovalsPastReminderPoint(ctx)
	if err != nil {
		t.Fatalf("ListApprovalsPastReminderPoint() error = %v", err)
	}
	if len(approvals) != 1 {
		t.Fatalf("ListApprovalsPastReminderPoint() len = %d, want 1", len(approvals))
	}
	if approvals[0].ID != approval.ID {
		t.Fatalf("ListApprovalsPastReminderPoint() id = %q, want %q", approvals[0].ID, approval.ID)
	}
}

func TestGetApprovalStats(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-approval-stats"
	wf := testutil.MustCreateWorkflow(t, ctx, q, &testutil.WorkflowOpts{
		ProjectID: testutil.Ptr(projectID),
	})
	stepJob := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: testutil.Ptr(projectID)})
	step := testutil.MustCreateWorkflowStep(t, ctx, q, wf.ID, &testutil.WorkflowStepOpts{
		JobID:   testutil.Ptr(stepJob.ID),
		StepRef: testutil.Ptr("approval-stats-step"),
	})
	wfRun := testutil.MustCreateWorkflowRun(t, ctx, q, wf.ID, &testutil.WorkflowRunOpts{
		ProjectID: testutil.Ptr(projectID),
	})
	stepRun := testutil.MustCreateWorkflowStepRun(t, ctx, q, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{
		Status:  testutil.Ptr(domain.StepPending),
		StepRef: testutil.Ptr(step.StepRef),
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
	if err := q.CreateWorkflowStepApproval(ctx, approval); err != nil {
		t.Fatalf("CreateWorkflowStepApproval() error = %v", err)
	}

	from := now.Add(-1 * time.Hour)
	to := now.Add(1 * time.Hour)
	stats, err := q.GetApprovalStats(ctx, projectID, from, to)
	if err != nil {
		t.Fatalf("GetApprovalStats() error = %v", err)
	}
	if stats.TotalRequested != 1 {
		t.Fatalf("TotalRequested = %d, want 1", stats.TotalRequested)
	}
	if stats.TotalPending != 1 {
		t.Fatalf("TotalPending = %d, want 1", stats.TotalPending)
	}

	// Empty range.
	emptyStats, err := q.GetApprovalStats(ctx, projectID, now.Add(-2*time.Hour), now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("GetApprovalStats(empty) error = %v", err)
	}
	if emptyStats.TotalRequested != 0 {
		t.Fatalf("GetApprovalStats(empty) TotalRequested = %d, want 0", emptyStats.TotalRequested)
	}
}

func TestGetStepRunByWorkflowRunAndRef_LookupAndNotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, wfRun, stepRun := mustCreateWorkflowStepFixture(t, ctx, q, "project-step-run-by-ref", domain.StepPending)

	got, err := q.GetStepRunByWorkflowRunAndRef(ctx, wfRun.ID, stepRun.StepRef)
	if err != nil {
		t.Fatalf("GetStepRunByWorkflowRunAndRef() error = %v", err)
	}
	if got.ID != stepRun.ID {
		t.Fatalf("GetStepRunByWorkflowRunAndRef() id = %q, want %q", got.ID, stepRun.ID)
	}

	// Not found.
	_, err = q.GetStepRunByWorkflowRunAndRef(ctx, wfRun.ID, "nonexistent-ref")
	if err == nil {
		t.Fatal("GetStepRunByWorkflowRunAndRef(notfound) expected error, got nil")
	}
}

func TestGetWorkflowStepApprovalByStepRunID_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	got, err := q.GetWorkflowStepApprovalByStepRunID(ctx, newID())
	if err != nil {
		t.Fatalf("GetWorkflowStepApprovalByStepRunID() error = %v", err)
	}
	if got != nil {
		t.Fatalf("GetWorkflowStepApprovalByStepRunID() = %+v, want nil", got)
	}
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
	if err := q.CreateWorkflowStepApproval(ctx, approval); err != nil {
		t.Fatalf("CreateWorkflowStepApproval() error = %v", err)
	}

	approvals, err := q.ListExpiredWorkflowStepApprovals(ctx)
	if err != nil {
		t.Fatalf("ListExpiredWorkflowStepApprovals() error = %v", err)
	}
	if len(approvals) != 1 {
		t.Fatalf("ListExpiredWorkflowStepApprovals() len = %d, want 1", len(approvals))
	}
	if approvals[0].ID != approval.ID {
		t.Fatalf("ListExpiredWorkflowStepApprovals() id = %q, want %q", approvals[0].ID, approval.ID)
	}
}

// mustCreateWorkflowStepFixtureApproval helper is intentionally not added;
// we reuse mustCreateWorkflowStepFixture from the main integration test file.
