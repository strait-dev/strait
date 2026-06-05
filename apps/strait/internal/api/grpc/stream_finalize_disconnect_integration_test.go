//go:build integration

package grpc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingReadyRunQueue struct {
	runIDs []string
}

func (q *recordingReadyRunQueue) EnqueueExisting(_ context.Context, run *domain.JobRun) error {
	if run != nil {
		q.runIDs = append(q.runIDs, run.ID)
	}
	return nil
}

// TestIntegration_FinalizeDisconnect_MarksOfflineAndAudits pins the disconnect
// cleanup contract: the worker row must move to `offline` and emit a
// worker.disconnected audit event, even after the stream context is cancelled.
//
// Pre-fix the deferred block reused the cancelled stream ctx, so neither the
// audit insert nor any status transition reached Postgres. The fix uses a
// detached context with a 5s timeout.
func TestIntegration_FinalizeDisconnect_MarksOfflineAndAudits(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)

	const workerID = "disco-worker"
	const projectID = "proj-disco"
	require.NoError(t,

		q.RegisterWorker(ctx,
			&domain.Worker{ID: workerID,
				ProjectID: projectID, QueueName: "q", Hostname: "host", Version: "1.0", Status: domain.
						WorkerStatusActive}))

	svc := &workerService{
		queries:        q,
		pub:            &noopPublisher{},
		registry:       NewConnectionRegistry(),
		resultChannels: NewResultChannelRegistry(),
	}

	// finalizeDisconnect deliberately takes no ctx — it must allocate its own
	// detached context internally so it remains effective when the stream
	// ctx is already cancelled at the time the deferred cleanup fires.
	svc.finalizeDisconnect(projectID, workerID)

	// Workers row must now be offline.
	var status string
	require.NoError(t,

		env.DB.Pool.
			QueryRow(ctx, `SELECT status FROM workers WHERE id = $1`,

				workerID).Scan(&status))
	assert.Equal(t, string(domain.
		WorkerStatusOffline,
	), status,
	)

	// Audit event must have landed.
	var auditCount int
	require.NoError(t,

		env.DB.Pool.
			QueryRow(ctx, `SELECT COUNT(*) FROM audit_events
		 WHERE resource_type = 'worker' AND resource_id = $1 AND action = $2`,

				workerID, domain.AuditActionWorkerDisconnected,
			).Scan(&auditCount),
	)
	assert.EqualValues(t, 1,

		auditCount,
	)

}

// TestIntegration_FinalizeDisconnect_RequeuesOpenWorkerRuns verifies that
// disconnect cleanup requeues in-flight worker-mode runs and closes out their
// worker_tasks rows instead of waiting for the generic stale-run reaper.
func TestIntegration_FinalizeDisconnect_RequeuesOpenWorkerRuns(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	readyQueue := &recordingReadyRunQueue{}

	svc := &workerService{
		queries:        q,
		pub:            &noopPublisher{},
		registry:       NewConnectionRegistry(),
		resultChannels: NewResultChannelRegistry(),
		readyRunQueue:  readyQueue,
	}

	svc.finalizeDisconnect(projectID, workerID)

	run, err := q.GetRun(ctx, runID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			StatusQueued,

		run.Status)
	require.Nil(t, run.StartedAt)
	require.Nil(t, run.FinishedAt)
	require.Nil(t, run.HeartbeatAt)
	require.Equal(t,
		"worker disconnected before reporting result",

		run.
			Error)

	task, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusFailed,

		task.Status,
	)
	require.False(t,
		task.
			FinishedAt ==
			nil ||
			task.FinishedAt.
				Before(
					time.Now().Add(-time.Minute)))
	require.False(t,
		len(readyQueue.
			runIDs,
		) != 1 || readyQueue.
			runIDs[0] != runID,
	)

}

// TestIntegration_FinalizeDisconnect_SkipsResultReceivedWorkerRuns verifies
// that disconnect cleanup cannot requeue a run after the API has already
// received the worker result but before executor finalization has completed.
func TestIntegration_FinalizeDisconnect_SkipsResultReceivedWorkerRuns(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	readyQueue := &recordingReadyRunQueue{}
	if marked, err := q.MarkWorkerTaskResultReceived(ctx, taskID); err != nil {
		require.Failf(t, "test failure",

			"MarkWorkerTaskResultReceived: %v", err)
	} else if !marked {
		require.Fail(t,

			"MarkWorkerTaskResultReceived marked = false, want true")
	}

	svc := &workerService{
		queries:        q,
		pub:            &noopPublisher{},
		registry:       NewConnectionRegistry(),
		resultChannels: NewResultChannelRegistry(),
		readyRunQueue:  readyQueue,
	}

	svc.finalizeDisconnect(projectID, workerID)

	run, err := q.GetRun(ctx, runID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			StatusExecuting,

		run.Status)
	require.Equal(t,
		"",
		run.Error,
	)

	task, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusResultReceived,

		task.Status,
	)
	require.Nil(t, task.
		FinishedAt)
	require.Len(t, readyQueue.
		runIDs,
		0)

}

