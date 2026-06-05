package dbscan

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNilIfEmptyString(t *testing.T) {
	t.Parallel()
	t.Run("empty returns nil", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, NilIfEmptyString(""))
	})

	t.Run("non-empty returns value", func(t *testing.T) {
		t.Parallel()
		got := NilIfEmptyString("hello")
		s, ok := got.(string)
		require.True(t, ok, "NilIfEmptyString(%q) type = %T, want string", "hello", got)
		assert.Equal(t, "hello", s)
	})

	t.Run("whitespace returns value", func(t *testing.T) {
		t.Parallel()
		got := NilIfEmptyString(" ")
		require.NotNil(t, got)
		assert.Equal(t, " ", got.(string))
	})

	t.Run("long string returns value", func(t *testing.T) {
		t.Parallel()
		input := "this is a longer string with multiple words"
		got := NilIfEmptyString(input)
		require.NotNil(t, got)
		assert.Equal(t, input, got.(string))
	})
}

func TestNilIfEmptyRawMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input json.RawMessage
		isNil bool
	}{
		{"nil input", nil, true},
		{"empty slice", json.RawMessage{}, true},
		{"object", json.RawMessage(`{"k":"v"}`), false},
		{"array", json.RawMessage(`[1,2]`), false},
		{"string", json.RawMessage(`"hi"`), false},
		{"number", json.RawMessage(`42`), false},
		{"boolean", json.RawMessage(`true`), false},
		{"null literal", json.RawMessage(`null`), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NilIfEmptyRawMessage(tt.input)
			if tt.isNil {
				assert.Nil(t, got)
				return
			}
			msg, ok := got.(json.RawMessage)
			require.True(t, ok, "NilIfEmptyRawMessage() type = %T, want json.RawMessage", got)
			assert.Equal(t, string(tt.input), string(msg))
		})
	}
}

func TestNilIfZeroInt(t *testing.T) {
	t.Parallel()
	t.Run("zero returns nil", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, NilIfZeroInt(0))
	})

	t.Run("non-zero returns value", func(t *testing.T) {
		t.Parallel()
		got := NilIfZeroInt(12)
		v, ok := got.(int)
		require.True(t, ok, "NilIfZeroInt(12) type = %T, want int", got)
		assert.Equal(t, 12, v)
	})
}

func TestNilIfEmptyIntSlice(t *testing.T) {
	t.Parallel()

	t.Run("nil slice returns nil", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, NilIfEmptyIntSlice(nil))
	})

	t.Run("empty slice returns nil", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, NilIfEmptyIntSlice([]int{}))
	})

	t.Run("non-empty slice returns value", func(t *testing.T) {
		t.Parallel()
		input := []int{1, 2, 3}
		got := NilIfEmptyIntSlice(input)
		require.NotNil(t, got)
		s, ok := got.([]int)
		require.True(t, ok, "expected []int, got %T", got)
		assert.Equal(t, []int{1, 2, 3}, s)
	})
}

// mockScanner implements Scanner for unit testing ScanRun.
// Uses reflect to assign values to arbitrary destination pointers.
type mockScanner struct {
	values []any
	err    error
}

func (m *mockScanner) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	for i := range dest {
		if i >= len(m.values) {
			break
		}
		dv := reflect.ValueOf(dest[i]).Elem()
		if m.values[i] == nil {
			dv.Set(reflect.Zero(dv.Type()))
			continue
		}
		dv.Set(reflect.ValueOf(m.values[i]))
	}
	return nil
}

func TestScanRun_Error(t *testing.T) {
	t.Parallel()
	scanErr := errors.New("connection reset")
	s := &mockScanner{err: scanErr}

	run, err := ScanRun(s)
	require.Error(t, err)
	assert.Nil(t, run)
}

