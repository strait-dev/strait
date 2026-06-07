package cdc

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	maxWebhookBodyBytes = 1 << 20
	webhookDedupeTTL    = 10 * time.Minute
)

// WebhookReceiver handles CDC events pushed by Sequin webhook sinks.
// It dispatches messages to the same handlers used by the poll-based consumer,
// enabling sub-second CDC delivery alongside the existing pull fallback.
type WebhookReceiver struct {
	handlers           map[string]Handler
	additionalHandlers map[string][]Handler
	publisher          EventPublisher
	logger             *slog.Logger
	secret             string
	dedupeTTL          time.Duration
	sharedDedupe       *SharedDedupeStore
	seenMu             sync.Mutex
	seen               map[string]time.Time
}

// WebhookReceiverOption configures a CDC webhook receiver.
type WebhookReceiverOption func(*WebhookReceiver)

// WithWebhookSecret enables HMAC-SHA256 request body verification for Sequin
// push delivery. Empty secrets leave verification disabled so test/dev pull-only
// deployments can instantiate a receiver without synthetic signatures.
func WithWebhookSecret(secret string) WebhookReceiverOption {
	return func(wr *WebhookReceiver) {
		wr.secret = strings.TrimSpace(secret)
	}
}

// WithWebhookDedupeTTL overrides duplicate suppression TTL for tests.
func WithWebhookDedupeTTL(ttl time.Duration) WebhookReceiverOption {
	return func(wr *WebhookReceiver) {
		wr.dedupeTTL = ttl
	}
}

func WithWebhookSharedDedupe(store *SharedDedupeStore) WebhookReceiverOption {
	return func(wr *WebhookReceiver) {
		wr.sharedDedupe = store
	}
}

// NewWebhookReceiver creates a new CDC webhook receiver.
func NewWebhookReceiver(publisher EventPublisher, logger *slog.Logger, opts ...WebhookReceiverOption) *WebhookReceiver {
	if logger == nil {
		logger = slog.Default()
	}
	wr := &WebhookReceiver{
		handlers:           make(map[string]Handler),
		additionalHandlers: make(map[string][]Handler),
		publisher:          publisher,
		logger:             logger,
		dedupeTTL:          webhookDedupeTTL,
		seen:               make(map[string]time.Time),
	}
	for _, opt := range opts {
		opt(wr)
	}
	return wr
}

// RegisterHandler adds the primary handler for a specific table name.
func (wr *WebhookReceiver) RegisterHandler(h Handler) {
	wr.handlers[h.Table()] = h
}

// RegisterAdditionalHandler adds a secondary handler for a table.
// Additional handlers run after the primary handler for CDC-driven
// side effects (webhook delivery, notifications, audit log, etc.).
func (wr *WebhookReceiver) RegisterAdditionalHandler(h Handler) {
	wr.additionalHandlers[h.Table()] = append(wr.additionalHandlers[h.Table()], h)
}

