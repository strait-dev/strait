package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"orchestrator/internal/config"
)

func TestCORS_AllowedOrigin(t *testing.T) {
	srv := NewServer(&config.Config{
		InternalSecret:     "test-secret",
		JWTSigningKey:      "test-jwt-key-must-be-32-chars-long",
		CORSAllowedOrigins: []string{"https://example.com"},
	}, &mockAPIStore{}, &mockQueue{}, &mockPublisher{}, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://example.com")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	origin := w.Header().Get("Access-Control-Allow-Origin")
	if origin != "https://example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", origin, "https://example.com")
	}
}

func TestCORS_Preflight(t *testing.T) {
	srv := NewServer(&config.Config{
		InternalSecret:     "test-secret",
		JWTSigningKey:      "test-jwt-key-must-be-32-chars-long",
		CORSAllowedOrigins: []string{"https://example.com"},
	}, &mockAPIStore{}, &mockQueue{}, &mockPublisher{}, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodOptions, "/v1/jobs", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 200 or 204", w.Code)
	}

	origin := w.Header().Get("Access-Control-Allow-Origin")
	if origin != "https://example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", origin, "https://example.com")
	}

	methods := w.Header().Get("Access-Control-Allow-Methods")
	if methods == "" {
		t.Error("Access-Control-Allow-Methods header missing")
	}
}

func TestCORS_WildcardOrigin(t *testing.T) {
	srv := NewServer(&config.Config{
		InternalSecret:     "test-secret",
		JWTSigningKey:      "test-jwt-key-must-be-32-chars-long",
		CORSAllowedOrigins: []string{"*"},
	}, &mockAPIStore{}, &mockQueue{}, &mockPublisher{}, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://any-domain.com")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	origin := w.Header().Get("Access-Control-Allow-Origin")
	if origin != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", origin, "*")
	}
}

func TestCORS_Credentials(t *testing.T) {
	srv := NewServer(&config.Config{
		InternalSecret:       "test-secret",
		JWTSigningKey:        "test-jwt-key-must-be-32-chars-long",
		CORSAllowedOrigins:   []string{"https://example.com"},
		CORSAllowCredentials: true,
	}, &mockAPIStore{}, &mockQueue{}, &mockPublisher{}, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://example.com")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	creds := w.Header().Get("Access-Control-Allow-Credentials")
	if creds != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want %q", creds, "true")
	}
}

func TestCORS_NoOriginHeader(t *testing.T) {
	srv := NewServer(&config.Config{
		InternalSecret:     "test-secret",
		JWTSigningKey:      "test-jwt-key-must-be-32-chars-long",
		CORSAllowedOrigins: []string{"https://example.com"},
	}, &mockAPIStore{}, &mockQueue{}, &mockPublisher{}, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	origin := w.Header().Get("Access-Control-Allow-Origin")
	if origin != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty (no Origin sent)", origin)
	}
}

func TestCORS_ExposedHeaders(t *testing.T) {
	srv := NewServer(&config.Config{
		InternalSecret:     "test-secret",
		JWTSigningKey:      "test-jwt-key-must-be-32-chars-long",
		CORSAllowedOrigins: []string{"*"},
	}, &mockAPIStore{}, &mockQueue{}, &mockPublisher{}, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://example.com")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	exposed := w.Header().Get("Access-Control-Expose-Headers")
	if exposed == "" {
		t.Error("Access-Control-Expose-Headers header missing")
	}
}
