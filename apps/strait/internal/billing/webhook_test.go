package billing

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signStripeWebhook creates a Stripe-Signature header value for a test request.
func signStripeWebhook(t *testing.T, secret string, body []byte) string {
	t.Helper()
	ts := fmt.Sprintf("%d", time.Now().Unix())
	signedContent := ts + "." + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedContent))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("t=%s,v1=%s", ts, sig)
}

// testSecret is the webhook signing secret used in tests.
var testSecret = "whsec_test_secret_for_unit_tests_only"

func TestWebhookHandler_VerifySignature(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, testSecret, slog.Default(), nil, nil)

	body := []byte(`{"type":"customer.subscription.created","data":{"object":{"id":"sub_sig","status":"active","items":{"data":[{"price":{"id":"starter-id"}}]},"customer":{"id":"cust_sig","metadata":{"org_id":"550e8400-e29b-41d4-a716-446655440000"}}}}}`)

	t.Run("valid_signature", func(t *testing.T) {
		t.Parallel()
		sig := signStripeWebhook(t, testSecret, body)
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
		req.Header.Set("Stripe-Signature", sig)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.NotEqual(t, http.StatusUnauthorized,

			rr.Code)
	})

	t.Run("invalid_signature", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
		req.Header.Set("Stripe-Signature", "t=1234567890,v1=invalidsig")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t,
			http.StatusUnauthorized,

			rr.Code)
	})

	t.Run("missing_headers", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t,
			http.StatusUnauthorized,

			rr.Code)
	})

	t.Run("expired_timestamp", func(t *testing.T) {
		t.Parallel()
		oldTS := fmt.Sprintf("%d", time.Now().Add(-10*time.Minute).Unix())
		signedContent := oldTS + "." + string(body)
		mac := hmac.New(sha256.New, []byte(testSecret))
		mac.Write([]byte(signedContent))
		sig := fmt.Sprintf("t=%s,v1=%s", oldTS, hex.EncodeToString(mac.Sum(nil)))

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
		req.Header.Set("Stripe-Signature", sig)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t,
			http.StatusUnauthorized,

			rr.Code)
	})
}

func TestWebhookHandler_SubscriptionCreated(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_123",
			ProductID:  "pro-id",
			CustomerID: "cust_456",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000001"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t,
		http.StatusOK,
		rr.Code)
	require.NotNil(
		t, store.lastUpserted,
	)
	assert.Equal(t,
		"00000000-0000-0000-0000-000000000001",

		store.
			lastUpserted.OrgID,
	)
	assert.Equal(t,
		"pro", store.
			lastUpserted.
			PlanTier)
}

func TestWebhookHandler_SubscriptionCreated_EmptyOrgID_ReturnsError(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Subscription with no org_id in metadata -- should fail so Stripe retries.
	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_no_org",
			ProductID:  "pro-id",
			CustomerID: "cust_no_org",
			Metadata:   map[string]string{}, // no org_id
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.NotEqual(t, http.StatusOK,
		rr.Code,
	)
	assert.Nil(t, store.
		lastUpserted)

	// The handler should return a non-200 status so Stripe retries the webhook.
}

