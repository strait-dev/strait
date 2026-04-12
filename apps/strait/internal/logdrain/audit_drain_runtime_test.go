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

	"strait/internal/domain"
)

// TestAuditSIEMDrain_BufferFullDropsAndCounts forces the internal channel
// to a tiny size and ensures floods beyond capacity are dropped without
// blocking the caller. Verifies Enqueue is non-blocking and drops are
// visible (observed via not-all-enqueued making it through a hanging
// server — we instead rely on no-hang behavior and that the non-dropped
// subset never exceeds the buffer + in-flight batch).
func TestAuditSIEMDrain_BufferFullDropsAndCounts(t *testing.T) {
	t.Parallel()

	// Server that hangs until the test ends, to guarantee the forwarder
	// goroutine is stuck on the HTTP call and the buffer fills.
	release := make(chan struct{})
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-release
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer close(release)

	// batchSize=1 -> buffer = max(4*1, 256) = 256. To force drops quickly we
	// use a tight loop well above 256.
	drain := NewAuditSIEMDrain(srv.URL, "", 1, time.Hour)
	drain.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		drain.Stop(ctx)
	})

	// Prime: fill the buffer well beyond cap. With server hanging, the
	// forwarder has consumed 1 event (stuck in flight), and the rest pile
	// up until the buffer fills. Any Enqueue beyond that must drop.
	const flood = 2000
	done := make(chan struct{})
	go func() {
		for i := 0; i < flood; i++ {
			drain.Enqueue(domain.AuditEvent{ID: "ev", Action: "a"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Enqueue blocked — non-blocking contract violated")
	}

	// With buffer=256 plus 1 in-flight, we must have dropped a large chunk.
	// We cannot read the counter directly (package-private OTel counter),
	// but we assert: flood completed without blocking AND the server was
	// hit exactly once (the single in-flight request).
	if got := hits.Load(); got != 1 {
		t.Errorf("expected exactly 1 in-flight request while hanging; got %d", got)
	}
}

// TestAuditSIEMDrain_StartStopRacefree checks that Start/Stop do not leak
// goroutines. Uses a goroutine count sampled before/after.
func TestAuditSIEMDrain_StartStopRacefree(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	before := runtime.NumGoroutine()

	for i := 0; i < 10; i++ {
		drain := NewAuditSIEMDrain(srv.URL, "", 5, 20*time.Millisecond)
		drain.Start(context.Background())
		for j := 0; j < 20; j++ {
			drain.Enqueue(domain.AuditEvent{ID: "ev", Action: "a"})
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		drain.Stop(ctx)
		cancel()
	}

	// Give lingering stacks a moment to unwind.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before+2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	after := runtime.NumGoroutine()
	// Allow a tiny slack for test-server / httptest internals.
	if after > before+5 {
		t.Errorf("goroutine leak: before=%d after=%d", before, after)
	}
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
}
