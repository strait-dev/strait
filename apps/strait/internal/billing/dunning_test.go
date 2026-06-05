package billing

import (
	"context"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDunningStore is an in-memory DunningStore that mirrors PgStore's
// semantics for the Dunner: StartDunning is idempotent on active cycles,
// ResolveDunning is idempotent on resolved cycles, and ProcessDueDunningRows
// hands each due row to fn under a mutex so a parallel Tick cannot
// double-process a row (mirroring FOR UPDATE SKIP LOCKED).
type fakeDunningStore struct {
	mu      sync.Mutex
	rows    map[string]*fakeDunningRow
	emails  map[string][]string
	claimed map[string]struct{}
}

type fakeDunningRow struct {
	OrgID            string
	PlanTier         string
	PaymentStatus    string
	DunningStep      int
	DunningEnteredAt time.Time
	DunningLastTick  *time.Time
	DunningResolved  *time.Time
}

func newFakeDunningStore() *fakeDunningStore {
	return &fakeDunningStore{
		rows:    map[string]*fakeDunningRow{},
		emails:  map[string][]string{},
		claimed: map[string]struct{}{},
	}
}

func (f *fakeDunningStore) seed(row *fakeDunningRow) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[row.OrgID] = row
}

func (f *fakeDunningStore) get(orgID string) *fakeDunningRow {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r := f.rows[orgID]; r != nil {
		copy := *r
		return &copy
	}
	return nil
}

func (f *fakeDunningStore) StartDunning(_ context.Context, orgID string, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[orgID]
	if !ok {
		return ErrSubscriptionNotFound
	}
	// Idempotent: active cycles (entered_at set, resolved_at nil) are no-ops.
	if !r.DunningEnteredAt.IsZero() && r.DunningResolved == nil {
		return nil
	}
	r.DunningStep = 1
	r.DunningEnteredAt = now
	r.DunningLastTick = nil
	r.DunningResolved = nil
	return nil
}

func (f *fakeDunningStore) ResolveDunning(_ context.Context, orgID string, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[orgID]
	if !ok {
		return nil
	}
	if r.DunningEnteredAt.IsZero() || r.DunningResolved != nil {
		return nil
	}
	resolved := now
	r.DunningStep = 0
	r.DunningEnteredAt = time.Time{}
	r.DunningLastTick = nil
	r.DunningResolved = &resolved
	return nil
}

func (f *fakeDunningStore) ProcessDueDunningRows(
	ctx context.Context,
	now time.Time,
	cooldown time.Duration,
	limit int,
	fn func(ctx context.Context, row DunningRow) (DunningTransition, error),
) (int, error) {
	candidates := f.snapshotDue(now, cooldown, limit)
	processed := 0
	for _, c := range candidates {
		fresh := f.claimWithCooldown(c.OrgID, now, cooldown)
		if fresh == nil {
			continue
		}
		row := DunningRow{
			OrgID:            fresh.OrgID,
			PlanTier:         fresh.PlanTier,
			PaymentStatus:    fresh.PaymentStatus,
			DunningStep:      fresh.DunningStep,
			DunningEnteredAt: fresh.DunningEnteredAt,
		}
		transition, err := fn(ctx, row)
		if err != nil {
			f.release(fresh.OrgID)
			return processed, err
		}
		f.apply(transition)
		f.release(fresh.OrgID)
		processed++
	}
	return processed, nil
}

func (f *fakeDunningStore) snapshotDue(now time.Time, cooldown time.Duration, limit int) []fakeDunningRow {
	f.mu.Lock()
	defer f.mu.Unlock()
	cutoff := now.Add(-cooldown)
	out := make([]fakeDunningRow, 0)
	for _, r := range f.rows {
		if r.DunningEnteredAt.IsZero() || r.DunningResolved != nil {
			continue
		}
		if r.DunningLastTick != nil && r.DunningLastTick.After(cutoff) {
			continue
		}
		out = append(out, *r)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func (f *fakeDunningStore) claimWithCooldown(orgID string, now time.Time, cooldown time.Duration) *fakeDunningRow {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, locked := f.claimed[orgID]; locked {
		return nil
	}
	r, ok := f.rows[orgID]
	if !ok || r.DunningEnteredAt.IsZero() || r.DunningResolved != nil {
		return nil
	}
	if r.DunningLastTick != nil && r.DunningLastTick.After(now.Add(-cooldown)) {
		return nil
	}
	f.claimed[orgID] = struct{}{}
	cp := *r
	return &cp
}

func (f *fakeDunningStore) release(orgID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.claimed, orgID)
}

func (f *fakeDunningStore) apply(t DunningTransition) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[t.OrgID]
	if !ok {
		return
	}
	r.DunningStep = t.NewStep
	tickAt := t.TickAt
	r.DunningLastTick = &tickAt
	if t.PaymentStatus != "" {
		r.PaymentStatus = t.PaymentStatus
	}
}

type fakeDunningEmailSender struct {
	mu    sync.Mutex
	calls []dunningEmailCall
}

type dunningEmailCall struct {
	to       []string
	planName string
	step     int
}

func (f *fakeDunningEmailSender) SendDunningStep(_ context.Context, to []string, planName string, step int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]string, len(to))
	copy(cp, to)
	f.calls = append(f.calls, dunningEmailCall{to: cp, planName: planName, step: step})
}

type fakeDunningAdminLookup struct{ emails []string }

func (f fakeDunningAdminLookup) ListOrgAdminEmails(_ context.Context, _ string) ([]string, error) {
	return f.emails, nil
}

