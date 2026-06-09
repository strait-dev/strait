package worker

import (
	"time"

	"strait/internal/domain"
)

func runQueueWaitUntil(run *domain.JobRun, end time.Time) time.Duration {
	if run == nil {
		return 0
	}
	if run.CreatedAt.IsZero() {
		return 0
	}
	if end.IsZero() {
		return 0
	}
	return nonNegativeDuration(end.Sub(run.CreatedAt))
}

func runStartedQueueWait(run *domain.JobRun) time.Duration {
	if run == nil || run.StartedAt == nil {
		return 0
	}
	return runQueueWaitUntil(run, *run.StartedAt)
}

func runDequeueDurationUntil(run *domain.JobRun, end time.Time) time.Duration {
	if run == nil {
		return 0
	}
	if run.StartedAt == nil {
		return 0
	}
	if end.IsZero() {
		return 0
	}
	return nonNegativeDuration(end.Sub(*run.StartedAt))
}

func nonNegativeDuration(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	return d
}

func durationMillisecondsAtLeastOne(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	ms := d.Milliseconds()
	if ms == 0 {
		return 1
	}
	return ms
}
