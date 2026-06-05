package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// TestCreateJob_EndpointRequireTLS locks in the ENDPOINT_REQUIRE_TLS gate: job
// dispatch injects decrypted secrets and the run-token JWT, so when an operator
// opts in, a plaintext http endpoint must be rejected before any row is written,
// while https is accepted. With the knob off (default) http remains permitted so
// self-host/dev topologies are unaffected.
func TestCreateJob_EndpointRequireTLS(t *testing.T) {
	t.Parallel()

	newServer := func(t *testing.T, requireTLS bool, created *atomic.Bool) *Server {
		t.Helper()
		ms := &APIStoreMock{
			CreateJobFunc: func(_ context.Context, job *domain.Job) error {
				created.Store(true)
				job.ID = "job-123"
				job.CreatedAt = time.Now()
				job.UpdatedAt = time.Now()
				return nil
			},
		}
		srv := newTestServer(t, ms, &mockQueue{}, nil)
		srv.config.EndpointRequireTLS = requireTLS
		return srv
	}

	post := func(t *testing.T, srv *Server, endpointURL string) *httptest.ResponseRecorder {
		t.Helper()
		body := `{
			"project_id": "proj-1",
			"name": "Test Job",
			"slug": "test-job",
			"endpoint_url": "` + endpointURL + `"
		}`
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))
		return w
	}

	t.Run("require tls rejects http endpoint", func(t *testing.T) {
		t.Parallel()
		var created atomic.Bool
		srv := newServer(t, true, &created)
		w := post(t, srv, "http://example.com/callback")
		require.GreaterOrEqual(t,
			w.Code, 400)
		require.False(t, created.
			Load())

	})

	t.Run("require tls accepts https endpoint", func(t *testing.T) {
		t.Parallel()
		var created atomic.Bool
		srv := newServer(t, true, &created)
		w := post(t, srv, "https://example.com/callback")
		require.Equal(t, http.StatusCreated,
			w.Code)
		require.True(
			t, created.Load())

	})

	t.Run("knob off permits http endpoint", func(t *testing.T) {
		t.Parallel()
		var created atomic.Bool
		srv := newServer(t, false, &created)
		w := post(t, srv, "http://example.com/callback")
		require.Equal(t, http.StatusCreated,
			w.Code)
		require.True(
			t, created.Load())

	})
}
