package logdrain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"strait/internal/domain"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
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

	// Resilience tunables. Retries follow 100ms → 400ms → 1.6s with full
	// jitter, which is bounded above by roughly 2s + jitter — well under the
	// 30s HTTP client deadline. The breaker opens for 30s after 5 consecutive
	// failures; a single probe is allowed in half-open.
	siemMaxRetryAttempts         = 3
	siemBreakerFailureThreshold  = 5
	siemBreakerHalfOpenSuccesses = 1
	siemSubDLQCapacity           = 1024
)

// Tunables exposed as vars (not consts) so tests can shrink timings.
var (
	siemRetryInitialBackoff = 100 * time.Millisecond
	siemRetryMaxBackoff     = 1600 * time.Millisecond
	siemRetryBackoffFactor  = 4.0
	siemBreakerOpenDuration = 30 * time.Second
)

// ErrSIEMCircuitOpen is returned when the SIEM circuit breaker is open and a
// forward call was short-circuited without hitting the network.
var ErrSIEMCircuitOpen = errors.New("audit SIEM circuit open")

// errRequestConstruct wraps failures to build the outbound HTTP request
// (malformed URL, invalid method, etc.). These are deterministic and
// must not burn the retry budget or flip the circuit breaker — a second
// attempt will produce the same error.
var errRequestConstruct = errors.New("audit SIEM request construction failed")

// terminalStatusError wraps a 4xx HTTP response so retry policy and caller
// can distinguish "terminal reject" from "transient failure". 4xx responses
// are not retried; the batch is dropped to the SIEM sub-DLQ.
type terminalStatusError struct {
	status int
}

func (e *terminalStatusError) Error() string {
	return fmt.Sprintf("SIEM terminal status %d", e.status)
}

// retryableStatusError wraps a 5xx response. Retry policy treats it as
// retryable; caller records it as a 5xx exhaustion on final failure.
type retryableStatusError struct {
	status int
}

func (e *retryableStatusError) Error() string {
	return fmt.Sprintf("SIEM retryable status %d", e.status)
}

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

	// Resilience and metrics.
	retryPolicy    failsafe.Policy[*http.Response]
	breaker        circuitbreaker.CircuitBreaker[*http.Response]
	breakerWasOpen atomic.Bool
	forwardedCount metric.Int64Counter
	failedCount    metric.Int64Counter
	circuitOpenCnt metric.Int64Counter
	batchSizeHist  metric.Int64Histogram
	subDLQ         *failureRingBuffer
}

// failureRingBuffer is a bounded FIFO that retains the last N events that
// were dropped to the SIEM sub-DLQ (terminal 4xx or exhausted retries).
// It is the in-memory forensic trail for "events that never reached the
// SIEM" and is accessed by operators via DrainedFailures.
type failureRingBuffer struct {
	mu    sync.Mutex
	items []FailedForward
	cap   int
	next  int
	size  int
}

// FailedForward captures one event that could not be delivered to the SIEM.
type FailedForward struct {
	Event  domain.AuditEvent
	Reason string
	When   time.Time
}

func newFailureRingBuffer(capacity int) *failureRingBuffer {
	if capacity <= 0 {
		capacity = siemSubDLQCapacity
	}
	return &failureRingBuffer{items: make([]FailedForward, capacity), cap: capacity}
}

func (r *failureRingBuffer) add(ev domain.AuditEvent, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[r.next] = FailedForward{Event: ev, Reason: reason, When: time.Now()}
	r.next = (r.next + 1) % r.cap
	if r.size < r.cap {
		r.size++
	}
}

func (r *failureRingBuffer) snapshot() []FailedForward {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]FailedForward, r.size)
	start := (r.next - r.size + r.cap) % r.cap
	for i := range r.size {
		out[i] = r.items[(start+i)%r.cap]
	}
	return out
}

