//go:build integration

package queue_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

// ---------------------------------------------------------------------------
// Adversarial tests
// ---------------------------------------------------------------------------

func TestClaimTable_SQLInjection_RunID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-claim-sqli-runid")
	q := mustQueue(t)

	// Use a valid UUID for the actual run ID (FK/PK constraints require it),
	// but smuggle the injection payload through the idempotency key and
	// concurrency key — fields that flow into the claim table.
	run := &domain.JobRun{
		ID:             newID(),
		JobID:          job.ID,
		ProjectID:      job.ProjectID,
		IdempotencyKey: "'; DROP TABLE job_run_queue; --",
		Priority:       1,
	}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("enqueue with injection key: %v", err)
	}

	// Verify claim table still exists and has the row.
	var count int
	err := testDB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_run_queue WHERE run_id = $1`, run.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("claim table broken after injection: %v", err)
	}
	if count != 1 {
		t.Errorf("claim row count = %d, want 1", count)
	}

	// Dequeue still works.
	batch, err := q.DequeueNClaim(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueNClaim after injection: %v", err)
	}
	if len(batch) != 1 {
		t.Errorf("dequeued %d runs, want 1", len(batch))
	}
}

func TestClaimTable_SQLInjection_ConcurrencyKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-claim-sqli-ck")
	q := mustQueue(t)

	injectionPayload := "'; DROP TABLE job_run_queue; --"
	run := &domain.JobRun{
		ID:             newID(),
		JobID:          job.ID,
		ProjectID:      job.ProjectID,
		ConcurrencyKey: injectionPayload,
		Priority:       1,
	}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("enqueue with injection ck: %v", err)
	}

	batch, err := q.DequeueNClaim(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueNClaim: %v", err)
	}
	if len(batch) != 1 {
		t.Fatalf("dequeued %d runs, want 1", len(batch))
	}
	if batch[0].ConcurrencyKey != injectionPayload {
		t.Errorf("concurrency_key = %q, want %q", batch[0].ConcurrencyKey, injectionPayload)
	}

	// Table still intact.
	var tableExists bool
	err = testDB.Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'job_run_queue')`,
	).Scan(&tableExists)
	if err != nil || !tableExists {
		t.Fatal("job_run_queue table was dropped by injection")
	}
}

func TestClaimTable_OrphanClaimRow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	q := mustQueue(t)

	// Insert an orphan claim row with no corresponding job_runs row.
	orphanID := newID()
	_, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_run_queue (run_id, job_id, project_id, priority, created_at, job_enabled, job_paused)
		VALUES ($1, 'nonexistent-job', 'orphan-project', 1, NOW(), true, false)
	`, orphanID)
	if err != nil {
		t.Fatalf("insert orphan claim row: %v", err)
	}

	// DequeueNClaim: the DELETE phase removes the claim row, the UPDATE
	// touches 0 job_runs rows, and the fetch returns 0 runs.
	batch, err := q.DequeueNClaim(ctx, 10)
	if err != nil {
		t.Fatalf("DequeueNClaim with orphan: %v", err)
	}
	// The orphan claim was deleted but no job_runs row exists to fetch,
	// so we expect 0 actual runs returned (no panic, no error).
	if len(batch) != 0 {
		t.Errorf("expected 0 runs from orphan claim, got %d", len(batch))
	}

	// Orphan claim row should be gone (consumed by DELETE).
	var remaining int
	_ = testDB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_run_queue WHERE run_id = $1`, orphanID,
	).Scan(&remaining)
	if remaining != 0 {
		t.Errorf("orphan claim row still present, count = %d", remaining)
	}
}

func TestClaimTable_OrphanJobRunsRow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-claim-orphan-jr")
	q := mustQueue(t)

	run := mustEnqueueRun(t, ctx, q, job)

	// Manually delete the claim row to create an orphan job_runs row.
	_, err := testDB.Pool.Exec(ctx,
		`DELETE FROM job_run_queue WHERE run_id = $1`, run.ID,
	)
	if err != nil {
		t.Fatalf("delete claim row: %v", err)
	}

	// DequeueNClaim should NOT find the run (no claim row).
	batch, err := q.DequeueNClaim(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueNClaim: %v", err)
	}
	if len(batch) != 0 {
		t.Errorf("DequeueNClaim found %d runs without claim row, want 0", len(batch))
	}

	// DequeueNTwoPhase should still find it (scans job_runs directly).
	batch2, err := q.DequeueNTwoPhase(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueNTwoPhase: %v", err)
	}
	if len(batch2) != 1 {
		t.Errorf("DequeueNTwoPhase found %d runs, want 1", len(batch2))
	} else if batch2[0].ID != run.ID {
		t.Errorf("DequeueNTwoPhase returned run %s, want %s", batch2[0].ID, run.ID)
	}
}

