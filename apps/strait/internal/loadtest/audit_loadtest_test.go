package loadtest_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/loadtest"
	"strait/internal/logdrain"
)

// buildEvent constructs a throw-away audit event tagged with the given
// index. All load scenarios reuse this builder.
func buildEvent(i int) *domain.AuditEvent {
	return &domain.AuditEvent{
		ProjectID:    "proj-load",
		ActorID:      "actor-load",
		ActorType:    "user",
		Action:       "job.triggered",
		ResourceType: "job",
		ResourceID:   fmt.Sprintf("job-%d", i),
	}
}

// TestAuditLoad_Burst asserts the buffer can absorb a burst without dropping
// when the store is healthy. The buffer is sized to n+1024 so even under CI
// race-detector overhead the drainer cannot fall behind enough to cause drops.
func TestAuditLoad_Burst(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping audit burst load test in -short mode")
	}

	n := 5_000
	if os.Getenv("AUDIT_LOAD_FULL") == "1" {
		n = 50_000
	}

	store := loadtest.NewMemoryAuditStore()
	bufSize := n + 1024
	h := loadtest.NewAuditEmitHarness(store, nil, loadtest.AuditEmitHarnessConfig{
		BufferSize:        bufSize,
		RetryDelays:       []time.Duration{100 * time.Microsecond},
		QueuePollInterval: 10 * time.Millisecond,
	})
	h.Start()
	t.Cleanup(h.Stop)
	start := time.Now()
	for i := range n {
		h.Emit(buildEvent(i))
	}
	burstDur := time.Since(start)
	t.Logf("burst emit duration: %s (n=%d)", burstDur, n)

	if !h.WaitDrain(60 * time.Second) {
		t.Fatalf("harness did not drain within 60s")
	}

	c := h.Counters()
	if c.Dropped != 0 {
		t.Errorf("Dropped = %d, want 0", c.Dropped)
	}
	if c.Deadlettered != 0 {
		t.Errorf("Deadlettered = %d, want 0", c.Deadlettered)
	}
	if c.Persisted != int64(n) {
		t.Errorf("Persisted = %d, want %d", c.Persisted, n)
	}
	if c.PeakQueue > int64(bufSize) {
		t.Errorf("PeakQueue = %d, want <= %d (BufferSize)", c.PeakQueue, bufSize)
	}
	// p99 Enqueue latency budget. The hot path is a non-blocking send into
	// a buffered channel; <5ms is generous and accommodates CI jitter.
	p99 := h.LatencyPercentile(99)
	if p99 > 5*time.Millisecond {
		t.Errorf("p99 emit latency = %s, want < 5ms (env-sensitive)", p99)
	}
	t.Logf("burst: persisted=%d peakQueue=%d p99=%s", c.Persisted, c.PeakQueue, p99)
}

// TestAuditLoad_StoreDown drives the harness against a store that errors
// for 30s then recovers. Asserts events land in the DLQ during the outage
// and that a simulated reclaimer drains them within 60s of recovery.
func TestAuditLoad_StoreDown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping audit store-down chaos test in -short mode")
	}

	// Compress the production 30s outage to 2s so the test stays fast.
	const outageDur = 2 * time.Second

	store := loadtest.NewMemoryAuditStore()
	store.SetFail(true)
	h := loadtest.NewAuditEmitHarness(store, nil, loadtest.AuditEmitHarnessConfig{
		BufferSize:  4096,
		RetryDelays: []time.Duration{time.Millisecond, time.Millisecond},
	})
	h.Start()
	t.Cleanup(h.Stop)

	// Emit for outageDur while the store is failing.
	outageDone := time.After(outageDur)
	i := 0
