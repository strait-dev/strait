//go:build integration

package scheduler

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
)

func TestClaimReconciler_RestoresMissingClaimUsingJobQueueName(t *testing.T) {
	ctx := context.Background()
	tdb, err := testutil.SetupTestDB(ctx, "../../migrations")
	if err != nil {
		t.Fatalf("setup db: %v", err)
	}
	t.Cleanup(func() { tdb.Cleanup(ctx) })

	st := store.New(tdb.Pool)
	pq := queue.NewPostgresQueue(tdb.Pool)
	job := &domain.Job{
		ID:            uuid.Must(uuid.NewV7()).String(),
		ProjectID:     "claim-repair-" + uuid.Must(uuid.NewV7()).String(),
		Name:          "claim repair",
		Slug:          "claim-repair-" + uuid.Must(uuid.NewV7()).String()[:8],
		EndpointURL:   "https://example.com/worker",
		MaxAttempts:   3,
		TimeoutSecs:   60,
		Enabled:       true,
		ExecutionMode: domain.ExecutionModeWorker,
		Queue:         "critical-repair",
	}
	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	run := &domain.JobRun{
		ID:            uuid.Must(uuid.NewV7()).String(),
		JobID:         job.ID,
		ProjectID:     job.ProjectID,
		Status:        domain.StatusQueued,
		ExecutionMode: domain.ExecutionModeWorker,
	}
	if err := pq.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if _, err := tdb.Pool.Exec(ctx, `DELETE FROM job_run_queue WHERE run_id = $1`, run.ID); err != nil {
		t.Fatalf("delete claim row: %v", err)
	}
	if _, err := tdb.Pool.Exec(ctx, `UPDATE job_runs SET queue_name = '' WHERE id = $1`, run.ID); err != nil {
		t.Fatalf("clear run queue metadata: %v", err)
	}

	r := NewClaimReconciler(tdb.Pool, 0)
	if err := r.reconcileOnce(ctx); err != nil {
		t.Fatalf("reconcileOnce() error = %v", err)
	}

	var queueName, executionMode string
	if err := tdb.Pool.QueryRow(ctx,
		`SELECT queue_name, execution_mode FROM job_run_queue WHERE run_id = $1`,
		run.ID,
	).Scan(&queueName, &executionMode); err != nil {
		t.Fatalf("query repaired claim: %v", err)
	}
	if queueName != job.Queue {
		t.Fatalf("queue_name = %q, want %q", queueName, job.Queue)
	}
	if executionMode != string(domain.ExecutionModeWorker) {
		t.Fatalf("execution_mode = %q, want %q", executionMode, domain.ExecutionModeWorker)
	}
}
