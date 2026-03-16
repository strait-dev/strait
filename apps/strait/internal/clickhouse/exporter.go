package clickhouse

import (
	"context"
	"log/slog"
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

// Exporter batches events and periodically flushes them to ClickHouse.
type Exporter struct {
	client *Client
	config ExporterConfig
	logger *slog.Logger

	mu       sync.Mutex
	pending  []any // buffered records
	stopping atomic.Bool
	stopCh   chan struct{}
	done     chan struct{}
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
	}
}

// insertBatch writes a batch of records to ClickHouse.
func (e *Exporter) insertBatch(ctx context.Context, batch []any) error {
	if len(batch) == 0 || e.client == nil {
		return nil
	}
	// Batch insert: each record type (RunEvent, RunAnalytics, ComputeUsage)
	// maps to a ClickHouse table. Type-switch inserts into the correct one.
	// TODO(STR-6): implement per-type INSERT INTO ... VALUES batch.
	e.logger.Debug("clickhouse exporter flushed batch", "count", len(batch))
	return e.client.Exec(ctx, "SELECT 1") // Validate connection is alive.
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
