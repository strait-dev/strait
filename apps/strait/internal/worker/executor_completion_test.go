package worker

import (
	"context"
	"errors"
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

func TestCompleteRunWithWebhook_WithTxPool_NoWebhook(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{}
	txPool := &mockTxBeginner{}
	exec := newCompletionTestExecutor(t, store, txPool)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}
	job := &domain.Job{ID: "job-1", WebhookURL: ""}

	err := exec.completeRunWithWebhook(context.Background(), run, job,
		domain.StatusCompleted, map[string]any{"result": "ok"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should NOT have started a transaction.
	if txPool.beginCalled {
		t.Fatal("transaction should not start when WebhookURL is empty")
	}

	calls := store.statusUpdates()
	if len(calls) != 1 {
		t.Fatalf("expected 1 plain status update, got %d", len(calls))
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

	exec.handleSuccess(context.Background(), run, job, nil, nil)

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
type mockPgxTx struct{}

func (m *mockPgxTx) Begin(_ context.Context) (pgx.Tx, error) {
	return &mockPgxTx{}, nil
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
	return &mockRow{}
}
func (m *mockPgxTx) Conn() *pgx.Conn {
	return nil
}

// mockRow satisfies pgx.Row for QueryRow in the mock tx.
type mockRow struct{}

func (m *mockRow) Scan(_ ...any) error {
	return errors.New("mock row: not implemented")
}
