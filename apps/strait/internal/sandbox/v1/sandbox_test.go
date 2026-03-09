package sandboxv1

import (
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestExecuteRequest_Roundtrip(t *testing.T) {
	t.Parallel()

	req := &ExecuteRequest{
		RunId:    "run-123",
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

	data, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ExecuteRequest
	if err := proto.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"RunId", decoded.GetRunId(), "run-123"},
		{"Language", decoded.GetLanguage(), "python"},
		{"TimeoutSecs", decoded.GetLimits().GetTimeoutSecs(), int32(30)},
		{"MemoryBytes", decoded.GetLimits().GetMemoryBytes(), int64(256 * 1024 * 1024)},
		{"NetworkEnabled", decoded.GetLimits().GetNetworkEnabled(), false},
		{"Env[FOO]", decoded.GetEnv()["FOO"], "bar"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestExecutionEvent_OneofVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		event   *ExecutionEvent
		hasLog  bool
		hasCp   bool
		hasTool bool
		hasRes  bool
	}{
		{
			name: "log event",
			event: &ExecutionEvent{
				Event: &ExecutionEvent_Log{
					Log: &LogEntry{Level: "info", Message: "hello world", TimestampMs: 1234567890},
				},
			},
			hasLog: true,
		},
		{
			name: "checkpoint event",
			event: &ExecutionEvent{
				Event: &ExecutionEvent_Checkpoint{
					Checkpoint: &Checkpoint{Sequence: 1, State: []byte(`{"step": 1}`)},
				},
			},
			hasCp: true,
		},
		{
			name: "tool call event",
			event: &ExecutionEvent{
				Event: &ExecutionEvent_ToolCall{
					ToolCall: &ToolCall{
						ToolName:   "web_search",
						Input:      []byte(`{"query": "test"}`),
						Output:     []byte(`{"results": []}`),
						DurationMs: 150,
						Status:     "success",
					},
				},
			},
			hasTool: true,
		},
		{
			name: "result success",
			event: &ExecutionEvent{
				Event: &ExecutionEvent_Result{
					Result: &ExecutionResult{
						Success:    true,
						Result:     []byte(`{"output": "done"}`),
						DurationMs: 1500,
					},
				},
			},
			hasRes: true,
		},
		{
			name: "result failure",
			event: &ExecutionEvent{
				Event: &ExecutionEvent_Result{
					Result: &ExecutionResult{
						Success:    false,
						Error:      "runtime error: division by zero",
						DurationMs: 50,
					},
				},
			},
			hasRes: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Roundtrip through protobuf
			data, err := proto.Marshal(tt.event)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var decoded ExecutionEvent
			if err := proto.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if (decoded.GetLog() != nil) != tt.hasLog {
				t.Errorf("hasLog: got %v, want %v", decoded.GetLog() != nil, tt.hasLog)
			}
			if (decoded.GetCheckpoint() != nil) != tt.hasCp {
				t.Errorf("hasCheckpoint: got %v, want %v", decoded.GetCheckpoint() != nil, tt.hasCp)
			}
			if (decoded.GetToolCall() != nil) != tt.hasTool {
				t.Errorf("hasToolCall: got %v, want %v", decoded.GetToolCall() != nil, tt.hasTool)
			}
			if (decoded.GetResult() != nil) != tt.hasRes {
				t.Errorf("hasResult: got %v, want %v", decoded.GetResult() != nil, tt.hasRes)
			}
		})
	}
}

func TestResourceLimitsDefaults(t *testing.T) {
	t.Parallel()

	limits := &ResourceLimits{}
	if limits.GetTimeoutSecs() != 0 {
		t.Errorf("expected 0 default timeout, got %d", limits.GetTimeoutSecs())
	}
	if limits.GetMemoryBytes() != 0 {
		t.Errorf("expected 0 default memory, got %d", limits.GetMemoryBytes())
	}
	if limits.GetNetworkEnabled() {
		t.Error("expected network disabled by default")
	}
}
