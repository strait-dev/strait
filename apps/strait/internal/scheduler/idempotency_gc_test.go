package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeIdempotencyGCStore struct {
	deleted  int64
	err      error
	panicRun bool
	calls    int
}

func (f *fakeIdempotencyGCStore) DeleteExpiredIdempotencyEntries(_ context.Context, _ int) (int64, error) {
	f.calls++
	if f.panicRun {
		panic("idempotency store panic")
	}
	return f.deleted, f.err
}

func TestIdempotencyGC_Defaults(t *testing.T) {
	g := NewIdempotencyGC(&fakeIdempotencyGCStore{}, IdempotencyGCConfig{})
	assert.Equal(t, time.
		Hour, g.
		interval)
	assert.EqualValues(t, 10000,
		g.batchLimit,
	)

}

func TestIdempotencyGC_RunOnceAccumulates(t *testing.T) {
	s := &fakeIdempotencyGCStore{deleted: 23}
	g := NewIdempotencyGC(s, IdempotencyGCConfig{})
	_ = g.runOnce(context.Background())
	assert.EqualValues(t, 23,
		g.TotalDeleted())
	assert.EqualValues(t, 1,
		s.calls)

}

func TestIdempotencyGC_LockNotAcquired(t *testing.T) {
	s := &fakeIdempotencyGCStore{deleted: 9}
	locker := &fakeLocker{acquireOK: false}
	g := NewIdempotencyGC(s, IdempotencyGCConfig{}).WithAdvisoryLocker(locker)
	_ = g.runOnce(context.Background())
	assert.EqualValues(t, 0,
		s.calls)

}

func TestIdempotencyGC_LockAcquiredAndReleased(t *testing.T) {
	s := &fakeIdempotencyGCStore{deleted: 4}
	locker := &fakeLocker{acquireOK: true}
	g := NewIdempotencyGC(s, IdempotencyGCConfig{}).WithAdvisoryLocker(locker)
	_ = g.runOnce(context.Background())
	assert.False(t, s.calls !=
		1 ||
		!locker.
			acquired ||
		!locker.released)

}

func TestIdempotencyGC_DeleteError(t *testing.T) {
	s := &fakeIdempotencyGCStore{err: errors.New("boom")}
	g := NewIdempotencyGC(s, IdempotencyGCConfig{})
	assert.Error(t, g.runOnce(context.
		Background()))

}

func TestIdempotencyGC_PanicReturnsError(t *testing.T) {
	s := &fakeIdempotencyGCStore{panicRun: true}
	g := NewIdempotencyGC(s, IdempotencyGCConfig{})
	require.Error(t, g.
		runOnce(context.
			Background()),
	)
	require.EqualValues(t, 1,
		g.Iterations())

}

func TestIdempotencyGC_RunExitsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	s := &fakeIdempotencyGCStore{}
	g := NewIdempotencyGC(s, IdempotencyGCConfig{Interval: 5 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		g.Run(ctx)
		close(done)
	})
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "did not exit on cancel")
	}
	assert.GreaterOrEqual(t, g.Iterations(),
		int64(2))

}
