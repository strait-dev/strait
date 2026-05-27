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

func TestAuditHandler_TerminalUpdate_CreatesEvent(t *testing.T) {
	t.Parallel()
	store := &mockAuditStore{}
	h := NewAuditHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(store.events))
	}
	if store.events[0].Action != "run.completed" {
		t.Errorf("expected action=run.completed, got %s", store.events[0].Action)
	}
}

func TestAuditHandler_RedeliveredTerminalUpdateCreatesAuditEventOnce(t *testing.T) {
	t.Parallel()
	store := &mockAuditStore{}
	h := NewAuditHandler(store, nil)

	msg := cdcUpdateMsg("completed", "p1", "run-redelivered", "job-1")
	msg.Metadata.IdempotencyKey = "wal:job_runs:run-redelivered:completed"
	if err := h.Handle(context.Background(), msg); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	msg.AckID = "ack-redelivery"
	if err := h.Handle(context.Background(), msg); err != nil {
		t.Fatalf("redelivery: %v", err)
	}

	if len(store.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(store.events))
	}
}

func TestAuditHandler_StoreErrorDoesNotConsumeRedeliveryDedupe(t *testing.T) {
	t.Parallel()
	store := &mockAuditStore{err: errors.New("temporary store failure")}
	h := NewAuditHandler(store, nil)

	msg := cdcUpdateMsg("completed", "p1", "run-retry", "job-1")
	msg.Metadata.IdempotencyKey = "wal:job_runs:run-retry:completed"
	if err := h.Handle(context.Background(), msg); err == nil {
		t.Fatal("first delivery error = nil, want store failure")
	}

	store.mu.Lock()
	store.err = nil
	store.mu.Unlock()
	msg.AckID = "ack-redelivery"
	if err := h.Handle(context.Background(), msg); err != nil {
		t.Fatalf("redelivery after store recovery: %v", err)
	}

	if len(store.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(store.events))
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

func TestDeepSecAuditHandler_DetailsExcludeSensitiveRecordFields(t *testing.T) {
	t.Parallel()
	store := &mockAuditStore{}
	h := NewAuditHandler(store, nil)

	record, _ := json.Marshal(map[string]any{
		"id":         "run-42",
		"job_id":     "job-7",
		"project_id": "p1",
		"status":     "completed",
		"attempt":    2,
		"payload":    map[string]any{"api_key": "secret"},
		"result":     map[string]any{"token": "secret"},
		"error":      "contains user payload",
	})
	err := h.Handle(context.Background(), Message{
		AckID:    "ack-1",
		Action:   ActionUpdate,
		Record:   record,
		Metadata: Metadata{TableName: "job_runs"},
	})
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
	if details["run_id"] != "run-42" {
		t.Errorf("expected run_id=run-42, got %v", details["run_id"])
	}
	if details["job_id"] != "job-7" {
		t.Errorf("expected job_id=job-7, got %v", details["job_id"])
	}
	for _, sensitive := range []string{"payload", "result", "error"} {
		if _, ok := details[sensitive]; ok {
			t.Fatalf("audit details included sensitive field %q: %#v", sensitive, details)
		}
	}
}

func TestAuditHandler_ActionMatchesStatus(t *testing.T) {
	t.Parallel()
	store := &mockAuditStore{}
	h := NewAuditHandler(store, nil)

	// Only terminal statuses trigger an audit event.
	statuses := []string{"completed", "failed", "timed_out", "executing", "queued"}
	for _, status := range statuses {
		err := h.Handle(context.Background(), cdcUpdateMsg(status, "p1", "run-1", "job-1"))
		if err != nil {
			t.Fatalf("unexpected error for status %s: %v", status, err)
		}
	}

	wantActions := []string{"run.completed", "run.failed", "run.timed_out"}
	if len(store.events) != len(wantActions) {
		t.Fatalf("expected %d events, got %d", len(wantActions), len(store.events))
	}
	for i, want := range wantActions {
		if store.events[i].Action != want {
			t.Errorf("event %d: action = %q, want %q", i, store.events[i].Action, want)
		}
	}
}

func TestAuditHandler_GatesNonTerminalUpdates(t *testing.T) {
	t.Parallel()

	// Table-test every RunStatus enum value. Only terminal statuses
	// must produce an audit event on an UPDATE CDC message.
	statuses := []domain.RunStatus{
		domain.StatusDelayed,
		domain.StatusQueued,
		domain.StatusDequeued,
		domain.StatusExecuting,
		domain.StatusWaiting,
		domain.StatusCompleted,
		domain.StatusFailed,
		domain.StatusTimedOut,
		domain.StatusCrashed,
		domain.StatusSystemFailed,
		domain.StatusCanceled,
		domain.StatusExpired,
		domain.StatusDeadLetter,
	}

	for _, s := range statuses {
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			store := &mockAuditStore{}
			h := NewAuditHandler(store, nil)
			if err := h.Handle(context.Background(), cdcUpdateMsg(string(s), "p1", "run-1", "job-1")); err != nil {
				t.Fatalf("Handle: %v", err)
			}
			wantEvents := 0
			if s.IsTerminal() {
				wantEvents = 1
			}
			if len(store.events) != wantEvents {
				t.Fatalf("status=%s: got %d events, want %d", s, len(store.events), wantEvents)
			}
		})
	}
}

