package billing

import (
	"context"
	"sync"
	"time"
)

type planUpdate struct {
	orgID  string
	tier   string
	status string
}

type statusUpdate struct {
	orgID  string
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

type pendingDowngradeUpdate struct {
	orgID       string
	pendingTier string
	periodStart *time.Time
	periodEnd   *time.Time
}

type mockBillingStore struct {
	mu                         sync.Mutex // guards lastEntitlementsUpdates and capEventMarks against concurrent enforcer-driven writes
	capEventMarks              map[string]map[BillingCapEvent]time.Time
	lastUpserted               *OrgSubscription
	upsertCount                int
	lastPlanUpdate             *planUpdate
	lastStatusUpdate           *statusUpdate
	lastFullUpdate             *fullUpdate
	lastOverageDisabledOrg     string
	lastOverageDisabled        bool
	lastPendingTier            string
	lastClearedPending         string
	pendingDowngradeOrg        string
	lastPendingDowngrade       *pendingDowngradeUpdate
	lastEntitlementsUpdates    map[string]OrgPlanLimits
	lastPaymentStatusUpdate    *paymentStatusUpdate
	subscriptions              map[string]*OrgSubscription
	projects                   map[string][]string
	countProjectsErr           error
	memberCounts               map[string]int
	countMembersErr            error
	orgCountsByUser            map[string]int
	countOrgsByUserErr         error
	executingRuns              map[string]int
	usageRecords               []UsageRecord
	periodSpendByOrg           map[string]int64
	sumSpendErr                error
	recordWebhookErr           error
	claimWebhookErr            error
	claimWebhookResult         *bool
	webhookProcessingStatus    string
	webhookProcessingStatusErr error
	releasedWebhookIDs         []string
	recordedWebhookIDs         []string
	getOrgSubscriptionFn       func(ctx context.Context, orgID string) (*OrgSubscription, error)
	getProjectOrgIDFn          func(ctx context.Context, projectID string) (string, error)
	enterpriseContracts        map[string]*EnterpriseContract
	upsertEnterpriseContractFn func(ctx context.Context, c *EnterpriseContract) error
	activeAddons               []Addon
	listActiveAddonsErr        error
	countActiveAddonsErr       error
	lastAddonCreated           *Addon
	deactivatedAddonIDs        []string
	httpJobCount               int
	countHTTPJobsErr           error
	pausedOrgID                string
	pausedReason               string
	pausedJobIDs               []string
	unpausedOrgID              string
	unpausedReason             string
	unpausedCount              int64
	isProjectSuspendedErr      error
	getProjectBudgetFn         func(ctx context.Context, projectID string) (int64, string, error)
	getProjectPeriodSpendFn    func(ctx context.Context, projectID string, periodStart time.Time) (int64, error)
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

func (m *mockBillingStore) GetOrgSubscriptionByStripeSubscriptionID(_ context.Context, stripeSubscriptionID string) (*OrgSubscription, error) {
	if m.subscriptions != nil {
		for _, sub := range m.subscriptions {
			if sub.StripeSubscriptionID != nil && *sub.StripeSubscriptionID == stripeSubscriptionID {
				return sub, nil
			}
		}
	}
	return nil, ErrSubscriptionNotFound
}

func (m *mockBillingStore) GetOrgSubscriptionByStripeCustomerID(_ context.Context, stripeCustomerID string) (*OrgSubscription, error) {
	if m.subscriptions != nil {
		for _, sub := range m.subscriptions {
			if sub.StripeCustomerID != nil && *sub.StripeCustomerID == stripeCustomerID {
				return sub, nil
			}
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

func (m *mockBillingStore) UpdateOrgSubscriptionStatus(_ context.Context, orgID, status string) error {
	m.lastStatusUpdate = &statusUpdate{orgID: orgID, status: status}
	if m.subscriptions != nil {
		if sub, ok := m.subscriptions[orgID]; ok {
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

func (m *mockBillingStore) UpdateOverageDisabled(_ context.Context, orgID string, disabled bool) error {
	m.lastOverageDisabledOrg = orgID
	m.lastOverageDisabled = disabled
	if m.subscriptions != nil {
		if sub, ok := m.subscriptions[orgID]; ok {
			sub.OverageDisabled = disabled
			return nil
		}
	}
	return ErrSubscriptionNotFound
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

func (m *mockBillingStore) SetPendingDowngrade(_ context.Context, orgID, pendingTier string, periodStart, periodEnd *time.Time) error {
	m.lastPendingTier = pendingTier
	m.lastPendingDowngrade = &pendingDowngradeUpdate{
		orgID:       orgID,
		pendingTier: pendingTier,
		periodStart: periodStart,
		periodEnd:   periodEnd,
	}
	if m.subscriptions != nil {
		if sub, ok := m.subscriptions[orgID]; ok {
			sub.PendingPlanTier = &pendingTier
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

func (m *mockBillingStore) TryMarkBillingCapEvent(_ context.Context, orgID string, ev BillingCapEvent) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.capEventMarks == nil {
		m.capEventMarks = make(map[string]map[BillingCapEvent]time.Time)
	}
	per, ok := m.capEventMarks[orgID]
	if !ok {
		per = make(map[BillingCapEvent]time.Time)
		m.capEventMarks[orgID] = per
	}
	if _, already := per[ev]; already {
		return false, nil
	}
	per[ev] = time.Now()
	return true, nil
}

func (m *mockBillingStore) UpdateEntitlements(_ context.Context, orgID string, entitlements OrgPlanLimits) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastEntitlementsUpdates == nil {
		m.lastEntitlementsUpdates = make(map[string]OrgPlanLimits)
	}
	m.lastEntitlementsUpdates[orgID] = entitlements
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

func (m *mockBillingStore) GetProjectOrgID(ctx context.Context, projectID string) (string, error) {
	if m.getProjectOrgIDFn != nil {
		return m.getProjectOrgIDFn(ctx, projectID)
	}
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
	if m.countProjectsErr != nil {
		return 0, m.countProjectsErr
	}
	if m.projects != nil {
		return len(m.projects[orgID]), nil
	}
	return 0, nil
}

func (m *mockBillingStore) CountMembersByOrg(_ context.Context, orgID string) (int, error) {
	if m.countMembersErr != nil {
		return 0, m.countMembersErr
	}
	if m.memberCounts != nil {
		return m.memberCounts[orgID], nil
	}
	return 0, nil
}

func (m *mockBillingStore) CountOrgsByUser(_ context.Context, userID string) (int, error) {
	if m.countOrgsByUserErr != nil {
		return 0, m.countOrgsByUserErr
	}
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

func (m *mockBillingStore) SetProjectOrgID(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockBillingStore) UpsertUsageRecord(_ context.Context, _ *UsageRecord) error {
	return nil
}

func (m *mockBillingStore) GetOrgUsageForPeriod(_ context.Context, _ string, _, _ time.Time) ([]UsageRecord, error) {
	return m.usageRecords, nil
}

func (m *mockBillingStore) GetOrgUsageForPeriodLimited(_ context.Context, _ string, _, _ time.Time, limit int) ([]UsageRecord, error) {
	if len(m.usageRecords) > limit {
		return m.usageRecords[:limit], nil
	}
	return m.usageRecords, nil
}

func (m *mockBillingStore) GetProjectUsageForPeriod(_ context.Context, _ string, _, _ time.Time) ([]UsageRecord, error) {
	return m.usageRecords, nil
}

func (m *mockBillingStore) GetOrgDailyUsage(_ context.Context, _ string, _ time.Time) ([]UsageRecord, error) {
	return m.usageRecords, nil
}

func (m *mockBillingStore) SumOrgPeriodSpend(_ context.Context, orgID string, _ time.Time) (int64, error) {
	if m.sumSpendErr != nil {
		return 0, m.sumSpendErr
	}
	if m.periodSpendByOrg != nil {
		return m.periodSpendByOrg[orgID], nil
	}
	return 0, nil
}

func (m *mockBillingStore) GetProjectBudget(ctx context.Context, projectID string) (int64, string, error) {
	if m.getProjectBudgetFn != nil {
		return m.getProjectBudgetFn(ctx, projectID)
	}
	return -1, "notify", nil
}

func (m *mockBillingStore) SetProjectBudget(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}

func (m *mockBillingStore) GetProjectPeriodSpend(ctx context.Context, projectID string, periodStart time.Time) (int64, error) {
	if m.getProjectPeriodSpendFn != nil {
		return m.getProjectPeriodSpendFn(ctx, projectID, periodStart)
	}
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
	if m.isProjectSuspendedErr != nil {
		return false, m.isProjectSuspendedErr
	}
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
	if m.listActiveAddonsErr != nil {
		return nil, m.listActiveAddonsErr
	}
	return m.activeAddons, nil
}

func (m *mockBillingStore) CreateAddon(_ context.Context, addon *Addon) error {
	m.lastAddonCreated = addon
	if addon != nil {
		m.activeAddons = append(m.activeAddons, *addon)
	}
	return nil
}

func (m *mockBillingStore) DeactivateAddon(_ context.Context, id string) error {
	m.deactivatedAddonIDs = append(m.deactivatedAddonIDs, id)
	for i := range m.activeAddons {
		if m.activeAddons[i].ID == id {
			m.activeAddons[i].Active = false
		}
	}
	return nil
}

func (m *mockBillingStore) CountActiveAddonsByType(_ context.Context, orgID string, addonType AddonType) (int, error) {
	if m.countActiveAddonsErr != nil {
		return 0, m.countActiveAddonsErr
	}
	count := 0
	for _, addon := range m.activeAddons {
		if addon.Active && addon.OrgID == orgID && addon.AddonType == addonType {
			count++
		}
	}
	return count, nil
}

func (m *mockBillingStore) RecordProcessedWebhook(_ context.Context, msgID string) error {
	m.recordedWebhookIDs = append(m.recordedWebhookIDs, msgID)
	return m.recordWebhookErr
}

func (m *mockBillingStore) ClaimWebhookForProcessing(_ context.Context, _ string, _ time.Duration) (bool, error) {
	if m.claimWebhookErr != nil {
		return false, m.claimWebhookErr
	}
	if m.claimWebhookResult != nil {
		return *m.claimWebhookResult, nil
	}
	return true, nil
}

func (m *mockBillingStore) MarkWebhookProcessed(_ context.Context, msgID string) error {
	m.recordedWebhookIDs = append(m.recordedWebhookIDs, msgID)
	return m.recordWebhookErr
}

func (m *mockBillingStore) ReleaseWebhookClaim(_ context.Context, msgID string) error {
	m.releasedWebhookIDs = append(m.releasedWebhookIDs, msgID)
	return nil
}

func (m *mockBillingStore) GetWebhookProcessingStatus(_ context.Context, _ string) (string, error) {
	return m.webhookProcessingStatus, m.webhookProcessingStatusErr
}

func (m *mockBillingStore) IsWebhookProcessed(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *mockBillingStore) DeleteOldWebhookMessages(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (m *mockBillingStore) GetEnterpriseContract(_ context.Context, orgID string) (*EnterpriseContract, error) {
	if m.enterpriseContracts != nil {
		if c, ok := m.enterpriseContracts[orgID]; ok {
			return c, nil
		}
	}
	return nil, ErrContractNotFound
}

func (m *mockBillingStore) UpsertEnterpriseContract(ctx context.Context, c *EnterpriseContract) error {
	if m.upsertEnterpriseContractFn != nil {
		return m.upsertEnterpriseContractFn(ctx, c)
	}
	if m.enterpriseContracts == nil {
		m.enterpriseContracts = make(map[string]*EnterpriseContract)
	}
	m.enterpriseContracts[c.OrgID] = c
	return nil
}

func (m *mockBillingStore) ListExpiringContracts(_ context.Context, _ int) ([]EnterpriseContract, error) {
	return nil, nil
}

func (m *mockBillingStore) PauseHTTPJobsByOrg(_ context.Context, orgID string, reason string) ([]string, error) {
	m.pausedOrgID = orgID
	m.pausedReason = reason
	if m.pausedJobIDs != nil {
		return m.pausedJobIDs, nil
	}
	return nil, nil
}

func (m *mockBillingStore) UnpauseJobsByPauseReason(_ context.Context, orgID string, reason string) (int64, error) {
	m.unpausedOrgID = orgID
	m.unpausedReason = reason
	return m.unpausedCount, nil
}

func (m *mockBillingStore) CountHTTPJobsByOrg(_ context.Context, _ string) (int, error) {
	if m.countHTTPJobsErr != nil {
		return 0, m.countHTTPJobsErr
	}
	return m.httpJobCount, nil
}