func (r *failureRingBuffer) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.size
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
	d := &AuditSIEMDrain{
		endpoint:      endpoint,
		authToken:     authToken,
		client:        &http.Client{Timeout: 30 * time.Second},
		logger:        slog.Default(),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		subDLQ:        newFailureRingBuffer(siemSubDLQCapacity),
	}
	d.retryPolicy = retrypolicy.NewBuilder[*http.Response]().
		WithMaxRetries(siemMaxRetryAttempts-1).
		WithBackoffFactor(siemRetryInitialBackoff, siemRetryMaxBackoff, siemRetryBackoffFactor).
		WithJitterFactor(1.0).
		HandleIf(func(_ *http.Response, err error) bool {
			if err == nil {
				return false
			}
			var terminal *terminalStatusError
			// Terminal 4xx, request construction errors, and context
			// cancellation / deadline from the caller are all
			// deterministic or caller-directed. They must not burn the
			// retry budget or open the circuit. Only transient network
			// errors and retryableStatusError are retried.
			if errors.As(err, &terminal) {
				return false
			}
			if errors.Is(err, errRequestConstruct) {
				return false
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return false
			}
			return true
		}).
		ReturnLastFailure().
		Build()
	d.breaker = circuitbreaker.NewBuilder[*http.Response]().
		WithFailureThreshold(siemBreakerFailureThreshold).
		WithDelay(siemBreakerOpenDuration).
		WithSuccessThreshold(siemBreakerHalfOpenSuccesses).
		HandleIf(func(_ *http.Response, err error) bool {
			if err == nil {
				return false
			}
			// Terminal 4xx does NOT count toward breaker failures — the SIEM
			// is healthy, the payload was rejected. Request-construction
			// errors and caller-context cancellations are also excluded —
			// they tell us nothing about SIEM health.
			var terminal *terminalStatusError
			if errors.As(err, &terminal) {
				return false
			}
			if errors.Is(err, errRequestConstruct) {
				return false
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return false
			}
			return true
		}).
		OnOpen(func(_ circuitbreaker.StateChangedEvent) {
			d.breakerWasOpen.Store(true)
			if d.circuitOpenCnt != nil {
				d.circuitOpenCnt.Add(context.Background(), 1)
			}
			d.logger.Warn("audit SIEM circuit breaker opened",
				"endpoint", d.endpoint,
				"threshold", siemBreakerFailureThreshold,
				"open_duration", siemBreakerOpenDuration)
		}).
		Build()
	return d
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

// SetMetrics wires optional Prometheus/OTel instruments for forward outcomes.
// Any argument may be nil. Safe to call before or after Start.
func (d *AuditSIEMDrain) SetMetrics(forwarded, failed, circuitOpen metric.Int64Counter, batchSize metric.Int64Histogram) {
	if d == nil {
		return
	}
	d.forwardedCount = forwarded
	d.failedCount = failed
	d.circuitOpenCnt = circuitOpen
	d.batchSizeHist = batchSize
}

// DrainedFailures returns a snapshot of the in-memory SIEM sub-DLQ. Each
// entry is an audit event that could not be delivered to the SIEM endpoint
// (either a terminal 4xx or exhausted retries/circuit-open). The buffer is
// bounded; oldest entries are overwritten once the capacity is reached.
func (d *AuditSIEMDrain) DrainedFailures() []FailedForward {
	if d == nil || d.subDLQ == nil {
		return nil
	}
	return d.subDLQ.snapshot()
}

// DrainedFailureCount returns the number of entries currently retained in
// the SIEM sub-DLQ ring buffer.
func (d *AuditSIEMDrain) DrainedFailureCount() int {
	if d == nil || d.subDLQ == nil {
		return 0
	}
	return d.subDLQ.len()
}

