//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"strait/internal/store"
)

func TestReindexIndexConcurrently(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	// Reindex a known index. projects_pkey should always exist.
	if err := q.ReindexIndexConcurrently(ctx, "projects_pkey"); err != nil {
		t.Fatalf("ReindexIndexConcurrently() error = %v", err)
	}
}

func TestReindexIndexConcurrently_EmptyNameReturnsError(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.ReindexIndexConcurrently(ctx, "")
	if err == nil {
		t.Fatal("ReindexIndexConcurrently(empty) expected error, got nil")
	}
}

func TestReindexIndexConcurrently_MissingIndexReturnsErrIndexNotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.ReindexIndexConcurrently(ctx, "idx_strait_missing_for_security_test")
	if !errors.Is(err, store.ErrIndexNotFound) {
		t.Fatalf("ReindexIndexConcurrently(missing) error = %v, want ErrIndexNotFound", err)
	}
}
