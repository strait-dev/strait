package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	orcstore "strait/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// completeRunWithWebhook branching tests.

func TestCompleteRunWithWebhook_NoTxPool_NoWebhook(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0) // txPool is nil by default

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}
	job := &domain.Job{ID: "job-1", WebhookURL: ""}

	err := exec.completeRunWithWebhook(context.Background(), run, job,
		domain.StatusCompleted, map[string]any{"result": "ok"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(calls))
	}
	if calls[0].to != domain.StatusCompleted {
		t.Fatalf("expected Completed, got %s", calls[0].to)
	}
}

func TestCompleteRunWithWebhook_NoTxPool_WithWebhook(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0) // txPool is nil

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}
	job := &domain.Job{ID: "job-1", WebhookURL: "https://example.com/hook"}

	err := exec.completeRunWithWebhook(context.Background(), run, job,
		domain.StatusCompleted, map[string]any{"result": "ok"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Status update happens via plain path (no transaction).
	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(calls))
	}
	// Webhook is silently skipped (but warning log emitted).
}

func TestCompleteRunWithWebhook_WithTxPool_NoWebhookUsesTransaction(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	txPool := &mockTxBeginner{tx: &mockPgxTx{scanAttempt: 1}}
	exec := newCompletionTestExecutor(t, store, txPool)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}
	job := &domain.Job{ID: "job-1", WebhookURL: ""}

	err := exec.completeRunWithWebhook(context.Background(), run, job,
		domain.StatusCompleted, map[string]any{"result": "ok"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !txPool.beginCalled {
		t.Fatal("expected transaction to be started for terminal status update")
	}

	calls := store.statusUpdates()
	if len(calls) != 0 {
		t.Fatalf("expected 0 plain store calls (tx path should be used), got %d", len(calls))
	}
}

func TestCompleteRunWithWebhook_WithTxPool_WithWebhook(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	txPool := &mockTxBeginner{
		tx: &mockPgxTx{},
	}
	exec := newCompletionTestExecutor(t, store, txPool)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}
	job := &domain.Job{ID: "job-1", WebhookURL: "https://example.com/hook"}

	// The tx path will call store.New(tx).UpdateRunStatus which runs real SQL
	// against our mock tx. This will fail, but that's expected — we're testing
	// that the transaction path is entered.
	err := exec.completeRunWithWebhook(context.Background(), run, job,
		domain.StatusCompleted, map[string]any{"result": "ok"})

	if !txPool.beginCalled {
		t.Fatal("expected transaction to be started")
	}

	// The tx path executes real SQL against our mock tx, which returns a mock row
	// that fails on Scan. An error here confirms the transaction path was taken.
	if err == nil {
		t.Fatal("expected error from SQL execution inside tx (confirms tx path was taken)")
	}

	// The plain store path should NOT have been called — the tx path was used.
	calls := store.statusUpdates()
	if len(calls) != 0 {
		t.Fatalf("expected 0 plain store calls (tx path should be used), got %d", len(calls))
	}
}

func TestCompleteRunWithWebhook_TxBeginError(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	txPool := &mockTxBeginner{
		beginErr: errors.New("connection refused"),
	}
	exec := newCompletionTestExecutor(t, store, txPool)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}
	job := &domain.Job{ID: "job-1", WebhookURL: "https://example.com/hook"}

	err := exec.completeRunWithWebhook(context.Background(), run, job,
		domain.StatusCompleted, map[string]any{})
	if err == nil {
		t.Fatal("expected error from Begin failure")
	}
	if !txPool.beginCalled {
		t.Fatal("Begin should have been called")
	}
}

