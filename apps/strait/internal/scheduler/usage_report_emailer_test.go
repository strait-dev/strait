package scheduler

import (
	"context"
	"testing"
	"time"

	"strait/internal/billing"

	"github.com/resend/resend-go/v2"
)

type mockReportStore struct {
	orgIDs        []string
	subscriptions map[string]*billing.OrgSubscription
	adminEmails   map[string][]string
	usageRecords  []billing.UsageRecord
}

func (m *mockReportStore) ListAllSubscribedOrgIDs(context.Context) ([]string, error) {
	return m.orgIDs, nil
}

func (m *mockReportStore) GetOrgSubscription(_ context.Context, orgID string) (*billing.OrgSubscription, error) {
	if sub, ok := m.subscriptions[orgID]; ok {
		return sub, nil
	}
	return nil, billing.ErrSubscriptionNotFound
}

func (m *mockReportStore) GetOrgUsageForPeriod(context.Context, string, time.Time, time.Time) ([]billing.UsageRecord, error) {
	return m.usageRecords, nil
}

func (m *mockReportStore) ListOrgAdminEmails(_ context.Context, orgID string) ([]string, error) {
	return m.adminEmails[orgID], nil
}

// Stub all remaining billing.Store methods.
func (m *mockReportStore) EnsureOrgSubscription(context.Context, string) error { return nil }
func (m *mockReportStore) UpsertOrgSubscription(context.Context, *billing.OrgSubscription) error {
	return nil
}
func (m *mockReportStore) UpdateOrgSubscriptionPlan(context.Context, string, string, string) error {
	return nil
}
func (m *mockReportStore) UpdateOrgSubscriptionFull(context.Context, string, string, string, *time.Time, *time.Time) error {
	return nil
}
func (m *mockReportStore) UpdateSpendingLimit(context.Context, string, int64, string) error {
	return nil
}
func (m *mockReportStore) SetPendingPlanTier(context.Context, string, string) error { return nil }
func (m *mockReportStore) SetPendingDowngrade(context.Context, string, string, *time.Time, *time.Time) error {
	return nil
}
func (m *mockReportStore) ClearPendingPlanTier(context.Context, string) error  { return nil }
func (m *mockReportStore) ApplyPendingDowngrade(context.Context, string) error { return nil }
func (m *mockReportStore) ListOrgsWithPendingDowngrade(context.Context) ([]billing.OrgSubscription, error) {
	return nil, nil
}
func (m *mockReportStore) GetProjectOrgID(context.Context, string) (string, error) { return "", nil }
func (m *mockReportStore) GetActiveProjectOrgID(context.Context, string) (string, error) {
	return "", nil
}
func (m *mockReportStore) ListProjectsByOrg(context.Context, string) ([]string, error) {
	return nil, nil
}
func (m *mockReportStore) CountProjectsByOrg(context.Context, string) (int, error) { return 0, nil }
func (m *mockReportStore) CountMembersByOrg(context.Context, string) (int, error)  { return 0, nil }
func (m *mockReportStore) CountOrgsByUser(context.Context, string) (int, error)    { return 0, nil }
func (m *mockReportStore) CountExecutingRunsByOrg(context.Context, string) (int, error) {
	return 0, nil
}
func (m *mockReportStore) BulkCountExecutingRunsByOrg(context.Context, []string) (map[string]int, error) {
	return nil, nil
}
func (m *mockReportStore) CountAIModelCallsByOrg(context.Context, string, time.Time, time.Time) (int64, error) {
	return 0, nil
}
func (m *mockReportStore) SetProjectOrgID(context.Context, string, string) error { return nil }
func (m *mockReportStore) UpsertUsageRecord(context.Context, *billing.UsageRecord) error {
	return nil
}
func (m *mockReportStore) GetOrgDailyUsage(context.Context, string, time.Time) ([]billing.UsageRecord, error) {
	return nil, nil
}
func (m *mockReportStore) GetProjectUsageForPeriod(context.Context, string, time.Time, time.Time) ([]billing.UsageRecord, error) {
	return nil, nil
}
func (m *mockReportStore) SumOrgPeriodSpend(context.Context, string, time.Time) (int64, error) {
	return 0, nil
}
func (m *mockReportStore) GetProjectBudget(context.Context, string) (int64, string, error) {
	return -1, "", nil
}
func (m *mockReportStore) SetProjectBudget(context.Context, string, int64, string) error { return nil }
func (m *mockReportStore) GetProjectPeriodSpend(context.Context, string, time.Time) (int64, error) {
	return 0, nil
}
func (m *mockReportStore) UpdateAnomalyThresholds(context.Context, string, float64, float64) error {
	return nil
}
func (m *mockReportStore) UpdatePaymentStatus(context.Context, string, string, *time.Time) error {
	return nil
}
func (m *mockReportStore) ListOrgsInGracePeriod(context.Context) ([]billing.OrgSubscription, error) {
	return nil, nil
}
func (m *mockReportStore) ListStaleSubscriptions(context.Context) ([]billing.OrgSubscription, error) {
	return nil, nil
}
func (m *mockReportStore) IsProjectSuspended(context.Context, string) (bool, error) {
	return false, nil
}
func (m *mockReportStore) SuspendExcessProjects(context.Context, string, int) (int, error) {
	return 0, nil
}

