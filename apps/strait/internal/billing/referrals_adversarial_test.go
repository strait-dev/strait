package billing

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestReferralCode_Format verifies that generated referral codes have the expected hex format.
func TestReferralCode_Format(t *testing.T) {
	t.Parallel()

	store := newMockReferralStore()
	svc := NewReferralService(store)

	ref, err := svc.GenerateCode(context.Background(), "org-fmt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// referralCodeLength is 8 bytes, hex-encoded = 16 characters.
	if len(ref.ReferralCode) != 16 {
		t.Fatalf("expected 16-char hex code, got %d chars: %q", len(ref.ReferralCode), ref.ReferralCode)
	}

	for _, c := range ref.ReferralCode {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Fatalf("expected lowercase hex character, got %q in code %q", c, ref.ReferralCode)
		}
	}
}

// TestReferralCode_Uniqueness verifies that 1000 generated codes are all unique.
func TestReferralCode_Uniqueness(t *testing.T) {
	t.Parallel()

	store := newMockReferralStore()
	svc := NewReferralService(store)

	seen := make(map[string]struct{}, 1000)
	for i := range 1000 {
		ref, err := svc.GenerateCode(context.Background(), "org-uniq")
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		if _, exists := seen[ref.ReferralCode]; exists {
			t.Fatalf("duplicate code generated at iteration %d: %q", i, ref.ReferralCode)
		}
		seen[ref.ReferralCode] = struct{}{}
	}
}

// TestActivateReferral_SelfReferral_Adversarial verifies that an org cannot use its own referral code.
func TestActivateReferral_SelfReferral_Adversarial(t *testing.T) {
	t.Parallel()

	store := newMockReferralStore()
	svc := NewReferralService(store)

	ref, err := svc.GenerateCode(context.Background(), "org-self")
	if err != nil {
		t.Fatalf("unexpected error generating code: %v", err)
	}

	_, err = svc.ActivateReferral(context.Background(), ref.ReferralCode, "org-self", "self@example.com")
	if err == nil {
		t.Fatal("expected error for self-referral, got nil")
	}
	if got := err.Error(); got != "cannot use own referral code" {
		t.Fatalf("expected 'cannot use own referral code', got %q", got)
	}
}

// TestActivateReferral_AlreadyActivated_Adversarial verifies that a double activation is rejected.
func TestActivateReferral_AlreadyActivated_Adversarial(t *testing.T) {
	t.Parallel()

	store := newMockReferralStore()
	svc := NewReferralService(store)

	ref, err := svc.GenerateCode(context.Background(), "org-dbl")
	if err != nil {
		t.Fatalf("unexpected error generating code: %v", err)
	}

	// First activation should succeed.
	_, err = svc.ActivateReferral(context.Background(), ref.ReferralCode, "org-dbl-target", "user@example.com")
	if err != nil {
		t.Fatalf("unexpected error on first activation: %v", err)
	}

	// Second activation should fail because status is no longer pending.
	_, err = svc.ActivateReferral(context.Background(), ref.ReferralCode, "org-dbl-third", "other@example.com")
	if err == nil {
		t.Fatal("expected error for double activation, got nil")
	}
}

// TestActivateReferral_ExpiredCode verifies that an expired referral cannot be activated.
func TestActivateReferral_ExpiredCode(t *testing.T) {
	t.Parallel()

	store := newMockReferralStore()
	svc := NewReferralService(store)

	// Manually insert an expired referral.
	store.referrals["expired-adv-code"] = &Referral{
		ReferrerOrgID:  "org-exp",
		ReferralCode:   "expired-adv-code",
		Status:         ReferralStatusExpired,
		CreditMicrousd: defaultReferralCreditMicro,
		CreatedAt:      time.Now().Add(-180 * 24 * time.Hour),
	}

	_, err := svc.ActivateReferral(context.Background(), "expired-adv-code", "org-exp-target", "user@example.com")
	if err == nil {
		t.Fatal("expected error for expired referral, got nil")
	}
}

// TestActivateReferral_EmptyCode verifies that an empty code returns an error.
func TestActivateReferral_EmptyCode(t *testing.T) {
	t.Parallel()

	store := newMockReferralStore()
	svc := NewReferralService(store)

	_, err := svc.ActivateReferral(context.Background(), "", "org-empty", "user@example.com")
	if err == nil {
		t.Fatal("expected error for empty referral code, got nil")
	}
}

// TestActivateReferral_NullByteCode verifies that null bytes in a code are handled safely.
func TestActivateReferral_NullByteCode(t *testing.T) {
	t.Parallel()

	store := newMockReferralStore()
	svc := NewReferralService(store)

	_, err := svc.ActivateReferral(context.Background(), "code\x00with\x00nulls", "org-null", "user@example.com")
	if err == nil {
		t.Fatal("expected error for null-byte referral code, got nil")
	}
}

// TestAutoActivate_NoPending verifies that AutoActivateReferral is a no-op when
// there is no pending referral.
func TestAutoActivate_NoPending(t *testing.T) {
	t.Parallel()

	store := newMockReferralStore()
	svc := NewReferralService(store)

	err := svc.AutoActivateReferral(context.Background(), "org-no-pending")
	if err != nil {
		t.Fatalf("expected nil error for no pending referral, got %v", err)
	}
}

// FuzzReferralCode fuzzes referral code lookups to ensure no panics.
func FuzzReferralCode(f *testing.F) {
	f.Add("abc123")
	f.Add("")
	f.Add("\x00\x00\x00")
	f.Add("a]b[c{d}e")

	store := newMockReferralStore()
	svc := NewReferralService(store)

	f.Fuzz(func(t *testing.T, code string) {
		// ActivateReferral should return an error for any fuzzed code, never panic.
		_, _ = svc.ActivateReferral(context.Background(), code, "org-fuzz", "fuzz@example.com")
	})
}

// TestReferralMaxCount verifies that the yearly referral cap is enforced.
func TestReferralMaxCount(t *testing.T) {
	t.Parallel()

	store := newMockReferralStore()
	svc := NewReferralService(store)

	// Generate and activate MaxActivatedPerYear referrals.
	for i := range MaxActivatedPerYear {
		ref, err := svc.GenerateCode(context.Background(), "org-max")
		if err != nil {
			t.Fatalf("unexpected error generating code %d: %v", i, err)
		}
		_, err = svc.ActivateReferral(context.Background(), ref.ReferralCode, fmt.Sprintf("org-max-ref-%d", i), fmt.Sprintf("max-user%d@example.com", i))
		if err != nil {
			t.Fatalf("unexpected error activating code %d: %v", i, err)
		}
	}

	// The next generation should fail due to the yearly cap.
	_, err := svc.GenerateCode(context.Background(), "org-max")
	if err == nil {
		t.Fatal("expected error when yearly referral cap is reached, got nil")
	}
	if got := err.Error(); got != "yearly referral cap reached" {
		t.Fatalf("expected 'yearly referral cap reached', got %q", got)
	}
}
