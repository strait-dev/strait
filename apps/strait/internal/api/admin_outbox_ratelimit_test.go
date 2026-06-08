package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// TestAdminOutbox_MutationsRateLimited is the regression guard for the missing
// per-endpoint rate limit on the outbox retry/purge admin mutations.
func TestAdminOutbox_MutationsRateLimited(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"/v1/admin/outbox/ob-1/retry", "/v1/admin/outbox/ob-1/purge"} {
		cfg := &config.Config{
			InternalSecret:      "test-secret-value",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       testJWTSigningKey,
			RateLimitRequests:   10000, // global cap high; per-route cap is 10/min
			RateLimitWindow:     time.Minute,
		}
		srv := NewServer(ServerDeps{Config: cfg, Store: &APIStoreMock{}, Queue: &mockQueue{}, Edition: domain.EditionCommunity})
		t.Cleanup(srv.Close)

		var got429 bool
		for range 15 {
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, path, `{}`))
			if w.Code == http.StatusTooManyRequests {
				got429 = true
				break
			}
		}
		require.Truef(t, got429, "%s must be rate limited", path)
	}
}
