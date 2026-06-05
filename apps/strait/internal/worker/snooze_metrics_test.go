package worker

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
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
		require.NoError(
			t, provider.
				Shutdown(
					context.
						Background()))
	})
	return reader
}

func snoozeSkippedSum(t *testing.T, reader *sdkmetric.ManualReader, from, reason string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(
		t, reader.
			Collect(context.
				Background(), &rm))

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "strait_worker_snooze_skipped_total" {
				continue
			}
			data, ok := m.Data.(metricdata.Sum[int64])
			require.True(t,
				ok)

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
	require.EqualValues(t, 1, snoozeSkippedSum(
		t,
		reader, string(domain.StatusDequeued), "locked"))
	require.EqualValues(t, 0, snoozeSkippedSum(
		t,
		reader, string(domain.StatusDequeued), "conflict",
	),
	)
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
	require.EqualValues(t, 1, snoozeSkippedSum(
		t,
		reader, string(domain.StatusDequeued), "conflict",
	),
	)
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
	require.EqualValues(t, 1, snoozeSkippedSum(
		t,
		reader, string(domain.StatusExecuting), "locked"),
	)
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
	require.EqualValues(t, 0, snoozeSkippedSum(
		t,
		reader, string(domain.StatusDequeued), "locked"))
	require.EqualValues(t, 0, snoozeSkippedSum(
		t,
		reader, string(domain.StatusDequeued), "conflict",
	),
	)
}
