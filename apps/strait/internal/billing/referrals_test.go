package billing

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type mockReferralStore struct {
	referrals map[string]*Referral
}

func newMockReferralStore() *mockReferralStore {
	return &mockReferralStore{
		referrals: make(map[string]*Referral),
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

func (m *mockReferralStore) ActivateReferral(_ context.Context, code string, referredOrgID string) error {
	r, ok := m.referrals[code]
	if !ok {
		return fmt.Errorf("referral not found")
	}
	r.Status = ReferralStatusActivated
	r.ReferredOrgID = referredOrgID
	now := time.Now().UTC()
	r.ActivatedAt = &now
	return nil
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

	activated, err := svc.ActivateReferral(context.Background(), referral.ReferralCode, "org-2")
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

	_, err = svc.ActivateReferral(context.Background(), referral.ReferralCode, "org-1")
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

	_, err = svc.ActivateReferral(context.Background(), referral.ReferralCode, "org-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = svc.ActivateReferral(context.Background(), referral.ReferralCode, "org-3")
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
