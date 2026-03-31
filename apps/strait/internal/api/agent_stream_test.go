package api

import (
	"testing"
)

func TestExtractEventType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  string
		want string
	}{
		{"usage event", `{"type":"usage","provider":"openai","model":"gpt-5.4"}`, "usage"},
		{"tool_call event", `{"type":"tool_call","tool_name":"search"}`, "tool_call"},
		{"checkpoint event", `{"type":"checkpoint","sequence":1}`, "checkpoint"},
		{"stream event", `{"type":"stream_chunk","chunk":"hello"}`, "stream_chunk"},
		{"complete event", `{"type":"complete","result":{}}`, "complete"},
		{"fail event", `{"type":"fail","error":"boom"}`, "fail"},
		{"no type field", `{"data":"hello"}`, ""},
		{"invalid json", `not json`, ""},
		{"empty string", ``, ""},
		{"empty type", `{"type":""}`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractEventType(tt.msg)
			if got != tt.want {
				t.Fatalf("extractEventType(%q) = %q, want %q", tt.msg, got, tt.want)
			}
		})
	}
}

func TestPublishRunEventNilPubsub(t *testing.T) {
	t.Parallel()

	// Server with nil pubsub should not panic.
	srv := &Server{}
	srv.publishRunEvent(t.Context(), "run-1", map[string]any{
		"type": "usage", "model": "gpt-5.4",
	})
}
