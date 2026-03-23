package api

import (
	"context"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
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
	TypedHandler(srv, 201, srv.handleSDKResourceSnapshot)(w, r)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if insertedEvent == nil {
		t.Fatal("expected InsertEvent to be called for OOM risk")
	}
	if insertedEvent.Type != "resource.oom_risk" {
		t.Errorf("expected event type resource.oom_risk, got %s", insertedEvent.Type)
	}
	if insertedEvent.Level != "warn" {
		t.Errorf("expected level warn, got %s", insertedEvent.Level)
	}
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
	TypedHandler(srv, 201, srv.handleSDKResourceSnapshot)(w, r)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if eventCalled {
		t.Fatal("expected no OOM risk event when below 90% threshold")
	}
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
	TypedHandler(srv, 201, srv.handleSDKResourceSnapshot)(w, r)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if eventCalled {
		t.Fatal("expected no OOM risk event when memory_limit_mb is zero")
	}
}
