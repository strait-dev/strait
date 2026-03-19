package manifest

// ProjectConfig represents the top-level strait.json or strait.config.yaml structure.
type ProjectConfig struct {
	Project   ProjectInfo          `json:"project" yaml:"project"`
	Runtime   string               `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	Build     BuildConfig          `json:"build" yaml:"build"`
	Deploy    DeployConfig         `json:"deploy" yaml:"deploy"`
	Jobs      []JobDefinition      `json:"jobs,omitempty" yaml:"jobs,omitempty"`
	Workflows []WorkflowDefinition `json:"workflows,omitempty" yaml:"workflows,omitempty"`
}

// ProjectInfo contains project identification.
type ProjectInfo struct {
	ID   string `json:"id" yaml:"id"`
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
}

// BuildConfig contains build settings.
type BuildConfig struct {
	OutDir string `json:"outDir,omitempty" yaml:"outDir,omitempty"`
}

// DeployConfig contains deployment defaults.
type DeployConfig struct {
	DefaultEnvironment string `json:"defaultEnvironment,omitempty" yaml:"defaultEnvironment,omitempty"`
}

// JobDefinition represents a job in the config.
type JobDefinition struct {
	Slug        string `json:"slug" yaml:"slug"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	EndpointURL string `json:"endpointUrl,omitempty" yaml:"endpointUrl,omitempty"`
	Cron        string `json:"cron,omitempty" yaml:"cron,omitempty"`
	MaxAttempts int    `json:"maxAttempts,omitempty" yaml:"maxAttempts,omitempty"`
	TimeoutSecs int    `json:"timeoutSecs,omitempty" yaml:"timeoutSecs,omitempty"`
}

// WorkflowDefinition represents a workflow in the config.
type WorkflowDefinition struct {
	Slug        string            `json:"slug" yaml:"slug"`
	Name        string            `json:"name" yaml:"name"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Steps       []WorkflowStepDef `json:"steps,omitempty" yaml:"steps,omitempty"`
}

// WorkflowStepDef represents a workflow step in the config.
type WorkflowStepDef struct {
	StepRef   string   `json:"stepRef" yaml:"stepRef"`
	JobRef    string   `json:"jobRef" yaml:"jobRef"`
	DependsOn []string `json:"dependsOn,omitempty" yaml:"dependsOn,omitempty"`
	OnFailure string   `json:"onFailure,omitempty" yaml:"onFailure,omitempty"`
}

// ProjectManifest is the compiled output of a ProjectConfig.
type ProjectManifest struct {
	Version     int                  `json:"version"`
	ProjectID   string               `json:"project_id"`
	ProjectName string               `json:"project_name,omitempty"`
	Runtime     string               `json:"runtime,omitempty"`
	Checksum    string               `json:"checksum"`
	Jobs        []JobDefinition      `json:"jobs,omitempty"`
	Workflows   []WorkflowDefinition `json:"workflows,omitempty"`
}
