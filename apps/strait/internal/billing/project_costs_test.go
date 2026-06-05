package billing

import (
	"context"
	"testing"
	"time"
)

type mockProjectCostStore struct {
	mockBillingStore
	usageRecords []UsageRecord
}

func (m *mockProjectCostStore) GetOrgUsageForPeriod(_ context.Context, _ string, _, _ time.Time) ([]UsageRecord, error) {
	return m.usageRecords, nil
}

func TestGetProjectCosts_Empty(t *testing.T) {
	store := &mockProjectCostStore{}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)

	entries, err := GetProjectCosts(context.Background(), store, "org-1", from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestGetProjectCosts_AggregatesByProject(t *testing.T) {
	records := []UsageRecord{
		{ProjectID: "proj-a", RunsCount: 10, ComputeCostMicro: 1000},
		{ProjectID: "proj-a", RunsCount: 5, ComputeCostMicro: 2000},
		{ProjectID: "proj-b", RunsCount: 3, ComputeCostMicro: 800},
	}

	store := &mockProjectCostStore{usageRecords: records}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)

	entries, err := GetProjectCosts(context.Background(), store, "org-1", from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	entryMap := make(map[string]ProjectCostEntry)
	for _, e := range entries {
		entryMap[e.ProjectID] = e
	}

	projA := entryMap["proj-a"]
	if projA.Runs != 15 {
		t.Errorf("proj-a runs: expected 15, got %d", projA.Runs)
	}
	if projA.SpendMicro != 3000 {
		t.Errorf("proj-a spend: expected 3000, got %d", projA.SpendMicro)
	}
	if projA.TotalMicro != 3000 {
		t.Errorf("proj-a total: expected 3000, got %d", projA.TotalMicro)
	}

	projB := entryMap["proj-b"]
	if projB.Runs != 3 {
		t.Errorf("proj-b runs: expected 3, got %d", projB.Runs)
	}
	if projB.TotalMicro != 800 {
		t.Errorf("proj-b total: expected 800, got %d", projB.TotalMicro)
	}
}