func TestWebhookHandler_SubscriptionRevoked(t *testing.T) {
	t.Parallel()

	pendingTier := "starter"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000002": {
				OrgID:           "00000000-0000-0000-0000-000000000002",
				PlanTier:        "pro",
				Status:          "active",
				PendingPlanTier: &pendingTier,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Stripe fires customer.subscription.deleted with CancelAtPeriodEnd=false for immediate revocation.
	payload := StripeWebhookPayload{
		Type: "customer.subscription.deleted",
		Data: mustJSON(t, testSubscriptionData{
			ID:                "sub_123",
			ProductID:         "pro-id",
			CustomerID:        "cust_456",
			CancelAtPeriodEnd: false,
			Metadata:          map[string]string{"org_id": "00000000-0000-0000-0000-000000000002"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t,
		http.StatusOK,
		rr.Code)
	require.NotNil(
		t, store.lastPlanUpdate,
	)
	assert.Equal(t,
		"free", store.
			lastPlanUpdate.
			tier)
	assert.Equal(t,
		"revoked",
		store.lastPlanUpdate.
			status)
	assert.Equal(t,
		"00000000-0000-0000-0000-000000000002",

		store.
			lastClearedPending,
	)
	require.Nil(t, store.
		subscriptions["00000000-0000-0000-0000-000000000002"].PendingPlanTier,
	)
}

func TestWebhookHandler_UnknownEventType(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("", "", "", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "some.unknown.event",
		Data: json.RawMessage(`{}`),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t,
		http.StatusOK,
		rr.Code)
}

func TestWebhookHandler_IdempotentUpsert(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_idem",
			ProductID:  "starter-id",
			CustomerID: "cust_idem",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000003"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	// Send twice
	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t,
			http.StatusOK,
			rr.Code)
	}
	assert.Equal(t, 2, store.upsertCount)
}

func TestWebhook_DuplicateCreatedPreservesSpendingLimit(t *testing.T) {
	t.Parallel()

	pendingTier := "free"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000004": {
				OrgID:                 "00000000-0000-0000-0000-000000000004",
				PlanTier:              "starter",
				Status:                "active",
				SpendingLimitMicrousd: 50000000, // $50
				LimitAction:           "notify",
				PendingPlanTier:       &pendingTier,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Re-deliver the same subscription.created webhook.
	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_dup",
			ProductID:  "starter-id",
			CustomerID: "cust_dup",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000004"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	// The spending limit should be preserved (not reset to -1).
	sub := store.subscriptions["00000000-0000-0000-0000-000000000004"]
	assert.EqualValues(t, 50000000,
		sub.SpendingLimitMicrousd,
	)
	assert.Equal(t,
		"notify",
		sub.LimitAction,
	)
	require.Nil(t, sub.
		PendingPlanTier,
	)
}

func TestWebhook_UpdatedRefreshesPeriodDates(t *testing.T) {
	t.Parallel()

	oldStart := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	oldEnd := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000005": {
				OrgID:              "00000000-0000-0000-0000-000000000005",
				PlanTier:           "starter",
				Status:             "active",
				CurrentPeriodStart: &oldStart,
				CurrentPeriodEnd:   &oldEnd,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	newStart := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	newEnd := time.Date(2025, 2, 28, 0, 0, 0, 0, time.UTC)

	payload := StripeWebhookPayload{
		Type: "customer.subscription.updated",
		Data: mustJSON(t, testSubscriptionData{
			ID:                 "sub_period",
			ProductID:          "starter-id",
			CustomerID:         "cust_period",
			CurrentPeriodStart: &newStart,
			CurrentPeriodEnd:   &newEnd,
			Metadata:           map[string]string{"org_id": "00000000-0000-0000-0000-000000000005"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.NotNil(
		t, store.lastFullUpdate,
	)
	assert.False(t,
		store.lastFullUpdate.
			periodStart ==
			nil ||
			!store.lastFullUpdate.
				periodStart.
				Equal(newStart))
	assert.False(t,
		store.lastFullUpdate.
			periodEnd ==
			nil || !store.lastFullUpdate.
			periodEnd.Equal(newEnd))
}

func TestWebhook_DowngradeDeferred(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000006": {
				OrgID:    "00000000-0000-0000-0000-000000000006",
				PlanTier: "pro",
				Status:   "active",
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Update from pro to starter (downgrade).
	payload := StripeWebhookPayload{
		Type: "customer.subscription.updated",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_down",
			ProductID:  "starter-id",
			CustomerID: "cust_down",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000006"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	// Plan should still be "pro" (not immediately downgraded).
	sub := store.subscriptions["00000000-0000-0000-0000-000000000006"]
	assert.Equal(t,
		"pro", sub.
			PlanTier)
	assert.Equal(t,
		"starter",
		store.lastPendingTier,
	)

	// Pending tier should be set.
}

func TestWebhook_UpgradeImmediate(t *testing.T) {
	t.Parallel()

	pendingTier := "free"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000007": {
				OrgID:           "00000000-0000-0000-0000-000000000007",
				PlanTier:        "starter",
				Status:          "active",
				PendingPlanTier: &pendingTier,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Update from starter to pro (upgrade).
	payload := StripeWebhookPayload{
		Type: "customer.subscription.updated",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_up",
			ProductID:  "pro-id",
			CustomerID: "cust_up",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000007"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.NotNil(
		t, store.lastFullUpdate,
	)
	assert.Equal(t,
		"pro", store.
			lastFullUpdate.
			tier)
	assert.Empty(t,
		store.
			lastPendingTier,
	)
	assert.Equal(t,
		"00000000-0000-0000-0000-000000000007",

		store.
			lastClearedPending,
	)
	require.Nil(t, store.
		subscriptions["00000000-0000-0000-0000-000000000007"].PendingPlanTier,
	)

	// Plan should be immediately updated to "pro".

	// No pending tier should be set.
}

func TestWebhook_CancellationThenUpgradeClearsPendingFreeTier(t *testing.T) {
	t.Parallel()

	pendingTier := "free"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000008": {
				OrgID:           "00000000-0000-0000-0000-000000000008",
				PlanTier:        "starter",
				Status:          "canceled",
				PendingPlanTier: &pendingTier,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.updated",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_reactivate",
			ProductID:  "pro-id",
			CustomerID: "cust_reactivate",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000008"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.Nil(t, store.
		subscriptions["00000000-0000-0000-0000-000000000008"].PendingPlanTier,
	)
	require.Equal(t,
		"pro", store.
			subscriptions["00000000-0000-0000-0000-000000000008"].PlanTier)
}

func TestWebhook_CanceledSetsPendingFreeTier(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000009": {
				OrgID:    "00000000-0000-0000-0000-000000000009",
				PlanTier: "pro",
				Status:   "active",
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	now := time.Now()
	// Stripe fires customer.subscription.deleted with CancelAtPeriodEnd=true for deferred cancellation.
	payload := StripeWebhookPayload{
		Type: "customer.subscription.deleted",
		Data: mustJSON(t, testSubscriptionData{
			ID:                "sub_cancel",
			ProductID:         "pro-id",
			CustomerID:        "cust_cancel",
			CanceledAt:        &now,
			CancelAtPeriodEnd: true,
			Metadata:          map[string]string{"org_id": "00000000-0000-0000-0000-000000000009"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	assert.Equal(t,
		"free", store.
			lastPendingTier,
	)

	// Verify pending tier was set to "free".

	// Plan should still be "pro" (not immediately changed).
	sub := store.subscriptions["00000000-0000-0000-0000-000000000009"]
	assert.Equal(t,
		"pro", sub.
			PlanTier)
	assert.Equal(t,
		"canceled",
		sub.Status)
}

func TestWebhook_CanceledWithNoPriorSubscription(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.deleted",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_noexist",
			ProductID:  "pro-id",
			CustomerID: "cust_noexist",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000000a"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	assert.Empty(t,
		store.
			lastPendingTier,
	)

	// No pending tier should be set since no subscription existed.
}

func TestWebhookHandler_SubscriptionCreated_SetsMonthlyUsageEmail(t *testing.T) {
	t.Parallel()

	t.Run("starter_plan_enables_monthly_usage_email", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

		payload := StripeWebhookPayload{
			Type: "customer.subscription.created",
			Data: mustJSON(t, testSubscriptionData{
				ID:         "sub_starter",
				ProductID:  "starter-id",
				CustomerID: "cust_starter",
				Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000000b"},
			}),
		}

		body, err := json.Marshal(payload)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		require.Equal(t,
			http.StatusOK,
			rr.Code)
		require.NotNil(
			t, store.lastUpserted,
		)
		assert.True(t,
			store.lastUpserted.
				MonthlyUsageEmail,
		)
	})

	t.Run("pro_plan_enables_monthly_usage_email", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")
		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

		payload := StripeWebhookPayload{
			Type: "customer.subscription.created",
			Data: mustJSON(t, testSubscriptionData{
				ID:         "sub_pro",
				ProductID:  "pro-id",
				CustomerID: "cust_pro",
				Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000000c"},
			}),
		}

		body, err := json.Marshal(payload)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		require.Equal(t,
			http.StatusOK,
			rr.Code)
		require.NotNil(
			t, store.lastUpserted,
		)
		assert.True(t,
			store.lastUpserted.
				MonthlyUsageEmail,
		)
	})
}

func TestWebhookHandler_SubscriptionCreated_WelcomeEmail(t *testing.T) {
	t.Parallel()

	t.Run("paid_plan_calls_welcome_email", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")

		type welcomeCall struct {
			orgID         string
			tier          domain.PlanTier
			customerEmail string
		}
		var calls []welcomeCall
		done := make(chan struct{}, 1)

		welcomeFn := func(_ context.Context, orgID string, tier domain.PlanTier, email string) error {
			calls = append(calls, welcomeCall{orgID: orgID, tier: tier, customerEmail: email})
			done <- struct{}{}
			return nil
		}

		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
			WithDevBypassSignatureCheck(), WithWelcomeEmail(welcomeFn))

		payload := StripeWebhookPayload{
			Type: "customer.subscription.created",
			Data: mustJSON(t, testSubscriptionData{
				ID:         "sub_welcome",
				ProductID:  "starter-id",
				CustomerID: "cust_welcome",
				Customer: &testCustomerData{
					ID:    "cust_welcome",
					Email: "user@example.com",
				},
				Metadata: map[string]string{"org_id": "00000000-0000-0000-0000-00000000000d"},
			}),
		}

		body, err := json.Marshal(payload)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		require.Equal(t,
			http.StatusOK,
			rr.Code)

		// Wait for the async goroutine to complete.
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			require.Fail(t, "timed out waiting for welcome email callback")
		}
		require.Len(t,
			calls, 1)
		assert.Equal(t,
			"00000000-0000-0000-0000-00000000000d",

			calls[0].orgID)
		assert.Equal(t,
			domain.PlanStarter,
			calls[0].tier)
		assert.Equal(t,
			"user@example.com",
			calls[0].customerEmail,
		)
	})

	t.Run("no_customer_email_skips_welcome", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")

		done := make(chan struct{}, 1)
		welcomeFn := func(_ context.Context, _ string, _ domain.PlanTier, _ string) error {
			select {
			case done <- struct{}{}:
			default:
			}
			return nil
		}

		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
			WithDevBypassSignatureCheck(), WithWelcomeEmail(welcomeFn))

		payload := StripeWebhookPayload{
			Type: "customer.subscription.created",
			Data: mustJSON(t, testSubscriptionData{
				ID:         "sub_noemail",
				ProductID:  "starter-id",
				CustomerID: "cust_noemail",
				Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000000e"},
			}),
		}

		body, err := json.Marshal(payload)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		require.Equal(t,
			http.StatusOK,
			rr.Code)

		select {
		case <-done:
			assert.Fail(t, "welcome email should not be called when customer email is empty")
		case <-time.After(200 * time.Millisecond):
		}
	})

	t.Run("no_welcome_fn_configured", func(t *testing.T) {
		t.Parallel()

		store := &mockBillingStore{}
		mapping := NewStripeMapping("starter-id", "", "pro-id", "")

		// No WithWelcomeEmail option -- should not panic.
		handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

		payload := StripeWebhookPayload{
			Type: "customer.subscription.created",
			Data: mustJSON(t, testSubscriptionData{
				ID:         "sub_nofn",
				ProductID:  "starter-id",
				CustomerID: "cust_nofn",
				Customer: &testCustomerData{
					ID:    "cust_nofn",
					Email: "user@example.com",
				},
				Metadata: map[string]string{"org_id": "00000000-0000-0000-0000-00000000000f"},
			}),
		}

		body, err := json.Marshal(payload)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		require.Equal(t,
			http.StatusOK,
			rr.Code)
	})
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)

	return b
}

