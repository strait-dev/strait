package clickhouse

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactWebhookAnalyticsURL_RemovesSecrets(t *testing.T) {
	t.Parallel()

	raw := "https://user:pass@hooks.example.com/path/secret-token?token=abc#frag"
	got := redactWebhookAnalyticsURL(raw)

	for _, secret := range []string{"user", "pass", "secret-token", "token=abc", "frag", "?"} {
		require.False(t, strings.Contains(
			got, secret))

	}
	require.Equal(t, "https://hooks.example.com",

		got)

}

func TestRedactWebhookAnalyticsURL_InvalidURLDoesNotEchoSecret(t *testing.T) {
	t.Parallel()

	got := redactWebhookAnalyticsURL("://bad/secret-token?token=abc")
	require.False(t, strings.Contains(
		got, "secret-token") || strings.Contains(got, "token=abc"))
	require.Equal(t, "[invalid-url]",

		got)

}
