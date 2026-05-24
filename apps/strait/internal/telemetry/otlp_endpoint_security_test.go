package telemetry

import (
	"net/url"
	"strings"
	"testing"
)

func TestRedactOTLPEndpoint_RemovesUserInfoAndCredentialQuery(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("https://user:pass@otel.example.com:4318/v1/traces?api_key=abc&auth_token=def&tenant=prod")
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}

	got := redactOTLPEndpoint(u)
	for _, secret := range []string{"user", "pass", "abc", "def"} {
		if strings.Contains(got, secret) {
			t.Fatalf("redacted endpoint %q still contains secret fragment %q", got, secret)
		}
	}
	for _, want := range []string{"api_key=%5Bredacted%5D", "auth_token=%5Bredacted%5D", "tenant=prod"} {
		if !strings.Contains(got, want) {
			t.Fatalf("redacted endpoint %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "@otel.example.com") {
		t.Fatalf("redacted endpoint %q still contains userinfo separator", got)
	}
}

func TestRedactOTLPEndpoint_DoesNotMutateInput(t *testing.T) {
	t.Parallel()

	raw := "https://user:pass@otel.example.com:4318/v1/logs?token=secret"
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}

	_ = redactOTLPEndpoint(u)
	if u.String() != raw {
		t.Fatalf("input URL mutated: got %q, want %q", u.String(), raw)
	}
}
