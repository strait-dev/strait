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

type paymentStatusUpdate struct {
	orgID    string
	status   string
	graceEnd *time.Time
}

type mockBillingStore struct {
	lastUpserted            *OrgSubscription
	upsertCount             int
	lastPlanUpdate          *planUpdate
	lastFullUpdate          *fullUpdate
	lastPendingTier         string
	lastClearedPending      string
	pendingDowngradeOrg     string
	lastPaymentStatusUpdate *paymentStatusUpdate
	subscriptions           map[string]*OrgSubscription
	projects                map[string][]string
	memberCounts            map[string]int
	orgCountsByUser         map[string]int
	executingRuns           map[string]int
	aiModelCallCounts       map[string]int64
	usageRecords            []UsageRecord
	periodSpendByOrg        map[string]int64
	getOrgSubscriptionFn    func(ctx context.Context, orgID string) (*OrgSubscription, error)
}

func (m *mockBillingStore) GetOrgSubscription(ctx context.Context, orgID string) (*OrgSubscription, error) {
	if m.getOrgSubscriptionFn != nil {
		return m.getOrgSubscriptionFn(ctx, orgID)
	}
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
		sub.PendingPlanTier = nil
	}
	m.subscriptions[sub.OrgID] = sub
	return nil
}

func (m *mockBillingStore) UpdateOrgSubscriptionPlan(_ context.Context, orgID, planTier, status string) error {
	m.lastPlanUpdate = &planUpdate{orgID: orgID, tier: planTier, status: status}
	if m.subscriptions != nil {
		if sub, ok := m.subscriptions[orgID]; ok {
			sub.PlanTier = planTier
			sub.Status = status
			return nil
		}
	}
	return ErrSubscriptionNotFound
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
			return nil
		}
	}
	return ErrSubscriptionNotFound
}

func (m *mockBillingStore) UpdateSpendingLimit(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}

func (m *mockBillingStore) SetPendingPlanTier(_ context.Context, orgID, tier string) error {
	m.lastPendingTier = tier
	if m.subscriptions != nil {
		if sub, ok := m.subscriptions[orgID]; ok {
			sub.PendingPlanTier = &tier
			return nil
		}
	}
	return ErrSubscriptionNotFound
}

func (m *mockBillingStore) SetPendingDowngrade(_ context.Context, orgID, pendingTier string, _, _ *time.Time) error {
	m.lastPendingTier = pendingTier
	if m.subscriptions != nil {
		if sub, ok := m.subscriptions[orgID]; ok {
			sub.PendingPlanTier = &pendingTier
			return nil
		}
	}
	return ErrSubscriptionNotFound
}

func (m *mockBillingStore) ClearPendingPlanTier(_ context.Context, orgID string) error {
	m.lastClearedPending = orgID
	if m.subscriptions != nil {
		if sub, ok := m.subscriptions[orgID]; ok {
			sub.PendingPlanTier = nil
			return nil
		}
	}
	return ErrSubscriptionNotFound
}