// TestIntegration_TaskResultHandoffPrecedesDisconnectRequeue verifies the
// stream recv path marks the worker_task non-requeueable before delivering the
// buffered TaskResult to WorkerDispatch. This pins the race where a worker
// disconnect immediately after sending a result could requeue completed work.
func TestIntegration_TaskResultHandoffPrecedesDisconnectRequeue(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	resultChannels := NewResultChannelRegistry()
	resultCh := resultChannels.Register(runID, projectID, workerID, taskID, 1)
	t.Cleanup(func() { resultChannels.Deregister(runID) })

	svc := &workerService{
		queries:        q,
		pub:            &noopPublisher{},
		registry:       NewConnectionRegistry(),
		resultChannels: resultChannels,
	}
	require.NoError(t,

		svc.handleTaskResult(ctx, workerID,
			projectID,
			&workerv1.
				TaskResult{RunId: runID, Status: "success", OutputJson: []byte(`{"ok":true}`),
				AssignmentId: taskID, Attempt: 1}))

	task, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusResultReceived,

		task.Status,
	)
	require.NotNil(t,

		task.Result,
	)

	var durableOutput map[string]bool
	require.NoError(t,

		json.Unmarshal(task.
			Result.Output, &durableOutput,
		))
	require.False(t,
		task.
			Result.
			Status !=
			"success" || !durableOutput["ok"])

	svc.finalizeDisconnect(projectID, workerID)

	run, err := q.GetRun(ctx, runID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			StatusExecuting,

		run.Status)

	select {
	case got := <-resultCh:
		if got == nil || got.RunId != runID || got.Status != "success" {
			require.Failf(t, "test failure",

				"delivered result = %#v, want success for run %s", got, runID)
		}
	default:
		require.Fail(t, "expected buffered result to be delivered to dispatcher channel")
	}
}

func TestIntegration_TaskResultHandoffRejectsStaleAssignmentIdentity(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	resultChannels := NewResultChannelRegistry()
	resultCh := resultChannels.Register(runID, projectID, workerID, taskID, 1)
	t.Cleanup(func() { resultChannels.Deregister(runID) })

	svc := &workerService{
		queries:        q,
		pub:            &noopPublisher{},
		registry:       NewConnectionRegistry(),
		resultChannels: resultChannels,
	}

	staleResults := []*workerv1.TaskResult{
		{RunId: runID, Status: "success", Attempt: 1},
		{RunId: runID, Status: "success", AssignmentId: "old-task", Attempt: 1},
		{RunId: runID, Status: "success", AssignmentId: taskID, Attempt: 2},
	}
	for _, tr := range staleResults {
		require.NoError(t,

			svc.handleTaskResult(ctx, workerID,
				projectID,
				tr))

	}

	task, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusAssigned,

		task.
			Status)
	require.Nil(t, task.
		Result)

	select {
	case got := <-resultCh:
		require.Failf(t, "test failure", "stale result delivered to dispatcher channel: %#v", got)
	default:
	}

	exact := assignedTaskResult(runID, taskID, "success")
	exact.OutputJson = []byte(`{"ok":true}`)
	require.NoError(t,

		svc.handleTaskResult(ctx, workerID,
			projectID,
			exact))

	select {
	case got := <-resultCh:
		if got == nil || got.AssignmentId != taskID || got.Attempt != 1 {
			require.Failf(t, "test failure",

				"delivered result = %#v, want exact assignment", got)
		}
	default:
		require.Fail(t, "expected exact result to be delivered")
	}
}

// TestIntegration_CleanupRegistration_StaleReconnectDoesNotRequeue verifies
// that a stale stream from a same-ID reconnect cannot run disconnect cleanup
// for the replacement connection.
func TestIntegration_CleanupRegistration_StaleReconnectDoesNotRequeue(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)

	reg := NewConnectionRegistry()
	oldWorker := registerWorkerInRegistry(t, reg, workerID, projectID, 1)
	oldToken := oldWorker.regToken
	newWorker := &ConnectedWorker{
		WorkerID:       workerID,
		ProjectID:      projectID,
		APIKeyID:       oldWorker.APIKeyID,
		Queues:         []string{"default"},
		SlotsTotal:     1,
		SlotsAvailable: 1,
		Status:         "active",
		SendCh:         make(chan *workerv1.ServerMessage, 1),
		revokeCh:       make(chan struct{}),
	}
	require.NoError(t,

		reg.Register(newWorker))

	svc := &workerService{
		queries:        q,
		pub:            &noopPublisher{},
		registry:       reg,
		resultChannels: NewResultChannelRegistry(),
	}

	svc.cleanupRegistration(projectID, workerID, oldToken)

	run, err := q.GetRun(ctx, runID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			StatusExecuting,

		run.Status)

	task, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusAssigned,

		task.
			Status)

	worker, err := q.GetWorker(ctx, workerID, projectID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerStatusActive,

		worker.Status,
	)

}
