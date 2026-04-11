package api

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

// withShortRetries replaces auditRetryDelays with near-zero sleeps for the
// duration of a test. Restored on cleanup. Tests run fast and deterministic.
func withShortRetries(t *testing.T) {
	t.Helper()
	orig := auditRetryDelays
	auditRetryDelays = []time.Duration{
		1 * time.Millisecond,
		1 * time.Millisecond,
		1 * time.Millisecond,
	}
	t.Cleanup(func() {
		auditRetryDelays = orig
	})
}

// TestDrainer_RetriesTransientErrors: store fails twice then succeeds.
// Expectation: event is written exactly once, no deadletter writes.
func TestDrainer_RetriesTransientErrors(t *testing.T) {
	t.Parallel()
	withShortRetries(t)

	var attempts, writes, dlqCalls atomic.Int32
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			n := attempts.Add(1)
			if n <= 2 {
				return errors.New("transient")
			}
			writes.Add(1)
			return nil
		},
		CreateAuditEventDeadletterFunc: func(_ context.Context, _ *domain.AuditEvent, _ string, _ int) error {
			dlqCalls.Add(1)
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "job-1", map[string]any{"run_id": "r1"})
	srv.Close()

	if writes.Load() != 1 {
		t.Errorf("writes = %d, want 1", writes.Load())
	}
	if dlqCalls.Load() != 0 {
		t.Errorf("deadletter calls = %d, want 0", dlqCalls.Load())
	}
}

// TestDrainer_DeadlettersAfterExhaustingRetries: store always fails.
// Expectation: event ends up in the deadletter with retry_count=3.
func TestDrainer_DeadlettersAfterExhaustingRetries(t *testing.T) {
	t.Parallel()
	withShortRetries(t)

	var mu sync.Mutex
	var captured struct {
		ev         *domain.AuditEvent
		lastErr    string
		retryCount int
	}
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return errors.New("db down")
		},
		CreateAuditEventDeadletterFunc: func(_ context.Context, ev *domain.AuditEvent, lastErr string, retryCount int) error {
			mu.Lock()
			defer mu.Unlock()
			clone := *ev
			captured.ev = &clone
			captured.lastErr = lastErr
			captured.retryCount = retryCount
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "job-1", map[string]any{"run_id": "r1"})
	srv.Close()

	mu.Lock()
	defer mu.Unlock()
	if captured.ev == nil {
		t.Fatal("expected deadletter write")
	}
	if captured.ev.Action != domain.AuditActionJobTriggered {
		t.Errorf("deadletter action = %q", captured.ev.Action)
	}
	if captured.lastErr != "db down" {
		t.Errorf("last_error = %q, want 'db down'", captured.lastErr)
	}
	if captured.retryCount != len(auditRetryDelays) {
		t.Errorf("retry_count = %d, want %d", captured.retryCount, len(auditRetryDelays))
	}
}

// TestDrainer_LogsIfDeadletterAlsoFails: both primary and DLQ fail.
// Expectation: no crash, event is lost with deadletter_failed counter.
func TestDrainer_LogsIfDeadletterAlsoFails(t *testing.T) {
	t.Parallel()
	withShortRetries(t)

	var dlqCalls atomic.Int32
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return errors.New("db down")
		},
		CreateAuditEventDeadletterFunc: func(_ context.Context, _ *domain.AuditEvent, _ string, _ int) error {
			dlqCalls.Add(1)
			return errors.New("dlq also down")
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "job-1", nil)
	srv.Close()

	if dlqCalls.Load() != 1 {
		t.Errorf("deadletter calls = %d, want 1", dlqCalls.Load())
	}
}

// TestDrainer_RetriesDoNotReorderEvents: submit 5 events, first one
// requires 2 retries. All 5 must be written in submission order.
// The retry blocks the drainer (documented trade-off).
func TestDrainer_RetriesDoNotReorderEvents(t *testing.T) {
	t.Parallel()
	withShortRetries(t)

	var mu sync.Mutex
	var writeOrder []string
	var firstAttempt atomic.Int32
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			if ev.ResourceID == "evt-1" {
				n := firstAttempt.Add(1)
				if n <= 2 {
					return errors.New("transient")
				}
			}
			mu.Lock()
			defer mu.Unlock()
			writeOrder = append(writeOrder, ev.ResourceID)
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	ids := []string{"evt-1", "evt-2", "evt-3", "evt-4", "evt-5"}
	for _, id := range ids {
		srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", id, nil)
	}
	srv.Close()

	mu.Lock()
	defer mu.Unlock()
	if len(writeOrder) != 5 {
		t.Fatalf("wrote %d, want 5: %v", len(writeOrder), writeOrder)
	}
	for i, id := range ids {
		if writeOrder[i] != id {
			t.Errorf("order[%d] = %q, want %q (full order: %v)", i, writeOrder[i], id, writeOrder)
		}
	}
}
