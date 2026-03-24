package debug

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

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
