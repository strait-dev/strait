package workflow

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"strait/internal/domain"

	"github.com/tidwall/gjson"
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

	cfg, err := parseStageNotificationConfig(step.StageNotifications)
	if err != nil {
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

	payload, _ := marshalStageNotificationPayload(wfRun.WorkflowID, wfRun.ID, step.StepRef, stepRun.ID, string(newStatus), time.Now().UTC())

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

func parseStageNotificationConfig(raw []byte) (domain.StageNotificationConfig, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return domain.StageNotificationConfig{}, fmt.Errorf("empty stage notification config")
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return domain.StageNotificationConfig{}, nil
	}
	if trimmed[0] != '{' || !gjson.ValidBytes(trimmed) {
		return domain.StageNotificationConfig{}, fmt.Errorf("invalid stage notification config")
	}

	onComplete, err := stageNotificationBool(trimmed, "on_complete")
	if err != nil {
		return domain.StageNotificationConfig{}, err
	}
	onFailure, err := stageNotificationBool(trimmed, "on_failure")
	if err != nil {
		return domain.StageNotificationConfig{}, err
	}
	onSkipped, err := stageNotificationBool(trimmed, "on_skipped")
	if err != nil {
		return domain.StageNotificationConfig{}, err
	}
	return domain.StageNotificationConfig{
		OnComplete: onComplete,
		OnFailure:  onFailure,
		OnSkipped:  onSkipped,
	}, nil
}

func stageNotificationBool(raw []byte, field string) (bool, error) {
	value := gjson.GetBytes(raw, field)
	if !value.Exists() || value.Type == gjson.Null {
		return false, nil
	}
	switch value.Type {
	case gjson.True:
		return true, nil
	case gjson.False:
		return false, nil
	default:
		return false, fmt.Errorf("stage notification field %s must be a bool", field)
	}
}

func marshalStageNotificationPayload(workflowID, workflowRunID, stepRef, stepRunID, status string, triggeredAt time.Time) ([]byte, error) {
	out := make([]byte, 0, 120+len(workflowID)+len(workflowRunID)+len(stepRef)+len(stepRunID)+len(status))
	out = append(out, `{"workflow_id":`...)
	out = strconv.AppendQuote(out, workflowID)
	out = append(out, `,"workflow_run_id":`...)
	out = strconv.AppendQuote(out, workflowRunID)
	out = append(out, `,"step_ref":`...)
	out = strconv.AppendQuote(out, stepRef)
	out = append(out, `,"step_run_id":`...)
	out = strconv.AppendQuote(out, stepRunID)
	out = append(out, `,"status":`...)
	out = strconv.AppendQuote(out, status)
	out = append(out, `,"triggered_at":`...)
	out = appendStageNotificationJSONTime(out, triggeredAt)
	out = append(out, '}')
	return out, nil
}

func appendStageNotificationJSONTime(out []byte, triggeredAt time.Time) []byte {
	out = append(out, '"')
	out = triggeredAt.AppendFormat(out, time.RFC3339)
	out = append(out, '"')
	return out
}
