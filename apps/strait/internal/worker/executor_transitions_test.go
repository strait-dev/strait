package worker

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
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

	if transition.from != domain.StatusQueued {
		t.Fatalf("from = %s, want %s", transition.from, domain.StatusQueued)
	}
	if transition.to != domain.StatusSystemFailed {
		t.Fatalf("to = %s, want %s", transition.to, domain.StatusSystemFailed)
	}
	if !transition.finished.Equal(finishedAt) {
		t.Fatalf("finished = %s, want %s", transition.finished, finishedAt)
	}
	if transition.fields["finished_at"] != finishedAt {
		t.Fatalf("finished_at field = %v, want %s", transition.fields["finished_at"], finishedAt)
	}
	if transition.fields["error"] != "pool unavailable" {
		t.Fatalf("error field = %v, want pool unavailable", transition.fields["error"])
	}
	if transition.fields["error_class"] != domain.ErrorClassServer {
		t.Fatalf("error_class field = %v, want %s", transition.fields["error_class"], domain.ErrorClassServer)
	}
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

	if transition.to != domain.StatusCompleted {
		t.Fatalf("to = %s, want %s", transition.to, domain.StatusCompleted)
	}
	if !transition.finished.Equal(finishedAt) {
		t.Fatalf("finished = %s, want %s", transition.finished, finishedAt)
	}
	if transition.execDur != 1500*time.Millisecond {
		t.Fatalf("execDur = %s, want 1.5s", transition.execDur)
	}
	if !transition.started {
		t.Fatal("started = false, want true")
	}
	if transition.fields["finished_at"] != finishedAt {
		t.Fatalf("finished_at field = %v, want %s", transition.fields["finished_at"], finishedAt)
	}
	if string(transition.fields["result"].(json.RawMessage)) != string(result) {
		t.Fatalf("result field = %v, want %s", transition.fields["result"], result)
	}
	if transition.fields["execution_trace"] != trace {
		t.Fatalf("execution_trace field = %v, want trace pointer", transition.fields["execution_trace"])
	}
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

	if transition.execDur != 0 {
		t.Fatalf("execDur = %s, want 0", transition.execDur)
	}
	if transition.started {
		t.Fatal("started = true, want false")
	}
	if _, ok := transition.fields["result"]; ok {
		t.Fatal("empty result should not be persisted")
	}
	if _, ok := transition.fields["execution_trace"]; ok {
		t.Fatal("trace mode off should not persist execution_trace")
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

	if !transition.retry {
		t.Fatal("expected timeout transition to retry")
	}
	if transition.retryAt.Before(before) {
		t.Fatalf("retryAt = %s, want after %s", transition.retryAt, before)
	}
	if transition.fields["attempt"] != 2 {
		t.Fatalf("attempt field = %v, want 2", transition.fields["attempt"])
	}
	if transition.fields["error"] != executionTimedOutError {
		t.Fatalf("error field = %v, want %q", transition.fields["error"], executionTimedOutError)
	}
	if transition.fields["error_class"] != domain.ErrorClassTransient {
		t.Fatalf("error_class field = %v, want %q", transition.fields["error_class"], domain.ErrorClassTransient)
	}
	if transition.fields["priority"] != 6 {
		t.Fatalf("priority field = %v, want 6", transition.fields["priority"])
	}
	if transition.fields["started_at"] != nil {
		t.Fatalf("started_at field = %v, want nil", transition.fields["started_at"])
	}
	if transition.fields["finished_at"] != nil {
		t.Fatalf("finished_at field = %v, want nil", transition.fields["finished_at"])
	}
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

	if transition.retry {
		t.Fatal("expected terminal timeout transition")
	}
	if !transition.retryAt.IsZero() {
		t.Fatalf("retryAt = %s, want zero time", transition.retryAt)
	}
	if transition.fields["finished_at"] != finishedAt {
		t.Fatalf("finished_at field = %v, want %s", transition.fields["finished_at"], finishedAt)
	}
	if transition.fields["error"] != executionTimedOutError {
		t.Fatalf("error field = %v, want %q", transition.fields["error"], executionTimedOutError)
	}
	if transition.fields["error_class"] != domain.ErrorClassTransient {
		t.Fatalf("error_class field = %v, want %q", transition.fields["error_class"], domain.ErrorClassTransient)
	}
	if _, ok := transition.fields["priority"]; ok {
		t.Fatal("terminal timeout transition should not set retry priority")
	}
	if _, ok := transition.fields["attempt"]; ok {
		t.Fatal("terminal timeout transition should not advance attempt")
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

	if !transition.retry {
		t.Fatal("expected failure transition to retry")
	}
	if transition.retryAt.Before(before) {
		t.Fatalf("retryAt = %s, want after %s", transition.retryAt, before)
	}
	if transition.poisonPill != nil {
		t.Fatal("first tracked failure should not trip poison pill")
	}
	if transition.errMsg != errInput.Error() {
		t.Fatalf("errMsg = %q, want %q", transition.errMsg, errInput.Error())
	}
	if transition.errClass != domain.ErrorClassServer {
		t.Fatalf("errClass = %q, want %q", transition.errClass, domain.ErrorClassServer)
	}
	if transition.fields["attempt"] != 2 {
		t.Fatalf("attempt field = %v, want 2", transition.fields["attempt"])
	}
	if transition.fields["priority"] != 6 {
		t.Fatalf("priority field = %v, want 6", transition.fields["priority"])
	}
	meta, ok := transition.fields["metadata"].(map[string]string)
	if !ok {
		t.Fatalf("metadata field type = %T, want map[string]string", transition.fields["metadata"])
	}
	if meta["_error_hash"] != errorHashForError(errInput) {
		t.Fatalf("_error_hash = %q, want %q", meta["_error_hash"], errorHashForError(errInput))
	}
	if meta["_error_hash_count"] != "1" {
		t.Fatalf("_error_hash_count = %q, want 1", meta["_error_hash_count"])
	}
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

	if transition.retry {
		t.Fatal("expected poison pill transition to be terminal")
	}
	if transition.poisonPill == nil {
		t.Fatal("expected poison pill detection details")
	}
	if transition.poisonPill.count != 3 {
		t.Fatalf("poison count = %d, want 3", transition.poisonPill.count)
	}
	if transition.poisonPill.threshold != threshold {
		t.Fatalf("poison threshold = %d, want %d", transition.poisonPill.threshold, threshold)
	}
	if !strings.Contains(transition.errMsg, "poison pill detected (same error 3 times)") {
		t.Fatalf("errMsg = %q, want poison pill message", transition.errMsg)
	}
	if transition.fields["finished_at"] != finishedAt {
		t.Fatalf("finished_at field = %v, want %s", transition.fields["finished_at"], finishedAt)
	}
	if _, ok := transition.fields["attempt"]; ok {
		t.Fatal("poison pill terminal transition should not advance attempt")
	}
	if _, ok := transition.fields["priority"]; ok {
		t.Fatal("poison pill terminal transition should not set retry priority")
	}
	meta, ok := transition.fields["metadata"].(map[string]string)
	if !ok {
		t.Fatalf("metadata field type = %T, want map[string]string", transition.fields["metadata"])
	}
	if meta["_error_hash_count"] != "3" {
		t.Fatalf("_error_hash_count = %q, want 3", meta["_error_hash_count"])
	}
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

	if transition.retry {
		t.Fatal("expected non-retryable client error to be terminal")
	}
	if transition.poisonPill != nil {
		t.Fatal("non-retryable error should not trip poison pill")
	}
	if run.Metadata != nil {
		t.Fatalf("run metadata = %#v, want nil", run.Metadata)
	}
	if _, ok := transition.fields["metadata"]; ok {
		t.Fatal("non-retryable error should not write poison metadata")
	}
	if transition.fields["finished_at"] != finishedAt {
		t.Fatalf("finished_at field = %v, want %s", transition.fields["finished_at"], finishedAt)
	}
	if transition.fields["error"] != errInput.Error() {
		t.Fatalf("error field = %v, want %q", transition.fields["error"], errInput.Error())
	}
}
