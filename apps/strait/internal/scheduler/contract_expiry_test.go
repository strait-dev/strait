package scheduler

import (
	"context"
	"testing"
	"time"

	"strait/internal/billing"
)

type mockContractExpiryStore struct {
	contracts30 []billing.EnterpriseContract
	contracts7  []billing.EnterpriseContract
	expired     []billing.EnterpriseContract
	adminEmails map[string][]string
	restricted  []string
	restrictOK  map[string]bool
	listErr     error
	claims      map[string]bool
}

func (m *mockContractExpiryStore) ListExpiringContracts(_ context.Context, withinDays int) ([]billing.EnterpriseContract, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	if withinDays <= 7 {
		return m.contracts7, nil
	}
	return m.contracts30, nil
}

func (m *mockContractExpiryStore) ListExpiredContracts(context.Context) ([]billing.EnterpriseContract, error) {
	return m.expired, nil
}

func (m *mockContractExpiryStore) ListOrgAdminEmails(_ context.Context, orgID string) ([]string, error) {
	return m.adminEmails[orgID], nil
}

func (m *mockContractExpiryStore) UpdatePaymentStatus(_ context.Context, orgID string, status string, _ *time.Time) error {
	if status == "restricted" {
		m.restricted = append(m.restricted, orgID)
	}
	return nil
}

func (m *mockContractExpiryStore) RestrictExpiredContractIfCurrent(_ context.Context, orgID string, _ time.Time) (bool, error) {
	if m.restrictOK != nil && !m.restrictOK[orgID] {
		return false, nil
	}
	m.restricted = append(m.restricted, orgID)
	return true, nil
}

func (m *mockContractExpiryStore) ClaimContractReminderSend(_ context.Context, orgID string, contractEndDate time.Time, reminderWindowDays int) (bool, error) {
	if m.claims == nil {
		m.claims = make(map[string]bool)
	}
	key := contractReminderKey(billing.EnterpriseContract{OrgID: orgID, ContractEndDate: contractEndDate}, reminderWindowDays)
	if m.claims[key] {
		return false, nil
	}
	m.claims[key] = true
	return true, nil
}

type mockContractEmailSender struct {
	sent []contractReminderCall
}

type contractReminderCall struct {
	to            []string
	endDate       string
	autoRenew     bool
	daysRemaining int
}

func (m *mockContractEmailSender) SendEnterpriseContractReminder(_ context.Context, to []string, endDate string, autoRenew bool, daysRemaining int) {
	m.sent = append(m.sent, contractReminderCall{
		to:            to,
		endDate:       endDate,
		autoRenew:     autoRenew,
		daysRemaining: daysRemaining,
	})
}

func TestContractExpiryChecker_SendsReminderForExpiringContract(t *testing.T) {
	t.Parallel()

	endDate := time.Now().Add(25 * 24 * time.Hour)
	store := &mockContractExpiryStore{
		contracts30: []billing.EnterpriseContract{
			{OrgID: "org-1", EnterpriseTier: "enterprise_starter", ContractEndDate: endDate, AutoRenew: true},
		},
		adminEmails: map[string][]string{
			"org-1": {"admin@example.com"},
		},
	}
	emails := &mockContractEmailSender{}
	checker := NewContractExpiryChecker(store, emails, time.Hour)
	checker.check(context.Background())

	if len(emails.sent) == 0 {
		t.Fatal("expected at least one reminder to be sent")
	}
	if !emails.sent[0].autoRenew {
		t.Error("expected auto-renew reminder, got expiry warning")
	}
}

