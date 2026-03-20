package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/domain"
)

func (s *Server) publishWorkflowRunHook(ctx context.Context, run *domain.WorkflowRun, from, to domain.WorkflowRunStatus, reason string) {
	if run == nil {
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

	if s.pubsub != nil {
		if err := s.pubsub.Publish(ctx, fmt.Sprintf("workflow-run:%s", run.ID), payload); err != nil {
			slog.Warn("failed to publish workflow run hook", "workflow_run_id", run.ID, "error", err)
		}
		if err := s.pubsub.Publish(ctx, fmt.Sprintf("workflow:%s:runs", run.WorkflowID), payload); err != nil {
			slog.Warn("failed to publish workflow hook", "workflow_id", run.WorkflowID, "error", err)
		}
	}

	// Enqueue webhook deliveries for matching subscriptions (non-fatal).
	// Use detached context so client disconnect doesn't abort webhook delivery.
	eventType := "workflow_run." + reason
	go func() { //nolint:gosec // intentional detached context for webhook delivery
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in workflow webhook delivery",
					"workflow_run_id", run.ID, "panic", r)
			}
		}()
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		subs, listErr := s.store.ListWebhookSubscriptions(bgCtx, run.ProjectID)
		if listErr != nil {
			slog.Warn("failed to list webhook subscriptions for workflow hook", "project_id", run.ProjectID, "error", listErr)
			return
		}
		now := time.Now()
		for _, sub := range subs {
			if !sub.Active {
				continue
			}
			matched := false
			for _, et := range sub.EventTypes {
				if et == eventType || et == "*" {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			d := &domain.WebhookDelivery{
				WebhookURL:  sub.WebhookURL,
				Status:      "pending",
				Attempts:    0,
				MaxAttempts: 5,
				NextRetryAt: &now,
				LastError:   string(payload),
			}
			if createErr := s.store.CreateWebhookDelivery(bgCtx, d); createErr != nil {
				slog.Warn("failed to enqueue workflow webhook delivery",
					"subscription_id", sub.ID, "event_type", eventType, "error", createErr)
			}
		}
	}()
}
