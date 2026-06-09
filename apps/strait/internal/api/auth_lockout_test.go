package api

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/config"
	"strait/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	// 10.0.0.0/8 represents an internal proxy network for these tests.
	internalProxies := parseTrustedProxies([]string{"10.0.0.0/8"})

	tests := []struct {
		name      string
		xff       string
		addr      string
		trusted   []net.IPNet
		want      string
		whyItHelp string
	}{
		// Fail-safe default: no trusted proxies means XFF is ignored.
		// This blocks the lockout/rate-limit bypass where an attacker
		// sets X-Forwarded-For: <random> per request to spoof a fresh IP.
		{
			name: "no trusted proxies ignores xff and uses remote addr",
			xff:  "1.2.3.4, 5.6.7.8", addr: "9.0.0.1:1234", trusted: nil, want: "9.0.0.1",
		},
		{
			name: "no trusted proxies still uses remote addr without xff",
			xff:  "", addr: "5.6.7.8:1234", trusted: nil, want: "5.6.7.8",
		},
		{
			name: "no trusted proxies even with single xff entry uses remote addr",
			xff:  "1.2.3.4", addr: "5.6.7.8:1234", trusted: nil, want: "5.6.7.8",
		},

		// With trusted proxies configured, peer must itself be a trusted
		// proxy for XFF to be considered. Otherwise XFF is rejected.
		{
			name: "untrusted peer with xff falls back to remote addr",
			xff:  "1.2.3.4", addr: "5.6.7.8:1234", trusted: internalProxies, want: "5.6.7.8",
		},
		{
			name: "trusted peer with single xff uses xff",
			xff:  "1.2.3.4", addr: "10.0.0.5:1234", trusted: internalProxies, want: "1.2.3.4",
		},
		{
			name: "trusted peer with chain skips trusted hops",
			xff:  "1.2.3.4, 10.0.0.7, 10.0.0.8", addr: "10.0.0.5:1234", trusted: internalProxies, want: "1.2.3.4",
		},
		{
			name: "trusted peer skips empty and whitespace xff hops",
			xff:  " 1.2.3.4 , , 10.0.0.7,   ", addr: "10.0.0.5:1234", trusted: internalProxies, want: "1.2.3.4",
		},
		{
			name: "trusted peer with xff that is all trusted falls back to remote addr",
			xff:  "10.0.0.7, 10.0.0.8", addr: "10.0.0.5:1234", trusted: internalProxies, want: "10.0.0.5",
		},
		{
			name: "ipv6 remote addr is parsed correctly",
			xff:  "", addr: "[::1]:1234", trusted: nil, want: "::1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.addr
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			got := realIP(r, tc.trusted)
			assert.Equal(t, tc.want,
				got)
		})
	}
}

func TestRealIP_LockoutSpoofingRegression(t *testing.T) {
	t.Parallel()

	// Regression: an attacker sending many requests with rotating XFF
	// values must not be able to "rotate" the IP that the lockout limiter
	// keys on. Without trusted proxies the IP must be stable across
	// arbitrary XFF inputs.
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	r1.RemoteAddr = "203.0.113.5:1234"
	r1.Header.Set("X-Forwarded-For", "10.0.0.1")

	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.RemoteAddr = "203.0.113.5:1234"
	r2.Header.Set("X-Forwarded-For", "10.0.0.2")

	if got1, got2 := realIP(r1, nil), realIP(r2, nil); got1 != got2 {
		require.Failf(t, "test failure",

			"IP changed under XFF rotation: %q vs %q (lockout bypass possible)", got1, got2)
	}
}

func TestAuthLockout_429AfterThreshold(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithRedis(t, &APIStoreMock{})

	// Send 10 failed auth requests (bad API key).
	for range 10 {
		req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
		req.Header.Set("Authorization", "Bearer strait_invalid_key_attempt")
		req.RemoteAddr = "10.0.0.99:1234"
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		require.Equal(t, http.
			StatusUnauthorized,
			w.Code)
	}

	// 11th request should be rate limited.
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer strait_another_invalid_key")
	req.RemoteAddr = "10.0.0.99:1234"
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.
		StatusTooManyRequests,
		w.Code)
	assert.NotEmpty(t, w.Header().Get("Retry-After"))
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
	assert.NotEqual(t, http.
		StatusTooManyRequests,
		w.Code,
	)
}

func TestAuthLockout_InternalSecretSuccessDoesNotClearAPIKeyFailures(t *testing.T) {
	t.Parallel()

	srv := newTestServerWithRedis(t, &APIStoreMock{})
	remoteAddr := "10.0.0.77:1234"

	for range 10 {
		req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
		req.Header.Set("Authorization", "Bearer strait_bad_key")
		req.RemoteAddr = remoteAddr
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		require.Equal(t, http.
			StatusUnauthorized,
			w.Code)
	}

	internalReq := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	internalReq.Header.Set("X-Internal-Secret", "test-secret-value")
	internalReq.RemoteAddr = remoteAddr
	internalW := httptest.NewRecorder()
	srv.ServeHTTP(internalW, internalReq)
	require.False(t, internalW.
		Code == http.StatusUnauthorized ||
		internalW.Code ==
			http.StatusTooManyRequests,
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer strait_bad_key")
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.
		StatusTooManyRequests,
		w.Code)
}

func TestAuthLockout_NoRedis_FailsOpen(t *testing.T) {
	t.Parallel()

	// Server without Redis -- auth limiter should fail open.
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	// Even after many failed attempts, should get 401 not 429.
	for range 20 {
		req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
		req.Header.Set("Authorization", "Bearer strait_bad_key")
		req.RemoteAddr = "10.0.0.99:1234"
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		require.NotEqual(t, http.
			StatusTooManyRequests,
			w.Code,
		)
	}
}
