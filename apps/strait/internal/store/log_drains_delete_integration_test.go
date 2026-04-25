//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

// Integration tests for DeleteLogDrain, which had zero direct coverage
// despite being a destructive mutation. log_drains stores outbound auth
// tokens, so a silent delete-the-wrong-row bug would be a credential-
// exposure regression.

func createDrainForDelete(t *testing.T, ctx context.Context, q *store.Queries, projectID, name string) *domain.LogDrain {
	t.Helper()
	drain := &domain.LogDrain{
		ID:          "drain-" + newID(),
		ProjectID:   projectID,
		Name:        name,
		DrainType:   "http",
		EndpointURL: "https://" + name + ".example.com/log",
		AuthType:    "none",
		Enabled:     true,
	}
	if err := q.CreateLogDrain(ctx, drain); err != nil {
		t.Fatalf("CreateLogDrain: %v", err)
	}
	return drain
}

func TestDeleteLogDrain_RemovesRow(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-drain-del-" + newID()
	drain := createDrainForDelete(t, ctx, q, projectID, "deleteme")

	if err := q.DeleteLogDrain(ctx, drain.ID, projectID); err != nil {
		t.Fatalf("DeleteLogDrain: %v", err)
	}

	if _, err := q.GetLogDrain(ctx, drain.ID, projectID); !errors.Is(err, store.ErrLogDrainNotFound) {
		t.Fatalf("GetLogDrain after delete: err = %v, want ErrLogDrainNotFound", err)
	}
}

func TestDeleteLogDrain_NotFound_ReturnsErr(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.DeleteLogDrain(ctx, "drain-does-not-exist", "proj-"+newID())
	if !errors.Is(err, store.ErrLogDrainNotFound) {
		t.Fatalf("err = %v, want ErrLogDrainNotFound", err)
	}
}

func TestDeleteLogDrain_WrongProject_NoOp(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectA := "proj-drain-a-" + newID()
	projectB := "proj-drain-b-" + newID()

	drain := createDrainForDelete(t, ctx, q, projectA, "a-drain")

	// Attempt to delete A's drain while asserting project = B.
	// Must return not-found without actually deleting anything.
	if err := q.DeleteLogDrain(ctx, drain.ID, projectB); !errors.Is(err, store.ErrLogDrainNotFound) {
		t.Fatalf("cross-tenant delete: err = %v, want ErrLogDrainNotFound", err)
	}

	// A's drain must still be there.
	got, err := q.GetLogDrain(ctx, drain.ID, projectA)
	if err != nil {
		t.Fatalf("GetLogDrain after cross-tenant delete attempt: %v", err)
	}
	if got.ID != drain.ID {
		t.Fatalf("drain ID = %q, want %q", got.ID, drain.ID)
	}
}
