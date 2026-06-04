package billing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeUptimeSource returns a fixed uptime, optionally with an error.
type fakeUptimeSource struct {
	pct float64
	err error
}

func (f fakeUptimeSource) MonthlyUptimePct(_ context.Context, _ string, _, _ time.Time) (float64, error) {
	return f.pct, f.err
}

// fakeIssuer captures issuance attempts and can be forced to fail.
type fakeIssuer struct {
	mu     sync.Mutex
	calls  []issuerCall
	noteID string
	err    error
}

type issuerCall struct {
	orgID          string
	creditMicrousd int64
	periodEnd      time.Time
}

func (f *fakeIssuer) IssueCredit(_ context.Context, orgID string, creditMicrousd int64, periodEnd time.Time) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, issuerCall{orgID: orgID, creditMicrousd: creditMicrousd, periodEnd: periodEnd})
	if f.err != nil {
		return "", f.err
	}
	return f.noteID, nil
}

// fakeSLAStore is an in-memory SLACalculatorStore for tests.
type fakeSLAStore struct {
	mu        sync.Mutex
	contracts []EnterpriseContract
	credits   map[string]SLACreditRow // key = orgID|start|end
}

func newFakeSLAStore(contracts ...EnterpriseContract) *fakeSLAStore {
	return &fakeSLAStore{contracts: contracts, credits: map[string]SLACreditRow{}}
}

func (f *fakeSLAStore) ListEnterpriseContractsActiveAt(_ context.Context, at time.Time) ([]EnterpriseContract, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]EnterpriseContract, 0, len(f.contracts))
	for _, contract := range f.contracts {
		if !contract.ContractStartDate.After(at) && contract.ContractEndDate.After(at) {
			out = append(out, contract)
		}
	}
	return out, nil
}

func (f *fakeSLAStore) ListEnterpriseContractsOverlappingPeriod(_ context.Context, periodStart, periodEnd time.Time) ([]EnterpriseContract, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]EnterpriseContract, 0, len(f.contracts))
	for _, contract := range f.contracts {
		if contract.ContractStartDate.Before(periodEnd) && contract.ContractEndDate.After(periodStart) {
			out = append(out, contract)
		}
	}
	return out, nil
}

func (f *fakeSLAStore) key(orgID string, start, end time.Time) string {
	return orgID + "|" + start.UTC().Format(time.RFC3339) + "|" + end.UTC().Format(time.RFC3339)
}

func (f *fakeSLAStore) InsertSLACredit(_ context.Context, row SLACreditRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := f.key(row.OrgID, row.PeriodStart, row.PeriodEnd)
	if _, ok := f.credits[k]; ok {
		return ErrSLACreditAlreadyIssued
	}
	f.credits[k] = row
	return nil
}

func (f *fakeSLAStore) GetSLACredit(_ context.Context, orgID string, start, end time.Time) (*SLACreditRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if row, ok := f.credits[f.key(orgID, start, end)]; ok {
		c := row
		return &c, nil
	}
	return nil, nil
}

func (f *fakeSLAStore) MarkSLACreditWebhookDispatched(_ context.Context, orgID string, start, end, dispatchedAt time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := f.key(orgID, start, end)
	row, ok := f.credits[k]
	if !ok || row.WebhookDispatchedAt != nil {
		return false, nil
	}
	row.WebhookDispatchedAt = &dispatchedAt
	f.credits[k] = row
	return true, nil
}

func (f *fakeSLAStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.credits)
}

func (f *fakeSLAStore) creditFor(orgID string, start, end time.Time) *SLACreditRow {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.credits[f.key(orgID, start, end)]
	if !ok {
		return nil
	}
	return &row
}

