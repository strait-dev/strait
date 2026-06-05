// Package webhook provides durable webhook delivery for event triggers.
//
// Deliveries are persisted to the webhook_deliveries table on creation,
// then processed by a background worker with exponential backoff. If all
// attempts are exhausted, the delivery moves to "dead" status (DLQ).
// This survives process restarts — no in-memory state is required.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"strait/internal/billing"
	"strait/internal/clickhouse"
	"strait/internal/domain"
	"strait/internal/httputil"
	"strait/internal/telemetry"

	"github.com/getsentry/sentry-go"
	concpool "github.com/sourcegraph/conc/pool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// DeliveryStore is the subset of store operations needed by the webhook worker.
type DeliveryStore interface {
	CreateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	UpdateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	ListPendingWebhookRetries(ctx context.Context) ([]domain.WebhookDelivery, error)
	UpdateEventTriggerNotifyStatus(ctx context.Context, id string, notifyStatus string) error
	GetWebhookSubscriptionSecrets(ctx context.Context, subscriptionID string) (string, string, *time.Time, error)
}

// SecretDecryptor decrypts webhook signing secrets that were stored encrypted
// at rest. When STRAIT_ENCRYPTION_KEY is set, the API persists secrets as
// AES-GCM ciphertext; the delivery worker must decrypt before computing the
// outbound HMAC signature, otherwise subscribers cannot verify signatures.
type SecretDecryptor interface {
	Decrypt(ciphertext []byte) ([]byte, error)
}

const (
	defaultDeliveryConcurrency    = 50
	defaultWebhookMaxPayloadBytes = 1 << 20 // 1 MB
	defaultMaxBatchSize           = 50
	defaultDeliveryClaimLimit     = 100
	defaultDeliveryClaimLease     = 2 * time.Minute
	maxResponseBodyDrainBytes     = 1 << 20 // 1 MB — cap response body drain to prevent memory exhaustion
)

// webhookRand is a process-local PRNG used only for retry-backoff jitter.
// It's seeded from crypto/rand to avoid the deterministic default seed in
// math/rand/v2's NewPCG, which would make jitter predictable across pods.
// We don't need cryptographic randomness here — just enough to break up
// synchronized retry storms — so a non-blocking PCG is appropriate.
//
// math/rand/v2's *rand.Rand over PCG is not safe for concurrent use, so the
// delivery worker dispatches retries from many goroutines in parallel; guard
// access with webhookRandMu.
var webhookRand = func() *rand.Rand {
	var seed [16]byte
	_, _ = cryptorand.Read(seed[:])
	s1 := binary.LittleEndian.Uint64(seed[0:8])
	s2 := binary.LittleEndian.Uint64(seed[8:16])
	return rand.New(rand.NewPCG(s1, s2)) //nolint:gosec // jitter only; seeded from crypto/rand
}()

var webhookRandMu sync.Mutex
var newDefaultDeliveryTransport = httputil.NewExternalTransport

type webhookDeliveryClaimer interface {
	ClaimPendingWebhookRetries(ctx context.Context, limit int, leaseDuration time.Duration) ([]domain.WebhookDelivery, error)
}

type claimedWebhookDeliveryUpdater interface {
	UpdateClaimedWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) (bool, error)
}

type DeliveryWorker struct {
	client *http.Client
	store  DeliveryStore
	logger *slog.Logger

	concurrency           int
	defaultRetryPolicy    string
	circuitBreaker        WebhookCircuitBreaker
	metrics               *telemetry.Metrics
	chExporter            *clickhouse.Exporter
	costRecorder          *billing.RunCostRecorder
	secretDecryptor       SecretDecryptor
	maxPayloadBytes       int64
	batchByURL            bool
	maxBatchSize          int
	allowPrivateEndpoints bool
	httpTimeout           time.Duration
	httpIdleConnTimeout   time.Duration
	httpMaxIdleConns      int
	httpMaxIdleConnsHost  int
	httpTransportTuned    bool
	stop                  chan struct{}
	done                  chan struct{}
	stopOnce              sync.Once
	pollWG                sync.WaitGroup
	pollInFlight          atomic.Int64
	runStarted            atomic.Bool
}

type EventNotifier = DeliveryWorker

type DeliveryWorkerOption func(*DeliveryWorker)

func WithConcurrency(n int) DeliveryWorkerOption {
	return func(w *DeliveryWorker) {
		if n > 0 {
			w.concurrency = n
		}
	}
}

func WithRetryPolicy(policy string) DeliveryWorkerOption {
	return func(w *DeliveryWorker) {
		switch policy {
		case domain.WebhookRetryPolicyExponential, domain.WebhookRetryPolicyLinear, domain.WebhookRetryPolicyFixed:
			w.defaultRetryPolicy = policy
		}
	}
}

func WithCircuitBreaker(circuitBreaker WebhookCircuitBreaker) DeliveryWorkerOption {
	return func(w *DeliveryWorker) {
		w.circuitBreaker = circuitBreaker
	}
}

func WithMetrics(metrics *telemetry.Metrics) DeliveryWorkerOption {
	return func(w *DeliveryWorker) {
		w.metrics = metrics
	}
}

func WithChExporter(exporter *clickhouse.Exporter) DeliveryWorkerOption {
	return func(w *DeliveryWorker) {
		w.chExporter = exporter
	}
}

// WithRunCostRecorder wires a billing cost recorder so that each successful
// webhook delivery is recorded as a billable event (flat 20 micro-USD).
// Failed deliveries that are retried and eventually succeed are billed once;
// deliveries that never succeed are not billed.
func WithRunCostRecorder(recorder *billing.RunCostRecorder) DeliveryWorkerOption {
	return func(w *DeliveryWorker) {
		w.costRecorder = recorder
	}
}

func WithMaxPayloadBytes(maxPayloadBytes int64) DeliveryWorkerOption {
	return func(w *DeliveryWorker) {
		if maxPayloadBytes > 0 {
			w.maxPayloadBytes = maxPayloadBytes
		}
	}
}

func WithHTTPTransport(timeout, idleConnTimeout time.Duration, maxIdleConns, maxIdleConnsPerHost int) DeliveryWorkerOption {
	return func(w *DeliveryWorker) {
		w.httpTimeout = timeout
		w.httpIdleConnTimeout = idleConnTimeout
		w.httpMaxIdleConns = maxIdleConns
		w.httpMaxIdleConnsHost = maxIdleConnsPerHost
		w.httpTransportTuned = true
		w.rebuildHTTPClient()
	}
}

func WithAllowPrivateEndpoints(allow bool) DeliveryWorkerOption {
	return func(w *DeliveryWorker) {
		w.allowPrivateEndpoints = allow
		w.rebuildHTTPClient()
	}
}

// noFollowRedirects refuses to follow HTTP redirects on outbound webhook
// deliveries. Following 3xx without re-validating the destination IP would
// allow a public webhook target to bounce the request to internal addresses
// (cloud metadata, 10.x, 127.x) after the initial SSRF check has passed.
func noFollowRedirects(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}

func WithBatchByURL(enabled bool) DeliveryWorkerOption {
	return func(w *DeliveryWorker) {
		w.batchByURL = enabled
	}
}

func WithMaxBatchSize(n int) DeliveryWorkerOption {
	return func(w *DeliveryWorker) {
		if n > 0 {
			w.maxBatchSize = n
		}
	}
}

