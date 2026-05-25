//go:build integration

package grpc

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/domain"
	"strait/internal/store"
)

// TestIntegration_HandleTaskResult_OversizedRunIDRejected ensures a malicious
// worker cannot use an oversized RunId to amplify pubsub channel names or
// blow up DB-key allocations: the result must be silently dropped before any
// store call.
func TestIntegration_HandleTaskResult_OversizedRunIDRejected(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, _, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	huge := strings.Repeat("x", maxRunIDLen+1)
	tr := &workerv1.TaskResult{RunId: huge, Status: "success", AssignmentId: taskID, Attempt: 1}
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
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedRunWithTask(t, ctx, q, env)
	svc := fallbackService(q)

	hugeErr := strings.Repeat("e", maxErrorMsgBytes*4)
	tr := assignedTaskResult(runID, taskID, "failed")
	tr.ErrorMessage = hugeErr
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

// TestIntegration_StreamTasks_InvalidRegistrationBoundsRejectedBeforeRegistry
// verifies registration-size checks run before the stream mutates in-memory
// worker state. These fields are SDK-controlled and otherwise persist in the
// registry, audit payloads, and DB sync path.
func TestIntegration_StreamTasks_InvalidRegistrationBoundsRejectedBeforeRegistry(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	const projectID = "proj-registration-bounds"

	cases := []struct {
		name   string
		mutate func(*workerv1.WorkerRegistration)
	}{
		{
			name: "oversized queue name",
			mutate: func(reg *workerv1.WorkerRegistration) {
				reg.Queues = []string{strings.Repeat("q", maxQueueNameBytes+1)}
			},
		},
		{
			name: "oversized job slug",
			mutate: func(reg *workerv1.WorkerRegistration) {
				reg.JobSlugs = []string{strings.Repeat("s", maxJobSlugBytes+1)}
			},
		},
		{
			name: "oversized metadata value",
			mutate: func(reg *workerv1.WorkerRegistration) {
				reg.Metadata = map[string]string{"sdk": strings.Repeat("m", maxRegistrationMetadataValueBytes+1)}
			},
		},
		{
			name:   "blank queue name",
			mutate: func(reg *workerv1.WorkerRegistration) { reg.Queues = []string{" "} },
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workerID := fmt.Sprintf("worker-registration-bounds-%d", i)
			apiKeyID := fmt.Sprintf("key-registration-bounds-%d", i)
			rawKey := fmt.Sprintf("strait_registrationBoundsKey%d", i)
			seedGRPCAPIKey(t, ctx, q, projectID, apiKeyID, rawKey)

			reg := &workerv1.WorkerRegistration{
				WorkerId:       workerID,
				Name:           "bounds worker",
				Hostname:       "host",
				SdkVersion:     "1.0.0",
				SdkLanguage:    "go",
				Queues:         []string{"default"},
				SlotsTotal:     1,
				SlotsAvailable: 1,
			}
			tc.mutate(reg)

			stream := newBlockingWorkerStream(ctx, rawKey)
			stream.recvCh <- &workerv1.WorkerMessage{
				Payload: &workerv1.WorkerMessage_Registration{Registration: reg},
			}
			svc := &workerService{
				queries:        q,
				pub:            &noopPublisher{},
				registry:       NewConnectionRegistry(),
				resultChannels: NewResultChannelRegistry(),
			}

			err := svc.StreamTasks(stream)
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("StreamTasks error = %v, want InvalidArgument", err)
			}
			if got := svc.registry.Snapshot(); len(got) != 0 {
				t.Fatalf("invalid registration mutated registry: got %d workers", len(got))
			}
			select {
			case msg := <-stream.sentCh:
				t.Fatalf("invalid registration sent server message: %T", msg.Payload)
			default:
			}
		})
	}
}
