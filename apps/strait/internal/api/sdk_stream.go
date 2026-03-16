package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleSDKStreamChunk(w http.ResponseWriter, r *http.Request) {
	applySDKResponseHeaders(r.Context(), w)
	runID := chi.URLParam(r, "runID")

	var req struct {
		Chunk    string `json:"chunk"`
		StreamID string `json:"stream_id,omitempty"`
		Done     bool   `json:"done,omitempty"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if s.pubsub == nil {
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	streamID := req.StreamID
	if streamID == "" {
		streamID = "default"
	}

	payload, err := json.Marshal(map[string]any{
		"type":      "stream_chunk",
		"chunk":     req.Chunk,
		"stream_id": streamID,
		"done":      req.Done,
		"timestamp": time.Now().UTC(),
	})
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to marshal chunk")
		return
	}

	channel := "run_stream:" + runID
	if err := s.pubsub.Publish(r.Context(), channel, payload); err != nil {
		slog.Warn("failed to publish stream chunk", "run_id", runID, "error", err)
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
