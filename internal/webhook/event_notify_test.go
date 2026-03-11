package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

// mockDeliveryStore implements DeliveryStore for testing.
type mockDeliveryStore struct {
	mu            sync.Mutex
	deliveries    []*domain.WebhookDelivery
	notifyStatus  string
	listPendingFn func(context.Context) ([]domain.WebhookDelivery, error)
}

func (m *mockDeliveryStore) CreateWebhookDelivery(_ context.Context, d *domain.WebhookDelivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d.ID == "" {
		d.ID = fmt.Sprintf("whd-test-%d", len(m.deliveries)+1)
	}
	d.CreatedAt = time.Now()
	d.UpdatedAt = time.Now()
	m.deliveries = append(m.deliveries, d)
	return nil
}

func TestEnqueueRunWebhook_EnqueuesTerminalRunDelivery(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	job := &domain.Job{
		ID:         "job-1",
		ProjectID:  "proj-1",
		WebhookURL: "http://example.com/run-hook",
	}
	run := &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusCompleted,
		Attempt:   2,
		Result:    json.RawMessage(`{"ok":true}`),
	}

	if err := worker.EnqueueRunWebhook(context.Background(), job, run); err != nil {
		t.Fatalf("enqueue run webhook: %v", err)
	}

	deliveries := ms.getDeliveries()
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}

	d := deliveries[0]
	if d.RunID != run.ID {
		t.Fatalf("expected run_id=%s, got %s", run.ID, d.RunID)
	}
	if d.JobID != job.ID {
		t.Fatalf("expected job_id=%s, got %s", job.ID, d.JobID)
	}
	if d.EventTriggerID != "" {
		t.Fatalf("expected empty event_trigger_id, got %s", d.EventTriggerID)
	}
	if d.WebhookURL != job.WebhookURL {
		t.Fatalf("expected webhook_url=%s, got %s", job.WebhookURL, d.WebhookURL)
	}
	if d.Status != domain.WebhookStatusPending {
		t.Fatalf("expected status=pending, got %s", d.Status)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(d.LastError), &payload); err != nil {
		t.Fatalf("expected JSON payload in last_error: %v", err)
	}
	if payload["run_id"] != run.ID {
		t.Fatalf("expected payload run_id=%s, got %v", run.ID, payload["run_id"])
	}
	if payload["status"] != string(run.Status) {
		t.Fatalf("expected payload status=%s, got %v", run.Status, payload["status"])
	}
}

func TestProcessBatch_ConcurrentDelivery(t *testing.T) {
	t.Parallel()

	const total = 10
	var inFlight atomic.Int32
	var maxInFlight atomic.Int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		current := inFlight.Add(1)
		for {
			max := maxInFlight.Load()
			if current <= max {
				break
			}
			if maxInFlight.CompareAndSwap(max, current) {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
		inFlight.Add(-1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	for i := range total {
		id := fmt.Sprintf("batch-%d", i)
		delivery := &domain.WebhookDelivery{
			ID:          id,
			RunID:       fmt.Sprintf("run-%d", i),
			JobID:       fmt.Sprintf("job-%d", i),
			WebhookURL:  ts.URL,
			Status:      domain.WebhookStatusPending,
			Attempts:    0,
			MaxAttempts: 5,
			NextRetryAt: &now,
			LastError:   fmt.Sprintf(`{"delivery_id":"%s"}`, id),
		}
		if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
			t.Fatalf("create delivery: %v", err)
		}
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithConcurrency(total))
	worker.processBatch(context.Background())

	if maxInFlight.Load() <= 1 {
		t.Fatalf("expected concurrent processing, max in-flight=%d", maxInFlight.Load())
	}

	for _, d := range ms.getDeliveries() {
		if d.Status != domain.WebhookStatusDelivered {
			t.Fatalf("expected all deliveries to be delivered, got %s for %s", d.Status, d.ID)
		}
	}
}

func TestProcessBatch_MixedEventAndRunDeliveries(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)

	eventDelivery := &domain.WebhookDelivery{
		ID:             "evt-delivery",
		EventTriggerID: "evt-10",
		WebhookURL:     ts.URL,
		Status:         domain.WebhookStatusPending,
		MaxAttempts:    5,
		NextRetryAt:    &now,
		LastError:      `{"event_key":"k"}`,
	}
	runDelivery := &domain.WebhookDelivery{
		ID:          "run-delivery",
		RunID:       "run-10",
		JobID:       "job-10",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   `{"run_id":"run-10"}`,
	}

	if err := ms.CreateWebhookDelivery(context.Background(), eventDelivery); err != nil {
		t.Fatalf("create event delivery: %v", err)
	}
	if err := ms.CreateWebhookDelivery(context.Background(), runDelivery); err != nil {
		t.Fatalf("create run delivery: %v", err)
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithConcurrency(2))
	worker.processBatch(context.Background())

	if requests.Load() != 2 {
		t.Fatalf("expected 2 webhook requests, got %d", requests.Load())
	}

	for _, d := range ms.getDeliveries() {
		if d.Status != domain.WebhookStatusDelivered {
			t.Fatalf("expected delivery %s to be delivered, got %s", d.ID, d.Status)
		}
	}
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

func (m *mockDeliveryStore) ListPendingWebhookRetries(ctx context.Context) ([]domain.WebhookDelivery, error) {
	if m.listPendingFn != nil {
		return m.listPendingFn(ctx)
	}

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

func TestDeliveryWorker_Shutdown_Idle(t *testing.T) {
	t.Parallel()

	worker := NewDeliveryWorker(&mockDeliveryStore{}, slog.Default())
	runCtx, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)

	runDone := make(chan error, 1)
	go func() {
		runDone <- worker.RunWorker(runCtx, time.Hour)
	}()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	if err := worker.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v, want nil", err)
	}

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("RunWorker() error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunWorker did not stop after shutdown")
	}
}

