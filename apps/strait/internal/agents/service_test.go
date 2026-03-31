package agents

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/golang-jwt/jwt/v5"
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

func TestValidateCron(t *testing.T) {
	t.Parallel()

	valid := []string{"", "0 * * * *", "*/5 * * * *", "0 9 * * 1-5", "30 2 1 * *"}
	for _, expr := range valid {
		if err := validateCron(expr); err != nil {
			t.Fatalf("validateCron(%q) unexpected error: %v", expr, err)
		}
	}

	invalid := []string{"not a cron", "60 * * * *", "* * *", "0 25 * * *"}
	for _, expr := range invalid {
		if err := validateCron(expr); err == nil {
			t.Fatalf("validateCron(%q) expected error", expr)
		}
	}
}

func TestValidateCronTimezone(t *testing.T) {
	t.Parallel()

	valid := []string{"", "UTC", "America/New_York", "Europe/London", "Asia/Tokyo"}
	for _, tz := range valid {
		if err := validateCronTimezone(tz); err != nil {
			t.Fatalf("validateCronTimezone(%q) unexpected error: %v", tz, err)
		}
	}

	invalid := []string{"Not/A/Zone", "InvalidTZ", "Foo"}
	for _, tz := range invalid {
		if err := validateCronTimezone(tz); err == nil {
			t.Fatalf("validateCronTimezone(%q) expected error", tz)
		}
	}
}

func TestBuildBackingJobWithCron(t *testing.T) {
	t.Parallel()

	req := CreateAgentRequest{
		ProjectID:    "proj-1",
		Name:         "Scheduled Agent",
		Slug:         "scheduled-agent",
		Model:        "gpt-5.4",
		Config:       json.RawMessage(`{}`),
		Cron:         "0 * * * *",
		CronTimezone: "America/New_York",
		Actor:        "user-1",
	}
	job := buildBackingJob(req)
	if job.Cron != "0 * * * *" {
		t.Fatalf("job.Cron = %q, want '0 * * * *'", job.Cron)
	}
	if job.Timezone != "America/New_York" {
		t.Fatalf("job.Timezone = %q, want 'America/New_York'", job.Timezone)
	}
	if !job.Enabled {
		t.Fatal("job.Enabled should be true when cron is set")
	}
}