// mockAuditStore records audit events for test assertions.
type mockAuditStore struct {
	events []*domain.AuditEvent
}

func (m *mockAuditStore) CreateAuditEvent(_ context.Context, event *domain.AuditEvent) error {
	m.events = append(m.events, event)
	return nil
}

func TestWebhook_SubscriptionCreated_CreatesAuditEvent(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	audit := &mockAuditStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_audit",
			ProductID:  "pro-id",
			CustomerID: "cust_audit",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000010"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.Len(t,
		audit.events,
		1)
	assert.Equal(t,
		"subscription.created",
		audit.
			events[0].Action,
	)
}

func TestWebhook_SubscriptionCreated_AuditDetails_ContainsPlanTier(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	audit := &mockAuditStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_details",
			ProductID:  "pro-id",
			CustomerID: "cust_details",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000011"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.Len(t,
		audit.events,
		1)

	var details map[string]string
	require.NoError(t, json.Unmarshal(audit.events[0].Details,
		&details))
	assert.Equal(t,
		"pro", details["plan_tier"])
	assert.Equal(t,
		"sub_details",
		details["stripe_subscription_id"])
}

func TestWebhook_SubscriptionUpdated_Upgrade_AuditHasPreviousTier(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000012": {
				OrgID:    "00000000-0000-0000-0000-000000000012",
				PlanTier: "starter",
				Status:   "active",
			},
		},
	}
	audit := &mockAuditStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.updated",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_upgrade_audit",
			ProductID:  "pro-id",
			CustomerID: "cust_upgrade_audit",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000012"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.Len(t,
		audit.events,
		1)

	var details map[string]string
	require.NoError(t, json.Unmarshal(audit.events[0].Details,
		&details))
	assert.Equal(t,
		"starter",
		details["previous_tier"])
	assert.Equal(t,
		"pro", details["plan_tier"])
}

