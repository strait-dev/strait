// Package loadtest exposes harnesses for load and chaos testing of
// Strait subsystems. This file contains the audit-emit harness used by
// audit_loadtest_test.go to exercise the async emit + DLQ + SIEM drain
// pipeline under burst, store-outage, and SIEM-outage conditions.
//
// The harness intentionally does NOT import internal/api. Re-implementing
// the drainer here mirrors the production semantics of
// processAuditAsyncEvent closely enough to measure the same failure
// modes (buffer saturation, retry-then-DLQ, SIEM sub-DLQ growth) without
// dragging the full api.Server dependency graph into the test package.
// See processAuditAsyncEvent in apps/strait/internal/api/audit_emit.go
// for the production path this mirrors.
package loadtest

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"strait/internal/domain"
)

// AuditEmitStore is the minimum surface the harness requires from a
// store implementation. It mirrors the two methods processAuditAsyncEvent
// calls against api.APIStore.
type AuditEmitStore interface {
	CreateAuditEvent(ctx context.Context, ev *domain.AuditEvent) error
	CreateAuditEventDeadletter(ctx context.Context, ev *domain.AuditEvent, lastErr string, retryCount int) error
}

// AuditEmitSink is an optional post-persist hook invoked once per
// successful CreateAuditEvent. Mirrors the siemDrain.Enqueue call on the
// happy path. May be nil.
type AuditEmitSink interface {
	Enqueue(ev domain.AuditEvent)
}

// AuditEmitHarnessConfig tunes harness behavior. Zero values select
// production-equivalent defaults.
type AuditEmitHarnessConfig struct {
	// BufferSize is the capacity of the async channel. Defaults to 4096
	// (the production value of auditAsyncBufferSize).
	BufferSize int

	// RetryDelays is the per-attempt backoff schedule. Defaults to a
	// tight schedule (3x 1ms) so load tests stay fast. Production uses
	// 50ms/200ms/1s.
	RetryDelays []time.Duration

	// LatencySamples is the size of the circular buffer used to sample
	// emit latency. Defaults to 8192.
	LatencySamples int

	// QueuePollInterval is the cadence at which the harness samples
	// len(ch) to compute PeakQueue. Defaults to 10ms.
	QueuePollInterval time.Duration
}

func (c *AuditEmitHarnessConfig) withDefaults() AuditEmitHarnessConfig {
	out := *c
	if out.BufferSize <= 0 {
		out.BufferSize = 4096
	}
	if len(out.RetryDelays) == 0 {
		out.RetryDelays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	}
	if out.LatencySamples <= 0 {
		out.LatencySamples = 8192
	}
	if out.QueuePollInterval <= 0 {
		out.QueuePollInterval = 10 * time.Millisecond
	}
	return out
}

// AuditEmitHarness mirrors the production async audit-emit drainer.
// It is NOT a public API of Strait — it is a test-only construct. The
// goal is to measure emit latency, queue depth, and deadletter behavior
// under configurable store/SIEM failure modes.
type AuditEmitHarness struct {
	cfg   AuditEmitHarnessConfig
	store AuditEmitStore
	sink  AuditEmitSink

	ch     chan *domain.AuditEvent
	done   chan struct{}
	startO sync.Once
	stopO  sync.Once

	// Atomic counters.
	emitted       atomic.Int64
	persisted     atomic.Int64
	deadlettered  atomic.Int64
	dropped       atomic.Int64
	dlqFailed     atomic.Int64
	peakQueue     atomic.Int64
	pollerStopCh  chan struct{}
	pollerDone    chan struct{}

	// Latency ring buffer. Guarded by latMu.
	latMu    sync.Mutex
	latBuf   []time.Duration
	latNext  int
	latSize  int
	latCap   int
}

// NewAuditEmitHarness builds a harness around the given store + optional
// sink. Call Start to launch the drainer.
func NewAuditEmitHarness(store AuditEmitStore, sink AuditEmitSink, cfg AuditEmitHarnessConfig) *AuditEmitHarness {
	c := cfg.withDefaults()
	return &AuditEmitHarness{
		cfg:    c,
		store:  store,
		sink:   sink,
		latBuf: make([]time.Duration, c.LatencySamples),
		latCap: c.LatencySamples,
	}
}

// Start launches the drainer goroutine and the queue-depth poller. Safe
// to call once; subsequent calls are no-ops.
func (h *AuditEmitHarness) Start() {
	h.startO.Do(func() {
		h.ch = make(chan *domain.AuditEvent, h.cfg.BufferSize)
		h.done = make(chan struct{})
		h.pollerStopCh = make(chan struct{})
		h.pollerDone = make(chan struct{})
		go h.drain()
		go h.pollQueueDepth()
	})
}

// Stop closes the channel and waits for the drainer + poller to exit.
func (h *AuditEmitHarness) Stop() {
	h.stopO.Do(func() {
		if h.ch != nil {
			close(h.ch)
		}
		if h.pollerStopCh != nil {
			close(h.pollerStopCh)
		}
	})
	if h.done != nil {
		<-h.done
	}
	if h.pollerDone != nil {
		<-h.pollerDone
	}
}

// Emit performs a non-blocking send mirroring emitAuditEventAsync.
// If the buffer is full, the event is counted as dropped and discarded
// (no sync-fallback — the harness measures buffer saturation directly).
func (h *AuditEmitHarness) Emit(ev *domain.AuditEvent) {
	h.emitted.Add(1)
	start := time.Now()
	select {
	case h.ch <- ev:
		h.recordLatency(time.Since(start))
	default:
		h.dropped.Add(1)
	}
}

