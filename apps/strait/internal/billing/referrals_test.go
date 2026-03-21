package billing

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

type mockReferralStore struct {
	referrals       map[string]*Referral
	activatedEmails map[string]bool // tracks emails that have been activated
}

func newMockReferralStore() *mockReferralStore {
	return &mockReferralStore{
		referrals:       make(map[string]*Referral),
		activatedEmails: make(map[string]bool),
	}
}

func (m *mockReferralStore) CreateReferral(_ context.Context, referral *Referral) error {
	if _, exists := m.referrals[referral.ReferralCode]; exists {
		return fmt.Errorf("duplicate referral code")
	}
	m.referrals[referral.ReferralCode] = referral
	return nil
}

func (m *mockReferralStore) GetReferralByCode(_ context.Context, code string) (*Referral, error) {
	r, ok := m.referrals[code]
	if !ok {
		return nil, fmt.Errorf("referral not found")
	}
	return r, nil
}

func (m *mockReferralStore) ListReferralsByOrg(_ context.Context, orgID string) ([]Referral, error) {
	var result []Referral
	for _, r := range m.referrals {
		if r.ReferrerOrgID == orgID {
			result = append(result, *r)
		}
	}
	return result, nil
}

func (m *mockReferralStore) ActivateReferral(_ context.Context, code, referredOrgID, referredEmail string, expiresAt time.Time) error {
	r, ok := m.referrals[code]
	if !ok {
		return fmt.Errorf("referral not found")
	}
	if referredEmail != "" && m.activatedEmails[referredEmail] {
		return fmt.Errorf("duplicate referred email")
	}
	r.Status = ReferralStatusActivated
	r.ReferredOrgID = referredOrgID
	r.ReferredEmail = referredEmail
	now := time.Now().UTC()
	r.ActivatedAt = &now
	r.ExpiresAt = &expiresAt
	if referredEmail != "" {
		m.activatedEmails[referredEmail] = true
	}
	return nil
}

func (m *mockReferralStore) CountActivatedReferralsInYear(_ context.Context, orgID string) (int, error) {
	count := 0
	oneYearAgo := time.Now().UTC().AddDate(-1, 0, 0)
	for _, r := range m.referrals {
		if r.ReferrerOrgID == orgID && r.Status == ReferralStatusActivated && r.ActivatedAt != nil && r.ActivatedAt.After(oneYearAgo) {
			count++
		}
	}
	return count, nil
}

func (m *mockReferralStore) GetPendingReferralByReferredOrg(_ context.Context, referredOrgID string) (*Referral, error) {
	for _, r := range m.referrals {
		if r.ReferredOrgID == referredOrgID && r.Status == ReferralStatusPending {
			return r, nil
		}
	}
	return nil, nil
}

func (m *mockReferralStore) ExpireOldReferrals(_ context.Context) (int64, error) {
	var count int64
	now := time.Now().UTC()
	for _, r := range m.referrals {
		if r.Status == ReferralStatusActivated && r.ExpiresAt != nil && r.ExpiresAt.Before(now) {
			r.Status = ReferralStatusExpired
			count++
		}
	}
	return count, nil
}

