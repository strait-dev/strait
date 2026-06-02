//go:build longtest && integration

package queue_test

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/sourcegraph/conc"
)

// Queue Health Benchmark.
//
// Measures the Postgres queue's health under sustained load, capturing
// the exact metrics that predict the MVCC/bloat death spiral described
// in Brandur's 2015 post and PlanetScale's 2026 analysis.
//
// Metrics collected per snapshot (every 5s):
//   - Dead tuples (from pg_stat_user_tables)
//   - Live tuples
//   - Dead-to-live ratio
//   - HOT update ratio
//   - Dequeue P50/P95/P99 latency
//   - Enqueue throughput (rows/sec)
//   - Dequeue throughput (rows/sec)
//   - Oldest queued row age
//   - Index dead items (from pgstatindex where available)
//
// Run:
//
//	go test -tags=longtest,integration -run TestQueueHealthBench -v ./internal/queue/...
//	BENCH_DURATION=5m go test -tags=longtest,integration -run TestQueueHealthBench -v ./internal/queue/...
//	BENCH_WORKERS=40 BENCH_ENQUEUE_RATE=200 go test ...

// healthSnapshot holds one point-in-time measurement of queue health.
type healthSnapshot struct {
	Timestamp       time.Time
	ElapsedSec      float64
	DeadTuples      int64
	LiveTuples      int64
	DeadTupleRatio  float64
	TotalUpdates    int64
	HotUpdates      int64
	HotUpdateRatio  float64
	OldestQueuedAge float64
	EnqueuedTotal   int64
	DequeuedTotal   int64
	EnqueueRate     float64 // rows/sec since last snapshot
	DequeueRate     float64
	DequeueP50us    int64 // microseconds
	DequeueP95us    int64
	DequeueP99us    int64
	DequeueMaxUs    int64
	IndexDeadItems  int64 // -1 if pgstatindex not available
	SlotWalLagBytes int64
	Relations       []healthRelationSnapshot
}

type healthRelationSnapshot struct {
	Name            string
	DeadTuples      int64
	LiveTuples      int64
	TotalUpdates    int64
	HotUpdates      int64
	DeadTupleRatio  float64
	HotUpdateRatio  float64
	TotalTableBytes int64
	TotalIndexBytes int64
}

// benchConfig controls the benchmark parameters.
type benchConfig struct {
	QueueEngine     string
	Duration        time.Duration
	Workers         int
	BatchSize       int
	EnqueueRateHz   int // enqueue operations per second (each inserts BatchSize runs)
	SnapshotEvery   time.Duration
	UseDenormalized bool
	UseCursor       bool
	UseTwoPhase     bool
	UseClaimTable   bool
}

func defaultBenchConfig() benchConfig {
	return benchConfig{
		QueueEngine:   queue.EngineLegacy,
		Duration:      2 * time.Minute,
		Workers:       20,
		BatchSize:     5,
		EnqueueRateHz: 50,
		SnapshotEvery: 5 * time.Second,
	}
}

func benchConfigFromEnv() benchConfig {
	cfg := defaultBenchConfig()
	if s := os.Getenv("BENCH_QUEUE_ENGINE"); s != "" {
		cfg.QueueEngine = s
	}
	if s := os.Getenv("BENCH_DURATION"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			cfg.Duration = d
		}
	}
	if s := os.Getenv("BENCH_WORKERS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			cfg.Workers = n
		}
	}
	if s := os.Getenv("BENCH_BATCH_SIZE"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			cfg.BatchSize = n
		}
	}
	if s := os.Getenv("BENCH_ENQUEUE_RATE"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			cfg.EnqueueRateHz = n
		}
	}
	if os.Getenv("BENCH_USE_DENORMALIZED") == "true" {
		cfg.UseDenormalized = true
	}
	if os.Getenv("BENCH_USE_CURSOR") == "true" {
		cfg.UseCursor = true
	}
	if os.Getenv("BENCH_USE_TWO_PHASE") == "true" {
		cfg.UseTwoPhase = true
	}
	if os.Getenv("BENCH_USE_CLAIM_TABLE") == "true" {
		cfg.UseClaimTable = true
	}
	return cfg
}

type healthBenchQueue struct {
	engine        string
	enqueue       func(context.Context, *domain.JobRun) error
	dequeue       func(context.Context, int) ([]domain.JobRun, error)
	afterEnqueue  func(context.Context) error
	beforeDequeue func(context.Context) error
}

