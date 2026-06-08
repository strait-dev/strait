package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sourcegraph/conc"

	"strait/internal/domain"
)

var (
	activityStreamEventPrefix    = []byte("event: activity\ndata: ")
	activityStreamFrameSuffix    = []byte("\n\n")
	activityStreamKeepaliveFrame = []byte(": keepalive\n\n")
)

func (s *Server) requireActivityStreamPermissions(next http.Handler) http.Handler {
	return s.requirePermission(domain.ScopeRunsRead)(
		s.requirePermission(domain.ScopeWorkflowsRead)(
			s.requirePermission(domain.ScopeJobsRead)(next),
		),
	)
}

func activityStreamJobRunsChannel(projectID string) string {
	return "cdc:project:" + projectID + ":job_runs"
}

func activityStreamWorkflowRunsChannel(projectID string) string {
	return "cdc:project:" + projectID + ":workflow_runs"
}

func activityStreamEventTriggersChannel(projectID string) string {
	return "cdc:project:" + projectID + ":event_triggers"
}

func activityStreamChannels(projectID string) [3]string {
	return [3]string{
		activityStreamJobRunsChannel(projectID),
		activityStreamWorkflowRunsChannel(projectID),
		activityStreamEventTriggersChannel(projectID),
	}
}

func writeActivityStreamEvent(w io.Writer, msg []byte) error {
	if _, err := w.Write(activityStreamEventPrefix); err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	_, err := w.Write(activityStreamFrameSuffix)
	return err
}

func writeActivityStreamKeepalive(w io.Writer) error {
	_, err := w.Write(activityStreamKeepaliveFrame)
	return err
}

// handleProjectActivityStream serves a real-time SSE stream of all CDC events
// for a project. Subscribes to job_runs, workflow_runs, and event_triggers
// channels and merges them into a single stream.
func (s *Server) handleProjectActivityStream(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	// Tenant isolation: SSE long-lived handlers cannot rely on RLS, so we
	// must verify the URL projectID matches the caller's authenticated project.
	// 404 on mismatch to avoid cross-tenant existence disclosure.
	if callerProjectID := projectIDFromContext(r.Context()); callerProjectID == "" || projectID != callerProjectID {
		respondError(w, r, http.StatusNotFound, "project not found")
		return
	}
	if environmentIDFromContext(r.Context()) != "" {
		respondError(w, r, http.StatusForbidden, "activity stream requires a project-wide key")
		return
	}

	if s.pubsub == nil {
		respondError(w, r, http.StatusServiceUnavailable, "real-time streaming not available")
		return
	}

	if !s.acquireSSEConn(projectID) {
		respondError(w, r, http.StatusServiceUnavailable, "too many SSE connections")
		return
	}
	defer s.releaseSSEConn(projectID)

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, r, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Subscribe to all CDC channels for this project.
	channels := activityStreamChannels(projectID)

	merged := make(chan []byte, 64)

	// Apply max connection duration timeout.
	maxDuration := s.config.SSEMaxConnDuration
	if maxDuration <= 0 {
		maxDuration = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(r.Context(), maxDuration)
	defer cancel()

	var fanoutWG conc.WaitGroup
	for _, ch := range channels {
		sub, err := s.pubsub.Subscribe(ctx, ch)
		if err != nil {
			slog.Warn("activity stream: subscribe failed", "channel", ch, "project_id", projectID, "error", err)
			continue
		}
		defer sub.Close()
		fanoutWG.Go(func() {
			for {
				select {
				case msg, ok := <-sub.Ch:
					if !ok {
						return
					}
					select {
					case merged <- msg:
					case <-ctx.Done():
						return
					}
				case <-ctx.Done():
					return
				}
			}
		})
	}

	// SSE event loop with keepalive.
	keepalive := s.config.SSEKeepaliveInterval
	if keepalive <= 0 {
		keepalive = 15 * time.Second
	}
	ticker := time.NewTicker(keepalive)
	defer ticker.Stop()

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case msg, ok := <-merged:
			if !ok {
				break loop
			}
			if err := writeActivityStreamEvent(w, msg); err != nil {
				slog.Warn("activity stream: write failed", "project_id", projectID, "error", err)
				break loop
			}
			flusher.Flush()
		case <-ticker.C:
			if err := writeActivityStreamKeepalive(w); err != nil {
				slog.Warn("activity stream: keepalive write failed", "project_id", projectID, "error", err)
				break loop
			}
			flusher.Flush()
		}
	}

	// Cancel the fanout goroutines and wait for them to drain before the deferred
	// sub.Close()/releaseSSEConn run. conc.WaitGroup only re-raises a recovered
	// panic from a fanout goroutine on Wait(); without this barrier a panic would
	// be silently discarded and goroutines could outlive the handler. cancel()
	// (also deferred, idempotent) is called explicitly so Wait() cannot block.
	cancel()
	fanoutWG.Wait()
}
