//go:build integration && longtest

package queue_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/loadtest"
	"strait/internal/queue"
	"strait/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/sourcegraph/conc"
)

func TestQueueBloatBaseline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	report := runLegacyQueueBaseline(t, ctx, baselineConfig{
		Name:        "legacy_queue_baseline",
		ProjectID:   "project-queue-baseline",
		SingleRuns:  30,
		BatchRuns:   30,
		DequeueSize: 10,
	})

	if report.Counters.DuplicateClaims != 0 {
		t.Fatalf("duplicate claims = %d, want 0", report.Counters.DuplicateClaims)
	}
	if report.Counters.LostClaims != 0 {
		t.Fatalf("lost claims = %d, want 0", report.Counters.LostClaims)
	}
	if report.Counters.Dequeued == 0 {
		t.Fatal("expected baseline to dequeue runs")
	}
	writeQueueBaselineReport(t, report)
	t.Logf("queue baseline:\n%s", report.Markdown())
}

func TestQueueBloatComparison(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for _, cfg := range []baselineConfig{
		{
			Name:        "queue_bloat_comparison_60",
			ProjectID:   "project-queue-comparison-60",
			SingleRuns:  30,
			BatchRuns:   30,
			DequeueSize: 10,
		},
		{
			Name:        "queue_bloat_comparison_1000",
			ProjectID:   "project-queue-comparison-1000",
			SingleRuns:  500,
			BatchRuns:   500,
			DequeueSize: 50,
		},
	} {
		t.Run(cfg.Name, func(t *testing.T) {
			legacy := runLegacyQueueBaseline(t, ctx, cfg.withName("legacy_"+cfg.Name))
			batchlog := runBatchlogQueueBaseline(t, ctx, cfg.withName("batchlog_"+cfg.Name))
			pgque := runPgQueQueueBaseline(t, ctx, cfg.withName("pgque_"+cfg.Name))
			comparison := loadtest.CompareQueueBenchmarkReports(cfg.Name, legacy, batchlog)
			pgqueComparison := loadtest.CompareQueueBenchmarkReports("pgque_"+cfg.Name, legacy, pgque)

			if comparison.Candidate.Counters.DuplicateClaims != 0 {
				t.Fatalf("candidate duplicate claims = %d, want 0", comparison.Candidate.Counters.DuplicateClaims)
			}
			if comparison.Candidate.Counters.LostClaims != 0 {
				t.Fatalf("candidate lost claims = %d, want 0", comparison.Candidate.Counters.LostClaims)
			}
			if pgqueComparison.Candidate.Counters.DuplicateClaims != 0 {
				t.Fatalf("pgque duplicate claims = %d, want 0", pgqueComparison.Candidate.Counters.DuplicateClaims)
			}
			if pgqueComparison.Candidate.Counters.LostClaims != 0 {
				t.Fatalf("pgque lost claims = %d, want 0", pgqueComparison.Candidate.Counters.LostClaims)
			}
			writeQueueBaselineReport(t, legacy)
			writeQueueBaselineReport(t, batchlog)
			writeQueueBaselineReport(t, pgque)
			writeQueueComparisonReport(t, comparison)
			writeQueueComparisonReport(t, pgqueComparison)
			t.Logf("queue bloat comparison:\n%s", comparison.Markdown())
			t.Logf("pgque bloat comparison:\n%s", pgqueComparison.Markdown())
		})
	}
}

func BenchmarkQueueBaseline(b *testing.B) {
	for _, size := range []int{64, 256} {
		b.Run(fmt.Sprintf("runs=%d", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				report := runLegacyQueueBaseline(b, ctx, baselineConfig{
					Name:        fmt.Sprintf("legacy_queue_baseline_%d", size),
					ProjectID:   fmt.Sprintf("project-queue-benchmark-%d-%d", size, i),
					SingleRuns:  size / 2,
					BatchRuns:   size / 2,
					DequeueSize: 32,
				})
				cancel()
				b.ReportMetric(float64(report.Counters.Dequeued), "runs/op")
				if report.Duration > 0 {
					b.ReportMetric(float64(report.Counters.Dequeued)/report.Duration.Seconds(), "runs/s")
				}
				b.ReportMetric(float64(report.Counters.NotifyCount), "notify/op")
				if report.Counters.DuplicateClaims > 0 || report.Counters.LostClaims > 0 {
					b.Fatalf("duplicates=%d lost=%d", report.Counters.DuplicateClaims, report.Counters.LostClaims)
				}
			}
		})
	}
}

