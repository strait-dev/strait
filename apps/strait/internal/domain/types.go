package domain

import (
	"encoding/json"
	"time"
)

type RunStatus string

const (
	StatusDelayed      RunStatus = "delayed"
	StatusQueued       RunStatus = "queued"
	StatusDequeued     RunStatus = "dequeued"
	StatusExecuting    RunStatus = "executing"
	StatusWaiting      RunStatus = "waiting"
	StatusCompleted    RunStatus = "completed"
	StatusFailed       RunStatus = "failed"
	StatusTimedOut     RunStatus = "timed_out"
	StatusCrashed      RunStatus = "crashed"
	StatusSystemFailed RunStatus = "system_failed"
	StatusCanceled     RunStatus = "canceled"
	StatusExpired      RunStatus = "expired"
	StatusDeadLetter   RunStatus = "dead_letter"
	StatusReplayStaged RunStatus = "replay_staged"
)

const (
	TriggerManual   = "manual"
	TriggerCron     = "cron"
	TriggerSpawn    = "spawn"
	TriggerWorkflow = "workflow"
	TriggerRetry    = "retry"
	TriggerDebounce = "debounce"
)

const (
	WebhookEventRunCompleted      = "run.completed"
	WebhookEventRunFailed         = "run.failed"
	WebhookEventRunTimedOut       = "run.timed_out"
	WebhookEventRunCanceled       = "run.canceled"
	WebhookEventWorkflowCompleted = "workflow.completed"
	WebhookEventWorkflowFailed    = "workflow.failed"
)

type EventType string

const (
	EventLog         EventType = "log"
	EventStateChange EventType = "state_change"
	EventError       EventType = "error"
	EventProgress    EventType = "progress"
)

// ProjectRole defines a named set of permissions within a project.
type ProjectRole struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	Permissions  []string  `json:"permissions"`
	ParentRoleID string    `json:"parent_role_id,omitempty"`
	IsSystem     bool      `json:"is_system"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ProjectMemberRole links a user (from external auth) to a role within a project.
type ProjectMemberRole struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	UserID    string    `json:"user_id"`
	RoleID    string    `json:"role_id"`
	GrantedBy string    `json:"granted_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ResourcePolicy grants specific actions on a specific resource to a user,
// overriding or extending their role-based permissions.
type ResourcePolicy struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	UserID       string    `json:"user_id"`
	Actions      []string  `json:"actions"`
	CreatedAt    time.Time `json:"created_at"`
}

// TagPolicy grants actions on resources matching tags (e.g. team=payments).
type TagPolicy struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	ResourceType string    `json:"resource_type"`
	UserID       string    `json:"user_id"`
	TagKey       string    `json:"tag_key"`
	TagValue     string    `json:"tag_value,omitempty"`
	Actions      []string  `json:"actions"`
	CreatedAt    time.Time `json:"created_at"`
}

// SystemRolePermissions defines the default permission sets for system roles.
var SystemRolePermissions = map[string][]string{
	"admin": {"*"},
	"operator": {
		ScopeJobsRead, ScopeJobsWrite, ScopeJobsTrigger,
		ScopeRunsRead, ScopeRunsWrite,
		ScopeWorkflowsRead, ScopeWorkflowsWrite, ScopeWorkflowsTrigger,
		ScopeSecretsRead, ScopeStatsRead, ScopeRBACManage,
	},
	"viewer": {
		ScopeJobsRead, ScopeRunsRead, ScopeWorkflowsRead, ScopeStatsRead,
	},
	"triggerer": {
		ScopeJobsRead, ScopeJobsTrigger,
		ScopeRunsRead,
		ScopeWorkflowsRead, ScopeWorkflowsTrigger,
	},
}

// KnownActor is a lightweight cache of user info from an external auth provider.
type KnownActor struct {
	ID        string    `json:"id"`
	Email     string    `json:"email,omitempty"`
	Name      string    `json:"name,omitempty"`
	AvatarURL string    `json:"avatar_url,omitempty"`
	SyncedAt  time.Time `json:"synced_at"`
}

