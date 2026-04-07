package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/config"
	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestServerWithRedis(t *testing.T, s APIStore) *Server {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:      cfg,
		Store:       s,
		Queue:       &mockQueue{},
		PubSub:      &mockPublisher{},
		RedisClient: rdb,
		Edition:     domain.EditionCloud,
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestRealIP_XForwardedFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		xff  string
		addr string
		want string
	}{
		{"xff single", "1.2.3.4", "5.6.7.8:1234", "1.2.3.4"},
		{"xff multiple takes last", "1.2.3.4, 5.6.7.8", "9.0.0.1:1234", "5.6.7.8"},
		{"xff with spaces", "  1.2.3.4  , 5.6.7.8  ", "9.0.0.1:1234", "5.6.7.8"},
		{"no xff uses remote addr", "", "5.6.7.8:1234", "5.6.7.8"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.addr
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			got := realIP(r)
			if got != tc.want {
				t.Errorf("realIP = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAuthLockout_429AfterThreshold(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithRedis(t, &APIStoreMock{})

	// Send 10 failed auth requests (bad API key).
	for i := range 10 {
		req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
		req.Header.Set("Authorization", "Bearer strait_invalid_key_attempt")
		req.RemoteAddr = "10.0.0.99:1234"
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: status = %d, want 401", i+1, w.Code)
		}
	}

	// 11th request should be rate limited.
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer strait_another_invalid_key")
	req.RemoteAddr = "10.0.0.99:1234"
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("11th request: status = %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("missing Retry-After header on 429 response")
	}
}

func TestAuthLockout_DifferentIP_NotBlocked(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithRedis(t, &APIStoreMock{})

	// Exhaust lockout for one IP.
	for range 10 {
		req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
		req.Header.Set("Authorization", "Bearer strait_bad_key")
		req.RemoteAddr = "10.0.0.1:1234"
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
	}

	// Different IP should not be blocked.
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer strait_bad_key")
	req.RemoteAddr = "10.0.0.2:1234"
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code == http.StatusTooManyRequests {
		t.Error("different IP should not be rate limited")
	}
}

func TestAuthLockout_NoRedis_FailsOpen(t *testing.T) {
	t.Parallel()

	// Server without Redis -- auth limiter should fail open.
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	// Even after many failed attempts, should get 401 not 429.
	for i := range 20 {
		req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
		req.Header.Set("Authorization", "Bearer strait_bad_key")
		req.RemoteAddr = "10.0.0.99:1234"
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d: got 429, want 401 (no Redis = fail open)", i+1)
		}
	}
}
