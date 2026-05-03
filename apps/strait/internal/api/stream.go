package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func (s *Server) handleRunStream(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

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
		slog.Error("pubsub not configured", "run_id", runID)
		if _, err := fmt.Fprintf(w, "event: error\ndata: {\"error\":\"streaming not available\"}\n\n"); err != nil {
			slog.Warn("failed to write SSE error", "run_id", runID, "error", err)
		}
		flusher.Flush()
		return
	}

	channel := fmt.Sprintf("run:%s", runID)
	sub, err := s.pubsub.Subscribe(r.Context(), channel)
	if err != nil {
		slog.Error("failed to subscribe", "run_id", runID, "error", err)
		if _, err := fmt.Fprintf(w, "event: error\ndata: {\"error\":\"failed to subscribe\"}\n\n"); err != nil {
			slog.Warn("failed to write SSE subscribe error", "run_id", runID, "error", err)
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
			if _, err := fmt.Fprintf(w, "data: %s\n\n", msg); err != nil {
				slog.Warn("failed to write SSE data", "run_id", runID, "error", err)
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				slog.Warn("failed to write SSE keepalive", "run_id", runID, "error", err)
				return
			}
			flusher.Flush()
		}
	}
}

// handleRunLogStream subscribes to the worker:log:<runID> pub/sub channel and
// forwards structured log lines from a worker-mode run to the HTTP client via
// SSE (text/event-stream). For HTTP-mode runs the channel will simply have no
// messages and the SSE connection will idle until the run completes.
func (s *Server) handleRunLogStream(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	_, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, r, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, r, http.StatusInternalServerError, "failed to get run")
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
		slog.Error("pubsub not configured for log stream", "run_id", runID)
		if _, err := fmt.Fprintf(w, "event: error\ndata: {\"error\":\"streaming not available\"}\n\n"); err != nil {
			slog.Warn("failed to write SSE error", "run_id", runID, "error", err)
		}
		flusher.Flush()
		return
	}

	channel := fmt.Sprintf("worker:log:%s", runID)
	sub, err := s.pubsub.Subscribe(r.Context(), channel)
	if err != nil {
		slog.Error("failed to subscribe to log channel", "run_id", runID, "error", err)
		if _, err := fmt.Fprintf(w, "event: error\ndata: {\"error\":\"failed to subscribe\"}\n\n"); err != nil {
			slog.Warn("failed to write SSE subscribe error", "run_id", runID, "error", err)
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
			if _, err := fmt.Fprintf(w, "event: log\ndata: %s\n\n", msg); err != nil {
				slog.Warn("failed to write SSE log data", "run_id", runID, "error", err)
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				slog.Warn("failed to write SSE log keepalive", "run_id", runID, "error", err)
				return
			}
			flusher.Flush()
		}
	}
}

// handleRunLLMStream forwards LLM stream chunks to frontend consumers via SSE.
func (s *Server) handleRunLLMStream(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

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
		if _, err := fmt.Fprintf(w, "event: error\ndata: {\"error\":\"streaming not available\"}\n\n"); err != nil {
			slog.Warn("failed to write SSE error", "run_id", runID, "error", err)
		}
		flusher.Flush()
		return
	}

	channel := "run_stream:" + runID
	sub, err := s.pubsub.Subscribe(r.Context(), channel)
	if err != nil {
		if _, err := fmt.Fprintf(w, "event: error\ndata: {\"error\":\"failed to subscribe\"}\n\n"); err != nil {
			slog.Warn("failed to write SSE subscribe error", "run_id", runID, "error", err)
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
			if _, err := fmt.Fprintf(w, "data: %s\n\n", msg); err != nil {
				slog.Warn("failed to write LLM SSE data", "run_id", runID, "error", err)
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				slog.Warn("failed to write LLM SSE keepalive", "run_id", runID, "error", err)
				return
			}
			flusher.Flush()
		}
	}
}
