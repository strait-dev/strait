package dbscan

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestNilIfEmptyString(t *testing.T) {
	t.Parallel()
	t.Run("empty returns nil", func(t *testing.T) {
		t.Parallel()
		if got := NilIfEmptyString(""); got != nil {
			t.Errorf("NilIfEmptyString(%q) = %v, want nil", "", got)
		}
	})

	t.Run("non-empty returns value", func(t *testing.T) {
		t.Parallel()
		got := NilIfEmptyString("hello")
		s, ok := got.(string)
		if !ok {
			t.Fatalf("NilIfEmptyString(%q) type = %T, want string", "hello", got)
		}
		if s != "hello" {
			t.Errorf("NilIfEmptyString(%q) = %q, want %q", "hello", s, "hello")
		}
	})

	t.Run("whitespace returns value", func(t *testing.T) {
		t.Parallel()
		got := NilIfEmptyString(" ")
		if got == nil {
			t.Fatal("got nil for whitespace, want non-nil")
		}
		if got.(string) != " " {
			t.Errorf("got %q, want %q", got, " ")
		}
	})

	t.Run("long string returns value", func(t *testing.T) {
		t.Parallel()
		input := "this is a longer string with multiple words"
		got := NilIfEmptyString(input)
		if got == nil {
			t.Fatal("got nil, want non-nil")
		}
		if got.(string) != input {
			t.Errorf("got %q, want %q", got, input)
		}
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
				if got != nil {
					t.Errorf("NilIfEmptyRawMessage() = %v, want nil", got)
				}
				return
			}
			msg, ok := got.(json.RawMessage)
			if !ok {
				t.Fatalf("NilIfEmptyRawMessage() type = %T, want json.RawMessage", got)
			}
			if string(msg) != string(tt.input) {
				t.Errorf("NilIfEmptyRawMessage() = %s, want %s", msg, tt.input)
			}
		})
	}
}

func TestNilIfZeroInt(t *testing.T) {
	t.Parallel()
	t.Run("zero returns nil", func(t *testing.T) {
		t.Parallel()
		if got := NilIfZeroInt(0); got != nil {
			t.Errorf("NilIfZeroInt(0) = %v, want nil", got)
		}
	})

	t.Run("non-zero returns value", func(t *testing.T) {
		t.Parallel()
		got := NilIfZeroInt(12)
		v, ok := got.(int)
		if !ok {
			t.Fatalf("NilIfZeroInt(12) type = %T, want int", got)
		}
		if v != 12 {
			t.Errorf("NilIfZeroInt(12) = %d, want 12", v)
		}
	})
}

