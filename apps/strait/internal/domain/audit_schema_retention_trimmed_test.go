package domain

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuditActionRetentionTrimmed_Registered asserts the
// audit.retention_trimmed action is registered in both the action set and the
// schema with the expected required keys and common forbidden-key coverage.
func TestAuditActionRetentionTrimmed_Registered(t *testing.T) {
	t.Parallel()
	require.True(t,
		IsKnownAuditAction(AuditActionRetentionTrimmed))

	schema, ok := AuditActionSchemas[AuditActionRetentionTrimmed]
	require.True(t,
		ok)

	for _, required := range []string{"deleted_count", "trimmed_before", "previous_hash"} {
		assert.True(t,
			slices.Contains(schema.Required, required))
	}

	// Common forbidden keys (defense-in-depth) must still apply to the
	// tombstone action.
	forbidden := ForbiddenKeysFor(AuditActionRetentionTrimmed)
	for _, must := range []string{"secret", "token", "private_key", "password"} {
		assert.True(t,
			slices.Contains(forbidden, must))
	}
}
