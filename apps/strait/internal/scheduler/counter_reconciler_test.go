package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Unit tests for the counter reconciler.

type reconFakeDB struct {
	queryCalls int
	forcedErr  error
	delta      int64
}

func (f *reconFakeDB) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (f *reconFakeDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, errors.New("not used")
}
func (f *reconFakeDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	f.queryCalls++
	return &fakeRow{err: f.forcedErr, delta: f.delta}
}

type fakeRow struct {
	err   error
	delta int64
}

func (r *fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if p, ok := dest[0].(*int64); ok {
		*p = r.delta
	}
	return nil
}

func TestCounterReconciler_Defaults(t *testing.T) {
	r := NewCounterReconciler(&reconFakeDB{}, CounterReconcilerConfig{})
	if r.interval != time.Hour {
		t.Errorf("interval = %v", r.interval)
	}
	if r.logger == nil {
		t.Error("logger should default")
	}
}

func TestCounterReconciler_RunOnce_NoLock(t *testing.T) {
	db := &reconFakeDB{delta: 0}
	r := NewCounterReconciler(db, CounterReconcilerConfig{})
	if err := r.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if db.queryCalls != 2 {
		t.Errorf("query calls = %d, want 2 (active + dlq)", db.queryCalls)
	}
	if r.Iterations() != 1 {
		t.Errorf("iterations = %d, want 1", r.Iterations())
	}
}

func TestCounterReconciler_RunOnce_AccumulatesDrift(t *testing.T) {
	db := &reconFakeDB{delta: 7}
	r := NewCounterReconciler(db, CounterReconcilerConfig{})
	_ = r.runOnce(context.Background())
	// Each query returns delta=7, two queries run → total 14.
	if r.TotalDrift() != 14 {
		t.Errorf("total drift = %d, want 14", r.TotalDrift())
	}
}

func TestCounterReconciler_LockAcquireFailure(t *testing.T) {
	db := &reconFakeDB{}
	locker := &fakeLocker{err: errors.New("lock down")}
	r := NewCounterReconciler(db, CounterReconcilerConfig{}).WithAdvisoryLocker(locker)
	err := r.runOnce(context.Background())
	if err == nil {
		t.Error("expected lock error")
	}
	if db.queryCalls != 0 {
		t.Errorf("no queries should run on lock failure, got %d", db.queryCalls)
	}
}

func TestCounterReconciler_LockNotAcquired(t *testing.T) {
	db := &reconFakeDB{}
	locker := &fakeLocker{acquireOK: false}
	r := NewCounterReconciler(db, CounterReconcilerConfig{}).WithAdvisoryLocker(locker)
	_ = r.runOnce(context.Background())
	if db.queryCalls != 0 {
		t.Errorf("no queries should run when lock not acquired, got %d", db.queryCalls)
	}
	if locker.released {
		t.Error("locker should not release when not acquired")
	}
}

func TestCounterReconciler_LockAcquiredAndReleased(t *testing.T) {
	db := &reconFakeDB{delta: 3}
	locker := &fakeLocker{acquireOK: true}
	r := NewCounterReconciler(db, CounterReconcilerConfig{}).WithAdvisoryLocker(locker)
	_ = r.runOnce(context.Background())
	if !locker.acquired || !locker.released {
		t.Errorf("lock not used correctly acquired=%v released=%v", locker.acquired, locker.released)
	}
	if db.queryCalls != 2 {
		t.Errorf("queries = %d, want 2", db.queryCalls)
	}
}

func TestCounterReconciler_QueryErrorLogsButContinues(t *testing.T) {
	db := &reconFakeDB{forcedErr: errors.New("deadlock")}
	r := NewCounterReconciler(db, CounterReconcilerConfig{})
	if err := r.runOnce(context.Background()); err != nil {
		t.Errorf("runOnce should not propagate per-query errors: %v", err)
	}
	if r.Iterations() != 1 {
		t.Errorf("iterations = %d", r.Iterations())
	}
}

func TestCounterReconciler_RunExitsOnCancel(t *testing.T) {
	db := &reconFakeDB{}
	r := NewCounterReconciler(db, CounterReconcilerConfig{Interval: 5 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not exit on cancel")
	}
	if r.Iterations() < 2 {
		t.Errorf("iterations = %d, want >= 2", r.Iterations())
	}
}
