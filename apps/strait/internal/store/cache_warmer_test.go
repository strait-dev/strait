package store

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
)

type fakeRow struct {
	err error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) > 0 {
		if v, ok := dest[0].(*int64); ok {
			*v = 1
		}
	}
	return nil
}

type fakeQueryRower struct {
	queries []string
	errs    map[string]error
}

func (f *fakeQueryRower) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	f.queries = append(f.queries, sql)
	return fakeRow{err: f.errs[sql]}
}

func TestCacheWarmer_WarmExecutesQueries(t *testing.T) {
	t.Parallel()

	fakeDB := &fakeQueryRower{}
	w := &CacheWarmer{db: fakeDB, logger: slog.New(slog.DiscardHandler)}

	if err := w.Warm(t.Context()); err != nil {
		t.Fatalf("warm cache: %v", err)
	}

	if len(fakeDB.queries) != 4 {
		t.Fatalf("executed queries = %d, want 4", len(fakeDB.queries))
	}
}

func TestCacheWarmer_WarmIgnoresNoRows(t *testing.T) {
	t.Parallel()

	query := "SELECT 1 FROM jobs LIMIT 1"
	fakeDB := &fakeQueryRower{errs: map[string]error{query: pgx.ErrNoRows}}
	w := &CacheWarmer{db: fakeDB, logger: slog.New(slog.DiscardHandler)}

	if err := w.Warm(t.Context()); err != nil {
		t.Fatalf("warm cache with no rows: %v", err)
	}
}

func TestCacheWarmer_WarmReturnsError(t *testing.T) {
	t.Parallel()

	query := "SELECT COUNT(*) FROM job_runs WHERE status = 'queued'"
	fakeDB := &fakeQueryRower{errs: map[string]error{query: errors.New("boom")}}
	w := &CacheWarmer{db: fakeDB, logger: slog.New(slog.DiscardHandler)}

	err := w.Warm(t.Context())
	if err == nil {
		t.Fatal("expected error")
	}
}
