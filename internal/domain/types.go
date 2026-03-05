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
)

const (
	TriggerManual   = "manual"
	TriggerCron     = "cron"
	TriggerSpawn    = "spawn"
	TriggerWorkflow = "workflow"
)

type EventType string

const (
	EventLog         EventType = "log"
	EventStateChange EventType = "state_change"
	EventError       EventType = "error"
	EventProgress    EventType = "progress"
)

type Job struct {
	ID                  string          `json:"id"`
	ProjectID           string          `json:"project_id"`
	Name                string          `json:"name"`
	Slug                string          `json:"slug"`
	Description         string          `json:"description,omitempty"`
	Cron                string          `json:"cron,omitempty"`
	PayloadSchema       json.RawMessage `json:"payload_schema,omitempty"`
	EndpointURL         string          `json:"endpoint_url"`
	MaxAttempts         int             `json:"max_attempts"`
	TimeoutSecs         int             `json:"timeout_secs"`
	MaxConcurrency      int             `json:"max_concurrency,omitempty"`
	ExecutionWindowCron string          `json:"execution_window_cron,omitempty"`
	Timezone            string          `json:"timezone,omitempty"`
	RateLimitMax        int             `json:"rate_limit_max,omitempty"`
	RateLimitWindowSecs int             `json:"rate_limit_window_secs,omitempty"`
	DedupWindowSecs     int             `json:"dedup_window_secs,omitempty"`
	Enabled             bool            `json:"enabled"`
	WebhookURL          string          `json:"webhook_url,omitempty"`
	WebhookSecret       string          `json:"webhook_secret,omitempty"`
	RunTTLSecs          int             `json:"run_ttl_secs,omitempty"`
	Version             int             `json:"version"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

type JobRun struct {
	ID                string          `json:"id"`
	JobID             string          `json:"job_id"`
	ProjectID         string          `json:"project_id"`
	Status            RunStatus       `json:"status"`
	Attempt           int             `json:"attempt"`
	Payload           json.RawMessage `json:"payload,omitempty"`
	Result            json.RawMessage `json:"result,omitempty"`
	Error             string          `json:"error,omitempty"`
	TriggeredBy       string          `json:"triggered_by"`
	ScheduledAt       *time.Time      `json:"scheduled_at,omitempty"`
	StartedAt         *time.Time      `json:"started_at,omitempty"`
	FinishedAt        *time.Time      `json:"finished_at,omitempty"`
	HeartbeatAt       *time.Time      `json:"heartbeat_at,omitempty"`
	NextRetryAt       *time.Time      `json:"next_retry_at,omitempty"`
	ExpiresAt         *time.Time      `json:"expires_at,omitempty"`
	ParentRunID       string          `json:"parent_run_id,omitempty"`
	Priority          int             `json:"priority"`
	IdempotencyKey    string          `json:"idempotency_key,omitempty"`
	JobVersion        int             `json:"job_version"`
	WorkflowStepRunID string          `json:"workflow_step_run_id,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
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

type WebhookDelivery struct {
	ID             string     `json:"id"`
	RunID          string     `json:"run_id"`
	JobID          string     `json:"job_id"`
	WebhookURL     string     `json:"webhook_url"`
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

// APIKey represents a per-project API key for authentication.
type APIKey struct {
	ID         string     `json:"id"`
	ProjectID  string     `json:"project_id"`
	Name       string     `json:"name"`
	KeyHash    string     `json:"-"`
	KeyPrefix  string     `json:"key_prefix"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

type JobVersion struct {
	ID            string          `json:"id"`
	JobID         string          `json:"job_id"`
	Version       int             `json:"version"`
	Name          string          `json:"name"`
	Slug          string          `json:"slug"`
	Description   string          `json:"description,omitempty"`
	Cron          string          `json:"cron,omitempty"`
	PayloadSchema json.RawMessage `json:"payload_schema,omitempty"`
	EndpointURL   string          `json:"endpoint_url"`
	MaxAttempts   int             `json:"max_attempts"`
	TimeoutSecs   int             `json:"timeout_secs"`
	WebhookURL    string          `json:"webhook_url,omitempty"`
	WebhookSecret string          `json:"webhook_secret,omitempty"`
	RunTTLSecs    int             `json:"run_ttl_secs,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

func (s RunStatus) IsTerminal() bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusTimedOut, StatusCrashed, StatusSystemFailed, StatusCanceled, StatusExpired:
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
	WfStatusCompleted WorkflowRunStatus = "completed"
	WfStatusFailed    WorkflowRunStatus = "failed"
	WfStatusCanceled  WorkflowRunStatus = "canceled"
)

func (s WorkflowRunStatus) IsTerminal() bool {
	switch s {
	case WfStatusCompleted, WfStatusFailed, WfStatusCanceled:
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

// Workflow represents a workflow DAG definition.
type Workflow struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description,omitempty"`
	Enabled     bool      `json:"enabled"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// WorkflowStep represents a step (node) within a workflow DAG.
type WorkflowStep struct {
	ID         string          `json:"id"`
	WorkflowID string          `json:"workflow_id"`
	JobID      string          `json:"job_id"`
	StepRef    string          `json:"step_ref"`
	DependsOn  []string        `json:"depends_on"`
	Condition  json.RawMessage `json:"condition,omitempty"`
	OnFailure  FailurePolicy   `json:"on_failure"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// WorkflowRun represents an execution instance of a workflow.
type WorkflowRun struct {
	ID          string            `json:"id"`
	WorkflowID  string            `json:"workflow_id"`
	ProjectID   string            `json:"project_id"`
	Status      WorkflowRunStatus `json:"status"`
	TriggeredBy string            `json:"triggered_by"`
	Payload     json.RawMessage   `json:"payload,omitempty"`
	Error       string            `json:"error,omitempty"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	FinishedAt  *time.Time        `json:"finished_at,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

// WorkflowStepRun represents the execution of a single step within a workflow run.
type WorkflowStepRun struct {
	ID             string          `json:"id"`
	WorkflowRunID  string          `json:"workflow_run_id"`
	WorkflowStepID string          `json:"workflow_step_id"`
	StepRef        string          `json:"step_ref"`
	JobRunID       string          `json:"job_run_id,omitempty"`
	Status         StepRunStatus   `json:"status"`
	DepsCompleted  int             `json:"deps_completed"`
	DepsRequired   int             `json:"deps_required"`
	Output         json.RawMessage `json:"output,omitempty"`
	Error          string          `json:"error,omitempty"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	FinishedAt     *time.Time      `json:"finished_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}
