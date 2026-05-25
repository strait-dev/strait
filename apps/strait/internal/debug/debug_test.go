package debug

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestMountPprofRoutes_RegistersEndpoints(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	MountPprofRoutes(r)

	tests := []struct {
		name string
		path string
	}{
		{name: "allocs", path: "/debug/pprof/allocs"},
		{name: "block", path: "/debug/pprof/block"},
		{name: "goroutine", path: "/debug/pprof/goroutine"},
		{name: "mutex", path: "/debug/pprof/mutex"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code == http.StatusNotFound {
				t.Fatalf("GET %s returned 404, route should be registered", tt.path)
			}
		})
	}
}

func TestMountPprofRoutes_DoesNotExposeExploratoryEndpoints(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	MountPprofRoutes(r)

	for _, path := range []string{
		"/debug/pprof",
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/heap",
		"/debug/pprof/symbol",
		"/debug/pprof/threadcreate",
		"/debug/pprof/trace",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want %d", path, w.Code, http.StatusNotFound)
		}
	}
}

func TestMountPprofRoutes_RejectsTextDebugOutput(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	MountPprofRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine?debug=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("GET /debug/pprof/goroutine?debug=1 status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCapSeconds_ClampsLongProfiles(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/profile?seconds=120", nil)
	got := capSeconds(req, MaxPprofProfileSeconds)

	seconds, err := strconv.Atoi(got.URL.Query().Get("seconds"))
	if err != nil {
		t.Fatalf("seconds not an integer: %v", err)
	}
	if seconds != MaxPprofProfileSeconds {
		t.Fatalf("seconds = %d, want %d", seconds, MaxPprofProfileSeconds)
	}
}
