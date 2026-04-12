package logdrain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"strait/internal/domain"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Default batching and flushing tunables for the SIEM drain. These are used
// when the constructor receives zero values.
const (
	defaultSIEMBatchSize     = 100
	defaultSIEMFlushInterval = 10 * time.Second
	minSIEMBufferSize        = 256
	siemShutdownTimeout      = 5 * time.Second
)

// AuditSIEMDrain forwards audit events to an external SIEM endpoint
// as NDJSON over HTTP POST. Each batch is sent with a Bearer token.
//
// When Start has been called, the drain maintains an internal buffered
// channel; Enqueue is a non-blocking send and the background goroutine
// flushes batches when either the batch size threshold is reached or
// the flush interval elapses.
type AuditSIEMDrain struct {
	endpoint      string
	authToken     string
	client        *http.Client
	logger        *slog.Logger
	batchSize     int
	flushInterval time.Duration

	// Runtime state (populated by Start).
	ch          chan domain.AuditEvent
	done        chan struct{}
	startOnce   sync.Once
	stopOnce    sync.Once
	stopped     atomic.Bool
	droppedFull metric.Int64Counter
}

// NewAuditSIEMDrain creates a new SIEM drain. Returns nil if endpoint is empty.
// batchSize and flushInterval fall back to defaults when non-positive.
func NewAuditSIEMDrain(endpoint, authToken string, batchSize int, flushInterval time.Duration) *AuditSIEMDrain {
	if endpoint == "" {
		return nil
	}
	if batchSize <= 0 {
		batchSize = defaultSIEMBatchSize
	}
	if flushInterval <= 0 {
		flushInterval = defaultSIEMFlushInterval
	}
	return &AuditSIEMDrain{
		endpoint:      endpoint,
		authToken:     authToken,
		client:        &http.Client{Timeout: 30 * time.Second},
		logger:        slog.Default(),
		batchSize:     batchSize,
		flushInterval: flushInterval,
	}
}

// SetDroppedCounter wires an optional Prometheus/OTel counter that is
// incremented every time Enqueue drops an event because the internal buffer
// is full. Safe to call before or after Start.
func (d *AuditSIEMDrain) SetDroppedCounter(c metric.Int64Counter) {
	if d == nil {
		return
	}
	d.droppedFull = c
}

// Start launches the background forwarder goroutine. Safe to call multiple
// times — only the first call takes effect. The goroutine runs until Stop
// is called (or the channel is drained after Stop).
func (d *AuditSIEMDrain) Start(ctx context.Context) {
	if d == nil {
		return
	}
	d.startOnce.Do(func() {
		bufSize := d.batchSize * 4
		if bufSize < minSIEMBufferSize {
			bufSize = minSIEMBufferSize
		}
		d.ch = make(chan domain.AuditEvent, bufSize)
		d.done = make(chan struct{})
		go d.run(ctx)
	})
}

// Enqueue attempts a non-blocking send into the forwarder channel. If the
// channel is full or the drain is stopped/not started, the event is
// dropped and the drop counter is incremented.
func (d *AuditSIEMDrain) Enqueue(ev domain.AuditEvent) {
	if d == nil || d.ch == nil || d.stopped.Load() {
		return
	}
	select {
	case d.ch <- ev:
	default:
		if d.droppedFull != nil {
			d.droppedFull.Add(context.Background(), 1,
				metric.WithAttributes(attribute.String("reason", "buffer_full")))
		}
		d.logger.Warn("audit SIEM drain buffer full, dropping event",
			"action", ev.Action, "resource_type", ev.ResourceType)
	}
}

// Stop closes the forwarder channel and waits for the background goroutine
// to flush remaining buffered events. Bounded by a 5s timeout so a
// misbehaving SIEM endpoint cannot indefinitely block shutdown.
func (d *AuditSIEMDrain) Stop(ctx context.Context) {
	if d == nil {
		return
	}
	d.stopOnce.Do(func() {
		d.stopped.Store(true)
		if d.ch != nil {
			close(d.ch)
		}
	})
	if d.done == nil {
		return
	}
	timeout := siemShutdownTimeout
	select {
	case <-d.done:
	case <-ctx.Done():
	case <-time.After(timeout):
		d.logger.Warn("audit SIEM drain did not finish within shutdown timeout",
			"timeout", timeout)
	}
}

// run reads from the channel and flushes whenever the batch is full or the
// ticker fires. Exits once the channel is closed AND drained.
func (d *AuditSIEMDrain) run(ctx context.Context) {
	defer close(d.done)
	ticker := time.NewTicker(d.flushInterval)
	defer ticker.Stop()

	batch := make([]domain.AuditEvent, 0, d.batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		flushCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := d.ForwardBatch(flushCtx, batch); err != nil {
			d.logger.Warn("audit SIEM batch forward failed",
				"count", len(batch), "error", err)
		}
		cancel()
		batch = batch[:0]
	}

	for {
		select {
		case ev, ok := <-d.ch:
			if !ok {
				// Channel closed — drain any remaining batch and exit.
				flush()
				return
			}
			batch = append(batch, ev)
			if len(batch) >= d.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			// Best-effort drain of queued events, then exit.
			for {
				select {
				case ev, ok := <-d.ch:
					if !ok {
						flush()
						return
					}
					batch = append(batch, ev)
					if len(batch) >= d.batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

// ForwardBatch sends a slice of audit events to the SIEM endpoint as NDJSON.
func (d *AuditSIEMDrain) ForwardBatch(ctx context.Context, events []domain.AuditEvent) error {
	if len(events) == 0 {
		return nil
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for i := range events {
		if err := enc.Encode(&events[i]); err != nil {
			return fmt.Errorf("encode audit event %d: %w", i, err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, &buf)
	if err != nil {
		return fmt.Errorf("create SIEM request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	req.Header.Set("User-Agent", "Strait-Audit-SIEM/1.0")
	if d.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.authToken)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("SIEM request failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("SIEM returned status %d", resp.StatusCode)
	}

	d.logger.Info("audit events forwarded to SIEM",
		"count", len(events), "endpoint", d.endpoint, "status", resp.StatusCode)
	return nil
}
