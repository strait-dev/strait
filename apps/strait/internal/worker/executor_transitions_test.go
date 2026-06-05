package worker

import (
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestSystemFailureTransition_PreservesSourceStatus(t *testing.T) {
	t.Parallel()

	finishedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	run := &domain.JobRun{
		ID:     "run-1",
		JobID:  "job-1",
		Status: domain.StatusQueued,
	}

	transition := newSystemFailureTransition(run, "pool unavailable", finishedAt)
	require.Equal(t, domain.
		StatusQueued,
		transition.
			from,
	)
	require.Equal(t, domain.
		StatusSystemFailed,

		transition.
			to)
	require.True(t, transition.
		finished.
		Equal(finishedAt),
	)
	require.Equal(t, finishedAt,
		transition.
			fields["finished_at"])
	require.Equal(t, "pool unavailable",

		transition.
			fields["error"])
	require.Equal(t, domain.
		ErrorClassServer,
		transition.
			fields["error_class"])
}

func TestSuccessfulRunTransition_WithResultTraceAndDuration(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	finishedAt := startedAt.Add(1500 * time.Millisecond)
	result := json.RawMessage(`{"ok":true}`)
	trace := &domain.ExecutionTrace{DispatchMs: 42}
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		Status:    domain.StatusExecuting,
		StartedAt: &startedAt,
	}
	exec := &Executor{executionTraceMode: executionTraceFull}

	transition := exec.newSuccessfulRunTransition(run, result, trace, finishedAt)
	require.Equal(t, domain.
		StatusCompleted,
		transition.
			to,
	)
	require.True(t, transition.
		finished.
		Equal(finishedAt),
	)
	require.Equal(t, 1500*
		time.Millisecond,
		transition.
			execDur)
	require.True(t, transition.
		started,
	)
	require.Equal(t, finishedAt,
		transition.
			fields["finished_at"])
	require.Equal(t, string(result),
		string(transition.
			fields["result"].(json.RawMessage)))
	require.Equal(t, trace,
		transition.
			fields["execution_trace"])
}

func TestSuccessfulRunTransition_EmptyResultSkipsOptionalFields(t *testing.T) {
	t.Parallel()

	finishedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	trace := &domain.ExecutionTrace{DispatchMs: 42}
	run := &domain.JobRun{
		ID:     "run-1",
		JobID:  "job-1",
		Status: domain.StatusExecuting,
	}
	exec := &Executor{executionTraceMode: executionTraceOff}

	transition := exec.newSuccessfulRunTransition(run, nil, trace, finishedAt)
	require.EqualValues(t, 0, transition.
		execDur,
	)
	require.False(t, transition.
		started,
	)

	if _, ok := transition.fields["result"]; ok {
		require.Fail(t,

			"empty result should not be persisted")
	}
	if _, ok := transition.fields["execution_trace"]; ok {
		require.Fail(t,

			"trace mode off should not persist execution_trace")
	}
}

