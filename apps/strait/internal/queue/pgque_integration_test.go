//go:build integration

package queue_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
)

func mustPgQueQueue(t *testing.T) *queue.PgQueQueue {
	t.Helper()
	return queue.NewPgQueQueue(testDB.Pool, queue.NewPostgresQueue(testDB.Pool), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "test-" + newID(),
		NackDelay:     10 * time.Millisecond,
		ReceiveWindow: 100,
	})
}

func TestQueueEngine_PgQueSelectable(t *testing.T) {
	q, err := queue.NewQueueEngine(testDB.Pool, queue.EnginePgQue, queue.BatchlogConfig{TickInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewQueueEngine(pgque): %v", err)
	}
	if _, ok := q.(*queue.PgQueQueue); !ok {
		t.Fatalf("queue = %T, want *PgQueQueue", q)
	}
}

func TestPgQue_EnqueueInTxRollbackLeavesNoClaimableEvent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-rollback")
	q := mustPgQueQueue(t)

	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.EnqueueInTx(ctx, tx, run); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("EnqueueInTx: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	claimed, err := q.DequeueN(ctx, 10)
	if err != nil {
		t.Fatalf("DequeueN: %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("claimed len = %d, want 0 after rollback", len(claimed))
	}
}

func TestPgQue_CreatesRouteIdempotently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-route")
	q := mustPgQueQueue(t)

	for range 2 {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}

	var routeRows, queueRows int
	if err := testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM strait_pgque_routes WHERE route_key = 'http'`).Scan(&routeRows); err != nil {
		t.Fatalf("route count: %v", err)
	}
	if routeRows != 1 {
		t.Fatalf("route rows = %d, want 1", routeRows)
	}
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pgque.queue q
		JOIN strait_pgque_routes r ON r.queue_name = q.queue_name
		WHERE r.route_key = 'http'`).Scan(&queueRows); err != nil {
		t.Fatalf("pgque queue count: %v", err)
	}
	if queueRows != 1 {
		t.Fatalf("pgque queue rows = %d, want 1", queueRows)
	}
}

