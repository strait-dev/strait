package cdc

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

type mockWebhookStore struct {
	mu          sync.Mutex
	subs        []domain.WebhookSubscription
	subsErr     error
	deliveries  []domain.WebhookDelivery
	deliveryErr error
}

func (m *mockWebhookStore) ListWebhookSubscriptions(_ context.Context, _ string) ([]domain.WebhookSubscription, error) {
	if m.subsErr != nil {
		return nil, m.subsErr
	}
	return m.subs, nil
}

func (m *mockWebhookStore) CreateWebhookDelivery(_ context.Context, d *domain.WebhookDelivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deliveryErr != nil {
		return m.deliveryErr
	}
	m.deliveries = append(m.deliveries, *d)
	return nil
}

func cdcUpdateMsg(status, projectID, runID, jobID string) Message {
	record, _ := json.Marshal(map[string]any{
		"id":         runID,
		"job_id":     jobID,
		"project_id": projectID,
		"status":     status,
		"attempt":    1,
		"error":      "",
	})
	return Message{
		AckID:    "ack-1",
		Action:   ActionUpdate,
		Record:   record,
		Metadata: Metadata{TableName: "job_runs"},
	}
}

func TestWebhookTrigger_CompletedRun_CreatesDelivery(t *testing.T) {
	t.Parallel()
	store := &mockWebhookStore{
		subs: []domain.WebhookSubscription{
			{ID: "sub-1", ProjectID: "p1", WebhookURL: "https://example.com/hook", EventTypes: []string{"run.completed"}, Secret: "whsec", Active: true},
		},
	}
	h := NewWebhookTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	require.NoError(t, err)
	require.Len(t,
		store.deliveries,
		1)
	assert.Equal(
		t, "https://example.com/hook",

		store.
			deliveries[0].WebhookURL)
	assert.Equal(
		t, "sub-1", store.
			deliveries[0].SubscriptionID,
	)
	require.Equal(t, "whsec", store.
		deliveries[0].WebhookSecret,
	)
	require.NotEmpty(t, store.
		deliveries[0].DedupeKey,
	)
	require.NotEmpty(t, store.deliveries[0].
		Payload)
}

func TestWebhookTrigger_FailedRun_CreatesDelivery(t *testing.T) {
	t.Parallel()
	store := &mockWebhookStore{
		subs: []domain.WebhookSubscription{
			{ID: "sub-1", ProjectID: "p1", WebhookURL: "https://example.com/hook", EventTypes: []string{"run.failed"}, Active: true},
		},
	}
	h := NewWebhookTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("failed", "p1", "run-1", "job-1"))
	require.NoError(t, err)
	require.Len(t,
		store.deliveries,
		1)
}

func TestWebhookTrigger_TimedOutRun_CreatesDelivery(t *testing.T) {
	t.Parallel()
	store := &mockWebhookStore{
		subs: []domain.WebhookSubscription{
			{ID: "sub-1", ProjectID: "p1", WebhookURL: "https://example.com/hook", EventTypes: []string{"run.timed_out"}, Active: true},
		},
	}
	h := NewWebhookTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("timed_out", "p1", "run-1", "job-1"))
	require.NoError(t, err)
	require.Len(t,
		store.deliveries,
		1)
}

func TestWebhookTrigger_NonTerminalStatus_Skipped(t *testing.T) {
	t.Parallel()
	store := &mockWebhookStore{
		subs: []domain.WebhookSubscription{
			{ID: "sub-1", ProjectID: "p1", WebhookURL: "https://example.com/hook", EventTypes: []string{"run.completed"}, Active: true},
		},
	}
	h := NewWebhookTriggerHandler(store, nil)

	for _, status := range []string{"queued", "executing", "dequeued", "delayed"} {
		err := h.Handle(context.Background(), cdcUpdateMsg(status, "p1", "run-1", "job-1"))
		require.NoError(t, err)
	}
	require.Empty(t,
		store.deliveries)
}