func newTestContract(orgID string, tier EnterpriseTier) EnterpriseContract {
	return EnterpriseContract{
		ID:                    "contract-" + orgID,
		OrgID:                 orgID,
		EnterpriseTier:        tier,
		AnnualCommitmentCents: 1_800_000, // $18,000/yr → $1,500/mo
		ContractStartDate:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ContractEndDate:       time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// 100% uptime → no credit issued, no row inserted.
func TestSLACalculator_HealthyUptime_NoCredit(t *testing.T) {
	t.Parallel()

	store := newFakeSLAStore(newTestContract("org-healthy", EnterpriseTierStarter))
	issuer := &fakeIssuer{noteID: "cn_1"}
	calc := NewSLACalculator(store, fakeUptimeSource{pct: 100.0}, time.Hour, nil).
		WithIssuer(issuer).
		WithClock(fixedClock(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)))

	if err := calc.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if store.count() != 0 {
		t.Errorf("expected 0 credit rows, got %d", store.count())
	}
	if len(issuer.calls) != 0 {
		t.Errorf("expected 0 issuance calls, got %d", len(issuer.calls))
	}
}

// 99.5% uptime against Starter's 99.9% target → 10% credit band.
func TestSLACalculator_99_5_Pct_IssuesTenPercent(t *testing.T) {
	t.Parallel()

	store := newFakeSLAStore(newTestContract("org-band-10", EnterpriseTierStarter))
	issuer := &fakeIssuer{noteID: "cn_band10"}
	calc := NewSLACalculator(store, fakeUptimeSource{pct: 99.5}, time.Hour, nil).
		WithIssuer(issuer).
		WithClock(fixedClock(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)))

	if err := calc.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if store.count() != 1 {
		t.Fatalf("expected 1 credit row, got %d", store.count())
	}
	if len(issuer.calls) != 1 {
		t.Fatalf("expected 1 issuance call, got %d", len(issuer.calls))
	}
	got := issuer.calls[0]
	// $1,500/mo monthly base × 10% = $150 = 150_000_000 micro-USD.
	if got.creditMicrousd != 150_000_000 {
		t.Errorf("credit_microusd = %d, want 150_000_000", got.creditMicrousd)
	}
}

func TestSLACalculator_IncludesContractThatLapsedDuringCreditedMonth(t *testing.T) {
	t.Parallel()

	contract := newTestContract("org-lapsed-mid-period", EnterpriseTierStarter)
	contract.ContractStartDate = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	contract.ContractEndDate = time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	store := newFakeSLAStore(contract)
	issuer := &fakeIssuer{noteID: "cn_lapsed"}
	calc := NewSLACalculator(store, fakeUptimeSource{pct: 95.0}, time.Hour, nil).
		WithIssuer(issuer).
		WithClock(fixedClock(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)))

	if err := calc.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if store.count() != 1 {
		t.Fatalf("expected 1 credit row for contract overlapping credited month, got %d", store.count())
	}
	if len(issuer.calls) != 1 {
		t.Fatalf("expected 1 issuance call, got %d", len(issuer.calls))
	}
}

// 98% uptime → 25% credit band (band is 95.0 <= u < 99.0).
func TestSLACalculator_98_Pct_IssuesTwentyFivePercent(t *testing.T) {
	t.Parallel()

	store := newFakeSLAStore(newTestContract("org-band-25", EnterpriseTierStarter))
	issuer := &fakeIssuer{noteID: "cn_band25"}
	calc := NewSLACalculator(store, fakeUptimeSource{pct: 98.0}, time.Hour, nil).
		WithIssuer(issuer).
		WithClock(fixedClock(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)))

	if err := calc.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if store.count() != 1 {
		t.Fatalf("expected 1 credit row, got %d", store.count())
	}
	got := issuer.calls[0]
	// $1,500 × 25% = $375 = 375_000_000 micro-USD.
	if got.creditMicrousd != 375_000_000 {
		t.Errorf("credit_microusd = %d, want 375_000_000", got.creditMicrousd)
	}
}

// 90% uptime → 50% credit band (capped).
func TestSLACalculator_DeepOutage_IssuesFiftyPercent(t *testing.T) {
	t.Parallel()

	store := newFakeSLAStore(newTestContract("org-band-50", EnterpriseTierStarter))
	issuer := &fakeIssuer{noteID: "cn_band50"}
	calc := NewSLACalculator(store, fakeUptimeSource{pct: 90.0}, time.Hour, nil).
		WithIssuer(issuer).
		WithClock(fixedClock(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)))

	if err := calc.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if store.count() != 1 {
		t.Fatalf("expected 1 credit row, got %d", store.count())
	}
	got := issuer.calls[0]
	// $1,500 × 50% = $750 = 750_000_000 micro-USD.
	if got.creditMicrousd != 750_000_000 {
		t.Errorf("credit_microusd = %d, want 750_000_000", got.creditMicrousd)
	}
}

