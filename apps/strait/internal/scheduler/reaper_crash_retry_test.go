package scheduler

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
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
			if runID != "run-1" {
				t.Fatalf("scheduled run id = %s, want run-1", runID)
			}
			if !at.After(time.Now()) {
				t.Fatalf("retry time = %s, want future", at)
			}
			scheduledAttempt = attempt
			return nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, from, to domain.RunStatus, fields map[string]any) error {
			if from != domain.StatusExecuting {
				t.Fatalf("from = %s, want executing", from)
			}
			if fields["error_class"] != "transient" {
				t.Fatalf("error_class = %v, want transient", fields["error_class"])
			}
			transitionedTo = to
			return nil
		},
	}
	reaper := NewReaper(store, time.Second, time.Minute, 0, 0, false, callback)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Status: domain.StatusExecuting, Attempt: 1}

	if !reaper.retryStaleRun(context.Background(), run) {
		t.Fatal("retryStaleRun() = false, want true")
	}
	if scheduledAttempt != 2 {
		t.Fatalf("scheduled attempt = %d, want 2", scheduledAttempt)
	}
	if transitionedTo != domain.StatusQueued {
		t.Fatalf("transitioned to = %s, want queued", transitionedTo)
	}
	if callbackCalled {
		t.Fatal("workflow callback should not fire for non-terminal retry")
	}
}

func TestReaper_RetryStaleRunStopsWhenAttemptsExhausted(t *testing.T) {
	t.Parallel()

	store := &mockReaperStore{
		getJobFn: func(context.Context, string) (*domain.Job, error) {
			return &domain.Job{MaxAttempts: 2}, nil
		},
		scheduleRetryFn: func(context.Context, string, time.Time, int) error {
			t.Fatal("ScheduleRetry should not be called after attempts are exhausted")
			return nil
		},
	}
	reaper := NewReaper(store, time.Second, time.Minute, 0, 0, false, nil)
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Status: domain.StatusExecuting, Attempt: 2}

	if reaper.retryStaleRun(context.Background(), run) {
		t.Fatal("retryStaleRun() = true, want false")
	}
}

func FuzzNextStaleRunRetryAt(f *testing.F) {
	for _, seed := range []int{-10, 0, 1, 2, 8, 64} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, attempt int) {
		before := time.Now()
		got := nextStaleRunRetryAt(attempt)
		if got.Before(before) {
			t.Fatalf("retry time %s is before %s", got, before)
		}
		if got.After(before.Add(time.Hour + time.Second)) {
			t.Fatalf("retry time %s exceeds one-hour cap from %s", got, before)
		}
	})
}

func BenchmarkNextStaleRunRetryAt(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = nextStaleRunRetryAt(3)
	}
}
