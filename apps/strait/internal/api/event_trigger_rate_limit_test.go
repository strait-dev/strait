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
	"github.com/stretchr/testify/require"
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
	installEventTriggerProjectLookupFallback(ms)

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

// TestEventTriggerStreamRateLimited asserts the projectRateLimit
// middleware now runs on /v1/events/{eventKey}/stream. Before the fix this
// route was mounted with only sseTokenAuth + apiKeyOrSecretAuth, so a single
// API key could spam the endpoint without bound. After the fix, an API key
// configured for 1 req/60s gets 429 on its second call.
func TestEventTriggerStreamRateLimited(t *testing.T) {
	t.Parallel()

	srv, keys := makeEventStreamAPIKeyServer(t)
	rawA := "strait_proj_a_key_value_for_event_stream_test"
	_ = keys

	// First call: under the 1 req/60s budget -- handler runs to completion.
	w1 := httptest.NewRecorder()
	srv.ServeHTTP(w1, eventStreamRequest("evt-a", rawA))
	require.Equal(t, http.StatusOK,
		w1.Code)
	require.Equal(t, "1", w1.
		Header().Get("X-RateLimit-Limit"))

	// Second call within the same window: projectRateLimit must reject with 429.
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, eventStreamRequest("evt-a", rawA))
	require.Equal(t, http.StatusTooManyRequests,
		w2.
			Code)
	require.Equal(t, "60", w2.
		Header().Get("Retry-After"))
}

// TestEventTriggerStreamRateLimitIsolatedPerProject pins that
// exhausting Project A's bucket does not bleed into Project B. The
// projectRateLimit middleware keys by API-key id (which is in turn bound to
// a single project), so two distinct keys must hold independent quotas.
func TestEventTriggerStreamRateLimitIsolatedPerProject(t *testing.T) {
	t.Parallel()

	srv, _ := makeEventStreamAPIKeyServer(t)
	rawA := "strait_proj_a_key_value_for_event_stream_test"
	rawB := "strait_proj_b_key_value_for_event_stream_test"

	// Saturate project A.
	w1 := httptest.NewRecorder()
	srv.ServeHTTP(w1, eventStreamRequest("evt-a", rawA))
	require.Equal(t, http.StatusOK,
		w1.Code)

	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, eventStreamRequest("evt-a", rawA))
	require.Equal(t, http.StatusTooManyRequests,
		w2.
			Code)

	// Project B's bucket is independent -- still allowed.
	w3 := httptest.NewRecorder()
	srv.ServeHTTP(w3, eventStreamRequest("evt-b", rawB))
	require.Equal(t, http.StatusOK,
		w3.Code)
}
