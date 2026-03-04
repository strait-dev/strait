package cdc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

type publishCall struct {
	channel string
	data    []byte
}

type mockPublisher struct {
	publishFn func(ctx context.Context, channel string, data []byte) error
	calls     []publishCall
}

func (m *mockPublisher) Publish(ctx context.Context, channel string, data []byte) error {
	m.calls = append(m.calls, publishCall{channel: channel, data: append([]byte(nil), data...)})
	if m.publishFn != nil {
		return m.publishFn(ctx, channel, data)
	}
	return nil
}

func TestJobRunHandlerTable(t *testing.T) {
	h := NewJobRunHandler(nil, nil)
	if got := h.Table(); got != "job_runs" {
		t.Fatalf("Table() = %q, want %q", got, "job_runs")
	}
}

func TestJobRunHandlerHandlePublishes(t *testing.T) {
	tests := []struct {
		name   string
		action Action
	}{
		{name: "insert", action: ActionInsert},
		{name: "update", action: ActionUpdate},
		{name: "delete", action: ActionDelete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub := &mockPublisher{}
			logger, logs := newBufferedLogger()
			h := NewJobRunHandler(pub, logger)

			msg := Message{
				Action:  tt.action,
				Record:  json.RawMessage(`{"id":"run_1","job_id":"job_1","project_id":"proj_1","status":"queued"}`),
				Changes: json.RawMessage(`{"status":"queued"}`),
				Metadata: Metadata{
					CommitTimestamp: "2026-01-01T00:00:00Z",
				},
			}

			if err := h.Handle(context.Background(), msg); err != nil {
				t.Fatalf("Handle returned error: %v", err)
			}

			if len(pub.calls) != 1 {
				t.Fatalf("publish calls = %d, want 1", len(pub.calls))
			}

			call := pub.calls[0]
			if call.channel != "cdc:project:proj_1:job_runs" {
				t.Fatalf("channel = %q, want %q", call.channel, "cdc:project:proj_1:job_runs")
			}

			var event ChangeEvent
			if err := json.Unmarshal(call.data, &event); err != nil {
				t.Fatalf("unmarshal published event: %v", err)
			}
			if event.Table != "job_runs" {
				t.Fatalf("event table = %q, want %q", event.Table, "job_runs")
			}
			if event.Action != tt.action {
				t.Fatalf("event action = %q, want %q", event.Action, tt.action)
			}
			if string(event.Record) != string(msg.Record) {
				t.Fatalf("event record = %s, want %s", string(event.Record), string(msg.Record))
			}
			if string(event.Changes) != string(msg.Changes) {
				t.Fatalf("event changes = %s, want %s", string(event.Changes), string(msg.Changes))
			}
			if event.Timestamp != msg.Metadata.CommitTimestamp {
				t.Fatalf("event timestamp = %q, want %q", event.Timestamp, msg.Metadata.CommitTimestamp)
			}

			if tt.action == ActionUpdate {
				if !strings.Contains(logs.String(), `"action":"update"`) {
					t.Fatalf("logs do not contain update action: %s", logs.String())
				}
			}
		})
	}
}

func TestJobRunHandlerHandleNilPublisher(t *testing.T) {
	logger, _ := newBufferedLogger()
	h := NewJobRunHandler(nil, logger)
	msg := Message{
		Action: ActionInsert,
		Record: json.RawMessage(`{"id":"run_1","job_id":"job_1","project_id":"proj_1","status":"queued"}`),
	}

	if err := h.Handle(context.Background(), msg); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
}

func TestJobRunHandlerHandleInvalidRecord(t *testing.T) {
	h := NewJobRunHandler(&mockPublisher{}, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	msg := Message{Action: ActionInsert, Record: json.RawMessage(`{`)}

	err := h.Handle(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "decode job_run record") {
		t.Fatalf("error = %q, want decode job_run record", err.Error())
	}
}

func TestJobRunHandlerHandlePublisherErrorBestEffort(t *testing.T) {
	pub := &mockPublisher{publishFn: func(context.Context, string, []byte) error { return errors.New("boom") }}
	logger, logs := newBufferedLogger()
	h := NewJobRunHandler(pub, logger)
	msg := Message{
		Action: ActionInsert,
		Record: json.RawMessage(`{"id":"run_1","job_id":"job_1","project_id":"proj_1","status":"queued"}`),
	}

	if err := h.Handle(context.Background(), msg); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("publish calls = %d, want 1", len(pub.calls))
	}
	if !strings.Contains(logs.String(), "failed to publish cdc event") {
		t.Fatalf("warning log not found: %s", logs.String())
	}
}

