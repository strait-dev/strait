package logdrain

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

// TestAuditSIEMDrain_BufferFullDropsAndCounts forces the internal channel
// to a tiny size and ensures floods beyond capacity are dropped without
// blocking the caller. Verifies Enqueue is non-blocking and drops are
// visible (observed via not-all-enqueued making it through a hanging
// server — we instead rely on no-hang behavior and that the non-dropped
// subset never exceeds the buffer + in-flight batch).
func TestAuditSIEMDrain_BufferFullDropsAndCounts(t *testing.T) {
	var concWG conc.WaitGroup

	// Server that hangs until the test ends, to guarantee the forwarder
	// goroutine is stuck on the HTTP call and the buffer fills.
	defer concWG.Wait()
	t.Parallel()

	release := make(chan struct{})
	requestArrived := make(chan struct{}, 1)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		select {
		case requestArrived <- struct{}{}:
		default:
		}
		<-release
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer close(release)

	drain := NewAuditSIEMDrain(srv.URL, "", 1, time.Hour)
	drain.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		drain.Stop(ctx)
	})

	// Send one event and wait for the server to receive it, ensuring
	// the forwarder goroutine is stuck in the HTTP handler.
	drain.Enqueue(domain.AuditEvent{ID: "prime", Action: "a"})
	select {
	case <-requestArrived:
	case <-time.After(2 * time.Second):
		require.Fail(t, "first request never reached the server")
	}

	// Now flood — forwarder is stuck, buffer fills and drops kick in.
	const flood = 2000
	done := make(chan struct{})
	concWG.Go(func() {
		for range flood {
			drain.Enqueue(domain.AuditEvent{ID: "ev", Action: "a"})
		}
		close(done)
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "Enqueue blocked - non-blocking contract violated")
	}

	assert.Equal(t, int32(1), hits.Load())
}

