package domain

import (
	"slices"
	"testing"
)

// TestAuditActionRetentionTrimmed_Registered asserts the
// audit.retention_trimmed action is registered in both the action set and the
// schema with the expected required keys and common forbidden-key coverage.
func TestAuditActionRetentionTrimmed_Registered(t *testing.T) {
	t.Parallel()

	if !IsKnownAuditAction(AuditActionRetentionTrimmed) {
		t.Fatal("audit.retention_trimmed is not registered in allAuditActions")
	}

	schema, ok := AuditActionSchemas[AuditActionRetentionTrimmed]
	if !ok {
		t.Fatal("audit.retention_trimmed has no schema entry")
	}

	for _, required := range []string{"deleted_count", "trimmed_before", "previous_hash"} {
		if !slices.Contains(schema.Required, required) {
			t.Errorf("schema.Required missing %q, have %v", required, schema.Required)
		}
	}

	// Common forbidden keys (defense-in-depth) must still apply to the
	// tombstone action.
	forbidden := ForbiddenKeysFor(AuditActionRetentionTrimmed)
	for _, must := range []string{"secret", "token", "private_key", "password"} {
		if !slices.Contains(forbidden, must) {
			t.Errorf("ForbiddenKeysFor(audit.retention_trimmed) missing %q, have %v", must, forbidden)
		}
	}
}
