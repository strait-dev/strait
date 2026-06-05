package cdc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollect_AllActions(t *testing.T) {
	t.Parallel()
	actions := []Action{ActionInsert, ActionUpdate, ActionDelete}
	pub := &mockPublisher{}

	for _, action := range actions {
		t.Run(string(action), func(t *testing.T) {
			t.Parallel()
			h := NewJobRunHandler(pub, nil)
			msg := Message{
				Action:   action,
				Record:   json.RawMessage(`{"id":"r1","job_id":"j1","project_id":"p1","status":"completed"}`),
				Changes:  json.RawMessage(`{}`),
				Metadata: Metadata{TableName: "job_runs", CommitTimestamp: "2026-03-18T00:00:00Z"},
			}
			pubMsg, err := h.Collect(context.Background(), msg)
			require.NoError(t, err)
			require.NotNil(t, pubMsg)

			var event ChangeEvent
			require.NoError(t, json.Unmarshal(pubMsg.
				Data,

				&event))
			assert.Equal(
				t, action, event.
					Action)
		})
	}
}

func TestCollect_PreservesChangesField(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{}
	h := NewJobRunHandler(pub, nil)

	changes := json.RawMessage(`{"status":{"old":"executing","new":"completed"}}`)
	msg := Message{
		Action:   ActionUpdate,
		Record:   json.RawMessage(`{"id":"r1","job_id":"j1","project_id":"p1","status":"completed"}`),
		Changes:  changes,
		Metadata: Metadata{TableName: "job_runs", CommitTimestamp: "2026-03-18T00:00:00Z"},
	}

	pubMsg, err := h.Collect(context.Background(), msg)
	require.NoError(t, err)

	var event ChangeEvent
	require.NoError(t, json.Unmarshal(pubMsg.
		Data,

		&event))
	assert.Equal(
		t, string(changes), string(event.
			Changes))
}

func TestCollect_PreservesTimestamp(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{}
	h := NewWorkflowRunHandler(pub, nil)

	ts := "2026-03-18T12:34:56Z"
	msg := Message{
		Action:   ActionInsert,
		Record:   json.RawMessage(`{"id":"wfr-1","workflow_id":"wf-1","project_id":"p1","status":"running"}`),
		Metadata: Metadata{TableName: "workflow_runs", CommitTimestamp: ts},
	}

	pubMsg, err := h.Collect(context.Background(), msg)
	require.NoError(t, err)

	var event ChangeEvent
	require.NoError(t, json.Unmarshal(pubMsg.
		Data,

		&event))
	assert.Equal(
		t, ts, event.Timestamp,
	)
}

func TestCollect_InvalidRecord_AllHandlers(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{}
	invalid := json.RawMessage(`{broken`)

	handlers := []CollectableHandler{
		NewJobRunHandler(pub, nil),
		NewWorkflowRunHandler(pub, nil),
		NewWorkflowStepRunHandler(pub, nil),
		NewEventTriggerHandler(pub, nil),
	}

	for _, h := range handlers {
		t.Run(h.Table(), func(t *testing.T) {
			t.Parallel()
			msg := Message{
				Action:   ActionInsert,
				Record:   invalid,
				Metadata: Metadata{TableName: h.Table()},
			}
			_, err := h.Collect(context.Background(), msg)
			assert.Error(
				t, err)
		})
	}
}

func TestCollect_EmptyProjectID(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{}
	h := NewJobRunHandler(pub, nil)

	msg := Message{
		Action:   ActionInsert,
		Record:   json.RawMessage(`{"id":"r1","job_id":"j1","project_id":"","status":"completed"}`),
		Metadata: Metadata{TableName: "job_runs", CommitTimestamp: "2026-03-18T00:00:00Z"},
	}

	pubMsg, err := h.Collect(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(
		t, "cdc:project::job_runs",

		pubMsg.
			Channel)

	// Channel should still be formed, even with empty project ID.
}

func TestCollect_LargeRecord(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{}
	h := NewJobRunHandler(pub, nil)

	// Build a large record with a big payload field.
	largePayload := make([]byte, 100000)
	for i := range largePayload {
		largePayload[i] = 'x'
	}
	record, _ := json.Marshal(map[string]any{
		"id":         "r1",
		"job_id":     "j1",
		"project_id": "p1",
		"status":     "completed",
		"payload":    string(largePayload),
	})

	msg := Message{
		Action:   ActionInsert,
		Record:   record,
		Metadata: Metadata{TableName: "job_runs", CommitTimestamp: "2026-03-18T00:00:00Z"},
	}

	pubMsg, err := h.Collect(context.Background(), msg)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(pubMsg.
		Data),

		100000,
	)
}

func TestCollectChangeEvent_EmptyRecord(t *testing.T) {
	t.Parallel()
	event := ChangeEvent{
		Table:  "job_runs",
		Action: ActionInsert,
		Record: json.RawMessage(`{}`),
		Source: "cdc",
	}

	msg, err := collectChangeEvent(event, "ch")
	require.NoError(t, err)
	assert.Equal(
		t, "ch", msg.Channel,
	)
}

func TestCollectChangeEvent_NilRecord(t *testing.T) {
	t.Parallel()
	event := ChangeEvent{
		Table:  "job_runs",
		Action: ActionInsert,
		Record: nil,
		Source: "cdc",
	}

	msg, err := collectChangeEvent(event, "ch")
	require.NoError(t, err)
	require.NotNil(t, msg)
}