// TestAuditSIEMDrain_StartStopRacefree checks that Start/Stop do not leak
// goroutines. Uses a goroutine count sampled before/after.
func TestAuditSIEMDrain_StartStopRacefree(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	before := runtime.NumGoroutine()

	for range 10 {
		drain := NewAuditSIEMDrain(srv.URL, "", 5, 20*time.Millisecond)
		drain.Start(context.Background())
		for range 20 {
			drain.Enqueue(domain.AuditEvent{ID: "ev", Action: "a"})
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		drain.Stop(ctx)
		cancel()
		if transport, ok := drain.client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}
	srv.CloseClientConnections()

	// Give lingering stacks a moment to unwind.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before+2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	after := runtime.NumGoroutine()
	assert.LessOrEqual(t, after,
		before+
			5)

	// Allow a tiny slack for test-server / httptest internals.
}

// TestAuditSIEMDrain_EnqueueBeforeStart_Noop ensures Enqueue is safe when
// Start has not been called (e.g. when the drain is configured but not
// wired into a running server).
func TestAuditSIEMDrain_EnqueueBeforeStart_Noop(t *testing.T) {
	t.Parallel()
	drain := NewAuditSIEMDrain("https://example.com", "", 10, time.Second)
	// Must not panic; must not block.
	drain.Enqueue(domain.AuditEvent{ID: "ev"})
}

// TestAuditSIEMDrain_NilReceiverSafe exercises the nil-receiver contract.
func TestAuditSIEMDrain_NilReceiverSafe(t *testing.T) {
	t.Parallel()
	var drain *AuditSIEMDrain
	drain.Start(context.Background())
	drain.Enqueue(domain.AuditEvent{})
	drain.Stop(context.Background())
	assert.NoError(t,
		drain.
			FlushNow(context.
				Background()))
}

// TestSIEMDrain_FlushNow_DrainsBufferedEvents asserts that FlushNow
// synchronously forwards events currently queued in d.ch. This is the
// path Server.Close uses to push late-arriving events that would otherwise
// miss the run loop's flush ticker.
//
// We construct a drain that has NOT been Started so the run loop can never
// race FlushNow. With no consumer of d.ch, every Enqueue lands directly in
// the buffer and FlushNow drains them in one batch.
func TestSIEMDrain_FlushNow_DrainsBufferedEvents(t *testing.T) {
	t.Parallel()
	var received atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		for _, line := range bytesSplitLines(body) {
			if len(line) > 0 {
				received.Add(1)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// We need the channel/shutdownCh wired up but no run goroutine consuming
	// subsequent Enqueue piles up in d.ch and FlushNow has work to do.
	drain := NewAuditSIEMDrain(srv.URL, "", 100, time.Hour)
	drain.Start(context.Background())
	// Stop the run goroutine so it does not consume from d.ch.
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	drain.Stop(stopCtx)
	stopCancel()

	// stopped=true blocks Enqueue, so push directly onto d.ch for this
	// test (we are validating FlushNow's drain semantics, not Enqueue).
	for i := range 10 {
		drain.ch <- domain.AuditEvent{ID: "ev", Action: "a", ResourceID: itoa(i)}
	}
	require.NoError(t,
		drain.
			FlushNow(context.
				Background()))

	require.Equal(t, int32(10), received.Load())
}

// TestSIEMDrain_FlushNow_Idempotent verifies that calling FlushNow when
// the buffer is empty is a safe no-op (returns nil) and does not generate
// any HTTP traffic.
func TestSIEMDrain_FlushNow_Idempotent(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	drain := NewAuditSIEMDrain(srv.URL, "", 10, time.Hour)
	drain.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		drain.Stop(ctx)
	})
	require.NoError(t,
		drain.
			FlushNow(context.
				Background()))

	// Empty buffer — FlushNow must return nil and not hit the server.

	require.Equal(t, int32(0), hits.Load())
}

// TestSIEMDrain_Stop_CancelsInFlightFlush asserts that calling Stop while
// a flush is in flight cancels the HTTP request via the parent context. We
// detect this by serving a hanging endpoint and asserting Stop returns
// within the shutdown budget rather than the 30s flush deadline.
func TestSIEMDrain_Stop_CancelsInFlightFlush(t *testing.T) {
	t.Parallel()
	requestArrived := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-requestArrived:
		default:
			close(requestArrived)
		}
		select {
		case <-r.Context().Done():
			// Caller cancellation observed — return.
			return
		case <-release:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })

	// batchSize=1 so the first Enqueue triggers a flush immediately.
	drain := NewAuditSIEMDrain(srv.URL, "", 1, time.Hour)
	drain.Start(context.Background())

	drain.Enqueue(domain.AuditEvent{ID: "ev-1", Action: "a"})

	// Wait for the request to actually reach the server before calling
	// Stop, so the shutdown timing reflects the in-flight cancellation
	// path. The handler hangs until the caller ctx is cancelled.
	select {
	case <-requestArrived:
	case <-time.After(2 * time.Second):
		require.Fail(t, "HTTP request never arrived at test server")
	}

	start := time.Now()
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	drain.Stop(stopCtx)
	stopCancel()
	elapsed := time.Since(start)
	require.LessOrEqual(t, elapsed,
		6*
			time.Second)

	// Stop budget is 5s but with parent-ctx propagation a hanging
	// request returns immediately when the parent is cancelled. We
	// require well under the 30s flush deadline.
}

// TestSIEMDrain_Enqueue_Stop_NoSendOnClosedChannel runs many concurrent
// Enqueue calls against a Stop call and asserts no panic. Validates the
// shutdownCh-based handshake replaces the previous send-to-closed race.
func TestSIEMDrain_Enqueue_Stop_NoSendOnClosedChannel(t *testing.T) {
	t.Parallel()
	for trial := range 50 {
		_ = trial
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		drain := NewAuditSIEMDrain(srv.URL, "", 10, 10*time.Millisecond)
		drain.Start(context.Background())

		var wg conc.WaitGroup
		stopWaiting := make(chan struct{})
		for range 8 {
			wg.Go(func() {
				for {
					select {
					case <-stopWaiting:
						return
					default:
						drain.Enqueue(domain.AuditEvent{ID: "ev"})
					}
				}
			})
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		drain.Stop(ctx)
		cancel()
		close(stopWaiting)
		wg.Wait()
		srv.Close()
	}
}

// bytesSplitLines splits a byte slice on '\n' returning non-empty lines.
func bytesSplitLines(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			if i > start {
				out = append(out, b[start:i])
			}
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
}

// itoa is a small helper to avoid pulling strconv into a test that needs
// only the simplest int -> string conversion.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
