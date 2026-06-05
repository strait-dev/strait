package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"strait/internal/billing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockUsageFlusherStore implements UsageFlusherStore for testing.
type mockUsageFlusherStore struct {
	listAllSubscribedOrgIDsFn func(ctx context.Context) ([]string, error)
	getOrgDailyUsageFn        func(ctx context.Context, orgID string, date time.Time) ([]billing.UsageRecord, error)
	replaceUsageRecordFn      func(ctx context.Context, rec *billing.UsageRecord) error
	reconcileFlatUsageCostsFn func(ctx context.Context, orgID string, date time.Time) error
}

func (m *mockUsageFlusherStore) ListAllSubscribedOrgIDs(ctx context.Context) ([]string, error) {
	if m.listAllSubscribedOrgIDsFn != nil {
		return m.listAllSubscribedOrgIDsFn(ctx)
	}
	return nil, nil
}

func (m *mockUsageFlusherStore) GetOrgDailyUsage(ctx context.Context, orgID string, date time.Time) ([]billing.UsageRecord, error) {
	if m.getOrgDailyUsageFn != nil {
		return m.getOrgDailyUsageFn(ctx, orgID, date)
	}
	return nil, nil
}

func (m *mockUsageFlusherStore) ReplaceUsageRecord(ctx context.Context, rec *billing.UsageRecord) error {
	if m.replaceUsageRecordFn != nil {
		return m.replaceUsageRecordFn(ctx, rec)
	}
	return nil
}

func (m *mockUsageFlusherStore) ReconcileFlatUsageCosts(ctx context.Context, orgID string, date time.Time) error {
	if m.reconcileFlatUsageCostsFn != nil {
		return m.reconcileFlatUsageCostsFn(ctx, orgID, date)
	}
	return nil
}

func TestUsageFlusher_FlushesRecordsForAllOrgs(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	var mu sync.Mutex
	upserted := make(map[string]*billing.UsageRecord)

	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1", "org-2"}, nil
		},
		getOrgDailyUsageFn: func(_ context.Context, orgID string, date time.Time) ([]billing.UsageRecord, error) {
			if !date.Equal(today) {
				return nil, nil
			}
			return []billing.UsageRecord{
				{
					OrgID:            orgID,
					ProjectID:        "proj-" + orgID,
					PeriodDate:       today,
					RunsCount:        10,
					ComputeCostMicro: 5000,
				},
			}, nil
		},
		replaceUsageRecordFn: func(_ context.Context, rec *billing.UsageRecord) error {
			mu.Lock()
			upserted[rec.OrgID+":"+rec.ProjectID] = rec
			mu.Unlock()
			return nil
		},
	}

	uf := NewUsageFlusher(s, time.Minute)
	uf.flush(context.Background())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, upserted,
		2)

	r1, ok := upserted["org-1:proj-org-1"]
	require.True(t, ok)
	assert.EqualValues(t, 10,
		r1.RunsCount,
	)
	assert.EqualValues(t, 5000,
		r1.ComputeCostMicro,
	)

	if _, ok := upserted["org-2:proj-org-2"]; !ok {
		require.Fail(t,

			"expected record for org-2:proj-org-2")
	}
}

func TestUsageFlusher_EmptyOrgs_NoFlush(t *testing.T) {
	t.Parallel()

	upsertCalled := false
	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return nil, nil
		},
		replaceUsageRecordFn: func(_ context.Context, _ *billing.UsageRecord) error {
			upsertCalled = true
			return nil
		},
	}

	uf := NewUsageFlusher(s, time.Minute)
	uf.flush(context.Background())
	require.False(t, upsertCalled)

}

func TestUsageFlusher_ReplacesSnapshotEachFlush(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	var mu sync.Mutex
	var upsertCount int

	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgDailyUsageFn: func(_ context.Context, orgID string, date time.Time) ([]billing.UsageRecord, error) {
			if !date.Equal(today) {
				return nil, nil
			}
			return []billing.UsageRecord{
				{
					OrgID:            orgID,
					ProjectID:        "proj-1",
					PeriodDate:       today,
					RunsCount:        5,
					ComputeCostMicro: 2500,
				},
			}, nil
		},
		replaceUsageRecordFn: func(_ context.Context, _ *billing.UsageRecord) error {
			mu.Lock()
			upsertCount++
			mu.Unlock()
			return nil
		},
	}

	uf := NewUsageFlusher(s, time.Minute)

	// Flush twice. The flusher writes a replacement snapshot each time; the
	// store must not add cumulative daily totals on top of prior snapshots.
	uf.flush(context.Background())
	uf.flush(context.Background())

	mu.Lock()
	defer mu.Unlock()
	require.EqualValues(t, 2,
		upsertCount,
	)

}

