package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecurityHeaders(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("sets all security headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		expected := map[string]string{
			"X-Content-Type-Options":            "nosniff",
			"X-Frame-Options":                   "DENY",
			"X-XSS-Protection":                  "0",
			"Content-Security-Policy":           "default-src 'none'",
			"Referrer-Policy":                   "no-referrer",
			"Permissions-Policy":                "camera=(), microphone=(), geolocation=(), payment=()",
			"X-Permitted-Cross-Domain-Policies": "none",
			"Cross-Origin-Resource-Policy":      "same-origin",
		}

		for header, want := range expected {
			got := rec.Header().Get(header)
			assert.Equal(t,
				want, got)
		}
	})

	t.Run("no HSTS on plain HTTP", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Empty(t,
			rec.Header().Get("Strict-Transport-Security"))
	})

	t.Run("HSTS set on TLS", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.TLS = &tls.ConnectionState{}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		want := "max-age=63072000; includeSubDomains"
		assert.Equal(t,
			want, rec.Header().Get("Strict-Transport-Security"))
	})

	t.Run("HSTS set via X-Forwarded-Proto", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.NotEmpty(
			t, rec.Header().Get("Strict-Transport-Security"))
	})
}

func TestSecurityHeaders_StripsServerHeader(t *testing.T) {
	// Simulate a reverse proxy that sets a Server header.
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "Fly/58128dbb4 (2026-03-25)")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Empty(t,
		rec.Header().Get("Server"))
}

func TestSecurityHeaders_StripsServerHeaderOnImplicitWrite(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "Fly/58128dbb4 (2026-03-25)")
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Empty(t,
		rec.Header().Get("Server"))
}

func TestSecurityHeaders_StripsServerHeaderOnFlush(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "Fly/58128dbb4 (2026-03-25)")
		flusher, ok := w.(http.Flusher)
		assert.True(t,
			ok)

		flusher.Flush()
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Empty(t,
		rec.Header().Get("Server"))
	require.True(t,
		rec.Flushed)
}

func TestRequestIsHTTPS(t *testing.T) {
	tests := []struct {
		name string
		req  *http.Request
		want bool
	}{
		{"nil request", nil, false},
		{"plain HTTP", httptest.NewRequest(http.MethodGet, "/", nil), false},
		{"TLS connection", func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.TLS = &tls.ConnectionState{}
			return r
		}(), true},
		{"X-Forwarded-Proto https", func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("X-Forwarded-Proto", "https")
			return r
		}(), true},
		{"X-Forwarded-Proto http", func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("X-Forwarded-Proto", "http")
			return r
		}(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t,
				tt.want, requestIsHTTPS(tt.req))
		})
	}
}

func TestSecureCookie_Defaults(t *testing.T) {
	cookie := SecureCookie("session", "abc123", 3600)
	assert.Equal(t,
		"session", cookie.
			Name)
	assert.Equal(t,
		"abc123", cookie.
			Value)
	assert.Equal(t, 3600, cookie.
		MaxAge)
	assert.Equal(t,
		"/", cookie.
			Path)
	assert.True(t, cookie.
		Secure,
	)
	assert.True(t, cookie.
		HttpOnly,
	)
	assert.Equal(t,
		http.SameSiteStrictMode,
		cookie.SameSite,
	)
}