// A second tick within the same billing period must not issue a duplicate credit.
func TestSLACalculator_Idempotent_AlreadyIssued(t *testing.T) {
	t.Parallel()

	store := newFakeSLAStore(newTestContract("org-idem", EnterpriseTierStarter))
	issuer := &fakeIssuer{noteID: "cn_idem"}
	calc := NewSLACalculator(store, fakeUptimeSource{pct: 95.0}, time.Hour, nil).
		WithIssuer(issuer).
		WithClock(fixedClock(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)))

	if err := calc.Tick(context.Background()); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	if err := calc.Tick(context.Background()); err != nil {
		t.Fatalf("tick 2: %v", err)
	}

	if store.count() != 1 {
		t.Errorf("expected 1 credit row after two ticks, got %d", store.count())
	}
	if len(issuer.calls) != 1 {
		t.Errorf("expected exactly 1 issuance call after two ticks, got %d", len(issuer.calls))
	}
}

// If the Stripe-side issuance fails, no credit row is persisted (atomic).
func TestSLACalculator_IssuerFailure_DoesNotPersist(t *testing.T) {
	t.Parallel()

	store := newFakeSLAStore(newTestContract("org-fail", EnterpriseTierStarter))
	issuer := &fakeIssuer{err: errors.New("stripe down")}
	calc := NewSLACalculator(store, fakeUptimeSource{pct: 95.0}, time.Hour, nil).
		WithIssuer(issuer).
		WithClock(fixedClock(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)))

	if err := calc.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if store.count() != 0 {
		t.Errorf("expected 0 credit rows after issuer failure, got %d", store.count())
	}
	if len(issuer.calls) != 1 {
		t.Errorf("expected 1 issuance attempt, got %d", len(issuer.calls))
	}
}

func TestSLACalculator_DispatchFailurePersistsAndRetriesUndispatchedCredit(t *testing.T) {
	t.Parallel()

	orgID := "org-dispatch-fail"
	store := newFakeSLAStore(newTestContract(orgID, EnterpriseTierStarter))
	dispatcher := &fakeDispatcher{err: errors.New("webhook outbox down")}
	calc := NewSLACalculator(store, fakeUptimeSource{pct: 95.0}, time.Hour, nil).
		WithDispatcher(dispatcher).
		WithClock(fixedClock(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)))

	ctx := context.Background()
	if err := calc.Tick(ctx); err != nil {
		t.Fatalf("tick 1: %v", err)
	}

	if store.count() != 1 {
		t.Fatalf("expected 1 durable credit row after dispatch failure, got %d", store.count())
	}
	if len(dispatcher.calls) != 1 {
		t.Errorf("expected 1 dispatch attempt, got %d", len(dispatcher.calls))
	}
	periodStart, periodEnd := previousCalendarMonth(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC))
	row := store.creditFor(orgID, periodStart, periodEnd)
	if row == nil {
		t.Fatal("expected persisted credit row")
		return
	}
	if row.WebhookDispatchedAt != nil {
		t.Fatal("failed dispatch must not mark webhook_dispatched_at")
	}

	dispatcher.err = nil
	if err := calc.Tick(ctx); err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	if len(dispatcher.calls) != 2 {
		t.Fatalf("expected retry dispatch attempt, got %d", len(dispatcher.calls))
	}
	row = store.creditFor(orgID, periodStart, periodEnd)
	if row == nil || row.WebhookDispatchedAt == nil {
		t.Fatal("successful retry should mark webhook_dispatched_at")
	}
}

