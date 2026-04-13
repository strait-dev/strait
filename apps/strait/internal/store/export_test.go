package store

import "context"

// SetTombstoneInsertHookForTest installs a pre-insert hook invoked inside
// writeRetentionTombstone just before the anchor INSERT. Passing nil
// restores the no-op default. Test-only — lives in an _test.go file so it
// is never compiled into the production binary.
func SetTombstoneInsertHookForTest(fn func(ctx context.Context) error) {
	tombstoneInsertHook = fn
}

// SetAuditEventPostInsertHookForTest installs a post-insert hook invoked
// inside CreateAuditEvent, after the signed INSERT statement but before
// the surrounding transaction commits. Returning a non-nil error forces
// the tx to roll back, leaving no row behind. Test-only.
func SetAuditEventPostInsertHookForTest(fn func(ctx context.Context) error) {
	auditEventPostInsertHook = fn
}
