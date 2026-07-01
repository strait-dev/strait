package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestLoadConfigRequiresAPIKey(t *testing.T) {
	t.Setenv("DOGFOOD_WORKER_API_KEY", "")

	_, err := loadConfig()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DOGFOOD_WORKER_API_KEY")
}

func TestLoadConfigParsesLocalWorkerSettings(t *testing.T) {
	t.Setenv("DOGFOOD_WORKER_API_KEY", "sk_test")
	t.Setenv("DOGFOOD_GRPC_ADDR", "127.0.0.1:15053")
	t.Setenv("DOGFOOD_WORKER_ID", "worker-1")
	t.Setenv("DOGFOOD_WORKER_QUEUE", "critical")
	t.Setenv("DOGFOOD_WORKER_SLOTS", "3")
	t.Setenv("DOGFOOD_WORKER_HEARTBEAT", "500ms")
	t.Setenv("DOGFOOD_WORKER_DELAY", "25ms")
	t.Setenv("DOGFOOD_WORKER_JOB_SLUGS", "alpha, beta ,, gamma")

	cfg, err := loadConfig()

	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:15053", cfg.GRPCAddr)
	assert.Equal(t, "worker-1", cfg.WorkerID)
	assert.Equal(t, "critical", cfg.QueueName)
	assert.Equal(t, int32(3), cfg.Slots)
	assert.Equal(t, 500*time.Millisecond, cfg.HeartbeatInterval)
	assert.Equal(t, 25*time.Millisecond, cfg.WorkDelay)
	assert.Equal(t, []string{"alpha", "beta", "gamma"}, cfg.JobSlugs)
}

func TestLoadConfigRejectsNonPositiveSlots(t *testing.T) {
	t.Setenv("DOGFOOD_WORKER_API_KEY", "sk_test")
	t.Setenv("DOGFOOD_WORKER_SLOTS", "0")

	_, err := loadConfig()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DOGFOOD_WORKER_SLOTS")
}

func TestTransportCredentialsUsePlaintextForLoopback(t *testing.T) {
	t.Setenv("DOGFOOD_GRPC_PLAINTEXT", "")

	creds := transportCredentials("localhost:15053")

	assert.Equal(t, "insecure", creds.Info().SecurityProtocol)
}

func TestIsCleanStreamCloseAcceptsLocalShutdownErrors(t *testing.T) {
	assert.True(t, isCleanStreamClose(io.EOF))
	assert.True(t, isCleanStreamClose(status.Error(codes.Canceled, "context canceled")))
	assert.False(t, isCleanStreamClose(status.Error(codes.Unavailable, "down")))
	assert.False(t, isCleanStreamClose(errors.New("boom")))
	assert.False(t, isCleanStreamClose(nil))
}

func TestExecuteAssignmentReturnsDogfoodOutput(t *testing.T) {
	cfg := config{WorkerID: "worker-1", WorkDelay: time.Nanosecond}
	assignment := &workerv1.TaskAssignment{
		RunId:        "run-1",
		JobSlug:      "job-a",
		Queue:        "critical",
		AssignmentId: "assignment-1",
		Attempt:      2,
	}

	msg := executeAssignment(context.Background(), cfg, assignment)
	result := msg.GetTaskResult()

	require.NotNil(t, result)
	assert.Equal(t, "run-1", result.GetRunId())
	assert.Equal(t, "success", result.GetStatus())
	assert.Equal(t, "assignment-1", result.GetAssignmentId())
	assert.Equal(t, int32(2), result.GetAttempt())

	var output map[string]any
	require.NoError(t, json.Unmarshal(result.GetOutputJson(), &output))
	assert.Equal(t, true, output["ok"])
	assert.Equal(t, "worker-1", output["worker_id"])
	assert.Equal(t, "job-a", output["job_slug"])
	assert.Equal(t, "critical", output["queue"])
}

func TestExecuteAssignmentReportsStoppedContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	msg := executeAssignment(ctx, config{WorkDelay: time.Hour}, &workerv1.TaskAssignment{RunId: "run-1"})
	result := msg.GetTaskResult()

	require.NotNil(t, result)
	assert.Equal(t, "failed", result.GetStatus())
	assert.Contains(t, result.GetErrorMessage(), "stopped")
}
