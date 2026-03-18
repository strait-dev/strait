package billing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

const (
	// defaultReferralCreditMicro is the credit amount for a successful referral (10 USD).
	defaultReferralCreditMicro int64 = 10_000_000

	// referralCodeLength is the byte length used to generate referral codes (16 hex chars).
	referralCodeLength = 8
)

// ReferralStatus represents the state of a referral.
type ReferralStatus string

const (
	ReferralStatusPending   ReferralStatus = "pending"
	ReferralStatusActivated ReferralStatus = "activated"
	ReferralStatusExpired   ReferralStatus = "expired"
)

// Referral represents a referral record.
type Referral struct {
	ID             string         `json:"id"`
	ReferrerOrgID  string         `json:"referrer_org_id"`
	ReferralCode   string         `json:"referral_code"`
	ReferredEmail  string         `json:"referred_email,omitempty"`
	ReferredOrgID  string         `json:"referred_org_id,omitempty"`
	Status         ReferralStatus `json:"status"`
	CreditMicrousd int64          `json:"credit_microusd"`
	ActivatedAt    *time.Time     `json:"activated_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

// ReferralStore defines data access operations for referrals.
type ReferralStore interface {
	CreateReferral(ctx context.Context, referral *Referral) error
	GetReferralByCode(ctx context.Context, code string) (*Referral, error)
	ListReferralsByOrg(ctx context.Context, orgID string) ([]Referral, error)
	ActivateReferral(ctx context.Context, code string, referredOrgID string) error
}

// ReferralService handles referral code generation and activation.
type ReferralService struct {
	store ReferralStore
}

// NewReferralService creates a new referral service.
func NewReferralService(store ReferralStore) *ReferralService {
	return &ReferralService{store: store}
}

// GenerateCode creates a new referral code for the given org.
func (s *ReferralService) GenerateCode(ctx context.Context, orgID string) (*Referral, error) {
	code, err := generateReferralCode()
	if err != nil {
		return nil, fmt.Errorf("generating referral code: %w", err)
	}

	referral := &Referral{
		ReferrerOrgID:  orgID,
		ReferralCode:   code,
		Status:         ReferralStatusPending,
		CreditMicrousd: defaultReferralCreditMicro,
		CreatedAt:      time.Now().UTC(),
	}

	if err := s.store.CreateReferral(ctx, referral); err != nil {
		return nil, fmt.Errorf("creating referral: %w", err)
	}

	return referral, nil
}

// ActivateReferral marks a referral as activated by the referred org.
func (s *ReferralService) ActivateReferral(ctx context.Context, code string, referredOrgID string) (*Referral, error) {
	referral, err := s.store.GetReferralByCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("looking up referral code: %w", err)
	}

	if referral.Status != ReferralStatusPending {
		return nil, fmt.Errorf("referral code %q has already been used", code)
	}

	if referral.ReferrerOrgID == referredOrgID {
		return nil, fmt.Errorf("cannot use own referral code")
	}

	if err := s.store.ActivateReferral(ctx, code, referredOrgID); err != nil {
		return nil, fmt.Errorf("activating referral: %w", err)
	}

	referral.Status = ReferralStatusActivated
	referral.ReferredOrgID = referredOrgID
	now := time.Now().UTC()
	referral.ActivatedAt = &now

	return referral, nil
}

// ListReferrals returns all referrals for an org.
func (s *ReferralService) ListReferrals(ctx context.Context, orgID string) ([]Referral, error) {
	referrals, err := s.store.ListReferralsByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing referrals: %w", err)
	}
	return referrals, nil
}

func generateReferralCode() (string, error) {
	b := make([]byte, referralCodeLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}
