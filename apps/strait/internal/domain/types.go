package domain

import (
	"encoding/json"
	"time"
)

// RunStatus is the enum for job_runs.status.
//
//nolint:recvcheck // Scan requires a pointer receiver; Value uses value receiver by convention
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
	StatusPaused       RunStatus = "paused"
)

const (
	TriggerManual        = "manual"
	TriggerCron          = "cron"
	TriggerSpawn         = "spawn"
	TriggerWorkflow      = "workflow"
	TriggerRetry         = "retry"
	TriggerDebounce      = "debounce"
	TriggerJobCompletion = "job_completion"
	TriggerJobFailure    = "job_failure"
	TriggerJobChain      = "job_chain"
)

// MaxJobChainDepth is the maximum allowed lineage depth for job chaining
// to prevent infinite loops when jobs trigger each other.
const MaxJobChainDepth = 10

const (
	WebhookEventRunCompleted      = "run.completed"
	WebhookEventRunFailed         = "run.failed"
	WebhookEventRunTimedOut       = "run.timed_out"
	WebhookEventRunCanceled       = "run.canceled"
	WebhookEventWorkflowCompleted = "workflow.completed"
	WebhookEventWorkflowFailed    = "workflow.failed"
	WebhookEventSLOBudgetWarning  = "slo.budget_warning"

	// Outbound billing-state events. Dispatched via the same
	// webhook_subscriptions pipeline as run/workflow events, with HMAC
	// signing, retries, and the circuit breaker.
	WebhookEventBillingCapWarning            = "billing.cap_warning"
	WebhookEventBillingCapReached            = "billing.cap_reached"
	WebhookEventBillingCapDisabled           = "billing.cap_disabled"
	WebhookEventBillingOverageDisabled       = "billing.overage_disabled"
	WebhookEventBillingSuspended             = "billing.suspended"
	WebhookEventBillingDelinquent            = "billing.delinquent"
	WebhookEventBillingPaymentSucceeded      = "billing.payment_succeeded"
	WebhookEventScheduleSuspended            = "schedule.suspended"
	WebhookEventWorkflowRegistrationRejected = "workflow.registration_rejected"
	WebhookEventSLACreditIssued              = "sla.credit_issued"
)

const (
	ChannelTypeSlack   = "slack"
	ChannelTypeDiscord = "discord"
	ChannelTypeWebhook = "webhook"
	ChannelTypeEmail   = "email"
)

const (
	NotificationEventApprovalRequested    = "approval.requested"
	NotificationEventApprovalReminder     = "approval.reminder"
	NotificationEventApprovalExpired      = "approval.expired"
	NotificationEventApprovalCompleted    = "approval.completed"
	NotificationEventBudgetThreshold      = "budget.threshold_reached"
	NotificationEventCostAnomaly          = "cost.anomaly_detected"
	NotificationEventSpendingLimitWarning = "spending.limit_warning"
	NotificationEventSpendingLimitReached = "spending.limit_reached"
	NotificationEventRunLimitApproaching  = "runs.limit_approaching"
)

// NotificationChannel represents a configured notification destination for a project.
type NotificationChannel struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	ChannelType string          `json:"channel_type"`
	Name        string          `json:"name"`
	Config      json.RawMessage `json:"config"`
	Enabled     bool            `json:"enabled"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// NotificationDelivery tracks a single notification delivery attempt.
type NotificationDelivery struct {
	ID          string          `json:"id"`
	ChannelID   string          `json:"channel_id"`
	ProjectID   string          `json:"project_id"`
	EventType   string          `json:"event_type"`
	Payload     json.RawMessage `json:"payload"`
	Status      string          `json:"status"`
	Attempts    int             `json:"attempts"`
	MaxAttempts int             `json:"max_attempts"`
	LastError   string          `json:"last_error,omitempty"`
	NextRetryAt *time.Time      `json:"next_retry_at,omitempty"`
	DeliveredAt *time.Time      `json:"delivered_at,omitempty"`
	DedupeKey   string          `json:"-"`
	ClaimToken  string          `json:"-"`
	LeaseExpiry *time.Time      `json:"-"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// Error class constants for run error categorization.
const (
	ErrorClassRateLimited = "rate_limited"
	ErrorClassAuth        = "auth"
	ErrorClassClient      = "client"
	ErrorClassServer      = "server"
	ErrorClassTransient   = "transient"
	ErrorClassTimeout     = "timeout"
	ErrorClassOOM         = "oom"
	ErrorClassConnection  = "connection"
	ErrorClassBudget      = "budget"
	ErrorClassUnknown     = "unknown"
)

// ValidErrorClasses is the set of recognized error class values for API filtering.
var ValidErrorClasses = map[string]bool{
	ErrorClassRateLimited: true,
	ErrorClassAuth:        true,
	ErrorClassClient:      true,
	ErrorClassServer:      true,
	ErrorClassTransient:   true,
	ErrorClassTimeout:     true,
	ErrorClassOOM:         true,
	ErrorClassConnection:  true,
	ErrorClassBudget:      true,
	ErrorClassUnknown:     true,
}

type EventType string

const (
	EventLog         EventType = "log"
	EventStateChange EventType = "state_change"
	EventError       EventType = "error"
	EventProgress    EventType = "progress"
)