func mustHealthBenchQueue(t *testing.T, cfg benchConfig) healthBenchQueue {
	t.Helper()
	if testDB == nil || testDB.Pool == nil {
		t.Fatal("testDB is not initialized")
	}
	legacy := queue.NewPostgresQueue(testDB.Pool)
	switch cfg.QueueEngine {
	case "", queue.EngineLegacy:
		dequeue := func(ctx context.Context, n int) ([]domain.JobRun, error) {
			switch {
			case cfg.UseClaimTable:
				return legacy.DequeueNClaim(ctx, n)
			case cfg.UseTwoPhase:
				return legacy.DequeueNTwoPhase(ctx, n)
			case cfg.UseDenormalized:
				return legacy.DequeueNDenormalized(ctx, n)
			case cfg.UseCursor:
				return nil, fmt.Errorf("cursor dequeue requires per-worker cursor")
			default:
				return legacy.DequeueNClaim(ctx, n)
			}
		}
		return healthBenchQueue{engine: queue.EngineLegacy, enqueue: legacy.Enqueue, dequeue: dequeue}
	case queue.EngineBatchlog:
		q := queue.NewBatchlogQueue(testDB.Pool, legacy, queue.BatchlogConfig{
			TickInterval:  10 * time.Millisecond,
			LeaseDuration: 30 * time.Second,
			LeaseOwner:    "health-bench-" + newID(),
		})
		seal := func(ctx context.Context) error {
			_, err := q.SealDueBatches(ctx)
			return err
		}
		return healthBenchQueue{engine: queue.EngineBatchlog, enqueue: q.Enqueue, dequeue: q.DequeueN, afterEnqueue: seal, beforeDequeue: seal}
	case queue.EnginePgQue:
		q := queue.NewPgQueQueue(testDB.Pool, legacy, queue.PgQueConfig{
			TickInterval:  50 * time.Millisecond,
			ConsumerName:  "health-bench-" + newID(),
			NackDelay:     10 * time.Millisecond,
			ReceiveWindow: 100,
		})
		return healthBenchQueue{engine: queue.EnginePgQue, enqueue: q.Enqueue, dequeue: q.DequeueN}
	default:
		t.Fatalf("unknown BENCH_QUEUE_ENGINE %q", cfg.QueueEngine)
		return healthBenchQueue{}
	}
}

func completeHealthBenchRun(ctx context.Context, st *store.Queries, run domain.JobRun) error {
	from := run.Status
	if from == "" {
		from = domain.StatusDequeued
	}
	switch from {
	case domain.StatusQueued:
		if err := st.UpdateRunStatus(ctx, run.ID, domain.StatusQueued, domain.StatusExecuting, map[string]any{
			"started_at": time.Now(),
		}); err != nil {
			return err
		}
		from = domain.StatusExecuting
	case domain.StatusDequeued:
		if err := st.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{
			"started_at": time.Now(),
		}); err != nil {
			return err
		}
		from = domain.StatusExecuting
	}
	return st.UpdateRunStatus(ctx, run.ID, from, domain.StatusCompleted, map[string]any{
		"finished_at": time.Now(),
	})
}

// latencyRecorder is a sharded, lock-striped latency sample collector.
type latencyRecorder struct {
	shards [16]struct {
		mu      sync.Mutex
		samples []int64
	}
}

func (r *latencyRecorder) record(workerID int, us int64) {
	s := &r.shards[workerID%len(r.shards)]
	s.mu.Lock()
	s.samples = append(s.samples, us)
	s.mu.Unlock()
}

func (r *latencyRecorder) drain() []int64 {
	var all []int64
	for i := range r.shards {
		r.shards[i].mu.Lock()
		all = append(all, r.shards[i].samples...)
		r.shards[i].samples = r.shards[i].samples[:0]
		r.shards[i].mu.Unlock()
	}
	slices.SortFunc(all, cmp.Compare[int64])
	return all
}

