//go:build integration && cloud

package billing_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"strait/internal/billing"

	"github.com/stripe/stripe-go/v82"
)

// slaStripeFake mirrors the issuer-level fake but lives in the
// _test (external) package so it can sit alongside the integration
// suite without changing the issuer's stripe.Backend wiring.
type slaStripeFake struct {
	mu       sync.Mutex
	requests []slaRecordedRequest
	server   *httptest.Server

	invoiceOpenID string
}

type slaRecordedRequest struct {
	Method         string
	Path           string
	Body           string
	IdempotencyKey string
}

func newSLAStripeFake(t *testing.T, invoiceOpenID string) *slaStripeFake {
	t.Helper()
	f := &slaStripeFake{invoiceOpenID: invoiceOpenID}
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

func (f *slaStripeFake) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.requests = append(f.requests, slaRecordedRequest{
		Method:         r.Method,
		Path:           r.URL.Path,
		Body:           string(body) + "?" + r.URL.RawQuery,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/invoices":
		if r.URL.Query().Get("status") == "open" && f.invoiceOpenID != "" {
			_, _ = fmt.Fprintf(w, `{"object":"list","data":[{"id":%q,"object":"invoice","status":"open","amount_remaining":50000}],"has_more":false,"url":"/v1/invoices"}`, f.invoiceOpenID)
			return
		}
		_, _ = fmt.Fprintf(w, `{"object":"list","data":[],"has_more":false,"url":"/v1/invoices"}`)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/credit_notes":
		_, _ = fmt.Fprintf(w, `{"id":"cn_pipeline_001","object":"credit_note"}`)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/customers/") && strings.HasSuffix(r.URL.Path, "/balance_transactions"):
		_, _ = fmt.Fprintf(w, `{"id":"cbtxn_pipeline_001","object":"customer_balance_transaction"}`)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, `{"error":{"message":"unhandled path %s %s"}}`, r.Method, r.URL.Path)
	}
}

func (f *slaStripeFake) snapshot() []slaRecordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]slaRecordedRequest, len(f.requests))
	copy(out, f.requests)
	return out
}

// pipelineStore composes PgStore (enterprise contracts + customer lookup)
// and PgSLACreditStore (credit insert/get) into the surface the calculator
// needs. The real binary does the same composition at wiring time.
type pipelineStore struct {
	*billing.PgStore
	*billing.PgSLACreditStore
}

// staticUptime is a local UptimeSource that returns a fixed reading. We can't
// reuse billing.NewStaticUptimeSource because its constructor pegs us to a
// fixed value at construction time, which is fine — that's the production
// fallback. Defining our own here keeps the test honest about expressing
// the breach percentage inline.
type staticUptime struct {
	pct float64
}

func (s staticUptime) MonthlyUptimePct(_ context.Context, _ string, _, _ time.Time) (float64, error) {
	return s.pct, nil
}

