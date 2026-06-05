//go:build cloud

package billing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v82"
)

// stubLookup is a minimal CustomerLookupStore for issuer tests.
type stubLookup struct {
	sub *OrgSubscription
	err error
}

func (s stubLookup) GetOrgSubscription(_ context.Context, _ string) (*OrgSubscription, error) {
	return s.sub, s.err
}

// recordedRequest captures one request received by the fake Stripe.
type recordedRequest struct {
	Method         string
	Path           string
	Body           string
	IdempotencyKey string
}

// stripeFake stands up an httptest server that mimics the Stripe REST
// shape we exercise: invoice list, credit-note create, and customer-
// balance-transaction create.
type stripeFake struct {
	mu       sync.Mutex
	requests []recordedRequest

	invoiceIDByStatus              map[string]string // "open" / "paid" → invoice ID; missing = empty list
	invoiceAmountRemainingByStatus map[string]int64  // "open" / "paid" → amount_remaining cents
	creditNoteID                   string
	balanceTxnID                   string
	failNext                       int // when > 0, return 500 for next N writes
	server                         *httptest.Server
}

func newStripeFake(t *testing.T) *stripeFake {
	t.Helper()
	f := &stripeFake{
		invoiceIDByStatus:              map[string]string{},
		invoiceAmountRemainingByStatus: map[string]int64{},
		creditNoteID:                   "cn_test_123",
		balanceTxnID:                   "cbtxn_test_123",
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)

	prev := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
		URL:               stripe.String(f.server.URL),
		MaxNetworkRetries: stripe.Int64(0),
		LeveledLogger:     &stripe.LeveledLogger{Level: stripe.LevelNull},
	}))
	t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, prev) })
	return f
}

