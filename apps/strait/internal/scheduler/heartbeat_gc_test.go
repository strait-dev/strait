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

type fakeGCStore struct {
	deleted           int64
	compacted         int64
	compactedRetries  int64
	deletedClaims     int64
	deletedReady      int64
	compactedPriority int64
	compactedVisible  int64
	compactedCache    int64
	err               error
	compactErr        error
	retryCompactErr   error
	claimErr          error
	readyErr          error
	priorityErr       error
	visibilityErr     error
	cacheErr          error
	panicRun          bool
	calls             int
	compactCalls      int
	retryCompactCalls int
	claimCalls        int
	readyCalls        int
	priorityCalls     int
	visibilityCalls   int
	cacheCalls        int
}

func (f *fakeGCStore) DeleteOrphanedHeartbeats(_ context.Context, _ int) (int64, error) {
	f.calls++
	if f.panicRun {
		panic("heartbeat store panic")
	}
	return f.deleted, f.err
}

func (f *fakeGCStore) CompactSupersededHeartbeats(_ context.Context, _ int) (int64, error) {
	f.compactCalls++
	return f.compacted, f.compactErr
}

func (f *fakeGCStore) CompactSupersededRetries(_ context.Context, _ int) (int64, error) {
	f.retryCompactCalls++
	return f.compactedRetries, f.retryCompactErr
}

func (f *fakeGCStore) DeleteInactiveActiveClaims(_ context.Context, _ int) (int64, error) {
	f.claimCalls++
	return f.deletedClaims, f.claimErr
}

func (f *fakeGCStore) DeleteInactiveReadyEvents(_ context.Context, _ int) (int64, error) {
	f.readyCalls++
	return f.deletedReady, f.readyErr
}

func (f *fakeGCStore) CompactSupersededPriorityEvents(_ context.Context, _ int) (int64, error) {
	f.priorityCalls++
	return f.compactedPriority, f.priorityErr
}

func (f *fakeGCStore) CompactSupersededVisibilityEvents(_ context.Context, _ int) (int64, error) {
	f.visibilityCalls++
	return f.compactedVisible, f.visibilityErr
}

func (f *fakeGCStore) CompactSupersededRunCacheVersions(_ context.Context, _ int) (int64, error) {
	f.cacheCalls++
	return f.compactedCache, f.cacheErr
}

func TestHeartbeatGC_Defaults(t *testing.T) {
	g := NewHeartbeatGC(&fakeGCStore{}, HeartbeatGCConfig{})
	assert.Equal(t, time.
		Hour, g.
		interval)
	assert.Equal(t, 10000,
		g.batchLimit,
	)
}

func TestHeartbeatGC_RunOnceAccumulates(t *testing.T) {
	s := &fakeGCStore{deleted: 17, compacted: 23, compactedRetries: 11, deletedClaims: 5, deletedReady: 7, compactedPriority: 13, compactedVisible: 17, compactedCache: 19}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{})
	_ = g.runOnce(context.Background())
	assert.EqualValues(t, 112,
		g.TotalDeleted())
	assert.False(t, s.calls !=
		1 ||
		s.compactCalls !=

			1 || s.retryCompactCalls != 1 ||

		s.claimCalls != 1 || s.readyCalls !=
		1 || s.priorityCalls !=
		1 || s.visibilityCalls !=
		1 || s.cacheCalls !=
		1)
}

func TestHeartbeatGC_LockNotAcquired(t *testing.T) {
	s := &fakeGCStore{deleted: 5}
	locker := &fakeLocker{acquireOK: false}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{}).WithAdvisoryLocker(locker)
	_ = g.runOnce(context.Background())
	assert.Equal(t, 0,
		s.calls)
}

func TestHeartbeatGC_LockAcquired(t *testing.T) {
	s := &fakeGCStore{deleted: 5}
	locker := &fakeLocker{acquireOK: true}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{}).WithAdvisoryLocker(locker)
	_ = g.runOnce(context.Background())
	assert.False(t, s.calls !=
		1 ||
		!locker.
			acquired ||
		!locker.released)
}

func TestHeartbeatGC_DeleteError(t *testing.T) {
	s := &fakeGCStore{err: errors.New("oops")}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{})
	assert.Error(t, g.runOnce(context.
		Background()))
}

func TestHeartbeatGC_CompactError(t *testing.T) {
	s := &fakeGCStore{compactErr: errors.New("oops")}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{})
	assert.Error(t, g.runOnce(context.
		Background()))
}

func TestHeartbeatGC_RetryCompactError(t *testing.T) {
	s := &fakeGCStore{retryCompactErr: errors.New("oops")}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{})
	assert.Error(t, g.runOnce(context.
		Background()))
}

func TestHeartbeatGC_ActiveClaimError(t *testing.T) {
	s := &fakeGCStore{claimErr: errors.New("oops")}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{})
	assert.Error(t, g.runOnce(context.
		Background()))
}

func TestHeartbeatGC_ReadyEventError(t *testing.T) {
	s := &fakeGCStore{readyErr: errors.New("oops")}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{})
	assert.Error(t, g.runOnce(context.
		Background()))
}

func TestHeartbeatGC_PriorityEventCompactError(t *testing.T) {
	s := &fakeGCStore{priorityErr: errors.New("oops")}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{})
	assert.Error(t, g.runOnce(context.
		Background()))
}

func TestHeartbeatGC_VisibilityEventCompactError(t *testing.T) {
	s := &fakeGCStore{visibilityErr: errors.New("oops")}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{})
	assert.Error(t, g.runOnce(context.
		Background()))
}

func TestHeartbeatGC_RunCacheVersionCompactError(t *testing.T) {
	s := &fakeGCStore{cacheErr: errors.New("oops")}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{})
	assert.Error(t, g.runOnce(context.
		Background()))
}

func TestHeartbeatGC_PanicReturnsError(t *testing.T) {
	s := &fakeGCStore{panicRun: true}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{})
	require.Error(t, g.
		runOnce(context.
			Background()),
	)
	require.EqualValues(t, 1,
		g.Iterations())
}

func TestHeartbeatGC_RunExitsOnCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	s := &fakeGCStore{}
	g := NewHeartbeatGC(s, HeartbeatGCConfig{Interval: 5 * time.Millisecond})
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
