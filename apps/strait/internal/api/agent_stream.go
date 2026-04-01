package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// handleAgentRunEvents streams typed agent execution events via SSE.
// Events include tool_call, usage, checkpoint, stream, state_change,
// complete, and fail. Uses the existing pubsub infrastructure.
func (s *Server) handleAgentRunEvents(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentID")
	runID := chi.URLParam(r, "runID")

	// Verify the agent belongs to the project.
	projectID := projectIDFromContext(r.Context())
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project context is required")
		return
	}

	if s.agentService == nil {
		respondError(w, r, http.StatusServiceUnavailable, "agent service unavailable")
		return
	}
	if _, agentErr := s.agentService.GetAgent(r.Context(), projectID, agentID); agentErr != nil {
		respondError(w, r, http.StatusNotFound, "agent not found")
		return
	}

	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get run")
		return
	}

	if run.Status.IsTerminal() {
		respondError(w, r, http.StatusGone, "run already in terminal state")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, r, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	if s.pubsub == nil {
		slog.Error("pubsub not configured for agent event stream", "run_id", runID)
		if _, writeErr := fmt.Fprintf(w, "event: error\ndata: {\"error\":\"streaming not available\"}\n\n"); writeErr != nil {
			slog.Warn("failed to write SSE error", "run_id", runID, "error", writeErr)
		}
		flusher.Flush()
		return
	}

	channel := fmt.Sprintf("run:%s", runID)
	sub, subErr := s.pubsub.Subscribe(r.Context(), channel)
	if subErr != nil {
		slog.Error("failed to subscribe to agent run events", "run_id", runID, "error", subErr)
		if _, writeErr := fmt.Fprintf(w, "event: error\ndata: {\"error\":\"failed to subscribe\"}\n\n"); writeErr != nil {
			slog.Warn("failed to write SSE subscribe error", "run_id", runID, "error", writeErr)
		}
		flusher.Flush()
		return
	}
	defer sub.Close()

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
			msgStr := string(msg)
			eventType := extractEventType(msgStr)
			if eventType != "" {
				if _, writeErr := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, msgStr); writeErr != nil {
					slog.Warn("failed to write typed SSE event", "run_id", runID, "error", writeErr)
					return
				}
			} else {
				if _, writeErr := fmt.Fprintf(w, "data: %s\n\n", msgStr); writeErr != nil {
					slog.Warn("failed to write SSE data", "run_id", runID, "error", writeErr)
					return
				}
			}
			flusher.Flush()
		case <-ticker.C:
			if _, writeErr := fmt.Fprintf(w, ": keepalive\n\n"); writeErr != nil {
				slog.Warn("failed to write SSE keepalive", "run_id", runID, "error", writeErr)
				return
			}
			flusher.Flush()
		}
	}
}

// extractEventType reads the "type" field from a JSON message to use as the SSE event label.
func extractEventType(msg string) string {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(msg), &envelope); err != nil {
		return ""
	}
	return envelope.Type
}