func (m *mockBillingStore) ApplyPendingDowngrade(_ context.Context, orgID string) error {
	m.pendingDowngradeOrg = orgID
	if m.subscriptions != nil {
		if sub, ok := m.subscriptions[orgID]; ok && sub.PendingPlanTier != nil {
			sub.PlanTier = *sub.PendingPlanTier
			sub.PendingPlanTier = nil
			return nil
		}
	}
	return ErrSubscriptionNotFound
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

func (m *mockBillingStore) GetActiveProjectOrgID(_ context.Context, _ string) (string, error) {
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

func (m *mockBillingStore) CountMembersByOrg(_ context.Context, orgID string) (int, error) {
	if m.memberCounts != nil {
		return m.memberCounts[orgID], nil
	}
	return 0, nil
}

func (m *mockBillingStore) CountOrgsByUser(_ context.Context, userID string) (int, error) {
	if m.orgCountsByUser != nil {
		return m.orgCountsByUser[userID], nil
	}
	return 0, nil
}

func (m *mockBillingStore) CountExecutingRunsByOrg(_ context.Context, orgID string) (int, error) {
	if m.executingRuns != nil {
		return m.executingRuns[orgID], nil
	}
	return 0, nil
}

func (m *mockBillingStore) BulkCountExecutingRunsByOrg(_ context.Context, orgIDs []string) (map[string]int, error) {
	result := make(map[string]int, len(orgIDs))
	for _, orgID := range orgIDs {
		if m.executingRuns != nil {
			result[orgID] = m.executingRuns[orgID]
		}
	}
	return result, nil
}

func (m *mockBillingStore) CountAIModelCallsByOrg(_ context.Context, orgID string, _, _ time.Time) (int64, error) {
	if m.aiModelCallCounts != nil {
		return m.aiModelCallCounts[orgID], nil
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
	return m.usageRecords, nil
}

func (m *mockBillingStore) GetProjectUsageForPeriod(_ context.Context, _ string, _, _ time.Time) ([]UsageRecord, error) {
	return m.usageRecords, nil
}

func (m *mockBillingStore) GetOrgDailyUsage(_ context.Context, _ string, _ time.Time) ([]UsageRecord, error) {
	return m.usageRecords, nil
}

func (m *mockBillingStore) SumOrgPeriodSpend(_ context.Context, orgID string, _ time.Time) (int64, error) {
	if m.periodSpendByOrg != nil {
		return m.periodSpendByOrg[orgID], nil
	}
	return 0, nil
}

func (m *mockBillingStore) GetProjectBudget(_ context.Context, _ string) (int64, string, error) {
	return -1, "notify", nil
}

func (m *mockBillingStore) SetProjectBudget(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}

func (m *mockBillingStore) GetProjectPeriodSpend(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}

func (m *mockBillingStore) UpdateAnomalyThresholds(_ context.Context, _ string, _, _ float64) error {
	return nil
}

func (m *mockBillingStore) UpdatePaymentStatus(_ context.Context, orgID string, status string, graceEnd *time.Time) error {
	m.lastPaymentStatusUpdate = &paymentStatusUpdate{orgID: orgID, status: status, graceEnd: graceEnd}
	if m.subscriptions != nil {
		if sub, ok := m.subscriptions[orgID]; ok {
			sub.PaymentStatus = status
			sub.GracePeriodEnd = graceEnd
			return nil
		}
	}
	return ErrSubscriptionNotFound
}

func (m *mockBillingStore) ListOrgsInGracePeriod(_ context.Context) ([]OrgSubscription, error) {
	var subs []OrgSubscription
	for _, sub := range m.subscriptions {
		if sub.PaymentStatus == "grace" && sub.GracePeriodEnd != nil && sub.GracePeriodEnd.Before(time.Now()) {
			subs = append(subs, *sub)
		}
	}
	return subs, nil
}

func (m *mockBillingStore) ListAllSubscribedOrgIDs(_ context.Context) ([]string, error) {
	var ids []string
	for _, sub := range m.subscriptions {
		if sub.Status == "active" {
			ids = append(ids, sub.OrgID)
		}
	}
	return ids, nil
}

func (m *mockBillingStore) EnsureOrgSubscription(_ context.Context, _ string) error { return nil }

func (m *mockBillingStore) ListStaleSubscriptions(_ context.Context) ([]OrgSubscription, error) {
	return nil, nil
}

func (m *mockBillingStore) IsProjectSuspended(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *mockBillingStore) SuspendExcessProjects(_ context.Context, _ string, _ int) (int, error) {
	return 0, nil
}

func (m *mockBillingStore) ListOrgAdminEmails(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (m *mockBillingStore) HasSentUsageReport(_ context.Context, _ string, _ time.Time) (bool, error) {
	return false, nil
}

func (m *mockBillingStore) RecordSentUsageReport(_ context.Context, _ string, _ time.Time) error {
	return nil
}

func (m *mockBillingStore) UpdateMonthlyUsageEmail(_ context.Context, _ string, _ bool) error {
	return nil
}

func (m *mockBillingStore) ListActiveAddons(_ context.Context, _ string) ([]Addon, error) {
	return nil, nil
}

func (m *mockBillingStore) CreateAddon(_ context.Context, _ *Addon) error {
	return nil
}

func (m *mockBillingStore) DeactivateAddon(_ context.Context, _ string) error {
	return nil
}

func (m *mockBillingStore) CountActiveAddonsByType(_ context.Context, _ string, _ AddonType) (int, error) {
	return 0, nil
}

func (m *mockBillingStore) RecordProcessedWebhook(_ context.Context, _ string) error {
	return nil
}

func (m *mockBillingStore) IsWebhookProcessed(_ context.Context, _ string) (bool, error) {
	return false, nil
}
