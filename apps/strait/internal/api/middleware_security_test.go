package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
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
		}

		for header, want := range expected {
			got := rec.Header().Get(header)
			if got != want {
				t.Errorf("header %s = %q, want %q", header, got, want)
			}
		}
	})

	t.Run("no HSTS on plain HTTP", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
			t.Errorf("HSTS should not be set on plain HTTP, got %q", got)
		}
	})

	t.Run("HSTS set on TLS", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.TLS = &tls.ConnectionState{}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		want := "max-age=63072000; includeSubDomains"
		if got := rec.Header().Get("Strict-Transport-Security"); got != want {
			t.Errorf("HSTS = %q, want %q", got, want)
		}
	})

	t.Run("HSTS set via X-Forwarded-Proto", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Strict-Transport-Security"); got == "" {
			t.Error("HSTS should be set when X-Forwarded-Proto is https")
		}
	})
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
			if got := requestIsHTTPS(tt.req); got != tt.want {
				t.Errorf("requestIsHTTPS() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSecureCookie_Defaults(t *testing.T) {
	cookie := SecureCookie("session", "abc123", 3600)

	if cookie.Name != "session" {
		t.Errorf("Name = %q, want %q", cookie.Name, "session")
	}
	if cookie.Value != "abc123" {
		t.Errorf("Value = %q, want %q", cookie.Value, "abc123")
	}
	if cookie.MaxAge != 3600 {
		t.Errorf("MaxAge = %d, want %d", cookie.MaxAge, 3600)
	}
	if cookie.Path != "/" {
		t.Errorf("Path = %q, want %q", cookie.Path, "/")
	}
	if !cookie.Secure {
		t.Error("Secure should be true")
	}
	if !cookie.HttpOnly {
		t.Error("HttpOnly should be true")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want SameSiteStrictMode", cookie.SameSite)
	}
}
