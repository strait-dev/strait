package cdc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"strait/internal/pubsub"

	"github.com/stretchr/testify/require"
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

func (m *mockPublisher) PublishBatch(ctx context.Context, messages []pubsub.PubSubMessage) error {
	for _, msg := range messages {
		if err := m.Publish(ctx, msg.Channel, msg.Data); err != nil {
			return err
		}
	}
	return nil
}

func TestJobRunHandlerTable(t *testing.T) {
	t.Parallel()
	h := NewJobRunHandler(nil, nil)
	require.Equal(t, "job_runs",
		h.Table())
}

func TestJobRunHandlerHandlePublishes(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
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
			require.NoError(t, h.Handle(context.Background(),
				msg))
			require.Len(t, pub.calls,
				1)

			call := pub.calls[0]
			require.Equal(t, "cdc:project:proj_1:job_runs",

				call.
					channel)

			var event ChangeEvent
			require.NoError(t, json.
				Unmarshal(call.
					data, &event,
				))
			require.Equal(t, "job_runs",
				event.Table,
			)
			require.Equal(t, tt.action,
				event.Action,
			)
			require.Equal(t, string(msg.Record), string(event.
				Record))
			require.Equal(t, string(msg.Changes), string(event.
				Changes))
			require.Equal(t, msg.Metadata.
				CommitTimestamp,
				event.
					Timestamp,
			)

			if tt.action == ActionUpdate {
				require.Contains(t, logs.String(), `"action":"update"`)
			}
		})
	}
}

func TestJobRunHandlerHandleNilPublisher(t *testing.T) {
	t.Parallel()
	logger, _ := newBufferedLogger()
	h := NewJobRunHandler(nil, logger)
	msg := Message{
		Action: ActionInsert,
		Record: json.RawMessage(`{"id":"run_1","job_id":"job_1","project_id":"proj_1","status":"queued"}`),
	}
	require.NoError(t, h.Handle(context.Background(),
		msg))
}

func TestJobRunHandlerHandleInvalidRecord(t *testing.T) {
	t.Parallel()
	h := NewJobRunHandler(&mockPublisher{}, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	msg := Message{Action: ActionInsert, Record: json.RawMessage(`{`)}

	err := h.Handle(context.Background(), msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode job_run record")
}

func TestJobRunHandlerHandlePublisherErrorBestEffort(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{publishFn: func(context.Context, string, []byte) error { return errors.New("boom") }}
	logger, logs := newBufferedLogger()
	h := NewJobRunHandler(pub, logger)
	msg := Message{
		Action: ActionInsert,
		Record: json.RawMessage(`{"id":"run_1","job_id":"job_1","project_id":"proj_1","status":"queued"}`),
	}
	require.NoError(t, h.Handle(context.Background(),
		msg))
	require.Len(t, pub.calls,
		1)
	require.Contains(t, logs.String(), "failed to publish cdc event")
}

func TestWorkflowRunHandlerTable(t *testing.T) {
	t.Parallel()
	h := NewWorkflowRunHandler(nil, nil)
	require.Equal(t, "workflow_runs",
		h.Table())
}

func TestWorkflowRunHandlerHandlePatterns(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
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
			require.NoError(t, h.Handle(context.Background(),
				msg))
			require.Len(t, pub.calls,
				1)

			call := pub.calls[0]
			require.Equal(t, "cdc:project:proj_9:workflow_runs",

				call.channel,
			)

			var event ChangeEvent
			require.NoError(t, json.
				Unmarshal(call.
					data, &event,
				))
			require.Equal(t, "workflow_runs",
				event.
					Table)
			require.Equal(t, tt.action,
				event.Action,
			)
			require.Equal(t, string(msg.Record), string(event.
				Record))
			require.Equal(t, string(msg.Changes), string(event.
				Changes))
			require.Equal(t, msg.Metadata.
				CommitTimestamp,
				event.
					Timestamp,
			)
		})
	}
}