// AuditEvent records sensitive control-plane actions for compliance and forensics.
type AuditEvent struct {
	ID           string          `json:"id"`
	ProjectID    string          `json:"project_id"`
	ActorID      string          `json:"actor_id"`
	ActorType    string          `json:"actor_type"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	Details      json.RawMessage `json:"details,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

type Job struct {
	ID                       string            `json:"id"`
	ProjectID                string            `json:"project_id"`
	GroupID                  string            `json:"group_id,omitempty"`
	Name                     string            `json:"name"`
	Slug                     string            `json:"slug"`
	Description              string            `json:"description,omitempty"`
	Cron                     string            `json:"cron,omitempty"`
	PayloadSchema            json.RawMessage   `json:"payload_schema,omitempty"`
	Tags                     map[string]string `json:"tags,omitempty"`
	EndpointURL              string            `json:"endpoint_url"`
	FallbackEndpointURL      string            `json:"fallback_endpoint_url,omitempty"`
	MaxAttempts              int               `json:"max_attempts"`
	TimeoutSecs              int               `json:"timeout_secs"`
	MaxConcurrency           int               `json:"max_concurrency,omitempty"`
	MaxConcurrencyPerKey     int               `json:"max_concurrency_per_key,omitempty"`
	ExecutionWindowCron      string            `json:"execution_window_cron,omitempty"`
	Timezone                 string            `json:"timezone,omitempty"`
	RateLimitMax             int               `json:"rate_limit_max,omitempty"`
	RateLimitWindowSecs      int               `json:"rate_limit_window_secs,omitempty"`
	RateLimitKeys            []RateLimitKey    `json:"rate_limit_keys,omitempty"`
	DedupWindowSecs          int               `json:"dedup_window_secs,omitempty"`
	Enabled                  bool              `json:"enabled"`
	WebhookURL               string            `json:"webhook_url,omitempty"`
	WebhookSecret            string            `json:"webhook_secret,omitempty"`
	RunTTLSecs               int               `json:"run_ttl_secs,omitempty"`
	RetryStrategy            string            `json:"retry_strategy,omitempty"`
	RetryDelaysSecs          []int             `json:"retry_delays_secs,omitempty"`
	RetryPriorityBoost       int               `json:"retry_priority_boost,omitempty"`
	DLQAlertThreshold        *int              `json:"dlq_alert_threshold,omitempty"`
	QueueDepthAlertThreshold *int              `json:"queue_depth_alert_threshold,omitempty"`
	EnvironmentID            string            `json:"environment_id,omitempty"`
	DefaultRunMetadata       map[string]string `json:"default_run_metadata,omitempty"`
	Version                  int               `json:"version"`
	VersionID                string            `json:"version_id,omitempty"`
	VersionPolicy            VersionPolicy     `json:"version_policy,omitempty"`
	BackwardsCompatible      bool              `json:"backwards_compatible,omitempty"`
	SkipIfRunning            bool              `json:"skip_if_running,omitempty"`
	ResultSchema             json.RawMessage   `json:"result_schema,omitempty"`
	DebounceWindowSecs       int               `json:"debounce_window_secs,omitempty"`
	BatchWindowSecs          int               `json:"batch_window_secs,omitempty"`
	BatchMaxSize             int               `json:"batch_max_size,omitempty"`
	CreatedBy                string            `json:"created_by,omitempty"`
	UpdatedBy                string            `json:"updated_by,omitempty"`
	CreatedAt                time.Time         `json:"created_at"`
	UpdatedAt                time.Time         `json:"updated_at"`
}

// DebouncePending represents a pending debounced trigger waiting to fire.
type DebouncePending struct {
	ID             string          `json:"id"`
	JobID          string          `json:"job_id"`
	ProjectID      string          `json:"project_id"`
	DebounceKey    string          `json:"debounce_key"`
	Payload        json.RawMessage `json:"payload,omitempty"`
	Tags           json.RawMessage `json:"tags,omitempty"`
	Priority       int             `json:"priority"`
	ConcurrencyKey string          `json:"concurrency_key,omitempty"`
	TTLSecs        *int            `json:"ttl_secs,omitempty"`
	TriggeredBy    string          `json:"triggered_by"`
	CreatedBy      string          `json:"created_by,omitempty"`
	FireAt         time.Time       `json:"fire_at"`
	CreatedAt      time.Time       `json:"created_at"`
}

// BatchBufferItem represents a single trigger payload buffered for batch processing.
type BatchBufferItem struct {
	ID          string          `json:"id"`
	JobID       string          `json:"job_id"`
	ProjectID   string          `json:"project_id"`
	BatchKey    string          `json:"batch_key"`
	Payload     json.RawMessage `json:"payload"`
	Tags        json.RawMessage `json:"tags,omitempty"`
	Priority    int             `json:"priority"`
	TriggeredBy string          `json:"triggered_by"`
	CreatedBy   string          `json:"created_by,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// RunState represents a mutable key-value entry scoped to a run.
type RunState struct {
	RunID     string          `json:"run_id"`
	StateKey  string          `json:"state_key"`
	Value     json.RawMessage `json:"value"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type JobGroup struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Environment struct {
	ID        string            `json:"id"`
	ProjectID string            `json:"project_id"`
	Name      string            `json:"name"`
	Slug      string            `json:"slug"`
	ParentID  string            `json:"parent_id,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type JobDependency struct {
	ID             string    `json:"id"`
	JobID          string    `json:"job_id"`
	DependsOnJobID string    `json:"depends_on_job_id"`
	Condition      string    `json:"condition"`
	CreatedAt      time.Time `json:"created_at"`
}

type JobSecret struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"project_id"`
	JobID          string    `json:"job_id,omitempty"`
	Environment    string    `json:"environment"`
	SecretKey      string    `json:"secret_key"`
	EncryptedValue string    `json:"-"`
	KeyVersion     int       `json:"key_version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type JobRun struct {
	ID                    string            `json:"id"`
	JobID                 string            `json:"job_id"`
	ProjectID             string            `json:"project_id"`
	Tags                  map[string]string `json:"tags,omitempty"`
	Status                RunStatus         `json:"status"`
	Attempt               int               `json:"attempt"`
	Payload               json.RawMessage   `json:"payload,omitempty"`
	Result                json.RawMessage   `json:"result,omitempty"`
	Metadata              map[string]string `json:"metadata,omitempty"`
	Error                 string            `json:"error,omitempty"`
	TriggeredBy           string            `json:"triggered_by"`
	ScheduledAt           *time.Time        `json:"scheduled_at,omitempty"`
	StartedAt             *time.Time        `json:"started_at,omitempty"`
	FinishedAt            *time.Time        `json:"finished_at,omitempty"`
	HeartbeatAt           *time.Time        `json:"heartbeat_at,omitempty"`
	NextRetryAt           *time.Time        `json:"next_retry_at,omitempty"`
	ExpiresAt             *time.Time        `json:"expires_at,omitempty"`
	ParentRunID           string            `json:"parent_run_id,omitempty"`
	Priority              int               `json:"priority"`
	IdempotencyKey        string            `json:"idempotency_key,omitempty"`
	JobVersion            int               `json:"job_version"`
	JobVersionID          string            `json:"job_version_id,omitempty"`
	WorkflowStepRunID     string            `json:"workflow_step_run_id,omitempty"`
	MaxAttemptsOverride   int               `json:"max_attempts_override,omitempty"`
	TimeoutSecsOverride   int               `json:"timeout_secs_override,omitempty"`
	RetryBackoff          string            `json:"retry_backoff,omitempty"`
	RetryInitialDelaySecs int               `json:"retry_initial_delay_secs,omitempty"`
	RetryMaxDelaySecs     int               `json:"retry_max_delay_secs,omitempty"`
	ExecutionTrace        *ExecutionTrace   `json:"execution_trace,omitempty"`
	DebugMode             bool              `json:"debug_mode"`
	ContinuationOf        string            `json:"continuation_of,omitempty"`
	LineageDepth          int               `json:"lineage_depth"`
	CreatedBy             string            `json:"created_by,omitempty"`
	BatchID               string            `json:"batch_id,omitempty"`
	ConcurrencyKey        string            `json:"concurrency_key,omitempty"`
	CreatedAt             time.Time         `json:"created_at"`
}

type BatchOperation struct {
	ID           string     `json:"id"`
	ProjectID    string     `json:"project_id"`
	JobID        string     `json:"job_id"`
	ItemCount    int        `json:"item_count"`
	CreatedCount int        `json:"created_count"`
	CreatedBy    string     `json:"created_by,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

type RateLimitKey struct {
	Name       string `json:"name"`
	Max        int    `json:"max"`
	WindowSecs int    `json:"window_secs"`
}

type LogDrain struct {
	ID          string            `json:"id"`
	ProjectID   string            `json:"project_id"`
	Name        string            `json:"name"`
	DrainType   string            `json:"drain_type"`
	EndpointURL string            `json:"endpoint_url"`
	AuthType    string            `json:"auth_type"`
	AuthConfig  map[string]string `json:"auth_config,omitempty"`
	LevelFilter []string          `json:"level_filter,omitempty"`
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type EventSource struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
	Enabled     bool            `json:"enabled"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type EventSubscription struct {
	ID         string          `json:"id"`
	SourceID   string          `json:"source_id"`
	TargetType string          `json:"target_type"`
	TargetID   string          `json:"target_id"`
	FilterExpr json.RawMessage `json:"filter_expr,omitempty"`
	Enabled    bool            `json:"enabled"`
	CreatedAt  time.Time       `json:"created_at"`
}

type RunEvent struct {
	ID        string          `json:"id"`
	RunID     string          `json:"run_id"`
	Type      EventType       `json:"type"`
	Level     string          `json:"level,omitempty"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type RunCheckpoint struct {
	ID        string          `json:"id"`
	RunID     string          `json:"run_id"`
	Sequence  int             `json:"sequence"`
	Source    string          `json:"source"`
	State     json.RawMessage `json:"state"`
	CreatedAt time.Time       `json:"created_at"`
}

type RunUsage struct {
	ID               string    `json:"id"`
	RunID            string    `json:"run_id"`
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	CostMicrousd     int64     `json:"cost_microusd"`
	CreatedAt        time.Time `json:"created_at"`
}

type RunToolCall struct {
	ID         string          `json:"id"`
	RunID      string          `json:"run_id"`
	ToolName   string          `json:"tool_name"`
	Input      json.RawMessage `json:"input,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	DurationMs int             `json:"duration_ms,omitempty"`
	Status     string          `json:"status"`
	CreatedAt  time.Time       `json:"created_at"`
}

type RunOutput struct {
	ID        string          `json:"id"`
	RunID     string          `json:"run_id"`
	OutputKey string          `json:"output_key"`
	Schema    json.RawMessage `json:"schema,omitempty"`
	Value     json.RawMessage `json:"value"`
	CreatedAt time.Time       `json:"created_at"`
}

// ExecutionTrace captures timing breakdown for a job run execution.
type ExecutionTrace struct {
	QueueWaitMs int64 `json:"queue_wait_ms"` // time from created_at to dequeue
	DequeueMs   int64 `json:"dequeue_ms"`    // time in dequeue operation
	ConnectMs   int64 `json:"connect_ms"`    // TCP + TLS connection time
	TtfbMs      int64 `json:"ttfb_ms"`       // time to first byte (after connect)
	TransferMs  int64 `json:"transfer_ms"`   // response body transfer time
	TotalMs     int64 `json:"total_ms"`      // total wall time from dequeue to terminal
	DispatchMs  int64 `json:"dispatch_ms"`   // HTTP roundtrip time (connect + ttfb + transfer)
}

// DebugBundle aggregates all debug data for a run.
type DebugBundle struct {
	Run         *JobRun         `json:"run"`
	Events      []RunEvent      `json:"events"`
	Checkpoints []RunCheckpoint `json:"checkpoints"`
	Usage       []RunUsage      `json:"usage"`
	ToolCalls   []RunToolCall   `json:"tool_calls"`
	Outputs     []RunOutput     `json:"outputs"`
}

type CircuitState string

const (
	CircuitStateClosed   CircuitState = "closed"
	CircuitStateOpen     CircuitState = "open"
	CircuitStateHalfOpen CircuitState = "half_open"
)

type EndpointCircuitState struct {
	EndpointURL         string       `json:"endpoint_url"`
	State               CircuitState `json:"state"`
	ConsecutiveFailures int          `json:"consecutive_failures"`
	OpenedAt            *time.Time   `json:"opened_at,omitempty"`
	HalfOpenUntil       *time.Time   `json:"half_open_until,omitempty"`
	UpdatedAt           time.Time    `json:"updated_at"`
	CreatedAt           time.Time    `json:"created_at"`
}

type WebhookDelivery struct {
	ID             string     `json:"id"`
	RunID          string     `json:"run_id,omitempty"`
	JobID          string     `json:"job_id,omitempty"`
	EventTriggerID string     `json:"event_trigger_id,omitempty"`
	WebhookURL     string     `json:"webhook_url"`
	RetryPolicy    string     `json:"webhook_retry_policy,omitempty"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	MaxAttempts    int        `json:"max_attempts"`
	LastStatusCode *int       `json:"last_status_code,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
	NextRetryAt    *time.Time `json:"next_retry_at,omitempty"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type WebhookSubscription struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	WebhookURL string    `json:"webhook_url"`
	EventTypes []string  `json:"event_types"`
	Secret     string    `json:"secret"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at"`
}

// APIKey represents a per-project API key for authentication.
type APIKey struct {
	ID                  string     `json:"id"`
	ProjectID           string     `json:"project_id"`
	Name                string     `json:"name"`
	KeyHash             string     `json:"-"`
	KeyPrefix           string     `json:"key_prefix"`
	Scopes              []string   `json:"scopes"`
	ExpiresAt           *time.Time `json:"expires_at,omitempty"`
	LastUsedAt          *time.Time `json:"last_used_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	RevokedAt           *time.Time `json:"revoked_at,omitempty"`
	ReplacedByKeyID     string     `json:"replaced_by_key_id,omitempty"`
	GraceExpiresAt      *time.Time `json:"grace_expires_at,omitempty"`
	RateLimitRequests   int        `json:"rate_limit_requests,omitempty"`
	RateLimitWindowSecs int        `json:"rate_limit_window_secs,omitempty"`
}

type JobVersion struct {
	ID                  string            `json:"id"`
	JobID               string            `json:"job_id"`
	Version             int               `json:"version"`
	VersionID           string            `json:"version_id,omitempty"`
	BackwardsCompatible bool              `json:"backwards_compatible,omitempty"`
	Name                string            `json:"name"`
	Slug                string            `json:"slug"`
	Description         string            `json:"description,omitempty"`
	Cron                string            `json:"cron,omitempty"`
	PayloadSchema       json.RawMessage   `json:"payload_schema,omitempty"`
	Tags                map[string]string `json:"tags,omitempty"`
	EndpointURL         string            `json:"endpoint_url"`
	FallbackEndpointURL string            `json:"fallback_endpoint_url,omitempty"`
	MaxAttempts         int               `json:"max_attempts"`
	TimeoutSecs         int               `json:"timeout_secs"`
	WebhookURL          string            `json:"webhook_url,omitempty"`
	WebhookSecret       string            `json:"webhook_secret,omitempty"`
	RunTTLSecs          int               `json:"run_ttl_secs,omitempty"`
	CreatedAt           time.Time         `json:"created_at"`
}

// WorkflowVersion is a point-in-time snapshot of a workflow.
type WorkflowVersion struct {
	ID                string    `json:"id"`
	WorkflowID        string    `json:"workflow_id"`
	Version           int       `json:"version"`
	ProjectID         string    `json:"project_id"`
	Name              string    `json:"name"`
	Slug              string    `json:"slug"`
	Description       string    `json:"description,omitempty"`
	Enabled           bool      `json:"enabled"`
	TimeoutSecs       int       `json:"timeout_secs"`
	MaxConcurrentRuns int       `json:"max_concurrent_runs"`
	MaxParallelSteps  int       `json:"max_parallel_steps"`
	Cron              string    `json:"cron,omitempty"`
	CronTimezone      string    `json:"cron_timezone,omitempty"`
	SkipIfRunning     bool      `json:"skip_if_running,omitempty"`
	VersionID         string    `json:"version_id,omitempty"`
	CreatedBy         string    `json:"created_by,omitempty"`
	UpdatedBy         string    `json:"updated_by,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type WorkflowPolicy struct {
	ID                       string    `json:"id"`
	ProjectID                string    `json:"project_id"`
	MaxFanOut                int       `json:"max_fan_out"`
	MaxDepth                 int       `json:"max_depth"`
	ForbiddenStepTypes       []string  `json:"forbidden_step_types,omitempty"`
	RequireApprovalForDeploy bool      `json:"require_approval_for_deploy"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

func (s RunStatus) IsTerminal() bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusTimedOut, StatusCrashed, StatusSystemFailed, StatusCanceled, StatusExpired:
		return true
	default:
		return false
	}
}

func (s RunStatus) IsValid() bool {
	switch s {
	case StatusDelayed, StatusQueued, StatusDequeued, StatusExecuting, StatusWaiting,
		StatusCompleted, StatusFailed, StatusTimedOut, StatusCrashed, StatusSystemFailed,
		StatusCanceled, StatusExpired, StatusDeadLetter, StatusReplayStaged:
		return true
	default:
		return false
	}
}

func TerminalStatuses() []RunStatus {
	return []RunStatus{
		StatusCompleted,
		StatusFailed,
		StatusTimedOut,
		StatusCrashed,
		StatusSystemFailed,
		StatusCanceled,
		StatusExpired,
	}
}

// WorkflowRunStatus represents the status of a workflow run.
type WorkflowRunStatus string

const (
	WfStatusPending   WorkflowRunStatus = "pending"
	WfStatusRunning   WorkflowRunStatus = "running"
	WfStatusPaused    WorkflowRunStatus = "paused"
	WfStatusCompleted WorkflowRunStatus = "completed"
	WfStatusFailed    WorkflowRunStatus = "failed"
	WfStatusTimedOut  WorkflowRunStatus = "timed_out"
	WfStatusCanceled  WorkflowRunStatus = "canceled"
)

func (s WorkflowRunStatus) IsTerminal() bool {
	switch s {
	case WfStatusCompleted, WfStatusFailed, WfStatusTimedOut, WfStatusCanceled:
		return true
	default:
		return false
	}
}

func (s WorkflowRunStatus) IsValid() bool {
	switch s {
	case WfStatusPending, WfStatusRunning, WfStatusPaused, WfStatusCompleted, WfStatusFailed, WfStatusTimedOut, WfStatusCanceled:
		return true
	default:
		return false
	}
}

// StepRunStatus represents the status of a workflow step run.
type StepRunStatus string

const (
	StepPending   StepRunStatus = "pending"
	StepWaiting   StepRunStatus = "waiting"
	StepRunning   StepRunStatus = "running"
	StepCompleted StepRunStatus = "completed"
	StepFailed    StepRunStatus = "failed"
	StepSkipped   StepRunStatus = "skipped"
	StepCanceled  StepRunStatus = "canceled"
)

func (s StepRunStatus) IsTerminal() bool {
	switch s {
	case StepCompleted, StepFailed, StepSkipped, StepCanceled:
		return true
	default:
		return false
	}
}

// FailurePolicy determines what happens when a workflow step fails.
type FailurePolicy string

const (
	FailWorkflow   FailurePolicy = "fail_workflow"
	SkipDependents FailurePolicy = "skip_dependents"
	Continue       FailurePolicy = "continue"
)

type WorkflowStepType string

const (
	WorkflowStepTypeJob          WorkflowStepType = "job"
	WorkflowStepTypeApproval     WorkflowStepType = "approval"
	WorkflowStepTypeSubWorkflow  WorkflowStepType = "sub_workflow"
	WorkflowStepTypeWaitForEvent WorkflowStepType = "wait_for_event"
	WorkflowStepTypeSleep        WorkflowStepType = "sleep"
)

// ApprovalStatus constants for workflow step approvals.
const (
	ApprovalStatusPending  = "pending"
	ApprovalStatusApproved = "approved"
	ApprovalStatusRejected = "rejected"
)

// EventTriggerStatus constants for event triggers.
const (
	EventTriggerStatusWaiting  = "waiting"
	EventTriggerStatusReceived = "received"
	EventTriggerStatusTimedOut = "timed_out"
	EventTriggerStatusCanceled = "canceled"
)

// Event trigger source types.
const (
	EventSourceWorkflowStep = "workflow_step"
	EventSourceJobRun       = "job_run"
)

// Trigger type constants.
const (
	TriggerTypeEvent = "event"
	TriggerTypeSleep = "sleep"
)

// DefaultEventTimeoutSecs is the default timeout for wait_for_event steps (1 hour).
const DefaultEventTimeoutSecs = 3600

const (
	WebhookStatusPending   = "pending"
	WebhookStatusDelivered = "delivered"
	WebhookStatusFailed    = "failed"
	WebhookStatusDead      = "dead"
)

const (
	WebhookRetryPolicyExponential = "exponential"
	WebhookRetryPolicyLinear      = "linear"
	WebhookRetryPolicyFixed       = "fixed"
)

// VersionPolicy controls how queued runs handle new job/workflow deployments.
type VersionPolicy string

const (
	VersionPolicyPin    VersionPolicy = "pin"
	VersionPolicyLatest VersionPolicy = "latest"
	VersionPolicyMinor  VersionPolicy = "minor"
)

func (p VersionPolicy) IsValid() bool {
	switch p {
	case VersionPolicyPin, VersionPolicyLatest, VersionPolicyMinor:
		return true
	default:
		return false
	}
}

type RetryBackoffPolicy string

const (
	RetryBackoffExponential RetryBackoffPolicy = "exponential"
	RetryBackoffFixed       RetryBackoffPolicy = "fixed"
)

// StepOverride allows selectively enabling or disabling steps at trigger time.
type StepOverride struct {
	StepRef string `json:"step_ref"`
	Enabled bool   `json:"enabled"`
}

// Workflow represents a workflow DAG definition.
type Workflow struct {
	ID                  string            `json:"id"`
	ProjectID           string            `json:"project_id"`
	Name                string            `json:"name"`
	Slug                string            `json:"slug"`
	Description         string            `json:"description,omitempty"`
	Tags                map[string]string `json:"tags,omitempty"`
	Enabled             bool              `json:"enabled"`
	Version             int               `json:"version"`
	TimeoutSecs         int               `json:"timeout_secs,omitempty"`
	MaxConcurrentRuns   int               `json:"max_concurrent_runs,omitempty"`
	MaxParallelSteps    int               `json:"max_parallel_steps,omitempty"`
	Cron                string            `json:"cron,omitempty"`
	CronTimezone        string            `json:"cron_timezone,omitempty"`
	SkipIfRunning       bool              `json:"skip_if_running,omitempty"`
	VersionID           string            `json:"version_id,omitempty"`
	VersionPolicy       VersionPolicy     `json:"version_policy,omitempty"`
	BackwardsCompatible bool              `json:"backwards_compatible,omitempty"`
	CreatedBy           string            `json:"created_by,omitempty"`
	UpdatedBy           string            `json:"updated_by,omitempty"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
}

// WorkflowStep represents a step (node) within a workflow DAG.
type WorkflowStep struct {
	ID                    string             `json:"id"`
	WorkflowID            string             `json:"workflow_id"`
	JobID                 string             `json:"job_id,omitempty"`
	StepRef               string             `json:"step_ref"`
	DependsOn             []string           `json:"depends_on"`
	Condition             json.RawMessage    `json:"condition,omitempty"`
	OnFailure             FailurePolicy      `json:"on_failure"`
	Payload               json.RawMessage    `json:"payload,omitempty"`
	StepType              WorkflowStepType   `json:"step_type,omitempty"`
	ApprovalTimeoutSecs   int                `json:"approval_timeout_secs,omitempty"`
	ApprovalApprovers     []string           `json:"approval_approvers,omitempty"`
	RetryMaxAttempts      int                `json:"retry_max_attempts,omitempty"`
	RetryBackoff          RetryBackoffPolicy `json:"retry_backoff,omitempty"`
	RetryInitialDelaySecs int                `json:"retry_initial_delay_secs,omitempty"`
	RetryMaxDelaySecs     int                `json:"retry_max_delay_secs,omitempty"`
	TimeoutSecsOverride   int                `json:"timeout_secs_override,omitempty"`
	OutputTransform       string             `json:"output_transform,omitempty"`
	SubWorkflowID         string             `json:"sub_workflow_id,omitempty"`
	MaxNestingDepth       int                `json:"max_nesting_depth,omitempty"`
	EventKey              string             `json:"event_key,omitempty"`
	EventTimeoutSecs      int                `json:"event_timeout_secs,omitempty"`
	EventNotifyURL        string             `json:"event_notify_url,omitempty"`
	SleepDurationSecs     int                `json:"sleep_duration_secs,omitempty"`
	EventEmitKey          string             `json:"event_emit_key,omitempty"` // auto-send event on step completion
	ConcurrencyKey        string             `json:"concurrency_key,omitempty"`
	ResourceClass         string             `json:"resource_class,omitempty"`
	CreatedAt             time.Time          `json:"created_at"`
}

// WorkflowRun represents an execution instance of a workflow.
type WorkflowRun struct {
	ID                  string            `json:"id"`
	WorkflowID          string            `json:"workflow_id"`
	ProjectID           string            `json:"project_id"`
	Tags                map[string]string `json:"tags,omitempty"`
	Status              WorkflowRunStatus `json:"status"`
	TriggeredBy         string            `json:"triggered_by"`
	WorkflowVersion     int               `json:"workflow_version"`
	MaxParallelSteps    int               `json:"max_parallel_steps,omitempty"`
	Payload             json.RawMessage   `json:"payload,omitempty"`
	Error               string            `json:"error,omitempty"`
	StartedAt           *time.Time        `json:"started_at,omitempty"`
	FinishedAt          *time.Time        `json:"finished_at,omitempty"`
	ExpiresAt           *time.Time        `json:"expires_at,omitempty"`
	RetryOfRunID        string            `json:"retry_of_run_id,omitempty"`
	ParentWorkflowRunID string            `json:"parent_workflow_run_id,omitempty"`
	ParentStepRunID     string            `json:"parent_step_run_id,omitempty"`
	WorkflowVersionID   string            `json:"workflow_version_id,omitempty"`
	CreatedBy           string            `json:"created_by,omitempty"`
	TraceContext        map[string]string `json:"trace_context,omitempty"`
	CreatedAt           time.Time         `json:"created_at"`
}

// WorkflowStepRun represents execution of a single step within a workflow run.
type WorkflowStepRun struct {
	ID             string          `json:"id"`
	WorkflowRunID  string          `json:"workflow_run_id"`
	WorkflowStepID string          `json:"workflow_step_id"`
	StepRef        string          `json:"step_ref"`
	JobRunID       string          `json:"job_run_id,omitempty"`
	Attempt        int             `json:"attempt"`
	Status         StepRunStatus   `json:"status"`
	DepsCompleted  int             `json:"deps_completed"`
	DepsRequired   int             `json:"deps_required"`
	Output         json.RawMessage `json:"output,omitempty"`
	Error          string          `json:"error,omitempty"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	FinishedAt     *time.Time      `json:"finished_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

type WorkflowStepApproval struct {
	ID                string     `json:"id"`
	WorkflowRunID     string     `json:"workflow_run_id"`
	WorkflowStepRunID string     `json:"workflow_step_run_id"`
	Approvers         []string   `json:"approvers"`
	Status            string     `json:"status"`
	ApprovedBy        string     `json:"approved_by,omitempty"`
	RequestedAt       time.Time  `json:"requested_at"`
	ApprovedAt        *time.Time `json:"approved_at,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	Error             string     `json:"error,omitempty"`
}

type WorkflowStepDecision struct {
	ID            string          `json:"id"`
	WorkflowRunID string          `json:"workflow_run_id"`
	StepRunID     string          `json:"step_run_id"`
	StepRef       string          `json:"step_ref"`
	DecisionType  string          `json:"decision_type"`
	Decision      string          `json:"decision"`
	Explanation   string          `json:"explanation"`
	Details       json.RawMessage `json:"details,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// TimelineStep represents a single step in a Gantt-chart-friendly timeline view.
type TimelineStep struct {
	StepRunID      string     `json:"step_run_id"`
	StepRef        string     `json:"step_ref"`
	Status         string     `json:"status"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	DurationMs     int64      `json:"duration_ms"`
	ParallelWith   []string   `json:"parallel_with,omitempty"`
	OnCriticalPath bool       `json:"on_critical_path"`
	WaitMs         int64      `json:"wait_ms"`
}

// TimelineResponse is the response for the workflow run timeline endpoint.
type TimelineResponse struct {
	WorkflowRunID string         `json:"workflow_run_id"`
	Status        string         `json:"status"`
	StartedAt     *time.Time     `json:"started_at,omitempty"`
	FinishedAt    *time.Time     `json:"finished_at,omitempty"`
	TotalMs       int64          `json:"total_ms"`
	Steps         []TimelineStep `json:"steps"`
}

// EventTrigger represents a durable wait for an external event signal.
// Used by wait_for_event workflow steps and SDK wait-for-event on job runs.
type EventTrigger struct {
	ID                string          `json:"id"`
	EventKey          string          `json:"event_key"`
	ProjectID         string          `json:"project_id"`
	SourceType        string          `json:"source_type"`                    // "workflow_step" or "job_run"
	WorkflowRunID     string          `json:"workflow_run_id,omitempty"`      // set if source_type = workflow_step
	WorkflowStepRunID string          `json:"workflow_step_run_id,omitempty"` // set if source_type = workflow_step
	JobRunID          string          `json:"job_run_id,omitempty"`           // set if source_type = job_run
	Status            string          `json:"status"`                         // waiting, received, timed_out, canceled
	RequestPayload    json.RawMessage `json:"request_payload,omitempty"`
	ResponsePayload   json.RawMessage `json:"response_payload,omitempty"`
	TimeoutSecs       int             `json:"timeout_secs"`
	RequestedAt       time.Time       `json:"requested_at"`
	ReceivedAt        *time.Time      `json:"received_at,omitempty"`
	ExpiresAt         time.Time       `json:"expires_at"`
	Error             string          `json:"error,omitempty"`
	NotifyURL         string          `json:"notify_url,omitempty"`    // optional webhook URL to call on creation
	NotifyStatus      string          `json:"notify_status,omitempty"` // pending, sent, failed
	TriggerType       string          `json:"trigger_type,omitempty"`  // "event" (default) or "sleep"
	SentBy            string          `json:"sent_by,omitempty"`       // who resolved the trigger (API key ID, "internal", or "auto-emit")
}