func TestBuildBackingJobWithoutCron(t *testing.T) {
	t.Parallel()

	req := CreateAgentRequest{
		ProjectID: "proj-1",
		Name:      "Manual Agent",
		Slug:      "manual-agent",
		Model:     "gpt-5.4",
		Config:    json.RawMessage(`{}`),
		Actor:     "user-1",
	}
	job := buildBackingJob(req)
	if job.Cron != "" {
		t.Fatalf("job.Cron = %q, want empty", job.Cron)
	}
	if job.Enabled {
		t.Fatal("job.Enabled should be false when no cron")
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
		APIToken:          testCFToken,
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
			name: "valid dynamic worker",
			cfg: CloudflareConfig{
				AccountID:         "acct-1",
				APIToken:          testCFToken,
				DispatchNamespace: "ns-prod",
				DispatchWorkerURL: "https://dispatch.example.com",
				CompatibilityDate: "2026-03-29",
				SandboxMode:       CloudflareSandboxModeDynamicWorker,
			},
		},
		{
			name: "valid outbound worker",
			cfg: CloudflareConfig{
				AccountID:          "acct-1",
				APIToken:           testCFToken,
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
				APIToken:          testCFToken,
				DispatchNamespace: "ns-prod",
				DispatchWorkerURL: "https://dispatch.example.com",
				CompatibilityDate: "2026-03-29",
				SandboxMode:       "broken_mode",
			},
			wantErr: true,
		},
		{
			name: "outbound worker requires name",
			cfg: CloudflareConfig{
				AccountID:         "acct-1",
				APIToken:          testCFToken,
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
				APIToken:          testCFToken,
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
			Mode:          CloudflareSandboxModeDynamicWorker,
			DefaultAction: CloudflareSandboxDefaultActionDeny,
			NetworkClass:  "sandbox",
			PolicyTag:     "default",
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

func TestResolveCloudflareSandboxPolicyFromConfigSnapshot(t *testing.T) {
	t.Parallel()

	policy := resolveCloudflareSandboxPolicy(CloudflareConfig{
		SandboxMode: CloudflareSandboxModeDynamicWorker,
	}, json.RawMessage(`{
		"sandbox": {
			"policy": {
				"allow_hosts": ["api.openai.com", "api.openai.com", " example.com "],
				"default_action": "allow",
				"network_class": "public",
				"policy_tag": "external-llm"
			}
		}
	}`))

	if policy.DefaultAction != CloudflareSandboxDefaultActionAllow {
		t.Fatalf("policy.DefaultAction = %q, want %q", policy.DefaultAction, CloudflareSandboxDefaultActionAllow)
	}
	if policy.NetworkClass != "public" {
		t.Fatalf("policy.NetworkClass = %q, want public", policy.NetworkClass)
	}
	if policy.PolicyTag != "external-llm" {
		t.Fatalf("policy.PolicyTag = %q, want external-llm", policy.PolicyTag)
	}
	if strings.Join(policy.AllowHosts, ",") != "api.openai.com,example.com" {
		t.Fatalf("policy.AllowHosts = %v", policy.AllowHosts)
	}
}

func TestClassifyRuntimeError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		errMsg         string
		wantClass      string
		wantSuggestion bool
	}{
		{name: "cloudflare 1101 status", errMsg: "HTTP 1101: worker failed", wantClass: "oom", wantSuggestion: true},
		{name: "exceeded resource limits", errMsg: "Worker exceeded resource limits", wantClass: "oom", wantSuggestion: true},
		{name: "exceeded cpu", errMsg: "Exceeded CPU time limit", wantClass: "oom", wantSuggestion: true},
		{name: "out of memory", errMsg: "process out of memory", wantClass: "oom", wantSuggestion: true},
		{name: "oom keyword", errMsg: "worker OOM killed", wantClass: "oom", wantSuggestion: true},
		{name: "timeout", errMsg: "execution timeout after 30s", wantClass: "timeout", wantSuggestion: true},
		{name: "timed out", errMsg: "request timed out", wantClass: "timeout", wantSuggestion: true},
		{name: "deadline exceeded", errMsg: "context deadline exceeded", wantClass: "timeout", wantSuggestion: true},
		{name: "rate limit", errMsg: "rate limit exceeded", wantClass: "rate_limited", wantSuggestion: true},
		{name: "http 429", errMsg: "HTTP 429 Too Many Requests", wantClass: "rate_limited", wantSuggestion: true},
		{name: "generic error", errMsg: "unexpected internal error", wantClass: "runtime_error", wantSuggestion: false},
		{name: "empty error", errMsg: "", wantClass: "runtime_error", wantSuggestion: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			class, suggestion := classifyRuntimeError(tt.errMsg)
			if class != tt.wantClass {
				t.Fatalf("classifyRuntimeError(%q) class = %q, want %q", tt.errMsg, class, tt.wantClass)
			}
			if tt.wantSuggestion && suggestion == "" {
				t.Fatalf("classifyRuntimeError(%q) expected non-empty suggestion", tt.errMsg)
			}
			if !tt.wantSuggestion && suggestion != "" {
				t.Fatalf("classifyRuntimeError(%q) expected empty suggestion, got %q", tt.errMsg, suggestion)
			}
		})
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

func TestGenerateRunTokenIncludesAgentID(t *testing.T) {
	t.Parallel()

	svc := &localService{
		jwtSigningKey: testCFToken, // reuse the random test token as a signing key
		now:           time.Now,
	}

	token, err := svc.generateRunToken("run-123", "agent-abc", 300, nil)
	if err != nil {
		t.Fatalf("generateRunToken() error = %v", err)
	}

	// Parse the token back and verify the agent_id claim.
	claims := &agentRunClaims{}
	parsed, parseErr := jwt.ParseWithClaims(token, claims, func(_ *jwt.Token) (any, error) {
		return []byte(svc.jwtSigningKey), nil
	})
	if parseErr != nil || !parsed.Valid {
		t.Fatalf("ParseWithClaims() error = %v, valid = %v", parseErr, parsed.Valid)
	}
	if claims.Subject != "run-123" {
		t.Fatalf("Subject = %q, want run-123", claims.Subject)
	}
	if claims.AgentID != "agent-abc" {
		t.Fatalf("AgentID = %q, want agent-abc", claims.AgentID)
	}
}

func TestGenerateRunTokenBackwardCompatParsing(t *testing.T) {
	t.Parallel()

	svc := &localService{
		jwtSigningKey: testCFToken,
		now:           time.Now,
	}

	// Generate a token without agent ID (empty string).
	token, err := svc.generateRunToken("run-456", "", 300, nil)
	if err != nil {
		t.Fatalf("generateRunToken() error = %v", err)
	}

	// Parsing with agentRunClaims should still work -- AgentID is empty.
	claims := &agentRunClaims{}
	parsed, parseErr := jwt.ParseWithClaims(token, claims, func(_ *jwt.Token) (any, error) {
		return []byte(svc.jwtSigningKey), nil
	})
	if parseErr != nil || !parsed.Valid {
		t.Fatalf("ParseWithClaims() error = %v, valid = %v", parseErr, parsed.Valid)
	}
	if claims.Subject != "run-456" {
		t.Fatalf("Subject = %q, want run-456", claims.Subject)
	}
	if claims.AgentID != "" {
		t.Fatalf("AgentID = %q, want empty", claims.AgentID)
	}

	// Also verify that old-style RegisteredClaims parsing still works.
	oldClaims := &jwt.RegisteredClaims{}
	parsed2, parseErr2 := jwt.ParseWithClaims(token, oldClaims, func(_ *jwt.Token) (any, error) {
		return []byte(svc.jwtSigningKey), nil
	})
	if parseErr2 != nil || !parsed2.Valid {
		t.Fatalf("old-style parse error = %v, valid = %v", parseErr2, parsed2.Valid)
	}
	if oldClaims.Subject != "run-456" {
		t.Fatalf("old-style Subject = %q, want run-456", oldClaims.Subject)
	}
}

func TestFilterAllowedPatchKeys(t *testing.T) {
	t.Parallel()

	patch := map[string]any{
		"model":          "gpt-5.4-mini",
		"budget":         "$5.00",
		"prompt_caching": true,
		"webhook_url":    "https://evil.com",                    // should be dropped
		"sandbox":        map[string]any{"policy": "allow_all"}, // should be dropped
	}

	safe := FilterAllowedPatchKeys(patch)

	if len(safe) != 3 {
		t.Fatalf("expected 3 keys, got %d: %v", len(safe), safe)
	}
	if safe["model"] != "gpt-5.4-mini" {
		t.Fatalf("model = %v", safe["model"])
	}
	if safe["budget"] != "$5.00" {
		t.Fatalf("budget = %v", safe["budget"])
	}
	if safe["prompt_caching"] != true {
		t.Fatalf("prompt_caching = %v", safe["prompt_caching"])
	}
	if _, exists := safe["webhook_url"]; exists {
		t.Fatal("webhook_url should have been filtered out")
	}
	if _, exists := safe["sandbox"]; exists {
		t.Fatal("sandbox should have been filtered out")
	}
}

func TestFilterAllowedPatchKeysEmptyPatch(t *testing.T) {
	t.Parallel()

	safe := FilterAllowedPatchKeys(map[string]any{})
	if len(safe) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(safe))
	}
}