// WithSecretDecryptor wires a decryptor for webhook signing secrets stored
// encrypted at rest. Required when the API server is configured with a
// STRAIT_ENCRYPTION_KEY so that the outbound HMAC is computed over the
// plaintext secret subscribers know about, not the AES-GCM ciphertext.
func WithSecretDecryptor(dec SecretDecryptor) DeliveryWorkerOption {
	return func(w *DeliveryWorker) {
		w.secretDecryptor = dec
	}
}

func NewDeliveryWorker(store DeliveryStore, logger *slog.Logger, opts ...DeliveryWorkerOption) *DeliveryWorker {
	if logger == nil {
		logger = slog.Default()
	}
	w := &DeliveryWorker{
		store:              store,
		logger:             logger,
		concurrency:        defaultDeliveryConcurrency,
		defaultRetryPolicy: domain.WebhookRetryPolicyExponential,
		maxPayloadBytes:    defaultWebhookMaxPayloadBytes,
		maxBatchSize:       defaultMaxBatchSize,
		stop:               make(chan struct{}),
		done:               make(chan struct{}),
	}
	w.rebuildHTTPClient()
	for _, opt := range opts {
		opt(w)
	}
	return w
}

func (n *DeliveryWorker) rebuildHTTPClient() {
	// Per-request timeout via context; see attemptDelivery. Redirect following
	// is disabled to keep the SSRF guard intact on every hop.
	transport := newDefaultDeliveryTransport(n.allowPrivateEndpoints)
	if n.httpTransportTuned {
		transport = httputil.NewExternalTransport(n.allowPrivateEndpoints)
	}
	if n.httpIdleConnTimeout > 0 {
		transport.IdleConnTimeout = n.httpIdleConnTimeout
	}
	if n.httpMaxIdleConns > 0 {
		transport.MaxIdleConns = n.httpMaxIdleConns
	}
	if n.httpMaxIdleConnsHost > 0 {
		transport.MaxIdleConnsPerHost = n.httpMaxIdleConnsHost
	}
	n.client = &http.Client{
		Timeout:       n.httpTimeout,
		Transport:     transport,
		CheckRedirect: noFollowRedirects,
	}
}

// NewEventNotifier creates a new event notifier.
func NewEventNotifier(store DeliveryStore, logger *slog.Logger, opts ...DeliveryWorkerOption) *DeliveryWorker {
	return NewDeliveryWorker(store, logger, opts...)
}

// maybeDecryptSecret attempts to decrypt a webhook signing secret if a
// decryptor is wired. Returns the plaintext on success; returns the raw
// value with a warning log if decryption fails or no decryptor is set
// (the latter is the legitimate operating mode when STRAIT_ENCRYPTION_KEY
// is not configured).
func (n *DeliveryWorker) maybeDecryptSecret(deliveryID, kind, secret string) string {
	if secret == "" || n.secretDecryptor == nil {
		return secret
	}
	ciphertext, decodeErr := base64.StdEncoding.DecodeString(secret)
	if decodeErr != nil {
		ciphertext = []byte(secret)
	}
	plain, err := n.secretDecryptor.Decrypt(ciphertext)
	if err != nil {
		n.logger.Warn("failed to decrypt webhook signing secret; subscriber will not be able to verify signature",
			"delivery_id", deliveryID, "secret_kind", kind, "error", err)
		return secret
	}
	return string(plain)
}

// SetRunCostRecorder sets the billing cost recorder after construction.
// This allows wiring the recorder when it is created after the worker
// (e.g. main.go creates the worker before the billing store is initialised).
func (n *DeliveryWorker) SetRunCostRecorder(recorder *billing.RunCostRecorder) {
	n.costRecorder = recorder
}

// NotifyAsync persists a webhook delivery for the given trigger.
// This is the synchronous entry point called during trigger creation
// via the onTriggerCreate callback. The actual HTTP delivery happens
// asynchronously via RunWorker.
func (n *DeliveryWorker) NotifyAsync(trigger *domain.EventTrigger) {
	n.NotifyAsyncWithContext(context.Background(), trigger)
}

func (n *DeliveryWorker) NotifyAsyncWithContext(ctx context.Context, trigger *domain.EventTrigger) {
	if trigger.NotifyURL == "" {
		return
	}

	payload, err := json.Marshal(map[string]any{
		"event_key":    trigger.EventKey,
		"trigger_id":   trigger.ID,
		"project_id":   trigger.ProjectID,
		"expires_at":   trigger.ExpiresAt,
		"callback_url": fmt.Sprintf("/v1/events/%s/send", trigger.EventKey),
	})
	if err != nil {
		n.logger.Error("failed to marshal notify payload", "trigger_id", trigger.ID, "error", err)
		return
	}

	now := time.Now()
	d := &domain.WebhookDelivery{
		EventTriggerID: trigger.ID,
		WebhookURL:     trigger.NotifyURL,
		RetryPolicy:    n.defaultRetryPolicy,
		Status:         domain.WebhookStatusPending,
		Attempts:       0,
		MaxAttempts:    5,
		NextRetryAt:    &now,
		Payload:        payload,
	}

	createCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := n.store.CreateWebhookDelivery(createCtx, d); err != nil {
		n.logger.Error("failed to enqueue webhook delivery", "trigger_id", trigger.ID, "error", err)
		return
	}

	n.logger.Info("webhook delivery enqueued", "delivery_id", d.ID, "trigger_id", trigger.ID, "url_host", extractDomain(trigger.NotifyURL))
}

func (n *DeliveryWorker) EnqueueRunWebhook(ctx context.Context, job *domain.Job, run *domain.JobRun) error {
	if job == nil || run == nil {
		return fmt.Errorf("enqueue run webhook: job and run are required")
	}
	if job.WebhookURL == "" || !run.Status.IsTerminal() {
		return nil
	}

	payload, err := json.Marshal(map[string]any{
		"run_id":     run.ID,
		"job_id":     run.JobID,
		"project_id": run.ProjectID,
		"status":     string(run.Status),
		"attempt":    run.Attempt,
		"result":     run.Result,
		"error":      run.Error,
		"timestamp":  time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("enqueue run webhook: marshal payload: %w", err)
	}

	now := time.Now()
	d := &domain.WebhookDelivery{
		RunID:         run.ID,
		JobID:         run.JobID,
		WebhookURL:    job.WebhookURL,
		WebhookSecret: job.WebhookSecret,
		RetryPolicy:   n.defaultRetryPolicy,
		Status:        domain.WebhookStatusPending,
		Attempts:      0,
		MaxAttempts:   5,
		NextRetryAt:   &now,
		Payload:       payload,
	}

	if err := n.store.CreateWebhookDelivery(ctx, d); err != nil {
		return fmt.Errorf("enqueue run webhook: create delivery: %w", err)
	}

	return nil
}

// RunWorker polls for pending deliveries and attempts them. Blocks until ctx is canceled.
// Call this in a goroutine from the service startup (e.g., concpool group).
func (n *DeliveryWorker) RunWorker(ctx context.Context, pollInterval time.Duration) error {
	n.runStarted.Store(true)
	defer close(n.done)

	n.logger.Info("webhook delivery worker started", "poll_interval", pollInterval)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			n.logger.Info("webhook delivery worker stopped")
			return ctx.Err()
		case <-n.stop:
			n.logger.Info("webhook delivery worker stopped")
			return nil
		case <-ticker.C:
			n.pollWG.Add(1)
			n.pollInFlight.Add(1)
			n.processBatch(ctx)
			n.pollInFlight.Add(-1)
			n.pollWG.Done()
		}
	}
}

func (n *DeliveryWorker) Shutdown(ctx context.Context) error {
	n.stopOnce.Do(func() {
		close(n.stop)
	})

	if !n.runStarted.Load() {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-n.done:
	}

	n.pollWG.Wait()
	return nil
}

