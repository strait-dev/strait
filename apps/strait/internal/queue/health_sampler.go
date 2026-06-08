package queue

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"strait/internal/store"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// HealthSampler periodically queries pg_stat_user_tables and publishes
// per-partition live/dead tuple counts and HOT update ratio. It also samples
// the oldest queued row age so the dashboard has a direct signal for
// backlog growth.
//
// It is safe to run multiple samplers per process (tests often do) but in
// production one is enough — the query cost is low and it does not take
// locks.
type HealthSampler struct {
	db       store.DBTX
	interval time.Duration
	metrics  *QueueMetrics
	logger   *slog.Logger

	iterations atomic.Int64
}

// NewHealthSampler builds a sampler using the shared queue metrics singleton.
// interval <= 0 defaults to 30s.
func NewHealthSampler(db store.DBTX, interval time.Duration, logger *slog.Logger) (*HealthSampler, error) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	m, err := Metrics()
	if err != nil {
		return nil, err
	}
	return &HealthSampler{db: db, interval: interval, metrics: m, logger: logger}, nil
}

// Run blocks until ctx is cancelled, sampling every interval. The first
// sample runs immediately so tests do not have to wait a full interval.
func (h *HealthSampler) Run(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	h.SampleOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.SampleOnce(ctx)
		}
	}
}

// Iterations returns the number of completed sample iterations. Exposed for
// tests.
func (h *HealthSampler) Iterations() int64 { return h.iterations.Load() }

// SampleOnce runs a single sample iteration. It catches panics so a
// malformed pg_stat_user_tables row (or a dropped partition mid-query)
// cannot crash the service.
func (h *HealthSampler) SampleOnce(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			h.logger.Warn("queue health sampler panic recovered", "panic", r)
		}
		h.iterations.Add(1)
	}()

	h.samplePartitions(ctx)
	h.sampleOldestQueued(ctx)
	h.sampleHistoryLiveTuples(ctx)
	h.sampleQueueDepthByStatus(ctx)
	h.sampleStrandedTerminal(ctx)
	h.sampleIndexHealth(ctx)
	h.sampleOutboxClaimHealth(ctx)
}

