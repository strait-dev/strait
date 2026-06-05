//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		require.NoError(t, q.CreateAuditEventDeadletter(ctx, ev, "db down",
			i))

	}

	// List should return all 3.
	events, ids, err := q.ListAuditEventsDeadletter(ctx, 10)
	require.NoError(t, err)
	require.False(t, len(events) != 3 ||
		len(ids) != 3)

	// Reclaim: write to primary table, delete from DLQ.
	for i, ev := range events {
		evCopy := ev
		require.NoError(t, q.CreateAuditEvent(ctx, &evCopy))
		require.NoError(t, q.DeleteAuditEventDeadletter(ctx, ids[i],
			ev.ProjectID,
		))

	}

	// DLQ should be empty.
	count, _ := q.CountAuditEventsDeadletter(ctx)
	assert.EqualValues(t, 0, count)

	// Primary chain should be valid.
	vc, err := q.VerifyAuditChain(ctx, "proj-reclaim")
	require.NoError(t, err)
	assert.True(t, vc.Valid)
	assert.EqualValues(t, 3, vc.EventsChecked)

}