// processBatch fetches and processes a batch of pending deliveries.
func (n *DeliveryWorker) processBatch(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "webhook.ProcessBatch")
	defer span.End()

	deliveries, err := n.claimPendingDeliveries(ctx)
	if err != nil {
		n.logger.Error("failed to list pending webhook deliveries", "error", err)
		return
	}

	if !n.batchByURL {
		n.processIndividual(ctx, deliveries)
		return
	}

	n.processBatched(ctx, deliveries)
}

func (n *DeliveryWorker) claimPendingDeliveries(ctx context.Context) ([]domain.WebhookDelivery, error) {
	if claimer, ok := n.store.(webhookDeliveryClaimer); ok {
		return claimer.ClaimPendingWebhookRetries(ctx, defaultDeliveryClaimLimit, defaultDeliveryClaimLease)
	}
	return n.store.ListPendingWebhookRetries(ctx)
}

// processIndividual dispatches each delivery as a separate HTTP request.
func (n *DeliveryWorker) processIndividual(ctx context.Context, deliveries []domain.WebhookDelivery) {
	p := concpool.New().WithContext(ctx).WithMaxGoroutines(n.concurrency)
	for i := range deliveries {
		delivery := deliveries[i]
		p.Go(func(ctx context.Context) error {
			n.attemptDelivery(ctx, &delivery)
			return nil
		})
	}

	if err := p.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		n.logger.Error("webhook batch delivery failed", "error", err)
	}
}

// processBatched groups deliveries by URL and sends multi-event batches where possible.
func (n *DeliveryWorker) processBatched(ctx context.Context, deliveries []domain.WebhookDelivery) {
	groups := groupByURL(deliveries)

	p := concpool.New().WithContext(ctx).WithMaxGoroutines(n.concurrency)
	for _, group := range groups {
		if len(group) == 1 || hasSignedDelivery(group) {
			// Signed webhooks require per-delivery HMAC headers and replay keys,
			// so they must stay on the individual delivery path. Batching is
			// safe only for unsigned legacy run/event deliveries.
			for i := range group {
				delivery := group[i]
				p.Go(func(ctx context.Context) error {
					n.attemptDelivery(ctx, &delivery)
					return nil
				})
			}
			continue
		}

		// Within a group, all deliveries share the same (org_id, url) per
		// groupByURL, so the first entry's URL is correct for the HTTP
		// request and its OrgID is correct for circuit-breaker scoping.
		batchURL := group[0].WebhookURL
		for _, chunk := range chunkDeliveries(group, n.maxBatchSize) {
			p.Go(func(ctx context.Context) error {
				n.attemptBatchDelivery(ctx, batchURL, chunk)
				return nil
			})
		}
	}

	if err := p.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		n.logger.Error("webhook batched delivery failed", "error", err)
	}
}

func hasSignedDelivery(deliveries []domain.WebhookDelivery) bool {
	for i := range deliveries {
		if deliveries[i].SubscriptionID != "" || deliveries[i].WebhookSecret != "" {
			return true
		}
	}
	return false
}

// batchPayloadItem is a single entry in a batch webhook POST.
type batchPayloadItem struct {
	DeliveryID string          `json:"delivery_id"`
	Payload    json.RawMessage `json:"payload"`
}

// attemptBatchDelivery sends multiple deliveries to the same URL as a JSON array.
func (n *DeliveryWorker) attemptBatchDelivery(ctx context.Context, webhookURL string, deliveries []domain.WebhookDelivery) {
	start := time.Now()
	now := time.Now()

	ctx, span := otel.Tracer("strait").Start(ctx, "webhook.AttemptBatchDelivery", trace.WithAttributes(
		attribute.String("webhook.host", extractDomain(webhookURL)),
		attribute.Int("batch.size", len(deliveries)),
	))
	defer span.End()

	// Emit ClickHouse events for every delivery after the function returns,
	// regardless of success or failure (mirrors attemptDelivery's deferred emit).
	// Skipped on the payload-too-large fallback path because attemptDelivery
	// handles its own CH events there (avoids double-counting).
	var skipDeferredCHEvents bool
	defer func() {
		if !skipDeferredCHEvents {
			n.enqueueBatchDeliveryEvents(deliveries, start)
		}
	}()

	if blocked := n.checkBatchCircuitBreaker(ctx, webhookURL, deliveries, now, span); blocked {
		return
	}

	// Extract payloads first, preserving them so we can restore on fallback.
	extractedPayloads := make([]json.RawMessage, len(deliveries))
	items := make([]batchPayloadItem, len(deliveries))
	for i := range deliveries {
		deliveries[i].Attempts++
		extractedPayloads[i] = extractPayload(&deliveries[i])
		items[i] = batchPayloadItem{
			DeliveryID: deliveries[i].ID,
			Payload:    extractedPayloads[i],
		}
	}

	batchBody, err := json.Marshal(items)
	if err != nil {
		for i := range deliveries {
			n.recordFailure(ctx, &deliveries[i], now, false, fmt.Sprintf("marshal batch: %v", err))
		}
		span.SetStatus(codes.Error, "marshal batch failed")
		return
	}

	// Check aggregate payload size; fall back to individual delivery if too large.
	if n.maxPayloadBytes > 0 && int64(len(batchBody)) > n.maxPayloadBytes {
		n.logger.Warn("batch payload too large, falling back to individual delivery",
			"url_host", extractDomain(webhookURL), "batch_size", len(deliveries), "payload_bytes", len(batchBody))
		// Reset attempts and restore payloads; attemptDelivery will re-increment
		// attempts and re-extract from LastError.
		for i := range deliveries {
			deliveries[i].Attempts--
			deliveries[i].LastError = string(extractedPayloads[i])
		}
		// Skip deferred CH events; each attemptDelivery emits its own.
		skipDeferredCHEvents = true
		// Fan out concurrently to avoid serializing the fallback path.
		p := concpool.New().WithContext(ctx).WithMaxGoroutines(n.concurrency)
		for i := range deliveries {
			d := &deliveries[i]
			p.Go(func(ctx context.Context) error {
				n.attemptDelivery(ctx, d)
				return nil
			})
		}
		_ = p.Wait()
		return
	}

	if n.metrics != nil {
		n.metrics.WebhookPayloadBytes.Record(ctx, int64(len(batchBody)))
		for i := range deliveries {
			n.metrics.WebhookDeliveryAttempts.Add(ctx, 1, metric.WithAttributes(
				attribute.String("retry_policy", n.retryPolicyForDelivery(&deliveries[i])),
			))
		}
	}

	reqCtx, reqCancel := context.WithTimeout(ctx, 15*time.Second)
	defer reqCancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, webhookURL, bytes.NewReader(batchBody))
	if err != nil {
		errMsg := "create request: invalid webhook URL"
		for i := range deliveries {
			n.recordFailure(ctx, &deliveries[i], now, false, errMsg)
		}
		span.SetStatus(codes.Error, errMsg)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Strait-Batch", "true")
	req.Header.Set("X-Strait-Batch-Size", strconv.Itoa(len(deliveries)))

	resp, err := n.client.Do(req)
	if err != nil {
		if n.circuitBreaker != nil {
			n.circuitBreaker.RecordFailure(ctx, breakerKey(batchOrgID(deliveries), webhookURL))
		}
		errMsg := fmt.Sprintf("http request: %s", sanitizeHTTPClientError(err))
		for i := range deliveries {
			n.recordFailure(ctx, &deliveries[i], now, true, errMsg)
		}
		span.SetStatus(codes.Error, errMsg)
		return
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodyDrainBytes))
		_ = resp.Body.Close()
	}()

	statusCode := resp.StatusCode
	span.SetAttributes(attribute.Int("status_code", statusCode))

	if n.handleBatchResponseStatus(ctx, webhookURL, deliveries, now, statusCode, span) {
		return
	}

	n.markBatchDelivered(ctx, webhookURL, deliveries, now, statusCode)
	n.logger.Info("webhook batch delivered", "url_host", extractDomain(webhookURL), "batch_size", len(deliveries))
}

