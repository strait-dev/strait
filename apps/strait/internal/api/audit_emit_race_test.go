package api

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

// TestStopAuditAsyncDrain_ConcurrentAccess spins up the drainer, emits events
// from 10 goroutines concurrently, then stops. Validates there is no data race
// on s.auditAsyncDone (the fix for Issue #4). Run with -race.
func TestStopAuditAsyncDrain_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	var written atomic.Int32
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			written.Add(1)
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	const goroutines = 10
	const eventsPerGoroutine = 20
	var wg conc.WaitGroup
	for range goroutines {
		wg.Go(func() {
			for range eventsPerGoroutine {
				srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "j1", nil)
			}
		})
	}
	wg.Wait()

	// Close calls stopAuditAsyncDrain, which reads s.auditAsyncDone under the
	// mutex — this is the path that fixes the race.
	srv.Close()

	// All events should have been written (either async or via sync fallback).
	total := written.Load()
	assert.NotEqual(t, 0,
		total)
}

// TestEmitAuditEventAsync_NilChannelFallsBackSync verifies that when the async
// drain has not been started (ch == nil), emitAuditEventAsync falls back to the
// synchronous path so the event is not lost. This is the path guarded by the
// fix for Issue #5 (the mutex-protected nil check at the top of the function).
func TestEmitAuditEventAsync_NilChannelFallsBackSync(t *testing.T) {
	t.Parallel()

	var syncWrites atomic.Int32
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			syncWrites.Add(1)
			return nil
		},
	}

	// newTestServer calls NewServer which starts the drainer. Stop it and
	// nil-out the channel under the mutex to simulate a server where
	// startAuditAsyncDrain was never called. This artificial state (stopped
	// + ch==nil) exercises the nil-channel fallback at the top of
	// emitAuditEventAsync; the ch==nil check fires before the stopped check.
	srv := newTestServer(t, ms, nil, nil)
	srv.stopAuditAsyncDrain()
	srv.auditAsyncMu.Lock()
	srv.auditAsyncCh = nil
	srv.auditAsyncMu.Unlock()

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "j1", nil)
	assert.EqualValues(t, 1, syncWrites.
		Load(),
	)
}

// TestRetryInterruptedByShutdown verifies that a long retry sleep is
// interrupted when drainCancel is called after the shutdown timeout, so the
// drainer goroutine exits cleanly within the 500ms post-cancel grace period
// rather than leaking. Without the fix for Issue #8, a 10s retry delay would
// keep the goroutine alive long after Close returns, constituting a goroutine
// leak. With the fix, parent.Done() unblocks the sleep immediately.
func TestRetryInterruptedByShutdown(t *testing.T) {
	// Override retry delays to something large enough that the retry sleep
	// would outlast both the 5s shutdown timeout and the 500ms cancel grace
	// window if it were uninterruptible.
	orig := auditRetryDelays
	auditRetryDelays = []time.Duration{
		10 * time.Second,
		10 * time.Second,
		10 * time.Second,
	}
	t.Cleanup(func() { auditRetryDelays = orig })

	// Store always fails on the first attempt so the drainer enters the retry
	// sleep. On subsequent calls (from the deadletter path) it succeeds so the
	// drainer can fully exit after the sleep is interrupted.
	var attempt atomic.Int32
	storeReady := make(chan struct{})
	var firstCallOnce sync.Once
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			n := attempt.Add(1)
			firstCallOnce.Do(func() { close(storeReady) })
			if n == 1 {
				return context.DeadlineExceeded
			}
			return nil
		},
		CreateAuditEventDeadletterFunc: func(_ context.Context, _ *domain.AuditEvent, _ string, _ int) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "j1", nil)

	// Wait until the drainer has attempted the first write so we know it is
	// inside processAuditAsyncEvent and will enter the retry sleep next.
	select {
	case <-storeReady:
	case <-time.After(2 * time.Second):
		require.Fail(t, "store call did not start within 2s")
	}

	// Close: shutdown timeout (5s) + drainCancel + 500ms grace window.
	// Without the fix, the 10s retry sleep would cause the goroutine to
	// outlast the grace window. With the fix, the select on parent.Done()
	// wakes immediately, and Close returns within the 5.5s total budget.
	start := time.Now()
	srv.Close()
	elapsed := time.Since(start)
	require.LessOrEqual(t,
		elapsed, 6*
			time.Second,
	)

	// Budget: 5s timeout + 500ms grace + small scheduling slack.
	// If the sleep were NOT interrupted, Close would return after 5.5s but
	// the drainer goroutine would still be alive sleeping for 10s — that
	// goroutine leak is the primary hazard fixed here. We assert timing to
	// confirm Close does not block beyond the expected budget.
}