func TestTimeoutRunTransition_Retry(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{
		ID:       "run-1",
		JobID:    "job-1",
		Attempt:  1,
		Priority: 4,
	}
	job := &domain.Job{
		ID:                 "job-1",
		RetryPriorityBoost: 2,
	}
	policy := executionPolicy{
		maxAttempts:      3,
		retryBackoff:     domain.RetryBackoffFixed,
		retryInitialSecs: 1,
		retryMaxSecs:     30,
	}

	before := time.Now()
	transition := newTimeoutRunTransition(run, job, policy, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	require.True(t, transition.
		retry)
	require.False(t, transition.
		retryAt.
		Before(before))
	require.EqualValues(t, 2, transition.
		fields["attempt"])
	require.Equal(t, executionTimedOutError,

		transition.
			fields["error"])
	require.Equal(t, domain.
		ErrorClassTransient,

		transition.
			fields["error_class"])
	require.EqualValues(t, 6, transition.
		fields["priority"])
	require.Nil(t, transition.
		fields["started_at"],
	)
	require.Nil(t, transition.
		fields["finished_at"])
}

func TestTimeoutRunTransition_Terminal(t *testing.T) {
	t.Parallel()

	finishedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	run := &domain.JobRun{
		ID:       "run-1",
		JobID:    "job-1",
		Attempt:  3,
		Priority: 4,
	}
	job := &domain.Job{
		ID:                 "job-1",
		RetryPriorityBoost: 2,
	}
	policy := executionPolicy{maxAttempts: 3}

	transition := newTimeoutRunTransition(run, job, policy, finishedAt)
	require.False(t, transition.
		retry,
	)
	require.True(t, transition.
		retryAt.
		IsZero())
	require.Equal(t, finishedAt,
		transition.
			fields["finished_at"])
	require.Equal(t, executionTimedOutError,

		transition.
			fields["error"])
	require.Equal(t, domain.
		ErrorClassTransient,

		transition.
			fields["error_class"])

	if _, ok := transition.fields["priority"]; ok {
		require.Fail(t,

			"terminal timeout transition should not set retry priority")
	}
	if _, ok := transition.fields["attempt"]; ok {
		require.Fail(t,

			"terminal timeout transition should not advance attempt")
	}
}

func TestFailureRunTransition_RetryTracksPoisonMetadata(t *testing.T) {
	t.Parallel()

	threshold := 3
	errInput := &domain.EndpointError{StatusCode: 500, Body: "db down"}
	run := &domain.JobRun{
		ID:       "run-1",
		JobID:    "job-1",
		Attempt:  1,
		Priority: 4,
	}
	job := &domain.Job{
		ID:                  "job-1",
		RetryPriorityBoost:  2,
		PoisonPillThreshold: &threshold,
	}
	policy := executionPolicy{
		maxAttempts:      3,
		retryBackoff:     domain.RetryBackoffFixed,
		retryInitialSecs: 1,
		retryMaxSecs:     30,
	}

	before := time.Now()
	transition := newFailureRunTransition(
		run,
		job,
		policy,
		errInput,
		errInput.Error(),
		domain.ErrorClassServer,
		time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	)
	require.True(t, transition.
		retry)
	require.False(t, transition.
		retryAt.
		Before(before))
	require.Nil(t, transition.
		poisonPill)
	require.Equal(t, errInput.
		Error(),
		transition.
			errMsg,
	)
	require.Equal(t, domain.
		ErrorClassServer,
		transition.
			errClass)
	require.EqualValues(t, 2, transition.
		fields["attempt"])
	require.EqualValues(t, 6, transition.
		fields["priority"])

	meta, ok := transition.fields["metadata"].(map[string]string)
	require.True(t, ok)
	require.Equal(t, errorHashForError(errInput),
		meta["_error_hash"])
	require.Equal(t, "1",
		meta["_error_hash_count"])
}

func TestFailureRunTransition_PoisonPillTerminal(t *testing.T) {
	t.Parallel()

	threshold := 3
	finishedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	errInput := &domain.EndpointError{StatusCode: 500, Body: "db down"}
	run := &domain.JobRun{
		ID:       "run-1",
		JobID:    "job-1",
		Attempt:  2,
		Priority: 4,
		Metadata: map[string]string{
			"_error_hash":       errorHashForError(errInput),
			"_error_hash_count": "2",
		},
	}
	job := &domain.Job{
		ID:                  "job-1",
		RetryPriorityBoost:  2,
		PoisonPillThreshold: &threshold,
	}
	policy := executionPolicy{maxAttempts: 5}

	transition := newFailureRunTransition(run, job, policy, errInput, errInput.Error(), domain.ErrorClassServer, finishedAt)
	require.False(t, transition.
		retry,
	)
	require.NotNil(t, transition.
		poisonPill,
	)
	require.Equal(t, 3, transition.
		poisonPill.
		count,
	)
	require.Equal(t, threshold,
		transition.
			poisonPill.
			threshold,
	)
	require.Contains(t, transition.
		errMsg, "poison pill detected (same error 3 times)")
	require.Equal(t, finishedAt,
		transition.
			fields["finished_at"])

	if _, ok := transition.fields["attempt"]; ok {
		require.Fail(t,

			"poison pill terminal transition should not advance attempt")
	}
	if _, ok := transition.fields["priority"]; ok {
		require.Fail(t,

			"poison pill terminal transition should not set retry priority")
	}
	meta, ok := transition.fields["metadata"].(map[string]string)
	require.True(t, ok)
	require.Equal(t, "3",
		meta["_error_hash_count"])
}

func TestFailureRunTransition_NonRetryableSkipsPoisonMetadata(t *testing.T) {
	t.Parallel()

	threshold := 3
	errInput := &domain.EndpointError{StatusCode: 400, Body: "bad request"}
	run := &domain.JobRun{
		ID:      "run-1",
		JobID:   "job-1",
		Attempt: 1,
	}
	job := &domain.Job{
		ID:                  "job-1",
		PoisonPillThreshold: &threshold,
	}
	policy := executionPolicy{maxAttempts: 3}
	finishedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	transition := newFailureRunTransition(run, job, policy, errInput, errInput.Error(), domain.ErrorClassClient, finishedAt)
	require.False(t, transition.
		retry,
	)
	require.Nil(t, transition.
		poisonPill)
	require.Nil(t, run.Metadata)

	if _, ok := transition.fields["metadata"]; ok {
		require.Fail(t,

			"non-retryable error should not write poison metadata")
	}
	require.Equal(t, finishedAt,
		transition.
			fields["finished_at"])
	require.Equal(t, errInput.
		Error(),
		transition.
			fields["error"])
}
