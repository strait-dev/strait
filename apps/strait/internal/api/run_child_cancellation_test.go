package api

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestCancelChildRunsRecursive_PaginatesChildrenForNextDepth(t *testing.T) {
	t.Parallel()

	var canceledBatches [][]string
	childPageCalls := 0
	ms := &APIStoreMock{
		CancelChildRunsByParentIDsFunc: func(_ context.Context, parentIDs []string, _ time.Time, _ string) (int64, error) {
			canceledBatches = append(canceledBatches, slices.Clone(parentIDs))
			if len(canceledBatches) == 1 {
				return 3, nil
			}
			return 0, nil
		},
		ListChildRunsFunc: func(_ context.Context, parentRunID string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
			if parentRunID != "run-parent" {
				return nil, nil
			}
			if limit != childCancelPageLimit {
				t.Fatalf("limit = %d, want %d", limit, childCancelPageLimit)
			}
			childPageCalls++
			switch childPageCalls {
			case 1:
				if cursor != nil {
					t.Fatalf("first cursor = %v, want nil", cursor)
				}
				return []domain.JobRun{
					{ID: "child-1", CreatedAt: time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)},
					{ID: "child-2", CreatedAt: time.Date(2026, 6, 3, 12, 0, 1, 0, time.UTC)},
				}, nil
			case 2:
				if cursor == nil || !cursor.Equal(time.Date(2026, 6, 3, 12, 0, 1, 0, time.UTC)) {
					t.Fatalf("second cursor = %v, want last first-page created_at", cursor)
				}
				return []domain.JobRun{
					{ID: "child-3", CreatedAt: time.Date(2026, 6, 3, 12, 0, 2, 0, time.UTC)},
				}, nil
			default:
				return nil, nil
			}
		},
	}
	srv := &Server{store: ms}

	total := srv.cancelChildRunsRecursive(context.Background(), "run-parent")

	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	wantBatches := [][]string{{"run-parent"}, {"child-1", "child-2", "child-3"}}
	if !slices.EqualFunc(canceledBatches, wantBatches, slices.Equal[[]string]) {
		t.Fatalf("canceled batches = %#v, want %#v", canceledBatches, wantBatches)
	}
}

func TestCancelChildRunsRecursive_StopsOnCancelError(t *testing.T) {
	t.Parallel()

	cancelCalls := 0
	ms := &APIStoreMock{
		CancelChildRunsByParentIDsFunc: func(_ context.Context, _ []string, _ time.Time, _ string) (int64, error) {
			cancelCalls++
			if cancelCalls == 1 {
				return 2, nil
			}
			return 0, errors.New("cancel failed")
		},
		ListChildRunsFunc: func(_ context.Context, _ string, _ int, cursor *time.Time) ([]domain.JobRun, error) {
			if cursor != nil {
				return nil, nil
			}
			return []domain.JobRun{{ID: "child-1", CreatedAt: time.Now()}}, nil
		},
	}
	srv := &Server{store: ms}

	total := srv.cancelChildRunsRecursive(context.Background(), "run-parent")

	if total != 2 {
		t.Fatalf("total = %d, want already-canceled count 2", total)
	}
	if cancelCalls != 2 {
		t.Fatalf("cancel calls = %d, want 2", cancelCalls)
	}
}

func TestCancelChildRunsRecursive_StopsAtDepthLimit(t *testing.T) {
	t.Parallel()

	cancelCalls := 0
	ms := &APIStoreMock{
		CancelChildRunsByParentIDsFunc: func(_ context.Context, _ []string, _ time.Time, _ string) (int64, error) {
			cancelCalls++
			return 1, nil
		},
		ListChildRunsFunc: func(_ context.Context, _ string, _ int, cursor *time.Time) ([]domain.JobRun, error) {
			if cursor != nil {
				return nil, nil
			}
			return []domain.JobRun{{ID: "next-child", CreatedAt: time.Now()}}, nil
		},
	}
	srv := &Server{store: ms}

	total := srv.cancelChildRunsRecursive(context.Background(), "run-parent")

	if total != int64(maxCancelDepth) {
		t.Fatalf("total = %d, want %d", total, maxCancelDepth)
	}
	if cancelCalls != maxCancelDepth {
		t.Fatalf("cancel calls = %d, want %d", cancelCalls, maxCancelDepth)
	}
}

func TestNextChildCancellationParents_StopsParentOnListError(t *testing.T) {
	t.Parallel()

	var listedParents []string
	ms := &APIStoreMock{
		ListChildRunsFunc: func(_ context.Context, parentRunID string, _ int, cursor *time.Time) ([]domain.JobRun, error) {
			listedParents = append(listedParents, parentRunID)
			if parentRunID == "parent-a" {
				return nil, errors.New("list failed")
			}
			if cursor != nil {
				return nil, nil
			}
			return []domain.JobRun{{ID: "child-b", CreatedAt: time.Now()}}, nil
		},
	}
	srv := &Server{store: ms}

	nextParents := srv.nextChildCancellationParents(context.Background(), []string{"parent-a", "parent-b"})

	if !slices.Equal(nextParents, []string{"child-b"}) {
		t.Fatalf("next parents = %#v, want child-b", nextParents)
	}
	if !slices.Equal(listedParents, []string{"parent-a", "parent-b", "parent-b"}) {
		t.Fatalf("listed parents = %#v, want parent-a then parent-b page traversal", listedParents)
	}
}
