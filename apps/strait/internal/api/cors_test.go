package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/config"

	"github.com/stretchr/testify/assert"
)

func TestCORS_AllowedOrigin(t *testing.T) {
	t.Parallel()
	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:     "test-secret-value",
			JWTSigningKey:      testJWTSigningKey,
			CORSAllowedOrigins: []string{"https://example.com"},
		},
		Store:  &APIStoreMock{},
		Queue:  &mockQueue{},
		PubSub: &mockPublisher{},
	})
	t.Cleanup(srv.Close)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://example.com")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t,
		http.StatusOK, w.Code,
	)

	origin := w.Header().Get("Access-Control-Allow-Origin")
	assert.Equal(t,
		"https://example.com",
		origin)

}

func TestCORS_Preflight(t *testing.T) {
	t.Parallel()
	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:     "test-secret-value",
			JWTSigningKey:      testJWTSigningKey,
			CORSAllowedOrigins: []string{"https://example.com"},
		},
		Store:  &APIStoreMock{},
		Queue:  &mockQueue{},
		PubSub: &mockPublisher{},
	})
	t.Cleanup(srv.Close)

	req := httptest.NewRequest(http.MethodOptions, "/v1/jobs", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.False(t,
		w.Code != http.StatusOK &&
			w.Code !=
				http.StatusNoContent,
	)

	origin := w.Header().Get("Access-Control-Allow-Origin")
	assert.Equal(t,
		"https://example.com",
		origin)

	methods := w.Header().Get("Access-Control-Allow-Methods")
	assert.NotEqual(t, "", methods)

}

func TestCORS_WildcardOrigin(t *testing.T) {
	t.Parallel()
	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:     "test-secret-value",
			JWTSigningKey:      testJWTSigningKey,
			CORSAllowedOrigins: []string{"*"},
		},
		Store:  &APIStoreMock{},
		Queue:  &mockQueue{},
		PubSub: &mockPublisher{},
	})
	t.Cleanup(srv.Close)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://any-domain.com")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	origin := w.Header().Get("Access-Control-Allow-Origin")
	assert.Equal(t,
		"*", origin)

}

func TestCORS_Credentials(t *testing.T) {
	t.Parallel()
	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:       "test-secret-value",
			JWTSigningKey:        testJWTSigningKey,
			CORSAllowedOrigins:   []string{"https://example.com"},
			CORSAllowCredentials: true,
		},
		Store:  &APIStoreMock{},
		Queue:  &mockQueue{},
		PubSub: &mockPublisher{},
	})
	t.Cleanup(srv.Close)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://example.com")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	creds := w.Header().Get("Access-Control-Allow-Credentials")
	assert.Equal(t,
		"true", creds)

}

func TestCORS_NoOriginHeader(t *testing.T) {
	t.Parallel()
	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:     "test-secret-value",
			JWTSigningKey:      testJWTSigningKey,
			CORSAllowedOrigins: []string{"https://example.com"},
		},
		Store:  &APIStoreMock{},
		Queue:  &mockQueue{},
		PubSub: &mockPublisher{},
	})
	t.Cleanup(srv.Close)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	origin := w.Header().Get("Access-Control-Allow-Origin")
	assert.Equal(t,
		"", origin)

}

func TestCORS_ExposedHeaders(t *testing.T) {
	t.Parallel()
	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:     "test-secret-value",
			JWTSigningKey:      testJWTSigningKey,
			CORSAllowedOrigins: []string{"*"},
		},
		Store:  &APIStoreMock{},
		Queue:  &mockQueue{},
		PubSub: &mockPublisher{},
	})
	t.Cleanup(srv.Close)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://example.com")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	exposed := w.Header().Get("Access-Control-Expose-Headers")
	assert.NotEqual(t, "", exposed)

}
