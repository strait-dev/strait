//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	require.True(t, result.
		Valid,
	)
	assert.EqualValues(t, 5, result.
		EventsChecked,
	)
	assert.True(t, result.Incremental)

	cp, err := q.GetAuditChainCheckpoint(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, cp)
	assert.Equal(t, result.
		LastEventID,

		cp.LastVerifiedEventID,
	)

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
	require.NoError(t, err)
	require.False(t, !first.
		Valid ||
		first.EventsChecked !=

			10)

	// Second verify with no new writes still revalidates the surviving prefix.
	second, err := q.VerifyAuditChainIncremental(ctx, projectID)
	require.NoError(t, err)
	assert.True(t, second.Valid)
	assert.EqualValues(t, 10, second.
		EventsChecked,
	)
	assert.True(t, second.Incremental)

	// Append 3 new events, then verify: the whole surviving chain is checked.
	insertTestChain(ctx, t, q, projectID, 3)

	third, err := q.VerifyAuditChainIncremental(ctx, projectID)
	require.NoError(t, err)
	assert.True(t, third.Valid)
	assert.EqualValues(t, 13, third.
		EventsChecked,
	)

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
		require.Failf(t, "test failure",

			"initial verify: %v", err)
	}

	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE audit_events SET signature = 'deadbeefdeadbeefdeadbeefdeadbeef' WHERE id = $1`,
		ids[1],
	); err != nil {
		require.Failf(t, "test failure",

			"tamper historical row: %v", err)
	}

	result, err := q.VerifyAuditChainIncremental(ctx, projectID)
	require.NoError(t, err)
	require.False(t, result.
		Valid)
	require.Equal(t, ids[1],

		result.BrokenAtID,
	)

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
		require.Failf(t, "test failure",

			"initial verify: %v", err)
	}
	cpBefore, err := q.GetAuditChainCheckpoint(ctx, projectID)
	require.False(t, err !=

		nil || cpBefore ==
		nil,
	)

	// Append 2 more events, then tamper with the second one's signature.
	newIDs := insertTestChain(ctx, t, q, projectID, 2)
	tamperedID := newIDs[1]
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE audit_events SET signature = 'deadbeefdeadbeefdeadbeefdeadbeef' WHERE id = $1`,
		tamperedID,
	); err != nil {
		require.Failf(t, "test failure",

			"tamper update: %v", err)
	}

	result, err := q.VerifyAuditChainIncremental(ctx, projectID)
	require.NoError(t, err)
	assert.False(t, result.
		Valid,
	)
	assert.Equal(t, tamperedID,

		result.
			BrokenAtID,
	)

	// Checkpoint must NOT have advanced past the original anchor.
	cpAfter, err := q.GetAuditChainCheckpoint(ctx, projectID)
	require.False(t, err !=

		nil || cpAfter ==
		nil,
	)
	assert.Equal(t, cpBefore.
		LastVerifiedEventID,

		cpAfter.LastVerifiedEventID,
	)

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
		require.Failf(t, "test failure",

			"initial verify: %v", err)
	}
	cp, err := q.GetAuditChainCheckpoint(ctx, projectID)
	require.False(t, err !=

		nil || cp ==
		nil)

	// Simulate retention trimming the checkpointed event away.
	if _, err := testDB.Pool.Exec(ctx,
		`DELETE FROM audit_events WHERE id = $1`, cp.LastVerifiedEventID,
	); err != nil {
		require.Failf(t, "test failure",

			"simulate retention trim: %v", err)
	}

	// Incremental verify must not crash on the missing anchor — it must
	// fall back to a full verify over the surviving rows.
	result, err := q.VerifyAuditChainIncremental(ctx, projectID)
	require.NoError(t, err)
	assert.True(t, result.Valid)
	assert.True(t, result.Incremental)

}
