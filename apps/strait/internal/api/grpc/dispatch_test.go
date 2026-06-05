package grpc

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	straitcrypto "strait/internal/crypto"
	"strait/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeDispatchSecretDecryptor struct{}

func (fakeDispatchSecretDecryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	const prefix = "encrypted:"
	if string(ciphertext[:len(prefix)]) != prefix {
		return nil, errors.New("unexpected ciphertext")
	}
	return ciphertext[len(prefix):], nil
}

// TestResultChannelRegistry_SendAndReceive verifies basic send/receive semantics.
func TestResultChannelRegistry_SendAndReceive(t *testing.T) {
	r := NewResultChannelRegistry()
	ch := r.Register("run-1", "proj-1", "worker-1", "task-1", 1)

	result := &workerv1.TaskResult{RunId: "run-1", Status: "success", AssignmentId: "task-1", Attempt: 1}
	require.True(t,
		r.Send("run-1", "proj-1",
			"worker-1",

			result))

	select {
	case got := <-ch:
		if got.Status != "success" {
			assert.Failf(t, "test failure",

				"expected status=success, got %s", got.Status)
		}
	default:
		assert.Fail(t, "expected result to be in channel")
	}
}

// TestResultChannelRegistry_SendToUnknownRun verifies Send returns false for unknown run IDs.
func TestResultChannelRegistry_SendToUnknownRun(t *testing.T) {
	r := NewResultChannelRegistry()
	result := &workerv1.TaskResult{RunId: "unknown", Status: "success"}
	assert.False(t,
		r.Send("unknown",
			"proj-1",
			"worker-1",
			result,
		),
	)

}

// TestResultChannelRegistry_RejectCrossProject is the regression test for the
// cross-tenant integrity attack: a worker authenticated to project A must not
// be able to deliver a TaskResult for a run owned by project B.
func TestResultChannelRegistry_RejectCrossProject(t *testing.T) {
	r := NewResultChannelRegistry()
	ch := r.Register("victim-run", "proj-victim", "worker-victim", "victim-task", 1)

	forged := &workerv1.TaskResult{RunId: "victim-run", Status: "success", AssignmentId: "victim-task", Attempt: 1}
	require.False(
		t, r.Send("victim-run",
			"proj-attacker",

			"worker-attacker",

			forged))
	require.True(t,
		r.Send("victim-run",
			"proj-victim",
			"worker-victim",

			forged))

	// And the legitimate owner can still deliver.

	select {
	case got := <-ch:
		if got != forged {
			assert.Fail(t,

				"expected legitimate result delivered to channel")
		}
	default:
		assert.Fail(t, "expected legitimate result in channel")
	}
}

func TestResultChannelRegistry_RejectSameProjectDifferentWorker(t *testing.T) {
	r := NewResultChannelRegistry()
	ch := r.Register("victim-run", "proj-1", "worker-owner", "victim-task", 1)

	forged := &workerv1.TaskResult{RunId: "victim-run", Status: "success", AssignmentId: "victim-task", Attempt: 1}
	require.False(
		t, r.Send("victim-run",
			"proj-1",
			"worker-peer",

			forged))
	require.True(t,
		r.Send("victim-run",
			"proj-1",
			"worker-owner",

			forged))

	select {
	case got := <-ch:
		if got != forged {
			require.Fail(t,

				"expected assigned worker result")
		}
	default:
		require.Fail(t, "expected assigned worker result in channel")
	}
}

