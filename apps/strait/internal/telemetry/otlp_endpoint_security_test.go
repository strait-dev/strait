package telemetry

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactOTLPEndpoint_RemovesUserInfoAndCredentialQuery(t *testing.T) {
	t.Parallel()

	u, err := url.Parse("https://user:pass@otel.example.com:4318/v1/traces?api_key=abc&auth_token=def&tenant=prod")
	require.NoError(t,
		err)

	got := redactOTLPEndpoint(u)
	for _, secret := range []string{"user", "pass", "abc", "def"} {
		require.NotContains(t, got, secret)
	}
	for _, want := range []string{"api_key=%5Bredacted%5D", "auth_token=%5Bredacted%5D", "tenant=prod"} {
		require.Contains(t, got, want)
	}
	require.NotContains(t, got, "@otel.example.com")
}

func TestRedactOTLPEndpoint_DoesNotMutateInput(t *testing.T) {
	t.Parallel()

	raw := "https://user:pass@otel.example.com:4318/v1/logs?token=secret"
	u, err := url.Parse(raw)
	require.NoError(t,
		err)

	_ = redactOTLPEndpoint(u)
	require.Equal(t, raw,
		u.String())
}