func TestWorkflowRunHandlerHandleNilPublisher(t *testing.T) {
	t.Parallel()
	h := NewWorkflowRunHandler(nil, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	msg := Message{Action: ActionInsert, Record: json.RawMessage(`{"id":"wf_run_1","workflow_id":"wf_1","project_id":"proj_9","status":"running"}`)}
	require.NoError(t, h.Handle(context.Background(),
		msg))
}

func TestWorkflowRunHandlerHandleInvalidRecord(t *testing.T) {
	t.Parallel()
	h := NewWorkflowRunHandler(&mockPublisher{}, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	err := h.Handle(context.Background(), Message{Action: ActionInsert, Record: json.RawMessage(`{`)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode workflow_run record")
}

func TestWorkflowRunHandlerHandlePublisherErrorBestEffort(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{publishFn: func(context.Context, string, []byte) error { return errors.New("boom") }}
	logger, logs := newBufferedLogger()
	h := NewWorkflowRunHandler(pub, logger)
	msg := Message{Action: ActionInsert, Record: json.RawMessage(`{"id":"wf_run_1","workflow_id":"wf_1","project_id":"proj_9","status":"running"}`)}
	require.NoError(t, h.Handle(context.Background(),
		msg))
	require.Len(t, pub.calls,
		1)
	require.Contains(t, logs.String(), "failed to publish cdc event")
}

func TestWorkflowStepRunHandlerTable(t *testing.T) {
	t.Parallel()
	h := NewWorkflowStepRunHandler(nil, nil)
	require.Equal(t, "workflow_step_runs",

		h.Table())
}

func TestWorkflowStepRunHandlerHandlePatterns(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
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
			require.NoError(t, h.Handle(context.Background(),
				msg))
			require.Len(t, pub.calls,
				1)

			call := pub.calls[0]
			require.Equal(t, "cdc:workflow_run:wf_run_123:steps",

				call.channel,
			)

			var event ChangeEvent
			require.NoError(t, json.
				Unmarshal(call.
					data, &event,
				))
			require.Equal(t, "workflow_step_runs",

				event.Table,
			)
			require.Equal(t, tt.action,
				event.Action,
			)
			require.Equal(t, string(msg.Record), string(event.
				Record))
			require.Equal(t, string(msg.Changes), string(event.
				Changes))
			require.Equal(t, msg.Metadata.
				CommitTimestamp,
				event.
					Timestamp,
			)
		})
	}
}

func TestWorkflowStepRunHandlerHandleNilPublisher(t *testing.T) {
	t.Parallel()
	h := NewWorkflowStepRunHandler(nil, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	msg := Message{Action: ActionInsert, Record: json.RawMessage(`{"id":"step_run_1","workflow_run_id":"wf_run_123","step_ref":"build","status":"running"}`)}
	require.NoError(t, h.Handle(context.Background(),
		msg))
}

func TestWorkflowStepRunHandlerHandleInvalidRecord(t *testing.T) {
	t.Parallel()
	h := NewWorkflowStepRunHandler(&mockPublisher{}, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	err := h.Handle(context.Background(), Message{Action: ActionInsert, Record: json.RawMessage(`{`)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode workflow_step_run record")
}

func TestWorkflowStepRunHandlerHandlePublisherErrorBestEffort(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{publishFn: func(context.Context, string, []byte) error { return errors.New("boom") }}
	logger, logs := newBufferedLogger()
	h := NewWorkflowStepRunHandler(pub, logger)
	msg := Message{Action: ActionInsert, Record: json.RawMessage(`{"id":"step_run_1","workflow_run_id":"wf_run_123","step_ref":"build","status":"running"}`)}
	require.NoError(t, h.Handle(context.Background(),
		msg))
	require.Len(t, pub.calls,
		1)
	require.Contains(t, logs.String(), "failed to publish cdc event")
}

func TestChangeEventMarshalWithChanges(t *testing.T) {
	t.Parallel()
	e := ChangeEvent{
		Table:     "job_runs",
		Action:    ActionUpdate,
		Record:    json.RawMessage(`{"id":"run_1"}`),
		Changes:   json.RawMessage(`{"status":"completed"}`),
		Timestamp: "2026-01-01T00:00:00Z",
	}

	data, err := json.Marshal(e)
	require.NoError(t, err)
	require.Contains(t, string(data), `"changes":{`)
}

func TestChangeEventMarshalOmitsNilChanges(t *testing.T) {
	t.Parallel()
	e := ChangeEvent{
		Table:     "job_runs",
		Action:    ActionInsert,
		Record:    json.RawMessage(`{"id":"run_1"}`),
		Timestamp: "2026-01-01T00:00:00Z",
	}

	data, err := json.Marshal(e)
	require.NoError(t, err)
	require.NotContains(t, string(data), `"changes":`)
}

func newBufferedLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewJSONHandler(buf, nil)), buf
}

func TestEventTriggerHandlerTable(t *testing.T) {
	t.Parallel()
	h := NewEventTriggerHandler(nil, nil)
	require.Equal(t, "event_triggers",
		h.Table())
}

func TestEventTriggerHandlerHandlePublishes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		action Action
	}{
		{name: "insert", action: ActionInsert},
		{name: "update", action: ActionUpdate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pub := &mockPublisher{}
			logger, logs := newBufferedLogger()
			h := NewEventTriggerHandler(pub, logger)

			msg := Message{
				Action:  tt.action,
				Record:  json.RawMessage(`{"id":"evt_1","event_key":"aml:app-1","project_id":"proj_1","status":"waiting"}`),
				Changes: json.RawMessage(`{"status":"waiting"}`),
				Metadata: Metadata{
					TableName:       "event_triggers",
					CommitTimestamp: "2026-01-01T00:00:00Z",
				},
				AckID: "ack-1",
			}
			require.NoError(t, h.Handle(context.Background(),
				msg))
			require.Len(t, pub.calls,
				1)
			require.Equal(t, "cdc:project:proj_1:event_triggers",

				pub.calls[0].channel,
			)

			var event ChangeEvent
			require.NoError(t, json.
				Unmarshal(pub.calls[0].data,
					&event))
			require.Equal(t, "event_triggers",
				event.
					Table)
			require.Equal(t, tt.action,
				event.Action,
			)

			logOutput := logs.String()
			require.Contains(t, logOutput, "cdc event_trigger change")
			require.Contains(t, logOutput, "aml:app-1")
		})
	}
}

