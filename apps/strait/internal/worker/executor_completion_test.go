package worker

import (
	"context"
	"encoding/json"
	"errors"
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