func TestScanRun_AllFields(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Microsecond)
	started := now.Add(-time.Minute)
	errMsg := "partial failure"
	errorClass := "timeout"
	parentRunID := "parent-001"
	idempotencyKey := "idem-abc"
	workflowStepRunID := "step-run-001"
	continuationOf := "cont-001"
	jobVersionID := "jv-001"
	createdBy := "user-1"
	batchID := "batch-001"
	concurrencyKey := "key-001"
	executionMode := "http"
	replayedRunID := "run-orig-001"
	metadata := []byte(`{"env":"prod","region":"eu"}`)

	s := &mockScanner{
		values: []any{
			"run-001",                     // ID
			"job-001",                     // JobID
			"proj-001",                    // ProjectID
			domain.RunStatus("executing"), // Status
			2,                             // Attempt
			[]byte(`{"input":"data"}`),    // Payload
			[]byte(`{"output":"ok"}`),     // Result
			metadata,
			&errMsg,            // Error
			&errorClass,        // ErrorClass
			"cron",             // TriggeredBy
			&started,           // ScheduledAt
			&started,           // StartedAt
			(*time.Time)(nil),  // FinishedAt
			&now,               // HeartbeatAt
			(*time.Time)(nil),  // NextRetryAt
			(*time.Time)(nil),  // ExpiresAt
			&parentRunID,       // ParentRunID
			5,                  // Priority
			&idempotencyKey,    // IdempotencyKey
			3,                  // JobVersion
			now,                // CreatedAt
			&workflowStepRunID, // WorkflowStepRunID
			[]byte(`{"queue_wait_ms":10,"total_ms":500}`), // ExecutionTrace
			true,                     // DebugMode
			&continuationOf,          // ContinuationOf
			2,                        // LineageDepth
			[]byte(`{"env":"prod"}`), // Tags
			&jobVersionID,            // JobVersionID
			&createdBy,               // CreatedBy
			&batchID,                 // BatchID
			&concurrencyKey,          // ConcurrencyKey
			&executionMode,           // ExecutionMode
			true,                     // IsRollback
			&replayedRunID,           // ReplayedRunID
		},
	}

	run, err := ScanRun(s)
	require.NoError(t, err)

	assert.Equal(t, "run-001", run.ID)
	assert.Equal(t, "job-001", run.JobID)
	assert.Equal(t, "proj-001", run.ProjectID)
	assert.Equal(t, domain.RunStatus("executing"), run.Status)
	assert.Equal(t, 2, run.Attempt)
	assert.JSONEq(t, `{"input":"data"}`, string(run.Payload))
	assert.JSONEq(t, `{"output":"ok"}`, string(run.Result))
	assert.Equal(t, "partial failure", run.Error)
	assert.Equal(t, "timeout", run.ErrorClass)
	assert.Equal(t, map[string]string{"env": "prod", "region": "eu"}, run.Metadata)
	assert.Equal(t, "cron", run.TriggeredBy)
	assert.Equal(t, "parent-001", run.ParentRunID)
	assert.Equal(t, 5, run.Priority)
	assert.Equal(t, "idem-abc", run.IdempotencyKey)
	assert.Equal(t, "step-run-001", run.WorkflowStepRunID)
	assert.NotNil(t, run.ScheduledAt)
	assert.NotNil(t, run.StartedAt)
	assert.NotNil(t, run.HeartbeatAt)
	assert.Nil(t, run.FinishedAt)
	assert.True(t, run.DebugMode)
	assert.Equal(t, "cont-001", run.ContinuationOf)
	assert.Equal(t, 2, run.LineageDepth)
	assert.Equal(t, "prod", run.Tags["env"])
	assert.Equal(t, "jv-001", run.JobVersionID)
	assert.Equal(t, "user-1", run.CreatedBy)
	assert.Equal(t, "batch-001", run.BatchID)
	assert.Equal(t, "key-001", run.ConcurrencyKey)
	assert.Equal(t, domain.ExecutionModeHTTP, run.ExecutionMode)
	assert.True(t, run.IsRollback)
	assert.Equal(t, "run-orig-001", run.ReplayedRunID)
	assert.NotNil(t, run.ExecutionTrace)
}

func TestScanRun_NilOptionals(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Microsecond)

	s := &mockScanner{
		values: []any{
			"run-002",                  // ID
			"job-002",                  // JobID
			"proj-002",                 // ProjectID
			domain.RunStatus("queued"), // Status
			1,                          // Attempt
			nil,                        // Payload (nil bytes)
			nil,                        // Result (nil bytes)
			nil,
			(*string)(nil),    // Error
			(*string)(nil),    // ErrorClass
			"manual",          // TriggeredBy
			(*time.Time)(nil), // ScheduledAt
			(*time.Time)(nil), // StartedAt
			(*time.Time)(nil), // FinishedAt
			(*time.Time)(nil), // HeartbeatAt
			(*time.Time)(nil), // NextRetryAt
			(*time.Time)(nil), // ExpiresAt
			(*string)(nil),    // ParentRunID
			0,                 // Priority
			(*string)(nil),    // IdempotencyKey
			0,                 // JobVersion
			now,               // CreatedAt
			(*string)(nil),    // WorkflowStepRunID
			nil,               // ExecutionTrace
			false,             // DebugMode
			(*string)(nil),    // ContinuationOf
			0,                 // LineageDepth
			nil,               // Tags
			(*string)(nil),    // JobVersionID
			(*string)(nil),    // CreatedBy
			(*string)(nil),    // BatchID
			(*string)(nil),    // ConcurrencyKey
			(*string)(nil),    // ExecutionMode
			false,             // IsRollback
			(*string)(nil),    // ReplayedRunID
		},
	}

	run, err := ScanRun(s)
	require.NoError(t, err)

	assert.Nil(t, run.Payload)
	assert.Nil(t, run.Result)
	assert.Empty(t, run.Error)
	assert.Empty(t, run.ErrorClass)
	assert.Empty(t, run.ParentRunID)
	assert.Empty(t, run.IdempotencyKey)
	assert.Empty(t, run.WorkflowStepRunID)
	assert.Nil(t, run.ScheduledAt)
	assert.Nil(t, run.StartedAt)
	assert.Nil(t, run.FinishedAt)
	assert.False(t, run.DebugMode)
	assert.Empty(t, run.ContinuationOf)
	assert.Zero(t, run.LineageDepth)
	assert.Empty(t, run.Tags)
	assert.Empty(t, run.JobVersionID)
	assert.Empty(t, run.CreatedBy)
	assert.Empty(t, run.BatchID)
	assert.Empty(t, run.ConcurrencyKey)
	assert.Empty(t, run.ExecutionMode)
	assert.False(t, run.IsRollback)
	assert.Empty(t, run.ReplayedRunID)
	assert.Nil(t, run.ExecutionTrace)
}

