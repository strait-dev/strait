package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/health"
)

func TestHandleHealth_BasicFields(t *testing.T) {
	t.Parallel()
	s := &Server{
		edition:   domain.EditionCommunity,
		version:   "v1.2.3",
		startedAt: time.Now().Add(-10 * time.Second),
	}

	rr := httptest.NewRecorder()
	s.handleHealth(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", resp["status"])
	}
	if resp["edition"] != "community" {
		t.Errorf("expected edition=community, got %v", resp["edition"])
	}
	if resp["version"] != "v1.2.3" {
		t.Errorf("expected version=v1.2.3, got %v", resp["version"])
	}
	uptime, ok := resp["uptime_seconds"].(float64)
	if !ok || uptime < 10 {
		t.Errorf("expected uptime_seconds >= 10, got %v", resp["uptime_seconds"])
	}
}

func TestHandleHealth_NoRegistry_NoSubsystems(t *testing.T) {
	t.Parallel()
	s := &Server{
		edition:   domain.EditionCloud,
		version:   "dev",
		startedAt: time.Now(),
	}

	rr := httptest.NewRecorder()
	s.handleHealth(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	if _, exists := resp["subsystems"]; exists {
		t.Error("expected no subsystems key when registry is nil")
	}
}

func TestHandleHealth_WithRegistry_ReturnsSubsystems(t *testing.T) {
	t.Parallel()
	reg := health.NewRegistry()
	reg.Register(health.NewChecker("database", func(_ context.Context) error { return nil }))
	reg.Register(health.NewChecker("redis", func(_ context.Context) error { return nil }))

	s := &Server{
		edition:        domain.EditionCommunity,
		version:        "dev",
		startedAt:      time.Now(),
		healthRegistry: reg,
	}

	rr := httptest.NewRecorder()
	s.handleHealth(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	subsystems, ok := resp["subsystems"].(map[string]any)
	if !ok {
		t.Fatalf("expected subsystems map, got %T", resp["subsystems"])
	}
	if subsystems["database"] != "up" {
		t.Errorf("expected database=up, got %v", subsystems["database"])
	}
	if subsystems["redis"] != "up" {
		t.Errorf("expected redis=up, got %v", subsystems["redis"])
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", resp["status"])
	}
}

func TestHandleHealth_DegradedSubsystem(t *testing.T) {
	t.Parallel()
	reg := health.NewRegistry()
	reg.Register(health.NewChecker("database", func(_ context.Context) error { return nil }))
	reg.Register(health.NewChecker("redis", func(_ context.Context) error { return errors.New("connection refused") }))

	s := &Server{
		edition:        domain.EditionCommunity,
		version:        "dev",
		startedAt:      time.Now(),
		healthRegistry: reg,
	}

	rr := httptest.NewRecorder()
	s.handleHealth(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	// Still 200 (for load balancer compatibility).
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 even when degraded, got %d", rr.Code)
	}

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	// Status should reflect degradation.
	if resp["status"] == "ok" {
		t.Error("expected non-ok status when a subsystem is down")
	}

	subsystems := resp["subsystems"].(map[string]any)
	if subsystems["database"] != "up" {
		t.Errorf("expected database=up, got %v", subsystems["database"])
	}
}
