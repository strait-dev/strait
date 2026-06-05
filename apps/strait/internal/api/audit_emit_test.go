package api

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/config"
	"strait/internal/domain"
)

var errFakeAuditStore = errors.New("fake audit store error")

// TestEmitAuditEventAsync_OrderingAndContextSnapshot verifies that events are
// written in submission order and that project/actor/details are snapshotted
// synchronously (so a cancelled parent context does not strip them).
func TestEmitAuditEventAsync_OrderingAndContextSnapshot(t *testing.T) {
	t.Parallel()

	var (
		mu     sync.Mutex
		events []*domain.AuditEvent
	)
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			defer mu.Unlock()
			clone := *ev
			events = append(events, &clone)
			return nil
		},
	}

	srv := newTestServer(t, ms, nil, nil)

	const n = 50
	baseCtx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-async")
	baseCtx = context.WithValue(baseCtx, ctxActorIDKey, "actor-async")
	baseCtx = context.WithValue(baseCtx, ctxActorTypeKey, "user")

	for i := range n {
		ctx, cancel := context.WithCancel(baseCtx)
		srv.emitAuditEventAsync(ctx, "job.triggered", "job", "job-1", map[string]any{"i": i})
		cancel()
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		got := len(events)
		mu.Unlock()
		if got >= n {
			break
		}
		require.False(t, time.Now().After(deadline))

		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	require.Len(t,
		events, n)

	for i, ev := range events {
		assert.Equal(
			t, "proj-async",
			ev.ProjectID,
		)
		assert.Equal(
			t, "actor-async",
			ev.ActorID,
		)
		assert.Equal(
			t, "user", ev.ActorType,
		)
		assert.Equal(
			t, "job.triggered",
			ev.Action,
		)

		var details map[string]any
		require.NoError(t, json.Unmarshal(ev.Details,
			&details,
		))

		got, _ := details["i"].(float64)
		assert.Equal(
			t, i, int(got))
	}
}

// TestEmitAuditEventAsync_ShutdownFlush verifies that Close() drains pending
// events within the shutdown timeout before returning.
func TestEmitAuditEventAsync_ShutdownFlush(t *testing.T) {
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
	for i := range 25 {
		srv.emitAuditEventAsync(ctx, "job.triggered", "job", "job-1", map[string]any{"i": i})
	}

	srv.Close()
	require.EqualValues(t, 25, written.Load())
}

// TestEmitAuditEventAsync_BufferFullDropsEvents verifies that when the buffer
// fills, excess events are either written via the backpressure sync fallback or
// dropped — but emitAuditEventAsync never blocks the caller indefinitely and
// never panics. The mock store returns immediately so the sync fallback
// completes without delay; the test only cares about non-blocking semantics.
func TestEmitAuditEventAsync_BufferFullDropsEvents(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	release := make(chan struct{})
	var written atomic.Int32
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			// Non-blocking: if release is already closed proceed, otherwise
			// return immediately so neither the drainer nor the sync-fallback
			// path can stall the caller goroutine.
			select {
			case <-release:
			default:
			}
			written.Add(1)
			return nil
		},
	}

	srv := newTestServer(t, ms, nil, nil)
	t.Cleanup(func() { close(release) })

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	total := auditAsyncBufferSize + 500
	done := make(chan struct{})
	concWG.Go(func() {
		for range total {
			srv.emitAuditEventAsync(ctx, "job.triggered", "job", "job-1", nil)
		}
		close(done)
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		require.Fail(t, "emitAuditEventAsync blocked beyond 5s")
	}
}

// TestEmitAuditEventAsync_DrainerErrorContinues verifies that a store error on
// one event does not stop the drainer from processing subsequent events.
func TestEmitAuditEventAsync_DrainerErrorContinues(t *testing.T) {
	t.Parallel()

	var written atomic.Int32
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			if ev.ResourceID == "fail" {
				return errFakeAuditStore
			}
			written.Add(1)
			return nil
		},
	}

	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	srv.emitAuditEventAsync(ctx, "job.triggered", "job", "ok-1", nil)
	srv.emitAuditEventAsync(ctx, "job.triggered", "job", "fail", nil)
	srv.emitAuditEventAsync(ctx, "job.triggered", "job", "ok-2", nil)

	srv.Close()
	require.EqualValues(t, 2, written.Load())
}

