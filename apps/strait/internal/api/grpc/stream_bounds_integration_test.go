//go:build integration

package grpc

import (
	"context"
	"strings"
	"testing"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"
)

// TestIntegration_HandleTaskResult_OversizedRunIDRejected ensures a malicious
// worker cannot use an oversized RunId to amplify pubsub channel names or
// blow up DB-key allocations: the result must be silently dropped before any
// store call.
func TestIntegration_HandleTaskResult_OversizedRunIDRejected(t *testing.T) {
	ctx := context.Background()
	env, err := testutil.SetupTestEnv(ctx, "../../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

	q := store.New(env.DB.Pool)
	projectID, workerID, _, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	huge := strings.Repeat("x", maxRunIDLen+1)
	tr := &workerv1.TaskResult{RunId: huge, Status: "success"}
	if err := svc.handleTaskResult(ctx, workerID, projectID, tr); err != nil {
		t.Fatalf("handleTaskResult unexpectedly errored: %v", err)
	}

	// Original task must remain assigned (the oversized RunId can't match it).
	got, err := q.GetWorkerTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetWorkerTask: %v", err)
	}
	if got.Status != domain.WorkerTaskStatusAssigned {
		t.Fatalf("oversized run_id should not affect state: got %q", got.Status)
	}
}

// TestIntegration_HandleTaskResult_OversizedErrorTruncated ensures a worker
// cannot bloat DB rows with an unbounded error message — the message is
// truncated to maxErrorMsgBytes before the run is updated.
func TestIntegration_HandleTaskResult_OversizedErrorTruncated(t *testing.T) {
	ctx := context.Background()
	env, err := testutil.SetupTestEnv(ctx, "../../../migrations")
	if err != nil {
		t.Fatalf("setup test env: %v", err)
	}
	t.Cleanup(func() { env.Cleanup(ctx) })
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean: %v", err)
	}

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, _ := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	hugeErr := strings.Repeat("e", maxErrorMsgBytes*4)
	tr := &workerv1.TaskResult{RunId: runID, Status: "failed", ErrorMessage: hugeErr}
	if err := svc.handleTaskResult(ctx, workerID, projectID, tr); err != nil {
		t.Fatalf("handleTaskResult: %v", err)
	}

	got, err := q.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Error == "" {
		t.Fatal("expected error to be persisted on failed run")
	}
	if len(got.Error) > maxErrorMsgBytes {
		t.Fatalf("error message not truncated: got %d bytes, want <= %d", len(got.Error), maxErrorMsgBytes)
	}
}
