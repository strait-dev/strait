//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetCostAnalytics_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	now := time.Now().UTC()
	// Short period (live path).
	result, err := q.GetCostAnalytics(ctx, "project-cost-analytics", now.Add(-1*time.Hour), now)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.EqualValues(t, 0, result.
		TotalSpendMicrousd,
	)
	require.EqualValues(t, 0, result.
		RunCount,
	)

}

func TestGetCostAnalytics_MaterializedPath(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	now := time.Now().UTC()
	// Long period (materialized path).
	result, err := q.GetCostAnalytics(ctx, "project-cost-analytics-mat", now.Add(-48*time.Hour), now)
	require.NoError(t, err)
	require.NotNil(t, result)

}

func TestGetCostTrends_Live(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	now := time.Now().UTC()
	// Short period triggers live path.
	points, err := q.GetCostTrends(ctx, "project-cost-trends-live", now.Add(-1*time.Hour), now)
	require.NoError(t, err)
	require.Len(t, points,
		0,
	)

}

func TestGetCostTrends_Materialized(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	now := time.Now().UTC()
	// Long period triggers materialized path.
	points, err := q.GetCostTrends(ctx, "project-cost-trends-mat", now.Add(-48*time.Hour), now)
	require.NoError(t, err)
	require.Len(t, points,
		0,
	)

}

func TestGetTopCosts_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	now := time.Now().UTC()
	items, err := q.GetTopCosts(ctx, "project-top-costs", now.Add(-1*time.Hour), now, 10)
	require.NoError(t, err)
	require.Len(t, items, 0)

}

func TestAggregateCostStatsHourly(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// Should succeed even with no data.
	hour := time.Now().UTC().Truncate(time.Hour)
	require.NoError(t, q.AggregateCostStatsHourly(ctx, hour))

}

func TestGetCostOutliers_Empty(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	now := time.Now().UTC()
	outliers, err := q.GetCostOutliers(ctx, "project-cost-outliers", now.Add(-1*time.Hour), now, 2.0)
	require.NoError(t, err)
	require.Len(t, outliers,

		0)

}
