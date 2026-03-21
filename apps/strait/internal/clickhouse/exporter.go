package clickhouse

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ExporterConfig controls the async export worker behavior.
type ExporterConfig struct {
	BatchSize     int           // Max events per batch insert.
	FlushInterval time.Duration // Max time between flushes.
	Enabled       bool          // Feature gate.
}

// RunEventRecord maps to the run_events ClickHouse table.
type RunEventRecord struct {
	EventID   string
	RunID     string
	ProjectID string
	JobID     string
	EventType string
	Level     string
	Message   string
	Metadata  string
	CreatedAt time.Time
}

// RunAnalyticsRecord maps to the run_analytics ClickHouse table.
type RunAnalyticsRecord struct {
	RunID               string
	JobID               string
	ProjectID           string
	Status              string
	ExecutionMode       string
	MachinePreset       string
	Attempt             int
	DurationMs          uint64
	QueueWaitMs         uint64
	CostMicrousd        int64
	ComputeCostMicrousd int64
	TriggeredBy         string
	CreatedAt           time.Time
	StartedAt           *time.Time
	FinishedAt          *time.Time
}

// ComputeUsageRecord maps to the compute_usage ClickHouse table.
type ComputeUsageRecord struct {
	RunID         string
	ProjectID     string
	MachinePreset string
	MachineID     string
	DurationSecs  float64
	CostMicrousd  int64
	StartedAt     time.Time
	FinishedAt    time.Time
}

// maxFlushRetries is the maximum number of consecutive flush failures before
// a batch is dropped to prevent unbounded growth.
const maxFlushRetries = 2

// Exporter batches events and periodically flushes them to ClickHouse.
type Exporter struct {
	client *Client
	config ExporterConfig
	logger *slog.Logger

	mu                  sync.Mutex
	pending             []any // buffered records
	consecutiveFailures int
	stopping            atomic.Bool
	stopCh              chan struct{}
	done                chan struct{}
}