func pct(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := max(int(math.Ceil(float64(len(sorted))*p/100.0))-1, 0)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// snapshotCollector gathers point-in-time queue stats from the live PG instance.
type snapshotCollector struct {
	ctx          context.Context
	enqueued     *atomic.Int64
	dequeued     *atomic.Int64
	latencies    *latencyRecorder
	startTime    time.Time
	lastSnapTime time.Time
	lastEnqueued int64
	lastDequeued int64
}

func (c *snapshotCollector) collect() healthSnapshot {
	now := time.Now()
	elapsed := now.Sub(c.startTime).Seconds()
	sinceLast := max(now.Sub(c.lastSnapTime).Seconds(), 0.001)

	snap := healthSnapshot{
		Timestamp:     now,
		ElapsedSec:    elapsed,
		EnqueuedTotal: c.enqueued.Load(),
		DequeuedTotal: c.dequeued.Load(),
	}
	snap.EnqueueRate = float64(snap.EnqueuedTotal-c.lastEnqueued) / sinceLast
	snap.DequeueRate = float64(snap.DequeuedTotal-c.lastDequeued) / sinceLast
	c.lastEnqueued = snap.EnqueuedTotal
	c.lastDequeued = snap.DequeuedTotal
	c.lastSnapTime = now

	lat := c.latencies.drain()
	snap.DequeueP50us = pct(lat, 50)
	snap.DequeueP95us = pct(lat, 95)
	snap.DequeueP99us = pct(lat, 99)
	if len(lat) > 0 {
		snap.DequeueMaxUs = lat[len(lat)-1]
	}

	rows, err := testDB.Pool.Query(c.ctx, `
		SELECT relname,
		       n_dead_tup,
		       n_live_tup,
		       n_tup_upd,
		       n_tup_hot_upd,
		       pg_total_relation_size(relid),
		       pg_indexes_size(relid)
		FROM pg_stat_user_tables
		WHERE relname = 'job_run_state'
		   OR relname = 'job_run_terminal_state'
		   OR relname = 'job_active_counts'
		   OR relname = 'job_run_lifecycle_events'
		   OR relname = 'queue_entries'
		   OR relname = 'strait_pgque_routes'
		   OR relname = 'job_runs'
		   OR relname LIKE 'job_runs_%'
		   OR relname = 'event_template'
		   OR relname ~ '^event_[0-9]+(_[0-9]+)?$'
		ORDER BY relname
	`)
	if err == nil {
		for rows.Next() {
			var rel healthRelationSnapshot
			_ = rows.Scan(
				&rel.Name,
				&rel.DeadTuples,
				&rel.LiveTuples,
				&rel.TotalUpdates,
				&rel.HotUpdates,
				&rel.TotalTableBytes,
				&rel.TotalIndexBytes,
			)
			if total := rel.LiveTuples + rel.DeadTuples; total > 0 {
				rel.DeadTupleRatio = float64(rel.DeadTuples) / float64(total)
			}
			if rel.TotalUpdates > 0 {
				rel.HotUpdateRatio = float64(rel.HotUpdates) / float64(rel.TotalUpdates)
			}
			snap.Relations = append(snap.Relations, rel)
			snap.DeadTuples += rel.DeadTuples
			snap.LiveTuples += rel.LiveTuples
			snap.TotalUpdates += rel.TotalUpdates
			snap.HotUpdates += rel.HotUpdates
		}
		rows.Close()
	}
	if total := snap.LiveTuples + snap.DeadTuples; total > 0 {
		snap.DeadTupleRatio = float64(snap.DeadTuples) / float64(total)
	}
	if snap.TotalUpdates > 0 {
		snap.HotUpdateRatio = float64(snap.HotUpdates) / float64(snap.TotalUpdates)
	}

	_ = testDB.Pool.QueryRow(c.ctx, `
		SELECT COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(created_at))), 0)
		FROM job_runs jr
		LEFT JOIN job_run_state s ON s.run_id = jr.id
		WHERE COALESCE(s.status, jr.status) = 'queued'
	`).Scan(&snap.OldestQueuedAge)

	snap.IndexDeadItems = -1
	_ = testDB.Pool.QueryRow(c.ctx, `
		SELECT COALESCE(dead_items, -1)
		FROM pgstatindex('idx_runs_queue_covering')
	`).Scan(&snap.IndexDeadItems)

	_ = testDB.Pool.QueryRow(c.ctx, `
		SELECT COALESCE(MAX(pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn)), 0)::bigint
		FROM pg_replication_slots
		WHERE slot_type = 'logical'
		  AND database = current_database()
		  AND restart_lsn IS NOT NULL
	`).Scan(&snap.SlotWalLagBytes)

	return snap
}

func TestQueueHealthBench(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	cfg := benchConfigFromEnv()
	if testing.Short() {
		t.Skip("short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration+60*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-health-bench")
	benchQ := mustHealthBenchQueue(t, cfg)

	if _, err := testDB.Pool.Exec(ctx, "ANALYZE job_runs, job_run_state"); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, "SELECT pg_stat_reset()"); err != nil {
		t.Logf("pg_stat_reset: %v (non-fatal)", err)
	}

	t.Logf("=== Queue Health Benchmark ===")
	t.Logf("Duration: %v | Engine: %s | Workers: %d | Batch: %d | Enqueue: %d runs/sec | Denorm: %v | Cursor: %v",
		cfg.Duration, benchQ.engine, cfg.Workers, cfg.BatchSize, cfg.EnqueueRateHz*cfg.BatchSize, cfg.UseDenormalized, cfg.UseCursor)

	var enqueued, dequeuedCount atomic.Int64
	var rec latencyRecorder

	collector := &snapshotCollector{
		ctx: ctx, enqueued: &enqueued, dequeued: &dequeuedCount,
		latencies: &rec, startTime: time.Now(), lastSnapTime: time.Now(),
	}

	// Producer.
	stopEnq := make(chan struct{})
	var producerWg sync.WaitGroup
	producerWg.Add(1)
	concWG.Go(func() {
		defer producerWg.Done()
		ticker := time.NewTicker(time.Second / time.Duration(cfg.EnqueueRateHz))
		defer ticker.Stop()
		for {
			select {
			case <-stopEnq:
				return
			case <-ticker.C:
				for range cfg.BatchSize {
					run := &domain.JobRun{
						ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 1,
					}
					if err := benchQ.enqueue(ctx, run); err == nil {
						enqueued.Add(1)
					}
				}
				if benchQ.afterEnqueue != nil {
					_ = benchQ.afterEnqueue(ctx)
				}
			}
		}
	})

	// Consumer workers.
	var cursor *queue.ClaimCursor
	var cursorQueue *queue.PostgresQueue
	if cfg.UseCursor {
		cursor = queue.NewClaimCursor(30 * time.Second)
		if benchQ.engine == queue.EngineLegacy {
			cursorQueue = queue.NewPostgresQueue(testDB.Pool)
		}
	}

	var workerWg sync.WaitGroup
	end := time.Now().Add(cfg.Duration)
	for w := range cfg.Workers {
		workerWg.Add(1)
		concWG.Go(func() {
			defer workerWg.Done()
			for time.Now().Before(end) {
				start := time.Now()
				var batch []domain.JobRun
				var err error
				if benchQ.beforeDequeue != nil {
					_ = benchQ.beforeDequeue(ctx)
				}
				if cursorQueue != nil {
					batch, err = cursorQueue.DequeueNWithCursor(ctx, cfg.BatchSize, cursor)
				} else {
					batch, err = benchQ.dequeue(ctx, cfg.BatchSize)
				}
				elapsed := time.Since(start).Microseconds()
				if err != nil {
					time.Sleep(10 * time.Millisecond)
					continue
				}
				if len(batch) > 0 {
					rec.record(w, elapsed)
				}
				for _, r := range batch {
					dequeuedCount.Add(1)
					_ = completeHealthBenchRun(ctx, st, r)
				}
				if len(batch) == 0 {
					time.Sleep(5 * time.Millisecond)
				}
			}
		})
	}

	// Snapshot loop.
	var snapshots []healthSnapshot
	snapTicker := time.NewTicker(cfg.SnapshotEvery)
	defer snapTicker.Stop()
	snapshots = append(snapshots, collector.collect())

	done := make(chan struct{})
	concWG.Go(func() { workerWg.Wait(); close(done) })

	for {
		select {
		case <-done:
			goto finished
		case <-snapTicker.C:
			snap := collector.collect()
			snapshots = append(snapshots, snap)
			t.Logf("[%5.0fs] dead=%6d live=%6d ratio=%.4f hot=%.2f enq/s=%.0f deq/s=%.0f p50=%dus p95=%dus p99=%dus oldest=%.1fs",
				snap.ElapsedSec, snap.DeadTuples, snap.LiveTuples,
				snap.DeadTupleRatio, snap.HotUpdateRatio,
				snap.EnqueueRate, snap.DequeueRate,
				snap.DequeueP50us, snap.DequeueP95us, snap.DequeueP99us,
				snap.OldestQueuedAge)
		}
	}

finished:
	close(stopEnq)
	producerWg.Wait()
	time.Sleep(1 * time.Second)
	if _, err := testDB.Pool.Exec(ctx, "SELECT pg_stat_clear_snapshot()"); err != nil {
		t.Logf("clear snapshot: %v (non-fatal)", err)
	}
	final := collector.collect()
	snapshots = append(snapshots, final)

	printReport(t, cfg, snapshots)

	if final.DeadTupleRatio > 0.50 {
		t.Errorf("CRITICAL: final dead tuple ratio %.4f exceeds 50%% threshold", final.DeadTupleRatio)
	}
	if final.DequeueP99us > 1_000_000 {
		t.Errorf("CRITICAL: P99 dequeue latency %dus exceeds 1s threshold", final.DequeueP99us)
	}

	writeResults(t, "queue_health_bench_results.json", map[string]any{
		"config": cfg, "snapshots": snapshots,
	})
}

// TestQueueHealthBench_WithLongTxn simulates the PlanetScale death spiral:
// sustained queue load + a long-running transaction that pins the xmin horizon.
func TestQueueHealthBench_WithLongTxn(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	cfg := benchConfigFromEnv()
	if testing.Short() {
		t.Skip("short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration+60*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-health-longtxn")
	benchQ := mustHealthBenchQueue(t, cfg)

	if _, err := testDB.Pool.Exec(ctx, "ANALYZE job_runs, job_run_state"); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	t.Logf("=== Queue Health Benchmark WITH LONG TRANSACTION (PlanetScale scenario) ===")
	t.Logf("Duration: %v | Engine: %s | Workers: %d | Enqueue: %d runs/sec",
		cfg.Duration, benchQ.engine, cfg.Workers, cfg.EnqueueRateHz*cfg.BatchSize)

	// Pin the xmin horizon with a long-running read transaction.
	longTx, err := testDB.Pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		t.Fatalf("begin long txn: %v", err)
	}
	if _, err := longTx.Exec(ctx, "SELECT count(*) FROM job_runs LEFT JOIN job_run_state ON job_run_state.run_id = job_runs.id"); err != nil {
		t.Fatalf("long txn read: %v", err)
	}
	t.Logf("Long transaction started (xmin pinned)")

	var enqueued, dequeuedCount atomic.Int64
	var rec latencyRecorder

	collector := &snapshotCollector{
		ctx: ctx, enqueued: &enqueued, dequeued: &dequeuedCount,
		latencies: &rec, startTime: time.Now(), lastSnapTime: time.Now(),
	}

	// Producer.
	stopEnq := make(chan struct{})
	var producerWg sync.WaitGroup
	producerWg.Add(1)
	concWG.Go(func() {
		defer producerWg.Done()
		ticker := time.NewTicker(time.Second / time.Duration(cfg.EnqueueRateHz))
		defer ticker.Stop()
		for {
			select {
			case <-stopEnq:
				return
			case <-ticker.C:
				for range cfg.BatchSize {
					run := &domain.JobRun{
						ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 1,
					}
					if err := benchQ.enqueue(ctx, run); err == nil {
						enqueued.Add(1)
					}
				}
				if benchQ.afterEnqueue != nil {
					_ = benchQ.afterEnqueue(ctx)
				}
			}
		}
	})

	// Workers.
	var workerWg sync.WaitGroup
	end := time.Now().Add(cfg.Duration)
	for w := range cfg.Workers {
		workerWg.Add(1)
		concWG.Go(func() {
			defer workerWg.Done()
			for time.Now().Before(end) {
				start := time.Now()
				if benchQ.beforeDequeue != nil {
					_ = benchQ.beforeDequeue(ctx)
				}
				batch, bErr := benchQ.dequeue(ctx, cfg.BatchSize)
				elapsed := time.Since(start).Microseconds()
				if bErr != nil {
					time.Sleep(10 * time.Millisecond)
					continue
				}
				if len(batch) > 0 {
					rec.record(w, elapsed)
				}
				for _, r := range batch {
					dequeuedCount.Add(1)
					_ = completeHealthBenchRun(ctx, st, r)
				}
				if len(batch) == 0 {
					time.Sleep(5 * time.Millisecond)
				}
			}
		})
	}

	// Snapshot loop with midpoint long-txn release.
	var snapshots []healthSnapshot
	snapTicker := time.NewTicker(cfg.SnapshotEvery)
	defer snapTicker.Stop()
	snapshots = append(snapshots, collector.collect())

	done := make(chan struct{})
	concWG.Go(func() { workerWg.Wait(); close(done) })

	midpoint := time.Now().Add(cfg.Duration / 2)
	longTxnReleased := false

	for {
		select {
		case <-done:
			goto finished2
		case <-snapTicker.C:
			if !longTxnReleased && time.Now().After(midpoint) {
				if err := longTx.Commit(ctx); err != nil {
					t.Logf("long txn commit: %v", err)
				}
				longTxnReleased = true
				t.Logf("[%.0fs] Long transaction RELEASED -- vacuum can now reclaim",
					time.Since(collector.startTime).Seconds())
			}
			snap := collector.collect()
			snapshots = append(snapshots, snap)
			t.Logf("[%5.0fs] dead=%6d live=%6d ratio=%.4f hot=%.2f enq/s=%.0f deq/s=%.0f p99=%dus",
				snap.ElapsedSec, snap.DeadTuples, snap.LiveTuples,
				snap.DeadTupleRatio, snap.HotUpdateRatio,
				snap.EnqueueRate, snap.DequeueRate,
				snap.DequeueP99us)
		}
	}

finished2:
	if !longTxnReleased {
		_ = longTx.Commit(ctx)
	}
	close(stopEnq)
	producerWg.Wait()
	time.Sleep(1 * time.Second)
	final := collector.collect()
	snapshots = append(snapshots, final)

	printReport(t, cfg, snapshots)

	writeResults(t, "queue_health_bench_longtxn_results.json", map[string]any{
		"config": cfg, "scenario": "long_transaction_xmin_pin", "snapshots": snapshots,
	})
}

// TestQueueHealthBench_WithLogicalSlot simulates a stalled logical
// replication consumer. The slot is created before queue load starts and is
// intentionally left unconsumed so Postgres must retain WAL from the slot's
// restart_lsn while the queue churns.
func TestQueueHealthBench_WithLogicalSlot(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	cfg := benchConfigFromEnv()
	if testing.Short() {
		t.Skip("short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration+60*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-health-logical-slot")
	benchQ := mustHealthBenchQueue(t, cfg)

	var walLevel string
	if err := testDB.Pool.QueryRow(ctx, "SHOW wal_level").Scan(&walLevel); err != nil {
		t.Fatalf("show wal_level: %v", err)
	}
	if walLevel != "logical" {
		t.Fatalf("wal_level = %q, want logical", walLevel)
	}

	slotName := fmt.Sprintf("strait_bench_%d", time.Now().UnixNano())
	if _, err := testDB.Pool.Exec(ctx,
		`SELECT pg_create_logical_replication_slot($1, 'pgoutput')`,
		slotName,
	); err != nil {
		t.Fatalf("create logical replication slot: %v", err)
	}
	defer func() {
		_, _ = testDB.Pool.Exec(context.Background(), `
			SELECT pg_drop_replication_slot(slot_name)
			FROM pg_replication_slots
			WHERE slot_name = $1
		`, slotName)
	}()

	if _, err := testDB.Pool.Exec(ctx, "ANALYZE job_runs, job_run_state"); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, "SELECT pg_stat_reset()"); err != nil {
		t.Logf("pg_stat_reset: %v (non-fatal)", err)
	}

	t.Logf("=== Queue Health Benchmark WITH STALLED LOGICAL SLOT ===")
	t.Logf("Duration: %v | Engine: %s | Workers: %d | Enqueue: %d runs/sec | Slot: %s",
		cfg.Duration, benchQ.engine, cfg.Workers, cfg.EnqueueRateHz*cfg.BatchSize, slotName)

	var enqueued, dequeuedCount atomic.Int64
	var rec latencyRecorder
	collector := &snapshotCollector{
		ctx: ctx, enqueued: &enqueued, dequeued: &dequeuedCount,
		latencies: &rec, startTime: time.Now(), lastSnapTime: time.Now(),
	}

	stopEnq := make(chan struct{})
	var producerWg sync.WaitGroup
	producerWg.Add(1)
	concWG.Go(func() {
		defer producerWg.Done()
		ticker := time.NewTicker(time.Second / time.Duration(cfg.EnqueueRateHz))
		defer ticker.Stop()
		for {
			select {
			case <-stopEnq:
				return
			case <-ticker.C:
				for range cfg.BatchSize {
					run := &domain.JobRun{
						ID: newID(), JobID: job.ID, ProjectID: job.ProjectID, Priority: 1,
					}
					if err := benchQ.enqueue(ctx, run); err == nil {
						enqueued.Add(1)
					}
				}
				if benchQ.afterEnqueue != nil {
					_ = benchQ.afterEnqueue(ctx)
				}
			}
		}
	})

	var workerWg sync.WaitGroup
	end := time.Now().Add(cfg.Duration)
	for w := range cfg.Workers {
		workerWg.Add(1)
		concWG.Go(func() {
			defer workerWg.Done()
			for time.Now().Before(end) {
				start := time.Now()
				if benchQ.beforeDequeue != nil {
					_ = benchQ.beforeDequeue(ctx)
				}
				batch, err := benchQ.dequeue(ctx, cfg.BatchSize)
				elapsed := time.Since(start).Microseconds()
				if err != nil {
					time.Sleep(10 * time.Millisecond)
					continue
				}
				if len(batch) > 0 {
					rec.record(w, elapsed)
				}
				for _, r := range batch {
					dequeuedCount.Add(1)
					_ = completeHealthBenchRun(ctx, st, r)
				}
				if len(batch) == 0 {
					time.Sleep(5 * time.Millisecond)
				}
			}
		})
	}

	var snapshots []healthSnapshot
	snapTicker := time.NewTicker(cfg.SnapshotEvery)
	defer snapTicker.Stop()
	snapshots = append(snapshots, collector.collect())

	done := make(chan struct{})
	concWG.Go(func() { workerWg.Wait(); close(done) })

	for {
		select {
		case <-done:
			goto finished
		case <-snapTicker.C:
			snap := collector.collect()
			snapshots = append(snapshots, snap)
			t.Logf("[%5.0fs] dead=%6d live=%6d ratio=%.4f hot=%.2f enq/s=%.0f deq/s=%.0f p99=%dus slot_wal_lag=%d",
				snap.ElapsedSec, snap.DeadTuples, snap.LiveTuples,
				snap.DeadTupleRatio, snap.HotUpdateRatio,
				snap.EnqueueRate, snap.DequeueRate,
				snap.DequeueP99us, snap.SlotWalLagBytes)
		}
	}

finished:
	close(stopEnq)
	producerWg.Wait()
	time.Sleep(1 * time.Second)
	if _, err := testDB.Pool.Exec(ctx, "SELECT pg_stat_clear_snapshot()"); err != nil {
		t.Logf("clear snapshot: %v (non-fatal)", err)
	}
	final := collector.collect()
	snapshots = append(snapshots, final)

	printReport(t, cfg, snapshots)

	if final.SlotWalLagBytes <= 0 {
		t.Errorf("expected stalled logical slot to accumulate WAL lag")
	}

	writeResults(t, "queue_health_bench_logical_slot_results.json", map[string]any{
		"config": cfg, "scenario": "logical_slot_wal_retention", "slot_name": slotName, "snapshots": snapshots,
	})
}

func printReport(t *testing.T, cfg benchConfig, snapshots []healthSnapshot) {
	t.Helper()
	if len(snapshots) == 0 {
		return
	}

	final := snapshots[len(snapshots)-1]

	var maxDead, maxLive, maxP99, maxSlotWalLag int64
	var maxDeadRatio, maxOldestAge, sumEnqRate, sumDeqRate float64
	dataPoints := 0

	for _, s := range snapshots {
		maxDead = max(maxDead, s.DeadTuples)
		maxLive = max(maxLive, s.LiveTuples)
		if s.DeadTupleRatio > maxDeadRatio {
			maxDeadRatio = s.DeadTupleRatio
		}
		if s.OldestQueuedAge > maxOldestAge {
			maxOldestAge = s.OldestQueuedAge
		}
		maxP99 = max(maxP99, s.DequeueP99us)
		maxSlotWalLag = max(maxSlotWalLag, s.SlotWalLagBytes)
		if s.EnqueueRate > 0 || s.DequeueRate > 0 {
			sumEnqRate += s.EnqueueRate
			sumDeqRate += s.DequeueRate
			dataPoints++
		}
	}

	avgEnqRate, avgDeqRate := 0.0, 0.0
	if dataPoints > 0 {
		avgEnqRate = sumEnqRate / float64(dataPoints)
		avgDeqRate = sumDeqRate / float64(dataPoints)
	}

	bloatTrend := "stable"
	if len(snapshots) >= 4 {
		first := snapshots[1]
		last := snapshots[len(snapshots)-2]
		if last.ElapsedSec > first.ElapsedSec {
			slope := float64(last.DeadTuples-first.DeadTuples) / (last.ElapsedSec - first.ElapsedSec)
			if slope > 10 {
				bloatTrend = fmt.Sprintf("GROWING (+%.0f dead/sec)", slope)
			} else if slope < -10 {
				bloatTrend = fmt.Sprintf("shrinking (%.0f dead/sec)", slope)
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("====================================================================\n")
	sb.WriteString("              QUEUE HEALTH BENCHMARK RESULTS\n")
	sb.WriteString("====================================================================\n")
	fmt.Fprintf(&sb, "  Duration:          %v\n", cfg.Duration)
	fmt.Fprintf(&sb, "  Queue engine:      %s\n", cfg.QueueEngine)
	fmt.Fprintf(&sb, "  Workers:           %d\n", cfg.Workers)
	fmt.Fprintf(&sb, "  Batch size:        %d\n", cfg.BatchSize)
	fmt.Fprintf(&sb, "  Target enqueue:    %d ops/sec (%d runs/sec)\n", cfg.EnqueueRateHz, cfg.EnqueueRateHz*cfg.BatchSize)
	fmt.Fprintf(&sb, "  Denormalized:      %v\n", cfg.UseDenormalized)
	fmt.Fprintf(&sb, "  Cursor:            %v\n", cfg.UseCursor)
	sb.WriteString("\n")
	sb.WriteString("---- Throughput ----\n")
	fmt.Fprintf(&sb, "  Total enqueued:    %d\n", final.EnqueuedTotal)
	fmt.Fprintf(&sb, "  Total dequeued:    %d\n", final.DequeuedTotal)
	fmt.Fprintf(&sb, "  Avg enqueue rate:  %.0f runs/sec\n", avgEnqRate)
	fmt.Fprintf(&sb, "  Avg dequeue rate:  %.0f runs/sec\n", avgDeqRate)
	sb.WriteString("\n")
	sb.WriteString("---- Dequeue Latency (claim batch) ----\n")
	fmt.Fprintf(&sb, "  Final P50:         %d us\n", final.DequeueP50us)
	fmt.Fprintf(&sb, "  Final P95:         %d us\n", final.DequeueP95us)
	fmt.Fprintf(&sb, "  Final P99:         %d us\n", final.DequeueP99us)
	fmt.Fprintf(&sb, "  Max P99:           %d us\n", maxP99)
	fmt.Fprintf(&sb, "  Max single:        %d us\n", final.DequeueMaxUs)
	sb.WriteString("\n")
	sb.WriteString("---- Dead Tuples (MVCC Bloat) ----\n")
	fmt.Fprintf(&sb, "  Final dead:        %d\n", final.DeadTuples)
	fmt.Fprintf(&sb, "  Final live:        %d\n", final.LiveTuples)
	fmt.Fprintf(&sb, "  Final ratio:       %.4f (%.1f%%)\n", final.DeadTupleRatio, final.DeadTupleRatio*100)
	fmt.Fprintf(&sb, "  Peak dead:         %d\n", maxDead)
	fmt.Fprintf(&sb, "  Peak ratio:        %.4f (%.1f%%)\n", maxDeadRatio, maxDeadRatio*100)
	fmt.Fprintf(&sb, "  Bloat trend:       %s\n", bloatTrend)
	sb.WriteString("\n")
	sb.WriteString("---- HOT Updates ----\n")
	fmt.Fprintf(&sb, "  Total updates:     %d\n", final.TotalUpdates)
	fmt.Fprintf(&sb, "  HOT updates:       %d\n", final.HotUpdates)
	fmt.Fprintf(&sb, "  HOT ratio:         %.4f (%.1f%%)\n", final.HotUpdateRatio, final.HotUpdateRatio*100)
	if len(final.Relations) > 0 {
		sb.WriteString("\n")
		sb.WriteString("---- Relation Bloat Breakdown ----\n")
		fmt.Fprintf(&sb, "  %-34s %-8s %-8s %-8s %-8s %-8s %-10s %-10s\n",
			"Relation", "Dead", "Live", "Ratio", "Updates", "HOT%", "Table", "Index")
		for _, rel := range final.Relations {
			fmt.Fprintf(&sb, "  %-34s %-8d %-8d %-8.4f %-8d %-8.1f %-10d %-10d\n",
				rel.Name,
				rel.DeadTuples,
				rel.LiveTuples,
				rel.DeadTupleRatio,
				rel.TotalUpdates,
				rel.HotUpdateRatio*100,
				rel.TotalTableBytes,
				rel.TotalIndexBytes,
			)
		}
	}
	sb.WriteString("\n")
	sb.WriteString("---- Queue Backlog ----\n")
	fmt.Fprintf(&sb, "  Max oldest queued: %.1f sec\n", maxOldestAge)
	fmt.Fprintf(&sb, "  Final oldest:      %.1f sec\n", final.OldestQueuedAge)
	sb.WriteString("\n")
	sb.WriteString("---- Index Health ----\n")
	if final.IndexDeadItems >= 0 {
		fmt.Fprintf(&sb, "  Index dead items:  %d\n", final.IndexDeadItems)
	} else {
		sb.WriteString("  Index dead items:  N/A (pgstattuple extension not available)\n")
	}
	if maxSlotWalLag > 0 {
		sb.WriteString("\n")
		sb.WriteString("---- Logical Slot WAL Retention ----\n")
		fmt.Fprintf(&sb, "  Final slot lag:    %d bytes\n", final.SlotWalLagBytes)
		fmt.Fprintf(&sb, "  Peak slot lag:     %d bytes\n", maxSlotWalLag)
	}
	sb.WriteString("\n")
	sb.WriteString("---- Snapshot Timeline ----\n")
	fmt.Fprintf(&sb, "  %-8s %-8s %-8s %-8s %-7s %-8s %-8s %-8s %-8s\n",
		"Time(s)", "Dead", "Live", "Ratio", "HOT%", "Enq/s", "Deq/s", "P99(us)", "Age(s)")
	for _, s := range snapshots {
		fmt.Fprintf(&sb, "  %-8.0f %-8d %-8d %-8.4f %-7.1f %-8.0f %-8.0f %-8d %-8.1f\n",
			s.ElapsedSec, s.DeadTuples, s.LiveTuples,
			s.DeadTupleRatio, s.HotUpdateRatio*100,
			s.EnqueueRate, s.DequeueRate,
			s.DequeueP99us, s.OldestQueuedAge)
	}
	sb.WriteString("====================================================================\n")

	t.Log(sb.String())
}

func writeResults(t *testing.T, filename string, data any) {
	t.Helper()
	f, err := os.Create(filename)
	if err != nil {
		t.Logf("write results: %v (non-fatal)", err)
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	_ = enc.Encode(data)
	t.Logf("Results written to %s", filename)
}

// TestQueueHealthBench_Compare loads two JSON result files and prints a
// before/after diff table. Set BENCH_BEFORE and BENCH_AFTER env vars.
func TestQueueHealthBench_Compare(t *testing.T) {
	beforeFile := os.Getenv("BENCH_BEFORE")
	afterFile := os.Getenv("BENCH_AFTER")
	if beforeFile == "" || afterFile == "" {
		t.Skip("set BENCH_BEFORE and BENCH_AFTER to compare")
	}

	type result struct {
		Snapshots []healthSnapshot `json:"snapshots"`
	}

	loadResult := func(path string) result {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var r result
		if err := json.Unmarshal(data, &r); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		return r
	}

	before := loadResult(beforeFile)
	after := loadResult(afterFile)

	last := func(snaps []healthSnapshot) healthSnapshot {
		if len(snaps) == 0 {
			return healthSnapshot{}
		}
		return snaps[len(snaps)-1]
	}

	b := last(before.Snapshots)
	a := last(after.Snapshots)

	delta := func(label string, bv, av float64, unit string, lowerBetter bool) string {
		if bv == 0 {
			return fmt.Sprintf("  %-30s %10.1f -> %10.1f %s", label, bv, av, unit)
		}
		pctChange := (av - bv) / bv * 100
		direction := "WORSE"
		if (lowerBetter && pctChange < 0) || (!lowerBetter && pctChange > 0) {
			direction = "BETTER"
		}
		if pctChange > -1 && pctChange < 1 {
			direction = "~same"
		}
		return fmt.Sprintf("  %-30s %10.1f -> %10.1f %s (%+.1f%% %s)", label, bv, av, unit, pctChange, direction)
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("====================================================================\n")
	sb.WriteString("              BEFORE / AFTER COMPARISON\n")
	sb.WriteString("====================================================================\n")
	fmt.Fprintf(&sb, "  Before: %s\n", beforeFile)
	fmt.Fprintf(&sb, "  After:  %s\n", afterFile)
	sb.WriteString("\n")
	sb.WriteString(delta("Dead tuple ratio", b.DeadTupleRatio*100, a.DeadTupleRatio*100, "%", true) + "\n")
	sb.WriteString(delta("Dead tuples", float64(b.DeadTuples), float64(a.DeadTuples), "", true) + "\n")
	sb.WriteString(delta("HOT update ratio", b.HotUpdateRatio*100, a.HotUpdateRatio*100, "%", false) + "\n")
	sb.WriteString(delta("P50 dequeue (us)", float64(b.DequeueP50us), float64(a.DequeueP50us), "us", true) + "\n")
	sb.WriteString(delta("P95 dequeue (us)", float64(b.DequeueP95us), float64(a.DequeueP95us), "us", true) + "\n")
	sb.WriteString(delta("P99 dequeue (us)", float64(b.DequeueP99us), float64(a.DequeueP99us), "us", true) + "\n")
	sb.WriteString(delta("Enqueue rate", b.EnqueueRate, a.EnqueueRate, "runs/s", false) + "\n")
	sb.WriteString(delta("Dequeue rate", b.DequeueRate, a.DequeueRate, "runs/s", false) + "\n")
	sb.WriteString(delta("Oldest queued", b.OldestQueuedAge, a.OldestQueuedAge, "s", true) + "\n")
	sb.WriteString("====================================================================\n")

	t.Log(sb.String())
}