func (h *AuditEmitHarness) recordLatency(d time.Duration) {
	h.latMu.Lock()
	h.latBuf[h.latNext] = d
	h.latNext = (h.latNext + 1) % h.latCap
	if h.latSize < h.latCap {
		h.latSize++
	}
	h.latMu.Unlock()
}

// LatencyPercentile returns the approximate latency at the given
// percentile (0 < p <= 100) across the currently retained samples.
func (h *AuditEmitHarness) LatencyPercentile(p float64) time.Duration {
	if p <= 0 || p > 100 {
		return 0
	}
	h.latMu.Lock()
	size := h.latSize
	snap := make([]time.Duration, size)
	copy(snap, h.latBuf[:size])
	h.latMu.Unlock()
	if size == 0 {
		return 0
	}
	asFloats := make([]float64, size)
	for i, d := range snap {
		asFloats[i] = float64(d)
	}
	sort.Float64s(asFloats)
	idx := int(float64(size) * p / 100.0)
	if idx >= size {
		idx = size - 1
	}
	return time.Duration(asFloats[idx])
}

// Counters returns a snapshot of atomic counters.
func (h *AuditEmitHarness) Counters() AuditEmitCounters {
	return AuditEmitCounters{
		Emitted:      h.emitted.Load(),
		Persisted:    h.persisted.Load(),
		Deadlettered: h.deadlettered.Load(),
		Dropped:      h.dropped.Load(),
		DLQFailed:    h.dlqFailed.Load(),
		PeakQueue:    h.peakQueue.Load(),
	}
}

// AuditEmitCounters is the snapshot returned by Counters.
type AuditEmitCounters struct {
	Emitted      int64
	Persisted    int64
	Deadlettered int64
	Dropped      int64
	DLQFailed    int64
	PeakQueue    int64
}

// WaitDrain blocks until the internal channel is empty or timeout
// elapses. Returns true if the channel drained.
func (h *AuditEmitHarness) WaitDrain(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(h.ch) == 0 {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return len(h.ch) == 0
}

func (h *AuditEmitHarness) drain() {
	defer close(h.done)
	for ev := range h.ch {
		h.processOne(ev)
	}
}

// processOne mirrors the retry-then-DLQ logic in
// api.Server.processAuditAsyncEvent. It is deliberately simpler
// (no OTel spans, no slog) so harness overhead stays low.
func (h *AuditEmitHarness) processOne(ev *domain.AuditEvent) {
	var lastErr error
	attempts := len(h.cfg.RetryDelays) + 1
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			time.Sleep(h.cfg.RetryDelays[attempt-1])
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := h.store.CreateAuditEvent(ctx, ev)
		cancel()
		if err == nil {
			h.persisted.Add(1)
			if h.sink != nil {
				h.sink.Enqueue(*ev)
			}
			return
		}
		lastErr = err
	}

	dlqCtx, dlqCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dlqCancel()
	errMsg := ""
	if lastErr != nil {
		errMsg = lastErr.Error()
	}
	if err := h.store.CreateAuditEventDeadletter(dlqCtx, ev, errMsg, len(h.cfg.RetryDelays)); err != nil {
		h.dlqFailed.Add(1)
		return
	}
	h.deadlettered.Add(1)
}

func (h *AuditEmitHarness) pollQueueDepth() {
	defer close(h.pollerDone)
	t := time.NewTicker(h.cfg.QueuePollInterval)
	defer t.Stop()
	for {
		select {
		case <-h.pollerStopCh:
			return
		case <-t.C:
			n := int64(len(h.ch))
			for {
				prev := h.peakQueue.Load()
				if n <= prev {
					break
				}
				if h.peakQueue.CompareAndSwap(prev, n) {
					break
				}
			}
		}
	}
}

// --- Test-friendly in-memory store ---

// MemoryAuditStore is a thread-safe in-memory AuditEmitStore used by
// the harness tests. It can be toggled between healthy and failing modes
// to simulate store outages.
type MemoryAuditStore struct {
	fail atomic.Bool

	mu         sync.Mutex
	events     []*domain.AuditEvent
	deadletter []*domain.AuditEvent
}

// NewMemoryAuditStore returns a fresh in-memory store in the healthy state.
func NewMemoryAuditStore() *MemoryAuditStore {
	return &MemoryAuditStore{}
}

// SetFail toggles failure mode. While true, CreateAuditEvent returns an
// error; CreateAuditEventDeadletter continues to succeed so events can
// still land in the DLQ.
func (m *MemoryAuditStore) SetFail(v bool) { m.fail.Store(v) }

// ErrMemoryStoreDown is returned when fail mode is active.
var ErrMemoryStoreDown = errors.New("memory audit store: simulated outage")

// CreateAuditEvent appends the event to the in-memory chain unless
// fail mode is active.
func (m *MemoryAuditStore) CreateAuditEvent(_ context.Context, ev *domain.AuditEvent) error {
	if m.fail.Load() {
		return ErrMemoryStoreDown
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	clone := *ev
	m.events = append(m.events, &clone)
	return nil
}

// CreateAuditEventDeadletter appends to the in-memory DLQ. Never fails.
func (m *MemoryAuditStore) CreateAuditEventDeadletter(_ context.Context, ev *domain.AuditEvent, _ string, _ int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	clone := *ev
	m.deadletter = append(m.deadletter, &clone)
	return nil
}

// EventCount returns the number of successfully persisted chain events.
func (m *MemoryAuditStore) EventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

// DeadletterCount returns the current DLQ depth.
func (m *MemoryAuditStore) DeadletterCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.deadletter)
}

// DrainDeadletter removes and returns all DLQ rows, mirroring the
// reclaimer loop's batch semantics.
func (m *MemoryAuditStore) DrainDeadletter() []*domain.AuditEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.deadletter
	m.deadletter = nil
	return out
}
