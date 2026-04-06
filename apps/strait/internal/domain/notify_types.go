package domain

import (
	"encoding/json"
	"time"
)

const (
	NotifyRecipientTypeSubscriber    = "subscriber"
	NotifyRecipientTypeDashboardUser = "dashboard_user"
)

const (
	NotifySubscriberStatusActive       = "active"
	NotifySubscriberStatusUnsubscribed = "unsubscribed"
	NotifySubscriberStatusDeleted      = "deleted"
)

const (
	NotifyCategoryTypeProduct       = "product"
	NotifyCategoryTypeTransactional = "transactional"
	NotifyCategoryTypeCritical      = "critical"
)

const (
	NotifyMessageStatusRendering  = "rendering"
	NotifyMessageStatusScheduled  = "scheduled"
	NotifyMessageStatusPending    = "pending"
	NotifyMessageStatusProcessing = "processing"
	NotifyMessageStatusDelivered  = "delivered"
	NotifyMessageStatusFailed     = "failed"
	NotifyMessageStatusBounced    = "bounced"
	NotifyMessageStatusCancelled  = "cancelled"
)

const (
	NotifyBatchStatusCollecting = "collecting"
	NotifyBatchStatusProcessing = "processing"
	NotifyBatchStatusSent       = "sent"
	NotifyBatchStatusFailed     = "failed"
)

const (
	NotifyEscalationStatusActive       = "active"
	NotifyEscalationStatusProcessing   = "processing"
	NotifyEscalationStatusCompleted    = "completed"
	NotifyEscalationStatusAcknowledged = "acknowledged"
	NotifyEscalationStatusFailed       = "failed"
)

const (
	NotifyInboxStateUnread   = "unread"
	NotifyInboxStateRead     = "read"
	NotifyInboxStateArchived = "archived"
	NotifyInboxStateActioned = "actioned"
)

const (
	NotifyPolicyScopeProject      = "project"
	NotifyPolicyScopeCategory     = "category"
	NotifyPolicyScopeWorkflowStep = "workflow_step"
)

const (
	NotifySuppressionActionSuppressed   = "suppressed"
	NotifySuppressionActionUnsuppressed = "unsuppressed"
)

const (
	NotifySuppressionSourceProviderCallback = "provider_callback"
	NotifySuppressionSourceAdminAPI         = "admin_api"
	NotifySuppressionSourceSubscriberAPI    = "subscriber_api"
)

// NotifySubscriber is an end-user recipient managed by a developer project.
type NotifySubscriber struct {
	ID         string          `json:"id"`
	ProjectID  string          `json:"project_id"`
	ExternalID string          `json:"external_id"`
	Email      string          `json:"email,omitempty"`
	Phone      string          `json:"phone,omitempty"`
	Locale     string          `json:"locale"`
	Timezone   string          `json:"timezone"`
	PushTokens json.RawMessage `json:"push_tokens"`
	Attributes json.RawMessage `json:"attributes"`
	TenantID   string          `json:"tenant_id,omitempty"`
	Status     string          `json:"status"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// NotifyTopic groups subscribers for fan-out sends.
type NotifyTopic struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	TopicKey    string          `json:"topic_key"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Attributes  json.RawMessage `json:"attributes"`
	CreatedAt   time.Time       `json:"created_at"`
}

