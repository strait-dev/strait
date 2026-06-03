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
