//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

// All analytics_extended.go methods are Postgres fallback stubs that return empty results.
// They are designed as no-op placeholders since ClickHouse is the primary data source.
// We verify each returns the expected empty value without error.

func TestGetRunTimeline(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetRunTimeline(ctx, "project-analytics", now.Add(-1*time.Hour), now, "1h")
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetRunDurationDistribution(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetRunDurationDistribution(ctx, "project-analytics", now.Add(-1*time.Hour), now)
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetRunFailureReasons(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetRunFailureReasons(ctx, "project-analytics", now.Add(-1*time.Hour), now, 10)
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetRunSummary(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetRunSummary(ctx, "project-analytics", now.Add(-1*time.Hour), now)
	require.NoError(t, err)
	require.NotNil(t, result)

}

func TestGetRunsByTrigger(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetRunsByTrigger(ctx, "project-analytics", now.Add(-1*time.Hour), now)
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetJobHistory(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetJobHistory(ctx, "project-analytics", "job-1", now.Add(-1*time.Hour), now, "1h")
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetJobComparison(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetJobComparison(ctx, "project-analytics", []string{"job-1"}, now.Add(-1*time.Hour), now)
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetJobReliability(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetJobReliability(ctx, "project-analytics", now.Add(-1*time.Hour), now, 10)
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetRunsByVersion(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetRunsByVersion(ctx, "project-analytics", "job-1", now.Add(-1*time.Hour), now)
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetJobCostRanking(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetJobCostRanking(ctx, "project-analytics", now.Add(-1*time.Hour), now, 10)
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetTopFailingJobs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetTopFailingJobs(ctx, "project-analytics", now.Add(-1*time.Hour), now, 10)
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetTagSummary(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetTagSummary(ctx, "project-analytics", now.Add(-1*time.Hour), now, 10)
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetTopFailingTags(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetTopFailingTags(ctx, "project-analytics", now.Add(-1*time.Hour), now, 10)
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetTagCost(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetTagCost(ctx, "project-analytics", now.Add(-1*time.Hour), now, 10)
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetWorkflowStepDurations(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetWorkflowStepDurations(ctx, "project-analytics", "wf-1", now.Add(-1*time.Hour), now)
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetWorkflowCompletionRates(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetWorkflowCompletionRates(ctx, "project-analytics", now.Add(-1*time.Hour), now, "1h")
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetWorkflowSummary(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetWorkflowSummary(ctx, "project-analytics", now.Add(-1*time.Hour), now)
	require.NoError(t, err)
	require.NotNil(t, result)

}

func TestGetWebhookDeliveryStats(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetWebhookDeliveryStats(ctx, "project-analytics", now.Add(-1*time.Hour), now)
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetWebhookEndpointHealth(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetWebhookEndpointHealth(ctx, "project-analytics", now.Add(-1*time.Hour), now, "1h")
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetTopFailingWebhooks(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetTopFailingWebhooks(ctx, "project-analytics", now.Add(-1*time.Hour), now, 10)
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetEventVolume(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetEventVolume(ctx, "project-analytics", now.Add(-1*time.Hour), now, "1h")
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

func TestGetEventLatency(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetEventLatency(ctx, "project-analytics", now.Add(-1*time.Hour), now)
	require.NoError(t, err)
	require.NotNil(t, result)

}

func TestGetCostForecast(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetCostForecast(ctx, "project-analytics", now.Add(-1*time.Hour), now)
	require.NoError(t, err)
	require.NotNil(t, result)

}

func TestGetCostByTrigger(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetCostByTrigger(ctx, "project-analytics", now.Add(-1*time.Hour), now)
	require.NoError(t, err)
	require.Len(t, result,
		0,
	)

}

// Ensure unused import is referenced.
var _ = store.New