// NotifyTopicMembership maps subscribers to topics.
type NotifyTopicMembership struct {
	TopicID      string    `json:"topic_id"`
	SubscriberID string    `json:"subscriber_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// UnsubscribeToken authorizes one-click unsubscribe operations.
type UnsubscribeToken struct {
	ID           string     `json:"id"`
	ProjectID    string     `json:"project_id"`
	SubscriberID string     `json:"subscriber_id"`
	Scope        string     `json:"scope"`
	Token        string     `json:"token,omitempty"`
	TokenHash    string     `json:"-"`
	UsedAt       *time.Time `json:"used_at,omitempty"`
	ExpiresAt    time.Time  `json:"expires_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

// NotificationTemplate stores versioned, per-channel template content.
type NotificationTemplate struct {
	ID              string          `json:"id"`
	ProjectID       string          `json:"project_id"`
	TemplateKey     string          `json:"template_key"`
	Name            string          `json:"name"`
	Description     string          `json:"description,omitempty"`
	Version         int             `json:"version"`
	Channels        json.RawMessage `json:"channels"`
	Variables       json.RawMessage `json:"variables"`
	LocaleTemplates json.RawMessage `json:"locale_templates"`
	DefaultLocale   string          `json:"default_locale"`
	Status          string          `json:"status"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// NotificationCategory controls preference semantics (product/transactional/critical).
type NotificationCategory struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	CategoryKey string    `json:"category_key"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Type        string    `json:"type"`
	CreatedAt   time.Time `json:"created_at"`
}

// NotificationPreference stores recipient-level delivery and digest settings.
type NotificationPreference struct {
	ID                string          `json:"id"`
	RecipientType     string          `json:"recipient_type"`
	RecipientID       string          `json:"recipient_id"`
	Scope             string          `json:"scope"`
	ChannelPrefs      json.RawMessage `json:"channel_prefs"`
	QuietHours        json.RawMessage `json:"quiet_hours,omitempty"`
	Phone             string          `json:"phone,omitempty"`
	Timezone          string          `json:"timezone"`
	DigestPolicy      string          `json:"digest_policy"`
	CriticalOverride  bool            `json:"critical_override"`
	RateLimitOverride *int            `json:"rate_limit_override,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// NotifyPolicyOverride configures delivery and escalation knobs by scope.
type NotifyPolicyOverride struct {
	ID                        string    `json:"id"`
	ProjectID                 string    `json:"project_id"`
	ScopeType                 string    `json:"scope_type"`
	ScopeKey                  string    `json:"scope_key"`
	Channel                   string    `json:"channel,omitempty"`
	DigestPolicy              string    `json:"digest_policy,omitempty"`
	RetryMaxAttempts          *int      `json:"retry_max_attempts,omitempty"`
	RetryBaseDelaySecs        *int      `json:"retry_base_delay_secs,omitempty"`
	RetryMaxDelaySecs         *int      `json:"retry_max_delay_secs,omitempty"`
	EscalationTiers           *int      `json:"escalation_tiers,omitempty"`
	EscalationMinIntervalSecs *int      `json:"escalation_min_interval_secs,omitempty"`
	Enabled                   bool      `json:"enabled"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

// NotifySuppressionEvent records suppression and unsuppression actions for recipients.
type NotifySuppressionEvent struct {
	ID            string          `json:"id"`
	ProjectID     string          `json:"project_id"`
	RecipientType string          `json:"recipient_type"`
	RecipientID   string          `json:"recipient_id"`
	Scope         string          `json:"scope"`
	Channel       string          `json:"channel"`
	Action        string          `json:"action"`
	Reason        string          `json:"reason,omitempty"`
	Source        string          `json:"source"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// NotificationMessage tracks channel delivery status and lifecycle timestamps.
type NotificationMessage struct {
	ID                string          `json:"id"`
	ProjectID         string          `json:"project_id"`
	IdempotencyKey    string          `json:"idempotency_key,omitempty"`
	RecipientType     string          `json:"recipient_type"`
	RecipientID       string          `json:"recipient_id"`
	TenantID          string          `json:"tenant_id,omitempty"`
	WorkflowRunID     string          `json:"workflow_run_id,omitempty"`
	StepRunID         string          `json:"step_run_id,omitempty"`
	TemplateID        string          `json:"template_id,omitempty"`
	CategoryKey       string          `json:"category_key,omitempty"`
	Channel           string          `json:"channel"`
	ProviderID        string          `json:"provider_id,omitempty"`
	RenderedContent   json.RawMessage `json:"rendered_content,omitempty"`
	AIGenerated       bool            `json:"ai_generated"`
	Status            string          `json:"status"`
	Attempts          int             `json:"attempts"`
	ProviderResponse  json.RawMessage `json:"provider_response,omitempty"`
	DeliveredAt       *time.Time      `json:"delivered_at,omitempty"`
	ReadAt            *time.Time      `json:"read_at,omitempty"`
	ClickedAt         *time.Time      `json:"clicked_at,omitempty"`
	BouncedAt         *time.Time      `json:"bounced_at,omitempty"`
	SuppressionReason string          `json:"suppression_reason,omitempty"`
	BatchID           string          `json:"batch_id,omitempty"`
	ScheduledAt       *time.Time      `json:"scheduled_at,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
}

// NotificationProvider stores per-channel provider credentials and failover links.
type NotificationProvider struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	Channel    string    `json:"channel"`
	Provider   string    `json:"provider"`
	Name       string    `json:"name"`
	ConfigEnc  []byte    `json:"-"`
	IsDefault  bool      `json:"is_default"`
	FallbackID string    `json:"fallback_id,omitempty"`
	Health     string    `json:"health"`
	RateLimit  *int      `json:"rate_limit,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// EscalationState tracks approval reminder escalation progression.
type EscalationState struct {
	ID               string     `json:"id"`
	ProjectID        string     `json:"project_id"`
	StepRunID        string     `json:"step_run_id"`
	WorkflowRunID    string     `json:"workflow_run_id"`
	CurrentTier      int        `json:"current_tier"`
	TotalTiers       int        `json:"total_tiers"`
	Acknowledged     bool       `json:"acknowledged"`
	AcknowledgedBy   string     `json:"acknowledged_by,omitempty"`
	AcknowledgedAt   *time.Time `json:"acknowledged_at,omitempty"`
	NextEscalationAt *time.Time `json:"next_escalation_at,omitempty"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// NotificationBatch groups events for digest delivery windows.
type NotificationBatch struct {
	ID            string          `json:"id"`
	ProjectID     string          `json:"project_id"`
	RecipientType string          `json:"recipient_type"`
	RecipientID   string          `json:"recipient_id"`
	BatchKey      string          `json:"batch_key"`
	Channel       string          `json:"channel"`
	Status        string          `json:"status"`
	Events        json.RawMessage `json:"events"`
	EventCount    int             `json:"event_count"`
	WindowStart   time.Time       `json:"window_start"`
	WindowEnd     time.Time       `json:"window_end"`
	SentAt        *time.Time      `json:"sent_at,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// InboxItem is a recipient-facing notification shown in in-app inboxes.
type InboxItem struct {
	ID            string          `json:"id"`
	RecipientType string          `json:"recipient_type"`
	RecipientID   string          `json:"recipient_id"`
	ProjectID     string          `json:"project_id"`
	TenantID      string          `json:"tenant_id,omitempty"`
	WorkflowID    string          `json:"workflow_id,omitempty"`
	WorkflowRunID string          `json:"workflow_run_id,omitempty"`
	MessageID     string          `json:"message_id,omitempty"`
	CategoryKey   string          `json:"category_key,omitempty"`
	Title         string          `json:"title"`
	Body          string          `json:"body,omitempty"`
	Avatar        string          `json:"avatar,omitempty"`
	Priority      string          `json:"priority"`
	State         string          `json:"state"`
	Actions       json.RawMessage `json:"actions"`
	DedupKey      string          `json:"dedup_key,omitempty"`
	DedupCount    int             `json:"dedup_count"`
	ReadAt        *time.Time      `json:"read_at,omitempty"`
	ArchivedAt    *time.Time      `json:"archived_at,omitempty"`
	ActionedAt    *time.Time      `json:"actioned_at,omitempty"`
	ActionResult  json.RawMessage `json:"action_result,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}