func TestWebhookTrigger_InsertAction_Skipped(t *testing.T) {
	t.Parallel()
	store := &mockWebhookStore{
		subs: []domain.WebhookSubscription{
			{ID: "sub-1", ProjectID: "p1", WebhookURL: "https://example.com/hook", EventTypes: []string{"run.completed"}, Active: true},
		},
	}
	h := NewWebhookTriggerHandler(store, nil)

	msg := cdcUpdateMsg("completed", "p1", "run-1", "job-1")
	msg.Action = ActionInsert

	err := h.Handle(context.Background(), msg)
	require.NoError(t, err)
	require.Empty(t,
		store.deliveries)
}

func TestWebhookTrigger_NoSubscriptions_NoDelivery(t *testing.T) {
	t.Parallel()
	store := &mockWebhookStore{subs: nil}
	h := NewWebhookTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	require.NoError(t, err)
	require.Empty(t,
		store.deliveries)
}

func TestWebhookTrigger_FilteredSubscription_Skipped(t *testing.T) {
	t.Parallel()
	store := &mockWebhookStore{
		subs: []domain.WebhookSubscription{
			{ID: "sub-1", ProjectID: "p1", WebhookURL: "https://example.com/hook", EventTypes: []string{"run.completed"}, Active: true},
		},
	}
	h := NewWebhookTriggerHandler(store, nil)

	// Send a failed event but subscription only watches completed.
	err := h.Handle(context.Background(), cdcUpdateMsg("failed", "p1", "run-1", "job-1"))
	require.NoError(t, err)
	require.Empty(t,
		store.deliveries)
}

func TestWebhookTrigger_MultipleSubscriptions_AllFired(t *testing.T) {
	t.Parallel()
	store := &mockWebhookStore{
		subs: []domain.WebhookSubscription{
			{ID: "sub-1", ProjectID: "p1", WebhookURL: "https://a.com/hook", EventTypes: []string{"run.completed"}, Active: true},
			{ID: "sub-2", ProjectID: "p1", WebhookURL: "https://b.com/hook", EventTypes: []string{"run.completed"}, Active: true},
			{ID: "sub-3", ProjectID: "p1", WebhookURL: "https://c.com/hook", EventTypes: []string{"run.failed"}, Active: true},
		},
	}
	h := NewWebhookTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	require.NoError(t, err)
	require.Len(t,
		store.deliveries,
		2)

	// sub-1 and sub-2 match (run.completed), sub-3 doesn't (run.failed).
}

func TestDeepSecWebhookTrigger_StoreErrorReturnsForRetry(t *testing.T) {
	t.Parallel()
	store := &mockWebhookStore{
		subsErr: errors.New("db connection failed"),
	}
	h := NewWebhookTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	require.Error(t, err)
}

func TestWebhookTrigger_InvalidJSON_ReturnsError(t *testing.T) {
	t.Parallel()
	store := &mockWebhookStore{}
	h := NewWebhookTriggerHandler(store, nil)

	msg := Message{
		Action:   ActionUpdate,
		Record:   json.RawMessage(`not valid json`),
		Metadata: Metadata{TableName: "job_runs"},
	}

	err := h.Handle(context.Background(), msg)
	require.Error(t, err)
}

func TestWebhookTrigger_MissingProjectID_Skipped(t *testing.T) {
	t.Parallel()
	store := &mockWebhookStore{
		subs: []domain.WebhookSubscription{
			{ID: "sub-1", ProjectID: "p1", WebhookURL: "https://example.com/hook", EventTypes: []string{"run.completed"}, Active: true},
		},
	}
	h := NewWebhookTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "", "run-1", "job-1"))
	require.NoError(t, err)
	require.Empty(t,
		store.deliveries)
}

func TestWebhookTrigger_InactiveSubscription_Skipped(t *testing.T) {
	t.Parallel()
	store := &mockWebhookStore{
		subs: []domain.WebhookSubscription{
			{ID: "sub-1", ProjectID: "p1", WebhookURL: "https://example.com/hook", EventTypes: []string{"run.completed"}, Active: false},
		},
	}
	h := NewWebhookTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	require.NoError(t, err)
	require.Empty(t,
		store.deliveries)
}

