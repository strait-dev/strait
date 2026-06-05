package cdc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseChangeEvent_MalformedJSON verifies that handlers return errors
// when given a Message with invalid JSON in the Record field.
func TestParseChangeEvent_MalformedJSON(t *testing.T) {
	t.Parallel()

	malformed := []json.RawMessage{
		json.RawMessage(`{broken`),
		json.RawMessage(`not json at all`),
		json.RawMessage(``),
		json.RawMessage(`{"unterminated": `),
	}

	pub := &mockPublisher{}
	handler := NewJobRunHandler(pub, nil)

	for _, raw := range malformed {
		msg := Message{
			Action:   ActionInsert,
			Record:   raw,
			Metadata: Metadata{TableName: "job_runs"},
		}

		err := handler.Handle(context.Background(), msg)
		assert.Error(t, err)
	}
}

// TestParseChangeEvent_MissingFields verifies that a record with missing
// required fields still produces a change event without error (the handler
// extracts what it can from the record).
func TestParseChangeEvent_MissingFields(t *testing.T) {
	t.Parallel()

	pub := &mockPublisher{}
	handler := NewJobRunHandler(pub, nil)

	// Valid JSON but missing all expected fields.
	msg := Message{
		Action:   ActionInsert,
		Record:   json.RawMessage(`{}`),
		Metadata: Metadata{TableName: "job_runs", CommitTimestamp: "2026-01-01T00:00:00Z"},
	}

	err := handler.Handle(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, pub.calls, 1)

	// Verify publish was called (the handler publishes even with empty fields).

	var event ChangeEvent
	require.NoError(t, json.Unmarshal(pub.calls[0].data, &event))
	assert.Equal(t, "job_runs", event.Table)
}

// TestParseChangeEvent_UnknownAction verifies that an unknown action string
// is passed through without error. The CDC types do not validate Action values.
func TestParseChangeEvent_UnknownAction(t *testing.T) {
	t.Parallel()

	pub := &mockPublisher{}
	handler := NewJobRunHandler(pub, nil)

	msg := Message{
		Action:   Action("drop_table"),
		Record:   json.RawMessage(`{"id":"r1","job_id":"j1","project_id":"p1","status":"completed"}`),
		Metadata: Metadata{TableName: "job_runs", CommitTimestamp: "2026-01-01T00:00:00Z"},
	}

	err := handler.Handle(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, pub.calls, 1)

	var event ChangeEvent
	require.NoError(t, json.Unmarshal(pub.calls[0].data, &event))
	assert.Equal(t, Action("drop_table"), event.Action)
}

// TestBatchProcess_MixedValidInvalid verifies that Collect correctly handles
// a mix of valid and invalid records. Valid records produce messages; invalid
// records return errors.
func TestBatchProcess_MixedValidInvalid(t *testing.T) {
	t.Parallel()

	pub := &mockPublisher{}
	handler := NewJobRunHandler(pub, nil)

	cases := []struct {
		name    string
		record  json.RawMessage
		wantErr bool
	}{
		{
			name:    "valid record",
			record:  json.RawMessage(`{"id":"r1","job_id":"j1","project_id":"p1","status":"completed"}`),
			wantErr: false,
		},
		{
			name:    "malformed json",
			record:  json.RawMessage(`{broken`),
			wantErr: true,
		},
		{
			name:    "another valid record",
			record:  json.RawMessage(`{"id":"r2","job_id":"j2","project_id":"p2","status":"failed"}`),
			wantErr: false,
		},
		{
			name:    "empty string",
			record:  json.RawMessage(``),
			wantErr: true,
		},
		{
			name:    "null json",
			record:  json.RawMessage(`null`),
			wantErr: false,
		},
	}

	var successCount int
	for _, tc := range cases {
		msg := Message{
			Action:   ActionInsert,
			Record:   tc.record,
			Metadata: Metadata{TableName: "job_runs", CommitTimestamp: "2026-01-01T00:00:00Z"},
		}

		pubMsg, err := handler.Collect(context.Background(), msg)
		if tc.wantErr {
			assert.Error(t, err)
			continue
		}
		if err != nil {
			assert.Failf(t, "test failure",

				"%s: unexpected error: %v", tc.name, err)
			continue
		}
		if pubMsg == nil {
			assert.Failf(t, "test failure",

				"%s: expected non-nil message", tc.name)
			continue
		}
		successCount++
	}
	assert.EqualValues(t, 3, successCount)
}

// FuzzChangeEventParsing fuzzes raw CDC event JSON to check for panics
// in the JobRunHandler.Handle path.
func FuzzChangeEventParsing(f *testing.F) {
	f.Add([]byte(`{"id":"r1","job_id":"j1","project_id":"p1","status":"completed"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{broken`))
	f.Add([]byte(`null`))
	f.Add([]byte(`[]`))
	f.Add([]byte{0x00, 0x01, 0x02})

	pub := &mockPublisher{}
	handler := NewJobRunHandler(pub, nil)

	f.Fuzz(func(t *testing.T, record []byte) {
		msg := Message{
			Action:   ActionInsert,
			Record:   record,
			Metadata: Metadata{TableName: "job_runs", CommitTimestamp: "2026-01-01T00:00:00Z"},
		}
		// Must not panic.
		_ = handler.Handle(context.Background(), msg)
	})
}
