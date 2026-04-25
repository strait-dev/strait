package logdrain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sourcegraph/conc"

	"strait/internal/domain"
)

// TestSIEMDrain_FlushNowAndRunNeverRace starts the drain, enqueues events from
// multiple goroutines, calls FlushNow concurrently, then stops. Run with -race
// to confirm there is no data race between the run loop's shutdown drain path
// and FlushNow reading from d.ch (the fix for Issue #7).
func TestSIEMDrain_FlushNowAndRunNeverRace(t *testing.T) {
	t.Parallel()

	// HTTP server that accepts all requests immediately so flushes do not
	// stall the test. The actual forwarding outcome is irrelevant — we only
	// care about the absence of data races.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	drain := NewAuditSIEMDrain(srv.URL, "", 10, 5*time.Millisecond)
	drain.Start(context.Background())

	// Enqueue 50 events from multiple goroutines.
	const producers = 5
	const eventsPerProducer = 10
	var enqueueWg conc.WaitGroup
	for range producers {
		enqueueWg.Go(func() {
			for range eventsPerProducer {
				drain.Enqueue(domain.AuditEvent{
					ID:     "ev",
					Action: domain.AuditActionJobTriggered,
				})
			}
		})
	}
	enqueueWg.Wait()

	// Call FlushNow 5 times concurrently while the run loop is also active.
	const flushCalls = 5
	var flushWg conc.WaitGroup
	for range flushCalls {
		flushWg.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = drain.FlushNow(ctx)
		})
	}
	flushWg.Wait()

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
	drain.Stop(stopCtx)
	stopCancel()
}