func TestWebhook_SubscriptionUpdated_Downgrade_AuditHasPendingTier(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000013": {
				OrgID:    "00000000-0000-0000-0000-000000000013",
				PlanTier: "pro",
				Status:   "active",
			},
		},
	}
	audit := &mockAuditStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.updated",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_down_audit",
			ProductID:  "starter-id",
			CustomerID: "cust_down_audit",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000013"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.Len(t,
		audit.events,
		1)

	var details map[string]string
	require.NoError(t, json.Unmarshal(audit.events[0].Details,
		&details))
	assert.Equal(t,
		"starter",
		details["pending_plan_tier"])
	assert.Equal(t,
		"pro", details["previous_tier"])
}

func TestWebhook_SubscriptionCanceled_CreatesAuditEvent(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000014": {
				OrgID:    "00000000-0000-0000-0000-000000000014",
				PlanTier: "pro",
				Status:   "active",
			},
		},
	}
	audit := &mockAuditStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit, WithDevBypassSignatureCheck())

	now := time.Now()
	payload := StripeWebhookPayload{
		Type: "customer.subscription.deleted",
		Data: mustJSON(t, testSubscriptionData{
			ID:                "sub_cancel_audit",
			ProductID:         "pro-id",
			CustomerID:        "cust_cancel_audit",
			CanceledAt:        &now,
			CancelAtPeriodEnd: true,
			Metadata:          map[string]string{"org_id": "00000000-0000-0000-0000-000000000014"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.Len(t,
		audit.events,
		1)
	assert.Equal(t,
		"subscription.canceled",

		audit.events[0].Action,
	)
}

func TestWebhook_SubscriptionRevoked_CreatesAuditEvent(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000015": {
				OrgID:    "00000000-0000-0000-0000-000000000015",
				PlanTier: "pro",
				Status:   "active",
			},
		},
	}
	audit := &mockAuditStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit, WithDevBypassSignatureCheck())

	// Stripe fires customer.subscription.deleted with CancelAtPeriodEnd=false for immediate revocation.
	payload := StripeWebhookPayload{
		Type: "customer.subscription.deleted",
		Data: mustJSON(t, testSubscriptionData{
			ID:                "sub_revoke_audit",
			ProductID:         "pro-id",
			CustomerID:        "cust_revoke_audit",
			CancelAtPeriodEnd: false,
			Metadata:          map[string]string{"org_id": "00000000-0000-0000-0000-000000000015"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.Len(t,
		audit.events,
		1)
	assert.Equal(t,
		"subscription.revoked",
		audit.
			events[0].Action,
	)
}

func TestWebhook_AuditStore_Nil_DoesNotPanic(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	// Pass nil for auditStore - should not panic.
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_nil_audit",
			ProductID:  "pro-id",
			CustomerID: "cust_nil_audit",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000016"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
}

func TestWebhook_AuditEvent_HasCorrectResourceType(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	audit := &mockAuditStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_restype",
			ProductID:  "pro-id",
			CustomerID: "cust_restype",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000017"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.Len(t,
		audit.events,
		1)
	assert.Equal(t,
		"subscription",
		audit.events[0].ResourceType,
	)
	assert.Equal(t,
		"00000000-0000-0000-0000-000000000017",

		audit.
			events[0].ResourceID,
	)
	assert.Equal(t,
		"system",
		audit.events[0].
			ActorType)
	assert.Equal(t,
		"stripe-webhook",
		audit.events[0].ActorID)
}

// Grace period webhook tests.

func TestWebhook_PaymentFailed_SetsGracePeriod72h(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000018": {
				OrgID:         "00000000-0000-0000-0000-000000000018",
				PlanTier:      "pro",
				Status:        "active",
				PaymentStatus: "ok",
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Stripe fires invoice.payment_failed when a payment attempt fails.
	payload := StripeWebhookPayload{
		Type: "invoice.payment_failed",
		Data: mustJSON(t, testInvoiceData{
			ID:         "inv_pastdue",
			CustomerID: "cust_pastdue",
			SubID:      "sub_pastdue",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000018"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	before := time.Now()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	sub := store.subscriptions["00000000-0000-0000-0000-000000000018"]
	assert.Equal(t,
		"grace", sub.
			PaymentStatus,
	)
	require.NotNil(
		t, sub.GracePeriodEnd,
	)

	// Grace period should be roughly 72 hours from now.
	expected := before.Add(72 * time.Hour)
	diff := sub.GracePeriodEnd.Sub(expected)
	assert.False(t,
		diff < -5*
			time.Second ||
			diff > 5*time.Second,
	)
}

func TestWebhook_PaymentFailed_StatusBecomesGrace(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000019": {
				OrgID:         "00000000-0000-0000-0000-000000000019",
				PlanTier:      "starter",
				Status:        "active",
				PaymentStatus: "ok",
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Stripe fires invoice.payment_failed when a payment attempt fails.
	payload := StripeWebhookPayload{
		Type: "invoice.payment_failed",
		Data: mustJSON(t, testInvoiceData{
			ID:         "inv_grace",
			CustomerID: "cust_grace",
			SubID:      "sub_grace",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000019"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.NotNil(
		t, store.lastPaymentStatusUpdate,
	)
	assert.Equal(t,
		"grace", store.
			lastPaymentStatusUpdate.
			status,
	)
}

func TestWebhook_PaymentSucceeded_ClearsGracePeriod(t *testing.T) {
	t.Parallel()

	graceEnd := time.Now().Add(48 * time.Hour)
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-00000000001a": {
				OrgID:          "00000000-0000-0000-0000-00000000001a",
				PlanTier:       "pro",
				Status:         "active",
				PaymentStatus:  "grace",
				GracePeriodEnd: &graceEnd,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Stripe fires invoice.paid when payment actually recovers.
	payload := StripeWebhookPayload{
		Type: "invoice.paid",
		Data: mustJSON(t, testInvoiceData{
			ID:         "sub_recover",
			CustomerID: "cust_recover",
			SubID:      "sub_recover",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000001a"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	sub := store.subscriptions["00000000-0000-0000-0000-00000000001a"]
	assert.Equal(t,
		"ok", sub.
			PaymentStatus)
	assert.Nil(t, sub.GracePeriodEnd)
}

func TestWebhook_SubscriptionUpdatedActiveDoesNotClearRestriction(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-00000000001f": {
				OrgID:         "00000000-0000-0000-0000-00000000001f",
				PlanTier:      "pro",
				Status:        "active",
				PaymentStatus: "restricted",
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.updated",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_restricted",
			ProductID:  "pro-id",
			CustomerID: "cust_restricted",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000001f"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.False(t,
		store.lastPaymentStatusUpdate !=
			nil && store.
			lastPaymentStatusUpdate.
			status ==
			"ok")

	sub := store.subscriptions["00000000-0000-0000-0000-00000000001f"]
	require.Equal(t,
		"restricted",
		sub.PaymentStatus,
	)
}

func TestWebhook_PaymentFailed_AlreadyInGrace_Extends(t *testing.T) {
	t.Parallel()

	oldGrace := time.Now().Add(24 * time.Hour) // 24h left
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-00000000001b": {
				OrgID:          "00000000-0000-0000-0000-00000000001b",
				PlanTier:       "pro",
				Status:         "active",
				PaymentStatus:  "grace",
				GracePeriodEnd: &oldGrace,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Stripe fires invoice.payment_failed for each failed payment attempt.
	payload := StripeWebhookPayload{
		Type: "invoice.payment_failed",
		Data: mustJSON(t, testInvoiceData{
			ID:         "inv_extend",
			CustomerID: "cust_extend",
			SubID:      "sub_extend",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000001b"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	sub := store.subscriptions["00000000-0000-0000-0000-00000000001b"]
	assert.Equal(t,
		"grace", sub.
			PaymentStatus,
	)
	require.NotNil(
		t, sub.GracePeriodEnd,
	)
	assert.False(t,
		sub.GracePeriodEnd.
			Before(time.Now().Add(70*
				time.Hour)))

	// Grace period should be extended to ~72h from now, not the old 24h.
}

func TestWebhook_PaymentFailed_DoesNotRestoreRestrictedOrgToGrace(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000020": {
				OrgID:         "00000000-0000-0000-0000-000000000020",
				PlanTier:      "pro",
				Status:        "active",
				PaymentStatus: "restricted",
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "invoice.payment_failed",
		Data: mustJSON(t, testInvoiceData{
			ID:         "inv_restricted_failed",
			CustomerID: "cust_restricted_failed",
			SubID:      "sub_restricted_failed",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000020"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.False(t,
		store.lastPaymentStatusUpdate !=
			nil && store.
			lastPaymentStatusUpdate.
			status ==
			"grace")

	sub := store.subscriptions["00000000-0000-0000-0000-000000000020"]
	require.Equal(t,
		"restricted",
		sub.PaymentStatus,
	)
	require.Nil(t, sub.
		GracePeriodEnd)
}

func TestWebhook_PaymentFailed_FreeOrg_Ignored(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-00000000001c": {
				OrgID:         "00000000-0000-0000-0000-00000000001c",
				PlanTier:      "free",
				Status:        "active",
				PaymentStatus: "ok",
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	// Stripe fires invoice.payment_failed for each failed payment attempt.
	payload := StripeWebhookPayload{
		Type: "invoice.payment_failed",
		Data: mustJSON(t, testInvoiceData{
			ID:         "inv_free_pay",
			CustomerID: "cust_free_pay",
			SubID:      "sub_free_pay",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000001c"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	// Free org should not have grace period set.
	sub := store.subscriptions["00000000-0000-0000-0000-00000000001c"]
	assert.Equal(t,
		"ok", sub.
			PaymentStatus)
}

func TestWebhook_EmptySecretCloudMode_Rejects(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")

	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithEdition("cloud"))

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_1",
			ProductID:  "starter-id",
			CustomerID: "cust_1",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000001d"},
		}),
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	require.Equal(t,
		http.StatusServiceUnavailable,

		rec.Code)
}

func TestWebhook_EmptySecretCommunityMode_Rejects(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")

	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithEdition("community"))

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_1",
			ProductID:  "starter-id",
			CustomerID: "cust_1",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000001d"},
		}),
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	require.Equal(t,
		http.StatusServiceUnavailable,

		rec.Code)
}

func TestWebhook_EmptySecretDefaultEdition_Rejects(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")

	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_1",
			ProductID:  "starter-id",
			CustomerID: "cust_1",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000001d"},
		}),
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	require.Equal(t,
		http.StatusServiceUnavailable,

		rec.Code)
}

func TestWebhook_EmptySecretWithDevBypass_Allows(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")

	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_1",
			ProductID:  "starter-id",
			CustomerID: "cust_1",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-00000000001d"},
		}),
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	require.NotEqual(t, http.StatusServiceUnavailable,

		rec.Code,
	)
}

func TestWebhook_InvoiceUncollectible_SetsRestricted(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000050": {
				OrgID:         "00000000-0000-0000-0000-000000000050",
				PlanTier:      "pro",
				Status:        "active",
				PaymentStatus: "ok",
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "invoice.marked_uncollectible",
		Data: mustJSON(t, testInvoiceData{
			ID:         "inv_uncoll",
			CustomerID: "cust_uncoll",
			SubID:      "sub_uncoll",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000050"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	sub := store.subscriptions["00000000-0000-0000-0000-000000000050"]
	assert.Equal(t,
		"restricted",
		sub.PaymentStatus,
	)
}

func FuzzWebhookSignatureHeader(f *testing.F) {
	f.Add("v1,abc123")
	f.Add("")
	f.Add("v1,")
	f.Add("v2,something")
	f.Add("v1,dGVzdA== v1,aW52YWxpZA==")
	f.Add(strings.Repeat("v1,x", 1000))

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte("test-secret-key-32bytes-long!!!!"))
	handler := NewWebhookHandler(store, mapping, secret, slog.Default(), nil, nil)

	f.Fuzz(func(t *testing.T, sigHeader string) {
		payload := `{"type":"customer.subscription.created","data":{"id":"sub_1","product_id":"starter-id","customer_id":"cust_1","status":"active","metadata":{"org_id":"00000000-0000-0000-0000-00000000001d"}}}`
		req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(payload))
		req.Header.Set("webhook-id", "msg_test")
		req.Header.Set("webhook-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
		req.Header.Set("webhook-signature", sigHeader)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		// Must never panic, regardless of signature header content.
		// Valid responses: 200 (if sig matches), 401 (if sig doesn't match),
		// or other codes (e.g., 400 for bad payload). The key assertion is
		// that no panic occurred -- reaching this point means the handler
		// survived the fuzzed input.
		_ = rec.Code
	})
}

// Issue 8: UpsertEnterpriseContract failure must return error so Stripe retries.
func TestWebhook_EnterpriseContractFailure_ReturnsError(t *testing.T) {
	t.Parallel()

	RegisterEnterprisePriceTier("fix8_ent_starter", EnterpriseTierStarter)

	contractErr := fmt.Errorf("db connection refused")
	store := &mockBillingStore{
		subscriptions: make(map[string]*OrgSubscription),
		upsertEnterpriseContractFn: func(_ context.Context, _ *EnterpriseContract) error {
			return contractErr
		},
	}
	mapping := NewStripeMappingFromOptions(
		WithEnterpriseStarterPrice("fix8_ent_starter"),
	)
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_fix8",
			ProductID:  "fix8_ent_starter",
			CustomerID: "cust_fix8",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000f08"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t,
		http.StatusInternalServerError,

		rr.Code)

	// The handler must return 500 so Stripe retries the webhook.
}

// Issue 8: When UpsertEnterpriseContract succeeds, webhook returns 200.
func TestWebhook_EnterpriseContractSuccess_Returns200(t *testing.T) {
	t.Parallel()

	RegisterEnterprisePriceTier("fix8_ok_starter", EnterpriseTierStarter)

	store := &mockBillingStore{
		subscriptions: make(map[string]*OrgSubscription),
	}
	mapping := NewStripeMappingFromOptions(
		WithEnterpriseStarterPrice("fix8_ok_starter"),
	)
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_fix8_ok",
			ProductID:  "fix8_ok_starter",
			CustomerID: "cust_fix8_ok",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000f80"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t,
		http.StatusOK,
		rr.Code)

	// Verify contract was actually created.
	if _, err := store.GetEnterpriseContract(context.Background(), "00000000-0000-0000-0000-000000000f80"); err != nil {
		assert.Failf(t, "test failure",

			"enterprise contract should exist: %v", err)
	}
}

// Issue 9: Audit event for enterprise upgrade fires when old tier differs from new tier.
func TestWebhook_EnterpriseUpgradeAudit_FiresOnTransition(t *testing.T) {
	t.Parallel()

	RegisterEnterprisePriceTier("fix9_ent_starter", EnterpriseTierStarter)

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000f09": {
				OrgID:    "00000000-0000-0000-0000-000000000f09",
				PlanTier: "pro",
				Status:   "active",
			},
		},
	}
	audit := &mockAuditStore{}
	mapping := NewStripeMappingFromOptions(
		WithStarterPrices("starter-id", ""),
		WithProPrices("pro-id", ""),
		WithEnterpriseStarterPrice("fix9_ent_starter"),
	)
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_fix9",
			ProductID:  "fix9_ent_starter",
			CustomerID: "cust_fix9",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000f09"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	// Should have 2 audit events: subscription.upgraded_to_enterprise and subscription.created.
	foundUpgradeAudit := false
	for _, ev := range audit.events {
		if ev.Action == "subscription.upgraded_to_enterprise" {
			foundUpgradeAudit = true
			var details map[string]string
			require.NoError(t, json.Unmarshal(ev.Details,
				&details))
			assert.Equal(t,
				"pro", details["previous_plan"])
			assert.Equal(t,
				"enterprise",
				details["new_plan"])
		}
	}
	assert.True(t,
		foundUpgradeAudit,
	)
}

// Issue 9: Enterprise upgrade audit must NOT fire when already on enterprise tier.
func TestWebhook_EnterpriseUpgradeAudit_NoFireWhenAlreadyEnterprise(t *testing.T) {
	t.Parallel()

	RegisterEnterprisePriceTier("fix9_no_fire", EnterpriseTierStarter)

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-000000000f90": {
				OrgID:    "00000000-0000-0000-0000-000000000f90",
				PlanTier: "enterprise", // already enterprise
				Status:   "active",
			},
		},
	}
	audit := &mockAuditStore{}
	mapping := NewStripeMappingFromOptions(
		WithEnterpriseStarterPrice("fix9_no_fire"),
	)
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, audit, WithDevBypassSignatureCheck())

	payload := StripeWebhookPayload{
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_fix9_nofire",
			ProductID:  "fix9_no_fire",
			CustomerID: "cust_fix9_nofire",
			Metadata:   map[string]string{"org_id": "00000000-0000-0000-0000-000000000f90"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)

	for _, ev := range audit.events {
		assert.NotEqual(t, "subscription.upgraded_to_enterprise",

			ev.Action)
	}
}