func TestClaimTable_NegativePriority(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-claim-negpri")
	q := mustQueue(t)

	lowRun := &domain.JobRun{
		ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
		Priority: -1000,
	}
	highRun := &domain.JobRun{
		ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
		Priority: 1000,
	}
	// Enqueue low first, high second.
	if err := q.Enqueue(ctx, lowRun); err != nil {
		t.Fatalf("enqueue low: %v", err)
	}
	if err := q.Enqueue(ctx, highRun); err != nil {
		t.Fatalf("enqueue high: %v", err)
	}
	// Verify both claim rows exist before dequeuing.
	var claimCount int
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT count(*) FROM job_run_queue WHERE run_id IN ($1, $2)`,
		lowRun.ID, highRun.ID,
	).Scan(&claimCount); err != nil {
		t.Fatalf("count claim rows: %v", err)
	}
	if claimCount != 2 {
		t.Fatalf("expected 2 claim rows, got %d (trigger may have failed)", claimCount)
	}

	// DequeueNClaim claims in priority DESC order via the DELETE, but the
	// final SELECT orders by created_at ASC. Both runs have nearly identical
	// created_at, so result order is nondeterministic. Verify both are returned.
	batch, err := q.DequeueNClaim(ctx, 2)
	if err != nil {
		t.Fatalf("DequeueNClaim: %v", err)
	}
	if len(batch) < 2 {
		t.Fatalf("dequeued %d runs, want 2", len(batch))
	}
	ids := map[string]int{}
	for _, r := range batch {
		ids[r.ID] = r.Priority
	}
	if _, ok := ids[highRun.ID]; !ok {
		t.Errorf("high-priority run %s not in result set", highRun.ID)
	}
	if _, ok := ids[lowRun.ID]; !ok {
		t.Errorf("low-priority run %s not in result set", lowRun.ID)
	}
}

func TestClaimTable_MaxConcurrencyZero_MeansUnlimited(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)

	// max_concurrency = 0 means "no limit" (same as NULL).
	job := &domain.Job{
		ID:             newID(),
		ProjectID:      "project-claim-maxconc0",
		Name:           "job-" + newID(),
		Slug:           "slug-" + newID(),
		EndpointURL:    "https://example.com/unlimited",
		MaxAttempts:    3,
		TimeoutSecs:    300,
		Enabled:        true,
		MaxConcurrency: 0,
	}
	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	q := mustQueue(t)

	// Enqueue 3 runs. All should be dequeued since 0 = unlimited.
	for i := 0; i < 3; i++ {
		run := &domain.JobRun{
			ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 1,
		}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	batch, err := q.DequeueNClaim(ctx, 10)
	if err != nil {
		t.Fatalf("DequeueNClaim: %v", err)
	}
	if len(batch) != 3 {
		t.Errorf("dequeued %d runs, want 3 (max_concurrency=0 means unlimited)", len(batch))
	}
}

// ---------------------------------------------------------------------------
// Chaos tests
// ---------------------------------------------------------------------------

func TestClaimTable_CrashBetweenDeleteAndUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-claim-crash")
	q := mustQueue(t)

	run := mustEnqueueRun(t, ctx, q, job)

	// Simulate a crash: start a transaction that DELETEs from the claim
	// table but never commits (rolls back).
	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	_, err = tx.Exec(ctx,
		`DELETE FROM job_run_queue WHERE run_id = $1`, run.ID,
	)
	if err != nil {
		t.Fatalf("delete in tx: %v", err)
	}

	// Rollback simulates the crash — claim row is restored.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	// Claim row should still exist.
	var count int
	_ = testDB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM job_run_queue WHERE run_id = $1`, run.ID,
	).Scan(&count)
	if count != 1 {
		t.Fatalf("claim row not restored after rollback, count = %d", count)
	}

	// A subsequent DequeueNClaim should succeed.
	batch, err := q.DequeueNClaim(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueNClaim after rollback: %v", err)
	}
	if len(batch) != 1 || batch[0].ID != run.ID {
		t.Errorf("expected to dequeue %s, got %v", run.ID, batch)
	}
}

func TestClaimTable_StaleClaimRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-claim-stale")
	q := mustQueue(t)

	run := mustEnqueueRun(t, ctx, q, job)

	// Backdate the claim row's timestamps to simulate stale rows.
	_, err := testDB.Pool.Exec(ctx, `
		UPDATE job_run_queue
		SET created_at = NOW() - INTERVAL '1 hour',
		    next_retry_at = NOW() - INTERVAL '30 minutes'
		WHERE run_id = $1
	`, run.ID)
	if err != nil {
		t.Fatalf("backdate claim row: %v", err)
	}

	// DequeueNClaim should still pick up these stale rows (they're past
	// their scheduled/retry time).
	batch, err := q.DequeueNClaim(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueNClaim stale: %v", err)
	}
	if len(batch) != 1 || batch[0].ID != run.ID {
		t.Errorf("expected to dequeue stale run %s, got %v", run.ID, batch)
	}
}