// checkBatchCircuitBreaker checks the circuit breaker for webhookURL and records failures
// for all deliveries if the circuit is open. Returns true if the caller should return early.
func (n *DeliveryWorker) checkBatchCircuitBreaker(
	ctx context.Context,
	webhookURL string,
	deliveries []domain.WebhookDelivery,
	now time.Time,
	span trace.Span,
) bool {
	if n.circuitBreaker == nil {
		return false
	}
	cbKey := breakerKey(batchOrgID(deliveries), webhookURL)
	canDeliver, err := n.circuitBreaker.CanDeliver(ctx, cbKey)
	if err != nil {
		n.logger.Warn("webhook circuit breaker check failed", "url_host", extractDomain(webhookURL), "error", err)
		return false
	}
	if !canDeliver {
		n.recordCircuitBreakerState(ctx, webhookURL, "open")
		for i := range deliveries {
			n.deferCircuitBreakerRetry(ctx, &deliveries[i], now)
		}
		span.SetStatus(codes.Error, "circuit breaker is open")
		return true
	}
	n.recordCircuitBreakerState(ctx, webhookURL, "closed")
	return false
}

// handleBatchResponseStatus processes non-2xx HTTP responses for a batch delivery.
// Returns true if the caller should return early (error path).
func (n *DeliveryWorker) handleBatchResponseStatus(
	ctx context.Context,
	webhookURL string,
	deliveries []domain.WebhookDelivery,
	now time.Time,
	statusCode int,
	span trace.Span,
) bool {
	if statusCode >= 500 {
		if n.circuitBreaker != nil {
			n.circuitBreaker.RecordFailure(ctx, breakerKey(batchOrgID(deliveries), webhookURL))
		}
		errMsg := fmt.Sprintf("server error: status %d", statusCode)
		for i := range deliveries {
			deliveries[i].LastStatusCode = &statusCode
			n.recordFailure(ctx, &deliveries[i], now, true, errMsg)
		}
		span.SetStatus(codes.Error, errMsg)
		return true
	}
	if statusCode >= 400 {
		errMsg := fmt.Sprintf("client error: status %d", statusCode)
		for i := range deliveries {
			deliveries[i].LastStatusCode = &statusCode
			n.recordFailure(ctx, &deliveries[i], now, false, errMsg)
		}
		span.SetStatus(codes.Error, errMsg)
		return true
	}
	if statusCode >= 300 {
		// 3xx: redirect. We deliberately do not follow redirects to defend
		// against SSRF-via-redirect, so a 3xx is a configuration error on
		// the receiver side and should not be treated as success.
		errMsg := fmt.Sprintf("redirect not followed: status %d", statusCode)
		for i := range deliveries {
			deliveries[i].LastStatusCode = &statusCode
			n.recordFailure(ctx, &deliveries[i], now, false, errMsg)
		}
		span.SetStatus(codes.Error, errMsg)
		return true
	}
	return false
}

// markBatchDelivered records a successful batch delivery for all deliveries.
func (n *DeliveryWorker) markBatchDelivered(ctx context.Context, webhookURL string, deliveries []domain.WebhookDelivery, now time.Time, statusCode int) {
	if n.circuitBreaker != nil {
		n.circuitBreaker.RecordSuccess(ctx, breakerKey(batchOrgID(deliveries), webhookURL))
	}
	for i := range deliveries {
		n.markSingleDelivered(ctx, &deliveries[i], now, statusCode)
	}
}

// markSingleDelivered persists a delivered status for one webhook delivery.
func (n *DeliveryWorker) markSingleDelivered(ctx context.Context, d *domain.WebhookDelivery, now time.Time, statusCode int) {
	d.Status = domain.WebhookStatusDelivered
	d.DeliveredAt = &now
	d.LastError = ""
	d.LastStatusCode = &statusCode
	updated, err := n.updateWebhookDelivery(ctx, d)
	if err != nil {
		n.logger.Error("failed to mark webhook delivered", "delivery_id", d.ID, "error", err)
		return
	}
	if !updated {
		n.logger.Warn("skipped webhook delivered update after lost claim", "delivery_id", d.ID)
		return
	}
	if n.costRecorder != nil && d.OrgID != "" && d.ProjectID != "" {
		if err := n.costRecorder.RecordWebhookDeliveryCost(ctx, d.OrgID, d.ProjectID, d.ID); err != nil {
			n.logger.Warn("failed to record webhook delivery cost", "delivery_id", d.ID, "error", err)
		}
	}
	if n.metrics != nil {
		n.metrics.WebhookDeliveriesTotal.Add(ctx, 1, metric.WithAttributes(
			attribute.String("status", "delivered"),
			attribute.String("retry_policy", n.retryPolicyForDelivery(d)),
		))
	}
	if d.EventTriggerID != "" {
		_ = n.store.UpdateEventTriggerNotifyStatus(ctx, d.EventTriggerID, "sent")
	}
}

// enqueueBatchDeliveryEvents emits a ClickHouse event for each delivery in a batch.
func (n *DeliveryWorker) enqueueBatchDeliveryEvents(deliveries []domain.WebhookDelivery, start time.Time) {
	durationMs := uint64(max(time.Since(start).Milliseconds(), 0))
	for i := range deliveries {
		eventType := "run_webhook"
		if deliveries[i].EventTriggerID != "" {
			eventType = "event_trigger_notify"
		}
		n.enqueueDeliveryEvent(&deliveries[i], durationMs, eventType)
	}
}

// extractPayload returns the JSON payload for a delivery. The canonical source
// is d.Payload (persisted on insert). Older rows that predate the payload
// column stash the payload inside d.LastError; in that case we lift it back
// out and clear LastError so a subsequent retry-error message cannot be
// mistaken for the payload. The minimal fallback exists for rows missing both.
func extractPayload(d *domain.WebhookDelivery) json.RawMessage {
	if len(d.Payload) > 0 {
		return d.Payload
	}
	if d.LastError != "" && json.Valid([]byte(d.LastError)) {
		payload := json.RawMessage(d.LastError)
		d.LastError = ""
		return payload
	}
	fallback, _ := json.Marshal(map[string]any{
		"trigger_id":  d.EventTriggerID,
		"delivery_id": d.ID,
	})
	return fallback
}

// groupByURL groups deliveries by (tenant scope, webhook_url). Including the
// tenant scope keeps cross-tenant deliveries to the same external URL out of the
// same batch and out of the same circuit-breaker bucket — otherwise one
// tenant's failing endpoint would silently trip the breaker for every other
// tenant pointing at the same host.
func groupByURL(deliveries []domain.WebhookDelivery) map[string][]domain.WebhookDelivery {
	groups := make(map[string][]domain.WebhookDelivery, len(deliveries))
	for i := range deliveries {
		key := breakerKey(deliveryTenantScope(deliveries[i]), deliveries[i].WebhookURL)
		groups[key] = append(groups[key], deliveries[i])
	}
	return groups
}

