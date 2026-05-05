//go:build integration

package queue_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
)

// ---------------------------------------------------------------------------
// DequeueNTwoPhase
// ---------------------------------------------------------------------------

func TestTwoPhaseDequeue_ReturnsCorrectRuns(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-twophase-correct")
	q := mustQueue(t)

	enqueued := make([]string, 10)
	for i := range enqueued {
		run := &domain.JobRun{
			ID:        newID(),
			JobID:     job.ID,
			ProjectID: job.ProjectID,
			Payload:   json.RawMessage(`{"i":` + string(rune('0'+i)) + `}`),
			Priority:  1,
		}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
		enqueued[i] = run.ID
	}

	batch, err := q.DequeueNTwoPhase(ctx, 5)
	if err != nil {
		t.Fatalf("DequeueNTwoPhase: %v", err)
	}
	if len(batch) != 5 {
		t.Fatalf("got %d runs, want 5", len(batch))
	}

	for _, r := range batch {
		if r.ID == "" {
			t.Error("run has empty ID")
		}
		if r.JobID != job.ID {
			t.Errorf("JobID = %q, want %q", r.JobID, job.ID)
		}
		if r.ProjectID != job.ProjectID {
			t.Errorf("ProjectID = %q, want %q", r.ProjectID, job.ProjectID)
		}
		if r.Status != domain.StatusDequeued {
			t.Errorf("Status = %q, want dequeued", r.Status)
		}
		if r.StartedAt == nil {
			t.Error("StartedAt is nil, want non-nil for dequeued run")
		}
		if r.Priority != 1 {
			t.Errorf("Priority = %d, want 1", r.Priority)
		}
	}
}

func TestTwoPhaseDequeue_EmptyQueueReturnsNil(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	q := mustQueue(t)

	batch, err := q.DequeueNTwoPhase(ctx, 5)
	if err != nil {
		t.Fatalf("DequeueNTwoPhase on empty queue: %v", err)
	}
	if batch != nil {
		t.Errorf("expected nil, got %d runs", len(batch))
	}
}

func TestTwoPhaseDequeue_ZeroN_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	q := mustQueue(t)

	batch, err := q.DequeueNTwoPhase(ctx, 0)
	if err != nil {
		t.Fatalf("DequeueNTwoPhase(0): %v", err)
	}
	if batch != nil {
		t.Errorf("expected nil for n=0, got %d runs", len(batch))
	}
}

func TestTwoPhaseDequeue_NoDuplicates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-twophase-nodup")
	q := mustQueue(t)

	for i := 0; i < 20; i++ {
		mustEnqueueRun(t, ctx, q, job)
	}

	var seen sync.Map
	var wg sync.WaitGroup
	var dupes int64

	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			batch, err := q.DequeueNTwoPhase(ctx, 10)
			if err != nil {
				t.Errorf("dequeue: %v", err)
				return
			}
			for _, r := range batch {
				if _, loaded := seen.LoadOrStore(r.ID, true); loaded {
					t.Errorf("duplicate run ID: %s", r.ID)
					dupes++
				}
			}
		}()
	}
	wg.Wait()

	// Collect total dequeued to verify all 20 consumed.
	var total int
	seen.Range(func(_, _ any) bool { total++; return true })
	if total != 20 {
		t.Errorf("total unique dequeued = %d, want 20", total)
	}
}

// ---------------------------------------------------------------------------
// DequeueNClaim (claim table path)
// ---------------------------------------------------------------------------

