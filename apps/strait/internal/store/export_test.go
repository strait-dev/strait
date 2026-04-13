package store

import "context"

// SetTombstoneInsertHookForTest installs a pre-insert hook invoked inside
// writeRetentionTombstone just before the anchor INSERT on the provided
// *Queries instance. Passing nil restores the no-op default. Test-only —
// lives in an _test.go file so it is never compiled into the production
// binary. The hook is stored on the Queries struct (not a package-level
// global) so tests cannot leak it across instances.
func SetTombstoneInsertHookForTest(q *Queries, fn func(ctx context.Context) error) {
	if q == nil {
		return
	}
	q.tombstoneInsertHook = fn
}

// SetAuditEventPostInsertHookForTest installs a post-insert hook invoked
// inside CreateAuditEvent on the provided *Queries, after the signed
// INSERT statement but before the surrounding transaction commits.
// Returning a non-nil error forces the tx to roll back, leaving no row
// behind. Test-only.
func SetAuditEventPostInsertHookForTest(q *Queries, fn func(ctx context.Context) error) {
	if q == nil {
		return
	}
	q.auditEventPostInsertHook = fn
}