func TestCompleteRunWithWebhook_StoreError_Propagated(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return errors.New("db write failed")
		},
	}
	exec := newSnoozeTestExecutor(t, store, 0)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}
	job := &domain.Job{ID: "job-1", WebhookURL: ""}

	err := exec.completeRunWithWebhook(context.Background(), run, job,
		domain.StatusCompleted, map[string]any{})
	if err == nil {
		t.Fatal("expected error propagation from store")
	}
	if err.Error() != "db write failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTerminalRunCompletion_CompletedEndpointWebhook(t *testing.T) {
	t.Parallel()

	finishedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	result := json.RawMessage(`{"ok":true}`)
	run := &domain.JobRun{
		ID:     "run-1",
		JobID:  "job-1",
		Status: domain.StatusExecuting,
	}
	job := &domain.Job{
		ID:          "job-1",
		ProjectID:   "project-1",
		EndpointURL: "https://example.com/run",
		WebhookURL:  "https://example.com/hook",
	}
	fields := map[string]any{
		"finished_at": finishedAt,
		"result":      result,
	}

	completion := newTerminalRunCompletion(run, job, domain.StatusCompleted, fields)

	if completion.from != domain.StatusExecuting {
		t.Fatalf("from = %s, want %s", completion.from, domain.StatusExecuting)
	}
	if completion.to != domain.StatusCompleted {
		t.Fatalf("to = %s, want %s", completion.to, domain.StatusCompleted)
	}
	if string(completion.fields["result"].(json.RawMessage)) != string(result) {
		t.Fatalf("result field = %v, want %s", completion.fields["result"], result)
	}
	if !completion.recordEndpointSuccess {
		t.Fatal("completed endpoint run should record endpoint success")
	}
	if !completion.enqueueWebhook {
		t.Fatal("webhook URL should enqueue webhook delivery")
	}
	if completion.webhookRun.Status != domain.StatusCompleted {
		t.Fatalf("webhook run status = %s, want %s", completion.webhookRun.Status, domain.StatusCompleted)
	}
	if string(completion.webhookRun.Result) != string(result) {
		t.Fatalf("webhook result = %s, want %s", completion.webhookRun.Result, result)
	}
	if completion.webhookRun.FinishedAt == nil || !completion.webhookRun.FinishedAt.Equal(finishedAt) {
		t.Fatalf("webhook finished_at = %v, want %s", completion.webhookRun.FinishedAt, finishedAt)
	}
}

func TestTerminalRunCompletion_FailedRunSkipsEndpointSuccess(t *testing.T) {
	t.Parallel()

	finishedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	run := &domain.JobRun{
		ID:     "run-1",
		JobID:  "job-1",
		Status: domain.StatusExecuting,
	}
	job := &domain.Job{
		ID:          "job-1",
		ProjectID:   "project-1",
		EndpointURL: "https://example.com/run",
	}
	fields := map[string]any{
		"finished_at": finishedAt,
		"error":       "failed",
	}

	completion := newTerminalRunCompletion(run, job, domain.StatusDeadLetter, fields)

	if completion.recordEndpointSuccess {
		t.Fatal("failed run should not record endpoint success")
	}
	if completion.enqueueWebhook {
		t.Fatal("empty webhook URL should not enqueue webhook delivery")
	}
	if completion.webhookRun.Status != domain.StatusDeadLetter {
		t.Fatalf("webhook run status = %s, want %s", completion.webhookRun.Status, domain.StatusDeadLetter)
	}
	if completion.webhookRun.Error != "failed" {
		t.Fatalf("webhook error = %q, want failed", completion.webhookRun.Error)
	}
	if completion.webhookRun.FinishedAt == nil || !completion.webhookRun.FinishedAt.Equal(finishedAt) {
		t.Fatalf("webhook finished_at = %v, want %s", completion.webhookRun.FinishedAt, finishedAt)
	}
}

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

// Handler integration tests.

func TestHandleSuccess_EmitsCompletedEvent(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{
		getJobHealthStatsFn: func(_ context.Context, _ string, _ time.Time) (*orcstore.JobHealthStats, error) {
			return nil, nil
		},
	}
	exec := newSnoozeTestExecutor(t, store, 0)
	getEvents := collectEvents(exec)
	t.Cleanup(func() { close(exec.eventCh) })

	run := testRun(1)
	run.Status = domain.StatusExecuting
	job := testJob("http://localhost", 3, 30)

	exec.handleSuccess(context.Background(), run, job, nil)

	calls := store.statusUpdates()
	if len(calls) == 0 {
		t.Fatal("expected at least one status update")
	}
	found := false
	for _, c := range calls {
		if c.to == domain.StatusCompleted {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected a transition to Completed")
	}

	events := getEvents()
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}
	if events[0].Type != EventCompleted {
		t.Fatalf("expected EventCompleted, got %s", events[0].Type)
	}
}

