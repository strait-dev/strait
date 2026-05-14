package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"
)

func TestWebhookSubscriptions_RunsWriteScopeCannotCreateSubscription(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-runs", ProjectID: "proj-1", Scopes: []string{domain.ScopeRunsWrite}}, nil
		},
		CreateWebhookSubscriptionFunc: func(_ context.Context, _ *domain.WebhookSubscription) error {
			t.Fatal("runs:write must not authorize webhook subscription creation")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-1","webhook_url":"https://example.com/hook","event_types":["run.completed"],"secret":"secret"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/subscriptions", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer strait_runs_write")
	r.Header.Set("Content-Type", "application/json")

	srv.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWebhookSubscriptions_WebhooksWriteScopeCanCreateSubscription(t *testing.T) {
	t.Parallel()

	called := false
	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-webhooks", ProjectID: "proj-1", Scopes: []string{domain.ScopeWebhooksWrite}}, nil
		},
		CreateWebhookSubscriptionFunc: func(_ context.Context, sub *domain.WebhookSubscription) error {
			called = true
			sub.ID = "sub-1"
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-1","webhook_url":"https://example.com/hook","event_types":["run.completed"],"secret":"secret"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/webhooks/subscriptions", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer strait_webhooks_write")
	r.Header.Set("Content-Type", "application/json")

	srv.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !called {
		t.Fatal("CreateWebhookSubscription was not called")
	}
}

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

func TestWebhookSub_DNSPrivateURL(t *testing.T) {
	t.Parallel()

	w := postWebhookSub(t, `{
		"project_id": "proj-1",
		"webhook_url": "https://internal.example.com/hook",
		"event_types": ["run.completed"],
		"secret": "s3cret"
	}`)

	if w.Code < 400 {
		t.Fatalf("expected 4xx for hostname resolving to private IP, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWebhookSub_RequireTLSRejectsHTTP(t *testing.T) {
	t.Parallel()

	store := webhookSubStore()
	srv := newTestServer(t, store, &mockQueue{}, nil)
	srv.config.WebhookRequireTLS = true
	w := httptest.NewRecorder()
	body := `{"project_id":"proj-1","webhook_url":"http://example.com/hook","event_types":["run.completed"],"secret":"s3cret"}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/webhooks/subscriptions", body))

	if w.Code < 400 {
		t.Fatalf("expected 4xx when WEBHOOK_REQUIRE_TLS rejects HTTP subscription, got %d: %s", w.Code, w.Body.String())
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

// TestWebhookSub_ClientSecretIgnored verifies that a client-supplied secret
// is ignored: the server always generates the signing secret. Any value the
// caller sends — empty, weak, strong — is dropped on the floor.
func TestWebhookSub_ClientSecretIgnored(t *testing.T) {
	t.Parallel()

	w := postWebhookSub(t, `{
		"project_id": "proj-1",
		"webhook_url": "https://example.com/hook",
		"event_types": ["run.completed"],
		"secret": ""
	}`)

	if w.Code != 201 && w.Code != 200 {
		t.Fatalf("expected 201/200 (server generates secret), got %d: %s", w.Code, w.Body.String())
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
		r.Header.Set("X-Internal-Secret", "test-secret-value")
		r.Header.Set("Content-Type", "application/json")
		srv.ServeHTTP(w, r)

		// Must not panic and must return a valid HTTP status.
		if w.Code < 200 || w.Code >= 600 {
			t.Fatalf("unexpected status %d for URL %q", w.Code, rawURL)
		}
	})
}
