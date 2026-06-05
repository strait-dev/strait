//go:build integration

package billing_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/billing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPgSLACreditStore_ConcurrentInsert_OneWinner asserts the UNIQUE
// (org_id, period_start, period_end) constraint plus the read-after-insert
// dedupe logic: with N goroutines racing on the same period, exactly one row
// lands, and the losers observe ErrSLACreditAlreadyIssued so the calculator
// can short-circuit cleanly.
func TestPgSLACreditStore_ConcurrentInsert_OneWinner(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	store := billing.NewPgSLACreditStore(testDB.Pool)

	orgID := "org-sla-race"
	periodStart := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)

	const goroutines = 16
	var wg sync.WaitGroup
	var winners atomic.Int32
	var alreadyIssued atomic.Int32
	var otherErr atomic.Int32

	start := make(chan struct{})
	for range goroutines {
		wg.Go(func() {
			<-start

			err := store.InsertSLACredit(ctx, billing.SLACreditRow{
				ID:             uuid.Must(uuid.NewV7()).String(),
				OrgID:          orgID,
				PeriodStart:    periodStart,
				PeriodEnd:      periodEnd,
				UptimePct:      93.0,
				TargetPct:      99.9,
				CreditPct:      25,
				CreditMicrousd: 12_500_000,
				IssuedAt:       now,
			})
			switch {
			case err == nil:
				winners.Add(1)
			case errors.Is(err, billing.ErrSLACreditAlreadyIssued):
				alreadyIssued.Add(1)
			default:
				t.Logf("unexpected insert err: %v", err)
				otherErr.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()
	assert.EqualValues(t, 1, winners.
		Load())
	assert.EqualValues(t, goroutines-
		1, alreadyIssued.
		Load())
	assert.EqualValues(t, 0, otherErr.
		Load())

	row, err := store.GetSLACredit(ctx, orgID, periodStart, periodEnd)
	require.NoError(t, err)
	require.NotNil(t, row)

}

// TestPgSLACreditStore_ConcurrentDifferentPeriods_AllPersist asserts that
// the unique key really is scoped to (org, period_start, period_end). Two
// inserts for the same org but different periods must both land — otherwise
// the calculator could lose months when a backfill catches up.
func TestPgSLACreditStore_ConcurrentDifferentPeriods_AllPersist(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	store := billing.NewPgSLACreditStore(testDB.Pool)
	orgID := "org-sla-multi-period"
	now := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)

	periods := []struct {
		start, end time.Time
	}{
		{time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		{time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		{time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
	}

	var wg sync.WaitGroup
	for _, p := range periods {
		wg.Go(func() {
			err := store.InsertSLACredit(ctx, billing.SLACreditRow{
				ID:             uuid.Must(uuid.NewV7()).String(),
				OrgID:          orgID,
				PeriodStart:    p.start,
				PeriodEnd:      p.end,
				UptimePct:      95.0,
				TargetPct:      99.9,
				CreditPct:      25,
				CreditMicrousd: 10_000_000,
				IssuedAt:       now,
			})
			assert.NoError(t, err)

		})
	}
	wg.Wait()

	for _, p := range periods {
		row, err := store.GetSLACredit(ctx, orgID, p.start, p.end)
		require.NoError(t, err)
		assert.NotNil(t, row)

	}
}

// TestPgSLACreditStore_ReInsertSameID_Idempotent verifies the "loser
// observable" branch: a second insert with the same (org, period) but a
// different ID surfaces ErrSLACreditAlreadyIssued, and a re-issue with the
// exact same ID (the calculator retrying after a crash with cached state)
// is a clean no-op.
func TestPgSLACreditStore_ReInsertSameID_Idempotent(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	store := billing.NewPgSLACreditStore(testDB.Pool)
	orgID := "org-sla-reissue"
	periodStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	originalID := uuid.Must(uuid.NewV7()).String()
	require.NoError(t, store.
		InsertSLACredit(ctx,
			billing.SLACreditRow{ID: originalID,
				OrgID: orgID, PeriodStart: periodStart, PeriodEnd: periodEnd, UptimePct: 90.0, TargetPct: 99.9, CreditPct: 50, CreditMicrousd: 50_000_000, IssuedAt: now}))
	assert.NoError(t, store.InsertSLACredit(ctx, billing.
		SLACreditRow{ID: originalID,
		OrgID:       orgID,
		PeriodStart: periodStart, PeriodEnd: periodEnd, UptimePct: 90.0,
		TargetPct: 99.9, CreditPct: 50, CreditMicrousd: 50_000_000, IssuedAt: now}))

	if err := store.InsertSLACredit(ctx, billing.SLACreditRow{
		ID:             uuid.Must(uuid.NewV7()).String(),
		OrgID:          orgID,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		UptimePct:      90.0,
		TargetPct:      99.9,
		CreditPct:      50,
		CreditMicrousd: 50_000_000,
		IssuedAt:       now,
	}); !errors.Is(err, billing.ErrSLACreditAlreadyIssued) {
		assert.Failf(t, "test failure",

			"re-insert with new ID = %v, want ErrSLACreditAlreadyIssued", err)
	}
}
