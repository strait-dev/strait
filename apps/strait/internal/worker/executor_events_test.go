package worker

import (
	"testing"
	"time"

	"strait/internal/domain"
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

	if event.Type != EventCompleted {
		t.Fatalf("type = %s, want %s", event.Type, EventCompleted)
	}
	if event.Run != run {
		t.Fatal("event run did not preserve run pointer")
	}
	if event.Job != job {
		t.Fatal("event job did not preserve job pointer")
	}
	if event.FromStatus != domain.StatusExecuting {
		t.Fatalf("from = %s, want %s", event.FromStatus, domain.StatusExecuting)
	}
	if event.ToStatus != domain.StatusCompleted {
		t.Fatalf("to = %s, want %s", event.ToStatus, domain.StatusCompleted)
	}
	if event.ExecTrace != trace {
		t.Fatal("event trace did not preserve trace pointer")
	}
	if event.ExecDur != 1500*time.Millisecond {
		t.Fatalf("execDur = %s, want 1.5s", event.ExecDur)
	}
	if event.Attempt != 2 {
		t.Fatalf("attempt = %d, want 2", event.Attempt)
	}
	if event.QueueWait != 250*time.Millisecond {
		t.Fatalf("queueWait = %s, want 250ms", event.QueueWait)
	}
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

	if event.Type != EventTimedOut {
		t.Fatalf("type = %s, want %s", event.Type, EventTimedOut)
	}
	if event.Run != run {
		t.Fatal("event run did not preserve run pointer")
	}
	if event.Job != job {
		t.Fatal("event job did not preserve job pointer")
	}
	if event.FromStatus != domain.StatusExecuting {
		t.Fatalf("from = %s, want %s", event.FromStatus, domain.StatusExecuting)
	}
	if event.ToStatus != domain.StatusTimedOut {
		t.Fatalf("to = %s, want %s", event.ToStatus, domain.StatusTimedOut)
	}
	if event.ExecTrace != trace {
		t.Fatal("event trace did not preserve trace pointer")
	}
	if event.ExecDur != 0 {
		t.Fatalf("execDur = %s, want 0", event.ExecDur)
	}
	if event.Attempt != 3 {
		t.Fatalf("attempt = %d, want 3", event.Attempt)
	}
	if event.QueueWait != 300*time.Millisecond {
		t.Fatalf("queueWait = %s, want 300ms", event.QueueWait)
	}
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

	if event.Type != EventDeadLettered {
		t.Fatalf("type = %s, want %s", event.Type, EventDeadLettered)
	}
	if event.ToStatus != domain.StatusDeadLetter {
		t.Fatalf("to = %s, want %s", event.ToStatus, domain.StatusDeadLetter)
	}
	if event.Attempt != 4 {
		t.Fatalf("attempt = %d, want 4", event.Attempt)
	}
	if event.QueueWait != 0 {
		t.Fatalf("queueWait = %s, want 0", event.QueueWait)
	}
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

	if event.Type != EventRetried {
		t.Fatalf("type = %s, want %s", event.Type, EventRetried)
	}
	if event.Run != run {
		t.Fatal("event run did not preserve run pointer")
	}
	if event.Job != job {
		t.Fatal("event job did not preserve job pointer")
	}
	if event.FromStatus != domain.StatusExecuting {
		t.Fatalf("from = %s, want %s", event.FromStatus, domain.StatusExecuting)
	}
	if event.ToStatus != domain.StatusQueued {
		t.Fatalf("to = %s, want %s", event.ToStatus, domain.StatusQueued)
	}
	if event.ExecTrace != trace {
		t.Fatal("event trace did not preserve trace pointer")
	}
	if event.Attempt != 3 {
		t.Fatalf("attempt = %d, want 3", event.Attempt)
	}
	if event.QueueWait != 400*time.Millisecond {
		t.Fatalf("queueWait = %s, want 400ms", event.QueueWait)
	}
	if event.ExecDur != 0 {
		t.Fatalf("execDur = %s, want 0", event.ExecDur)
	}
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

	if event.Type != EventSystemFailed {
		t.Fatalf("type = %s, want %s", event.Type, EventSystemFailed)
	}
	if event.Run != run {
		t.Fatal("event run did not preserve run pointer")
	}
	if event.Job != nil {
		t.Fatalf("event job = %#v, want nil", event.Job)
	}
	if event.FromStatus != domain.StatusQueued {
		t.Fatalf("from = %s, want %s", event.FromStatus, domain.StatusQueued)
	}
	if event.ToStatus != domain.StatusSystemFailed {
		t.Fatalf("to = %s, want %s", event.ToStatus, domain.StatusSystemFailed)
	}
	if event.Attempt != 5 {
		t.Fatalf("attempt = %d, want 5", event.Attempt)
	}
	if event.QueueWait != 500*time.Millisecond {
		t.Fatalf("queueWait = %s, want 500ms", event.QueueWait)
	}
	if event.ExecTrace != nil {
		t.Fatalf("execTrace = %#v, want nil", event.ExecTrace)
	}
	if event.ExecDur != 0 {
		t.Fatalf("execDur = %s, want 0", event.ExecDur)
	}
}
