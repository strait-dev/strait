//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/store"
)

// TestVerifyAuditChainIncremental_ColdPath asserts the first-ever
// incremental call delegates to a full verify, sets Incremental=true on
// the result, and plants a checkpoint so subsequent calls take the fast
// path.
func TestVerifyAuditChainIncremental_ColdPath(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	key, _ := store.DeriveAuditSigningKey("incremental-cold")
	q.SetAuditSigningKey(key)

	projectID := "proj-incremental-cold"
	insertTestChain(ctx, t, q, projectID, 5)

	result, err := q.VerifyAuditChainIncremental(ctx, projectID)
	if err != nil {
		t.Fatalf("VerifyAuditChainIncremental: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected Valid=true on pristine chain, got error=%q", result.Error)
	}
	if result.EventsChecked != 5 {
		t.Errorf("EventsChecked = %d, want 5 on cold-path full scan", result.EventsChecked)
	}
	if !result.Incremental {
		t.Error("Incremental = false on cold-path result; handler must know this was a checkpoint-tracked call")
	}

	cp, err := q.GetAuditChainCheckpoint(ctx, projectID)
	if err != nil {
		t.Fatalf("GetAuditChainCheckpoint: %v", err)
	}
	if cp == nil {
		t.Fatal("checkpoint was not planted after successful cold-path verify")
	}
	if cp.LastVerifiedEventID != result.LastEventID {
		t.Errorf("checkpoint event id = %q, want %q (tail of the scan)", cp.LastVerifiedEventID, result.LastEventID)
	}
}

// TestVerifyAuditChainIncremental_WarmPath_RevalidatesPrefix asserts the
// second (and subsequent) incremental verifies re-check the full surviving
// chain before refreshing the checkpoint. The checkpoint is a cursor, not a
// trust root.
func TestVerifyAuditChainIncremental_WarmPath_RevalidatesPrefix(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	key, _ := store.DeriveAuditSigningKey("incremental-warm")
	q.SetAuditSigningKey(key)

	projectID := "proj-incremental-warm"
	insertTestChain(ctx, t, q, projectID, 10)

	// First verify: cold path, scans all 10.
	first, err := q.VerifyAuditChainIncremental(ctx, projectID)
	if err != nil {
		t.Fatalf("first verify: %v", err)
	}
	if !first.Valid || first.EventsChecked != 10 {
		t.Fatalf("first verify: Valid=%v EventsChecked=%d (want 10 valid)", first.Valid, first.EventsChecked)
	}

	// Second verify with no new writes still revalidates the surviving prefix.
	second, err := q.VerifyAuditChainIncremental(ctx, projectID)
	if err != nil {
		t.Fatalf("second verify: %v", err)
	}
	if !second.Valid {
		t.Errorf("second verify: Valid = false on idle tail; error=%q", second.Error)
	}
	if second.EventsChecked != 10 {
		t.Errorf("second verify: EventsChecked = %d, want 10 on full prefix revalidation", second.EventsChecked)
	}
	if !second.Incremental {
		t.Error("second verify: Incremental = false")
	}

	// Append 3 new events, then verify: the whole surviving chain is checked.
	insertTestChain(ctx, t, q, projectID, 3)

	third, err := q.VerifyAuditChainIncremental(ctx, projectID)
	if err != nil {
		t.Fatalf("third verify: %v", err)
	}
	if !third.Valid {
		t.Errorf("third verify: Valid = false; error=%q", third.Error)
	}
	if third.EventsChecked != 13 {
		t.Errorf("third verify: EventsChecked = %d, want 13 (full chain)", third.EventsChecked)
	}
}

