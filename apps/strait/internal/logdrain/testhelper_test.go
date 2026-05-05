package logdrain

import (
	"net/http"

	"strait/internal/httputil"
)

// init disables SSRF endpoint validation for all unit tests in this package.
// Tests use httptest.NewServer which binds to 127.0.0.1, and the SSRF
// validator correctly rejects loopback addresses. The API handlers still
// validate URLs at creation/update time as defense in depth.
func init() {
	validateEndpointURL = func(_ string) error { return nil }
	newServiceTransport = func(bool) *http.Transport {
		return httputil.NewExternalTransport(true)
	}
	newAuditSIEMTransport = func(bool) *http.Transport {
		return httputil.NewExternalTransport(true)
	}
}
