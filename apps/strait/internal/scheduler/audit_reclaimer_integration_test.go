//go:build integration

package scheduler_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/scheduler"
	"strait/internal/store"
)

// seedDLQ inserts n deadletter audit events for the given project.
func seedDLQ(t *testing.T, ctx context.Context, q *store.Queries, projectID string, n int) {
	t.Helper()
	for i := range n {
		ev := &domain.AuditEvent{
			ProjectID:    projectID,
			ActorID:      "actor-dlq",
			ActorType:    "user",
			Action:       domain.AuditActionJobTriggered,
			ResourceType: "job",
			ResourceID:   "job-dlq",
			Details:      json.RawMessage(`{}`),
			CreatedAt:    time.Now().UTC().Truncate(time.Microsecond),
		}
		if err := q.CreateAuditEventDeadletter(ctx, ev, "seed", 0); err != nil {
			t.Fatalf("seed DLQ %d: %v", i, err)
		}
	}
}

func TestReclaimAuditDeadletter_ReplaysIntoChain(t *testing.T) {
	ctx := context.Background()
	intTestClean(t, ctx)

	q := intTestStore(t)
	key, _ := store.DeriveAuditSigningKey("reclaimer-integration")
	q.SetAuditSigningKey(key)

	const projectID = "proj-reclaim-int"
	seedDLQ(t, ctx, q, projectID, 5)

	r := scheduler.NewReaper(q, time.Second, time.Minute, 0, 0, false, nil).
		WithAuditDLQReclaimBatch(200)
	r.ReapOnce(ctx)

	count, err := q.CountAuditEventsDeadletter(ctx)
	if err != nil {
		t.Fatalf("CountAuditEventsDeadletter: %v", err)
	}
	if count != 0 {
		t.Errorf("DLQ count after reclaim = %d, want 0", count)
	}

	vc, err := q.VerifyAuditChain(ctx, projectID)
	if err != nil {
		t.Fatalf("VerifyAuditChain: %v", err)
	}
	if !vc.Valid {
		t.Errorf("chain invalid after reclaim: %s", vc.Error)
	}
	if vc.EventsChecked != 5 {
		t.Errorf("events_checked = %d, want 5", vc.EventsChecked)
	}
}

func TestReclaimAuditDeadletter_RespectsBatchCap(t *testing.T) {
	ctx := context.Background()
	intTestClean(t, ctx)

	q := intTestStore(t)
	key, _ := store.DeriveAuditSigningKey("reclaimer-cap")
	q.SetAuditSigningKey(key)

	const projectID = "proj-reclaim-cap"
	seedDLQ(t, ctx, q, projectID, 500)

	r := scheduler.NewReaper(q, time.Second, time.Minute, 0, 0, false, nil).
		WithAuditDLQReclaimBatch(100)

	// One tick should drain exactly 100 (the cap).
	r.ReapOnce(ctx)

	count, err := q.CountAuditEventsDeadletter(ctx)
	if err != nil {
		t.Fatalf("CountAuditEventsDeadletter: %v", err)
	}
	if count != 400 {
		t.Fatalf("DLQ remaining = %d, want 400 after single capped tick", count)
	}
}

type failingReplayStore struct {
	*store.Queries
}

func (f *failingReplayStore) ReplayAuditEventDeadletter(context.Context, string, string, string) (*domain.AuditEvent, bool, error) {
	return nil, false, errors.New("simulated replay failure")
}

func TestReclaimAuditDeadletter_PermanentFailure_LeavesInDLQ(t *testing.T) {
	ctx := context.Background()
	intTestClean(t, ctx)

	q := intTestStore(t)
	key, _ := store.DeriveAuditSigningKey("reclaimer-permfail")
	q.SetAuditSigningKey(key)

	const projectID = "proj-reclaim-permfail"
	seedDLQ(t, ctx, q, projectID, 3)

	r := scheduler.NewReaper(&failingReplayStore{Queries: q}, time.Second, time.Minute, 0, 0, false, nil).
		WithAuditDLQReclaimBatch(200)
	r.ReapOnce(ctx)

	count, err := q.CountAuditEventsDeadletter(ctx)
	if err != nil {
		t.Fatalf("CountAuditEventsDeadletter: %v", err)
	}
	if count != 3 {
		t.Fatalf("DLQ count after failed reclaim = %d, want 3 (rows must remain)", count)
	}
}
