//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWorkflowProgressionEvents_ClaimAndAck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	q := mustStore(t)
	require.NoError(t, q.CreateWorkflowProgressionEvent(ctx,
		"wf-run-1",

		"step-run-1", "step-a", "completed"))
	require.NoError(t, q.CreateWorkflowProgressionEvent(ctx,
		"wf-run-1",

		"step-run-1", "step-a", "completed"))

	events, err := q.ClaimWorkflowProgressionEvents(ctx, 10)
	require.NoError(t, err)
	require.Len(t, events,
		1,
	)

	var xminAfterClaim string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM workflow_progression_events
		WHERE id = $1`,
		events[0].
			ID).Scan(&xminAfterClaim))
	require.NoError(t, q.MarkWorkflowProgressionEventProcessed(ctx,
		events[0].ID))

	var xminAfterFirstMark string
	var processedAt time.Time
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT wpe.xmin::text, processed.processed_at
		FROM workflow_progression_events wpe
		JOIN workflow_progression_event_processed processed ON processed.event_id = wpe.id
		WHERE wpe.id = $1`,

		events[0].ID).Scan(&xminAfterFirstMark, &processedAt))
	require.False(t, processedAt.
		IsZero())
	require.Equal(t, xminAfterClaim,

		xminAfterFirstMark,
	)

	var claimRows int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM workflow_progression_event_claims
		WHERE event_id = $1`,

		events[0].ID).Scan(&claimRows))
	require.EqualValues(t, 0, claimRows)
	require.NoError(t, q.MarkWorkflowProgressionEventProcessed(ctx,
		events[0].ID))

	var xminAfterDuplicateMark string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM workflow_progression_events
		WHERE id = $1`,
		events[0].
			ID).Scan(&xminAfterDuplicateMark))
	require.Equal(t, xminAfterFirstMark,

		xminAfterDuplicateMark,
	)
	require.NoError(t, q.MarkWorkflowProgressionEventsProcessed(ctx,

		[]int64{events[0].ID}))

	var xminAfterDuplicateBatchMark string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM workflow_progression_events
		WHERE id = $1`,
		events[0].
			ID).Scan(&xminAfterDuplicateBatchMark))
	require.Equal(t, xminAfterFirstMark,

		xminAfterDuplicateBatchMark,
	)

	var processedRows int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM workflow_progression_event_processed
		WHERE event_id = $1`,

		events[0].ID).Scan(&processedRows))
	require.EqualValues(t, 1, processedRows)

	events, err = q.ClaimWorkflowProgressionEvents(ctx, 10)
	require.NoError(t, err)
	require.Len(t, events,
		0,
	)

}

func TestWorkflowProgressionEvents_ReleaseSkipsUnlockedEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	q := mustStore(t)
	require.NoError(t, q.CreateWorkflowProgressionEvent(ctx,
		"wf-run-release",

		"step-run-release-single", "step-a", "completed"))
	require.NoError(t, q.CreateWorkflowProgressionEvent(ctx,
		"wf-run-release",

		"step-run-release-batch", "step-b", "completed"))

	eventID := func(stepRunID string) int64 {
		t.Helper()
		var id int64
		require.NoError(t, testDB.
			Pool.QueryRow(ctx,
			`
			SELECT id
			FROM workflow_progression_events
			WHERE step_run_id = $1`,
			stepRunID,
		).Scan(&id))

		return id
	}
	eventState := func(id int64) (string, bool) {
		t.Helper()
		var xmin string
		var locked bool
		require.NoError(t, testDB.
			Pool.QueryRow(ctx,
			`
			SELECT wpe.xmin::text,
			       EXISTS (
			           SELECT 1
			           FROM workflow_progression_event_claims claim
			           WHERE claim.event_id = wpe.id
			       )
			FROM workflow_progression_events wpe
			WHERE wpe.id = $1`,

			id).Scan(&xmin, &locked))

		return xmin, locked
	}

	singleID := eventID("step-run-release-single")
	batchID := eventID("step-run-release-batch")
	singleXminBefore, singleLocked := eventState(singleID)
	require.False(t, singleLocked)

	batchXminBefore, batchLocked := eventState(batchID)
	require.False(t, batchLocked)
	require.NoError(t, q.ReleaseWorkflowProgressionEvent(ctx,
		singleID,
	))
	require.NoError(t, q.ReleaseWorkflowProgressionEvents(ctx,
		[]int64{batchID}))

	singleXminAfterNoOp, singleLocked := eventState(singleID)
	require.False(t, singleLocked)
	require.Equal(t, singleXminBefore,

		singleXminAfterNoOp,
	)

	batchXminAfterNoOp, batchLocked := eventState(batchID)
	require.False(t, batchLocked)
	require.Equal(t, batchXminBefore,

		batchXminAfterNoOp,
	)

	events, err := q.ClaimWorkflowProgressionEvents(ctx, 10)
	require.NoError(t, err)
	require.Len(t, events,
		2,
	)

	singleXminAfterClaim, singleLocked := eventState(singleID)
	require.True(t, singleLocked)
	require.Equal(t, singleXminBefore,

		singleXminAfterClaim,
	)

	batchXminAfterClaim, batchLocked := eventState(batchID)
	require.True(t, batchLocked)
	require.Equal(t, batchXminBefore,

		batchXminAfterClaim,
	)
	require.NoError(t, q.ReleaseWorkflowProgressionEvent(ctx,
		singleID,
	))
	require.NoError(t, q.ReleaseWorkflowProgressionEvents(ctx,
		[]int64{batchID}))

	singleXminAfterRelease, singleLocked := eventState(singleID)
	require.False(t, singleLocked)
	require.Equal(t, singleXminAfterClaim,

		singleXminAfterRelease,
	)

	batchXminAfterRelease, batchLocked := eventState(batchID)
	require.False(t, batchLocked)
	require.Equal(t, batchXminAfterClaim,

		batchXminAfterRelease,
	)

}

