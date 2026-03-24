//go:build integration

package queue_test

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/queue"
)

func TestDequeue_SkipsPausedJob(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-pause-dequeue")

	// Pause the job via raw SQL.
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET paused = true, paused_at = NOW(), pause_reason = 'test' WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("pause job: %v", err)
	}

	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Priority:  1,
	}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	got, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil run (paused job should be skipped), got run %s", got.ID)
	}
}

func TestDequeueN_SkipsPausedJobs(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	pausedJob := mustCreateJob(t, ctx, st, "project-pause-dequeue-n")
	activeJob := mustCreateJob(t, ctx, st, "project-pause-dequeue-n")

	// Pause only the first job.
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET paused = true, paused_at = NOW(), pause_reason = 'test' WHERE id = $1`, pausedJob.ID); err != nil {
		t.Fatalf("pause job: %v", err)
	}

	for _, j := range []*domain.Job{pausedJob, activeJob} {
		run := &domain.JobRun{
			ID:        newID(),
			JobID:     j.ID,
			ProjectID: j.ProjectID,
			Priority:  1,
		}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	runs, err := q.DequeueN(ctx, 2)
	if err != nil {
		t.Fatalf("DequeueN() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run (paused job skipped), got %d", len(runs))
	}
	if runs[0].JobID != activeJob.ID {
		t.Fatalf("expected run from active job %s, got %s", activeJob.ID, runs[0].JobID)
	}
}

func TestDequeueFairN_SkipsPausedJobs(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	pausedJob := mustCreateJob(t, ctx, st, "project-pause-fair")
	activeJob := mustCreateJob(t, ctx, st, "project-pause-fair")

	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET paused = true, paused_at = NOW(), pause_reason = 'test' WHERE id = $1`, pausedJob.ID); err != nil {
		t.Fatalf("pause job: %v", err)
	}

	for _, j := range []*domain.Job{pausedJob, activeJob} {
		run := &domain.JobRun{
			ID:        newID(),
			JobID:     j.ID,
			ProjectID: j.ProjectID,
			Priority:  1,
		}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	runs, err := q.DequeueNFair(ctx, 2)
	if err != nil {
		t.Fatalf("DequeueNFair() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].JobID != activeJob.ID {
		t.Fatalf("expected run from active job %s, got %s", activeJob.ID, runs[0].JobID)
	}
}

func TestDequeueByProject_SkipsPausedJobs(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-pause-by-project"
	pausedJob := mustCreateJob(t, ctx, st, projectID)
	activeJob := mustCreateJob(t, ctx, st, projectID)

	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET paused = true, paused_at = NOW(), pause_reason = 'test' WHERE id = $1`, pausedJob.ID); err != nil {
		t.Fatalf("pause job: %v", err)
	}

	for _, j := range []*domain.Job{pausedJob, activeJob} {
		run := &domain.JobRun{
			ID:        newID(),
			JobID:     j.ID,
			ProjectID: j.ProjectID,
			Priority:  1,
		}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	runs, err := q.DequeueNByProject(ctx, 2, projectID)
	if err != nil {
		t.Fatalf("DequeueNByProject() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].JobID != activeJob.ID {
		t.Fatalf("expected run from active job %s, got %s", activeJob.ID, runs[0].JobID)
	}
}

func TestDequeue_PausedJobDoesNotAffectExecutingRuns(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-pause-executing")

	// Enqueue two runs.
	run1 := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 1}
	run2 := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 1}
	if err := q.Enqueue(ctx, run1); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if err := q.Enqueue(ctx, run2); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	// Dequeue first run (simulates it starting execution).
	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if dequeued == nil {
		t.Fatal("expected a dequeued run")
	}

	// Now pause the job.
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET paused = true, paused_at = NOW(), pause_reason = 'test' WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("pause job: %v", err)
	}

	// The first run should still be in dequeued state (not cancelled).
	dequeuedRun, err := st.GetRun(ctx, dequeued.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if dequeuedRun.Status != domain.StatusDequeued {
		t.Fatalf("expected dequeued run to remain in %s status, got %s", domain.StatusDequeued, dequeuedRun.Status)
	}

	// The second run should NOT be dequeued because the job is paused.
	got, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil (second run should not dequeue while paused), got run %s", got.ID)
	}
}

func TestDequeue_ResumeAllowsImmediateDequeue(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-pause-resume")

	// Pause the job.
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET paused = true, paused_at = NOW(), pause_reason = 'test' WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("pause job: %v", err)
	}

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 1}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	// Dequeue should fail while paused.
	got, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil while paused, got run %s", got.ID)
	}

	// Resume the job.
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET paused = false, paused_at = NULL, pause_reason = NULL WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("resume job: %v", err)
	}

	// Dequeue should succeed immediately.
	got, err = q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() after resume error = %v", err)
	}
	if got == nil {
		t.Fatal("expected run after resume, got nil")
	}
	if got.ID != run.ID {
		t.Fatalf("expected run %s, got %s", run.ID, got.ID)
	}
}

func TestPauseResumeLifecycle(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	// 1. Create job.
	job := mustCreateJob(t, ctx, st, "project-lifecycle")

	// 2. Pause the job.
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET paused = true, paused_at = NOW(), pause_reason = 'lifecycle test' WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("pause job: %v", err)
	}

	// 3. Enqueue a run (simulating a trigger while paused).
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 1}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	// 4. Attempt dequeue -- should return nil.
	got, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil (job paused), got run %s", got.ID)
	}

	// 5. Resume the job.
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET paused = false, paused_at = NULL, pause_reason = NULL WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("resume job: %v", err)
	}

	// 6. Dequeue -- should return the run.
	got, err = q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() after resume error = %v", err)
	}
	if got == nil {
		t.Fatal("expected run after resume, got nil")
	}
	if got.ID != run.ID {
		t.Fatalf("expected run %s, got %s", run.ID, got.ID)
	}
}

func TestPausedJob_ManualTriggersStillQueue(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-manual-trigger-pause")

	// Pause the job.
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET paused = true, paused_at = NOW(), pause_reason = 'manual trigger test' WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("pause job: %v", err)
	}

	// Manual API triggers should still be accepted (enqueue succeeds).
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 1}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() should succeed even when paused, error = %v", err)
	}

	// But the run should not be dequeued.
	got, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if got != nil {
		t.Fatal("expected nil (paused job), but a run was dequeued")
	}

	// Verify the run is still queued in the DB.
	stored, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if stored.Status != domain.StatusQueued {
		t.Fatalf("expected run to remain queued, got %s", stored.Status)
	}

	// Resume and dequeue.
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET paused = false, paused_at = NULL, pause_reason = NULL WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("resume job: %v", err)
	}

	got, err = q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() after resume error = %v", err)
	}
	if got == nil {
		t.Fatal("expected run after resume")
	}
	if got.ID != run.ID {
		t.Fatalf("expected run %s, got %s", run.ID, got.ID)
	}
}

func TestPausedJob_CronExcludedFromListCronJobs(t *testing.T) {
	ctx := context.Background()
	_ = mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-cron-excluded")
	// mustCreateJob doesn't set a cron expression; add one so ListCronJobs returns it.
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET cron = '*/5 * * * *' WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("set cron: %v", err)
	}

	// Verify cron job is listed before pause.
	cronJobs, err := st.ListCronJobs(ctx)
	if err != nil {
		t.Fatalf("ListCronJobs() error = %v", err)
	}
	var foundBefore bool
	for _, j := range cronJobs {
		if j.ID == job.ID {
			foundBefore = true
			break
		}
	}
	if !foundBefore {
		t.Fatal("cron job should appear before pausing")
	}

	// Pause -- cron scheduler should no longer see this job.
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET paused = true, paused_at = NOW(), pause_reason = 'cron test' WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("pause job: %v", err)
	}

	cronJobs, err = st.ListCronJobs(ctx)
	if err != nil {
		t.Fatalf("ListCronJobs() error = %v", err)
	}
	for _, j := range cronJobs {
		if j.ID == job.ID {
			t.Fatal("paused cron job should NOT appear in ListCronJobs")
		}
	}
}

func TestPauseResume_NoStaleRunsAfterLongPause(t *testing.T) {
	ctx := context.Background()
	q := mustQueue(t)
	st := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, st, "project-no-stale")
	// mustCreateJob doesn't set a cron expression; add one so ListCronJobs returns it.
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET cron = '*/5 * * * *' WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("set cron: %v", err)
	}

	// Pause the job.
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET paused = true, paused_at = NOW(), pause_reason = 'long pause' WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("pause job: %v", err)
	}

	// Since ListCronJobs excludes paused jobs, the cron scheduler won't
	// queue any runs. Verify no runs exist for this job.
	cronJobs, err := st.ListCronJobs(ctx)
	if err != nil {
		t.Fatalf("ListCronJobs() error = %v", err)
	}
	for _, j := range cronJobs {
		if j.ID == job.ID {
			t.Fatal("paused job should not be in cron list")
		}
	}

	// Resume the job.
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET paused = false, paused_at = NULL, pause_reason = NULL WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("resume job: %v", err)
	}

	// After resume, ListCronJobs should return the job again.
	cronJobs, err = st.ListCronJobs(ctx)
	if err != nil {
		t.Fatalf("ListCronJobs() after resume error = %v", err)
	}
	var found bool
	for _, j := range cronJobs {
		if j.ID == job.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("resumed job should reappear in cron list")
	}

	// The cron scheduler would now queue the next run. Enqueue and verify dequeue works.
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 1}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	got, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if got == nil || got.ID != run.ID {
		t.Fatal("expected the freshly queued run to dequeue after resume")
	}
}

// Verify the queue type is accessible (compile-time check).
var _ interface {
	Dequeue(ctx context.Context) (*domain.JobRun, error)
	DequeueN(ctx context.Context, n int) ([]domain.JobRun, error)
	DequeueNFair(ctx context.Context, n int) ([]domain.JobRun, error)
	DequeueNByProject(ctx context.Context, n int, projectID string) ([]domain.JobRun, error)
} = (*queue.PostgresQueue)(nil)
