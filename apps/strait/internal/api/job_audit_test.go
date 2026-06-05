package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizedJobUpdateAuditChangesRedactsSigningCredentials(t *testing.T) {
	t.Parallel()

	name := "renamed"
	webhookSecret := "sdk-supplied-audit-secret-32-bytes"
	endpointSecret := "platform-audit-secret-32-bytes-ok"

	changes := sanitizedJobUpdateAuditChanges(UpdateJobRequest{
		Name:                  &name,
		WebhookSecret:         &webhookSecret,
		EndpointSigningSecret: &endpointSecret,
	})
	require.Equal(t, name, changes["name"])
	require.Equal(t, true, changes["signing_credential_changed"])

	for _, forbidden := range []string{"webhook_secret", "endpoint_signing_secret"} {
		if _, ok := changes[forbidden]; ok {
			require.Failf(t, "test failure",

				"changes includes secret field %q: %#v", forbidden, changes)
		}
	}
}

func TestSanitizedJobUpdateAuditChangesOmitsSigningMarkerWhenUnchanged(t *testing.T) {
	t.Parallel()

	enabled := true

	changes := sanitizedJobUpdateAuditChanges(UpdateJobRequest{Enabled: &enabled})
	require.Equal(t, true, changes["enabled"])

	if _, ok := changes["signing_credential_changed"]; ok {
		require.Failf(t, "test failure",

			"signing_credential_changed present for non-secret update: %#v", changes)
	}
}
