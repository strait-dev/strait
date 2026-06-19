package grpc

import (
	"context"
	"strings"
	"testing"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBounds_Constants pins the worker-plane resource bounds. These caps are
// the only thing standing between a malicious or buggy worker and unbounded
// memory / DB / pubsub-channel growth on the server. Any change to a cap
// should be deliberate and reviewed.
func TestBounds_Constants(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"maxWorkerIDLen", maxWorkerIDLen, 128},
		{"maxQueuesPerWorker", maxQueuesPerWorker, 64},
		{"maxQueueNameBytes", maxQueueNameBytes, 128},
		{"maxJobSlugsPerWorker", maxJobSlugsPerWorker, 256},
		{"maxJobSlugBytes", maxJobSlugBytes, 128},
		{"maxInFlightTasks", maxInFlightTasks, 256},
		{"maxLogMessageBytes", maxLogMessageBytes, 4096},
		{"maxLogLevelBytes", maxLogLevelBytes, 32},
		{"maxRunIDLen", maxRunIDLen, 128},
		{"maxErrorMsgBytes", maxErrorMsgBytes, 8192},
		{"maxSlotsPerWorker", maxSlotsPerWorker, 1024},
		{"maxHostnameBytes", maxHostnameBytes, 255},
		{"maxSDKVersionBytes", maxSDKVersionBytes, 64},
		{"maxSDKLanguageBytes", maxSDKLanguageBytes, 32},
		{"maxNameBytes", maxNameBytes, 128},
		{"maxRegistrationMetadataEntries", maxRegistrationMetadataEntries, 64},
		{"maxRegistrationMetadataKeyBytes", maxRegistrationMetadataKeyBytes, 64},
		{"maxRegistrationMetadataValueBytes", maxRegistrationMetadataValueBytes, 512},
	}
	for _, tc := range cases {
		assert.Equal(t,
			tc.want,
			tc.got)
	}
}

func TestDeepSecHandleAck_OversizedRunIDRejectedBeforeStore(t *testing.T) {
	t.Parallel()

	svc := &workerService{}
	err := svc.handleAck(context.Background(), "worker-1", "proj-1", &workerv1.Acknowledged{
		Id: strings.Repeat("r", maxRunIDLen+1),
	})
	require.NoError(t, err)
}

func TestHandleTaskResult_RejectsInvalidIdentityBeforeStore(t *testing.T) {
	t.Parallel()

	svc := &workerService{}
	tests := []struct {
		name   string
		result *workerv1.TaskResult
	}{
		{
			name:   "nil result",
			result: nil,
		},
		{
			name:   "empty run id",
			result: &workerv1.TaskResult{AssignmentId: "task-1", Attempt: 1},
		},
		{
			name: "oversized run id",
			result: &workerv1.TaskResult{
				RunId:        strings.Repeat("r", maxRunIDLen+1),
				AssignmentId: "task-1",
				Attempt:      1,
			},
		},
		{
			name: "missing assignment id",
			result: &workerv1.TaskResult{
				RunId:   "run-1",
				Attempt: 1,
			},
		},
		{
			name: "zero attempt",
			result: &workerv1.TaskResult{
				RunId:        "run-1",
				AssignmentId: "task-1",
			},
		},
		{
			name: "negative attempt",
			result: &workerv1.TaskResult{
				RunId:        "run-1",
				AssignmentId: "task-1",
				Attempt:      -1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.NoError(t, svc.handleTaskResult(context.Background(), "worker-1", "proj-1", tt.result))
		})
	}
}

func TestCanAcknowledgeWorkerTaskStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status domain.WorkerTaskStatus
		want   bool
	}{
		{name: "assigned", status: domain.WorkerTaskStatusAssigned, want: true},
		{name: "accepted", status: domain.WorkerTaskStatusAccepted, want: false},
		{name: "result received", status: domain.WorkerTaskStatusResultReceived, want: false},
		{name: "completed", status: domain.WorkerTaskStatusCompleted, want: false},
		{name: "failed", status: domain.WorkerTaskStatusFailed, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, canAcknowledgeWorkerTaskStatus(tt.status))
		})
	}
}
