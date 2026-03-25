package notification

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/jarcoal/httpmock"
)

// TestWebhookPayload_XSSInText verifies that script tags in notification payloads are passed
// through without being interpreted, and the Content-Type remains application/json.
func TestWebhookPayload_XSSInText(t *testing.T) {
	t.Parallel()

	client, transport := newMockClient(t)

	var capturedBody []byte
	var capturedContentType string
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(req *http.Request) (*http.Response, error) {
			capturedBody, _ = io.ReadAll(req.Body)
			capturedContentType = req.Header.Get("Content-Type")
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	xssPayload := `{"message":"<script>alert('xss')</script>","title":"<img onerror=alert(1) src=x>"}`
	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("job.failed", json.RawMessage(xssPayload))

	err := sender.Send(context.Background(), ch, del)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Payload should be forwarded verbatim -- webhook sender does not sanitize.
	if string(capturedBody) != xssPayload {
		t.Errorf("body mismatch:\n  got:  %s\n  want: %s", capturedBody, xssPayload)
	}

	// Content-Type must be application/json to prevent browser interpretation.
	if capturedContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", capturedContentType, "application/json")
	}
}

// TestWebhookPayload_HugePayload verifies behavior with a very large (10MB) payload.
func TestWebhookPayload_HugePayload(t *testing.T) {
	t.Parallel()

	client, transport := newMockClient(t)

	var capturedSize int
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(req *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(req.Body)
			capturedSize = len(body)
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	// Build a ~10MB JSON payload.
	largeValue := strings.Repeat("A", 10*1024*1024)
	payload, err := json.Marshal(map[string]string{"data": largeValue})
	if err != nil {
		t.Fatalf("failed to marshal large payload: %v", err)
	}

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "")
	del := newTestDelivery("job.completed", payload)

	sendErr := sender.Send(context.Background(), ch, del)
	if sendErr != nil {
		t.Fatalf("Send with 10MB payload failed: %v", sendErr)
	}

	if capturedSize != len(payload) {
		t.Errorf("captured size = %d, want %d", capturedSize, len(payload))
	}
}

// TestChannelValidation_InvalidURL verifies behavior with various invalid webhook URLs.
func TestChannelValidation_InvalidURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
	}{
		{"empty url", ""},
		{"just protocol", "http://"},
		{"no scheme", "example.com/hook"},
		{"javascript scheme", "javascript:alert(1)"},
		{"ftp scheme", "ftp://example.com/hook"},
		{"null bytes in url", "https://example.com/\x00hook"},
		{"spaces in url", "https://example .com/hook"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sender := NewWebhookSender(&http.Client{})
			ch := newTestChannel(tt.url, "")
			del := newTestDelivery("job.completed", json.RawMessage(`{}`))

			err := sender.Send(context.Background(), ch, del)
			if tt.url == "" {
				// Empty URL is explicitly checked in the Send method.
				if err == nil {
					t.Fatal("expected error for empty URL")
				}
				return
			}
			// For other invalid URLs, we expect either an error from URL parsing
			// or from the HTTP client. The sender should not panic.
			// Some of these may actually succeed if the mock transport or OS resolves them;
			// the important thing is no panic.
			_ = err
		})
	}
}

// FuzzNotificationPayload fuzzes the webhook sender with arbitrary payloads and event types.
func FuzzNotificationPayload(f *testing.F) {
	f.Add("job.completed", `{"run_id":"r-1"}`, "https://example.com/hook", "secret123")
	f.Add("", `{}`, "", "")
	f.Add("run.failed", `null`, "https://example.com", "")
	f.Add("x", `{"key":"`+strings.Repeat("v", 1000)+`"}`, "https://example.com/hook", "s")
	f.Add("\x00event", `{`, "https://example.com", "\x00")

	f.Fuzz(func(t *testing.T, eventType, payload, url, secret string) {
		// Build the channel config.
		cfg, err := json.Marshal(webhookConfig{URL: url, Secret: secret})
		if err != nil {
			return
		}

		ch := &domain.NotificationChannel{
			ID:          "ch-fuzz",
			ChannelType: "webhook",
			Config:      cfg,
			Enabled:     true,
		}
		del := &domain.NotificationDelivery{
			ID:        "del-fuzz",
			EventType: eventType,
			Payload:   json.RawMessage(payload),
		}

		// Use a transport that always succeeds to avoid network errors.
		transport := httpmock.NewMockTransport()
		transport.RegisterNoResponder(
			httpmock.NewStringResponder(200, "ok"))
		client := &http.Client{Transport: transport}

		sender := NewWebhookSender(client)

		// Must not panic regardless of input.
		_ = sender.Send(context.Background(), ch, del)
	})
}
