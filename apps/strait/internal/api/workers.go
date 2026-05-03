package api

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

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
	channel := fmt.Sprintf("worker:disconnect:%s", input.WorkerID)
	if pubErr := s.pubsub.Publish(ctx, channel, []byte(input.WorkerID)); pubErr != nil {
		slog.Warn("worker force-disconnect: publish failed",
			"worker_id", input.WorkerID,
			"error", pubErr,
		)
	}

	s.emitAuditEvent(ctx, domain.AuditActionWorkerForceDisconnected, "worker", input.WorkerID, map[string]any{
		"worker_id": input.WorkerID,
	})

	slog.Info("worker force-disconnect requested",
		"worker_id", input.WorkerID,
		"project_id", projectID,
		"actor", actorFromContext(ctx),
	)

	return &DeleteWorkerOutput{Body: map[string]string{"status": "disconnect_requested"}}, nil
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

	tasks, err := s.store.ListWorkerTasksByWorker(ctx, input.WorkerID, "", limit+1, offset)
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
