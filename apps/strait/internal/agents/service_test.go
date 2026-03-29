package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"strait/internal/domain"
)

func TestValidateCreateRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     CreateAgentRequest
		wantErr bool
	}{
		{
			name: "valid",
			req: CreateAgentRequest{
				ProjectID: "proj-1",
				Name:      "Support Agent",
				Slug:      "support-agent",
				Model:     "gpt-5.4",
				Config:    json.RawMessage(`{"temperature":0.2}`),
			},
		},
		{
			name: "missing name",
			req: CreateAgentRequest{
				ProjectID: "proj-1",
				Slug:      "support-agent",
				Model:     "gpt-5.4",
			},
			wantErr: true,
		},
		{
			name: "invalid config",
			req: CreateAgentRequest{
				ProjectID: "proj-1",
				Name:      "Support Agent",
				Slug:      "support-agent",
				Model:     "gpt-5.4",
				Config:    json.RawMessage(`{"temperature":`),
			},
			wantErr: true,
		},
		{
			name: "config must be object",
			req: CreateAgentRequest{
				ProjectID: "proj-1",
				Name:      "Support Agent",
				Slug:      "support-agent",
				Model:     "gpt-5.4",
				Config:    json.RawMessage(`"not-an-object"`),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateCreateRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateCreateRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLocalStubProviderRun(t *testing.T) {
	t.Parallel()

	provider := LocalStubProvider{}
	run, err := provider.Run(
		context.Background(),
		&domain.Agent{ID: "agent-1", Slug: "support-agent"},
		&domain.AgentDeployment{ID: "dep-1", Version: 2},
		&domain.JobRun{Payload: json.RawMessage(`{"question":"hello"}`)},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(run, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got["agent_id"] != "agent-1" {
		t.Fatalf("agent_id = %v, want agent-1", got["agent_id"])
	}
	if got["provider"] != ProviderNameLocalStub {
		t.Fatalf("provider = %v, want %s", got["provider"], ProviderNameLocalStub)
	}
}

func TestLocalStubProviderRunReturnsStubError(t *testing.T) {
	t.Parallel()

	_, err := (LocalStubProvider{}).Run(
		context.Background(),
		&domain.Agent{ID: "agent-1", Slug: "support-agent"},
		&domain.AgentDeployment{ID: "dep-1", Version: 1},
		&domain.JobRun{Payload: json.RawMessage(`{"_stub_error":"boom"}`)},
	)
	if err == nil {
		t.Fatal("expected stub error")
	}
	if !strings.Contains(err.Error(), "stub provider error") {
		t.Fatalf("expected stub provider error, got %v", err)
	}
}

func TestNormalizedConfigDefaultsToObject(t *testing.T) {
	t.Parallel()

	got := normalizedConfig(nil)
	if string(got) != "{}" {
		t.Fatalf("normalizedConfig(nil) = %s, want {}", got)
	}
}

func TestAdvisoryLockIDIsDeterministic(t *testing.T) {
	t.Parallel()

	a := advisoryLockID("agent-123")
	b := advisoryLockID("agent-123")
	c := advisoryLockID("agent-456")

	if a != b {
		t.Fatalf("advisoryLockID not deterministic: %d != %d", a, b)
	}
	if a == c {
		t.Fatalf("expected different lock IDs for different values, got %d", a)
	}
}

func TestSelectProviderDefaultsToLocal(t *testing.T) {
	t.Parallel()

	provider := SelectProvider(CloudflareConfig{})
	if provider.Name() != ProviderNameLocalStub {
		t.Fatalf("provider.Name() = %q, want %q", provider.Name(), ProviderNameLocalStub)
	}
}

func TestSelectProviderReturnsCloudflare(t *testing.T) {
	t.Parallel()

	provider := SelectProvider(CloudflareConfig{
		AccountID:         "acct-1",
		APIToken:          "token-1",
		DispatchNamespace: "ns-prod",
		DispatchWorkerURL: "https://dispatch.example.com",
		CompatibilityDate: "2026-03-29",
		SandboxMode:       CloudflareSandboxModeDisabled,
	})
	if provider.Name() != ProviderNameCloudflare {
		t.Fatalf("provider.Name() = %q, want %q", provider.Name(), ProviderNameCloudflare)
	}
}

func TestCloudflareConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     CloudflareConfig
		wantErr bool
	}{
		{
			name: "disabled config",
			cfg:  CloudflareConfig{},
		},
		{
			name: "valid outbound worker",
			cfg: CloudflareConfig{
				AccountID:          "acct-1",
				APIToken:           "token-1",
				DispatchNamespace:  "ns-prod",
				DispatchWorkerURL:  "https://dispatch.example.com",
				OutboundWorkerName: "agents-outbound",
				CompatibilityDate:  "2026-03-29",
				SandboxMode:        CloudflareSandboxModeOutboundWorker,
			},
		},
		{
			name: "missing required when enabled",
			cfg: CloudflareConfig{
				AccountID: "acct-1",
			},
			wantErr: true,
		},
		{
			name: "invalid sandbox mode",
			cfg: CloudflareConfig{
				AccountID:         "acct-1",
				APIToken:          "token-1",
				DispatchNamespace: "ns-prod",
				DispatchWorkerURL: "https://dispatch.example.com",
				CompatibilityDate: "2026-03-29",
				SandboxMode:       "loader",
			},
			wantErr: true,
		},
		{
			name: "outbound worker requires name",
			cfg: CloudflareConfig{
				AccountID:         "acct-1",
				APIToken:          "token-1",
				DispatchNamespace: "ns-prod",
				DispatchWorkerURL: "https://dispatch.example.com",
				CompatibilityDate: "2026-03-29",
				SandboxMode:       CloudflareSandboxModeOutboundWorker,
			},
			wantErr: true,
		},
		{
			name: "invalid dispatch worker url",
			cfg: CloudflareConfig{
				AccountID:         "acct-1",
				APIToken:          "token-1",
				DispatchNamespace: "ns-prod",
				DispatchWorkerURL: "://bad-url",
				CompatibilityDate: "2026-03-29",
				SandboxMode:       CloudflareSandboxModeDisabled,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCloudflareDeploymentMetadataRoundTrip(t *testing.T) {
	t.Parallel()

	raw := MarshalCloudflareDeploymentMetadata(CloudflareDeploymentMetadata{
		Provider:          ProviderNameCloudflare,
		Namespace:         "ns-prod",
		ScriptName:        "agent-agent-1-v2",
		DeploymentVersion: 2,
		DispatchWorkerURL: "https://dispatch.example.com",
		OutboundWorker:    "agents-outbound",
		CompatibilityDate: "2026-03-29",
		ContentSHA256:     "hash",
		Etag:              "etag-1",
		SandboxPolicy: CloudflareSandboxPolicy{
			Mode:               CloudflareSandboxModeOutboundWorker,
			OutboundWorkerName: "agents-outbound",
			NetworkClass:       "restricted",
		},
	})

	metadata, err := ParseCloudflareDeploymentMetadata(raw)
	if err != nil {
		t.Fatalf("ParseCloudflareDeploymentMetadata() error = %v", err)
	}
	if metadata.Provider != ProviderNameCloudflare {
		t.Fatalf("metadata.Provider = %q, want %q", metadata.Provider, ProviderNameCloudflare)
	}
	if metadata.ScriptName != "agent-agent-1-v2" {
		t.Fatalf("metadata.ScriptName = %q, want agent-agent-1-v2", metadata.ScriptName)
	}
}

func TestParseCloudflareDeploymentMetadataRejectsCorruptInput(t *testing.T) {
	t.Parallel()

	tests := []json.RawMessage{
		nil,
		json.RawMessage(`{"provider":"local_stub"}`),
		json.RawMessage(`{"provider":"cloudflare","namespace":"","script_name":"worker","dispatch_worker_url":"https://dispatch.example.com","compatibility_date":"2026-03-29"}`),
		json.RawMessage(`{"provider":"cloudflare","namespace":"ns-prod","script_name":"","dispatch_worker_url":"https://dispatch.example.com","compatibility_date":"2026-03-29"}`),
		json.RawMessage(`{"provider":"cloudflare","namespace":"ns-prod","script_name":"worker","compatibility_date":"2026-03-29"}`),
		json.RawMessage(`{"provider":"cloudflare","namespace":"ns-prod","script_name":"worker","dispatch_worker_url":"https://dispatch.example.com"}`),
		json.RawMessage(`not-json`),
	}

	for _, raw := range tests {
		t.Run(string(raw), func(t *testing.T) {
			t.Parallel()
			if _, err := ParseCloudflareDeploymentMetadata(raw); err == nil {
				t.Fatalf("expected parse error for %s", raw)
			}
		})
	}
}
