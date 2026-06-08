package api

import "net/http"

const (
	securityHeaderXContentTypeOptions = "X-Content-Type-Options"
	securityHeaderXFrameOptions       = "X-Frame-Options"
	securityHeaderXXSSProtection      = "X-XSS-Protection"
	securityHeaderHSTS                = "Strict-Transport-Security"
	securityHeaderCSP                 = "Content-Security-Policy"
	securityHeaderReferrerPolicy      = "Referrer-Policy"
	securityHeaderPermissionsPolicy   = "Permissions-Policy"
	securityHeaderCrossDomainPolicies = "X-Permitted-Cross-Domain-Policies"
)

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(securityHeaderXContentTypeOptions, "nosniff")
		w.Header().Set(securityHeaderXFrameOptions, "DENY")
		w.Header().Set(securityHeaderXXSSProtection, "0")
		w.Header().Set(securityHeaderCSP, "default-src 'none'")
		w.Header().Set(securityHeaderReferrerPolicy, "no-referrer")
		w.Header().Set(securityHeaderPermissionsPolicy, "camera=(), microphone=(), geolocation=(), payment=()")
		w.Header().Set(securityHeaderCrossDomainPolicies, "none")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")

		if s.requestIsHTTPS(r) {
			w.Header().Set(securityHeaderHSTS, "max-age=63072000; includeSubDomains")
		}

		next.ServeHTTP(&serverHeaderStripper{ResponseWriter: w}, r)
	})
}

// serverHeaderStripper wraps http.ResponseWriter to strip the Server header
// before it is sent. This prevents reverse proxies from leaking version
// information via the Server response header.
type serverHeaderStripper struct {
	http.ResponseWriter
}

func (s *serverHeaderStripper) WriteHeader(code int) {
	s.Header().Del("Server")
	s.ResponseWriter.WriteHeader(code)
}

func (s *serverHeaderStripper) Write(b []byte) (int, error) {
	s.Header().Del("Server")
	return s.ResponseWriter.Write(b)
}

// Flush delegates to the underlying ResponseWriter if it supports http.Flusher.
// This is required for SSE streaming to work correctly.
func (s *serverHeaderStripper) Flush() {
	s.Header().Del("Server")
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter for middleware that needs
// to inspect the original writer (e.g. chi's timeout middleware).
func (s *serverHeaderStripper) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}

func (s *Server) requestIsHTTPS(r *http.Request) bool {
	if r == nil {
		return false
	}

	if r.TLS != nil {
		return true
	}

	// Only honor X-Forwarded-Proto from a trusted reverse proxy. With no trusted
	// proxies configured, or when the direct peer is not one, any client could
	// spoof X-Forwarded-Proto: https on a plaintext connection and induce an HSTS
	// header. Mirrors realIP's trusted-proxy handling of X-Forwarded-For.
	if len(s.trustedProxies) == 0 || !ipInNets(remoteAddrIP(r), s.trustedProxies) {
		return false
	}

	return r.Header.Get("X-Forwarded-Proto") == "https"
}

// SecureCookie creates an http.Cookie with security defaults.
// All cookies set by the API must use this helper to ensure
// SameSite=Strict, Secure, and HttpOnly are always set.
func SecureCookie(name, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		MaxAge:   maxAge,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
}
