package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/telemetry"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestDebugStatsviz_RequiresAuth(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret: "test-secret-value",
			JWTSigningKey:  testJWTSigningKey,
			DebugStatsviz:  true,
		},
		Store:   &APIStoreMock{},
		Queue:   &mockQueue{},
		PubSub:  &mockPublisher{},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	t.Run("no auth returns 401", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/debug/statsviz/", nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		assert.Equal(
			t, http.StatusUnauthorized,
			w.Code,
		)
	})

	t.Run("wrong secret returns 401", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/debug/statsviz/", nil)
		req.Header.Set("X-Internal-Secret", "wrong-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		assert.Equal(
			t, http.StatusUnauthorized,
			w.Code,
		)
	})

	t.Run("correct secret returns 200", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/debug/statsviz/", nil)
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		assert.Equal(
			t, http.StatusOK,
			w.Code)
	})
}

func TestDebugStatsviz_Disabled_Returns404(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret: "test-secret-value",
			JWTSigningKey:  testJWTSigningKey,
			DebugStatsviz:  false,
		},
		Store:   &APIStoreMock{},
		Queue:   &mockQueue{},
		PubSub:  &mockPublisher{},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	req := httptest.NewRequest(http.MethodGet, "/debug/statsviz/", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(
		t, http.StatusNotFound,
		w.Code)
}

func TestPprof_RequiresAuth(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:   "test-secret-value",
			JWTSigningKey:    testJWTSigningKey,
			ProfilingEnabled: true,
		},
		Store:   &APIStoreMock{},
		Queue:   &mockQueue{},
		PubSub:  &mockPublisher{},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	t.Run("no auth returns 401", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		assert.Equal(
			t, http.StatusUnauthorized,
			w.Code,
		)
	})

	t.Run("wrong secret returns 401", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
		req.Header.Set("X-Internal-Secret", "wrong-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		assert.Equal(
			t, http.StatusUnauthorized,
			w.Code,
		)
	})

	t.Run("correct secret returns 200", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		assert.Equal(
			t, http.StatusOK,
			w.Code)
	})

	t.Run("bearer secret returns 200", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
		req.Header.Set("Authorization", "Bearer test-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		assert.Equal(
			t, http.StatusOK,
			w.Code)
	})
}

func TestPprof_ProfilingSecretOverridesInternalSecret(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:   "test-secret-value",
			JWTSigningKey:    testJWTSigningKey,
			ProfilingEnabled: true,
			ProfilingSecret:  "pprof-secret-value",
		},
		Store:   &APIStoreMock{},
		Queue:   &mockQueue{},
		PubSub:  &mockPublisher{},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	t.Run("internal secret no longer authorizes pprof", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		assert.Equal(
			t, http.StatusUnauthorized,
			w.Code,
		)
	})

	t.Run("profiling secret authorizes pprof", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
		req.Header.Set("X-Internal-Secret", "pprof-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		assert.Equal(
			t, http.StatusOK,
			w.Code)
	})
}

func TestPprof_AuthLimiterScopeIsDedicated(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:   "test-secret-value",
			JWTSigningKey:    testJWTSigningKey,
			ProfilingEnabled: true,
		},
		Store:          &APIStoreMock{},
		Queue:          &mockQueue{},
		PubSub:         &mockPublisher{},
		MetricsHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
		RedisClient:    rdb,
		Edition:        domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	for range 10 {
		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
		req.RemoteAddr = "198.51.100.10:1234"
		req.Header.Set("X-Internal-Secret", "wrong-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
	}

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
	req.RemoteAddr = "198.51.100.10:1234"
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusTooManyRequests,
		w.
			Code)

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "198.51.100.10:1234"
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code)
}

func TestPprof_AuthLimiterScopeIsDedicatedWithProfilingSecret(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:   "test-secret-value",
			JWTSigningKey:    testJWTSigningKey,
			ProfilingEnabled: true,
			ProfilingSecret:  "pprof-secret-value",
		},
		Store:          &APIStoreMock{},
		Queue:          &mockQueue{},
		PubSub:         &mockPublisher{},
		MetricsHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
		RedisClient:    rdb,
		Edition:        domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	for range 10 {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.RemoteAddr = "198.51.100.20:1234"
		req.Header.Set("X-Internal-Secret", "wrong-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "198.51.100.20:1234"
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusTooManyRequests,
		w.
			Code)

	req = httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
	req.RemoteAddr = "198.51.100.20:1234"
	req.Header.Set("X-Internal-Secret", "pprof-secret-value")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code)
}

