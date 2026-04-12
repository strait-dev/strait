package api

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for events: got %d/%d", got, n)
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != n {
		t.Fatalf("events len = %d, want %d", len(events), n)
	}
	for i, ev := range events {
		if ev.ProjectID != "proj-async" {
			t.Errorf("event %d project_id = %q, want proj-async", i, ev.ProjectID)
		}
		if ev.ActorID != "actor-async" {
			t.Errorf("event %d actor_id = %q, want actor-async", i, ev.ActorID)
		}
		if ev.ActorType != "user" {
			t.Errorf("event %d actor_type = %q, want user", i, ev.ActorType)
		}
		if ev.Action != "job.triggered" {
			t.Errorf("event %d action = %q", i, ev.Action)
		}
		var details map[string]any
		if err := json.Unmarshal(ev.Details, &details); err != nil {
			t.Fatalf("event %d details unmarshal: %v", i, err)
		}
		got, _ := details["i"].(float64)
		if int(got) != i {
			t.Errorf("event %d details.i = %v, want %d (FIFO ordering)", i, details["i"], i)
		}
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

	if got := written.Load(); got != 25 {
		t.Fatalf("written = %d after Close, want 25", got)
	}
}

// TestEmitAuditEventAsync_BufferFullDropsEvents verifies that when the drainer
// blocks and the channel fills, excess events are dropped without blocking
// the caller and without crashing.
func TestEmitAuditEventAsync_BufferFullDropsEvents(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	var written atomic.Int32
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			<-release
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
	go func() {
		for range total {
			srv.emitAuditEventAsync(ctx, "job.triggered", "job", "job-1", nil)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("emitAuditEventAsync blocked beyond 3s while drainer was stuck")
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

	if got := written.Load(); got != 2 {
		t.Fatalf("written = %d, want 2 (ok-1 and ok-2)", got)
	}
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
