//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestAuditReclaimer_ListAndDeleteDeadletter(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("reclaimer-test")
	q.SetAuditSigningKey(key)

	// Insert 3 deadletter events.
	for i := range 3 {
		ev := &domain.AuditEvent{
			ProjectID:    "proj-reclaim",
			ActorID:      "actor-1",
			ActorType:    "user",
			Action:       domain.AuditActionJobTriggered,
			ResourceType: "job",
			ResourceID:   "job-1",
			Details:      json.RawMessage(`{}`),
			CreatedAt:    time.Now().UTC().Truncate(time.Microsecond),
		}
		if err := q.CreateAuditEventDeadletter(ctx, ev, "db down", i); err != nil {
			t.Fatalf("insert deadletter %d: %v", i, err)
		}
	}

	// List should return all 3.
	events, ids, err := q.ListAuditEventsDeadletter(ctx, 10)
	if err != nil {
		t.Fatalf("ListAuditEventsDeadletter: %v", err)
	}
	if len(events) != 3 || len(ids) != 3 {
		t.Fatalf("len = %d/%d, want 3/3", len(events), len(ids))
	}

	// Reclaim: write to primary table, delete from DLQ.
	for i, ev := range events {
		evCopy := ev
		if err := q.CreateAuditEvent(ctx, &evCopy); err != nil {
			t.Fatalf("reclaim %d CreateAuditEvent: %v", i, err)
		}
		if err := q.DeleteAuditEventDeadletter(ctx, ids[i]); err != nil {
			t.Fatalf("reclaim %d DeleteDeadletter: %v", i, err)
		}
	}

	// DLQ should be empty.
	count, _ := q.CountAuditEventsDeadletter(ctx)
	if count != 0 {
		t.Errorf("deadletter count = %d after reclaim, want 0", count)
	}

	// Primary chain should be valid.
	vc, err := q.VerifyAuditChain(ctx, "proj-reclaim")
	if err != nil {
		t.Fatalf("VerifyAuditChain: %v", err)
	}
	if !vc.Valid {
		t.Errorf("chain invalid after reclaim: %s", vc.Error)
	}
	if vc.EventsChecked != 3 {
		t.Errorf("events_checked = %d, want 3", vc.EventsChecked)
	}
}
