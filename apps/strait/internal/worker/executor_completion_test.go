package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	orcstore "strait/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
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
	require.NoError(
		t, err)

	calls := store.statusUpdates()
	require.Len(t, calls,
		1)
	require.Equal(t,
		domain.StatusCompleted,
		calls[0].to)
}

func TestCompleteRunWithWebhook_NoTxPool_WithWebhook(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0) // txPool is nil

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}
	job := &domain.Job{ID: "job-1", WebhookURL: "https://example.com/hook"}

	err := exec.completeRunWithWebhook(context.Background(), run, job,
		domain.StatusCompleted, map[string]any{"result": "ok"})
	require.NoError(
		t, err)

	// Status update happens via plain path (no transaction).
	calls := store.statusUpdates()
	require.Len(t, calls,
		1)

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
	require.NoError(
		t, err)
	require.True(t,
		txPool.beginCalled,
	)

	calls := store.statusUpdates()
	require.Empty(t, calls)
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
	require.True(t,
		txPool.beginCalled,
	)
	require.Error(t,
		err)

	// The tx path executes real SQL against our mock tx, which returns a mock row
	// that fails on Scan. An error here confirms the transaction path was taken.

	// The plain store path should NOT have been called — the tx path was used.
	calls := store.statusUpdates()
	require.Empty(t, calls)
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
	require.Error(t,
		err)
	require.True(t,
		txPool.beginCalled,
	)
}

func TestCompleteRunWithWebhook_WithTxPool_RecordsEndpointSuccess(t *testing.T) {
	t.Parallel()

	tx := &mockPgxTx{scanAttempt: 1}
	exec := newCompletionTestExecutor(t, &mockExecutorStore{}, &mockTxBeginner{tx: tx})

	run := &domain.JobRun{
		ID:        "run-endpoint-success",
		JobID:     "job-endpoint-success",
		ProjectID: "project-endpoint-success",
		Status:    domain.StatusExecuting,
	}
	job := &domain.Job{
		ID:          run.JobID,
		ProjectID:   run.ProjectID,
		EndpointURL: "https://example.com/run",
	}

	err := exec.completeRunWithWebhook(context.Background(), run, job, domain.StatusCompleted, map[string]any{})
	require.NoError(t, err)
	require.Equal(t, 1, tx.circuitSuccessCalls)
}

func TestCompleteRunWithWebhook_WithTxPool_EndpointSuccessError(t *testing.T) {
	t.Parallel()

	tx := &mockPgxTx{
		scanAttempt:       1,
		circuitSuccessErr: errors.New("circuit update failed"),
	}
	exec := newCompletionTestExecutor(t, &mockExecutorStore{}, &mockTxBeginner{tx: tx})

	run := &domain.JobRun{
		ID:        "run-endpoint-success-error",
		JobID:     "job-endpoint-success-error",
		ProjectID: "project-endpoint-success-error",
		Status:    domain.StatusExecuting,
	}
	job := &domain.Job{
		ID:          run.JobID,
		ProjectID:   run.ProjectID,
		EndpointURL: "https://example.com/run",
	}

	err := exec.completeRunWithWebhook(context.Background(), run, job, domain.StatusCompleted, map[string]any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "record endpoint circuit success")
	require.Equal(t, 1, tx.circuitSuccessCalls)
}

func TestCompleteRunWithWebhook_WithTxPool_RunWebhookEnqueueError(t *testing.T) {
	t.Parallel()

	tx := &mockPgxTx{
		scanAttempt:       1,
		enqueueWebhookErr: errors.New("enqueue failed"),
	}
	exec := newCompletionTestExecutor(t, &mockExecutorStore{}, &mockTxBeginner{tx: tx})

	run := &domain.JobRun{
		ID:        "run-webhook-error",
		JobID:     "job-webhook-error",
		ProjectID: "project-webhook-error",
		Status:    domain.StatusExecuting,
	}
	job := &domain.Job{
		ID:         run.JobID,
		ProjectID:  run.ProjectID,
		WebhookURL: "https://example.com/run-webhook",
	}

	err := exec.completeRunWithWebhook(context.Background(), run, job, domain.StatusCompleted, map[string]any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "enqueue run webhook")
}

