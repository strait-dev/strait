package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
)

func TestSetJobEndpoint_RejectsHostnameResolvingToPrivateIP(t *testing.T) {
	t.Parallel()

	store := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
		UpdateJobEndpointFunc: func(context.Context, string, string, string, string) error {
			t.Fatal("UpdateJobEndpoint must not be called for private DNS target")
			return nil
		},
	}
	srv := newTestServer(t, store, &mockQueue{}, nil)
	body := `{"endpoint_url":"https://internal.example.com/hook"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/endpoint", body))

	if w.Code < 400 {
		t.Fatalf("expected 4xx for hostname resolving to private IP, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSetJobEndpoint_RejectsPrivateFallbackHostname(t *testing.T) {
	t.Parallel()

	store := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1"}, nil
		},
		UpdateJobEndpointFunc: func(context.Context, string, string, string, string) error {
			t.Fatal("UpdateJobEndpoint must not be called for private fallback DNS target")
			return nil
		},
	}
	srv := newTestServer(t, store, &mockQueue{}, nil)
	body := `{"endpoint_url":"https://example.com/hook","fallback_endpoint_url":"https://loopback.example.com/hook"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/endpoint", body))

	if w.Code < 400 {
		t.Fatalf("expected 4xx for fallback hostname resolving to loopback, got %d: %s", w.Code, w.Body.String())
	}
}
