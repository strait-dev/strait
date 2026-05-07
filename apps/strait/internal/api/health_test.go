package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/health"
)

func TestHandleHealth_PublicFields(t *testing.T) {
	t.Parallel()
	s := &Server{
		edition:   domain.EditionCommunity,
		version:   "v1.2.3",
		startedAt: time.Now().Add(-10 * time.Second),
		config:    &config.Config{InternalSecret: "test-secret"},
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
	if resp["version"] != "v1.2.3" {
		t.Errorf("expected version=v1.2.3, got %v", resp["version"])
	}
	if _, exists := resp["timestamp"]; !exists {
		t.Error("expected timestamp in public health response")
	}
	if _, exists := resp["edition"]; exists {
		t.Error("public health should not expose edition (internal only)")
	}
	if _, exists := resp["uptime_seconds"]; exists {
		t.Error("public health should not expose uptime_seconds")
	}
	if _, exists := resp["subsystems"]; exists {
		t.Error("public health should not expose subsystems")
	}
}

func TestHandleHealth_InternalSecret_ShowsDetails(t *testing.T) {
	t.Parallel()
	reg := health.NewRegistry()
	reg.Register(health.NewChecker("database", func(_ context.Context) error { return nil }))
	reg.Register(health.NewChecker("redis", func(_ context.Context) error { return nil }))

	s := &Server{
		edition:        domain.EditionCommunity,
		version:        "dev",
		startedAt:      time.Now().Add(-10 * time.Second),
		config:         &config.Config{InternalSecret: "test-secret"},
		healthRegistry: reg,
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Internal-Secret", "test-secret")
	rr := httptest.NewRecorder()
	s.handleHealth(rr, req)

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	if resp["edition"] != "community" {
		t.Errorf("expected edition=community in internal response, got %v", resp["edition"])
	}
	uptime, ok := resp["uptime_seconds"].(float64)
	if !ok || uptime < 10 {
		t.Errorf("expected uptime_seconds >= 10 with internal secret, got %v", resp["uptime_seconds"])
	}

	subsystems, ok := resp["subsystems"].(map[string]any)
	if !ok {
		t.Fatalf("expected subsystems map with internal secret, got %T", resp["subsystems"])
	}
	if subsystems["database"] != "up" {
		t.Errorf("expected database=up, got %v", subsystems["database"])
	}
	if subsystems["redis"] != "up" {
		t.Errorf("expected redis=up, got %v", subsystems["redis"])
	}
}

func TestHandleHealth_WrongSecret_NoDetails(t *testing.T) {
	t.Parallel()
	reg := health.NewRegistry()
	reg.Register(health.NewChecker("database", func(_ context.Context) error { return nil }))

	s := &Server{
		edition:        domain.EditionCommunity,
		version:        "dev",
		startedAt:      time.Now(),
		config:         &config.Config{InternalSecret: "real-secret"},
		healthRegistry: reg,
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Internal-Secret", "wrong-secret")
	rr := httptest.NewRecorder()
	s.handleHealth(rr, req)

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	if _, exists := resp["subsystems"]; exists {
		t.Error("wrong secret should not expose subsystems")
	}
	if _, exists := resp["uptime_seconds"]; exists {
		t.Error("wrong secret should not expose uptime")
	}
}

func TestHandleHealth_NoRegistry_NoSubsystems(t *testing.T) {
	t.Parallel()
	s := &Server{
		edition:   domain.EditionCloud,
		version:   "dev",
		startedAt: time.Now(),
		config:    &config.Config{InternalSecret: "test-secret"},
	}

	rr := httptest.NewRecorder()
	s.handleHealth(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	if _, exists := resp["subsystems"]; exists {
		t.Error("expected no subsystems key when registry is nil")
	}
}

// Regression: /health/ready must not leak subsystem inventory to
// unauthenticated callers. The ready probe is reachable from any
// network position (it is a load-balancer health check) and must
// never reveal "database vs redis vs clickhouse went down" to
// fingerprinters.

func TestHandleHealthReady_PublicHidesSubsystems(t *testing.T) {
	t.Parallel()
	reg := health.NewRegistry()
	reg.Register(health.NewChecker("database", func(_ context.Context) error { return nil }))
	reg.Register(health.NewChecker("redis", func(_ context.Context) error { return errors.New("redis is down") }))

	s := &Server{
		edition:        domain.EditionCommunity,
		version:        "dev",
		startedAt:      time.Now(),
		config:         &config.Config{InternalSecret: "real-secret"},
		healthRegistry: reg,
	}

	rr := httptest.NewRecorder()
	s.handleHealthReady(rr, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	body := rr.Body.String()
	if rr.Code != http.StatusServiceUnavailable && rr.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", rr.Code, body)
	}
	for _, leak := range []string{"database", "redis", "components", "redis is down", "connection refused"} {
		if contains := func() bool {
			for i := 0; i+len(leak) <= len(body); i++ {
				if body[i:i+len(leak)] == leak {
					return true
				}
			}
			return false
		}(); contains {
			t.Errorf("/health/ready leaked %q to unauthenticated caller: %s", leak, body)
		}
	}
}

func TestHandleHealthReady_InternalSecretShowsDetails(t *testing.T) {
	t.Parallel()
	reg := health.NewRegistry()
	reg.Register(health.NewChecker("database", func(_ context.Context) error { return nil }))
	reg.Register(health.NewChecker("redis", func(_ context.Context) error { return errors.New("redis is down") }))

	s := &Server{
		edition:        domain.EditionCommunity,
		version:        "dev",
		startedAt:      time.Now(),
		config:         &config.Config{InternalSecret: "real-secret"},
		healthRegistry: reg,
	}

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	req.Header.Set("X-Internal-Secret", "real-secret")
	rr := httptest.NewRecorder()
	s.handleHealthReady(rr, req)

	body := rr.Body.String()
	// With auth, subsystem detail must be present so operators can debug.
	for _, want := range []string{"database", "redis"} {
		found := false
		for i := 0; i+len(want) <= len(body); i++ {
			if body[i:i+len(want)] == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("internal /health/ready missing %q: %s", want, body)
		}
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
		config:         &config.Config{InternalSecret: "test-secret"},
		healthRegistry: reg,
	}

	rr := httptest.NewRecorder()
	s.handleHealth(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 even when degraded, got %d", rr.Code)
	}

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	if resp["status"] == "ok" {
		t.Error("expected non-ok status when a subsystem is down")
	}
}
