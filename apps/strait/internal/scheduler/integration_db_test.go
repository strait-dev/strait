//go:build integration

package scheduler

import (
	"context"
	"sync"
	"testing"

	"strait/internal/testutil"

	"github.com/stretchr/testify/require"
)

var schedulerTestDBOnce = newIntegrationDBOnce()

type integrationDBOnce struct {
	once sync.Once
	db   *testutil.TestDB
	err  error
}

func newIntegrationDBOnce() *integrationDBOnce {
	return &integrationDBOnce{}
}

func schedulerIntegrationDB(t *testing.T, ctx context.Context) *testutil.TestDB {
	t.Helper()
	schedulerTestDBOnce.once.Do(func() {
		schedulerTestDBOnce.db, schedulerTestDBOnce.err = testutil.SetupSharedTestDB(ctx, "../../migrations", "scheduler")
	})
	require.Nil(t, schedulerTestDBOnce.
		err,
	)
	require.False(t, schedulerTestDBOnce.
		db ==
		nil || schedulerTestDBOnce.
		db.Pool == nil)

	return schedulerTestDBOnce.db
}

func cleanSchedulerIntegrationDB(t *testing.T, ctx context.Context) *testutil.TestDB {
	t.Helper()
	tdb := schedulerIntegrationDB(t, ctx)
	require.NoError(t, tdb.
		CleanTables(ctx))

	return tdb
}
