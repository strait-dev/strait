package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"
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

	if err := archiver.ArchiveOnceForTest(context.Background()); err != nil {
		t.Fatalf("ArchiveOnceForTest() error = %v", err)
	}
	if store.calls != 1 {
		t.Fatalf("calls = %d, want 1", store.calls)
	}
	if store.older != 2*time.Second {
		t.Fatalf("olderThan = %s, want 2s", store.older)
	}
	if store.limit != 7 {
		t.Fatalf("batchSize = %d, want 7", store.limit)
	}
	if archiver.Archived() != 3 {
		t.Fatalf("Archived() = %d, want 3", archiver.Archived())
	}
	if archiver.Iterations() != 1 {
		t.Fatalf("Iterations() = %d, want 1", archiver.Iterations())
	}
}

func TestOutboxArchiver_ArchiveOnceRecordsErrors(t *testing.T) {
	wantErr := errors.New("archive failed")
	store := &testOutboxArchiveStore{err: wantErr}
	archiver := NewOutboxArchiver(store, OutboxArchiverConfig{})

	err := archiver.ArchiveOnceForTest(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("ArchiveOnceForTest() error = %v, want %v", err, wantErr)
	}
	if archiver.Errors() != 1 {
		t.Fatalf("Errors() = %d, want 1", archiver.Errors())
	}
	if archiver.Iterations() != 1 {
		t.Fatalf("Iterations() = %d, want 1", archiver.Iterations())
	}
}
