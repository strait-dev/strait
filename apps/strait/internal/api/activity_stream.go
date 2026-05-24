package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sourcegraph/conc"

	"strait/internal/domain"
)

func (s *Server) requireActivityStreamPermissions(next http.Handler) http.Handler {
	return s.requirePermission(domain.ScopeRunsRead)(
		s.requirePermission(domain.ScopeWorkflowsRead)(
			s.requirePermission(domain.ScopeJobsRead)(next),
		),
	)
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
	channels := []string{
		fmt.Sprintf("cdc:project:%s:job_runs", projectID),
		fmt.Sprintf("cdc:project:%s:workflow_runs", projectID),
		fmt.Sprintf("cdc:project:%s:event_triggers", projectID),
	}

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

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-merged:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: activity\ndata: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
