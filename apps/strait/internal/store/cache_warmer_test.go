package store

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
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
	if err, ok := f.errs[sql]; ok {
		return fakeRow{err: err}
	}
	for fragment, err := range f.errs {
		if strings.Contains(sql, fragment) {
			return fakeRow{err: err}
		}
	}
	return fakeRow{}
}

func TestCacheWarmer_WarmExecutesQueries(t *testing.T) {
	t.Parallel()

	fakeDB := &fakeQueryRower{}
	w := &CacheWarmer{db: fakeDB, logger: slog.New(slog.DiscardHandler)}
	require.NoError(t, w.Warm(t.Context()))
	require.Len(t,
		fakeDB.queries,
		4)

}

func TestCacheWarmer_WarmIgnoresNoRows(t *testing.T) {
	t.Parallel()

	query := "SELECT 1 FROM jobs LIMIT 1"
	fakeDB := &fakeQueryRower{errs: map[string]error{query: pgx.ErrNoRows}}
	w := &CacheWarmer{db: fakeDB, logger: slog.New(slog.DiscardHandler)}
	require.NoError(t, w.Warm(t.Context()))

}

func TestCacheWarmer_WarmReturnsError(t *testing.T) {
	t.Parallel()

	query := "COALESCE(s.status, jr.status) = 'queued'"
	fakeDB := &fakeQueryRower{errs: map[string]error{query: errors.New("boom")}}
	w := &CacheWarmer{db: fakeDB, logger: slog.New(slog.DiscardHandler)}

	err := w.Warm(t.Context())
	require.Error(t,
		err)

}
