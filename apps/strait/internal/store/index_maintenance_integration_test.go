//go:build integration

package store_test

import (
	"context"
	"testing"
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
