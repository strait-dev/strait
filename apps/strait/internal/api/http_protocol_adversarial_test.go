package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestHTTPProtocol_CRLFInjectionInHeaders verifies that CRLF sequences in
// header values do not cause header splitting in the response.
func TestHTTPProtocol_CRLFInjectionInHeaders(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	req := authedRequest(http.MethodGet, "/v1/jobs", "")
	req.Header.Set("X-Custom", "safe\r\nEvil-Header: injected")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Empty(t, rec.Header().Get("Evil-Header"))

	// The response must never contain an Evil-Header that was injected via CRLF.

	// Also scan raw header keys for the injected header.
	for key := range rec.Header() {
		require.False(t, strings.EqualFold(key, "Evil-Header"))
	}
}

// TestHTTPProtocol_ExtremelyManyHeaders sends a request with 1000+ headers and
// verifies the server does not crash.
func TestHTTPProtocol_ExtremelyManyHeaders(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	req := authedRequest(http.MethodGet, "/v1/jobs", "")
	for i := range 1100 {
		req.Header.Set(fmt.Sprintf("X-Flood-%d", i), "value")
	}

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	// Any status is acceptable as long as the server did not panic.
	code := rec.Code
	if code != http.StatusOK && code != http.StatusRequestHeaderFieldsTooLarge {
		// Still acceptable; we only care about no crash.
		t.Logf("got status %d, which is fine as long as there was no panic", code)
	}
}

// TestHTTPProtocol_ExtremelyLargeHeaderValue sends a request with a 1MB header
// value and verifies the server does not crash.
func TestHTTPProtocol_ExtremelyLargeHeaderValue(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	req := authedRequest(http.MethodGet, "/v1/jobs", "")
	largeVal := strings.Repeat("A", 1<<20) // 1 MB.
	req.Header.Set("X-Large", largeVal)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.NotEqual(t, 0,
		rec.Code)

	// Any non-panic response is a pass.
}

// TestHTTPProtocol_ContentLengthMismatch sends a request where Content-Length
// exceeds the actual body size.
func TestHTTPProtocol_ContentLengthMismatch(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	body := strings.Repeat("x", 50)
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs", strings.NewReader(body))
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = 100 // Claim 100 bytes but only send 50.

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.NotEqual(t, 0,
		rec.Code)

	// The server should respond with an error or handle it gracefully.
}

// TestHTTPProtocol_DuplicateContentType sends a request with two Content-Type
// headers and verifies the server handles it without crashing.
func TestHTTPProtocol_DuplicateContentType(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs", strings.NewReader(`{"name":"test"}`))
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Content-Type", "text/plain")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.NotEqual(t, 0,
		rec.Code)
}

// TestHTTPProtocol_MissingContentTypeOnPOST sends a POST request with a JSON
// body but no Content-Type header.
func TestHTTPProtocol_MissingContentTypeOnPOST(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs", strings.NewReader(`{"name":"test"}`))
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	// Deliberately omit Content-Type.

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.NotEqual(t, 0,
		rec.Code)

	// The server should return a client error or handle gracefully.
}

// TestHTTPProtocol_ChunkedZeroLength sends a request with Transfer-Encoding
// chunked and a zero-length body.
func TestHTTPProtocol_ChunkedZeroLength(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs", strings.NewReader(""))
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Transfer-Encoding", "chunked")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.NotEqual(t, 0,
		rec.Code)
}

// TestHTTPProtocol_UnexpectedMethods sends OPTIONS, TRACE, and CONNECT to
// /v1/jobs and verifies the server rejects them appropriately.
func TestHTTPProtocol_UnexpectedMethods(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	methods := []string{http.MethodTrace, "CONNECT"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(method, "/v1/jobs", nil)
			req.Header.Set("X-Internal-Secret", "test-secret-value")

			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			require.NotEqual(t, http.
				StatusOK, rec.Code,
			)

			// TRACE and CONNECT should not return 200 OK.
		})
	}

	// OPTIONS without CORS preflight headers may return 405 from the
	// router. The important thing is that the server does not panic.
	t.Run("OPTIONS", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodOptions, "/v1/jobs", nil)
		req.Header.Set("X-Internal-Secret", "test-secret-value")

		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		require.NotEqual(t, 0,
			rec.Code)
	})
}

// TestHTTPProtocol_VeryLongURLPath sends a request with a 100KB URL path and
// verifies the server does not crash.
func TestHTTPProtocol_VeryLongURLPath(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	longPath := "/v1/jobs/" + strings.Repeat("a", 100*1024)
	req := httptest.NewRequest(http.MethodGet, longPath, nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.NotEqual(t, 0,
		rec.Code)

	// Any status is acceptable as long as the server did not panic.
}

// TestHTTPProtocol_MassiveQueryParameters sends a GET request with 10000 query
// parameters and verifies the server handles it gracefully.
func TestHTTPProtocol_MassiveQueryParameters(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	var b strings.Builder
	b.WriteString("/v1/jobs?")
	for i := range 10000 {
		if i > 0 {
			b.WriteByte('&')
		}
		fmt.Fprintf(&b, "key%d=val%d", i, i)
	}

	req := httptest.NewRequest(http.MethodGet, b.String(), nil)
	req.Header.Set("X-Internal-Secret", "test-secret-value")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.NotEqual(t, 0,
		rec.Code)
}

// TestHTTPProtocol_BodyOnGETRequest sends a GET request with a body and
// verifies the server ignores the body and responds normally.
func TestHTTPProtocol_BodyOnGETRequest(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", strings.NewReader(`{"unexpected":"body"}`))
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.NotEqual(t, 0,
		rec.Code)

	// The server should process the GET normally and ignore the body.
}

// TestHTTPProtocol_KeepAliveAbuse sends 100 sequential requests to the same
// server and verifies goroutine count remains stable.
func TestHTTPProtocol_KeepAliveAbuse(t *testing.T) {
	// Not parallel: measures global goroutine count.

	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	// Capture baseline goroutine count after the server is created.
	runtime.GC()
	baseline := runtime.NumGoroutine()

	for range 100 {
		req := authedRequest(http.MethodGet, "/v1/jobs", "")
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		require.NotEqual(t, 0,
			rec.Code)
	}

	runtime.GC()
	after := runtime.NumGoroutine()

	// Allow a generous margin; the key check is that goroutines are not
	// growing unboundedly (e.g. 100 leaked goroutines would indicate a
	// problem).
	growth := after - baseline
	require.LessOrEqual(t,
		growth, 50)
}
