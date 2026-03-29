package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"strait/internal/domain"
)

const (
	ProviderNameLocalStub  = "local_stub"
	ProviderNameCloudflare = "cloudflare"
)

type CloudflareSandboxMode string

const (
	CloudflareSandboxModeDisabled       CloudflareSandboxMode = "disabled"
	CloudflareSandboxModeDynamicWorker  CloudflareSandboxMode = "dynamic_worker"
	CloudflareSandboxModeOutboundWorker CloudflareSandboxMode = "outbound_worker"
)

type CloudflareSandboxDefaultAction string

const (
	CloudflareSandboxDefaultActionAllow CloudflareSandboxDefaultAction = "allow"
	CloudflareSandboxDefaultActionDeny  CloudflareSandboxDefaultAction = "deny"
)

var ErrCloudflareProviderUnimplemented = errors.New("cloudflare provider client is not configured")

type CloudflareProviderOption interface {
	applyProvider(*CloudflareProvider)
	applyClient(*CloudflareAPIClient)
}

type cloudflareProviderOptionFunc struct {
	applyProviderFn func(*CloudflareProvider)
	applyClientFn   func(*CloudflareAPIClient)
}

func (o cloudflareProviderOptionFunc) applyProvider(p *CloudflareProvider) {
	if o.applyProviderFn != nil {
		o.applyProviderFn(p)
	}
}

func (o cloudflareProviderOptionFunc) applyClient(c *CloudflareAPIClient) {
	if o.applyClientFn != nil {
		o.applyClientFn(c)
	}
}

func WithCloudflareHTTPClient(client *http.Client) CloudflareProviderOption {
	return cloudflareProviderOptionFunc{
		applyProviderFn: func(p *CloudflareProvider) {
			if client != nil {
				if cfClient, ok := p.client.(*CloudflareAPIClient); ok {
					cfClient.httpClient = client
				}
			}
		},
		applyClientFn: func(c *CloudflareAPIClient) {
			if client != nil {
				c.httpClient = client
			}
		},
	}
}

func WithCloudflareAPIBaseURL(baseURL string) CloudflareProviderOption {
	return cloudflareProviderOptionFunc{
		applyProviderFn: func(p *CloudflareProvider) {
			if strings.TrimSpace(baseURL) != "" {
				if cfClient, ok := p.client.(*CloudflareAPIClient); ok {
					cfClient.baseURL = baseURL
				}
			}
		},
		applyClientFn: func(c *CloudflareAPIClient) {
			if strings.TrimSpace(baseURL) != "" {
				c.baseURL = baseURL
			}
		},
	}
}

type CloudflareConfig struct {
	AccountID                string
	APIToken                 string
	DispatchNamespace        string
	DispatchNamespaceStaging string
	DispatchWorkerURL        string
	OutboundWorkerName       string
	CompatibilityDate        string
	SandboxMode              CloudflareSandboxMode
}

func (c CloudflareConfig) Enabled() bool {
	return strings.TrimSpace(c.AccountID) != "" ||
		strings.TrimSpace(c.APIToken) != "" ||
		strings.TrimSpace(c.DispatchNamespace) != "" ||
		strings.TrimSpace(c.DispatchNamespaceStaging) != "" ||
		strings.TrimSpace(c.DispatchWorkerURL) != "" ||
		strings.TrimSpace(c.OutboundWorkerName) != "" ||
		strings.TrimSpace(c.CompatibilityDate) != ""
}

