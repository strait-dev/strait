package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"strait/internal/domain"
)

// BenchmarkHandleTriggerJob_AsyncAudit measures the cost of the job
// trigger hot path with the default async audit emit enabled. This is
// the production code path.
func BenchmarkHandleTriggerJob_AsyncAudit(b *testing.B) {
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return benchmarkJob(id), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil },
	}
	srv := NewServer(ServerDeps{
		Config: benchmarkConfig(),
		Store:  ms,
		Queue:  mq,
	})
	b.Cleanup(srv.Close)

	body := `{"payload":{"key":"value"}}`
	var reqCount atomic.Uint64

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/v1/jobs/job-123/trigger", strings.NewReader(body))
			r.Header.Set("X-Internal-Secret", "test-secret-value")
			r.Header.Set("Content-Type", "application/json")
			r.RemoteAddr = uniqueRemoteAddr(&reqCount)
			srv.ServeHTTP(w, r)
			if w.Code != http.StatusCreated {
				b.Fatalf("expected 201, got %d", w.Code)
			}
		}
	})
}

// BenchmarkEmitAuditEventAsync_EmptyDetails measures the direct cost of
// the async emit helper with a zero-size details payload. This isolates
// the cost of actor extraction + marshal + channel send.
func BenchmarkEmitAuditEventAsync_EmptyDetails(b *testing.B) {
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}
	srv := NewServer(ServerDeps{
		Config: benchmarkConfig(),
		Store:  ms,
	})
	b.Cleanup(srv.Close)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-bench")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-bench")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "job-1", nil)
	}
}

// BenchmarkEmitAuditEventAsync_SmallDetails measures cost with a typical
// production-size details payload (~256 bytes after marshal).
func BenchmarkEmitAuditEventAsync_SmallDetails(b *testing.B) {
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}
	srv := NewServer(ServerDeps{
		Config: benchmarkConfig(),
		Store:  ms,
	})
	b.Cleanup(srv.Close)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-bench")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-bench")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	details := map[string]any{
		"run_id":               "01HXABCXYZ1234567890ABCDEF",
		"scheduled_at":         "2026-04-11T12:00:00Z",
		"priority":             5,
		"idempotency_key_hash": "abcdef0123456789",
		"tag_keys":             []string{"env", "team", "region"},
		"triggered_by":         "manual",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "job-1", details)
	}
}

// BenchmarkEmitAuditEventRawAsync_ImmediateDetails measures the immediate
// trigger fast path for known-safe audit details.
func BenchmarkEmitAuditEventRawAsync_ImmediateDetails(b *testing.B) {
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}
	srv := NewServer(ServerDeps{
		Config: benchmarkConfig(),
		Store:  ms,
	})
	b.Cleanup(srv.Close)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-bench")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-bench")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")
	detailsJSON := immediateTriggerAuditDetailsJSON(&domain.JobRun{
		ID:          "01HXABCXYZ1234567890ABCDEF",
		Priority:    5,
		TriggeredBy: domain.TriggerManual,
	}, nil, "", false)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		srv.emitAuditEventRawAsync(ctx, domain.AuditActionJobTriggered, "job", "job-1", detailsJSON)
	}
}

// BenchmarkEmitAuditEvent_Sync measures the direct cost of the
// synchronous emit path (used by low-rate handlers). This exists as a
// comparison for the async path — the hot-path trigger handler must not
// pay this cost per request.
func BenchmarkEmitAuditEvent_Sync(b *testing.B) {
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}
	srv := NewServer(ServerDeps{
		Config: benchmarkConfig(),
		Store:  ms,
	})
	b.Cleanup(srv.Close)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-bench")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-bench")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	details := map[string]any{
		"name":           "Bench",
		"slug":           "bench",
		"execution_mode": "http",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		srv.emitAuditEvent(ctx, domain.AuditActionJobCreated, "job", "job-1", details)
	}
}

// BenchmarkMarshalAndCapDetails_UnderCap measures the marshal + size
// check for a normal payload.
func BenchmarkMarshalAndCapDetails_UnderCap(b *testing.B) {
	srv := &Server{}
	details := map[string]any{
		"name":  "Bench",
		"slug":  "bench",
		"count": 42,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = srv.marshalAndCapDetails(context.Background(), domain.AuditActionJobCreated, details)
	}
}

// BenchmarkMarshalAndCapDetails_OverCap measures the truncation path
// for an oversize payload.
func BenchmarkMarshalAndCapDetails_OverCap(b *testing.B) {
	srv := &Server{}
	big := map[string]any{
		"bloat": strings.Repeat("X", 32*1024),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = srv.marshalAndCapDetails(context.Background(), domain.AuditActionJobCreated, big)
	}
}

// BenchmarkComputeAuditSignature measures the HMAC compute cost per event.
// Mirrors the bench that would normally live in internal/store/ but uses
// the api package for local visibility into the hot path.
func BenchmarkComputeAuditSignature(b *testing.B) {
	details := json.RawMessage(`{"run_id":"r1","priority":5}`)
	ev := &domain.AuditEvent{
		ID:           "ev-1",
		ProjectID:    "proj-1",
		ActorID:      "actor-1",
		ActorType:    "user",
		Action:       domain.AuditActionJobTriggered,
		ResourceType: "job",
		ResourceID:   "job-1",
		Details:      details,
		PreviousHash: "0000000000000000000000000000000000000000000000000000000000000000",
	}
	_ = ev // placeholder: storing the sig bench in the store package is cleaner
	// but we keep a doc marker here.
	b.Skip("see internal/store benchmarks for HMAC compute cost")
}
