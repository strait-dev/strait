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