// Issue 12: Downgrade webhook uses atomic SetPendingDowngrade instead of separate calls.
func TestWebhook_DowngradeUsesAtomicSetPendingDowngrade(t *testing.T) {
	t.Parallel()

	periodStart := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"00000000-0000-0000-0000-00000000f012": {
				OrgID:              "00000000-0000-0000-0000-00000000f012",
				PlanTier:           "pro",
				Status:             "active",
				CurrentPeriodStart: &periodStart,
				CurrentPeriodEnd:   &periodEnd,
			},
		},
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	newStart := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	newEnd := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)

	payload := StripeWebhookPayload{
		Type: "customer.subscription.updated",
		Data: mustJSON(t, testSubscriptionData{
			ID:                 "sub_fix12",
			ProductID:          "starter-id",
			CustomerID:         "cust_fix12",
			CurrentPeriodStart: &newStart,
			CurrentPeriodEnd:   &newEnd,
			Metadata:           map[string]string{"org_id": "00000000-0000-0000-0000-00000000f012"},
		}),
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t,
		http.StatusOK,
		rr.Code)
	require.NotNil(
		t, store.lastPendingDowngrade,
	)
	assert.Equal(t,
		"00000000-0000-0000-0000-00000000f012",

		store.
			lastPendingDowngrade.
			orgID)
	assert.Equal(t,
		"starter",
		store.lastPendingDowngrade.
			pendingTier,
	)
	assert.Nil(t, store.
		lastFullUpdate,
	)
	assert.False(t,
		store.lastPendingDowngrade.
			periodStart ==
			nil || !store.lastPendingDowngrade.
			periodStart.Equal(newStart))
	assert.False(t,
		store.lastPendingDowngrade.
			periodEnd == nil ||
			!store.lastPendingDowngrade.
				periodEnd.
				Equal(newEnd))

	// Verify SetPendingDowngrade was called atomically (not SetPendingPlanTier + UpdateOrgSubscriptionFull).

	// Verify the full update was NOT called separately (proving atomicity).

	// Verify period dates were passed through.

	// Verify the current plan tier is preserved (not overwritten to starter).
	sub := store.subscriptions["00000000-0000-0000-0000-00000000f012"]
	assert.Equal(t,
		"pro", sub.
			PlanTier)
	assert.False(t,
		sub.PendingPlanTier ==
			nil ||
			*sub.PendingPlanTier !=
				"starter",
	)
}