func TestHandleFailure_DeadLetter_AtMaxAttempts(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)
	getEvents := collectEvents(exec)
	t.Cleanup(func() { close(exec.eventCh) })

	run := testRun(3) // At max attempts.
	run.Status = domain.StatusExecuting
	job := testJob("http://localhost", 3, 30) // MaxAttempts=3.
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	exec.handleFailure(context.Background(), run, job, policy, errors.New("server error"), nil)

	calls := store.statusUpdates()
	found := false
	for _, c := range calls {
		if c.to == domain.StatusDeadLetter {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected a transition to DeadLetter at max attempts")
	}

	events := getEvents()
	foundDL := false
	for _, ev := range events {
		if ev.Type == EventDeadLettered {
			foundDL = true
			break
		}
	}
	if !foundDL {
		t.Fatal("expected EventDeadLettered")
	}
}

func TestHandleTimeout_Terminal_AtMaxAttempts(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)
	getEvents := collectEvents(exec)
	t.Cleanup(func() { close(exec.eventCh) })

	run := testRun(3)
	run.Status = domain.StatusExecuting
	job := testJob("http://localhost", 3, 30)
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	exec.handleTimeout(context.Background(), run, job, policy, nil)

	calls := store.statusUpdates()
	found := false
	for _, c := range calls {
		if c.to == domain.StatusTimedOut {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected a transition to TimedOut at max attempts")
	}

	events := getEvents()
	foundTO := false
	for _, ev := range events {
		if ev.Type == EventTimedOut {
			foundTO = true
			break
		}
	}
	if !foundTO {
		t.Fatal("expected EventTimedOut")
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

// Test helpers.

func newCompletionTestExecutor(t *testing.T, s *mockExecutorStore, txPool *mockTxBeginner) *Executor {
	t.Helper()
	pool := NewPool(4)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	return NewExecutor(ExecutorConfig{
		Pool:         pool,
		Queue:        &mockExecQueue{},
		Store:        s,
		PollInterval: time.Millisecond,
		TxPool:       txPool,
	})
}

// mockTxBeginner tracks whether Begin was called and returns a mock tx.
type mockTxBeginner struct {
	beginCalled bool
	beginErr    error
	tx          *mockPgxTx
}

func (m *mockTxBeginner) Begin(_ context.Context) (pgx.Tx, error) {
	m.beginCalled = true
	if m.beginErr != nil {
		return nil, m.beginErr
	}
	if m.tx == nil {
		return &mockPgxTx{}, nil
	}
	return m.tx, nil
}

// mockPgxTx is a minimal pgx.Tx implementation for testing the transaction path.
type mockPgxTx struct {
	scanAttempt int
}

func (m *mockPgxTx) Begin(_ context.Context) (pgx.Tx, error) {
	return &mockPgxTx{scanAttempt: m.scanAttempt}, nil
}
func (m *mockPgxTx) Commit(_ context.Context) error { return nil }

func (m *mockPgxTx) Rollback(_ context.Context) error { return nil }
func (m *mockPgxTx) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, _ pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (m *mockPgxTx) SendBatch(_ context.Context, _ *pgx.Batch) pgx.BatchResults {
	return nil
}
func (m *mockPgxTx) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}
func (m *mockPgxTx) Prepare(_ context.Context, _, _ string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (m *mockPgxTx) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func (m *mockPgxTx) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, errors.New("mock tx: query not implemented")
}
func (m *mockPgxTx) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return &mockRow{attempt: m.scanAttempt}
}
func (m *mockPgxTx) Conn() *pgx.Conn {
	return nil
}

// mockRow satisfies pgx.Row for QueryRow in the mock tx.
type mockRow struct {
	attempt int
}

func (m *mockRow) Scan(dest ...any) error {
	if m.attempt > 0 && len(dest) > 0 {
		if p, ok := dest[0].(*int); ok {
			*p = m.attempt
			return nil
		}
	}
	return errors.New("mock row: not implemented")
}