func (c CloudflareConfig) Validate() error {
	if !c.Enabled() {
		return nil
	}

	required := []struct {
		field string
		value string
	}{
		{field: "CF_ACCOUNT_ID", value: c.AccountID},
		{field: "CF_API_TOKEN", value: c.APIToken},
		{field: "CF_DISPATCH_NAMESPACE", value: c.DispatchNamespace},
		{field: "CF_DISPATCH_WORKER_URL", value: c.DispatchWorkerURL},
		{field: "CF_COMPATIBILITY_DATE", value: c.CompatibilityDate},
	}
	for _, item := range required {
		if strings.TrimSpace(item.value) == "" {
			return &domain.ConfigError{Field: item.field, Message: "is required when Cloudflare agents are enabled"}
		}
	}

	switch c.SandboxMode {
	case "", CloudflareSandboxModeDisabled, CloudflareSandboxModeDynamicWorker, CloudflareSandboxModeOutboundWorker:
	default:
		return &domain.ConfigError{Field: "CF_SANDBOX_MODE", Message: "must be disabled, dynamic_worker, or outbound_worker"}
	}

	if c.SandboxMode == CloudflareSandboxModeOutboundWorker && strings.TrimSpace(c.OutboundWorkerName) == "" {
		return &domain.ConfigError{Field: "CF_OUTBOUND_WORKER_NAME", Message: "is required when CF_SANDBOX_MODE=outbound_worker"}
	}

	u, err := url.Parse(c.DispatchWorkerURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return &domain.ConfigError{Field: "CF_DISPATCH_WORKER_URL", Message: "must be a valid HTTP(S) URL"}
	}

	return nil
}

type CloudflareSandboxPolicy struct {
	Mode               CloudflareSandboxMode          `json:"mode"`
	OutboundWorkerName string                         `json:"outbound_worker_name,omitempty"`
	DefaultAction      CloudflareSandboxDefaultAction `json:"default_action,omitempty"`
	AllowHosts         []string                       `json:"allow_hosts,omitempty"`
	NetworkClass       string                         `json:"network_class,omitempty"`
	PolicyTag          string                         `json:"policy_tag,omitempty"`
}

type CloudflareDeploymentMetadata struct {
	Provider          string                  `json:"provider"`
	Namespace         string                  `json:"namespace"`
	ScriptName        string                  `json:"script_name"`
	DeploymentVersion int                     `json:"deployment_version"`
	DispatchWorkerURL string                  `json:"dispatch_worker_url"`
	OutboundWorker    string                  `json:"outbound_worker_name,omitempty"`
	CompatibilityDate string                  `json:"compatibility_date"`
	ContentSHA256     string                  `json:"content_sha256,omitempty"`
	Etag              string                  `json:"etag,omitempty"`
	SandboxPolicy     CloudflareSandboxPolicy `json:"sandbox_policy"`
}

type CloudflareDispatchRequest struct {
	DeploymentID  string                  `json:"deployment_id"`
	Provider      string                  `json:"provider"`
	Namespace     string                  `json:"namespace"`
	ScriptName    string                  `json:"script_name"`
	RunID         string                  `json:"run_id"`
	SandboxPolicy CloudflareSandboxPolicy `json:"sandbox_policy"`
	Envelope      RuntimeDispatchEnvelope `json:"envelope"`
}

func MarshalCloudflareDeploymentMetadata(metadata CloudflareDeploymentMetadata) json.RawMessage {
	raw, _ := json.Marshal(metadata)
	return raw
}

func ParseCloudflareDeploymentMetadata(raw json.RawMessage) (*CloudflareDeploymentMetadata, error) {
	if len(raw) == 0 {
		return nil, errors.New("cloudflare deployment metadata is required")
	}

	var metadata CloudflareDeploymentMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, fmt.Errorf("decode cloudflare deployment metadata: %w", err)
	}
	if metadata.Provider != ProviderNameCloudflare {
		return nil, fmt.Errorf("unexpected provider %q", metadata.Provider)
	}
	if strings.TrimSpace(metadata.Namespace) == "" {
		return nil, errors.New("cloudflare deployment metadata namespace is required")
	}
	if strings.TrimSpace(metadata.ScriptName) == "" {
		return nil, errors.New("cloudflare deployment metadata script_name is required")
	}
	if strings.TrimSpace(metadata.DispatchWorkerURL) == "" {
		return nil, errors.New("cloudflare deployment metadata dispatch_worker_url is required")
	}
	if strings.TrimSpace(metadata.CompatibilityDate) == "" {
		return nil, errors.New("cloudflare deployment metadata compatibility_date is required")
	}
	if err := validateCloudflareSandboxPolicy(metadata.SandboxPolicy); err != nil {
		return nil, fmt.Errorf("cloudflare deployment metadata sandbox_policy: %w", err)
	}
	return &metadata, nil
}

