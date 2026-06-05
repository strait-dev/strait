//go:build integration

package billing_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v82"
)

// signStripeWebhookForTest signs a body for the integration test in the
// billing_test package. We can't reuse the unexported helper from
// webhook_test.go, so a small inline copy keeps the test self-contained.
func signStripeWebhookForTest(t *testing.T, secret string, body []byte) string {
	t.Helper()
	ts := fmt.Sprintf("%d", time.Now().Unix())
	signed := ts + "." + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signed))
	return fmt.Sprintf("t=%s,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

// buildSubscriptionCreatedEvent assembles a Stripe customer.subscription.created
// event payload that points at the Pro price and carries the given org_id.
func buildSubscriptionCreatedEvent(t *testing.T, eventID, subID, customerID, priceID, orgID string) []byte {
	t.Helper()
	sub := stripe.Subscription{
		ID:     subID,
		Status: stripe.SubscriptionStatusActive,
		Customer: &stripe.Customer{
			ID: customerID,
		},
		Items: &stripe.SubscriptionItemList{
			Data: []*stripe.SubscriptionItem{
				{Price: &stripe.Price{ID: priceID}},
			},
		},
		Metadata: map[string]string{"org_id": orgID},
	}
	rawSub, err := json.Marshal(sub)
	require.NoError(t, err)

	envelope := map[string]any{
		"id":   eventID,
		"type": "customer.subscription.created",
		"data": map[string]any{"object": json.RawMessage(rawSub)},
	}
	body, err := json.Marshal(envelope)
	require.NoError(t, err)

	return body
}

// TestWebhookHandler_StripeReplayIdempotency posts the same Stripe event three
// times against the real PgStore and asserts:
//   - All three requests return 200.
//   - The subscription is upserted exactly once.
//   - The processed_webhook_messages row is in status='processed'.
//
// This is the regression we care about: Stripe retries are aggressive, and a
// non-idempotent handler would double-flip plan tier or audit-log spam.
func TestWebhookHandler_StripeReplayIdempotency(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	const (
		proPrice = "price_pro_integration"
		secret   = "whsec_integration_test"
	)

	pgStore := billing.NewPgStore(testDB.Pool)
	mapping := billing.NewStripeMapping("", "", proPrice, "")
	handler := billing.NewWebhookHandler(pgStore, mapping, secret, slog.Default(), nil, nil)

	orgID := "00000000-0000-0000-0000-000000000abc"
	eventID := "evt_integration_" + newID()
	subID := "sub_integration_" + newID()
	customerID := "cust_integration_" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		SetPendingPlanTier(ctx, orgID,
			"pro"))

	// Seed a pending-intent row so resolveOrgIDForNewSubscription accepts the
	// org_id from metadata. In production this row is written by the API when
	// the checkout session is created.

	body := buildSubscriptionCreatedEvent(t, eventID, subID, customerID, proPrice, orgID)

	for attempt := 1; attempt <= 3; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
		req.Header.Set("Stripe-Signature", signStripeWebhookForTest(t, secret, body))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK,

			rec.
				Code)

	}

	// Subscription must exist, in Pro tier, with the customer attached.
	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, "pro", sub.
		PlanTier,
	)
	assert.False(t, sub.StripeCustomerID ==
		nil ||
		*sub.StripeCustomerID !=
			customerID)

	// The processed_webhook_messages row must be present and marked processed
	// exactly once — duplicate posts should not duplicate the row.
	var rowCount int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM processed_webhook_messages WHERE msg_id = $1",

		eventID,
	).Scan(&rowCount))
	assert.EqualValues(t, 1, rowCount)

	processed, err := pgStore.IsWebhookProcessed(ctx, eventID)
	require.NoError(t, err)
	assert.True(t, processed)

}

// TestWebhookHandler_RejectsUnknownPrice verifies that an event referencing a
// price ID we don't know about is rejected (so Stripe will retry, surfacing a
// misconfiguration) without leaving a half-baked subscription row behind.
func TestWebhookHandler_RejectsUnknownPrice(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	const (
		knownPrice = "price_pro_only"
		secret     = "whsec_unknown_price"
	)

	pgStore := billing.NewPgStore(testDB.Pool)
	mapping := billing.NewStripeMapping("", "", knownPrice, "")
	handler := billing.NewWebhookHandler(pgStore, mapping, secret, slog.Default(), nil, nil)

	orgID := "00000000-0000-0000-0000-000000000def"
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		SetPendingPlanTier(ctx, orgID,
			"pro"))

	body := buildSubscriptionCreatedEvent(t,
		"evt_unknown_"+newID(),
		"sub_unknown_"+newID(),
		"cust_unknown_"+newID(),
		"price_NOT_IN_MAPPING",
		orgID,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", signStripeWebhookForTest(t, secret, body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError,

		rec.Code,
	)

	// The pre-seeded pending row must not have been upgraded — an unknown
	// price must never silently promote an org to a paid tier.
	sub, err := pgStore.GetOrgSubscription(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, string(domain.
		PlanFree,
	), sub.
		PlanTier)
	assert.Nil(t, sub.
		StripeSubscriptionID,
	)

}
