package agents

import (
	"encoding/json"
	"testing"
)

func FuzzValidateConfig(f *testing.F) {
	f.Add([]byte(`{"temperature":0.2}`))
	f.Add([]byte(`{"temperature":`))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > maxAgentConfigSize*2 {
			t.Skip()
		}

		err := validateConfig(json.RawMessage(raw))
		switch {
		case len(raw) == 0:
			if err != nil {
				t.Fatalf("validateConfig(empty) error = %v", err)
			}
		case len(raw) > maxAgentConfigSize:
			if err == nil {
				t.Fatal("expected oversized config error")
			}
		case json.Valid(raw):
			if err != nil {
				t.Fatalf("validateConfig(valid) error = %v", err)
			}
		default:
			if err == nil {
				t.Fatal("expected invalid JSON error")
			}
		}
	})
}

func FuzzValidateRunRequestPayload(f *testing.F) {
	f.Add([]byte(`{"message":"hello"}`))
	f.Add([]byte(`{"message":`))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > maxAgentConfigSize*2 {
			t.Skip()
		}

		err := validateRunRequest(RunAgentRequest{
			ProjectID: "proj-1",
			AgentID:   "agent-1",
			Payload:   json.RawMessage(raw),
		})
		switch {
		case len(raw) == 0:
			if err != nil {
				t.Fatalf("validateRunRequest(empty) error = %v", err)
			}
		case len(raw) > maxAgentConfigSize:
			if err == nil {
				t.Fatal("expected oversized payload error")
			}
		case json.Valid(raw):
			if err != nil {
				t.Fatalf("validateRunRequest(valid) error = %v", err)
			}
		default:
			if err == nil {
				t.Fatal("expected invalid payload error")
			}
		}
	})
}
