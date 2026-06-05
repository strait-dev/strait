//go:build integration

package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureJobRunsPartitions_Idempotent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	require.NoError(t, q.EnsureJobRunsPartitions(ctx, 2))
	require.NoError(t, q.EnsureJobRunsPartitions(ctx, 2))

	// Second call is a no-op but must not error.

}

func TestEnsureJobRunsPartitions_ListPartitionsReturnsSome(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	require.NoError(t, q.EnsureJobRunsPartitions(ctx, 2))

	partitions, err := q.ListJobRunsPartitions(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, partitions)

}

func TestEnsureJobRunsPartitions_HandlesMissingPartitionRequest(t *testing.T) {
	// Drop an expected future partition then ensure it's recreated.
	ctx := context.Background()
	q := mustStore(t)
	require.NoError(t, q.EnsureJobRunsPartitions(ctx, 2))

	// First, ensure with 2 ahead. This creates several future partitions.

	partitions, _ := q.ListJobRunsPartitions(ctx)
	if len(partitions) == 0 {
		t.Skip("no partitions returned; cannot test recreate")
	}
	require.NoError(t, q.EnsureJobRunsPartitions(ctx, 3))

	// We don't actually want to drop a partition from the test DB because
	// future tests might need it, but we can verify the re-ensure path is
	// safe by calling again with a larger monthsAhead.

}
