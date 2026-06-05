package billing

import (
	"context"
	"time"
)

// SubscriptionAddOns captures the legacy organization_subscriptions.add_ons
// payload. Launch add-ons are stored in organization_addons; this JSONB column
// is intentionally inert so stale rows cannot grant entitlements.
type SubscriptionAddOns struct{}

// OrgSubscription represents an organization's subscription state.
type OrgSubscription struct {
	ID                         string
	OrgID                      string
	PlanTier                   string
	StripeSubscriptionID       *string
	StripeCustomerID           *string
	StripeLookupKey            *string
	Status                     string
	CurrentPeriodStart         *time.Time
	CurrentPeriodEnd           *time.Time
	SpendingLimitMicrousd      int64
	LimitAction                string
	OverageDisabled            bool
	PendingPlanTier            *string
	CanceledAt                 *time.Time
	AnomalyThresholdWarning    float64
	AnomalyThresholdCritical   float64
	GracePeriodEnd             *time.Time
	PaymentStatus              string // "ok", "grace", "restricted"
	OverrideDailyRunLimit      *int
	OverrideConcurrentRunLimit *int
	EnforcementMode            string // "enforce" (default), "warn", "disabled"
	MonthlyUsageEmail          bool   // opt-in for monthly PDF usage report emails
	AddOns                     SubscriptionAddOns
	// Entitlements holds the raw JSONB snapshot from
	// organization_subscriptions.entitlements. When non-empty (and != "{}")
	// it represents the authoritative resolved plan limits as of the most
	// recent mutator. The Enforcer reads this directly on the hot path
	// when available; callers that need the typed value should use
	// ComputeEntitlements over (sub, addons) instead.
	Entitlements []byte
	CreatedAt    time.Time
	UpdatedAt    time.Time
	CacheVersion int64
}

// BillingCapEvent identifies a per-period spend-cap webhook event. The string
// returned by Column is the organization_subscriptions column that records
// when the corresponding event was last dispatched in the current period.
// Restricting the enum to these four values keeps the SQL safe from caller-
// controlled column names without per-call validation.
type BillingCapEvent int

const (
	BillingCapEventWarning BillingCapEvent = iota
	BillingCapEventReached
	BillingCapEventDisabled
	BillingCapEventOverageDisabled
)

func (e BillingCapEvent) Column() string {
	switch e {
	case BillingCapEventWarning:
		return "cap_warning_dispatched_at"
	case BillingCapEventReached:
		return "cap_reached_dispatched_at"
	case BillingCapEventDisabled:
		return "cap_disabled_dispatched_at"
	case BillingCapEventOverageDisabled:
		return "overage_disabled_dispatched_at"
	}
	return ""
}

