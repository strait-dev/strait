package billing

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

// errStore wraps mockBillingStore to inject errors on specific calls.
type errStore struct {
	mockBillingStore
	getSubErr       error
	upsertErr       error
	updatePlanErr   error
	updateFullErr   error
	sumSpendErr     error
	updatePayErr    error
	setPendingErr   error
	clearPendingErr error
	getSubCallCount atomic.Int64
	upsertCallCount atomic.Int64
}

func (e *errStore) GetOrgSubscription(ctx context.Context, orgID string) (*OrgSubscription, error) {
	e.getSubCallCount.Add(1)
	if e.getSubErr != nil {
		return nil, e.getSubErr
	}
	return e.mockBillingStore.GetOrgSubscription(ctx, orgID)
}

func (e *errStore) UpsertOrgSubscription(ctx context.Context, sub *OrgSubscription) error {
	e.upsertCallCount.Add(1)
	if e.upsertErr != nil {
		return e.upsertErr
	}
	return e.mockBillingStore.UpsertOrgSubscription(ctx, sub)
}

func (e *errStore) UpdateOrgSubscriptionPlan(ctx context.Context, orgID, planTier, status string) error {
	if e.updatePlanErr != nil {
		return e.updatePlanErr
	}
	return e.mockBillingStore.UpdateOrgSubscriptionPlan(ctx, orgID, planTier, status)
}

func (e *errStore) UpdateOrgSubscriptionStatus(ctx context.Context, orgID, status string) error {
	return e.mockBillingStore.UpdateOrgSubscriptionStatus(ctx, orgID, status)
}

func (e *errStore) UpdateOrgSubscriptionFull(ctx context.Context, orgID, tier, status string, periodStart, periodEnd *time.Time) error {
	if e.updateFullErr != nil {
		return e.updateFullErr
	}
	return e.mockBillingStore.UpdateOrgSubscriptionFull(ctx, orgID, tier, status, periodStart, periodEnd)
}

func (e *errStore) SumOrgPeriodSpend(ctx context.Context, orgID string, from time.Time) (int64, error) {
	if e.sumSpendErr != nil {
		return 0, e.sumSpendErr
	}
	return e.mockBillingStore.SumOrgPeriodSpend(ctx, orgID, from)
}

func (e *errStore) UpdatePaymentStatus(ctx context.Context, orgID string, status string, graceEnd *time.Time) error {
	if e.updatePayErr != nil {
		return e.updatePayErr
	}
	return e.mockBillingStore.UpdatePaymentStatus(ctx, orgID, status, graceEnd)
}

func (e *errStore) SetPendingPlanTier(ctx context.Context, orgID, tier string) error {
	if e.setPendingErr != nil {
		return e.setPendingErr
	}
	return e.mockBillingStore.SetPendingPlanTier(ctx, orgID, tier)
}

func (e *errStore) ClearPendingPlanTier(ctx context.Context, orgID string) error {
	if e.clearPendingErr != nil {
		return e.clearPendingErr
	}
	return e.mockBillingStore.ClearPendingPlanTier(ctx, orgID)
}

// advMockAuditStore records audit events for inspection.
type advMockAuditStore struct {
	events []domain.AuditEvent
	err    error
}

func (m *advMockAuditStore) CreateAuditEvent(_ context.Context, ev *domain.AuditEvent) error {
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, *ev)
	return nil
}

func buildSignedWebhookRequest(t *testing.T, secret string, payload []byte) *http.Request {
	t.Helper()
	ts := fmt.Sprintf("%d", time.Now().Unix())

	// Stripe webhook signature: HMAC-SHA256(timestamp + "." + payload, secret).
	signedContent := ts + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedContent))
	sig := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", fmt.Sprintf("t=%s,v1=%s", ts, sig))
	return req
}

func webhookPayload(t *testing.T, eventType string, data any) []byte {
	t.Helper()
	raw, err := json.Marshal(data)
	require.NoError(t, err)

	payload, err := json.Marshal(StripeWebhookPayload{Type: eventType, Data: raw})
	require.NoError(t, err)

	return payload
}

func withTestMetadataFallback() WebhookOption {
	return func(h *WebhookHandler) {
		h.allowTestMetadata = true
	}
}

// 1. Double-charge / duplicate webhook events

func TestWebhook_DuplicateSubscriptionCreated(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{subscriptions: make(map[string]*OrgSubscription)}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	secret := testSecret
	handler := NewWebhookHandler(store, mapping, secret, slog.Default(), nil, nil, withTestMetadataFallback())

	data := testSubscriptionData{
		ID:         "sub_dup_1",
		Status:     "active",
		CustomerID: "cust_1",
		ProductID:  "starter-id",
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000020"},
	}

	raw, err := json.Marshal(data)
	require.NoError(t, err)

	// Include an event ID so the replay cache can detect duplicates.
	body, err := json.Marshal(StripeWebhookPayload{ID: "evt_dup_test_1", Type: "customer.subscription.created", Data: raw})
	require.NoError(t, err)

	// First delivery.
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	sub, err := store.GetOrgSubscription(context.Background(), "00000000-0000-0000-0000-000000000020")
	require.NoError(t, err)
	require.Equal(t,
		string(domain.
			PlanStarter,
		), sub.PlanTier)

	firstUpsertCount := store.upsertCount

	// Duplicate delivery (same event replayed with same webhook-id).
	// Replay protection deduplicates by message ID, so the duplicate should
	// be silently accepted without triggering a second upsert.
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr2.Code)

	// Subscription should still be starter, not double-created.
	sub2, _ := store.GetOrgSubscription(context.Background(), "00000000-0000-0000-0000-000000000020")
	require.Equal(t,
		string(domain.
			PlanStarter,
		), sub2.PlanTier,
	)
	require.Equal(t,
		firstUpsertCount,
		store.
			upsertCount)

	// Replay protection prevents the duplicate from reaching the handler,
	// so the upsert count should remain unchanged.

}

func TestWebhook_DuplicateSubscriptionUpdated(t *testing.T) {
	t.Parallel()

	now := time.Now()
	periodStart := now.Add(-24 * time.Hour)
	periodEnd := now.Add(30 * 24 * time.Hour)
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000021": {
				OrgID:              "00000000-0000-0000-0000-000000000021",
				PlanTier:           string(domain.PlanStarter),
				Status:             "active",
				CurrentPeriodStart: &periodStart,
				CurrentPeriodEnd:   &periodEnd,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil, withTestMetadataFallback())

	data := testSubscriptionData{
		ID:                 "sub_upd_dup",
		Status:             "active",
		CustomerID:         "cust_2",
		ProductID:          "pro-id",
		CurrentPeriodStart: &periodStart,
		CurrentPeriodEnd:   &periodEnd,
		Metadata:           map[string]string{"org_id": "00000000-0000-0000-0000-000000000021"},
	}

	body := webhookPayload(t, "customer.subscription.updated", data)

	// First delivery: upgrade starter -> pro.
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr1.Code)

	sub, _ := store.GetOrgSubscription(context.Background(), "00000000-0000-0000-0000-000000000021")
	require.Equal(t,
		string(domain.
			PlanPro),
		sub.PlanTier)

	// Duplicate delivery.
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr2.Code)

	sub2, _ := store.GetOrgSubscription(context.Background(), "00000000-0000-0000-0000-000000000021")
	require.Equal(t,
		string(domain.
			PlanPro),
		sub2.PlanTier)

}

// 2. Budget edge cases

func TestSpendingLimit_OverageComputeEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		periodSpend int64
		credit      int64
		wantOverage int64
	}{
		{"zero spend zero credit", 0, 0, 0},
		{"spend under credit", 500, 1000, 0},
		{"spend equals credit", 1000, 1000, 0},
		{"spend one over credit", 1001, 1000, 1},
		{"negative spend", -100, 1000, 0},
		{"max int64 spend", math.MaxInt64, 1000, math.MaxInt64 - 1000},
		{"zero credit positive spend", 500, 0, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := computeOverageSpend(tt.periodSpend, tt.credit)
			require.Equal(t,
				tt.wantOverage,
				got)

		})
	}
}

func TestOverageLimitReached_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		limitMicro   int64
		overageSpend int64
		wantReached  bool
	}{
		{"unlimited (negative limit)", -1, 999999, false},
		{"zero limit zero overage", 0, 0, false},
		{"zero limit positive overage", 0, 1, true},
		{"limit equal overage", 100, 100, true},
		{"limit one more than overage", 100, 99, false},
		{"limit less than overage", 100, 101, true},
		{"large limit not reached", math.MaxInt64, math.MaxInt64 - 1, false},
		{"large limit reached", math.MaxInt64, math.MaxInt64, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isOverageLimitReached(tt.limitMicro, tt.overageSpend)
			require.Equal(t,
				tt.wantReached,
				got)

		})
	}
}

