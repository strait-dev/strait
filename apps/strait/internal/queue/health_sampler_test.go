package queue

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthSamplerSkipsPgstatindexWhenExtensionUnavailable(t *testing.T) {
	metrics, err := Metrics()
	require.NoError(t, err)

	db := &healthSamplerFakeDB{
		rows: []healthSamplerFakeRow{
			{scan: func(dest ...any) error {
				*dest[0].(*bool) = false
				return nil
			}},
		},
	}
	sampler := &HealthSampler{
		db:      db,
		metrics: metrics,
		logger:  slog.New(slog.DiscardHandler),
	}

	sampler.sampleIndexHealth(context.Background())
	sampler.sampleIndexHealth(context.Background())

	require.Len(t, db.queries, 1)
	assert.Contains(t, db.queries[0], "to_regprocedure")
	assert.NotContains(t, strings.Join(db.queries, "\n"), "FROM pgstatindex")
}

func TestHealthSamplerCachesAvailablePgstatindex(t *testing.T) {
	metrics, err := Metrics()
	require.NoError(t, err)

	db := &healthSamplerFakeDB{
		rows: []healthSamplerFakeRow{
			{scan: func(dest ...any) error {
				*dest[0].(*bool) = true
				return nil
			}},
			{scan: func(dest ...any) error {
				*dest[0].(*int64) = 7
				return nil
			}},
			{scan: func(dest ...any) error {
				*dest[0].(*int64) = 9
				return nil
			}},
		},
	}
	sampler := &HealthSampler{
		db:      db,
		metrics: metrics,
		logger:  slog.New(slog.DiscardHandler),
	}

	sampler.sampleIndexHealth(context.Background())
	sampler.sampleIndexHealth(context.Background())

	require.Len(t, db.queries, 3)
	assert.Contains(t, db.queries[0], "to_regprocedure")
	assert.Contains(t, db.queries[1], "FROM pgstatindex")
	assert.Contains(t, db.queries[2], "FROM pgstatindex")
}

type healthSamplerFakeDB struct {
	queries []string
	rows    []healthSamplerFakeRow
}

func (db *healthSamplerFakeDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (db *healthSamplerFakeDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unexpected query")
}

func (db *healthSamplerFakeDB) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	db.queries = append(db.queries, sql)
	if len(db.rows) == 0 {
		return healthSamplerFakeRow{scan: func(...any) error {
			return errors.New("unexpected query row")
		}}
	}
	row := db.rows[0]
	db.rows = db.rows[1:]
	return row
}

type healthSamplerFakeRow struct {
	scan func(dest ...any) error
}

func (r healthSamplerFakeRow) Scan(dest ...any) error {
	return r.scan(dest...)
}
