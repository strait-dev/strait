package worker

import (
	"net/http"
	"os"
	"testing"

	"strait/internal/httputil"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func TestMain(m *testing.M) {
	// Production webhookClient blocks private addresses via the SSRF
	// dial-time guard. The bulk of the worker test suite uses
	// httptest.NewServer (binds to 127.0.0.1), so we swap to an
	// allow-private transport for the duration of the package's tests.
	// CheckRedirect is preserved so redirect-related assertions still
	// reflect the production wiring. SSRF-blocking behaviour is
	// exercised in fix_05_webhook_ssrf_test.go via a freshly built
	// production-shape client that bypasses this swap.
	allowPrivateTransport := httputil.NewExternalTransport(true)
	allowPrivateTransport.MaxIdleConns = webhookMaxIdleConns
	allowPrivateTransport.MaxIdleConnsPerHost = webhookMaxIdlePerHost
	allowPrivateTransport.IdleConnTimeout = webhookIdleConnTimeout
	webhookClient = &http.Client{
		Timeout:       webhookTimeout,
		Transport:     otelhttp.NewTransport(allowPrivateTransport),
		CheckRedirect: noFollowWebhookRedirects,
	}

	// goleak.VerifyTestMain is not used because this package includes integration
	// tests (worker_integration_test.go) that create testcontainer instances.
	// Testcontainer goroutines (Reaper, Redis connections, HTTP pool) are detected
	// as leaks by goleak, causing os.Exit(1) which tears down the Redis client
	// mid-flight, failing all subsequent integration tests with "client is closed".
	os.Exit(m.Run())
}
