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

	// MaxReferralCreditsPerYear is the maximum referral credits per year (100 USD).
	MaxReferralCreditsPerYear int64 = 100_000_000

	// ReferralExpiryDays is the number of days before a referral credit expires.
	ReferralExpiryDays = 90

	// MaxActivatedPerYear is the maximum number of activated referrals per org per year.
	MaxActivatedPerYear = 10

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
	ExpiresAt      *time.Time     `json:"expires_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

// ReferralStore defines data access operations for referrals.
type ReferralStore interface {
	CreateReferral(ctx context.Context, referral *Referral) error
	GetReferralByCode(ctx context.Context, code string) (*Referral, error)
	ListReferralsByOrg(ctx context.Context, orgID string) ([]Referral, error)
	ActivateReferral(ctx context.Context, code, referredOrgID, referredEmail string, expiresAt time.Time) error
	CountActivatedReferralsInYear(ctx context.Context, orgID string) (int, error)
	GetPendingReferralByReferredOrg(ctx context.Context, referredOrgID string) (*Referral, error)
	ExpireOldReferrals(ctx context.Context) (int64, error)
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
// NOTE: The count-then-create pattern has a small TOCTOU window where concurrent
// calls could exceed MaxActivatedPerYear. The risk is low because code generation
// is a manual user action. A full fix requires transactional store operations.
func (s *ReferralService) GenerateCode(ctx context.Context, orgID string) (*Referral, error) {
	count, err := s.store.CountActivatedReferralsInYear(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("counting activated referrals: %w", err)
	}
	if count >= MaxActivatedPerYear {
		return nil, fmt.Errorf("yearly referral cap reached")
	}

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
func (s *ReferralService) ActivateReferral(ctx context.Context, code, referredOrgID, referredEmail string) (*Referral, error) {
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

	now := time.Now().UTC()
	expiresAt := now.AddDate(0, 0, ReferralExpiryDays)

	if err := s.store.ActivateReferral(ctx, code, referredOrgID, referredEmail, expiresAt); err != nil {
		return nil, fmt.Errorf("activating referral: %w", err)
	}

	referral.Status = ReferralStatusActivated
	referral.ReferredOrgID = referredOrgID
	referral.ReferredEmail = referredEmail
	referral.ActivatedAt = &now
	referral.ExpiresAt = &expiresAt

	return referral, nil
}

// CountActivatedInYear returns the number of activated referrals in the last year for an org.
func (s *ReferralService) CountActivatedInYear(ctx context.Context, orgID string) (int, error) {
	return s.store.CountActivatedReferralsInYear(ctx, orgID)
}

// AutoActivateReferral looks for a pending referral where referred_org_id matches
// the given org and activates it. This is called after the first project is created.
// If no pending referral exists, it returns nil (no-op).
func (s *ReferralService) AutoActivateReferral(ctx context.Context, orgID string) error {
	referral, err := s.store.GetPendingReferralByReferredOrg(ctx, orgID)
	if err != nil {
		return fmt.Errorf("looking up pending referral for org %s: %w", orgID, err)
	}
	if referral == nil {
		return nil
	}

	now := time.Now().UTC()
	expiresAt := now.AddDate(0, 0, ReferralExpiryDays)

	if err := s.store.ActivateReferral(ctx, referral.ReferralCode, orgID, referral.ReferredEmail, expiresAt); err != nil {
		return fmt.Errorf("auto-activating referral %s: %w", referral.ReferralCode, err)
	}

	return nil
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
