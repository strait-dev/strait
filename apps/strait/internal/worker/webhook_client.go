package worker

import (
	"net/http"
	"time"

	"strait/internal/httputil"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	webhookTimeout         time.Duration = 10_000_000_000
	webhookMaxIdleConns                  = 20
	webhookMaxIdlePerHost                = 5
	webhookIdleConnTimeout time.Duration = 60_000_000_000
)

// noFollowWebhookRedirects refuses to follow HTTP redirects on outbound
// webhook deliveries. Following 3xx without re-validating the destination
// IP would let a public webhook target bounce the request to internal
// addresses (cloud metadata, 10.x, 127.x) after the initial SSRF check.
func noFollowWebhookRedirects(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}

func newSafeWebhookTransport() *http.Transport {
	transport := httputil.NewExternalTransport(false)
	transport.MaxIdleConns = webhookMaxIdleConns
	transport.MaxIdleConnsPerHost = webhookMaxIdlePerHost
	transport.IdleConnTimeout = webhookIdleConnTimeout
	return transport
}

var webhookClient = &http.Client{
	Timeout:       webhookTimeout,
	Transport:     otelhttp.NewTransport(newSafeWebhookTransport()),
	CheckRedirect: noFollowWebhookRedirects,
}