func newSeededDunner(t *testing.T, now func() time.Time) (*Dunner, *fakeDunningStore, *fakeDispatcher, *fakeDunningEmailSender) {
	t.Helper()
	store := newFakeDunningStore()
	store.seed(&fakeDunningRow{
		OrgID:            "org_dun_1",
		PlanTier:         string(domain.PlanPro),
		PaymentStatus:    "grace",
		DunningStep:      1,
		DunningEnteredAt: now().Add(-1 * time.Hour),
	})
	disp := &fakeDispatcher{}
	emails := &fakeDunningEmailSender{}
	d := NewDunner(store,
		WithDunnerDispatcher(disp),
		WithDunnerEmails(emails),
		WithDunnerAdminLookup(fakeDunningAdminLookup{emails: []string{"admin@example.com"}}),
		WithDunnerClock(now),
		WithDunnerCooldown(1*time.Second),
	)
	return d, store, disp, emails
}

func TestDunner_AdvancesStepAtDayBoundary(t *testing.T) {
	t.Parallel()
	entered := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	clock := entered.Add(3*24*time.Hour + 1*time.Minute) // past day 3
	d, store, disp, emails := newSeededDunner(t, func() time.Time { return clock })
	store.rows["org_dun_1"].DunningEnteredAt = entered
	require.NoError(t,
		d.Tick(context.Background()))

	got := store.get("org_dun_1")
	require.Equal(t,
		DunningStepDay3,
		got.
			DunningStep,
	)
	require.False(t,
		len(emails.
			calls) !=
			1 || emails.
			calls[0].step != DunningStepDay3,
	)
	require.Equal(t, 1, countEvent(dispatchedEventTypes(disp), domain.WebhookEventBillingDelinquent))
}

func TestDunner_SameDayTickIsCooldownNoOp(t *testing.T) {
	t.Parallel()
	entered := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	clock := entered.Add(3*24*time.Hour + 1*time.Minute)
	d, store, disp, emails := newSeededDunner(t, func() time.Time { return clock })
	store.rows["org_dun_1"].DunningEnteredAt = entered
	// Use the production 24h cooldown so a second tick in the same day is suppressed.
	d.cooldown = 24 * time.Hour
	require.NoError(t,
		d.Tick(context.Background()))
	require.NoError(t,
		d.Tick(context.Background()))
	assert.Len(t, emails.
		calls,
		1)
	assert.Equal(t, 1,
		countEvent(dispatchedEventTypes(disp), domain.WebhookEventBillingDelinquent))
}

func TestDunner_LateTickJumpsMultipleSteps(t *testing.T) {
	t.Parallel()
	entered := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	clock := entered.Add(15 * 24 * time.Hour) // past day 14
	d, store, disp, emails := newSeededDunner(t, func() time.Time { return clock })
	store.rows["org_dun_1"].DunningEnteredAt = entered
	require.NoError(t,
		d.Tick(context.Background()))

	got := store.get("org_dun_1")
	assert.Equal(t, DunningStepDay14,

		got.
			DunningStep,
	)
	assert.Equal(t, "restricted",

		got.PaymentStatus,
	)
	assert.False(t, len(emails.
		calls) !=
		1 || emails.
		calls[0].step != DunningStepDay14,
	)
	assert.Equal(t, 1,
		countEvent(dispatchedEventTypes(disp), domain.WebhookEventBillingDelinquent))
}

func TestDunner_Step6SuspendsAndDispatchesSuspended(t *testing.T) {
	t.Parallel()
	entered := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	clock := entered.Add(75 * 24 * time.Hour)
	d, store, disp, _ := newSeededDunner(t, func() time.Time { return clock })
	store.rows["org_dun_1"].DunningEnteredAt = entered
	require.NoError(t,
		d.Tick(context.Background()))

	got := store.get("org_dun_1")
	assert.Equal(t, DunningStepDay74,

		got.
			DunningStep,
	)
	assert.Equal(t, "suspended",

		got.PaymentStatus,
	)

	events := dispatchedEventTypes(disp)
	assert.Equal(t, 1,
		countEvent(events,
			domain.
				WebhookEventBillingDelinquent,
		))
	assert.Equal(t, 1,
		countEvent(events,
			domain.
				WebhookEventBillingSuspended,
		),
	)
}

func TestDunner_ResolutionClearsAndAllowsReentry(t *testing.T) {
	t.Parallel()
	entered := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	clock := entered.Add(2 * time.Hour)
	store := newFakeDunningStore()
	store.seed(&fakeDunningRow{
		OrgID:            "org_resolve",
		PlanTier:         string(domain.PlanPro),
		PaymentStatus:    "grace",
		DunningStep:      1,
		DunningEnteredAt: entered,
	})
	require.NoError(t,
		store.
			ResolveDunning(context.
				Background(), "org_resolve",

				clock))

	row := store.get("org_resolve")
	require.False(t,
		!row.DunningEnteredAt.
			IsZero() || row.DunningStep != 0 ||

			row.DunningResolved == nil)
	require.NoError(t,
		store.
			StartDunning(context.
				Background(), "org_resolve",

				clock.Add(1*time.Hour)))

	row = store.get("org_resolve")
	require.False(t,
		row.DunningStep !=
			1 || row.
			DunningResolved != nil)
}

func TestDunner_StartDunningIsIdempotentForActiveCycle(t *testing.T) {
	t.Parallel()
	entered := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	store := newFakeDunningStore()
	store.seed(&fakeDunningRow{
		OrgID:            "org_replay",
		PlanTier:         string(domain.PlanPro),
		DunningStep:      1,
		DunningEnteredAt: entered,
	})
	require.NoError(t,
		store.
			StartDunning(context.
				Background(), "org_replay",

				entered.Add(1*time.Hour)))

	row := store.get("org_replay")
	assert.True(t, row.
		DunningEnteredAt.
		Equal(entered))
}