// breakerKey composes a tenant-scoped key for circuit-breaker / batching
// lookups. Callers that lack an org ID should pass a project-scoped fallback
// rather than collapsing unrelated tenants into one URL-only bucket.
func breakerKey(tenantScope, url string) string {
	if tenantScope == "" {
		return url
	}
	return tenantScope + "|" + url
}

func deliveryTenantScope(delivery domain.WebhookDelivery) string {
	if delivery.OrgID != "" {
		return "org:" + delivery.OrgID
	}
	if delivery.ProjectID != "" {
		return "project:" + delivery.ProjectID
	}
	return ""
}

// batchOrgID returns the OrgID shared by every delivery in a batch.
// groupByURL guarantees homogeneity, so the first non-empty value wins.
func batchOrgID(deliveries []domain.WebhookDelivery) string {
	for i := range deliveries {
		if deliveries[i].OrgID != "" {
			return deliveries[i].OrgID
		}
	}
	return ""
}

// chunkDeliveries splits a slice into chunks of at most size elements.
func chunkDeliveries(deliveries []domain.WebhookDelivery, size int) [][]domain.WebhookDelivery {
	if len(deliveries) == 0 || size <= 0 {
		return nil
	}
	chunks := make([][]domain.WebhookDelivery, 0, (len(deliveries)+size-1)/size)
	for i := 0; i < len(deliveries); i += size {
		end := min(i+size, len(deliveries))
		chunks = append(chunks, deliveries[i:end])
	}
	return chunks
}

// enqueueDeliveryEvent sends a WebhookDeliveryEventRecord to the ClickHouse exporter.
func (n *DeliveryWorker) enqueueDeliveryEvent(d *domain.WebhookDelivery, durationMs uint64, eventType string) {
	if n.chExporter == nil {
		return
	}
	var statusCode uint16
	if d.LastStatusCode != nil && *d.LastStatusCode > 0 {
		statusCode = uint16(*d.LastStatusCode) //nolint:gosec // HTTP status codes fit in uint16
	}
	var deliveredAt *time.Time
	if d.DeliveredAt != nil {
		t := *d.DeliveredAt
		deliveredAt = &t
	}
	rec := clickhouse.WebhookDeliveryEventRecord{
		DeliveryID:     d.ID,
		RunID:          d.RunID,
		JobID:          d.JobID,
		ProjectID:      d.ProjectID,
		WebhookURL:     d.WebhookURL,
		Status:         d.Status,
		Attempts:       uint8(min(d.Attempts, 255)), //nolint:gosec // clamped to uint8 range
		LastStatusCode: statusCode,
		DurationMs:     durationMs,
		EventType:      eventType,
		CreatedAt:      d.CreatedAt,
		DeliveredAt:    deliveredAt,
	}
	n.chExporter.Enqueue(rec)
}

type deliverySigningSecrets struct {
	current              string
	previous             string
	previousGraceExpires *time.Time
}

func webhookDeliveryPayload(d *domain.WebhookDelivery) []byte {
	if len(d.Payload) > 0 {
		return d.Payload
	}
	if d.LastError != "" {
		var js json.RawMessage
		if json.Unmarshal([]byte(d.LastError), &js) == nil {
			return []byte(d.LastError)
		}
	}
	body, _ := json.Marshal(map[string]any{
		"trigger_id":  d.EventTriggerID,
		"delivery_id": d.ID,
	})
	return body
}

func webhookDeliveryTimeout(attempts int) time.Duration {
	if attempts > 1 {
		return 15 * time.Second
	}
	return 5 * time.Second
}

func applyDeliveryMetadataHeaders(req *http.Request, d *domain.WebhookDelivery) {
	req.Header.Set("Content-Type", "application/json")
	if d.EventTriggerID != "" {
		req.Header.Set("X-Strait-Trigger-ID", d.EventTriggerID)
	}
	if d.RunID != "" {
		req.Header.Set("X-Run-ID", d.RunID)
	}
	if d.JobID != "" {
		req.Header.Set("X-Job-ID", d.JobID)
	}
	req.Header.Set("X-Strait-Delivery-ID", d.ID)
	req.Header.Set("X-Strait-Attempt", fmt.Sprintf("%d/%d", d.Attempts, d.MaxAttempts))
}

func (n *DeliveryWorker) deliverySigningSecrets(ctx context.Context, d *domain.WebhookDelivery) (deliverySigningSecrets, error) {
	if d.SubscriptionID == "" {
		if d.WebhookSecret == "" {
			return deliverySigningSecrets{}, nil
		}
		return deliverySigningSecrets{
			current: n.maybeDecryptSecret(d.ID, "job", d.WebhookSecret),
		}, nil
	}

	secret, prevSecret, graceExpires, err := n.store.GetWebhookSubscriptionSecrets(ctx, d.SubscriptionID)
	if err != nil {
		n.logger.Warn("failed to look up webhook signing secret", "delivery_id", d.ID, "subscription_id", d.SubscriptionID, "error", err)
		return deliverySigningSecrets{}, err
	}

	current := n.maybeDecryptSecret(d.ID, "current", secret)
	if current == "" {
		return deliverySigningSecrets{}, errors.New("webhook subscription signing secret unavailable")
	}
	return deliverySigningSecrets{
		current:              current,
		previous:             n.maybeDecryptSecret(d.ID, "previous", prevSecret),
		previousGraceExpires: graceExpires,
	}, nil
}

func applyDeliverySignatureHeaders(
	req *http.Request,
	d *domain.WebhookDelivery,
	body []byte,
	signatureTimestamp string,
	secrets deliverySigningSecrets,
) {
	req.Header.Set("X-Strait-Idempotency-Key", ComputeIdempotencyKey([]byte(secrets.current), d.ID, d.Attempts))
	req.Header.Set("X-Strait-Replay-Key", ComputeReplayKey([]byte(secrets.current), d.ID))
	if secrets.current == "" {
		return
	}

	sig := ComputeTimestampedHMACSHA256(secrets.current, signatureTimestamp, body)
	req.Header.Set("X-Strait-Signature", "v1="+sig)
	req.Header.Set("X-Webhook-Signature", "v1="+sig)
	if secrets.previous != "" && secrets.previousGraceExpires != nil && time.Now().Before(*secrets.previousGraceExpires) {
		oldSig := ComputeTimestampedHMACSHA256(secrets.previous, signatureTimestamp, body)
		req.Header.Set("X-Strait-Signature-Old", "v1="+oldSig)
		req.Header.Set("X-Webhook-Signature-Old", "v1="+oldSig)
	}
}

// attemptDelivery makes one HTTP request for a delivery.
func (n *DeliveryWorker) attemptDelivery(ctx context.Context, d *domain.WebhookDelivery) {
	start := time.Now()
	now := time.Now()
	retryPolicy := n.retryPolicyForDelivery(d)

	ctx, span := otel.Tracer("strait").Start(ctx, "webhook.AttemptDelivery", trace.WithAttributes(
		attribute.String("delivery.id", d.ID),
		attribute.String("webhook.host", extractDomain(d.WebhookURL)),
		attribute.Int("attempt", d.Attempts+1),
		attribute.Int("max_attempts", d.MaxAttempts),
		attribute.Int("status_code", 0),
	))
	defer span.End()

	defer func() {
		if n.metrics != nil {
			n.metrics.WebhookDeliveryDuration.Record(ctx, time.Since(start).Seconds())
		}
	}()

	// Enqueue ClickHouse record after every delivery attempt (success or failure).
	defer func() {
		durationMs := uint64(max(time.Since(start).Milliseconds(), 0))
		eventType := "run_webhook"
		if d.EventTriggerID != "" {
			eventType = "event_trigger_notify"
		}
		n.enqueueDeliveryEvent(d, durationMs, eventType)
	}()

	if !n.checkDeliveryCircuit(ctx, d, now, span) {
		return
	}

	d.Attempts++
	if n.metrics != nil {
		n.metrics.WebhookDeliveryAttempts.Add(ctx, 1, metric.WithAttributes(attribute.String("retry_policy", retryPolicy)))
	}

	prepared, ok := n.prepareDeliveryRequest(ctx, d, now, span)
	if !ok {
		return
	}
	defer prepared.cancel()

	resp, ok := n.executeDeliveryRequest(ctx, d, prepared.request, now, span)
	if !ok {
		return
	}
	defer closeDeliveryResponseBody(resp)

	n.handleDeliveryResponse(ctx, d, now, retryPolicy, resp.StatusCode, span)
}

