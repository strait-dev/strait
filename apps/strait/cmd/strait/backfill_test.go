package main

import (
	"context"
	"testing"
	"time"
)

type mockBackfillHistoryStore struct {
	countShortRetention   time.Duration
	countLongRetention    time.Duration
	archiveShortRetention time.Duration
	archiveLongRetention  time.Duration
	archiveBatchSize      int
	archiveCalls          int
	moved                 []int64
}

func (m *mockBackfillHistoryStore) CountStrandedTerminalRuns(_ context.Context, shortRetention, longRetention time.Duration) (int64, error) {
	m.countShortRetention = shortRetention
	m.countLongRetention = longRetention
	return 0, nil
}

func (m *mockBackfillHistoryStore) ArchiveTerminalRunsPastRetention(_ context.Context, shortRetention, longRetention time.Duration, batchSize int) (int64, error) {
	m.archiveCalls++
	m.archiveShortRetention = shortRetention
	m.archiveLongRetention = longRetention
	m.archiveBatchSize = batchSize
	if len(m.moved) == 0 {
		return 0, nil
	}
	moved := m.moved[0]
	m.moved = m.moved[1:]
	return moved, nil
}

func (m *mockBackfillHistoryStore) CountDuplicateHistoryRuns(context.Context) (int64, error) {
	return 0, nil
}

func TestRunBackfillHistoryWithStore_ExecutionUsesRetentionAwareArchive(t *testing.T) {
	t.Parallel()

	shortRetention := 30 * 24 * time.Hour
	longRetention := 90 * 24 * time.Hour
	store := &mockBackfillHistoryStore{moved: []int64{100, 0}}

	if err := runBackfillHistoryWithStore(context.Background(), store, shortRetention, longRetention, 100, false); err != nil {
		t.Fatalf("runBackfillHistoryWithStore returned error: %v", err)
	}
	if store.archiveCalls != 2 {
		t.Fatalf("archive calls = %d, want 2", store.archiveCalls)
	}
	if store.archiveShortRetention != shortRetention {
		t.Fatalf("short retention = %s, want %s", store.archiveShortRetention, shortRetention)
	}
	if store.archiveLongRetention != longRetention {
		t.Fatalf("long retention = %s, want %s", store.archiveLongRetention, longRetention)
	}
	if store.archiveBatchSize != 100 {
		t.Fatalf("batch size = %d, want 100", store.archiveBatchSize)
	}
}

func TestRunBackfillHistoryWithStore_DryRunUsesSameRetentions(t *testing.T) {
	t.Parallel()

	shortRetention := 30 * 24 * time.Hour
	longRetention := 90 * 24 * time.Hour
	store := &mockBackfillHistoryStore{}

	if err := runBackfillHistoryWithStore(context.Background(), store, shortRetention, longRetention, 100, true); err != nil {
		t.Fatalf("runBackfillHistoryWithStore returned error: %v", err)
	}
	if store.archiveCalls != 0 {
		t.Fatalf("dry run archive calls = %d, want 0", store.archiveCalls)
	}
	if store.countShortRetention != shortRetention {
		t.Fatalf("count short retention = %s, want %s", store.countShortRetention, shortRetention)
	}
	if store.countLongRetention != longRetention {
		t.Fatalf("count long retention = %s, want %s", store.countLongRetention, longRetention)
	}
}