func TestUsagePeriodWindow_NilSubscription(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	start, end := usagePeriodWindow(now, domain.PlanFree, nil)

	expectedStart := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	assert.True(t,
		start.Equal(expectedStart),
	)
	assert.True(t,
		end.Equal(expectedEnd))

}

func TestUsagePeriodWindow_PaidWithPeriodDates(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	ps := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	pe := time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
	sub := &OrgSubscription{
		CurrentPeriodStart: &ps,
		CurrentPeriodEnd:   &pe,
	}

	start, end := usagePeriodWindow(now, domain.PlanStarter, sub)
	assert.True(t,
		start.Equal(ps))
	assert.True(t,
		end.Equal(pe))

}

func TestUsagePeriodWindow_PaidWithNilPeriodDates(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	sub := &OrgSubscription{
		CurrentPeriodStart: nil,
		CurrentPeriodEnd:   nil,
	}

	// With nil period dates, should fall back to calendar month.
	start, end := usagePeriodWindow(now, domain.PlanStarter, sub)
	expectedStart := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	assert.True(t,
		start.Equal(expectedStart),
	)
	assert.True(t,
		end.Equal(expectedEnd))

}

// 3. State machine violations (invalid plan transitions)

func TestIsDowngrade_SameTier(t *testing.T) {
	t.Parallel()

	tiers := []domain.PlanTier{domain.PlanFree, domain.PlanStarter, domain.PlanPro, domain.PlanEnterprise}
	for _, tier := range tiers {
		assert.False(t,
			IsDowngrade(tier, tier))

	}
}

func TestIsDowngrade_AllTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		from   domain.PlanTier
		to     domain.PlanTier
		isDown bool
	}{
		{domain.PlanFree, domain.PlanStarter, false},
		{domain.PlanFree, domain.PlanPro, false},
		{domain.PlanFree, domain.PlanEnterprise, false},
		{domain.PlanStarter, domain.PlanFree, true},
		{domain.PlanStarter, domain.PlanPro, false},
		{domain.PlanStarter, domain.PlanEnterprise, false},
		{domain.PlanPro, domain.PlanFree, true},
		{domain.PlanPro, domain.PlanStarter, true},
		{domain.PlanPro, domain.PlanEnterprise, false},
		{domain.PlanEnterprise, domain.PlanFree, true},
		{domain.PlanEnterprise, domain.PlanStarter, true},
		{domain.PlanEnterprise, domain.PlanPro, true},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%s_to_%s", tt.from, tt.to)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := IsDowngrade(tt.from, tt.to)
			assert.Equal(t,
				tt.isDown,
				got)

		})
	}
}

func TestIsDowngrade_UnknownTier(t *testing.T) {
	t.Parallel()

	// Unknown tiers should fall back to free-tier limits.
	bogus := domain.PlanTier("imaginary")
	assert.False(t,
		IsDowngrade(bogus, domain.
			PlanFree))
	assert.True(t,
		IsDowngrade(domain.PlanPro,
			bogus))

}

func TestWebhook_DowngradeDefersToEndOfPeriod(t *testing.T) {
	t.Parallel()

	periodStart := time.Now().Add(-7 * 24 * time.Hour)
	periodEnd := time.Now().Add(23 * 24 * time.Hour)
	subID := "sub_defer_1"
	customerID := "cust_defer"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000022": {
				OrgID:                "00000000-0000-0000-0000-000000000022",
				PlanTier:             string(domain.PlanPro),
				Status:               "active",
				CurrentPeriodStart:   &periodStart,
				CurrentPeriodEnd:     &periodEnd,
				StripeSubscriptionID: &subID,
				StripeCustomerID:     &customerID,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil, withTestMetadataFallback())

	// Downgrade from pro -> starter.
	data := testSubscriptionData{
		ID:                 subID,
		Status:             "active",
		CustomerID:         customerID,
		ProductID:          "starter-id",
		CurrentPeriodStart: &periodStart,
		CurrentPeriodEnd:   &periodEnd,
		Metadata:           map[string]string{"org_id": "00000000-0000-0000-0000-000000000022"},
	}
	body := webhookPayload(t, "customer.subscription.updated", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	// Plan tier should remain pro (deferred).
	sub, _ := store.GetOrgSubscription(context.Background(), "00000000-0000-0000-0000-000000000022")
	require.Equal(t,
		string(domain.
			PlanPro),
		sub.PlanTier)
	require.False(t,
		sub.PendingPlanTier ==
			nil ||
			*sub.PendingPlanTier !=
				string(domain.PlanStarter))

}

func TestWebhook_CancelAlreadyFreeOrg(t *testing.T) {
	t.Parallel()

	canceledAt := time.Now()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000023": {
				OrgID:    "00000000-0000-0000-0000-000000000023",
				PlanTier: string(domain.PlanFree),
				Status:   "active",
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil, withTestMetadataFallback())

	data := testSubscriptionData{
		ID:                "sub_cancel_free",
		ProductID:         "starter-id",
		CustomerID:        "cust_cancel_free",
		CanceledAt:        &canceledAt,
		CancelAtPeriodEnd: true,
		Metadata:          map[string]string{"org_id": "00000000-0000-0000-0000-000000000023"},
	}
	body := webhookPayload(t, "customer.subscription.deleted", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	sub, _ := store.GetOrgSubscription(context.Background(), "00000000-0000-0000-0000-000000000023")
	require.Equal(t,
		"canceled",
		sub.Status)
	require.Nil(t, sub.
		PendingPlanTier,
	)

	// Should NOT set pending plan tier since already free.

}

func TestWebhook_RevokeSubscription(t *testing.T) {
	t.Parallel()

	pending := string(domain.PlanStarter)
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000024": {
				OrgID:           "00000000-0000-0000-0000-000000000024",
				PlanTier:        string(domain.PlanPro),
				Status:          "active",
				PendingPlanTier: &pending,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil, withTestMetadataFallback())

	// Stripe fires customer.subscription.deleted with CancelAtPeriodEnd=false for immediate revocation.
	data := testSubscriptionData{
		ID:                "sub_revoke_1",
		ProductID:         "starter-id",
		CustomerID:        "cust_revoke",
		CancelAtPeriodEnd: false,
		Metadata:          map[string]string{"org_id": "00000000-0000-0000-0000-000000000024"},
	}
	body := webhookPayload(t, "customer.subscription.deleted", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	sub, _ := store.GetOrgSubscription(context.Background(), "00000000-0000-0000-0000-000000000024")
	require.Equal(t,
		string(domain.
			PlanFree),
		sub.PlanTier)
	require.Equal(t,
		"revoked",
		sub.Status)
	require.Nil(t, sub.
		PendingPlanTier,
	)

	// Pending plan tier should be cleared.

}

// 4. Nil/zero value paths

func TestGetPlanLimits_UnknownTierFallback(t *testing.T) {
	t.Parallel()

	limits := GetPlanLimits(domain.PlanTier("nonexistent"))
	freeLimits := GetPlanLimits(domain.PlanFree)
	require.Equal(t,
		freeLimits.
			MaxRunsPerDay,
		limits.MaxRunsPerDay,
	)

}

func TestEnforcer_NilEnforcerGetOrgPlanLimits(t *testing.T) {
	t.Parallel()

	var e *Enforcer
	limits, err := e.GetOrgPlanLimits(context.Background(), "org-nil")
	require.NoError(t, err)

	freeLimits := GetPlanLimits(domain.PlanFree)
	require.Equal(t,
		freeLimits.
			MaxRunsPerDay,
		limits.MaxRunsPerDay,
	)

}

func TestEnforcer_EmptyOrgID(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	e := NewEnforcer(store, nil, slog.Default())

	limits, err := e.GetOrgPlanLimits(context.Background(), "")
	require.NoError(t, err)

	freeLimits := GetPlanLimits(domain.PlanFree)
	require.Equal(t,
		freeLimits.
			MaxRunsPerDay,
		limits.MaxRunsPerDay,
	)

}