type baselineConfig struct {
	Name        string
	ProjectID   string
	SingleRuns  int
	BatchRuns   int
	DequeueSize int
}

func (c baselineConfig) withName(name string) baselineConfig {
	c.Name = name
	c.ProjectID = c.ProjectID + "-" + name
	return c
}

type baselineTB interface {
	Helper()
	Fatalf(format string, args ...any)
	Logf(format string, args ...any)
}

type benchmarkQueue interface {
	Enqueue(context.Context, *domain.JobRun) error
	EnqueueBatch(context.Context, []*domain.JobRun) (int64, error)
	Dequeue(context.Context) (*domain.JobRun, error)
	DequeueN(context.Context, int) ([]domain.JobRun, error)
}

type queueBaselineHooks struct {
	afterEnqueue  func(context.Context)
	beforeDequeue func(context.Context)
	exerciseExtra func(baselineTB, context.Context, *store.Queries, *domain.Job) int64
	plans         func(baselineTB, context.Context) []loadtest.SQLPlanSample
}

func runLegacyQueueBaseline(tb baselineTB, ctx context.Context, cfg baselineConfig) loadtest.QueueBenchmarkReport {
	tb.Helper()
	q := mustQueueTB(tb)
	return runQueueBaseline(tb, ctx, cfg, "legacy", q, queueBaselineHooks{
		exerciseExtra: func(tb baselineTB, ctx context.Context, st *store.Queries, job *domain.Job) int64 {
			return exerciseRetryAndStalePaths(tb, ctx, q, st, job)
		},
		plans: sampleLegacyDequeuePlans,
	})
}

func runBatchlogQueueBaseline(tb baselineTB, ctx context.Context, cfg baselineConfig) loadtest.QueueBenchmarkReport {
	tb.Helper()
	if testDB == nil || testDB.Pool == nil {
		tb.Fatalf("testDB is not initialized")
	}
	q := queue.NewBatchlogQueue(testDB.Pool, queue.NewPostgresQueue(testDB.Pool), queue.BatchlogConfig{
		TickInterval:  10 * time.Millisecond,
		LeaseDuration: 15 * time.Millisecond,
		LeaseOwner:    "bloat-comparison-" + newID(),
	})
	return runQueueBaseline(tb, ctx, cfg, "batchlog", q, queueBaselineHooks{
		afterEnqueue: func(ctx context.Context) {
			if _, err := q.SealDueBatches(ctx); err != nil {
				tb.Fatalf("batchlog seal due batches: %v", err)
			}
		},
		exerciseExtra: func(tb baselineTB, ctx context.Context, _ *store.Queries, job *domain.Job) int64 {
			return exerciseBatchlogLeaseRedelivery(tb, ctx, q, job)
		},
		plans: sampleBatchlogDequeuePlans,
	})
}

func runPgQueQueueBaseline(tb baselineTB, ctx context.Context, cfg baselineConfig) loadtest.QueueBenchmarkReport {
	tb.Helper()
	if testDB == nil || testDB.Pool == nil {
		tb.Fatalf("testDB is not initialized")
	}
	q := queue.NewPgQueQueue(testDB.Pool, queue.NewPostgresQueue(testDB.Pool), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "bloat-comparison-" + newID(),
		NackDelay:     10 * time.Millisecond,
		ReceiveWindow: 100,
	})
	return runQueueBaseline(tb, ctx, cfg, "pgque", q, queueBaselineHooks{
		afterEnqueue: func(ctx context.Context) {
			if err := q.ForceTick(ctx, "http"); err != nil {
				tb.Fatalf("pgque force tick: %v", err)
			}
		},
		beforeDequeue: func(ctx context.Context) {
			if err := q.ForceTick(ctx, "http"); err != nil {
				tb.Fatalf("pgque force tick: %v", err)
			}
		},
		plans: samplePgQueDequeuePlans,
	})
}