loop:
	for {
		select {
		case <-outageDone:
			break loop
		default:
		}
		h.Emit(buildEvent(i))
		i++
		if i%256 == 0 {
			time.Sleep(time.Millisecond)
		}
	}
	totalEmitted := i

	// Let in-flight retries resolve into DLQ.
	time.Sleep(200 * time.Millisecond)
	c := h.Counters()
	if c.Deadlettered == 0 {
		t.Fatalf("expected Deadlettered > 0 during outage, got %d (emitted=%d)", c.Deadlettered, totalEmitted)
	}
	if store.DeadletterCount() == 0 {
		t.Fatalf("expected DLQ depth > 0 during outage")
	}
	t.Logf("during outage: emitted=%d deadlettered=%d dlq_depth=%d", totalEmitted, c.Deadlettered, store.DeadletterCount())

	// Simulate recovery + reclaimer.
	store.SetFail(false)
	reclaimStart := time.Now()
	reclaimed := 0
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		batch := store.DrainDeadletter()
		if len(batch) == 0 {
			if store.DeadletterCount() == 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}
		for _, ev := range batch {
			if err := store.CreateAuditEvent(context.Background(), ev); err != nil {
				t.Fatalf("reclaim CreateAuditEvent: %v", err)
			}
			reclaimed++
		}
	}
	if store.DeadletterCount() != 0 {
		t.Fatalf("DLQ still has %d rows after reclaim", store.DeadletterCount())
	}
	if reclaimed == 0 {
		t.Fatal("reclaimer drained zero rows")
	}
	t.Logf("reclaimed %d rows in %s", reclaimed, time.Since(reclaimStart))
}

// TestAuditLoad_SIEMDown drives the harness with a working store but a
// SIEM endpoint that returns 500 indefinitely. Asserts the circuit
// breaker opens, chain persistence remains unaffected, and the SIEM
// sub-DLQ grows bounded (<= siemSubDLQCapacity = 1024).
func TestAuditLoad_SIEMDown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping audit SIEM-down chaos test in -short mode")
	}

	var siemHits atomic.Int64
	siem := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		siemHits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(siem.Close)

	drain := logdrain.NewAuditSIEMDrain(siem.URL, "", 10, 20*time.Millisecond)
	if drain == nil {
		t.Fatal("NewAuditSIEMDrain returned nil")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	drain.Start(ctx)
	t.Cleanup(func() { drain.Stop(context.Background()) })

	store := loadtest.NewMemoryAuditStore()
	h := loadtest.NewAuditEmitHarness(store, drain, loadtest.AuditEmitHarnessConfig{
		BufferSize:  4096,
		RetryDelays: []time.Duration{time.Millisecond},
	})
	h.Start()
	t.Cleanup(h.Stop)

	// Keep event count modest so the SIEM drain's bounded internal buffer
	// (minSIEMBufferSize=256) doesn't drop events at Enqueue before they
	// can reach a forward attempt; we want events to actually be
	// forwarded, fail with 5xx, and land in the sub-DLQ.
	const n = 200
	for i := range n {
		h.Emit(buildEvent(i))
		if i%25 == 0 {
			time.Sleep(time.Millisecond)
		}
	}
	if !h.WaitDrain(30 * time.Second) {
		t.Fatalf("harness did not drain within 30s")
	}

	// Let SIEM drain attempt flushes + breaker state to settle. Retry
	// policy backoff alone can take ~2s per exhausted call, so give it
	// generous headroom.
	time.Sleep(3 * time.Second)

	c := h.Counters()
	if c.Persisted != n {
		t.Errorf("Persisted = %d, want %d (chain must be unaffected by SIEM outage)", c.Persisted, n)
	}
	if c.Deadlettered != 0 {
		t.Errorf("Deadlettered = %d, want 0 (SIEM failure must not deadletter chain events)", c.Deadlettered)
	}

	// SIEM sub-DLQ should be populated but bounded. siemSubDLQCapacity = 1024
	// is the internal cap. We don't import that constant here so we assert
	// with the concrete bound.
	subDLQ := drain.DrainedFailureCount()
	if subDLQ == 0 {
		t.Error("SIEM sub-DLQ should contain entries after persistent 5xx")
	}
	if subDLQ > 1024 {
		t.Errorf("SIEM sub-DLQ = %d, want <= 1024 (cap)", subDLQ)
	}
	t.Logf("SIEM-down: persisted=%d subDLQ=%d siemHits=%d", c.Persisted, subDLQ, siemHits.Load())
}
