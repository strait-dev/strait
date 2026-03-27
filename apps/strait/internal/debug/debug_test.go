package debug

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arl/statsviz"
	"github.com/go-chi/chi/v5"
)

func TestMountDebugRoutes_ServerCreationError(t *testing.T) {
	// Not parallel: mutates package-level newStatsvizServer.
	original := newStatsvizServer
	t.Cleanup(func() { newStatsvizServer = original })

	newStatsvizServer = func() (*statsviz.Server, error) {
		return nil, errors.New("simulated statsviz error")
	}

	r := chi.NewRouter()
	MountDebugRoutes(r)

	// No routes should be registered when server creation fails.
	req := httptest.NewRequest(http.MethodGet, "/debug/statsviz/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when server creation fails, got %d", w.Code)
	}
}

func TestMountDebugRoutes_RegistersEndpoint(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	MountDebugRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/debug/statsviz/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /debug/statsviz/ status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestMountDebugRoutes_WebSocket(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	MountDebugRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/debug/statsviz/ws", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// WebSocket endpoint should not 404 (it may return a different status
	// since there's no actual WebSocket upgrade in httptest).
	if w.Code == http.StatusNotFound {
		t.Fatal("GET /debug/statsviz/ws returned 404, expected route to exist")
	}
}

func TestMountDebugRoutes_OtherRoutesUnaffected(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	r.Get("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	MountDebugRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/health status = %d, want %d", w.Code, http.StatusOK)
	}
}
