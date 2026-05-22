package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"

	"strait/internal/domain"
)

type mockStageNotifierStore struct {
	mu         sync.Mutex
	channels   []domain.NotificationChannel
	deliveries []domain.NotificationDelivery
	channelErr error
	deliverErr error
	listCalls  int
}

func (m *mockStageNotifierStore) ListEnabledNotificationChannels(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listCalls++
	if m.channelErr != nil {
		return nil, m.channelErr
	}
	return m.channels, nil
}

func (m *mockStageNotifierStore) CreateNotificationDelivery(_ context.Context, d *domain.NotificationDelivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deliverErr != nil {
		return m.deliverErr
	}
	m.deliveries = append(m.deliveries, *d)
	return nil
}

func TestStageNotification_OnStepCompleted(t *testing.T) {
	t.Parallel()
	store := &mockStageNotifierStore{
		channels: []domain.NotificationChannel{
			{ID: "ch-1", ProjectID: "proj-1", ChannelType: "webhook"},
		},
	}
	notifier := NewStageNotifier(store, nil)

	step := &domain.WorkflowStep{
		StepRef:            "charge",
		StageNotifications: json.RawMessage(`{"on_complete":true}`),
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1"}
	wfRun := &domain.WorkflowRun{ID: "wfr-1", WorkflowID: "wf-1", ProjectID: "proj-1"}

	notifier.NotifyStepTransition(context.Background(), step, stepRun, wfRun, domain.StepCompleted)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(store.deliveries))
	}
	if store.deliveries[0].EventType != "step.completed" {
		t.Errorf("event_type = %q, want step.completed", store.deliveries[0].EventType)
	}
}

func TestStageNotification_OnStepFailed(t *testing.T) {
	t.Parallel()
	store := &mockStageNotifierStore{
		channels: []domain.NotificationChannel{
			{ID: "ch-1", ProjectID: "proj-1"},
		},
	}
	notifier := NewStageNotifier(store, nil)

	step := &domain.WorkflowStep{
		StepRef:            "charge",
		StageNotifications: json.RawMessage(`{"on_failure":true}`),
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1"}
	wfRun := &domain.WorkflowRun{ID: "wfr-1", ProjectID: "proj-1"}

	notifier.NotifyStepTransition(context.Background(), step, stepRun, wfRun, domain.StepFailed)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(store.deliveries))
	}
	if store.deliveries[0].EventType != "step.failed" {
		t.Errorf("event_type = %q, want step.failed", store.deliveries[0].EventType)
	}
}

func TestStageNotification_NoChannelsConfigured(t *testing.T) {
	t.Parallel()
	store := &mockStageNotifierStore{
		channels: nil,
	}
	notifier := NewStageNotifier(store, nil)

	step := &domain.WorkflowStep{
		StepRef:            "charge",
		StageNotifications: json.RawMessage(`{"on_complete":true}`),
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1"}
	wfRun := &domain.WorkflowRun{ID: "wfr-1", ProjectID: "proj-1"}

	notifier.NotifyStepTransition(context.Background(), step, stepRun, wfRun, domain.StepCompleted)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.deliveries) != 0 {
		t.Fatalf("expected 0 deliveries when no channels, got %d", len(store.deliveries))
	}
}

func TestStageNotification_MultipleChannels(t *testing.T) {
	t.Parallel()
	store := &mockStageNotifierStore{
		channels: []domain.NotificationChannel{
			{ID: "ch-1", ProjectID: "proj-1", ChannelType: "webhook"},
			{ID: "ch-2", ProjectID: "proj-1", ChannelType: "slack"},
		},
	}
	notifier := NewStageNotifier(store, nil)

	step := &domain.WorkflowStep{
		StepRef:            "charge",
		StageNotifications: json.RawMessage(`{"on_complete":true}`),
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1"}
	wfRun := &domain.WorkflowRun{ID: "wfr-1", ProjectID: "proj-1"}

	notifier.NotifyStepTransition(context.Background(), step, stepRun, wfRun, domain.StepCompleted)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.deliveries) != 2 {
		t.Fatalf("expected 2 deliveries (one per channel), got %d", len(store.deliveries))
	}
}

