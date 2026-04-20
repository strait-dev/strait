package cdc

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

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
			{ID: "sub-1", ProjectID: "p1", WebhookURL: "https://example.com/hook", EventTypes: []string{"run.completed"}, Active: true},
		},
	}
	h := NewWebhookTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(store.deliveries))
	}
	if store.deliveries[0].WebhookURL != "https://example.com/hook" {
		t.Errorf("unexpected URL: %s", store.deliveries[0].WebhookURL)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(store.deliveries))
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(store.deliveries))
	}
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
		if err != nil {
			t.Fatalf("unexpected error for status %s: %v", status, err)
		}
	}
	if len(store.deliveries) != 0 {
		t.Fatalf("expected 0 deliveries for non-terminal statuses, got %d", len(store.deliveries))
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.deliveries) != 0 {
		t.Fatal("expected no deliveries for insert action")
	}
}

func TestWebhookTrigger_NoSubscriptions_NoDelivery(t *testing.T) {
	t.Parallel()
	store := &mockWebhookStore{subs: nil}
	h := NewWebhookTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.deliveries) != 0 {
		t.Fatal("expected no deliveries when no subscriptions")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.deliveries) != 0 {
		t.Fatal("expected no deliveries when event type doesn't match subscription")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// sub-1 and sub-2 match (run.completed), sub-3 doesn't (run.failed).
	if len(store.deliveries) != 2 {
		t.Fatalf("expected 2 deliveries, got %d", len(store.deliveries))
	}
}

func TestWebhookTrigger_StoreError_NoNack(t *testing.T) {
	t.Parallel()
	store := &mockWebhookStore{
		subsErr: errors.New("db connection failed"),
	}
	h := NewWebhookTriggerHandler(store, nil)

	err := h.Handle(context.Background(), cdcUpdateMsg("completed", "p1", "run-1", "job-1"))
	// Should not return error (no nack), just log warning.
	if err != nil {
		t.Fatalf("expected nil error on store failure, got: %v", err)
	}
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
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.deliveries) != 0 {
		t.Fatal("expected no deliveries when project_id is empty")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.deliveries) != 0 {
		t.Fatal("expected no deliveries for inactive subscription")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(store.deliveries) != 1 {
		t.Fatal("expected 1 delivery")
	}

	// Payload is stored in LastError field.
	var payload map[string]any
	if err := json.Unmarshal([]byte(store.deliveries[0].LastError), &payload); err != nil {
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

func TestWebhookTrigger_CreateDeliveryError_Resilient(t *testing.T) {
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
	if err != nil {
		t.Fatalf("expected nil error on delivery creation failure, got: %v", err)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(store.deliveries))
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(store.deliveries[0].LastError), &payload); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if payload["event_type"] != "run.canceled" {
		t.Errorf("expected event_type=run.canceled, got %v", payload["event_type"])
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

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			status := "completed"
			if i%2 == 0 {
				status = "failed"
			}
			_ = h.Handle(context.Background(), cdcUpdateMsg(status, "p1", "run-"+string(rune('0'+i)), "job-1"))
		}(i)
	}
	wg.Wait()

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.deliveries) != 10 {
		t.Fatalf("expected 10 deliveries from concurrent events, got %d", len(store.deliveries))
	}
}