func TestContractExpiryChecker_Sends7DayReminder(t *testing.T) {
	t.Parallel()

	endDate := time.Now().Add(5 * 24 * time.Hour)
	store := &mockContractExpiryStore{
		contracts7: []billing.EnterpriseContract{
			{OrgID: "org-1", EnterpriseTier: "enterprise_growth", ContractEndDate: endDate, AutoRenew: false},
		},
		adminEmails: map[string][]string{
			"org-1": {"admin@example.com"},
		},
	}
	emails := &mockContractEmailSender{}
	checker := NewContractExpiryChecker(store, emails, time.Hour)
	checker.check(context.Background())

	found := false
	for _, call := range emails.sent {
		if !call.autoRenew {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 7-day expiry warning to be sent")
	}
}

func TestContractExpiryChecker_DoesNotSend30And7DayReminderSameTick(t *testing.T) {
	t.Parallel()

	endDate := time.Now().Add(5 * 24 * time.Hour)
	contract := billing.EnterpriseContract{OrgID: "org-1", EnterpriseTier: "enterprise_growth", ContractEndDate: endDate, AutoRenew: false}
	store := &mockContractExpiryStore{
		contracts30: []billing.EnterpriseContract{contract},
		contracts7:  []billing.EnterpriseContract{contract},
		adminEmails: map[string][]string{"org-1": {"admin@example.com"}},
	}
	emails := &mockContractEmailSender{}
	checker := NewContractExpiryChecker(store, emails, time.Hour)
	checker.check(context.Background())

	if len(emails.sent) != 1 {
		t.Fatalf("expected one reminder for same 5-day contract, got %d", len(emails.sent))
	}
}

func TestContractExpiryChecker_DurableClaimPreventsDuplicateReminder(t *testing.T) {
	t.Parallel()

	endDate := time.Now().Add(25 * 24 * time.Hour)
	contract := billing.EnterpriseContract{OrgID: "org-claimed", EnterpriseTier: "enterprise_starter", ContractEndDate: endDate, AutoRenew: true}
	store := &mockContractExpiryStore{
		contracts30: []billing.EnterpriseContract{contract},
		adminEmails: map[string][]string{"org-claimed": {"admin@example.com"}},
	}
	emailsA := &mockContractEmailSender{}
	checkerA := NewContractExpiryChecker(store, emailsA, time.Hour)
	checkerA.check(context.Background())

	emailsB := &mockContractEmailSender{}
	checkerB := NewContractExpiryChecker(store, emailsB, time.Hour)
	checkerB.check(context.Background())

	if len(emailsA.sent) != 1 {
		t.Fatalf("first checker sent %d reminders, want 1", len(emailsA.sent))
	}
	if len(emailsB.sent) != 0 {
		t.Fatalf("second checker sent %d reminders after durable claim, want 0", len(emailsB.sent))
	}
}

func TestContractExpiryChecker_RestrictsExpiredNonRenewingContract(t *testing.T) {
	t.Parallel()

	store := &mockContractExpiryStore{
		expired: []billing.EnterpriseContract{
			{OrgID: "org-expired", ContractEndDate: time.Now().Add(-time.Hour), AutoRenew: false},
			{OrgID: "org-renewing", ContractEndDate: time.Now().Add(-time.Hour), AutoRenew: true},
		},
	}
	checker := NewContractExpiryChecker(store, nil, time.Hour)
	checker.check(context.Background())

	if len(store.restricted) != 1 || store.restricted[0] != "org-expired" {
		t.Fatalf("restricted orgs = %v, want [org-expired]", store.restricted)
	}
}

func TestContractExpiryChecker_SkipsStaleExpiredContractRestriction(t *testing.T) {
	t.Parallel()

	store := &mockContractExpiryStore{
		expired: []billing.EnterpriseContract{
			{OrgID: "org-renewed", ContractEndDate: time.Now().Add(-time.Hour), AutoRenew: false},
		},
		restrictOK: map[string]bool{"org-renewed": false},
	}
	checker := NewContractExpiryChecker(store, nil, time.Hour)
	checker.check(context.Background())

	if len(store.restricted) != 0 {
		t.Fatalf("restricted orgs = %v, want none for stale contract", store.restricted)
	}
}

func TestContractExpiryChecker_AutoRenewGetsRenewalNotice(t *testing.T) {
	t.Parallel()

	endDate := time.Now().Add(20 * 24 * time.Hour)
	store := &mockContractExpiryStore{
		contracts30: []billing.EnterpriseContract{
			{OrgID: "org-1", ContractEndDate: endDate, AutoRenew: true},
		},
		adminEmails: map[string][]string{
			"org-1": {"admin@example.com"},
		},
	}
	emails := &mockContractEmailSender{}
	checker := NewContractExpiryChecker(store, emails, time.Hour)
	checker.check(context.Background())

	for _, call := range emails.sent {
		if !call.autoRenew {
			t.Error("auto-renew contract should get renewal notice, not expiry warning")
		}
	}
}

func TestContractExpiryChecker_NoContracts(t *testing.T) {
	t.Parallel()

	store := &mockContractExpiryStore{}
	emails := &mockContractEmailSender{}
	checker := NewContractExpiryChecker(store, emails, time.Hour)
	checker.check(context.Background())

	if len(emails.sent) != 0 {
		t.Fatalf("expected 0 emails, got %d", len(emails.sent))
	}
}

func TestContractExpiryChecker_NilEmailSender(t *testing.T) {
	t.Parallel()

	endDate := time.Now().Add(20 * 24 * time.Hour)
	store := &mockContractExpiryStore{
		contracts30: []billing.EnterpriseContract{
			{OrgID: "org-1", ContractEndDate: endDate, AutoRenew: true},
		},
		adminEmails: map[string][]string{
			"org-1": {"admin@example.com"},
		},
	}
	checker := NewContractExpiryChecker(store, nil, time.Hour)
	// Should not panic.
	checker.check(context.Background())
	if len(store.claims) != 0 {
		t.Fatalf("durable claims = %d, want 0 when email sender is nil", len(store.claims))
	}
}

func TestContractExpiryChecker_NoAdminEmails(t *testing.T) {
	t.Parallel()

	endDate := time.Now().Add(20 * 24 * time.Hour)
	store := &mockContractExpiryStore{
		contracts30: []billing.EnterpriseContract{
			{OrgID: "org-no-admins", ContractEndDate: endDate, AutoRenew: true},
		},
		adminEmails: map[string][]string{
			"org-no-admins": {},
		},
	}
	emails := &mockContractEmailSender{}
	checker := NewContractExpiryChecker(store, emails, time.Hour)
	checker.check(context.Background())

	if len(emails.sent) != 0 {
		t.Fatalf("expected 0 emails for org with no admin emails, got %d", len(emails.sent))
	}
}
