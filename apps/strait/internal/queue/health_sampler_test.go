package queue

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHealthSamplerDefaultsAndPreservesOptions(t *testing.T) {
	sampler, err := NewHealthSampler(&healthSamplerFakeDB{}, 0, nil)
	require.NoError(t, err)
	require.Equal(t, 30*time.Second, sampler.interval)
	require.NotNil(t, sampler.logger)

	logger := slog.New(slog.DiscardHandler)
	sampler, err = NewHealthSampler(&healthSamplerFakeDB{}, 5*time.Second, logger)
	require.NoError(t, err)
	require.Equal(t, 5*time.Second, sampler.interval)
	require.Same(t, logger, sampler.logger)
}

func TestHealthSamplerSampleOnceRecoversPanicAndCountsIteration(t *testing.T) {
	metrics, err := Metrics()
	require.NoError(t, err)
	db := &healthSamplerFakeDB{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			panic("partition query panic")
		},
	}
	sampler := &HealthSampler{
		db:      db,
		metrics: metrics,
		logger:  slog.New(slog.DiscardHandler),
	}

	sampler.SampleOnce(context.Background())

	require.EqualValues(t, 1, sampler.Iterations())
}

func TestHealthSamplerSamplePartitionsRecordsRowsAndHandlesErrors(t *testing.T) {
	metrics, err := Metrics()
	require.NoError(t, err)
	ctx := context.Background()

	t.Run("query error", func(t *testing.T) {
		db := &healthSamplerFakeDB{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return nil, errors.New("query failed")
			},
		}
		sampler := &HealthSampler{db: db, metrics: metrics, logger: slog.New(slog.DiscardHandler)}

		sampler.samplePartitions(ctx)

		require.Len(t, db.queries, 1)
	})

	t.Run("scan success scan error and rows error", func(t *testing.T) {
		rowsErr := errors.New("rows failed")
		db := &healthSamplerFakeDB{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &healthSamplerRows{
					rows: []healthSamplerRowScan{
						func(dest ...any) error {
							*dest[0].(*string) = "job_runs"
							*dest[1].(*int64) = 7
							*dest[2].(*int64) = 3
							*dest[3].(*int64) = 5
							*dest[4].(*int64) = 2
							return nil
						},
						func(dest ...any) error {
							*dest[0].(*string) = "job_runs_empty"
							*dest[1].(*int64) = 0
							*dest[2].(*int64) = 0
							*dest[3].(*int64) = 0
							*dest[4].(*int64) = 0
							return nil
						},
						func(...any) error {
							return errors.New("scan failed")
						},
					},
					err: rowsErr,
				}, nil
			},
		}
		sampler := &HealthSampler{db: db, metrics: metrics, logger: slog.New(slog.DiscardHandler)}

		sampler.samplePartitions(ctx)

		require.Len(t, db.queries, 1)
	})
}

func TestHealthSamplerScalarSamplesHandleSuccessAndFailure(t *testing.T) {
	metrics, err := Metrics()
	require.NoError(t, err)
	ctx := context.Background()

	tests := []struct {
		name   string
		sample func(*HealthSampler, context.Context)
		assign func(dest ...any) error
	}{
		{
			name:   "history live tuples",
			sample: (*HealthSampler).sampleHistoryLiveTuples,
			assign: func(dest ...any) error {
				*dest[0].(*int64) = 12
				return nil
			},
		},
		{
			name:   "stranded terminal",
			sample: (*HealthSampler).sampleStrandedTerminal,
			assign: func(dest ...any) error {
				*dest[0].(*int64) = 4
				return nil
			},
		},
		{
			name:   "oldest queued",
			sample: (*HealthSampler).sampleOldestQueued,
			assign: func(dest ...any) error {
				*dest[0].(*float64) = 8.5
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+" success", func(t *testing.T) {
			db := &healthSamplerFakeDB{rows: []healthSamplerFakeRow{{scan: tt.assign}}}
			sampler := &HealthSampler{db: db, metrics: metrics, logger: slog.New(slog.DiscardHandler)}

			tt.sample(sampler, ctx)

			require.Len(t, db.queries, 1)
		})

		t.Run(tt.name+" scan error", func(t *testing.T) {
			db := &healthSamplerFakeDB{rows: []healthSamplerFakeRow{{scan: func(...any) error {
				return errors.New("scan failed")
			}}}}
			sampler := &HealthSampler{db: db, metrics: metrics, logger: slog.New(slog.DiscardHandler)}

			tt.sample(sampler, ctx)

			require.Len(t, db.queries, 1)
		})
	}
}

func TestHealthSamplerQueueDepthByStatusHandlesRows(t *testing.T) {
	metrics, err := Metrics()
	require.NoError(t, err)
	ctx := context.Background()

	t.Run("query error", func(t *testing.T) {
		db := &healthSamplerFakeDB{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return nil, errors.New("query failed")
			},
		}
		sampler := &HealthSampler{db: db, metrics: metrics, logger: slog.New(slog.DiscardHandler)}

		sampler.sampleQueueDepthByStatus(ctx)

		require.Len(t, db.queries, 1)
	})

	t.Run("success and scan error", func(t *testing.T) {
		db := &healthSamplerFakeDB{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &healthSamplerRows{
					rows: []healthSamplerRowScan{
						func(dest ...any) error {
							*dest[0].(*string) = "queued"
							*dest[1].(*int64) = 3
							return nil
						},
						func(...any) error {
							return errors.New("scan failed")
						},
					},
				}, nil
			},
		}
		sampler := &HealthSampler{db: db, metrics: metrics, logger: slog.New(slog.DiscardHandler)}

		sampler.sampleQueueDepthByStatus(ctx)

		require.Len(t, db.queries, 1)
	})
}

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