func TestUsageFlusher_PartialFailure_ContinuesOtherOrgs(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	var mu sync.Mutex
	upsertedOrgs := make(map[string]bool)

	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1", "org-2", "org-3"}, nil
		},
		getOrgDailyUsageFn: func(_ context.Context, orgID string, date time.Time) ([]billing.UsageRecord, error) {
			if orgID == "org-2" {
				return nil, errors.New("db timeout")
			}
			if !date.Equal(today) {
				return nil, nil
			}
			return []billing.UsageRecord{
				{
					OrgID:            orgID,
					ProjectID:        "proj-" + orgID,
					PeriodDate:       today,
					RunsCount:        1,
					ComputeCostMicro: 100,
				},
			}, nil
		},
		replaceUsageRecordFn: func(_ context.Context, rec *billing.UsageRecord) error {
			mu.Lock()
			upsertedOrgs[rec.OrgID] = true
			mu.Unlock()
			return nil
		},
	}

	uf := NewUsageFlusher(s, time.Minute)
	uf.flush(context.Background())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, upsertedOrgs,

		2)
	assert.True(t, upsertedOrgs["org-1"])
	assert.False(t, upsertedOrgs["org-2"])
	assert.True(t, upsertedOrgs["org-3"])

}

func TestUsageFlusher_DedupesOrgIDsAndSkipsEmpty(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	getCalls := make(map[string]int)
	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1", "", "org-1", "org-2"}, nil
		},
		getOrgDailyUsageFn: func(_ context.Context, orgID string, _ time.Time) ([]billing.UsageRecord, error) {
			mu.Lock()
			getCalls[orgID]++
			mu.Unlock()
			return nil, nil
		},
	}

	NewUsageFlusher(s, time.Minute).flush(context.Background())

	mu.Lock()
	defer mu.Unlock()
	require.False(t, len(getCalls) != 2 ||
		getCalls["org-1"] !=
			usageFlusherReconcileLookbackDays ||
		getCalls["org-2"] != usageFlusherReconcileLookbackDays)

	if _, ok := getCalls[""]; ok {
		require.Fail(t,

			"empty org ID should not be flushed")
	}
}

func TestUsageFlusher_FlushesSnapshotsAcrossLookback(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var requested []time.Time
	var upserted []time.Time
	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgDailyUsageFn: func(_ context.Context, orgID string, date time.Time) ([]billing.UsageRecord, error) {
			mu.Lock()
			requested = append(requested, date)
			mu.Unlock()
			return []billing.UsageRecord{{OrgID: orgID, ProjectID: "proj-1", RunsCount: 1}}, nil
		},
		replaceUsageRecordFn: func(_ context.Context, rec *billing.UsageRecord) error {
			mu.Lock()
			upserted = append(upserted, rec.PeriodDate)
			mu.Unlock()
			return nil
		},
	}

	NewUsageFlusher(s, time.Minute).flush(context.Background())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, requested,
		usageFlusherReconcileLookbackDays,
	)
	require.Len(t, upserted,
		usageFlusherReconcileLookbackDays,
	)

	for i := 1; i < len(upserted); i++ {
		require.True(t, upserted[i].After(upserted[i-1]))

	}
	for i := range requested {
		require.True(t, requested[i].
			Equal(upserted[i]))

	}
}

func TestUsageFlusher_ReconcilesFlatUsageCostsAcrossLookback(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	reconciled := make(map[string][]time.Time)
	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		reconcileFlatUsageCostsFn: func(_ context.Context, orgID string, date time.Time) error {
			mu.Lock()
			reconciled[orgID] = append(reconciled[orgID], date)
			mu.Unlock()
			return nil
		},
	}

	NewUsageFlusher(s, time.Minute).flush(context.Background())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, reconciled["org-1"], usageFlusherReconcileLookbackDays)

	for i := 1; i < len(reconciled["org-1"]); i++ {
		require.True(t, reconciled["org-1"][i].After(reconciled["org-1"][i-1]))

	}
}

func TestUsageFlusher_NormalizesEmptySnapshotFields(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	var got *billing.UsageRecord
	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgDailyUsageFn: func(_ context.Context, orgID string, date time.Time) ([]billing.UsageRecord, error) {
			if !date.Equal(today) {
				return nil, nil
			}
			return []billing.UsageRecord{{OrgID: orgID, ProjectID: "proj-1", RunsCount: 1}}, nil
		},
		replaceUsageRecordFn: func(_ context.Context, rec *billing.UsageRecord) error {
			copy := *rec
			got = &copy
			return nil
		},
	}

	NewUsageFlusher(s, time.Minute).flush(context.Background())
	require.NotNil(t, got)
	require.NotEqual(t,
		"", got.ID,
	)
	require.False(t, got.
		PeriodDate.
		IsZero())
	require.False(t, got.
		CreatedAt.
		IsZero() || got.UpdatedAt.
		IsZero())

}
