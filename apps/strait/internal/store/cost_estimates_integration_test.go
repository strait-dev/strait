//go:build integration

package store_test

import (
	"context"
	"testing"
)

func TestGetJobCostEstimate_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	est, err := q.GetJobCostEstimate(ctx, newID())
	if err != nil {
		t.Fatalf("GetJobCostEstimate() error = %v", err)
	}
	if est != nil {
		t.Fatalf("GetJobCostEstimate() = %+v, want nil", est)
	}
}