func TestWorkflowProgressionEvents_StaleSideClaimIsReclaimed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	q := mustStore(t)
	require.NoError(t, q.CreateWorkflowProgressionEvent(ctx,
		"wf-run-stale",

		"step-run-stale", "step-a", "completed"))

	events, err := q.ClaimWorkflowProgressionEvents(ctx, 1)
	require.NoError(t, err)
	require.Len(t, events,
		1,
	)

	var xminAfterClaim string
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM workflow_progression_events
		WHERE id = $1`,
		events[0].
			ID).Scan(&xminAfterClaim))

	staleLockedAt := time.Now().UTC().Add(-time.Minute)
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE workflow_progression_event_claims
		SET locked_at = $1
		WHERE event_id = $2`,
		staleLockedAt,
		events[0].ID,
	); err != nil {
		require.Failf(t, "test failure",

			"age progression claim: %v", err)
	}

	reclaimed, err := q.ClaimWorkflowProgressionEvents(ctx, 1)
	require.NoError(t, err)
	require.False(t, len(reclaimed) !=
		1 || reclaimed[0].ID !=
		events[0].ID)

	var xminAfterReclaim string
	var claimRows int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT wpe.xmin::text,
		       (
		           SELECT COUNT(*)
		           FROM workflow_progression_event_claims claim
		           WHERE claim.event_id = wpe.id
		       )
		FROM workflow_progression_events wpe
		WHERE wpe.id = $1`,

		events[0].ID).Scan(&xminAfterReclaim, &claimRows))
	require.Equal(t, xminAfterClaim,

		xminAfterReclaim,
	)
	require.EqualValues(t, 1, claimRows)

}

func TestWorkflowProgressionEvents_DeleteProcessedUsesSideStateAndLegacyState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	q := mustStore(t)
	require.NoError(t, q.CreateWorkflowProgressionEvent(ctx,
		"wf-run-delete",

		"step-run-side", "step-a", "completed"))

	sideEvents, err := q.ClaimWorkflowProgressionEvents(ctx, 1)
	require.NoError(t, err)
	require.Len(t, sideEvents,

		1)
	require.NoError(t, q.MarkWorkflowProgressionEventProcessed(ctx,
		sideEvents[0].ID))

	oldProcessedAt := time.Now().UTC().Add(-time.Hour)
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE workflow_progression_event_processed
		SET processed_at = $1
		WHERE event_id = $2`,
		oldProcessedAt,
		sideEvents[0].ID,
	); err != nil {
		require.Failf(t, "test failure",

			"age side processed event: %v", err)
	}
	require.NoError(t, q.CreateWorkflowProgressionEvent(ctx,
		"wf-run-delete",

		"step-run-legacy", "step-b", "completed"))

	var legacyID int64
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		UPDATE workflow_progression_events
		SET processed_at = $1
		WHERE step_run_id = 'step-run-legacy'
		RETURNING id`,
		oldProcessedAt).Scan(&legacyID))

	deleted, err := q.DeleteProcessedWorkflowProgressionEvents(ctx, 30*time.Minute, 10)
	require.NoError(t, err)
	require.EqualValues(t, 2, deleted)

	var remainingEvents, remainingSideRows int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM workflow_progression_events
		WHERE id = ANY($1)`,
		[]int64{sideEvents[0].ID, legacyID}).Scan(&remainingEvents))
	require.EqualValues(t, 0, remainingEvents)
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT COUNT(*)
		FROM workflow_progression_event_processed
		WHERE event_id = $1`,

		sideEvents[0].ID).Scan(&remainingSideRows))
	require.EqualValues(t, 0, remainingSideRows)

}
