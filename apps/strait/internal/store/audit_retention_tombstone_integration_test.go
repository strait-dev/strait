//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedDatedChain inserts n events into projectID spaced 1 hour apart, with
// the earliest event anchored at base. Returns the inserted event ids in
// chronological order. We manually update created_at AFTER CreateAuditEvent
// so each row is aged predictably; because the HMAC signature is bound to
// created_at, we must recompute + persist it under the same signing key.
func seedDatedChain(ctx context.Context, t *testing.T, q *store.Queries, projectID string, n int, base time.Time, signingKey []byte) []string {
	t.Helper()
	ids := make([]string, n)
	for i := range n {
		ev := &domain.AuditEvent{
			ProjectID:    projectID,
			ActorID:      "actor",
			ActorType:    "user",
			Action:       domain.AuditActionJobCreated,
			ResourceType: "job",
			ResourceID:   "job",
			Details:      json.RawMessage(`{"i":` + itoaBench(i) + `}`),
		}
		require.NoError(t, q.CreateAuditEvent(ctx, ev))

		// Backdate this row to base + i hours and re-sign under the SAME key the
		// production CreateAuditEvent used. When the test fixture has a
		// SecretEncryptionKey configured, CreateAuditEvent bootstraps and signs
		// with a per-epoch key in audit_signing_keys; signing the backdated row
		// with the global key would diverge from what VerifyAuditChain looks up.
		// Fall back to the supplied global signingKey only when no per-epoch key
		// is stored (the legacy / no-encryption path).
		newCreatedAt := base.Add(time.Duration(i) * time.Hour).UTC().Truncate(time.Microsecond)
		ev.CreatedAt = newCreatedAt
		effectiveKey := signingKey
		if perEpoch, gerr := q.GetAuditSigningKey(ctx, projectID, ev.RotationEpoch); gerr == nil && perEpoch != nil {
			effectiveKey = perEpoch
		}
		sig := store.ComputeAuditSignature(ev, effectiveKey)
		if _, err := testDB.Pool.Exec(ctx,
			`UPDATE audit_events SET created_at = $1, signature = $2 WHERE id = $3`,
			newCreatedAt, sig, ev.ID,
		); err != nil {
			require.Failf(t, "test failure",

				"backdate+resign iter %d: %v", i, err)
		}
		ids[i] = ev.ID
	}
	return ids
}

// TestDeleteAuditEventsBefore_WritesTombstone — seed 10 rows across a cutoff
// and assert that after trimming, exactly one tombstone anchor row exists
// with the expected details and previous_hash pointing at the surviving tail.
func TestDeleteAuditEventsBefore_WritesTombstone(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("tombstone-test-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-tombstone-writes"
	// 10 events: hours 0..9 (older → newer). Cutoff between index 5 and 6.
	base := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Hour)
	seedDatedChain(ctx, t, q, projectID, 10, base, key)
	cutoff := base.Add(5*time.Hour + 30*time.Minute)

	deleted, err := q.DeleteAuditEventsBefore(ctx, projectID, cutoff)
	require.NoError(t, err)
	require.EqualValues(t, 6, deleted)

	// Exactly one tombstone exists.
	events, err := q.ListAuditEvents(ctx, projectID, "", "", "", 1000, nil, nil, nil, true)
	require.NoError(t, err)

	var tombstones []domain.AuditEvent
	for _, ev := range events {
		if ev.Action == domain.AuditActionRetentionTrimmed {
			tombstones = append(tombstones, ev)
		}
	}
	require.Len(t, tombstones,

		1)

	ts := tombstones[0]
	assert.True(t, ts.IsAnchor)

	var details map[string]any
	require.NoError(t, json.
		Unmarshal(ts.Details,
			&details))

	if got, want := details["deleted_count"], float64(6); got != want {
		assert.Failf(t, "test failure",

			"deleted_count = %v, want %v", got, want)
	}
	if _, ok := details["trimmed_before"].(string); !ok {
		assert.Failf(t, "test failure",

			"trimmed_before not a string: %v", details["trimmed_before"])
	}
	prevHash, _ := details["previous_hash"].(string)
	assert.NotEqual(t, "",
		prevHash,
	)

	chainStart, _ := details["chain_start"].(string)
	assert.NotEqual(t, "",
		chainStart,
	)

	firstSurvivingID, _ := details["first_surviving_event_id"].(string)
	assert.NotEqual(t, "",
		firstSurvivingID,
	)

	// The tombstone's own previous_hash must match the surviving tail
	// (i.e. the signature of the last pre-tombstone event).
	var tailSig string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT signature FROM audit_events
		WHERE project_id = $1 AND action = $2
		ORDER BY created_at DESC LIMIT 1
	`,

		projectID,
		domain.
			AuditActionJobCreated).Scan(&tailSig))
	assert.Equal(t, tailSig,

		ts.PreviousHash,
	)
	assert.Equal(t, tailSig,

		prevHash,
	)

	var survivingHead struct {
		id           string
		previousHash string
	}
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT id, previous_hash FROM audit_events
		WHERE project_id = $1 AND action = $2
		ORDER BY rotation_epoch ASC, created_at ASC, id ASC LIMIT 1
	`,

		projectID, domain.AuditActionJobCreated).Scan(&survivingHead.
		id, &survivingHead.previousHash))
	assert.Equal(t, survivingHead.
		id,
		firstSurvivingID,
	)
	assert.Equal(t, survivingHead.
		previousHash,

		chainStart)

}

