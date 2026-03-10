package webhook

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
)

type mockNotifyStore struct {
	mu     sync.Mutex
	status string
}

func (m *mockNotifyStore) UpdateEventTriggerNotifyStatus(_ context.Context, _ string, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = status
	return nil
}

func (m *mockNotifyStore) getStatus() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func TestEventNotifier_NotifyAsync_Success(t *testing.T) {
	t.Parallel()

	var receivedPayload map[string]any
	var mu sync.Mutex
	done := make(chan struct{})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
		close(done)
	}))
	defer ts.Close()

	ms := &mockNotifyStore{}
	notifier := NewEventNotifier(ms, slog.Default())

	trigger := &domain.EventTrigger{
		ID:        "evt-1",
		EventKey:  "test-key",
		ProjectID: "proj-1",
		NotifyURL: ts.URL,
		ExpiresAt: time.Now().Add(time.Hour),
	}

	notifier.NotifyAsync(trigger)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for notify")
	}

	// Verify status updated to sent.
	time.Sleep(50 * time.Millisecond) // Allow goroutine to finish store update.
	if ms.getStatus() != "sent" {
		t.Fatalf("expected notify_status=sent, got %s", ms.getStatus())
	}

	mu.Lock()
	if receivedPayload["event_key"] != "test-key" {
		t.Fatalf("expected event_key=test-key, got %v", receivedPayload["event_key"])
	}
	if receivedPayload["trigger_id"] != "evt-1" {
		t.Fatalf("expected trigger_id=evt-1, got %v", receivedPayload["trigger_id"])
	}
	mu.Unlock()
}

func TestEventNotifier_NotifyAsync_ServerError_Retries(t *testing.T) {
	t.Parallel()

	var attempts int32
	var attemptsMu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attemptsMu.Lock()
		attempts++
		attemptsMu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	ms := &mockNotifyStore{}
	notifier := NewEventNotifier(ms, slog.Default())

	trigger := &domain.EventTrigger{
		ID:        "evt-2",
		EventKey:  "fail-key",
		ProjectID: "proj-1",
		NotifyURL: ts.URL,
		ExpiresAt: time.Now().Add(time.Hour),
	}

	notifier.NotifyAsync(trigger)

	// Wait for all 3 retries to complete (1s + 2s backoff + execution).
	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for retries")
		case <-time.After(100 * time.Millisecond):
			attemptsMu.Lock()
			n := attempts
			attemptsMu.Unlock()
			if n >= int32(maxNotifyAttempts) {
				goto done
			}
		}
	}
done:
	time.Sleep(100 * time.Millisecond) // Allow goroutine to finish store update.

	attemptsMu.Lock()
	if attempts != int32(maxNotifyAttempts) {
		t.Fatalf("expected %d attempts, got %d", maxNotifyAttempts, attempts)
	}
	attemptsMu.Unlock()

	if ms.getStatus() != "failed" {
		t.Fatalf("expected notify_status=failed, got %s", ms.getStatus())
	}
}

func TestEventNotifier_NotifyAsync_ClientError_NoRetry(t *testing.T) {
	t.Parallel()

	var attempts int32
	var attemptsMu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attemptsMu.Lock()
		attempts++
		attemptsMu.Unlock()
		w.WriteHeader(http.StatusBadRequest) // 400 → not retryable
	}))
	defer ts.Close()

	ms := &mockNotifyStore{}
	notifier := NewEventNotifier(ms, slog.Default())

	trigger := &domain.EventTrigger{
		ID:        "evt-4",
		EventKey:  "client-err-key",
		ProjectID: "proj-1",
		NotifyURL: ts.URL,
		ExpiresAt: time.Now().Add(time.Hour),
	}

	notifier.NotifyAsync(trigger)
	time.Sleep(500 * time.Millisecond)

	attemptsMu.Lock()
	if attempts != 1 {
		t.Fatalf("expected 1 attempt for 4xx, got %d", attempts)
	}
	attemptsMu.Unlock()

	if ms.getStatus() != "failed" {
		t.Fatalf("expected notify_status=failed, got %s", ms.getStatus())
	}
}

func TestEventNotifier_NotifyAsync_NoURL(t *testing.T) {
	t.Parallel()

	ms := &mockNotifyStore{}
	notifier := NewEventNotifier(ms, slog.Default())

	trigger := &domain.EventTrigger{
		ID:       "evt-3",
		EventKey: "no-url",
	}

	notifier.NotifyAsync(trigger)
	time.Sleep(50 * time.Millisecond)

	if ms.getStatus() != "" {
		t.Fatalf("expected no status update for empty URL, got %s", ms.getStatus())
	}
}
