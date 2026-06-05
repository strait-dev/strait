package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSDKResourceSnapshot_OOMRisk_InsertsEvent(t *testing.T) {
	t.Parallel()
	var insertedEvent *domain.RunEvent
	ms := &APIStoreMock{
		InsertEventFunc: func(_ context.Context, event *domain.RunEvent) error {
			insertedEvent = event
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, "POST", "/sdk/v1/runs/run-1/resource-snapshot", "run-1",
		`{"cpu_percent":50,"memory_mb":950,"memory_limit_mb":1000,"network_rx_bytes":100,"network_tx_bytes":200}`)
	TypedHandler(srv, http.StatusCreated, srv.handleSDKResourceSnapshot)(w, r)
	require.EqualValues(t, 201, w.Code)
	require.NotNil(t, insertedEvent)
	assert.Equal(
		t, domain.EventType("resource.oom_risk"),
		insertedEvent.
			Type)
	assert.Equal(
		t, "warn", insertedEvent.
			Level)

}

func TestSDKResourceSnapshot_NoOOMRisk_BelowThreshold(t *testing.T) {
	t.Parallel()
	eventCalled := false
	ms := &APIStoreMock{
		InsertEventFunc: func(_ context.Context, _ *domain.RunEvent) error {
			eventCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, "POST", "/sdk/v1/runs/run-1/resource-snapshot", "run-1",
		`{"cpu_percent":50,"memory_mb":800,"memory_limit_mb":1000,"network_rx_bytes":100,"network_tx_bytes":200}`)
	TypedHandler(srv, http.StatusCreated, srv.handleSDKResourceSnapshot)(w, r)
	require.EqualValues(t, 201, w.Code)
	require.False(t, eventCalled)

}

func TestSDKResourceSnapshot_NoOOMRisk_ZeroLimit(t *testing.T) {
	t.Parallel()
	eventCalled := false
	ms := &APIStoreMock{
		InsertEventFunc: func(_ context.Context, _ *domain.RunEvent) error {
			eventCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, "POST", "/sdk/v1/runs/run-1/resource-snapshot", "run-1",
		`{"cpu_percent":50,"memory_mb":800,"memory_limit_mb":0,"network_rx_bytes":100,"network_tx_bytes":200}`)
	TypedHandler(srv, http.StatusCreated, srv.handleSDKResourceSnapshot)(w, r)
	require.EqualValues(t, 201, w.Code)
	require.False(t, eventCalled)

}
