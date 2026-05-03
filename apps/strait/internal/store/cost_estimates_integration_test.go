//go:build integration

package store_test

import (
	"context"
	"testing"
)

func TestGetJobCostEstimate_NoHistory(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// With no ClickHouse wired and no run history, GetJobCostEstimate must
	// return the flat-rate fallback (20 micro-USD) rather than nil.
	est, err := q.GetJobCostEstimate(ctx, newID())
	if err != nil {
		t.Fatalf("GetJobCostEstimate() error = %v", err)
	}
	if est == nil {
		t.Fatal("GetJobCostEstimate() = nil, want flat-rate fallback")
	}
	if est.AvgCostMicrousd != 20 {
		t.Errorf("AvgCostMicrousd = %d, want 20 (flat-rate fallback)", est.AvgCostMicrousd)
	}
	if est.SampleCount != 0 {
		t.Errorf("SampleCount = %d, want 0 for flat-rate fallback", est.SampleCount)
	}
}
