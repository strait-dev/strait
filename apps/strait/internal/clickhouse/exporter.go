package clickhouse

import (
	"context"
	"log/slog"
	"sync"
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

	mu      sync.Mutex
	pending []any // buffered records
	stopCh  chan struct{}
	done    chan struct{}
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

// Enqueue adds a record to the export buffer.
func (e *Exporter) Enqueue(record any) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	e.pending = append(e.pending, record)

	// Backpressure: drop oldest if buffer exceeds 10x batch size.
	maxBuffer := e.config.BatchSize * 10
	if len(e.pending) > maxBuffer {
		dropped := len(e.pending) - maxBuffer
		e.pending = e.pending[dropped:]
		e.logger.Warn("clickhouse exporter buffer overflow, dropped oldest records", "dropped", dropped)
	}
}

// Start begins the background flush loop.
func (e *Exporter) Start(ctx context.Context) {
	if e == nil {
		return
	}
	go func() {
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
				e.flush(ctx) // Final flush.
				return
			}
		}
	}()
}

// Stop signals the exporter to flush remaining records and shut down.
func (e *Exporter) Stop() {
	if e == nil {
		return
	}
	close(e.stopCh)
	<-e.done
}

func (e *Exporter) flush(_ context.Context) {
	e.mu.Lock()
	if len(e.pending) == 0 {
		e.mu.Unlock()
		return
	}
	batch := e.pending
	e.pending = make([]any, 0, e.config.BatchSize)
	e.mu.Unlock()

	e.logger.Debug("clickhouse exporter flushing", "count", len(batch))
	// Actual ClickHouse insert would go here.
	// For now this is a foundation — the insert logic is in Phase 12.
	_ = batch
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
