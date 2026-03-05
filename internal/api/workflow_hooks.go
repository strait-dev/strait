package api

import (
	"context"
	"encoding/json"
	"fmt"
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

	_ = s.pubsub.Publish(ctx, fmt.Sprintf("workflow-run:%s", run.ID), payload)
	_ = s.pubsub.Publish(ctx, fmt.Sprintf("workflow:%s:runs", run.WorkflowID), payload)
}
