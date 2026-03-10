package webhook

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

// mockDeliveryStore implements DeliveryStore for testing.
type mockDeliveryStore struct {
	mu           sync.Mutex
	deliveries   []*domain.WebhookDelivery
	notifyStatus string
}

func (m *mockDeliveryStore) CreateWebhookDelivery(_ context.Context, d *domain.WebhookDelivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d.ID == "" {
		d.ID = "whd-test"
	}
	d.CreatedAt = time.Now()
	d.UpdatedAt = time.Now()
	m.deliveries = append(m.deliveries, d)
	return nil
}

func (m *mockDeliveryStore) UpdateWebhookDelivery(_ context.Context, d *domain.WebhookDelivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, existing := range m.deliveries {
		if existing.ID == d.ID {
			m.deliveries[i] = d
			return nil
		}
	}
	return nil
}

func (m *mockDeliveryStore) ListPendingWebhookRetries(_ context.Context) ([]domain.WebhookDelivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var pending []domain.WebhookDelivery
	now := time.Now()
	for _, d := range m.deliveries {
		if d.Status == domain.WebhookStatusPending && d.NextRetryAt != nil && !d.NextRetryAt.After(now) {
			pending = append(pending, *d)
		}
	}
	return pending, nil
}

func (m *mockDeliveryStore) UpdateEventTriggerNotifyStatus(_ context.Context, _ string, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifyStatus = status
	return nil
}

func (m *mockDeliveryStore) getNotifyStatus() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.notifyStatus
}

func (m *mockDeliveryStore) getDeliveries() []*domain.WebhookDelivery {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]*domain.WebhookDelivery, len(m.deliveries))
	copy(cp, m.deliveries)
	return cp
}

func TestNotifyAsync_EnqueuesDelivery(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	notifier := NewEventNotifier(ms, slog.Default())

	trigger := &domain.EventTrigger{
		ID:        "evt-1",
		EventKey:  "test-key",
		ProjectID: "proj-1",
		NotifyURL: "http://example.com/hook",
		ExpiresAt: time.Now().Add(time.Hour),
	}

	notifier.NotifyAsync(trigger)

	deliveries := ms.getDeliveries()
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	d := deliveries[0]
	if d.EventTriggerID != "evt-1" {
		t.Fatalf("expected trigger_id=evt-1, got %s", d.EventTriggerID)
	}
	if d.WebhookURL != "http://example.com/hook" {
		t.Fatalf("expected url=http://example.com/hook, got %s", d.WebhookURL)
	}
	if d.Status != domain.WebhookStatusPending {
		t.Fatalf("expected status=pending, got %s", d.Status)
	}
	if d.MaxAttempts != 5 {
		t.Fatalf("expected max_attempts=5, got %d", d.MaxAttempts)
	}
}

func TestNotifyAsync_NoURL_Skips(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	notifier := NewEventNotifier(ms, slog.Default())

	notifier.NotifyAsync(&domain.EventTrigger{ID: "evt-2", EventKey: "no-url"})

	if len(ms.getDeliveries()) != 0 {
		t.Fatal("expected no deliveries for trigger without URL")
	}
}

func TestWorker_DeliversSuccessfully(t *testing.T) {
	t.Parallel()

	var receivedPayload map[string]any
	var mu sync.Mutex
	var delivered atomic.Bool

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
		delivered.Store(true)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	notifier := NewEventNotifier(ms, slog.Default())

	trigger := &domain.EventTrigger{
		ID:        "evt-3",
		EventKey:  "deliver-key",
		ProjectID: "proj-1",
		NotifyURL: ts.URL,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	notifier.NotifyAsync(trigger)

	// Run worker once.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = notifier.RunWorker(ctx, 100*time.Millisecond)
	}()

	// Wait for delivery.
	deadline := time.After(5 * time.Second)
	for !delivered.Load() {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for delivery")
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Wait for store update.
	time.Sleep(100 * time.Millisecond)

	if ms.getNotifyStatus() != "sent" {
		t.Fatalf("expected notify_status=sent, got %s", ms.getNotifyStatus())
	}

	mu.Lock()
	if receivedPayload["event_key"] != "deliver-key" {
		t.Fatalf("expected event_key=deliver-key, got %v", receivedPayload["event_key"])
	}
	mu.Unlock()

	// Verify delivery record updated.
	for _, d := range ms.getDeliveries() {
		if d.EventTriggerID == "evt-3" && d.Status != domain.WebhookStatusDelivered {
			t.Fatalf("expected status=delivered, got %s", d.Status)
		}
	}
}

func TestWorker_ServerError_RetriesWithBackoff(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	notifier := NewEventNotifier(ms, slog.Default())

	trigger := &domain.EventTrigger{
		ID:        "evt-4",
		EventKey:  "fail-key",
		ProjectID: "proj-1",
		NotifyURL: ts.URL,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	notifier.NotifyAsync(trigger)

	// Run worker — first attempt should fail and schedule next_retry_at in the future.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = notifier.RunWorker(ctx, 100*time.Millisecond)
	}()

	// Wait for first attempt.
	deadline := time.After(2 * time.Second)
	for attempts.Load() < 1 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for first attempt")
		case <-time.After(50 * time.Millisecond):
		}
	}

	time.Sleep(200 * time.Millisecond)
	cancel()

	// Should only have had 1 attempt — next retry is 5s in the future.
	if a := attempts.Load(); a != 1 {
		t.Fatalf("expected 1 attempt (next retry is in the future), got %d", a)
	}

	// Delivery should still be pending with increased attempts.
	for _, d := range ms.getDeliveries() {
		if d.EventTriggerID == "evt-4" {
			if d.Attempts != 1 {
				t.Fatalf("expected 1 attempt recorded, got %d", d.Attempts)
			}
			if d.Status != domain.WebhookStatusPending {
				t.Fatalf("expected status=pending after first failure, got %s", d.Status)
			}
			if d.NextRetryAt == nil || d.NextRetryAt.Before(time.Now()) {
				t.Fatal("expected next_retry_at to be in the future")
			}
		}
	}
}

func TestWorker_ClientError_DeadLetters(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest) // 400 → not retryable
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	notifier := NewEventNotifier(ms, slog.Default())

	trigger := &domain.EventTrigger{
		ID:        "evt-5",
		EventKey:  "client-err",
		ProjectID: "proj-1",
		NotifyURL: ts.URL,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	notifier.NotifyAsync(trigger)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = notifier.RunWorker(ctx, 100*time.Millisecond)
	}()

	// Wait for processing.
	time.Sleep(500 * time.Millisecond)
	cancel()

	if ms.getNotifyStatus() != "failed" {
		t.Fatalf("expected notify_status=failed, got %s", ms.getNotifyStatus())
	}

	for _, d := range ms.getDeliveries() {
		if d.EventTriggerID == "evt-5" && d.Status != domain.WebhookStatusDead {
			t.Fatalf("expected status=dead for client error, got %s", d.Status)
		}
	}
}

func TestPow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		base, exp, want int
	}{
		{5, 0, 1},
		{5, 1, 5},
		{5, 2, 25},
		{5, 3, 125},
		{5, 4, 625},
	}
	for _, tc := range cases {
		if got := pow(tc.base, tc.exp); got != tc.want {
			t.Errorf("pow(%d, %d) = %d, want %d", tc.base, tc.exp, got, tc.want)
		}
	}
}