func TestWorkflowRunHandlerTable(t *testing.T) {
	h := NewWorkflowRunHandler(nil, nil)
	if got := h.Table(); got != "workflow_runs" {
		t.Fatalf("Table() = %q, want %q", got, "workflow_runs")
	}
}

func TestWorkflowRunHandlerHandlePatterns(t *testing.T) {
	tests := []struct {
		name   string
		action Action
	}{
		{name: "insert", action: ActionInsert},
		{name: "update", action: ActionUpdate},
		{name: "delete", action: ActionDelete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub := &mockPublisher{}
			h := NewWorkflowRunHandler(pub, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
			msg := Message{
				Action:  tt.action,
				Record:  json.RawMessage(`{"id":"wf_run_1","workflow_id":"wf_1","project_id":"proj_9","status":"running"}`),
				Changes: json.RawMessage(`{"status":"running"}`),
				Metadata: Metadata{
					CommitTimestamp: "2026-01-01T00:00:00Z",
				},
			}

			if err := h.Handle(context.Background(), msg); err != nil {
				t.Fatalf("Handle returned error: %v", err)
			}

			if len(pub.calls) != 1 {
				t.Fatalf("publish calls = %d, want 1", len(pub.calls))
			}

			call := pub.calls[0]
			if call.channel != "cdc:project:proj_9:workflow_runs" {
				t.Fatalf("channel = %q, want %q", call.channel, "cdc:project:proj_9:workflow_runs")
			}

			var event ChangeEvent
			if err := json.Unmarshal(call.data, &event); err != nil {
				t.Fatalf("unmarshal published event: %v", err)
			}
			if event.Table != "workflow_runs" {
				t.Fatalf("event table = %q, want %q", event.Table, "workflow_runs")
			}
			if event.Action != tt.action {
				t.Fatalf("event action = %q, want %q", event.Action, tt.action)
			}
			if string(event.Record) != string(msg.Record) {
				t.Fatalf("event record = %s, want %s", string(event.Record), string(msg.Record))
			}
			if string(event.Changes) != string(msg.Changes) {
				t.Fatalf("event changes = %s, want %s", string(event.Changes), string(msg.Changes))
			}
			if event.Timestamp != msg.Metadata.CommitTimestamp {
				t.Fatalf("event timestamp = %q, want %q", event.Timestamp, msg.Metadata.CommitTimestamp)
			}
		})
	}
}

func TestWorkflowRunHandlerHandleNilPublisher(t *testing.T) {
	h := NewWorkflowRunHandler(nil, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	msg := Message{Action: ActionInsert, Record: json.RawMessage(`{"id":"wf_run_1","workflow_id":"wf_1","project_id":"proj_9","status":"running"}`)}
	if err := h.Handle(context.Background(), msg); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
}

func TestWorkflowRunHandlerHandleInvalidRecord(t *testing.T) {
	h := NewWorkflowRunHandler(&mockPublisher{}, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	err := h.Handle(context.Background(), Message{Action: ActionInsert, Record: json.RawMessage(`{`)})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "decode workflow_run record") {
		t.Fatalf("error = %q, want decode workflow_run record", err.Error())
	}
}

func TestWorkflowRunHandlerHandlePublisherErrorBestEffort(t *testing.T) {
	pub := &mockPublisher{publishFn: func(context.Context, string, []byte) error { return errors.New("boom") }}
	logger, logs := newBufferedLogger()
	h := NewWorkflowRunHandler(pub, logger)
	msg := Message{Action: ActionInsert, Record: json.RawMessage(`{"id":"wf_run_1","workflow_id":"wf_1","project_id":"proj_9","status":"running"}`)}

	if err := h.Handle(context.Background(), msg); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("publish calls = %d, want 1", len(pub.calls))
	}
	if !strings.Contains(logs.String(), "failed to publish cdc event") {
		t.Fatalf("warning log not found: %s", logs.String())
	}
}

func TestWorkflowStepRunHandlerTable(t *testing.T) {
	h := NewWorkflowStepRunHandler(nil, nil)
	if got := h.Table(); got != "workflow_step_runs" {
		t.Fatalf("Table() = %q, want %q", got, "workflow_step_runs")
	}
}

