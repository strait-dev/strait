//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
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
	require.NoError(t, q.CreateLogDrain(ctx, drain))

	return drain
}

func TestDeleteLogDrain_RemovesRow(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-drain-del-" + newID()
	drain := createDrainForDelete(t, ctx, q, projectID, "deleteme")
	require.NoError(t, q.DeleteLogDrain(ctx, drain.
		ID, projectID,
	))

	if _, err := q.GetLogDrain(ctx, drain.ID, projectID); !errors.Is(err, store.ErrLogDrainNotFound) {
		require.Failf(t, "test failure",

			"GetLogDrain after delete: err = %v, want ErrLogDrainNotFound", err)
	}
}

func TestDeleteLogDrain_NotFound_ReturnsErr(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.DeleteLogDrain(ctx, "drain-does-not-exist", "proj-"+newID())
	require.True(t, errors.Is(err, store.
		ErrLogDrainNotFound,
	))

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
		require.Failf(t, "test failure",

			"cross-tenant delete: err = %v, want ErrLogDrainNotFound", err)
	}

	// A's drain must still be there.
	got, err := q.GetLogDrain(ctx, drain.ID, projectA)
	require.NoError(t, err)
	require.Equal(t, drain.
		ID,
		got.ID,
	)

}
