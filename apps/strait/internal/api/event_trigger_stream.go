package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
)

// handleEventTriggerStream streams SSE updates for a specific event trigger.
// Subscribes to the trigger-specific channel ("event_trigger:{id}") which receives
// direct publishes from send/cancel handlers for sub-millisecond delivery.
// CDC also publishes to the project-level channel as a reliable catch-all;
// we subscribe to the trigger-specific channel for targeted, low-latency updates.
func (s *Server) handleEventTriggerStream(w http.ResponseWriter, r *http.Request) {
	eventKey := chi.URLParam(r, "eventKey")
	if errMsg := validateEventKey(eventKey); errMsg != "" {
		respondError(w, r, http.StatusBadRequest, errMsg)
		return
	}

	trigger, err := s.store.GetEventTriggerByEventKey(r.Context(), eventKey)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get event trigger")
		return
	}
	if trigger == nil {
		respondError(w, r, http.StatusNotFound, "event trigger not found")
		return
	}

	if projectID := projectIDFromContext(r.Context()); projectID != "" && trigger.ProjectID != projectID {
		respondError(w, r, http.StatusNotFound, "event trigger not found")
		return
	}

	// If already terminal, return the final state as a single SSE message.
	if trigger.Status != domain.EventTriggerStatusWaiting {
		s.writeTerminalTriggerSSE(w, trigger)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, r, http.StatusInternalServerError, "streaming not supported")
		return
	}

	if s.pubsub == nil {
		respondError(w, r, http.StatusServiceUnavailable, "streaming not available")
		return
	}

	// Subscribe to the trigger-specific channel (same pattern as run:{runID}).
	channel := fmt.Sprintf("event_trigger:%s", trigger.ID)
	sub, err := s.pubsub.Subscribe(r.Context(), channel)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to subscribe")
		return
	}
	defer sub.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Send initial state.
	if data, err := json.Marshal(trigger); err == nil {
		fmt.Fprintf(w, "event: status\ndata: %s\n\n", data)
		flusher.Flush()
	}

	keepalive := s.config.SSEKeepaliveInterval
	if keepalive <= 0 {
		keepalive = 15 * time.Second
	}
	ticker := time.NewTicker(keepalive)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-sub.Ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: status\ndata: %s\n\n", msg)
			flusher.Flush()

			// Close stream when trigger reaches terminal state.
			var envelope struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(msg, &envelope); err == nil {
				if envelope.Status != "" && envelope.Status != domain.EventTriggerStatusWaiting {
					return
				}
			}
		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeTerminalTriggerSSE sends a single SSE message for a terminal trigger and closes.
func (s *Server) writeTerminalTriggerSSE(w http.ResponseWriter, trigger *domain.EventTrigger) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, nil, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	if data, err := json.Marshal(trigger); err == nil {
		fmt.Fprintf(w, "event: status\ndata: %s\n\n", data)
	}
	flusher.Flush()
}

// publishTriggerStatusChange publishes a status change to the trigger-specific
// Redis pubsub channel for real-time SSE delivery. Non-fatal on error.
func (s *Server) publishTriggerStatusChange(ctx context.Context, trigger *domain.EventTrigger) {
	if s.pubsub == nil {
		return
	}

	payload, err := json.Marshal(map[string]any{
		"id":          trigger.ID,
		"event_key":   trigger.EventKey,
		"status":      trigger.Status,
		"project_id":  trigger.ProjectID,
		"source_type": trigger.SourceType,
		"received_at": trigger.ReceivedAt,
		"error":       trigger.Error,
		"timestamp":   time.Now().UTC(),
	})
	if err != nil {
		slog.Warn("failed to marshal trigger status payload", "trigger_id", trigger.ID, "error", err)
		return
	}

	channel := fmt.Sprintf("event_trigger:%s", trigger.ID)
	if err := s.pubsub.Publish(ctx, channel, payload); err != nil {
		slog.Warn("failed to publish trigger status change", "trigger_id", trigger.ID, "channel", channel, "error", err)
	}
}
