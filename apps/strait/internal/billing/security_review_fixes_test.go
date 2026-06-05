package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"strait/internal/domain"
)

// Issue 1: SumOrgPeriodSpend error must fail closed, not bypass spending caps.

func TestCheckSpendingLimit_SumSpendError_FailsClosed(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	ps := time.Now().Add(-24 * time.Hour)
	pe := time.Now().Add(24 * time.Hour)

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-spend-err": {
				OrgID:                 "org-spend-err",
				PlanTier:              string(domain.PlanStarter),
				Status:                "active",
				SpendingLimitMicrousd: 50_000_000,
				LimitAction:           "reject",
				CurrentPeriodStart:    &ps,
				CurrentPeriodEnd:      &pe,
			},
		},
		sumSpendErr: fmt.Errorf("simulated DB connection error"),
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	ctx := context.Background()

	err := enforcer.CheckSpendingLimit(ctx, "org-spend-err")
	if err == nil {
		t.Fatal("expected fail-closed error, got nil")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "service_degraded" {
		t.Fatalf("expected code service_degraded, got %q", le.Code)
	}
}

func TestCheckSpendingLimit_SumSpendError_SuccessAfterRecovery(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	ps := time.Now().Add(-24 * time.Hour)
	pe := time.Now().Add(24 * time.Hour)

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-reset": {
				OrgID:                 "org-reset",
				PlanTier:              string(domain.PlanStarter),
				Status:                "active",
				SpendingLimitMicrousd: 50_000_000,
				LimitAction:           "reject",
				CurrentPeriodStart:    &ps,
				CurrentPeriodEnd:      &pe,
			},
		},
		sumSpendErr: fmt.Errorf("temp error"),
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	ctx := context.Background()

	err := enforcer.CheckSpendingLimit(ctx, "org-reset")
	if err == nil {
		t.Fatal("expected initial spend read error to fail closed")
	}

	// Fix the error -- next call succeeds and resets the tracker.
	store.sumSpendErr = nil
	store.periodSpendByOrg = map[string]int64{"org-reset": 10_000_000}

	err = enforcer.CheckSpendingLimit(ctx, "org-reset")
	if err != nil {
		t.Fatalf("expected nil after error resolved, got: %v", err)
	}
}

func TestCheckSpendingLimit_SumSpendError_CrossOrgFailsClosed(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	ps := time.Now().Add(-24 * time.Hour)
	pe := time.Now().Add(24 * time.Hour)

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-A": {OrgID: "org-A", PlanTier: string(domain.PlanStarter), Status: "active", SpendingLimitMicrousd: 50_000_000, CurrentPeriodStart: &ps, CurrentPeriodEnd: &pe},
			"org-B": {OrgID: "org-B", PlanTier: string(domain.PlanStarter), Status: "active", SpendingLimitMicrousd: 50_000_000, CurrentPeriodStart: &ps, CurrentPeriodEnd: &pe},
		},
		sumSpendErr: fmt.Errorf("DB error"),
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	ctx := context.Background()

	for _, orgID := range []string{"org-A", "org-B"} {
		err := enforcer.CheckSpendingLimit(ctx, orgID)
		var le *LimitError
		if !errors.As(err, &le) {
			t.Fatalf("%s expected *LimitError, got %T: %v", orgID, err, err)
		}
		if le.Code != "service_degraded" {
			t.Fatalf("%s expected code service_degraded, got %q", orgID, le.Code)
		}
	}
}

// Issue 2: RecordProcessedWebhook error must be logged (not silently discarded).

func TestWebhook_RecordProcessedWebhookError_StillReturns200(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{
		recordWebhookErr: fmt.Errorf("simulated DB write error"),
	}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithDevBypassSignatureCheck(), WithEdition("community"))

	body := `{"id":"evt-record-err","type":"customer.subscription.created","data":{"object":{"id":"sub_record_err","status":"active","items":{"data":[{"price":{"id":"starter-id"},"current_period_start":1700000000,"current_period_end":1702592000}]},"customer":{"id":"cust_1","email":"test@example.com","metadata":{"org_id":"550e8400-e29b-41d4-a716-446655440000"}}}}}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Should still return 200 (webhook processed successfully, recording is best-effort).
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 even with record error, got %d", rec.Code)
	}

	// Verify the webhook was processed (subscription upserted).
	if store.lastUpserted == nil {
		t.Fatal("expected subscription to be upserted despite record error")
	}

	// Verify the record was attempted using the Stripe event ID.
	if !slices.Contains(store.recordedWebhookIDs, "evt-record-err") {
		t.Fatal("RecordProcessedWebhook was not called")
	}
}

func TestWebhook_RecordProcessedWebhookSuccess_IDStored(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithDevBypassSignatureCheck(), WithEdition("community"))

	payload := StripeWebhookPayload{
		ID:   "evt-record-ok",
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_record_ok",
			ProductID:  "starter-id",
			CustomerID: "cust_1",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "550e8400-e29b-41d4-a716-446655440000"},
		}),
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Verify the Stripe event ID was recorded.
	if !slices.Contains(store.recordedWebhookIDs, "evt-record-ok") {
		t.Fatal("RecordProcessedWebhook was not called on success")
	}
}

func TestWebhook_RecordProcessedWebhook_NotCalledOnHandlerError(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithDevBypassSignatureCheck(), WithEdition("community"))

	// Send a webhook with unknown product ID -- handler will return error.
	payload := StripeWebhookPayload{
		ID:   "evt-handler-err",
		Type: "customer.subscription.created",
		Data: mustJSON(t, testSubscriptionData{
			ID:         "sub_err",
			ProductID:  "unknown-product-id",
			CustomerID: "cust_1",
			Status:     "active",
			Metadata:   map[string]string{"org_id": "550e8400-e29b-41d4-a716-446655440000"},
		}),
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Handler error returns 500.
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for unknown product, got %d", rec.Code)
	}

	// RecordProcessedWebhook should NOT be called when handler errors.
	for _, id := range store.recordedWebhookIDs {
		if id == "evt-handler-err" {
			t.Fatal("RecordProcessedWebhook should not be called when handler returns error")
		}
	}
}
