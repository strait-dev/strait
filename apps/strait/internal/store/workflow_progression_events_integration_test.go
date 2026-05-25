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
	events, err = q.ClaimWorkflowProgressionEvents(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimWorkflowProgressionEvents() after ack error = %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("ClaimWorkflowProgressionEvents() after ack len = %d, want 0", len(events))
	}
}