func TestPgQue_DoesNotCreateLegacyQueueEntries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-no-legacy-queue-entry")
	q := mustPgQueQueue(t)

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	var queueEntries int
	if err := testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM queue_entries WHERE run_id = $1`, run.ID).Scan(&queueEntries); err != nil {
		t.Fatalf("queue_entries count after enqueue: %v", err)
	}
	if queueEntries != 0 {
		t.Fatalf("queue_entries after PgQue enqueue = %d, want 0", queueEntries)
	}

	if err := q.ForceTick(ctx, "http"); err != nil {
		t.Fatalf("ForceTick: %v", err)
	}
	claimed, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueN: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != run.ID {
		t.Fatalf("claimed = %+v, want run %s", claimed, run.ID)
	}
	if err := st.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{"finished_at": time.Now()}); err != nil {
		t.Fatalf("UpdateRunStatus completed: %v", err)
	}

	if err := testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM queue_entries WHERE run_id = $1`, run.ID).Scan(&queueEntries); err != nil {
		t.Fatalf("queue_entries count after complete: %v", err)
	}
	if queueEntries != 0 {
		t.Fatalf("queue_entries after PgQue completion = %d, want 0", queueEntries)
	}

	batchRuns := []*domain.JobRun{
		{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID},
		{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID},
	}
	inserted, err := q.EnqueueBatch(ctx, batchRuns)
	if err != nil {
		t.Fatalf("EnqueueBatch: %v", err)
	}
	if inserted != int64(len(batchRuns)) {
		t.Fatalf("EnqueueBatch inserted = %d, want %d", inserted, len(batchRuns))
	}
	if err := testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM queue_entries WHERE run_id = ANY($1)`, []string{batchRuns[0].ID, batchRuns[1].ID}).Scan(&queueEntries); err != nil {
		t.Fatalf("queue_entries count after batch enqueue: %v", err)
	}
	if queueEntries != 0 {
		t.Fatalf("queue_entries after PgQue batch enqueue = %d, want 0", queueEntries)
	}
}

func TestPgQue_DequeueWindowDoesNotLoseUnseenBatchMessages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-window")
	q := queue.NewPgQueQueue(testDB.Pool, queue.NewPostgresQueue(testDB.Pool), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "test-" + newID(),
		NackDelay:     10 * time.Millisecond,
		ReceiveWindow: 10,
	})

	want := 25
	for i := 0; i < want; i++ {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}
	if err := q.ForceTick(ctx, "http"); err != nil {
		t.Fatalf("ForceTick: %v", err)
	}

	seen := make(map[string]struct{}, want)
	deadline := time.Now().Add(5 * time.Second)
	for len(seen) < want && time.Now().Before(deadline) {
		runs, err := q.DequeueN(ctx, 5)
		if err != nil {
			t.Fatalf("DequeueN: %v", err)
		}
		if len(runs) == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		for _, run := range runs {
			if _, ok := seen[run.ID]; ok {
				t.Fatalf("duplicate claim for run %s", run.ID)
			}
			seen[run.ID] = struct{}{}
		}
	}
	if len(seen) != want {
		t.Fatalf("claimed %d runs, want %d; small PgQue receive windows must not ack away unseen batch messages", len(seen), want)
	}
}

func TestPgQue_ConcurrentDequeueDrainsSingleBatchWithoutDuplicates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-concurrent-batch")
	q := queue.NewPgQueQueue(testDB.Pool, queue.NewPostgresQueue(testDB.Pool), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "test-" + newID(),
		NackDelay:     10 * time.Millisecond,
		ReceiveWindow: 100,
	})

	const want = 120
	runs := make([]*domain.JobRun, 0, want)
	for range want {
		runs = append(runs, &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID})
	}
	inserted, err := q.EnqueueBatch(ctx, runs)
	if err != nil {
		t.Fatalf("EnqueueBatch: %v", err)
	}
	if inserted != want {
		t.Fatalf("EnqueueBatch inserted = %d, want %d", inserted, want)
	}
	if err := q.ForceTick(ctx, "http"); err != nil {
		t.Fatalf("ForceTick: %v", err)
	}

	var mu sync.Mutex
	seen := make(map[string]struct{}, want)
	errCh := make(chan error, 1)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for {
				mu.Lock()
				done := len(seen) >= want
				mu.Unlock()
				if done {
					return
				}
				claimed, err := q.DequeueN(ctx, 4)
				if err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}
				if len(claimed) == 0 {
					time.Sleep(5 * time.Millisecond)
					continue
				}
				mu.Lock()
				for _, run := range claimed {
					if _, ok := seen[run.ID]; ok {
						mu.Unlock()
						select {
						case errCh <- fmt.Errorf("duplicate claim for run %s", run.ID):
						default:
						}
						return
					}
					seen[run.ID] = struct{}{}
				}
				mu.Unlock()
			}
		}()
	}
	close(start)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("concurrent dequeue: %v", err)
	case <-done:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for concurrent dequeue: claimed %d of %d", len(seen), want)
	}
	if len(seen) != want {
		t.Fatalf("claimed %d runs, want %d", len(seen), want)
	}
}

func TestPgQue_ClaimUsesRunStateNotFatLedger(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-state-claim")
	q := mustPgQueQueue(t)

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := q.ForceTick(ctx, "http"); err != nil {
		t.Fatalf("ForceTick: %v", err)
	}
	claimed, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueN: %v", err)
	}
	if len(claimed) != 1 || claimed[0].Status != domain.StatusExecuting {
		t.Fatalf("claimed = %+v, want one executing run", claimed)
	}

	var ledgerStatus, stateStatus string
	if err := testDB.Pool.QueryRow(ctx, `SELECT status FROM job_runs WHERE id = $1`, run.ID).Scan(&ledgerStatus); err != nil {
		t.Fatalf("ledger status: %v", err)
	}
	if err := testDB.Pool.QueryRow(ctx, `SELECT status FROM job_run_state WHERE run_id = $1`, run.ID).Scan(&stateStatus); err != nil {
		t.Fatalf("state status: %v", err)
	}
	if ledgerStatus != string(domain.StatusQueued) {
		t.Fatalf("ledger status = %q, want queued to avoid fat-row claim churn", ledgerStatus)
	}
	if stateStatus != string(domain.StatusQueued) {
		t.Fatalf("state status = %q, want queued to avoid hot-row claim churn", stateStatus)
	}
	var claimRows int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_run_active_claims
		WHERE run_id = $1`, run.ID).Scan(&claimRows); err != nil {
		t.Fatalf("active claims: %v", err)
	}
	if claimRows != 1 {
		t.Fatalf("active claims = %d, want 1 append-only ownership row", claimRows)
	}
	var readStatus string
	if err := testDB.Pool.QueryRow(ctx, `SELECT status FROM job_run_read_state WHERE run_id = $1`, run.ID).Scan(&readStatus); err != nil {
		t.Fatalf("read state status: %v", err)
	}
	if readStatus != string(domain.StatusExecuting) {
		t.Fatalf("read state status = %q, want executing from active claim overlay", readStatus)
	}
	got, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Status != domain.StatusExecuting {
		t.Fatalf("GetRun status = %q, want executing from active claim overlay", got.Status)
	}
}