func (h *HealthSampler) samplePartitions(ctx context.Context) {
	const q = `
SELECT
  relname,
  COALESCE(n_live_tup, 0)          AS live_tup,
  COALESCE(n_dead_tup, 0)          AS dead_tup,
  COALESCE(n_tup_upd, 0)           AS upd,
  COALESCE(n_tup_hot_upd, 0)       AS hot_upd
FROM pg_stat_user_tables
WHERE relname = 'job_runs' OR relname LIKE 'job_runs_%'
`
	rows, err := h.db.Query(ctx, q)
	if err != nil {
		h.logger.Debug("queue health sample: query failed", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var s PartitionStats
		if err := rows.Scan(&s.Relname, &s.LiveTuples, &s.DeadTuples, &s.TotalUpdates, &s.HotUpdates); err != nil {
			h.logger.Debug("queue health sample: scan failed", "error", err)
			continue
		}
		total := s.LiveTuples + s.DeadTuples
		if total > 0 {
			s.DeadTupleRatio = float64(s.DeadTuples) / float64(total)
		}
		h.metrics.RecordPartitionStats(ctx, s.Relname, s)
	}
	if err := rows.Err(); err != nil {
		h.logger.Debug("queue health sample: rows error", "error", err)
	}
}

func (h *HealthSampler) sampleHistoryLiveTuples(ctx context.Context) {
	const q = `
SELECT COALESCE(n_live_tup, 0) FROM pg_stat_user_tables
WHERE relname = 'job_runs_history'
`
	var liveTuples int64
	if err := h.db.QueryRow(ctx, q).Scan(&liveTuples); err != nil {
		h.logger.Debug("queue health sample: history live tuples query failed", "error", err)
		return
	}
	h.metrics.HistoryLiveTuples.Record(ctx, liveTuples)
}

func (h *HealthSampler) sampleQueueDepthByStatus(ctx context.Context) {
	const q = `
SELECT status, COUNT(*) FROM job_run_read_state
WHERE status IN ('queued', 'delayed', 'dequeued', 'executing', 'waiting', 'dead_letter')
GROUP BY status
`
	rows, err := h.db.Query(ctx, q)
	if err != nil {
		h.logger.Debug("queue health sample: queue depth by status query failed", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			continue
		}
		h.metrics.QueueDepthByStatus.Record(ctx, count,
			metric.WithAttributes(attribute.String("status", status)))
	}
}

func (h *HealthSampler) sampleStrandedTerminal(ctx context.Context) {
	const q = `
SELECT COUNT(*) FROM job_runs
WHERE finished_at IS NOT NULL
  AND finished_at < NOW() - INTERVAL '48 hours'
  AND status IN ('completed', 'failed', 'canceled', 'expired', 'timed_out', 'crashed', 'system_failed')
`
	var count int64
	if err := h.db.QueryRow(ctx, q).Scan(&count); err != nil {
		h.logger.Debug("queue health sample: stranded terminal query failed", "error", err)
		return
	}
	h.metrics.ArchiveStrandedTerminal.Record(ctx, count)
}

func (h *HealthSampler) sampleOldestQueued(ctx context.Context) {
	const q = `
SELECT COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(created_at))), 0)
FROM job_runs jr
LEFT JOIN job_run_read_state s ON s.run_id = jr.id
WHERE COALESCE(s.status, jr.status) = 'queued'
`
	var age float64
	if err := h.db.QueryRow(ctx, q).Scan(&age); err != nil {
		h.logger.Debug("queue health sample: oldest queued query failed", "error", err)
		return
	}
	h.metrics.OldestQueuedAge.Record(ctx, age)
}

func (h *HealthSampler) sampleIndexHealth(ctx context.Context) {
	// pgstatindex requires the pgstattuple extension. When the extension
	// is not installed (common on managed Postgres without explicit
	// CREATE EXTENSION), the query fails gracefully and we skip the
	// metric.
	const q = `SELECT COALESCE(dead_items, 0) FROM pgstatindex('idx_runs_queue_covering')`
	var deadItems int64
	if err := h.db.QueryRow(ctx, q).Scan(&deadItems); err != nil {
		// Expected on instances without pgstattuple; debug-level only.
		h.logger.Debug("queue health sample: pgstatindex not available", "error", err)
		return
	}
	h.metrics.IndexDeadItems.Record(ctx, deadItems)
}

func (h *HealthSampler) sampleOutboxClaimHealth(ctx context.Context) {
	const depthQ = `
SELECT status, COUNT(*)
FROM outbox_claims
GROUP BY status
`
	rows, err := h.db.Query(ctx, depthQ)
	if err != nil {
		h.logger.Debug("queue health sample: outbox claim depth failed", "error", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			continue
		}
		h.metrics.OutboxClaimDepth.Record(ctx, count,
			metric.WithAttributes(attribute.String("status", status)))
	}
	if err := rows.Err(); err != nil {
		h.logger.Debug("queue health sample: outbox claim depth rows error", "error", err)
	}

	const ageQ = `
SELECT COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(created_at))), 0)
FROM outbox_claims
WHERE status = 'ready'
`
	var oldestReadyAge float64
	if err := h.db.QueryRow(ctx, ageQ).Scan(&oldestReadyAge); err != nil {
		h.logger.Debug("queue health sample: outbox oldest ready age failed", "error", err)
	} else {
		h.metrics.OutboxOldestReadyAge.Record(ctx, oldestReadyAge)
	}

	const expiredQ = `
SELECT COUNT(*)
FROM outbox_claims
WHERE status = 'leased'
  AND lease_expires_at <= NOW()
`
	var expired int64
	if err := h.db.QueryRow(ctx, expiredQ).Scan(&expired); err != nil {
		h.logger.Debug("queue health sample: outbox expired leases failed", "error", err)
	} else {
		h.metrics.OutboxExpiredLeases.Record(ctx, expired)
	}

	const tableQ = `SELECT COALESCE(n_live_tup, 0), COALESCE(n_dead_tup, 0) FROM pg_stat_user_tables WHERE relname = 'outbox_claims'`
	var live, dead int64
	if err := h.db.QueryRow(ctx, tableQ).Scan(&live, &dead); err != nil {
		h.logger.Debug("queue health sample: outbox claim table stats failed", "error", err)
		return
	}
	h.metrics.OutboxClaimTableLiveTuples.Record(ctx, live)
	h.metrics.OutboxClaimTableDeadTuples.Record(ctx, dead)
}
