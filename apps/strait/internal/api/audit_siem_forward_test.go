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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	if drain != nil {
		drain.SetHTTPClientForTest(&http.Client{Timeout: 30 * time.Second})
	}
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
	require.EqualValues(t, total, received.
		Load())

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
	require.Nil(t, s.siemDrain)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	for range 5 {
		s.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "job-x", nil)
	}
	s.Close()
	assert.EqualValues(t, 0, hits.Load())

}

// TestAuditSIEMForward_ShutdownFlushesPending ensures Server.Close drains
// buffered events before returning. The post-FlushNow path is the only way
// the trailing events reach SIEM when the periodic flush interval (5s) is
// longer than the SIEM stop budget (5s).
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
	assert.Equal(
		t, total, received,
	)

}

// TestAuditSIEMForward_FlushNowOnCloseRunsBeforeStop verifies the explicit
// ordering: Server.Close calls FlushNow BEFORE Stop on the SIEM drain. We
// observe this indirectly: with batch=1000 and flush=5s, the only way a
// burst of events lands at the SIEM during the 5s shutdown budget is via
// FlushNow — Stop alone would not trigger a flush.
func TestAuditSIEMForward_FlushNowOnCloseRunsBeforeStop(t *testing.T) {
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
	// batchSize bigger than total events so the run loop never auto-flushes.
	// flushInterval longer than shutdownTimeout so the ticker never fires
	// during shutdown. Only FlushNow can deliver events under these settings.
	s := newServerWithSIEM(t, ms, srv.URL, 5000, 1*time.Hour)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	const total = 7
	for range total {
		s.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "j-fn", nil)
	}

	// Drain the API drainer queue so events reach siemDrain.
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
	require.EqualValues(t, total, received.
		Load())

}