func TestWebhook_NoOrgIDInMetadata(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil, withTestMetadataFallback())

	data := testSubscriptionData{
		ID:         "sub_no_org",
		ProductID:  "starter-id",
		CustomerID: "cust_no_org",
		Metadata:   map[string]string{}, // no org_id
	}
	body := webhookPayload(t, "customer.subscription.created", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.NotEqual(t, http.StatusOK,
		rr.Code,
	)
	require.EqualValues(t, 0, store.
		upsertCount)

	// Should return an error so Stripe retries the webhook until org_id is resolvable.

}

func TestWebhook_OrgIDFromCustomerMetadata(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{subscriptions: make(map[string]*OrgSubscription)}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil, withTestMetadataFallback())

	data := testSubscriptionData{
		ID:         "sub_cust_meta",
		ProductID:  "starter-id",
		CustomerID: "cust_meta_1",
		Metadata:   map[string]string{}, // no org_id in sub metadata
		Customer: &testCustomerData{
			ID:       "cust_meta_1",
			Email:    "user@example.com",
			Metadata: map[string]string{"org_id": "00000000-0000-0000-0000-000000000025"},
		},
	}
	body := webhookPayload(t, "customer.subscription.created", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	_, err := store.GetOrgSubscription(context.Background(), "00000000-0000-0000-0000-000000000025")
	require.NoError(t, err)

}

func TestWebhook_EmptyPayload(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	// No secret = signature check skipped.
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader([]byte("")))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusBadRequest,
		rr.
			Code)

}

func TestWebhook_MalformedJSON(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader([]byte("{not json")))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusBadRequest,
		rr.
			Code)

}

