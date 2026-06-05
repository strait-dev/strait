package debug

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMountDebugRoutes_StatsvizIndexReturnsHTML(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	MountDebugRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/debug/statsviz/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	ct := w.Header().Get("Content-Type")
	assert.Contains(t, ct, "text/html")

	body := w.Body.String()
	assert.NotEmpty(t, body)
}

func TestMountDebugRoutes_StatsvizWsRouteExists(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	MountDebugRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/debug/statsviz/ws", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// The WebSocket endpoint is registered but will not complete a WS upgrade
	// in httptest. Verify it does not 404 (route exists).
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestMountDebugRoutes_NonDebugRouteReturns404(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	MountDebugRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestMountDebugRoutes_PostMethodNotAllowed(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	MountDebugRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/debug/statsviz/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// POST to a GET-only route should return 405 Method Not Allowed.
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestMountDebugRoutes_DoesNotAffectExistingRoutes(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	MountDebugRoutes(r)

	// Verify existing route still works.
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", w.Body.String())

	// Verify debug route also works.
	req = httptest.NewRequest(http.MethodGet, "/debug/statsviz/", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestMountDebugRoutes_MultipleMounts(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	// Mounting twice should not panic.
	MountDebugRoutes(r)
	MountDebugRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/debug/statsviz/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestMountDebugRoutes_StatsvizIndexBodyContainsStatsviz(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	MountDebugRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/debug/statsviz/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// The statsviz index page should contain identifying content.
	body := w.Body.String()
	assert.True(t, strings.Contains(strings.ToLower(body), "statsviz") || strings.Contains(body, "<html"))
}

func TestMountDebugRoutes_HeadRequest(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	MountDebugRoutes(r)

	req := httptest.NewRequest(http.MethodHead, "/debug/statsviz/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// HEAD on a GET route should return 200 with no body (or 405 depending on router).
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}