// NewExporter creates a new async exporter. Returns nil if client is nil or disabled.
func NewExporter(client *Client, config ExporterConfig, logger *slog.Logger) *Exporter {
	if client == nil || !config.Enabled {
		return nil
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 1000
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = 5 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Exporter{
		client:  client,
		config:  config,
		logger:  logger,
		pending: make([]any, 0, config.BatchSize),
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// Enqueue adds a record to the export buffer. Safe for concurrent use.
// Returns false if the exporter is stopping.
func (e *Exporter) Enqueue(record any) bool {
	if e == nil || e.stopping.Load() {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	e.pending = append(e.pending, record)

	// Backpressure: drop oldest if buffer exceeds 10x batch size.
	// Reallocate to release memory held by dropped elements.
	maxBuffer := e.config.BatchSize * 10
	if len(e.pending) > maxBuffer {
		dropped := len(e.pending) - maxBuffer
		kept := make([]any, maxBuffer)
		copy(kept, e.pending[dropped:])
		e.pending = kept
		e.logger.Warn("clickhouse exporter buffer overflow, dropped oldest records", "dropped", dropped)
	}
	return true
}

// Start begins the background flush loop.
func (e *Exporter) Start(ctx context.Context) {
	if e == nil {
		return
	}
	go func() { //nolint:gosec // ctx is intentionally captured for the flush loop lifetime.
		defer close(e.done)
		ticker := time.NewTicker(e.config.FlushInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				e.flush(ctx)
			case <-e.stopCh:
				e.flush(ctx) // Final flush.
				return
			case <-ctx.Done():
				e.flush(context.Background()) // Use background ctx for final flush.
				return
			}
		}
	}()
}

// Stop signals the exporter to flush remaining records and shut down.
// Records enqueued after Stop() returns are silently dropped.
func (e *Exporter) Stop() {
	if e == nil {
		return
	}
	e.stopping.Store(true)
	close(e.stopCh)
	<-e.done
}

func (e *Exporter) flush(ctx context.Context) {
	e.mu.Lock()
	if len(e.pending) == 0 {
		e.mu.Unlock()
		return
	}
	batch := e.pending
	e.pending = make([]any, 0, e.config.BatchSize)
	e.mu.Unlock()

	if err := e.insertBatch(ctx, batch); err != nil {
		e.logger.Error("clickhouse exporter flush failed", "count", len(batch), "error", err)
		e.mu.Lock()
		e.consecutiveFailures++
		if e.consecutiveFailures <= maxFlushRetries {
			maxBuffer := e.config.BatchSize * 10
			combined := append(batch, e.pending...) //nolint:gocritic // intentional prepend of failed batch
			if len(combined) > maxBuffer {
				// Keep the front (failed batch first) and drop newest overflow.
				combined = combined[:maxBuffer]
			}
			e.pending = combined
			e.logger.Warn("clickhouse requeued failed batch", "attempt", e.consecutiveFailures)
		} else {
			e.logger.Error("clickhouse dropping batch after max retries", "dropped", len(batch))
		}
		e.mu.Unlock()
		return
	}
	e.mu.Lock()
	e.consecutiveFailures = 0
	e.mu.Unlock()
}

// insertBatch writes a batch of records to ClickHouse, grouping by record type.
func (e *Exporter) insertBatch(ctx context.Context, batch []any) error {
	if len(batch) == 0 || e.client == nil {
		return nil
	}

	var events []RunEventRecord
	var analytics []RunAnalyticsRecord
	var usage []ComputeUsageRecord

	for _, rec := range batch {
		switch r := rec.(type) {
		case RunEventRecord:
			events = append(events, r)
		case RunAnalyticsRecord:
			analytics = append(analytics, r)
		case ComputeUsageRecord:
			usage = append(usage, r)
		default:
			e.logger.Warn("clickhouse exporter: unknown record type", "type", fmt.Sprintf("%T", rec))
		}
	}

	var errs []error
	if len(events) > 0 {
		if err := e.insertRunEvents(ctx, events); err != nil {
			errs = append(errs, fmt.Errorf("run_events: %w", err))
		}
	}
	if len(analytics) > 0 {
		if err := e.insertRunAnalytics(ctx, analytics); err != nil {
			errs = append(errs, fmt.Errorf("run_analytics: %w", err))
		}
	}
	if len(usage) > 0 {
		if err := e.insertComputeUsage(ctx, usage); err != nil {
			errs = append(errs, fmt.Errorf("compute_usage: %w", err))
		}
	}

	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, err := range errs {
			msgs[i] = err.Error()
		}
		return fmt.Errorf("batch insert errors: %s", strings.Join(msgs, "; "))
	}

	e.logger.Debug("clickhouse exporter flushed batch",
		"events", len(events),
		"analytics", len(analytics),
		"usage", len(usage),
	)
	return nil
}

func (e *Exporter) insertRunEvents(ctx context.Context, records []RunEventRecord) error {
	const row = "(?, ?, ?, ?, ?, ?, ?, ?, ?)"
	query := "INSERT INTO run_events (event_id, run_id, project_id, job_id, event_type, level, message, metadata, created_at) VALUES "
	placeholders := make([]string, len(records))
	args := make([]any, 0, len(records)*9)

	for i, r := range records {
		placeholders[i] = row
		args = append(args, r.EventID, r.RunID, r.ProjectID, r.JobID, r.EventType, r.Level, r.Message, r.Metadata, r.CreatedAt)
	}

	return e.client.Exec(ctx, query+strings.Join(placeholders, ", "), args...)
}

func (e *Exporter) insertRunAnalytics(ctx context.Context, records []RunAnalyticsRecord) error {
	const row = "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	query := "INSERT INTO run_analytics (run_id, job_id, project_id, status, execution_mode, machine_preset, attempt, duration_ms, queue_wait_ms, cost_microusd, compute_cost_microusd, triggered_by, created_at, started_at, finished_at) VALUES "
	placeholders := make([]string, len(records))
	args := make([]any, 0, len(records)*15)

	for i, r := range records {
		placeholders[i] = row
		args = append(args, r.RunID, r.JobID, r.ProjectID, r.Status, r.ExecutionMode, r.MachinePreset,
			r.Attempt, r.DurationMs, r.QueueWaitMs, r.CostMicrousd, r.ComputeCostMicrousd, r.TriggeredBy,
			r.CreatedAt, r.StartedAt, r.FinishedAt)
	}

	return e.client.Exec(ctx, query+strings.Join(placeholders, ", "), args...)
}

func (e *Exporter) insertComputeUsage(ctx context.Context, records []ComputeUsageRecord) error {
	const row = "(?, ?, ?, ?, ?, ?, ?, ?)"
	query := "INSERT INTO compute_usage (run_id, project_id, machine_preset, machine_id, duration_secs, cost_microusd, started_at, finished_at) VALUES "
	placeholders := make([]string, len(records))
	args := make([]any, 0, len(records)*8)

	for i, r := range records {
		placeholders[i] = row
		args = append(args, r.RunID, r.ProjectID, r.MachinePreset, r.MachineID, r.DurationSecs, r.CostMicrousd, r.StartedAt, r.FinishedAt)
	}

	return e.client.Exec(ctx, query+strings.Join(placeholders, ", "), args...)
}

// PendingCount returns the number of records waiting to be flushed.
func (e *Exporter) PendingCount() int {
	if e == nil {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.pending)
}
