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
			respondError(w, http.StatusNotFound, "run not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get run")
		return
	}

	if run.Status.IsTerminal() {
		respondError(w, http.StatusGone, "run already in terminal state")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	if s.pubsub == nil {
		slog.Error("pubsub not configured", "run_id", runID) //nolint:gosec // structured logging sanitizes values
		_, _ = fmt.Fprintf(w, "event: error\ndata: {\"error\":\"streaming not available\"}\n\n")
		flusher.Flush()
		return
	}

	channel := fmt.Sprintf("run:%s", runID)
	sub, err := s.pubsub.Subscribe(r.Context(), channel)
	if err != nil {
		slog.Error("failed to subscribe", "run_id", runID, "error", err) //nolint:gosec // structured logging sanitizes values
		_, _ = fmt.Fprintf(w, "event: error\ndata: {\"error\":\"failed to subscribe\"}\n\n")
		flusher.Flush()
		return
	}
	defer sub.Close()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-sub.Ch:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			_, _ = fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