func TestEventTriggerHandlerBadRecord(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{}
	h := NewEventTriggerHandler(pub, slog.Default())

	msg := Message{
		Action: ActionInsert,
		Record: json.RawMessage(`{invalid`),
	}
	require.Error(t, h.Handle(context.Background(), msg))
	require.Empty(t, pub.calls)
}

func TestEventTriggerHandlerNilPublisher(t *testing.T) {
	t.Parallel()
	h := NewEventTriggerHandler(nil, nil)

	msg := Message{
		Action: ActionInsert,
		Record: json.RawMessage(`{"id":"evt_1","event_key":"key","project_id":"proj_1","status":"waiting"}`),
		Metadata: Metadata{
			TableName:       "event_triggers",
			CommitTimestamp: "2026-01-01T00:00:00Z",
		},
	}
	require.NoError(t, h.Handle(context.Background(),
		msg))
}

func TestEventTriggerHandlerPublishError(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{
		publishFn: func(_ context.Context, _ string, _ []byte) error {
			return errors.New("redis down")
		},
	}
	logger, logs := newBufferedLogger()
	h := NewEventTriggerHandler(pub, logger)

	msg := Message{
		Action: ActionUpdate,
		Record: json.RawMessage(`{"id":"evt_1","event_key":"key","project_id":"proj_1","status":"received"}`),
		Metadata: Metadata{
			TableName:       "event_triggers",
			CommitTimestamp: "2026-01-01T00:00:00Z",
		},
	}
	require.NoError(t, h.Handle(context.Background(),
		msg))

	// Should not return error (publish errors are logged, not returned)

	logOutput := logs.String()
	require.Contains(t, logOutput, "failed to publish cdc event")
}

func TestChangeEventSourceField(t *testing.T) {
	t.Parallel()
	pub := &mockPublisher{}
	h := NewJobRunHandler(pub, slog.Default())

	msg := Message{
		Action: ActionInsert,
		Record: json.RawMessage(`{"id":"run_1","job_id":"job_1","project_id":"proj_1","status":"queued"}`),
		Metadata: Metadata{
			TableName:       "job_runs",
			CommitTimestamp: "2026-01-01T00:00:00Z",
		},
	}
	require.NoError(t, h.Handle(context.Background(),
		msg))
	require.Len(t, pub.calls,
		1)

	var event ChangeEvent
	require.NoError(t, json.
		Unmarshal(pub.calls[0].data,
			&event))
	require.Equal(t, "cdc",
		event.Source)
}

func TestAllHandlersIncludeSource(t *testing.T) {
	t.Parallel()

	tables := []string{"job_runs", "workflow_runs", "workflow_step_runs", "event_triggers"}
	records := map[string]json.RawMessage{
		"job_runs":           json.RawMessage(`{"id":"1","job_id":"j","project_id":"p","status":"queued"}`),
		"workflow_runs":      json.RawMessage(`{"id":"1","workflow_id":"w","project_id":"p","status":"running"}`),
		"workflow_step_runs": json.RawMessage(`{"id":"1","workflow_run_id":"wr","step_ref":"s","status":"pending"}`),
		"event_triggers":     json.RawMessage(`{"id":"1","event_key":"k","project_id":"p","status":"waiting"}`),
	}

	for _, table := range tables {
		pub := &mockPublisher{}
		var handler Handler
		switch table {
		case "job_runs":
			handler = NewJobRunHandler(pub, slog.Default())
		case "workflow_runs":
			handler = NewWorkflowRunHandler(pub, slog.Default())
		case "workflow_step_runs":
			handler = NewWorkflowStepRunHandler(pub, slog.Default())
		case "event_triggers":
			handler = NewEventTriggerHandler(pub, slog.Default())
		}

		msg := Message{
			Action:   ActionUpdate,
			Record:   records[table],
			Metadata: Metadata{TableName: table, CommitTimestamp: "2026-01-01T00:00:00Z"},
		}
		require.NoError(t, handler.
			Handle(context.
				Background(), msg))
		require.Len(t, pub.calls,
			1)

		var event ChangeEvent
		require.NoError(t, json.
			Unmarshal(pub.calls[0].data,
				&event))
		require.Equal(t, "cdc",
			event.Source)
	}
}
