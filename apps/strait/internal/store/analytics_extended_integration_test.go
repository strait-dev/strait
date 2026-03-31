//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/store"
)

// All analytics_extended.go methods are Postgres fallback stubs that return empty results.
// They are designed as no-op placeholders since ClickHouse is the primary data source.
// We verify each returns the expected empty value without error.

func TestGetRunTimeline(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetRunTimeline(ctx, "project-analytics", now.Add(-1*time.Hour), now, "1h")
	if err != nil {
		t.Fatalf("GetRunTimeline() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetRunTimeline() len = %d, want 0", len(result))
	}
}

func TestGetRunDurationDistribution(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetRunDurationDistribution(ctx, "project-analytics", now.Add(-1*time.Hour), now)
	if err != nil {
		t.Fatalf("GetRunDurationDistribution() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetRunDurationDistribution() len = %d, want 0", len(result))
	}
}

func TestGetRunFailureReasons(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetRunFailureReasons(ctx, "project-analytics", now.Add(-1*time.Hour), now, 10)
	if err != nil {
		t.Fatalf("GetRunFailureReasons() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetRunFailureReasons() len = %d, want 0", len(result))
	}
}

func TestGetRunSummary(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetRunSummary(ctx, "project-analytics", now.Add(-1*time.Hour), now)
	if err != nil {
		t.Fatalf("GetRunSummary() error = %v", err)
	}
	if result == nil {
		t.Fatal("GetRunSummary() returned nil")
	}
}

func TestGetRunsByTrigger(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetRunsByTrigger(ctx, "project-analytics", now.Add(-1*time.Hour), now)
	if err != nil {
		t.Fatalf("GetRunsByTrigger() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetRunsByTrigger() len = %d, want 0", len(result))
	}
}

func TestGetJobHistory(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetJobHistory(ctx, "project-analytics", "job-1", now.Add(-1*time.Hour), now, "1h")
	if err != nil {
		t.Fatalf("GetJobHistory() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetJobHistory() len = %d, want 0", len(result))
	}
}

func TestGetJobComparison(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetJobComparison(ctx, "project-analytics", []string{"job-1"}, now.Add(-1*time.Hour), now)
	if err != nil {
		t.Fatalf("GetJobComparison() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetJobComparison() len = %d, want 0", len(result))
	}
}

func TestGetJobReliability(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetJobReliability(ctx, "project-analytics", now.Add(-1*time.Hour), now, 10)
	if err != nil {
		t.Fatalf("GetJobReliability() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetJobReliability() len = %d, want 0", len(result))
	}
}

func TestGetRunsByVersion(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetRunsByVersion(ctx, "project-analytics", "job-1", now.Add(-1*time.Hour), now)
	if err != nil {
		t.Fatalf("GetRunsByVersion() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetRunsByVersion() len = %d, want 0", len(result))
	}
}

func TestGetJobCostRanking(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetJobCostRanking(ctx, "project-analytics", now.Add(-1*time.Hour), now, 10)
	if err != nil {
		t.Fatalf("GetJobCostRanking() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetJobCostRanking() len = %d, want 0", len(result))
	}
}

func TestGetTopFailingJobs(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetTopFailingJobs(ctx, "project-analytics", now.Add(-1*time.Hour), now, 10)
	if err != nil {
		t.Fatalf("GetTopFailingJobs() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetTopFailingJobs() len = %d, want 0", len(result))
	}
}

func TestGetTagSummary(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetTagSummary(ctx, "project-analytics", now.Add(-1*time.Hour), now, 10)
	if err != nil {
		t.Fatalf("GetTagSummary() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetTagSummary() len = %d, want 0", len(result))
	}
}

func TestGetTopFailingTags(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetTopFailingTags(ctx, "project-analytics", now.Add(-1*time.Hour), now, 10)
	if err != nil {
		t.Fatalf("GetTopFailingTags() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetTopFailingTags() len = %d, want 0", len(result))
	}
}

func TestGetTagCost(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetTagCost(ctx, "project-analytics", now.Add(-1*time.Hour), now, 10)
	if err != nil {
		t.Fatalf("GetTagCost() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetTagCost() len = %d, want 0", len(result))
	}
}

func TestGetWorkflowStepDurations(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetWorkflowStepDurations(ctx, "project-analytics", "wf-1", now.Add(-1*time.Hour), now)
	if err != nil {
		t.Fatalf("GetWorkflowStepDurations() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetWorkflowStepDurations() len = %d, want 0", len(result))
	}
}

func TestGetWorkflowCompletionRates(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetWorkflowCompletionRates(ctx, "project-analytics", now.Add(-1*time.Hour), now, "1h")
	if err != nil {
		t.Fatalf("GetWorkflowCompletionRates() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetWorkflowCompletionRates() len = %d, want 0", len(result))
	}
}

func TestGetWorkflowSummary(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetWorkflowSummary(ctx, "project-analytics", now.Add(-1*time.Hour), now)
	if err != nil {
		t.Fatalf("GetWorkflowSummary() error = %v", err)
	}
	if result == nil {
		t.Fatal("GetWorkflowSummary() returned nil")
	}
}

func TestGetWebhookDeliveryStats(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetWebhookDeliveryStats(ctx, "project-analytics", now.Add(-1*time.Hour), now)
	if err != nil {
		t.Fatalf("GetWebhookDeliveryStats() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetWebhookDeliveryStats() len = %d, want 0", len(result))
	}
}

func TestGetWebhookEndpointHealth(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetWebhookEndpointHealth(ctx, "project-analytics", now.Add(-1*time.Hour), now, "1h")
	if err != nil {
		t.Fatalf("GetWebhookEndpointHealth() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetWebhookEndpointHealth() len = %d, want 0", len(result))
	}
}

func TestGetTopFailingWebhooks(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetTopFailingWebhooks(ctx, "project-analytics", now.Add(-1*time.Hour), now, 10)
	if err != nil {
		t.Fatalf("GetTopFailingWebhooks() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetTopFailingWebhooks() len = %d, want 0", len(result))
	}
}

func TestGetEventVolume(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetEventVolume(ctx, "project-analytics", now.Add(-1*time.Hour), now, "1h")
	if err != nil {
		t.Fatalf("GetEventVolume() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetEventVolume() len = %d, want 0", len(result))
	}
}

func TestGetEventLatency(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetEventLatency(ctx, "project-analytics", now.Add(-1*time.Hour), now)
	if err != nil {
		t.Fatalf("GetEventLatency() error = %v", err)
	}
	if result == nil {
		t.Fatal("GetEventLatency() returned nil")
	}
}

func TestGetCostForecast(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetCostForecast(ctx, "project-analytics", now.Add(-1*time.Hour), now)
	if err != nil {
		t.Fatalf("GetCostForecast() error = %v", err)
	}
	if result == nil {
		t.Fatal("GetCostForecast() returned nil")
	}
}

func TestGetCostByTrigger(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetCostByTrigger(ctx, "project-analytics", now.Add(-1*time.Hour), now)
	if err != nil {
		t.Fatalf("GetCostByTrigger() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetCostByTrigger() len = %d, want 0", len(result))
	}
}

func TestGetCostByMachine(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)

	now := time.Now().UTC()
	result, err := q.GetCostByMachine(ctx, "project-analytics", now.Add(-1*time.Hour), now)
	if err != nil {
		t.Fatalf("GetCostByMachine() error = %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("GetCostByMachine() len = %d, want 0", len(result))
	}
}

// Ensure unused import is referenced.
var _ = store.New
