package billing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/stripe-go/v82"
)

// TestInvoiceAuditFields_AmountDueIsMinorUnitsString locks the canonical
// field shape: minor units, encoded as a string via strconv.FormatInt to
// avoid float precision loss. Audit consumers (ClickHouse, SIEM forwarder)
// already parse string ints; an int64-shaped field would force a column
// type change downstream.
func TestInvoiceAuditFields_AmountDueIsMinorUnitsString(t *testing.T) {
	t.Parallel()
	got := invoiceAuditFields(&stripe.Invoice{
		ID:        "in_1",
		AmountDue: 4900,
		Currency:  stripe.CurrencyUSD,
	})
	assert.Equal(t, "4900",

		got["amount_due_minor"])

	if _, exists := got["amount_due"]; exists {
		assert.Failf(t, "test failure",

			"legacy amount_due field must be gone — replaced by amount_due_minor")
	}
}

// TestInvoiceAuditFields_LargeAmountSurvivesPrecision is the regression
// guard for a previous "%.2f" formatting path: a $99,999,999.99 invoice
// (top-tier custom contract) prints as 9999999999 minor units exactly and
// never reaches the double-precision rounding floor.
func TestInvoiceAuditFields_LargeAmountSurvivesPrecision(t *testing.T) {
	t.Parallel()
	got := invoiceAuditFields(&stripe.Invoice{
		ID:        "in_big",
		AmountDue: 9_999_999_999,
		Currency:  stripe.CurrencyUSD,
	})
	assert.Equal(t, "9999999999",

		got["amount_due_minor"],
	)
}

// TestInvoiceAuditFields_CurrencyIsLowercased proves the canonicalisation:
// Stripe documents lowercase but webhook payloads have shipped both, and
// dashboards keying off the raw value got duplicate buckets ("USD" vs
// "usd"). Lowercasing is the single point of truth.
func TestInvoiceAuditFields_CurrencyIsLowercased(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"USD", "Usd", "usd", "EUR"} {
		got := invoiceAuditFields(&stripe.Invoice{
			ID:        "in_cur",
			AmountDue: 0,
			Currency:  stripe.Currency(in),
		})
		want := map[string]string{"USD": "usd", "Usd": "usd", "usd": "usd", "EUR": "eur"}[in]
		assert.Equal(t, want,

			got["currency"])
	}
}

// TestInvoiceAuditFields_StripeInvoiceNumIncludedWhenPresent asserts the
// human-readable invoice number lands under the canonical key. Audit
// consumers correlate to billing emails by this number.
func TestInvoiceAuditFields_StripeInvoiceNumIncludedWhenPresent(t *testing.T) {
	t.Parallel()
	got := invoiceAuditFields(&stripe.Invoice{
		ID:        "in_with_num",
		Number:    "STRAIT-0001",
		AmountDue: 100,
		Currency:  stripe.CurrencyUSD,
	})
	assert.Equal(t, "STRAIT-0001",

		got["stripe_invoice_num"],
	)
}

// TestInvoiceAuditFields_StripeInvoiceNumOmittedWhenEmpty rejects blank
// cells in ClickHouse: we omit the field rather than emit it as empty so a
// query for invoice number IS NOT NULL has the correct semantics.
func TestInvoiceAuditFields_StripeInvoiceNumOmittedWhenEmpty(t *testing.T) {
	t.Parallel()
	got := invoiceAuditFields(&stripe.Invoice{
		ID:        "in_no_num",
		AmountDue: 0,
		Currency:  stripe.CurrencyUSD,
	})
	if _, present := got["stripe_invoice_num"]; present {
		assert.Failf(t, "test failure",

			"expected stripe_invoice_num to be omitted when empty, got map=%v", got)
	}
}

// TestInvoiceAuditFields_NilInvoiceReturnsEmpty guards against a panic
// when a malformed Stripe payload leaves invoice nil — we emit an empty
// map and let the caller decide whether to skip the audit row, but never
// crash the webhook handler.
func TestInvoiceAuditFields_NilInvoiceReturnsEmpty(t *testing.T) {
	t.Parallel()
	got := invoiceAuditFields(nil)
	assert.Empty(t, got)
}