func TestDeliveryWorker_Shutdown_WaitsForBatch(t *testing.T) {
	t.Parallel()

	batchStarted := make(chan struct{})
	allowBatchExit := make(chan struct{})

	store := &mockDeliveryStore{
		listPendingFn: func(ctx context.Context) ([]domain.WebhookDelivery, error) {
			select {
			case <-batchStarted:
			default:
				close(batchStarted)
			}
			select {
			case <-allowBatchExit:
				return nil, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}

	worker := NewDeliveryWorker(store, slog.Default())
	runCtx, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)

	runDone := make(chan error, 1)
	go func() {
		runDone <- worker.RunWorker(runCtx, time.Millisecond)
	}()

	select {
	case <-batchStarted:
	case <-time.After(time.Second):
		t.Fatal("batch did not start")
	}

	shutdownDone := make(chan error, 1)
	go func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		shutdownDone <- worker.Shutdown(shutdownCtx)
	}()

	select {
	case err := <-shutdownDone:
		t.Fatalf("Shutdown returned early with err=%v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(allowBatchExit)

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("Shutdown() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown did not return after batch completed")
	}

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("RunWorker() error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunWorker did not stop after shutdown")
	}
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

func TestWorker_PayloadTooLarge_DeadLettersWithoutHTTPCall(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	largePayload := `{"payload":"` + strings.Repeat("x", 2048) + `"}`
	delivery := &domain.WebhookDelivery{
		ID:          "too-large",
		RunID:       "run-too-large",
		JobID:       "job-too-large",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   largePayload,
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	notifier := NewEventNotifier(ms, slog.Default(), WithMaxPayloadBytes(1024))
	notifier.processBatch(context.Background())

	if requests.Load() != 0 {
		t.Fatalf("expected no HTTP requests, got %d", requests.Load())
	}

	updated := ms.getDeliveries()[0]
	if updated.Status != domain.WebhookStatusDead {
		t.Fatalf("expected status=dead, got %s", updated.Status)
	}
	if !strings.Contains(updated.LastError, "payload too large") {
		t.Fatalf("expected payload too large error, got %q", updated.LastError)
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

func TestBackoffForRetryPolicy_Linear(t *testing.T) {
	t.Parallel()

	if got := backoffForRetryPolicy(domain.WebhookRetryPolicyLinear, 1); got != 5*time.Second {
		t.Fatalf("linear attempt 1 backoff = %s, want %s", got, 5*time.Second)
	}

	if got := backoffForRetryPolicy(domain.WebhookRetryPolicyLinear, 3); got != 15*time.Second {
		t.Fatalf("linear attempt 3 backoff = %s, want %s", got, 15*time.Second)
	}
}

func TestBackoffForRetryPolicy_Fixed(t *testing.T) {
	t.Parallel()

	if got := backoffForRetryPolicy(domain.WebhookRetryPolicyFixed, 1); got != 5*time.Second {
		t.Fatalf("fixed attempt 1 backoff = %s, want %s", got, 5*time.Second)
	}

	if got := backoffForRetryPolicy(domain.WebhookRetryPolicyFixed, 7); got != 5*time.Second {
		t.Fatalf("fixed attempt 7 backoff = %s, want %s", got, 5*time.Second)
	}
}
