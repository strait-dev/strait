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
)

func TestSecurityHeaders_HTTPResponse(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want DENY", got)
	}
	if got := w.Header().Get("X-XSS-Protection"); got != "0" {
		t.Fatalf("X-XSS-Protection = %q, want 0", got)
	}
	if got := w.Header().Get("Content-Security-Policy"); got != "default-src 'none'" {
		t.Fatalf("Content-Security-Policy = %q, want default-src 'none'", got)
	}
	if got := w.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q, want no-referrer", got)
	}
	if got := w.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("Strict-Transport-Security = %q, want empty on HTTP", got)
	}
}

func TestSecurityHeaders_HTTPSIncludesHSTS(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.TLS = &tls.ConnectionState{}
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if got := w.Header().Get("Strict-Transport-Security"); got != "max-age=63072000; includeSubDomains" {
		t.Fatalf("Strict-Transport-Security = %q, want max-age=63072000; includeSubDomains", got)
	}
}

func TestHandleCreateJob_RejectsLongNameAndSlug(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	nameBody := `{"project_id":"proj-1","name":"` + strings.Repeat("a", 256) + `","slug":"valid-slug","endpoint_url":"https://example.com/callback"}`
	w1 := httptest.NewRecorder()
	srv.ServeHTTP(w1, authedRequest(http.MethodPost, "/v1/jobs/", nameBody))
	if w1.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for long name, got %d: %s", w1.Code, w1.Body.String())
	}

	slugBody := `{"project_id":"proj-1","name":"valid","slug":"` + strings.Repeat("s", 129) + `","endpoint_url":"https://example.com/callback"}`
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, authedRequest(http.MethodPost, "/v1/jobs/", slugBody))
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for long slug, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestHandleCreateJob_RejectsCGNATEndpointURL(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	body := `{"project_id":"proj-1","name":"Test","slug":"test-job","endpoint_url":"http://100.64.0.1/callback"}`
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for CGNAT endpoint, got %d: %s", w.Code, w.Body.String())
	}
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

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized payload, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "payload too large") {
		t.Fatalf("expected payload too large error, got %s", w.Body.String())
	}
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

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty resolved job id, got %d: %s", w.Code, w.Body.String())
	}
}
