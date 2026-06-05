package worker

import (
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestCompletedRunEvent_UsesTransitionAndRunState(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	startedAt := createdAt.Add(250 * time.Millisecond)
	trace := &domain.ExecutionTrace{DispatchMs: 42}
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		Status:    domain.StatusCompleted,
		Attempt:   2,
		CreatedAt: createdAt,
		StartedAt: &startedAt,
	}
	job := &domain.Job{ID: "job-1"}
	transition := successfulRunTransition{
		to:      domain.StatusCompleted,
		execDur: 1500 * time.Millisecond,
	}

	event := newCompletedRunEvent(run, job, trace, transition)
	require.Equal(t,
		EventCompleted,

		event.Type)
	require.Equal(t,
		run, event.
			Run)
	require.Equal(t,
		job, event.
			Job)
	require.Equal(t,
		domain.
			StatusExecuting,
		event.FromStatus,
	)
	require.Equal(t,
		domain.
			StatusCompleted,
		event.ToStatus,
	)
	require.Equal(t,
		trace,
		event.
			ExecTrace)
	require.Equal(t,
		1500*
			time.
				Millisecond, event.ExecDur,
	)
	require.EqualValues(t, 2, event.
		Attempt)
	require.Equal(t,
		250*time.
			Millisecond, event.QueueWait,
	)

}

func TestTerminalRunEvent_UsesTerminalStatusAndRunState(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	startedAt := createdAt.Add(300 * time.Millisecond)
	trace := &domain.ExecutionTrace{DispatchMs: 24}
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		Status:    domain.StatusTimedOut,
		Attempt:   3,
		CreatedAt: createdAt,
		StartedAt: &startedAt,
	}
	job := &domain.Job{ID: "job-1"}

	event := newTerminalRunEvent(EventTimedOut, run, job, domain.StatusTimedOut, trace)
	require.Equal(t,
		EventTimedOut,

		event.Type)
	require.Equal(t,
		run, event.
			Run)
	require.Equal(t,
		job, event.
			Job)
	require.Equal(t,
		domain.
			StatusExecuting,
		event.FromStatus,
	)
	require.Equal(t,
		domain.
			StatusTimedOut,
		event.ToStatus,
	)
	require.Equal(t,
		trace,
		event.
			ExecTrace)
	require.EqualValues(t, 0, event.
		ExecDur)
	require.EqualValues(t, 3, event.
		Attempt)
	require.Equal(t,
		300*time.
			Millisecond, event.QueueWait,
	)

}

func TestTerminalRunEvent_DeadLettered(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{
		ID:      "run-1",
		JobID:   "job-1",
		Status:  domain.StatusDeadLetter,
		Attempt: 4,
	}
	job := &domain.Job{ID: "job-1"}

	event := newTerminalRunEvent(EventDeadLettered, run, job, domain.StatusDeadLetter, nil)
	require.Equal(t,
		EventDeadLettered,

		event.Type)
	require.Equal(t,
		domain.
			StatusDeadLetter,
		event.ToStatus,
	)
	require.EqualValues(t, 4, event.
		Attempt)
	require.EqualValues(t, 0, event.
		QueueWait)

}

func TestRetriedRunEvent_UsesNextAttemptAndRunState(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	startedAt := createdAt.Add(400 * time.Millisecond)
	trace := &domain.ExecutionTrace{DispatchMs: 12}
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		Status:    domain.StatusExecuting,
		Attempt:   2,
		CreatedAt: createdAt,
		StartedAt: &startedAt,
	}
	job := &domain.Job{ID: "job-1"}

	event := newRetriedRunEvent(run, job, trace)
	require.Equal(t,
		EventRetried,

		event.Type)
	require.Equal(t,
		run, event.
			Run)
	require.Equal(t,
		job, event.
			Job)
	require.Equal(t,
		domain.
			StatusExecuting,
		event.FromStatus,
	)
	require.Equal(t,
		domain.
			StatusQueued,
		event.ToStatus,
	)
	require.Equal(t,
		trace,
		event.
			ExecTrace)
	require.EqualValues(t, 3, event.
		Attempt)
	require.Equal(t,
		400*time.
			Millisecond, event.QueueWait,
	)
	require.EqualValues(t, 0, event.
		ExecDur)

}

func TestSystemFailedRunEvent_UsesTransitionAndRunState(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	startedAt := createdAt.Add(500 * time.Millisecond)
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		Status:    domain.StatusSystemFailed,
		Attempt:   5,
		CreatedAt: createdAt,
		StartedAt: &startedAt,
	}
	transition := systemFailureTransition{
		from: domain.StatusQueued,
		to:   domain.StatusSystemFailed,
	}

	event := newSystemFailedRunEvent(run, transition)
	require.Equal(t,
		EventSystemFailed,

		event.Type)
	require.Equal(t,
		run, event.
			Run)
	require.Nil(t, event.
		Job)
	require.Equal(t,
		domain.
			StatusQueued,
		event.FromStatus,
	)
	require.Equal(t,
		domain.
			StatusSystemFailed,
		event.ToStatus,
	)
	require.EqualValues(t, 5, event.
		Attempt)
	require.Equal(t,
		500*time.
			Millisecond, event.QueueWait,
	)
	require.Nil(t, event.
		ExecTrace)
	require.EqualValues(t, 0, event.
		ExecDur)

}
