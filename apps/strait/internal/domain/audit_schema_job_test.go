package domain

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestForbiddenKeysFor_JobUpdatedIncludesSigningSecretAliases(t *testing.T) {
	t.Parallel()

	forbidden := ForbiddenKeysFor(AuditActionJobUpdated)
	for _, must := range []string{"endpoint_signing_secret", "webhook_secret", "signing_secret"} {
		require.True(t,
			slices.Contains(forbidden, must))
	}
}
