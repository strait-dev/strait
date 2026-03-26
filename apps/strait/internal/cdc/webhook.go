package cdc

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

// WebhookReceiver handles CDC events pushed by Sequin webhook sinks.
// It dispatches messages to the same handlers used by the poll-based consumer,
// enabling sub-second CDC delivery alongside the existing pull fallback.
type WebhookReceiver struct {
	handlers  map[string]Handler
	publisher EventPublisher
	logger    *slog.Logger
}

// NewWebhookReceiver creates a new CDC webhook receiver.
func NewWebhookReceiver(publisher EventPublisher, logger *slog.Logger) *WebhookReceiver {
	if logger == nil {
		logger = slog.Default()
	}
	return &WebhookReceiver{
		handlers:  make(map[string]Handler),
		publisher: publisher,
		logger:    logger,
	}
}

// RegisterHandler adds a handler for a specific table name.
func (wr *WebhookReceiver) RegisterHandler(h Handler) {
	wr.handlers[h.Table()] = h
}

// ServeHTTP processes a CDC webhook push from Sequin.
// Returns 200 on success (Sequin marks delivered), 500 on failure (Sequin retries).
func (wr *WebhookReceiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil {
		http.Error(w, "invalid message format", http.StatusBadRequest)
		return
	}

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
				http.Error(w, "publish error", http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	// Fallback: inline handle.
	if handleErr := handler.Handle(r.Context(), msg); handleErr != nil {
		wr.logger.Error("cdc webhook: handle failed", "table", tableName, "error", handleErr)
		http.Error(w, "handler error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
