package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// DBPool is the minimal surface the watchdog needs; satisfied by *pgxpool.Pool
// and *testutil.TestDB's pool. Kept narrow so tests can inject a fake.
type DBPool interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// LongTxnSample is one row returned by the watchdog's pg_stat_activity probe.
type LongTxnSample struct {
	PID             int32
	ApplicationName string
	State           string
	TxnStartAge     time.Duration
	QueryStartAge   time.Duration
	BackendXminAge  int64
	Query           string
}

// DBWatchdog periodically polls pg_stat_activity and reports gauges and alert
// logs for long-running transactions that could pin the MVCC horizon. It is
// safe to stop via Run's context.
type DBWatchdog struct {
	pool            DBPool
	interval        time.Duration
	alertThreshold  time.Duration
	logger          *slog.Logger
	longestTxnGauge metric.Float64Gauge
	idleInTxnGauge  metric.Int64Gauge
	oldestXminGauge metric.Int64Gauge
	sampleCount     atomic.Int64
	alertCount      atomic.Int64
	lastSamples     atomic.Pointer[[]LongTxnSample]
}

// NewDBWatchdog creates a watchdog with the given pool and thresholds. The
// meter is taken from otel.GetMeterProvider() so it is wired into the same
// exporter as the rest of telemetry.
func NewDBWatchdog(pool DBPool, interval, alertThreshold time.Duration, logger *slog.Logger) (*DBWatchdog, error) {
	if pool == nil {
		return nil, errors.New("db watchdog: pool is nil")
	}
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if alertThreshold <= 0 {
		alertThreshold = 60 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}

	meter := otel.Meter("strait/db_watchdog")
	longest, err := meter.Float64Gauge(
		"strait.db.longest_txn_seconds",
		metric.WithDescription("Age of the longest-running transaction in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("create longest txn gauge: %w", err)
	}
	idleInTxn, err := meter.Int64Gauge(
		"strait.db.idle_in_txn_count",
		metric.WithDescription("Number of connections idle in transaction"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create idle in txn gauge: %w", err)
	}
	oldestXmin, err := meter.Int64Gauge(
		"strait.db.oldest_xmin_age_txids",
		metric.WithDescription("Age of the oldest backend xmin in transaction ids"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("create oldest xmin gauge: %w", err)
	}

	return &DBWatchdog{
		pool:            pool,
		interval:        interval,
		alertThreshold:  alertThreshold,
		logger:          logger,
		longestTxnGauge: longest,
		idleInTxnGauge:  idleInTxn,
		oldestXminGauge: oldestXmin,
	}, nil
}

// Run blocks until ctx is cancelled, sampling on the configured interval.
func (w *DBWatchdog) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	w.sampleOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.sampleOnce(ctx)
		}
	}
}

// SampleCount returns the number of sampling iterations completed. Exposed for
// tests.
func (w *DBWatchdog) SampleCount() int64 { return w.sampleCount.Load() }

// AlertCount returns the number of WARN-level alerts emitted. Exposed for
// tests.
func (w *DBWatchdog) AlertCount() int64 { return w.alertCount.Load() }

// LastSamples returns the samples observed on the most recent iteration. May
// be nil before the first iteration. Exposed for tests.
func (w *DBWatchdog) LastSamples() []LongTxnSample {
	if p := w.lastSamples.Load(); p != nil {
		return *p
	}
	return nil
}

// sampleOnce runs a single probe. It is resilient to transient query failures
// and never panics.
func (w *DBWatchdog) sampleOnce(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			w.logger.Warn("db watchdog panic recovered", "panic", r)
		}
	}()
	defer w.sampleCount.Add(1)

	// R2 Phase 2: exclude the watchdog's own connection from the scan. The
	// application_name filter ensures the watchdog does not see itself as
	// a long transaction when the pool is saturated; cmd/strait sets the
	// matching application_name on the watchdog-owned connection via the
	// AfterConnect hook.
	const q = `
SELECT
  pid,
  COALESCE(application_name, '')                          AS application_name,
  COALESCE(state, '')                                     AS state,
  COALESCE(EXTRACT(EPOCH FROM (NOW() - xact_start)), 0)   AS xact_age_s,
  COALESCE(EXTRACT(EPOCH FROM (NOW() - query_start)), 0)  AS query_age_s,
  COALESCE(age(backend_xmin), 0)                          AS xmin_age,
  COALESCE(LEFT(query, 500), '')                          AS query
FROM pg_stat_activity
WHERE backend_type = 'client backend'
  AND pid <> pg_backend_pid()
  AND COALESCE(application_name, '') NOT IN ('strait-watchdog', 'strait-reconciler')
  AND (xact_start IS NOT NULL OR state = 'idle in transaction')
`
	rows, err := w.pool.Query(ctx, q)
	if err != nil {
		w.logger.Debug("db watchdog query failed", "error", err)
		return
	}
	defer rows.Close()

	var samples []LongTxnSample
	var longestSeconds float64
	var idleInTxn int64
	var oldestXmin int64
	for rows.Next() {
		var s LongTxnSample
		var xactAgeS, queryAgeS float64
		if err := rows.Scan(&s.PID, &s.ApplicationName, &s.State, &xactAgeS, &queryAgeS, &s.BackendXminAge, &s.Query); err != nil {
			w.logger.Debug("db watchdog scan failed", "error", err)
			continue
		}
		s.TxnStartAge = time.Duration(xactAgeS * float64(time.Second))
		s.QueryStartAge = time.Duration(queryAgeS * float64(time.Second))
		if xactAgeS > longestSeconds {
			longestSeconds = xactAgeS
		}
		if s.State == "idle in transaction" || s.State == "idle in transaction (aborted)" {
			idleInTxn++
		}
		if s.BackendXminAge > oldestXmin {
			oldestXmin = s.BackendXminAge
		}
		samples = append(samples, s)

		if s.TxnStartAge >= w.alertThreshold {
			w.alertCount.Add(1)
			w.logger.Warn("long-running transaction detected",
				"pid", s.PID,
				"application_name", s.ApplicationName,
				"state", s.State,
				"txn_age_seconds", xactAgeS,
				"query_age_seconds", queryAgeS,
				"backend_xmin_age", s.BackendXminAge,
				"query", s.Query,
			)
		}
	}
	if err := rows.Err(); err != nil {
		w.logger.Debug("db watchdog rows error", "error", err)
	}

	w.longestTxnGauge.Record(ctx, longestSeconds)
	w.idleInTxnGauge.Record(ctx, idleInTxn)
	w.oldestXminGauge.Record(ctx, oldestXmin)
	w.lastSamples.Store(&samples)
}
