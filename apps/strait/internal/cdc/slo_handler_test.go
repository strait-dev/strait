package cdc

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSLOStore struct {
	mu          sync.Mutex
	slos        []domain.JobSLOStatus
	slosErr     error
	evaluations []domain.JobSLOEvaluation
	evalErr     error
}

func (m *mockSLOStore) ListJobSLOs(_ context.Context, _ string) ([]domain.JobSLOStatus, error) {
	if m.slosErr != nil {
		return nil, m.slosErr
	}
	return m.slos, nil
}

func (m *mockSLOStore) InsertSLOEvaluation(_ context.Context, eval *domain.JobSLOEvaluation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.evalErr != nil {
		return m.evalErr
	}
	m.evaluations = append(m.evaluations, *eval)
	return nil
}

func TestSLOHandler_TerminalRun_InsertsEvaluation(t *testing.T) {
	t.Parallel()
	store := &mockSLOStore{
		slos: []domain.JobSLOStatus{
			{JobSLO: domain.JobSLO{ID: "slo-1", JobID: "job-1"}},
		},
	}
	h := NewSLOHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	require.NoError(t, err)
	require.Len(t,
		store.evaluations,
		1)
	assert.Equal(
		t, "slo-1", store.
			evaluations[0].SLOID,
	)
	require.NotEmpty(t, store.
		evaluations[0].ID)
	require.False(t, store.evaluations[0].EvaluatedAt.
		IsZero())
	assert.InDelta(t, 1.0, store.evaluations[0].CurrentValue, 1e-9)
}

func TestSLOHandler_NonTerminal_Skipped(t *testing.T) {
	t.Parallel()
	store := &mockSLOStore{
		slos: []domain.JobSLOStatus{
			{JobSLO: domain.JobSLO{ID: "slo-1", JobID: "job-1"}},
		},
	}
	h := NewSLOHandler(store, nil)

	for _, status := range []string{"queued", "executing", "dequeued"} {
		err := h.Handle(context.Background(), cdcUpdateMsg(status, "p1", "run-1", "job-1"))
		require.NoError(t, err)
	}
	require.Empty(t,
		store.evaluations)
}

func TestSLOHandler_NoSLOs_NoEvaluation(t *testing.T) {
	t.Parallel()
	store := &mockSLOStore{slos: nil}
	h := NewSLOHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	require.NoError(t, err)
	require.Empty(t,
		store.evaluations)
}

func TestSLOHandler_MultipleSLOs_AllEvaluated(t *testing.T) {
	t.Parallel()
	store := &mockSLOStore{
		slos: []domain.JobSLOStatus{
			{JobSLO: domain.JobSLO{ID: "slo-1", JobID: "job-1"}},
			{JobSLO: domain.JobSLO{ID: "slo-2", JobID: "job-1"}},
			{JobSLO: domain.JobSLO{ID: "slo-3", JobID: "job-1"}},
		},
	}
	h := NewSLOHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("failed", "p1", "run-1", "job-1"))
	require.NoError(t, err)
	require.Len(t,
		store.evaluations,
		3)

	for _, eval := range store.evaluations {
		assert.InDelta(t, 0.0, eval.CurrentValue, 1e-9)
	}
}

func TestSLOHandler_DoesNotOverwriteExistingSLOEvaluationWithPlaceholder(t *testing.T) {
	t.Parallel()
	currentValue := 0.997
	budgetRemaining := 0.84
	evaluatedAt := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	store := &mockSLOStore{
		slos: []domain.JobSLOStatus{
			{
				JobSLO:          domain.JobSLO{ID: "slo-real", JobID: "job-1"},
				CurrentValue:    &currentValue,
				BudgetRemaining: &budgetRemaining,
				EvaluatedAt:     &evaluatedAt,
			},
			{JobSLO: domain.JobSLO{ID: "slo-empty", JobID: "job-1"}},
		},
	}
	h := NewSLOHandler(store, nil)
	require.NoError(t, h.Handle(context.
		Background(),
		cdcUpdateMsg("failed",

			"p1", "run-1", "job-1",
		),
	),
	)
	require.Len(t,
		store.evaluations,
		1)
	require.Equal(t, "slo-empty",
		store.evaluations[0].SLOID)
}

func TestSLOHandler_RedeliveredTerminalUpdateInsertsEvaluationOnce(t *testing.T) {
	t.Parallel()
	store := &mockSLOStore{
		slos: []domain.JobSLOStatus{
			{JobSLO: domain.JobSLO{ID: "slo-1", JobID: "job-1"}},
		},
	}
	h := NewSLOHandler(store, nil)

	msg := cdcUpdateMsg("completed", "p1", "run-redelivered", "job-1")
	msg.Metadata.IdempotencyKey = "wal:job_runs:run-redelivered:completed"
	require.NoError(t, h.Handle(context.
		Background(),
		msg))

	msg.AckID = "ack-redelivery"
	require.NoError(t, h.Handle(context.
		Background(),
		msg))
	require.Len(t,
		store.evaluations,
		1)
}

