//go:build cloud

package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/creditnote"
	"github.com/stripe/stripe-go/v82/customerbalancetransaction"
	"github.com/stripe/stripe-go/v82/invoice"
)

// StripeSLAIssuer creates a Stripe credit note for the org's most recent
// invoice in the SLA period, or — when no invoice exists — falls back to
// a negative customer-balance transaction. Either path is idempotent via
// (orgID, periodEnd)-derived idempotency key, so re-runs within the same
// month do not double-issue.
type StripeSLAIssuer struct {
	apiKey string
	store  CustomerLookupStore
	logger *slog.Logger
}

// NewStripeSLAIssuer constructs the cloud-only SLA credit issuer. The
// Stripe global API key is initialized once for the process; passing an
// empty key produces a usable struct that returns an error from
// IssueCredit so callers can detect misconfiguration on the first call.
func NewStripeSLAIssuer(apiKey string, store CustomerLookupStore, logger *slog.Logger) *StripeSLAIssuer {
	if logger == nil {
		logger = slog.Default()
	}
	ensureStripeKey(apiKey)
	return &StripeSLAIssuer{
		apiKey: apiKey,
		store:  store,
		logger: logger,
	}
}

// IssueCredit attempts to credit the org for an SLA breach. Returns the
// Stripe object ID (credit note or balance transaction) on success.
func (i *StripeSLAIssuer) IssueCredit(ctx context.Context, orgID string, creditMicrousd int64, periodEnd time.Time) (string, error) {
	if i.apiKey == "" {
		return "", errors.New("stripe sla issuer: api key not configured")
	}
	if creditMicrousd <= 0 {
		return "", fmt.Errorf("stripe sla issuer: non-positive credit %d microusd", creditMicrousd)
	}

	sub, err := i.store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		return "", fmt.Errorf("stripe sla issuer: load subscription for %s: %w", orgID, err)
	}
	if sub == nil || sub.StripeCustomerID == nil || *sub.StripeCustomerID == "" {
		return "", fmt.Errorf("stripe sla issuer: org %s has no stripe customer", orgID)
	}
	customerID := *sub.StripeCustomerID
	amountCents := microusdToCents(creditMicrousd)
	if amountCents <= 0 {
		return "", fmt.Errorf("stripe sla issuer: rounded credit is %d cents", amountCents)
	}
	idemKey := fmt.Sprintf("sla-credit-%s-%s", orgID, periodEnd.UTC().Format("2006-01"))

	if inv, ok := i.findInvoiceForPeriod(ctx, customerID, periodEnd); ok && inv.AmountRemaining >= amountCents {
		amount := amountCents
		params := &stripe.CreditNoteParams{
			Invoice: stripe.String(inv.ID),
			Amount:  &amount,
			Reason:  stripe.String(string(stripe.CreditNoteReasonOrderChange)),
			Memo:    stripe.String(fmt.Sprintf("SLA credit for %s", slaCreditPeriodLabel(periodEnd))),
		}
		params.Context = ctx
		params.SetIdempotencyKey(idemKey)
		cn, err := creditnote.New(params)
		if err != nil {
			return "", fmt.Errorf("stripe sla issuer: create credit note for %s: %w", orgID, err)
		}
		i.logger.Info("stripe sla credit note issued",
			"org_id", orgID,
			"customer_id", customerID,
			"invoice_id", inv.ID,
			"amount_cents", amountCents,
			"credit_note_id", cn.ID,
		)
		return cn.ID, nil
	}

	negAmount := -amountCents
	cbtParams := &stripe.CustomerBalanceTransactionParams{
		Customer:    stripe.String(customerID),
		Amount:      &negAmount,
		Currency:    stripe.String("usd"),
		Description: stripe.String(fmt.Sprintf("SLA credit for %s", slaCreditPeriodLabel(periodEnd))),
	}
	cbtParams.Context = ctx
	cbtParams.SetIdempotencyKey(idemKey)
	cbt, err := customerbalancetransaction.New(cbtParams)
	if err != nil {
		return "", fmt.Errorf("stripe sla issuer: create balance transaction for %s: %w", orgID, err)
	}
	i.logger.Info("stripe sla balance credit issued",
		"org_id", orgID,
		"customer_id", customerID,
		"amount_cents", -amountCents,
		"balance_transaction_id", cbt.ID,
	)
	return cbt.ID, nil
}

func slaCreditPeriodLabel(periodEnd time.Time) string {
	end := periodEnd.UTC()
	if end.IsZero() {
		return end.Format("January 2006")
	}
	return end.Add(-time.Nanosecond).Format("January 2006")
}

// findInvoiceForPeriod returns the most relevant invoice for the SLA
// period: an open invoice with a non-zero remaining balance. Stripe
// rejects credit notes against fully-paid invoices ("amount must be
// less than invoice amount ($0.00)"), so paid invoices are not
// candidates and the caller falls back to a customer-balance
// transaction — which ends up in the same place for the customer
// (negative balance applied to the next invoice).
func (i *StripeSLAIssuer) findInvoiceForPeriod(ctx context.Context, customerID string, periodEnd time.Time) (*stripe.Invoice, bool) {
	periodStart, periodEnd := slaCreditInvoiceWindow(periodEnd)
	if inv, ok := i.lookupInvoice(ctx, customerID, "open", periodStart, periodEnd); ok && inv.AmountRemaining > 0 {
		return inv, true
	}
	return nil, false
}

func slaCreditInvoiceWindow(periodEnd time.Time) (time.Time, time.Time) {
	labelInstant := periodEnd.UTC().Add(-time.Nanosecond)
	start := time.Date(labelInstant.Year(), labelInstant.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0)
}

func (i *StripeSLAIssuer) lookupInvoice(ctx context.Context, customerID, status string, periodStart, periodEnd time.Time) (*stripe.Invoice, bool) {
	params := &stripe.InvoiceListParams{
		Customer: stripe.String(customerID),
		Status:   stripe.String(status),
		CreatedRange: &stripe.RangeQueryParams{
			GreaterThanOrEqual: periodStart.Unix(),
			LesserThan:         periodEnd.Unix(),
		},
	}
	params.Filters.AddFilter("limit", "", "1")
	params.Context = ctx
	iter := invoice.List(params)
	if iter.Next() {
		if inv := iter.Invoice(); inv != nil && inv.ID != "" {
			return inv, true
		}
	}
	if err := iter.Err(); err != nil {
		i.logger.Warn("stripe sla issuer: invoice list failed",
			"customer_id", customerID,
			"status", status,
			"error", err,
		)
	}
	return nil, false
}

// microusdToCents converts micro-USD (1 unit = $0.000001) to cents
// (1 unit = $0.01). 10_000 microusd = 1 cent. Bankers' rounding so a
// micro-fraction does not systematically round up.
func microusdToCents(microusd int64) int64 {
	const scale = 10_000
	q := microusd / scale
	r := microusd % scale
	half := int64(scale / 2)
	switch {
	case r > half:
		return q + 1
	case r < half:
		return q
	default:
		if q%2 == 0 {
			return q
		}
		return q + 1
	}
}