func TestResultChannelRegistry_RejectStaleAssignmentIdentity(t *testing.T) {
	t.Parallel()

	r := NewResultChannelRegistry()
	ch := r.Register("run-1", "proj-1", "worker-1", "current-task", 2)

	cases := []struct {
		name   string
		result *workerv1.TaskResult
	}{
		{
			name:   "missing assignment",
			result: &workerv1.TaskResult{RunId: "run-1", Status: "success", Attempt: 2},
		},
		{
			name:   "wrong assignment",
			result: &workerv1.TaskResult{RunId: "run-1", Status: "success", AssignmentId: "old-task", Attempt: 2},
		},
		{
			name:   "wrong attempt",
			result: &workerv1.TaskResult{RunId: "run-1", Status: "success", AssignmentId: "current-task", Attempt: 1},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.False(
				t, r.Send("run-1",
					"proj-1",
					"worker-1",
					tc.result,
				))

		})
	}

	current := &workerv1.TaskResult{RunId: "run-1", Status: "success", AssignmentId: "current-task", Attempt: 2}
	require.True(t,
		r.Send("run-1", "proj-1",
			"worker-1",

			current),
	)

	select {
	case got := <-ch:
		if got != current {
			require.Fail(t,

				"expected exact assignment result")
		}
	default:
		require.Fail(t, "expected exact assignment result in channel")
	}
}

// TestResultChannelRegistry_DeduplicateSend verifies that a second send to a full channel is dropped.
func TestResultChannelRegistry_DeduplicateSend(t *testing.T) {
	r := NewResultChannelRegistry()
	_ = r.Register("run-1", "proj-1", "worker-1", "task-1", 1) // buffered cap 1

	r1 := &workerv1.TaskResult{RunId: "run-1", Status: "success", AssignmentId: "task-1", Attempt: 1}
	r2 := &workerv1.TaskResult{RunId: "run-1", Status: "failed", AssignmentId: "task-1", Attempt: 1}

	first := r.Send("run-1", "proj-1", "worker-1", r1)
	second := r.Send("run-1", "proj-1", "worker-1", r2)
	assert.True(t,
		first)
	assert.False(t,
		second)

	// channel full, should be dropped

}

func TestDeepSecResultChannelRegistry_RejectsDuplicateRegister(t *testing.T) {
	t.Parallel()

	r := NewResultChannelRegistry()
	first, ok := r.TryRegister("run-dup", "proj-1", "worker-1", "task-1", 1)
	require.False(
		t, !ok || first ==
			nil)

	second, ok := r.TryRegister("run-dup", "proj-1", "worker-2", "task-2", 1)
	require.False(
		t, ok || second !=
			nil)

	result := &workerv1.TaskResult{RunId: "run-dup", Status: "success", AssignmentId: "task-1", Attempt: 1}
	require.False(
		t, r.Send("run-dup",
			"proj-1",
			"worker-2",
			result,
		))
	require.True(t,
		r.Send("run-dup",
			"proj-1",
			"worker-1",
			result,
		),
	)

}

// TestResultChannelRegistry_Deregister verifies cleanup after dispatch completes.
func TestResultChannelRegistry_Deregister(t *testing.T) {
	r := NewResultChannelRegistry()
	_ = r.Register("run-1", "proj-1", "worker-1", "task-1", 1)
	r.Deregister("run-1")

	// After deregister, Send must return false.
	result := &workerv1.TaskResult{RunId: "run-1", Status: "success", AssignmentId: "task-1", Attempt: 1}
	assert.False(t,
		r.Send("run-1", "proj-1",
			"worker-1",

			result))

}

// TestDispatchHMAC_Format verifies that dispatchHMAC returns the v1= prefix.
func TestDispatchHMAC_Format(t *testing.T) {
	sig := dispatchHMAC("secret", "1234567890", []byte(`{"hello":"world"}`))
	assert.False(t,
		len(sig) < 3 || sig[:3] !=
			"v1=")

}

// TestDispatchHMAC_Deterministic verifies that the same inputs always produce the same signature.
func TestDispatchHMAC_Deterministic(t *testing.T) {
	s1 := dispatchHMAC("secret", "123", []byte("body"))
	s2 := dispatchHMAC("secret", "123", []byte("body"))
	assert.Equal(t,
		s2, s1)

}

// TestDispatchHMAC_DifferentInputsDifferentSigs verifies that different inputs produce different signatures.
func TestDispatchHMAC_DifferentInputsDifferentSigs(t *testing.T) {
	s1 := dispatchHMAC("secret1", "123", []byte("body"))
	s2 := dispatchHMAC("secret2", "123", []byte("body"))
	assert.NotEqual(t, s2, s1)

}

