package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

type fakeReadyRunRepairer struct {
	repaired  int64
	err       error
	calls     atomic.Int32
	lastLimit atomic.Int64
}

func (f *fakeReadyRunRepairer) ReconcileReadyRuns(_ context.Context, limit int) (int64, error) {
	f.calls.Add(1)
	f.lastLimit.Store(int64(limit))
	return f.repaired, f.err
}

func TestReadyRunReconciler_Defaults(t *testing.T) {
	r := NewReadyRunReconciler(&fakeReadyRunRepairer{}, 0, 0)

	require.Equal(t, 5*time.Minute, r.interval)
	require.Equal(t, 1000, r.repairMax)
	require.NotNil(t, r.logger)
}

func TestReadyRunReconciler_ReconcileOnceSkipsNilRepairer(t *testing.T) {
	r := NewReadyRunReconciler(nil, time.Second, 10)

	require.NoError(t, r.reconcileOnce(context.Background()))
}

func TestReadyRunReconciler_ReconcileOnceCallsRepairer(t *testing.T) {
	repairer := &fakeReadyRunRepairer{}
	r := NewReadyRunReconciler(repairer, time.Second, 25)

	require.NoError(t, r.reconcileOnce(context.Background()))
	require.EqualValues(t, 1, repairer.calls.Load())
	require.EqualValues(t, 25, repairer.lastLimit.Load())
}

func TestReadyRunReconciler_ReconcileOnceWrapsRepairError(t *testing.T) {
	repairer := &fakeReadyRunRepairer{err: errors.New("repair failed")}
	r := NewReadyRunReconciler(repairer, time.Second, 25)

	err := r.reconcileOnce(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "reconcile pgque ready runs")
	require.ErrorIs(t, err, repairer.err)
}

func TestReadyRunReconciler_ReconcileOnceLogsRepairedRuns(t *testing.T) {
	var logs lockedTestBuffer
	repairer := &fakeReadyRunRepairer{repaired: 2}
	r := NewReadyRunReconciler(repairer, time.Second, 25)
	r.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))

	require.NoError(t, r.reconcileOnce(context.Background()))
	require.True(t, logs.Contains("ready run reconciler: re-emitted pgque ready runs"))
	require.True(t, logs.Contains("count=2"))
}

func TestReadyRunReconciler_ReconcileOnceSkipsZeroRepairLog(t *testing.T) {
	var logs lockedTestBuffer
	r := NewReadyRunReconciler(&fakeReadyRunRepairer{}, time.Second, 25)
	r.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))

	require.NoError(t, r.reconcileOnce(context.Background()))
	require.False(t, logs.Contains("ready run reconciler: re-emitted pgque ready runs"))
}

func TestReadyRunReconciler_RunLogsRepairError(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()

	var logs lockedTestBuffer
	repairer := &fakeReadyRunRepairer{err: errors.New("repair failed")}
	r := NewReadyRunReconciler(repairer, 5*time.Millisecond, 25)
	r.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		r.Run(ctx)
		close(done)
	})

	require.Eventually(t, func() bool {
		return logs.Contains("ready run reconciler failed")
	}, time.Second, time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "ready run reconciler did not stop")
	}
}
