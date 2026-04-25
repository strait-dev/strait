package domain

import (
	"slices"
	"testing"
)

// TestAuditActionKeyRotated_Registered asserts the audit.key_rotated
// action is registered in both the action set and the schema, with the
// expected required keys and forbidden-key coverage.
func TestAuditActionKeyRotated_Registered(t *testing.T) {
	t.Parallel()

	if !IsKnownAuditAction(AuditActionKeyRotated) {
		t.Fatal("audit.key_rotated is not registered in allAuditActions")
	}

	schema, ok := AuditActionSchemas[AuditActionKeyRotated]
	if !ok {
		t.Fatal("audit.key_rotated has no schema entry")
	}

	for _, required := range []string{"previous_epoch", "new_epoch", "rotated_by"} {
		if !slices.Contains(schema.Required, required) {
			t.Errorf("schema.Required missing %q, have %v", required, schema.Required)
		}
	}

	// ForbiddenKeysFor must union action-specific and common forbidden keys;
	// common-set entries like "secret", "token", "private_key" must apply
	// to audit.key_rotated too as defense-in-depth.
	forbidden := ForbiddenKeysFor(AuditActionKeyRotated)
	for _, must := range []string{"secret", "token", "private_key", "key", "new_key"} {
		if !slices.Contains(forbidden, must) {
			t.Errorf("ForbiddenKeysFor(audit.key_rotated) missing %q, have %v", must, forbidden)
		}
	}
}
