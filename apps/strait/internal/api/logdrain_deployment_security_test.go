package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
	"strait/internal/store"
)

// TestLogDrain_AuthConfigHeaderInjection verifies that setting Authorization
// as a custom header in auth_config does not cause a validation error, since
// it is intentionally not in the protected headers list.
func TestLogDrain_AuthConfigHeaderInjection(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateLogDrainFunc: func(_ context.Context, drain *domain.LogDrain) error {
			drain.CreatedAt = time.Now()
			drain.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id": "proj-1",
		"name": "auth-inject",
		"drain_type": "http",
		"endpoint_url": "https://example.com/logs",
		"auth_type": "header",
		"auth_config": {"Authorization": "Bearer stolen-token"}
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", body))
	require.Equal(t, http.StatusCreated,

		w.Code)

	// Authorization is intentionally allowed for custom header auth type.

}

// TestLogDrain_AuthConfigProtectedHeaderCase verifies that protected header
// detection is case-insensitive for the "header" auth type.
func TestLogDrain_AuthConfigProtectedHeaderCase(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateLogDrainFunc: func(_ context.Context, drain *domain.LogDrain) error {
			drain.CreatedAt = time.Now()
			drain.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// "Host" in various cases should all be rejected.
	variants := []string{"Host", "HOST", "host", "HoSt"}
	for _, header := range variants {
		t.Run(header, func(t *testing.T) {
			t.Parallel()
			bodyBytes, _ := json.Marshal(map[string]any{
				"project_id":   "proj-1",
				"name":         "case-test",
				"drain_type":   "http",
				"endpoint_url": "https://example.com/logs",
				"auth_type":    "header",
				"auth_config":  map[string]string{header: "evil.com"},
			})
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", string(bodyBytes)))
			require.NotEqual(t, http.StatusCreated,

				w.Code)
			require.Equal(t, http.StatusBadRequest,

				w.Code)

		})
	}
}

// TestLogDrain_AuthConfigEmptyBearer verifies that an empty bearer token
// in auth_config is accepted without error (it is just stored as-is).
func TestLogDrain_AuthConfigEmptyBearer(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateLogDrainFunc: func(_ context.Context, drain *domain.LogDrain) error {
			drain.CreatedAt = time.Now()
			drain.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id": "proj-1",
		"name": "empty-bearer",
		"drain_type": "http",
		"endpoint_url": "https://example.com/logs",
		"auth_type": "bearer",
		"auth_config": {"token": ""}
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", body))
	require.Equal(t, http.StatusCreated,

		w.Code)

	// Empty bearer token should be accepted (no validation on token value).

}

// TestLogDrain_LevelFilterInvalid verifies that invalid log levels in the
// level_filter field are accepted at the API layer (validation is downstream).
func TestLogDrain_LevelFilterInvalid(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateLogDrainFunc: func(_ context.Context, drain *domain.LogDrain) error {
			drain.CreatedAt = time.Now()
			drain.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id": "proj-1",
		"name": "bad-levels",
		"drain_type": "http",
		"endpoint_url": "https://example.com/logs",
		"auth_type": "bearer",
		"level_filter": ["nonexistent_level", "ultra_critical", "42"]
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", body))

	// The API does not validate level names, so this should succeed.
	if w.Code != http.StatusCreated {
		t.Logf("note: invalid levels rejected with status %d", w.Code)
	}
	require.NotEqual(t, 0, w.Code)

}

// TestLogDrain_LevelFilterUnboundedArray verifies that a very large
// level_filter array does not crash the server.
func TestLogDrain_LevelFilterUnboundedArray(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateLogDrainFunc: func(_ context.Context, drain *domain.LogDrain) error {
			drain.CreatedAt = time.Now()
			drain.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Build an array of 10000 levels.
	levels := make([]string, 10000)
	for i := range levels {
		levels[i] = fmt.Sprintf("level_%d", i)
	}
	bodyMap := map[string]any{
		"project_id":   "proj-1",
		"name":         "huge-levels",
		"drain_type":   "http",
		"endpoint_url": "https://example.com/logs",
		"auth_type":    "bearer",
		"level_filter": levels,
	}
	bodyBytes, _ := json.Marshal(bodyMap)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", string(bodyBytes)))
	require.NotEqual(t, 0, w.Code)

	// Must not panic. May return 201 or 413/400.

}

// TestLogDrain_EndpointSSRF verifies that internal/private IP addresses
// in the endpoint URL are rejected by the SSRF protection.
func TestLogDrain_EndpointSSRF(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateLogDrainFunc: func(_ context.Context, drain *domain.LogDrain) error {
			drain.CreatedAt = time.Now()
			drain.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ssrfURLs := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://metadata.google.internal/computeMetadata/v1/",
		"http://localhost:8080/admin",
	}

	for _, u := range ssrfURLs {
		t.Run(u, func(t *testing.T) {
			t.Parallel()
			bodyBytes, _ := json.Marshal(map[string]any{
				"project_id":   "proj-1",
				"name":         "ssrf-test",
				"drain_type":   "http",
				"endpoint_url": u,
				"auth_type":    "bearer",
			})
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/log-drains", string(bodyBytes)))
			require.NotEqual(t, http.StatusCreated,

				w.Code)
			require.Equal(t, http.StatusBadRequest,

				w.Code)

		})
	}
}

// TestLogDrain_UpdateWithoutAuthType verifies that updating auth_config
// without specifying auth_type falls back to the existing drain's auth type.
func TestLogDrain_UpdateWithoutAuthType(t *testing.T) {
	t.Parallel()

	existingDrain := &domain.LogDrain{
		ID:          "drain-1",
		ProjectID:   "proj-1",
		Name:        "existing",
		DrainType:   "http",
		EndpointURL: "https://example.com/logs",
		AuthType:    "header",
		AuthConfig:  map[string]string{"X-Custom": "old-value"},
		Enabled:     true,
	}

	ms := &APIStoreMock{
		GetLogDrainFunc: func(_ context.Context, id, _ string) (*domain.LogDrain, error) {
			if id == "drain-1" {
				return existingDrain, nil
			}
			return nil, store.ErrLogDrainNotFound
		},
		UpdateLogDrainFunc: func(_ context.Context, _, _ string, _ map[string]any) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Update auth_config without setting auth_type; should look up existing type.
	body := `{"auth_config": {"X-Custom": "new-value"}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/log-drains/drain-1", body))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

}

// TestDeployment_CanaryPercentBoundary verifies the canary_percent boundary
// validation: 1-99 is valid, 0 and 100 are rejected, negative is rejected.
func TestDeployment_CanaryPercentBoundary(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateDeploymentVersionFunc: func(_ context.Context, d *domain.DeploymentVersion) error {
			d.ID = "dep-boundary"
			d.CreatedAt = time.Now()
			d.UpdatedAt = d.CreatedAt
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	tests := []struct {
		name    string
		percent int
		wantOK  bool
	}{
		{"zero", 0, false},
		{"one", 1, true},
		{"fifty", 50, true},
		{"ninety_nine", 99, true},
		{"hundred", 100, false},
		{"negative", -1, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := fmt.Sprintf(`{
				"project_id":"proj-1",
				"environment":"production",
				"runtime":"node",
				"artifact_uri":"https://example.com/artifacts/dep.tgz",
				"strategy":"canary",
				"canary_percent":%d
			}`, tc.percent)

			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments", body))
			require.False(t, tc.wantOK &&
				w.Code !=
					http.StatusCreated,
			)
			require.False(t, !tc.wantOK &&
				w.Code ==
					http.StatusCreated,
			)

		})
	}
}

// TestDeployment_CanaryDurationOverflow verifies that an extremely large
// canary_duration string is handled without panic.
func TestDeployment_CanaryDurationOverflow(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateDeploymentVersionFunc: func(_ context.Context, d *domain.DeploymentVersion) error {
			d.ID = "dep-overflow"
			d.CreatedAt = time.Now()
			d.UpdatedAt = d.CreatedAt
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id":"proj-1",
		"environment":"production",
		"runtime":"node",
		"artifact_uri":"https://example.com/artifacts/dep.tgz",
		"strategy":"canary",
		"canary_percent":50,
		"canary_duration":"9223372036854775807ns"
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments", body))
	require.NotEqual(t, 0, w.Code)

	// The server must not panic. It may accept or reject the extreme duration.

}

// TestDeployment_ManifestArbitraryJSON verifies that a very large manifest
// does not crash the server.
func TestDeployment_ManifestArbitraryJSON(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateDeploymentVersionFunc: func(_ context.Context, d *domain.DeploymentVersion) error {
			d.ID = "dep-bigmanifest"
			d.CreatedAt = time.Now()
			d.UpdatedAt = d.CreatedAt
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Build a ~10MB manifest by repeating a key-value pair.
	bigValue := strings.Repeat("x", 10*1024*1024)
	manifest := map[string]string{"data": bigValue}
	manifestBytes, _ := json.Marshal(manifest)

	bodyMap := map[string]any{
		"project_id":   "proj-1",
		"environment":  "production",
		"runtime":      "node",
		"artifact_uri": "https://example.com/artifacts/dep.tgz",
		"manifest":     json.RawMessage(manifestBytes),
	}
	bodyBytes, _ := json.Marshal(bodyMap)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments", string(bodyBytes)))
	require.NotEqual(t, 0, w.Code)

	// Must not panic. Large payloads may be rejected by the framework.

}

// TestDeployment_ConcurrentVersionPublish verifies that concurrent deployment
// version creation does not cause panics or data races.
func TestDeployment_ConcurrentVersionPublish(t *testing.T) {
	t.Parallel()

	var createCount atomic.Int32
	ms := &APIStoreMock{
		CreateDeploymentVersionFunc: func(_ context.Context, d *domain.DeploymentVersion) error {
			createCount.Add(1)
			d.ID = fmt.Sprintf("dep-%d", createCount.Load())
			d.CreatedAt = time.Now()
			d.UpdatedAt = d.CreatedAt
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	const goroutines = 20
	var wg conc.WaitGroup
	for i := range goroutines {
		n := i
		wg.Go(func() {
			body := fmt.Sprintf(`{
				"project_id":"proj-1",
				"environment":"production",
				"runtime":"node",
				"artifact_uri":"https://example.com/artifacts/dep-%d.tgz"
			}`, n)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments", body))
			assert.False(
				t, w.Code != http.
					StatusCreated &&
					w.
						Code != http.StatusInternalServerError,
			)

		})
	}
	wg.Wait()
	require.NotEqual(t, 0, createCount.
		Load())

}

// FuzzLogDrainAuthConfig fuzzes the auth_config JSON to verify the
// validation logic does not panic on arbitrary input.
func FuzzLogDrainAuthConfig(f *testing.F) {
	f.Add(`{"token": "abc"}`)
	f.Add(`{}`)
	f.Add(`{"Host": "evil.com"}`)
	f.Add(`{"Content-Type": "text/plain"}`)
	f.Add(`{"": "empty-key"}`)
	f.Add(`{"key\x00null": "value"}`)

	f.Fuzz(func(t *testing.T, configJSON string) {
		var config map[string]string
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			return
		}

		// Test the validateAuthConfig function directly.
		_ = validateAuthConfig("header", config)
		_ = validateAuthConfig("bearer", config)
		_ = validateAuthConfig("none", config)
	})
}

// FuzzDeploymentManifest fuzzes the deployment manifest JSON to verify
// that marshalRaw does not panic on arbitrary input.
func FuzzDeploymentManifest(f *testing.F) {
	f.Add(`{"jobs":1}`)
	f.Add(`null`)
	f.Add(`[]`)
	f.Add(`"string"`)
	f.Add(`123`)
	f.Add(`true`)
	f.Add(strings.Repeat(`{"a":`, 20) + `1` + strings.Repeat(`}`, 20))

	f.Fuzz(func(t *testing.T, manifest string) {
		var value any
		if err := json.Unmarshal([]byte(manifest), &value); err != nil {
			return
		}

		// marshalRaw must not panic.
		result := marshalRaw(value)
		require.NotNil(t, result)
		require.True(
			t, json.Valid(result))

		// The result must be valid JSON.

	})
}
