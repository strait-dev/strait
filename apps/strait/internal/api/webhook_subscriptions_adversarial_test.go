package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"
)

// webhookSubStore returns an APIStoreMock suitable for webhook subscription
// creation tests. It records the created subscription.
func webhookSubStore() *APIStoreMock {
	return &APIStoreMock{
		CreateWebhookSubscriptionFunc: func(_ context.Context, sub *domain.WebhookSubscription) error {
			sub.ID = "sub-created"
			return nil
		},
	}
}

// postWebhookSub is a helper that sends a POST request to the webhook
// subscriptions endpoint and returns the recorder.
func postWebhookSub(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	srv := newTestServer(t, webhookSubStore(), &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhooks/subscriptions", body))
	return w
}

// TestWebhookSub_LocalhostURL verifies that http://localhost is rejected as a
// webhook URL to prevent SSRF against the loopback interface.
func TestWebhookSub_LocalhostURL(t *testing.T) {
	t.Parallel()

	w := postWebhookSub(t, `{
		"project_id": "proj-1",
		"webhook_url": "http://localhost/hook",
		"event_types": ["run.completed"],
		"secret": "s3cret"
	}`)

	if w.Code < 400 {
		t.Fatalf("expected 4xx for localhost URL, got %d: %s", w.Code, w.Body.String())
	}
}

// TestWebhookSub_PrivateIPURL verifies that private IP addresses (RFC 1918)
// are rejected to prevent SSRF.
func TestWebhookSub_PrivateIPURL(t *testing.T) {
	t.Parallel()

	privateURLs := []string{
		"http://10.0.0.1/hook",
		"http://172.16.0.1/hook",
		"http://192.168.1.1/hook",
		"http://127.0.0.1/hook",
	}

	for _, u := range privateURLs {
		t.Run(u, func(t *testing.T) {
			t.Parallel()
			body := `{"project_id":"proj-1","webhook_url":"` + u + `","event_types":["run.completed"],"secret":"s3cret"}`
			w := postWebhookSub(t, body)

			if w.Code < 400 {
				t.Fatalf("expected 4xx for private IP URL %s, got %d: %s", u, w.Code, w.Body.String())
			}
		})
	}
}

// TestWebhookSub_MetadataURL verifies that the cloud metadata endpoint
// (169.254.169.254) is rejected.
func TestWebhookSub_MetadataURL(t *testing.T) {
	t.Parallel()

	w := postWebhookSub(t, `{
		"project_id": "proj-1",
		"webhook_url": "http://169.254.169.254/latest/meta-data/",
		"event_types": ["run.completed"],
		"secret": "s3cret"
	}`)

	if w.Code < 400 {
		t.Fatalf("expected 4xx for metadata URL, got %d: %s", w.Code, w.Body.String())
	}
}

// TestWebhookSub_FileScheme verifies that non-HTTP schemes like file:// are
// rejected.
func TestWebhookSub_FileScheme(t *testing.T) {
	t.Parallel()

	w := postWebhookSub(t, `{
		"project_id": "proj-1",
		"webhook_url": "file:///etc/passwd",
		"event_types": ["run.completed"],
		"secret": "s3cret"
	}`)

	if w.Code < 400 {
		t.Fatalf("expected 4xx for file:// scheme, got %d: %s", w.Code, w.Body.String())
	}
}

// TestWebhookSub_EmbeddedCredentials verifies that a URL with embedded
// user:pass credentials is handled without panicking. The validator may accept
// or reject it; we just assert the response is a valid HTTP status.
func TestWebhookSub_EmbeddedCredentials(t *testing.T) {
	t.Parallel()

	w := postWebhookSub(t, `{
		"project_id": "proj-1",
		"webhook_url": "https://user:pass@example.com/hook",
		"event_types": ["run.completed"],
		"secret": "s3cret"
	}`)

	if w.Code < 200 || w.Code >= 600 {
		t.Fatalf("unexpected status %d: %s", w.Code, w.Body.String())
	}
}

// TestWebhookSub_EmptyEventTypes verifies that an empty event_types array is
// rejected by validation.
func TestWebhookSub_EmptyEventTypes(t *testing.T) {
	t.Parallel()

	w := postWebhookSub(t, `{
		"project_id": "proj-1",
		"webhook_url": "https://example.com/hook",
		"event_types": [],
		"secret": "s3cret"
	}`)

	if w.Code < 400 {
		t.Fatalf("expected 4xx for empty event_types, got %d: %s", w.Code, w.Body.String())
	}
}

// TestWebhookSub_InvalidEventType verifies that an unknown event type string
// does not cause a panic. The endpoint may accept or reject unknown types
// depending on business logic.
func TestWebhookSub_InvalidEventType(t *testing.T) {
	t.Parallel()

	w := postWebhookSub(t, `{
		"project_id": "proj-1",
		"webhook_url": "https://example.com/hook",
		"event_types": ["totally.bogus.event"],
		"secret": "s3cret"
	}`)

	if w.Code < 200 || w.Code >= 600 {
		t.Fatalf("unexpected status %d: %s", w.Code, w.Body.String())
	}
}

// TestWebhookSub_EmptySecret verifies that an empty secret is rejected by
// validation (the field has a "required" tag).
func TestWebhookSub_EmptySecret(t *testing.T) {
	t.Parallel()

	w := postWebhookSub(t, `{
		"project_id": "proj-1",
		"webhook_url": "https://example.com/hook",
		"event_types": ["run.completed"],
		"secret": ""
	}`)

	if w.Code < 400 {
		t.Fatalf("expected 4xx for empty secret, got %d: %s", w.Code, w.Body.String())
	}
}

// TestWebhookSub_NullByteInURL verifies that a null byte embedded in the
// webhook URL does not cause a panic.
func TestWebhookSub_NullByteInURL(t *testing.T) {
	t.Parallel()

	// Build a JSON body with a literal null byte inside the URL value.
	body := `{"project_id":"proj-1","webhook_url":"https://example.com/\u0000hook","event_types":["run.completed"],"secret":"s3cret"}`
	w := postWebhookSub(t, body)

	if w.Code < 200 || w.Code >= 600 {
		t.Fatalf("unexpected status %d: %s", w.Code, w.Body.String())
	}
}

// FuzzWebhookSubURL fuzzes the webhook URL field to ensure the handler never
// panics regardless of input.
func FuzzWebhookSubURL(f *testing.F) {
	f.Add("https://example.com/hook")
	f.Add("http://localhost")
	f.Add("http://10.0.0.1")
	f.Add("http://169.254.169.254/latest")
	f.Add("file:///etc/passwd")
	f.Add("ftp://example.com")
	f.Add("")
	f.Add("http://user:pass@example.com")
	f.Add("https://example.com:99999")
	f.Add("http://[::1]/hook")

	f.Fuzz(func(t *testing.T, rawURL string) {
		// Escape the URL for safe JSON embedding.
		escaped := strings.ReplaceAll(rawURL, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		body := `{"project_id":"proj-1","webhook_url":"` + escaped + `","event_types":["run.completed"],"secret":"s3cret"}`

		srv := newTestServer(t, webhookSubStore(), &mockQueue{}, nil)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/subscriptions", strings.NewReader(body))
		r.Header.Set("X-Internal-Secret", testInternalSecret)
		r.Header.Set("Content-Type", "application/json")
		srv.ServeHTTP(w, r)

		// Must not panic and must return a valid HTTP status.
		if w.Code < 200 || w.Code >= 600 {
			t.Fatalf("unexpected status %d for URL %q", w.Code, rawURL)
		}
	})
}
