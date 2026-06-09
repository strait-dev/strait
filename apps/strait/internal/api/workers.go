package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/domain"
)

// -- List workers --.

type ListWorkersInput struct {
	Limit  string `query:"limit"`
	Offset string `query:"offset"`
}

type ListWorkersOutput struct {
	Body struct {
		Data    []domain.Worker `json:"data"`
		Offset  int             `json:"offset"`
		HasMore bool            `json:"has_more"`
	}
}

func (s *Server) handleListWorkers(ctx context.Context, input *ListWorkersInput) (*ListWorkersOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	limit, offset, err := parseSimplePagination(input.Limit, input.Offset)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	workers, err := s.store.ListWorkers(ctx, projectID, "", limit+1, offset)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list workers")
	}

	hasMore := len(workers) > limit
	if hasMore {
		workers = workers[:limit]
	}

	out := &ListWorkersOutput{}
	out.Body.Data = workers
	out.Body.Offset = offset
	out.Body.HasMore = hasMore
	if out.Body.Data == nil {
		out.Body.Data = []domain.Worker{}
	}
	return out, nil
}

// -- Get worker --.

type GetWorkerInput struct {
	WorkerID string `path:"workerID"`
}

type GetWorkerOutput struct {
	Body *domain.Worker
}

func (s *Server) handleGetWorker(ctx context.Context, input *GetWorkerInput) (*GetWorkerOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	worker, err := s.store.GetWorker(ctx, input.WorkerID, projectID)
	if err != nil {
		// Return 404 regardless of the error to avoid existence leak across projects.
		return nil, huma.Error404NotFound("worker not found")
	}
	if worker == nil {
		return nil, huma.Error404NotFound("worker not found")
	}

	return &GetWorkerOutput{Body: worker}, nil
}

// -- Delete (force-disconnect) worker --.

type DeleteWorkerInput struct {
	WorkerID string `path:"workerID"`
}

type DeleteWorkerOutput struct {
	Body map[string]string
}

var workerDisconnectAckTimeout = 5 * time.Second

func workerDisconnectChannel(projectID, workerID string) string {
	return "worker:disconnect:" + projectID + ":" + workerID
}

func workerDisconnectAckChannel(projectID, workerID string) string {
	return "worker:disconnect_ack:" + projectID + ":" + workerID
}

func (s *Server) workerDisconnectAckTimeout() time.Duration {
	if s != nil && s.config != nil && s.config.WorkerDisconnectAckTimeout > 0 {
		return s.config.WorkerDisconnectAckTimeout
	}
	return workerDisconnectAckTimeout
}

func retryAfterSeconds(d time.Duration) string {
	if d <= 0 {
		return "1"
	}
	seconds := int((d + time.Second - time.Nanosecond) / time.Second)
	return strconv.Itoa(max(seconds, 1))
}

