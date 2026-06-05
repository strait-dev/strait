package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Len(t, store.
		deliveries,
		1)
	assert.Equal(t,
		"step.completed",
		store.deliveries[0].EventType,
	)

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
	require.Len(t, store.
		deliveries,
		1)
	assert.Equal(t,
		"step.failed",
		store.deliveries[0].EventType,
	)

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
	require.Len(t, store.
		deliveries,
		0)

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
	require.Len(t, store.
		deliveries,
		2)

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
	require.Len(t, store.
		deliveries,
		0)

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
	require.Len(t, store.
		deliveries,
		0)

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
	require.Len(t, store.
		deliveries,
		0)

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
	require.EqualValues(t, 0, store.listCalls)
	require.Len(t, store.
		deliveries,
		0)
	require.EqualValues(t, 0, logBuf.
		Len())

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
	require.Len(t, store.
		deliveries,
		1)
	assert.Equal(t,
		"step.skipped",
		store.deliveries[0].EventType,
	)

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
	require.Len(t, store.
		deliveries,
		1)

	var payload map[string]any
	require.NoError(
		t, json.Unmarshal(store.deliveries[0].Payload,
			&payload))
	assert.Equal(t,
		"wf-7", payload["workflow_id"])
	assert.Equal(t,
		"wfr-99", payload["workflow_run_id"])
	assert.Equal(t,
		"process",
		payload["step_ref"])
	assert.Equal(t,
		"sr-42", payload["step_run_id"])
	assert.Equal(t,
		"completed",
		payload["status"])

}

func BenchmarkStageNotification_NonTerminal(b *testing.B) {
	store := &mockStageNotifierStore{}
	notifier := NewStageNotifier(store, slog.New(slog.DiscardHandler))
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
	for range b.N {
		notifier.NotifyStepTransition(ctx, step, stepRun, wfRun, domain.StepRunning)
	}
}

func BenchmarkStageNotification_CompletedNoChannels(b *testing.B) {
	store := &mockStageNotifierStore{}
	notifier := NewStageNotifier(store, slog.New(slog.DiscardHandler))
	step := &domain.WorkflowStep{
		StepRef:            "charge",
		StageNotifications: json.RawMessage(`{"on_complete": true}`),
	}
	stepRun := &domain.WorkflowStepRun{ID: "sr-1"}
	wfRun := &domain.WorkflowRun{ID: "wfr-1", WorkflowID: "wf-1", ProjectID: "proj-1"}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		notifier.NotifyStepTransition(ctx, step, stepRun, wfRun, domain.StepCompleted)
	}
}
