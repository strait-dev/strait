package api

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmitAuditEvent_DetailsSizeCap verifies that oversize details are
// replaced with a truncation marker and the captured event is well-formed.
func TestEmitAuditEvent_DetailsSizeCap(t *testing.T) {
	t.Parallel()

	var captured *domain.AuditEvent
	var mu sync.Mutex
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			defer mu.Unlock()
			clone := *ev
			captured = &clone
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	// Build a payload that serializes to ~32 KiB.
	bigValue := strings.Repeat("X", 32*1024)
	srv.emitAuditEvent(ctx, domain.AuditActionJobCreated, "job", "job-1", map[string]any{
		"name":           "job",
		"slug":           "job",
		"execution_mode": "http",
		"bloat":          bigValue,
	})

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, captured)

	var details map[string]any
	require.NoError(t, json.Unmarshal(captured.
		Details,
		&details))

	if truncated, _ := details["_truncated"].(bool); !truncated {
		assert.Failf(t, "test failure",

			"expected _truncated=true, got %v", details)
	}
	origBytes, _ := details["original_bytes"].(float64)
	assert.GreaterOrEqual(t, int(origBytes),
		32*1024,
	)
	assert.LessOrEqual(t, len(captured.
		Details,
	), auditMaxDetailsBytes,
	)

}

// TestEmitAuditEvent_RejectsMissingActorOnUserRequest verifies emit is
// blocked when the context claims to be a user/api_key request but the
// actor ID is empty — this is a middleware bug and would ship an
// unattributed audit row.
func TestEmitAuditEvent_RejectsMissingActorOnUserRequest(t *testing.T) {
	t.Parallel()

	for _, actorType := range []string{"user", "api_key"} {
		t.Run(actorType, func(t *testing.T) {
			var called atomic.Int32
			ms := &APIStoreMock{
				CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
					called.Add(1)
					return nil
				},
			}
			srv := newTestServer(t, ms, nil, nil)

			ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
			ctx = context.WithValue(ctx, ctxActorTypeKey, actorType)
			// actor ID intentionally omitted
			srv.emitAuditEvent(ctx, domain.AuditActionJobCreated, "job", "job-1", map[string]any{
				"name": "x", "slug": "x", "execution_mode": "http",
			})
			assert.EqualValues(t, 0, called.Load())

		})
	}
}

// TestEmitAuditEvent_AllowsEmptyActorForInternal verifies that internal
// callers without an explicit actor (scheduler, worker, legacy
// internal-secret path) are allowed to emit.
func TestEmitAuditEvent_AllowsEmptyActorForInternal(t *testing.T) {
	t.Parallel()

	var called atomic.Int32
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			called.Add(1)
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	// No actor, no type: plain internal caller.
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	srv.emitAuditEvent(ctx, domain.AuditActionJobCreated, "job", "job-1", map[string]any{
		"name": "x", "slug": "x", "execution_mode": "http",
	})
	assert.EqualValues(t, 1, called.Load())

}

// TestEmitAuditEvent_AllowsInternalActor verifies that an empty actor with
// actor_type="internal" is accepted (backward compat for internal-secret
// callers that operate without a logical user identity).
func TestEmitAuditEvent_AllowsInternalActor(t *testing.T) {
	t.Parallel()

	var called atomic.Int32
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			called.Add(1)
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "internal")
	srv.emitAuditEvent(ctx, domain.AuditActionJobCreated, "job", "job-1", map[string]any{
		"name": "x", "slug": "x", "execution_mode": "http",
	})
	assert.EqualValues(t, 1, called.Load())

}

// TestEmitAuditEventAsync_DrainerPanicIsContained verifies a panic in one
// event's store write does not take down the drainer.
func TestEmitAuditEventAsync_DrainerPanicIsContained(t *testing.T) {
	t.Parallel()

	var goodWrites atomic.Int32
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			if ev.ResourceID == "boom" {
				panic("simulated drainer panic")
			}
			goodWrites.Add(1)
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "ok-1", map[string]any{"run_id": "r1"})
	srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "boom", map[string]any{"run_id": "r2"})
	srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "ok-2", map[string]any{"run_id": "r3"})

	srv.Close()
	assert.EqualValues(t, 2, goodWrites.
		Load())

}

// TestEmitAuditEventAsync_DetailsImmutableAfterSend verifies the marshaled
// details snapshot is not affected by mutation of the caller's map after
// the emit call returns.
func TestEmitAuditEventAsync_DetailsImmutableAfterSend(t *testing.T) {
	t.Parallel()

	var captured *domain.AuditEvent
	var mu sync.Mutex
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			defer mu.Unlock()
			clone := *ev
			captured = &clone
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	details := map[string]any{"run_id": "original"}
	srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "job-1", details)
	// Race the drainer by mutating the map immediately.
	details["run_id"] = "mutated"
	details["extra"] = "added"

	// Drain.
	srv.Close()

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, captured)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(captured.
		Details,
		&parsed))
	assert.Equal(
		t, "original", parsed["run_id"])

	if _, ok := parsed["extra"]; ok {
		assert.Failf(t, "test failure",

			"extra key leaked into captured event (post-send mutation visible)")
	}
}

// TestEmitAuditEvent_RejectsUnknownAction verifies that typos are caught
// at the emit boundary and never reach the store.
func TestEmitAuditEvent_RejectsUnknownAction(t *testing.T) {
	t.Parallel()

	var called atomic.Int32
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			called.Add(1)
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	srv.emitAuditEvent(ctx, "job.delted", "job", "job-1", nil) // typo
	srv.emitAuditEventAsync(ctx, "job.whatevver", "job", "job-1", nil)

	// Give the async path a chance.
	time.Sleep(10 * time.Millisecond)
	srv.Close()
	assert.EqualValues(t, 0, called.Load())

}
