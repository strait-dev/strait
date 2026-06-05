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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExportRateLimit_FailsClosed_OnRedisError verifies that handleExportAuditEvents
// returns a 503 (not success) when the rate limiter's Redis backend is down.
// This is the fail-closed security requirement: a downed rate-limit service
// must deny exports rather than silently permit them.
func TestExportRateLimit_FailsClosed_OnRedisError(t *testing.T) {
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

	// Close miniredis to simulate Redis being unavailable.
	mr.Close()

	handler := TypedHandler(srv, http.StatusOK, srv.handleExportAuditEvents)

	now := time.Now().UTC()
	from := now.Add(-time.Hour).Format(time.RFC3339)
	to := now.Format(time.RFC3339)
	url := "/v1/audit-events/export?format=ndjson&from=" + from + "&to=" + to

	req := httptest.NewRequest(http.MethodGet, url, nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "internal:admin")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "internal")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.NotEqual(t, http.StatusOK,
		w.Code)
	assert.Equal(
		t, http.StatusServiceUnavailable,

		w.Code)
}

// TestListAuditEvents_TimeWindowCap_Rejects91Days verifies that handleListAuditEvents
// returns 400 when from..to spans more than 90 days.
func TestListAuditEvents_TimeWindowCap_Rejects91Days(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}

	srv := newTestServer(t, ms, nil, nil)

	now := time.Now().UTC()
	to := now
	from := now.Add(-91 * 24 * time.Hour)

	_, err := srv.handleListAuditEvents(adminCtx("proj-1"), &ListAuditEventsInput{
		From: from.Format(time.RFC3339Nano),
		To:   to.Format(time.RFC3339Nano),
	})
	require.Error(t, err)

	// The error must be a 400 Bad Request.
	humaErr, ok := err.(interface{ GetStatus() int })
	require.True(
		t, ok)
	assert.Equal(
		t, http.StatusBadRequest,
		humaErr.
			GetStatus())
}

// TestListAuditEvents_TimeWindowCap_Accepts89Days verifies that handleListAuditEvents
// succeeds when from..to spans fewer than 90 days.
func TestListAuditEvents_TimeWindowCap_Accepts89Days(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListAuditEventsFunc: func(_ context.Context, _ string, _, _, _ string, _ int, _ *time.Time, _, _ *time.Time, _ bool) ([]domain.AuditEvent, error) {
			return []domain.AuditEvent{}, nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}

	srv := newTestServer(t, ms, nil, nil)

	now := time.Now().UTC()
	to := now
	from := now.Add(-89 * 24 * time.Hour)

	_, err := srv.handleListAuditEvents(adminCtx("proj-1"), &ListAuditEventsInput{
		From: from.Format(time.RFC3339Nano),
		To:   to.Format(time.RFC3339Nano),
	})
	assert.NoError(t, err)
}