// UsageRecord represents a daily usage aggregate per org and project.
type UsageRecord struct {
	ID               string
	OrgID            string
	ProjectID        string
	PeriodDate       time.Time
	RunsCount        int64
	ComputeCostMicro int64
	UsageTokensTotal int64
	UsageCostMicro   int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Store defines billing data access operations.
type Store interface {
	// Organization subscriptions
	EnsureOrgSubscription(ctx context.Context, orgID string) error
	GetOrgSubscription(ctx context.Context, orgID string) (*OrgSubscription, error)
	GetOrgSubscriptionByStripeSubscriptionID(ctx context.Context, stripeSubscriptionID string) (*OrgSubscription, error)
	GetOrgSubscriptionByStripeCustomerID(ctx context.Context, stripeCustomerID string) (*OrgSubscription, error)
	UpsertOrgSubscription(ctx context.Context, sub *OrgSubscription) error
	UpdateOrgSubscriptionPlan(ctx context.Context, orgID, planTier, status string) error
	UpdateOrgSubscriptionStatus(ctx context.Context, orgID, status string) error
	UpdateOrgSubscriptionFull(ctx context.Context, orgID, planTier, status string, periodStart, periodEnd *time.Time) error
	UpdateSpendingLimit(ctx context.Context, orgID string, limitMicrousd int64, action string) error
	UpdateOverageDisabled(ctx context.Context, orgID string, disabled bool) error
	SetPendingPlanTier(ctx context.Context, orgID, tier string) error
	SetPendingDowngrade(ctx context.Context, orgID, pendingTier string, periodStart, periodEnd *time.Time) error
	ClearPendingPlanTier(ctx context.Context, orgID string) error
	ApplyPendingDowngrade(ctx context.Context, orgID string) error
	ListOrgsWithPendingDowngrade(ctx context.Context) ([]OrgSubscription, error)
	UpdateEntitlements(ctx context.Context, orgID string, entitlements OrgPlanLimits) error

	// Project-org mapping
	GetProjectOrgID(ctx context.Context, projectID string) (string, error)
	GetActiveProjectOrgID(ctx context.Context, projectID string) (string, error)
	ListProjectsByOrg(ctx context.Context, orgID string) ([]string, error)
	CountProjectsByOrg(ctx context.Context, orgID string) (int, error)
	CountMembersByOrg(ctx context.Context, orgID string) (int, error)
	CountOrgsByUser(ctx context.Context, userID string) (int, error)
	CountExecutingRunsByOrg(ctx context.Context, orgID string) (int, error)
	BulkCountExecutingRunsByOrg(ctx context.Context, orgIDs []string) (map[string]int, error)
	SetProjectOrgID(ctx context.Context, projectID, orgID string) error

	// Usage records
	UpsertUsageRecord(ctx context.Context, rec *UsageRecord) error
	GetOrgUsageForPeriod(ctx context.Context, orgID string, from, to time.Time) ([]UsageRecord, error)
	GetProjectUsageForPeriod(ctx context.Context, projectID string, from, to time.Time) ([]UsageRecord, error)
	GetOrgDailyUsage(ctx context.Context, orgID string, date time.Time) ([]UsageRecord, error)
	SumOrgPeriodSpend(ctx context.Context, orgID string, from time.Time) (int64, error)

	// Project budget
	GetProjectBudget(ctx context.Context, projectID string) (int64, string, error)
	SetProjectBudget(ctx context.Context, projectID string, budgetMicro int64, action string) error
	GetProjectPeriodSpend(ctx context.Context, projectID string, from time.Time) (int64, error)

	// Anomaly thresholds
	UpdateAnomalyThresholds(ctx context.Context, orgID string, warning, critical float64) error

	// TryMarkBillingCapEvent atomically stamps the cap-event dispatched-at
	// column to NOW() when it is NULL. Returns true when the caller is the
	// first dispatcher in the current period; false when a prior caller
	// already marked the column. The dedup column is reset to NULL on
	// current_period_start rollover by UpsertOrgSubscription.
	TryMarkBillingCapEvent(ctx context.Context, orgID string, ev BillingCapEvent) (bool, error)

	// Grace period
	UpdatePaymentStatus(ctx context.Context, orgID string, status string, graceEnd *time.Time) error
	ListOrgsInGracePeriod(ctx context.Context) ([]OrgSubscription, error)

	// Org listing
	ListAllSubscribedOrgIDs(ctx context.Context) ([]string, error)

	// Stale subscription detection
	ListStaleSubscriptions(ctx context.Context) ([]OrgSubscription, error)

	// Project suspension
	IsProjectSuspended(ctx context.Context, projectID string) (bool, error)
	SuspendExcessProjects(ctx context.Context, orgID string, maxProjects int) (int, error)

	// Org admin emails (for usage report emails)
	ListOrgAdminEmails(ctx context.Context, orgID string) ([]string, error)

	// Usage report deduplication
	HasSentUsageReport(ctx context.Context, orgID string, periodEnd time.Time) (bool, error)
	RecordSentUsageReport(ctx context.Context, orgID string, periodEnd time.Time) error

	// Email preference
	UpdateMonthlyUsageEmail(ctx context.Context, orgID string, enabled bool) error

	// Organization add-ons
	ListActiveAddons(ctx context.Context, orgID string) ([]Addon, error)
	CreateAddon(ctx context.Context, addon *Addon) error
	DeactivateAddon(ctx context.Context, addonID string) error
	CountActiveAddonsByType(ctx context.Context, orgID string, addonType AddonType) (int, error)

	// Webhook idempotency
	RecordProcessedWebhook(ctx context.Context, msgID string) error
	IsWebhookProcessed(ctx context.Context, msgID string) (bool, error)

	// Webhook message cleanup
	DeleteOldWebhookMessages(ctx context.Context, olderThan time.Time) (int64, error)

	// Enterprise contracts
	GetEnterpriseContract(ctx context.Context, orgID string) (*EnterpriseContract, error)
	UpsertEnterpriseContract(ctx context.Context, contract *EnterpriseContract) error
	ListExpiringContracts(ctx context.Context, withinDays int) ([]EnterpriseContract, error)

	// HTTP-mode job lifecycle (downgrade auto-pause / upgrade auto-unpause)
	PauseHTTPJobsByOrg(ctx context.Context, orgID, reason string) ([]string, error)
	UnpauseJobsByPauseReason(ctx context.Context, orgID, reason string) (int64, error)
	CountHTTPJobsByOrg(ctx context.Context, orgID string) (int, error)
}
