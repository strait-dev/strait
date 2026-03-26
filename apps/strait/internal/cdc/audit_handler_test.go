package cdc

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"strait/internal/domain"
)

type mockAuditStore struct {
	mu     sync.Mutex
	events []domain.AuditEvent
	err    error
}

func (m *mockAuditStore) CreateAuditEvent(_ context.Context, ev *domain.AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, *ev)
	return nil
}

func TestAuditHandler_StatusChange_CreatesEvent(t *testing.T) {
	t.Parallel()
	store := &mockAuditStore{}
	h := NewAuditHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("executing", "p1", "run-1", "job-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(store.events))
	}
	if store.events[0].Action != "run.executing" {
		t.Errorf("expected action=run.executing, got %s", store.events[0].Action)
	}
}

func TestAuditHandler_InsertAction_CreatesEvent(t *testing.T) {
	t.Parallel()
	store := &mockAuditStore{}
	h := NewAuditHandler(store, nil)

	record, _ := json.Marshal(map[string]any{
		"id":         "run-1",
		"job_id":     "job-1",
		"project_id": "p1",
		"status":     "queued",
	})
	msg := Message{
		AckID:    "ack-1",
		Action:   ActionInsert,
		Record:   record,
		Metadata: Metadata{TableName: "job_runs"},
	}

	err := h.Handle(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(store.events))
	}
	if store.events[0].Action != "run.created" {
		t.Errorf("expected action=run.created, got %s", store.events[0].Action)
	}
}

func TestAuditHandler_ActorIsSystemCDC(t *testing.T) {
	t.Parallel()
	store := &mockAuditStore{}
	h := NewAuditHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.events) != 1 {
		t.Fatal("expected 1 event")
	}
	if store.events[0].ActorID != "system:cdc" {
		t.Errorf("expected actor_id=system:cdc, got %s", store.events[0].ActorID)
	}
	if store.events[0].ActorType != "system" {
		t.Errorf("expected actor_type=system, got %s", store.events[0].ActorType)
	}
}

func TestAuditHandler_DetailsContainFullRecord(t *testing.T) {
	t.Parallel()
	store := &mockAuditStore{}
	h := NewAuditHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-42", "job-7"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.events) != 1 {
		t.Fatal("expected 1 event")
	}

	var details map[string]any
	if err := json.Unmarshal(store.events[0].Details, &details); err != nil {
		t.Fatalf("details is not valid JSON: %v", err)
	}
	if details["id"] != "run-42" {
		t.Errorf("expected id=run-42, got %v", details["id"])
	}
	if details["job_id"] != "job-7" {
		t.Errorf("expected job_id=job-7, got %v", details["job_id"])
	}
}

func TestAuditHandler_ActionMatchesStatus(t *testing.T) {
	t.Parallel()
	store := &mockAuditStore{}
	h := NewAuditHandler(store, nil)

	statuses := []string{"completed", "failed", "timed_out", "executing", "queued"}
	for _, status := range statuses {
		err := h.Handle(context.Background(), cdcUpdateMsg(status, "p1", "run-1", "job-1"))
		if err != nil {
			t.Fatalf("unexpected error for status %s: %v", status, err)
		}
	}

	if len(store.events) != len(statuses) {
		t.Fatalf("expected %d events, got %d", len(statuses), len(store.events))
	}
	for i, status := range statuses {
		expected := "run." + status
		if store.events[i].Action != expected {
			t.Errorf("event %d: expected action=%s, got %s", i, expected, store.events[i].Action)
		}
	}
}

func TestAuditHandler_StoreError_Resilient(t *testing.T) {
	t.Parallel()
	store := &mockAuditStore{
		err: errors.New("db connection failed"),
	}
	h := NewAuditHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	if err != nil {
		t.Fatalf("expected nil error on store failure, got: %v", err)
	}
}
