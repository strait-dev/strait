package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"strait/internal/clickhouse"
	"strait/internal/domain"

	"github.com/sourcegraph/conc"
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

	// Enqueue ClickHouse workflow run analytics on terminal status transitions.
	if to.IsTerminal() && s.chExporter != nil {
		var durationMs uint64
		if run.StartedAt != nil {
			finishedAt := time.Now()
			if run.FinishedAt != nil {
				finishedAt = *run.FinishedAt
			}
			durationMs = uint64(max(finishedAt.Sub(*run.StartedAt).Milliseconds(), 0))
		}
		s.chExporter.Enqueue(clickhouse.WorkflowRunAnalyticsRecord{
			WorkflowRunID: run.ID,
			WorkflowID:    run.WorkflowID,
			ProjectID:     run.ProjectID,
			Status:        string(to),
			TriggeredBy:   run.TriggeredBy,
			StepCount:     0, // Step count is not readily available here.
			DurationMs:    durationMs,
			CreatedAt:     run.CreatedAt,
			StartedAt:     run.StartedAt,
			FinishedAt:    run.FinishedAt,
		})
	}

	// Enqueue webhook deliveries for matching subscriptions (non-fatal).
	// Use detached context so client disconnect doesn't abort webhook delivery.
	eventType, ok := workflowWebhookEventType(to)
	if !ok {
		return
	}
	var deliveryWG conc.WaitGroup
	deliveryWG.Go(func() {
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
				if workflowWebhookEventTypeMatches(et, eventType) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			d := &domain.WebhookDelivery{
				SubscriptionID: sub.ID,
				ProjectID:      sub.ProjectID,
				WebhookURL:     sub.WebhookURL,
				Status:         "pending",
				Attempts:       0,
				MaxAttempts:    5,
				NextRetryAt:    &now,
				Payload:        payload,
			}
			if createErr := s.store.CreateWebhookDelivery(bgCtx, d); createErr != nil {
				slog.Warn("failed to enqueue workflow webhook delivery",
					"subscription_id", sub.ID, "event_type", eventType, "error", createErr)
			}
		}
	})
}

func workflowWebhookEventType(status domain.WorkflowRunStatus) (string, bool) {
	switch status {
	case domain.WfStatusCompleted:
		return domain.WebhookEventWorkflowCompleted, true
	case domain.WfStatusFailed, domain.WfStatusTimedOut, domain.WfStatusCanceled,
		domain.WfStatusCompensated, domain.WfStatusCompensationFailed:
		return domain.WebhookEventWorkflowFailed, true
	default:
		return "", false
	}
}

func workflowWebhookEventTypeMatches(candidate, target string) bool {
	return candidate == target || candidate == "*"
}
