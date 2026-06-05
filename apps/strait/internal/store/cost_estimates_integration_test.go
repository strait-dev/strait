//go:build integration

package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetJobCostEstimate_NoHistory(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// With no ClickHouse wired and no run history, GetJobCostEstimate must
	// return the flat-rate fallback (20 micro-USD) rather than nil.
	est, err := q.GetJobCostEstimate(ctx, newID())
	require.NoError(t, err)
	require.NotNil(t, est)
	assert.EqualValues(t, 20, est.
		AvgCostMicrousd,
	)
	assert.EqualValues(t, 0, est.
		SampleCount,
	)

}
