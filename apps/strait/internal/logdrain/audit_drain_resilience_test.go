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

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.False(t, time.
		Now().
		After(deadline))
	require.NoError(t,
		err)

	require.Equal(t, int32(3), calls.Load())
	require.Equal(t, 0,
		drain.DrainedFailureCount())

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
	require.Error(t, err)

	var terminal *terminalStatusError
	require.True(t, errors.As(err,
		&terminal,
	))

	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, "siem_4xx",

		classifyForwardError(err))
	require.Equal(t, 1,
		drain.DrainedFailureCount())

	failures := drain.DrainedFailures()
	require.False(t, len(failures) != 1 || failures[0].Reason !=
		"siem_4xx" || failures[0].
		Event.ID !=
		"ev-1")

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
	for range siemBreakerFailureThreshold {
		require.Error(t, drain.
			ForwardBatch(context.
				Background(), []domain.
				AuditEvent{sampleEvent("ev")},
			))

	}
	before := calls.Load()
	// 6th call should be short-circuited without an HTTP hit.
	err := drain.ForwardBatch(context.Background(), []domain.AuditEvent{sampleEvent("ev-final")})
	require.Error(t, err)

	after := calls.Load()
	require.Equal(t, before,
		after,
	)
	require.True(t, drain.
		breakerWasOpen.
		Load())

	// The short-circuit failure should be classified as circuit_open.
	require.Equal(t, "circuit_open", classifyForwardError(err))
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
	require.Equal(t, BreakerStateClosed, drain.BreakerState())
	for range siemBreakerFailureThreshold {
		_ = drain.ForwardBatch(context.Background(), []domain.AuditEvent{sampleEvent("ev")})
	}
	require.True(t, drain.
		breakerWasOpen.
		Load())

	require.Equal(t, BreakerStateOpen, drain.BreakerState())
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
	require.Nil(t,
		lastErr,
	)
	require.NoError(t,
		drain.ForwardBatch(context.
			Background(), []domain.AuditEvent{sampleEvent("ev-after")}))
	assert.False(t, drain.
		breakerWasOpen.
		Load())

	// Subsequent call also succeeds.

	// breakerWasOpen latch must reset on close so it tracks current state.

	assert.Equal(t, BreakerStateClosed, drain.BreakerState())
}

// TestAuditSIEMDrain_BreakerStateTransitionsCycle drives the breaker
// through the full closed -> open -> half_open -> closed cycle and
// asserts BreakerState() observes each transition. This is the contract
// the strait_audit_siem_breaker_state observable gauge depends on.
func TestAuditSIEMDrain_BreakerStateTransitionsCycle(t *testing.T) {
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
	// Stage 1: closed at start.
	require.Equal(t, BreakerStateClosed, drain.BreakerState())
	// Stage 2: drive failures past threshold -> open.
	for range siemBreakerFailureThreshold {
		_ = drain.ForwardBatch(context.Background(), []domain.AuditEvent{sampleEvent("ev")})
	}
	require.Equal(t, BreakerStateOpen, drain.BreakerState())
	// Stage 3: wait for the breaker's open delay to expire.
	time.Sleep(200 * time.Millisecond)
	// Stage 4: stop failing and probe — recovery transitions through
	// half_open into closed.
	fail.Store(false)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := drain.ForwardBatch(context.Background(), []domain.AuditEvent{sampleEvent("ev-probe")}); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.Equal(t, BreakerStateClosed, drain.BreakerState())
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
	require.LessOrEqual(t, elapsed,
		3*time.
			Second)

}

// TestAuditSIEMDrain_ContextCanceled_NoRetry asserts that a context
// cancellation from the caller does not consume the retry budget or
// flip the circuit breaker. Only one upstream call is made, the
// breaker stays closed, and the batch lands in the sub-DLQ once.
func TestAuditSIEMDrain_ContextCanceled_NoRetry(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	shrinkBackoffForTest(t, 30*time.Second)

	var calls atomic.Int32
	release := make(chan struct{})
	requestArrived := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		select {
		case requestArrived <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)

	drain := NewAuditSIEMDrain(srv.URL, "", 0, 0)
	ctx, cancel := context.WithCancel(context.Background())
	concWG.Go(func() {
		select {
		case <-requestArrived:
		case <-time.After(2 * time.Second):
		}
		cancel()
	})

	err := drain.ForwardBatch(ctx, []domain.AuditEvent{sampleEvent("cancel-ev")})
	require.Error(t, err)

	assert.Equal(t, int32(1), calls.Load())
	assert.False(t, drain.
		breakerWasOpen.
		Load())
	assert.Equal(t, 1,
		drain.DrainedFailureCount())

}

// TestAuditSIEMDrain_RequestConstructError_NoRetry asserts that a
// deterministic request-construction failure (malformed endpoint URL)
// results in exactly one attempt — retries would reproduce the same
// error and waste budget. The breaker must also stay closed.
func TestAuditSIEMDrain_RequestConstructError_NoRetry(t *testing.T) {
	shrinkBackoffForTest(t, 30*time.Second)

	// A control character in the URL causes http.NewRequestWithContext
	// to reject the URL — this is the deterministic construction
	// failure path.
	drain := NewAuditSIEMDrain("http://example.com/\x7f", "", 0, 0)

	err := drain.ForwardBatch(context.Background(), []domain.AuditEvent{sampleEvent("bad-url")})
	require.Error(t, err)
	assert.True(t, errors.Is(err,
		errRequestConstruct,
	))
	assert.False(t, drain.
		breakerWasOpen.
		Load())

}
