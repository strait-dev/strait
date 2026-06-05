package scheduler

import (
	"context"
	"testing"
	"time"

	"strait/internal/billing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

type mockContractExpiryInvalidator struct {
	orgs []string
}

func (m *mockContractExpiryInvalidator) InvalidateOrgCache(orgID string) {
	m.orgs = append(m.orgs, orgID)
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
	require.NotEmpty(t,
		emails.sent,
	)
	assert.True(t, emails.
		sent[0].
		autoRenew,
	)
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
	require.True(t, found)
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
	require.Len(t, emails.
		sent, 1)
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
	require.Len(t, emailsA.
		sent, 1,
	)
	require.Empty(t, emailsB.
		sent,
	)
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
	require.False(t, len(store.restricted) !=
		1 || store.restricted[0] !=

		"org-expired")
}

func TestContractExpiryChecker_InvalidatesOrgCacheAfterRestriction(t *testing.T) {
	t.Parallel()

	store := &mockContractExpiryStore{
		expired: []billing.EnterpriseContract{
			{OrgID: "org-expired", ContractEndDate: time.Now().Add(-time.Hour), AutoRenew: false},
			{OrgID: "org-stale", ContractEndDate: time.Now().Add(-time.Hour), AutoRenew: false},
		},
		restrictOK: map[string]bool{"org-expired": true, "org-stale": false},
	}
	invalidator := &mockContractExpiryInvalidator{}
	checker := NewContractExpiryChecker(store, nil, time.Hour).WithOrgCacheInvalidator(invalidator)
	checker.check(context.Background())
	require.False(t, len(invalidator.
		orgs) !=
		1 || invalidator.orgs[0] !=

		"org-expired")
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
	require.Empty(t, store.
		restricted)
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
		assert.True(t, call.
			autoRenew)
	}
}

func TestContractExpiryChecker_NoContracts(t *testing.T) {
	t.Parallel()

	store := &mockContractExpiryStore{}
	emails := &mockContractEmailSender{}
	checker := NewContractExpiryChecker(store, emails, time.Hour)
	checker.check(context.Background())
	require.Empty(t, emails.
		sent)
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
	require.Empty(t, store.
		claims,
	)
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
	require.Empty(t, emails.
		sent)
}
