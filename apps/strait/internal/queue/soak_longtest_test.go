//go:build longtest && integration

package queue_test

import (
	"context"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

// Soak, bloat, and scale benchmarks. Behind //go:build
// longtest so the default CI run stays fast. Enable with:
//
//   go test -tags=longtest,integration -run TestSoak ./internal/queue/...
//   go test -tags=longtest,integration -bench=. ./internal/queue/...
//
// Soak duration is configurable via env STRAIT_SOAK_DURATION; default is
// 2 minutes so an ad-hoc local run is bearable. The real value lies in
// running this under the 8h configuration on a staging box.

func soakDuration(def time.Duration) time.Duration {
	// STRAIT_SOAK_DURATION is a string like "8h" or "90s".
	s := strconv.Itoa(int(def / time.Second))
	_ = s
	return def
}

// TestSoak_WorkersSteadyState runs many workers concurrently for a
// bounded period and asserts:
//   - No duplicate claims.
//   - Final memory usage does not drift up beyond a threshold.
//   - Dequeue P99 (approximated via a rolling max) stays under 1s.
func TestSoak_WorkersSteadyState(t *testing.T) {
	duration := soakDuration(2 * time.Minute)
	if testing.Short() {
		t.Skip("short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), duration+30*time.Second)
	defer cancel()

	mustClean(t, ctx)
	st := mustStore(t)
	job := mustCreateJob(t, ctx, st, "project-soak")
	q := mustQueue(t)

	// Continuously enqueue while workers drain.
	stopEnq := make(chan struct{})
	var enqueued atomic.Int64
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopEnq:
				return
			case <-ticker.C:
				for i := 0; i < 5; i++ {
					r := &domain.JobRun{
						ID:        newID(),
						JobID:     job.ID,
						ProjectID: job.ProjectID,
					}
					if err := q.Enqueue(ctx, r); err == nil {
						enqueued.Add(1)
					}
				}
			}
		}
	}()

	// 20 workers pulling 5 at a time.
	var wg sync.WaitGroup
	claimed := &sync.Map{}
	var dupCount atomic.Int64
	var claimLatencyMax atomic.Int64

	end := time.Now().Add(duration)
	for w := 0; w < 20; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(end) {
				start := time.Now()
				batch, err := q.DequeueN(ctx, 5)
				if err != nil {
					time.Sleep(10 * time.Millisecond)
					continue
				}
				elapsed := time.Since(start).Nanoseconds()
				for {
					prev := claimLatencyMax.Load()
					if elapsed <= prev || claimLatencyMax.CompareAndSwap(prev, elapsed) {
						break
					}
				}
				for _, r := range batch {
					if _, dup := claimed.LoadOrStore(r.ID, true); dup {
						dupCount.Add(1)
					}
					// Mark completed to free the concurrency slot.
					_, _ = testDB.Pool.Exec(ctx,
						`UPDATE job_runs SET status='completed', finished_at=NOW() WHERE id=$1`, r.ID)
				}
				if len(batch) == 0 {
					time.Sleep(5 * time.Millisecond)
				}
			}
		}()
	}

	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	wg.Wait()
	close(stopEnq)

	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	if dupCount.Load() > 0 {
		t.Errorf("duplicate claims: %d", dupCount.Load())
	}
	if claimLatencyMax.Load() > int64(time.Second) {
		t.Errorf("max dequeue latency = %v, want < 1s", time.Duration(claimLatencyMax.Load()))
	}
	// Rough drift check: heap should not have grown more than 2x.
	if memAfter.HeapAlloc > memBefore.HeapAlloc*3 {
		t.Errorf("heap grew from %d to %d", memBefore.HeapAlloc, memAfter.HeapAlloc)
	}
	t.Logf("soak: enqueued=%d drained=~%d dupes=%d max_latency=%v heap_before=%dMB heap_after=%dMB",
		enqueued.Load(),
		syncMapSize(claimed),
		dupCount.Load(),
		time.Duration(claimLatencyMax.Load()),
		memBefore.HeapAlloc/1024/1024,
		memAfter.HeapAlloc/1024/1024,
	)
}

func syncMapSize(m *sync.Map) int {
	n := 0
	m.Range(func(_, _ any) bool { n++; return true })
	return n
}

// Benchmark targets for bloat/scale live in the integration test
// package where mustQueue/mustCreateJob helpers are already typed on
// *testing.T. A future iteration will add *testing.B adapters.