func TestPgQue_TerminalTransitionCompletesActiveClaimWithoutUpdatingHotState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-active-claim-terminal")
	q := mustPgQueQueue(t)

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := q.ForceTick(ctx, "http"); err != nil {
		t.Fatalf("ForceTick: %v", err)
	}
	claimed, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueN: %v", err)
	}
	if len(claimed) != 1 || claimed[0].Status != domain.StatusExecuting {
		t.Fatalf("claimed = %+v, want executing run", claimed)
	}

	finishedAt := time.Now().UTC()
	if err := st.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"finished_at": finishedAt,
	}); err != nil {
		t.Fatalf("UpdateRunStatus executing->completed: %v", err)
	}

	var hotStatus, readStatus, terminalStatus domain.RunStatus
	if err := testDB.Pool.QueryRow(ctx, `SELECT status FROM job_run_state WHERE run_id = $1`, run.ID).Scan(&hotStatus); err != nil {
		t.Fatalf("hot status: %v", err)
	}
	if hotStatus != domain.StatusQueued {
		t.Fatalf("hot status = %s, want queued retained after terminal append", hotStatus)
	}
	if err := testDB.Pool.QueryRow(ctx, `SELECT status FROM job_run_terminal_state WHERE run_id = $1`, run.ID).Scan(&terminalStatus); err != nil {
		t.Fatalf("terminal status: %v", err)
	}
	if terminalStatus != domain.StatusCompleted {
		t.Fatalf("terminal status = %s, want completed", terminalStatus)
	}
	if err := testDB.Pool.QueryRow(ctx, `SELECT status FROM job_run_read_state WHERE run_id = $1`, run.ID).Scan(&readStatus); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if readStatus != domain.StatusCompleted {
		t.Fatalf("read status = %s, want completed terminal overlay", readStatus)
	}
}

func TestPgQue_ActiveClaimEnforcesLimitedConcurrencyWithoutCounterWrites(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-active-claim-counts")
	q := mustPgQueQueue(t)

	runs := []*domain.JobRun{
		{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID},
		{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID},
	}
	for _, run := range runs {
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_run_state SET job_max_concurrency = 1 WHERE job_id = $1`, job.ID); err != nil {
		t.Fatalf("set max concurrency: %v", err)
	}

	claimed, err := q.DequeueN(ctx, 2)
	if err != nil {
		t.Fatalf("DequeueN: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed %d runs, want 1 due to max concurrency", len(claimed))
	}
	var activeClaims int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_run_active_claims claim
		JOIN job_run_state s
		  ON s.run_id = claim.run_id
		 AND s.ready_generation = claim.ready_generation
		LEFT JOIN job_run_terminal_state terminal ON terminal.run_id = s.run_id
		WHERE s.job_id = $1
		  AND s.status = 'queued'
		  AND terminal.run_id IS NULL`, job.ID).Scan(&activeClaims); err != nil {
		t.Fatalf("active claims after claim: %v", err)
	}
	if activeClaims != 1 {
		t.Fatalf("active claims after claim = %d, want 1", activeClaims)
	}
	var count int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(count), 0)
		FROM job_active_counts
		WHERE job_id = $1`, job.ID).Scan(&count); err != nil {
		t.Fatalf("active count after claim: %v", err)
	}
	if count != 0 {
		t.Fatalf("job_active_counts after PgQue claim = %d, want 0 append-only claims to avoid counter churn", count)
	}
	if err := st.UpdateRunStatus(ctx, claimed[0].ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{"finished_at": time.Now()}); err != nil {
		t.Fatalf("UpdateRunStatus completed: %v", err)
	}
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(count), 0)
		FROM job_active_counts
		WHERE job_id = $1`, job.ID).Scan(&count); err != nil {
		t.Fatalf("active count after completion: %v", err)
	}
	if count != 0 {
		t.Fatalf("active count after completion = %d, want 0", count)
	}

	next, err := q.DequeueN(ctx, 2)
	if err != nil {
		t.Fatalf("DequeueN after completion: %v", err)
	}
	if len(next) != 1 {
		t.Fatalf("claimed %d runs after completion, want 1 released by terminal overlay", len(next))
	}
	if next[0].ID == claimed[0].ID {
		t.Fatalf("claimed completed run %s again", next[0].ID)
	}
}

