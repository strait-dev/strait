package domain

import (
	"slices"
	"testing"
)

func TestForbiddenKeysFor_JobUpdatedIncludesSigningSecretAliases(t *testing.T) {
	t.Parallel()

	forbidden := ForbiddenKeysFor(AuditActionJobUpdated)
	for _, must := range []string{"endpoint_signing_secret", "webhook_secret", "signing_secret"} {
		if !slices.Contains(forbidden, must) {
			t.Fatalf("ForbiddenKeysFor(%s) missing %q, have %v", AuditActionJobUpdated, must, forbidden)
		}
	}
}
