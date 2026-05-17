package domain

import "testing"

func TestForbiddenKeysFor_JobUpdatedIncludesSigningSecretAliases(t *testing.T) {
	t.Parallel()

	forbidden := ForbiddenKeysFor(AuditActionJobUpdated)
	for _, must := range []string{"endpoint_signing_secret", "webhook_secret", "signing_secret"} {
		found := false
		for _, key := range forbidden {
			if key == must {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("ForbiddenKeysFor(%s) missing %q, have %v", AuditActionJobUpdated, must, forbidden)
		}
	}
}