func validateCloudflareSandboxPolicy(policy CloudflareSandboxPolicy) error {
	switch policy.Mode {
	case "", CloudflareSandboxModeDisabled, CloudflareSandboxModeDynamicWorker, CloudflareSandboxModeOutboundWorker:
	default:
		return fmt.Errorf("mode %q is invalid", policy.Mode)
	}

	switch policy.DefaultAction {
	case "", CloudflareSandboxDefaultActionAllow, CloudflareSandboxDefaultActionDeny:
	default:
		return fmt.Errorf("default_action %q is invalid", policy.DefaultAction)
	}

	for _, host := range policy.AllowHosts {
		if strings.TrimSpace(host) == "" {
			return errors.New("allow_hosts cannot contain empty values")
		}
	}
	return nil
}

type cloudflareSandboxPolicyOverride struct {
	AllowHosts    []string `json:"allow_hosts,omitempty"`
	DefaultAction string   `json:"default_action,omitempty"`
	NetworkClass  string   `json:"network_class,omitempty"`
	PolicyTag     string   `json:"policy_tag,omitempty"`
}

func resolveCloudflareSandboxPolicy(cfg CloudflareConfig, snapshot json.RawMessage) CloudflareSandboxPolicy {
	policy := CloudflareSandboxPolicy{
		Mode:               cfg.SandboxMode,
		OutboundWorkerName: cfg.OutboundWorkerName,
	}
	if cfg.SandboxMode == CloudflareSandboxModeDynamicWorker || cfg.SandboxMode == CloudflareSandboxModeOutboundWorker {
		policy.DefaultAction = CloudflareSandboxDefaultActionDeny
		policy.NetworkClass = "sandbox"
		policy.PolicyTag = "default"
	}

	override, ok := parseCloudflareSandboxPolicyOverride(snapshot)
	if !ok {
		return policy
	}
	if action := CloudflareSandboxDefaultAction(strings.TrimSpace(override.DefaultAction)); action != "" {
		policy.DefaultAction = action
	}
	if hosts := normalizeHostAllowlist(override.AllowHosts); len(hosts) > 0 {
		policy.AllowHosts = hosts
	}
	if networkClass := strings.TrimSpace(override.NetworkClass); networkClass != "" {
		policy.NetworkClass = networkClass
	}
	if policyTag := strings.TrimSpace(override.PolicyTag); policyTag != "" {
		policy.PolicyTag = policyTag
	}
	return policy
}

func parseCloudflareSandboxPolicyOverride(raw json.RawMessage) (cloudflareSandboxPolicyOverride, bool) {
	if len(raw) == 0 {
		return cloudflareSandboxPolicyOverride{}, false
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return cloudflareSandboxPolicyOverride{}, false
	}

	sandboxRaw, ok := envelope["sandbox"]
	if !ok || len(sandboxRaw) == 0 {
		return cloudflareSandboxPolicyOverride{}, false
	}

	var sandboxEnvelope map[string]json.RawMessage
	if err := json.Unmarshal(sandboxRaw, &sandboxEnvelope); err != nil {
		return cloudflareSandboxPolicyOverride{}, false
	}

	policyRaw, ok := sandboxEnvelope["policy"]
	if !ok || len(policyRaw) == 0 {
		return cloudflareSandboxPolicyOverride{}, false
	}

	var override cloudflareSandboxPolicyOverride
	if err := json.Unmarshal(policyRaw, &override); err != nil {
		return cloudflareSandboxPolicyOverride{}, false
	}

	return override, true
}

func normalizeHostAllowlist(hosts []string) []string {
	if len(hosts) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(hosts))
	normalized := make([]string, 0, len(hosts))
	for _, host := range hosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host == "" {
			continue
		}
		if _, exists := seen[host]; exists {
			continue
		}
		seen[host] = struct{}{}
		normalized = append(normalized, host)
	}
	return normalized
}

type CloudflareProvider struct {
	config CloudflareConfig
	client cloudflareScriptsClient
}

func NewCloudflareProvider(cfg CloudflareConfig, opts ...CloudflareProviderOption) *CloudflareProvider {
	normalized := cfg
	if normalized.SandboxMode == "" {
		normalized.SandboxMode = CloudflareSandboxModeDisabled
	}
	provider := &CloudflareProvider{
		config: normalized,
		client: NewCloudflareAPIClient(normalized, opts...),
	}
	for _, opt := range opts {
		if opt != nil {
			opt.applyProvider(provider)
		}
	}
	return provider
}

