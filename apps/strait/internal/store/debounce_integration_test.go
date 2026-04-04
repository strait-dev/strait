//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestUpsertDebouncePending(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-debounce-upsert")

	d := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "key-1",
		Payload:     json.RawMessage(`{"hello":"world"}`),
		Priority:    5,
		TriggeredBy: "api",
		FireAt:      time.Now().UTC().Add(30 * time.Second),
	}
	if err := q.UpsertDebouncePending(ctx, d); err != nil {
		t.Fatalf("UpsertDebouncePending() error = %v", err)
	}
	if d.ID == "" {
		t.Fatal("UpsertDebouncePending() did not set ID")
	}
	if d.CreatedAt.IsZero() {
		t.Fatal("UpsertDebouncePending() did not set CreatedAt")
	}
}

func TestUpsertDebouncePending_UpsertReplaces(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-debounce-replace")

	d1 := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "replace-key",
		Payload:     json.RawMessage(`{"v":1}`),
		Priority:    1,
		TriggeredBy: "api",
		FireAt:      time.Now().UTC().Add(30 * time.Second),
	}
	if err := q.UpsertDebouncePending(ctx, d1); err != nil {
		t.Fatalf("UpsertDebouncePending(1) error = %v", err)
	}

	d2 := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "replace-key",
		Payload:     json.RawMessage(`{"v":2}`),
		Priority:    10,
		TriggeredBy: "api",
		FireAt:      time.Now().UTC().Add(60 * time.Second),
	}
	if err := q.UpsertDebouncePending(ctx, d2); err != nil {
		t.Fatalf("UpsertDebouncePending(2) error = %v", err)
	}

	// Should have only one row (replaced).
}

func TestListDueDebouncePending(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-debounce-list-due")

	// Create a due debounce (fire_at in the past).
	due := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "due-key",
		Payload:     json.RawMessage(`{}`),
		TriggeredBy: "api",
		FireAt:      time.Now().UTC().Add(-1 * time.Minute),
	}
	if err := q.UpsertDebouncePending(ctx, due); err != nil {
		t.Fatalf("UpsertDebouncePending(due) error = %v", err)
	}

	// Create a future debounce.
	future := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "future-key",
		Payload:     json.RawMessage(`{}`),
		TriggeredBy: "api",
		FireAt:      time.Now().UTC().Add(10 * time.Minute),
	}
	if err := q.UpsertDebouncePending(ctx, future); err != nil {
		t.Fatalf("UpsertDebouncePending(future) error = %v", err)
	}

	items, err := q.ListDueDebouncePending(ctx)
	if err != nil {
		t.Fatalf("ListDueDebouncePending() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListDueDebouncePending() len = %d, want 1", len(items))
	}
	if items[0].DebounceKey != "due-key" {
		t.Fatalf("ListDueDebouncePending() key = %q, want due-key", items[0].DebounceKey)
	}
}

func TestDeleteDebouncePending(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	job := mustCreateJob(t, ctx, q, "project-debounce-delete")

	d := &domain.DebouncePending{
		JobID:       job.ID,
		ProjectID:   job.ProjectID,
		DebounceKey: "delete-key",
		Payload:     json.RawMessage(`{}`),
		TriggeredBy: "api",
		FireAt:      time.Now().UTC().Add(-1 * time.Minute),
	}
	if err := q.UpsertDebouncePending(ctx, d); err != nil {
		t.Fatalf("UpsertDebouncePending() error = %v", err)
	}

	if err := q.DeleteDebouncePending(ctx, d.ID); err != nil {
		t.Fatalf("DeleteDebouncePending() error = %v", err)
	}

	// Verify it was deleted by listing due items.
	items, err := q.ListDueDebouncePending(ctx)
	if err != nil {
		t.Fatalf("ListDueDebouncePending() error = %v", err)
	}
	for _, item := range items {
		if item.ID == d.ID {
			t.Fatalf("DeleteDebouncePending() did not delete the item")
		}
	}
}