func TestVerifyAuditChainIncremental_HistoricalTamperBeforeCheckpoint(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	key, _ := store.DeriveAuditSigningKey("incremental-prefix-tamper")
	q.SetAuditSigningKey(key)

	projectID := "proj-incremental-prefix-tamper"
	ids := insertTestChain(ctx, t, q, projectID, 5)

	if _, err := q.VerifyAuditChainIncremental(ctx, projectID); err != nil {
		t.Fatalf("initial verify: %v", err)
	}

	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE audit_events SET signature = 'deadbeefdeadbeefdeadbeefdeadbeef' WHERE id = $1`,
		ids[1],
	); err != nil {
		t.Fatalf("tamper historical row: %v", err)
	}

	result, err := q.VerifyAuditChainIncremental(ctx, projectID)
	if err != nil {
		t.Fatalf("historical tamper verify: %v", err)
	}
	if result.Valid {
		t.Fatal("incremental verify returned valid after pre-checkpoint tamper")
	}
	if result.BrokenAtID != ids[1] {
		t.Fatalf("BrokenAtID = %q, want historical tampered row %q", result.BrokenAtID, ids[1])
	}
}

// TestVerifyAuditChainIncremental_TailTampered asserts a post-checkpoint
// tamper is still caught by an incremental verify: the new event's
// signature doesn't match the key and the result is Valid=false with
// the correct BrokenAtID. Crucially, the checkpoint is NOT advanced on
// a failed incremental — a subsequent retry must reproduce the failure.
func TestVerifyAuditChainIncremental_TailTampered(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	key, _ := store.DeriveAuditSigningKey("incremental-tamper")
	q.SetAuditSigningKey(key)

	projectID := "proj-incremental-tamper"
	insertTestChain(ctx, t, q, projectID, 3)

	// First verify anchors the checkpoint at event 3.
	if _, err := q.VerifyAuditChainIncremental(ctx, projectID); err != nil {
		t.Fatalf("initial verify: %v", err)
	}
	cpBefore, err := q.GetAuditChainCheckpoint(ctx, projectID)
	if err != nil || cpBefore == nil {
		t.Fatalf("initial checkpoint missing: err=%v cp=%v", err, cpBefore)
	}

	// Append 2 more events, then tamper with the second one's signature.
	newIDs := insertTestChain(ctx, t, q, projectID, 2)
	tamperedID := newIDs[1]
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE audit_events SET signature = 'deadbeefdeadbeefdeadbeefdeadbeef' WHERE id = $1`,
		tamperedID,
	); err != nil {
		t.Fatalf("tamper update: %v", err)
	}

	result, err := q.VerifyAuditChainIncremental(ctx, projectID)
	if err != nil {
		t.Fatalf("tampered verify: %v", err)
	}
	if result.Valid {
		t.Errorf("expected Valid=false on tampered tail")
	}
	if result.BrokenAtID != tamperedID {
		t.Errorf("BrokenAtID = %q, want %q (tampered event)", result.BrokenAtID, tamperedID)
	}

	// Checkpoint must NOT have advanced past the original anchor.
	cpAfter, err := q.GetAuditChainCheckpoint(ctx, projectID)
	if err != nil || cpAfter == nil {
		t.Fatalf("post-failure checkpoint missing: err=%v cp=%v", err, cpAfter)
	}
	if cpAfter.LastVerifiedEventID != cpBefore.LastVerifiedEventID {
		t.Errorf("checkpoint advanced after failed verify: before=%q after=%q", cpBefore.LastVerifiedEventID, cpAfter.LastVerifiedEventID)
	}
}

// TestVerifyAuditChainIncremental_CheckpointTrimmed asserts the
// incremental path falls back to a full verify when the checkpointed
// event has been retention-trimmed, rather than erroring on the
// missing anchor. After the fallback it re-plants the checkpoint at
// the surviving tail.
func TestVerifyAuditChainIncremental_CheckpointTrimmed(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	q.SetSecretEncryptionKey(testEncKey)
	key, _ := store.DeriveAuditSigningKey("incremental-trim")
	q.SetAuditSigningKey(key)

	projectID := "proj-incremental-trim"
	insertTestChain(ctx, t, q, projectID, 5)

	if _, err := q.VerifyAuditChainIncremental(ctx, projectID); err != nil {
		t.Fatalf("initial verify: %v", err)
	}
	cp, err := q.GetAuditChainCheckpoint(ctx, projectID)
	if err != nil || cp == nil {
		t.Fatalf("initial checkpoint missing")
	}

	// Simulate retention trimming the checkpointed event away.
	if _, err := testDB.Pool.Exec(ctx,
		`DELETE FROM audit_events WHERE id = $1`, cp.LastVerifiedEventID,
	); err != nil {
		t.Fatalf("simulate retention trim: %v", err)
	}

	// Incremental verify must not crash on the missing anchor — it must
	// fall back to a full verify over the surviving rows.
	result, err := q.VerifyAuditChainIncremental(ctx, projectID)
	if err != nil {
		t.Fatalf("verify after trim: %v", err)
	}
	if !result.Valid {
		t.Errorf("expected Valid=true over surviving rows; got error=%q", result.Error)
	}
	if !result.Incremental {
		t.Error("result Incremental = false after fallback; handler must know this came from the incremental API")
	}
}
