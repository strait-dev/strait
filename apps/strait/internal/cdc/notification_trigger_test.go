package cdc

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"strait/internal/domain"
)

type mockNotificationStore struct {
	mu          sync.Mutex
	channels    []domain.NotificationChannel
	channelsErr error
	deliveries  []domain.NotificationDelivery
	deliveryErr error
}

func (m *mockNotificationStore) ListNotificationChannels(_ context.Context, _ string) ([]domain.NotificationChannel, error) {
	if m.channelsErr != nil {
		return nil, m.channelsErr
	}
	return m.channels, nil
}

func (m *mockNotificationStore) CreateNotificationDelivery(_ context.Context, d *domain.NotificationDelivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deliveryErr != nil {
		return m.deliveryErr
	}
	m.deliveries = append(m.deliveries, *d)
	return nil
}

func TestNotificationTrigger_CompletedRun_CreatesDelivery(t *testing.T) {
	t.Parallel()
	store := &mockNotificationStore{
		channels: []domain.NotificationChannel{
			{ID: "ch-1", ProjectID: "p1", ChannelType: "slack", Enabled: true},
		},
	}
	h := NewNotificationTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(store.deliveries))
	}
	if store.deliveries[0].ChannelID != "ch-1" {
		t.Errorf("expected channel_id=ch-1, got %s", store.deliveries[0].ChannelID)
	}
	if store.deliveries[0].EventType != "run.completed" {
		t.Errorf("expected event_type=run.completed, got %s", store.deliveries[0].EventType)
	}
	if store.deliveries[0].DedupeKey == "" {
		t.Fatal("expected dedupe key to be set")
	}
}

func TestNotificationTrigger_NonTerminalStatus_Skipped(t *testing.T) {
	t.Parallel()
	store := &mockNotificationStore{
		channels: []domain.NotificationChannel{
			{ID: "ch-1", ProjectID: "p1", ChannelType: "slack", Enabled: true},
		},
	}
	h := NewNotificationTriggerHandler(store, nil)

	for _, status := range []string{"queued", "executing", "dequeued", "delayed"} {
		err := h.Handle(context.Background(), cdcUpdateMsg(status, "p1", "run-1", "job-1"))
		if err != nil {
			t.Fatalf("unexpected error for status %s: %v", status, err)
		}
	}
	if len(store.deliveries) != 0 {
		t.Fatalf("expected 0 deliveries for non-terminal statuses, got %d", len(store.deliveries))
	}
}

func TestNotificationTrigger_DisabledChannel_Skipped(t *testing.T) {
	t.Parallel()
	store := &mockNotificationStore{
		channels: []domain.NotificationChannel{
			{ID: "ch-1", ProjectID: "p1", ChannelType: "slack", Enabled: false},
		},
	}
	h := NewNotificationTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.deliveries) != 0 {
		t.Fatal("expected no deliveries for disabled channel")
	}
}

func TestNotificationTrigger_NoChannels(t *testing.T) {
	t.Parallel()
	store := &mockNotificationStore{channels: nil}
	h := NewNotificationTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.deliveries) != 0 {
		t.Fatal("expected no deliveries when no channels")
	}
}

func TestNotificationTrigger_MultipleChannels(t *testing.T) {
	t.Parallel()
	store := &mockNotificationStore{
		channels: []domain.NotificationChannel{
			{ID: "ch-1", ProjectID: "p1", ChannelType: "slack", Enabled: true},
			{ID: "ch-2", ProjectID: "p1", ChannelType: "email", Enabled: true},
			{ID: "ch-3", ProjectID: "p1", ChannelType: "discord", Enabled: true},
		},
	}
	h := NewNotificationTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("failed", "p1", "run-1", "job-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.deliveries) != 3 {
		t.Fatalf("expected 3 deliveries, got %d", len(store.deliveries))
	}
}

