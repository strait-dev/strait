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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedDLQ inserts n deadletter audit events for the given project.
func seedDLQ(t *testing.T, ctx context.Context, q *store.Queries, projectID string, n int) {
	t.Helper()
	for range n {
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
		require.NoError(t, q.CreateAuditEventDeadletter(
			ctx, ev, "seed",
			0))

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
	require.NoError(t, err)
	assert.EqualValues(t, 0, count)

	vc, err := q.VerifyAuditChain(ctx, projectID)
	require.NoError(t, err)
	assert.True(t, vc.Valid)
	assert.EqualValues(t, 5, vc.EventsChecked)

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
	require.NoError(t, err)
	require.EqualValues(t, 400, count)

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
	require.NoError(t, err)
	require.EqualValues(t, 3, count)

}
