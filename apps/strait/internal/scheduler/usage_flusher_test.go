package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"strait/internal/billing"
)

// mockUsageFlusherStore implements UsageFlusherStore for testing.
type mockUsageFlusherStore struct {
	listAllSubscribedOrgIDsFn func(ctx context.Context) ([]string, error)
	getOrgDailyUsageFn        func(ctx context.Context, orgID string, date time.Time) ([]billing.UsageRecord, error)
	replaceUsageRecordFn      func(ctx context.Context, rec *billing.UsageRecord) error
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
		getOrgDailyUsageFn: func(_ context.Context, orgID string, _ time.Time) ([]billing.UsageRecord, error) {
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

	if len(upserted) != 2 {
		t.Fatalf("expected 2 upserted records, got %d", len(upserted))
	}

	r1, ok := upserted["org-1:proj-org-1"]
	if !ok {
		t.Fatal("expected record for org-1:proj-org-1")
	}
	if r1.RunsCount != 10 {
		t.Errorf("expected 10 runs, got %d", r1.RunsCount)
	}
	if r1.ComputeCostMicro != 5000 {
		t.Errorf("expected 5000 compute cost, got %d", r1.ComputeCostMicro)
	}

	if _, ok := upserted["org-2:proj-org-2"]; !ok {
		t.Fatal("expected record for org-2:proj-org-2")
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

	if upsertCalled {
		t.Fatal("expected no upsert when org list is empty")
	}
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
		getOrgDailyUsageFn: func(_ context.Context, orgID string, _ time.Time) ([]billing.UsageRecord, error) {
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
	if upsertCount != 2 {
		t.Fatalf("expected 2 upsert calls (one per flush), got %d", upsertCount)
	}
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
		getOrgDailyUsageFn: func(_ context.Context, orgID string, _ time.Time) ([]billing.UsageRecord, error) {
			if orgID == "org-2" {
				return nil, errors.New("db timeout")
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

	if len(upsertedOrgs) != 2 {
		t.Fatalf("expected 2 upserted orgs (skipping org-2), got %d", len(upsertedOrgs))
	}
	if !upsertedOrgs["org-1"] {
		t.Error("expected org-1 to be flushed")
	}
	if upsertedOrgs["org-2"] {
		t.Error("expected org-2 to be skipped due to error")
	}
	if !upsertedOrgs["org-3"] {
		t.Error("expected org-3 to be flushed despite org-2 failure")
	}
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
	if len(getCalls) != 2 || getCalls["org-1"] != 1 || getCalls["org-2"] != 1 {
		t.Fatalf("daily usage calls = %v, want one per non-empty org", getCalls)
	}
	if _, ok := getCalls[""]; ok {
		t.Fatal("empty org ID should not be flushed")
	}
}

func TestUsageFlusher_NormalizesEmptySnapshotFields(t *testing.T) {
	t.Parallel()

	var got *billing.UsageRecord
	s := &mockUsageFlusherStore{
		listAllSubscribedOrgIDsFn: func(context.Context) ([]string, error) {
			return []string{"org-1"}, nil
		},
		getOrgDailyUsageFn: func(_ context.Context, orgID string, _ time.Time) ([]billing.UsageRecord, error) {
			return []billing.UsageRecord{{OrgID: orgID, ProjectID: "proj-1", RunsCount: 1}}, nil
		},
		replaceUsageRecordFn: func(_ context.Context, rec *billing.UsageRecord) error {
			copy := *rec
			got = &copy
			return nil
		},
	}

	NewUsageFlusher(s, time.Minute).flush(context.Background())

	if got == nil {
		t.Fatal("expected replacement record")
	}
	if got.ID == "" {
		t.Fatal("expected generated ID")
	}
	if got.PeriodDate.IsZero() {
		t.Fatal("expected period date to be set")
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("expected timestamps to be set: created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
	}
}