func TestHealthSamplerUsesCachedPgstatindexUnavailableState(t *testing.T) {
	metrics, err := Metrics()
	require.NoError(t, err)
	db := &healthSamplerFakeDB{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			require.Failf(t, "test failure", "unexpected QueryRow SQL = %q", sql)
			return healthSamplerFakeRow{}
		},
	}
	sampler := &HealthSampler{
		db:      db,
		metrics: metrics,
		logger:  slog.New(slog.DiscardHandler),
	}
	sampler.pgstatindexAvailable.Store(-1)

	require.False(t, sampler.hasPgstatindex(context.Background()))
	require.Empty(t, db.queries)
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

func TestHealthSamplerPgstatindexProbeErrorIsNotCached(t *testing.T) {
	metrics, err := Metrics()
	require.NoError(t, err)
	db := &healthSamplerFakeDB{
		rows: []healthSamplerFakeRow{
			{scan: func(...any) error { return errors.New("probe failed") }},
			{scan: func(dest ...any) error {
				*dest[0].(*bool) = true
				return nil
			}},
			{scan: func(dest ...any) error {
				*dest[0].(*int64) = 5
				return nil
			}},
		},
	}
	sampler := &HealthSampler{
		db:      db,
		metrics: metrics,
		logger:  slog.New(slog.DiscardHandler),
	}

	require.False(t, sampler.hasPgstatindex(context.Background()))
	sampler.sampleIndexHealth(context.Background())

	require.Len(t, db.queries, 3)
	assert.Contains(t, db.queries[0], "to_regprocedure")
	assert.Contains(t, db.queries[1], "to_regprocedure")
	assert.Contains(t, db.queries[2], "FROM pgstatindex")
}

func TestHealthSamplerOutboxClaimHealthHandlesDepthAndScalarPaths(t *testing.T) {
	metrics, err := Metrics()
	require.NoError(t, err)
	ctx := context.Background()

	t.Run("depth query error", func(t *testing.T) {
		db := &healthSamplerFakeDB{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return nil, errors.New("depth failed")
			},
		}
		sampler := &HealthSampler{db: db, metrics: metrics, logger: slog.New(slog.DiscardHandler)}

		sampler.sampleOutboxClaimHealth(ctx)

		require.Len(t, db.queries, 1)
	})

	t.Run("depth rows and scalar successes", func(t *testing.T) {
		db := &healthSamplerFakeDB{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &healthSamplerRows{
					rows: []healthSamplerRowScan{
						func(dest ...any) error {
							*dest[0].(*string) = "ready"
							*dest[1].(*int64) = 2
							return nil
						},
						func(...any) error {
							return errors.New("scan failed")
						},
					},
					err: errors.New("rows failed"),
				}, nil
			},
			rows: []healthSamplerFakeRow{
				{scan: func(dest ...any) error {
					*dest[0].(*float64) = 1.5
					return nil
				}},
				{scan: func(dest ...any) error {
					*dest[0].(*int64) = 6
					return nil
				}},
				{scan: func(dest ...any) error {
					*dest[0].(*int64) = 10
					*dest[1].(*int64) = 4
					return nil
				}},
			},
		}
		sampler := &HealthSampler{db: db, metrics: metrics, logger: slog.New(slog.DiscardHandler)}

		sampler.sampleOutboxClaimHealth(ctx)

		require.Len(t, db.queries, 4)
	})

	t.Run("age expired and table stats errors", func(t *testing.T) {
		db := &healthSamplerFakeDB{
			queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
				return &healthSamplerRows{}, nil
			},
			rows: []healthSamplerFakeRow{
				{scan: func(...any) error { return errors.New("age failed") }},
				{scan: func(...any) error { return errors.New("expired failed") }},
				{scan: func(...any) error { return errors.New("table failed") }},
			},
		}
		sampler := &HealthSampler{db: db, metrics: metrics, logger: slog.New(slog.DiscardHandler)}

		sampler.sampleOutboxClaimHealth(ctx)

		require.Len(t, db.queries, 4)
	})
}

type healthSamplerFakeDB struct {
	queries    []string
	rows       []healthSamplerFakeRow
	queryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (db *healthSamplerFakeDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (db *healthSamplerFakeDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	db.queries = append(db.queries, sql)
	if db.queryFn != nil {
		return db.queryFn(ctx, sql, args...)
	}
	return nil, errors.New("unexpected query")
}

func (db *healthSamplerFakeDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	db.queries = append(db.queries, sql)
	if db.queryRowFn != nil {
		return db.queryRowFn(ctx, sql, args...)
	}
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

type healthSamplerRowScan func(dest ...any) error

type healthSamplerRows struct {
	rows []healthSamplerRowScan
	err  error
	idx  int
}

func (r *healthSamplerRows) Close()                                       {}
func (r *healthSamplerRows) Err() error                                   { return r.err }
func (r *healthSamplerRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *healthSamplerRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *healthSamplerRows) Next() bool {
	if r.idx >= len(r.rows) {
		return false
	}
	r.idx++
	return true
}
func (r *healthSamplerRows) Scan(dest ...any) error {
	return r.rows[r.idx-1](dest...)
}
func (r *healthSamplerRows) Values() ([]any, error) { return nil, nil }
func (r *healthSamplerRows) RawValues() [][]byte    { return nil }
func (r *healthSamplerRows) Conn() *pgx.Conn        { return nil }