func (f *stripeFake) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	rec := recordedRequest{
		Method:         r.Method,
		Path:           r.URL.Path,
		Body:           string(body) + "?" + r.URL.RawQuery,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
	}
	f.mu.Lock()
	f.requests = append(f.requests, rec)
	failNow := false
	if f.failNext > 0 && r.Method == http.MethodPost {
		f.failNext--
		failNow = true
	}
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if failNow {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"forced failure","type":"api_error"}}`))
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/invoices":
		status := r.URL.Query().Get("status")
		id, ok := f.invoiceIDByStatus[status]
		if !ok || id == "" {
			_, _ = fmt.Fprintf(w, `{"object":"list","data":[],"has_more":false,"url":"/v1/invoices"}`)
			return
		}
		amountRemaining := f.invoiceAmountRemainingByStatus[status]
		_, _ = fmt.Fprintf(w, `{"object":"list","data":[{"id":%q,"object":"invoice","status":%q,"amount_remaining":%d}],"has_more":false,"url":"/v1/invoices"}`, id, status, amountRemaining)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/credit_notes":
		_, _ = fmt.Fprintf(w, `{"id":%q,"object":"credit_note"}`, f.creditNoteID)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/customers/") && strings.HasSuffix(r.URL.Path, "/balance_transactions"):
		_, _ = fmt.Fprintf(w, `{"id":%q,"object":"customer_balance_transaction"}`, f.balanceTxnID)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, `{"error":{"message":"unhandled path %s %s"}}`, r.Method, r.URL.Path)
	}
}

func (f *stripeFake) snapshot() []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]recordedRequest, len(f.requests))
	copy(out, f.requests)
	return out
}

func subWithCustomer(customerID string) *OrgSubscription {
	return &OrgSubscription{OrgID: "org-1", StripeCustomerID: stripe.String(customerID)}
}

func newTestIssuer(store CustomerLookupStore) *StripeSLAIssuer {
	return NewStripeSLAIssuer("sk_test_fake", store, slog.New(slog.DiscardHandler))
}

// TestStripeSLAIssuer_OpenInvoice_IssuesCreditNote guards the happy path:
// an open invoice exists for the customer, so the issuer creates a Stripe
// credit note and threads the canonical idempotency key.
func TestStripeSLAIssuer_OpenInvoice_IssuesCreditNote(t *testing.T) {
	fake := newStripeFake(t)
	fake.invoiceIDByStatus["open"] = "in_test_open"
	fake.invoiceAmountRemainingByStatus["open"] = 10_000 // $100 still owed

	issuer := newTestIssuer(stubLookup{sub: subWithCustomer("cus_test_123")})
	periodEnd := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	id, err := issuer.IssueCredit(context.Background(), "org-1", 50_000_000, periodEnd)
	require.NoError(t, err)
	require.Equal(
		t,
		"cn_test_123",
		id)

	reqs := fake.snapshot()
	var sawCreditNote bool
	for _, r := range reqs {
		if r.Method == http.MethodPost && r.Path == "/v1/credit_notes" {
			sawCreditNote = true
			require.Equal(
				t,
				"sla-credit-org-1-2026-05",

				r.IdempotencyKey,
			)
			require.True(t,

				strings.Contains(r.
					Body, "invoice=in_test_open",
				))
			require.True(t,

				strings.Contains(r.
					Body, "amount=5000",
				))

		}
	}
	require.True(t,

		sawCreditNote,
	)

}

func TestDeepSecStripeSLAIssuer_OpenInvoiceBelowCreditFallsBackToBalanceTxn(t *testing.T) {
	fake := newStripeFake(t)
	fake.invoiceIDByStatus["open"] = "in_test_low_balance"
	fake.invoiceAmountRemainingByStatus["open"] = 1_000 // $10 still owed

	issuer := newTestIssuer(stubLookup{sub: subWithCustomer("cus_low_balance")})
	periodEnd := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	id, err := issuer.IssueCredit(context.Background(), "org-1", 50_000_000, periodEnd)
	require.NoError(t, err)
	require.Equal(
		t,
		"cbtxn_test_123",

		id)

	reqs := fake.snapshot()
	for _, r := range reqs {
		require.False(
			t,
			r.Method ==
				http.MethodPost &&
				r.Path ==
					"/v1/credit_notes",
		)

	}
	var sawCBT bool
	for _, r := range reqs {
		if r.Method == http.MethodPost && strings.HasSuffix(r.Path, "/balance_transactions") {
			sawCBT = true
		}
	}
	require.True(t,

		sawCBT)

}

// TestStripeSLAIssuer_PaidInvoice_FallsThroughToBalanceTxn guards the
// post-payment branch: when the only matching invoice is fully paid
// (amount_remaining == 0) Stripe rejects credit-note `amount` with a
// 400 ("must be less than invoice amount $0.00"), so the issuer must
// skip it and fall through to the customer-balance-transaction path.
// Net effect for the customer is identical — credit lands on their
// next invoice — but it gets there via the path Stripe actually
// accepts on a settled invoice.
func TestStripeSLAIssuer_PaidInvoice_FallsThroughToBalanceTxn(t *testing.T) {
	fake := newStripeFake(t)
	// Only a paid invoice with no remaining balance — no open invoice.
	fake.invoiceIDByStatus["paid"] = "in_test_paid"
	fake.invoiceAmountRemainingByStatus["paid"] = 0

	issuer := newTestIssuer(stubLookup{sub: subWithCustomer("cus_paid")})
	periodEnd := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	id, err := issuer.IssueCredit(context.Background(), "org-1", 50_000_000, periodEnd)
	require.NoError(t, err)
	require.Equal(
		t,
		"cbtxn_test_123",

		id)

	reqs := fake.snapshot()
	for _, r := range reqs {
		require.False(
			t,
			r.Method ==
				http.MethodPost &&
				r.Path ==
					"/v1/credit_notes",
		)

	}
	var sawCBT bool
	for _, r := range reqs {
		if r.Method == http.MethodPost && strings.HasSuffix(r.Path, "/balance_transactions") {
			sawCBT = true
			require.Equal(
				t,
				"sla-credit-org-1-2026-05",

				r.IdempotencyKey,
			)

		}
	}
	require.True(t,

		sawCBT)

}

// TestStripeSLAIssuer_NoInvoice_FallsBackToBalanceTxn guards the
// trial / metered case: no invoices exist, so the issuer credits the
// customer balance instead.
func TestStripeSLAIssuer_NoInvoice_FallsBackToBalanceTxn(t *testing.T) {
	fake := newStripeFake(t)
	// no invoices in either status

	issuer := newTestIssuer(stubLookup{sub: subWithCustomer("cus_no_invoice")})
	periodEnd := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	id, err := issuer.IssueCredit(context.Background(), "org-1", 12_500_000, periodEnd)
	require.NoError(t, err)
	require.Equal(
		t,
		"cbtxn_test_123",

		id)

	reqs := fake.snapshot()
	var sawCBT bool
	for _, r := range reqs {
		if r.Method == http.MethodPost && strings.HasSuffix(r.Path, "/balance_transactions") {
			sawCBT = true
			require.Equal(
				t,
				"/v1/customers/cus_no_invoice/balance_transactions",

				r.Path)
			require.Equal(
				t,
				"sla-credit-org-1-2026-05",

				r.IdempotencyKey,
			)
			require.True(t,

				strings.Contains(r.
					Body, "amount=-1250",
				))
			require.True(t,

				strings.Contains(r.
					Body, "currency=usd",
				))

		}
	}
	require.True(t,

		sawCBT)

}

// TestStripeSLAIssuer_MissingCustomerID_NoStripeCall ensures we never
// hit Stripe when the org has no customer binding.
func TestStripeSLAIssuer_MissingCustomerID_NoStripeCall(t *testing.T) {
	fake := newStripeFake(t)
	issuer := newTestIssuer(stubLookup{sub: &OrgSubscription{OrgID: "org-1"}}) // nil StripeCustomerID

	id, err := issuer.IssueCredit(context.Background(), "org-1", 1_000_000, time.Now().UTC())
	require.Error(
		t,
		err)
	require.Len(t,

		fake.snapshot(), 0)

}

// TestStripeSLAIssuer_IdempotencyKeyStableAcrossCalls guards the
// retry-safety contract: re-running the same (orgID, periodEnd) in a
// month surfaces the same idempotency key, which Stripe uses to dedup.
func TestStripeSLAIssuer_IdempotencyKeyStableAcrossCalls(t *testing.T) {
	fake := newStripeFake(t)
	fake.invoiceIDByStatus["open"] = "in_test_open"
	fake.invoiceAmountRemainingByStatus["open"] = 10_000

	issuer := newTestIssuer(stubLookup{sub: subWithCustomer("cus_idem")})
	periodEnd := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	if _, err := issuer.IssueCredit(context.Background(), "org-1", 5_000_000, periodEnd); err != nil {
		require.Failf(t, "test failure",

			"first IssueCredit: %v", err)
	}
	if _, err := issuer.IssueCredit(context.Background(), "org-1", 5_000_000, periodEnd); err != nil {
		require.Failf(t, "test failure",

			"second IssueCredit: %v", err)
	}

	var keys []string
	for _, r := range fake.snapshot() {
		if r.Method == http.MethodPost && r.Path == "/v1/credit_notes" {
			keys = append(keys, r.IdempotencyKey)
		}
	}
	require.Len(t,

		keys, 2)
	require.Equal(
		t,
		keys[1],
		keys[0])

}

func TestStripeSLAIssuer_InvoiceLookupConstrainedToSLAPeriod(t *testing.T) {
	fake := newStripeFake(t)
	fake.invoiceIDByStatus["open"] = "in_test_open"
	fake.invoiceAmountRemainingByStatus["open"] = 10_000

	issuer := newTestIssuer(stubLookup{sub: subWithCustomer("cus_period")})
	periodEnd := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	if _, err := issuer.IssueCredit(context.Background(), "org-1", 5_000_000, periodEnd); err != nil {
		require.Failf(t, "test failure",

			"IssueCredit: %v", err)
	}

	var invoiceQuery url.Values
	for _, r := range fake.snapshot() {
		if r.Method == http.MethodGet && r.Path == "/v1/invoices" {
			rawQuery := strings.TrimPrefix(r.Body, "?")
			query, err := url.ParseQuery(rawQuery)
			require.NoError(t, err)

			invoiceQuery = query
			break
		}
	}
	require.NotNil(
		t, invoiceQuery,
	)

	wantStart := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC).Unix()
	wantEnd := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC).Unix()
	require.Equal(
		t,
		fmt.Sprint(wantStart), invoiceQuery.
			Get("created[gte]"))
	require.Equal(
		t,
		fmt.Sprint(wantEnd), invoiceQuery.
			Get("created[lt]"))
	require.Equal(
		t,
		"cus_period",
		invoiceQuery.
			Get("customer"))

}

// TestStripeSLAIssuer_StripeFailure_PropagatesWrappedError guards the
// "atomic on failure" contract relied on by the SLACalculator: a Stripe
// 5xx must surface as an error so the caller skips persisting the
// credit row.
func TestStripeSLAIssuer_StripeFailure_PropagatesWrappedError(t *testing.T) {
	fake := newStripeFake(t)
	fake.invoiceIDByStatus["open"] = "in_test_open"
	fake.invoiceAmountRemainingByStatus["open"] = 10_000
	fake.failNext = 1 // fail the credit-note POST

	issuer := newTestIssuer(stubLookup{sub: subWithCustomer("cus_fail")})
	periodEnd := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	id, err := issuer.IssueCredit(context.Background(), "org-1", 5_000_000, periodEnd)
	require.Error(
		t,
		err)
	require.True(t,

		strings.Contains(err.
			Error(),
			"create credit note",
		))

}

// TestStripeSLAIssuer_StoreError_NoStripeCall guards the lookup-failure
// path: if we can't load the subscription, we never call Stripe.
func TestStripeSLAIssuer_StoreError_NoStripeCall(t *testing.T) {
	fake := newStripeFake(t)
	sentinel := errors.New("db down")
	issuer := newTestIssuer(stubLookup{err: sentinel})

	_, err := issuer.IssueCredit(context.Background(), "org-1", 1_000_000, time.Now().UTC())
	require.True(t,

		errors.Is(err, sentinel))
	require.Len(t,

		fake.snapshot(), 0)

}

// TestStripeSLAIssuer_RoundsMicrousdToCents pins the rounding contract.
// Banker's rounding: 4_999 microusd (under a half-cent) → 0 cents, but
// the issuer rejects that as non-positive; 15_000 microusd lands on the
// half and rounds to the nearest even → 2 cents.
func TestStripeSLAIssuer_RoundsMicrousdToCents(t *testing.T) {
	cases := []struct {
		in   int64
		want int64
	}{
		{10_000, 1},
		{14_999, 1},
		{15_000, 2}, // banker's: half → even
		{15_001, 2},
		{25_000, 2}, // banker's: half → even
		{35_000, 4}, // banker's: half → even
	}
	for _, c := range cases {
		assert.Equal(t,

			c.want,
			microusdToCents(c.in),
		)

	}
}

func TestSLACreditPeriodLabel_UsesExclusivePeriodEnd(t *testing.T) {
	t.Parallel()
	periodEnd := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	require.Equal(
		t,
		"April 2026",
		slaCreditPeriodLabel(
			periodEnd,
		))

}
