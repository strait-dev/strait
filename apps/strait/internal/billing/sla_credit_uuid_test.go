package billing

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// The store layer convention is UUID v7 for primary keys used in
// time-ordered queries. SLACreditRow.ID was originally v4; this test
// locks the alignment so a future change cannot regress without an
// explicit signal.
func TestSLACredit_IDUsesUUIDV7(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	periodStart, periodEnd := previousCalendarMonth(now)

	store := newFakeSLAStore(newTestContract("org-uuid-test", EnterpriseTierStarter))
	calc := NewSLACalculator(store, fakeUptimeSource{pct: 93.0}, time.Hour, nil).
		WithClock(fixedClock(now))

	if err := calc.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	row, err := store.GetSLACredit(context.Background(), "org-uuid-test", periodStart, periodEnd)
	if err != nil {
		t.Fatalf("GetSLACredit: %v", err)
	}
	if row == nil {
		t.Fatal("expected an SLA credit row to be inserted")
		return
	}

	parsed, err := uuid.Parse(row.ID)
	if err != nil {
		t.Fatalf("ID %q is not a valid UUID: %v", row.ID, err)
	}
	if v := parsed.Version(); v != 7 {
		t.Errorf("ID version = %d, want 7 (store-layer convention); ID = %q", v, row.ID)
	}
}
