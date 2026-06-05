package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type testOutboxArchiveStore struct {
	archived int64
	err      error
	calls    int
	older    time.Duration
	limit    int
}

func (s *testOutboxArchiveStore) ArchiveConsumedOutboxBatch(_ context.Context, olderThan time.Duration, batchSize int) (int64, error) {
	s.calls++
	s.older = olderThan
	s.limit = batchSize
	if s.err != nil {
		return 0, s.err
	}
	return s.archived, nil
}

func TestOutboxArchiver_ArchiveOnceRecordsProgress(t *testing.T) {
	store := &testOutboxArchiveStore{archived: 3}
	archiver := NewOutboxArchiver(store, OutboxArchiverConfig{
		OlderThan: 2 * time.Second,
		BatchSize: 7,
	})
	require.NoError(t,
		archiver.ArchiveOnceForTest(context.
			Background()))
	require.Equal(t, 1,
		store.calls,
	)
	require.Equal(t, 2*
		time.Second,
		store.
			older)
	require.Equal(t, 7,
		store.limit,
	)
	require.EqualValues(t, 3,
		archiver.
			Archived())
	require.EqualValues(t, 1,
		archiver.
			Iterations())
}

func TestOutboxArchiver_ArchiveOnceRecordsErrors(t *testing.T) {
	wantErr := errors.New("archive failed")
	store := &testOutboxArchiveStore{err: wantErr}
	archiver := NewOutboxArchiver(store, OutboxArchiverConfig{})

	err := archiver.ArchiveOnceForTest(context.Background())
	require.ErrorIs(t, err, wantErr)
	require.EqualValues(t, 1,
		archiver.
			Errors())
	require.EqualValues(t, 1,
		archiver.
			Iterations())
}
