package logdrain

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

// shrinkBackoffForTest collapses the production retry/breaker delays to
// millisecond-scale so resilience tests do not hang. Restored by the
// returned cleanup func.
func shrinkBackoffForTest(t *testing.T, breakerOpen time.Duration) {
	t.Helper()
	origInit := siemRetryInitialBackoff
	origMax := siemRetryMaxBackoff
	origFactor := siemRetryBackoffFactor
	origBreaker := siemBreakerOpenDuration
	siemRetryInitialBackoff = 5 * time.Millisecond
	siemRetryMaxBackoff = 20 * time.Millisecond
	siemRetryBackoffFactor = 2.0
	siemBreakerOpenDuration = breakerOpen
	t.Cleanup(func() {
		siemRetryInitialBackoff = origInit
		siemRetryMaxBackoff = origMax
		siemRetryBackoffFactor = origFactor
		siemBreakerOpenDuration = origBreaker
	})
}

func sampleEvent(id string) domain.AuditEvent {
	return domain.AuditEvent{
		ID:           id,
		ProjectID:    "p1",
		ActorID:      "a1",
		ActorType:    "user",
		Action:       "job.created",
		ResourceType: "job",
		ResourceID:   "r1",
		Details:      []byte(`{"name":"n","runtime":"docker"}`),
	}
}

func TestAuditSIEMDrain_RetryOn5xx(t *testing.T) {
	shrinkBackoffForTest(t, 30*time.Second)

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	drain := NewAuditSIEMDrain(srv.URL, "", 0, 0)
	deadline := time.Now().Add(3 * time.Second)
	err := drain.ForwardBatch(context.Background(), []domain.AuditEvent{sampleEvent("ev-1")})
	if time.Now().After(deadline) {
		t.Fatalf("ForwardBatch took too long")
	}
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("expected 3 HTTP calls (2 retries), got %d", got)
	}
	if drain.DrainedFailureCount() != 0 {
		t.Fatalf("expected sub-DLQ empty, got %d", drain.DrainedFailureCount())
	}
}

func TestAuditSIEMDrain_NoRetryOn4xx(t *testing.T) {
	shrinkBackoffForTest(t, 30*time.Second)

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	drain := NewAuditSIEMDrain(srv.URL, "", 0, 0)
	err := drain.ForwardBatch(context.Background(), []domain.AuditEvent{sampleEvent("ev-1")})
	if err == nil {
		t.Fatal("expected error for 4xx")
	}
	var terminal *terminalStatusError
	if !errors.As(err, &terminal) {
		t.Fatalf("expected terminalStatusError, got %T: %v", err, err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 HTTP call, got %d", got)
	}
	if classifyForwardError(err) != "siem_4xx" {
		t.Fatalf("expected reason siem_4xx, got %s", classifyForwardError(err))
	}
	if drain.DrainedFailureCount() != 1 {
		t.Fatalf("expected sub-DLQ size 1, got %d", drain.DrainedFailureCount())
	}
	failures := drain.DrainedFailures()
	if len(failures) != 1 || failures[0].Reason != "siem_4xx" || failures[0].Event.ID != "ev-1" {
		t.Fatalf("unexpected sub-DLQ contents: %+v", failures)
	}
}

func TestAuditSIEMDrain_CircuitOpens(t *testing.T) {
	shrinkBackoffForTest(t, 30*time.Second)

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	drain := NewAuditSIEMDrain(srv.URL, "", 0, 0)

	// 5 consecutive batch-forward calls, each exhausts retries — each
	// counts as 1 failure toward the breaker's threshold.
	for i := 0; i < siemBreakerFailureThreshold; i++ {
		if err := drain.ForwardBatch(context.Background(), []domain.AuditEvent{sampleEvent("ev")}); err == nil {
			t.Fatalf("iteration %d: expected error", i)
		}
	}
	before := calls.Load()
	// 6th call should be short-circuited without an HTTP hit.
	err := drain.ForwardBatch(context.Background(), []domain.AuditEvent{sampleEvent("ev-final")})
	if err == nil {
		t.Fatal("expected error when circuit open")
	}
	after := calls.Load()
	if after != before {
		t.Fatalf("expected no new HTTP calls while open, got %d -> %d", before, after)
	}
	if !drain.breakerWasOpen.Load() {
		t.Fatal("expected breaker to have opened")
	}
	// The short-circuit failure should be classified as circuit_open.
	if got := classifyForwardError(err); got != "circuit_open" {
		t.Fatalf("expected reason circuit_open, got %s (err=%v)", got, err)
	}
}

func TestAuditSIEMDrain_CircuitHalfOpenRecovery(t *testing.T) {
	shrinkBackoffForTest(t, 100*time.Millisecond)

	var fail atomic.Bool
	fail.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	drain := NewAuditSIEMDrain(srv.URL, "", 0, 0)
	for i := 0; i < siemBreakerFailureThreshold; i++ {
		_ = drain.ForwardBatch(context.Background(), []domain.AuditEvent{sampleEvent("ev")})
	}
	if !drain.breakerWasOpen.Load() {
		t.Fatal("expected breaker open after threshold")
	}
	// Wait for half-open transition.
	time.Sleep(200 * time.Millisecond)
	// Stop failing.
	fail.Store(false)
	// Try until the breaker closes (half-open probe should pass and reset).
	deadline := time.Now().Add(2 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		lastErr = drain.ForwardBatch(context.Background(), []domain.AuditEvent{sampleEvent("ev-probe")})
		if lastErr == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("expected breaker to recover, got %v", lastErr)
	}
	// Subsequent call also succeeds.
	if err := drain.ForwardBatch(context.Background(), []domain.AuditEvent{sampleEvent("ev-after")}); err != nil {
		t.Fatalf("post-recovery call failed: %v", err)
	}
}

func TestAuditSIEMDrain_JitterBackoffBounded(t *testing.T) {
	shrinkBackoffForTest(t, 30*time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	drain := NewAuditSIEMDrain(srv.URL, "", 0, 0)
	start := time.Now()
	_ = drain.ForwardBatch(context.Background(), []domain.AuditEvent{sampleEvent("ev")})
	elapsed := time.Since(start)
	if elapsed > 3*time.Second {
		t.Fatalf("retry sequence exceeded 3s bound: %v", elapsed)
	}
}