// TestSLAPipeline_EndToEnd_IssuesCreditAndPersists wires the production
// pieces end-to-end against a fake Stripe and a real Postgres, then verifies
// that one breached month produces:
//   - exactly one Stripe credit_note POST with the canonical idempotency key,
//   - exactly one sla_credits row with the credit note ID stored, and
//   - a second Tick is a clean no-op (no second Stripe call, no second row).
//
// Catches drift between the in-memory unit tests and the live composition of
// PgStore + PgSLACreditStore + StripeSLAIssuer + SLACalculator that ships.
func TestSLAPipeline_EndToEnd_IssuesCreditAndPersists(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	const (
		orgID         = "org-sla-e2e"
		customerID    = "cus_sla_e2e"
		subID         = "sub_sla_e2e"
		invoiceOpenID = "in_sla_e2e_open"
	)

	fake := newSLAStripeFake(t, invoiceOpenID)

	pgStore := billing.NewPgStore(testDB.Pool)
	credStore := billing.NewPgSLACreditStore(testDB.Pool)
	store := pipelineStore{PgStore: pgStore, PgSLACreditStore: credStore}

	if err := pgStore.UpsertOrgSubscription(ctx, &billing.OrgSubscription{
		ID:                   "sub-row-1",
		OrgID:                orgID,
		PlanTier:             "enterprise",
		Status:               "active",
		StripeSubscriptionID: stripe.String(subID),
		StripeCustomerID:     stripe.String(customerID),
	}); err != nil {
		t.Fatalf("UpsertOrgSubscription: %v", err)
	}

	contractStart := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	contractEnd := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO enterprise_contracts (
			id, org_id, enterprise_tier, annual_commitment_cents,
			overage_discount_pct,
			contract_start_date, contract_end_date,
			auto_renew, billing_cadence, stripe_subscription_id
		) VALUES (
			$1, $2, 'enterprise_starter', $3,
			0,
			$4, $5,
			true, 'annual', $6
		)
	`, newID(), orgID, int64(1_200_000), contractStart, contractEnd, subID); err != nil {
		t.Fatalf("insert enterprise_contracts: %v", err)
	}

	issuer := billing.NewStripeSLAIssuer("sk_test_fake", pgStore, slog.New(slog.DiscardHandler))

	// Reference instant inside May 2026 → previous calendar month is April 2026.
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	calc := billing.NewSLACalculator(store, staticUptime{pct: 93.0}, time.Hour, slog.New(slog.DiscardHandler)).
		WithIssuer(issuer).
		WithClock(func() time.Time { return now })

	if err := calc.Tick(ctx); err != nil {
		t.Fatalf("first Tick: %v", err)
	}

	row, err := credStore.GetSLACredit(ctx, orgID,
		time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("GetSLACredit: %v", err)
	}
	if row == nil {
		t.Fatal("expected an sla_credits row after first Tick")
	}
	if row.StripeCreditNoteID != "cn_pipeline_001" {
		t.Errorf("stripe_credit_note_id = %q, want cn_pipeline_001", row.StripeCreditNoteID)
	}

	creditNotePOSTs := 0
	for _, r := range fake.snapshot() {
		if r.Method == http.MethodPost && r.Path == "/v1/credit_notes" {
			creditNotePOSTs++
			if r.IdempotencyKey != "sla-credit-"+orgID+"-2026-05" {
				t.Errorf("idempotency key = %q, want sla-credit-%s-2026-05", r.IdempotencyKey, orgID)
			}
		}
	}
	if creditNotePOSTs != 1 {
		t.Errorf("credit-note POSTs = %d after first Tick, want 1", creditNotePOSTs)
	}

	// Second tick must be a clean no-op.
	before := len(fake.snapshot())
	if err := calc.Tick(ctx); err != nil {
		t.Fatalf("second Tick: %v", err)
	}
	if got := len(fake.snapshot()) - before; got != 0 {
		t.Errorf("second Tick made %d Stripe calls, want 0", got)
	}
}

// TestSLAPipeline_StripeFailure_DoesNotPersistRow asserts the atomicity
// contract: when Stripe returns 500 mid-tick, no sla_credits row lands —
// the calculator will retry on the next tick instead of leaving an
// orphan row with an empty credit-note ID.
func TestSLAPipeline_StripeFailure_DoesNotPersistRow(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	const (
		orgID      = "org-sla-fail"
		customerID = "cus_sla_fail"
		subID      = "sub_sla_fail"
	)

	// Fake that always 500s on POST /v1/credit_notes — no balance fallback
	// because we provide an open invoice up front.
	failingFake := newSLAStripeFakeWithFailure(t, "in_sla_fail_open")

	pgStore := billing.NewPgStore(testDB.Pool)
	credStore := billing.NewPgSLACreditStore(testDB.Pool)
	store := pipelineStore{PgStore: pgStore, PgSLACreditStore: credStore}

	if err := pgStore.UpsertOrgSubscription(ctx, &billing.OrgSubscription{
		ID:                   "sub-row-2",
		OrgID:                orgID,
		PlanTier:             "enterprise",
		Status:               "active",
		StripeSubscriptionID: stripe.String(subID),
		StripeCustomerID:     stripe.String(customerID),
	}); err != nil {
		t.Fatalf("UpsertOrgSubscription: %v", err)
	}

	contractStart := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	contractEnd := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := testDB.Pool.Exec(ctx, `
		INSERT INTO enterprise_contracts (
			id, org_id, enterprise_tier, annual_commitment_cents,
			overage_discount_pct,
			contract_start_date, contract_end_date,
			auto_renew, billing_cadence, stripe_subscription_id
		) VALUES (
			$1, $2, 'enterprise_starter', $3,
			0,
			$4, $5,
			true, 'annual', $6
		)
	`, newID(), orgID, int64(1_200_000), contractStart, contractEnd, subID); err != nil {
		t.Fatalf("insert enterprise_contracts: %v", err)
	}

	issuer := billing.NewStripeSLAIssuer("sk_test_fake", pgStore, slog.New(slog.DiscardHandler))
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	calc := billing.NewSLACalculator(store, staticUptime{pct: 93.0}, time.Hour, slog.New(slog.DiscardHandler)).
		WithIssuer(issuer).
		WithClock(func() time.Time { return now })

	// processContract swallows per-org errors so Tick itself returns nil.
	if err := calc.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	row, err := credStore.GetSLACredit(ctx, orgID,
		time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("GetSLACredit: %v", err)
	}
	if row != nil {
		t.Errorf("expected NO sla_credits row when Stripe failed; got %+v", row)
	}

	if c := failingFake.creditNotePOSTs(); c == 0 {
		t.Error("expected at least one credit-note POST attempt before the failure")
	}
}

// newSLAStripeFakeWithFailure is the failing-Stripe variant used by the
// atomicity test. Keeps the happy-path fake above unconditional.
func newSLAStripeFakeWithFailure(t *testing.T, invoiceOpenID string) *slaStripeFake {
	t.Helper()
	f := &slaStripeFake{invoiceOpenID: invoiceOpenID}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.requests = append(f.requests, slaRecordedRequest{
			Method:         r.Method,
			Path:           r.URL.Path,
			Body:           string(body) + "?" + r.URL.RawQuery,
			IdempotencyKey: r.Header.Get("Idempotency-Key"),
		})
		f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/invoices":
			_, _ = fmt.Fprintf(w, `{"object":"list","data":[{"id":%q,"object":"invoice","status":"open","amount_remaining":50000}],"has_more":false,"url":"/v1/invoices"}`, invoiceOpenID)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/credit_notes":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"forced failure","type":"api_error"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprintf(w, `{"error":{"message":"unhandled %s %s"}}`, r.Method, r.URL.Path)
		}
	}))
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

func (f *slaStripeFake) creditNotePOSTs() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if r.Method == http.MethodPost && r.Path == "/v1/credit_notes" {
			n++
		}
	}
	return n
}
