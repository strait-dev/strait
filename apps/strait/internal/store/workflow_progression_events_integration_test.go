//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"
)

func TestWorkflowProgressionEvents_ClaimAndAck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	q := mustStore(t)

	if err := q.CreateWorkflowProgressionEvent(ctx, "wf-run-1", "step-run-1", "step-a", "completed"); err != nil {
		t.Fatalf("CreateWorkflowProgressionEvent() error = %v", err)
	}
	if err := q.CreateWorkflowProgressionEvent(ctx, "wf-run-1", "step-run-1", "step-a", "completed"); err != nil {
		t.Fatalf("duplicate CreateWorkflowProgressionEvent() error = %v", err)
	}
	events, err := q.ClaimWorkflowProgressionEvents(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimWorkflowProgressionEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("ClaimWorkflowProgressionEvents() len = %d, want 1", len(events))
	}
	var xminAfterClaim string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM workflow_progression_events
		WHERE id = $1`,
		events[0].ID,
	).Scan(&xminAfterClaim); err != nil {
		t.Fatalf("query workflow progression event after claim: %v", err)
	}
	if err := q.MarkWorkflowProgressionEventProcessed(ctx, events[0].ID); err != nil {
		t.Fatalf("MarkWorkflowProgressionEventProcessed() error = %v", err)
	}
	var xminAfterFirstMark string
	var processedAt time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT wpe.xmin::text, processed.processed_at
		FROM workflow_progression_events wpe
		JOIN workflow_progression_event_processed processed ON processed.event_id = wpe.id
		WHERE wpe.id = $1`,
		events[0].ID,
	).Scan(&xminAfterFirstMark, &processedAt); err != nil {
		t.Fatalf("query workflow progression event after first mark: %v", err)
	}
	if processedAt.IsZero() {
		t.Fatal("processed_at is zero after first mark")
	}
	if xminAfterFirstMark != xminAfterClaim {
		t.Fatalf("processed mark changed event xmin from %s to %s", xminAfterClaim, xminAfterFirstMark)
	}
	var claimRows int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM workflow_progression_event_claims
		WHERE event_id = $1`,
		events[0].ID,
	).Scan(&claimRows); err != nil {
		t.Fatalf("query progression claim rows after mark: %v", err)
	}
	if claimRows != 0 {
		t.Fatalf("progression claim rows after mark = %d, want 0", claimRows)
	}
	if err := q.MarkWorkflowProgressionEventProcessed(ctx, events[0].ID); err != nil {
		t.Fatalf("duplicate MarkWorkflowProgressionEventProcessed() error = %v", err)
	}
	var xminAfterDuplicateMark string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM workflow_progression_events
		WHERE id = $1`,
		events[0].ID,
	).Scan(&xminAfterDuplicateMark); err != nil {
		t.Fatalf("query workflow progression event after duplicate mark: %v", err)
	}
	if xminAfterDuplicateMark != xminAfterFirstMark {
		t.Fatalf("duplicate processed mark changed xmin from %s to %s", xminAfterFirstMark, xminAfterDuplicateMark)
	}
	if err := q.MarkWorkflowProgressionEventsProcessed(ctx, []int64{events[0].ID}); err != nil {
		t.Fatalf("duplicate MarkWorkflowProgressionEventsProcessed() error = %v", err)
	}
	var xminAfterDuplicateBatchMark string
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text
		FROM workflow_progression_events
		WHERE id = $1`,
		events[0].ID,
	).Scan(&xminAfterDuplicateBatchMark); err != nil {
		t.Fatalf("query workflow progression event after duplicate batch mark: %v", err)
	}
	if xminAfterDuplicateBatchMark != xminAfterFirstMark {
		t.Fatalf("duplicate batch processed mark changed xmin from %s to %s", xminAfterFirstMark, xminAfterDuplicateBatchMark)
	}
	var processedRows int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM workflow_progression_event_processed
		WHERE event_id = $1`,
		events[0].ID,
	).Scan(&processedRows); err != nil {
		t.Fatalf("query processed side rows: %v", err)
	}
	if processedRows != 1 {
		t.Fatalf("processed side rows = %d, want 1", processedRows)
	}
	events, err = q.ClaimWorkflowProgressionEvents(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimWorkflowProgressionEvents() after ack error = %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("ClaimWorkflowProgressionEvents() after ack len = %d, want 0", len(events))
	}
}