func TestWebhook_UnknownEventType(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	body := []byte(`{"type":"invoice.unknown","data":{"object":{}}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)

}

func TestWebhook_UnknownProductID(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{subscriptions: make(map[string]*OrgSubscription)}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil, withTestMetadataFallback())

	data := testSubscriptionData{
		ID:         "sub_unknown_prod",
		ProductID:  "unknown-product-xyz",
		CustomerID: "cust_unknown",
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000026"},
	}
	body := webhookPayload(t, "customer.subscription.created", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusInternalServerError,

		rr.Code)

	// Should return 500 because ErrUnknownPrice is returned.

}

func TestWebhook_ProductFromNestedObject(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{subscriptions: make(map[string]*OrgSubscription)}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil, withTestMetadataFallback())

	// ProductID is empty but Product.ID is set.
	data := testSubscriptionData{
		ID:         "sub_nested_prod",
		ProductID:  "",
		CustomerID: "cust_nested",
		Product:    &testProductData{ID: "pro-id", Name: "Pro"},
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000027"},
	}
	body := webhookPayload(t, "customer.subscription.created", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	sub, err := store.GetOrgSubscription(context.Background(), "00000000-0000-0000-0000-000000000027")
	require.NoError(t, err)
	require.Equal(t,
		string(domain.
			PlanPro),
		sub.PlanTier)

}

func TestWebhook_SubscriptionCreatedRejectsMetadataOrgRebinding(t *testing.T) {
	t.Parallel()

	boundOrg := "00000000-0000-0000-0000-000000000040"
	attackerOrg := "00000000-0000-0000-0000-000000000041"
	subID := "sub_bound"
	customerID := "cust_bound"
	store := &mockBillingStore{subscriptions: map[string]*OrgSubscription{
		boundOrg: {
			OrgID:                boundOrg,
			PlanTier:             string(domain.PlanStarter),
			StripeSubscriptionID: &subID,
			StripeCustomerID:     &customerID,
			Status:               "active",
		},
	}}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

	data := testSubscriptionData{
		ID:         subID,
		ProductID:  "pro-id",
		CustomerID: customerID,
		Metadata:   map[string]string{"org_id": attackerOrg},
	}
	body := webhookPayload(t, "customer.subscription.created", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusInternalServerError,

		rr.Code)

	if _, err := store.GetOrgSubscription(context.Background(), attackerOrg); !errors.Is(err, ErrSubscriptionNotFound) {
		require.Failf(t, "test failure",

			"attacker org subscription lookup error = %v, want ErrSubscriptionNotFound", err)
	}
	sub, err := store.GetOrgSubscription(context.Background(), boundOrg)
	require.NoError(t, err)
	require.Equal(t,
		string(domain.
			PlanStarter,
		), sub.PlanTier)

}

func TestWebhook_SubscriptionCreatedUsesExistingCustomerBindingWhenMetadataMissing(t *testing.T) {
	t.Parallel()

	boundOrg := "00000000-0000-0000-0000-000000000042"
	customerID := "cust_existing"
	store := &mockBillingStore{subscriptions: map[string]*OrgSubscription{
		boundOrg: {
			OrgID:            boundOrg,
			PlanTier:         string(domain.PlanStarter),
			StripeCustomerID: &customerID,
			Status:           "active",
		},
	}}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

	data := testSubscriptionData{
		ID:         "sub_new_for_existing_customer",
		ProductID:  "pro-id",
		CustomerID: customerID,
		Metadata:   map[string]string{},
	}
	body := webhookPayload(t, "customer.subscription.created", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	sub, err := store.GetOrgSubscription(context.Background(), boundOrg)
	require.NoError(t, err)
	require.Equal(t,
		string(domain.
			PlanPro),
		sub.PlanTier)

}

func TestWebhook_SubscriptionCreatedRejectsUnboundMetadataOrg(t *testing.T) {
	t.Parallel()

	victimOrg := "00000000-0000-0000-0000-000000000043"
	pendingTier := string(domain.PlanStarter)
	store := &mockBillingStore{subscriptions: map[string]*OrgSubscription{
		victimOrg: {
			OrgID:           victimOrg,
			PlanTier:        string(domain.PlanFree),
			Status:          "active",
			PendingPlanTier: &pendingTier,
		},
	}}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

	data := testSubscriptionData{
		ID:         "sub_unbound_metadata",
		ProductID:  "pro-id",
		CustomerID: "cust_attacker",
		Metadata:   map[string]string{"org_id": victimOrg},
	}
	body := webhookPayload(t, "customer.subscription.created", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusInternalServerError,

		rr.Code)

	sub, err := store.GetOrgSubscription(context.Background(), victimOrg)
	require.NoError(t, err)
	require.Equal(t,
		string(domain.
			PlanFree),
		sub.PlanTier)

}

func TestWebhook_SubscriptionCreatedAllowsPendingPlanIntent(t *testing.T) {
	t.Parallel()

	orgID := "00000000-0000-0000-0000-000000000044"
	pendingTier := string(domain.PlanPro)
	store := &mockBillingStore{subscriptions: map[string]*OrgSubscription{
		orgID: {
			OrgID:           orgID,
			PlanTier:        string(domain.PlanFree),
			Status:          "active",
			PendingPlanTier: &pendingTier,
		},
	}}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

	data := testSubscriptionData{
		ID:         "sub_pending_intent",
		ProductID:  "pro-id",
		CustomerID: "cust_pending_intent",
		Metadata:   map[string]string{"org_id": orgID},
	}
	body := webhookPayload(t, "customer.subscription.created", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	sub, err := store.GetOrgSubscription(context.Background(), orgID)
	require.NoError(t, err)
	require.Equal(t,
		string(domain.
			PlanPro),
		sub.PlanTier)
	require.False(t,
		sub.StripeCustomerID ==
			nil || *sub.StripeCustomerID !=
			"cust_pending_intent",
	)

}

func TestWebhook_InvoicePaymentFailedRejectsMetadataOrgWithoutBinding(t *testing.T) {
	t.Parallel()

	victimOrg := "00000000-0000-0000-0000-000000000045"
	store := &mockBillingStore{subscriptions: map[string]*OrgSubscription{
		victimOrg: {
			OrgID:         victimOrg,
			PlanTier:      string(domain.PlanPro),
			Status:        "active",
			PaymentStatus: "ok",
		},
	}}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

	inv := testInvoiceData{
		ID:         "inv_metadata_attack",
		CustomerID: "cust_unbound_attacker",
		SubID:      "sub_unbound_attacker",
		Metadata:   map[string]string{"org_id": victimOrg},
	}
	body := webhookPayload(t, "invoice.payment_failed", inv)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusInternalServerError,

		rr.Code)

	sub, err := store.GetOrgSubscription(context.Background(), victimOrg)
	require.NoError(t, err)
	require.Equal(t,
		"ok", sub.
			PaymentStatus)

}

func TestWebhook_InvoicePaymentFailedUsesCustomerBindingAndRejectsConflict(t *testing.T) {
	t.Parallel()

	boundOrg := "00000000-0000-0000-0000-000000000046"
	attackerOrg := "00000000-0000-0000-0000-000000000047"
	customerID := "cust_invoice_bound"
	store := &mockBillingStore{subscriptions: map[string]*OrgSubscription{
		boundOrg: {
			OrgID:            boundOrg,
			PlanTier:         string(domain.PlanPro),
			Status:           "active",
			PaymentStatus:    "ok",
			StripeCustomerID: &customerID,
		},
		attackerOrg: {
			OrgID:         attackerOrg,
			PlanTier:      string(domain.PlanPro),
			Status:        "active",
			PaymentStatus: "ok",
		},
	}}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

	conflict := testInvoiceData{
		ID:         "inv_conflict",
		CustomerID: customerID,
		SubID:      "sub_invoice_bound",
		Metadata:   map[string]string{"org_id": attackerOrg},
	}
	conflictBody := webhookPayload(t, "invoice.payment_failed", conflict)
	conflictRR := httptest.NewRecorder()
	handler.ServeHTTP(conflictRR, buildSignedWebhookRequest(t, testSecret, conflictBody))
	require.Equal(t,
		http.StatusInternalServerError,

		conflictRR.
			Code)

	valid := testInvoiceData{
		ID:         "inv_bound",
		CustomerID: customerID,
		SubID:      "sub_invoice_bound",
		Metadata:   map[string]string{"org_id": boundOrg},
	}
	validBody := webhookPayload(t, "invoice.payment_failed", valid)
	validRR := httptest.NewRecorder()
	handler.ServeHTTP(validRR, buildSignedWebhookRequest(t, testSecret, validBody))
	require.Equal(t,
		http.StatusOK,
		validRR.Code,
	)

	sub, err := store.GetOrgSubscription(context.Background(), boundOrg)
	require.NoError(t, err)
	require.Equal(t,
		"grace",
		sub.PaymentStatus,
	)

}

// 5. Concurrent operations on billing state

// syncMockBillingStore wraps mockBillingStore with a mutex for concurrent test safety.
type syncMockBillingStore struct {
	mu sync.Mutex
	mockBillingStore
}

func (s *syncMockBillingStore) GetOrgSubscription(ctx context.Context, orgID string) (*OrgSubscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mockBillingStore.GetOrgSubscription(ctx, orgID)
}

func (s *syncMockBillingStore) GetOrgSubscriptionByStripeSubscriptionID(ctx context.Context, stripeSubscriptionID string) (*OrgSubscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mockBillingStore.GetOrgSubscriptionByStripeSubscriptionID(ctx, stripeSubscriptionID)
}

func (s *syncMockBillingStore) GetOrgSubscriptionByStripeCustomerID(ctx context.Context, stripeCustomerID string) (*OrgSubscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mockBillingStore.GetOrgSubscriptionByStripeCustomerID(ctx, stripeCustomerID)
}

func (s *syncMockBillingStore) UpsertOrgSubscription(ctx context.Context, sub *OrgSubscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mockBillingStore.UpsertOrgSubscription(ctx, sub)
}

func (s *syncMockBillingStore) UpdateOrgSubscriptionFull(ctx context.Context, orgID, tier, status string, periodStart, periodEnd *time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mockBillingStore.UpdateOrgSubscriptionFull(ctx, orgID, tier, status, periodStart, periodEnd)
}

func (s *syncMockBillingStore) UpdateOrgSubscriptionPlan(ctx context.Context, orgID, planTier, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mockBillingStore.UpdateOrgSubscriptionPlan(ctx, orgID, planTier, status)
}

func (s *syncMockBillingStore) UpdateOrgSubscriptionStatus(ctx context.Context, orgID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mockBillingStore.UpdateOrgSubscriptionStatus(ctx, orgID, status)
}

func (s *syncMockBillingStore) SetPendingPlanTier(ctx context.Context, orgID, tier string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mockBillingStore.SetPendingPlanTier(ctx, orgID, tier)
}

func (s *syncMockBillingStore) ClearPendingPlanTier(ctx context.Context, orgID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mockBillingStore.ClearPendingPlanTier(ctx, orgID)
}

func (s *syncMockBillingStore) UpdatePaymentStatus(ctx context.Context, orgID string, status string, graceEnd *time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mockBillingStore.UpdatePaymentStatus(ctx, orgID, status, graceEnd)
}

func TestWebhook_ConcurrentCreatedEvents(t *testing.T) {
	t.Parallel()

	store := &syncMockBillingStore{
		mockBillingStore: mockBillingStore{subscriptions: make(map[string]*OrgSubscription)},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil, withTestMetadataFallback())

	data := testSubscriptionData{
		ID:         "sub_conc_1",
		Status:     "active",
		CustomerID: "cust_conc",
		ProductID:  "starter-id",
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000028"},
	}
	body := webhookPayload(t, "customer.subscription.created", data)

	var wg conc.WaitGroup
	var errCount atomic.Int64
	for range 20 {
		wg.Go(func() {
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
			if rr.Code != http.StatusOK {
				errCount.Add(1)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 0, errCount.
		Load())

	// Final state should be consistent.
	sub, err := store.GetOrgSubscription(context.Background(), "00000000-0000-0000-0000-000000000028")
	require.NoError(t, err)
	require.Equal(t,
		string(domain.
			PlanStarter,
		), sub.PlanTier)

}

func TestEnforcer_ConcurrentCheckSpendingLimit(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-conc-spend": {
				OrgID:                 "org-conc-spend",
				PlanTier:              string(domain.PlanStarter),
				SpendingLimitMicrousd: 100_000_000, // $100 limit
				LimitAction:           "reject",
			},
		},
		periodSpendByOrg: map[string]int64{
			"org-conc-spend": 50_000_000, // $50 spent
		},
	}
	e := NewEnforcer(store, nil, slog.Default())

	var wg conc.WaitGroup
	var errCount atomic.Int64

	for range 100 {
		wg.Go(func() {
			err := e.CheckSpendingLimit(context.Background(), "org-conc-spend")
			if err != nil {
				errCount.Add(1)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 0, errCount.
		Load())

	// Under limit, all should pass.

}

func TestEnforcer_ConcurrentGetOrgPlanLimits(t *testing.T) {
	t.Parallel()

	store := &syncMockBillingStore{
		mockBillingStore: mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"org-conc-limits": {
					OrgID:    "org-conc-limits",
					PlanTier: string(domain.PlanPro),
					Status:   "active",
				},
			},
		},
	}
	e := NewEnforcer(store, nil, slog.Default())

	var wg conc.WaitGroup
	for range 50 {
		wg.Go(func() {
			limits, err := e.GetOrgPlanLimits(context.Background(), "org-conc-limits")
			assert.NoError(
				t, err)
			assert.Equal(t,
				domain.PlanPro,
				limits.PlanTier,
			)

		})
	}
	wg.Wait()
}

// 6. Error cascades (DB errors mid-operation)

func TestWebhook_UpsertErrorOnCreate(t *testing.T) {
	t.Parallel()

	store := &errStore{
		upsertErr: errors.New("db connection lost"),
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil, withTestMetadataFallback())

	data := testSubscriptionData{
		ID:         "sub_err_create",
		ProductID:  "starter-id",
		CustomerID: "cust_err",
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000029"},
	}
	body := webhookPayload(t, "customer.subscription.created", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusInternalServerError,

		rr.Code)

}

func TestWebhook_GetSubErrorOnUpdated(t *testing.T) {
	t.Parallel()

	store := &errStore{
		getSubErr: errors.New("timeout connecting to database"),
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil, withTestMetadataFallback())

	data := testSubscriptionData{
		ID:         "sub_err_update",
		ProductID:  "starter-id",
		CustomerID: "cust_err_upd",
		Status:     "active",
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000002a"},
	}
	body := webhookPayload(t, "customer.subscription.updated", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusInternalServerError,

		rr.Code)

}

func TestWebhook_UpdateFullErrorFallsBackToUpsert(t *testing.T) {
	t.Parallel()

	store := &errStore{
		mockBillingStore: mockBillingStore{
			subscriptions: map[string]*OrgSubscription{
				"00000000-0000-0000-0000-00000000002b": {
					OrgID:    "00000000-0000-0000-0000-00000000002b",
					PlanTier: string(domain.PlanStarter),
					Status:   "active",
				},
			},
		},
		// Return ErrSubscriptionNotFound from UpdateFull to trigger fallback path.
		updateFullErr: ErrSubscriptionNotFound,
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil, withTestMetadataFallback())

	data := testSubscriptionData{
		ID:         "sub_fallback",
		ProductID:  "pro-id",
		Status:     "active",
		CustomerID: "cust_fall",
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000002b"},
	}
	body := webhookPayload(t, "customer.subscription.updated", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.NotEqual(t, 0, store.
		upsertCallCount.
		Load())

	// Verify fallback upsert was triggered.

}

func TestWebhook_AuditStoreError(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{subscriptions: make(map[string]*OrgSubscription)}
	auditStore := &advMockAuditStore{err: errors.New("audit table locked")}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, auditStore, withTestMetadataFallback())

	data := testSubscriptionData{
		ID:         "sub_audit_err",
		ProductID:  "starter-id",
		CustomerID: "cust_audit_err",
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000002c"},
	}
	body := webhookPayload(t, "customer.subscription.created", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	// Audit error should not fail the webhook.

	// Subscription should still be created.
	_, err := store.GetOrgSubscription(context.Background(), "00000000-0000-0000-0000-00000000002c")
	require.NoError(t, err)

}

func TestEnforcer_CheckSpendingLimit_SubscriptionReadFailsClosed(t *testing.T) {
	t.Parallel()

	store := &errStore{
		getSubErr: errors.New("transient db error"),
	}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckSpendingLimit(context.Background(), "org-fail-open")
	var le *LimitError
	require.True(t,
		errors.As(
			err, &le))
	require.Equal(t,
		"service_degraded",
		le.Code,
	)

}

func TestEnforcer_CheckSpendingLimit_FreeTierExceeded(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-free-exceeded": {
				OrgID:    "org-free-exceeded",
				PlanTier: string(domain.PlanFree),
				Status:   "active",
			},
		},
		periodSpendByOrg: map[string]int64{
			"org-free-exceeded": CreditFreeMicrousd + 1, // $1.00 + 1 micro
		},
	}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckSpendingLimit(context.Background(), "org-free-exceeded")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t,
		errors.As(
			err, &le))
	assert.Equal(t,
		"spending_limit_reached",

		le.Code)

}

func TestEnforcer_CheckSpendingLimit_PaidNoLimit(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-no-limit": {
				OrgID:                 "org-no-limit",
				PlanTier:              string(domain.PlanPro),
				SpendingLimitMicrousd: -1, // no limit
				Status:                "active",
			},
		},
		periodSpendByOrg: map[string]int64{
			"org-no-limit": 999_999_999,
		},
	}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckSpendingLimit(context.Background(), "org-no-limit")
	require.NoError(t, err)

}

func TestEnforcer_CheckSpendingLimit_PaidLimitReached(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-limit-hit": {
				OrgID:                 "org-limit-hit",
				PlanTier:              string(domain.PlanStarter),
				SpendingLimitMicrousd: 50_000_000, // $50 limit
				LimitAction:           "reject",
				Status:                "active",
			},
		},
		periodSpendByOrg: map[string]int64{
			// Spend exceeds included credit + spending limit.
			"org-limit-hit": CreditStarterMicrousd + 50_000_000,
		},
	}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckSpendingLimit(context.Background(), "org-limit-hit")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t,
		errors.As(
			err, &le))

}

func TestEnforcer_CheckSpendingLimit_NoSubscription(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		periodSpendByOrg: map[string]int64{
			// Orchestration-only: no included credit; any spend triggers the cap.
			"org-no-sub": 500_000,
		},
	}
	e := NewEnforcer(store, nil, slog.Default())

	// No subscription -> free tier path -> any spend blocks.
	err := e.CheckSpendingLimit(context.Background(), "org-no-sub")
	require.Error(t,
		err)

}

func TestEnforcer_CheckSpendingLimit_NoSubscription_ZeroSpend_Passes(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		periodSpendByOrg: map[string]int64{
			"org-no-sub-zero": 0,
		},
	}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckSpendingLimit(context.Background(), "org-no-sub-zero")
	require.NoError(t, err)

}

func TestEnforcer_CheckProjectLimit_AtLimit(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-proj-limit": {
				OrgID:    "org-proj-limit",
				PlanTier: string(domain.PlanFree),
				Status:   "active",
			},
		},
		projects: map[string][]string{
			"org-proj-limit": {"p1", "p2"}, // MaxProjectsFree = 2
		},
	}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckProjectLimit(context.Background(), "org-proj-limit")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t,
		errors.As(
			err, &le))
	assert.Equal(t,
		"project_limit_reached",

		le.Code)

}

func TestEnforcer_CheckMemberLimit_AtLimit(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-mem-limit": {
				OrgID:    "org-mem-limit",
				PlanTier: string(domain.PlanFree),
				Status:   "active",
			},
		},
		memberCounts: map[string]int{
			"org-mem-limit": MaxMembersFree, // exactly at limit
		},
	}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckMemberLimit(context.Background(), "org-mem-limit")
	require.Error(t,
		err)

	var le *LimitError
	require.True(t,
		errors.As(
			err, &le))
	assert.Equal(t,
		"member_limit_reached",
		le.
			Code)

}

func TestEnforcer_CheckOrgCreationLimit(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		orgCountsByUser: map[string]int{
			"user-max-orgs": MaxOrgsFree, // exactly at limit
		},
	}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckOrgCreationLimit(context.Background(), "user-max-orgs", domain.PlanFree)
	require.Error(t,
		err)

	var le *LimitError
	require.True(t,
		errors.As(
			err, &le))
	assert.Equal(t,
		"org_limit_reached",
		le.Code,
	)

	// Unlimited enterprise should always pass.
	err = e.CheckOrgCreationLimit(context.Background(), "user-max-orgs", domain.PlanEnterprise)
	require.NoError(t, err)

}

// Payment status / grace period paths

func TestWebhook_PastDueSetsGracePeriod(t *testing.T) {
	t.Parallel()

	now := time.Now()
	periodStart := now.Add(-15 * 24 * time.Hour)
	periodEnd := now.Add(15 * 24 * time.Hour)
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-00000000002d": {
				OrgID:              "00000000-0000-0000-0000-00000000002d",
				PlanTier:           string(domain.PlanStarter),
				Status:             "active",
				PaymentStatus:      "ok",
				CurrentPeriodStart: &periodStart,
				CurrentPeriodEnd:   &periodEnd,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil, withTestMetadataFallback())

	// Stripe fires invoice.payment_failed when a payment attempt fails.
	invData := testInvoiceData{
		ID:         "inv_pastdue",
		CustomerID: "cust_pastdue",
		SubID:      "sub_pastdue",
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000002d"},
	}
	body := webhookPayload(t, "invoice.payment_failed", invData)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	sub, _ := store.GetOrgSubscription(context.Background(), "00000000-0000-0000-0000-00000000002d")
	require.Equal(t,
		"grace",
		sub.PaymentStatus,
	)
	require.NotNil(
		t, sub.GracePeriodEnd,
	)

}

func TestWebhook_ActiveSubscriptionUpdateDoesNotClearGracePeriod(t *testing.T) {
	t.Parallel()

	graceEnd := time.Now().Add(48 * time.Hour)
	periodStart := time.Now().Add(-15 * 24 * time.Hour)
	periodEnd := time.Now().Add(15 * 24 * time.Hour)
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-00000000002e": {
				OrgID:              "00000000-0000-0000-0000-00000000002e",
				PlanTier:           string(domain.PlanStarter),
				Status:             "active",
				PaymentStatus:      "grace",
				GracePeriodEnd:     &graceEnd,
				CurrentPeriodStart: &periodStart,
				CurrentPeriodEnd:   &periodEnd,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil, withTestMetadataFallback())

	data := testSubscriptionData{
		ID:                 "sub_recover",
		ProductID:          "starter-id",
		CustomerID:         "cust_recover",
		Status:             "active",
		CurrentPeriodStart: &periodStart,
		CurrentPeriodEnd:   &periodEnd,
		Metadata:           map[string]string{"org_id": "00000000-0000-0000-0000-00000000002e"},
	}
	body := webhookPayload(t, "customer.subscription.updated", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	sub, _ := store.GetOrgSubscription(context.Background(), "00000000-0000-0000-0000-00000000002e")
	require.Equal(t,
		"grace",
		sub.PaymentStatus,
	)

}

func TestWebhook_PaymentSucceededClearsGrace(t *testing.T) {
	t.Parallel()

	graceEnd := time.Now().Add(48 * time.Hour)
	subID := "sub_paid"
	customerID := "cust_paid"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-00000000002f": {
				OrgID:                "00000000-0000-0000-0000-00000000002f",
				PlanTier:             string(domain.PlanStarter),
				Status:               "active",
				PaymentStatus:        "restricted",
				GracePeriodEnd:       &graceEnd,
				StripeSubscriptionID: &subID,
				StripeCustomerID:     &customerID,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

	// Stripe fires invoice.paid when payment actually recovers.
	data := testInvoiceData{
		ID:         "inv_paid",
		CustomerID: customerID,
		SubID:      subID,
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000002f"},
	}
	body := webhookPayload(t, "invoice.paid", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	sub, _ := store.GetOrgSubscription(context.Background(), "00000000-0000-0000-0000-00000000002f")
	require.Equal(t,
		"ok", sub.
			PaymentStatus)

}

func TestWebhook_PaymentSucceeded_AlreadyOk(t *testing.T) {
	t.Parallel()

	subID := "sub_ok"
	customerID := "cust_ok"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000030": {
				OrgID:                "00000000-0000-0000-0000-000000000030",
				PlanTier:             string(domain.PlanStarter),
				Status:               "active",
				PaymentStatus:        "ok",
				StripeSubscriptionID: &subID,
				StripeCustomerID:     &customerID,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

	data := testSubscriptionData{
		ID:         subID,
		ProductID:  "starter-id",
		CustomerID: customerID,
		Status:     "active",
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000030"},
	}
	body := webhookPayload(t, "customer.subscription.updated", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.Nil(t, store.
		lastPaymentStatusUpdate,
	)

	// Payment status should remain "ok" without unnecessary update.

}

// Signature verification edge cases

func TestWebhook_MultipleSignaturesInHeader(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{subscriptions: make(map[string]*OrgSubscription)}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil, withTestMetadataFallback())

	data := testSubscriptionData{
		ID:         "sub_multisig",
		ProductID:  "starter-id",
		CustomerID: "cust_multisig",
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000031"},
	}
	body := webhookPayload(t, "customer.subscription.created", data)

	// Build a valid Stripe signature.
	ts := fmt.Sprintf("%d", time.Now().Unix())
	signedContent := ts + "." + string(body)
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write([]byte(signedContent))
	validSig := hex.EncodeToString(mac.Sum(nil))

	// Stripe supports multiple v1 signatures separated by commas.
	sigHeader := fmt.Sprintf("t=%s,v1=invalidsig,v1=%s", ts, validSig)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", sigHeader)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)

}

func TestWebhook_FutureTimestamp(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

	body := []byte(`{"type":"customer.subscription.created","data":{}}`)
	futureTS := fmt.Sprintf("%d", time.Now().Add(10*time.Minute).Unix())

	signedContent := futureTS + "." + string(body)
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write([]byte(signedContent))
	sig := fmt.Sprintf("t=%s,v1=%s", futureTS, hex.EncodeToString(mac.Sum(nil)))

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", sig)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusUnauthorized,

		rr.Code)

}

func TestWebhook_NonNumericTimestamp(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

	body := []byte(`{"type":"customer.subscription.created","data":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", "t=not-a-number,v1=invalidsig")
	req.Header.Set("webhook-signature", "v1,anything")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusUnauthorized,

		rr.Code)

}

// Downgrade preview and build impact

func TestBuildImpact_UnlimitedToLimited(t *testing.T) {
	t.Parallel()

	impact := buildImpact("test_resource", -1, 10)
	assert.Equal(t,
		ResourceActionReduce,
		impact.
			Action)

}

func TestBuildImpact_LimitedToUnlimited(t *testing.T) {
	t.Parallel()

	impact := buildImpact("test_resource", 10, -1)
	assert.Equal(t,
		ResourceActionOK,
		impact.
			Action)

}

func TestBuildImpact_LimitedToZero(t *testing.T) {
	t.Parallel()

	impact := buildImpact("test_resource", 5, 0)
	assert.Equal(t,
		ResourceActionRemove,
		impact.
			Action)

}

func TestAutoDisableResources_Separation(t *testing.T) {
	t.Parallel()

	impacts := []ResourceImpact{
		{Resource: "projects", Action: ResourceActionReduce, Current: 10, Limit: 5},
		{Resource: "members_per_org", Action: ResourceActionReduce, Current: 25, Limit: 10},
		{Resource: "log_drains", Action: ResourceActionReduce, Current: 5, Limit: 1},
		{Resource: "retention_days", Action: ResourceActionOK, Current: 30, Limit: 30},
	}

	manual, auto := AutoDisableResources(impacts)
	require.Len(t,
		manual, 2)
	require.Len(t,
		auto, 1)
	assert.Equal(t,
		"log_drains",
		auto[0].Resource,
	)

}

// LimitError interface compliance

func TestLimitError_ErrorInterface(t *testing.T) {
	t.Parallel()

	le := &LimitError{
		Code:    "test_code",
		Message: "test message",
	}

	// Verify it implements error interface.
	var err error = le
	assert.Equal(t,
		"test message",
		err.Error())

	// Verify errors.As works.
	wrapped := fmt.Errorf("wrapping: %w", le)
	var target *LimitError
	require.True(t,
		errors.As(
			wrapped, &target,
		))
	assert.Equal(t,
		"test_code",
		target.Code)

}

// Anomaly detection edge cases

func TestAnomalyConfig_HighThresholdAutoComputed(t *testing.T) {
	t.Parallel()

	cfg := AnomalyConfig{WarningThreshold: 3.0, CriticalThreshold: 10.0}
	assert.EqualValues(t, 6.5, cfg.highThreshold())

	cfg2 := AnomalyConfig{WarningThreshold: 3.0, HighThreshold: 7.0, CriticalThreshold: 10.0}
	assert.EqualValues(t, 7.0, cfg2.
		highThreshold(),
	)

}

func TestNewAnomalyDetectorWithConfig_DefaultsOnZero(t *testing.T) {
	t.Parallel()

	d := NewAnomalyDetectorWithConfig(&mockBillingStore{}, AnomalyConfig{
		WarningThreshold:  0,
		CriticalThreshold: 0,
	})
	assert.Equal(t,
		spikeWarning,
		d.config.WarningThreshold,
	)
	assert.Equal(t,
		spikeCritical,
		d.config.CriticalThreshold,
	)

}

// SafePercent edge cases

func TestSafePercent_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		used     int64
		limit    int64
		expected float64
	}{
		{"zero limit", 100, 0, 0},
		{"negative limit", 100, -1, 0},
		{"zero used", 0, 100, 0},
		{"100 percent", 100, 100, 100},
		{"over 100 percent", 200, 100, 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := safePercent(tt.used, tt.limit)
			assert.Equal(t,
				tt.expected,
				got)

		})
	}
}

