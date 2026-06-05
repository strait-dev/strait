package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
	require.NoError(t, runBackfillHistoryWithStore(context.Background(), store,
		shortRetention, longRetention, 100,
		false))
	require.EqualValues(t, 2, store.
		archiveCalls,
	)
	require.Equal(
		t, shortRetention,

		store.
			archiveShortRetention)
	require.Equal(
		t, longRetention,
		store.
			archiveLongRetention)
	require.EqualValues(t, 100, store.
		archiveBatchSize,
	)

}

func TestRunBackfillHistoryWithStore_DryRunUsesSameRetentions(t *testing.T) {
	t.Parallel()

	shortRetention := 30 * 24 * time.Hour
	longRetention := 90 * 24 * time.Hour
	store := &mockBackfillHistoryStore{}
	require.NoError(t, runBackfillHistoryWithStore(context.Background(), store,
		shortRetention, longRetention, 100,
		true))
	require.EqualValues(t, 0, store.
		archiveCalls,
	)
	require.Equal(
		t, shortRetention,

		store.
			countShortRetention)
	require.Equal(
		t, longRetention,
		store.
			countLongRetention)

}