func TestStageNotification_NoNotificationsConfigured(t *testing.T) {
	t.Parallel()
	store := &mockStageNotifierStore{
		channels: []domain.NotificationChannel{
			{ID: "ch-1"},
		},
	}
	notifier := NewStageNotifier(store, nil)

	// No StageNotifications on step.
	step := &domain.WorkflowStep{StepRef: "charge"}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1"}
	wfRun := &domain.WorkflowRun{ID: "wfr-1", ProjectID: "proj-1"}

	notifier.NotifyStepTransition(context.Background(), step, stepRun, wfRun, domain.StepCompleted)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.deliveries) != 0 {
		t.Fatalf("expected 0 deliveries when no notifications config, got %d", len(store.deliveries))
	}
}

func TestStageNotification_CompletedNotConfiguredForFailure(t *testing.T) {
	t.Parallel()
	store := &mockStageNotifierStore{
		channels: []domain.NotificationChannel{{ID: "ch-1"}},
	}
	notifier := NewStageNotifier(store, nil)

	// Only on_failure configured, but step completed.
	step := &domain.WorkflowStep{
		StepRef:            "charge",
		StageNotifications: json.RawMessage(`{"on_failure":true}`),
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1"}
	wfRun := &domain.WorkflowRun{ID: "wfr-1", ProjectID: "proj-1"}

	notifier.NotifyStepTransition(context.Background(), step, stepRun, wfRun, domain.StepCompleted)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.deliveries) != 0 {
		t.Fatalf("expected 0 deliveries for mismatch, got %d", len(store.deliveries))
	}
}

func TestStageNotification_InvalidJSON(t *testing.T) {
	t.Parallel()
	store := &mockStageNotifierStore{
		channels: []domain.NotificationChannel{{ID: "ch-1"}},
	}
	notifier := NewStageNotifier(store, nil)

	step := &domain.WorkflowStep{
		StepRef:            "charge",
		StageNotifications: json.RawMessage(`not valid json`),
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1"}
	wfRun := &domain.WorkflowRun{ID: "wfr-1", ProjectID: "proj-1"}

	// Should not panic.
	notifier.NotifyStepTransition(context.Background(), step, stepRun, wfRun, domain.StepCompleted)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.deliveries) != 0 {
		t.Fatalf("expected 0 deliveries for invalid config, got %d", len(store.deliveries))
	}
}

