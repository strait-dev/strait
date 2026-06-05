//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

// Remaining methods.

// promoteDeploymentVersion is private, tested indirectly via higher-level promotion flows.
// scanEnvironment is private (scanner utility), skip.
// decryptNotificationConfig is private, tested indirectly.
// findLatestTerminalDependencyRun is private, tested indirectly.
// TryAdvisoryLock / ReleaseAdvisoryLock.

func TestStore_TryAdvisoryLock_AcquireAndRelease(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	lockID := int64(999999)

	acquired, err := q.TryAdvisoryLock(ctx, lockID)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, q.ReleaseAdvisoryLock(ctx,
		lockID))

}

func TestStore_TryAdvisoryLock_ReacquireAfterRelease(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	lockID := int64(999998)

	acquired, err := q.TryAdvisoryLock(ctx, lockID)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, q.ReleaseAdvisoryLock(ctx,
		lockID))

	acquired2, err := q.TryAdvisoryLock(ctx, lockID)
	require.NoError(t, err)
	require.True(t, acquired2)
	require.NoError(t, q.ReleaseAdvisoryLock(ctx,
		lockID))

}

func TestStore_TryAdvisoryLock_DifferentLocks(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	lockA := int64(888881)
	lockB := int64(888882)

	acquiredA, err := q.TryAdvisoryLock(ctx, lockA)
	require.NoError(t, err)
	require.True(t, acquiredA)

	acquiredB, err := q.TryAdvisoryLock(ctx, lockB)
	require.NoError(t, err)
	require.True(t, acquiredB)
	require.NoError(t, q.ReleaseAdvisoryLock(ctx,
		lockA))
	require.NoError(t, q.ReleaseAdvisoryLock(ctx,
		lockB))

}

// WithTx basic scenarios.

func TestStore_WithTx_NestedCreateAndRead(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	jobID := newID()
	err := store.WithTx(ctx, testDB.Pool, func(q *store.Queries) error {
		job := baseJob(jobID, "project-withtx-nested")
		return q.CreateJob(ctx, job)
	})
	require.NoError(t, err)

	q := mustStore(t)
	got, err := q.GetJob(ctx, jobID)
	require.NoError(t, err)
	require.Equal(t, jobID,

		got.ID)

}

// AdvisoryXactLock.

func TestStore_AdvisoryXactLock_InTransaction(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	err := store.WithTx(ctx, testDB.Pool, func(q *store.Queries) error {
		return q.AdvisoryXactLock(ctx, int64(777777))
	})
	require.NoError(t, err)

}
