package client

import (
	"encoding/json"
	"time"

	"strait/internal/domain"
)

type CreateJobRequest struct {
	ProjectID   string          `json:"project_id"`
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Description string          `json:"description,omitempty"`
	Cron        string          `json:"cron,omitempty"`
	EndpointURL string          `json:"endpoint_url"`
	MaxAttempts int             `json:"max_attempts,omitempty"`
	TimeoutSecs int             `json:"timeout_secs,omitempty"`
	RunTTLSecs  int             `json:"run_ttl_secs,omitempty"`
	Schema      json.RawMessage `json:"payload_schema,omitempty"`
}

type UpdateJobRequest struct {
	Name          *string          `json:"name,omitempty"`
	Slug          *string          `json:"slug,omitempty"`
	Description   *string          `json:"description,omitempty"`
	Cron          *string          `json:"cron,omitempty"`
	EndpointURL   *string          `json:"endpoint_url,omitempty"`
	MaxAttempts   *int             `json:"max_attempts,omitempty"`
	TimeoutSecs   *int             `json:"timeout_secs,omitempty"`
	RunTTLSecs    *int             `json:"run_ttl_secs,omitempty"`
	Enabled       *bool            `json:"enabled,omitempty"`
	Schema        *json.RawMessage `json:"payload_schema,omitempty"`
	ImageURI      *string          `json:"image_uri,omitempty"`
	MachinePreset *string          `json:"machine_preset,omitempty"`
	Region        *string          `json:"region,omitempty"`
}

type TriggerJobRequest struct {
	Payload     json.RawMessage `json:"payload,omitempty"`
	ScheduledAt *time.Time      `json:"scheduled_at,omitempty"`
	Priority    int             `json:"priority,omitempty"`
}

type TriggerJobResponse struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	RunToken       string `json:"run_token,omitempty"`
	PayloadHash    string `json:"payload_hash,omitempty"`
	IdempotencyHit bool   `json:"idempotency_hit"`
}

type BulkTriggerItem struct {
	Payload        json.RawMessage `json:"payload,omitempty"`
	ScheduledAt    *time.Time      `json:"scheduled_at,omitempty"`
	Priority       int             `json:"priority,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
}

type BulkTriggerRequest struct {
	Items []BulkTriggerItem `json:"items"`
}

type BulkTriggerResult struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	RunToken       string `json:"run_token,omitempty"`
	IdempotencyHit bool   `json:"idempotency_hit"`
}

type BulkTriggerResponse struct {
	Results []BulkTriggerResult `json:"results"`
	Total   int                 `json:"total"`
	Created int                 `json:"created"`
}

type HealthStatus struct {
	Status string `json:"status"`
}

type WorkflowStepRequest struct {
	JobID     string          `json:"job_id"`
	StepRef   string          `json:"step_ref"`
	DependsOn []string        `json:"depends_on,omitempty"`
	Condition json.RawMessage `json:"condition,omitempty"`
	OnFailure string          `json:"on_failure,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type CreateWorkflowRequest struct {
	ProjectID   string                `json:"project_id"`
	Name        string                `json:"name"`
	Slug        string                `json:"slug"`
	Description string                `json:"description,omitempty"`
	Enabled     *bool                 `json:"enabled,omitempty"`
	Steps       []WorkflowStepRequest `json:"steps,omitempty"`
}

type UpdateWorkflowRequest struct {
	Name        *string                `json:"name,omitempty"`
	Slug        *string                `json:"slug,omitempty"`
	Description *string                `json:"description,omitempty"`
	Enabled     *bool                  `json:"enabled,omitempty"`
	Steps       *[]WorkflowStepRequest `json:"steps,omitempty"`
}

type WorkflowResponse struct {
	domain.Workflow
	Steps []domain.WorkflowStep `json:"steps"`
}

type TriggerWorkflowRequest struct {
	ProjectID   string          `json:"project_id,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	TriggeredBy string          `json:"triggered_by,omitempty"`
}

type CreateAPIKeyRequest struct {
	ProjectID string   `json:"project_id"`
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes,omitempty"`
}

