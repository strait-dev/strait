//go:build integration && cloud

package billing_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"strait/internal/billing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/customer"
	"github.com/stripe/stripe-go/v82/customerbalancetransaction"
	"github.com/stripe/stripe-go/v82/invoice"
	"github.com/stripe/stripe-go/v82/invoiceitem"
)

// sandboxLookup is a CustomerLookupStore that returns a single hard-coded
// OrgSubscription. The Stripe sandbox owns the customer record; the
// PgStore-based composition is exercised by sla_pipeline_integration_test.go.
// This test focuses on the issuer's contract with api.stripe.com.
type sandboxLookup struct {
	sub *billing.OrgSubscription
}

func (s sandboxLookup) GetOrgSubscription(_ context.Context, _ string) (*billing.OrgSubscription, error) {
	return s.sub, nil
}

// TestStripeSLAIssuer_LiveSandbox_PaidInvoice_FallsThroughToBalanceTxn
// drives the issuer against the real Stripe sandbox with a customer that
// has a paid invoice (sandbox auto-collects on finalize). Stripe rejects
// credit notes against fully-paid invoices ("amount must be less than
// invoice amount ($0.00)"), so the issuer must skip the credit-note path
// and credit the customer balance instead. The net effect for the
// customer is identical — credit applies to their next invoice.
//
// Also exercises idempotency: a second IssueCredit with the same
// (orgID, periodEnd) must return the same Stripe object ID.
//
// Gated by STRAIT_STRIPE_INTEGRATION=1, same opt-in pattern as
// TestStripeCatalog_SandboxShape.
func TestStripeSLAIssuer_LiveSandbox_PaidInvoice_FallsThroughToBalanceTxn(t *testing.T) {
	if os.Getenv("STRAIT_STRIPE_INTEGRATION") != "1" {
		t.Skip("set STRAIT_STRIPE_INTEGRATION=1 to run; requires STRIPE_SECRET_KEY")
	}
	secret := os.Getenv("STRIPE_SECRET_KEY")
	require.NotEqual(t, "", secret)

	stripe.Key = secret

	ctx := context.Background()

	cus, err := customer.New(&stripe.CustomerParams{
		Description: stripe.String("strait sla issuer paid-invoice smoke"),
		Email:       stripe.String("sla-paid-smoke@strait.dev"),
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		if _, err := customer.Del(cus.ID, nil); err != nil {
			t.Logf("cleanup customer %s: %v", cus.ID, err)
		}
	})

	if _, err := invoiceitem.New(&stripe.InvoiceItemParams{
		Customer:    stripe.String(cus.ID),
		Amount:      stripe.Int64(5000),
		Currency:    stripe.String("usd"),
		Description: stripe.String("sla smoke seed line"),
	}); err != nil {
		require.Failf(t, "test failure",

			"create invoice item: %v", err)
	}

	inv, err := invoice.New(&stripe.InvoiceParams{
		Customer:         stripe.String(cus.ID),
		CollectionMethod: stripe.String("send_invoice"),
		DaysUntilDue:     stripe.Int64(30),
	})
	require.NoError(t, err)

	if _, err := invoice.FinalizeInvoice(inv.ID, &stripe.InvoiceFinalizeInvoiceParams{
		AutoAdvance: stripe.Bool(false),
	}); err != nil {
		require.Failf(t, "test failure",

			"finalize invoice: %v", err)
	}

	orgID := "org-sla-paid-sandbox-" + time.Now().UTC().Format("20060102T150405")
	store := sandboxLookup{sub: &billing.OrgSubscription{
		OrgID:            orgID,
		PlanTier:         "enterprise",
		StripeCustomerID: stripe.String(cus.ID),
	}}
	issuer := billing.NewStripeSLAIssuer(secret, store, slog.New(slog.DiscardHandler))

	periodEnd := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	const creditMicrousd = 10_000_000 // 1000 cents = $10.00

	firstID, err := issuer.IssueCredit(ctx, orgID, creditMicrousd, periodEnd)
	require.NoError(t, err)
	require.NotEqual(t, "", firstID)

	// Verify the returned ID is a customer balance transaction (the
	// fallback path) — not a credit note. Stripe IDs prefix the object
	// type: cn_… for credit notes, cbtxn_… for balance transactions.
	cbt, err := customerbalancetransaction.Get(firstID, &stripe.CustomerBalanceTransactionParams{
		Customer: stripe.String(cus.ID),
	})
	require.NoError(t, err)
	assert.EqualValues(t, -1000, cbt.Amount)

	// Second call with same (orgID, periodEnd) must reuse the
	// idempotency key and return the same Stripe object ID.
	secondID, err := issuer.IssueCredit(ctx, orgID, creditMicrousd, periodEnd)
	require.NoError(t, err)
	assert.Equal(
		t, firstID, secondID)

}

// TestStripeSLAIssuer_LiveSandbox_FallsBackToBalanceTransaction covers the
// no-invoice path: when the issuer can't find an invoice, it credits the
// customer balance instead. Same opt-in gate.
func TestStripeSLAIssuer_LiveSandbox_FallsBackToBalanceTransaction(t *testing.T) {
	if os.Getenv("STRAIT_STRIPE_INTEGRATION") != "1" {
		t.Skip("set STRAIT_STRIPE_INTEGRATION=1 to run; requires STRIPE_SECRET_KEY")
	}
	secret := os.Getenv("STRIPE_SECRET_KEY")
	require.NotEqual(t, "", secret)

	stripe.Key = secret

	ctx := context.Background()

	cus, err := customer.New(&stripe.CustomerParams{
		Description: stripe.String("strait sla issuer fallback smoke"),
		Email:       stripe.String("sla-fallback-smoke@strait.dev"),
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		if _, err := customer.Del(cus.ID, nil); err != nil {
			t.Logf("cleanup customer %s: %v", cus.ID, err)
		}
	})

	orgID := "org-sla-fallback-" + time.Now().UTC().Format("20060102T150405")
	store := sandboxLookup{sub: &billing.OrgSubscription{
		OrgID:            orgID,
		PlanTier:         "enterprise",
		StripeCustomerID: stripe.String(cus.ID),
	}}
	issuer := billing.NewStripeSLAIssuer(secret, store, slog.New(slog.DiscardHandler))

	periodEnd := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	const creditMicrousd = 10_000_000 // 1000 cents = $10.00

	id, err := issuer.IssueCredit(ctx, orgID, creditMicrousd, periodEnd)
	require.NoError(t, err)
	require.NotEqual(t, "", id)

	cbt, err := customerbalancetransaction.Get(id, &stripe.CustomerBalanceTransactionParams{
		Customer: stripe.String(cus.ID),
	})
	require.NoError(t, err)
	assert.EqualValues(t, -1000, cbt.Amount)

}