func (p *CloudflareProvider) Name() string {
	return ProviderNameCloudflare
}

func (p *CloudflareProvider) Config() CloudflareConfig {
	return p.config
}

func (p *CloudflareProvider) Deploy(ctx context.Context, agent *domain.Agent, deployment *domain.AgentDeployment) (json.RawMessage, error) {
	if p.client == nil {
		return nil, ErrCloudflareProviderUnimplemented
	}

	scriptName := buildCloudflareScriptName(agent.ID, deployment.Version)
	namespace := p.config.DispatchNamespace
	sandboxPolicy := resolveCloudflareSandboxPolicy(p.config, deployment.ConfigSnapshot)

	result, err := p.client.UpsertScript(ctx, CloudflareScriptUploadRequest{
		Namespace:         namespace,
		ScriptName:        scriptName,
		CompatibilityDate: p.config.CompatibilityDate,
		OutboundWorker:    p.config.OutboundWorkerName,
		SandboxPolicy:     sandboxPolicy,
		Tags: []string{
			"strait-agent",
			"agent:" + agent.ID,
			"deployment:" + deployment.ID,
		},
		Source: buildCloudflareWorkerSource(agent, deployment),
	})
	if err != nil {
		return nil, fmt.Errorf("deploy cloudflare worker: %w", err)
	}

	return MarshalCloudflareDeploymentMetadata(CloudflareDeploymentMetadata{
		Provider:          ProviderNameCloudflare,
		Namespace:         namespace,
		ScriptName:        scriptName,
		DeploymentVersion: deployment.Version,
		DispatchWorkerURL: p.config.DispatchWorkerURL,
		OutboundWorker:    p.config.OutboundWorkerName,
		CompatibilityDate: result.CompatibilityDate,
		ContentSHA256:     result.ContentSHA256,
		Etag:              result.ETag,
		SandboxPolicy:     sandboxPolicy,
	}), nil
}

func (p *CloudflareProvider) Undeploy(ctx context.Context, _ *domain.Agent, deployment *domain.AgentDeployment) error {
	if p.client == nil {
		return ErrCloudflareProviderUnimplemented
	}
	if deployment == nil || len(deployment.ProviderMetadata) == 0 || deployment.Provider != ProviderNameCloudflare {
		return nil
	}
	metadata, err := ParseCloudflareDeploymentMetadata(deployment.ProviderMetadata)
	if err != nil {
		return fmt.Errorf("parse deployment metadata for undeploy: %w", err)
	}
	if err := p.client.DeleteScript(ctx, metadata.Namespace, metadata.ScriptName); err != nil {
		return fmt.Errorf("undeploy cloudflare worker: %w", err)
	}
	return nil
}

func (p *CloudflareProvider) Run(_ context.Context, agent *domain.Agent, deployment *domain.AgentDeployment, run *domain.JobRun) (json.RawMessage, error) {
	// The Cloudflare dispatch path is handled by the service layer via
	// dispatchCloudflareRun, not through the Provider.Run interface.
	// This method returns metadata describing where the run would be
	// dispatched, so callers can inspect the target without triggering
	// the actual dispatch.
	metadata, err := ParseCloudflareDeploymentMetadata(deployment.ProviderMetadata)
	if err != nil {
		return nil, fmt.Errorf("parse cloudflare deployment metadata: %w", err)
	}

	return mustJSON(map[string]any{
		"provider":            ProviderNameCloudflare,
		"namespace":           metadata.Namespace,
		"script_name":         metadata.ScriptName,
		"dispatch_worker_url": metadata.DispatchWorkerURL,
		"agent_id":            agent.ID,
		"deployment_id":       deployment.ID,
		"run_id":              run.ID,
	}), nil
}

func SelectProvider(cf CloudflareConfig) Provider {
	if cf.Enabled() {
		return NewCloudflareProvider(cf)
	}
	return LocalStubProvider{}
}

func buildCloudflareWorkerSource(_ *domain.Agent, _ *domain.AgentDeployment) string {
	return cloudflareRuntimeSource()
}
