package cdc

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"strait/internal/domain"
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.evaluations) != 1 {
		t.Fatalf("expected 1 evaluation, got %d", len(store.evaluations))
	}
	if store.evaluations[0].SLOID != "slo-1" {
		t.Errorf("expected slo_id=slo-1, got %s", store.evaluations[0].SLOID)
	}
	if store.evaluations[0].ID == "" {
		t.Fatal("expected evaluation id to be set")
	}
	if store.evaluations[0].EvaluatedAt.IsZero() {
		t.Fatal("expected evaluated_at to be set")
	}
	if store.evaluations[0].CurrentValue != 1.0 {
		t.Errorf("expected current_value=1.0 for completed, got %f", store.evaluations[0].CurrentValue)
	}
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
		if err != nil {
			t.Fatalf("unexpected error for status %s: %v", status, err)
		}
	}
	if len(store.evaluations) != 0 {
		t.Fatalf("expected 0 evaluations for non-terminal, got %d", len(store.evaluations))
	}
}

func TestSLOHandler_NoSLOs_NoEvaluation(t *testing.T) {
	t.Parallel()
	store := &mockSLOStore{slos: nil}
	h := NewSLOHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.evaluations) != 0 {
		t.Fatal("expected no evaluations when no SLOs")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.evaluations) != 3 {
		t.Fatalf("expected 3 evaluations, got %d", len(store.evaluations))
	}
	for _, eval := range store.evaluations {
		if eval.CurrentValue != 0.0 {
			t.Errorf("expected current_value=0.0 for failed, got %f", eval.CurrentValue)
		}
	}
}

func TestDeepSecSLOHandler_StoreErrorReturnsForRetry(t *testing.T) {
	t.Parallel()
	store := &mockSLOStore{
		slosErr: errors.New("db connection failed"),
	}
	h := NewSLOHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	if err == nil {
		t.Fatal("expected error on store failure")
	}
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
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
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
	if err == nil {
		t.Fatal("expected error on insert failure")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.evaluations) != 1 {
		t.Fatalf("expected 1 evaluation, got %d", len(store.evaluations))
	}
	if store.evaluations[0].CurrentValue != 0.0 {
		t.Errorf("expected current_value=0.0 for timed_out, got %f", store.evaluations[0].CurrentValue)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.evaluations) != 1 {
		t.Fatalf("expected 1 evaluation, got %d", len(store.evaluations))
	}
	if store.evaluations[0].CurrentValue != 0.0 {
		t.Errorf("expected current_value=0.0 for canceled, got %f", store.evaluations[0].CurrentValue)
	}
}