func TestWorkflowStepRunHandlerHandlePatterns(t *testing.T) {
	tests := []struct {
		name   string
		action Action
	}{
		{name: "insert", action: ActionInsert},
		{name: "update", action: ActionUpdate},
		{name: "delete", action: ActionDelete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub := &mockPublisher{}
			h := NewWorkflowStepRunHandler(pub, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
			msg := Message{
				Action:  tt.action,
				Record:  json.RawMessage(`{"id":"step_run_1","workflow_run_id":"wf_run_123","step_ref":"build","status":"running"}`),
				Changes: json.RawMessage(`{"status":"running"}`),
				Metadata: Metadata{
					CommitTimestamp: "2026-01-01T00:00:00Z",
				},
			}

			if err := h.Handle(context.Background(), msg); err != nil {
				t.Fatalf("Handle returned error: %v", err)
			}

			if len(pub.calls) != 1 {
				t.Fatalf("publish calls = %d, want 1", len(pub.calls))
			}

			call := pub.calls[0]
			if call.channel != "cdc:workflow_run:wf_run_123:steps" {
				t.Fatalf("channel = %q, want %q", call.channel, "cdc:workflow_run:wf_run_123:steps")
			}

			var event ChangeEvent
			if err := json.Unmarshal(call.data, &event); err != nil {
				t.Fatalf("unmarshal published event: %v", err)
			}
			if event.Table != "workflow_step_runs" {
				t.Fatalf("event table = %q, want %q", event.Table, "workflow_step_runs")
			}
			if event.Action != tt.action {
				t.Fatalf("event action = %q, want %q", event.Action, tt.action)
			}
			if string(event.Record) != string(msg.Record) {
				t.Fatalf("event record = %s, want %s", string(event.Record), string(msg.Record))
			}
			if string(event.Changes) != string(msg.Changes) {
				t.Fatalf("event changes = %s, want %s", string(event.Changes), string(msg.Changes))
			}
			if event.Timestamp != msg.Metadata.CommitTimestamp {
				t.Fatalf("event timestamp = %q, want %q", event.Timestamp, msg.Metadata.CommitTimestamp)
			}
		})
	}
}

func TestWorkflowStepRunHandlerHandleNilPublisher(t *testing.T) {
	h := NewWorkflowStepRunHandler(nil, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	msg := Message{Action: ActionInsert, Record: json.RawMessage(`{"id":"step_run_1","workflow_run_id":"wf_run_123","step_ref":"build","status":"running"}`)}
	if err := h.Handle(context.Background(), msg); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
}

func TestWorkflowStepRunHandlerHandleInvalidRecord(t *testing.T) {
	h := NewWorkflowStepRunHandler(&mockPublisher{}, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	err := h.Handle(context.Background(), Message{Action: ActionInsert, Record: json.RawMessage(`{`)})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "decode workflow_step_run record") {
		t.Fatalf("error = %q, want decode workflow_step_run record", err.Error())
	}
}

func TestWorkflowStepRunHandlerHandlePublisherErrorBestEffort(t *testing.T) {
	pub := &mockPublisher{publishFn: func(context.Context, string, []byte) error { return errors.New("boom") }}
	logger, logs := newBufferedLogger()
	h := NewWorkflowStepRunHandler(pub, logger)
	msg := Message{Action: ActionInsert, Record: json.RawMessage(`{"id":"step_run_1","workflow_run_id":"wf_run_123","step_ref":"build","status":"running"}`)}

	if err := h.Handle(context.Background(), msg); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("publish calls = %d, want 1", len(pub.calls))
	}
	if !strings.Contains(logs.String(), "failed to publish cdc event") {
		t.Fatalf("warning log not found: %s", logs.String())
	}
}

func TestChangeEventMarshalWithChanges(t *testing.T) {
	e := ChangeEvent{
		Table:     "job_runs",
		Action:    ActionUpdate,
		Record:    json.RawMessage(`{"id":"run_1"}`),
		Changes:   json.RawMessage(`{"status":"completed"}`),
		Timestamp: "2026-01-01T00:00:00Z",
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal ChangeEvent: %v", err)
	}

	if !strings.Contains(string(data), `"changes":{`) {
		t.Fatalf("marshaled event missing changes field: %s", string(data))
	}
}

func TestChangeEventMarshalOmitsNilChanges(t *testing.T) {
	e := ChangeEvent{
		Table:     "job_runs",
		Action:    ActionInsert,
		Record:    json.RawMessage(`{"id":"run_1"}`),
		Timestamp: "2026-01-01T00:00:00Z",
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal ChangeEvent: %v", err)
	}

	if strings.Contains(string(data), `"changes":`) {
		t.Fatalf("marshaled event should omit changes when nil: %s", string(data))
	}
}

func newBufferedLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewJSONHandler(buf, nil)), buf
}
