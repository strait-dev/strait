package store

import (
	"context"
	"fmt"
	"time"
)

// Advisory lock namespaces.
//
// Three audit code paths acquire pg_advisory_xact_lock under distinct
// namespace prefixes so they do not serialize against each other
// unnecessarily:
//
//   - AdvisoryLockNsAuditChain       serializes same-project chain
//     inserts for the legacy (empty shard_id) chain in CreateAuditEvent.
//     Multiple tombstone / anchor writers for the same project queue up
//     so the chain tail read and the insert see the same committed state.
//   - AdvisoryLockNsAuditChainShard  serializes inserts within a single
//     (project, shard) sub-chain. The key is `<projectID>:<shardID>`
//     so unrelated shards within the same project do not contend.
//     Legacy rows (empty shard_id) continue to use AdvisoryLockNsAuditChain
//     so the pre-shard lock domain is unchanged for non-sharded writers.
//   - AdvisoryLockNsAuditRotate      serializes key rotation + retention
//     tombstone writes for the same project. A rotation cannot
//     interleave between the tombstone's max-rotation_epoch read and
//     its signed insert, and two concurrent rotations cannot produce
//     two anchors under the same new epoch.
//
// All namespaces are hashed into the int64 advisory key space via
// Postgres hashtext(). Adding a new namespace — or introducing a new
// caller that takes an advisory lock on a per-project key — MUST declare
// the prefix here so collisions are discoverable at code review time.
// Any call that bypasses AcquireAdvisoryLock is flagged by the audit
// advisory-lock test coverage guard.
const (
	AdvisoryLockNsAuditChain      = "audit_chain:"
	AdvisoryLockNsAuditChainShard = "audit_chain_shard:"
	AdvisoryLockNsAuditRotate     = "audit_rotate:"
)

// AcquireAdvisoryLock takes a per-transaction advisory lock scoped by
// namespace + key. The Postgres hashtext() of the concatenation feeds
// pg_advisory_xact_lock, so the lock auto-releases on COMMIT or
// ROLLBACK. namespace MUST be one of the AdvisoryLockNs* constants
// declared above; key is typically a tenant identifier (project id).
//
// This helper replaces ad-hoc literal prefixes at call sites with a
// single code path that can enforce invariants (non-empty namespace,
// non-empty key) and provides a stable place to layer observability
// (e.g. a wait-time histogram) without touching every caller.
func AcquireAdvisoryLock(ctx context.Context, q *Queries, namespace, key string) error {
	if q == nil {
		return fmt.Errorf("acquire advisory lock: queries is nil")
	}
	if namespace == "" {
		return fmt.Errorf("acquire advisory lock: namespace is empty")
	}
	if key == "" {
		return fmt.Errorf("acquire advisory lock: key is empty")
	}
	const (
		maxAttempts = 50
		retryDelay  = 100 * time.Millisecond
	)
	for attempt := range maxAttempts {
		var acquired bool
		if err := q.db.QueryRow(ctx, `
			SELECT pg_try_advisory_xact_lock(hashtext($1::text || $2::text))
		`, namespace, key).Scan(&acquired); err != nil {
			return fmt.Errorf("acquire advisory lock %s%s: %w", namespace, key, err)
		}
		if acquired {
			return nil
		}
		if attempt == maxAttempts-1 {
			break
		}
		t := time.NewTimer(retryDelay)
		select {
		case <-ctx.Done():
			t.Stop()
			return fmt.Errorf("acquire advisory lock %s%s: %w", namespace, key, ctx.Err())
		case <-t.C:
		}
	}
	return fmt.Errorf("acquire advisory lock %s%s: timed out after %d attempts", namespace, key, maxAttempts)
}
