package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
)

func TestSDKTelemetry_ToolCallRouteLaunchInactive(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/tool-call", "run-1", `{"tool_name":"web_search"}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for launch-inactive tool-call route, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSDKTelemetry_IterationRouteLaunchInactive(t *testing.T) {
	t.Parallel()
	var called bool
	ms := &APIStoreMock{
		CreateRunIterationFunc: func(_ context.Context, _ *domain.RunIteration) error {
			called = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/iteration", "run-1", `{"iteration":1}`)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for launch-inactive iteration route, got %d: %s", w.Code, w.Body.String())
	}
	if called {
		t.Fatal("CreateRunIteration should not be called for unregistered iteration route")
	}
}