func TestReferralService_GenerateCode(t *testing.T) {
	store := newMockReferralStore()
	svc := NewReferralService(store)

	referral, err := svc.GenerateCode(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if referral.ReferralCode == "" {
		t.Fatal("expected non-empty referral code")
	}
	if referral.ReferrerOrgID != "org-1" {
		t.Errorf("expected referrer_org_id org-1, got %s", referral.ReferrerOrgID)
	}
	if referral.Status != ReferralStatusPending {
		t.Errorf("expected status pending, got %s", referral.Status)
	}
	if referral.CreditMicrousd != defaultReferralCreditMicro {
		t.Errorf("expected credit %d, got %d", defaultReferralCreditMicro, referral.CreditMicrousd)
	}
}

func TestReferralService_ActivateReferral(t *testing.T) {
	store := newMockReferralStore()
	svc := NewReferralService(store)

	referral, err := svc.GenerateCode(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("unexpected error generating code: %v", err)
	}

	activated, err := svc.ActivateReferral(context.Background(), referral.ReferralCode, "org-2", "user@example.com")
	if err != nil {
		t.Fatalf("unexpected error activating: %v", err)
	}
	if activated.Status != ReferralStatusActivated {
		t.Errorf("expected status activated, got %s", activated.Status)
	}
	if activated.ReferredOrgID != "org-2" {
		t.Errorf("expected referred_org_id org-2, got %s", activated.ReferredOrgID)
	}
}

func TestReferralService_ActivateReferral_SelfReferral(t *testing.T) {
	store := newMockReferralStore()
	svc := NewReferralService(store)

	referral, err := svc.GenerateCode(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = svc.ActivateReferral(context.Background(), referral.ReferralCode, "org-1", "user@example.com")
	if err == nil {
		t.Fatal("expected error for self-referral")
	}
}

func TestReferralService_ActivateReferral_AlreadyUsed(t *testing.T) {
	store := newMockReferralStore()
	svc := NewReferralService(store)

	referral, err := svc.GenerateCode(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = svc.ActivateReferral(context.Background(), referral.ReferralCode, "org-2", "user@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = svc.ActivateReferral(context.Background(), referral.ReferralCode, "org-3", "other@example.com")
	if err == nil {
		t.Fatal("expected error for already-used referral code")
	}
}

func TestReferralService_ListReferrals(t *testing.T) {
	store := newMockReferralStore()
	svc := NewReferralService(store)

	_, err := svc.GenerateCode(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = svc.GenerateCode(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	referrals, err := svc.ListReferrals(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(referrals) != 2 {
		t.Errorf("expected 2 referrals, got %d", len(referrals))
	}
}

func TestGenerateCode_AtYearlyCap_ReturnsError(t *testing.T) {
	store := newMockReferralStore()
	svc := NewReferralService(store)

	// Create and activate MaxActivatedPerYear referrals.
	for i := range MaxActivatedPerYear {
		ref, err := svc.GenerateCode(context.Background(), "org-1")
		if err != nil {
			t.Fatalf("unexpected error generating code %d: %v", i, err)
		}
		_, err = svc.ActivateReferral(context.Background(), ref.ReferralCode, fmt.Sprintf("org-ref-%d", i), fmt.Sprintf("user%d@example.com", i))
		if err != nil {
			t.Fatalf("unexpected error activating code %d: %v", i, err)
		}
	}

	// The 11th should be blocked.
	_, err := svc.GenerateCode(context.Background(), "org-1")
	if err == nil {
		t.Fatal("expected error for yearly cap reached")
	}
	if !strings.Contains(err.Error(), "yearly referral cap reached") {
		t.Errorf("expected yearly cap error, got: %v", err)
	}
}

func TestGenerateCode_BelowCap_Succeeds(t *testing.T) {
	store := newMockReferralStore()
	svc := NewReferralService(store)

	// Create and activate 9 referrals.
	for i := range MaxActivatedPerYear - 1 {
		ref, err := svc.GenerateCode(context.Background(), "org-1")
		if err != nil {
			t.Fatalf("unexpected error generating code %d: %v", i, err)
		}
		_, err = svc.ActivateReferral(context.Background(), ref.ReferralCode, fmt.Sprintf("org-ref-%d", i), fmt.Sprintf("user%d@example.com", i))
		if err != nil {
			t.Fatalf("unexpected error activating code %d: %v", i, err)
		}
	}

	// The 10th generation should succeed.
	ref, err := svc.GenerateCode(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("expected success for 10th code, got error: %v", err)
	}
	if ref.ReferralCode == "" {
		t.Fatal("expected non-empty referral code")
	}
}

func TestActivateReferral_DuplicateEmail_Blocked(t *testing.T) {
	store := newMockReferralStore()
	svc := NewReferralService(store)

	ref1, err := svc.GenerateCode(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = svc.ActivateReferral(context.Background(), ref1.ReferralCode, "org-2", "dup@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ref2, err := svc.GenerateCode(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = svc.ActivateReferral(context.Background(), ref2.ReferralCode, "org-3", "dup@example.com")
	if err == nil {
		t.Fatal("expected error for duplicate email")
	}
}

func TestActivateReferral_UniqueEmail_Succeeds(t *testing.T) {
	store := newMockReferralStore()
	svc := NewReferralService(store)

	ref1, err := svc.GenerateCode(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = svc.ActivateReferral(context.Background(), ref1.ReferralCode, "org-2", "alice@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ref2, err := svc.GenerateCode(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	activated, err := svc.ActivateReferral(context.Background(), ref2.ReferralCode, "org-3", "bob@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if activated.Status != ReferralStatusActivated {
		t.Errorf("expected status activated, got %s", activated.Status)
	}
}

func TestActivateReferral_SetsExpiryTo90Days(t *testing.T) {
	store := newMockReferralStore()
	svc := NewReferralService(store)

	ref, err := svc.GenerateCode(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	activated, err := svc.ActivateReferral(context.Background(), ref.ReferralCode, "org-2", "user@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if activated.ExpiresAt == nil {
		t.Fatal("expected expires_at to be set")
	}

	expectedExpiry := time.Now().UTC().AddDate(0, 0, ReferralExpiryDays)
	diff := activated.ExpiresAt.Sub(expectedExpiry)
	if diff < -time.Minute || diff > time.Minute {
		t.Errorf("expected expires_at ~%v, got %v (diff %v)", expectedExpiry, *activated.ExpiresAt, diff)
	}
}

func TestActivateReferral_CreditAmount_Is10Dollars(t *testing.T) {
	store := newMockReferralStore()
	svc := NewReferralService(store)

	ref, err := svc.GenerateCode(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	activated, err := svc.ActivateReferral(context.Background(), ref.ReferralCode, "org-2", "user@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 10 USD = 10_000_000 microusd
	if activated.CreditMicrousd != 10_000_000 {
		t.Errorf("expected credit 10_000_000 (10 USD), got %d", activated.CreditMicrousd)
	}
}

func TestAutoActivation_FirstProjectCreated_ActivatesReferral(t *testing.T) {
	store := newMockReferralStore()
	svc := NewReferralService(store)

	// Generate a referral code from org-1 and assign it to org-2 (pending).
	ref, err := svc.GenerateCode(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Simulate the referred org being set (e.g., during signup) but not yet activated.
	store.referrals[ref.ReferralCode].ReferredOrgID = "org-2"

	// Auto-activate for org-2 (first project scenario).
	err = svc.AutoActivateReferral(context.Background(), "org-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the referral is now activated.
	r := store.referrals[ref.ReferralCode]
	if r.Status != ReferralStatusActivated {
		t.Errorf("expected status activated, got %s", r.Status)
	}
}

func TestAutoActivation_SecondProject_NoDoubleActivation(t *testing.T) {
	store := newMockReferralStore()
	svc := NewReferralService(store)

	// Generate and auto-activate a referral.
	ref, err := svc.GenerateCode(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	store.referrals[ref.ReferralCode].ReferredOrgID = "org-2"

	err = svc.AutoActivateReferral(context.Background(), "org-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second call should be a no-op (no pending referral left).
	err = svc.AutoActivateReferral(context.Background(), "org-2")
	if err != nil {
		t.Fatalf("unexpected error on second auto-activation: %v", err)
	}

	// Only one referral should be activated.
	activatedCount := 0
	for _, r := range store.referrals {
		if r.Status == ReferralStatusActivated && r.ReferredOrgID == "org-2" {
			activatedCount++
		}
	}
	if activatedCount != 1 {
		t.Errorf("expected exactly 1 activated referral, got %d", activatedCount)
	}
}

func TestAutoActivation_NoReferral_Skipped(t *testing.T) {
	store := newMockReferralStore()
	svc := NewReferralService(store)

	// No referral exists for org-2.
	err := svc.AutoActivateReferral(context.Background(), "org-2")
	if err != nil {
		t.Fatalf("expected no error when no referral exists, got: %v", err)
	}
}