func (s *Server) handleDeleteWorker(ctx context.Context, input *DeleteWorkerInput) (*DeleteWorkerOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	// Verify the worker exists and belongs to this project (404 to avoid existence leak).
	worker, err := s.store.GetWorker(ctx, input.WorkerID, projectID)
	if err != nil || worker == nil {
		return nil, huma.Error404NotFound("worker not found")
	}

	// Publish disconnect signal to the owning replica's subscriber.
	// Without a publisher, the disconnect cannot be propagated cross-replica;
	// fail loud rather than silently returning success.
	if s.pubsub == nil {
		slog.Error("worker force-disconnect: pubsub publisher not configured",
			"worker_id", input.WorkerID,
		)
		return nil, huma.Error503ServiceUnavailable("worker force-disconnect unavailable: pubsub not configured")
	}
	ackChannel := workerDisconnectAckChannel(projectID, input.WorkerID)
	ackSub, subErr := s.pubsub.Subscribe(ctx, ackChannel)
	if subErr != nil || ackSub == nil {
		slog.Error("worker force-disconnect: ack subscription failed",
			"worker_id", input.WorkerID,
			"error", subErr,
		)
		return nil, huma.Error503ServiceUnavailable("worker force-disconnect unavailable: ack subscription failed")
	}
	defer ackSub.Close()

	channel := workerDisconnectChannel(projectID, input.WorkerID)
	if pubErr := s.pubsub.Publish(ctx, channel, []byte(input.WorkerID)); pubErr != nil {
		slog.Error("worker force-disconnect: publish failed",
			"worker_id", input.WorkerID,
			"error", pubErr,
		)
		return nil, huma.Error503ServiceUnavailable("failed to broadcast disconnect signal")
	}

	s.emitAuditEvent(ctx, domain.AuditActionWorkerForceDisconnected, "worker", input.WorkerID, map[string]any{
		"worker_id": input.WorkerID,
		"reason":    "operator_requested",
	})

	ackTimeout := s.workerDisconnectAckTimeout()
	timer := time.NewTimer(ackTimeout)
	defer timer.Stop()
	select {
	case msg, ok := <-ackSub.Ch:
		if !ok || string(msg) != input.WorkerID {
			reason := "ack_worker_mismatch"
			if !ok {
				reason = "ack_channel_closed"
			}
			s.emitAuditEvent(ctx, domain.AuditActionWorkerDeleteTimeout, "worker", input.WorkerID, map[string]any{
				"worker_id":  input.WorkerID,
				"timeout_ms": ackTimeout.Milliseconds(),
				"reason":     reason,
			})
			err := huma.Error503ServiceUnavailable("worker_disconnect_pending")
			return nil, huma.ErrorWithHeaders(err, http.Header{
				"Retry-After": []string{retryAfterSeconds(ackTimeout)},
			})
		}
		s.emitAuditEvent(ctx, domain.AuditActionWorkerDeleteAcked, "worker", input.WorkerID, map[string]any{
			"worker_id": input.WorkerID,
		})
		slog.Info("worker force-disconnect acknowledged",
			"worker_id", input.WorkerID,
			"project_id", projectID,
			"actor", actorFromContext(ctx),
		)
	case <-timer.C:
		s.emitAuditEvent(ctx, domain.AuditActionWorkerDeleteTimeout, "worker", input.WorkerID, map[string]any{
			"worker_id":  input.WorkerID,
			"timeout_ms": ackTimeout.Milliseconds(),
		})
		err := huma.Error503ServiceUnavailable("worker_disconnect_pending")
		return nil, huma.ErrorWithHeaders(err, http.Header{
			"Retry-After": []string{retryAfterSeconds(ackTimeout)},
		})
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return &DeleteWorkerOutput{Body: map[string]string{"status": "disconnected"}}, nil
}

// -- List worker tasks --.

type ListWorkerTasksInput struct {
	WorkerID string `path:"workerID"`
	Limit    string `query:"limit"`
	Offset   string `query:"offset"`
}

type ListWorkerTasksOutput struct {
	Body struct {
		Data    []domain.WorkerTask `json:"data"`
		Offset  int                 `json:"offset"`
		HasMore bool                `json:"has_more"`
	}
}

func (s *Server) handleListWorkerTasks(ctx context.Context, input *ListWorkerTasksInput) (*ListWorkerTasksOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	// Confirm worker belongs to this project before exposing tasks.
	worker, err := s.store.GetWorker(ctx, input.WorkerID, projectID)
	if err != nil || worker == nil {
		return nil, huma.Error404NotFound("worker not found")
	}

	limit, offset, err := parseSimplePagination(input.Limit, input.Offset)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	tasks, err := s.store.ListWorkerTasksByWorker(ctx, input.WorkerID, projectID, "", limit+1, offset)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list worker tasks")
	}

	hasMore := len(tasks) > limit
	if hasMore {
		tasks = tasks[:limit]
	}

	out := &ListWorkerTasksOutput{}
	out.Body.Data = tasks
	out.Body.Offset = offset
	out.Body.HasMore = hasMore
	if out.Body.Data == nil {
		out.Body.Data = []domain.WorkerTask{}
	}
	return out, nil
}

// parseSimplePagination parses limit and offset query parameters with sane defaults.
func parseSimplePagination(limitStr, offsetStr string) (int, int, error) {
	limit := defaultPageLimit
	if limitStr != "" {
		v, err := strconv.Atoi(limitStr)
		if err != nil || v <= 0 {
			return 0, 0, fmt.Errorf("limit must be a positive integer")
		}
		if v > maxPageLimit {
			v = maxPageLimit
		}
		limit = v
	}

	offset := 0
	if offsetStr != "" {
		v, err := strconv.Atoi(offsetStr)
		if err != nil || v < 0 {
			return 0, 0, fmt.Errorf("offset must be a non-negative integer")
		}
		offset = v
	}

	return limit, offset, nil
}
