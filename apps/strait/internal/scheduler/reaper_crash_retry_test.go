package scheduler

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestReaper_RetryStaleRunUsesBackoffAndKeepsWorkflowCallbackSilent(t *testing.T) {
	t.Parallel()

	var scheduledAttempt int
	var transitionedTo domain.RunStatus
	callbackCalled := false
	callback := &mockWorkflowCallback{
		onJobRunTerminalFn: func(context.Context, *domain.JobRun) error {
			callbackCalled = true
			return nil
		},
	}
	store := &mockReaperStore{
		getJobFn: func(context.Context, string) (*domain.Job, error) {
			return &domain.Job{MaxAttempts: 3}, nil
		},
		scheduleRetryFn: func(_ context.Context, runID string, at time.Time, attempt int) error {
			require.Equal(t, "run-1",
				runID,
			)
			require.True(t, at.
				After(time.
					Now()))

			scheduledAttempt = attempt
			return nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, from, to domain.RunStatus, fields map[string]any) error {
			require.Equal(t, domain.
				StatusExecuting,

				from)
			require.Equal(t, "transient",

				fields["error_class"])

			transitionedTo = to
			return nil
		},
	}
	reaper := NewReaper(store, time.Second, time.Minute, 0, 0, false, callback)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Status: domain.StatusExecuting, Attempt: 1}
	require.True(t, reaper.
		retryStaleRun(context.
			Background(), run))
	require.EqualValues(t, 2,
		scheduledAttempt,
	)
	require.Equal(t, domain.
		StatusQueued,

		transitionedTo,
	)
	require.False(t, callbackCalled)

}

func TestReaper_RetryStaleRunStopsWhenAttemptsExhausted(t *testing.T) {
	t.Parallel()

	store := &mockReaperStore{
		getJobFn: func(context.Context, string) (*domain.Job, error) {
			return &domain.Job{MaxAttempts: 2}, nil
		},
		scheduleRetryFn: func(context.Context, string, time.Time, int) error {
			require.Fail(t,

				"ScheduleRetry should not be called after attempts are exhausted")
			return nil
		},
	}
	reaper := NewReaper(store, time.Second, time.Minute, 0, 0, false, nil)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Status: domain.StatusExecuting, Attempt: 2}
	require.False(t, reaper.
		retryStaleRun(
			context.
				Background(), run))

}

func FuzzNextStaleRunRetryAt(f *testing.F) {
	for _, seed := range []int{-10, 0, 1, 2, 8, 64} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, attempt int) {
		before := time.Now()
		got := nextStaleRunRetryAt(attempt)
		require.False(t, got.
			Before(before))
		require.False(t, got.
			After(before.
				Add(
					time.
						Hour+time.Second)))

	})
}

func BenchmarkNextStaleRunRetryAt(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = nextStaleRunRetryAt(3)
	}
}
