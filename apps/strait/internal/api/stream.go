package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"strait/internal/billing"
)

// sseStreamOptions configures the shared SSE pump used by every
// run-scoped streaming handler. Only the per-route knobs (channel,
// event name, terminal-state guard) belong here; flusher / keepalive /
// max-duration / connection-cap behavior is identical for every SSE
// endpoint and stays in streamSSE.
type sseStreamOptions struct {
	// channel is the pubsub channel name to subscribe to, formatted
	// using the resolved run ID.
	channel string
	// eventName, when non-empty, is emitted as "event: <eventName>"
	// before each "data:" line. Used by the log stream so clients can
	// disambiguate from the default unnamed events.
	eventName string
	// rejectIfTerminal, when true, returns 410 Gone if the run is
	// already in a terminal state at handler entry. Log streams skip
	// this so historical runs can replay buffered logs.
	rejectIfTerminal bool
	// featureGate, when non-zero, requires the resolved run's project
	// to have the named billing feature enabled before any SSE work
	// begins.
	featureGate featureGate
}

// featureGate names a plan-gated billing feature that the SSE pump
// must verify before allocating connection budget. Zero value (empty
// feature) means no gate.
type featureGate struct {
	feature billing.Feature
	name    string
}

// streamSSE is the single SSE pump every handler routes through. It
// owns the connection-cap acquire/release, the Flusher type assertion,
// the SSE response headers, the pubsub Subscribe + cleanup, the
// SSEMaxConnDuration timeout, and the keepalive ticker. Callers supply
// only the channel name, the event tag, and whether terminal runs
// should be rejected up front.
func (s *Server) streamSSE(w http.ResponseWriter, r *http.Request, opts sseStreamOptions) {
	runID := chi.URLParam(r, "runID")

	run, err := s.getRunForAccess(r.Context(), runID)
	if err != nil {
		writeTypedError(w, r, err)
		return
	}
	s.emitInternalSecretBypassAuditIfProjectless(r.Context(), "stream_sse.project_match", "streamSSE", "run", run.ID)
	if opts.rejectIfTerminal && run.Status.IsTerminal() {
		respondError(w, r, http.StatusGone, "run already in terminal state")
		return
	}
	if opts.featureGate.feature != "" {
		if err := s.checkFeatureAllowed(r.Context(), run.ProjectID, opts.featureGate.feature, opts.featureGate.name); err != nil {
			writeTypedError(w, r, err)
			return
		}
	}

	if !s.acquireSSEConn(run.ProjectID) {
		respondError(w, r, http.StatusServiceUnavailable, "too many SSE connections")
		return
	}
	defer s.releaseSSEConn(run.ProjectID)

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
		slog.Error("pubsub not configured", "run_id", runID, "channel", opts.channel)
		if _, err := fmt.Fprintf(w, "event: error\ndata: {\"error\":\"streaming not available\"}\n\n"); err != nil {
			slog.Warn("failed to write SSE error", "run_id", runID, "error", err)
		}
		flusher.Flush()
		return
	}

	maxDuration := s.config.SSEMaxConnDuration
	if maxDuration <= 0 {
		maxDuration = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(r.Context(), maxDuration)
	defer cancel()

	channel := fmt.Sprintf(opts.channel, runID)
	sub, err := s.pubsub.Subscribe(ctx, channel)
	if err != nil {
		slog.Error("failed to subscribe", "run_id", runID, "channel", channel, "error", err)
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

	dataPrefix := "data: %s\n\n"
	if opts.eventName != "" {
		dataPrefix = "event: " + opts.eventName + "\ndata: %s\n\n"
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-sub.Ch:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, dataPrefix, msg); err != nil {
				slog.Warn("failed to write SSE data", "run_id", runID, "channel", channel, "error", err)
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				slog.Warn("failed to write SSE keepalive", "run_id", runID, "channel", channel, "error", err)
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) handleRunStream(w http.ResponseWriter, r *http.Request) {
	s.streamSSE(w, r, sseStreamOptions{
		channel:          "run:%s",
		rejectIfTerminal: true,
	})
}

// handleRunLogStream subscribes to the worker:log:<runID> pub/sub channel and
// forwards structured log lines from a worker-mode run to the HTTP client via
// SSE (text/event-stream). For HTTP-mode runs the channel will simply have no
// messages and the SSE connection will idle until the run completes.
func (s *Server) handleRunLogStream(w http.ResponseWriter, r *http.Request) {
	s.streamSSE(w, r, sseStreamOptions{
		channel:   "worker:log:%s",
		eventName: "log",
		featureGate: featureGate{
			feature: billing.FeatureLogStreaming,
			name:    "Log streaming",
		},
	})
}

// handleRunChunkStream forwards run stream chunks to frontend consumers via SSE.
func (s *Server) handleRunChunkStream(w http.ResponseWriter, r *http.Request) {
	s.streamSSE(w, r, sseStreamOptions{
		channel:          "run_stream:%s",
		rejectIfTerminal: true,
	})
}
