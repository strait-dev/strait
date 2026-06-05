//go:build integration

package grpc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

type sweepRecoveryFinalizer struct {
	q      *store.Queries
	called int
}

func (f *sweepRecoveryFinalizer) FinalizeWorkerRunResult(ctx context.Context, runID, status, errorMessage string, output json.RawMessage) (domain.WorkerTaskStatus, error) {
	f.called++
	fields := map[string]any{"finished_at": time.Now()}
	if status == "success" {
		if len(output) > 0 {
			fields["result"] = output
		}
		if err := f.q.UpdateRunStatus(ctx, runID, domain.StatusExecuting, domain.StatusCompleted, fields); err != nil {
			return "", err
		}
		return domain.WorkerTaskStatusCompleted, nil
	}
	if errorMessage != "" {
		fields["error"] = errorMessage
	}
	if err := f.q.UpdateRunStatus(ctx, runID, domain.StatusExecuting, domain.StatusFailed, fields); err != nil {
		return "", err
	}
	return domain.WorkerTaskStatusFailed, nil
}

func TestIntegration_RecoverDurableResultHandoffs_FinalizesPersistedResult(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	marked, err := q.MarkWorkerTaskResultReceivedByAssignment(
		ctx,
		taskID,
		workerID,
		projectID,
		runID,
		1,
		"success",
		"",
		[]byte(`{"recovered":true}`),
		25,
	)
	require.NoError(t,

		err)
	require.True(t, marked)

	if _, err := env.DB.Pool.Exec(ctx, `UPDATE worker_tasks SET result_received_at = NOW() - INTERVAL '10 minutes' WHERE id = $1`, taskID); err != nil {
		require.Failf(t, "test failure",

			"age result handoff: %v", err)
	}

	finalizer := &sweepRecoveryFinalizer{q: q}
	recoverDurableResultHandoffs(ctx, q, func() WorkerRunResultFinalizer { return finalizer }, time.Now().Add(-5*time.Minute))
	require.EqualValues(t, 1,
		finalizer.
			called)

	run, err := q.GetRun(ctx, runID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			StatusCompleted,

		run.Status)

	var output map[string]bool
	require.NoError(t,

		json.Unmarshal(run.
			Result, &output))
	require.True(t, output["recovered"])

	task, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusCompleted,

		task.
			Status)

}

func TestIntegration_RecoverDurableResultHandoffs_RetryableAfterFinalizerFailure(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	if marked, err := q.MarkWorkerTaskResultReceivedByAssignment(ctx, taskID, workerID, projectID, runID, 1, "failed", "boom", nil, 10); err != nil {
		require.Failf(t, "test failure",

			"MarkWorkerTaskResultReceivedByAssignment: %v", err)
	} else if !marked {
		require.Fail(t,

			"expected durable result handoff to be marked")
	}
	if _, err := env.DB.Pool.Exec(ctx, `UPDATE worker_tasks SET result_received_at = NOW() - INTERVAL '10 minutes' WHERE id = $1`, taskID); err != nil {
		require.Failf(t, "test failure",

			"age result handoff: %v", err)
	}

	recoverDurableResultHandoffs(ctx, q, func() WorkerRunResultFinalizer {
		return failingFinalizer{}
	}, time.Now().Add(-5*time.Minute))

	task, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t,

		err)
	require.Equal(t,
		domain.
			WorkerTaskStatusResultReceived,

		task.Status,
	)

}

type failingFinalizer struct{}

func (failingFinalizer) FinalizeWorkerRunResult(context.Context, string, string, string, json.RawMessage) (domain.WorkerTaskStatus, error) {
	return "", context.DeadlineExceeded
}