type preparedDeliveryRequest struct {
	request *http.Request
	cancel  context.CancelFunc
}

func (n *DeliveryWorker) prepareDeliveryRequest(
	ctx context.Context,
	d *domain.WebhookDelivery,
	now time.Time,
	span trace.Span,
) (preparedDeliveryRequest, bool) {
	body := webhookDeliveryPayload(d)
	if !n.recordAndValidatePayloadSize(ctx, d, body, now, span) {
		return preparedDeliveryRequest{}, false
	}

	reqCtx, cancel := context.WithTimeout(ctx, webhookDeliveryTimeout(d.Attempts))
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, d.WebhookURL, bytes.NewReader(body))
	if err != nil {
		cancel()
		errMsg := "create request: invalid webhook URL"
		n.recordFailure(ctx, d, now, false, errMsg)
		span.SetStatus(codes.Error, errMsg)
		return preparedDeliveryRequest{}, false
	}
	signatureTimestamp := strconv.FormatInt(now.UTC().Unix(), 10)
	req.Header.Set("X-Strait-Timestamp", signatureTimestamp)
	applyDeliveryMetadataHeaders(req, d)

	secrets, err := n.deliverySigningSecrets(ctx, d)
	if err != nil {
		cancel()
		errMsg := "webhook subscription signing secret unavailable"
		n.recordFailure(ctx, d, now, true, errMsg)
		span.SetStatus(codes.Error, errMsg)
		return preparedDeliveryRequest{}, false
	}
	applyDeliverySignatureHeaders(req, d, body, signatureTimestamp, secrets)
	return preparedDeliveryRequest{request: req, cancel: cancel}, true
}

func (n *DeliveryWorker) recordAndValidatePayloadSize(
	ctx context.Context,
	d *domain.WebhookDelivery,
	body []byte,
	now time.Time,
	span trace.Span,
) bool {
	payloadSize := int64(len(body))
	if n.metrics != nil {
		n.metrics.WebhookPayloadBytes.Record(ctx, payloadSize)
	}
	if n.maxPayloadBytes <= 0 || payloadSize <= n.maxPayloadBytes {
		return true
	}

	errMsg := fmt.Sprintf("payload too large: %d bytes exceeds max %d", payloadSize, n.maxPayloadBytes)
	n.recordFailure(ctx, d, now, false, errMsg)
	span.SetStatus(codes.Error, errMsg)
	return false
}

func closeDeliveryResponseBody(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodyDrainBytes))
	_ = resp.Body.Close()
}

func (n *DeliveryWorker) checkDeliveryCircuit(
	ctx context.Context,
	d *domain.WebhookDelivery,
	now time.Time,
	span trace.Span,
) bool {
	if n.circuitBreaker == nil {
		return true
	}
	cbKey := breakerKey(d.OrgID, d.WebhookURL)
	canDeliver, err := n.circuitBreaker.CanDeliver(ctx, cbKey)
	if err != nil {
		n.logger.Warn("webhook circuit breaker check failed", "delivery_id", d.ID, "url_host", extractDomain(d.WebhookURL), "error", err)
		return true
	}
	if !canDeliver {
		n.recordCircuitBreakerState(ctx, d.WebhookURL, "open")
		n.deferCircuitBreakerRetry(ctx, d, now)
		span.SetStatus(codes.Error, "circuit breaker is open")
		return false
	}
	n.recordCircuitBreakerState(ctx, d.WebhookURL, "closed")
	return true
}

func (n *DeliveryWorker) executeDeliveryRequest(
	ctx context.Context,
	d *domain.WebhookDelivery,
	req *http.Request,
	now time.Time,
	span trace.Span,
) (*http.Response, bool) {
	resp, err := n.client.Do(req)
	if err == nil {
		return resp, true
	}
	if n.circuitBreaker != nil {
		n.circuitBreaker.RecordFailure(ctx, breakerKey(d.OrgID, d.WebhookURL))
	}
	errMsg := fmt.Sprintf("http request: %s", sanitizeHTTPClientError(err))
	n.recordFailure(ctx, d, now, true, errMsg)
	span.SetStatus(codes.Error, errMsg)
	return nil, false
}

func (n *DeliveryWorker) handleDeliveryResponse(
	ctx context.Context,
	d *domain.WebhookDelivery,
	now time.Time,
	retryPolicy string,
	statusCode int,
	span trace.Span,
) {
	span.SetAttributes(attribute.Int("status_code", statusCode))
	d.LastStatusCode = &statusCode

	if statusCode >= 500 {
		if n.circuitBreaker != nil {
			n.circuitBreaker.RecordFailure(ctx, breakerKey(d.OrgID, d.WebhookURL))
		}
		errMsg := fmt.Sprintf("server error: status %d", statusCode)
		n.recordFailure(ctx, d, now, true, errMsg)
		span.SetStatus(codes.Error, errMsg)
		return
	}
	if statusCode >= 400 {
		errMsg := fmt.Sprintf("client error: status %d", statusCode)
		n.recordFailure(ctx, d, now, false, errMsg)
		span.SetStatus(codes.Error, errMsg)
		return
	}
	if statusCode >= 300 {
		errMsg := fmt.Sprintf("redirect not followed: status %d", statusCode)
		n.recordFailure(ctx, d, now, false, errMsg)
		span.SetStatus(codes.Error, errMsg)
		return
	}

	n.markDeliverySucceeded(ctx, d, now, retryPolicy, span)
}

func (n *DeliveryWorker) markDeliverySucceeded(
	ctx context.Context,
	d *domain.WebhookDelivery,
	now time.Time,
	retryPolicy string,
	span trace.Span,
) {
	if n.circuitBreaker != nil {
		n.circuitBreaker.RecordSuccess(ctx, breakerKey(d.OrgID, d.WebhookURL))
	}
	d.Status = domain.WebhookStatusDelivered
	d.DeliveredAt = &now
	d.LastError = ""
	updated, err := n.updateWebhookDelivery(ctx, d)
	if err != nil {
		n.logger.Error("failed to mark webhook delivered", "delivery_id", d.ID, "error", err)
		span.SetStatus(codes.Error, "failed to persist delivered webhook")
		return
	}
	if !updated {
		n.logger.Warn("skipped webhook delivered update after lost claim", "delivery_id", d.ID)
		span.SetStatus(codes.Error, "webhook delivery claim lost before success update")
		return
	}
	if n.costRecorder != nil && d.OrgID != "" && d.ProjectID != "" {
		if err := n.costRecorder.RecordWebhookDeliveryCost(ctx, d.OrgID, d.ProjectID, d.ID); err != nil {
			n.logger.Warn("failed to record webhook delivery cost", "delivery_id", d.ID, "error", err)
		}
	}
	if n.metrics != nil {
		n.metrics.WebhookDeliveriesTotal.Add(ctx, 1, metric.WithAttributes(
			attribute.String("status", "delivered"),
			attribute.String("retry_policy", retryPolicy),
		))
	}
	if d.EventTriggerID != "" {
		_ = n.store.UpdateEventTriggerNotifyStatus(ctx, d.EventTriggerID, "sent")
	}
	n.logger.Info("webhook delivered", "delivery_id", d.ID, "url_host", extractDomain(d.WebhookURL), "attempt", d.Attempts)
}