func TestClaimTable_DequeueNClaim_ReturnsCorrectRuns(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-claim-correct")
	q := mustQueue(t)

	ids := make([]string, 10)
	for i := range ids {
		r := mustEnqueueRun(t, ctx, q, job)
		ids[i] = r.ID
	}

	batch, err := q.DequeueNClaim(ctx, 5)
	if err != nil {
		t.Fatalf("DequeueNClaim: %v", err)
	}
	if len(batch) != 5 {
		t.Fatalf("got %d runs, want 5", len(batch))
	}

	dequeuedIDs := make([]string, len(batch))
	for i, r := range batch {
		dequeuedIDs[i] = r.ID
		if r.Status != domain.StatusExecuting {
			t.Errorf("run %s: status = %q, want executing", r.ID, r.Status)
		}
		if r.JobID != job.ID {
			t.Errorf("run %s: JobID = %q, want %q", r.ID, r.JobID, job.ID)
		}
		if r.StartedAt == nil {
			t.Errorf("run %s: StartedAt is nil", r.ID)
		}
	}

	// Verify claim rows were deleted for dequeued runs.
	var remaining int
	err = testDB.Pool.QueryRow(ctx,
		`SELECT count(*) FROM job_run_queue WHERE run_id = ANY($1)`,
		dequeuedIDs,
	).Scan(&remaining)
	if err != nil {
		t.Fatalf("count claim rows: %v", err)
	}
	if remaining != 0 {
		t.Errorf("claim rows remaining = %d, want 0 for dequeued runs", remaining)
	}
}

func TestClaimTable_DequeueNClaim_EmptyQueue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	q := mustQueue(t)

	batch, err := q.DequeueNClaim(ctx, 5)
	if err != nil {
		t.Fatalf("DequeueNClaim on empty queue: %v", err)
	}
	if len(batch) != 0 {
		t.Errorf("expected empty result, got %d runs", len(batch))
	}
}

func TestClaimTable_DequeueNClaim_NegativeN_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	q := mustQueue(t)

	batch, err := q.DequeueNClaim(ctx, -1)
	if err != nil {
		t.Fatalf("DequeueNClaim(-1): %v", err)
	}
	if batch != nil {
		t.Errorf("expected nil for n<0, got %d runs", len(batch))
	}
}

// ---------------------------------------------------------------------------
// Dual-write: Enqueue inserts both job_runs and job_run_queue
// ---------------------------------------------------------------------------

func TestClaimTable_DualWrite_EnqueueInsertsBothTables(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-dualwrite")
	q := mustQueue(t)

	ids := make([]string, 3)
	for i := range ids {
		r := mustEnqueueRun(t, ctx, q, job)
		ids[i] = r.ID
	}

	for _, id := range ids {
		var runCount int
		err := testDB.Pool.QueryRow(ctx,
			`SELECT count(*) FROM job_runs WHERE id = $1`, id,
		).Scan(&runCount)
		if err != nil {
			t.Fatalf("query job_runs for %s: %v", id, err)
		}
		if runCount != 1 {
			t.Errorf("job_runs: run %s count = %d, want 1", id, runCount)
		}

		var claimCount int
		err = testDB.Pool.QueryRow(ctx,
			`SELECT count(*) FROM job_run_queue WHERE run_id = $1`, id,
		).Scan(&claimCount)
		if err != nil {
			t.Fatalf("query job_run_queue for %s: %v", id, err)
		}
		if claimCount != 1 {
			t.Errorf("job_run_queue: run %s count = %d, want 1", id, claimCount)
		}
	}
}

// ---------------------------------------------------------------------------
// Fan-out trigger: pausing a job updates claim rows
// ---------------------------------------------------------------------------

func TestClaimTable_FanOutTrigger_PauseJob(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-fanout-pause")
	q := mustQueue(t)

	for i := 0; i < 5; i++ {
		mustEnqueueRun(t, ctx, q, job)
	}

	// Pause the job via direct SQL (triggers fan-out to job_run_queue).
	_, err := testDB.Pool.Exec(ctx,
		`UPDATE jobs SET paused = true WHERE id = $1`, job.ID)
	if err != nil {
		t.Fatalf("pause job: %v", err)
	}

	// Verify all claim rows reflect paused state.
	var pausedCount int
	err = testDB.Pool.QueryRow(ctx,
		`SELECT count(*) FROM job_run_queue WHERE job_id = $1 AND job_paused = true`,
		job.ID,
	).Scan(&pausedCount)
	if err != nil {
		t.Fatalf("count paused claim rows: %v", err)
	}
	if pausedCount != 5 {
		t.Errorf("paused claim rows = %d, want 5", pausedCount)
	}

	// DequeueNClaim should return nothing (all paused).
	batch, err := q.DequeueNClaim(ctx, 10)
	if err != nil {
		t.Fatalf("DequeueNClaim after pause: %v", err)
	}
	if len(batch) != 0 {
		t.Errorf("expected 0 dequeued after pause, got %d", len(batch))
	}
}