// Start launches the background forwarder goroutine. Safe to call multiple
// times — only the first call takes effect. The goroutine runs until Stop
// is called (or the channel is drained after Stop).
func (d *AuditSIEMDrain) Start(ctx context.Context) {
	if d == nil {
		return
	}
	d.startOnce.Do(func() {
		bufSize := max(d.batchSize*4, minSIEMBufferSize)
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
// The call is wrapped in a retry policy (3 attempts, exponential backoff with
// full jitter, 5xx/network only) and a circuit breaker (opens after 5
// consecutive failures for 30s, with a single half-open probe). Terminal 4xx
// responses are NOT retried and the batch is recorded in the SIEM sub-DLQ.
func (d *AuditSIEMDrain) ForwardBatch(ctx context.Context, events []domain.AuditEvent) error {
	if d == nil || len(events) == 0 {
		return nil
	}

	// Serialize the batch once; every retry resends the same bytes.
	payload, err := encodeNDJSONBatch(events)
	if err != nil {
		return err
	}

	doOnce := func() (*http.Response, error) {
		req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, bytes.NewReader(payload))
		if rerr != nil {
			// Tag as non-retryable: a malformed URL / bad method is
			// deterministic — no amount of retrying fixes it.
			return nil, fmt.Errorf("%w: %w", errRequestConstruct, rerr)
		}
		req.Header.Set("Content-Type", "application/x-ndjson")
		req.Header.Set("User-Agent", "Strait-Audit-SIEM/1.0")
		if d.authToken != "" {
			req.Header.Set("Authorization", "Bearer "+d.authToken)
		}
		resp, herr := d.client.Do(req)
		if herr != nil {
			return nil, herr
		}
		// Drain and close immediately — we only care about the status code.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return resp, &terminalStatusError{status: resp.StatusCode}
		}
		if resp.StatusCode >= 500 {
			return resp, &retryableStatusError{status: resp.StatusCode}
		}
		return resp, nil
	}

	resp, execErr := failsafe.With[*http.Response](d.breaker, d.retryPolicy).
		WithContext(ctx).
		GetWithExecution(func(_ failsafe.Execution[*http.Response]) (*http.Response, error) {
			return doOnce()
		})

	if execErr != nil {
		d.recordFailure(ctx, events, execErr)
		return execErr
	}
	_ = resp
	d.recordSuccess(ctx, len(events))
	d.logger.Info("audit events forwarded to SIEM",
		"count", len(events), "endpoint", d.endpoint)
	return nil
}

// encodeNDJSONBatch serializes events as newline-delimited JSON. Exposed at
// package scope so the schema contract test can reuse the exact wire form.
func encodeNDJSONBatch(events []domain.AuditEvent) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for i := range events {
		if err := enc.Encode(&events[i]); err != nil {
			return nil, fmt.Errorf("encode audit event %d: %w", i, err)
		}
	}
	return buf.Bytes(), nil
}

// recordSuccess updates success metrics for a forwarded batch.
func (d *AuditSIEMDrain) recordSuccess(ctx context.Context, n int) {
	if d.forwardedCount != nil {
		d.forwardedCount.Add(ctx, int64(n))
	}
	if d.batchSizeHist != nil {
		d.batchSizeHist.Record(ctx, int64(n))
	}
}

// recordFailure classifies a forward failure, increments the correct
// failed-total reason, and (for terminal or permanently-lost batches) spills
// the events into the in-memory SIEM sub-DLQ.
func (d *AuditSIEMDrain) recordFailure(ctx context.Context, events []domain.AuditEvent, err error) {
	reason := classifyForwardError(err)
	if d.failedCount != nil {
		d.failedCount.Add(ctx, int64(len(events)),
			metric.WithAttributes(attribute.String("reason", reason)))
	}
	for i := range events {
		d.subDLQ.add(events[i], reason)
	}
	d.logger.Warn("audit SIEM batch forward failed",
		"count", len(events),
		"endpoint", d.endpoint,
		"reason", reason,
		"error", err)
}

// classifyForwardError maps a ForwardBatch error into one of the four
// metrics reasons: network_error, siem_4xx, siem_5xx_exhausted, circuit_open.
func classifyForwardError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrSIEMCircuitOpen) {
		return "circuit_open"
	}
	var terminal *terminalStatusError
	if errors.As(err, &terminal) {
		return "siem_4xx"
	}
	var retryable *retryableStatusError
	if errors.As(err, &retryable) {
		return "siem_5xx_exhausted"
	}
	// failsafe-go's circuitbreaker.ErrOpen indicates the breaker short-
	// circuited the call. Match by error string/type.
	if errors.Is(err, circuitbreaker.ErrOpen) {
		return "circuit_open"
	}
	return "network_error"
}
