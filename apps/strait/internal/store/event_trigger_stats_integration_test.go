//go:build integration

package store_test

import (
	"context"
	"testing"
)

func TestGetEventTriggerStats_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	stats, err := q.GetEventTriggerStats(ctx, "project-event-trigger-stats-empty", "")
	if err != nil {
		t.Fatalf("GetEventTriggerStats() error = %v", err)
	}
	if stats.TotalCount != 0 {
		t.Fatalf("TotalCount = %d, want 0", stats.TotalCount)
	}
	if stats.WaitingCount != 0 {
		t.Fatalf("WaitingCount = %d, want 0", stats.WaitingCount)
	}
	if stats.ReceivedCount != 0 {
		t.Fatalf("ReceivedCount = %d, want 0", stats.ReceivedCount)
	}
	if stats.AvgWaitDuration != 0 {
		t.Fatalf("AvgWaitDuration = %f, want 0", stats.AvgWaitDuration)
	}
}
