//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestReindexIndexConcurrently(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)
	require.NoError(t, q.ReindexIndexConcurrently(ctx, "projects_pkey"))

	// Reindex a known index. projects_pkey should always exist.

}

func TestReindexIndexConcurrently_EmptyNameReturnsError(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.ReindexIndexConcurrently(ctx, "")
	require.Error(t, err)

}

func TestReindexIndexConcurrently_MissingIndexReturnsErrIndexNotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.ReindexIndexConcurrently(ctx, "idx_strait_missing_for_security_test")
	require.True(t, errors.Is(err, store.
		ErrIndexNotFound,
	),
	)

}