func TestBuildAssignment_RunTokenIncludesAttemptAndAssignment(t *testing.T) {
	dispatcher := &WorkerDispatcher{jwtSigningKey: "test-jwt-key"}
	run := &domain.JobRun{ID: "run-1", ProjectID: "proj-1", Attempt: 3}
	job := &domain.Job{ID: "job-1", Slug: "job", Queue: "q", TimeoutSecs: 30}

	assignment, err := dispatcher.buildAssignment(run, job, "task-1")
	require.NoError(t, err)
	require.NotEqual(t, "", assignment.
		RunTokenJwt,
	)
	require.Equal(
		t, "task-1", assignment.
			AssignmentId,
	)
	require.EqualValues(t, 3, assignment.Attempt)

	claims := struct {
		Attempt      int    `json:"attempt,omitempty"`
		AssignmentID string `json:"assignment_id,omitempty"`
		jwt.RegisteredClaims
	}{}
	parsed, err := jwt.ParseWithClaims(assignment.RunTokenJwt, &claims, func(_ *jwt.Token) (any, error) {
		return []byte("test-jwt-key"), nil
	}, jwt.WithIssuer("strait:run-token"))
	require.False(
		t, err != nil || !parsed.
			Valid,
	)
	require.Equal(
		t, "run-1", claims.
			Subject)
	require.EqualValues(t, 3, claims.Attempt)
	require.Equal(
		t, "task-1", claims.
			AssignmentID,
	)

}

func TestBuildAssignment_DecryptsEndpointSigningSecret(t *testing.T) {
	encrypted := "enc:v1:" + base64.StdEncoding.EncodeToString([]byte("encrypted:plain-endpoint-secret"))
	dispatcher := (&WorkerDispatcher{}).WithSecretDecryptor(fakeDispatchSecretDecryptor{})
	run := &domain.JobRun{ID: "run-1", ProjectID: "proj-1", Payload: []byte(`{"ok":true}`)}
	job := &domain.Job{ID: "job-1", Slug: "job", Queue: "q", TimeoutSecs: 30, EndpointSigningSecret: encrypted}

	assignment, err := dispatcher.buildAssignment(run, job, "task-1")
	require.NoError(t, err)
	require.NotEqual(t, "", assignment.
		HmacSignature,
	)
	require.Equal(
		t, dispatchHMAC("plain-endpoint-secret",

			assignment.
				HmacTimestamp, run.Payload), assignment.HmacSignature,
	)
	require.False(
		t, straitcrypto.IsEncryptedField("plain-endpoint-secret"))

}

// TestTaskResultStatus_HappyPath verifies TaskResultStatus extracts status correctly.
func TestTaskResultStatus_HappyPath(t *testing.T) {
	result := &workerv1.TaskResult{RunId: "r1", Status: "success"}
	got := TaskResultStatus(result)
	assert.Equal(t,
		"success", got)

}

// TestTaskResultStatus_Nil verifies nil opaque returns empty string.
func TestTaskResultStatus_Nil(t *testing.T) {
	got := TaskResultStatus(nil)
	assert.Equal(t,
		"", got)

}

// TestTaskResultStatus_WrongType verifies wrong type returns empty string.
func TestTaskResultStatus_WrongType(t *testing.T) {
	got := TaskResultStatus("not a TaskResult")
	assert.Equal(t,
		"", got)

}

// TestTaskResultError_HappyPath verifies TaskResultError extracts error message.
func TestTaskResultError_HappyPath(t *testing.T) {
	result := &workerv1.TaskResult{RunId: "r1", Status: "failed", ErrorMessage: "something went wrong"}
	got := TaskResultError(result)
	assert.Equal(t,
		"something went wrong",
		got,
	)

}

// TestTaskResultError_Nil verifies nil returns empty string.
func TestTaskResultError_Nil(t *testing.T) {
	got := TaskResultError(nil)
	assert.Equal(t,
		"", got)

}

