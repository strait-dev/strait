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
	if got["provider"] != localProviderName {
		t.Fatalf("provider = %v, want %s", got["provider"], localProviderName)
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
