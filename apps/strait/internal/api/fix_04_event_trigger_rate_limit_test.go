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

// makeEventStreamAPIKeyServer wires a server whose rate limiter is backed by
// a real (miniredis) Redis client so projectRateLimit can actually count.
// The two API keys returned bind to distinct projects with a 1 req / 60s
// quota so the second hit on the same key trips the limiter.
func makeEventStreamAPIKeyServer(t *testing.T) (*Server, map[string]*domain.APIKey) {
	t.Helper()

	now := time.Now()
	keyA := &domain.APIKey{
		ID:                  "key-proj-a",
		ProjectID:           "proj-a",
		Name:                "key-a",
		KeyHash:             hashAPIKey("strait_proj_a_key_value_for_event_stream_test"),
		KeyPrefix:           "strait_pa_",
		Scopes:              []string{domain.ScopeJobsRead},
		RateLimitRequests:   1,
		RateLimitWindowSecs: 60,
		CreatedAt:           now,
	}
	keyB := &domain.APIKey{
		ID:                  "key-proj-b",
		ProjectID:           "proj-b",
		Name:                "key-b",
		KeyHash:             hashAPIKey("strait_proj_b_key_value_for_event_stream_test"),
		KeyPrefix:           "strait_pb_",
		Scopes:              []string{domain.ScopeJobsRead},
		RateLimitRequests:   1,
		RateLimitWindowSecs: 60,
		CreatedAt:           now,
	}
	keys := map[string]*domain.APIKey{
		keyA.KeyHash: keyA,
		keyB.KeyHash: keyB,
	}

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, h string) (*domain.APIKey, error) {
			if k, ok := keys[h]; ok {
				return k, nil
			}
			return nil, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error { return nil },
		GetEventTriggerByEventKeyFunc: func(_ context.Context, ek string) (*domain.EventTrigger, error) {
			projectID := "proj-a"
			if ek == "evt-b" {
				projectID = "proj-b"
			}
			return &domain.EventTrigger{
				ID:          "trig-" + ek,
				EventKey:    ek,
				ProjectID:   projectID,
				Status:      domain.EventTriggerStatusReceived, // terminal -> writeTerminalTriggerSSE
				RequestedAt: now,
				ReceivedAt:  &now,
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Real, enabled rate limiter backed by miniredis so quota actually decrements.
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	srv.rateLimiter = ratelimit.NewRedisRateLimiter(rdb, true)

	return srv, map[string]*domain.APIKey{
		"a": keyA,
		"b": keyB,
	}
}

func eventStreamRequest(eventKey, bearerToken string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/v1/events/"+eventKey+"/stream", nil)
	r.Header.Set("Authorization", "Bearer "+bearerToken)
	r.Header.Set("Accept", "text/event-stream")
	return r
}

// TestFix_04_EventTriggerStreamRateLimited asserts the projectRateLimit
// middleware now runs on /v1/events/{eventKey}/stream. Before the fix this
// route was mounted with only sseTokenAuth + apiKeyOrSecretAuth, so a single
// API key could spam the endpoint without bound. After the fix, an API key
// configured for 1 req/60s gets 429 on its second call.
func TestFix_04_EventTriggerStreamRateLimited(t *testing.T) {
	t.Parallel()

	srv, keys := makeEventStreamAPIKeyServer(t)
	rawA := "strait_proj_a_key_value_for_event_stream_test"
	_ = keys

	// First call: under the 1 req/60s budget -- handler runs to completion.
	w1 := httptest.NewRecorder()
	srv.ServeHTTP(w1, eventStreamRequest("evt-a", rawA))
	if w1.Code != http.StatusOK {
		t.Fatalf("first call: status = %d, want 200; body: %s", w1.Code, w1.Body.String())
	}
	if got := w1.Header().Get("X-RateLimit-Limit"); got != "1" {
		t.Fatalf("first call: X-RateLimit-Limit = %q, want %q (projectRateLimit middleware did not run)", got, "1")
	}

	// Second call within the same window: projectRateLimit must reject with 429.
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, eventStreamRequest("evt-a", rawA))
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second call: status = %d, want 429; body: %s", w2.Code, w2.Body.String())
	}
	if got := w2.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("second call: Retry-After = %q, want %q", got, "60")
	}
}

// TestFix_04_EventTriggerStreamRateLimitIsolatedPerProject pins that
// exhausting Project A's bucket does not bleed into Project B. The
// projectRateLimit middleware keys by API-key id (which is in turn bound to
// a single project), so two distinct keys must hold independent quotas.
func TestFix_04_EventTriggerStreamRateLimitIsolatedPerProject(t *testing.T) {
	t.Parallel()

	srv, _ := makeEventStreamAPIKeyServer(t)
	rawA := "strait_proj_a_key_value_for_event_stream_test"
	rawB := "strait_proj_b_key_value_for_event_stream_test"

	// Saturate project A.
	w1 := httptest.NewRecorder()
	srv.ServeHTTP(w1, eventStreamRequest("evt-a", rawA))
	if w1.Code != http.StatusOK {
		t.Fatalf("project A first call: status = %d, want 200; body: %s", w1.Code, w1.Body.String())
	}
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, eventStreamRequest("evt-a", rawA))
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("project A second call: status = %d, want 429 (precondition for isolation test)", w2.Code)
	}

	// Project B's bucket is independent -- still allowed.
	w3 := httptest.NewRecorder()
	srv.ServeHTTP(w3, eventStreamRequest("evt-b", rawB))
	if w3.Code != http.StatusOK {
		t.Fatalf("project B first call: status = %d, want 200 (rate limit must be per-API-key/per-project); body: %s", w3.Code, w3.Body.String())
	}
}
