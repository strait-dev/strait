package cdc

import (
	"context"
	"encoding/json"
	"testing"
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
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if pubMsg == nil {
		t.Fatal("Collect() returned nil message")
		return
	}
	if pubMsg.Channel != "cdc:project:proj-1:job_runs" {
		t.Errorf("Channel = %q, want %q", pubMsg.Channel, "cdc:project:proj-1:job_runs")
	}

	var event ChangeEvent
	if err := json.Unmarshal(pubMsg.Data, &event); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if event.Table != "job_runs" {
		t.Errorf("event.Table = %q, want %q", event.Table, "job_runs")
	}
	if event.Source != "cdc" {
		t.Errorf("event.Source = %q, want %q", event.Source, "cdc")
	}
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
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if pubMsg.Channel != "cdc:project:proj-2:workflow_runs" {
		t.Errorf("Channel = %q, want %q", pubMsg.Channel, "cdc:project:proj-2:workflow_runs")
	}
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
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if pubMsg.Channel != "cdc:workflow_run:wfr-1:steps" {
		t.Errorf("Channel = %q, want %q", pubMsg.Channel, "cdc:workflow_run:wfr-1:steps")
	}
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
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if pubMsg.Channel != "cdc:project:proj-3:event_triggers" {
		t.Errorf("Channel = %q, want %q", pubMsg.Channel, "cdc:project:proj-3:event_triggers")
	}
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
	if err == nil {
		t.Fatal("Collect() with invalid record should return error")
	}
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
		if h.Table() == "" {
			t.Errorf("handler %T has empty table", h)
		}
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
	if err != nil {
		t.Fatalf("collectChangeEvent() error = %v", err)
	}
	if msg.Channel != "cdc:project:p1:job_runs" {
		t.Errorf("Channel = %q, want %q", msg.Channel, "cdc:project:p1:job_runs")
	}
	if len(msg.Data) == 0 {
		t.Error("Data should not be empty")
	}
}