func TestCompleteRunWithWebhook_WithTxPool_RunSubscriptionListError(t *testing.T) {
	t.Parallel()

	tx := &mockPgxTx{
		scanAttempt:          1,
		listSubscriptionsErr: errors.New("list subscriptions failed"),
	}
	exec := newCompletionTestExecutor(t, &mockExecutorStore{}, &mockTxBeginner{tx: tx})

	run := &domain.JobRun{
		ID:        "run-subscription-list-error",
		JobID:     "job-subscription-list-error",
		ProjectID: "project-subscription-list-error",
		Status:    domain.StatusExecuting,
	}
	job := &domain.Job{ID: run.JobID, ProjectID: run.ProjectID}

	err := exec.completeRunWithWebhook(context.Background(), run, job, domain.StatusCompleted, map[string]any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "list run webhook subscriptions")
}

func TestCompleteRunWithWebhook_WithTxPool_RunSubscriptionPayloadError(t *testing.T) {
	t.Parallel()

	tx := &mockPgxTx{scanAttempt: 1}
	exec := newCompletionTestExecutor(t, &mockExecutorStore{}, &mockTxBeginner{tx: tx})

	run := &domain.JobRun{
		ID:        "run-subscription-payload-error",
		JobID:     "job-subscription-payload-error",
		ProjectID: "project-subscription-payload-error",
		Status:    domain.StatusExecuting,
	}
	job := &domain.Job{ID: run.JobID, ProjectID: run.ProjectID}

	err := exec.completeRunWithWebhookOnce(
		context.Background(),
		run,
		job,
		terminalRunCompletion{
			from:       domain.StatusExecuting,
			to:         domain.StatusCompleted,
			fields:     map[string]any{},
			webhookRun: &domain.JobRun{ID: run.ID, JobID: run.JobID, ProjectID: run.ProjectID, Status: domain.StatusCompleted, Result: json.RawMessage(`{`)},
		},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "marshal run subscription webhook payload")
}

func TestCompleteRunWithWebhook_WithTxPool_RunSubscriptionDeliveryError(t *testing.T) {
	t.Parallel()

	tx := &mockPgxTx{
		scanAttempt:              1,
		createWebhookDeliveryErr: errors.New("create delivery failed"),
		subscriptions: []domain.WebhookSubscription{{
			ID:         "sub-1",
			ProjectID:  "project-subscription-delivery-error",
			WebhookURL: "https://example.com/subscription",
			EventTypes: []string{domain.WebhookEventRunCompleted},
			Active:     true,
			CreatedAt:  time.Now(),
		}},
	}
	exec := newCompletionTestExecutor(t, &mockExecutorStore{}, &mockTxBeginner{tx: tx})

	run := &domain.JobRun{
		ID:        "run-subscription-delivery-error",
		JobID:     "job-subscription-delivery-error",
		ProjectID: "project-subscription-delivery-error",
		Status:    domain.StatusExecuting,
	}
	job := &domain.Job{ID: run.JobID, ProjectID: run.ProjectID}

	err := exec.completeRunWithWebhook(context.Background(), run, job, domain.StatusCompleted, map[string]any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "create run subscription webhook delivery")
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
	require.Error(t,
		err)
	require.Equal(t,
		"db write failed",
		err.Error())
}

func TestCompleteRunWithWebhook_RetryableStoreErrorRetries(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	store := &mockExecutorStore{
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			if attempts.Add(1) < 3 {
				return fmt.Errorf("update run status: %w", retryableCompletionErr{})
			}
			return nil
		},
	}
	exec := newSnoozeTestExecutor(t, store, 0)
	exec.terminalRetryTimeout = 100 * time.Millisecond
	exec.terminalRetryInitial = time.Millisecond
	exec.terminalRetryMax = time.Millisecond

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}
	job := &domain.Job{ID: "job-1", WebhookURL: ""}

	err := exec.completeRunWithWebhook(context.Background(), run, job,
		domain.StatusCompleted, map[string]any{})
	require.NoError(t, err)
	require.EqualValues(t, 3, attempts.Load())
	require.Len(t, store.statusUpdates(), 3)
}