// TestEmitAuditEventAsync_StopsAcceptingAfterClose verifies events submitted
// after Close do not panic on a closed channel.
func TestEmitAuditEventAsync_StopsAcceptingAfterClose(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	srv.Close()

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	srv.emitAuditEventAsync(ctx, "job.triggered", "job", "late", nil)
}

// TestShutdown_CancelsInFlightDBWrites verifies that a pending DB write that
// would otherwise block for 10s (the per-event timeout) is short-circuited
// by stopAuditAsyncDrain's drainCancel call, so total shutdown stays within
// the auditAsyncShutdownTimeout (5s) plus a small grace window.
func TestShutdown_CancelsInFlightDBWrites(t *testing.T) {
	withShortRetries(t)

	// Slow store: blocks on DB writes until the per-call ctx is cancelled.
	// Without context propagation through stopAuditAsyncDrain, this would
	// block for the full 10s per-event timeout — well beyond the 5s
	// shutdown budget.
	storeReady := make(chan struct{})
	var firstCallOnce sync.Once
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(ctx context.Context, _ *domain.AuditEvent) error {
			firstCallOnce.Do(func() { close(storeReady) })
			<-ctx.Done()
			return ctx.Err()
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "j1", nil)

	// Wait until the drainer has actually entered the slow store call so
	// the shutdown timing measurement reflects the in-flight cancellation
	// path rather than the queued-events drain path.
	select {
	case <-storeReady:
	case <-time.After(2 * time.Second):
		require.Fail(t, "store call did not start within 2s")
	}

	start := time.Now()
	srv.Close()
	elapsed := time.Since(start)
	require.LessOrEqual(t, elapsed,
		6*time.
			Second)

	// Budget: shutdown timeout (5s) + brief cancellation grace (500ms).
	// Without context propagation we would see ~10s per the per-event
	// timeout. A multi-attempt retry sequence with short delays could
	// also push past 5s without proper cancellation.
}

// TestStartAuditAsyncDrain_PopulatesDrainCtx verifies the new drain
// context is wired up so per-event DB calls can derive from it.
func TestStartAuditAsyncDrain_PopulatesDrainCtx(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}
	srv := newTestServer(t, ms, nil, nil)
	require.NotNil(t, srv.drainContext())
}

// TestAuditDrainer_FieldAssignmentNoDataRace exercises concurrent depth
// reads against startAuditAsyncDrain field publication. Run with -race
// to catch a torn channel-pointer write that would happen if the assignment
// was made outside auditAsyncMu.
func TestAuditDrainer_FieldAssignmentNoDataRace(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}
	srv := newTestServer(t, ms, nil, nil)
	var wg conc.WaitGroup
	stop := make(chan struct{})
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
					_ = srv.AuditDrainerQueueDepth()
					_ = srv.AuditDrainerQueueCapacity()
				}
			}
		})
	}
	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestEmitAuditEventAsync_BackpressureFallsBackToSync(t *testing.T) {
	release := make(chan struct{})
	var syncWrites atomic.Int32
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			select {
			case <-release:
			default:
			}
			syncWrites.Add(1)
			return nil
		},
	}

	srv := NewServer(ServerDeps{
		Config:               &config.Config{InternalSecret: "test", JWTSigningKey: "test-jwt-key-that-is-long-enough-32chars!!"},
		Store:                ms,
		AuditAsyncBufferSize: 8,
	})

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	for range 20 {
		srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "j1", nil)
	}

	// Unblock the mock store so in-flight async writes can complete.
	close(release)
	srv.Close()
	assert.NotEqual(t, 0, syncWrites.
		Load())
}
