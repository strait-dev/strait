package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestReaper_ReapSingletonLocks_ReleasesAndPromotes verifies the reaper releases
// every reapable holder it is handed.
func TestReaper_ReapSingletonLocks_ReleasesAndPromotes(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	released := []string{}
	ms := &mockReaperStore{
		listReapableSingletonHoldersFn: func(_ context.Context) ([]string, error) {
			return []string{"holder-1", "holder-2"}, nil
		},
		releaseSingletonAndPromoteFn: func(_ context.Context, holderRunID string) (bool, string, error) {
			mu.Lock()
			defer mu.Unlock()
			released = append(released, holderRunID)
			return true, "promoted-" + holderRunID, nil
		},
	}
	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapSingletonLocks(context.Background())

	if len(released) != 2 {
		t.Fatalf("expected 2 releases, got %d: %v", len(released), released)
	}
}

// TestReaper_ReapSingletonLocks_ListError_NoPanic: a failed listing aborts the
// cycle without panicking and without attempting any release.
func TestReaper_ReapSingletonLocks_ListError_NoPanic(t *testing.T) {
	t.Parallel()

	called := false
	ms := &mockReaperStore{
		listReapableSingletonHoldersFn: func(_ context.Context) ([]string, error) {
			return nil, errors.New("db down")
		},
		releaseSingletonAndPromoteFn: func(_ context.Context, _ string) (bool, string, error) {
			called = true
			return false, "", nil
		},
	}
	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapSingletonLocks(context.Background())

	if called {
		t.Fatal("release must not be called when listing errors")
	}
}

// TestReaper_ReapSingletonLocks_ReleaseError_ContinuesNextHolder: a release error
// for one holder must not stop the reaper from reaping the rest.
func TestReaper_ReapSingletonLocks_ReleaseError_ContinuesNextHolder(t *testing.T) {
	t.Parallel()

	var attempts []string
	ms := &mockReaperStore{
		listReapableSingletonHoldersFn: func(_ context.Context) ([]string, error) {
			return []string{"bad", "good"}, nil
		},
		releaseSingletonAndPromoteFn: func(_ context.Context, holderRunID string) (bool, string, error) {
			attempts = append(attempts, holderRunID)
			if holderRunID == "bad" {
				return false, "", errors.New("release failed")
			}
			return true, "promoted", nil
		},
	}
	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	r.reapSingletonLocks(context.Background())

	if len(attempts) != 2 {
		t.Fatalf("expected both holders attempted despite the first error, got %v", attempts)
	}
}

// TestReaper_ReapSingletonLocks_LostRace_NoPromotion: when another releaser won
// the race (released=false), the reaper treats it as a no-op for that holder.
func TestReaper_ReapSingletonLocks_LostRace_NoPromotion(t *testing.T) {
	t.Parallel()

	ms := &mockReaperStore{
		listReapableSingletonHoldersFn: func(_ context.Context) ([]string, error) {
			return []string{"holder-1"}, nil
		},
		releaseSingletonAndPromoteFn: func(_ context.Context, _ string) (bool, string, error) {
			return false, "", nil // someone else already released it
		},
	}
	r := NewReaper(ms, time.Second, 30*time.Second, 0, 0, false, nil)
	// Must not panic and must complete cleanly.
	r.reapSingletonLocks(context.Background())
}
