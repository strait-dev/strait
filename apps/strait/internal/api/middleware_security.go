package api

import "net/http"

const (
	securityHeaderXContentTypeOptions = "X-Content-Type-Options"
	securityHeaderXFrameOptions       = "X-Frame-Options"
	securityHeaderXXSSProtection      = "X-XSS-Protection"
	securityHeaderHSTS                = "Strict-Transport-Security"
	securityHeaderCSP                 = "Content-Security-Policy"
	securityHeaderReferrerPolicy      = "Referrer-Policy"
)

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(securityHeaderXContentTypeOptions, "nosniff")
		w.Header().Set(securityHeaderXFrameOptions, "DENY")
		w.Header().Set(securityHeaderXXSSProtection, "0")
		w.Header().Set(securityHeaderCSP, "default-src 'none'")
		w.Header().Set(securityHeaderReferrerPolicy, "no-referrer")

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