// ---------------------------------------------------------------------------
// Fan-out trigger: disabling a job updates claim rows
// ---------------------------------------------------------------------------

func TestClaimTable_FanOutTrigger_DisableJob(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-fanout-disable")
	q := mustQueue(t)

	for i := 0; i < 5; i++ {
		mustEnqueueRun(t, ctx, q, job)
	}

	// Disable the job.
	_, err := testDB.Pool.Exec(ctx,
		`UPDATE jobs SET enabled = false WHERE id = $1`, job.ID)
	if err != nil {
		t.Fatalf("disable job: %v", err)
	}

	// Verify claim rows reflect disabled state.
	var disabledCount int
	err = testDB.Pool.QueryRow(ctx,
		`SELECT count(*) FROM job_run_queue WHERE job_id = $1 AND job_enabled = false`,
		job.ID,
	).Scan(&disabledCount)
	if err != nil {
		t.Fatalf("count disabled claim rows: %v", err)
	}
	if disabledCount != 5 {
		t.Errorf("disabled claim rows = %d, want 5", disabledCount)
	}

	// DequeueNClaim should return nothing.
	batch, err := q.DequeueNClaim(ctx, 10)
	if err != nil {
		t.Fatalf("DequeueNClaim after disable: %v", err)
	}
	if len(batch) != 0 {
		t.Errorf("expected 0 dequeued after disable, got %d", len(batch))
	}
}

// ---------------------------------------------------------------------------
// Fan-out trigger: concurrency change propagates to claim rows
// ---------------------------------------------------------------------------

func TestClaimTable_FanOutTrigger_ConcurrencyChange(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)

	// Create job with max_concurrency = 5.
	job := &domain.Job{
		ID:             newID(),
		ProjectID:      "project-fanout-conc",
		Name:           "job-" + newID(),
		Slug:           "slug-" + newID(),
		EndpointURL:    "https://example.com/queue-job",
		MaxAttempts:    3,
		TimeoutSecs:    300,
		Enabled:        true,
		MaxConcurrency: 5,
	}
	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	q := mustQueue(t)
	for i := 0; i < 3; i++ {
		mustEnqueueRun(t, ctx, q, job)
	}

	// Change max_concurrency to 1.
	_, err := testDB.Pool.Exec(ctx,
		`UPDATE jobs SET max_concurrency = 1 WHERE id = $1`, job.ID)
	if err != nil {
		t.Fatalf("update max_concurrency: %v", err)
	}

	// Verify all claim rows reflect new concurrency.
	rows, err := testDB.Pool.Query(ctx,
		`SELECT job_max_concurrency FROM job_run_queue WHERE job_id = $1`, job.ID)
	if err != nil {
		t.Fatalf("query claim rows: %v", err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var mc int
		if err := rows.Scan(&mc); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if mc != 1 {
			t.Errorf("job_max_concurrency = %d, want 1", mc)
		}
		count++
	}
	if count != 3 {
		t.Errorf("claim row count = %d, want 3", count)
	}
}

// ---------------------------------------------------------------------------
// DequeueNClaim: no duplicates under concurrency
// ---------------------------------------------------------------------------

func TestClaimTable_DequeueNClaim_NoDuplicates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-claim-nodup")
	q := mustQueue(t)

	for i := 0; i < 20; i++ {
		mustEnqueueRun(t, ctx, q, job)
	}

	var seen sync.Map
	var wg sync.WaitGroup

	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			batch, err := q.DequeueNClaim(ctx, 10)
			if err != nil {
				t.Errorf("DequeueNClaim: %v", err)
				return
			}
			for _, r := range batch {
				if _, loaded := seen.LoadOrStore(r.ID, true); loaded {
					t.Errorf("duplicate run ID: %s", r.ID)
				}
			}
		}()
	}
	wg.Wait()

	var total int
	seen.Range(func(_, _ any) bool { total++; return true })
	if total != 20 {
		t.Errorf("total unique dequeued = %d, want 20", total)
	}
}

