package domain

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuditActionKeyRotated_Registered asserts the audit.key_rotated
// action is registered in both the action set and the schema, with the
// expected required keys and forbidden-key coverage.
func TestAuditActionKeyRotated_Registered(t *testing.T) {
	t.Parallel()
	require.True(t,
		IsKnownAuditAction(AuditActionKeyRotated))

	schema, ok := AuditActionSchemas[AuditActionKeyRotated]
	require.True(t,
		ok)

	for _, required := range []string{"previous_epoch", "new_epoch", "rotated_by"} {
		assert.True(t,
			slices.Contains(schema.Required, required))

	}

	// ForbiddenKeysFor must union action-specific and common forbidden keys;
	// common-set entries like "secret", "token", "private_key" must apply
	// to audit.key_rotated too as defense-in-depth.
	forbidden := ForbiddenKeysFor(AuditActionKeyRotated)
	for _, must := range []string{"secret", "token", "private_key", "key", "new_key"} {
		assert.True(t,
			slices.Contains(forbidden, must))

	}
}
