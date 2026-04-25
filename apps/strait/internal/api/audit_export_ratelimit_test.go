package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/ratelimit"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestAuditExportRateLimit_BlocksAfterThreshold(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	ms := &APIStoreMock{
		StreamAuditEventsFunc: func(_ context.Context, _, _, _ string, _, _ time.Time, fn func(*domain.AuditEvent) error) error {
			return fn(&domain.AuditEvent{
				ID:        "ev-1",
				ProjectID: "proj-1",
				Action:    domain.AuditActionJobCreated,
				CreatedAt: time.Now(),
			})
		},
		GetAuditExportRowCapFunc: func(_ context.Context, _ string) (int64, error) {
			return 100, nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}

	srv := newTestServer(t, ms, nil, nil)
	srv.rateLimiter = ratelimit.NewRedisRateLimiter(rdb, true)

	handler := TypedHandler(srv, http.StatusOK, srv.handleExportAuditEvents)

	makeExport := func(projectID string) int {
		now := time.Now().UTC()
		from := now.Add(-time.Hour).Format(time.RFC3339)
		to := now.Format(time.RFC3339)
		url := "/v1/audit-events/export?format=ndjson&from=" + from + "&to=" + to

		req := httptest.NewRequest(http.MethodGet, url, nil)
		ctx := context.WithValue(req.Context(), ctxProjectIDKey, projectID)
		ctx = context.WithValue(ctx, ctxActorIDKey, "internal:admin")
		ctx = context.WithValue(ctx, ctxActorTypeKey, "internal")
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w.Code
	}

	// First 10 exports should succeed.
	for i := range 10 {
		code := makeExport("proj-1")
		if code == http.StatusTooManyRequests {
			t.Fatalf("export %d returned 429, expected success", i+1)
		}
	}

	// 11th export should be rate-limited.
	code := makeExport("proj-1")
	if code != http.StatusTooManyRequests {
		t.Errorf("11th export returned %d, want 429", code)
	}
}

func TestAuditExportRateLimit_DifferentProjectsIndependent(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	ms := &APIStoreMock{
		StreamAuditEventsFunc: func(_ context.Context, _, _, _ string, _, _ time.Time, fn func(*domain.AuditEvent) error) error {
			return fn(&domain.AuditEvent{
				ID:        "ev-1",
				Action:    domain.AuditActionJobCreated,
				CreatedAt: time.Now(),
			})
		},
		GetAuditExportRowCapFunc: func(_ context.Context, _ string) (int64, error) {
			return 100, nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}

	srv := newTestServer(t, ms, nil, nil)
	srv.rateLimiter = ratelimit.NewRedisRateLimiter(rdb, true)

	handler := TypedHandler(srv, http.StatusOK, srv.handleExportAuditEvents)

	makeExport := func(projectID string) int {
		now := time.Now().UTC()
		from := now.Add(-time.Hour).Format(time.RFC3339)
		to := now.Format(time.RFC3339)
		url := "/v1/audit-events/export?format=ndjson&from=" + from + "&to=" + to

		req := httptest.NewRequest(http.MethodGet, url, nil)
		ctx := context.WithValue(req.Context(), ctxProjectIDKey, projectID)
		ctx = context.WithValue(ctx, ctxActorIDKey, "internal:admin")
		ctx = context.WithValue(ctx, ctxActorTypeKey, "internal")
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w.Code
	}

	// Exhaust proj-a's quota.
	for i := range 10 {
		code := makeExport("proj-a")
		if code == http.StatusTooManyRequests {
			t.Fatalf("proj-a export %d returned 429", i+1)
		}
	}

	// proj-a is now exhausted.
	code := makeExport("proj-a")
	if code != http.StatusTooManyRequests {
		t.Errorf("proj-a 11th export = %d, want 429", code)
	}

	// proj-b should still be allowed.
	code = makeExport("proj-b")
	if code == http.StatusTooManyRequests {
		t.Error("proj-b first export returned 429; rate limits should be per-project")
	}
}
