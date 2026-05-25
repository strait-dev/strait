package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/config"
	"strait/internal/domain"
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

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", w.Code)
		}
	})

	t.Run("wrong secret returns 401", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/debug/statsviz/", nil)
		req.Header.Set("X-Internal-Secret", "wrong-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", w.Code)
		}
	})

	t.Run("correct secret returns 200", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/debug/statsviz/", nil)
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
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

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when DebugStatsviz=false", w.Code)
	}
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

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", w.Code)
		}
	})

	t.Run("wrong secret returns 401", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
		req.Header.Set("X-Internal-Secret", "wrong-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", w.Code)
		}
	})

	t.Run("correct secret returns 200", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
	})

	t.Run("bearer secret returns 200", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
		req.Header.Set("Authorization", "Bearer test-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
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

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", w.Code)
		}
	})

	t.Run("profiling secret authorizes pprof", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
		req.Header.Set("X-Internal-Secret", "pprof-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
	})
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

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
	})

	t.Run("disallowed remote ip returns 403", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
		req.RemoteAddr = "203.0.113.10:1234"
		req.Header.Set("X-Internal-Secret", "test-secret-value")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", w.Code)
		}
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

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
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

		if w.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want 404", path, w.Code)
		}
	}
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

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when ProfilingEnabled=false", w.Code)
	}
}