func TestAuditHandler_InsertAlwaysEmits(t *testing.T) {
	t.Parallel()

	// INSERT is unconditional regardless of the status the new row carries.
	for _, status := range []string{"queued", "delayed", "completed"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			store := &mockAuditStore{}
			h := NewAuditHandler(store, nil)

			record, _ := json.Marshal(map[string]any{
				"id":         "run-1",
				"project_id": "p1",
				"status":     status,
			})
			msg := Message{Action: ActionInsert, Record: record, Metadata: Metadata{TableName: "job_runs"}}

			if err := h.Handle(context.Background(), msg); err != nil {
				t.Fatalf("Handle: %v", err)
			}
			if len(store.events) != 1 || store.events[0].Action != "run.created" {
				t.Fatalf("got %+v, want exactly one run.created event", store.events)
			}
		})
	}
}

func TestAuditHandler_DeleteAlwaysEmits(t *testing.T) {
	t.Parallel()

	// DELETE is unconditional regardless of the deleted row's status.
	for _, status := range []string{"queued", "executing", "completed"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			store := &mockAuditStore{}
			h := NewAuditHandler(store, nil)

			record, _ := json.Marshal(map[string]any{
				"id":         "run-1",
				"project_id": "p1",
				"status":     status,
			})
			msg := Message{Action: ActionDelete, Record: record, Metadata: Metadata{TableName: "job_runs"}}

			if err := h.Handle(context.Background(), msg); err != nil {
				t.Fatalf("Handle: %v", err)
			}
			if len(store.events) != 1 || store.events[0].Action != "run.deleted" {
				t.Fatalf("got %+v, want exactly one run.deleted event", store.events)
			}
		})
	}
}

func TestAuditHandler_HighHeartbeatVolume_NoWriteAmplification(t *testing.T) {
	t.Parallel()

	// Simulate 1000 non-terminal status updates on a single run (the
	// shape of a heartbeat-driven CDC stream). The handler must produce
	// zero audit rows. This keeps heartbeat traffic from amplifying writes.
	store := &mockAuditStore{}
	h := NewAuditHandler(store, nil)

	for range 1000 {
		if err := h.Handle(context.Background(), cdcUpdateMsg("executing", "p1", "run-hot", "job-1")); err != nil {
			t.Fatalf("Handle: %v", err)
		}
	}
	if len(store.events) != 0 {
		t.Fatalf("got %d audit events from heartbeat-shaped updates, want 0", len(store.events))
	}
}

func TestDeepSecAuditHandler_IgnoresReadEmptyAndUnknownActions(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		action Action
		status string
	}{
		{name: "read non terminal", action: ActionRead, status: "executing"},
		{name: "read terminal", action: ActionRead, status: "completed"},
		{name: "empty action", action: "", status: "completed"},
		{name: "unknown action", action: Action("snapshot"), status: "completed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &mockAuditStore{}
			h := NewAuditHandler(store, nil)
			record, _ := json.Marshal(map[string]any{
				"id":         "run-1",
				"job_id":     "job-1",
				"project_id": "p1",
				"status":     tt.status,
			})

			err := h.Handle(context.Background(), Message{
				AckID:    "ack-unsupported",
				Action:   tt.action,
				Record:   record,
				Metadata: Metadata{TableName: "job_runs"},
			})
			if err != nil {
				t.Fatalf("Handle error = %v", err)
			}
			if len(store.events) != 0 {
				t.Fatalf("events = %d, want 0: %#v", len(store.events), store.events)
			}
		})
	}
}

func TestAuditHandler_UnsupportedSnapshotActionsDoNotParseOrAudit(t *testing.T) {
	t.Parallel()

	for _, action := range []Action{ActionRead, Action("snapshot"), ""} {
		t.Run(string(action), func(t *testing.T) {
			t.Parallel()
			store := &mockAuditStore{}
			h := NewAuditHandler(store, nil)

			err := h.Handle(context.Background(), Message{
				AckID:    "ack-snapshot",
				Action:   action,
				Record:   json.RawMessage(`not valid json`),
				Metadata: Metadata{TableName: "job_runs"},
			})
			if err != nil {
				t.Fatalf("Handle error = %v, want nil for unsupported snapshot action", err)
			}
			if len(store.events) != 0 {
				t.Fatalf("events = %d, want 0: %#v", len(store.events), store.events)
			}
		})
	}
}

func TestDeepSecAuditHandler_StoreErrorReturnsForRetry(t *testing.T) {
	t.Parallel()
	store := &mockAuditStore{
		err: errors.New("db connection failed"),
	}
	h := NewAuditHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	if err == nil {
		t.Fatal("expected error on store failure")
	}
}