func TestNilIfEmptyIntSlice(t *testing.T) {
	t.Parallel()

	t.Run("nil slice returns nil", func(t *testing.T) {
		t.Parallel()
		if got := NilIfEmptyIntSlice(nil); got != nil {
			t.Errorf("NilIfEmptyIntSlice(nil) = %v, want nil", got)
		}
	})

	t.Run("empty slice returns nil", func(t *testing.T) {
		t.Parallel()
		if got := NilIfEmptyIntSlice([]int{}); got != nil {
			t.Errorf("NilIfEmptyIntSlice([]int{}) = %v, want nil", got)
		}
	})

	t.Run("non-empty slice returns value", func(t *testing.T) {
		t.Parallel()
		input := []int{1, 2, 3}
		got := NilIfEmptyIntSlice(input)
		if got == nil {
			t.Fatal("NilIfEmptyIntSlice(non-empty) should not be nil")
		}
		s, ok := got.([]int)
		if !ok {
			t.Fatalf("expected []int, got %T", got)
		}
		if len(s) != 3 || s[0] != 1 || s[1] != 2 || s[2] != 3 {
			t.Errorf("got %v, want [1 2 3]", s)
		}
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
	if err == nil {
		t.Fatal("ScanRun() expected error, got nil")
	}
	if run != nil {
		t.Errorf("ScanRun() run = %v, want nil on error", run)
	}
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
	machineID := "machine-001"
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
			&machineID,               // MachineID
		},
	}

	run, err := ScanRun(s)
	if err != nil {
		t.Fatalf("ScanRun() error = %v", err)
	}

	if run.ID != "run-001" {
		t.Errorf("ID = %q, want %q", run.ID, "run-001")
	}
	if run.JobID != "job-001" {
		t.Errorf("JobID = %q, want %q", run.JobID, "job-001")
	}
	if run.ProjectID != "proj-001" {
		t.Errorf("ProjectID = %q, want %q", run.ProjectID, "proj-001")
	}
	if run.Status != "executing" {
		t.Errorf("Status = %q, want %q", run.Status, "executing")
	}
	if run.Attempt != 2 {
		t.Errorf("Attempt = %d, want %d", run.Attempt, 2)
	}
	if string(run.Payload) != `{"input":"data"}` {
		t.Errorf("Payload = %s, want %s", run.Payload, `{"input":"data"}`)
	}
	if string(run.Result) != `{"output":"ok"}` {
		t.Errorf("Result = %s, want %s", run.Result, `{"output":"ok"}`)
	}
	if run.Error != "partial failure" {
		t.Errorf("Error = %q, want %q", run.Error, "partial failure")
	}
	if run.ErrorClass != "timeout" {
		t.Errorf("ErrorClass = %q, want %q", run.ErrorClass, "timeout")
	}
	if run.Metadata["env"] != "prod" || run.Metadata["region"] != "eu" {
		t.Errorf("Metadata = %+v, want env=prod region=eu", run.Metadata)
	}
	if run.TriggeredBy != "cron" {
		t.Errorf("TriggeredBy = %q, want %q", run.TriggeredBy, "cron")
	}
	if run.ParentRunID != "parent-001" {
		t.Errorf("ParentRunID = %q, want %q", run.ParentRunID, "parent-001")
	}
	if run.Priority != 5 {
		t.Errorf("Priority = %d, want %d", run.Priority, 5)
	}
	if run.IdempotencyKey != "idem-abc" {
		t.Errorf("IdempotencyKey = %q, want %q", run.IdempotencyKey, "idem-abc")
	}
	if run.WorkflowStepRunID != "step-run-001" {
		t.Errorf("WorkflowStepRunID = %q, want %q", run.WorkflowStepRunID, "step-run-001")
	}
	if run.ScheduledAt == nil {
		t.Error("ScheduledAt is nil, want non-nil")
	}
	if run.StartedAt == nil {
		t.Error("StartedAt is nil, want non-nil")
	}
	if run.HeartbeatAt == nil {
		t.Error("HeartbeatAt is nil, want non-nil")
	}
	if run.FinishedAt != nil {
		t.Errorf("FinishedAt = %v, want nil", run.FinishedAt)
	}
	if run.DebugMode != true {
		t.Errorf("DebugMode = %v, want true", run.DebugMode)
	}
	if run.ContinuationOf != "cont-001" {
		t.Errorf("ContinuationOf = %q, want %q", run.ContinuationOf, "cont-001")
	}
	if run.LineageDepth != 2 {
		t.Errorf("LineageDepth = %d, want %d", run.LineageDepth, 2)
	}
	if run.Tags["env"] != "prod" {
		t.Errorf("Tags = %v, want env=prod", run.Tags)
	}
	if run.JobVersionID != "jv-001" {
		t.Errorf("JobVersionID = %q, want %q", run.JobVersionID, "jv-001")
	}
	if run.CreatedBy != "user-1" {
		t.Errorf("CreatedBy = %q, want %q", run.CreatedBy, "user-1")
	}
	if run.BatchID != "batch-001" {
		t.Errorf("BatchID = %q, want %q", run.BatchID, "batch-001")
	}
	if run.ConcurrencyKey != "key-001" {
		t.Errorf("ConcurrencyKey = %q, want %q", run.ConcurrencyKey, "key-001")
	}
	if run.ExecutionMode != domain.ExecutionModeHTTP {
		t.Errorf("ExecutionMode = %q, want %q", run.ExecutionMode, domain.ExecutionModeHTTP)
	}
	if run.MachineID != "machine-001" {
		t.Errorf("MachineID = %q, want %q", run.MachineID, "machine-001")
	}
	if run.ExecutionTrace == nil {
		t.Error("ExecutionTrace is nil, want non-nil")
	}
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
			(*string)(nil),    // MachineID
		},
	}

	run, err := ScanRun(s)
	if err != nil {
		t.Fatalf("ScanRun() error = %v", err)
	}

	if run.Payload != nil {
		t.Errorf("Payload = %s, want nil", run.Payload)
	}
	if run.Result != nil {
		t.Errorf("Result = %s, want nil", run.Result)
	}
	if run.Error != "" {
		t.Errorf("Error = %q, want empty", run.Error)
	}
	if run.ErrorClass != "" {
		t.Errorf("ErrorClass = %q, want empty", run.ErrorClass)
	}
	if run.ParentRunID != "" {
		t.Errorf("ParentRunID = %q, want empty", run.ParentRunID)
	}
	if run.IdempotencyKey != "" {
		t.Errorf("IdempotencyKey = %q, want empty", run.IdempotencyKey)
	}
	if run.WorkflowStepRunID != "" {
		t.Errorf("WorkflowStepRunID = %q, want empty", run.WorkflowStepRunID)
	}
	if run.ScheduledAt != nil {
		t.Errorf("ScheduledAt = %v, want nil", run.ScheduledAt)
	}
	if run.StartedAt != nil {
		t.Errorf("StartedAt = %v, want nil", run.StartedAt)
	}
	if run.FinishedAt != nil {
		t.Errorf("FinishedAt = %v, want nil", run.FinishedAt)
	}
	if run.DebugMode != false {
		t.Errorf("DebugMode = %v, want false", run.DebugMode)
	}
	if run.ContinuationOf != "" {
		t.Errorf("ContinuationOf = %q, want empty", run.ContinuationOf)
	}
	if run.LineageDepth != 0 {
		t.Errorf("LineageDepth = %d, want 0", run.LineageDepth)
	}
	if len(run.Tags) != 0 {
		t.Errorf("Tags = %v, want empty", run.Tags)
	}
	if run.JobVersionID != "" {
		t.Errorf("JobVersionID = %q, want empty", run.JobVersionID)
	}
	if run.CreatedBy != "" {
		t.Errorf("CreatedBy = %q, want empty", run.CreatedBy)
	}
	if run.BatchID != "" {
		t.Errorf("BatchID = %q, want empty", run.BatchID)
	}
	if run.ConcurrencyKey != "" {
		t.Errorf("ConcurrencyKey = %q, want empty", run.ConcurrencyKey)
	}
	if run.ExecutionMode != "" {
		t.Errorf("ExecutionMode = %q, want empty", run.ExecutionMode)
	}
	if run.MachineID != "" {
		t.Errorf("MachineID = %q, want empty", run.MachineID)
	}
	if run.ExecutionTrace != nil {
		t.Errorf("ExecutionTrace = %v, want nil", run.ExecutionTrace)
	}
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
	if err == nil {
		t.Fatal("ScanRun() expected error for invalid metadata JSON")
	}
	if run != nil {
		t.Errorf("ScanRun() run = %v, want nil on error", run)
	}
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
	if err == nil {
		t.Fatal("ScanRun() expected error for invalid execution trace JSON")
	}
	if run != nil {
		t.Errorf("ScanRun() run = %v, want nil on error", run)
	}
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
	if err == nil {
		t.Fatal("ScanRun() expected error for invalid tags JSON")
	}
	if run != nil {
		t.Errorf("ScanRun() run = %v, want nil on error", run)
	}
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
	if err != nil {
		t.Fatalf("ScanRun() error = %v", err)
	}
	if len(run.Tags) != 0 {
		t.Errorf("Tags = %v, want empty for {}", run.Tags)
	}
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
		(*string)(nil),             // MachineID
	}
}
