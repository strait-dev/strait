package billing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t,
		err)
	require.Len(t, entries,
		0)

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
	require.NoError(t,
		err)
	require.Len(t, entries,
		2)

	entryMap := make(map[string]ProjectCostEntry)
	for _, e := range entries {
		entryMap[e.ProjectID] = e
	}

	projA := entryMap["proj-a"]
	assert.EqualValues(t, 15,
		projA.Runs,
	)
	assert.EqualValues(t, 3000,
		projA.
			SpendMicro,
	)
	assert.EqualValues(t, 3000,
		projA.
			TotalMicro,
	)

	projB := entryMap["proj-b"]
	assert.EqualValues(t, 3,
		projB.Runs,
	)
	assert.EqualValues(t, 800,
		projB.TotalMicro,
	)

}