func TestStageNotification_NonTerminalStatusSkipsInvalidConfig(t *testing.T) {
	t.Parallel()

	var logBuf bytes.Buffer
	store := &mockStageNotifierStore{
		channels: []domain.NotificationChannel{{ID: "ch-1"}},
	}
	notifier := NewStageNotifier(store, slog.New(slog.NewTextHandler(&logBuf, nil)))

	step := &domain.WorkflowStep{
		StepRef:            "charge",
		StageNotifications: json.RawMessage(`not valid json`),
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1"}
	wfRun := &domain.WorkflowRun{ID: "wfr-1", ProjectID: "proj-1"}

	notifier.NotifyStepTransition(context.Background(), step, stepRun, wfRun, domain.StepRunning)

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.listCalls != 0 {
		t.Fatalf("expected 0 channel lookups for non-terminal status, got %d", store.listCalls)
	}
	if len(store.deliveries) != 0 {
		t.Fatalf("expected 0 deliveries for non-terminal status, got %d", len(store.deliveries))
	}
	if logBuf.Len() != 0 {
		t.Fatalf("expected no log output for non-terminal status, got %q", logBuf.String())
	}
}

func TestStageNotification_NilStep(t *testing.T) {
	t.Parallel()
	store := &mockStageNotifierStore{}
	notifier := NewStageNotifier(store, nil)

	stepRun := &domain.WorkflowStepRun{ID: "sr-1"}
	wfRun := &domain.WorkflowRun{ID: "wfr-1", ProjectID: "proj-1"}

	// Should not panic.
	notifier.NotifyStepTransition(context.Background(), nil, stepRun, wfRun, domain.StepCompleted)
}

func TestStageNotification_OnSkipped(t *testing.T) {
	t.Parallel()
	store := &mockStageNotifierStore{
		channels: []domain.NotificationChannel{{ID: "ch-1"}},
	}
	notifier := NewStageNotifier(store, nil)

	step := &domain.WorkflowStep{
		StepRef:            "optional",
		StageNotifications: json.RawMessage(`{"on_skipped":true}`),
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1"}
	wfRun := &domain.WorkflowRun{ID: "wfr-1", ProjectID: "proj-1"}

	notifier.NotifyStepTransition(context.Background(), step, stepRun, wfRun, domain.StepSkipped)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.deliveries) != 1 {
		t.Fatalf("expected 1 delivery for skipped, got %d", len(store.deliveries))
	}
	if store.deliveries[0].EventType != "step.skipped" {
		t.Errorf("event_type = %q, want step.skipped", store.deliveries[0].EventType)
	}
}

func TestStageNotification_PayloadContent(t *testing.T) {
	t.Parallel()
	store := &mockStageNotifierStore{
		channels: []domain.NotificationChannel{{ID: "ch-1"}},
	}
	notifier := NewStageNotifier(store, nil)

	step := &domain.WorkflowStep{
		StepRef:            "process",
		StageNotifications: json.RawMessage(`{"on_complete":true}`),
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-42"}
	wfRun := &domain.WorkflowRun{ID: "wfr-99", WorkflowID: "wf-7", ProjectID: "proj-1"}

	notifier.NotifyStepTransition(context.Background(), step, stepRun, wfRun, domain.StepCompleted)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(store.deliveries))
	}

	var payload map[string]any
	if err := json.Unmarshal(store.deliveries[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["workflow_id"] != "wf-7" {
		t.Errorf("workflow_id = %v, want wf-7", payload["workflow_id"])
	}
	if payload["workflow_run_id"] != "wfr-99" {
		t.Errorf("workflow_run_id = %v, want wfr-99", payload["workflow_run_id"])
	}
	if payload["step_ref"] != "process" {
		t.Errorf("step_ref = %v, want process", payload["step_ref"])
	}
	if payload["step_run_id"] != "sr-42" {
		t.Errorf("step_run_id = %v, want sr-42", payload["step_run_id"])
	}
	if payload["status"] != "completed" {
		t.Errorf("status = %v, want completed", payload["status"])
	}
}

func BenchmarkStageNotification_NonTerminal(b *testing.B) {
	store := &mockStageNotifierStore{}
	notifier := NewStageNotifier(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	step := &domain.WorkflowStep{
		StepRef: "charge",
		StageNotifications: json.RawMessage(`{
			"on_complete": true,
			"on_failure": true,
			"on_skipped": true
		}`),
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1"}
	wfRun := &domain.WorkflowRun{ID: "wfr-1", WorkflowID: "wf-1", ProjectID: "proj-1"}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		notifier.NotifyStepTransition(ctx, step, stepRun, wfRun, domain.StepRunning)
	}
}

func BenchmarkStageNotification_CompletedNoChannels(b *testing.B) {
	store := &mockStageNotifierStore{}
	notifier := NewStageNotifier(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	step := &domain.WorkflowStep{
		StepRef:            "charge",
		StageNotifications: json.RawMessage(`{"on_complete": true}`),
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1"}
	wfRun := &domain.WorkflowRun{ID: "wfr-1", WorkflowID: "wf-1", ProjectID: "proj-1"}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		notifier.NotifyStepTransition(ctx, step, stepRun, wfRun, domain.StepCompleted)
	}
}