func TestFilterAllowedPatchKeysAllBlocked(t *testing.T) {
	t.Parallel()

	patch := map[string]any{
		"webhook_url":  "https://evil.com",
		"max_attempts": 999,
	}
	safe := FilterAllowedPatchKeys(patch)
	if len(safe) != 0 {
		t.Fatalf("expected 0 keys after filtering, got %d: %v", len(safe), safe)
	}
}

func TestValidateModelFallbacks(t *testing.T) {
	t.Parallel()

	if err := validateModelFallbacks(nil); err != nil {
		t.Fatalf("nil fallbacks: %v", err)
	}
	if err := validateModelFallbacks([]string{"gpt-5.4-mini", "claude-haiku-4-5"}); err != nil {
		t.Fatalf("valid fallbacks: %v", err)
	}
	if err := validateModelFallbacks([]string{"a", "b", "c", "d", "e", "f"}); err == nil {
		t.Fatal("expected error for too many fallbacks")
	}
	if err := validateModelFallbacks([]string{"gpt-5.4", ""}); err == nil {
		t.Fatal("expected error for empty model in fallbacks")
	}
}

func TestValidateProviderSecrets(t *testing.T) {
	t.Parallel()

	if err := validateProviderSecrets(nil); err != nil {
		t.Fatalf("nil secrets: %v", err)
	}
	if err := validateProviderSecrets(map[string]string{"openai": "sk-test", "anthropic": "sk-ant-test"}); err != nil {
		t.Fatalf("valid secrets: %v", err)
	}
	if err := validateProviderSecrets(map[string]string{"openai": ""}); err == nil {
		t.Fatal("expected error for empty secret value")
	}
	if err := validateProviderSecrets(map[string]string{"": "sk-test"}); err == nil {
		t.Fatal("expected error for empty provider name")
	}
	// Too many providers.
	many := make(map[string]string)
	for i := range 11 {
		many[strings.Repeat("p", i+1)] = "sk-test"
	}
	if err := validateProviderSecrets(many); err == nil {
		t.Fatal("expected error for too many providers")
	}
}

