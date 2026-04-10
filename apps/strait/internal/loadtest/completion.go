//go:build loadtest

package loadtest

import (
	"context"
	"maps"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// CompletionStats holds aggregate stats about tracked job runs.
type CompletionStats struct {
	Completed         int64         `json:"completed"`
	Failed            int64         `json:"failed"`
	TimedOut          int64         `json:"timed_out"`
	Pending           int64         `json:"pending"`
	AvgCompletionTime time.Duration `json:"avg_completion_time"`
	P50               time.Duration `json:"p50"`
	P95               time.Duration `json:"p95"`
	P99               time.Duration `json:"p99"`
}

// CompletionTracker monitors job completion rates by polling pending runs
// in the background and recording their terminal states.
type CompletionTracker struct {
	harness   *Harness
	projectID string

	mu              sync.Mutex
	pending         map[string]time.Time // runID -> trigger time
	completionTimes []time.Duration      // for percentile calculation

	completed atomic.Int64
	failed    atomic.Int64
	timedOut  atomic.Int64
}

// NewCompletionTracker creates a tracker that polls pending runs via the harness.
func NewCompletionTracker(h *Harness, projectID string) *CompletionTracker {
	return &CompletionTracker{
		harness:   h,
		projectID: projectID,
		pending:   make(map[string]time.Time),
	}
}

// Track registers a run for background completion tracking.
func (ct *CompletionTracker) Track(runID string) {
	ct.mu.Lock()
	ct.pending[runID] = time.Now()
	ct.mu.Unlock()
}

// Run polls pending runs every 1s until the context is cancelled.
func (ct *CompletionTracker) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ct.poll(ctx)
		}
	}
}

func (ct *CompletionTracker) poll(ctx context.Context) {
	ct.mu.Lock()
	// Snapshot the pending map so we can release the lock during HTTP calls.
	snapshot := make(map[string]time.Time, len(ct.pending))
	maps.Copy(snapshot, ct.pending)
	ct.mu.Unlock()

	for runID, triggerTime := range snapshot {
		if ctx.Err() != nil {
			return
		}

		status, err := ct.harness.GetRun(ctx, runID)
		if err != nil {
			continue
		}

		if !terminalStatuses[status] {
			continue
		}

		elapsed := time.Since(triggerTime)

		ct.mu.Lock()
		delete(ct.pending, runID)
		ct.completionTimes = append(ct.completionTimes, elapsed)
		ct.mu.Unlock()

		switch status {
		case "completed":
			ct.completed.Add(1)
		case "timed_out":
			ct.timedOut.Add(1)
		default:
			// failed, dead_letter, crashed, system_failed, canceled
			ct.failed.Add(1)
		}
	}
}

// Stats returns a snapshot of current completion statistics.
func (ct *CompletionTracker) Stats() CompletionStats {
	ct.mu.Lock()
	pending := int64(len(ct.pending))
	times := make([]time.Duration, len(ct.completionTimes))
	copy(times, ct.completionTimes)
	ct.mu.Unlock()

	stats := CompletionStats{
		Completed: ct.completed.Load(),
		Failed:    ct.failed.Load(),
		TimedOut:  ct.timedOut.Load(),
		Pending:   pending,
	}

	if len(times) > 0 {
		var total time.Duration
		for _, d := range times {
			total += d
		}
		stats.AvgCompletionTime = total / time.Duration(len(times))
		stats.P50 = percentileDuration(times, 50)
		stats.P95 = percentileDuration(times, 95)
		stats.P99 = percentileDuration(times, 99)
	}

	return stats
}

// percentileDuration computes the p-th percentile from an unsorted slice of durations.
func percentileDuration(durations []time.Duration, p float64) time.Duration {
	n := len(durations)
	if n == 0 {
		return 0
	}

	sorted := make([]time.Duration, n)
	copy(sorted, durations)
	slices.Sort(sorted)

	idx := max(int(math.Ceil(p/100*float64(n)))-1, 0)
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}
