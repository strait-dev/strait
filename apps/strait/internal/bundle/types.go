package bundle

import (
	"encoding/json"
	"time"
)

// Version is the current bundle format version.
const Version = "1"

// RedactedPlaceholder replaces sensitive values in exports.
const RedactedPlaceholder = "<REDACTED>"

// Bundle represents a complete project configuration export.
type Bundle struct {
	Version         string    `yaml:"version" json:"version"`
	ExportedAt      time.Time `yaml:"exported_at" json:"exported_at"`
	SourceProjectID string    `yaml:"source_project_id" json:"source_project_id"`
	Resources       Resources `yaml:"resources" json:"resources"`
}

// Resources contains all exportable project resources.
type Resources struct {
	Jobs                 []JobSpec                 `yaml:"jobs,omitempty" json:"jobs,omitempty"`
	Workflows            []WorkflowSpec            `yaml:"workflows,omitempty" json:"workflows,omitempty"`
	Environments         []EnvironmentSpec         `yaml:"environments,omitempty" json:"environments,omitempty"`
	WebhookSubscriptions []WebhookSubscriptionSpec `yaml:"webhook_subscriptions,omitempty" json:"webhook_subscriptions,omitempty"`
}

// JobSpec represents a job in the bundle format.
type JobSpec struct {
	Slug                      string            `yaml:"slug" json:"slug"`
	Name                      string            `yaml:"name" json:"name"`
	Description               string            `yaml:"description,omitempty" json:"description,omitempty"`
	EndpointURL               string            `yaml:"endpoint_url" json:"endpoint_url"`
	FallbackEndpointURL       string            `yaml:"fallback_endpoint_url,omitempty" json:"fallback_endpoint_url,omitempty"`
	MaxAttempts               int               `yaml:"max_attempts" json:"max_attempts"`
	TimeoutSecs               int               `yaml:"timeout_secs" json:"timeout_secs"`
	MaxConcurrency            int               `yaml:"max_concurrency,omitempty" json:"max_concurrency,omitempty"`
	Cron                      string            `yaml:"cron,omitempty" json:"cron,omitempty"`
	Timezone                  string            `yaml:"timezone,omitempty" json:"timezone,omitempty"`
	PayloadSchema             json.RawMessage   `yaml:"payload_schema,omitempty" json:"payload_schema,omitempty"`
	Tags                      map[string]string `yaml:"tags,omitempty" json:"tags,omitempty"`
	RetryStrategy             string            `yaml:"retry_strategy,omitempty" json:"retry_strategy,omitempty"`
	Enabled                   bool              `yaml:"enabled" json:"enabled"`
	WebhookURL                string            `yaml:"webhook_url,omitempty" json:"webhook_url,omitempty"`
	EnvironmentSlug           string            `yaml:"environment_slug,omitempty" json:"environment_slug,omitempty"`
	OnCompleteTriggerWorkflow string            `yaml:"on_complete_trigger_workflow,omitempty" json:"on_complete_trigger_workflow,omitempty"`
}

// WorkflowSpec represents a workflow in the bundle format.
type WorkflowSpec struct {
	Slug              string             `yaml:"slug" json:"slug"`
	Name              string             `yaml:"name" json:"name"`
	Description       string             `yaml:"description,omitempty" json:"description,omitempty"`
	MaxConcurrentRuns int                `yaml:"max_concurrent_runs,omitempty" json:"max_concurrent_runs,omitempty"`
	Steps             []WorkflowStepSpec `yaml:"steps,omitempty" json:"steps,omitempty"`
}

// WorkflowStepSpec represents a workflow step in the bundle format.
type WorkflowStepSpec struct {
	StepRef   string   `yaml:"step_ref" json:"step_ref"`
	JobSlug   string   `yaml:"job_slug,omitempty" json:"job_slug,omitempty"`
	DependsOn []string `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	Condition string   `yaml:"condition,omitempty" json:"condition,omitempty"`
	OnFailure string   `yaml:"on_failure,omitempty" json:"on_failure,omitempty"`
}

// EnvironmentSpec represents an environment in the bundle format.
type EnvironmentSpec struct {
	Name       string            `yaml:"name" json:"name"`
	Slug       string            `yaml:"slug" json:"slug"`
	ParentSlug string            `yaml:"parent_slug,omitempty" json:"parent_slug,omitempty"`
	IsStandard bool              `yaml:"is_standard,omitempty" json:"is_standard,omitempty"`
	Variables  map[string]string `yaml:"variables,omitempty" json:"variables,omitempty"`
}

// WebhookSubscriptionSpec represents a webhook subscription in the bundle format.
type WebhookSubscriptionSpec struct {
	URL    string   `yaml:"url" json:"url"`
	Events []string `yaml:"events" json:"events"`
}

// DiffAction describes the action to take during import.
type DiffAction string

const (
	DiffCreate DiffAction = "CREATE"
	DiffUpdate DiffAction = "UPDATE"
	DiffSkip   DiffAction = "SKIP"
)

// DiffEntry describes a single resource diff during dry-run import.
type DiffEntry struct {
	ResourceType string     `json:"resource_type"`
	Slug         string     `json:"slug"`
	Action       DiffAction `json:"action"`
	Details      string     `json:"details,omitempty"`
}

// ImportResult summarizes the outcome of an import operation.
type ImportResult struct {
	Created int         `json:"created"`
	Updated int         `json:"updated"`
	Skipped int         `json:"skipped"`
	Failed  int         `json:"failed"`
	Errors  []string    `json:"errors,omitempty"`
	Diff    []DiffEntry `json:"diff,omitempty"`
}
