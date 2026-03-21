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

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(securityHeaderXContentTypeOptions, "nosniff")
		w.Header().Set(securityHeaderXFrameOptions, "DENY")
		w.Header().Set(securityHeaderXXSSProtection, "0")
		w.Header().Set(securityHeaderCSP, "default-src 'none'")
		w.Header().Set(securityHeaderReferrerPolicy, "no-referrer")
		w.Header().Set(securityHeaderPermissionsPolicy, "camera=(), microphone=(), geolocation=(), payment=()")
		w.Header().Set(securityHeaderCrossDomainPolicies, "none")

		if requestIsHTTPS(r) {
			w.Header().Set(securityHeaderHSTS, "max-age=63072000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}

func requestIsHTTPS(r *http.Request) bool {
	if r == nil {
		return false
	}

	if r.TLS != nil {
		return true
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
