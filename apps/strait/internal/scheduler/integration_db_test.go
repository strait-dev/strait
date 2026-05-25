//go:build integration

package scheduler

import (
	"context"
	"sync"
	"testing"

	"strait/internal/testutil"
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
	if schedulerTestDBOnce.err != nil {
		t.Fatalf("setup test db: %v", schedulerTestDBOnce.err)
	}
	if schedulerTestDBOnce.db == nil || schedulerTestDBOnce.db.Pool == nil {
		t.Fatal("scheduler test db is not initialized")
	}
	return schedulerTestDBOnce.db
}

func cleanSchedulerIntegrationDB(t *testing.T, ctx context.Context) *testutil.TestDB {
	t.Helper()
	tdb := schedulerIntegrationDB(t, ctx)
	if err := tdb.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	return tdb
}