func runQueueBaseline(
	tb baselineTB,
	ctx context.Context,
	cfg baselineConfig,
	engine string,
	q benchmarkQueue,
	hooks queueBaselineHooks,
) loadtest.QueueBenchmarkReport {
	tb.Helper()

	started := time.Now()
	mustCleanTB(tb, ctx)
	st := mustStoreTB(tb)
	job := mustCreateJobTB(tb, ctx, st, cfg.ProjectID)

	notifyCtx, stopNotify := context.WithCancel(ctx)
	notifyCount, waitNotify := startNotifyCounter(tb, notifyCtx)
	defer waitNotify()
	defer stopNotify()

	beforeWAL := sampleWALBytes(ctx)
	for i := 0; i < cfg.SingleRuns; i++ {
		run := &domain.JobRun{
			ID:        newID(),
			JobID:     job.ID,
			ProjectID: job.ProjectID,
			Priority:  i % 10,
			Payload:   json.RawMessage(`{"source":"single"}`),
		}
		if err := q.Enqueue(ctx, run); err != nil {
			tb.Fatalf("Enqueue() error = %v", err)
		}
	}

	batchRuns := make([]*domain.JobRun, cfg.BatchRuns)
	for i := range batchRuns {
		batchRuns[i] = &domain.JobRun{
			ID:        newID(),
			JobID:     job.ID,
			ProjectID: job.ProjectID,
			Priority:  i % 10,
			Payload:   json.RawMessage(`{"source":"batch"}`),
		}
	}
	inserted, err := q.EnqueueBatch(ctx, batchRuns)
	if err != nil {
		tb.Fatalf("EnqueueBatch() error = %v", err)
	}
	if inserted != int64(len(batchRuns)) {
		tb.Fatalf("EnqueueBatch inserted = %d, want %d", inserted, len(batchRuns))
	}
	if hooks.afterEnqueue != nil {
		hooks.afterEnqueue(ctx)
	}
	plans := samplePlans(tb, ctx, hooks)

	totalRuns := cfg.SingleRuns + cfg.BatchRuns
	claimed := sync.Map{}
	latencies := make([]time.Duration, 0, totalRuns/cfg.DequeueSize+2)
	var duplicates atomic.Int64
	var dequeued int64
	for dequeued < int64(totalRuns) {
		if hooks.beforeDequeue != nil {
			hooks.beforeDequeue(ctx)
		}
		start := time.Now()
		runs, err := q.DequeueN(ctx, cfg.DequeueSize)
		latencies = append(latencies, time.Since(start))
		if err != nil {
			tb.Fatalf("DequeueN() error = %v", err)
		}
		if len(runs) == 0 {
			break
		}
		for _, run := range runs {
			if _, loaded := claimed.LoadOrStore(run.ID, true); loaded {
				duplicates.Add(1)
			}
			fromStatus := run.Status
			if fromStatus == "" {
				fromStatus = domain.StatusDequeued
			}
			if err := st.UpdateRunStatus(ctx, run.ID, fromStatus, domain.StatusExecuting, map[string]any{
				"started_at": time.Now(),
			}); err != nil {
				tb.Fatalf("UpdateRunStatus %s->executing: %v", fromStatus, err)
			}
			if err := st.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
				"finished_at": time.Now(),
				"result":      json.RawMessage(`{"ok":true}`),
			}); err != nil {
				tb.Fatalf("UpdateRunStatus executing->completed: %v", err)
			}
			dequeued++
		}
	}

	var retryRedelivery int64
	if hooks.exerciseExtra != nil {
		retryRedelivery = hooks.exerciseExtra(tb, ctx, st, job)
	}
	time.Sleep(250 * time.Millisecond)
	stopNotify()

	relations := sampleRelationBloat(tb, ctx)
	afterWAL := sampleWALBytes(ctx)
	lost := int64(totalRuns) - dequeued
	if lost < 0 {
		lost = 0
	}
	return loadtest.QueueBenchmarkReport{
		Name:      cfg.Name,
		Engine:    engine,
		StartedAt: started,
		Duration:  time.Since(started),
		Counters: loadtest.QueueBenchmarkCounters{
			Enqueued:        int64(totalRuns),
			Dequeued:        dequeued,
			Completed:       dequeued,
			RetryRedelivery: retryRedelivery,
			DuplicateClaims: duplicates.Load(),
			LostClaims:      lost,
			NotifyCount:     notifyCount.Load(),
			WALBytes:        maxInt64(0, afterWAL-beforeWAL),
		},
		DequeueLatency: loadtest.SummarizeLatencies(latencies),
		Relations:      relations,
		Plans:          plans,
	}
}

