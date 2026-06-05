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

type fakePartitionStore struct {
	err      error
	panicRun bool
	calls    int
}

func (f *fakePartitionStore) EnsureJobRunsPartitions(_ context.Context, _ int) error {
	f.calls++
	if f.panicRun {
		panic("partition store panic")
	}
	return f.err
}

func (f *fakePartitionStore) EnsureOutboxHistoryPartitions(_ context.Context, _ int) error {
	return f.err
}

func TestPartitionEnsurer_Defaults(t *testing.T) {
	p := NewPartitionEnsurer(&fakePartitionStore{}, PartitionEnsurerConfig{})
	assert.Equal(t, 24*
		time.Hour,
		p.interval,
	)
	assert.EqualValues(t, 2,
		p.monthsAhead,
	)

}

func TestPartitionEnsurer_RunOnceHappyPath(t *testing.T) {
	s := &fakePartitionStore{}
	p := NewPartitionEnsurer(s, PartitionEnsurerConfig{MonthsAhead: 3})
	require.NoError(t,
		p.runOnce(
			context.Background(),
		))
	assert.EqualValues(t, 1,
		s.calls)
	assert.EqualValues(t, 1,
		p.Iterations())
	assert.EqualValues(t, 0,
		p.Errors(),
	)

}

func TestPartitionEnsurer_StoreErrorAccumulates(t *testing.T) {
	s := &fakePartitionStore{err: errors.New("create partition failed")}
	p := NewPartitionEnsurer(s, PartitionEnsurerConfig{})
	_ = p.runOnce(context.Background())
	_ = p.runOnce(context.Background())
	assert.EqualValues(t, 2,
		p.Errors(),
	)

}

func TestPartitionEnsurer_PanicReturnsError(t *testing.T) {
	s := &fakePartitionStore{panicRun: true}
	p := NewPartitionEnsurer(s, PartitionEnsurerConfig{})
	require.Error(t, p.
		runOnce(context.
			Background()),
	)
	require.EqualValues(t, 1,
		p.Errors())

}

func TestPartitionEnsurer_RunOnceForTestPropagatesRecoveredPanic(t *testing.T) {
	s := &fakePartitionStore{panicRun: true}
	p := NewPartitionEnsurer(s, PartitionEnsurerConfig{})
	require.Error(t, p.
		RunOnceForTest(context.
			Background()))
	require.EqualValues(t, 1,
		p.Iterations())
	require.EqualValues(t, 1,
		p.Errors())

}

func TestPartitionEnsurer_LockNotAcquired(t *testing.T) {
	s := &fakePartitionStore{}
	locker := &fakeLocker{acquireOK: false}
	p := NewPartitionEnsurer(s, PartitionEnsurerConfig{}).WithAdvisoryLocker(locker)
	_ = p.runOnce(context.Background())
	assert.EqualValues(t, 0,
		s.calls)

}

func TestPartitionEnsurer_LockAcquiredAndReleased(t *testing.T) {
	s := &fakePartitionStore{}
	locker := &fakeLocker{acquireOK: true}
	p := NewPartitionEnsurer(s, PartitionEnsurerConfig{}).WithAdvisoryLocker(locker)
	_ = p.runOnce(context.Background())
	assert.False(t, !locker.
		acquired ||
		!locker.
			released,
	)

}

func TestPartitionEnsurer_RunExitsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	s := &fakePartitionStore{}
	p := NewPartitionEnsurer(s, PartitionEnsurerConfig{Interval: 5 * time.Millisecond})
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
		require.Fail(t, "did not exit on cancel")
	}
	assert.GreaterOrEqual(t, p.Iterations(),
		int64(2))

}
