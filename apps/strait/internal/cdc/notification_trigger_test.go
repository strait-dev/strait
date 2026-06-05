package cdc

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	require.Len(t,
		store.deliveries,
		1)
	assert.Equal(
		t, "ch-1", store.
			deliveries[0].ChannelID,
	)
	assert.Equal(
		t, "run.completed",
		store.deliveries[0].EventType,
	)
	require.NotEmpty(t, store.
		deliveries[0].DedupeKey,
	)
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
		require.NoError(t, err)
	}
	require.Empty(t,
		store.deliveries)
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
	require.NoError(t, err)
	require.Empty(t,
		store.deliveries)
}

func TestNotificationTrigger_NoChannels(t *testing.T) {
	t.Parallel()
	store := &mockNotificationStore{channels: nil}
	h := NewNotificationTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	require.NoError(t, err)
	require.Empty(t,
		store.deliveries)
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
	require.NoError(t, err)
	require.Len(t,
		store.deliveries,
		3)
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
			require.NoError(t, h.Handle(context.
				Background(),
				cdcUpdateMsg(status, "p1", "run-"+status,

					"job-1")))
			require.Len(t,
				store.deliveries,
				1)
			require.Equal(t, domain.WebhookEventRunFailed,

				store.
					deliveries[0].EventType)
			require.NotEmpty(t, store.
				deliveries[0].DedupeKey,
			)

			var payload map[string]any
			require.NoError(t, json.Unmarshal(store.
				deliveries[0].Payload,

				&payload))
			require.Equal(t, status, payload["status"])
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
	require.Error(t, err)
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
	require.NoError(t, err)
	require.Len(t,
		store.deliveries,
		1)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(store.
		deliveries[0].Payload,

		&payload))
	assert.Equal(
		t, "run-42", payload["run_id"])
	assert.Equal(
		t, "job-7", payload["job_id"])
	assert.Equal(
		t, "run.completed",
		payload["event_type"])
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
	require.Error(t, err)
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
	require.NoError(t, err)
	require.Empty(t,
		store.deliveries)
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
	require.Error(t, err)
}
