package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"strait/internal/domain"
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
	TypedHandler(srv, 201, srv.handleSDKResources)(w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	ev := captured.Load().(*domain.RunEvent)
	if ev.Type != "resource_sample" {
		t.Errorf("expected type resource_sample, got %s", ev.Type)
	}
	if ev.Level != "info" {
		t.Errorf("expected level info, got %s", ev.Level)
	}
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
	TypedHandler(srv, 201, srv.handleSDKResources)(w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	ev := captured.Load().(*domain.RunEvent)
	if ev.Level != "warn" {
		t.Errorf("expected level warn, got %s", ev.Level)
	}
	if !strings.Contains(ev.Message, "warning") {
		t.Errorf("expected message containing 'warning', got %s", ev.Message)
	}
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
	TypedHandler(srv, 201, srv.handleSDKResources)(w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	ev := captured.Load().(*domain.RunEvent)
	if ev.Level != "error" {
		t.Errorf("expected level error, got %s", ev.Level)
	}
	if !strings.Contains(ev.Message, "critical") {
		t.Errorf("expected message containing 'critical', got %s", ev.Message)
	}
}

func TestHandleSDKResources_NegativeMemoryMB(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	req := sdkRequest(t, "POST", "/sdk/runs/run-1/resources", "run-1",
		`{"memory_mb":-1}`)
	w := httptest.NewRecorder()
	TypedHandler(srv, 201, srv.handleSDKResources)(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleSDKResources_MemoryPercentOver100(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	req := sdkRequest(t, "POST", "/sdk/runs/run-1/resources", "run-1",
		`{"memory_percent":150}`)
	w := httptest.NewRecorder()
	TypedHandler(srv, 201, srv.handleSDKResources)(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleSDKResources_CPUPercentOver100(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	req := sdkRequest(t, "POST", "/sdk/runs/run-1/resources", "run-1",
		`{"cpu_percent":200}`)
	w := httptest.NewRecorder()
	TypedHandler(srv, 201, srv.handleSDKResources)(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleSDKResources_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	req := sdkRequest(t, "POST", "/sdk/runs/run-1/resources", "run-1",
		`{not json`)
	w := httptest.NewRecorder()
	TypedHandler(srv, 201, srv.handleSDKResources)(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
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
	TypedHandler(srv, 201, srv.handleSDKResources)(w, req)

	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err == nil {
		if !strings.Contains(resp["error"], "failed to store") {
			t.Errorf("expected error about storing, got %s", resp["error"])
		}
	}
}