func TestCompleteRunWithWebhook_RetryableStoreErrorStopsOnContextCancel(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return fmt.Errorf("update run status: %w", retryableCompletionErr{})
		},
	}
	exec := newSnoozeTestExecutor(t, store, 0)
	exec.terminalRetryTimeout = time.Second
	exec.terminalRetryInitial = time.Hour
	exec.terminalRetryMax = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}
	job := &domain.Job{ID: "job-1", WebhookURL: ""}

	err := exec.completeRunWithWebhook(ctx, run, job,
		domain.StatusCompleted, map[string]any{})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, store.statusUpdates(), 1)
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
	require.Equal(t,
		domain.StatusExecuting,
		completion.
			from,
	)
	require.Equal(t,
		domain.StatusCompleted,
		completion.
			to,
	)
	require.Equal(t,
		string(result),
		string(completion.
			fields["result"].(json.RawMessage)))
	require.True(t,
		completion.recordEndpointSuccess,
	)
	require.True(t,
		completion.enqueueWebhook,
	)
	require.Equal(t,
		domain.StatusCompleted,
		completion.
			webhookRun.
			Status)
	require.Equal(t,
		string(result),
		string(completion.
			webhookRun.
			Result,
		))
	require.False(t,
		completion.webhookRun.
			FinishedAt ==
			nil ||
			!completion.
				webhookRun.FinishedAt.
				Equal(finishedAt))
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
	require.False(t,
		completion.recordEndpointSuccess,
	)
	require.False(t,
		completion.enqueueWebhook,
	)
	require.Equal(t,
		domain.StatusDeadLetter,
		completion.
			webhookRun.
			Status)
	require.Equal(t,
		"failed", completion.
			webhookRun.
			Error,
	)
	require.False(t,
		completion.webhookRun.
			FinishedAt ==
			nil ||
			!completion.
				webhookRun.FinishedAt.
				Equal(finishedAt))
}

func TestRunWebhookEventType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status domain.RunStatus
		want   string
		ok     bool
	}{
		{status: domain.StatusCompleted, want: domain.WebhookEventRunCompleted, ok: true},
		{status: domain.StatusFailed, want: domain.WebhookEventRunFailed, ok: true},
		{status: domain.StatusTimedOut, want: domain.WebhookEventRunTimedOut, ok: true},
		{status: domain.StatusCanceled, want: domain.WebhookEventRunCanceled, ok: true},
		{status: domain.StatusQueued, ok: false},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got, ok := runWebhookEventType(tt.status)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestRunSubscriptionWebhookPayload(t *testing.T) {
	t.Parallel()

	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "project-1",
		Status:    domain.StatusCompleted,
		Attempt:   2,
		Result:    json.RawMessage(`{"ok":true}`),
	}

	payload, err := runSubscriptionWebhookPayload(run, domain.WebhookEventRunCompleted)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	require.Equal(t, domain.WebhookEventRunCompleted, decoded["type"])
	require.Equal(t, "run-1", decoded["run_id"])
	require.Equal(t, "job-1", decoded["job_id"])
	require.Equal(t, "project-1", decoded["project_id"])
	require.Equal(t, "completed", decoded["status"])
	require.EqualValues(t, 2, decoded["attempt"])
	require.Equal(t, map[string]any{"ok": true}, decoded["result"])
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
	require.NotEmpty(t, calls)

	found := false
	for _, c := range calls {
		if c.to == domain.StatusCompleted {
			found = true
			break
		}
	}
	require.True(t,
		found)

	events := getEvents()
	require.NotEmpty(t, events)
	require.Equal(t,
		EventCompleted,
		events[0].
			Type)
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
	require.True(t,
		found)

	events := getEvents()
	foundDL := false
	for _, ev := range events {
		if ev.Type == EventDeadLettered {
			foundDL = true
			break
		}
	}
	require.True(t,
		foundDL)
}

func TestHandleFailure_DLQCapUnderCapDeadLetters(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	exec := newSnoozeTestExecutor(t, store, 0)
	exec.dlqCapEnforcer = NewDLQCapEnforcer(newFakeDLQStore(), DLQCapConfig{
		MaxPerJob: 10,
		Policy:    DLQOverflowReject,
	}, nil)
	getEvents := collectEvents(exec)
	t.Cleanup(func() { close(exec.eventCh) })

	run := testRun(3)
	run.Status = domain.StatusExecuting
	job := testJob("http://localhost", 3, 30)
	policy := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	require.True(t, exec.handleFailure(context.Background(), run, job, policy, errors.New("server error"), nil))

	calls := store.statusUpdates()
	require.NotEmpty(t, calls)
	require.Equal(t, domain.StatusDeadLetter, calls[len(calls)-1].to)

	events := getEvents()
	require.NotEmpty(t, events)
	require.Equal(t, EventDeadLettered, events[len(events)-1].Type)
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
	require.True(t,
		found)

	events := getEvents()
	foundTO := false
	for _, ev := range events {
		if ev.Type == EventTimedOut {
			foundTO = true
			break
		}
	}
	require.True(t,
		foundTO)
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
	scanAttempt              int
	circuitSuccessErr        error
	circuitSuccessCalls      int
	enqueueWebhookErr        error
	createWebhookDeliveryErr error
	listSubscriptionsErr     error
	subscriptions            []domain.WebhookSubscription
}

