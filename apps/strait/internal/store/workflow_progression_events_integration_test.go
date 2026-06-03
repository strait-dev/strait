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
	if err := q.MarkWorkflowProgressionEventProcessed(ctx, events[0].ID); err != nil {
		t.Fatalf("MarkWorkflowProgressionEventProcessed() error = %v", err)
	}
	var xminAfterFirstMark string
	var processedAt time.Time
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT xmin::text, processed_at
		FROM workflow_progression_events
		WHERE id = $1`,
		events[0].ID,
	).Scan(&xminAfterFirstMark, &processedAt); err != nil {
		t.Fatalf("query workflow progression event after first mark: %v", err)
	}
	if processedAt.IsZero() {
		t.Fatal("processed_at is zero after first mark")
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
			SELECT xmin::text, locked_at IS NOT NULL
			FROM workflow_progression_events
			WHERE id = $1`,
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
	batchXminAfterClaim, batchLocked := eventState(batchID)
	if !batchLocked {
		t.Fatal("batch release event is unlocked after claim")
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
	if singleXminAfterRelease == singleXminAfterClaim {
		t.Fatalf("single release preserved locked event xmin %s", singleXminAfterRelease)
	}
	batchXminAfterRelease, batchLocked := eventState(batchID)
	if batchLocked {
		t.Fatal("batch release event is locked after release")
	}
	if batchXminAfterRelease == batchXminAfterClaim {
		t.Fatalf("batch release preserved locked event xmin %s", batchXminAfterRelease)
	}
}
