package sandboxv1

import (
	"encoding/json"
	"testing"
)

func TestExecuteRequestJSON(t *testing.T) {
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
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded ExecuteRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.RunID != "run-123" {
		t.Errorf("expected run-123, got %s", decoded.RunID)
	}
	if decoded.Language != "python" {
		t.Errorf("expected python, got %s", decoded.Language)
	}
	if decoded.Limits.TimeoutSecs != 30 {
		t.Errorf("expected 30s timeout, got %d", decoded.Limits.TimeoutSecs)
	}
	if decoded.Limits.MemoryBytes != 256*1024*1024 {
		t.Errorf("expected 256MB, got %d", decoded.Limits.MemoryBytes)
	}
	if decoded.Env["FOO"] != "bar" {
		t.Errorf("expected FOO=bar, got %s", decoded.Env["FOO"])
	}
}

func TestExecutionEventJSON(t *testing.T) {
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
			data, err := json.Marshal(tt.event)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			var decoded ExecutionEvent
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}

			// Re-marshal and compare
			data2, _ := json.Marshal(decoded)
			if string(data) != string(data2) {
				t.Errorf("roundtrip mismatch:\n  got:  %s\n  want: %s", data2, data)
			}
		})
	}
}

func TestResourceLimitsDefaults(t *testing.T) {
	var limits ResourceLimits
	if limits.TimeoutSecs != 0 {
		t.Errorf("expected 0 default timeout")
	}
	if limits.MemoryBytes != 0 {
		t.Errorf("expected 0 default memory")
	}
	if limits.NetworkEnabled {
		t.Errorf("expected network disabled by default")
	}
}
