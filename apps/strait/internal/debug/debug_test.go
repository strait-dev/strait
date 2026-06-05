package debug

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/arl/statsviz"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestMountDebugRoutes_RegistersEndpoint(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	MountDebugRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/debug/statsviz/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
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
	assert.NotEqual(t, http.StatusNotFound, w.Code)
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

	require.Equal(t, http.StatusOK, w.Code)
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

			assert.NotEqual(t, http.StatusNotFound, w.Code)
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

		require.Equal(t, http.StatusNotFound, w.Code)
	}
}

func TestMountPprofRoutes_RejectsTextDebugOutput(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	MountPprofRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine?debug=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCapSeconds_ClampsLongProfiles(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/profile?seconds=120", nil)
	got := capSeconds(req, MaxPprofProfileSeconds)

	seconds, err := strconv.Atoi(got.URL.Query().Get("seconds"))
	require.NoError(t, err)
	assert.Equal(t, MaxPprofProfileSeconds, seconds)
}