// Issue 15: ListOrgsWithPendingDowngrade includes MonthlyUsageEmail in returned data.
// This is a mock-level test; the pg_store fix adds the column to the real SQL query.
func TestMockStore_ListOrgsWithPendingDowngrade_IncludesMonthlyUsageEmail(t *testing.T) {
	t.Parallel()

	pendingTier := "free"
	pastEnd := time.Now().Add(-1 * time.Hour)
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-email-true": {
				OrgID:             "org-email-true",
				PlanTier:          "pro",
				Status:            "active",
				PendingPlanTier:   &pendingTier,
				CurrentPeriodEnd:  &pastEnd,
				MonthlyUsageEmail: true,
			},
			"org-email-false": {
				OrgID:             "org-email-false",
				PlanTier:          "starter",
				Status:            "active",
				PendingPlanTier:   &pendingTier,
				CurrentPeriodEnd:  &pastEnd,
				MonthlyUsageEmail: false,
			},
		},
	}

	subs, err := store.ListOrgsWithPendingDowngrade(context.Background())
	require.NoError(t, err)
	require.Len(t,
		subs, 2)

	emailByOrg := make(map[string]bool)
	for _, sub := range subs {
		emailByOrg[sub.OrgID] = sub.MonthlyUsageEmail
	}
	assert.True(t,
		emailByOrg["org-email-true"])
	assert.False(t,
		emailByOrg["org-email-false"])
}