func TestClaimTable_ConcurrentEnqueueDequeue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-claim-concurrent")
	q := mustQueue(t)

	const (
		numProducers    = 10
		numConsumers    = 10
		runsPerProducer = 50
		batchSize       = 10
		duration        = 5 * time.Second
	)

	var enqueued atomic.Int64
	var mu sync.Mutex
	dequeued := make(map[string]bool)

	// Producers: enqueue runs.
	var prodWG sync.WaitGroup
	for range numProducers {
		prodWG.Add(1)
		go func() {
			defer prodWG.Done()
			for i := range runsPerProducer {
				_ = i
				select {
				case <-ctx.Done():
					return
				default:
				}
				r := &domain.JobRun{
					ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
					Priority: 1,
				}
				if err := q.Enqueue(ctx, r); err != nil {
					continue
				}
				enqueued.Add(1)
			}
		}()
	}

	// Consumers: dequeue runs for `duration`.
	deadline := time.After(duration)
	var consWG sync.WaitGroup
	for range numConsumers {
		consWG.Add(1)
		go func() {
			defer consWG.Done()
			for {
				select {
				case <-deadline:
					return
				case <-ctx.Done():
					return
				default:
				}
				batch, err := q.DequeueNClaim(ctx, batchSize)
				if err != nil {
					continue
				}
				mu.Lock()
				for _, r := range batch {
					if dequeued[r.ID] {
						t.Errorf("DUPLICATE claim: run %s dequeued twice", r.ID)
					}
					dequeued[r.ID] = true
				}
				mu.Unlock()
			}
		}()
	}

	prodWG.Wait()
	// Let consumers drain a bit more.
	time.Sleep(1 * time.Second)
	consWG.Wait()

	mu.Lock()
	dequeuedCount := len(dequeued)
	mu.Unlock()

	totalEnqueued := int(enqueued.Load())
	if dequeuedCount > totalEnqueued {
		t.Errorf("dequeued %d > enqueued %d — phantom runs", dequeuedCount, totalEnqueued)
	}
	t.Logf("enqueued=%d dequeued=%d", totalEnqueued, dequeuedCount)
}

func TestClaimTable_TriggerRaceCondition(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-claim-race")
	q := mustQueue(t)

	// Enqueue 10 runs.
	runIDs := make([]string, 10)
	for i := range 10 {
		r := &domain.JobRun{
			ID: newID(), JobID: job.ID, ProjectID: job.ProjectID,
			Priority: 1,
		}
		if err := q.Enqueue(ctx, r); err != nil {
			t.Fatalf("enqueue run %d: %v", i, err)
		}
		runIDs[i] = r.ID
	}

	var wg sync.WaitGroup
	var dequeueErr, pauseErr error
	var dequeuedRuns []domain.JobRun

	// Concurrently: (a) pause the job, (b) dequeue.
	wg.Add(2)

	// (a) Pause the job by updating both the jobs table and the claim rows.
	go func() {
		defer wg.Done()
		_, err := testDB.Pool.Exec(ctx,
			`UPDATE jobs SET paused = true, paused_at = NOW() WHERE id = $1`, job.ID,
		)
		if err != nil {
			pauseErr = err
			return
		}
		_, err = testDB.Pool.Exec(ctx,
			`UPDATE job_run_queue SET job_paused = true WHERE job_id = $1`, job.ID,
		)
		if err != nil {
			pauseErr = err
		}
	}()

	// (b) DequeueNClaim.
	go func() {
		defer wg.Done()
		batch, err := q.DequeueNClaim(ctx, 10)
		dequeueErr = err
		dequeuedRuns = batch
	}()

	wg.Wait()

	if pauseErr != nil {
		t.Fatalf("pause failed: %v", pauseErr)
	}
	if dequeueErr != nil {
		t.Fatalf("dequeue failed (deadlock?): %v", dequeueErr)
	}

	// Consistency check: any runs that were dequeued should be in
	// 'dequeued' status; any that weren't should still be 'queued'.
	dequeuedSet := make(map[string]bool)
	for _, r := range dequeuedRuns {
		dequeuedSet[r.ID] = true
	}

	for _, id := range runIDs {
		var status string
		err := testDB.Pool.QueryRow(ctx,
			`SELECT status FROM job_runs WHERE id = $1`, id,
		).Scan(&status)
		if err != nil {
			t.Errorf("query run %s status: %v", id, err)
			continue
		}
		if dequeuedSet[id] {
			if status != string(domain.StatusExecuting) {
				t.Errorf("run %s dequeued but status = %s", id, status)
			}
		} else {
			if status != string(domain.StatusQueued) {
				t.Errorf("run %s not dequeued but status = %s (want queued)", id, status)
			}
		}
	}

	t.Logf("race result: dequeued %d of 10 runs", len(dequeuedRuns))
}