func TestScanRun_InvalidMetadataJSON(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Microsecond)

	s := &mockScanner{
		values: scanRunBaseValues(now, func(v *scanRunValues) {
			v.metadata = []byte(`{invalid`)
		}),
	}

	run, err := ScanRun(s)
	require.Error(t, err)
	assert.Nil(t, run)
}

func TestScanRun_InvalidExecutionTraceJSON(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Microsecond)

	s := &mockScanner{
		values: scanRunBaseValues(now, func(v *scanRunValues) {
			v.executionTrace = []byte(`not-json`)
		}),
	}

	run, err := ScanRun(s)
	require.Error(t, err)
	assert.Nil(t, run)
}

func TestScanRun_InvalidTagsJSON(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Microsecond)

	s := &mockScanner{
		values: scanRunBaseValues(now, func(v *scanRunValues) {
			v.tags = []byte(`{bad-tags`)
		}),
	}

	run, err := ScanRun(s)
	require.Error(t, err)
	assert.Nil(t, run)
}

func TestScanRun_EmptyObjectTags(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Microsecond)

	s := &mockScanner{
		values: scanRunBaseValues(now, func(v *scanRunValues) {
			v.tags = []byte(`{}`)
		}),
	}

	run, err := ScanRun(s)
	require.NoError(t, err)
	assert.Empty(t, run.Tags)
}

// scanRunValues holds the mutable fields for building mock scanner values.
type scanRunValues struct {
	metadata       []byte
	executionTrace []byte
	tags           []byte
}

// scanRunBaseValues creates a set of mock scanner values with sensible defaults.
// The mutate function allows overriding specific fields for targeted tests.
func scanRunBaseValues(now time.Time, mutate func(*scanRunValues)) []any {
	v := &scanRunValues{
		metadata:       nil,
		executionTrace: nil,
		tags:           nil,
	}
	if mutate != nil {
		mutate(v)
	}
	return []any{
		"run-test",                 // ID
		"job-test",                 // JobID
		"proj-test",                // ProjectID
		domain.RunStatus("queued"), // Status
		1,                          // Attempt
		nil,                        // Payload
		nil,                        // Result
		v.metadata,                 // Metadata
		(*string)(nil),             // Error
		(*string)(nil),             // ErrorClass
		"manual",                   // TriggeredBy
		(*time.Time)(nil),          // ScheduledAt
		(*time.Time)(nil),          // StartedAt
		(*time.Time)(nil),          // FinishedAt
		(*time.Time)(nil),          // HeartbeatAt
		(*time.Time)(nil),          // NextRetryAt
		(*time.Time)(nil),          // ExpiresAt
		(*string)(nil),             // ParentRunID
		0,                          // Priority
		(*string)(nil),             // IdempotencyKey
		0,                          // JobVersion
		now,                        // CreatedAt
		(*string)(nil),             // WorkflowStepRunID
		v.executionTrace,           // ExecutionTrace
		false,                      // DebugMode
		(*string)(nil),             // ContinuationOf
		0,                          // LineageDepth
		v.tags,                     // Tags
		(*string)(nil),             // JobVersionID
		(*string)(nil),             // CreatedBy
		(*string)(nil),             // BatchID
		(*string)(nil),             // ConcurrencyKey
		(*string)(nil),             // ExecutionMode
		false,                      // IsRollback
		(*string)(nil),             // ReplayedRunID
	}
}
