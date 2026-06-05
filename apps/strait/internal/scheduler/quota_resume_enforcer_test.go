package scheduler

import (
	"context"
	"testing"
	"time"

	"strait/internal/billing"

	"github.com/stretchr/testify/require"
)

type mockQuotaResumeStore struct {
	orgIDs        []string
	subs          map[string]*billing.OrgSubscription
	unpauseCalls  int
	boundaries    []time.Time
	resumeResults []int64
}

func (m *mockQuotaResumeStore) ListAllSubscribedOrgIDs(context.Context) ([]string, error) {
	return m.orgIDs, nil
}

func (m *mockQuotaResumeStore) GetOrgSubscription(_ context.Context, orgID string) (*billing.OrgSubscription, error) {
	return m.subs[orgID], nil
}

func (m *mockQuotaResumeStore) UnpauseJobsByPauseReason(context.Context, string, string) (int64, error) {
	m.unpauseCalls++
	return 1, nil
}

func (m *mockQuotaResumeStore) UnpauseJobsByPauseReasonBefore(_ context.Context, _ string, _ string, pausedBefore time.Time) (int64, error) {
	m.unpauseCalls++
	m.boundaries = append(m.boundaries, pausedBefore)
	if len(m.resumeResults) > 0 {
		resumed := m.resumeResults[0]
		m.resumeResults = m.resumeResults[1:]
		return resumed, nil
	}
	return 1, nil
}

func TestQuotaResumeEnforcer_DoesNotRepeatSamePeriod(t *testing.T) {
	t.Parallel()

	periodEnd := time.Now().UTC().Add(-time.Hour)
	store := &mockQuotaResumeStore{
		orgIDs: []string{"org-1"},
		subs: map[string]*billing.OrgSubscription{
			"org-1": {OrgID: "org-1", CurrentPeriodEnd: &periodEnd},
		},
	}
	enforcer := NewQuotaResumeEnforcer(store, nil, time.Minute)
	require.NoError(t,
		enforcer.enforceLocked(context.Background()))
	require.NoError(t,
		enforcer.enforceLocked(context.Background()))
	require.EqualValues(t, 1,
		store.unpauseCalls,
	)

}

func TestQuotaResumeEnforcer_UsesBillingBoundaryForUnpause(t *testing.T) {
	t.Parallel()

	periodEnd := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	store := &mockQuotaResumeStore{
		orgIDs: []string{"org-1"},
		subs: map[string]*billing.OrgSubscription{
			"org-1": {OrgID: "org-1", CurrentPeriodEnd: &periodEnd},
		},
	}
	enforcer := NewQuotaResumeEnforcer(store, nil, time.Minute)
	require.NoError(t,
		enforcer.enforceLocked(context.Background()))
	require.False(t, len(store.boundaries) !=
		1 || !store.boundaries[0].
		Equal(periodEnd))

}

func TestQuotaResumeEnforcer_NewPeriodCanResumeAgain(t *testing.T) {
	t.Parallel()

	periodEnd := time.Now().UTC().Add(-time.Hour)
	store := &mockQuotaResumeStore{
		orgIDs: []string{"org-1"},
		subs: map[string]*billing.OrgSubscription{
			"org-1": {OrgID: "org-1", CurrentPeriodEnd: &periodEnd},
		},
	}
	enforcer := NewQuotaResumeEnforcer(store, nil, time.Minute)
	require.NoError(t,
		enforcer.enforceLocked(context.Background()))

	nextPeriodEnd := time.Now().UTC().Add(-time.Minute)
	store.subs["org-1"].CurrentPeriodEnd = &nextPeriodEnd
	require.NoError(t,
		enforcer.enforceLocked(context.Background()))
	require.EqualValues(t, 2,
		store.unpauseCalls,
	)

}

func TestDeepSecQuotaResumeEnforcer_FreeTierCatchesUpAfterFirstOfMonth(t *testing.T) {
	t.Parallel()

	enforcer := NewQuotaResumeEnforcer(&mockQuotaResumeStore{}, nil, time.Minute)
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)

	periodKey, boundary, ok := enforcer.resumePeriodKey(now, &billing.OrgSubscription{OrgID: "org-free"})
	require.True(t, ok)
	require.Equal(t, "2026-05",
		periodKey,
	)

	wantBoundary := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	require.True(t, boundary.
		Equal(wantBoundary))

}

func TestQuotaResumeEnforcer_DoesNotMarkPeriodResumedWhenNoRowsMatched(t *testing.T) {
	t.Parallel()

	periodEnd := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	store := &mockQuotaResumeStore{
		orgIDs: []string{"org-1"},
		subs: map[string]*billing.OrgSubscription{
			"org-1": {OrgID: "org-1", CurrentPeriodEnd: &periodEnd},
		},
		resumeResults: []int64{0, 1},
	}
	enforcer := NewQuotaResumeEnforcer(store, nil, time.Minute)
	require.NoError(t,
		enforcer.enforceLocked(context.Background()))
	require.NoError(t,
		enforcer.enforceLocked(context.Background()))
	require.EqualValues(t, 2,
		store.unpauseCalls,
	)
	require.False(t, len(store.boundaries) !=
		2 || !store.boundaries[0].
		Equal(periodEnd) || !store.boundaries[1].Equal(periodEnd))

}
