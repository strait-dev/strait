package billing

import (
	"context"
	"time"
)

type planUpdate struct {
	orgID  string
	tier   string
	status string
}

type fullUpdate struct {
	orgID       string
	tier        string
	status      string
	periodStart *time.Time
	periodEnd   *time.Time
}

type mockBillingStore struct {
	lastUpserted        *OrgSubscription
	upsertCount         int
	lastPlanUpdate      *planUpdate
	lastFullUpdate      *fullUpdate
	lastPendingTier     string
	pendingDowngradeOrg string
	subscriptions       map[string]*OrgSubscription
	projects            map[string][]string
}

func (m *mockBillingStore) GetOrgSubscription(_ context.Context, orgID string) (*OrgSubscription, error) {
	if m.subscriptions != nil {
		if sub, ok := m.subscriptions[orgID]; ok {
			return sub, nil
		}
	}
	return nil, ErrSubscriptionNotFound
}

func (m *mockBillingStore) UpsertOrgSubscription(_ context.Context, sub *OrgSubscription) error {
	m.lastUpserted = sub
	m.upsertCount++
	if m.subscriptions == nil {
		m.subscriptions = make(map[string]*OrgSubscription)
	}
	// Simulate ON CONFLICT behavior: preserve spending_limit and limit_action.
	if existing, ok := m.subscriptions[sub.OrgID]; ok {
		sub.SpendingLimitMicrousd = existing.SpendingLimitMicrousd
		sub.LimitAction = existing.LimitAction
	}
	m.subscriptions[sub.OrgID] = sub
	return nil
}

func (m *mockBillingStore) UpdateOrgSubscriptionPlan(_ context.Context, orgID, planTier, status string) error {
	m.lastPlanUpdate = &planUpdate{orgID: orgID, tier: planTier, status: status}
	return nil
}

func (m *mockBillingStore) UpdateOrgSubscriptionFull(_ context.Context, orgID, tier, status string, periodStart, periodEnd *time.Time) error {
	m.lastFullUpdate = &fullUpdate{orgID: orgID, tier: tier, status: status, periodStart: periodStart, periodEnd: periodEnd}
	if m.subscriptions != nil {
		if sub, ok := m.subscriptions[orgID]; ok {
			sub.PlanTier = tier
			sub.Status = status
			if periodStart != nil {
				sub.CurrentPeriodStart = periodStart
			}
			if periodEnd != nil {
				sub.CurrentPeriodEnd = periodEnd
			}
		}
	}
	return nil
}

func (m *mockBillingStore) UpdateSpendingLimit(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}

func (m *mockBillingStore) SetPendingPlanTier(_ context.Context, orgID, tier string) error {
	m.lastPendingTier = tier
	if m.subscriptions != nil {
		if sub, ok := m.subscriptions[orgID]; ok {
			sub.PendingPlanTier = &tier
		}
	}
	return nil
}

func (m *mockBillingStore) ApplyPendingDowngrade(_ context.Context, orgID string) error {
	m.pendingDowngradeOrg = orgID
	if m.subscriptions != nil {
		if sub, ok := m.subscriptions[orgID]; ok && sub.PendingPlanTier != nil {
			sub.PlanTier = *sub.PendingPlanTier
			sub.PendingPlanTier = nil
		}
	}
	return nil
}

func (m *mockBillingStore) ListOrgsWithPendingDowngrade(_ context.Context) ([]OrgSubscription, error) {
	var subs []OrgSubscription
	for _, sub := range m.subscriptions {
		if sub.PendingPlanTier != nil && sub.CurrentPeriodEnd != nil && sub.CurrentPeriodEnd.Before(time.Now()) {
			subs = append(subs, *sub)
		}
	}
	return subs, nil
}

func (m *mockBillingStore) GetProjectOrgID(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (m *mockBillingStore) ListProjectsByOrg(_ context.Context, orgID string) ([]string, error) {
	if m.projects != nil {
		return m.projects[orgID], nil
	}
	return nil, nil
}

func (m *mockBillingStore) CountProjectsByOrg(_ context.Context, orgID string) (int, error) {
	if m.projects != nil {
		return len(m.projects[orgID]), nil
	}
	return 0, nil
}

func (m *mockBillingStore) SetProjectOrgID(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockBillingStore) UpsertUsageRecord(_ context.Context, _ *UsageRecord) error {
	return nil
}

func (m *mockBillingStore) GetOrgUsageForPeriod(_ context.Context, _ string, _, _ time.Time) ([]UsageRecord, error) {
	return nil, nil
}

func (m *mockBillingStore) GetProjectUsageForPeriod(_ context.Context, _ string, _, _ time.Time) ([]UsageRecord, error) {
	return nil, nil
}

func (m *mockBillingStore) GetOrgDailyUsage(_ context.Context, _ string, _ time.Time) ([]UsageRecord, error) {
	return nil, nil
}

func (m *mockBillingStore) SumOrgPeriodSpend(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
