//go:build integration

package queue_test

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/testutil"
)

func TestRequeuePausedJobRuns_RemainsClaimableByLegacyDequeues(t *testing.T) {
	ctx := context.Background()

	for _, tt := range []struct {
		name       string
		dequeue    func(context.Context) ([]domain.JobRun, error)
		wantStatus domain.RunStatus
	}{
		{
			name: "scan_dequeue",
			dequeue: func(ctx context.Context) ([]domain.JobRun, error) {
				return mustQueue(t).DequeueN(ctx, 1)
			},
			wantStatus: domain.StatusDequeued,
		},
		{
			name: "claim_table_dequeue",
			dequeue: func(ctx context.Context) ([]domain.JobRun, error) {
				return mustQueue(t).DequeueNClaim(ctx, 1)
			},
			wantStatus: domain.StatusExecuting,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			st := mustStore(t)
			mustClean(t, ctx)

			projectID := "project-requeue-claimable-" + tt.name
			wf := testutil.MustCreateWorkflow(t, ctx, st, &testutil.WorkflowOpts{
				ProjectID: &projectID,
			})
			stepJob := testutil.MustCreateJob(t, ctx, st, &testutil.JobOpts{ProjectID: &projectID})
			step := testutil.MustCreateWorkflowStep(t, ctx, st, wf.ID, &testutil.WorkflowStepOpts{
				JobID: &stepJob.ID,
			})
			wfRun := testutil.MustCreateWorkflowRun(t, ctx, st, wf.ID, &testutil.WorkflowRunOpts{
				ProjectID: &projectID,
			})

			run := &domain.JobRun{
				ID:          newID(),
				JobID:       stepJob.ID,
				ProjectID:   stepJob.ProjectID,
				Status:      domain.StatusExecuting,
				Attempt:     1,
				TriggeredBy: domain.TriggerManual,
			}
			if err := st.CreateRun(ctx, run); err != nil {
				t.Fatalf("CreateRun: %v", err)
			}
			testutil.MustCreateWorkflowStepRun(t, ctx, st, wfRun.ID, step.ID, &testutil.WorkflowStepRunOpts{
				JobRunID: &run.ID,
			})

			paused, err := st.MarkJobRunsPausedByWorkflowRun(ctx, wfRun.ID)
			if err != nil {
				t.Fatalf("MarkJobRunsPausedByWorkflowRun: %v", err)
			}
			if paused != 1 {
				t.Fatalf("paused = %d, want 1", paused)
			}

			requeued, err := st.RequeuePausedJobRuns(ctx, wfRun.ID)
			if err != nil {
				t.Fatalf("RequeuePausedJobRuns: %v", err)
			}
			if requeued != 1 {
				t.Fatalf("requeued = %d, want 1", requeued)
			}

			var ledgerStatus, readStatus domain.RunStatus
			var claimRows int
			if err := testDB.Pool.QueryRow(ctx, `
				SELECT jr.status, rs.status,
				       (SELECT COUNT(*) FROM job_run_queue WHERE run_id = jr.id)
				FROM job_runs jr
				JOIN job_run_read_state rs ON rs.run_id = jr.id
				WHERE jr.id = $1`,
				run.ID,
			).Scan(&ledgerStatus, &readStatus, &claimRows); err != nil {
				t.Fatalf("query resumed run state: %v", err)
			}
			if ledgerStatus != domain.StatusExecuting {
				t.Fatalf("job_runs status = %q, want immutable executing ledger status before claim", ledgerStatus)
			}
			if readStatus != domain.StatusQueued {
				t.Fatalf("read status = %q, want queued", readStatus)
			}
			if claimRows != 1 {
				t.Fatalf("claim rows = %d, want 1", claimRows)
			}

			claimed, err := tt.dequeue(ctx)
			if err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			if len(claimed) != 1 {
				t.Fatalf("%s returned %d runs, want 1", tt.name, len(claimed))
			}
			if claimed[0].ID != run.ID {
				t.Fatalf("%s returned run %q, want %q", tt.name, claimed[0].ID, run.ID)
			}
			if claimed[0].Status != tt.wantStatus {
				t.Fatalf("%s status = %q, want %q", tt.name, claimed[0].Status, tt.wantStatus)
			}
		})
	}
}