func TestProviderSecretsNeverInAPIResponse(t *testing.T) {
	t.Parallel()

	agent := domain.Agent{
		ID:                       "agent-1",
		Model:                    "gpt-5.4",
		ProviderSecretsEncrypted: "encrypted-blob-here",
	}
	raw, err := json.Marshal(agent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "encrypted-blob-here") {
		t.Fatal("provider_secrets_encrypted should not appear in JSON output")
	}
	if strings.Contains(string(raw), "provider_secrets") {
		t.Fatal("no provider_secrets key should appear in JSON output")
	}
}

func TestProviderSecretsNotInWebhookPayload(t *testing.T) {
	t.Parallel()

	svc := &localService{now: time.Now}
	agent := &domain.Agent{
		ID:                       "agent-1",
		Slug:                     "test",
		ProviderSecretsEncrypted: "secret-cipher",
	}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	payload := svc.buildWebhookPayload(agent, run)
	if strings.Contains(string(payload), "secret") {
		t.Fatal("webhook payload should not contain provider secrets")
	}
}

func TestBuildBackingJobWithModelFallbacks(t *testing.T) {
	t.Parallel()

	req := CreateAgentRequest{
		ProjectID:      "proj-1",
		Name:           "Fallback Agent",
		Slug:           "fallback-agent",
		Model:          "claude-sonnet-4-6",
		ModelFallbacks: []string{"gpt-5.4-mini"},
		Config:         json.RawMessage(`{}`),
		Actor:          "user-1",
	}
	// buildBackingJob doesn't use ModelFallbacks (they're on the agent, not the job).
	// This test verifies no panic.
	job := buildBackingJob(req)
	if job == nil {
		t.Fatal("expected non-nil job")
	}
}

// Webhook delivery tests.

type mockWebhookStore struct {
	mu      sync.Mutex
	created *domain.WebhookDelivery
	count   int
	err     error
}

func (m *mockWebhookStore) CreateWebhookDelivery(_ context.Context, d *domain.WebhookDelivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.created = d
	m.count++
	return m.err
}

type mockAgentStoreForWebhook struct {
	agentStore
	run *domain.JobRun
}

func (m *mockAgentStoreForWebhook) GetRun(_ context.Context, _ string) (*domain.JobRun, error) {
	return m.run, nil
}

func TestFireAgentWebhookCreatesDurableDelivery(t *testing.T) {
	t.Parallel()

	ws := &mockWebhookStore{}
	svc := &localService{
		store: &mockAgentStoreForWebhook{
			run: &domain.JobRun{ID: "run-1", JobID: "job-1", Status: domain.StatusCompleted},
		},
		webhookStore: ws,
		now:          time.Now,
	}

	agent := &domain.Agent{
		ID:     "agent-1",
		Slug:   "test-agent",
		Config: json.RawMessage(`{"webhook_url":"https://www.google.com/webhook-test"}`),
	}
	svc.fireAgentWebhook(context.Background(), agent, "run-1")

	if ws.created == nil {
		t.Fatal("expected webhook delivery to be created")
	}
	if ws.created.WebhookURL != "https://www.google.com/webhook-test" {
		t.Fatalf("WebhookURL = %q", ws.created.WebhookURL)
	}
	if ws.created.MaxAttempts != 5 {
		t.Fatalf("MaxAttempts = %d, want 5", ws.created.MaxAttempts)
	}
	if ws.created.RetryPolicy != domain.WebhookRetryPolicyExponential {
		t.Fatalf("RetryPolicy = %q, want exponential", ws.created.RetryPolicy)
	}
	if ws.created.RunID != "run-1" {
		t.Fatalf("RunID = %q, want run-1", ws.created.RunID)
	}
}

func TestFireAgentWebhookFallsBackWhenNoStore(t *testing.T) {
	t.Parallel()

	// When webhookStore is nil and dispatchHTTP is nil, the fallback path
	// exits after checking dispatchHTTP == nil. This confirms the durable
	// delivery path is skipped and the direct-fire path is entered.
	svc := &localService{
		store: &mockAgentStoreForWebhook{
			run: &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted},
		},
		dispatchHTTP: nil, // no HTTP client -> fallback path exits silently
		webhookStore: nil,
		now:          time.Now,
	}

	agent := &domain.Agent{
		ID:     "agent-1",
		Slug:   "test-agent",
		Config: json.RawMessage(`{"webhook_url":"https://www.google.com/fallback-test"}`),
	}
	// Should not panic. Falls through to direct-fire path, exits because dispatchHTTP is nil.
	svc.fireAgentWebhook(context.Background(), agent, "run-1")
}