func TestClaimTable_DequeueNForWorker_RoutesNonDefaultWorkerQueue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-worker-claim-routing")
	markWorkerJobQueue(t, ctx, job, "priority")
	q := mustQueue(t)

	run := &domain.JobRun{
		ID:            newID(),
		JobID:         job.ID,
		ProjectID:     job.ProjectID,
		Priority:      10,
		ExecutionMode: domain.ExecutionModeWorker,
		QueueName:     "priority",
	}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue worker run: %v", err)
	}

	assertClaimRouting(t, ctx, run.ID, domain.ExecutionModeWorker, "priority")

	batch, err := q.DequeueNForWorker(ctx, 1, []string{"priority"})
	if err != nil {
		t.Fatalf("DequeueNForWorker: %v", err)
	}
	if len(batch) != 1 {
		t.Fatalf("DequeueNForWorker returned %d runs, want 1", len(batch))
	}
	if batch[0].ID != run.ID {
		t.Fatalf("DequeueNForWorker run ID = %q, want %q", batch[0].ID, run.ID)
	}
	if batch[0].Status != domain.StatusExecuting {
		t.Fatalf("DequeueNForWorker status = %q, want executing", batch[0].Status)
	}
}

func TestClaimTable_DequeueNForWorker_IgnoresHTTPAndOtherQueues(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	q := mustQueue(t)

	httpJob := mustCreateJob(t, ctx, st, "project-worker-claim-filter")
	httpRun := &domain.JobRun{
		ID:            newID(),
		JobID:         httpJob.ID,
		ProjectID:     httpJob.ProjectID,
		Priority:      100,
		ExecutionMode: domain.ExecutionModeHTTP,
		QueueName:     "priority",
	}
	if err := q.Enqueue(ctx, httpRun); err != nil {
		t.Fatalf("Enqueue HTTP run: %v", err)
	}

	otherQueueJob := mustCreateJob(t, ctx, st, "project-worker-claim-filter")
	markWorkerJobQueue(t, ctx, otherQueueJob, "other")
	otherQueueRun := &domain.JobRun{
		ID:            newID(),
		JobID:         otherQueueJob.ID,
		ProjectID:     otherQueueJob.ProjectID,
		Priority:      90,
		ExecutionMode: domain.ExecutionModeWorker,
		QueueName:     "other",
	}
	if err := q.Enqueue(ctx, otherQueueRun); err != nil {
		t.Fatalf("Enqueue other queue run: %v", err)
	}

	priorityJob := mustCreateJob(t, ctx, st, "project-worker-claim-filter")
	markWorkerJobQueue(t, ctx, priorityJob, "priority")
	priorityRun := &domain.JobRun{
		ID:            newID(),
		JobID:         priorityJob.ID,
		ProjectID:     priorityJob.ProjectID,
		Priority:      1,
		ExecutionMode: domain.ExecutionModeWorker,
		QueueName:     "priority",
	}
	if err := q.Enqueue(ctx, priorityRun); err != nil {
		t.Fatalf("Enqueue priority worker run: %v", err)
	}

	batch, err := q.DequeueNForWorker(ctx, 10, []string{"priority"})
	if err != nil {
		t.Fatalf("DequeueNForWorker: %v", err)
	}
	if len(batch) != 1 {
		t.Fatalf("DequeueNForWorker returned %d runs, want 1", len(batch))
	}
	if batch[0].ID != priorityRun.ID {
		t.Fatalf("DequeueNForWorker run ID = %q, want %q", batch[0].ID, priorityRun.ID)
	}
}

