package sandboxv1

import (
	"encoding/json"
	"testing"
)

func TestExecuteRequestJSON(t *testing.T) {
	t.Parallel()

	req := ExecuteRequest{
		RunID:    "run-123",
		Language: "python",
		Code:     "print('hello')",
		Payload:  []byte(`{"key": "value"}`),
		Env:      map[string]string{"FOO": "bar"},
		Limits: &ResourceLimits{
			TimeoutSecs:    30,
			MemoryBytes:    256 * 1024 * 1024,
			NetworkEnabled: false,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ExecuteRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"RunID", decoded.RunID, "run-123"},
		{"Language", decoded.Language, "python"},
		{"TimeoutSecs", decoded.Limits.TimeoutSecs, int32(30)},
		{"MemoryBytes", decoded.Limits.MemoryBytes, int64(256 * 1024 * 1024)},
		{"NetworkEnabled", decoded.Limits.NetworkEnabled, false},
		{"Env[FOO]", decoded.Env["FOO"], "bar"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestExecutionEventJSON_Roundtrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event ExecutionEvent
	}{
		{
			name: "log event",
			event: ExecutionEvent{
				Log: &LogEntry{Level: "info", Message: "hello world", TimestampMs: 1234567890},
			},
		},
		{
			name: "checkpoint event",
			event: ExecutionEvent{
				Checkpoint: &Checkpoint{Sequence: 1, State: []byte(`{"step": 1}`)},
			},
		},
		{
			name: "tool call event",
			event: ExecutionEvent{
				ToolCall: &ToolCall{
					ToolName:   "web_search",
					Input:      []byte(`{"query": "test"}`),
					Output:     []byte(`{"results": []}`),
					DurationMs: 150,
					Status:     "success",
				},
			},
		},
		{
			name: "result success",
			event: ExecutionEvent{
				Result: &ExecutionResult{
					Success:    true,
					Result:     []byte(`{"output": "done"}`),
					DurationMs: 1500,
				},
			},
		},
		{
			name: "result failure",
			event: ExecutionEvent{
				Result: &ExecutionResult{
					Success:    false,
					Error:      "runtime error: division by zero",
					DurationMs: 50,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tt.event)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var decoded ExecutionEvent
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			data2, _ := json.Marshal(decoded)
			if string(data) != string(data2) {
				t.Errorf("roundtrip mismatch:\n  got:  %s\n  want: %s", data2, data)
			}
		})
	}
}

func TestResourceLimitsDefaults(t *testing.T) {
	t.Parallel()

	var limits ResourceLimits
	if limits.TimeoutSecs != 0 {
		t.Errorf("expected 0 default timeout, got %d", limits.TimeoutSecs)
	}
	if limits.MemoryBytes != 0 {
		t.Errorf("expected 0 default memory, got %d", limits.MemoryBytes)
	}
	if limits.NetworkEnabled {
		t.Error("expected network disabled by default")
	}
}
