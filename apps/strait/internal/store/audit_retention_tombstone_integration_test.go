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
)

// seedDatedChain inserts n events into projectID spaced 1 hour apart, with
// the earliest event anchored at base. Returns the inserted event ids in
// chronological order. We manually update created_at AFTER CreateAuditEvent
// so each row is aged predictably; because the HMAC signature is bound to
// created_at, we must recompute + persist it under the same signing key.
func seedDatedChain(ctx context.Context, t *testing.T, q *store.Queries, projectID string, n int, base time.Time, signingKey []byte) []string {
	t.Helper()
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ev := &domain.AuditEvent{
			ProjectID:    projectID,
			ActorID:      "actor",
			ActorType:    "user",
			Action:       domain.AuditActionJobCreated,
			ResourceType: "job",
			ResourceID:   "job",
			Details:      json.RawMessage(`{"i":` + itoaBench(i) + `}`),
		}
		if err := q.CreateAuditEvent(ctx, ev); err != nil {
			t.Fatalf("CreateAuditEvent iter %d: %v", i, err)
		}
		// Backdate this row to base + i hours and re-sign under the active key.
		newCreatedAt := base.Add(time.Duration(i) * time.Hour).UTC().Truncate(time.Microsecond)
		ev.CreatedAt = newCreatedAt
		sig := store.ComputeAuditSignature(ev, signingKey)
		if _, err := testDB.Pool.Exec(ctx,
			`UPDATE audit_events SET created_at = $1, signature = $2 WHERE id = $3`,
			newCreatedAt, sig, ev.ID,
		); err != nil {
			t.Fatalf("backdate+resign iter %d: %v", i, err)
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
	if err != nil {
		t.Fatalf("DeleteAuditEventsBefore: %v", err)
	}
	if deleted != 6 {
		t.Fatalf("deleted = %d, want 6 (indices 0..5)", deleted)
	}

	// Exactly one tombstone exists.
	events, err := q.ListAuditEvents(ctx, projectID, "", "", "", 1000, nil, nil, nil, true)
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	var tombstones []domain.AuditEvent
	for _, ev := range events {
		if ev.Action == domain.AuditActionRetentionTrimmed {
			tombstones = append(tombstones, ev)
		}
	}
	if len(tombstones) != 1 {
		t.Fatalf("expected 1 tombstone, got %d", len(tombstones))
	}
	ts := tombstones[0]
	if !ts.IsAnchor {
		t.Error("tombstone is_anchor = false")
	}

	var details map[string]any
	if err := json.Unmarshal(ts.Details, &details); err != nil {
		t.Fatalf("unmarshal tombstone details: %v", err)
	}
	if got, want := details["deleted_count"], float64(6); got != want {
		t.Errorf("deleted_count = %v, want %v", got, want)
	}
	if _, ok := details["trimmed_before"].(string); !ok {
		t.Errorf("trimmed_before not a string: %v", details["trimmed_before"])
	}
	prevHash, _ := details["previous_hash"].(string)
	if prevHash == "" {
		t.Error("previous_hash missing from tombstone details")
	}

	// The tombstone's own previous_hash must match the surviving tail
	// (i.e. the signature of the last pre-tombstone event).
	var tailSig string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT signature FROM audit_events
		WHERE project_id = $1 AND action = $2
		ORDER BY created_at DESC LIMIT 1
	`, projectID, domain.AuditActionJobCreated).Scan(&tailSig); err != nil {
		t.Fatalf("query surviving tail: %v", err)
	}
	if ts.PreviousHash != tailSig {
		t.Errorf("tombstone previous_hash = %q, want surviving tail %q", ts.PreviousHash, tailSig)
	}
	if prevHash != tailSig {
		t.Errorf("tombstone details.previous_hash = %q, want surviving tail %q", prevHash, tailSig)
	}
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
	if err != nil {
		t.Fatalf("DeleteAuditEventsBefore: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}

	var tombstoneCount int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_events
		WHERE project_id = $1 AND action = $2
	`, projectID, domain.AuditActionRetentionTrimmed).Scan(&tombstoneCount); err != nil {
		t.Fatalf("count tombstones: %v", err)
	}
	if tombstoneCount != 0 {
		t.Errorf("expected 0 tombstones, got %d", tombstoneCount)
	}
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
		t.Fatalf("DeleteAuditEventsBefore: %v", err)
	}

	// Add a few post-trim events to ensure the chain continues past the
	// tombstone cleanly.
	for i := 0; i < 3; i++ {
		ev := &domain.AuditEvent{
			ProjectID:    projectID,
			ActorID:      "actor",
			ActorType:    "user",
			Action:       domain.AuditActionJobUpdated,
			ResourceType: "job",
			ResourceID:   "job",
			Details:      json.RawMessage(`{"changes":"x"}`),
		}
		if err := q.CreateAuditEvent(ctx, ev); err != nil {
			t.Fatalf("post-trim CreateAuditEvent %d: %v", i, err)
		}
	}

	result, err := q.VerifyAuditChain(ctx, projectID)
	if err != nil {
		t.Fatalf("VerifyAuditChain: %v", err)
	}
	if !result.Valid {
		t.Fatalf("chain invalid: %s (broken at %q)", result.Error, result.BrokenAtID)
	}
	// 4 surviving + 1 tombstone + 3 post = 8.
	if result.EventsChecked != 8 {
		t.Errorf("EventsChecked = %d, want 8", result.EventsChecked)
	}
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
	if err != nil {
		t.Fatalf("DeleteAuditEventsBeforeExcluding: %v", err)
	}
	if deleted != 6 {
		t.Errorf("deleted = %d, want 6 (3 each in A and B)", deleted)
	}

	countTombstones := func(pid string) int {
		var n int
		if err := testDB.Pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM audit_events
			WHERE project_id = $1 AND action = $2
		`, pid, domain.AuditActionRetentionTrimmed).Scan(&n); err != nil {
			t.Fatalf("count tombstones for %s: %v", pid, err)
		}
		return n
	}

	if got := countTombstones(pA); got != 1 {
		t.Errorf("project A tombstones = %d, want 1", got)
	}
	if got := countTombstones(pB); got != 1 {
		t.Errorf("project B tombstones = %d, want 1", got)
	}
	if got := countTombstones(pC); got != 0 {
		t.Errorf("excluded project C tombstones = %d, want 0", got)
	}

	// Both affected projects still verify.
	if v, err := q.VerifyAuditChain(ctx, pA); err != nil || !v.Valid {
		t.Errorf("chain A invalid after bulk trim: err=%v valid=%v error=%s", err, v.Valid, v.Error)
	}
	if v, err := q.VerifyAuditChain(ctx, pB); err != nil || !v.Valid {
		t.Errorf("chain B invalid after bulk trim: err=%v valid=%v error=%s", err, v.Valid, v.Error)
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
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE project_id = $1`, projectID,
	).Scan(&pre); err != nil {
		t.Fatalf("pre count: %v", err)
	}
	if pre != 5 {
		t.Fatalf("pre = %d, want 5", pre)
	}

	cutoff := base.Add(3 * time.Hour)
	deleted, err := q.DeleteAuditEventsBefore(ctx, projectID, cutoff)
	if err != nil {
		t.Fatalf("DeleteAuditEventsBefore: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("deleted = %d, want 3", deleted)
	}

	var post int
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE project_id = $1`, projectID,
	).Scan(&post); err != nil {
		t.Fatalf("post count: %v", err)
	}
	// 5 - 3 (deleted) + 1 (tombstone) = 3.
	if post != 3 {
		t.Errorf("post = %d, want 3 (2 survivors + 1 tombstone)", post)
	}
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
	if err == nil {
		t.Fatalf("DeleteAuditEventsBefore: expected error, got nil (deleted=%d)", deleted)
	}
	if !errors.Is(err, forced) {
		t.Errorf("err = %v, want wrap of forced sentinel", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 on rollback", deleted)
	}

	// All seeded rows must still be present.
	var remaining int
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE project_id = $1`, projectID,
	).Scan(&remaining); err != nil {
		t.Fatalf("remaining count: %v", err)
	}
	if remaining != n {
		t.Errorf("remaining = %d, want %d (DELETE was not rolled back)", remaining, n)
	}

	// No tombstone row survived the rollback.
	var tombstones int
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE project_id = $1 AND action = $2`,
		projectID, domain.AuditActionRetentionTrimmed,
	).Scan(&tombstones); err != nil {
		t.Fatalf("tombstone count: %v", err)
	}
	if tombstones != 0 {
		t.Errorf("tombstones = %d, want 0 on rollback", tombstones)
	}
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
	if err == nil {
		t.Fatalf("DeleteAuditEventsBeforeExcluding: expected error, got nil (deleted=%d)", deleted)
	}
	if !errors.Is(err, forced) {
		t.Errorf("err = %v, want wrap of forced sentinel", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 on rollback", deleted)
	}

	for _, pid := range []string{pA, pB} {
		var remaining int
		if err := testDB.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM audit_events WHERE project_id = $1`, pid,
		).Scan(&remaining); err != nil {
			t.Fatalf("remaining count (%s): %v", pid, err)
		}
		if remaining != perProject {
			t.Errorf("project %s remaining = %d, want %d", pid, remaining, perProject)
		}

		var tombstones int
		if err := testDB.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM audit_events WHERE project_id = $1 AND action = $2`,
			pid, domain.AuditActionRetentionTrimmed,
		).Scan(&tombstones); err != nil {
			t.Fatalf("tombstone count (%s): %v", pid, err)
		}
		if tombstones != 0 {
			t.Errorf("project %s tombstones = %d, want 0 on rollback", pid, tombstones)
		}
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
	if err == nil {
		t.Fatalf("DeleteAuditEventsBefore(\"\", ...): expected error, got nil (deleted=%d)", deleted)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 on rejection", deleted)
	}

	// Sentinel: the seeded row must still be present. If the guard ever
	// regresses to the cross-tenant DELETE behavior, this row would be
	// wiped since its created_at is older than the cutoff.
	var remaining int
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE project_id = $1`, projectID,
	).Scan(&remaining); err != nil {
		t.Fatalf("remaining count: %v", err)
	}
	if remaining != 1 {
		t.Errorf("sentinel row was wiped: remaining = %d, want 1", remaining)
	}
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

	go func() {
		<-start
		_, err := q.DeleteAuditEventsBefore(ctx, projectID, cutoff)
		results <- outcome{who: "tombstone", err: err}
	}()
	go func() {
		<-start
		_, err := q.RotateAuditSigningKey(ctx, projectID, "actor-race")
		results <- outcome{who: "rotation", err: err}
	}()
	close(start)

	for i := 0; i < 2; i++ {
		o := <-results
		if o.err != nil {
			t.Fatalf("%s: %v", o.who, o.err)
		}
	}

	vc, err := q.VerifyAuditChain(ctx, projectID)
	if err != nil {
		t.Fatalf("VerifyAuditChain: %v", err)
	}
	if !vc.Valid {
		t.Fatalf("chain invalid after tombstone+rotation race: %s", vc.Error)
	}

	// Sanity: both forensic markers must exist exactly once.
	var tombstones, anchors int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE action = $1),
		       COUNT(*) FILTER (WHERE action = $2)
		FROM audit_events
		WHERE project_id = $3
	`, domain.AuditActionRetentionTrimmed, domain.AuditActionKeyRotated, projectID).Scan(&tombstones, &anchors); err != nil {
		t.Fatalf("count markers: %v", err)
	}
	if tombstones != 1 {
		t.Errorf("tombstone count = %d, want 1", tombstones)
	}
	if anchors != 1 {
		t.Errorf("rotation anchor count = %d, want 1", anchors)
	}
}
