package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	CloudflareSandboxModeOutboundWorker CloudflareSandboxMode = "outbound_worker"
)

var (
	ErrCloudflareProviderUnimplemented = errors.New("cloudflare provider behavior is not implemented yet")
)

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
	case "", CloudflareSandboxModeDisabled, CloudflareSandboxModeOutboundWorker:
	default:
		return &domain.ConfigError{Field: "CF_SANDBOX_MODE", Message: "must be disabled or outbound_worker"}
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
	Mode               CloudflareSandboxMode `json:"mode"`
	OutboundWorkerName string                `json:"outbound_worker_name,omitempty"`
	NetworkClass       string                `json:"network_class,omitempty"`
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
	DeploymentID string                  `json:"deployment_id"`
	Provider     string                  `json:"provider"`
	Namespace    string                  `json:"namespace"`
	ScriptName   string                  `json:"script_name"`
	RunID        string                  `json:"run_id"`
	Envelope     RuntimeDispatchEnvelope `json:"envelope"`
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
	return &metadata, nil
}

type CloudflareProvider struct {
	config CloudflareConfig
}

func NewCloudflareProvider(cfg CloudflareConfig) *CloudflareProvider {
	normalized := cfg
	if normalized.SandboxMode == "" {
		normalized.SandboxMode = CloudflareSandboxModeOutboundWorker
	}
	return &CloudflareProvider{config: normalized}
}

func (p *CloudflareProvider) Name() string {
	return ProviderNameCloudflare
}

func (p *CloudflareProvider) Config() CloudflareConfig {
	return p.config
}

func (p *CloudflareProvider) Deploy(context.Context, *domain.Agent, *domain.AgentDeployment) (json.RawMessage, error) {
	return nil, ErrCloudflareProviderUnimplemented
}

func (p *CloudflareProvider) Run(context.Context, *domain.Agent, *domain.AgentDeployment, *domain.JobRun) (json.RawMessage, error) {
	return nil, ErrCloudflareProviderUnimplemented
}

func SelectProvider(cf CloudflareConfig) Provider {
	if cf.Enabled() {
		return NewCloudflareProvider(cf)
	}
	return LocalStubProvider{}
}
