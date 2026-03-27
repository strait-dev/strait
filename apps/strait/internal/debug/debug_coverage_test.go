package debug

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMountDebugRoutes_StatsvizIndexReturnsHTML(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	MountDebugRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/debug/statsviz/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /debug/statsviz/ status = %d, want %d", w.Code, http.StatusOK)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}

	body := w.Body.String()
	if body == "" {
		t.Error("response body is empty")
	}
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
	if w.Code == http.StatusNotFound {
		t.Fatal("GET /debug/statsviz/ws returned 404, route should be registered")
	}
}

func TestMountDebugRoutes_NonDebugRouteReturns404(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	MountDebugRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/jobs status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestMountDebugRoutes_PostMethodNotAllowed(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	MountDebugRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/debug/statsviz/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// POST to a GET-only route should return 405 Method Not Allowed.
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /debug/statsviz/ status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "ok" {
		t.Errorf("GET /healthz body = %q, want %q", w.Body.String(), "ok")
	}

	// Verify debug route also works.
	req = httptest.NewRequest(http.MethodGet, "/debug/statsviz/", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /debug/statsviz/ status = %d, want %d", w.Code, http.StatusOK)
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("GET /debug/statsviz/ after double mount status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestMountDebugRoutes_StatsvizIndexBodyContainsStatsviz(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	MountDebugRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/debug/statsviz/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /debug/statsviz/ status = %d, want %d", w.Code, http.StatusOK)
	}

	// The statsviz index page should contain identifying content.
	body := w.Body.String()
	if !strings.Contains(strings.ToLower(body), "statsviz") && !strings.Contains(body, "<html") {
		t.Error("response body does not contain expected statsviz or html content")
	}
}

func TestMountDebugRoutes_HeadRequest(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	MountDebugRoutes(r)

	req := httptest.NewRequest(http.MethodHead, "/debug/statsviz/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// HEAD on a GET route should return 200 with no body (or 405 depending on router).
	if w.Code == http.StatusNotFound {
		t.Error("HEAD /debug/statsviz/ returned 404, route should be registered")
	}
}