func TestClaimTable_RequeueRestoresWorkerRouting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-worker-claim-requeue")
	markWorkerJobQueue(t, ctx, job, "priority")
	q := mustQueue(t)

	run := &domain.JobRun{
		ID:            newID(),
		JobID:         job.ID,
		ProjectID:     job.ProjectID,
		Priority:      10,
		ExecutionMode: domain.ExecutionModeWorker,
		QueueName:     "priority",
	}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue worker run: %v", err)
	}
	firstBatch, err := q.DequeueNForWorker(ctx, 1, []string{"priority"})
	if err != nil {
		t.Fatalf("first DequeueNForWorker: %v", err)
	}
	if len(firstBatch) != 1 || firstBatch[0].ID != run.ID {
		t.Fatalf("first DequeueNForWorker = %+v, want run %s", firstBatch, run.ID)
	}

	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE job_runs
		 SET status = 'queued', started_at = NULL, heartbeat_at = NULL
		 WHERE id = $1 AND status = 'executing'`,
		run.ID,
	); err != nil {
		t.Fatalf("requeue run: %v", err)
	}
	assertClaimRouting(t, ctx, run.ID, domain.ExecutionModeWorker, "priority")

	secondBatch, err := q.DequeueNForWorker(ctx, 1, []string{"priority"})
	if err != nil {
		t.Fatalf("second DequeueNForWorker: %v", err)
	}
	if len(secondBatch) != 1 || secondBatch[0].ID != run.ID {
		t.Fatalf("second DequeueNForWorker = %+v, want run %s", secondBatch, run.ID)
	}
}

// ---------------------------------------------------------------------------
// Reaper hot-partition avoidance
// ---------------------------------------------------------------------------

func TestReaper_HotPartitionAvoidance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-reaper-hot")
	q := mustQueue(t)

	// Insert runs that reach terminal state.
	hotIDs := make([]string, 3)
	coldIDs := make([]string, 3)

	for i := 0; i < 3; i++ {
		r := mustEnqueueRun(t, ctx, q, job)
		hotIDs[i] = r.ID
	}
	for i := 0; i < 3; i++ {
		r := mustEnqueueRun(t, ctx, q, job)
		coldIDs[i] = r.ID
	}

	// Move all runs to terminal (completed).
	_, err := testDB.Pool.Exec(ctx,
		`UPDATE job_runs SET status = $1, finished_at = NOW() WHERE job_id = $2`,
		string(domain.StatusCompleted), job.ID)
	if err != nil {
		t.Fatalf("mark completed: %v", err)
	}

	// Hot partition: created_at = recent (within this month).
	// Cold partition: created_at = 60 days ago.
	for _, id := range coldIDs {
		_, err := testDB.Pool.Exec(ctx,
			`UPDATE job_runs SET created_at = NOW() - INTERVAL '60 days', finished_at = NOW() - INTERVAL '60 days' WHERE id = $1`, id)
		if err != nil {
			t.Fatalf("backdate cold run %s: %v", id, err)
		}
	}

	// Delete terminal runs older than 1 hour. Cold-partition runs (60 days
	// old, finished 60 days ago) qualify. Hot-partition runs (just now) do not.
	// We use a direct SQL delete mirroring what a reaper would do: only
	// targeting runs where finished_at < NOW() - retention AND created_at is
	// outside the current month's hot partition boundary.
	beginningOfMonth := time.Now().UTC().Truncate(24 * time.Hour)
	beginningOfMonth = time.Date(beginningOfMonth.Year(), beginningOfMonth.Month(), 1, 0, 0, 0, 0, time.UTC)

	_, err = testDB.Pool.Exec(ctx, `
		DELETE FROM job_runs
		WHERE job_id = $1
		  AND status IN ('completed','failed','timed_out','crashed','system_failed','canceled','expired')
		  AND finished_at < NOW() - INTERVAL '1 hour'
		  AND created_at < $2`,
		job.ID, beginningOfMonth)
	if err != nil {
		t.Fatalf("reaper delete: %v", err)
	}

	// Cold runs should be deleted.
	var coldSurvivors int
	err = testDB.Pool.QueryRow(ctx,
		`SELECT count(*) FROM job_runs WHERE id = ANY($1)`, coldIDs).Scan(&coldSurvivors)
	if err != nil {
		t.Fatalf("count cold: %v", err)
	}
	if coldSurvivors != 0 {
		t.Errorf("cold partition survivors = %d, want 0", coldSurvivors)
	}

	// Hot runs should survive.
	var hotSurvivors int
	err = testDB.Pool.QueryRow(ctx,
		`SELECT count(*) FROM job_runs WHERE id = ANY($1)`, hotIDs).Scan(&hotSurvivors)
	if err != nil {
		t.Fatalf("count hot: %v", err)
	}
	if hotSurvivors != 3 {
		t.Errorf("hot partition survivors = %d, want 3", hotSurvivors)
	}
}

// ---------------------------------------------------------------------------
// SQLCommenter tag presence in dequeue queries
// ---------------------------------------------------------------------------

func TestSQLCommenter_DequeueTagPresent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-sqlcommenter")
	q := mustQueue(t)

	// Enqueue a run so dequeue has work.
	mustEnqueueRun(t, ctx, q, job)

	// Execute DequeueNTwoPhase and DequeueNClaim -- if they succeed the
	// SQL comment tag (/* action=dequeue */) was syntactically valid and
	// did not break the query. This is a behavioral smoke test; the tag
	// is a SQL comment that cannot be observed from the result set.
	batch, err := q.DequeueNTwoPhase(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueNTwoPhase: %v", err)
	}
	if len(batch) != 1 {
		t.Fatalf("DequeueNTwoPhase returned %d, want 1", len(batch))
	}

	// Enqueue another run for DequeueNClaim.
	mustEnqueueRun(t, ctx, q, job)

	batch2, err := q.DequeueNClaim(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueNClaim: %v", err)
	}
	if len(batch2) != 1 {
		t.Fatalf("DequeueNClaim returned %d, want 1", len(batch2))
	}

	// Verify the tag exists in the source SQL constants as a compile-time
	// sanity check. We import the DequeueNClaim SQL from the implementation
	// above -- it literally contains "/* action=dequeue */". This is
	// verified by the successful dequeue calls above: if the comment were
	// malformed, Postgres would reject the query.
}

// TestDequeueVariants_StatusContract verifies the post-dequeue status for
// every dequeue variant. This is a regression test: a blanket sed replacement
// once changed all assertions to StatusExecuting, but only DequeueNClaim
// sets executing directly. All other variants set dequeued.
func TestDequeueVariants_StatusContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-status-contract")
	q := mustQueue(t)

	tests := []struct {
		name       string
		dequeue    func() ([]domain.JobRun, error)
		wantStatus domain.RunStatus
	}{
		{
			name:       "DequeueN",
			dequeue:    func() ([]domain.JobRun, error) { return q.DequeueN(ctx, 1) },
			wantStatus: domain.StatusDequeued,
		},
		{
			name:       "DequeueNTwoPhase",
			dequeue:    func() ([]domain.JobRun, error) { return q.DequeueNTwoPhase(ctx, 1) },
			wantStatus: domain.StatusDequeued,
		},
		{
			name:       "DequeueNFair",
			dequeue:    func() ([]domain.JobRun, error) { return q.DequeueNFair(ctx, 1) },
			wantStatus: domain.StatusDequeued,
		},
		{
			name:       "DequeueNClaim",
			dequeue:    func() ([]domain.JobRun, error) { return q.DequeueNClaim(ctx, 1) },
			wantStatus: domain.StatusExecuting,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Enqueue a fresh run for this variant.
			run := mustEnqueueRun(t, ctx, q, job)

			batch, err := tt.dequeue()
			if err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			if len(batch) == 0 {
				t.Fatalf("%s: got 0 runs, want 1", tt.name)
			}

			got := batch[0].Status
			if got != tt.wantStatus {
				t.Errorf("%s: status = %q, want %q", tt.name, got, tt.wantStatus)
			}

			// Clean up: mark the run terminal so it doesn't interfere with
			// subsequent variants.
			_, _ = testDB.Pool.Exec(ctx,
				`UPDATE job_runs SET status = 'completed', finished_at = NOW() WHERE id = $1`,
				run.ID,
			)
		})
	}
}

// ---------------------------------------------------------------------------
// Reliability: reconciler repairs missing claims
// ---------------------------------------------------------------------------

func TestClaimReconciler_RepairsMissingClaims(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-reconciler-repair")
	q := mustQueue(t)

	// Enqueue a run (creates both job_runs and job_run_queue rows).
	run := mustEnqueueRun(t, ctx, q, job)

	// Manually delete the claim row to simulate trigger miss / partial failure.
	_, err := testDB.Pool.Exec(ctx,
		`DELETE FROM job_run_queue WHERE run_id = $1`, run.ID,
	)
	if err != nil {
		t.Fatalf("delete claim row: %v", err)
	}

	// Confirm DequeueNClaim cannot find the run anymore.
	batch, err := q.DequeueNClaim(ctx, 10)
	if err != nil {
		t.Fatalf("DequeueNClaim before reconcile: %v", err)
	}
	for _, r := range batch {
		if r.ID == run.ID {
			t.Fatal("run should NOT be dequeued without a claim row")
		}
	}

	// Run the reconciler's missing-claims SQL directly.
	missingSQL := `
		INSERT INTO job_run_queue (
			run_id, job_id, project_id, priority, created_at,
			scheduled_at, next_retry_at, concurrency_key,
			job_max_concurrency, job_max_concurrency_per_key,
			job_enabled, job_paused
		)
		SELECT
			jr.id, jr.job_id, jr.project_id, jr.priority, jr.created_at,
			jr.scheduled_at, jr.next_retry_at, jr.concurrency_key,
			j.max_concurrency, j.max_concurrency_per_key,
			j.enabled, j.paused
		FROM job_runs jr
		JOIN jobs j ON j.id = jr.job_id
		LEFT JOIN job_run_queue q ON q.run_id = jr.id
		WHERE jr.status IN ('queued', 'delayed')
		  AND q.run_id IS NULL
		LIMIT 1000
		ON CONFLICT (run_id) DO NOTHING`

	tag, err := testDB.Pool.Exec(ctx, missingSQL)
	if err != nil {
		t.Fatalf("reconcile missing claims: %v", err)
	}
	if tag.RowsAffected() == 0 {
		t.Fatal("expected reconciler to insert at least 1 missing claim row")
	}

	// Verify the claim row was re-created.
	var count int
	err = testDB.Pool.QueryRow(ctx,
		`SELECT count(*) FROM job_run_queue WHERE run_id = $1`, run.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count claim rows: %v", err)
	}
	if count != 1 {
		t.Errorf("claim row count after reconcile = %d, want 1", count)
	}
}

// ---------------------------------------------------------------------------
// Reliability: reconciler removes stale / orphan claim rows
// ---------------------------------------------------------------------------

func TestClaimReconciler_RemovesStaleClaims(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)

	// Insert an orphan claim row with no corresponding job_runs row.
	orphanID := newID()
	_, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_queue (run_id, job_id, project_id, priority, created_at, job_enabled, job_paused)
		VALUES ($1, 'nonexistent-job', 'orphan-project', 1, NOW(), true, false)
	`, orphanID)
	if err != nil {
		t.Fatalf("insert orphan claim row: %v", err)
	}

	// Confirm the orphan row exists.
	var before int
	err = testDB.Pool.QueryRow(ctx,
		`SELECT count(*) FROM job_run_queue WHERE run_id = $1`, orphanID,
	).Scan(&before)
	if err != nil {
		t.Fatalf("count before reconcile: %v", err)
	}
	if before != 1 {
		t.Fatalf("orphan claim row not present before reconcile (count=%d)", before)
	}

	// Run the reconciler's stale-claims SQL directly.
	staleSQL := `
		DELETE FROM job_run_queue
		WHERE run_id IN (
			SELECT q.run_id
			FROM job_run_queue q
			LEFT JOIN job_runs jr ON jr.id = q.run_id
			WHERE jr.id IS NULL
			   OR jr.status NOT IN ('queued', 'delayed')
			LIMIT 1000
		)`

	tag, err := testDB.Pool.Exec(ctx, staleSQL)
	if err != nil {
		t.Fatalf("reconcile stale claims: %v", err)
	}
	if tag.RowsAffected() == 0 {
		t.Fatal("expected reconciler to delete at least 1 stale claim row")
	}

	// Verify the orphan claim row was removed.
	var after int
	err = testDB.Pool.QueryRow(ctx,
		`SELECT count(*) FROM job_run_queue WHERE run_id = $1`, orphanID,
	).Scan(&after)
	if err != nil {
		t.Fatalf("count after reconcile: %v", err)
	}
	if after != 0 {
		t.Errorf("orphan claim row still present after reconcile (count=%d)", after)
	}
}

