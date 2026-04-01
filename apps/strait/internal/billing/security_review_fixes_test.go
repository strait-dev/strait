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

// Issue 1: SumOrgPeriodSpend error must use boundedFailOpen, not bare nil.

func TestCheckSpendingLimit_SumSpendError_BoundedFailOpen(t *testing.T) {
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

	// First maxConsecutiveFailOpen calls should return nil (fail-open under threshold).
	for i := range maxConsecutiveFailOpen {
		err := enforcer.CheckSpendingLimit(ctx, "org-spend-err")
		if err != nil {
			t.Fatalf("call %d: expected nil (fail-open), got: %v", i+1, err)
		}
	}

	// Next call should fail-closed (threshold exceeded).
	err := enforcer.CheckSpendingLimit(ctx, "org-spend-err")
	if err == nil {
		t.Fatal("expected fail-closed error after threshold exceeded, got nil")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected *LimitError, got %T: %v", err, err)
	}
	if le.Code != "service_degraded" {
		t.Fatalf("expected code service_degraded, got %q", le.Code)
	}
}

func TestCheckSpendingLimit_SumSpendError_ResetOnSuccess(t *testing.T) {
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

	// Burn through most of the threshold.
	for range maxConsecutiveFailOpen - 1 {
		_ = enforcer.CheckSpendingLimit(ctx, "org-reset")
	}

	// Fix the error -- next call succeeds and resets the tracker.
	store.sumSpendErr = nil
	store.periodSpendByOrg = map[string]int64{"org-reset": 10_000_000}

	err := enforcer.CheckSpendingLimit(ctx, "org-reset")
	if err != nil {
		t.Fatalf("expected nil after error resolved, got: %v", err)
	}

	// Now re-introduce the error. Should get fresh threshold (not immediately fail-closed).
	store.sumSpendErr = fmt.Errorf("another temp error")
	err = enforcer.CheckSpendingLimit(ctx, "org-reset")
	if err != nil {
		t.Fatal("expected nil on first fail-open after reset, got error (counter was not reset)")
	}
}

func TestCheckSpendingLimit_SumSpendError_CrossOrgIndependent(t *testing.T) {
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

	// Exhaust threshold for org-A.
	for range maxConsecutiveFailOpen + 1 {
		_ = enforcer.CheckSpendingLimit(ctx, "org-A")
	}

	// org-B should still be under threshold.
	err := enforcer.CheckSpendingLimit(ctx, "org-B")
	if err != nil {
		t.Fatal("org-B should not be affected by org-A's fail-open threshold")
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
		WithEdition("community"))

	body := `{"type":"subscription.created","data":{"id":"sub_record_err","product_id":"starter-id","customer_id":"cust_1","status":"active","metadata":{"org_id":"550e8400-e29b-41d4-a716-446655440000"}}}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(body))
	req.Header.Set("webhook-id", "msg_record_err_test")
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

	// Verify the record was attempted.
	if !slices.Contains(store.recordedWebhookIDs, "msg_record_err_test") {
		t.Fatal("RecordProcessedWebhook was not called")
	}
}

func TestWebhook_RecordProcessedWebhookSuccess_IDStored(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithEdition("community"))

	payload := StripeWebhookPayload{
		Type: "subscription.created",
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
	req.Header.Set("webhook-id", "msg_success_test")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Verify the msg ID was recorded.
	if !slices.Contains(store.recordedWebhookIDs, "msg_success_test") {
		t.Fatal("RecordProcessedWebhook was not called on success")
	}
}

func TestWebhook_RecordProcessedWebhook_NotCalledOnHandlerError(t *testing.T) {
	t.Parallel()

	store := &mockBillingStore{}
	mapping := NewStripeMapping("starter-id", "", "pro-id", "")
	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil,
		WithEdition("community"))

	// Send a webhook with unknown product ID -- handler will return error.
	payload := StripeWebhookPayload{
		Type: "subscription.created",
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
	req.Header.Set("webhook-id", "msg_handler_err")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Handler error returns 500.
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for unknown product, got %d", rec.Code)
	}

	// RecordProcessedWebhook should NOT be called when handler errors.
	for _, id := range store.recordedWebhookIDs {
		if id == "msg_handler_err" {
			t.Fatal("RecordProcessedWebhook should not be called when handler returns error")
		}
	}
}
