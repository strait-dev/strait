package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"orchestrator/internal/domain"
)

func (s *Server) publishWorkflowRunHook(ctx context.Context, run *domain.WorkflowRun, from, to domain.WorkflowRunStatus, reason string) {
	if s.pubsub == nil || run == nil {
		return
	}

	payload, err := json.Marshal(map[string]any{
		"type":            "workflow_status_change",
		"workflow_run_id": run.ID,
		"workflow_id":     run.WorkflowID,
		"project_id":      run.ProjectID,
		"from":            string(from),
		"to":              string(to),
		"reason":          reason,
		"timestamp":       time.Now().UTC(),
	})
	if err != nil {
		return
	}

	if err := s.pubsub.Publish(ctx, fmt.Sprintf("workflow-run:%s", run.ID), payload); err != nil {
		//nolint:gosec // Structured logging with controlled key/value pairs.
		slog.Warn("failed to publish workflow run hook", "workflow_run_id", run.ID, "error", err)
	}
	if err := s.pubsub.Publish(ctx, fmt.Sprintf("workflow:%s:runs", run.WorkflowID), payload); err != nil {
		//nolint:gosec // Structured logging with controlled key/value pairs.
		slog.Warn("failed to publish workflow hook", "workflow_id", run.WorkflowID, "error", err)
	}
}
