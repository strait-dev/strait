package cdc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobRunHandler_Collect(t *testing.T) {
	t.Parallel()
	h := NewJobRunHandler(&mockPublisher{}, nil)
	msg := Message{
		Action:  ActionInsert,
		Record:  json.RawMessage(`{"id":"run-1","job_id":"job-1","project_id":"proj-1","status":"completed"}`),
		Changes: json.RawMessage(`{}`),
		Metadata: Metadata{
			TableName:       "job_runs",
			CommitTimestamp: "2026-03-18T00:00:00Z",
		},
	}

	pubMsg, err := h.Collect(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, pubMsg)
	assert.Equal(
		t, "cdc:project:proj-1:job_runs",

		pubMsg.Channel)

	var event ChangeEvent
	require.NoError(t, json.Unmarshal(pubMsg.
		Data,

		&event))
	assert.Equal(
		t, "job_runs", event.
			Table)
	assert.Equal(
		t, "cdc", event.Source,
	)

}

func TestWorkflowRunHandler_Collect(t *testing.T) {
	t.Parallel()
	h := NewWorkflowRunHandler(&mockPublisher{}, nil)
	msg := Message{
		Action:  ActionUpdate,
		Record:  json.RawMessage(`{"id":"wfr-1","workflow_id":"wf-1","project_id":"proj-2","status":"running"}`),
		Changes: json.RawMessage(`{}`),
		Metadata: Metadata{
			TableName:       "workflow_runs",
			CommitTimestamp: "2026-03-18T00:00:00Z",
		},
	}

	pubMsg, err := h.Collect(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(
		t, "cdc:project:proj-2:workflow_runs",

		pubMsg.Channel)

}

func TestWorkflowStepRunHandler_Collect(t *testing.T) {
	t.Parallel()
	h := NewWorkflowStepRunHandler(&mockPublisher{}, nil)
	msg := Message{
		Action:  ActionInsert,
		Record:  json.RawMessage(`{"id":"step-1","workflow_run_id":"wfr-1","step_ref":"build","status":"running"}`),
		Changes: json.RawMessage(`{}`),
		Metadata: Metadata{
			TableName:       "workflow_step_runs",
			CommitTimestamp: "2026-03-18T00:00:00Z",
		},
	}

	pubMsg, err := h.Collect(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(
		t, "cdc:workflow_run:wfr-1:steps",

		pubMsg.Channel)

}

func TestEventTriggerHandler_Collect(t *testing.T) {
	t.Parallel()
	h := NewEventTriggerHandler(&mockPublisher{}, nil)
	msg := Message{
		Action:  ActionInsert,
		Record:  json.RawMessage(`{"id":"et-1","event_key":"deploy","project_id":"proj-3","status":"pending"}`),
		Changes: json.RawMessage(`{}`),
		Metadata: Metadata{
			TableName:       "event_triggers",
			CommitTimestamp: "2026-03-18T00:00:00Z",
		},
	}

	pubMsg, err := h.Collect(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(
		t, "cdc:project:proj-3:event_triggers",

		pubMsg.Channel)

}

func TestCollect_InvalidRecord(t *testing.T) {
	t.Parallel()
	h := NewJobRunHandler(&mockPublisher{}, nil)
	msg := Message{
		Action:  ActionInsert,
		Record:  json.RawMessage(`{invalid json`),
		Changes: json.RawMessage(`{}`),
		Metadata: Metadata{
			TableName: "job_runs",
		},
	}

	_, err := h.Collect(context.Background(), msg)
	require.Error(t, err)

}

func TestCollectableHandler_Interface(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{}

	// All four handlers should implement CollectableHandler.
	handlers := []CollectableHandler{
		NewJobRunHandler(pub, nil),
		NewWorkflowRunHandler(pub, nil),
		NewWorkflowStepRunHandler(pub, nil),
		NewEventTriggerHandler(pub, nil),
	}

	for _, h := range handlers {
		assert.NotEqual(t, "", h.Table())

	}
}

func TestCollectChangeEvent(t *testing.T) {
	t.Parallel()
	event := ChangeEvent{
		Table:     "job_runs",
		Action:    ActionInsert,
		Record:    json.RawMessage(`{"id":"r1"}`),
		Timestamp: "2026-03-18T00:00:00Z",
		Source:    "cdc",
	}

	msg, err := collectChangeEvent(event, "cdc:project:p1:job_runs")
	require.NoError(t, err)
	assert.Equal(
		t, "cdc:project:p1:job_runs",

		msg.
			Channel)
	assert.NotEmpty(t, msg.Data)

}