// Project represents a project in the Go service database.
type Project struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

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
		ScopeProjectsRead, ScopeProjectsWrite,
		ScopeDLQRead, ScopeDLQReplay,
		ScopeOutboxRead, ScopeOutboxRetry, ScopeOutboxPurge,
	},
	"viewer": {
		ScopeJobsRead, ScopeRunsRead, ScopeWorkflowsRead, ScopeStatsRead,
		ScopeProjectsRead, ScopeDLQRead,
	},
	"triggerer": {
		ScopeJobsRead, ScopeJobsTrigger,
		ScopeRunsRead,
		ScopeWorkflowsRead, ScopeWorkflowsTrigger,
		ScopeProjectsRead,
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
//
// SchemaVersion controls which canonical form is used to compute the HMAC
// signature. Version 1 includes only the original fields; version 2 extends
// the canonical form with RemoteIP, UserAgent, RequestID, TraceID, and
// SchemaVersion itself. The verifier branches on SchemaVersion when recomputing
// signatures so old and new events can coexist in the same chain.
type AuditEvent struct {
	ID            string          `json:"id"`
	ProjectID     string          `json:"project_id"`
	ActorID       string          `json:"actor_id"`
	ActorType     string          `json:"actor_type"`
	Action        string          `json:"action"`
	ResourceType  string          `json:"resource_type"`
	ResourceID    string          `json:"resource_id"`
	Details       json.RawMessage `json:"details,omitempty"`
	Signature     string          `json:"signature,omitempty" doc:"HMAC-SHA256 signature for tamper detection"`
	PreviousHash  string          `json:"previous_hash,omitempty" doc:"Hash of the preceding event in the chain"`
	CreatedAt     time.Time       `json:"created_at"`
	RemoteIP      string          `json:"remote_ip,omitempty" doc:"Client IP address that initiated the request"`
	UserAgent     string          `json:"user_agent,omitempty" doc:"HTTP User-Agent header (capped at 2048 chars)"`
	RequestID     string          `json:"request_id,omitempty" doc:"Unique request identifier for correlation"`
	TraceID       string          `json:"trace_id,omitempty" doc:"OpenTelemetry trace ID when available"`
	SchemaVersion uint16          `json:"schema_version,omitempty" doc:"Signature schema version (1=original, 2=with forensic fields)"`
	// IsAnchor marks the row as a chain-boundary anchor (e.g. signing key
	// rotation). Schema v3+ signs this value as part of the canonical HMAC
	// form so anchor markers cannot be toggled without detection.
	IsAnchor bool `json:"is_anchor,omitempty"`
	// RotationEpoch is the signing key epoch under which this row was
	// written. Epoch 0 is the initial key. Monotonically increasing per
	// project. Schema v3+ signs this value as part of the canonical HMAC form.
	RotationEpoch int `json:"rotation_epoch,omitempty"`
	// ShardID partitions the audit chain within a project. Legacy rows
	// (written before the shard column existed) carry '' and continue to
	// verify under the per-project chain. Non-empty values key a
	// per-(project, shard_id) sub-chain whose tail-read, signature, and
	// retention/rotation anchors are independent of other shards in the
	// same project. Schema v4+ binds shard_id into the canonical HMAC form.
	ShardID string `json:"shard_id,omitempty"`
}

// AuditEventSchemaVersionCurrent is the schema version stamped on new
// audit events. Bump whenever the canonical form changes.
const AuditEventSchemaVersionCurrent uint16 = 3

// AuditChainVerification is the result of verifying the HMAC chain
// integrity for a project's audit event log.
type AuditChainVerification struct {
	ProjectID     string `json:"project_id"`
	Valid         bool   `json:"valid"`
	EventsChecked int    `json:"events_checked"`
	FirstEventID  string `json:"first_event_id,omitempty"`
	LastEventID   string `json:"last_event_id,omitempty"`
	BrokenAtID    string `json:"broken_at_id,omitempty"`
	Error         string `json:"error,omitempty"`
	// ChainStart records the previous_hash of the first surviving event.
	// When retention has trimmed the tail, this is not the ZeroHash — it
	// is the signature of the (now deleted) event that preceded the first
	// surviving event. Consumers can record this to prove continuity
	// across retention windows.
	ChainStart string `json:"chain_start,omitempty"`
	// Incremental reports whether this verification was an incremental
	// re-check from a stored checkpoint or a full-chain scan from the
	// earliest surviving event. Clients can use this alongside
	// EventsChecked to distinguish a cheap re-check from an expensive
	// full verify.
	Incremental bool `json:"incremental,omitempty"`
}

type Job struct {
	ID                        string            `json:"id"`
	ProjectID                 string            `json:"project_id"`
	GroupID                   string            `json:"group_id,omitempty"`
	Name                      string            `json:"name"`
	Slug                      string            `json:"slug"`
	Description               string            `json:"description,omitempty"`
	Cron                      string            `json:"cron,omitempty"`
	PayloadSchema             json.RawMessage   `json:"payload_schema,omitempty"`
	Tags                      map[string]string `json:"tags,omitempty"`
	EndpointURL               string            `json:"endpoint_url"`
	FallbackEndpointURL       string            `json:"fallback_endpoint_url,omitempty"`
	MaxAttempts               int               `json:"max_attempts"`
	TimeoutSecs               int               `json:"timeout_secs"`
	MaxConcurrency            int               `json:"max_concurrency,omitempty"`
	MaxConcurrencyPerKey      int               `json:"max_concurrency_per_key,omitempty"`
	ExecutionWindowCron       string            `json:"execution_window_cron,omitempty"`
	Timezone                  string            `json:"timezone,omitempty"`
	RateLimitMax              int               `json:"rate_limit_max,omitempty"`
	RateLimitWindowSecs       int               `json:"rate_limit_window_secs,omitempty"`
	RateLimitKeys             []RateLimitKey    `json:"rate_limit_keys,omitempty"`
	DedupWindowSecs           int               `json:"dedup_window_secs,omitempty"`
	Enabled                   bool              `json:"enabled"`
	Paused                    bool              `json:"paused"`
	PausedAt                  *time.Time        `json:"paused_at,omitempty"`
	PauseReason               string            `json:"pause_reason,omitempty"`
	WebhookURL                string            `json:"webhook_url,omitempty"`
	WebhookSecret             string            `json:"-"`
	RunTTLSecs                int               `json:"run_ttl_secs,omitempty"`
	RetryStrategy             string            `json:"retry_strategy,omitempty"`
	RetryDelaysSecs           []int             `json:"retry_delays_secs,omitempty"`
	RetryPriorityBoost        int               `json:"retry_priority_boost,omitempty"`
	DLQAlertThreshold         *int              `json:"dlq_alert_threshold,omitempty"`
	QueueDepthAlertThreshold  *int              `json:"queue_depth_alert_threshold,omitempty"`
	PoisonPillThreshold       *int              `json:"poison_pill_threshold,omitempty"`
	EnvironmentID             string            `json:"environment_id,omitempty"`
	DefaultRunMetadata        map[string]string `json:"default_run_metadata,omitempty"`
	Version                   int               `json:"version"`
	VersionID                 string            `json:"version_id,omitempty"`
	VersionPolicy             VersionPolicy     `json:"version_policy,omitempty"`
	BackwardsCompatible       bool              `json:"backwards_compatible,omitempty"`
	CronOverlapPolicy         CronOverlapPolicy `json:"cron_overlap_policy,omitempty"`
	ResultSchema              json.RawMessage   `json:"result_schema,omitempty"`
	DebounceWindowSecs        int               `json:"debounce_window_secs,omitempty"`
	BatchWindowSecs           int               `json:"batch_window_secs,omitempty"`
	BatchMaxSize              int               `json:"batch_max_size,omitempty"`
	ExecutionMode             ExecutionMode     `json:"execution_mode,omitempty"`
	Queue                     string            `json:"queue,omitempty"`
	PreferredRegions          []string          `json:"-"`
	OnCompleteTriggerWorkflow string            `json:"on_complete_trigger_workflow,omitempty"`
	OnCompleteTriggerJob      string            `json:"on_complete_trigger_job,omitempty"`
	OnCompletePayloadMapping  json.RawMessage   `json:"on_complete_payload_mapping,omitempty"`
	OnFailureTriggerJob       string            `json:"on_failure_trigger_job,omitempty"`
	OnFailureTriggerWorkflow  string            `json:"on_failure_trigger_workflow,omitempty"`
	OnFailurePayloadMapping   json.RawMessage   `json:"on_failure_payload_mapping,omitempty"`
	EndpointSigningSecret     string            `json:"-"`
	CreatedBy                 string            `json:"created_by,omitempty"`
	UpdatedBy                 string            `json:"updated_by,omitempty"`
	CreatedAt                 time.Time         `json:"created_at"`
	UpdatedAt                 time.Time         `json:"updated_at"`
	CacheVersion              int64             `json:"-"`
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

// JobMemory represents a persistent key-value entry scoped to a job.
type JobMemory struct {
	ID           string          `json:"id"`
	JobID        string          `json:"job_id"`
	ProjectID    string          `json:"project_id"`
	MemoryKey    string          `json:"memory_key"`
	Value        json.RawMessage `json:"value"`
	SizeBytes    int             `json:"size_bytes"`
	TTLExpiresAt *time.Time      `json:"ttl_expires_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
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
	ID         string            `json:"id"`
	ProjectID  string            `json:"project_id"`
	Name       string            `json:"name"`
	Slug       string            `json:"slug"`
	ParentID   string            `json:"parent_id,omitempty"`
	IsStandard bool              `json:"is_standard"`
	Variables  map[string]string `json:"variables,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// StandardEnvironmentSlugs defines the legacy standard environment seed set.
// Launch project creation does not auto-provision these environments.
var StandardEnvironmentSlugs = []string{"development", "staging", "production"}

// StandardEnvironmentNames maps slugs to display names.
var StandardEnvironmentNames = map[string]string{
	"development": "Development",
	"staging":     "Staging",
	"production":  "Production",
}

type JobDependency struct {
	ID             string    `json:"id"`
	JobID          string    `json:"job_id"`
	DependsOnJobID string    `json:"depends_on_job_id"`
	Condition      string    `json:"condition"`
	CreatedAt      time.Time `json:"created_at"`
	CacheVersion   int64     `json:"-"`
}

type JobSecret struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"project_id"`
	JobID          string    `json:"job_id,omitempty"`
	Environment    string    `json:"environment"`
	SecretKey      string    `json:"secret_key"`
	EncryptedValue string    `json:"-"`
	Value          string    `json:"-"`
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
	ErrorClass            string            `json:"error_class,omitempty"`
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
	ExecutionMode         ExecutionMode     `json:"execution_mode,omitempty"`
	QueueName             string            `json:"queue_name,omitempty"`
	// IsRollback is retained for historical run records; always false for new runs.
	IsRollback bool `json:"is_rollback,omitempty"`
	// ReplayedRunID is set on a dead-letter run after it has been successfully
	// replayed via the admin DLQ endpoint; points to the new run that superseded
	// it. NULL for runs that have not been replayed.
	ReplayedRunID string    `json:"replayed_run_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	CacheVersion  int64     `json:"-"`
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
	ID                 string          `json:"id"`
	ProjectID          string          `json:"project_id"`
	Name               string          `json:"name"`
	Description        string          `json:"description,omitempty"`
	Schema             json.RawMessage `json:"schema,omitempty"`
	Enabled            bool            `json:"enabled"`
	SignatureHeader    string          `json:"signature_header,omitempty"`
	SignatureAlgorithm string          `json:"signature_algorithm,omitempty"`
	SignatureSecretEnc []byte          `json:"-"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
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

// RunIteration represents a single iteration of an agent run.
type RunIteration struct {
	ID          string    `json:"id"`
	RunID       string    `json:"run_id"`
	Iteration   int       `json:"iteration"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type RunOutput struct {
	ID        string          `json:"id"`
	RunID     string          `json:"run_id"`
	OutputKey string          `json:"output_key"`
	Schema    json.RawMessage `json:"schema,omitempty"`
	Value     json.RawMessage `json:"value"`
	CreatedAt time.Time       `json:"created_at"`
}

// RunResourceSnapshot records a point-in-time resource utilization sample for a run.
type RunResourceSnapshot struct {
	ID             string    `json:"id"`
	RunID          string    `json:"run_id"`
	CPUPercent     float64   `json:"cpu_percent"`
	MemoryMB       float64   `json:"memory_mb"`
	MemoryLimitMB  float64   `json:"memory_limit_mb"`
	NetworkRxBytes int64     `json:"network_rx_bytes"`
	NetworkTxBytes int64     `json:"network_tx_bytes"`
	CreatedAt      time.Time `json:"created_at"`
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
	Run               *JobRun               `json:"run"`
	Events            []RunEvent            `json:"events"`
	Checkpoints       []RunCheckpoint       `json:"checkpoints"`
	Outputs           []RunOutput           `json:"outputs"`
	ResourceSnapshots []RunResourceSnapshot `json:"resource_snapshots"`
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

// EndpointHealthScore tracks continuous health scoring for an endpoint.
// Score ranges from 0-100 where:
//   - Score > 60: healthy (full concurrency)
//   - Score 30-60: degraded (throttled concurrency)
//   - Score < 30: unhealthy (blocked, equivalent to circuit open)
type EndpointHealthScore struct {
	EndpointURL   string    `json:"endpoint_url"`
	HealthScore   float64   `json:"health_score"`
	SuccessRate   float64   `json:"success_rate"`
	TimeoutRate   float64   `json:"timeout_rate"`
	LatencyScore  float64   `json:"latency_score"`
	TotalRequests int64     `json:"total_requests"`
	LastLatencyMs float64   `json:"last_latency_ms"`
	UpdatedAt     time.Time `json:"updated_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// HealthLevel returns the health classification for the endpoint.
func (h *EndpointHealthScore) HealthLevel() string {
	if h.HealthScore < 30 {
		return "unhealthy"
	} else if h.HealthScore <= 60 {
		return "degraded"
	}
	return "healthy"
}

type WebhookDelivery struct {
	ID             string          `json:"id"`
	RunID          string          `json:"run_id,omitempty"`
	JobID          string          `json:"job_id,omitempty"`
	EventTriggerID string          `json:"event_trigger_id,omitempty"`
	SubscriptionID string          `json:"subscription_id,omitempty"`
	ProjectID      string          `json:"project_id,omitempty"`
	OrgID          string          `json:"org_id,omitempty"`
	WebhookURL     string          `json:"webhook_url"`
	WebhookSecret  string          `json:"-"`
	Payload        json.RawMessage `json:"-"`
	DedupeKey      string          `json:"-"`
	RetryPolicy    string          `json:"webhook_retry_policy,omitempty"`
	Status         string          `json:"status"`
	Attempts       int             `json:"attempts"`
	MaxAttempts    int             `json:"max_attempts"`
	LastStatusCode *int            `json:"last_status_code,omitempty"`
	LastError      string          `json:"last_error,omitempty"`
	NextRetryAt    *time.Time      `json:"next_retry_at,omitempty"`
	DeliveredAt    *time.Time      `json:"delivered_at,omitempty"`
	ClaimToken     string          `json:"-"`
	LeaseExpiresAt *time.Time      `json:"-"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type WebhookSubscription struct {
	ID                   string     `json:"id"`
	ProjectID            string     `json:"project_id"`
	WebhookURL           string     `json:"webhook_url"`
	EventTypes           []string   `json:"event_types"`
	Secret               string     `json:"-"`
	PreviousSecret       string     `json:"-"`
	SecretGraceExpiresAt *time.Time `json:"-"`
	Active               bool       `json:"active"`
	CreatedAt            time.Time  `json:"created_at"`
}

// APIKeyPrefixLen is the number of leading characters of a raw API key
// that we store as the public, non-secret prefix on every APIKey row. The
// prefix is what we surface in the UI and audit logs so an operator can
// recognise which key fired without revealing the secret. The value is the
// length of the literal "strait_" tag plus the first 5 hex characters of
// the random body — short enough to be unguessable, long enough to be
// visually distinguishable across thousands of keys per org.
//
// Every site that mints or stamps a key prefix must slice the raw key with
// this constant. Drift between mint sites (api, scheduler, testutil) would
// silently produce inconsistent prefixes that fail UI lookups.
const APIKeyPrefixLen = 12

// APIKey represents a per-project or org-scoped API key for authentication.
type APIKey struct {
	ID                    string     `json:"id"`
	ProjectID             string     `json:"project_id"`
	OrgID                 string     `json:"org_id,omitempty"`
	Name                  string     `json:"name"`
	KeyHash               string     `json:"-"`
	KeyPrefix             string     `json:"key_prefix"`
	Scopes                []string   `json:"scopes"`
	ExpiresAt             *time.Time `json:"expires_at,omitempty"`
	LastUsedAt            *time.Time `json:"last_used_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	RevokedAt             *time.Time `json:"revoked_at,omitempty"`
	ReplacedByKeyID       string     `json:"replaced_by_key_id,omitempty"`
	GraceExpiresAt        *time.Time `json:"grace_expires_at,omitempty"`
	RateLimitRequests     int        `json:"rate_limit_requests,omitempty"`
	RateLimitWindowSecs   int        `json:"rate_limit_window_secs,omitempty"`
	EnvironmentID         string     `json:"environment_id,omitempty"`
	RotationIntervalDays  *int       `json:"rotation_interval_days,omitempty"`
	NextRotationAt        *time.Time `json:"next_rotation_at,omitempty"`
	RotationWebhookURL    string     `json:"rotation_webhook_url,omitempty"`
	RotationWebhookSecret []byte     `json:"-"`
	CacheVersion          int64      `json:"-"`
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
	WebhookSecret       string            `json:"-"`
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

type DeploymentVersionStatus string

const (
	DeploymentVersionStatusDraft     DeploymentVersionStatus = "draft"
	DeploymentVersionStatusFinalized DeploymentVersionStatus = "finalized"
	DeploymentVersionStatusPromoted  DeploymentVersionStatus = "promoted"
)

func (s DeploymentVersionStatus) IsValid() bool {
	switch s {
	case DeploymentVersionStatusDraft, DeploymentVersionStatusFinalized, DeploymentVersionStatusPromoted:
		return true
	default:
		return false
	}
}

type DeploymentVersion struct {
	ID                     string                  `json:"id"`
	ProjectID              string                  `json:"project_id"`
	Environment            string                  `json:"environment"`
	Runtime                string                  `json:"runtime"`
	ArtifactURI            string                  `json:"artifact_uri"`
	Manifest               json.RawMessage         `json:"manifest,omitempty"`
	Checksum               string                  `json:"checksum,omitempty"`
	Status                 DeploymentVersionStatus `json:"status"`
	Strategy               DeploymentStrategy      `json:"strategy"`
	CanaryPercent          *int                    `json:"canary_percent,omitempty"`
	CanaryDuration         *time.Duration          `json:"canary_duration,omitempty"`
	FinalizedAt            *time.Time              `json:"finalized_at,omitempty"`
	PromotedAt             *time.Time              `json:"promoted_at,omitempty"`
	RollbackFromDeployment string                  `json:"rollback_from_deployment_id,omitempty"`
	CreatedBy              string                  `json:"created_by,omitempty"`
	UpdatedBy              string                  `json:"updated_by,omitempty"`
	CreatedAt              time.Time               `json:"created_at"`
	UpdatedAt              time.Time               `json:"updated_at"`
}

// IsTerminal reports whether the run is in a final state where no further
// state transitions or work are expected. Includes dead_letter — a
// permanently-failed run that has exhausted retries IS terminal from every
// callers' perspective (SSE handlers must hang up, webhooks/notifications
// must fire, the reaper must skip it, replay/idempotency must consider it
// already-resolved). Use IsDeadLetter when you need to distinguish DLQ
// runs from normally-completed ones.
func (s RunStatus) IsTerminal() bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusTimedOut, StatusCrashed, StatusSystemFailed, StatusCanceled, StatusExpired, StatusDeadLetter:
		return true
	default:
		return false
	}
}

func (s RunStatus) IsValid() bool {
	switch s {
	case StatusDelayed, StatusQueued, StatusDequeued, StatusExecuting, StatusWaiting,
		StatusCompleted, StatusFailed, StatusTimedOut, StatusCrashed, StatusSystemFailed,
		StatusCanceled, StatusExpired, StatusDeadLetter, StatusReplayStaged, StatusPaused:
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
		StatusDeadLetter,
	}
}

// WorkflowRunStatus represents the status of a workflow run.
type WorkflowRunStatus string

const (
	WfStatusPending            WorkflowRunStatus = "pending"
	WfStatusRunning            WorkflowRunStatus = "running"
	WfStatusPaused             WorkflowRunStatus = "paused"
	WfStatusCompleted          WorkflowRunStatus = "completed"
	WfStatusFailed             WorkflowRunStatus = "failed"
	WfStatusTimedOut           WorkflowRunStatus = "timed_out"
	WfStatusCanceled           WorkflowRunStatus = "canceled"
	WfStatusCompensating       WorkflowRunStatus = "compensating"
	WfStatusCompensated        WorkflowRunStatus = "compensated"
	WfStatusCompensationFailed WorkflowRunStatus = "compensation_failed"
)

func (s WorkflowRunStatus) IsTerminal() bool {
	switch s {
	case WfStatusCompleted, WfStatusFailed, WfStatusTimedOut, WfStatusCanceled,
		WfStatusCompensated, WfStatusCompensationFailed:
		return true
	default:
		return false
	}
}

func (s WorkflowRunStatus) IsValid() bool {
	switch s {
	case WfStatusPending, WfStatusRunning, WfStatusPaused, WfStatusCompleted, WfStatusFailed, WfStatusTimedOut, WfStatusCanceled,
		WfStatusCompensating, WfStatusCompensated, WfStatusCompensationFailed:
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

// DefaultSleepDurationSecs is the default duration for sleep steps (1 minute).
const DefaultSleepDurationSecs = 60

// MaxSleepDurationSecs is the upper bound enforced on sleep steps (30 days).
// Sleep steps create durable trigger rows; allowing unbounded durations lets a
// workflow definition pin rows and workflow runs for years.
const MaxSleepDurationSecs = 30 * 24 * 3600

// MaxEventTimeoutSecs is the upper bound enforced on wait_for_event step
// timeouts (30 days). Without a cap, a workflow author can pin a step in
// 'waiting' for ~68 years (INT_MAX seconds), creating a permanently
// occupied event_trigger row and an orphaned workflow_run that the reaper
// will never clean up — a slow-burn resource exhaustion vector.
const MaxEventTimeoutSecs = 30 * 24 * 3600

// SLO metric types.
const (
	SLOMetricSuccessRate    = "success_rate"
	SLOMetricP95LatencySecs = "p95_latency_secs"
	SLOMetricP99LatencySecs = "p99_latency_secs"
)

// JobSLO defines a service level objective for a job.
type JobSLO struct {
	ID          string    `json:"id"`
	JobID       string    `json:"job_id"`
	ProjectID   string    `json:"project_id"`
	Metric      string    `json:"metric"`
	Target      float64   `json:"target"`
	WindowHours int       `json:"window_hours"`
	CreatedAt   time.Time `json:"created_at"`
}

// JobSLOEvaluation records a point-in-time evaluation of an SLO.
type JobSLOEvaluation struct {
	ID              string    `json:"id"`
	SLOID           string    `json:"slo_id"`
	CurrentValue    float64   `json:"current_value"`
	BudgetRemaining float64   `json:"budget_remaining"`
	EvaluatedAt     time.Time `json:"evaluated_at"`
}

// JobSLOStatus combines an SLO with its latest evaluation.
type JobSLOStatus struct {
	JobSLO
	CurrentValue    *float64   `json:"current_value,omitempty"`
	BudgetRemaining *float64   `json:"budget_remaining,omitempty"`
	EvaluatedAt     *time.Time `json:"evaluated_at,omitempty"`
}

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

// ExecutionMode determines how a job run is dispatched.
type ExecutionMode string

const (
	ExecutionModeHTTP   ExecutionMode = "http"
	ExecutionModeWorker ExecutionMode = "worker"
)

// WorkerQueueRef identifies a connected worker queue scope. ProjectID is
// required so worker-mode dequeue never claims another tenant's queued runs.
// Empty EnvironmentID means the worker is project-wide for that queue.
type WorkerQueueRef struct {
	ProjectID     string
	QueueName     string
	EnvironmentID string
}

// WorkerQueueAvailability describes the worker queues that can accept runs
// on this replica and the total number of currently available worker slots.
type WorkerQueueAvailability struct {
	Queues         []WorkerQueueRef
	SlotsAvailable int
}

// IsValid returns true if the execution mode is a known value.
func (m ExecutionMode) IsValid() bool {
	switch m {
	case ExecutionModeHTTP, ExecutionModeWorker:
		return true
	default:
		return false
	}
}

// CronOverlapPolicy controls behavior when a cron tick fires while
// previous runs for the same job are still active.
type CronOverlapPolicy string

const (
	OverlapPolicyAllow         CronOverlapPolicy = "allow"
	OverlapPolicySkip          CronOverlapPolicy = "skip"
	OverlapPolicyCancelRunning CronOverlapPolicy = "cancel_running"
)

// IsValid returns true if the overlap policy is a known value.
func (p CronOverlapPolicy) IsValid() bool {
	switch p {
	case OverlapPolicyAllow, OverlapPolicySkip, OverlapPolicyCancelRunning:
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
	ID                        string             `json:"id"`
	WorkflowID                string             `json:"workflow_id"`
	JobID                     string             `json:"job_id,omitempty"`
	StepRef                   string             `json:"step_ref"`
	DependsOn                 []string           `json:"depends_on"`
	Condition                 json.RawMessage    `json:"condition,omitempty"`
	OnFailure                 FailurePolicy      `json:"on_failure"`
	Payload                   json.RawMessage    `json:"payload,omitempty"`
	StepType                  WorkflowStepType   `json:"step_type,omitempty"`
	ApprovalTimeoutSecs       int                `json:"approval_timeout_secs,omitempty"`
	ApprovalApprovers         []string           `json:"approval_approvers,omitempty"`
	RetryMaxAttempts          int                `json:"retry_max_attempts,omitempty"`
	RetryBackoff              RetryBackoffPolicy `json:"retry_backoff,omitempty"`
	RetryInitialDelaySecs     int                `json:"retry_initial_delay_secs,omitempty"`
	RetryMaxDelaySecs         int                `json:"retry_max_delay_secs,omitempty"`
	TimeoutSecsOverride       int                `json:"timeout_secs_override,omitempty"`
	OutputTransform           string             `json:"output_transform,omitempty"`
	SubWorkflowID             string             `json:"sub_workflow_id,omitempty"`
	MaxNestingDepth           int                `json:"max_nesting_depth,omitempty"`
	EventKey                  string             `json:"event_key,omitempty"`
	EventTimeoutSecs          int                `json:"event_timeout_secs,omitempty"`
	EventNotifyURL            string             `json:"event_notify_url,omitempty"`
	SleepDurationSecs         int                `json:"sleep_duration_secs,omitempty"`
	EventEmitKey              string             `json:"event_emit_key,omitempty"` // auto-send event on step completion
	ConcurrencyKey            string             `json:"concurrency_key,omitempty"`
	ResourceClass             string             `json:"resource_class,omitempty"`
	CostGateThresholdMicrousd int64              `json:"cost_gate_threshold_microusd,omitempty"`
	CostGateTimeoutSecs       int                `json:"cost_gate_timeout_secs,omitempty"`
	CostGateDefaultAction     string             `json:"cost_gate_default_action,omitempty"`
	ExpectedDurationSecs      int                `json:"expected_duration_secs,omitempty"`
	StageNotifications        json.RawMessage    `json:"stage_notifications,omitempty"`
	CompensationJobID         string             `json:"compensation_job_id,omitempty"`
	CompensationTimeoutSecs   int                `json:"compensation_timeout_secs,omitempty"`
	CreatedAt                 time.Time          `json:"created_at"`
}

// CompensationRun tracks a single compensation execution for a workflow step.
type CompensationRun struct {
	ID                string          `json:"id"`
	WorkflowRunID     string          `json:"workflow_run_id"`
	StepRunID         string          `json:"step_run_id"`
	StepRef           string          `json:"step_ref"`
	CompensationJobID string          `json:"compensation_job_id"`
	JobRunID          string          `json:"job_run_id,omitempty"`
	Status            string          `json:"status"` // pending, running, completed, failed
	Input             json.RawMessage `json:"input,omitempty"`
	Output            json.RawMessage `json:"output,omitempty"`
	Error             string          `json:"error,omitempty"`
	StartedAt         *time.Time      `json:"started_at,omitempty"`
	FinishedAt        *time.Time      `json:"finished_at,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
}

// CompensationRunStatus constants.
const (
	CompensationPending   = "pending"
	CompensationRunning   = "running"
	CompensationCompleted = "completed"
	CompensationFailed    = "failed"
)

// StageNotificationConfig defines notifications to send on step transitions.
type StageNotificationConfig struct {
	OnComplete bool `json:"on_complete,omitempty"`
	OnFailure  bool `json:"on_failure,omitempty"`
	OnSkipped  bool `json:"on_skipped,omitempty"`
}

// CanaryDeployment represents an active canary deployment between workflow versions.
type CanaryDeployment struct {
	ID            string          `json:"id"`
	WorkflowID    string          `json:"workflow_id"`
	ProjectID     string          `json:"project_id"`
	SourceVersion int             `json:"source_version"`
	TargetVersion int             `json:"target_version"`
	TrafficPct    int             `json:"traffic_pct"`
	Status        string          `json:"status"` // active, promoting, rolling_back, completed, rolled_back
	AutoPromote   json.RawMessage `json:"auto_promote_config,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
}

// JobCostEstimate holds the cached average cost for a job based on recent runs.
type JobCostEstimate struct {
	JobID           string    `json:"job_id"`
	AvgCostMicrousd int64     `json:"avg_cost_microusd"`
	SampleCount     int       `json:"sample_count"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// WorkflowSnapshot captures an immutable point-in-time workflow definition
// (metadata + all step fields) as JSONB. Used so in-flight runs are immune
// to live workflow_steps table changes.
type WorkflowSnapshot struct {
	ID         string          `json:"id"`
	WorkflowID string          `json:"workflow_id"`
	VersionID  string          `json:"version_id,omitempty"`
	Version    int             `json:"version"`
	Definition json.RawMessage `json:"definition"`
	CreatedAt  time.Time       `json:"created_at"`
}

// WorkflowSnapshotDefinition is the serialized content of a WorkflowSnapshot.Definition.
type WorkflowSnapshotDefinition struct {
	Workflow WorkflowSnapshotMeta `json:"workflow"`
	Steps    []WorkflowStep       `json:"steps"`
}

// WorkflowSnapshotMeta holds the workflow-level fields captured in the snapshot.
type WorkflowSnapshotMeta struct {
	ID                string            `json:"id"`
	ProjectID         string            `json:"project_id"`
	Name              string            `json:"name"`
	Slug              string            `json:"slug"`
	Description       string            `json:"description,omitempty"`
	Tags              map[string]string `json:"tags,omitempty"`
	Version           int               `json:"version"`
	VersionID         string            `json:"version_id,omitempty"`
	TimeoutSecs       int               `json:"timeout_secs,omitempty"`
	MaxConcurrentRuns int               `json:"max_concurrent_runs,omitempty"`
	MaxParallelSteps  int               `json:"max_parallel_steps,omitempty"`
}

// WorkflowRun represents an execution instance of a workflow.
type WorkflowRun struct {
	ID                   string            `json:"id"`
	WorkflowID           string            `json:"workflow_id"`
	ProjectID            string            `json:"project_id"`
	Tags                 map[string]string `json:"tags,omitempty"`
	Status               WorkflowRunStatus `json:"status"`
	TriggeredBy          string            `json:"triggered_by"`
	WorkflowVersion      int               `json:"workflow_version"`
	MaxParallelSteps     int               `json:"max_parallel_steps,omitempty"`
	Payload              json.RawMessage   `json:"payload,omitempty"`
	Error                string            `json:"error,omitempty"`
	StartedAt            *time.Time        `json:"started_at,omitempty"`
	FinishedAt           *time.Time        `json:"finished_at,omitempty"`
	ExpiresAt            *time.Time        `json:"expires_at,omitempty"`
	RetryOfRunID         string            `json:"retry_of_run_id,omitempty"`
	ParentWorkflowRunID  string            `json:"parent_workflow_run_id,omitempty"`
	ParentStepRunID      string            `json:"parent_step_run_id,omitempty"`
	WorkflowVersionID    string            `json:"workflow_version_id,omitempty"`
	WorkflowSnapshotID   string            `json:"workflow_snapshot_id,omitempty"`
	CreatedBy            string            `json:"created_by,omitempty"`
	ExpectedCompletionAt *time.Time        `json:"expected_completion_at,omitempty"`
	TraceContext         map[string]string `json:"trace_context,omitempty"`
	CreatedAt            time.Time         `json:"created_at"`
	CacheVersion         int64             `json:"-"`
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
type EventTrigger struct {
	ID                string          `json:"id"`
	EventKey          string          `json:"event_key"`
	ProjectID         string          `json:"project_id"`
	EnvironmentID     string          `json:"environment_id,omitempty"`       // optional env binding; empty means project-wide / legacy
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

// DeploymentStrategy defines the rollout strategy for a deployment version.
type DeploymentStrategy string

const (
	DeploymentStrategyDirect DeploymentStrategy = "direct"
	DeploymentStrategyCanary DeploymentStrategy = "canary"
)

// IsValid returns true if the deployment strategy is a known value.
func (s DeploymentStrategy) IsValid() bool {
	switch s {
	case DeploymentStrategyDirect, DeploymentStrategyCanary:
		return true
	default:
		return false
	}
}

// WorkerStatus is the lifecycle state of a registered worker.
type WorkerStatus string

const (
	WorkerStatusActive   WorkerStatus = "active"
	WorkerStatusDraining WorkerStatus = "draining"
	WorkerStatusOffline  WorkerStatus = "offline"
)

// Worker represents a registered worker process that polls for and executes job runs.
type Worker struct {
	ID           string       `json:"id"`
	ProjectID    string       `json:"project_id"`
	QueueName    string       `json:"queue_name"`
	Hostname     string       `json:"hostname"`
	Version      string       `json:"version"`
	Status       WorkerStatus `json:"status"`
	LastSeenAt   time.Time    `json:"last_seen_at"`
	RegisteredAt time.Time    `json:"registered_at"`
}

// WorkerTaskStatus is the lifecycle state of a task assigned to a worker.
type WorkerTaskStatus string

const (
	WorkerTaskStatusAssigned       WorkerTaskStatus = "assigned"
	WorkerTaskStatusAccepted       WorkerTaskStatus = "accepted"
	WorkerTaskStatusResultReceived WorkerTaskStatus = "result_received"
	WorkerTaskStatusFinalizing     WorkerTaskStatus = "finalizing"
	WorkerTaskStatusCompleted      WorkerTaskStatus = "completed"
	WorkerTaskStatusFailed         WorkerTaskStatus = "failed"
)

// WorkerTask represents a job run assigned to a specific worker.
type WorkerTask struct {
	ID         string            `json:"id"`
	WorkerID   string            `json:"worker_id"`
	RunID      string            `json:"run_id"`
	ProjectID  string            `json:"project_id"`
	Attempt    int               `json:"attempt"`
	Status     WorkerTaskStatus  `json:"status"`
	AssignedAt time.Time         `json:"assigned_at"`
	AcceptedAt *time.Time        `json:"accepted_at,omitempty"`
	FinishedAt *time.Time        `json:"finished_at,omitempty"`
	Result     *WorkerTaskResult `json:"result,omitempty"`
}

// WorkerTaskResult is the durable TaskResult payload accepted from the worker
// stream before executor finalization commits terminal run state.
type WorkerTaskResult struct {
	Status     string          `json:"status"`
	Output     json.RawMessage `json:"output,omitempty"`
	Error      string          `json:"error,omitempty"`
	DurationMS int64           `json:"duration_ms,omitempty"`
	ReceivedAt *time.Time      `json:"received_at,omitempty"`
}
