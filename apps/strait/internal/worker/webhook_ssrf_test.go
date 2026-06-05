package worker

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// productionLikeWebhookClient builds a client wired exactly the way the
// production package-level webhookClient is wired, so the SSRF
// behaviour the production code ships can be exercised in tests
// without being affected by the test-suite-wide allow-private
// override applied in main_test.go.
func productionLikeWebhookClient() *http.Client {
	return &http.Client{
		Timeout:       webhookTimeout,
		Transport:     newSafeWebhookTransport(),
		CheckRedirect: noFollowWebhookRedirects,
	}
}

// TestNoFollowWebhookRedirects_ReturnsUseLastResponse pins the
// CheckRedirect helper. http.Client.CheckRedirect must return
// http.ErrUseLastResponse so the client returns the 3xx response itself
// rather than dialling the redirect target -- otherwise a public
// webhook target could bounce the request to internal addresses
// (cloud metadata, 10.x, 127.x) after the SSRF check has passed.
func TestNoFollowWebhookRedirects_ReturnsUseLastResponse(t *testing.T) {
	t.Parallel()

	if got := noFollowWebhookRedirects(nil, nil); !errors.Is(got, http.ErrUseLastResponse) {
		require.Failf(t, "test failure",

			"noFollowWebhookRedirects returned %v, want http.ErrUseLastResponse", got)
	}
}

// TestProductionWebhookClient_RefusesToFollowRedirects pins the
// CheckRedirect wiring on the package-level webhookClient. The
// allow-private override in main_test.go also sets CheckRedirect, so
// this assertion holds for the test-suite-wide swap and the production
// definition alike. Before the fix the client used the default Go
// redirect-follow policy and CheckRedirect was nil.
func TestProductionWebhookClient_RefusesToFollowRedirects(t *testing.T) {
	t.Parallel()
	require.NotNil(t, webhookClient.
		CheckRedirect,
	)

	if got := webhookClient.CheckRedirect(nil, nil); !errors.Is(got, http.ErrUseLastResponse) {
		require.Failf(t, "test failure",

			"webhookClient.CheckRedirect returned %v, want http.ErrUseLastResponse", got)
	}
}

// TestNewSafeWebhookTransport_BlocksPrivateLoopback verifies that
// the production-shape transport refuses to dial 127.0.0.1.
// httptest.NewServer binds to a loopback address -- the test confirms the
// dial-time SSRF guard is wired in.
func TestNewSafeWebhookTransport_BlocksPrivateLoopback(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		assert.Fail(t,

			"server must never be reached: SSRF guard should refuse the dial")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := productionLikeWebhookClient()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL, bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)

	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		require.Failf(t, "test failure",

			"expected SSRF guard to block loopback dial; got status %d", resp.StatusCode)
	}
	require.Contains(t,
		err.
			Error(), "ssrf:")
}

// TestNewSafeWebhookTransport_BlocksLinkLocalMetadata pins the
// adversarial path: a webhook URL pointing at the cloud metadata service
// (169.254.169.254) must be refused at dial time.
func TestNewSafeWebhookTransport_BlocksLinkLocalMetadata(t *testing.T) {
	t.Parallel()

	client := productionLikeWebhookClient()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://169.254.169.254/latest/meta-data/", bytes.NewReader(nil))
	require.NoError(t, err)

	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		require.Failf(t, "test failure",

			"expected SSRF guard to refuse link-local metadata dial; got status %d", resp.StatusCode)
	}
	require.Contains(t,
		err.
			Error(), "ssrf:")
}
