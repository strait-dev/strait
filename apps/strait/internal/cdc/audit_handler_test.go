package cdc

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	require.Len(t,
		store.events, 1,
	)
	assert.Equal(
		t, "run.completed",
		store.events[0].
			Action)
}

func TestAuditHandler_RedeliveredTerminalUpdateCreatesAuditEventOnce(t *testing.T) {
	t.Parallel()
	store := &mockAuditStore{}
	h := NewAuditHandler(store, nil)

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
		store.events, 1,
	)
}

func TestAuditHandler_StoreErrorDoesNotConsumeRedeliveryDedupe(t *testing.T) {
	t.Parallel()
	store := &mockAuditStore{err: errors.New("temporary store failure")}
	h := NewAuditHandler(store, nil)

	msg := cdcUpdateMsg("completed", "p1", "run-retry", "job-1")
	msg.Metadata.IdempotencyKey = "wal:job_runs:run-retry:completed"
	require.Error(t, h.Handle(context.
		Background(),
		msg))

	store.mu.Lock()
	store.err = nil
	store.mu.Unlock()
	msg.AckID = "ack-redelivery"
	require.NoError(t, h.Handle(context.
		Background(),
		msg))
	require.Len(t,
		store.events, 1,
	)
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
	require.NoError(t, err)
	require.Len(t,
		store.events, 1,
	)
	assert.Equal(
		t, "run.created",
		store.events[0].Action,
	)
}

func TestAuditHandler_ActorIsSystemCDC(t *testing.T) {
	t.Parallel()
	store := &mockAuditStore{}
	h := NewAuditHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	require.NoError(t, err)
	require.Len(t,
		store.events, 1,
	)
	assert.Equal(
		t, "system:cdc",
		store.events[0].ActorID,
	)
	assert.Equal(
		t, "system", store.
			events[0].ActorType,
	)
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
	require.NoError(t, err)
	require.Len(t,
		store.events, 1,
	)

	var details map[string]any
	require.NoError(t, json.Unmarshal(store.
		events[0].Details,
		&details))
	assert.Equal(
		t, "run-42", details["run_id"])
	assert.Equal(
		t, "job-7", details["job_id"])

	for _, sensitive := range []string{"payload", "result", "error"} {
		if _, ok := details[sensitive]; ok {
			require.Failf(t, "test failure",

				"audit details included sensitive field %q: %#v", sensitive, details)
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
		require.NoError(t, err)
	}

	wantActions := []string{"run.completed", "run.failed", "run.timed_out"}
	require.Len(t,
		store.events, len(wantActions))

	for i, want := range wantActions {
		assert.Equal(
			t, want, store.events[i].Action,
		)
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
			require.NoError(t, h.Handle(context.
				Background(),
				cdcUpdateMsg(string(s), "p1", "run-1",

					"job-1")))

			wantEvents := 0
			if s.IsTerminal() {
				wantEvents = 1
			}
			require.Len(t,
				store.events, wantEvents,
			)
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
			require.NoError(t, h.Handle(context.
				Background(),
				msg))
			require.False(t, len(store.events) != 1 ||
				store.
					events[0].
					Action !=
					"run.created")
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
			require.NoError(t, h.Handle(context.
				Background(),
				msg))
			require.False(t, len(store.events) != 1 ||
				store.
					events[0].
					Action !=
					"run.deleted")
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
		require.NoError(t, h.Handle(context.
			Background(),
			cdcUpdateMsg("executing", "p1", "run-hot",

				"job-1")))
	}
	require.Empty(t,
		store.events,
	)
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
			require.NoError(t, err)
			require.Empty(t,
				store.events,
			)
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
			require.NoError(t, err)
			require.Empty(t,
				store.events,
			)
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
	require.Error(t, err)
}