func TestPprof_RequestsMetricRecordsEndpointAndStatus(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	counter, err := provider.Meter("pprof-test").Int64Counter("strait_pprof_requests_total")
	require.NoError(t, err)

	handler := NewProfilingHandler(ProfilingHandlerDeps{
		Config: &config.Config{
			InternalSecret:   "test-secret-value",
			JWTSigningKey:    testJWTSigningKey,
			ProfilingEnabled: true,
		},
		Metrics: &telemetry.Metrics{PprofRequests: counter},
		Edition: domain.EditionCloud,
	})

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK,
		w.Code)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.
		Collect(context.Background(), &rm))
	require.True(
		t, hasPprofRequestMetric(rm, "goroutine",
			"200",
		))
}

func TestPprof_AllowedCIDRs(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:        "test-secret-value",
			JWTSigningKey:         testJWTSigningKey,
			ProfilingEnabled:      true,
			ProfilingAllowedCIDRs: []string{"192.0.2.0/24"},
		},
		Store:   &APIStoreMock{},
		Queue:   &mockQueue{},
		PubSub:  &mockPublisher{},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	t.Run("allowed remote ip returns 200", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
		req.RemoteAddr = "192.0.2.10:1234"
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		assert.Equal(
			t, http.StatusOK,
			w.Code)
	})

	t.Run("disallowed remote ip returns 403", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
		req.RemoteAddr = "203.0.113.10:1234"
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		assert.Equal(
			t, http.StatusForbidden,
			w.Code)
	})
}

func TestPprof_TextDebugOutputRejected(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:   "test-secret-value",
			JWTSigningKey:    testJWTSigningKey,
			ProfilingEnabled: true,
		},
		Store:   &APIStoreMock{},
		Queue:   &mockQueue{},
		PubSub:  &mockPublisher{},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine?debug=1", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(
		t, http.StatusBadRequest,
		w.Code)
}

func TestPprof_ExploratoryEndpointsNotExposed(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:   "test-secret-value",
			JWTSigningKey:    testJWTSigningKey,
			ProfilingEnabled: true,
		},
		Store:   &APIStoreMock{},
		Queue:   &mockQueue{},
		PubSub:  &mockPublisher{},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	for _, path := range []string{"/debug/pprof/", "/debug/pprof/cmdline", "/debug/pprof/trace", "/debug/pprof/symbol"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		assert.Equal(
			t, http.StatusNotFound,
			w.Code)
	}
}

func TestPprof_ManagementHandlerOnlyExposesPprofRoutes(t *testing.T) {
	t.Parallel()

	handler := NewProfilingHandler(ProfilingHandlerDeps{
		Config: &config.Config{
			InternalSecret:   "test-secret-value",
			JWTSigningKey:    testJWTSigningKey,
			ProfilingEnabled: true,
		},
		Edition: domain.EditionCloud,
	})

	tests := []struct {
		path string
		want int
	}{
		{path: "/debug/pprof/goroutine", want: http.StatusOK},
		{path: "/health", want: http.StatusNotFound},
		{path: "/metrics", want: http.StatusNotFound},
		{path: "/v1/jobs", want: http.StatusNotFound},
		{path: "/debug/pprof/cmdline", want: http.StatusNotFound},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		require.Equal(t, tt.want,
			w.Code)
	}
}

func TestPprof_APIListenerCanBeDisabledForManagementOnly(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:              "test-secret-value",
			JWTSigningKey:               testJWTSigningKey,
			ProfilingEnabled:            true,
			ProfilingAPIEnabled:         false,
			ProfilingManagementEnabled:  true,
			ProfilingManagementBindAddr: "127.0.0.1",
			ProfilingManagementPort:     18080,
			ProfilingMutexFraction:      100,
			ProfilingBlockRate:          100000,
		},
		Store:   &APIStoreMock{},
		Queue:   &mockQueue{},
		PubSub:  &mockPublisher{},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound,
		w.Code)
}

func TestPprof_Disabled_Returns404(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:   "test-secret-value",
			JWTSigningKey:    testJWTSigningKey,
			ProfilingEnabled: false,
		},
		Store:   &APIStoreMock{},
		Queue:   &mockQueue{},
		PubSub:  &mockPublisher{},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(
		t, http.StatusNotFound,
		w.Code)
}

func hasPprofRequestMetric(rm metricdata.ResourceMetrics, endpoint, status string) bool {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "strait_pprof_requests_total" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, dp := range sum.DataPoints {
				if dp.Value <= 0 {
					continue
				}
				if attrValue(dp.Attributes, "endpoint") == endpoint && attrValue(dp.Attributes, "status") == status {
					return true
				}
			}
		}
	}
	return false
}

func attrValue(attrs attribute.Set, key string) string {
	for _, kv := range attrs.ToSlice() {
		if string(kv.Key) == key {
			return kv.Value.AsString()
		}
	}
	return ""
}
