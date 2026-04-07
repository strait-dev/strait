package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestCronScheduler_ProcessCanceledRuns_SemaphoreLimitsConcurrency(t *testing.T) {
	t.Parallel()

	var peak atomic.Int64
	var current atomic.Int64

	stopper := &mockMachineStopper{
		stopFn: func(_ context.Context, _ string) error {
			n := current.Add(1)
			// Track peak concurrency.
			for {
				old := peak.Load()
				if n <= old || peak.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			current.Add(-1)
			return nil
		},
	}

	// Create 20 managed runs to exceed the semaphore limit (10).
	runs := make([]store.CanceledRun, 20)
	for i := range runs {
		runs[i] = store.CanceledRun{
			ID:            "run-" + string(rune('a'+i)),
			MachineID:     "mach-" + string(rune('a'+i)),
			ExecutionMode: domain.ExecutionModeManaged,
		}
	}

	cs := NewCronScheduler(context.Background(), &mockCronStore{
		cancelChildRunsByParentIDFn: func(_ context.Context, _ []string, _ time.Time, _ string) (int64, error) {
			return 0, nil
		},
	}, &mockQueue{}, nil)
	cs.machineStopper = stopper

	cs.processCanceledRuns(context.Background(), "job-1", runs)

	if peak.Load() > int64(maxConcurrentStops) {
		t.Fatalf("peak concurrency = %d, want <= %d", peak.Load(), maxConcurrentStops)
	}
}

func TestCronScheduler_ProcessCanceledRuns_PanicRecovery(t *testing.T) {
	t.Parallel()

	var stopCount atomic.Int64
	stopper := &mockMachineStopper{
		stopFn: func(_ context.Context, machineID string) error {
			if machineID == "mach-panic" {
				panic("stopper panic")
			}
			stopCount.Add(1)
			return nil
		},
	}

	runs := []store.CanceledRun{
		{ID: "run-1", MachineID: "mach-panic", ExecutionMode: domain.ExecutionModeManaged},
		{ID: "run-2", MachineID: "mach-ok", ExecutionMode: domain.ExecutionModeManaged},
	}

	cs := NewCronScheduler(context.Background(), &mockCronStore{
		cancelChildRunsByParentIDFn: func(_ context.Context, _ []string, _ time.Time, _ string) (int64, error) {
			return 0, nil
		},
	}, &mockQueue{}, nil)
	cs.machineStopper = stopper

	// Should not panic -- the recovery catches it.
	cs.processCanceledRuns(context.Background(), "job-1", runs)

	if stopCount.Load() != 1 {
		t.Fatalf("expected 1 successful stop, got %d", stopCount.Load())
	}
}

func TestCronScheduler_ProcessCanceledRuns_SkipsNonManaged(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var stoppedIDs []string

	stopper := &mockMachineStopper{
		stopFn: func(_ context.Context, machineID string) error {
			mu.Lock()
			stoppedIDs = append(stoppedIDs, machineID)
			mu.Unlock()
			return nil
		},
	}

	runs := []store.CanceledRun{
		{ID: "run-1", MachineID: "mach-1", ExecutionMode: domain.ExecutionModeManaged},
		{ID: "run-2", MachineID: "", ExecutionMode: domain.ExecutionModeManaged},    // no machine
		{ID: "run-3", MachineID: "mach-3", ExecutionMode: domain.ExecutionModeHTTP}, // not managed
		{ID: "run-4", MachineID: "mach-4", ExecutionMode: domain.ExecutionModeManaged},
	}

	cs := NewCronScheduler(context.Background(), &mockCronStore{
		cancelChildRunsByParentIDFn: func(_ context.Context, _ []string, _ time.Time, _ string) (int64, error) {
			return 0, nil
		},
	}, &mockQueue{}, nil)
	cs.machineStopper = stopper

	cs.processCanceledRuns(context.Background(), "job-1", runs)

	mu.Lock()
	defer mu.Unlock()
	if len(stoppedIDs) != 2 {
		t.Fatalf("expected 2 stops, got %d: %v", len(stoppedIDs), stoppedIDs)
	}
}
