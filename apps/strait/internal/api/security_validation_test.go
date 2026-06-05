package api

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/config"
	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestSecurityHeaders_HTTPResponse(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	require.Equal(t, "nosniff",
		w.Header().
			Get("X-Content-Type-Options"))
	require.Equal(t, "DENY", w.
		Header().Get("X-Frame-Options"))
	require.Equal(t, "0", w.Header().Get("X-XSS-Protection"))
	require.Equal(t, "default-src 'none'",

		w.Header().Get("Content-Security-Policy"))
	require.Equal(t, "no-referrer",
		w.Header().Get("Referrer-Policy"))
	require.Equal(t, "", w.Header().Get("Strict-Transport-Security"))

}

func TestSecurityHeaders_HTTPSIncludesHSTS(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.TLS = &tls.ConnectionState{}
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	require.Equal(t, "max-age=63072000; includeSubDomains",

		w.Header().Get(
			"Strict-Transport-Security",
		))

}

func TestHandleCreateJob_RejectsLongNameAndSlug(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	nameBody := `{"project_id":"proj-1","name":"` + strings.Repeat("a", 256) + `","slug":"valid-slug","endpoint_url":"https://example.com/callback"}`
	w1 := httptest.NewRecorder()
	srv.ServeHTTP(w1, authedRequest(http.MethodPost, "/v1/jobs/", nameBody))
	require.Equal(t, http.StatusUnprocessableEntity,

		w1.Code)

	slugBody := `{"project_id":"proj-1","name":"valid","slug":"` + strings.Repeat("s", 129) + `","endpoint_url":"https://example.com/callback"}`
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, authedRequest(http.MethodPost, "/v1/jobs/", slugBody))
	require.Equal(t, http.StatusBadRequest,

		w2.Code)

}

func TestHandleCreateJob_RejectsCGNATEndpointURL(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	body := `{"project_id":"proj-1","name":"Test","slug":"test-job","endpoint_url":"http://100.64.0.1/callback"}`
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

func TestHandleTriggerJob_RejectsPayloadOver5MB(t *testing.T) {
	t.Parallel()

	store := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 30}, nil
		},
	}
	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:     "test-secret-value",
			JWTSigningKey:      testJWTSigningKey,
			MaxRequestBodySize: 8 * 1024 * 1024,
		},
		Store: store,
		Queue: &mockQueue{},
	})
	t.Cleanup(srv.Close)

	body := `{"payload":{"blob":"` + strings.Repeat("a", maxPayloadSize+1) + `"}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", body))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.True(
		t, strings.Contains(w.Body.
			String(), "payload too large",
		))

}

func TestHandleSDKSpawn_RejectsEmptyResolvedJobID(t *testing.T) {
	t.Parallel()

	store := &APIStoreMock{
		GetJobBySlugFunc: func(_ context.Context, _, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "", ProjectID: "proj-1", Enabled: true}, nil
		},
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-parent", Status: domain.StatusWaiting}, nil
		},
	}
	srv := newTestServer(t, store, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-parent/spawn", "run-parent", `{"job_slug":"child","project_id":"proj-1"}`)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}
