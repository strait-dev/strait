//go:build integration

package grpc

import (
	"context"
	"testing"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

// fallbackServiceWithRegistry mirrors fallbackService but exposes the
// registry so tests can inspect/decrement slots. Keeps resultChannels nil
// so every TaskResult lands on the fallback branch.
func fallbackServiceWithRegistry(q *store.Queries, reg *ConnectionRegistry) *workerService {
	return &workerService{
		queries:        q,
		pub:            &noopPublisher{},
		registry:       reg,
		resultChannels: NewResultChannelRegistry(),
	}
}

// registerWorkerInRegistry pushes a worker into the registry without going
// through the gRPC path. We just need an entry whose slots can be decremented
// and inspected.
func registerWorkerInRegistry(t *testing.T, reg *ConnectionRegistry, workerID, projectID string, slotsTotal int32) *ConnectedWorker {
	t.Helper()
	w := &ConnectedWorker{
		WorkerID:       workerID,
		ProjectID:      projectID,
		APIKeyID:       "key-" + workerID,
		Queues:         []string{"default"},
		SlotsTotal:     slotsTotal,
		SlotsAvailable: slotsTotal,
		Status:         "active",
		SendCh:         make(chan *workerv1.ServerMessage, 1),
		revokeCh:       make(chan struct{}),
	}
	require.NoError(t,

		reg.Register(w))

	return w
}

// TestIntegration_Fallback_DoesNotOverCreditSlots simulates the late-result
// race: a dispatcher decremented the slot, then ctx.Done() restored it
// (mirrored here by an explicit Decrement+Increment), then the worker's
// late TaskResult arrives. The fallback path must NOT credit the slot
// again — the worker's SlotsAvailable must remain capped at SlotsTotal.
func TestIntegration_Fallback_DoesNotOverCreditSlots(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)

	reg := NewConnectionRegistry()
	const slots = int32(4)
	registerWorkerInRegistry(t, reg, workerID, projectID, slots)

	// Simulate the dispatcher path: decrement on send, then restore on ctx.Done().
	reg.DecrementSlots(workerID)
	reg.IncrementSlots(workerID) // dispatcher's ctx.Done() restoration

	svc := fallbackServiceWithRegistry(q, reg)

	tr := assignedTaskResult(runID, taskID, "success")
	require.NoError(t,

		svc.handleTaskResult(ctx, workerID,

			projectID, tr))

	snap := reg.Snapshot()
	var got int32 = -1
	for _, w := range snap {
		if w.WorkerID == workerID {
			got = w.SlotsAvailable
			break
		}
	}
	require.NotEqual(
		t,
		-1, got,
	)
	require.Equal(t,
		slots,
		got,
	)

}

// TestIntegration_Fallback_RepeatedLateResultsStaySlot verifies the fix is
// stable across multiple late deliveries (duplicate worker resends, etc.):
// each pass through the fallback must be a slot no-op.
func TestIntegration_Fallback_RepeatedLateResultsStaySlot(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)

	reg := NewConnectionRegistry()
	const slots = int32(2)
	registerWorkerInRegistry(t, reg, workerID, projectID, slots)

	// Take one slot to make sure the cap is meaningfully testable
	// (SlotsTotal=2, SlotsAvailable=1 after this, so an over-credit would
	// push to 2 instead of staying at 1).
	reg.DecrementSlots(workerID)

	svc := fallbackServiceWithRegistry(q, reg)
	tr := assignedTaskResult(runID, taskID, "success")

	for range 3 {
		require.NoError(t,

			svc.handleTaskResult(ctx, workerID,

				projectID, tr))

	}

	snap := reg.Snapshot()
	var got int32 = -1
	for _, w := range snap {
		if w.WorkerID == workerID {
			got = w.SlotsAvailable
			break
		}
	}
	require.Equal(t,
		slots-
			1,
		got)
	require.Equal(t,
		domain.
			StatusCompleted,

		getRunStatus(t, ctx, q, runID))

}

// getRunStatus is a tiny inline helper kept here to avoid touching
// stream_fallback_integration_test.go for an unrelated change.
func getRunStatus(t *testing.T, ctx context.Context, q *store.Queries, runID string) domain.RunStatus {
	t.Helper()
	r, err := q.GetRun(ctx, runID)
	require.NoError(t,

		err)
	require.NotNil(t,

		r)

	return r.Status
}