func TestSLACalculator_DispatchesOnlyAfterCreditIsPersisted(t *testing.T) {
	t.Parallel()

	orgID := "org-dispatch-after-persist"
	store := newFakeSLAStore(newTestContract(orgID, EnterpriseTierStarter))
	periodStart, periodEnd := previousCalendarMonth(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC))
	dispatcher := &fakeDispatcher{
		onDispatch: func() {
			if row := store.creditFor(orgID, periodStart, periodEnd); row == nil {
				t.Fatal("sla.credit_issued dispatched before credit row was persisted")
			}
		},
	}
	calc := NewSLACalculator(store, fakeUptimeSource{pct: 95.0}, time.Hour, nil).
		WithDispatcher(dispatcher).
		WithClock(fixedClock(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)))

	if err := calc.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(dispatcher.calls) != 1 {
		t.Fatalf("expected one dispatch, got %d", len(dispatcher.calls))
	}
}

func TestSLACalculator_ConcurrentTicksDispatchOnceAfterInsertWins(t *testing.T) {
	t.Parallel()

	store := newFakeSLAStore(newTestContract("org-concurrent-dispatch", EnterpriseTierStarter))
	dispatcher := &fakeDispatcher{}
	calc := NewSLACalculator(store, fakeUptimeSource{pct: 95.0}, time.Hour, nil).
		WithDispatcher(dispatcher).
		WithClock(fixedClock(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)))

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_ = calc.Tick(context.Background())
		})
	}
	wg.Wait()

	if store.count() != 1 {
		t.Fatalf("expected one credit row, got %d", store.count())
	}
	if len(dispatcher.calls) != 1 {
		t.Fatalf("expected exactly one webhook dispatch, got %d", len(dispatcher.calls))
	}
}

// Without an issuer wired, the calculator still records the credit row and
// dispatches the event (the operator-escape-hatch / community-build path).
func TestSLACalculator_NoIssuer_PersistsRow(t *testing.T) {
	t.Parallel()

	store := newFakeSLAStore(newTestContract("org-no-issuer", EnterpriseTierStarter))
	calc := NewSLACalculator(store, fakeUptimeSource{pct: 95.0}, time.Hour, nil).
		WithClock(fixedClock(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)))

	if err := calc.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if store.count() != 1 {
		t.Errorf("expected 1 credit row without issuer, got %d", store.count())
	}
}

// Out-of-range uptime readings (negative, > 100) get clamped before band lookup.
func TestSLACalculator_ClampsOutOfRangeUptime(t *testing.T) {
	t.Parallel()

	// Negative uptime would otherwise slide into the 50% band silently.
	storeA := newFakeSLAStore(newTestContract("org-neg", EnterpriseTierStarter))
	calcA := NewSLACalculator(storeA, fakeUptimeSource{pct: -10.0}, time.Hour, nil).
		WithClock(fixedClock(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)))
	if err := calcA.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if storeA.count() != 1 {
		t.Errorf("expected 1 credit row for clamped-to-0 uptime, got %d", storeA.count())
	}

	// >100 uptime clamps to 100 → above target → no credit.
	storeB := newFakeSLAStore(newTestContract("org-over", EnterpriseTierStarter))
	calcB := NewSLACalculator(storeB, fakeUptimeSource{pct: 150.0}, time.Hour, nil).
		WithClock(fixedClock(time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)))
	if err := calcB.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if storeB.count() != 0 {
		t.Errorf("expected 0 credit rows for clamped-to-100 uptime, got %d", storeB.count())
	}
}

// previousCalendarMonth returns the prior month's [start, end) window
// independent of where in the current month the tick fires.
func TestPreviousCalendarMonth(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		ref       time.Time
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			name:      "mid_month",
			ref:       time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "month_first_instant",
			ref:       time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "year_rollover",
			ref:       time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
			wantStart: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			start, end := previousCalendarMonth(tc.ref)
			if !start.Equal(tc.wantStart) {
				t.Errorf("start = %s, want %s", start, tc.wantStart)
			}
			if !end.Equal(tc.wantEnd) {
				t.Errorf("end = %s, want %s", end, tc.wantEnd)
			}
		})
	}
}

func TestStaticUptimeSource(t *testing.T) {
	t.Parallel()

	src := NewStaticUptimeSource(99.95)
	got, err := src.MonthlyUptimePct(context.Background(), "any-org", time.Now(), time.Now())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != 99.95 {
		t.Errorf("uptime = %v, want 99.95", got)
	}
}