// recordFailure handles a failed delivery attempt. For retryable errors, schedules
// the next attempt with exponential backoff. For non-retryable errors or exhausted
// attempts, marks the delivery as dead (DLQ).
func (n *DeliveryWorker) recordFailure(ctx context.Context, d *domain.WebhookDelivery, now time.Time, retryable bool, errMsg string) {
	d.LastError = errMsg
	retryPolicy := n.retryPolicyForDelivery(d)
	metricStatus := "failed"
	if !retryable || d.Attempts >= d.MaxAttempts {
		metricStatus = "dead"
	}
	if n.metrics != nil {
		n.metrics.WebhookDeliveriesTotal.Add(ctx, 1, metric.WithAttributes(
			attribute.String("status", metricStatus),
			attribute.String("retry_policy", retryPolicy),
		))
	}

	// Non-retryable or exhausted → dead letter.
	if !retryable || d.Attempts >= d.MaxAttempts {
		d.Status = domain.WebhookStatusDead
		captureWebhookDeadLetter(ctx, d, retryable, errMsg)
		updated, err := n.updateWebhookDelivery(ctx, d)
		if err != nil {
			n.logger.Error("failed to dead-letter webhook", "delivery_id", d.ID, "error", err)
			return
		}
		if !updated {
			n.logger.Warn("skipped webhook dead-letter update after lost claim", "delivery_id", d.ID)
			return
		}
		if d.EventTriggerID != "" {
			_ = n.store.UpdateEventTriggerNotifyStatus(ctx, d.EventTriggerID, "failed")
		}
		n.logger.Error("webhook delivery dead-lettered",
			"delivery_id", d.ID, "url_host", extractDomain(d.WebhookURL),
			"attempts", d.Attempts, "max_attempts", d.MaxAttempts, "error", errMsg)
		return
	}

	backoff := backoffForRetryPolicy(retryPolicy, d.Attempts)
	nextAttempt := now.Add(backoff)
	d.NextRetryAt = &nextAttempt
	d.Status = domain.WebhookStatusPending

	updated, err := n.updateWebhookDelivery(ctx, d)
	if err != nil {
		n.logger.Error("failed to schedule webhook retry", "delivery_id", d.ID, "error", err)
		return
	}
	if !updated {
		n.logger.Warn("skipped webhook retry update after lost claim", "delivery_id", d.ID)
		return
	}

	n.logger.Warn("webhook delivery failed, scheduled retry",
		"delivery_id", d.ID, "url_host", extractDomain(d.WebhookURL),
		"attempt", d.Attempts, "max_attempts", d.MaxAttempts,
		"next_attempt", nextAttempt, "error", errMsg)
}

func (n *DeliveryWorker) deferCircuitBreakerRetry(ctx context.Context, d *domain.WebhookDelivery, now time.Time) {
	d.LastError = "circuit breaker is open"
	nextAttempt := now.Add(backoffForRetryPolicy(n.retryPolicyForDelivery(d), d.Attempts))
	d.NextRetryAt = &nextAttempt
	d.Status = domain.WebhookStatusPending

	updated, err := n.updateWebhookDelivery(ctx, d)
	if err != nil {
		n.logger.Error("failed to defer webhook delivery for open circuit breaker", "delivery_id", d.ID, "error", err)
		return
	}
	if !updated {
		n.logger.Warn("skipped webhook circuit-breaker deferral after lost claim", "delivery_id", d.ID)
		return
	}
	n.logger.Warn("webhook delivery deferred because circuit breaker is open",
		"delivery_id", d.ID,
		"url_host", extractDomain(d.WebhookURL),
		"attempts", d.Attempts,
		"max_attempts", d.MaxAttempts,
		"next_attempt", nextAttempt,
	)
}

func captureWebhookDeadLetter(ctx context.Context, d *domain.WebhookDelivery, retryable bool, errMsg string) {
	if d == nil {
		return
	}
	capture := func(hub *sentry.Hub) {
		hub.WithScope(func(scope *sentry.Scope) {
			applyWebhookDeadLetterSentryScope(scope, d, retryable, errMsg)
			hub.CaptureMessage(fmt.Sprintf("webhook delivery dead-lettered: %s", errMsg))
		})
	}
	if hub := sentry.GetHubFromContext(ctx); hub != nil {
		capture(hub)
		return
	}
	capture(sentry.CurrentHub())
}

func applyWebhookDeadLetterSentryScope(scope *sentry.Scope, d *domain.WebhookDelivery, retryable bool, errMsg string) {
	telemetry.ApplySentryRuntimeScope(scope, telemetry.SentryRuntime{
		Edition:   string(domain.BuildEdition()),
		Subsystem: telemetry.SubsystemWebhook,
	})
	telemetry.SetSentryTag(scope, telemetry.TagDeliveryID, d.ID)
	telemetry.SetSentryTag(scope, telemetry.TagRunID, d.RunID)
	telemetry.SetSentryTag(scope, telemetry.TagJobID, d.JobID)
	telemetry.SetSentryTag(scope, telemetry.TagProjectID, d.ProjectID)
	telemetry.SetSentryTag(scope, telemetry.TagOrgID, d.OrgID)
	telemetry.SetSentryTag(scope, telemetry.TagTriggerID, d.EventTriggerID)
	telemetry.SetSentryTag(scope, telemetry.TagSubscriptionID, d.SubscriptionID)
	telemetry.SetSentryTag(scope, telemetry.TagAttempt, strconv.Itoa(d.Attempts))
	telemetry.SetSentryTag(scope, telemetry.TagOperation, "dead_letter")
	scope.SetLevel(sentry.LevelWarning)
	scope.SetContext("webhook.delivery", sentry.Context{
		"delivery_id":        d.ID,
		"run_id":             d.RunID,
		"job_id":             d.JobID,
		"project_id":         d.ProjectID,
		"org_id":             d.OrgID,
		"event_trigger_id":   d.EventTriggerID,
		"subscription_id":    d.SubscriptionID,
		"webhook_url_domain": extractDomain(d.WebhookURL),
		"attempts":           d.Attempts,
		"max_attempts":       d.MaxAttempts,
		"retryable":          retryable,
		"retry_policy":       d.RetryPolicy,
		"last_status_code":   d.LastStatusCode,
		"error":              errMsg,
	})
	scope.SetFingerprint([]string{"webhook_dead_lettered"})
}

func (n *DeliveryWorker) updateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) (bool, error) {
	if d.ClaimToken != "" {
		if updater, ok := n.store.(claimedWebhookDeliveryUpdater); ok {
			return updater.UpdateClaimedWebhookDelivery(ctx, d)
		}
	}
	if err := n.store.UpdateWebhookDelivery(ctx, d); err != nil {
		return false, err
	}
	return true, nil
}

