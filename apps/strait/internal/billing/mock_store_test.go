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

type mockBillingStore struct {
	lastUpserted   *OrgSubscription
	upsertCount    int
	lastPlanUpdate *planUpdate
	subscriptions  map[string]*OrgSubscription
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
	m.subscriptions[sub.OrgID] = sub
	return nil
}

func (m *mockBillingStore) UpdateOrgSubscriptionPlan(_ context.Context, orgID, planTier, status string) error {
	m.lastPlanUpdate = &planUpdate{orgID: orgID, tier: planTier, status: status}
	return nil
}

func (m *mockBillingStore) UpdateSpendingLimit(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}

func (m *mockBillingStore) GetProjectOrgID(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (m *mockBillingStore) ListProjectsByOrg(_ context.Context, _ string) ([]string, error) {
	return nil, nil
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