func TestWorkflowProgressionEvents_ReleaseSkipsUnlockedEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	q := mustStore(t)

	if err := q.CreateWorkflowProgressionEvent(ctx, "wf-run-release", "step-run-release-single", "step-a", "completed"); err != nil {
		t.Fatalf("CreateWorkflowProgressionEvent(single) error = %v", err)
	}
	if err := q.CreateWorkflowProgressionEvent(ctx, "wf-run-release", "step-run-release-batch", "step-b", "completed"); err != nil {
		t.Fatalf("CreateWorkflowProgressionEvent(batch) error = %v", err)
	}

	eventID := func(stepRunID string) int64 {
		t.Helper()
		var id int64
		if err := testDB.Pool.QueryRow(ctx, `
			SELECT id
			FROM workflow_progression_events
			WHERE step_run_id = $1`,
			stepRunID,
		).Scan(&id); err != nil {
			t.Fatalf("query workflow progression event id for %s: %v", stepRunID, err)
		}
		return id
	}
	eventState := func(id int64) (string, bool) {
		t.Helper()
		var xmin string
		var locked bool
		if err := testDB.Pool.QueryRow(ctx, `
			SELECT wpe.xmin::text,
			       EXISTS (
			           SELECT 1
			           FROM workflow_progression_event_claims claim
			           WHERE claim.event_id = wpe.id
			       )
			FROM workflow_progression_events wpe
			WHERE wpe.id = $1`,
			id,
		).Scan(&xmin, &locked); err != nil {
			t.Fatalf("query workflow progression event state for %d: %v", id, err)
		}
		return xmin, locked
	}

	singleID := eventID("step-run-release-single")
	batchID := eventID("step-run-release-batch")
	singleXminBefore, singleLocked := eventState(singleID)
	if singleLocked {
		t.Fatal("single release event is locked before claim")
	}
	batchXminBefore, batchLocked := eventState(batchID)
	if batchLocked {
		t.Fatal("batch release event is locked before claim")
	}

	if err := q.ReleaseWorkflowProgressionEvent(ctx, singleID); err != nil {
		t.Fatalf("ReleaseWorkflowProgressionEvent(unlocked) error = %v", err)
	}
	if err := q.ReleaseWorkflowProgressionEvents(ctx, []int64{batchID}); err != nil {
		t.Fatalf("ReleaseWorkflowProgressionEvents(unlocked) error = %v", err)
	}

	singleXminAfterNoOp, singleLocked := eventState(singleID)
	if singleLocked {
		t.Fatal("single release event is locked after no-op release")
	}
	if singleXminAfterNoOp != singleXminBefore {
		t.Fatalf("single release changed unlocked event xmin from %s to %s", singleXminBefore, singleXminAfterNoOp)
	}
	batchXminAfterNoOp, batchLocked := eventState(batchID)
	if batchLocked {
		t.Fatal("batch release event is locked after no-op release")
	}
	if batchXminAfterNoOp != batchXminBefore {
		t.Fatalf("batch release changed unlocked event xmin from %s to %s", batchXminBefore, batchXminAfterNoOp)
	}

	events, err := q.ClaimWorkflowProgressionEvents(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimWorkflowProgressionEvents() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("ClaimWorkflowProgressionEvents() len = %d, want 2", len(events))
	}
	singleXminAfterClaim, singleLocked := eventState(singleID)
	if !singleLocked {
		t.Fatal("single release event is unlocked after claim")
	}
	if singleXminAfterClaim != singleXminBefore {
		t.Fatalf("single claim changed event xmin from %s to %s", singleXminBefore, singleXminAfterClaim)
	}
	batchXminAfterClaim, batchLocked := eventState(batchID)
	if !batchLocked {
		t.Fatal("batch release event is unlocked after claim")
	}
	if batchXminAfterClaim != batchXminBefore {
		t.Fatalf("batch claim changed event xmin from %s to %s", batchXminBefore, batchXminAfterClaim)
	}

	if err := q.ReleaseWorkflowProgressionEvent(ctx, singleID); err != nil {
		t.Fatalf("ReleaseWorkflowProgressionEvent(locked) error = %v", err)
	}
	if err := q.ReleaseWorkflowProgressionEvents(ctx, []int64{batchID}); err != nil {
		t.Fatalf("ReleaseWorkflowProgressionEvents(locked) error = %v", err)
	}

	singleXminAfterRelease, singleLocked := eventState(singleID)
	if singleLocked {
		t.Fatal("single release event is locked after release")
	}
	if singleXminAfterRelease != singleXminAfterClaim {
		t.Fatalf("single release changed event xmin from %s to %s", singleXminAfterClaim, singleXminAfterRelease)
	}
	batchXminAfterRelease, batchLocked := eventState(batchID)
	if batchLocked {
		t.Fatal("batch release event is locked after release")
	}
	if batchXminAfterRelease != batchXminAfterClaim {
		t.Fatalf("batch release changed event xmin from %s to %s", batchXminAfterClaim, batchXminAfterRelease)
	}
}