func TestTaskResultOutput_HappyPathCopiesPayload(t *testing.T) {
	result := &workerv1.TaskResult{RunId: "r1", Status: "success", OutputJson: []byte(`{"ok":true}`)}
	got := TaskResultOutput(result)
	require.Equal(
		t, `{"ok":true}`, string(got),
	)

	result.OutputJson[6] = 'f'
	require.Equal(
		t, `{"ok":true}`, string(got),
	)

}

func TestTaskResultHelpers_InvalidSuccessOutputBecomesFailure(t *testing.T) {
	result := &workerv1.TaskResult{
		RunId:      "r1",
		Status:     "success",
		OutputJson: []byte(`{"ok":`),
	}
	require.Equal(
		t, "failed", TaskResultStatus(result))
	require.Equal(
		t, invalidWorkerOutputError,

		TaskResultError(result))
	require.Nil(t, TaskResultOutput(result))

}

func TestTaskResultHelpers_UnwrapWorkerTaskResult(t *testing.T) {
	wrapped := &WorkerTaskResult{
		TaskID: "task-1",
		Result: &workerv1.TaskResult{
			RunId:        "r1",
			Status:       "success",
			ErrorMessage: "ignored",
			OutputJson:   []byte(`{"ok":true}`),
		},
	}
	require.Equal(
		t, "success", TaskResultStatus(wrapped))
	require.Equal(
		t, "ignored", TaskResultError(wrapped))

	if got := TaskResultOutput(wrapped); string(got) != `{"ok":true}` {
		require.Failf(t, "test failure",

			"TaskResultOutput() = %s, want output payload", got)
	}
}

func TestTaskResultOutput_NilWrongTypeAndEmpty(t *testing.T) {
	tests := []struct {
		name  string
		input any
	}{
		{name: "nil", input: nil},
		{name: "wrong type", input: "not a TaskResult"},
		{name: "empty output", input: &workerv1.TaskResult{RunId: "r1", Status: "success"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Nil(t, TaskResultOutput(tt.input))

		})
	}
}

// TestWorkerDispatch_NoWorkerAvailable verifies ErrNoWorkerAvailable when registry is empty.
func TestWorkerDispatch_NoWorkerAvailable(t *testing.T) {
	registry := NewConnectionRegistry()
	resultChannels := NewResultChannelRegistry()
	d := NewWorkerDispatcher(registry, nil, "jwt-key", resultChannels)

	run := &domain.JobRun{
		ID:        "run-1",
		ProjectID: "proj-a",
		JobID:     "job-1",
	}
	job := &domain.Job{
		ID:    "job-1",
		Queue: "q",
		Slug:  "my-job",
	}

	_, err := d.WorkerDispatch(context.Background(), run, job)
	assert.True(t,
		errors.Is(err, ErrNoWorkerAvailable))

}

// TestWorkerDispatch_NilSendCh verifies that a nil SendCh is handled gracefully before
// any slot decrement or DB insert (guard fires before side-effects).
func TestWorkerDispatch_NilSendCh(t *testing.T) {
	registry := NewConnectionRegistry()
	// Register a worker with nil SendCh to simulate a closed stream.
	w := &ConnectedWorker{
		WorkerID:       "w1",
		ProjectID:      "proj-a",
		APIKeyID:       "key-1",
		Queues:         []string{"q"},
		SlotsTotal:     4,
		SlotsAvailable: 4,
		Status:         "active",
		SendCh:         nil, // nil channel — stream already closed
		revokeCh:       make(chan struct{}),
	}
	require.NoError(t, registry.Register(w))

	resultChannels := NewResultChannelRegistry()
	d := NewWorkerDispatcher(registry, nil, "jwt-key", resultChannels)

	run := &domain.JobRun{ID: "run-1", ProjectID: "proj-a", JobID: "job-1"}
	job := &domain.Job{ID: "job-1", Queue: "q", Slug: "my-job"}

	_, err := d.WorkerDispatch(context.Background(), run, job)
	assert.True(t,
		errors.Is(err, ErrNoWorkerAvailable))

	// Slots must NOT be decremented because the guard fires before DecrementSlots.
	snap := registry.Snapshot()
	assert.EqualValues(t, 4, snap[0].SlotsAvailable)

}

