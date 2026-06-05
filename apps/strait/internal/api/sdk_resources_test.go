package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleSDKResources_ValidPayload_InfoLevel(t *testing.T) {
	t.Parallel()

	var captured atomic.Value
	store := &APIStoreMock{
		InsertEventFunc: func(_ context.Context, event *domain.RunEvent) error {
			captured.Store(event)
			return nil
		},
	}
	srv := newTestServer(t, store, nil, nil)

	req := sdkRequest(t, "POST", "/sdk/runs/run-1/resources", "run-1",
		`{"memory_mb":100,"memory_percent":40,"cpu_percent":20}`)
	w := httptest.NewRecorder()
	TypedHandler(srv, http.StatusCreated, srv.handleSDKResources)(w, req)
	require.EqualValues(t, 201, w.Code)

	ev := captured.Load().(*domain.RunEvent)
	assert.Equal(
		t, domain.EventType("resource_sample"),
		ev.Type,
	)
	assert.Equal(
		t, "info", ev.Level,
	)

}

func TestHandleSDKResources_MemoryWarn80(t *testing.T) {
	t.Parallel()

	var captured atomic.Value
	store := &APIStoreMock{
		InsertEventFunc: func(_ context.Context, event *domain.RunEvent) error {
			captured.Store(event)
			return nil
		},
	}
	srv := newTestServer(t, store, nil, nil)

	req := sdkRequest(t, "POST", "/sdk/runs/run-1/resources", "run-1",
		`{"memory_mb":200,"memory_percent":85,"cpu_percent":10}`)
	w := httptest.NewRecorder()
	TypedHandler(srv, http.StatusCreated, srv.handleSDKResources)(w, req)
	require.EqualValues(t, 201, w.Code)

	ev := captured.Load().(*domain.RunEvent)
	assert.Equal(
		t, "warn", ev.Level,
	)
	assert.True(t,
		strings.Contains(ev.Message,
			"warning",
		))

}

func TestHandleSDKResources_MemoryCritical90(t *testing.T) {
	t.Parallel()

	var captured atomic.Value
	store := &APIStoreMock{
		InsertEventFunc: func(_ context.Context, event *domain.RunEvent) error {
			captured.Store(event)
			return nil
		},
	}
	srv := newTestServer(t, store, nil, nil)

	req := sdkRequest(t, "POST", "/sdk/runs/run-1/resources", "run-1",
		`{"memory_mb":230,"memory_percent":95,"cpu_percent":50}`)
	w := httptest.NewRecorder()
	TypedHandler(srv, http.StatusCreated, srv.handleSDKResources)(w, req)
	require.EqualValues(t, 201, w.Code)

	ev := captured.Load().(*domain.RunEvent)
	assert.Equal(
		t, "error", ev.Level,
	)
	assert.True(t,
		strings.Contains(ev.Message,
			"critical",
		))

}

func TestHandleSDKResources_NegativeMemoryMB(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	req := sdkRequest(t, "POST", "/sdk/runs/run-1/resources", "run-1",
		`{"memory_mb":-1}`)
	w := httptest.NewRecorder()
	TypedHandler(srv, http.StatusCreated, srv.handleSDKResources)(w, req)
	require.EqualValues(t, 400, w.Code)

}

func TestHandleSDKResources_MemoryPercentOver100(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	req := sdkRequest(t, "POST", "/sdk/runs/run-1/resources", "run-1",
		`{"memory_percent":150}`)
	w := httptest.NewRecorder()
	TypedHandler(srv, http.StatusCreated, srv.handleSDKResources)(w, req)
	require.EqualValues(t, 400, w.Code)

}

func TestHandleSDKResources_CPUPercentOver100(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	req := sdkRequest(t, "POST", "/sdk/runs/run-1/resources", "run-1",
		`{"cpu_percent":200}`)
	w := httptest.NewRecorder()
	TypedHandler(srv, http.StatusCreated, srv.handleSDKResources)(w, req)
	require.EqualValues(t, 400, w.Code)

}

func TestHandleSDKResources_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	req := sdkRequest(t, "POST", "/sdk/runs/run-1/resources", "run-1",
		`{not json`)
	w := httptest.NewRecorder()
	TypedHandler(srv, http.StatusCreated, srv.handleSDKResources)(w, req)
	require.EqualValues(t, 400, w.Code)

}

func TestHandleSDKResources_InsertEventFailure(t *testing.T) {
	t.Parallel()

	store := &APIStoreMock{
		InsertEventFunc: func(_ context.Context, _ *domain.RunEvent) error {
			return errors.New("db down")
		},
	}
	srv := newTestServer(t, store, nil, nil)

	req := sdkRequest(t, "POST", "/sdk/runs/run-1/resources", "run-1",
		`{"memory_mb":100,"memory_percent":40,"cpu_percent":20}`)
	w := httptest.NewRecorder()
	TypedHandler(srv, http.StatusCreated, srv.handleSDKResources)(w, req)
	require.EqualValues(t, 500, w.Code)

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err == nil {
		assert.True(t,
			strings.Contains(resp["error"], "failed to store"))

	}
}
