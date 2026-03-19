package billing

import (
	"context"
	"time"
)

// OrgSubscription represents an organization's subscription state.
type OrgSubscription struct {
	ID                    string
	OrgID                 string
	PlanTier              string
	PolarSubscriptionID   *string
	PolarCustomerID       *string
	Status                string
	CurrentPeriodStart    *time.Time
	CurrentPeriodEnd      *time.Time
	SpendingLimitMicrousd int64
	LimitAction           string
	CanceledAt            *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// UsageRecord represents a daily usage aggregate per org and project.
type UsageRecord struct {
	ID               string
	OrgID            string
	ProjectID        string
	PeriodDate       time.Time
	RunsCount        int64
	ComputeCostMicro int64
	AITokensTotal    int64
	AICostMicro      int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Store defines billing data access operations.
type Store interface {
	// Organization subscriptions
	GetOrgSubscription(ctx context.Context, orgID string) (*OrgSubscription, error)
	UpsertOrgSubscription(ctx context.Context, sub *OrgSubscription) error
	UpdateOrgSubscriptionPlan(ctx context.Context, orgID, planTier, status string) error
	UpdateSpendingLimit(ctx context.Context, orgID string, limitMicrousd int64, action string) error

	// Project-org mapping
	GetProjectOrgID(ctx context.Context, projectID string) (string, error)
	ListProjectsByOrg(ctx context.Context, orgID string) ([]string, error)
	CountProjectsByOrg(ctx context.Context, orgID string) (int, error)
	SetProjectOrgID(ctx context.Context, projectID, orgID string) error

	// Usage records
	UpsertUsageRecord(ctx context.Context, rec *UsageRecord) error
	GetOrgUsageForPeriod(ctx context.Context, orgID string, from, to time.Time) ([]UsageRecord, error)
	GetProjectUsageForPeriod(ctx context.Context, projectID string, from, to time.Time) ([]UsageRecord, error)
	GetOrgDailyUsage(ctx context.Context, orgID string, date time.Time) ([]UsageRecord, error)
	SumOrgPeriodSpend(ctx context.Context, orgID string, from time.Time) (int64, error)
}
