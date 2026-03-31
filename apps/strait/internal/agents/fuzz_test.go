package agents

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
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

func FuzzParseCloudflareDeploymentMetadata(f *testing.F) {
	f.Add([]byte(`{"provider":"cloudflare","namespace":"ns-prod","script_name":"agent-agent-1-v1","dispatch_worker_url":"https://dispatch.example.com","compatibility_date":"2026-03-29"}`))
	f.Add([]byte(`{"provider":"cloudflare","namespace":"ns-prod","script_name":"agent-agent-1-v1","dispatch_worker_url":"https://dispatch.example.com","compatibility_date":"2026-03-29","sandbox_policy":{"mode":"dynamic_worker","default_action":"deny","allow_hosts":["api.openai.com"],"network_class":"sandbox","policy_tag":"default"}}`))
	f.Add([]byte(`{"provider":"local_stub"}`))
	f.Add([]byte(`not-json`))

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > maxAgentConfigSize {
			t.Skip()
		}
		_, _ = ParseCloudflareDeploymentMetadata(json.RawMessage(raw))
	})
}

func FuzzValidateModelFallbacks(f *testing.F) {
	f.Add("gpt-5.4-mini,claude-haiku-4-5")
	f.Add("")
	f.Add("a,b,c,d,e,f,g")

	f.Fuzz(func(t *testing.T, raw string) {
		var fallbacks []string
		if raw != "" {
			fallbacks = strings.Split(raw, ",")
		}
		if len(fallbacks) > 20 {
			t.Skip()
		}
		_ = validateModelFallbacks(fallbacks)
	})
}

func FuzzValidateProviderSecrets(f *testing.F) {
	f.Add("openai", "sk-test")
	f.Add("", "")
	f.Add("provider", "")

	f.Fuzz(func(t *testing.T, key, value string) {
		if len(key) > 256 || len(value) > 256 {
			t.Skip()
		}
		secrets := map[string]string{key: value}
		_ = validateProviderSecrets(secrets)
	})
}

func FuzzValidateCron(f *testing.F) {
	f.Add("0 * * * *")
	f.Add("*/5 * * * *")
	f.Add("not a cron")
	f.Add("")
	f.Add("60 25 32 13 8")

	f.Fuzz(func(t *testing.T, expr string) {
		if len(expr) > 256 {
			t.Skip()
		}
		// Should never panic regardless of input.
		_ = validateCron(expr)
	})
}

func FuzzWebhookPayloadMarshal(f *testing.F) {
	f.Add("agent-1", "support-agent", "completed", "")
	f.Add("", "", "failed", "some error")
	f.Add("agent-\x00-null", "slug/with/slashes", "system_failed", "OOM killed\n\ttrace")

	f.Fuzz(func(t *testing.T, agentID, slug, status, errMsg string) {
		svc := &localService{now: time.Now}
		run := &domain.JobRun{
			ID:      "run-1",
			Status:  domain.RunStatus(status),
			Attempt: 1,
			Error:   errMsg,
		}
		agent := &domain.Agent{ID: agentID, Slug: slug}
		payload := svc.buildWebhookPayload(agent, run)
		if !json.Valid(payload) {
			t.Fatalf("buildWebhookPayload produced invalid JSON for agent_id=%q slug=%q status=%q error=%q", agentID, slug, status, errMsg)
		}
	})
}

func FuzzNormalizePayload(f *testing.F) {
	f.Add([]byte(`{"key":"value"}`))
	f.Add([]byte(`null`))
	f.Add([]byte{})
	f.Add([]byte(`[1,2,3]`))

	f.Fuzz(func(t *testing.T, raw []byte) {
		result := normalizePayload(json.RawMessage(raw))
		if len(result) == 0 {
			t.Fatal("normalizePayload returned empty result")
		}
	})
}

func FuzzBuildCloudflareMultipartUpload(f *testing.F) {
	f.Add("ns-prod", "agent-script", "2026-03-29", `export default { async fetch() { return new Response("ok"); } };`)
	f.Add("", "agent-script", "2026-03-29", `export default {}`)
	f.Add("ns-prod", "", "2026-03-29", `export default {}`)

	f.Fuzz(func(t *testing.T, namespace, scriptName, compatibilityDate, source string) {
		if len(source) > maxAgentConfigSize*4 {
			t.Skip()
		}
		_, _, _, _ = buildCloudflareMultipartUpload(CloudflareScriptUploadRequest{
			Namespace:         namespace,
			ScriptName:        scriptName,
			CompatibilityDate: compatibilityDate,
			SandboxPolicy: CloudflareSandboxPolicy{
				Mode:          CloudflareSandboxModeDynamicWorker,
				DefaultAction: CloudflareSandboxDefaultActionDeny,
				AllowHosts:    []string{"api.openai.com"},
				NetworkClass:  "sandbox",
				PolicyTag:     "default",
			},
			Source: source,
		})
	})
}