func (m *mockReportStore) HasSentUsageReport(context.Context, string, time.Time) (bool, error) {
	return false, nil
}

func (m *mockReportStore) RecordSentUsageReport(context.Context, string, time.Time) error {
	return nil
}

func (m *mockReportStore) UpdateMonthlyUsageEmail(context.Context, string, bool) error {
	return nil
}

type mockResendAPI struct {
	sent []*resend.SendEmailRequest
}

func (m *mockResendAPI) SendWithContext(_ context.Context, params *resend.SendEmailRequest) (*resend.SendEmailResponse, error) {
	m.sent = append(m.sent, params)
	return &resend.SendEmailResponse{Id: "msg-123"}, nil
}

func TestUsageReportEmailer_SendsForEndedPeriod(t *testing.T) {
	t.Parallel()

	yesterday := time.Now().UTC().Add(-24 * time.Hour).Truncate(24 * time.Hour)
	periodStart := yesterday.AddDate(0, -1, 0)

	store := &mockReportStore{
		orgIDs: []string{"org-1"},
		subscriptions: map[string]*billing.OrgSubscription{
			"org-1": {
				OrgID:              "org-1",
				PlanTier:           "starter",
				Status:             "active",
				MonthlyUsageEmail:  true,
				CurrentPeriodStart: &periodStart,
				CurrentPeriodEnd:   &yesterday,
			},
		},
		adminEmails: map[string][]string{
			"org-1": {"admin@example.com"},
		},
	}

	emailAPI := &mockResendAPI{}
	emailer := NewUsageReportEmailer(store, emailAPI, "billing@test.dev", time.Hour)
	emailer.checkAndSend(context.Background())

	if len(emailAPI.sent) != 1 {
		t.Fatalf("expected 1 email sent, got %d", len(emailAPI.sent))
	}

	msg := emailAPI.sent[0]
	if msg.To[0] != "admin@example.com" {
		t.Errorf("expected recipient admin@example.com, got %s", msg.To[0])
	}
	if len(msg.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(msg.Attachments))
	}
	if msg.Attachments[0].Filename == "" {
		t.Error("attachment filename should not be empty")
	}
}

func TestUsageReportEmailer_SkipsFreePlan(t *testing.T) {
	t.Parallel()

	yesterday := time.Now().UTC().Add(-24 * time.Hour).Truncate(24 * time.Hour)
	periodStart := yesterday.AddDate(0, -1, 0)

	store := &mockReportStore{
		orgIDs: []string{"org-free"},
		subscriptions: map[string]*billing.OrgSubscription{
			"org-free": {
				OrgID:              "org-free",
				PlanTier:           "free",
				Status:             "active",
				CurrentPeriodStart: &periodStart,
				CurrentPeriodEnd:   &yesterday,
			},
		},
		adminEmails: map[string][]string{
			"org-free": {"admin@example.com"},
		},
	}

	emailAPI := &mockResendAPI{}
	emailer := NewUsageReportEmailer(store, emailAPI, "billing@test.dev", time.Hour)
	emailer.checkAndSend(context.Background())

	if len(emailAPI.sent) != 0 {
		t.Fatalf("expected 0 emails for free plan, got %d", len(emailAPI.sent))
	}
}

func TestUsageReportEmailer_SkipsFuturePeriodEnd(t *testing.T) {
	t.Parallel()

	tomorrow := time.Now().UTC().Add(24 * time.Hour).Truncate(24 * time.Hour)
	periodStart := tomorrow.AddDate(0, -1, 0)

	store := &mockReportStore{
		orgIDs: []string{"org-1"},
		subscriptions: map[string]*billing.OrgSubscription{
			"org-1": {
				OrgID:              "org-1",
				PlanTier:           "starter",
				Status:             "active",
				CurrentPeriodStart: &periodStart,
				CurrentPeriodEnd:   &tomorrow,
			},
		},
	}

	emailAPI := &mockResendAPI{}
	emailer := NewUsageReportEmailer(store, emailAPI, "billing@test.dev", time.Hour)
	emailer.checkAndSend(context.Background())

	if len(emailAPI.sent) != 0 {
		t.Fatalf("expected 0 emails for future period end, got %d", len(emailAPI.sent))
	}
}