func TestNotificationTrigger_FailureTerminalStatusesCreateFailedDelivery(t *testing.T) {
	t.Parallel()

	for _, status := range []string{"crashed", "system_failed", "expired", "dead_letter"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			store := &mockNotificationStore{
				channels: []domain.NotificationChannel{
					{ID: "ch-1", ProjectID: "p1", ChannelType: "slack", Enabled: true},
				},
			}
			h := NewNotificationTriggerHandler(store, nil)

			if err := h.Handle(context.Background(), cdcUpdateMsg(status, "p1", "run-"+status, "job-1")); err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if len(store.deliveries) != 1 {
				t.Fatalf("deliveries len = %d, want 1", len(store.deliveries))
			}
			if store.deliveries[0].EventType != domain.WebhookEventRunFailed {
				t.Fatalf("EventType = %q, want %q", store.deliveries[0].EventType, domain.WebhookEventRunFailed)
			}
			if store.deliveries[0].DedupeKey == "" {
				t.Fatal("expected dedupe key")
			}
			var payload map[string]any
			if err := json.Unmarshal(store.deliveries[0].Payload, &payload); err != nil {
				t.Fatalf("payload is not valid JSON: %v", err)
			}
			if payload["status"] != status {
				t.Fatalf("status = %v, want original status %s", payload["status"], status)
			}
		})
	}
}

func TestDeepSecNotificationTrigger_StoreErrorReturnsForRetry(t *testing.T) {
	t.Parallel()
	store := &mockNotificationStore{
		channelsErr: errors.New("db connection failed"),
	}
	h := NewNotificationTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	if err == nil {
		t.Fatal("expected error on store failure")
	}
}

func TestNotificationTrigger_PayloadHasRunData(t *testing.T) {
	t.Parallel()
	store := &mockNotificationStore{
		channels: []domain.NotificationChannel{
			{ID: "ch-1", ProjectID: "p1", ChannelType: "slack", Enabled: true},
		},
	}
	h := NewNotificationTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-42", "job-7"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.deliveries) != 1 {
		t.Fatal("expected 1 delivery")
	}

	var payload map[string]any
	if err := json.Unmarshal(store.deliveries[0].Payload, &payload); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if payload["run_id"] != "run-42" {
		t.Errorf("expected run_id=run-42, got %v", payload["run_id"])
	}
	if payload["job_id"] != "job-7" {
		t.Errorf("expected job_id=job-7, got %v", payload["job_id"])
	}
	if payload["event_type"] != "run.completed" {
		t.Errorf("expected event_type=run.completed, got %v", payload["event_type"])
	}
}

func TestNotificationTrigger_InvalidJSON(t *testing.T) {
	t.Parallel()
	store := &mockNotificationStore{}
	h := NewNotificationTriggerHandler(store, nil)

	msg := Message{
		Action:   ActionUpdate,
		Record:   json.RawMessage(`not valid json`),
		Metadata: Metadata{TableName: "job_runs"},
	}
	err := h.Handle(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestNotificationTrigger_EmptyProjectID(t *testing.T) {
	t.Parallel()
	store := &mockNotificationStore{
		channels: []domain.NotificationChannel{
			{ID: "ch-1", ProjectID: "p1", ChannelType: "slack", Enabled: true},
		},
	}
	h := NewNotificationTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "", "run-1", "job-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.deliveries) != 0 {
		t.Fatal("expected no deliveries for empty project_id")
	}
}

func TestDeepSecNotificationTrigger_CreateDeliveryErrorReturnsForRetry(t *testing.T) {
	t.Parallel()
	store := &mockNotificationStore{
		channels: []domain.NotificationChannel{
			{ID: "ch-1", ProjectID: "p1", ChannelType: "slack", Enabled: true},
			{ID: "ch-2", ProjectID: "p1", ChannelType: "email", Enabled: true},
		},
		deliveryErr: errors.New("db write failed"),
	}
	h := NewNotificationTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	if err == nil {
		t.Fatal("expected error on delivery creation failure")
	}
}