// TestDeleteAuditEventsBefore_NoRowsTrimmed_NoTombstone — cutoff before the
// earliest row leaves the chain untouched and emits no tombstone.
func TestDeleteAuditEventsBefore_NoRowsTrimmed_NoTombstone(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("tombstone-noop-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-tombstone-noop"
	base := time.Now().UTC().Add(-4 * time.Hour).Truncate(time.Hour)
	seedDatedChain(ctx, t, q, projectID, 3, base, key)
	cutoff := base.Add(-1 * time.Hour) // earlier than every row

	deleted, err := q.DeleteAuditEventsBefore(ctx, projectID, cutoff)
	require.NoError(t, err)
	assert.EqualValues(t, 0, deleted)

	var tombstoneCount int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*) FROM audit_events
		WHERE project_id = $1 AND action = $2
	`,

		projectID, domain.
			AuditActionRetentionTrimmed).
		Scan(&tombstoneCount))
	assert.EqualValues(t, 0, tombstoneCount)

}

// TestVerifyAuditChain_AcceptsTombstoneAnchor — the chain must remain
// verifiable across the tombstone boundary (the tombstone is a normal
// HMAC-signed row that chains from the surviving tail).
func TestVerifyAuditChain_AcceptsTombstoneAnchor(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("tombstone-verify-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-tombstone-verify"
	base := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Hour)
	seedDatedChain(ctx, t, q, projectID, 8, base, key)
	cutoff := base.Add(4 * time.Hour)

	if _, err := q.DeleteAuditEventsBefore(ctx, projectID, cutoff); err != nil {
		require.Failf(t, "test failure",

			"DeleteAuditEventsBefore: %v", err)
	}

	// Add a few post-trim events to ensure the chain continues past the
	// tombstone cleanly.
	for range 3 {
		ev := &domain.AuditEvent{
			ProjectID:    projectID,
			ActorID:      "actor",
			ActorType:    "user",
			Action:       domain.AuditActionJobUpdated,
			ResourceType: "job",
			ResourceID:   "job",
			Details:      json.RawMessage(`{"changes":"x"}`),
		}
		require.NoError(t, q.CreateAuditEvent(ctx, ev))

	}

	result, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.True(t, result.
		Valid,
	)
	assert.EqualValues(t, 8, result.
		EventsChecked,
	)

	// 4 surviving + 1 tombstone + 3 post = 8.

}

// TestVerifyAuditChain_RejectsDeletedPrefixWithoutTombstone asserts that an
// attacker cannot delete the oldest rows and have the verifier silently accept
// the first surviving row as a new chain head. Legitimate retention trims must
// leave a signed audit.retention_trimmed anchor.
func TestVerifyAuditChain_RejectsDeletedPrefixWithoutTombstone(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("tombstone-prefix-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-prefix-delete"
	base := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Hour)
	ids := seedDatedChain(ctx, t, q, projectID, 5, base, key)

	if _, err := testDB.Pool.Exec(ctx, `
		DELETE FROM audit_events
		WHERE id = ANY($1::text[])
	`, ids[:2]); err != nil {
		require.Failf(t, "test failure",

			"delete prefix rows: %v", err)
	}

	result, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.False(t, result.
		Valid)
	require.Equal(t, ids[2],

		result.BrokenAtID,
	)
	require.NotEqual(t, store.
		ZeroHash,
		result.ChainStart,
	)

}

func TestVerifyAuditChain_RejectsAdditionalPrefixDeleteAfterTombstone(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("tombstone-bound-prefix-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-prefix-delete-after-tombstone"
	base := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Hour)
	ids := seedDatedChain(ctx, t, q, projectID, 8, base, key)

	cutoff := base.Add(3 * time.Hour)
	if _, err := q.DeleteAuditEventsBefore(ctx, projectID, cutoff); err != nil {
		require.Failf(t, "test failure",

			"DeleteAuditEventsBefore: %v", err)
	}
	valid, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.True(t, valid.Valid)

	if _, err := testDB.Pool.Exec(ctx, `DELETE FROM audit_events WHERE id = $1`, ids[3]); err != nil {
		require.Failf(t, "test failure",

			"delete first surviving row after tombstone: %v", err)
	}

	result, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.False(t, result.
		Valid)
	require.Equal(t, ids[4],

		result.BrokenAtID,
	)

}

// TestDeleteAuditEventsBeforeExcluding_EmitsTombstonePerAffectedProject —
// when two projects are trimmed by the default sweep, exactly one tombstone
// is written per affected project.
func TestDeleteAuditEventsBeforeExcluding_EmitsTombstonePerAffectedProject(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("tombstone-bulk-secret")
	q.SetAuditSigningKey(key)

	base := time.Now().UTC().Add(-72 * time.Hour).Truncate(time.Hour)
	pA := "proj-bulk-a"
	pB := "proj-bulk-b"
	pC := "proj-bulk-c-excluded"

	seedDatedChain(ctx, t, q, pA, 5, base, key)
	seedDatedChain(ctx, t, q, pB, 5, base, key)
	seedDatedChain(ctx, t, q, pC, 5, base, key)

	cutoff := base.Add(3 * time.Hour) // deletes rows 0..2 in each project.

	deleted, err := q.DeleteAuditEventsBeforeExcluding(ctx, cutoff, []string{pC})
	require.NoError(t, err)
	assert.EqualValues(t, 6, deleted)

	countTombstones := func(pid string) int {
		var n int
		require.NoError(t, testDB.
			Pool.QueryRow(ctx,
			`
			SELECT COUNT(*) FROM audit_events
			WHERE project_id = $1 AND action = $2
		`,

			pid, domain.
				AuditActionRetentionTrimmed,
		).Scan(
			&n))

		return n
	}
	assert.EqualValues(t, 1, countTombstones(pA))
	assert.EqualValues(t, 1, countTombstones(pB))
	assert.EqualValues(t, 0, countTombstones(pC))

	// Both affected projects still verify.
	if v, err := q.VerifyAuditChain(ctx, pA); err != nil || !v.Valid {
		assert.Failf(t, "test failure",

			"chain A invalid after bulk trim: err=%v valid=%v error=%s", err, v.Valid, v.Error)
	}
	if v, err := q.VerifyAuditChain(ctx, pB); err != nil || !v.Valid {
		assert.Failf(t, "test failure",

			"chain B invalid after bulk trim: err=%v valid=%v error=%s", err, v.Valid, v.Error)
	}
}

// TestDeleteAuditEventsBefore_HappyPathRowCount — sanity-check the commit
// path: post_count = pre_count - deleted + 1 tombstone.
func TestDeleteAuditEventsBefore_HappyPathRowCount(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("tombstone-happy-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-atomic-good"
	base := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Hour)
	seedDatedChain(ctx, t, q, projectID, 5, base, key)

	var pre int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE project_id = $1`,

		projectID).Scan(&pre))
	require.EqualValues(t, 5, pre)

	cutoff := base.Add(3 * time.Hour)
	deleted, err := q.DeleteAuditEventsBefore(ctx, projectID, cutoff)
	require.NoError(t, err)
	require.EqualValues(t, 3, deleted)

	var post int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE project_id = $1`,

		projectID).Scan(&post))
	assert.EqualValues(t, 3, post)

	// 5 - 3 (deleted) + 1 (tombstone) = 3.

}

// TestDeleteAuditEventsBefore_AtomicWithTombstone — when the tombstone
// insert fails, the surrounding transaction must roll back and leave
// every seeded row intact with zero tombstones written.
//
// We force the failure via the test-only tombstoneInsertHook seam in
// audit_events.go: the hook runs inside writeRetentionTombstone,
// immediately before CreateAuditEvent, returning a forced error. The
// withTxInheritKeys wrapper propagates the error out of fn, WithTx
// issues Rollback, and the DELETE is undone.
func TestDeleteAuditEventsBefore_AtomicWithTombstone(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("tombstone-atomic-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-atomic-rollback"
	base := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Hour)
	const n = 5
	seedDatedChain(ctx, t, q, projectID, n, base, key)

	forced := errors.New("forced tombstone failure")
	store.SetTombstoneInsertHookForTest(q, func(context.Context) error {
		return forced
	})
	t.Cleanup(func() { store.SetTombstoneInsertHookForTest(q, nil) })

	cutoff := base.Add(3 * time.Hour) // would delete indices 0..2.
	deleted, err := q.DeleteAuditEventsBefore(ctx, projectID, cutoff)
	require.Error(t, err)
	assert.True(t, errors.Is(err, forced))
	assert.EqualValues(t, 0, deleted)

	// All seeded rows must still be present.
	var remaining int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE project_id = $1`,

		projectID).Scan(&remaining))
	assert.Equal(t, n, remaining)

	// No tombstone row survived the rollback.
	var tombstones int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE project_id = $1 AND action = $2`,

		projectID, domain.AuditActionRetentionTrimmed,
	).Scan(&tombstones))
	assert.EqualValues(t, 0, tombstones)

}

// TestDeleteAuditEventsBeforeExcluding_AtomicWithTombstone — the bulk
// path trims per-project and writes one tombstone per affected project
// inside a single transaction. A forced tombstone failure on any project
// must roll back the entire batch: every project's rows must remain and
// no tombstones may be persisted anywhere.
func TestDeleteAuditEventsBeforeExcluding_AtomicWithTombstone(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("tombstone-bulk-atomic-secret")
	q.SetAuditSigningKey(key)

	base := time.Now().UTC().Add(-72 * time.Hour).Truncate(time.Hour)
	pA := "proj-bulk-atomic-a"
	pB := "proj-bulk-atomic-b"
	const perProject = 5
	seedDatedChain(ctx, t, q, pA, perProject, base, key)
	seedDatedChain(ctx, t, q, pB, perProject, base, key)

	forced := errors.New("forced tombstone failure (bulk)")
	store.SetTombstoneInsertHookForTest(q, func(context.Context) error {
		return forced
	})
	t.Cleanup(func() { store.SetTombstoneInsertHookForTest(q, nil) })

	cutoff := base.Add(3 * time.Hour) // would delete indices 0..2 per project.
	deleted, err := q.DeleteAuditEventsBeforeExcluding(ctx, cutoff, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, forced))
	assert.EqualValues(t, 0, deleted)

	for _, pid := range []string{pA, pB} {
		var remaining int
		require.NoError(t, testDB.
			Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM audit_events WHERE project_id = $1`,

			pid).Scan(&remaining))
		assert.Equal(t, perProject,

			remaining,
		)

		var tombstones int
		require.NoError(t, testDB.
			Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM audit_events WHERE project_id = $1 AND action = $2`,

			pid, domain.AuditActionRetentionTrimmed,
		).Scan(&tombstones))
		assert.EqualValues(t, 0, tombstones)

	}
}

// TestDeleteAuditEventsBefore_RejectsEmptyProjectID — calling the
// per-project trim with an empty projectID must fail loudly. This path
// used to silently fall through to a cross-tenant DELETE (every row
// older than cutoff, any project). That defeated per-tenant isolation
// and could wipe unrelated projects on a buggy call site. The bulk
// operation belongs to DeleteAuditEventsBeforeExcluding, which emits
// per-project tombstones; callers that want cross-tenant trim must
// reach for that entrypoint explicitly.
func TestDeleteAuditEventsBefore_RejectsEmptyProjectID(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("reject-empty-pid-secret")
	q.SetAuditSigningKey(key)

	// Seed one row in a real project so the table is non-empty. If the
	// old buggy path ever re-emerged it would delete this row; we rely
	// on that as a secondary safety check below.
	projectID := "proj-reject-empty-sentinel"
	base := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Hour)
	seedDatedChain(ctx, t, q, projectID, 1, base, key)

	cutoff := time.Now().UTC()
	deleted, err := q.DeleteAuditEventsBefore(ctx, "", cutoff)
	require.Error(t, err)
	assert.EqualValues(t, 0, deleted)

	// Sentinel: the seeded row must still be present. If the guard ever
	// regresses to the cross-tenant DELETE behavior, this row would be
	// wiped since its created_at is older than the cutoff.
	var remaining int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE project_id = $1`,

		projectID).Scan(&remaining))
	assert.EqualValues(t, 1, remaining)

}