// ServeHTTP processes a CDC webhook push from Sequin.
// Returns 200 on success (Sequin marks delivered), 500 on failure (Sequin retries).
func (wr *WebhookReceiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBodyBytes))
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	if !wr.verifySignature(r, body) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil {
		http.Error(w, "invalid message format", http.StatusBadRequest)
		return
	}
	if err := validateWebhookMessage(msg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dedupeKey, claimed := wr.claimDedupe(r.Context(), msg)
	if !claimed {
		w.WriteHeader(http.StatusOK)
		return
	}
	// processed gates the dedupe claim: it is only set true once the primary
	// AND every additional handler have succeeded. Any earlier failure leaves
	// processed false so the deferred releaseDedupe runs, freeing the claim and
	// letting Sequin redeliver. CDC delivery is at-least-once and these handlers
	// are the same idempotent handlers used by the poll-based consumer, so a
	// redelivery re-running them is safe; retaining the claim on partial failure
	// would instead drop the side effect permanently.
	processed := false
	defer func() {
		if !processed {
			wr.releaseDedupe(r.Context(), dedupeKey)
		}
	}()

	tableName := msg.Metadata.TableName
	handler, ok := wr.handlers[tableName]
	if !ok {
		wr.logger.Warn("cdc webhook: no handler for table", "table", tableName)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Try batch collection if handler supports it and publisher is available.
	if ch, ok := handler.(CollectableHandler); ok && wr.publisher != nil {
		pubMsg, collectErr := ch.Collect(r.Context(), msg)
		if collectErr != nil {
			wr.logger.Error("cdc webhook: collect failed", "table", tableName, "error", collectErr)
			http.Error(w, "handler error", http.StatusInternalServerError)
			return
		}
		if pubMsg != nil {
			if pubErr := wr.publisher.Publish(r.Context(), pubMsg.Channel, pubMsg.Data); pubErr != nil {
				wr.logger.Error("cdc webhook: publish failed", "table", tableName, "channel", pubMsg.Channel, "error", pubErr)
			}
		}
	} else {
		// Fallback: inline handle.
		if handleErr := handler.Handle(r.Context(), msg); handleErr != nil {
			wr.logger.Error("cdc webhook: handle failed", "table", tableName, "error", handleErr)
			http.Error(w, "handler error", http.StatusInternalServerError)
			return
		}
	}

	// Run additional handlers (webhook delivery, notifications, audit, etc.).
	// Always runs regardless of whether the primary handler used Collect or Handle.
	// processed stays false until this loop completes: an additional-handler
	// failure must release the dedupe claim so Sequin redelivers, otherwise the
	// side effect is silently dropped.
	for _, ah := range wr.additionalHandlers[tableName] {
		if ahErr := ah.Handle(r.Context(), msg); ahErr != nil {
			wr.logger.Error("cdc webhook: additional handler failed",
				"table", tableName, "handler", ah.Table(), "error", ahErr)
			http.Error(w, "handler error", http.StatusInternalServerError)
			return
		}
	}

	processed = true
	w.WriteHeader(http.StatusOK)
}

func (wr *WebhookReceiver) verifySignature(r *http.Request, body []byte) bool {
	if wr.secret == "" {
		return true
	}
	got := firstHeader(r,
		"X-Sequin-Signature",
		"Sequin-Signature",
		"X-Hub-Signature-256",
		"X-Signature",
	)
	if got == "" {
		return false
	}
	got = strings.TrimSpace(got)
	if trimmed, ok := strings.CutPrefix(got, "sha256="); ok {
		got = trimmed
	}
	gotBytes, err := hex.DecodeString(got)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(wr.secret))
	_, _ = mac.Write(body)
	return hmac.Equal(gotBytes, mac.Sum(nil))
}

func firstHeader(r *http.Request, names ...string) string {
	for _, name := range names {
		if value := r.Header.Get(name); value != "" {
			return value
		}
	}
	return ""
}

func validateWebhookMessage(msg Message) error {
	if msg.Metadata.TableName == "" {
		return errors.New("metadata.table_name is required")
	}
	switch msg.Action {
	case ActionInsert, ActionUpdate, ActionDelete:
		return nil
	default:
		return errors.New("invalid action")
	}
}

func (wr *WebhookReceiver) dedupeKey(msg Message) string {
	if msg.Metadata.IdempotencyKey != "" {
		return msg.Metadata.IdempotencyKey
	}
	return msg.AckID
}

func (wr *WebhookReceiver) claimDedupe(ctx context.Context, msg Message) (string, bool) {
	key := wr.dedupeKey(msg)
	if key == "" || wr.dedupeTTL <= 0 {
		return "", true
	}
	now := time.Now()
	wr.seenMu.Lock()
	wr.pruneSeenLocked(now)
	expiresAt, ok := wr.seen[key]
	if ok && now.Before(expiresAt) {
		wr.seenMu.Unlock()
		return key, false
	}
	wr.seen[key] = now.Add(wr.dedupeTTL)
	shared := wr.sharedDedupe
	wr.seenMu.Unlock()
	if shared != nil {
		claimed, err := shared.Claim(ctx, "cdc_webhook:"+key)
		if err != nil {
			recordSharedDedupeFallback("webhook")
			wr.logger.Warn("cdc webhook dedupe: redis unavailable, falling back to local dedupe", "error", err)
			return key, true
		}
		if !claimed {
			// Another node holds the authoritative claim, so this node will not
			// process the message and the caller responds 200 before installing
			// the releaseDedupe defer. Drop the optimistic local entry written
			// above; otherwise it lingers for the full dedupeTTL and silently
			// suppresses a later redelivery routed here after the winning node
			// fails and releases the shared claim.
			wr.seenMu.Lock()
			delete(wr.seen, key)
			wr.seenMu.Unlock()
		}
		return key, claimed
	}
	return key, true
}

func (wr *WebhookReceiver) releaseDedupe(ctx context.Context, key string) {
	if key == "" || wr.dedupeTTL <= 0 {
		return
	}
	wr.seenMu.Lock()
	delete(wr.seen, key)
	shared := wr.sharedDedupe
	wr.seenMu.Unlock()
	if shared != nil {
		shared.Release(ctx, "cdc_webhook:"+key)
	}
}

func (wr *WebhookReceiver) pruneSeenLocked(now time.Time) {
	for key, expiresAt := range wr.seen {
		if !now.Before(expiresAt) {
			delete(wr.seen, key)
		}
	}
}
