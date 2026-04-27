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
}

// benchConfig controls the benchmark parameters.
type benchConfig struct {
	Duration        time.Duration
	Workers         int
	BatchSize       int
	EnqueueRateHz   int // enqueue operations per second (each inserts BatchSize runs)
	SnapshotEvery   time.Duration
	UseDenormalized bool
	UseCursor       bool
}

func defaultBenchConfig() benchConfig {
	return benchConfig{
		Duration:      2 * time.Minute,
		Workers:       20,
		BatchSize:     5,
		EnqueueRateHz: 50,
		SnapshotEvery: 5 * time.Second,
	}
}

func benchConfigFromEnv() benchConfig {
	cfg := defaultBenchConfig()
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
	return cfg
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
		SELECT
			COALESCE(SUM(n_dead_tup), 0),
			COALESCE(SUM(n_live_tup), 0),
			COALESCE(SUM(n_tup_upd), 0),
			COALESCE(SUM(n_tup_hot_upd), 0)
		FROM pg_stat_user_tables
		WHERE relname = 'job_runs' OR relname LIKE 'job_runs_%'
	`)
	if err == nil {
		if rows.Next() {
			_ = rows.Scan(&snap.DeadTuples, &snap.LiveTuples, &snap.TotalUpdates, &snap.HotUpdates)
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
		FROM job_runs WHERE status = 'queued'
	`).Scan(&snap.OldestQueuedAge)

	snap.IndexDeadItems = -1
	_ = testDB.Pool.QueryRow(c.ctx, `
		SELECT COALESCE(dead_items, -1)
		FROM pgstatindex('idx_runs_queue_covering')
	`).Scan(&snap.IndexDeadItems)

	return snap
}

func TestQueueHealthBench(t *testing.T) {
	cfg := benchConfigFromEnv()
	if testing.Short() {
		t.Skip("short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration+60*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-health-bench")
	q := mustQueue(t)

	if _, err := testDB.Pool.Exec(ctx, "ANALYZE job_runs"); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, "SELECT pg_stat_reset()"); err != nil {
		t.Logf("pg_stat_reset: %v (non-fatal)", err)
	}

	t.Logf("=== Queue Health Benchmark ===")
	t.Logf("Duration: %v | Workers: %d | Batch: %d | Enqueue: %d runs/sec | Denorm: %v | Cursor: %v",
		cfg.Duration, cfg.Workers, cfg.BatchSize, cfg.EnqueueRateHz*cfg.BatchSize, cfg.UseDenormalized, cfg.UseCursor)

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
	go func() {
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
					if err := q.Enqueue(ctx, run); err == nil {
						enqueued.Add(1)
					}
				}
			}
		}
	}()

	// Consumer workers.
	var cursor *queue.ClaimCursor
	if cfg.UseCursor {
		cursor = queue.NewClaimCursor(30 * time.Second)
	}

	var workerWg sync.WaitGroup
	end := time.Now().Add(cfg.Duration)
	for w := range cfg.Workers {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			for time.Now().Before(end) {
				start := time.Now()
				var batch []domain.JobRun
				var err error
				switch {
				case cfg.UseDenormalized:
					batch, err = q.DequeueNDenormalized(ctx, cfg.BatchSize)
				case cfg.UseCursor:
					batch, err = q.DequeueNWithCursor(ctx, cfg.BatchSize, cursor)
				default:
					batch, err = q.DequeueN(ctx, cfg.BatchSize)
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
					_, _ = testDB.Pool.Exec(ctx,
						`UPDATE job_runs SET status='completed', finished_at=NOW() WHERE id=$1`, r.ID)
				}
				if len(batch) == 0 {
					time.Sleep(5 * time.Millisecond)
				}
			}
		}()
	}

	// Snapshot loop.
	var snapshots []healthSnapshot
	snapTicker := time.NewTicker(cfg.SnapshotEvery)
	defer snapTicker.Stop()
	snapshots = append(snapshots, collector.collect())

	done := make(chan struct{})
	go func() { workerWg.Wait(); close(done) }()

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
	cfg := benchConfigFromEnv()
	if testing.Short() {
		t.Skip("short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration+60*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-health-longtxn")
	q := mustQueue(t)

	if _, err := testDB.Pool.Exec(ctx, "ANALYZE job_runs"); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	t.Logf("=== Queue Health Benchmark WITH LONG TRANSACTION (PlanetScale scenario) ===")
	t.Logf("Duration: %v | Workers: %d | Enqueue: %d runs/sec",
		cfg.Duration, cfg.Workers, cfg.EnqueueRateHz*cfg.BatchSize)

	// Pin the xmin horizon with a long-running read transaction.
	longTx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin long txn: %v", err)
	}
	if _, err := longTx.Exec(ctx, "SELECT count(*) FROM job_runs"); err != nil {
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
	go func() {
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
					if err := q.Enqueue(ctx, run); err == nil {
						enqueued.Add(1)
					}
				}
			}
		}
	}()

	// Workers.
	var workerWg sync.WaitGroup
	end := time.Now().Add(cfg.Duration)
	for w := range cfg.Workers {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			for time.Now().Before(end) {
				start := time.Now()
				batch, bErr := q.DequeueN(ctx, cfg.BatchSize)
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
					_, _ = testDB.Pool.Exec(ctx,
						`UPDATE job_runs SET status='completed', finished_at=NOW() WHERE id=$1`, r.ID)
				}
				if len(batch) == 0 {
					time.Sleep(5 * time.Millisecond)
				}
			}
		}()
	}

	// Snapshot loop with midpoint long-txn release.
	var snapshots []healthSnapshot
	snapTicker := time.NewTicker(cfg.SnapshotEvery)
	defer snapTicker.Stop()
	snapshots = append(snapshots, collector.collect())

	done := make(chan struct{})
	go func() { workerWg.Wait(); close(done) }()

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

func printReport(t *testing.T, cfg benchConfig, snapshots []healthSnapshot) {
	t.Helper()
	if len(snapshots) == 0 {
		return
	}

	final := snapshots[len(snapshots)-1]

	var maxDead, maxLive, maxP99 int64
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
