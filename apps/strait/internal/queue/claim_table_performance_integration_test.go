//go:build integration

package queue_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestClaimTable_DequeueNClaim_RespectsDelayedEligibility(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-claim-delayed")
	q := mustQueue(t)

	scheduledAt := time.Now().Add(30 * time.Minute)
	run := &domain.JobRun{
		ID:          newID(),
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		ScheduledAt: &scheduledAt,
	}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("enqueue delayed run: %v", err)
	}

	batch, err := q.DequeueNClaim(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueNClaim future delayed: %v", err)
	}
	if len(batch) != 0 {
		t.Fatalf("future delayed run was claimed: %+v", batch)
	}

	past := time.Now().Add(-time.Second)
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET scheduled_at = $1 WHERE id = $2`, past, run.ID); err != nil {
		t.Fatalf("promote delayed run: %v", err)
	}

	batch, err = q.DequeueNClaim(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueNClaim promoted delayed: %v", err)
	}
	if len(batch) != 1 || batch[0].ID != run.ID {
		t.Fatalf("promoted delayed dequeue = %+v, want run %s", batch, run.ID)
	}
	if batch[0].Status != domain.StatusExecuting {
		t.Fatalf("promoted delayed status = %q, want executing", batch[0].Status)
	}
}

func TestClaimTable_DequeueNClaim_RespectsRetrySchedule(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-claim-retry-schedule")
	q := mustQueue(t)
	run := mustEnqueueRun(t, ctx, q, job)

	nextRetryAt := time.Now().Add(20 * time.Minute)
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO job_retries (run_id, next_retry_at, attempt, scheduled_at)
		VALUES ($1, $2, 1, NOW())
		ON CONFLICT (run_id) DO UPDATE SET next_retry_at = EXCLUDED.next_retry_at`,
		run.ID, nextRetryAt,
	); err != nil {
		t.Fatalf("schedule future retry: %v", err)
	}

	batch, err := q.DequeueNClaim(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueNClaim future retry: %v", err)
	}
	if len(batch) != 0 {
		t.Fatalf("future retry run was claimed: %+v", batch)
	}

	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_retries SET next_retry_at = NOW() - INTERVAL '1 second' WHERE run_id = $1`, run.ID); err != nil {
		t.Fatalf("make retry due: %v", err)
	}

	batch, err = q.DequeueNClaim(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueNClaim due retry: %v", err)
	}
	if len(batch) != 1 || batch[0].ID != run.ID {
		t.Fatalf("due retry dequeue = %+v, want run %s", batch, run.ID)
	}
}

func TestClaimTable_DequeueNClaim_RespectsConcurrencyKeySlots(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-claim-key-slots")
	if _, err := testDB.Pool.Exec(ctx, `UPDATE jobs SET max_concurrency_per_key = 1 WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("set max_concurrency_per_key: %v", err)
	}
	q := mustQueue(t)

	sameKeyA := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, ConcurrencyKey: "tenant-a"}
	sameKeyB := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, ConcurrencyKey: "tenant-a"}
	otherKey := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, ConcurrencyKey: "tenant-b"}
	for _, run := range []*domain.JobRun{sameKeyA, sameKeyB, otherKey} {
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("enqueue %s: %v", run.ID, err)
		}
	}

	first, err := q.DequeueNClaim(ctx, 1)
	if err != nil {
		t.Fatalf("first DequeueNClaim: %v", err)
	}
	if len(first) != 1 || first[0].ConcurrencyKey != "tenant-a" {
		t.Fatalf("first dequeue = %+v, want one tenant-a run", first)
	}

	second, err := q.DequeueNClaim(ctx, 1)
	if err != nil {
		t.Fatalf("second DequeueNClaim: %v", err)
	}
	if len(second) != 1 || second[0].ID != otherKey.ID {
		t.Fatalf("second dequeue = %+v, want unblocked tenant-b run %s", second, otherKey.ID)
	}

	third, err := q.DequeueNClaim(ctx, 1)
	if err != nil {
		t.Fatalf("third DequeueNClaim: %v", err)
	}
	if len(third) != 0 {
		t.Fatalf("same-key run was claimed while tenant-a slot active: %+v", third)
	}
}

