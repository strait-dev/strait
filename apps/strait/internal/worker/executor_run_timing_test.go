package worker

import (
	"testing"
	"time"

	"strait/internal/domain"
)

func TestRunTimingHelpers(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	startedAt := createdAt.Add(250 * time.Millisecond)
	run := &domain.JobRun{
		CreatedAt: createdAt,
		StartedAt: &startedAt,
	}

	if got := runQueueWaitUntil(run, createdAt.Add(400*time.Millisecond)); got != 400*time.Millisecond {
		t.Fatalf("runQueueWaitUntil() = %s, want 400ms", got)
	}
	if got := runStartedQueueWait(run); got != 250*time.Millisecond {
		t.Fatalf("runStartedQueueWait() = %s, want 250ms", got)
	}
	if got := runDequeueDurationUntil(run, createdAt.Add(425*time.Millisecond)); got != 175*time.Millisecond {
		t.Fatalf("runDequeueDurationUntil() = %s, want 175ms", got)
	}
}

func TestRunTimingHelpersHandleMissingTimes(t *testing.T) {
	t.Parallel()

	if got := runQueueWaitUntil(nil, time.Now()); got != 0 {
		t.Fatalf("nil run queue wait = %s, want 0", got)
	}
	if got := runStartedQueueWait(&domain.JobRun{}); got != 0 {
		t.Fatalf("missing started_at queue wait = %s, want 0", got)
	}
	if got := runDequeueDurationUntil(&domain.JobRun{}, time.Now()); got != 0 {
		t.Fatalf("missing started_at dequeue = %s, want 0", got)
	}

	startedAt := time.Now()
	run := &domain.JobRun{StartedAt: &startedAt}
	if got := runQueueWaitUntil(run, time.Now()); got != 0 {
		t.Fatalf("zero created_at queue wait = %s, want 0", got)
	}
	if got := runDequeueDurationUntil(run, time.Time{}); got != 0 {
		t.Fatalf("zero end dequeue = %s, want 0", got)
	}
}

func TestRunTimingHelpersClampClockSkew(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	startedAt := createdAt.Add(-time.Second)
	run := &domain.JobRun{
		CreatedAt: createdAt,
		StartedAt: &startedAt,
	}

	if got := runQueueWaitUntil(run, createdAt.Add(-time.Millisecond)); got != 0 {
		t.Fatalf("negative queue wait = %s, want 0", got)
	}
	if got := runStartedQueueWait(run); got != 0 {
		t.Fatalf("negative started queue wait = %s, want 0", got)
	}
	if got := runDequeueDurationUntil(run, startedAt.Add(-time.Millisecond)); got != 0 {
		t.Fatalf("negative dequeue = %s, want 0", got)
	}
}
