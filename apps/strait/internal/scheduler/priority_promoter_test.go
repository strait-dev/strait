package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unit tests for the priority promoter. Integration tests live in
// priority_promoter_integration_test.go.

type fakeDB struct {
	calls    int
	lastSQL  string
	lastArgs []any
	rows     int64
	err      error
}

func (f *fakeDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.calls++
	f.lastSQL = sql
	f.lastArgs = args
	if f.err != nil {
		return pgconn.CommandTag{}, f.err
	}
	// pgconn.NewCommandTag requires a byte buffer like "UPDATE 3"; easier to
	// fabricate a tag that reports the right RowsAffected via fake string.
	return pgconn.NewCommandTag("UPDATE " + toBase10(f.rows)), nil
}
func (f *fakeDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return nil
}

func toBase10(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

type fakeLocker struct {
	acquired  bool
	released  bool
	acquireOK bool
	err       error
}

func (f *fakeLocker) TryAdvisoryLock(_ context.Context, _ int64) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	f.acquired = true
	return f.acquireOK, nil
}
func (f *fakeLocker) ReleaseAdvisoryLock(_ context.Context, _ int64) error {
	f.released = true
	return nil
}

func TestPriorityPromoter_Defaults(t *testing.T) {
	p := NewPriorityPromoter(&fakeDB{}, PriorityPromoterConfig{})
	assert.Equal(t, 60*
		time.Second,
		p.interval,
	)
	assert.Equal(t, 5*
		time.Minute,
		p.ageThreshold,
	)
	assert.Equal(t, 1000,
		p.maxPriority,
	)
	assert.Equal(t, 500,
		p.batchLimit,
	)
}

func TestPriorityPromoter_RunOnce_AppendsPriorityEvents(t *testing.T) {
	db := &fakeDB{rows: 3}
	p := NewPriorityPromoter(db, PriorityPromoterConfig{
		Interval:     time.Second,
		AgeThreshold: 10 * time.Second,
		MaxPriority:  100,
		BatchLimit:   50,
		Logger:       slog.Default(),
	})
	require.NoError(t,
		p.runOnce(
			context.Background()))
	assert.Equal(t, 1,
		db.calls)
	assert.EqualValues(t, 3,
		p.RowsPromoted())
	assert.EqualValues(t, 1,
		p.Iterations())
}

func TestPriorityPromoter_RunOnce_WithLock_Acquired(t *testing.T) {
	db := &fakeDB{rows: 1}
	locker := &fakeLocker{acquireOK: true}
	p := NewPriorityPromoter(db, PriorityPromoterConfig{}).WithAdvisoryLocker(locker)
	require.NoError(t,
		p.runOnce(
			context.Background()))
	assert.True(t, locker.
		acquired,
	)
	assert.True(t, locker.
		released,
	)
	assert.Equal(t, 1,
		db.calls)
}

func TestPriorityPromoter_RunOnce_WithLock_NotAcquired(t *testing.T) {
	db := &fakeDB{rows: 1}
	locker := &fakeLocker{acquireOK: false}
	p := NewPriorityPromoter(db, PriorityPromoterConfig{}).WithAdvisoryLocker(locker)
	require.NoError(t,
		p.runOnce(
			context.Background()))
	assert.Equal(t, 0,
		db.calls)
	assert.False(t, locker.
		released,
	)
}

func TestPriorityPromoter_RunOnce_LockError(t *testing.T) {
	db := &fakeDB{}
	locker := &fakeLocker{err: errors.New("pg down")}
	p := NewPriorityPromoter(db, PriorityPromoterConfig{}).WithAdvisoryLocker(locker)
	require.Error(t, p.runOnce(context.
		Background()))
	assert.Equal(t, 0,
		db.calls)
}

func TestPriorityPromoter_RunOnce_InsertError(t *testing.T) {
	db := &fakeDB{err: errors.New("deadlock")}
	p := NewPriorityPromoter(db, PriorityPromoterConfig{})
	require.Error(t, p.runOnce(context.
		Background()))
	assert.EqualValues(t, 1,
		p.Iterations())
}

func TestPriorityPromoter_QuerySanity(t *testing.T) {
	db := &fakeDB{rows: 0}
	p := NewPriorityPromoter(db, PriorityPromoterConfig{
		MaxPriority:  42,
		AgeThreshold: 30 * time.Second,
		BatchLimit:   100,
	})
	_ = p.runOnce(context.Background())
	require.NotEmpty(t,
		db.lastSQL,
	)

	// The exec must be parameterized (no inline values) so pg can cache the
	// plan. $1 $2 $3 should appear.
	for _, p := range []string{"$1", "$2", "$3", "job_run_state", "job_run_priority_events", "s.status = 'queued'", "FOR UPDATE OF s SKIP LOCKED", "INSERT INTO job_run_priority_events", "LEAST(COALESCE(priority.priority, s.priority) + 1"} {
		assert.True(t, contains(db.lastSQL,
			p))
	}
	assert.Len(t, db.lastArgs,
		3)
	assert.EqualValues(t, 42,
		db.lastArgs[0])
	assert.EqualValues(t, 100,
		db.lastArgs[2])
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func TestPriorityPromoter_RunExitsOnContextCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	db := &fakeDB{rows: 0}
	p := NewPriorityPromoter(db, PriorityPromoterConfig{
		Interval: 5 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		p.Run(ctx)
		close(done)
	})
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "Run did not exit on cancel")
	}
	assert.GreaterOrEqual(t, p.Iterations(),
		int64(2))
}
