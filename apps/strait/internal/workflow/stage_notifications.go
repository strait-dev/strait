package workflow

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"strait/internal/domain"
)

// StageNotifier sends notifications on step state transitions.
type StageNotifier struct {
	store  stageNotifierStore
	logger *slog.Logger
}

type stageNotifierStore interface {
	ListEnabledNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error)
	CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error
}

// NewStageNotifier creates a new StageNotifier.
func NewStageNotifier(store stageNotifierStore, logger *slog.Logger) *StageNotifier {
	if logger == nil {
		logger = slog.Default()
	}
	return &StageNotifier{store: store, logger: logger}
}

// NotifyStepTransition sends notifications for a step state change if the step
// has stage_notifications configured and the transition matches.
func (n *StageNotifier) NotifyStepTransition(
	ctx context.Context,
	step *domain.WorkflowStep,
	stepRun *domain.WorkflowStepRun,
	wfRun *domain.WorkflowRun,
	newStatus domain.StepRunStatus,
) {
	if step == nil || len(step.StageNotifications) == 0 {
		return
	}

	var eventType string
	switch newStatus {
	case domain.StepCompleted:
		eventType = "step.completed"
	case domain.StepFailed:
		eventType = "step.failed"
	case domain.StepSkipped:
		eventType = "step.skipped"
	default:
		return
	}

	var cfg domain.StageNotificationConfig
	if err := json.Unmarshal(step.StageNotifications, &cfg); err != nil {
		n.logger.Warn("invalid stage_notifications config",
			"step_ref", step.StepRef,
			"error", err,
		)
		return
	}

	shouldNotify := false
	switch newStatus { //nolint:exhaustive // only terminal states trigger notifications.
	case domain.StepCompleted:
		shouldNotify = cfg.OnComplete
	case domain.StepFailed:
		shouldNotify = cfg.OnFailure
	case domain.StepSkipped:
		shouldNotify = cfg.OnSkipped
	}

	if !shouldNotify {
		return
	}

	channels, err := n.store.ListEnabledNotificationChannels(ctx, wfRun.ProjectID)
	if err != nil {
		n.logger.Warn("failed to list notification channels for stage notification",
			"project_id", wfRun.ProjectID,
			"error", err,
		)
		return
	}

	if len(channels) == 0 {
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"workflow_id":     wfRun.WorkflowID,
		"workflow_run_id": wfRun.ID,
		"step_ref":        step.StepRef,
		"step_run_id":     stepRun.ID,
		"status":          string(newStatus),
		"triggered_at":    time.Now().UTC().Format(time.RFC3339),
	})

	for _, ch := range channels {
		delivery := &domain.NotificationDelivery{
			ChannelID:   ch.ID,
			ProjectID:   wfRun.ProjectID,
			EventType:   eventType,
			Payload:     payload,
			Status:      "pending",
			MaxAttempts: 3,
		}
		if createErr := n.store.CreateNotificationDelivery(ctx, delivery); createErr != nil {
			n.logger.Warn("failed to create stage notification delivery",
				"channel_id", ch.ID,
				"event_type", eventType,
				"step_ref", step.StepRef,
				"error", createErr,
			)
		}
	}

	n.logger.Info("stage notification enqueued",
		"step_ref", step.StepRef,
		"status", string(newStatus),
		"event_type", eventType,
		"channels", strconv.Itoa(len(channels)),
	)
}
