package clickhouse

import (
	"strings"
	"testing"
)

func TestRedactWebhookAnalyticsURL_RemovesSecrets(t *testing.T) {
	t.Parallel()

	raw := "https://user:pass@hooks.example.com/path/secret-token?token=abc#frag"
	got := redactWebhookAnalyticsURL(raw)

	for _, secret := range []string{"user", "pass", "secret-token", "token=abc", "frag", "?"} {
		if strings.Contains(got, secret) {
			t.Fatalf("redacted URL %q leaked %q", got, secret)
		}
	}
	if got != "https://hooks.example.com" {
		t.Fatalf("redacted URL = %q, want host-only redaction", got)
	}
}

func TestRedactWebhookAnalyticsURL_InvalidURLDoesNotEchoSecret(t *testing.T) {
	t.Parallel()

	got := redactWebhookAnalyticsURL("://bad/secret-token?token=abc")
	if strings.Contains(got, "secret-token") || strings.Contains(got, "token=abc") {
		t.Fatalf("redacted invalid URL leaked secret data: %q", got)
	}
	if got != "[invalid-url]" {
		t.Fatalf("redacted invalid URL = %q, want [invalid-url]", got)
	}
}