func TestSLOHandler_MultipleTerminalUpdatesForSameRunInsertEvaluationOnce(t *testing.T) {
	t.Parallel()
	store := &mockSLOStore{
		slos: []domain.JobSLOStatus{
			{JobSLO: domain.JobSLO{ID: "slo-1", JobID: "job-1"}},
		},
	}
	h := NewSLOHandler(store, nil)

	first := cdcUpdateMsg("failed", "p1", "run-terminal", "job-1")
	first.Metadata.IdempotencyKey = "wal:job_runs:run-terminal:failed"
	require.NoError(t, h.Handle(context.
		Background(),
		first))

	second := cdcUpdateMsg("dead_letter", "p1", "run-terminal", "job-1")
	second.Metadata.IdempotencyKey = "wal:job_runs:run-terminal:dead-letter"
	require.NoError(t, h.Handle(context.
		Background(),
		second))
	require.Len(t,
		store.evaluations,
		1)
}

func TestSLOHandler_InsertErrorDoesNotConsumeRedeliveryDedupe(t *testing.T) {
	t.Parallel()
	store := &mockSLOStore{
		slos: []domain.JobSLOStatus{
			{JobSLO: domain.JobSLO{ID: "slo-1", JobID: "job-1"}},
		},
		evalErr: errors.New("temporary insert failure"),
	}
	h := NewSLOHandler(store, nil)

	msg := cdcUpdateMsg("completed", "p1", "run-retry", "job-1")
	msg.Metadata.IdempotencyKey = "wal:job_runs:run-retry:completed"
	require.Error(t, h.Handle(context.
		Background(),
		msg))

	store.mu.Lock()
	store.evalErr = nil
	store.mu.Unlock()
	msg.AckID = "ack-redelivery"
	require.NoError(t, h.Handle(context.
		Background(),
		msg))
	require.Len(t,
		store.evaluations,
		1)
}

func TestDeepSecSLOHandler_StoreErrorReturnsForRetry(t *testing.T) {
	t.Parallel()
	store := &mockSLOStore{
		slosErr: errors.New("db connection failed"),
	}
	h := NewSLOHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	require.Error(t, err)
}

func TestSLOHandler_InvalidJSON(t *testing.T) {
	t.Parallel()
	store := &mockSLOStore{}
	h := NewSLOHandler(store, nil)

	msg := Message{
		Action:   ActionUpdate,
		Record:   json.RawMessage(`not valid json`),
		Metadata: Metadata{TableName: "job_runs"},
	}
	err := h.Handle(context.Background(), msg)
	require.Error(t, err)
}

func TestDeepSecSLOHandler_InsertEvaluationErrorReturnsForRetry(t *testing.T) {
	t.Parallel()
	store := &mockSLOStore{
		slos: []domain.JobSLOStatus{
			{JobSLO: domain.JobSLO{ID: "slo-1", JobID: "job-1"}},
			{JobSLO: domain.JobSLO{ID: "slo-2", JobID: "job-1"}},
		},
		evalErr: errors.New("db write failed"),
	}
	h := NewSLOHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	require.Error(t, err)
}

func TestSLOHandler_TimedOutStatus(t *testing.T) {
	t.Parallel()
	store := &mockSLOStore{
		slos: []domain.JobSLOStatus{
			{JobSLO: domain.JobSLO{ID: "slo-1", JobID: "job-1"}},
		},
	}
	h := NewSLOHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("timed_out", "p1", "run-1", "job-1"))
	require.NoError(t, err)
	require.Len(t,
		store.evaluations,
		1)
	assert.InDelta(t, 0.0, store.evaluations[0].CurrentValue, 1e-9)
}

func TestSLOHandler_CanceledStatus(t *testing.T) {
	t.Parallel()
	store := &mockSLOStore{
		slos: []domain.JobSLOStatus{
			{JobSLO: domain.JobSLO{ID: "slo-1", JobID: "job-1"}},
		},
	}
	h := NewSLOHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("canceled", "p1", "run-1", "job-1"))
	require.NoError(t, err)
	require.Len(t,
		store.evaluations,
		1)
	assert.InDelta(t, 0.0, store.evaluations[0].CurrentValue, 1e-9)
}

func TestDeepSecSLOHandler_TerminalFailureStatesEvaluateAsFailures(t *testing.T) {
	t.Parallel()

	for _, status := range []domain.RunStatus{
		domain.StatusFailed,
		domain.StatusTimedOut,
		domain.StatusCrashed,
		domain.StatusSystemFailed,
		domain.StatusCanceled,
		domain.StatusExpired,
		domain.StatusDeadLetter,
	} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			store := &mockSLOStore{
				slos: []domain.JobSLOStatus{
					{JobSLO: domain.JobSLO{ID: "slo-1", JobID: "job-1"}},
				},
			}
			h := NewSLOHandler(store, nil)

			err := h.Handle(context.Background(), cdcUpdateMsg(string(status), "p1", "run-1", "job-1"))
			require.NoError(t, err)
			require.Len(t,
				store.evaluations,
				1)
			require.InDelta(t, 0.0, store.evaluations[0].CurrentValue, 1e-9)
		})
	}
}

func TestDeepSecSLOCurrentValue_FailsClosedForUnknownStatus(t *testing.T) {
	t.Parallel()
	require.InDelta(t, 0.0, sloCurrentValue(domain.
		RunStatus("future_terminal")), 1e-9)
}
