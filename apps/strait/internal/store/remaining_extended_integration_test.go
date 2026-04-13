//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/store"
)

// --------------------------------------------------------------------------.
// Remaining methods.
// --------------------------------------------------------------------------.

// promoteDeploymentVersion is private, tested indirectly via higher-level promotion flows.
// scanEnvironment is private (scanner utility), skip.
// decryptNotificationConfig is private, tested indirectly.
// findLatestTerminalDependencyRun is private, tested indirectly.

// --------------------------------------------------------------------------.
// TryAdvisoryLock / ReleaseAdvisoryLock.
// --------------------------------------------------------------------------.

func TestStore_TryAdvisoryLock_AcquireAndRelease(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	lockID := int64(999999)

	acquired, err := q.TryAdvisoryLock(ctx, lockID)
	if err != nil {
		t.Fatalf("TryAdvisoryLock() error = %v", err)
	}
	if !acquired {
		t.Fatal("expected lock to be acquired")
	}

	if err := q.ReleaseAdvisoryLock(ctx, lockID); err != nil {
		t.Fatalf("ReleaseAdvisoryLock() error = %v", err)
	}
}

func TestStore_TryAdvisoryLock_ReacquireAfterRelease(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	lockID := int64(999998)

	acquired, err := q.TryAdvisoryLock(ctx, lockID)
	if err != nil {
		t.Fatalf("TryAdvisoryLock() first error = %v", err)
	}
	if !acquired {
		t.Fatal("first acquire should succeed")
	}
	if err := q.ReleaseAdvisoryLock(ctx, lockID); err != nil {
		t.Fatalf("ReleaseAdvisoryLock() error = %v", err)
	}

	acquired2, err := q.TryAdvisoryLock(ctx, lockID)
	if err != nil {
		t.Fatalf("TryAdvisoryLock() second error = %v", err)
	}
	if !acquired2 {
		t.Fatal("second acquire should succeed after release")
	}
	if err := q.ReleaseAdvisoryLock(ctx, lockID); err != nil {
		t.Fatalf("ReleaseAdvisoryLock() second error = %v", err)
	}
}

func TestStore_TryAdvisoryLock_DifferentLocks(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	lockA := int64(888881)
	lockB := int64(888882)

	acquiredA, err := q.TryAdvisoryLock(ctx, lockA)
	if err != nil {
		t.Fatalf("TryAdvisoryLock(A) error = %v", err)
	}
	if !acquiredA {
		t.Fatal("lock A should be acquired")
	}

	acquiredB, err := q.TryAdvisoryLock(ctx, lockB)
	if err != nil {
		t.Fatalf("TryAdvisoryLock(B) error = %v", err)
	}
	if !acquiredB {
		t.Fatal("lock B should be acquired independently")
	}

	if err := q.ReleaseAdvisoryLock(ctx, lockA); err != nil {
		t.Fatalf("ReleaseAdvisoryLock(A) error = %v", err)
	}
	if err := q.ReleaseAdvisoryLock(ctx, lockB); err != nil {
		t.Fatalf("ReleaseAdvisoryLock(B) error = %v", err)
	}
}

// --------------------------------------------------------------------------.
// WithTx basic scenarios.
// --------------------------------------------------------------------------.

func TestStore_WithTx_NestedCreateAndRead(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	jobID := newID()
	err := store.WithTx(ctx, testDB.Pool, func(q *store.Queries) error {
		job := baseJob(jobID, "project-withtx-nested")
		return q.CreateJob(ctx, job)
	})
	if err != nil {
		t.Fatalf("WithTx() error = %v", err)
	}

	q := mustStore(t)
	got, err := q.GetJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.ID != jobID {
		t.Fatalf("ID = %q, want %q", got.ID, jobID)
	}
}

// --------------------------------------------------------------------------.
// AdvisoryXactLock.
// --------------------------------------------------------------------------.

func TestStore_AdvisoryXactLock_InTransaction(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	err := store.WithTx(ctx, testDB.Pool, func(q *store.Queries) error {
		return q.AdvisoryXactLock(ctx, int64(777777))
	})
	if err != nil {
		t.Fatalf("AdvisoryXactLock in tx error = %v", err)
	}
}