func TestFireAgentWebhookSkipsEmptyURL(t *testing.T) {
	t.Parallel()

	ws := &mockWebhookStore{}
	svc := &localService{
		store:        &mockAgentStoreForWebhook{run: &domain.JobRun{ID: "run-1"}},
		webhookStore: ws,
		now:          time.Now,
	}

	agent := &domain.Agent{ID: "agent-1", Config: json.RawMessage(`{}`)}
	svc.fireAgentWebhook(context.Background(), agent, "run-1")

	if ws.created != nil {
		t.Fatal("expected no delivery for empty webhook URL")
	}
}

func TestFireAgentWebhookSkipsUnsafeURL(t *testing.T) {
	t.Parallel()

	ws := &mockWebhookStore{}
	svc := &localService{
		store:        &mockAgentStoreForWebhook{run: &domain.JobRun{ID: "run-1"}},
		webhookStore: ws,
		now:          time.Now,
	}

	agent := &domain.Agent{
		ID:     "agent-1",
		Config: json.RawMessage(`{"webhook_url":"http://169.254.169.254/metadata"}`),
	}
	svc.fireAgentWebhook(context.Background(), agent, "run-1")

	if ws.created != nil {
		t.Fatal("expected no delivery for unsafe URL")
	}
}

func TestFireAgentWebhookNilAgent(t *testing.T) {
	t.Parallel()

	svc := &localService{now: time.Now}
	// Should not panic.
	svc.fireAgentWebhook(context.Background(), nil, "run-1")
}

func TestFireAgentWebhookNilRun(t *testing.T) {
	t.Parallel()

	svc := &localService{
		store:        &mockAgentStoreForWebhook{run: nil},
		webhookStore: &mockWebhookStore{},
		now:          time.Now,
	}

	agent := &domain.Agent{
		ID:     "agent-1",
		Config: json.RawMessage(`{"webhook_url":"https://www.google.com/webhook-test"}`),
	}
	// Should not panic, should log error.
	svc.fireAgentWebhook(context.Background(), agent, "run-1")
}

func TestFireAgentWebhookPayloadShape(t *testing.T) {
	t.Parallel()

	svc := &localService{now: time.Now}
	run := &domain.JobRun{
		ID:      "run-1",
		Status:  domain.StatusCompleted,
		Attempt: 2,
		Error:   "retry succeeded",
	}
	agent := &domain.Agent{ID: "agent-1", Slug: "support-agent"}

	payload := svc.buildWebhookPayload(agent, run)
	var parsed map[string]any
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}

	requiredKeys := []string{"event", "agent_id", "agent_slug", "run_id", "status", "attempt", "timestamp"}
	for _, key := range requiredKeys {
		if _, ok := parsed[key]; !ok {
			t.Fatalf("payload missing required key %q", key)
		}
	}
	if parsed["event"] != "agent.run.terminal" {
		t.Fatalf("event = %q", parsed["event"])
	}
	if parsed["agent_id"] != "agent-1" {
		t.Fatalf("agent_id = %q", parsed["agent_id"])
	}
}

func TestFireAgentWebhookPayloadNilResult(t *testing.T) {
	t.Parallel()

	svc := &localService{now: time.Now}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusFailed, Result: nil}
	agent := &domain.Agent{ID: "agent-1", Slug: "test"}

	payload := svc.buildWebhookPayload(agent, run)
	if !json.Valid(payload) {
		t.Fatal("payload with nil result should still be valid JSON")
	}
}

func TestFireAgentWebhookConcurrentCalls(t *testing.T) {
	t.Parallel()

	ws := &mockWebhookStore{}
	store := &mockAgentStoreForWebhook{
		run: &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted},
	}
	svc := &localService{
		store:        store,
		webhookStore: ws,
		now:          time.Now,
	}

	agent := &domain.Agent{
		ID:     "agent-1",
		Slug:   "concurrent-agent",
		Config: json.RawMessage(`{"webhook_url":"https://www.google.com/webhook-concurrent"}`),
	}

	// Fire 100 concurrent webhook calls. Should not race.
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			svc.fireAgentWebhook(context.Background(), agent, "run-1")
		})
	}
	wg.Wait()
}