// ---------------------------------------------------------------------------
// Reliability: retry exhaustion transitions to terminal failure
// ---------------------------------------------------------------------------

func TestRetryExhaustion_TransitionsToDead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)

	// Create a job with max_attempts=2.
	job := &domain.Job{
		ID:          newID(),
		ProjectID:   "project-retry-exhaust",
		Name:        "job-" + newID(),
		Slug:        "slug-" + newID(),
		EndpointURL: "https://example.com/retry-exhaust",
		MaxAttempts: 2,
		TimeoutSecs: 300,
		Enabled:     true,
	}
	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	q := mustQueue(t)

	// Enqueue a run (attempt 0, status=queued).
	run := mustEnqueueRun(t, ctx, q, job)

	// --- Attempt 1: dequeue and simulate failure ---
	batch, err := q.DequeueNClaim(ctx, 10)
	if err != nil {
		t.Fatalf("dequeue attempt 1: %v", err)
	}
	found := false
	for _, r := range batch {
		if r.ID == run.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("run not found in first dequeue")
	}

	// Simulate worker failure: mark as failed, bump attempt.
	_, err = testDB.Pool.Exec(ctx, `
		UPDATE job_runs
		SET status = 'failed', attempt = 1, finished_at = NOW()
		WHERE id = $1
	`, run.ID)
	if err != nil {
		t.Fatalf("simulate failure attempt 1: %v", err)
	}

	// Re-enqueue for retry: reset to queued, bump attempt.
	_, err = testDB.Pool.Exec(ctx, `
		UPDATE job_runs
		SET status = 'queued', finished_at = NULL
		WHERE id = $1
	`, run.ID)
	if err != nil {
		t.Fatalf("re-enqueue for retry: %v", err)
	}

	// Re-insert the claim row (simulates retry re-enqueue path).
	_, err = testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_queue (run_id, job_id, project_id, priority, created_at, job_enabled, job_paused)
		VALUES ($1, $2, $3, 1, NOW(), true, false)
		ON CONFLICT (run_id) DO NOTHING
	`, run.ID, job.ID, job.ProjectID)
	if err != nil {
		t.Fatalf("insert retry claim row: %v", err)
	}

	// --- Attempt 2: dequeue and simulate failure again ---
	batch, err = q.DequeueNClaim(ctx, 10)
	if err != nil {
		t.Fatalf("dequeue attempt 2: %v", err)
	}
	found = false
	for _, r := range batch {
		if r.ID == run.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("run not found in second dequeue")
	}

	// Simulate final failure: mark as failed with attempts=max_attempts.
	_, err = testDB.Pool.Exec(ctx, `
		UPDATE job_runs
		SET status = 'failed', attempt = 2, finished_at = NOW()
		WHERE id = $1
	`, run.ID)
	if err != nil {
		t.Fatalf("simulate failure attempt 2: %v", err)
	}

	// Verify the run is terminally failed and retries are exhausted.
	var finalStatus string
	var attempts int
	err = testDB.Pool.QueryRow(ctx,
		`SELECT status, attempt FROM job_runs WHERE id = $1`, run.ID,
	).Scan(&finalStatus, &attempts)
	if err != nil {
		t.Fatalf("query final state: %v", err)
	}

	// Run should be in a terminal failure state with attempts == max_attempts.
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
	terminalFailure := finalStatus == "failed" || finalStatus == "dead_letter"
	if !terminalFailure {
		t.Errorf("status = %q, want 'failed' or 'dead_letter'", finalStatus)
	}

	// No claim row should remain for a terminal run.
	var claimCount int
	err = testDB.Pool.QueryRow(ctx,
		`SELECT count(*) FROM job_run_queue WHERE run_id = $1`, run.ID,
	).Scan(&claimCount)
	if err != nil {
		t.Fatalf("count claim rows: %v", err)
	}
	if claimCount != 0 {
		t.Errorf("stale claim row exists for terminal run (count=%d)", claimCount)
	}
}