// TestTombstoneDoesNotRaceWithRotation runs a tombstone-emitting trim and a
// signing-key rotation against the same project from two goroutines. The
// shared advisory lock (acquireProjectRotationLock) ensures one waits for the
// other; without it, the tombstone can read MAX(rotation_epoch) before the
// rotation commits but write its CreateAuditEvent after the new epoch lands —
// signing the tombstone under epoch N's key while the chain tail is in epoch
// N+1, leaving an unverifiable chain.
//
// Acceptance criteria: both operations complete without error and
// VerifyAuditChain reports the chain valid afterwards.
func TestTombstoneDoesNotRaceWithRotation(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	key, _ := store.DeriveAuditSigningKey("tombstone-rotation-race-secret")
	q.SetAuditSigningKey(key)

	projectID := "proj-tomb-vs-rotate"
	// Seed enough rows that the trim has work to do.
	base := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Hour)
	seedDatedChain(ctx, t, q, projectID, 12, base, key)
	cutoff := base.Add(6*time.Hour + 30*time.Minute)

	// Coordinate launch so both goroutines hit the advisory-lock contention
	// window. We don't need perfect simultaneity — the lock serializes them
	// either way; what we are asserting is that VerifyAuditChain remains
	// valid no matter who wins.
	type outcome struct {
		who string
		err error
	}
	results := make(chan outcome, 2)
	start := make(chan struct{})
	concWG.Go(func() {
		<-start
		_, err := q.DeleteAuditEventsBefore(ctx, projectID, cutoff)
		results <- outcome{who: "tombstone", err: err}
	})
	concWG.Go(func() {
		<-start
		_, err := q.RotateAuditSigningKey(ctx, projectID, "actor-race")
		results <- outcome{who: "rotation", err: err}
	})
	close(start)

	for range 2 {
		o := <-results
		require.Nil(t, o.
			err)

	}

	vc, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	require.True(t, vc.Valid)

	// Sanity: both forensic markers must exist exactly once.
	var tombstones, anchors int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*) FILTER (WHERE action = $1),
		       COUNT(*) FILTER (WHERE action = $2)
		FROM audit_events
		WHERE project_id = $3
	`,

		domain.AuditActionRetentionTrimmed, domain.AuditActionKeyRotated, projectID,
	).Scan(&tombstones, &anchors))
	assert.EqualValues(t, 1, tombstones)
	assert.EqualValues(t, 1, anchors)

}
