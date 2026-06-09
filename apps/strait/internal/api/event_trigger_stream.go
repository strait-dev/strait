package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
)

var eventTriggerStatusFramePrefix = sseDataFramePrefix("status")

func writeEventTriggerStatusFrame(w io.Writer, msg []byte) error {
	return writeSSEDataFrame(w, eventTriggerStatusFramePrefix, msg)
}

func writeEventTriggerKeepaliveFrame(w io.Writer) error {
	return writeSSEKeepaliveFrame(w)
}

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

	projectID := projectIDFromContext(r.Context())
	if projectID == "" && !isInternalCaller(r.Context()) {
		respondError(w, r, http.StatusBadRequest, "project context is required -- authenticate with an API key")
		return
	}

	trigger, err := s.resolveEventTriggerByKey(r.Context(), eventKey, projectID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get event trigger")
		return
	}
	if trigger == nil {
		respondError(w, r, http.StatusNotFound, "event trigger not found")
		return
	}
	if err := requireProjectMatch(r.Context(), trigger.ProjectID); err != nil {
		respondError(w, r, http.StatusNotFound, "event trigger not found")
		return
	}

	if err := requireEnvironmentMatch(r.Context(), trigger.EnvironmentID); err != nil {
		respondError(w, r, http.StatusNotFound, "event trigger not found")
		return
	}
	s.emitInternalSecretBypassAuditIfProjectless(r.Context(), "event_trigger_stream.project_match", "handleEventTriggerStream", "event_trigger", trigger.ID)

	// If already terminal, return the final state as a single SSE message.
	if trigger.Status != domain.EventTriggerStatusWaiting {
		s.writeTerminalTriggerSSE(w, trigger)
		return
	}

	if !s.acquireSSEConn(trigger.ProjectID) {
		respondError(w, r, http.StatusServiceUnavailable, "too many SSE connections")
		return
	}
	defer s.releaseSSEConn(trigger.ProjectID)

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, r, http.StatusInternalServerError, "streaming not supported")
		return
	}

	if s.pubsub == nil {
		respondError(w, r, http.StatusServiceUnavailable, "streaming not available")
		return
	}

	// Apply max connection duration timeout.
	maxDuration := s.config.SSEMaxConnDuration
	if maxDuration <= 0 {
		maxDuration = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(r.Context(), maxDuration)
	defer cancel()

	// Subscribe to the trigger-specific channel (same pattern as run:{runID}).
	channel := eventTriggerChannel(trigger.ID)
	sub, err := s.pubsub.Subscribe(ctx, channel)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to subscribe")
		return
	}
	defer sub.Close()

	trigger, err = s.resolveEventTriggerByKey(r.Context(), eventKey, projectID)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to get event trigger")
		return
	}
	if trigger == nil {
		respondError(w, r, http.StatusNotFound, "event trigger not found")
		return
	}
	if err := requireProjectMatch(r.Context(), trigger.ProjectID); err != nil {
		respondError(w, r, http.StatusNotFound, "event trigger not found")
		return
	}
	if err := requireEnvironmentMatch(r.Context(), trigger.EnvironmentID); err != nil {
		respondError(w, r, http.StatusNotFound, "event trigger not found")
		return
	}
	if trigger.Status != domain.EventTriggerStatusWaiting {
		s.writeTerminalTriggerSSE(w, trigger)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Send initial state.
	if data, err := json.Marshal(trigger); err == nil {
		_ = writeEventTriggerStatusFrame(w, data)
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
		case <-ctx.Done():
			return
		case msg, ok := <-sub.Ch:
			if !ok {
				return
			}
			envelope, ok := eventTriggerStreamEnvelopeAllowed(ctx, trigger, msg)
			if !ok {
				continue
			}
			if err := writeEventTriggerStatusFrame(w, stripSSENewlines(msg)); err != nil {
				return
			}
			flusher.Flush()

			// Close stream when trigger reaches terminal state.
			if envelope.Status != "" && envelope.Status != domain.EventTriggerStatusWaiting {
				return
			}
		case <-ticker.C:
			if err := writeEventTriggerKeepaliveFrame(w); err != nil {
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
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	if data, err := json.Marshal(trigger); err == nil {
		_ = writeEventTriggerStatusFrame(w, data)
	}
	flusher.Flush()
}

type eventTriggerStreamEnvelope struct {
	ID            string `json:"id"`
	ProjectID     string `json:"project_id"`
	EnvironmentID string `json:"environment_id"`
	Status        string `json:"status"`
}

func eventTriggerStreamEnvelopeAllowed(ctx context.Context, trigger *domain.EventTrigger, msg []byte) (eventTriggerStreamEnvelope, bool) {
	var envelope eventTriggerStreamEnvelope
	if err := json.Unmarshal(msg, &envelope); err != nil {
		return envelope, false
	}
	if envelope.ID != trigger.ID {
		return envelope, false
	}
	if envelope.ProjectID == "" || envelope.ProjectID != trigger.ProjectID {
		return envelope, false
	}
	if envelope.EnvironmentID != trigger.EnvironmentID {
		return envelope, false
	}
	callerEnv := environmentIDFromContext(ctx)
	if callerEnv != "" {
		if envelope.EnvironmentID == "" || envelope.EnvironmentID != callerEnv {
			return envelope, false
		}
		return envelope, true
	}
	return envelope, true
}

// publishTriggerStatusChange publishes a status change to the trigger-specific
// Redis pubsub channel for real-time SSE delivery. Non-fatal on error.
func (s *Server) publishTriggerStatusChange(ctx context.Context, trigger *domain.EventTrigger) {
	if s.pubsub == nil {
		return
	}

	payload, err := marshalTriggerStatusChangePayload(trigger, time.Now().UTC())
	if err != nil {
		slog.Warn("failed to marshal trigger status payload", "trigger_id", trigger.ID, "error", err)
		return
	}

	channel := eventTriggerChannel(trigger.ID)
	if err := s.pubsub.Publish(ctx, channel, payload); err != nil {
		slog.Warn("failed to publish trigger status change", "trigger_id", trigger.ID, "channel", channel, "error", err)
	}
}

func eventTriggerChannel(triggerID string) string {
	return "event_trigger:" + triggerID
}

func marshalTriggerStatusChangePayload(trigger *domain.EventTrigger, timestamp time.Time) ([]byte, error) {
	out := make([]byte, 0, 192+
		len(trigger.ID)+
		len(trigger.EventKey)+
		len(trigger.Status)+
		len(trigger.ProjectID)+
		len(trigger.EnvironmentID)+
		len(trigger.SourceType)+
		len(trigger.Error),
	)
	out = append(out, `{"id":`...)
	out = strconv.AppendQuote(out, trigger.ID)
	out = append(out, `,"event_key":`...)
	out = strconv.AppendQuote(out, trigger.EventKey)
	out = append(out, `,"status":`...)
	out = strconv.AppendQuote(out, trigger.Status)
	out = append(out, `,"project_id":`...)
	out = strconv.AppendQuote(out, trigger.ProjectID)
	out = append(out, `,"environment_id":`...)
	out = strconv.AppendQuote(out, trigger.EnvironmentID)
	out = append(out, `,"source_type":`...)
	out = strconv.AppendQuote(out, trigger.SourceType)
	out = append(out, `,"received_at":`...)
	if trigger.ReceivedAt == nil {
		out = append(out, "null"...)
	} else {
		out = appendTriggerStatusJSONTime(out, *trigger.ReceivedAt)
	}
	out = append(out, `,"error":`...)
	out = strconv.AppendQuote(out, trigger.Error)
	out = append(out, `,"timestamp":`...)
	out = appendTriggerStatusJSONTime(out, timestamp)
	out = append(out, '}')
	return out, nil
}

func appendTriggerStatusJSONTime(out []byte, timestamp time.Time) []byte {
	out = append(out, '"')
	out = timestamp.AppendFormat(out, time.RFC3339Nano)
	out = append(out, '"')
	return out
}
