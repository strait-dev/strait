//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"
)

func TestGetCostAnalytics_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	now := time.Now().UTC()
	// Short period (live path).
	result, err := q.GetCostAnalytics(ctx, "project-cost-analytics", now.Add(-1*time.Hour), now)
	if err != nil {
		t.Fatalf("GetCostAnalytics(live) error = %v", err)
	}
	if result == nil {
		t.Fatal("GetCostAnalytics(live) returned nil")
	}
	if result.TotalSpendMicrousd != 0 {
		t.Fatalf("TotalSpendMicrousd = %d, want 0", result.TotalSpendMicrousd)
	}
	if result.RunCount != 0 {
		t.Fatalf("RunCount = %d, want 0", result.RunCount)
	}
}

func TestGetCostAnalytics_MaterializedPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	now := time.Now().UTC()
	// Long period (materialized path).
	result, err := q.GetCostAnalytics(ctx, "project-cost-analytics-mat", now.Add(-48*time.Hour), now)
	if err != nil {
		t.Fatalf("GetCostAnalytics(materialized) error = %v", err)
	}
	if result == nil {
		t.Fatal("GetCostAnalytics(materialized) returned nil")
	}
}

func TestGetCostTrends_Live(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	now := time.Now().UTC()
	// Short period triggers live path.
	points, err := q.GetCostTrends(ctx, "project-cost-trends-live", now.Add(-1*time.Hour), now)
	if err != nil {
		t.Fatalf("GetCostTrends(live) error = %v", err)
	}
	if len(points) != 0 {
		t.Fatalf("GetCostTrends(live) len = %d, want 0", len(points))
	}
}

func TestGetCostTrends_Materialized(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	now := time.Now().UTC()
	// Long period triggers materialized path.
	points, err := q.GetCostTrends(ctx, "project-cost-trends-mat", now.Add(-48*time.Hour), now)
	if err != nil {
		t.Fatalf("GetCostTrends(materialized) error = %v", err)
	}
	if len(points) != 0 {
		t.Fatalf("GetCostTrends(materialized) len = %d, want 0", len(points))
	}
}

func TestGetTopCosts_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	now := time.Now().UTC()
	items, err := q.GetTopCosts(ctx, "project-top-costs", now.Add(-1*time.Hour), now, 10)
	if err != nil {
		t.Fatalf("GetTopCosts() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("GetTopCosts() len = %d, want 0", len(items))
	}
}

func TestAggregateCostStatsHourly(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// Should succeed even with no data.
	hour := time.Now().UTC().Truncate(time.Hour)
	if err := q.AggregateCostStatsHourly(ctx, hour); err != nil {
		t.Fatalf("AggregateCostStatsHourly() error = %v", err)
	}
}

func TestGetCostOutliers_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	now := time.Now().UTC()
	outliers, err := q.GetCostOutliers(ctx, "project-cost-outliers", now.Add(-1*time.Hour), now, 2.0)
	if err != nil {
		t.Fatalf("GetCostOutliers() error = %v", err)
	}
	if len(outliers) != 0 {
		t.Fatalf("GetCostOutliers() len = %d, want 0", len(outliers))
	}
}
