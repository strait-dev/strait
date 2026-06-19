package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unit tests for the counter reconciler.

type reconFakeDB struct {
	queryCalls int
	forcedErr  error
	delta      int64
	panicRun   bool
}

type reconTxBeginner struct {
	reconFakeDB

	beginErr error
	tx       *reconFakeTx
}

func (b *reconTxBeginner) Begin(context.Context) (pgx.Tx, error) {
	if b.beginErr != nil {
		return nil, b.beginErr
	}
	if b.tx == nil {
		b.tx = &reconFakeTx{}
	}
	return b.tx, nil
}

type reconFakeTx struct {
	reconFakeDB

	commitErr  error
	committed  bool
	rolledBack bool
}

func (tx *reconFakeTx) Begin(context.Context) (pgx.Tx, error) { return tx, nil }
func (tx *reconFakeTx) Commit(context.Context) error {
	if tx.commitErr != nil {
		return tx.commitErr
	}
	tx.committed = true
	return nil
}
func (tx *reconFakeTx) Rollback(context.Context) error {
	tx.rolledBack = true
	return nil
}
func (tx *reconFakeTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("not used")
}
func (tx *reconFakeTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (tx *reconFakeTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (tx *reconFakeTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("not used")
}
func (tx *reconFakeTx) Conn() *pgx.Conn { return nil }

func (f *reconFakeDB) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (f *reconFakeDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, errors.New("not used")
}
func (f *reconFakeDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	f.queryCalls++
	if f.panicRun {
		panic("counter db panic")
	}
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
	assert.Equal(t, time.
		Hour, r.
		interval)
	assert.NotNil(t, r.
		logger)
}

func TestCounterReconciler_RunOnce_NoLock(t *testing.T) {
	db := &reconFakeDB{delta: 0}
	r := NewCounterReconciler(db, CounterReconcilerConfig{})
	require.NoError(t,
		r.runOnce(
			context.Background(),
		))
	assert.Equal(t, 2,
		db.queryCalls,
	)
	assert.EqualValues(t, 1,
		r.Iterations())
}

func TestCounterReconciler_RunOnce_AccumulatesDrift(t *testing.T) {
	db := &reconFakeDB{delta: 7}
	r := NewCounterReconciler(db, CounterReconcilerConfig{})
	_ = r.runOnce(context.Background())
	assert.EqualValues(t, 14,
		r.TotalDrift())

	// Each query returns delta=7, two queries run → total 14.
}

func TestCounterReconciler_LockAcquireFailure(t *testing.T) {
	db := &reconFakeDB{}
	locker := &fakeLocker{err: errors.New("lock down")}
	r := NewCounterReconciler(db, CounterReconcilerConfig{}).WithAdvisoryLocker(locker)
	err := r.runOnce(context.Background())
	require.Error(t, err)
	assert.Equal(t, 0,
		db.queryCalls,
	)
}

func TestCounterReconciler_LockNotAcquired(t *testing.T) {
	db := &reconFakeDB{}
	locker := &fakeLocker{acquireOK: false}
	r := NewCounterReconciler(db, CounterReconcilerConfig{}).WithAdvisoryLocker(locker)
	_ = r.runOnce(context.Background())
	assert.Equal(t, 0,
		db.queryCalls,
	)
	assert.False(t, locker.
		released,
	)
}

func TestCounterReconciler_LockAcquiredAndReleased(t *testing.T) {
	db := &reconFakeDB{delta: 3}
	locker := &fakeLocker{acquireOK: true}
	r := NewCounterReconciler(db, CounterReconcilerConfig{}).WithAdvisoryLocker(locker)
	_ = r.runOnce(context.Background())
	assert.False(t, !locker.
		acquired ||
		!locker.
			released,
	)
	assert.Equal(t, 2,
		db.queryCalls,
	)
}

func TestCounterReconciler_QueryErrorLogsButContinues(t *testing.T) {
	db := &reconFakeDB{forcedErr: errors.New("deadlock")}
	r := NewCounterReconciler(db, CounterReconcilerConfig{})
	require.NoError(t, r.
		runOnce(context.
			Background(),
		),
	)
	assert.EqualValues(t, 1,
		r.Iterations())
}

func TestCounterReconciler_TxBeginFailure(t *testing.T) {
	t.Parallel()

	db := &reconTxBeginner{beginErr: errors.New("begin failed")}
	r := NewCounterReconciler(db, CounterReconcilerConfig{})

	err := r.reconcileLocked(context.Background())
	require.ErrorContains(t, err, "counter reconciler begin tx")
	assert.EqualValues(t, 0, r.TotalDrift())
}

func TestCounterReconciler_TxCommitFailureRollsBack(t *testing.T) {
	t.Parallel()

	tx := &reconFakeTx{
		reconFakeDB: reconFakeDB{delta: 2},
		commitErr:   errors.New("commit failed"),
	}
	db := &reconTxBeginner{tx: tx}
	r := NewCounterReconciler(db, CounterReconcilerConfig{})

	err := r.reconcileLocked(context.Background())
	require.ErrorContains(t, err, "counter reconciler commit")
	assert.True(t, tx.rolledBack)
	assert.False(t, tx.committed)
	assert.EqualValues(t, 0, r.TotalDrift())
}

func TestCounterReconciler_TxCommitAddsDriftOnce(t *testing.T) {
	t.Parallel()

	tx := &reconFakeTx{reconFakeDB: reconFakeDB{delta: 3}}
	db := &reconTxBeginner{tx: tx}
	r := NewCounterReconciler(db, CounterReconcilerConfig{})

	require.NoError(t, r.reconcileLocked(context.Background()))
	assert.True(t, tx.committed)
	assert.False(t, tx.rolledBack)
	assert.EqualValues(t, 6, r.TotalDrift())
	assert.Equal(t, 2, tx.queryCalls)
}

func TestCounterReconciler_PanicReturnsError(t *testing.T) {
	db := &reconFakeDB{panicRun: true}
	r := NewCounterReconciler(db, CounterReconcilerConfig{})
	require.Error(t, r.
		runOnce(context.
			Background()),
	)
	require.EqualValues(t, 1,
		r.Iterations())
}

func TestCounterReconciler_RunExitsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	db := &reconFakeDB{}
	r := NewCounterReconciler(db, CounterReconcilerConfig{Interval: 5 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		r.Run(ctx)
		close(done)
	})
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "Run did not exit on cancel")
	}
	assert.GreaterOrEqual(t, r.Iterations(),
		int64(2))
}
