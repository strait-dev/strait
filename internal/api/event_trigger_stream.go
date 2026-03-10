package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
)

// handleEventTriggerStream streams SSE updates for a specific event trigger.
// The client receives updates whenever the trigger's status changes via the
// CDC pubsub channel for the trigger's project.
func (s *Server) handleEventTriggerStream(w http.ResponseWriter, r *http.Request) {
	eventKey := chi.URLParam(r, "eventKey")
	if eventKey == "" {
		respondError(w, r, http.StatusBadRequest, "event key is required")
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

	// Subscribe to the project's event_triggers CDC channel.
	channel := fmt.Sprintf("cdc:project:%s:event_triggers", trigger.ProjectID)
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
			// Filter: only forward messages about this specific trigger.
			var envelope struct {
				ID       string `json:"id"`
				EventKey string `json:"event_key"`
				Status   string `json:"status"`
			}
			if err := json.Unmarshal(msg, &envelope); err != nil {
				continue
			}
			if envelope.ID != trigger.ID && envelope.EventKey != trigger.EventKey {
				continue
			}

			fmt.Fprintf(w, "event: status\ndata: %s\n\n", msg)
			flusher.Flush()

			// Close stream when trigger reaches terminal state.
			if envelope.Status != domain.EventTriggerStatusWaiting {
				return
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
