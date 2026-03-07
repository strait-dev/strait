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
	TriggerRetry    = "retry"
)

type EventType string

const (
	EventLog         EventType = "log"
	EventStateChange EventType = "state_change"
	EventError       EventType = "error"
	EventProgress    EventType = "progress"
)

type Job struct {
	ID            string          `json:"id"`
	ProjectID     string          `json:"project_id"`
	Name          string          `json:"name"`
	Slug          string          `json:"slug"`
	Description   string          `json:"description,omitempty"`
	Cron          string          `json:"cron,omitempty"`
	PayloadSchema json.RawMessage `json:"payload_schema,omitempty"`
	EndpointURL   string          `json:"endpoint_url"`
	MaxAttempts   int             `json:"max_attempts"`
	TimeoutSecs   int             `json:"timeout_secs"`
	Enabled       bool            `json:"enabled"`
	WebhookURL    string          `json:"webhook_url,omitempty"`
	WebhookSecret string          `json:"webhook_secret,omitempty"`
	RunTTLSecs    int             `json:"run_ttl_secs,omitempty"`
	Version       int             `json:"version"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type JobRun struct {
	ID                    string          `json:"id"`
	JobID                 string          `json:"job_id"`
	ProjectID             string          `json:"project_id"`
	Status                RunStatus       `json:"status"`
	Attempt               int             `json:"attempt"`
	Payload               json.RawMessage `json:"payload,omitempty"`
	Result                json.RawMessage `json:"result,omitempty"`
	Error                 string          `json:"error,omitempty"`
	TriggeredBy           string          `json:"triggered_by"`
	ScheduledAt           *time.Time      `json:"scheduled_at,omitempty"`
	StartedAt             *time.Time      `json:"started_at,omitempty"`
	FinishedAt            *time.Time      `json:"finished_at,omitempty"`
	HeartbeatAt           *time.Time      `json:"heartbeat_at,omitempty"`
	NextRetryAt           *time.Time      `json:"next_retry_at,omitempty"`
	ExpiresAt             *time.Time      `json:"expires_at,omitempty"`
	ParentRunID           string          `json:"parent_run_id,omitempty"`
	Priority              int             `json:"priority"`
	IdempotencyKey        string          `json:"idempotency_key,omitempty"`
	JobVersion            int             `json:"job_version"`
	WorkflowStepRunID     string          `json:"workflow_step_run_id,omitempty"`
	MaxAttemptsOverride   int             `json:"max_attempts_override,omitempty"`
	TimeoutSecsOverride   int             `json:"timeout_secs_override,omitempty"`
	RetryBackoff          string          `json:"retry_backoff,omitempty"`
	RetryInitialDelaySecs int             `json:"retry_initial_delay_secs,omitempty"`
	RetryMaxDelaySecs     int             `json:"retry_max_delay_secs,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
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

func (s RunStatus) IsValid() bool {
	switch s {
	case StatusDelayed, StatusQueued, StatusDequeued, StatusExecuting, StatusWaiting,
		StatusCompleted, StatusFailed, StatusTimedOut, StatusCrashed, StatusSystemFailed,
		StatusCanceled, StatusExpired:
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
	WorkflowStepTypeJob         WorkflowStepType = "job"
	WorkflowStepTypeApproval    WorkflowStepType = "approval"
	WorkflowStepTypeSubWorkflow WorkflowStepType = "sub_workflow"
)

// ApprovalStatus constants for workflow step approvals.
const (
	ApprovalStatusPending  = "pending"
	ApprovalStatusApproved = "approved"
	ApprovalStatusRejected = "rejected"
)

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
	ID                string    `json:"id"`
	ProjectID         string    `json:"project_id"`
	Name              string    `json:"name"`
	Slug              string    `json:"slug"`
	Description       string    `json:"description,omitempty"`
	Enabled           bool      `json:"enabled"`
	Version           int       `json:"version"`
	TimeoutSecs       int       `json:"timeout_secs,omitempty"`
	MaxConcurrentRuns int       `json:"max_concurrent_runs,omitempty"`
	MaxParallelSteps  int       `json:"max_parallel_steps,omitempty"`
	Cron              string    `json:"cron,omitempty"`
	CronTimezone      string    `json:"cron_timezone,omitempty"`
	SkipIfRunning     bool      `json:"skip_if_running,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
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
	CreatedAt             time.Time          `json:"created_at"`
}

// WorkflowRun represents an execution instance of a workflow.
type WorkflowRun struct {
	ID                  string            `json:"id"`
	WorkflowID          string            `json:"workflow_id"`
	ProjectID           string            `json:"project_id"`
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
