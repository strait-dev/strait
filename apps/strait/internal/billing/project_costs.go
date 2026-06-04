package billing

import (
	"context"
	"fmt"
	"time"
)

// ProjectCostEntry summarizes costs for a single project within a period.
type ProjectCostEntry struct {
	ProjectID  string `json:"project_id"`
	Name       string `json:"name"`
	Runs       int64  `json:"runs"`
	SpendMicro int64  `json:"spend_microusd"`
	TotalMicro int64  `json:"total_microusd"`
}

// GetProjectCosts returns per-project cost allocations for an org over the given period.
// It aggregates usage records by project ID.
func GetProjectCosts(ctx context.Context, store Store, orgID string, from, to time.Time) ([]ProjectCostEntry, error) {
	records, err := store.GetOrgUsageForPeriod(ctx, orgID, from, to)
	if err != nil {
		return nil, fmt.Errorf("getting org usage for project costs: %w", err)
	}

	projectMap := make(map[string]*ProjectCostEntry)

	for _, r := range records {
		entry, ok := projectMap[r.ProjectID]
		if !ok {
			entry = &ProjectCostEntry{
				ProjectID: r.ProjectID,
				Name:      r.ProjectID, // name defaults to ID; caller can enrich
			}
			projectMap[r.ProjectID] = entry
		}
		entry.Runs += r.RunsCount
		entry.SpendMicro += r.ComputeCostMicro
		entry.TotalMicro += r.ComputeCostMicro
	}

	entries := make([]ProjectCostEntry, 0, len(projectMap))
	for _, entry := range projectMap {
		entries = append(entries, *entry)
	}

	return entries, nil
}
