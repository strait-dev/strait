package billing

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t,
		calc.Tick(context.
			Background()))

	row, err := store.GetSLACredit(context.Background(), "org-uuid-test", periodStart, periodEnd)
	require.NoError(t,
		err)
	require.NotNil(t,
		row)

	parsed, err := uuid.Parse(row.ID)
	require.NoError(t,
		err)
	assert.EqualValues(t, 7,
		parsed.Version())

}