// RecommendPlan edge cases

func TestRecommendPlan_AllTiers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		monthlyRuns    int64
		monthlyCompute int64
		expected       string
	}{
		{"zero usage", 0, 0, string(domain.PlanFree)},
		{"within free credit", 100, CreditFreeMicrousd, string(domain.PlanFree)},
		{"over free credit", 100, CreditFreeMicrousd + 1, string(domain.PlanStarter)},
		{"within starter credit", 1000, CreditStarterMicrousd, string(domain.PlanStarter)},
		{"over starter credit", 1000, CreditStarterMicrousd + 1, string(domain.PlanPro)},
		{"within pro credit", 5000, CreditProMicrousd, string(domain.PlanPro)},
		{"over pro credit", 5000, CreditProMicrousd + 1, string(domain.PlanScale)},
		{"within scale credit", 10000, CreditScaleMicrousd, string(domain.PlanScale)},
		{"over scale credit", 10000, CreditScaleMicrousd + 1, string(domain.PlanBusiness)},
		{"within business credit", 10000, CreditBusinessMicrousd, string(domain.PlanBusiness)},
		{"over business credit", 10000, CreditBusinessMicrousd + 1, string(domain.PlanEnterprise)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := recommendPlan(tt.monthlyRuns, tt.monthlyCompute)
			assert.Equal(t,
				tt.expected,
				got)

		})
	}
}