func TestClaimTable_DequeueNClaim_UsesUpdatedPriority(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-claim-priority-fanout")
	q := mustQueue(t)

	low := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 1}
	high := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 10}
	for _, run := range []*domain.JobRun{low, high} {
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("enqueue %s: %v", run.ID, err)
		}
	}

	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET priority = 100 WHERE id = $1`, low.ID); err != nil {
		t.Fatalf("raise priority: %v", err)
	}

	var claimPriority int
	if err := testDB.Pool.QueryRow(ctx, `SELECT priority FROM job_run_queue WHERE run_id = $1`, low.ID).Scan(&claimPriority); err != nil {
		t.Fatalf("query claim priority: %v", err)
	}
	if claimPriority != 100 {
		t.Fatalf("claim priority = %d, want 100", claimPriority)
	}

	batch, err := q.DequeueNClaim(ctx, 1)
	if err != nil {
		t.Fatalf("DequeueNClaim: %v", err)
	}
	if len(batch) != 1 || batch[0].ID != low.ID {
		t.Fatalf("priority dequeue = %+v, want reprioritized run %s", batch, low.ID)
	}
}

func TestClaimTable_PerformanceSchemaGuards(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	mustClean(t, ctx)

	var opts []string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT reloptions
		FROM pg_class
		WHERE relname = 'job_run_queue'
	`).Scan(&opts); err != nil {
		t.Fatalf("query job_run_queue reloptions: %v", err)
	}
	wantOptions := []string{
		"autovacuum_vacuum_threshold=50",
		"autovacuum_vacuum_scale_factor=0.005",
		"autovacuum_vacuum_cost_delay=0",
		"autovacuum_vacuum_cost_limit=2000",
		"autovacuum_analyze_threshold=50",
		"autovacuum_analyze_scale_factor=0.005",
		"fillfactor=90",
	}
	gotOptions := strings.Join(opts, ",")
	for _, want := range wantOptions {
		if !strings.Contains(gotOptions, want) {
			t.Fatalf("job_run_queue reloptions missing %q; got %v", want, opts)
		}
	}

	indexes := map[string][]string{
		"idx_job_run_queue_dequeue": {
			"priority DESC",
			"created_at",
			"INCLUDE",
			"job_max_concurrency",
			"job_max_concurrency_per_key",
			"job_enabled",
			"job_paused",
		},
		"idx_job_run_queue_worker_routing": {
			"queue_name",
			"priority DESC",
			"WHERE (execution_mode = 'worker'::text)",
		},
	}
	for indexName, substrings := range indexes {
		var indexDef string
		if err := testDB.Pool.QueryRow(ctx, `
			SELECT indexdef
			FROM pg_indexes
			WHERE tablename = 'job_run_queue' AND indexname = $1
		`, indexName).Scan(&indexDef); err != nil {
			t.Fatalf("query index %s: %v", indexName, err)
		}
		for _, want := range substrings {
			if !strings.Contains(indexDef, want) {
				t.Fatalf("index %s definition missing %q:\n%s", indexName, want, indexDef)
			}
		}
	}
}

func TestClaimTable_PerformancePlansUseClaimIndexes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	q := mustQueue(t)

	httpJob := mustCreateJob(t, ctx, st, "project-claim-plan-http")
	for range 30 {
		mustEnqueueRun(t, ctx, q, httpJob)
	}

	workerJob := mustCreateJob(t, ctx, st, "project-claim-plan-worker")
	markWorkerJobQueue(t, ctx, workerJob, "priority")
	for range 30 {
		run := &domain.JobRun{
			ID:            newID(),
			JobID:         workerJob.ID,
			ProjectID:     workerJob.ProjectID,
			Priority:      10,
			ExecutionMode: domain.ExecutionModeWorker,
			QueueName:     "priority",
		}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("enqueue worker run: %v", err)
		}
	}
	if _, err := testDB.Pool.Exec(ctx, "ANALYZE job_run_queue"); err != nil {
		t.Fatalf("analyze job_run_queue: %v", err)
	}

	cases := []struct {
		name      string
		sql       string
		indexName string
		args      []any
	}{
		{
			name: "http claim scan",
			sql: `
				SELECT q.run_id
				FROM job_run_queue q
				WHERE COALESCE(q.job_enabled, true) = true
				  AND COALESCE(q.job_paused, false) = false
				  AND (q.scheduled_at IS NULL OR q.scheduled_at <= NOW())
				  AND COALESCE(q.execution_mode, 'http') = 'http'
				ORDER BY q.priority DESC, q.created_at ASC
				LIMIT 10`,
			indexName: "idx_job_run_queue_dequeue",
		},
		{
			name: "worker routing scan",
			sql: `
				SELECT q.run_id
				FROM job_run_queue q
				WHERE q.execution_mode = 'worker'
				  AND q.queue_name = $1
				ORDER BY q.priority DESC, q.created_at ASC
				LIMIT 10`,
			indexName: "idx_job_run_queue_worker_routing",
			args:      []any{"priority"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tx, err := testDB.Pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			defer tx.Rollback(ctx) //nolint:errcheck
			if _, err := tx.Exec(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
				t.Fatalf("disable seqscan: %v", err)
			}
			rows, err := tx.Query(ctx, "EXPLAIN (FORMAT TEXT) "+tc.sql, tc.args...)
			if err != nil {
				t.Fatalf("EXPLAIN: %v", err)
			}
			var plan strings.Builder
			for rows.Next() {
				var line string
				if err := rows.Scan(&line); err != nil {
					rows.Close()
					t.Fatalf("scan plan: %v", err)
				}
				plan.WriteString(line)
				plan.WriteByte('\n')
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				t.Fatalf("plan rows: %v", err)
			}
			if !strings.Contains(plan.String(), tc.indexName) {
				t.Fatalf("plan does not reference %s:\n%s", tc.indexName, plan.String())
			}
		})
	}
}