func TestPgQue_ConcurrentLimitedClaimsSerializeOnActiveClaims(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-active-claim-serialized")
	q := mustPgQueQueue(t)

	for range 2 {
		run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_run_state SET job_max_concurrency = 1 WHERE job_id = $1`, job.ID); err != nil {
		t.Fatalf("set max concurrency: %v", err)
	}
	if err := q.ForceTick(ctx, "http"); err != nil {
		t.Fatalf("ForceTick: %v", err)
	}

	start := make(chan struct{})
	errCh := make(chan error, 8)
	claimedCh := make(chan []domain.JobRun, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			claimed, err := q.DequeueN(ctx, 1)
			if err != nil {
				errCh <- err
				return
			}
			claimedCh <- claimed
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)
	close(claimedCh)

	for err := range errCh {
		t.Fatalf("DequeueN: %v", err)
	}
	claimedIDs := make(map[string]struct{})
	for claimed := range claimedCh {
		for _, run := range claimed {
			if _, ok := claimedIDs[run.ID]; ok {
				t.Fatalf("duplicate claim for run %s", run.ID)
			}
			claimedIDs[run.ID] = struct{}{}
		}
	}
	if len(claimedIDs) != 1 {
		t.Fatalf("concurrent claims = %d, want 1 due to max concurrency", len(claimedIDs))
	}

	var count int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(count), 0)
		FROM job_active_counts
		WHERE job_id = $1`, job.ID).Scan(&count); err != nil {
		t.Fatalf("active counter: %v", err)
	}
	if count != 0 {
		t.Fatalf("job_active_counts after concurrent PgQue claims = %d, want 0", count)
	}
}

func TestPgQue_StaleGenerationEventIsIgnored(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-stale-generation")
	q := mustPgQueQueue(t)

	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_run_state SET ready_generation = ready_generation + 1 WHERE run_id = $1`, run.ID); err != nil {
		t.Fatalf("bump ready generation: %v", err)
	}

	claimed, err := q.DequeueN(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueN: %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("claimed stale generation event = %+v, want none", claimed)
	}
}

func TestPgQue_DequeueNForWorkerQueuesFiltersByEnvironment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	projectID := "project-pgque-worker-env"
	prodEnvID := mustCreateEnvironment(t, ctx, st, projectID, "production")
	stagingEnvID := mustCreateEnvironment(t, ctx, st, projectID, "staging")
	prodJob := mustCreateJob(t, ctx, st, projectID)
	markWorkerJobQueueEnvironment(t, ctx, prodJob, "priority", prodEnvID)
	stagingJob := mustCreateJob(t, ctx, st, projectID)
	markWorkerJobQueueEnvironment(t, ctx, stagingJob, "priority", stagingEnvID)
	q := mustPgQueQueue(t)

	prodRun := &domain.JobRun{ID: newID(), JobID: prodJob.ID, ProjectID: projectID, ExecutionMode: domain.ExecutionModeWorker, QueueName: "priority"}
	if err := q.Enqueue(ctx, prodRun); err != nil {
		t.Fatalf("enqueue prod: %v", err)
	}
	stagingRun := &domain.JobRun{ID: newID(), JobID: stagingJob.ID, ProjectID: projectID, ExecutionMode: domain.ExecutionModeWorker, QueueName: "priority"}
	if err := q.Enqueue(ctx, stagingRun); err != nil {
		t.Fatalf("enqueue staging: %v", err)
	}

	stagingBatch, err := q.DequeueNForWorkerQueues(ctx, 1, []domain.WorkerQueueRef{{ProjectID: projectID, QueueName: "priority", EnvironmentID: stagingEnvID}})
	if err != nil {
		t.Fatalf("DequeueNForWorkerQueues(staging): %v", err)
	}
	if len(stagingBatch) != 1 || stagingBatch[0].ID != stagingRun.ID {
		t.Fatalf("staging batch = %+v, want staging run %s", stagingBatch, stagingRun.ID)
	}
	prodBatch, err := q.DequeueNForWorkerQueues(ctx, 1, []domain.WorkerQueueRef{{ProjectID: projectID, QueueName: "priority", EnvironmentID: prodEnvID}})
	if err != nil {
		t.Fatalf("DequeueNForWorkerQueues(prod): %v", err)
	}
	if len(prodBatch) != 1 || prodBatch[0].ID != prodRun.ID {
		t.Fatalf("prod batch = %+v, want prod run %s", prodBatch, prodRun.ID)
	}
}

func TestPgQue_MaxConcurrencyEnforcedFromRunState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-pgque-concurrency")
	q := mustPgQueQueue(t)

	runs := []*domain.JobRun{
		{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID},
		{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID},
	}
	for _, run := range runs {
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_run_state SET job_max_concurrency = 1 WHERE job_id = $1`, job.ID); err != nil {
		t.Fatalf("set max concurrency: %v", err)
	}

	claimed, err := q.DequeueN(ctx, 2)
	if err != nil {
		t.Fatalf("DequeueN: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed %d runs, want 1 due to max concurrency: %+v", len(claimed), claimed)
	}
}