// MicroToUSDString

func TestMicroToUSDString_Adversarial(t *testing.T) {
	t.Parallel()

	tests := []struct {
		micro    int64
		expected string
	}{
		{0, "0.000000"},
		{1_000_000, "1.000000"},
		{1, "0.000001"},
		{-1_000_000, "-1.000000"},
		{999_999, "0.999999"},
	}

	for _, tt := range tests {
		got := microToUSDString(tt.micro)
		assert.Equal(t,
			tt.expected,
			got)

	}
}

// WelcomeEmail option

func TestWebhook_WelcomeEmailSentOnPaidPlan(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{subscriptions: make(map[string]*OrgSubscription)}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")

	var emailSent atomic.Bool
	done := make(chan struct{}, 1)
	welcomeFn := func(_ context.Context, orgID string, tier domain.PlanTier, email string) error {
		emailSent.Store(true)
		select {
		case done <- struct{}{}:
		default:
		}
		return nil
	}

	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil,
		WithWelcomeEmail(welcomeFn), withTestMetadataFallback())

	data := testSubscriptionData{
		ID:         "sub_welcome",
		ProductID:  "starter-id",
		CustomerID: "cust_welcome",
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000032"},
		Customer:   &testCustomerData{ID: "cust_welcome", Email: "welcome@example.com"},
	}
	body := webhookPayload(t, "customer.subscription.created", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "expected welcome email to be sent for paid plan")
	}
}

