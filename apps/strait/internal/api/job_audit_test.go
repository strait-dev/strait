package api

import "testing"

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

	if changes["name"] != name {
		t.Fatalf("changes[name] = %v, want %q", changes["name"], name)
	}
	if changes["signing_credential_changed"] != true {
		t.Fatalf("signing_credential_changed = %v, want true", changes["signing_credential_changed"])
	}
	for _, forbidden := range []string{"webhook_secret", "endpoint_signing_secret"} {
		if _, ok := changes[forbidden]; ok {
			t.Fatalf("changes includes secret field %q: %#v", forbidden, changes)
		}
	}
}

func TestSanitizedJobUpdateAuditChangesOmitsSigningMarkerWhenUnchanged(t *testing.T) {
	t.Parallel()

	enabled := true

	changes := sanitizedJobUpdateAuditChanges(UpdateJobRequest{Enabled: &enabled})

	if changes["enabled"] != true {
		t.Fatalf("changes[enabled] = %v, want true", changes["enabled"])
	}
	if _, ok := changes["signing_credential_changed"]; ok {
		t.Fatalf("signing_credential_changed present for non-secret update: %#v", changes)
	}
}