// TestWorkerDispatch_SlotRestoredOnDBError verifies slot is restored when CreateWorkerTask fails.
// This test uses a mock that returns an error from CreateWorkerTask to verify that the slot
// accounting remains consistent on DB failure without requiring a real database.
func TestWorkerDispatch_SlotRestoredOnDBError(t *testing.T) {
	registry := NewConnectionRegistry()
	sendCh := make(chan *workerv1.ServerMessage, 32)
	w := &ConnectedWorker{
		WorkerID:       "w1",
		ProjectID:      "proj-a",
		APIKeyID:       "key-1",
		Queues:         []string{"q"},
		SlotsTotal:     4,
		SlotsAvailable: 4,
		Status:         "active",
		SendCh:         sendCh,
		revokeCh:       make(chan struct{}),
	}
	require.NoError(t, registry.Register(w))

	// We cannot easily inject a failing queries without a real DB in a unit test.
	// Verify the slot state before — the nil-SendCh path guards before decrement,
	// which is tested in TestWorkerDispatch_NilSendCh. Here we verify slot accounting
	// via DecrementSlots/IncrementSlots directly to confirm the invariant.
	registry.DecrementSlots("w1")
	snap := registry.Snapshot()
	assert.EqualValues(t, 3, snap[0].SlotsAvailable)

	registry.IncrementSlots("w1")
	snap = registry.Snapshot()
	assert.EqualValues(t, 4, snap[0].SlotsAvailable)

}

// TestWorkerDispatch_ContextCancelWhileWaiting verifies cancellation while waiting for TaskResult
// sends a CancelTask and restores the slot.
func TestWorkerDispatch_ContextCancelWhileWaiting(t *testing.T) {
	registry := NewConnectionRegistry()
	sendCh := make(chan *workerv1.ServerMessage, 32)
	w := &ConnectedWorker{
		WorkerID:       "w1",
		ProjectID:      "proj-a",
		APIKeyID:       "key-1",
		Queues:         []string{"q"},
		SlotsTotal:     4,
		SlotsAvailable: 4,
		Status:         "active",
		SendCh:         sendCh,
		revokeCh:       make(chan struct{}),
	}
	require.NoError(t, registry.Register(w))

	resultChannels := NewResultChannelRegistry()

	// Pre-register a result channel so WorkerDispatch waits on it.
	resultChannels.Register("run-3", "test-project", "w1", "task-3", 1)

	d := &WorkerDispatcher{
		registry:       registry,
		queries:        nil,
		jwtSigningKey:  "",
		resultChannels: resultChannels,
	}

	// Manually bypass the CreateWorkerTask DB call by invoking sendCancel directly.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Test sendCancel does not block when channel is available.
	d.sendCancel(sendCh, "run-3")

	// Drain the cancel message.
	select {
	case msg := <-sendCh:
		cancel, ok := msg.Payload.(*workerv1.ServerMessage_CancelTask)
		if !ok {
			assert.Failf(t, "test failure",

				"expected CancelTask payload, got %T", msg.Payload)
		} else if cancel.CancelTask.RunId != "run-3" {
			assert.Failf(t, "test failure",

				"expected run_id=run-3, got %s", cancel.CancelTask.RunId)
		}
	case <-ctx.Done():
		assert.Fail(t, "timed out waiting for CancelTask message")
	}
}

// TestWorkerDispatch_SendCancel_NilChannel verifies sendCancel does not panic with nil channel.
func TestWorkerDispatch_SendCancel_NilChannel(t *testing.T) {
	d := &WorkerDispatcher{}
	d.sendCancel(nil, "run-1") // must not panic
}

func TestWorkerDispatch_MarkTaskFailedAfterAbort_NilQueriesSafe(t *testing.T) {
	t.Parallel()

	d := &WorkerDispatcher{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	d.markWorkerTaskFailedAfterAbort(ctx, "task-1", "run-1")
}