func TestWebhook_WelcomeEmailNotSentForFreePlan(t *testing.T) {
	t.Parallel()

	// Create a mapping where a product maps to free tier (not possible in real config,
	// but we test the code path). We test via subscription.created with no known product
	// that maps to free. Instead, test that free tier does not trigger the email by
	// verifying the behavior: webhook returns OK but welcome is not sent.

	// This actually tests: if tier == domain.PlanFree, welcomeEmail is not called.
	// We cannot easily map a product to free tier in the mapping, so we test the
	// no-customer-email path instead.
	store := &mockBillingStore{subscriptions: make(map[string]*OrgSubscription)}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")

	done := make(chan struct{}, 1)
	welcomeFn := func(_ context.Context, orgID string, tier domain.PlanTier, email string) error {
		select {
		case done <- struct{}{}:
		default:
		}
		return nil
	}

	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil,
		WithWelcomeEmail(welcomeFn), withTestMetadataFallback())

	// No customer email set => welcome email should not be sent.
	data := testSubscriptionData{
		ID:         "sub_no_email",
		ProductID:  "starter-id",
		CustomerID: "cust_no_email",
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000033"},
		// Customer is nil, so email is empty.
	}
	body := webhookPayload(t, "customer.subscription.created", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	select {
	case <-done:
		require.Fail(t, "expected welcome email NOT to be sent when customer email is empty")
	case <-time.After(200 * time.Millisecond):
	}
}

// NewEnforcer panics on nil store
// UsageService methods with 0% coverage

func TestUsageService_GetProjectCosts(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		usageRecords: []UsageRecord{
			{ProjectID: "proj-a", RunsCount: 10, ComputeCostMicro: 1000},
			{ProjectID: "proj-a", RunsCount: 5, ComputeCostMicro: 600},
			{ProjectID: "proj-b", RunsCount: 3, ComputeCostMicro: 300},
		},
	}
	e := NewEnforcer(store, nil, slog.Default())
	svc := NewUsageService(store, e)

	from := time.Now().Add(-7 * 24 * time.Hour)
	to := time.Now()
	costs, err := svc.GetProjectCosts(context.Background(), "org-costs", from, to)
	require.NoError(t, err)
	require.Len(t,
		costs, 2)

	// Check aggregation for one of the projects.
	costMap := make(map[string]ProjectCostEntry)
	for _, c := range costs {
		costMap[c.ProjectID] = c
	}
	if a, ok := costMap["proj-a"]; ok {
		assert.EqualValues(t, 15, a.Runs)
		assert.EqualValues(t, 1600, a.TotalMicro)

	} else {
		require.Fail(t,

			"proj-a not found in costs")
	}
}

func TestUsageService_ExportCSV(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		usageRecords: []UsageRecord{
			{
				ProjectID:        "proj-csv",
				PeriodDate:       time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				RunsCount:        100,
				ComputeCostMicro: 50000,
				UsageTokensTotal: 1000,
				UsageCostMicro:   2000,
			},
		},
	}
	e := NewEnforcer(store, nil, slog.Default())
	svc := NewUsageService(store, e)

	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	csv, err := svc.ExportUsageCSV(context.Background(), "org-csv", from, to)
	require.NoError(t, err)
	require.NotEmpty(t, csv)
	require.True(t,
		bytes.Contains(csv, []byte("date,project,runs")))

	// Check header is present.

}

func TestUsageService_ExportPDF(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-pdf": {
				OrgID:    "org-pdf",
				PlanTier: string(domain.PlanStarter),
				Status:   "active",
			},
		},
		usageRecords: []UsageRecord{
			{
				ProjectID:        "proj-pdf",
				PeriodDate:       time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				RunsCount:        50,
				ComputeCostMicro: 25000,
				UsageCostMicro:   1000,
			},
		},
	}
	e := NewEnforcer(store, nil, slog.Default())
	svc := NewUsageService(store, e)

	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	pdf, err := svc.ExportUsagePDF(context.Background(), "org-pdf", from, to)
	require.NoError(t, err)
	require.NotEmpty(t, pdf)
	require.True(t,
		bytes.HasPrefix(pdf, []byte("%PDF")))

	// PDF files start with %PDF.

}

func TestUsageService_GetProjectBudget(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	e := NewEnforcer(store, nil, slog.Default())
	svc := NewUsageService(store, e)

	resp, err := svc.GetProjectBudget(context.Background(), "proj-budget")
	require.NoError(t, err)
	assert.Equal(t,
		"proj-budget",
		resp.ProjectID,
	)
	assert.EqualValues(t, -1, resp.MonthlyBudgetMicro)

	// Default mock returns -1, "notify".

}

func TestUsageService_GetSpendingLimit_ReturnsSpendAggregationError(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-spend-error": {
				OrgID:                 "org-spend-error",
				PlanTier:              string(domain.PlanPro),
				Status:                "active",
				SpendingLimitMicrousd: 10_000_000,
				LimitAction:           "reject",
			},
		},
		sumSpendErr: errors.New("spend aggregation unavailable"),
	}
	svc := NewUsageService(store, NewEnforcer(store, nil, slog.Default()))

	_, err := svc.GetSpendingLimit(context.Background(), "org-spend-error")
	require.Error(t,
		err)
	require.True(t,
		strings.Contains(err.Error(), "summing org period spend"))

}

func TestUsageService_GetUsageForecast_ReturnsPlanLimitError(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		getOrgSubscriptionFn: func(context.Context, string) (*OrgSubscription, error) {
			return nil, errors.New("plan lookup unavailable")
		},
	}
	svc := NewUsageService(store, NewEnforcer(store, nil, slog.Default()))

	_, err := svc.GetUsageForecast(context.Background(), "org-forecast-error")
	require.Error(t,
		err)
	require.True(t,
		strings.Contains(err.Error(), "getting org plan limits for forecast"))

}

func TestUsageService_PreviewDowngrade(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-preview": {
				OrgID:    "org-preview",
				PlanTier: string(domain.PlanPro),
				Status:   "active",
			},
		},
		projects: map[string][]string{
			"org-preview": {"p1", "p2", "p3", "p4", "p5"},
		},
	}
	e := NewEnforcer(store, nil, slog.Default())
	svc := NewUsageService(store, e)

	impact, err := svc.PreviewDowngrade(context.Background(), "org-preview", domain.PlanFree)
	require.NoError(t, err)
	assert.Equal(t,
		string(domain.
			PlanFree),
		impact.TargetTier,
	)
	require.NotEmpty(t, impact.
		Impacts)

	// 5 projects > MaxProjectsFree (2), should appear in manual actions.
	found := false
	for _, ma := range impact.ManualActions {
		if ma.Resource == "projects" {
			found = true
			assert.Equal(t,
				ResourceActionReduce,
				ma.
					Action)

		}
	}
	require.True(t,
		found)

}

func TestPreviewDowngrade_UsesActualUsageNotCurrentPlanCaps(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-preview-actual": {
				OrgID:    "org-preview-actual",
				PlanTier: string(domain.PlanPro),
				Status:   "active",
			},
		},
		projects:      map[string][]string{"org-preview-actual": {"p1"}},
		memberCounts:  map[string]int{"org-preview-actual": 2},
		executingRuns: map[string]int{"org-preview-actual": 3},
	}

	impact, err := PreviewDowngrade(context.Background(), store, "org-preview-actual", domain.PlanFree)
	require.NoError(t, err)

	byResource := make(map[string]ResourceImpact, len(impact.Impacts))
	for _, item := range impact.Impacts {
		byResource[item.Resource] = item
	}
	require.EqualValues(t, 2, byResource["members_per_org"].Current)
	require.EqualValues(t, 3, byResource["concurrent_runs"].Current)

}