func (m *mockPgxTx) Begin(_ context.Context) (pgx.Tx, error) {
	return m, nil
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
func (m *mockPgxTx) Exec(_ context.Context, query string, _ ...any) (pgconn.CommandTag, error) {
	if strings.Contains(query, "endpoint_circuit_state") {
		m.circuitSuccessCalls++
		if m.circuitSuccessErr != nil {
			return pgconn.CommandTag{}, m.circuitSuccessErr
		}
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func (m *mockPgxTx) Query(_ context.Context, query string, _ ...any) (pgx.Rows, error) {
	if strings.Contains(query, "FROM webhook_subscriptions") {
		if m.listSubscriptionsErr != nil {
			return nil, m.listSubscriptionsErr
		}
		return &mockSubscriptionRows{subscriptions: m.subscriptions}, nil
	}
	return nil, errors.New("mock tx: query not implemented")
}
func (m *mockPgxTx) QueryRow(_ context.Context, query string, _ ...any) pgx.Row {
	if strings.Contains(query, "INSERT INTO webhook_deliveries") {
		if strings.Contains(query, "event_type") && m.enqueueWebhookErr != nil {
			return &mockRow{err: m.enqueueWebhookErr}
		}
		if !strings.Contains(query, "event_type") && m.createWebhookDeliveryErr != nil {
			return &mockRow{err: m.createWebhookDeliveryErr}
		}
		return &mockRow{timestamps: true}
	}
	return &mockRow{attempt: m.scanAttempt}
}
func (m *mockPgxTx) Conn() *pgx.Conn {
	return nil
}

// mockRow satisfies pgx.Row for QueryRow in the mock tx.
type mockRow struct {
	attempt    int
	timestamps bool
	err        error
}

func (m *mockRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	if m.attempt > 0 && len(dest) > 0 {
		if p, ok := dest[0].(*int); ok {
			*p = m.attempt
			return nil
		}
	}
	if m.timestamps {
		now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		for _, d := range dest {
			switch p := d.(type) {
			case **time.Time:
				*p = &now
			case *time.Time:
				*p = now
			}
		}
		return nil
	}
	return errors.New("mock row: not implemented")
}

type mockSubscriptionRows struct {
	subscriptions []domain.WebhookSubscription
	index         int
	closed        bool
	err           error
}

func (m *mockSubscriptionRows) Close() {
	m.closed = true
}

func (m *mockSubscriptionRows) Err() error {
	return m.err
}

func (m *mockSubscriptionRows) CommandTag() pgconn.CommandTag {
	return pgconn.NewCommandTag("SELECT")
}

func (m *mockSubscriptionRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (m *mockSubscriptionRows) Next() bool {
	if m.index >= len(m.subscriptions) {
		m.Close()
		return false
	}
	m.index++
	return true
}

func (m *mockSubscriptionRows) Scan(dest ...any) error {
	if m.index == 0 || m.index > len(m.subscriptions) {
		return errors.New("mock subscription rows: scan without row")
	}
	sub := m.subscriptions[m.index-1]
	values := []any{
		sub.ID,
		sub.ProjectID,
		sub.WebhookURL,
		sub.EventTypes,
		sub.Secret,
		sub.Active,
		sub.CreatedAt,
	}
	for i := range dest {
		switch p := dest[i].(type) {
		case *string:
			*p = values[i].(string)
		case *[]string:
			*p = append((*p)[:0], values[i].([]string)...)
		case *bool:
			*p = values[i].(bool)
		case *time.Time:
			*p = values[i].(time.Time)
		default:
			return fmt.Errorf("mock subscription rows: unsupported dest %T", dest[i])
		}
	}
	return nil
}

func (m *mockSubscriptionRows) Values() ([]any, error) {
	return nil, errors.New("mock subscription rows: values not implemented")
}

func (m *mockSubscriptionRows) RawValues() [][]byte {
	return nil
}

func (m *mockSubscriptionRows) Conn() *pgx.Conn {
	return nil
}

type retryableCompletionErr struct{}

func (retryableCompletionErr) Error() string { return "conn closed" }
func (retryableCompletionErr) SafeToRetry() bool {
	return true
}