func TestWorkflowProgressionEvents_DeleteProcessedUsesSideStateAndLegacyState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mustClean(t, ctx)
	q := mustStore(t)

	if err := q.CreateWorkflowProgressionEvent(ctx, "wf-run-delete", "step-run-side", "step-a", "completed"); err != nil {
		t.Fatalf("CreateWorkflowProgressionEvent(side) error = %v", err)
	}
	sideEvents, err := q.ClaimWorkflowProgressionEvents(ctx, 1)
	if err != nil {
		t.Fatalf("ClaimWorkflowProgressionEvents(side) error = %v", err)
	}
	if len(sideEvents) != 1 {
		t.Fatalf("side claim len = %d, want 1", len(sideEvents))
	}
	if err := q.MarkWorkflowProgressionEventProcessed(ctx, sideEvents[0].ID); err != nil {
		t.Fatalf("MarkWorkflowProgressionEventProcessed(side) error = %v", err)
	}
	oldProcessedAt := time.Now().UTC().Add(-time.Hour)
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE workflow_progression_event_processed
		SET processed_at = $1
		WHERE event_id = $2`,
		oldProcessedAt,
		sideEvents[0].ID,
	); err != nil {
		t.Fatalf("age side processed event: %v", err)
	}

	if err := q.CreateWorkflowProgressionEvent(ctx, "wf-run-delete", "step-run-legacy", "step-b", "completed"); err != nil {
		t.Fatalf("CreateWorkflowProgressionEvent(legacy) error = %v", err)
	}
	var legacyID int64
	if err := testDB.Pool.QueryRow(ctx, `
		UPDATE workflow_progression_events
		SET processed_at = $1
		WHERE step_run_id = 'step-run-legacy'
		RETURNING id`,
		oldProcessedAt,
	).Scan(&legacyID); err != nil {
		t.Fatalf("mark legacy processed event: %v", err)
	}

	deleted, err := q.DeleteProcessedWorkflowProgressionEvents(ctx, 30*time.Minute, 10)
	if err != nil {
		t.Fatalf("DeleteProcessedWorkflowProgressionEvents() error = %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted events = %d, want 2", deleted)
	}

	var remainingEvents, remainingSideRows int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM workflow_progression_events
		WHERE id = ANY($1)`,
		[]int64{sideEvents[0].ID, legacyID},
	).Scan(&remainingEvents); err != nil {
		t.Fatalf("query remaining progression events: %v", err)
	}
	if remainingEvents != 0 {
		t.Fatalf("remaining progression events = %d, want 0", remainingEvents)
	}
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM workflow_progression_event_processed
		WHERE event_id = $1`,
		sideEvents[0].ID,
	).Scan(&remainingSideRows); err != nil {
		t.Fatalf("query remaining side rows: %v", err)
	}
	if remainingSideRows != 0 {
		t.Fatalf("remaining processed side rows = %d, want 0 cascade cleanup", remainingSideRows)
	}
}