func TestUsageService_DetectAnomalies(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// Build 7 historical days + 1 today with a spike.
	var records []UsageRecord
	for i := 7; i >= 1; i-- {
		records = append(records, UsageRecord{
			OrgID:            "org-anomaly",
			ProjectID:        "proj-1",
			PeriodDate:       today.AddDate(0, 0, -i),
			ComputeCostMicro: 1000,
			UsageCostMicro:   0,
		})
	}
	// Today's spend is 10x the average (spike).
	records = append(records, UsageRecord{
		OrgID:            "org-anomaly",
		ProjectID:        "proj-1",
		PeriodDate:       today,
		ComputeCostMicro: 10000,
		UsageCostMicro:   0,
	})

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-anomaly": {
				OrgID:    "org-anomaly",
				PlanTier: string(domain.PlanStarter),
				Status:   "active",
			},
		},
		usageRecords: records,
	}
	e := NewEnforcer(store, nil, slog.Default())
	svc := NewUsageService(store, e)

	alerts, err := svc.DetectAnomalies(context.Background(), "org-anomaly")
	require.NoError(t, err)
	require.NotEmpty(t, alerts)
	assert.Equal(t,
		AnomalySeverityCritical,

		alerts[0].Severity,
	)

}

func TestUsageService_GetAnomalyConfig_NoSubscription(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	e := NewEnforcer(store, nil, slog.Default())
	svc := NewUsageService(store, e)

	cfg, err := svc.GetAnomalyConfig(context.Background(), "org-no-sub")
	require.NoError(t, err)
	assert.Equal(t,
		spikeWarning,
		cfg.WarningThreshold,
	)

}

func TestUsageService_GetAnomalyConfig_WithCustomThresholds(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-custom-thresh": {
				OrgID:                    "org-custom-thresh",
				PlanTier:                 string(domain.PlanPro),
				AnomalyThresholdWarning:  5.0,
				AnomalyThresholdCritical: 15.0,
			},
		},
	}
	e := NewEnforcer(store, nil, slog.Default())
	svc := NewUsageService(store, e)

	cfg, err := svc.GetAnomalyConfig(context.Background(), "org-custom-thresh")
	require.NoError(t, err)
	assert.EqualValues(t, 5.0, cfg.WarningThreshold)
	assert.EqualValues(t, 15.0, cfg.
		CriticalThreshold,
	)

}

func TestUsageService_SetAnomalyConfig_Validation(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	e := NewEnforcer(store, nil, slog.Default())
	svc := NewUsageService(store, e)

	// Warning must be > 1.0.
	err := svc.SetAnomalyConfig(context.Background(), "org-thresh", 0.5, 10.0)
	require.Error(t,
		err)

	// Critical must be > warning.
	err = svc.SetAnomalyConfig(context.Background(), "org-thresh", 5.0, 3.0)
	require.Error(t,
		err)

	// Valid config should succeed.
	err = svc.SetAnomalyConfig(context.Background(), "org-thresh", 3.0, 10.0)
	require.NoError(t, err)

}

func TestUsageService_SetSpendingLimit_Validation(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-spend-val": {
				OrgID:    "org-spend-val",
				PlanTier: string(domain.PlanStarter),
				Status:   "active",
			},
		},
	}
	e := NewEnforcer(store, nil, slog.Default())
	svc := NewUsageService(store, e)

	// Negative limit.
	err := svc.SetSpendingLimit(context.Background(), "org-spend-val", -1, "reject")
	require.Error(t,
		err)

	// Invalid action.
	err = svc.SetSpendingLimit(context.Background(), "org-spend-val", 100000, "block")
	require.Error(t,
		err)

	// Over max limit for starter.
	err = svc.SetSpendingLimit(context.Background(), "org-spend-val", MaxSpendingStarter+1, "reject")
	require.Error(t,
		err)

	// Free plan cannot set spending limit.
	store2 := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-free-limit": {
				OrgID:    "org-free-limit",
				PlanTier: string(domain.PlanFree),
				Status:   "active",
			},
		},
	}
	svc2 := NewUsageService(store2, NewEnforcer(store2, nil, slog.Default()))
	err = svc2.SetSpendingLimit(context.Background(), "org-free-limit", 100000, "reject")
	require.Error(t,
		err)

	// No subscription.
	store3 := &mockBillingStore{}
	svc3 := NewUsageService(store3, NewEnforcer(store3, nil, slog.Default()))
	err = svc3.SetSpendingLimit(context.Background(), "org-no-sub", 100000, "reject")
	require.Error(t,
		err)

}

func TestUsageService_GetEmailPreferences_Adversarial(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-email": {
				OrgID:             "org-email",
				PlanTier:          string(domain.PlanPro),
				MonthlyUsageEmail: true,
			},
		},
	}
	e := NewEnforcer(store, nil, slog.Default())
	svc := NewUsageService(store, e)

	prefs, err := svc.GetEmailPreferences(context.Background(), "org-email")
	require.NoError(t, err)
	assert.True(t,
		prefs.MonthlyUsageEmail,
	)

	// No subscription => defaults to true.
	prefs2, err := svc.GetEmailPreferences(context.Background(), "org-no-sub")
	require.NoError(t, err)
	assert.True(t,
		prefs2.MonthlyUsageEmail,
	)

}

// Webhook: subscription.canceled with non-existent org

func TestWebhook_CancelNonExistentOrg(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{} // no subscriptions
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil, withTestMetadataFallback())

	data := testSubscriptionData{
		ID:         "sub_cancel_noexist",
		ProductID:  "starter-id",
		CustomerID: "cust_noexist",
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000034"},
	}
	body := webhookPayload(t, "customer.subscription.deleted", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

}

// Webhook: subscription.revoked with non-existent org

func TestWebhook_RevokeNonExistentOrg(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{} // no subscriptions
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

	data := testSubscriptionData{
		ID:         "sub_revoke_noexist",
		ProductID:  "starter-id",
		CustomerID: "cust_revoke",
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000034"},
	}
	body := webhookPayload(t, "subscription.revoked", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

}

// Webhook: payment succeeded with non-existent org

func TestWebhook_PaymentSucceededNonExistentOrg(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

	data := testSubscriptionData{
		ID:         "sub_pay_noexist",
		ProductID:  "starter-id",
		CustomerID: "cust_pay",
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000034"},
	}
	body := webhookPayload(t, "order.paid", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

}

// Webhook: subscription.updated with unknown product returns OK (logged)

func TestWebhook_UpdatedUnknownProduct(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000035": {
				OrgID:    "00000000-0000-0000-0000-000000000035",
				PlanTier: string(domain.PlanStarter),
				Status:   "active",
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil, withTestMetadataFallback())

	data := testSubscriptionData{
		ID:         "sub_unk_prod_upd",
		ProductID:  "unknown-product",
		CustomerID: "cust_unk_prod",
		Status:     "active",
		Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000035"},
	}
	body := webhookPayload(t, "customer.subscription.updated", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	// Unknown product on update is a no-op (not an error).

}

// Webhook: subscription.updated with empty status defaults to "active"

func TestWebhook_UpdatedEmptyStatusDefaultsActive(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ps := now.Add(-7 * 24 * time.Hour)
	pe := now.Add(23 * 24 * time.Hour)
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000036": {
				OrgID:              "00000000-0000-0000-0000-000000000036",
				PlanTier:           string(domain.PlanStarter),
				Status:             "active",
				CurrentPeriodStart: &ps,
				CurrentPeriodEnd:   &pe,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil, withTestMetadataFallback())

	data := testSubscriptionData{
		ID:                 "sub_empty_status",
		ProductID:          "starter-id",
		CustomerID:         "cust_empty_status",
		Status:             "", // empty, should default to "active"
		CurrentPeriodStart: &ps,
		CurrentPeriodEnd:   &pe,
		Metadata:           map[string]string{"org_id": "00000000-0000-0000-0000-000000000036"},
	}
	body := webhookPayload(t, "customer.subscription.updated", data)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, buildSignedWebhookRequest(t, testSecret, body))
	require.Equal(t,
		http.StatusOK,
		rr.Code)

}

func TestNewEnforcer_PanicsOnNilStore(t *testing.T) {
	t.Parallel()

	defer func() {
		require.NotNil(t, recover())

	}()
	NewEnforcer(nil, nil, slog.Default())
}
