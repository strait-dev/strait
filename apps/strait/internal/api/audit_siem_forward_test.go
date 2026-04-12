package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/logdrain"
)

// newServerWithSIEM constructs a minimal server wired with a live SIEM drain.
// The drain's Start is invoked by NewServer.
func newServerWithSIEM(t *testing.T, s APIStore, endpoint string, batchSize int, flushInterval time.Duration) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	drain := logdrain.NewAuditSIEMDrain(endpoint, "", batchSize, flushInterval)
	srv := NewServer(ServerDeps{
		Config:    cfg,
		Store:     s,
		Edition:   domain.EditionCloud,
		SIEMDrain: drain,
	})
	t.Cleanup(srv.Close)
	return srv
}

// countSIEMLines parses NDJSON payload from one HTTP POST and returns
// the list of audit-event IDs/actions it contains.
func countSIEMLines(body []byte) int {
	count := 0
	for line := range strings.SplitSeq(strings.TrimSpace(string(body)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev domain.AuditEvent
		if err := json.Unmarshal([]byte(line), &ev); err == nil {
			count++
		}
	}
	return count
}

// TestAuditSIEMForward_BatchesEventsFromDrainer asserts that every audit
// event successfully persisted via the async drainer is batched and
// forwarded to the configured SIEM endpoint as NDJSON.
func TestAuditSIEMForward_BatchesEventsFromDrainer(t *testing.T) {
	withShortRetries(t)

	var received atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received.Add(int32(countSIEMLines(body)))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}
	// Small batch + short flush so the test is fast and forces multiple flushes.
	s := newServerWithSIEM(t, ms, srv.URL, 25, 50*time.Millisecond)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	const total = 100
	for i := range total {
		s.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "job-x", map[string]any{"i": i})
	}

	// Wait up to 3s (well beyond flushInterval) for all events to arrive.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if received.Load() >= total {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Close triggers Stop, which flushes any pending batch.
	s.Close()

	if got := received.Load(); got != total {
		t.Fatalf("SIEM received %d events, want %d", got, total)
	}
}

// TestAuditSIEMForward_EmptyEndpoint_Noop verifies that when no SIEM endpoint
// is configured (drain is nil), the emit path still works and nothing is
// forwarded anywhere.
func TestAuditSIEMForward_EmptyEndpoint_Noop(t *testing.T) {
	withShortRetries(t)

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}
	// Pass empty endpoint -> drain is nil.
	s := newServerWithSIEM(t, ms, "", 10, 50*time.Millisecond)
	if s.siemDrain != nil {
		t.Fatal("siemDrain should be nil when endpoint is empty")
	}

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	for range 5 {
		s.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "job-x", nil)
	}
	s.Close()

	if hits.Load() != 0 {
		t.Errorf("test server was hit %d times, want 0", hits.Load())
	}
}

// TestAuditSIEMForward_ShutdownFlushesPending ensures Stop drains buffered
// events before returning.
func TestAuditSIEMForward_ShutdownFlushesPending(t *testing.T) {
	withShortRetries(t)

	var mu sync.Mutex
	received := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		received += countSIEMLines(body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}
	// Large batch + long flush so events sit in the buffer until Stop.
	s := newServerWithSIEM(t, ms, srv.URL, 1000, 5*time.Second)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	const total = 5
	for range total {
		s.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "job-x", nil)
	}

	// Give the async drainer time to push events into the SIEM channel
	// before shutdown. The events travel: emit -> audit drainer goroutine
	// -> store write -> siemDrain.Enqueue.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		s.auditAsyncMu.RLock()
		ch := s.auditAsyncCh
		s.auditAsyncMu.RUnlock()
		if ch != nil && len(ch) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	s.Close()

	mu.Lock()
	defer mu.Unlock()
	if received != total {
		t.Errorf("received %d events after shutdown, want %d", received, total)
	}
}