func exerciseRetryAndStalePaths(tb baselineTB, ctx context.Context, q interface {
	Enqueue(context.Context, *domain.JobRun) error
	Dequeue(context.Context) (*domain.JobRun, error)
}, st interface {
	ListStaleDequeued(context.Context, time.Duration) ([]domain.JobRun, error)
}, job *domain.Job) int64 {
	tb.Helper()
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(ctx, run); err != nil {
		tb.Fatalf("retry/stale enqueue: %v", err)
	}
	claimed, err := q.Dequeue(ctx)
	if err != nil {
		tb.Fatalf("retry/stale dequeue: %v", err)
	}
	if claimed == nil {
		tb.Fatalf("retry/stale dequeue returned nil")
		return 0
	}
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET started_at = NOW() - INTERVAL '10 minutes' WHERE id = $1`, claimed.ID); err != nil {
		tb.Fatalf("make stale dequeued: %v", err)
	}
	stale, err := st.ListStaleDequeued(ctx, time.Minute)
	if err != nil {
		tb.Fatalf("ListStaleDequeued: %v", err)
	}
	if len(stale) == 0 {
		tb.Fatalf("expected stale dequeued run")
	}
	// This is the legacy SKIP LOCKED baseline, so requeue through the old
	// ledger-backed path to keep the baseline behavior measurable after the
	// production transition path moved to job_run_state.
	if _, err := testDB.Pool.Exec(ctx, `UPDATE job_runs SET status = 'queued', started_at = NULL WHERE id = $1`, claimed.ID); err != nil {
		tb.Fatalf("requeue stale dequeued: %v", err)
	}
	redelivered, err := q.Dequeue(ctx)
	if err != nil {
		tb.Fatalf("redeliver stale dequeued: %v", err)
	}
	if redelivered == nil || redelivered.ID != claimed.ID {
		tb.Fatalf("redelivered = %+v, want %s", redelivered, claimed.ID)
	}
	return 1
}

func exerciseBatchlogLeaseRedelivery(tb baselineTB, ctx context.Context, q *queue.BatchlogQueue, job *domain.Job) int64 {
	tb.Helper()
	run := &domain.JobRun{ID: newID(), JobID: job.ID, ProjectID: job.ProjectID}
	if err := q.Enqueue(ctx, run); err != nil {
		tb.Fatalf("batchlog redelivery enqueue: %v", err)
	}
	if _, err := q.SealDueBatches(ctx); err != nil {
		tb.Fatalf("batchlog redelivery seal: %v", err)
	}
	claimed, err := q.Dequeue(ctx)
	if err != nil {
		tb.Fatalf("batchlog redelivery dequeue: %v", err)
	}
	if claimed == nil {
		tb.Fatalf("batchlog redelivery dequeue returned nil")
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := q.ReclaimExpiredLeases(ctx); err != nil {
		tb.Fatalf("batchlog redelivery reclaim: %v", err)
	}
	redelivered, err := q.Dequeue(ctx)
	if err != nil {
		tb.Fatalf("batchlog redelivery second dequeue: %v", err)
	}
	if redelivered == nil || redelivered.ID != claimed.ID {
		tb.Fatalf("batchlog redelivered = %+v, want %s", redelivered, claimed.ID)
	}
	return 1
}

func startNotifyCounter(tb baselineTB, ctx context.Context) (*atomic.Int64, func()) {
	tb.Helper()
	count := &atomic.Int64{}
	conn, err := pgx.Connect(ctx, testDB.ConnStr)
	if err != nil {
		tb.Fatalf("connect notify counter: %v", err)
	}
	if _, err := conn.Exec(ctx, "LISTEN strait_queue_wake"); err != nil {
		_ = conn.Close(context.Background())
		tb.Fatalf("listen queue wake: %v", err)
	}
	var concWG conc.WaitGroup
	concWG.Go(func() {
		defer conn.Close(context.Background()) //nolint:errcheck
		for {
			_, err := conn.WaitForNotification(ctx)
			if err != nil {
				return
			}
			count.Add(1)
		}
	})
	time.Sleep(100 * time.Millisecond)
	return count, concWG.Wait
}

func sampleRelationBloat(tb baselineTB, ctx context.Context) []loadtest.RelationBloatSample {
	tb.Helper()
	rows, err := testDB.Pool.Query(ctx, `
		SELECT
			s.relname,
			COALESCE(s.n_live_tup, 0),
			COALESCE(s.n_dead_tup, 0),
			COALESCE(s.n_tup_upd, 0),
			COALESCE(s.n_tup_hot_upd, 0),
			pg_relation_size(c.oid),
			pg_indexes_size(c.oid),
			pg_total_relation_size(c.oid)
		FROM pg_stat_user_tables s
		JOIN pg_class c ON c.relname = s.relname
		WHERE s.relname IN (
			'job_runs',
			'job_active_counts',
			'job_retries',
			'queue_entries',
			'queue_batches',
			'queue_batch_ticks',
			'queue_batch_seal_state',
			'enqueue_outbox',
			'enqueue_outbox_history',
			'outbox_claims',
			'outbox_batches',
			'workflow_step_runs',
			'workflow_progression_events',
			'event_triggers'
		)
		   OR s.relname LIKE 'job_runs_%'
		   OR s.relname = 'job_run_state'
		   OR s.relname = 'job_run_lifecycle_events'
		   OR s.relname = 'strait_pgque_routes'
		   OR s.relname LIKE 'event_%'
		   OR s.relname LIKE 'enqueue_outbox_%'
		   OR s.relname LIKE 'enqueue_outbox_history_%'
		ORDER BY s.relname ASC
	`)
	if err != nil {
		tb.Fatalf("sample relation bloat: %v", err)
	}
	defer rows.Close()

	out := make([]loadtest.RelationBloatSample, 0, 8)
	for rows.Next() {
		var sample loadtest.RelationBloatSample
		if err := rows.Scan(
			&sample.Name,
			&sample.LiveTuples,
			&sample.DeadTuples,
			&sample.TotalUpdates,
			&sample.HOTUpdates,
			&sample.RelationSize,
			&sample.TotalIndexSize,
			&sample.TotalTableSize,
		); err != nil {
			tb.Fatalf("scan relation bloat: %v", err)
		}
		out = append(out, sample)
	}
	if err := rows.Err(); err != nil {
		tb.Fatalf("relation bloat rows: %v", err)
	}
	return out
}

func samplePlans(tb baselineTB, ctx context.Context, hooks queueBaselineHooks) []loadtest.SQLPlanSample {
	tb.Helper()
	if hooks.plans == nil {
		return nil
	}
	return hooks.plans(tb, ctx)
}

func sampleLegacyDequeuePlans(tb baselineTB, ctx context.Context) []loadtest.SQLPlanSample {
	tb.Helper()
	return []loadtest.SQLPlanSample{{
		Name: "legacy candidate selection",
		Lines: explainText(tb, ctx, `
			EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
			SELECT jr.id
			FROM job_runs jr
			LEFT JOIN job_active_counts jac_job
			  ON jac_job.job_id = jr.job_id AND jac_job.concurrency_key = ''
			LEFT JOIN job_active_counts jac_key
			  ON jac_key.job_id = jr.job_id
			  AND jac_key.concurrency_key = COALESCE(jr.concurrency_key, '')
			WHERE jr.status = 'queued'
			  AND COALESCE(jr.job_enabled, true) = true
			  AND COALESCE(jr.job_paused, false) = false
			  AND (jr.scheduled_at IS NULL OR jr.scheduled_at <= NOW())
			  AND (jr.next_retry_at IS NULL OR jr.next_retry_at <= NOW())
			  AND (jr.job_max_concurrency IS NULL OR COALESCE(jac_job.count, 0) < jr.job_max_concurrency)
			  AND (jr.job_max_concurrency_per_key IS NULL
			       OR jr.concurrency_key IS NULL
			       OR jr.concurrency_key = ''
			       OR COALESCE(jac_key.count, 0) < jr.job_max_concurrency_per_key)
			ORDER BY jr.priority DESC, jr.created_at ASC
			LIMIT 50
		`),
	}}
}

func sampleBatchlogDequeuePlans(tb baselineTB, ctx context.Context) []loadtest.SQLPlanSample {
	tb.Helper()
	return []loadtest.SQLPlanSample{{
		Name: "batchlog candidate selection",
		Lines: explainText(tb, ctx, `
			EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
			WITH leased_job_counts AS (
				SELECT leased.job_id, COUNT(*)::int AS count
				FROM queue_entries leased
				WHERE leased.status = 'leased'
				  AND leased.run_status = 'queued'
				GROUP BY leased.job_id
			),
			leased_key_counts AS (
				SELECT leased.job_id, leased.concurrency_key, COUNT(*)::int AS count
				FROM queue_entries leased
				WHERE leased.status = 'leased'
				  AND leased.run_status = 'queued'
				  AND leased.concurrency_key <> ''
				GROUP BY leased.job_id, leased.concurrency_key
			)
			SELECT qe.run_id
			FROM queue_entries qe
			LEFT JOIN job_active_counts jac_job
			  ON jac_job.job_id = qe.job_id AND jac_job.concurrency_key = ''
			LEFT JOIN job_active_counts jac_key
			  ON jac_key.job_id = qe.job_id
			  AND jac_key.concurrency_key = qe.concurrency_key
			LEFT JOIN leased_job_counts leased_job
			  ON leased_job.job_id = qe.job_id
			LEFT JOIN leased_key_counts leased_key
			  ON leased_key.job_id = qe.job_id
			  AND leased_key.concurrency_key = qe.concurrency_key
			WHERE qe.status = 'ready'
			  AND qe.batch_id IS NOT NULL
			  AND qe.available_at <= NOW()
			  AND qe.run_status = 'queued'
			  AND COALESCE(qe.job_enabled, true) = true
			  AND COALESCE(qe.job_paused, false) = false
			  AND (qe.scheduled_at IS NULL OR qe.scheduled_at <= NOW())
			  AND (qe.next_retry_at IS NULL OR qe.next_retry_at <= NOW())
			  AND (qe.job_max_concurrency IS NULL
			       OR COALESCE(jac_job.count, 0) + COALESCE(leased_job.count, 0) < qe.job_max_concurrency)
			  AND (qe.job_max_concurrency_per_key IS NULL
			       OR qe.concurrency_key = ''
			       OR COALESCE(jac_key.count, 0) + COALESCE(leased_key.count, 0) < qe.job_max_concurrency_per_key)
			ORDER BY qe.batch_id ASC, qe.priority DESC, qe.run_created_at ASC
			LIMIT 50
		`),
	}}
}

func samplePgQueDequeuePlans(tb baselineTB, ctx context.Context) []loadtest.SQLPlanSample {
	tb.Helper()
	return []loadtest.SQLPlanSample{{
		Name: "pgque state candidate selection",
		Lines: explainText(tb, ctx, `
			EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
			SELECT s.run_id
			FROM job_run_state s
			JOIN job_runs jr ON jr.id = s.run_id
			WHERE s.status = 'queued'
			  AND s.ready_generation = 0
			  AND s.execution_mode = 'http'
			  AND COALESCE(s.job_enabled, true) = true
			  AND COALESCE(s.job_paused, false) = false
			  AND (s.scheduled_at IS NULL OR s.scheduled_at <= NOW())
			  AND (s.next_retry_at IS NULL OR s.next_retry_at <= NOW())
			ORDER BY s.priority DESC, jr.created_at ASC
			LIMIT 50
		`),
	}}
}

func explainText(tb baselineTB, ctx context.Context, query string) []string {
	tb.Helper()
	rows, err := testDB.Pool.Query(ctx, query)
	if err != nil {
		tb.Fatalf("explain query: %v", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			tb.Fatalf("scan explain: %v", err)
		}
		out = append(out, line)
	}
	if err := rows.Err(); err != nil {
		tb.Fatalf("explain rows: %v", err)
	}
	return out
}

func sampleWALBytes(ctx context.Context) int64 {
	var walBytes int64
	_ = testDB.Pool.QueryRow(ctx, `SELECT COALESCE(wal_bytes, 0)::bigint FROM pg_stat_wal`).Scan(&walBytes)
	return walBytes
}

func writeQueueBaselineReport(tb baselineTB, report loadtest.QueueBenchmarkReport) {
	tb.Helper()
	dir := filepath.Join("loadtest-results", "queue-baseline")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		tb.Fatalf("create report dir: %v", err)
	}
	jsonPath := filepath.Join(dir, report.Name+".json")
	markdownPath := filepath.Join(dir, report.Name+".md")
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		tb.Fatalf("marshal report: %v", err)
	}
	if err := os.WriteFile(jsonPath, data, 0o600); err != nil {
		tb.Fatalf("write report json: %v", err)
	}
	if err := os.WriteFile(markdownPath, []byte(report.Markdown()), 0o600); err != nil {
		tb.Fatalf("write report markdown: %v", err)
	}
}

func writeQueueComparisonReport(tb baselineTB, comparison loadtest.QueueBenchmarkComparison) {
	tb.Helper()
	dir := filepath.Join("loadtest-results", "queue-baseline")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		tb.Fatalf("create report dir: %v", err)
	}
	jsonPath := filepath.Join(dir, comparison.Name+".json")
	markdownPath := filepath.Join(dir, comparison.Name+".md")
	data, err := json.MarshalIndent(comparison, "", "  ")
	if err != nil {
		tb.Fatalf("marshal comparison report: %v", err)
	}
	if err := os.WriteFile(jsonPath, data, 0o600); err != nil {
		tb.Fatalf("write comparison report json: %v", err)
	}
	if err := os.WriteFile(markdownPath, []byte(comparison.Markdown()), 0o600); err != nil {
		tb.Fatalf("write comparison report markdown: %v", err)
	}
}

func mustQueueTB(tb baselineTB) *queue.PostgresQueue {
	tb.Helper()
	if testDB == nil || testDB.Pool == nil {
		tb.Fatalf("testDB is not initialized")
	}
	return queue.NewPostgresQueue(testDB.Pool)
}

func mustStoreTB(tb baselineTB) *store.Queries {
	tb.Helper()
	if testDB == nil || testDB.Pool == nil {
		tb.Fatalf("testDB is not initialized")
	}
	return store.New(testDB.Pool)
}

func mustCleanTB(tb baselineTB, ctx context.Context) {
	tb.Helper()
	if err := testDB.CleanTables(ctx); err != nil {
		tb.Fatalf("CleanTables() error = %v", err)
	}
}

func mustCreateJobTB(tb baselineTB, ctx context.Context, st *store.Queries, projectID string) *domain.Job {
	tb.Helper()
	job := &domain.Job{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "job-" + newID(),
		Slug:        "slug-" + newID(),
		EndpointURL: "https://example.com/queue-job",
		MaxAttempts: 3,
		TimeoutSecs: 300,
		Enabled:     true,
	}
	if err := st.CreateJob(ctx, job); err != nil {
		tb.Fatalf("CreateJob() error = %v", err)
	}
	return job
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