type APIKeyCreateResponse struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"project_id"`
	Name      string     `json:"name"`
	Key       string     `json:"key"`
	KeyPrefix string     `json:"key_prefix"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type RotateAPIKeyRequest struct {
	GracePeriodMinutes int `json:"grace_period_minutes,omitempty"`
}

type RotateAPIKeyResponse struct {
	OldKeyID       string     `json:"old_key_id"`
	NewKeyID       string     `json:"new_key_id"`
	ProjectID      string     `json:"project_id"`
	Name           string     `json:"name"`
	Key            string     `json:"key"`
	KeyPrefix      string     `json:"key_prefix"`
	Scopes         []string   `json:"scopes"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	GraceExpiresAt time.Time  `json:"grace_expires_at"`
}

type QueueStats struct {
	Queued    int `json:"queued"`
	Executing int `json:"executing"`
	Delayed   int `json:"delayed"`
}

// Deployment types.

type CreateDeploymentVersionRequest struct {
	ProjectID   string `json:"project_id"`
	Environment string `json:"environment"`
	Runtime     string `json:"runtime"`
	Manifest    any    `json:"manifest,omitempty"`
	Checksum    string `json:"checksum,omitempty"`
	ArtifactURI string `json:"artifact_uri"`
}

type DeploymentVersion struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Environment string    `json:"environment"`
	Status      string    `json:"status"`
	Checksum    string    `json:"checksum,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type FinalizeDeploymentRequest struct {
	ProjectID   string `json:"project_id"`
	Environment string `json:"environment"`
}

type PromoteDeploymentRequest struct {
	ProjectID   string `json:"project_id"`
	Environment string `json:"environment"`
}

type RollbackDeploymentRequest struct {
	ProjectID   string `json:"project_id"`
	Environment string `json:"environment"`
}

// Server-side secret types.

type ServerSecret struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	SecretKey   string    `json:"secret_key"`
	Environment string    `json:"environment"`
	JobID       string    `json:"job_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateServerSecretRequest struct {
	ProjectID   string `json:"project_id"`
	SecretKey   string `json:"secret_key"`
	SecretValue string `json:"secret_value"`
	Environment string `json:"environment"`
	JobID       string `json:"job_id,omitempty"`
}

// Performance analytics types.

type PerformanceAnalytics struct {
	SlowestJobs   []JobPerformance `json:"slowest_jobs"`
	Throughput    ThroughputStats  `json:"throughput"`
	HealthSummary HealthSummary    `json:"health_summary"`
}

type JobPerformance struct {
	JobID           string  `json:"job_id"`
	JobSlug         string  `json:"job_slug"`
	AvgDurationSecs float64 `json:"avg_duration_secs"`
	P95DurationSecs float64 `json:"p95_duration_secs"`
	TotalRuns       int     `json:"total_runs"`
	FailedRuns      int     `json:"failed_runs"`
}

type ThroughputStats struct {
	Completed   int `json:"completed"`
	Failed      int `json:"failed"`
	TimedOut    int `json:"timed_out"`
	Canceled    int `json:"canceled"`
	PeriodHours int `json:"period_hours"`
}

type HealthSummary struct {
	TotalJobs       int     `json:"total_jobs"`
	ActiveJobs      int     `json:"active_jobs"`
	SuccessRate     float64 `json:"success_rate"`
	AvgDurationSecs float64 `json:"avg_duration_secs"`
	QueueDepth      int     `json:"queue_depth"`
}

// Team/RBAC types.

type ProjectMember = domain.ProjectMemberRole

type AssignMemberRequest struct {
	UserID string `json:"user_id"`
	RoleID string `json:"role_id"`
}

type ProjectRole = domain.ProjectRole

type AuditEvent = domain.AuditEvent

type ListAuditEventsParams struct {
	ProjectID    string
	ActorID      string
	ResourceType string
	ResourceID   string
	Limit        int
	From         *time.Time
	To           *time.Time
	Order        string
}
