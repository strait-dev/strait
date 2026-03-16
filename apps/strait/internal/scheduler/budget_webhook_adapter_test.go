package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"strait/internal/domain"
)

type mockWebhookStore struct {
	listSubsFn       func(ctx context.Context, projectID string) ([]domain.WebhookSubscription, error)
	createDeliveryFn func(ctx context.Context, d *domain.WebhookDelivery) error
	deliveries       []*domain.WebhookDelivery
}

func (m *mockWebhookStore) ListWebhookSubscriptions(ctx context.Context, projectID string) ([]domain.WebhookSubscription, error) {
	if m.listSubsFn != nil {
		return m.listSubsFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockWebhookStore) CreateWebhookDelivery(ctx context.Context, d *domain.WebhookDelivery) error {
	m.deliveries = append(m.deliveries, d)
	if m.createDeliveryFn != nil {
		return m.createDeliveryFn(ctx, d)
	}
	return nil
}

func TestBudgetWebhookAdapter_CreatesDelivery(t *testing.T) {
	t.Parallel()

	ws := &mockWebhookStore{
		listSubsFn: func(_ context.Context, _ string) ([]domain.WebhookSubscription, error) {
			return []domain.WebhookSubscription{
				{
					ID:         "sub-1",
					ProjectID:  "proj-1",
					WebhookURL: "https://example.com/hook",
					EventTypes: []string{"compute_budget_warning"},
					Active:     true,
				},
			}, nil
		},
	}

	adapter := NewBudgetWebhookAdapter(ws)
	payload, _ := json.Marshal(map[string]string{"event": "compute_budget_warning"})
	err := adapter.EnqueueBudgetAlert(context.Background(), "proj-1", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ws.deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(ws.deliveries))
	}
	if ws.deliveries[0].WebhookURL != "https://example.com/hook" {
		t.Fatalf("unexpected webhook URL: %s", ws.deliveries[0].WebhookURL)
	}
	if ws.deliveries[0].Status != domain.WebhookStatusPending {
		t.Fatalf("expected pending status, got %s", ws.deliveries[0].Status)
	}
}

func TestBudgetWebhookAdapter_SkipsInactiveSubscription(t *testing.T) {
	t.Parallel()

	ws := &mockWebhookStore{
		listSubsFn: func(_ context.Context, _ string) ([]domain.WebhookSubscription, error) {
			return []domain.WebhookSubscription{
				{
					ID:         "sub-1",
					ProjectID:  "proj-1",
					WebhookURL: "https://example.com/hook",
					EventTypes: []string{"compute_budget_warning"},
					Active:     false,
				},
			}, nil
		},
	}

	adapter := NewBudgetWebhookAdapter(ws)
	err := adapter.EnqueueBudgetAlert(context.Background(), "proj-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ws.deliveries) != 0 {
		t.Fatalf("expected 0 deliveries for inactive sub, got %d", len(ws.deliveries))
	}
}

func TestBudgetWebhookAdapter_SkipsNonMatchingEventType(t *testing.T) {
	t.Parallel()

	ws := &mockWebhookStore{
		listSubsFn: func(_ context.Context, _ string) ([]domain.WebhookSubscription, error) {
			return []domain.WebhookSubscription{
				{
					ID:         "sub-1",
					ProjectID:  "proj-1",
					WebhookURL: "https://example.com/hook",
					EventTypes: []string{"run.completed", "run.failed"},
					Active:     true,
				},
			}, nil
		},
	}

	adapter := NewBudgetWebhookAdapter(ws)
	err := adapter.EnqueueBudgetAlert(context.Background(), "proj-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ws.deliveries) != 0 {
		t.Fatalf("expected 0 deliveries for non-matching event type, got %d", len(ws.deliveries))
	}
}

func TestBudgetWebhookAdapter_WildcardEventType(t *testing.T) {
	t.Parallel()

	ws := &mockWebhookStore{
		listSubsFn: func(_ context.Context, _ string) ([]domain.WebhookSubscription, error) {
			return []domain.WebhookSubscription{
				{
					ID:         "sub-1",
					ProjectID:  "proj-1",
					WebhookURL: "https://example.com/hook",
					EventTypes: []string{"*"},
					Active:     true,
				},
			}, nil
		},
	}

	adapter := NewBudgetWebhookAdapter(ws)
	err := adapter.EnqueueBudgetAlert(context.Background(), "proj-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ws.deliveries) != 1 {
		t.Fatalf("expected 1 delivery for wildcard, got %d", len(ws.deliveries))
	}
}

func TestBudgetWebhookAdapter_ListSubsError_ReturnsError(t *testing.T) {
	t.Parallel()

	ws := &mockWebhookStore{
		listSubsFn: func(_ context.Context, _ string) ([]domain.WebhookSubscription, error) {
			return nil, errors.New("db error")
		},
	}

	adapter := NewBudgetWebhookAdapter(ws)
	err := adapter.EnqueueBudgetAlert(context.Background(), "proj-1", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBudgetWebhookAdapter_CreateDeliveryError_ContinuesOthers(t *testing.T) {
	t.Parallel()

	callCount := 0
	ws := &mockWebhookStore{
		listSubsFn: func(_ context.Context, _ string) ([]domain.WebhookSubscription, error) {
			return []domain.WebhookSubscription{
				{ID: "sub-1", WebhookURL: "https://a.com", EventTypes: []string{"compute_budget_warning"}, Active: true},
				{ID: "sub-2", WebhookURL: "https://b.com", EventTypes: []string{"compute_budget_warning"}, Active: true},
			}, nil
		},
		createDeliveryFn: func(_ context.Context, _ *domain.WebhookDelivery) error {
			callCount++
			if callCount == 1 {
				return errors.New("first delivery failed")
			}
			return nil
		},
	}

	adapter := NewBudgetWebhookAdapter(ws)
	err := adapter.EnqueueBudgetAlert(context.Background(), "proj-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both were attempted (2 deliveries tracked even though first failed).
	if callCount != 2 {
		t.Fatalf("expected 2 create delivery calls, got %d", callCount)
	}
}

func TestBudgetWebhookAdapter_NoSubscriptions_NoDeliveries(t *testing.T) {
	t.Parallel()

	ws := &mockWebhookStore{
		listSubsFn: func(_ context.Context, _ string) ([]domain.WebhookSubscription, error) {
			return nil, nil
		},
	}

	adapter := NewBudgetWebhookAdapter(ws)
	err := adapter.EnqueueBudgetAlert(context.Background(), "proj-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ws.deliveries) != 0 {
		t.Fatalf("expected 0 deliveries, got %d", len(ws.deliveries))
	}
}