func TestWebhookTrigger_PayloadContainsRunData(t *testing.T) {
	t.Parallel()
	store := &mockWebhookStore{
		subs: []domain.WebhookSubscription{
			{ID: "sub-1", ProjectID: "p1", WebhookURL: "https://example.com/hook", EventTypes: []string{"run.completed"}, Active: true},
		},
	}
	h := NewWebhookTriggerHandler(store, nil)

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

func TestDeepSecWebhookTrigger_CreateDeliveryErrorReturnsForRetry(t *testing.T) {
	t.Parallel()
	store := &mockWebhookStore{
		subs: []domain.WebhookSubscription{
			{ID: "sub-1", ProjectID: "p1", WebhookURL: "https://a.com/hook", EventTypes: []string{"run.completed"}, Active: true},
			{ID: "sub-2", ProjectID: "p1", WebhookURL: "https://b.com/hook", EventTypes: []string{"run.completed"}, Active: true},
		},
		deliveryErr: errors.New("db write failed"),
	}
	h := NewWebhookTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	require.Error(t, err)
}

func TestWebhookTrigger_CanceledRun_CreatesDelivery(t *testing.T) {
	t.Parallel()
	store := &mockWebhookStore{
		subs: []domain.WebhookSubscription{
			{ID: "sub-1", ProjectID: "p1", WebhookURL: "https://example.com/hook", EventTypes: []string{"run.canceled"}, Active: true},
		},
	}
	h := NewWebhookTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("canceled", "p1", "run-1", "job-1"))
	require.NoError(t, err)
	require.Len(t,
		store.deliveries,
		1)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(store.
		deliveries[0].Payload,

		&payload))
	assert.Equal(
		t, "run.canceled",
		payload["event_type"])
}

func TestWebhookTrigger_FailureTerminalStatusesCreateFailedDelivery(t *testing.T) {
	t.Parallel()

	for _, status := range []string{"crashed", "system_failed", "expired", "dead_letter"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			store := &mockWebhookStore{
				subs: []domain.WebhookSubscription{
					{ID: "sub-1", ProjectID: "p1", WebhookURL: "https://example.com/hook", EventTypes: []string{domain.WebhookEventRunFailed}, Active: true},
				},
			}
			h := NewWebhookTriggerHandler(store, nil)
			require.NoError(t, h.Handle(context.
				Background(),
				cdcUpdateMsg(status, "p1", "run-"+status,

					"job-1")))
			require.Len(t,
				store.deliveries,
				1)
			require.NotEmpty(t, store.
				deliveries[0].DedupeKey,
			)

			var payload map[string]any
			require.NoError(t, json.Unmarshal(store.
				deliveries[0].Payload,

				&payload))
			require.Equal(t, domain.WebhookEventRunFailed,

				payload["event_type"])
			require.Equal(t, status, payload["status"])
		})
	}
}

func TestWebhookTrigger_ConcurrentEvents(t *testing.T) {
	t.Parallel()
	store := &mockWebhookStore{
		subs: []domain.WebhookSubscription{
			{ID: "sub-1", ProjectID: "p1", WebhookURL: "https://example.com/hook", EventTypes: []string{"run.completed", "run.failed"}, Active: true},
		},
	}
	h := NewWebhookTriggerHandler(store, nil)

	var wg conc.WaitGroup
	for i := range 10 {
		wg.Go(func() {
			status := "completed"
			if i%2 == 0 {
				status = "failed"
			}
			_ = h.Handle(context.Background(), cdcUpdateMsg(status, "p1", "run-"+string(rune('0'+i)), "job-1"))
		})
	}
	wg.Wait()

	store.mu.Lock()
	defer store.mu.Unlock()
	require.Len(t,
		store.deliveries,
		10)
}
