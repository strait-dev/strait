package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type watchdogPool struct {
	activityRows pgx.Rows
	activityErr  error
	slotRows     pgx.Rows
	slotErr      error
}

func (p *watchdogPool) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if containsQueryTable(sql, "pg_stat_activity") {
		return p.activityRows, p.activityErr
	}
	return p.slotRows, p.slotErr
}

func containsQueryTable(sql, table string) bool {
	return strings.Contains(sql, table)
}

type watchdogRows struct {
	rows    [][]any
	idx     int
	scanErr error
	err     error
	closed  bool
}

func (r *watchdogRows) Close() {
	r.closed = true
}

func (r *watchdogRows) Err() error {
	return r.err
}

func (r *watchdogRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

func (r *watchdogRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (r *watchdogRows) Next() bool {
	if r.idx >= len(r.rows) {
		r.Close()
		return false
	}
	r.idx++
	return true
}

func (r *watchdogRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	if r.idx == 0 || r.idx > len(r.rows) {
		return errors.New("scan without row")
	}
	row := r.rows[r.idx-1]
	if len(dest) != len(row) {
		return errors.New("scan destination count mismatch")
	}
	for i, value := range row {
		switch d := dest[i].(type) {
		case *int32:
			*d = value.(int32)
		case *string:
			*d = value.(string)
		case *float64:
			*d = value.(float64)
		case *int64:
			*d = value.(int64)
		case *bool:
			*d = value.(bool)
		default:
			return errors.New("unsupported scan destination")
		}
	}
	return nil
}

func (r *watchdogRows) Values() ([]any, error) {
	if r.idx == 0 || r.idx > len(r.rows) {
		return nil, errors.New("values without row")
	}
	return r.rows[r.idx-1], nil
}

func (r *watchdogRows) RawValues() [][]byte {
	return nil
}

func (r *watchdogRows) Conn() *pgx.Conn {
	return nil
}

func TestDBWatchdogUnitDefaultsAndSample(t *testing.T) {
	t.Parallel()

	pool := &watchdogPool{
		activityRows: &watchdogRows{rows: [][]any{
			{int32(10), "api", "active", 120.0, 3.0, int64(42), "SELECT 1"},
			{int32(11), "worker", "idle in transaction", 10.0, 1.0, int64(99), "BEGIN"},
		}},
		slotRows: &watchdogRows{rows: [][]any{
			{"slot-a", false, int64(123)},
			{"slot-b", true, int64(456)},
		}},
	}
	watchdog, err := NewDBWatchdog(pool, 0, 0, nil)
	require.NoError(t, err)
	require.Equal(t, 15*time.Second, watchdog.interval)
	require.Equal(t, 60*time.Second, watchdog.alertThreshold)
	require.NotNil(t, watchdog.logger)

	watchdog.sampleOnce(context.Background())

	require.EqualValues(t, 1, watchdog.SampleCount())
	require.EqualValues(t, 1, watchdog.AlertCount())
	samples := watchdog.LastSamples()
	require.Len(t, samples, 2)
	require.Equal(t, 120*time.Second, samples[0].TxnStartAge)
	require.Equal(t, 3*time.Second, samples[0].QueryStartAge)
	require.EqualValues(t, 99, samples[1].BackendXminAge)
}

func TestDBWatchdogUnitQueryAndScanFailures(t *testing.T) {
	t.Parallel()

	queryErr := errors.New("query failed")
	watchdog, err := NewDBWatchdog(&watchdogPool{activityErr: queryErr}, time.Millisecond, time.Second, slog.Default())
	require.NoError(t, err)
	watchdog.sampleOnce(context.Background())
	require.EqualValues(t, 1, watchdog.SampleCount())
	require.Empty(t, watchdog.LastSamples())

	watchdog.pool = &watchdogPool{
		activityRows: &watchdogRows{rows: [][]any{{int32(1), "api", "active", 1.0, 1.0, int64(1), "SELECT"}}, scanErr: errors.New("scan failed"), err: errors.New("rows failed")},
		slotRows:     &watchdogRows{scanErr: errors.New("slot scan failed"), rows: [][]any{{"slot", false, int64(1)}}, err: errors.New("slot rows failed")},
	}
	watchdog.sampleOnce(context.Background())
	require.EqualValues(t, 2, watchdog.SampleCount())
	require.Empty(t, watchdog.LastSamples())
}

func TestPoolSamplerDefaultsAndRun(t *testing.T) {
	t.Parallel()

	sampler, err := NewPoolSampler(nil, 0, nil)
	require.NoError(t, err)
	require.Equal(t, 15*time.Second, sampler.interval)
	require.NotNil(t, sampler.logger)

	cfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/db?sslmode=disable")
	require.NoError(t, err)
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	sampler.pool = pool
	sampler.interval = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		sampler.Run(ctx)
	}()
	time.Sleep(5 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "pool sampler did not stop")
	}
}

func TestTelemetryMutationHelpers(t *testing.T) {
	t.Parallel()

	require.Equal(t, "success", redisCommandOutcome(nil))
	require.Equal(t, "miss", redisCommandOutcome(redis.Nil))
	require.Equal(t, "error", redisCommandOutcome(errors.New("boom")))

	shutdown, err := InitSentry(SentryConfig{DSN: "://bad-dsn"})
	require.Error(t, err)
	require.Nil(t, shutdown)
	require.NoError(t, eventError(nil, nil))
	require.EqualError(t, eventError(&sentry.Event{Exception: []sentry.Exception{{Value: "boom"}}}, nil), "boom")

	got := sanitizeSentryValue("tags", []string{"a", "b"}, 0)
	require.Equal(t, []any{"a", "b"}, got)

	breadcrumb := sanitizeSentryBreadcrumbValue("items", []any{"a", "b"}, 0)
	require.Equal(t, []any{"a", "b"}, breadcrumb)
}
