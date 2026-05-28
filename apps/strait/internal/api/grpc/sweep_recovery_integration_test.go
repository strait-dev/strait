//go:build integration

package grpc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
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
	if err != nil {
		t.Fatalf("MarkWorkerTaskResultReceivedByAssignment: %v", err)
	}
	if !marked {
		t.Fatal("expected durable result handoff to be marked")
	}
	if _, err := env.DB.Pool.Exec(ctx, `UPDATE worker_tasks SET result_received_at = NOW() - INTERVAL '10 minutes' WHERE id = $1`, taskID); err != nil {
		t.Fatalf("age result handoff: %v", err)
	}

	finalizer := &sweepRecoveryFinalizer{q: q}
	recoverDurableResultHandoffs(ctx, q, func() WorkerRunResultFinalizer { return finalizer }, time.Now().Add(-5*time.Minute))

	if finalizer.called != 1 {
		t.Fatalf("finalizer calls = %d, want 1", finalizer.called)
	}
	run, err := q.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != domain.StatusCompleted {
		t.Fatalf("run status = %q, want completed", run.Status)
	}
	var output map[string]bool
	if err := json.Unmarshal(run.Result, &output); err != nil {
		t.Fatalf("unmarshal run result: %v", err)
	}
	if !output["recovered"] {
		t.Fatalf("run result = %s, want recovered=true", string(run.Result))
	}
	task, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if task.Status != domain.WorkerTaskStatusCompleted {
		t.Fatalf("worker task status = %q, want completed", task.Status)
	}
}

func TestIntegration_RecoverDurableResultHandoffs_RetryableAfterFinalizerFailure(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	if marked, err := q.MarkWorkerTaskResultReceivedByAssignment(ctx, taskID, workerID, projectID, runID, 1, "failed", "boom", nil, 10); err != nil {
		t.Fatalf("MarkWorkerTaskResultReceivedByAssignment: %v", err)
	} else if !marked {
		t.Fatal("expected durable result handoff to be marked")
	}
	if _, err := env.DB.Pool.Exec(ctx, `UPDATE worker_tasks SET result_received_at = NOW() - INTERVAL '10 minutes' WHERE id = $1`, taskID); err != nil {
		t.Fatalf("age result handoff: %v", err)
	}

	recoverDurableResultHandoffs(ctx, q, func() WorkerRunResultFinalizer {
		return failingFinalizer{}
	}, time.Now().Add(-5*time.Minute))

	task, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if task.Status != domain.WorkerTaskStatusResultReceived {
		t.Fatalf("worker task status = %q, want result_received after retryable failure", task.Status)
	}
}

type failingFinalizer struct{}

func (failingFinalizer) FinalizeWorkerRunResult(context.Context, string, string, string, json.RawMessage) (domain.WorkerTaskStatus, error) {
	return "", context.DeadlineExceeded
}
