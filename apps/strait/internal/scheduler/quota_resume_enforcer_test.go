package scheduler

import (
	"context"
	"testing"
	"time"

	"strait/internal/billing"
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

	if err := enforcer.enforceLocked(context.Background()); err != nil {
		t.Fatalf("first enforceLocked() error = %v", err)
	}
	if err := enforcer.enforceLocked(context.Background()); err != nil {
		t.Fatalf("second enforceLocked() error = %v", err)
	}
	if store.unpauseCalls != 1 {
		t.Fatalf("unpause calls = %d, want 1", store.unpauseCalls)
	}
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
	if err := enforcer.enforceLocked(context.Background()); err != nil {
		t.Fatalf("enforceLocked() error = %v", err)
	}
	if len(store.boundaries) != 1 || !store.boundaries[0].Equal(periodEnd) {
		t.Fatalf("unpause boundary = %v, want %v", store.boundaries, periodEnd)
	}
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

	if err := enforcer.enforceLocked(context.Background()); err != nil {
		t.Fatalf("first enforceLocked() error = %v", err)
	}
	nextPeriodEnd := time.Now().UTC().Add(-time.Minute)
	store.subs["org-1"].CurrentPeriodEnd = &nextPeriodEnd
	if err := enforcer.enforceLocked(context.Background()); err != nil {
		t.Fatalf("second enforceLocked() error = %v", err)
	}
	if store.unpauseCalls != 2 {
		t.Fatalf("unpause calls = %d, want 2", store.unpauseCalls)
	}
}

func TestDeepSecQuotaResumeEnforcer_FreeTierCatchesUpAfterFirstOfMonth(t *testing.T) {
	t.Parallel()

	enforcer := NewQuotaResumeEnforcer(&mockQuotaResumeStore{}, nil, time.Minute)
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)

	periodKey, boundary, ok := enforcer.resumePeriodKey(now, &billing.OrgSubscription{OrgID: "org-free"})
	if !ok {
		t.Fatal("expected free-tier resume boundary after missed first-of-month tick")
	}
	if periodKey != "2026-05" {
		t.Fatalf("periodKey = %q, want 2026-05", periodKey)
	}
	wantBoundary := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if !boundary.Equal(wantBoundary) {
		t.Fatalf("boundary = %v, want %v", boundary, wantBoundary)
	}
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

	if err := enforcer.enforceLocked(context.Background()); err != nil {
		t.Fatalf("first enforceLocked() error = %v", err)
	}
	if err := enforcer.enforceLocked(context.Background()); err != nil {
		t.Fatalf("second enforceLocked() error = %v", err)
	}
	if store.unpauseCalls != 2 {
		t.Fatalf("unpause calls = %d, want retry after empty boundary pass", store.unpauseCalls)
	}
	if len(store.boundaries) != 2 || !store.boundaries[0].Equal(periodEnd) || !store.boundaries[1].Equal(periodEnd) {
		t.Fatalf("unpause boundaries = %v, want two attempts at %v", store.boundaries, periodEnd)
	}
}
