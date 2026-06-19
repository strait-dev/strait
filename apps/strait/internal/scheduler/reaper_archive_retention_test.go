package scheduler

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type archiveTerminalCall struct {
	shortRetention time.Duration
	longRetention  time.Duration
	batchSize      int
}

type retentionCutoffCall struct {
	cutoff    time.Time
	batchSize int
}

type consumedOutboxCall struct {
	olderThan time.Duration
	batchSize int
}

type recordingArchiveRetentionStore struct {
	*mockReaperStore

	archiveTerminalCalls []archiveTerminalCall
	historyCalls         []retentionCutoffCall
	archiveOutboxCalls   []consumedOutboxCall
	outboxHistoryCalls   []retentionCutoffCall
	quarantinedCalls     []retentionCutoffCall

	archiveTerminalRows int64
	historyRows         int64
	archiveOutboxRows   int64
	outboxHistoryRows   int64
	quarantinedRows     int64

	archiveTerminalErr error
	historyErr         error
	archiveOutboxErr   error
	outboxHistoryErr   error
	quarantinedErr     error
}

func (s *recordingArchiveRetentionStore) ArchiveTerminalRunsPastRetention(_ context.Context, shortRetention, longRetention time.Duration, batchSize int) (int64, error) {
	s.archiveTerminalCalls = append(s.archiveTerminalCalls, archiveTerminalCall{
		shortRetention: shortRetention,
		longRetention:  longRetention,
		batchSize:      batchSize,
	})
	return s.archiveTerminalRows, s.archiveTerminalErr
}

func (s *recordingArchiveRetentionStore) DeleteHistoryRunsPastRetention(_ context.Context, cutoff time.Time, batchSize int) (int64, error) {
	s.historyCalls = append(s.historyCalls, retentionCutoffCall{cutoff: cutoff, batchSize: batchSize})
	return s.historyRows, s.historyErr
}

func (s *recordingArchiveRetentionStore) ArchiveConsumedOutboxBatch(_ context.Context, olderThan time.Duration, batchSize int) (int64, error) {
	s.archiveOutboxCalls = append(s.archiveOutboxCalls, consumedOutboxCall{olderThan: olderThan, batchSize: batchSize})
	return s.archiveOutboxRows, s.archiveOutboxErr
}

func (s *recordingArchiveRetentionStore) DeleteOutboxHistoryPastRetention(_ context.Context, cutoff time.Time, batchSize int) (int64, error) {
	s.outboxHistoryCalls = append(s.outboxHistoryCalls, retentionCutoffCall{cutoff: cutoff, batchSize: batchSize})
	return s.outboxHistoryRows, s.outboxHistoryErr
}

func (s *recordingArchiveRetentionStore) PurgeQuarantinedOutboxOlderThan(_ context.Context, cutoff time.Time, batchSize int) (int64, error) {
	s.quarantinedCalls = append(s.quarantinedCalls, retentionCutoffCall{cutoff: cutoff, batchSize: batchSize})
	return s.quarantinedRows, s.quarantinedErr
}

func TestReaper_ArchiveRetentionUsesDefaultBatchAndRetentionWindows(t *testing.T) {
	t.Parallel()

	shortRetention := 7 * 24 * time.Hour
	longRetention := 30 * 24 * time.Hour
	store := &recordingArchiveRetentionStore{
		mockReaperStore:     &mockReaperStore{},
		archiveTerminalRows: 1,
		historyRows:         2,
		archiveOutboxRows:   3,
		outboxHistoryRows:   4,
		quarantinedRows:     5,
	}
	reaper := NewReaper(store, time.Second, 30*time.Second, shortRetention, longRetention, true, nil).
		WithArchiveEnabled(true)
	reaper.deleteBatchLimit = 0

	before := time.Now()
	reaper.reapTerminalRetention(context.Background())
	after := time.Now()

	require.Equal(t, []archiveTerminalCall{{
		shortRetention: shortRetention,
		longRetention:  longRetention,
		batchSize:      100,
	}}, store.archiveTerminalCalls)
	require.Equal(t, []consumedOutboxCall{{
		olderThan: shortRetention,
		batchSize: 100,
	}}, store.archiveOutboxCalls)
	require.Len(t, store.historyCalls, 1)
	require.Len(t, store.outboxHistoryCalls, 1)
	require.Len(t, store.quarantinedCalls, 1)

	for _, call := range []retentionCutoffCall{
		store.historyCalls[0],
		store.outboxHistoryCalls[0],
		store.quarantinedCalls[0],
	} {
		require.Equal(t, 100, call.batchSize)
		require.False(t, call.cutoff.Before(before.Add(-longRetention)))
		require.False(t, call.cutoff.After(after.Add(-longRetention)))
	}
}

func TestReaper_ArchiveRetentionRecordsEachStoreError(t *testing.T) {
	t.Parallel()

	errArchiveTerminal := errors.New("archive terminal failed")
	errHistory := errors.New("history failed")
	errArchiveOutbox := errors.New("archive outbox failed")
	errOutboxHistory := errors.New("outbox history failed")
	errQuarantined := errors.New("quarantined failed")
	store := &recordingArchiveRetentionStore{
		mockReaperStore:    &mockReaperStore{},
		archiveTerminalErr: errArchiveTerminal,
		historyErr:         errHistory,
		archiveOutboxErr:   errArchiveOutbox,
		outboxHistoryErr:   errOutboxHistory,
		quarantinedErr:     errQuarantined,
	}
	reaper := NewReaper(store, time.Second, 30*time.Second, time.Hour, 2*time.Hour, true, nil).
		WithArchiveEnabled(true).
		WithDeleteBatchSize(7)

	reaper.reapTerminalRetention(context.Background())

	require.Len(t, store.archiveTerminalCalls, 1)
	require.Equal(t, 7, store.archiveTerminalCalls[0].batchSize)
	require.Len(t, store.archiveOutboxCalls, 1)
	require.Equal(t, 7, store.archiveOutboxCalls[0].batchSize)
	require.Len(t, store.quarantinedCalls, 1)
	require.Equal(t, 7, store.quarantinedCalls[0].batchSize)
	require.Len(t, store.historyCalls, 1)
	require.Equal(t, 7, store.historyCalls[0].batchSize)
	require.Len(t, store.outboxHistoryCalls, 1)
	require.Equal(t, 7, store.outboxHistoryCalls[0].batchSize)
}

func TestReaper_ReapHistoryRetentionErrorDoesNotReportDeletedRows(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	store := &recordingArchiveRetentionStore{
		mockReaperStore: &mockReaperStore{},
		historyRows:     9,
		historyErr:      errors.New("history delete failed"),
	}
	reaper := NewReaper(store, time.Second, 30*time.Second, time.Hour, 2*time.Hour, true, nil).
		WithDeleteBatchSize(7)

	reaper.reapHistoryRetention(context.Background())

	require.Len(t, store.historyCalls, 1)
	require.Contains(t, logs.String(), "failed to delete history runs past retention")
	require.NotContains(t, logs.String(), "deleted history runs past retention")
}
