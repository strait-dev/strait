package worker

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// setupSnoozeSkippedMetrics swaps in a fresh ManualReader-backed meter
// provider and rebuilds the package-level workerMetrics on top of it. The
// previous state is restored on t.Cleanup so the test is isolation-safe.
func setupSnoozeSkippedMetrics(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	oldProvider := otel.GetMeterProvider()
	oldMetrics := workerMetrics.Load()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(provider)
	fresh := newWorkerMetrics()
	workerMetrics.Store(&fresh)

	t.Cleanup(func() {
		workerMetrics.Store(oldMetrics)
		otel.SetMeterProvider(oldProvider)
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown meter provider: %v", err)
		}
	})
	return reader
}

func snoozeSkippedSum(t *testing.T, reader *sdkmetric.ManualReader, from, reason string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "strait_worker_snooze_skipped_total" {
				continue
			}
			data, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("snooze_skipped data = %T, want Sum[int64]", m.Data)
			}
			var total int64
			for _, dp := range data.DataPoints {
				if attrEq(dp.Attributes, "from", from) && attrEq(dp.Attributes, "reason", reason) {
					total += dp.Value
				}
			}
			return total
		}
	}
	return 0
}

func attrEq(set attribute.Set, key, want string) bool {
	got, ok := set.Value(attribute.Key(key))
	return ok && got.AsString() == want
}

func TestSnoozeRun_LockedIncrementsCounter(t *testing.T) {
	reader := setupSnoozeSkippedMetrics(t)

	st := &mockExecutorStore{
		snoozeRunWithLockFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return store.ErrRunLocked
		},
	}
	exec := newSnoozeTestExecutor(t, st, 0)
	run := testRun(1)
	run.Status = domain.StatusDequeued

	exec.snoozeRun(context.Background(), run, "raced with reaper", nil)

	if got := snoozeSkippedSum(t, reader, string(domain.StatusDequeued), "locked"); got != 1 {
		t.Fatalf("snooze_skipped{from=dequeued,reason=locked} = %d, want 1", got)
	}
	if got := snoozeSkippedSum(t, reader, string(domain.StatusDequeued), "conflict"); got != 0 {
		t.Fatalf("snooze_skipped{...,reason=conflict} = %d, want 0", got)
	}
}

func TestSnoozeRun_ConflictIncrementsCounter(t *testing.T) {
	reader := setupSnoozeSkippedMetrics(t)

	st := &mockExecutorStore{
		snoozeRunWithLockFn: func(_ context.Context, _ string, from, _ domain.RunStatus, _ map[string]any) error {
			return errors.Join(store.ErrRunConflict, errors.New("status moved on from "+string(from)))
		},
	}
	exec := newSnoozeTestExecutor(t, st, 0)
	run := testRun(1)
	run.Status = domain.StatusDequeued

	exec.snoozeRun(context.Background(), run, "raced with completion", nil)

	if got := snoozeSkippedSum(t, reader, string(domain.StatusDequeued), "conflict"); got != 1 {
		t.Fatalf("snooze_skipped{from=dequeued,reason=conflict} = %d, want 1", got)
	}
}

func TestSnoozeRunFromExecuting_LockedIncrementsCounter(t *testing.T) {
	reader := setupSnoozeSkippedMetrics(t)

	st := &mockExecutorStore{
		snoozeRunWithLockFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return store.ErrRunLocked
		},
	}
	exec := newSnoozeTestExecutor(t, st, 0)
	run := testRun(1)
	run.Status = domain.StatusExecuting

	exec.snoozeRunFromExecuting(context.Background(), run, "watchdog tick", nil)

	if got := snoozeSkippedSum(t, reader, string(domain.StatusExecuting), "locked"); got != 1 {
		t.Fatalf("snooze_skipped{from=executing,reason=locked} = %d, want 1", got)
	}
}

func TestSnoozeRun_HappyPathDoesNotIncrementCounter(t *testing.T) {
	reader := setupSnoozeSkippedMetrics(t)

	st := &mockExecutorStore{
		snoozeRunWithLockFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
	}
	exec := newSnoozeTestExecutor(t, st, 0)
	run := testRun(1)
	run.Status = domain.StatusDequeued

	exec.snoozeRun(context.Background(), run, "normal retry", nil)

	if got := snoozeSkippedSum(t, reader, string(domain.StatusDequeued), "locked"); got != 0 {
		t.Fatalf("snooze_skipped on happy path: locked = %d, want 0", got)
	}
	if got := snoozeSkippedSum(t, reader, string(domain.StatusDequeued), "conflict"); got != 0 {
		t.Fatalf("snooze_skipped on happy path: conflict = %d, want 0", got)
	}
}
