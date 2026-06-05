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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Equal(t, http.StatusOK,
		rr.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(
		t, "ok", resp["status"])
	assert.Equal(
		t, "v1.2.3", resp["version"])

	if _, exists := resp["timestamp"]; !exists {
		assert.Fail(t,

			"expected timestamp in public health response")
	}
	if _, exists := resp["edition"]; exists {
		assert.Fail(t,

			"public health should not expose edition (internal only)")
	}
	if _, exists := resp["uptime_seconds"]; exists {
		assert.Fail(t,

			"public health should not expose uptime_seconds")
	}
	if _, exists := resp["subsystems"]; exists {
		assert.Fail(t,

			"public health should not expose subsystems")
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
	assert.Equal(
		t, "community", resp["edition"])

	uptime, ok := resp["uptime_seconds"].(float64)
	assert.False(
		t, !ok || uptime <
			10)

	subsystems, ok := resp["subsystems"].(map[string]any)
	require.True(
		t, ok)
	assert.Equal(
		t, "up", subsystems["database"])
	assert.Equal(
		t, "up", subsystems["redis"])
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
		assert.Fail(t,

			"wrong secret should not expose subsystems")
	}
	if _, exists := resp["uptime_seconds"]; exists {
		assert.Fail(t,

			"wrong secret should not expose uptime")
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
		assert.Fail(t,

			"expected no subsystems key when registry is nil")
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
	require.False(t, rr.Code != http.
		StatusServiceUnavailable &&
		rr.Code != http.
			StatusOK,
	)

	for _, leak := range []string{"database", "redis", "components", "redis is down", "connection refused"} {
		if contains := func() bool {
			for i := 0; i+len(leak) <= len(body); i++ {
				if body[i:i+len(leak)] == leak {
					return true
				}
			}
			return false
		}(); contains {
			assert.Failf(t, "test failure",

				"/health/ready leaked %q to unauthenticated caller: %s", leak, body)
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
		assert.True(t,
			found)
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
	require.Equal(t, http.StatusOK,
		rr.Code)

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	assert.NotEqual(t, "ok", resp["status"])
}
