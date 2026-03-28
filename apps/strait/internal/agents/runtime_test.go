package agents

import (
	"encoding/json"
	"testing"
)

func TestRuntimeEventStateRejectsEventsAfterTerminal(t *testing.T) {
	t.Parallel()

	state := &runtimeEventState{}
	if err := state.Validate(&RuntimeEvent{Type: RuntimeEventComplete}); err != nil {
		t.Fatalf("Validate(complete) error = %v", err)
	}
	if err := state.Validate(&RuntimeEvent{Type: RuntimeEventUsage, Provider: "local", Model: "agent"}); err == nil {
		t.Fatal("expected event-after-terminal error")
	}
}

func TestRuntimeEventStateRejectsStreamAfterDone(t *testing.T) {
	t.Parallel()

	state := &runtimeEventState{}
	if err := state.Validate(&RuntimeEvent{Type: RuntimeEventStream, StreamID: "default", Chunk: "hello", Done: true}); err != nil {
		t.Fatalf("Validate(done stream) error = %v", err)
	}
	if err := state.Validate(&RuntimeEvent{Type: RuntimeEventStream, StreamID: "default", Chunk: "late"}); err == nil {
		t.Fatal("expected stream-after-done error")
	}
}

func TestRuntimeEventStateNormalizesDefaults(t *testing.T) {
	t.Parallel()

	state := &runtimeEventState{}
	event := RuntimeEvent{
		Type:     RuntimeEventToolCall,
		ToolName: "local.echo",
	}
	if err := state.Validate(&event); err != nil {
		t.Fatalf("Validate(tool_call) error = %v", err)
	}
	if event.Status != "completed" {
		t.Fatalf("tool status = %q, want completed", event.Status)
	}

	stream := RuntimeEvent{
		Type:  RuntimeEventStream,
		Chunk: "chunk",
	}
	if err := state.Validate(&stream); err != nil {
		t.Fatalf("Validate(stream) error = %v", err)
	}
	if stream.StreamID != "default" {
		t.Fatalf("stream id = %q, want default", stream.StreamID)
	}
}

func TestRuntimeDispatchEnvelopeJSONRoundTrip(t *testing.T) {
	t.Parallel()

	envelope := RuntimeDispatchEnvelope{
		Version: runtimeContractVersion,
		Run: RuntimeDispatchRun{
			ID:          "run-1",
			ProjectID:   "proj-1",
			Attempt:     1,
			TimeoutSecs: 30,
		},
		Agent: RuntimeDispatchAgent{
			ID:     "agent-1",
			Slug:   "support-agent",
			Model:  "gpt-5.4",
			Config: json.RawMessage(`{"temperature":0.2}`),
		},
		Deployment: RuntimeDispatchDeployment{
			ID:             "dep-1",
			Version:        2,
			Provider:       localProviderName,
			ConfigSnapshot: json.RawMessage(`{"temperature":0.2}`),
		},
		Payload: json.RawMessage(`{"prompt":"hello"}`),
		Callback: RuntimeDispatchCallback{
			BaseURL:  "http://127.0.0.1:8080",
			RunID:    "run-1",
			RunToken: "token",
		},
	}

	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var roundTrip RuntimeDispatchEnvelope
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if roundTrip.Run.ID != envelope.Run.ID {
		t.Fatalf("run id = %q, want %q", roundTrip.Run.ID, envelope.Run.ID)
	}
	if roundTrip.Callback.BaseURL != envelope.Callback.BaseURL {
		t.Fatalf("callback base url = %q, want %q", roundTrip.Callback.BaseURL, envelope.Callback.BaseURL)
	}
}
