package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/config"
	"strait/internal/domain"
)

func newJobTestServer(t *testing.T, s APIStore) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret: "test-secret",
		JWTSigningKey:  "test-jwt-key-must-be-32-chars-long",
	}
	return NewServer(ServerDeps{
		Config: cfg,
		Store:  s,
		Queue:  &mockQueue{},
		PubSub: &mockPublisher{},
	})
}

func TestHandleCreateJob_SandboxMode(t *testing.T) {
	t.Parallel()
	var created *domain.Job
	ms := &mockAPIStore{
		createJobFn: func(_ context.Context, j *domain.Job) error {
			created = j
			return nil
		},
	}
	srv := newJobTestServer(t, ms)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs", `{
		"project_id": "proj-1",
		"name": "Sandbox Job",
		"slug": "sandbox-job",
		"endpoint_url": "https://example.com/run",
		"execution_mode": "sandbox",
		"sandbox_code": "print('hello')",
		"sandbox_language": "python"
	}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if created == nil {
		t.Fatal("job was not created")
	}
	if created.ExecutionMode != domain.ExecutionModeSandbox {
		t.Errorf("expected sandbox mode, got %s", created.ExecutionMode)
	}
	if created.SandboxCode != "print('hello')" {
		t.Errorf("expected sandbox code, got %s", created.SandboxCode)
	}
	if created.SandboxLanguage != "python" {
		t.Errorf("expected python, got %s", created.SandboxLanguage)
	}
}

func TestHandleCreateJob_SandboxMissingCode(t *testing.T) {
	t.Parallel()
	srv := newJobTestServer(t, &mockAPIStore{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs", `{
		"project_id": "proj-1",
		"name": "Bad Sandbox",
		"slug": "bad-sandbox",
		"endpoint_url": "https://example.com/run",
		"execution_mode": "sandbox",
		"sandbox_language": "python"
	}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateJob_SandboxMissingLanguage(t *testing.T) {
	t.Parallel()
	srv := newJobTestServer(t, &mockAPIStore{})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs", `{
		"project_id": "proj-1",
		"name": "Bad Sandbox",
		"slug": "bad-sandbox",
		"endpoint_url": "https://example.com/run",
		"execution_mode": "sandbox",
		"sandbox_code": "print('hello')"
	}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateJob_HTTPModeDefault(t *testing.T) {
	t.Parallel()
	var created *domain.Job
	ms := &mockAPIStore{
		createJobFn: func(_ context.Context, j *domain.Job) error {
			created = j
			return nil
		},
	}
	srv := newJobTestServer(t, ms)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs", `{
		"project_id": "proj-1",
		"name": "HTTP Job",
		"slug": "http-job",
		"endpoint_url": "https://example.com/run"
	}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if created.ExecutionMode != domain.ExecutionModeHTTP {
		t.Errorf("expected http mode default, got %s", created.ExecutionMode)
	}
}

func TestHandleCreateJob_CancelEndpointURL(t *testing.T) {
	t.Parallel()
	var created *domain.Job
	ms := &mockAPIStore{
		createJobFn: func(_ context.Context, j *domain.Job) error {
			created = j
			return nil
		},
	}
	srv := newJobTestServer(t, ms)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs", `{
		"project_id": "proj-1",
		"name": "With Cancel",
		"slug": "with-cancel",
		"endpoint_url": "https://example.com/run",
		"cancel_endpoint_url": "https://example.com/cancel"
	}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if created.CancelEndpointURL != "https://example.com/cancel" {
		t.Errorf("expected cancel URL, got %s", created.CancelEndpointURL)
	}
}
