package agents

import (
	"encoding/json"
	"testing"
)

func FuzzValidateConfig(f *testing.F) {
	f.Add([]byte(`{"temperature":0.2}`))
	f.Add([]byte(`"string-config"`))
	f.Add([]byte(`[1,2,3]`))
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
			var decoded any
			if unmarshalErr := json.Unmarshal(raw, &decoded); unmarshalErr != nil {
				t.Fatalf("json.Unmarshal(valid) error = %v", unmarshalErr)
			}
			_, isObject := decoded.(map[string]any)
			if isObject {
				if err != nil {
					t.Fatalf("validateConfig(valid object) error = %v", err)
				}
				return
			}
			if err != nil {
				return
			}
			t.Fatal("expected non-object config error")
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

func FuzzRuntimeEventDecode(f *testing.F) {
	f.Add([]byte(`{"type":"usage","provider":"local","model":"gpt-5.4"}`))
	f.Add([]byte(`{"type":"checkpoint","state":{"cursor":1}}`))
	f.Add([]byte(`{"type":"stream","chunk":"hello","done":true}`))
	f.Add([]byte(`{"type":"fail","error":"boom"}`))
	f.Add([]byte(`{"type":"complete","result":{"ok":true}}`))

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > maxAgentConfigSize*2 {
			t.Skip()
		}

		var event RuntimeEvent
		err := json.Unmarshal(raw, &event)
		if err != nil {
			return
		}

		state := &runtimeEventState{}
		_ = state.Validate(&event)
	})
}
