package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestSDKTelemetry_ToolCallRouteLaunchInactive(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/tool-call", "run-1", `{"tool_name":"web_search"}`)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusNotFound,
		w.Code)

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
	require.Equal(t, http.StatusNotFound,
		w.Code)
	require.False(t, called)

}