func (n *DeliveryWorker) recordCircuitBreakerState(ctx context.Context, url string, currentState string) {
	if n.metrics == nil || url == "" {
		return
	}

	states := []string{"closed", "open", "half_open"}
	for _, state := range states {
		value := int64(0)
		if state == currentState {
			value = 1
		}
		n.metrics.WebhookCircuitBreaker.Record(ctx, value, metric.WithAttributes(
			attribute.String("url_host", extractDomain(url)),
			attribute.String("state", state),
		))
	}
}

func sanitizeHTTPClientError(err error) string {
	return httputil.SanitizeHTTPClientError(err)
}

func (n *DeliveryWorker) retryPolicyForDelivery(d *domain.WebhookDelivery) string {
	switch d.RetryPolicy {
	case domain.WebhookRetryPolicyExponential, domain.WebhookRetryPolicyLinear, domain.WebhookRetryPolicyFixed:
		return d.RetryPolicy
	default:
		return n.defaultRetryPolicy
	}
}

const maxWebhookBackoff = 30 * time.Minute

func backoffForRetryPolicy(policy string, attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}

	var backoff time.Duration
	switch policy {
	case domain.WebhookRetryPolicyLinear:
		backoff = linearWebhookBackoff(attempts)
	case domain.WebhookRetryPolicyFixed:
		backoff = 5 * time.Second
	default:
		backoff = exponentialWebhookBackoff(attempts)
	}

	if backoff > maxWebhookBackoff {
		backoff = maxWebhookBackoff
	}

	// Decorrelated jitter (+/- 20%) prevents thundering-herd retries when a
	// downstream goes down: without it, every failed delivery from the same
	// poll cycle reschedules at identical timestamps and DDoSes the
	// recovering endpoint in synchronized waves.
	return jitterBackoff(backoff)
}

func linearWebhookBackoff(attempts int) time.Duration {
	maxSeconds := int64(maxWebhookBackoff / time.Second)
	if attempts > int(maxSeconds/5) {
		return maxWebhookBackoff
	}
	return cappedBackoffSeconds(int64(attempts) * 5)
}

func exponentialWebhookBackoff(attempts int) time.Duration {
	maxSeconds := int64(maxWebhookBackoff / time.Second)
	seconds := int64(1)
	for range attempts {
		if seconds > maxSeconds/5 {
			return maxWebhookBackoff
		}
		seconds *= 5
	}
	return cappedBackoffSeconds(seconds)
}

func cappedBackoffSeconds(seconds int64) time.Duration {
	maxSeconds := int64(maxWebhookBackoff / time.Second)
	if seconds > maxSeconds {
		seconds = maxSeconds
	}
	return time.Duration(seconds) * time.Second
}

// jitterBackoff applies a uniform +/- 20% jitter to a backoff duration. The
// fraction is intentionally bounded so the average stays at the configured
// value while breaking up retry synchronization.
func jitterBackoff(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	webhookRandMu.Lock()
	jitter := webhookRand.Int64N(int64(d) / 5) // up to 20%
	sign := webhookRand.Int64N(2)
	webhookRandMu.Unlock()
	if sign == 0 {
		return d - time.Duration(jitter)
	}
	return d + time.Duration(jitter)
}

// EnqueueSubscriptionWebhooks creates webhook delivery records for all
// active subscriptions matching the given event type.
func (n *DeliveryWorker) EnqueueSubscriptionWebhooks(ctx context.Context, subs []domain.WebhookSubscription, eventType string, payload json.RawMessage) {
	for _, sub := range subs {
		if !sub.Active {
			continue
		}
		if !matchesEventType(sub.EventTypes, eventType) {
			continue
		}

		// Back-date NextRetryAt by a second so the row is reliably "due" by
		// the time the worker's `next_retry_at <= NOW()` filter runs, even
		// when Go and Postgres see slightly different wall clocks (e.g. host
		// vs. container in integration tests).
		now := time.Now().Add(-1 * time.Second)
		delivery := &domain.WebhookDelivery{
			SubscriptionID: sub.ID,
			ProjectID:      sub.ProjectID,
			WebhookURL:     sub.WebhookURL,
			RetryPolicy:    n.defaultRetryPolicy,
			Status:         domain.WebhookStatusPending,
			Attempts:       0,
			MaxAttempts:    3,
			NextRetryAt:    &now,
			Payload:        payload,
		}

		if err := n.store.CreateWebhookDelivery(ctx, delivery); err != nil {
			n.logger.Error("failed to create subscription webhook delivery",
				"subscription_id", sub.ID, "event_type", eventType, "error", err)
		}
	}
}

func matchesEventType(types []string, eventType string) bool {
	for _, t := range types {
		if t == eventType || t == "*" {
			return true
		}
	}
	return false
}

func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "unknown"
	}
	return u.Host
}

// replayKeyPrefix is the shared prefix for both the signed and unsigned
// replay-key derivations.
const replayKeyPrefix = "rk_"

// replayKeyHexLen bounds the HMAC hex output so the header stays small
// while retaining enough entropy (128 bits) to make collisions
// effectively impossible across a single subscription's delivery
// history.
const replayKeyHexLen = 32

const idempotencyKeyPrefix = "ik_"

// ComputeReplayKey derives a subscriber-visible replay key that is
// stable across every retry of the same physical delivery row AND bound
// to the subscription's HMAC secret. Subscribers can verify the key by
// recomputing hmac_sha256(secret, delivery_id) and comparing the first
// replayKeyHexLen hex chars, which proves the key was produced by a
// party that holds the secret (i.e. our service) rather than leaking
// internal delivery ids in the clear.
//
// The key is deterministic in (secret, delivery_id) by construction, so
// no server-side persistence is required. Callers that do not have a
// subscription secret available should use ComputeReplayKeyUnsigned.
func ComputeReplayKey(secret []byte, deliveryID string) string {
	if deliveryID == "" {
		return ""
	}
	if len(secret) == 0 {
		return ComputeReplayKeyUnsigned(deliveryID)
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(deliveryID))
	// Truncate the raw HMAC bytes before hex-encoding rather than
	// hex-encoding all 32 bytes and slicing 32 chars off the end: same
	// output, roughly half the intermediate allocation on a hot
	// signing path. replayKeyHexLen hex chars == replayKeyHexLen/2 raw
	// bytes.
	sum := mac.Sum(nil)
	rawLen := min(replayKeyHexLen/2, len(sum))
	return replayKeyPrefix + hex.EncodeToString(sum[:rawLen])
}

func ComputeIdempotencyKey(secret []byte, deliveryID string, attempt int) string {
	if deliveryID == "" || attempt < 1 {
		return ""
	}
	raw := fmt.Sprintf("%s:%d", deliveryID, attempt)
	if len(secret) == 0 {
		return raw
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(raw))
	sum := mac.Sum(nil)
	rawLen := min(replayKeyHexLen/2, len(sum))
	return idempotencyKeyPrefix + hex.EncodeToString(sum[:rawLen])
}

// ComputeReplayKeyUnsigned derives a deterministic replay key for
// deliveries where no subscription or job webhook secret is available.
// The returned key is stable across retries of the same underlying id but
// is NOT HMAC-bound; subscribers must treat it as an opaque replay
// discriminator rather than a proof of origin.
func ComputeReplayKeyUnsigned(id string) string {
	if id == "" {
		return ""
	}
	return replayKeyPrefix + id
}

// replayKeyFromDeliveryID is retained for back-compat with call sites
// that previously produced the unsigned form. New code should use
// ComputeReplayKey (subscription deliveries) or ComputeReplayKeyUnsigned
// (run-terminal deliveries) directly.
func replayKeyFromDeliveryID(deliveryID string) string {
	return ComputeReplayKeyUnsigned(deliveryID)
}
