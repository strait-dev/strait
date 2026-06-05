package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"

	"strait/internal/clickhouse"
	"strait/internal/domain"
	"strait/internal/httputil"
	"strait/internal/telemetry"

	"github.com/sourcegraph/conc"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func init() {
	newDefaultDeliveryTransport = func(bool) *http.Transport {
		return httputil.NewExternalTransport(true)
	}
}

func TestApplyWebhookDeadLetterSentryScope(t *testing.T) {
	t.Parallel()

	statusCode := http.StatusBadGateway
	delivery := &domain.WebhookDelivery{
		ID:             "del-1",
		RunID:          "run-1",
		JobID:          "job-1",
		ProjectID:      "proj-1",
		OrgID:          "org-1",
		EventTriggerID: "trigger-1",
		SubscriptionID: "sub-1",
		WebhookURL:     "https://hooks.example.com/path",
		RetryPolicy:    domain.WebhookRetryPolicyExponential,
		Attempts:       3,
		MaxAttempts:    3,
		LastStatusCode: &statusCode,
	}

	scope := sentry.NewScope()
	applyWebhookDeadLetterSentryScope(scope, delivery, true, "bad gateway")
	event := scope.ApplyToEvent(&sentry.Event{}, nil, nil)

	wantTags := map[string]string{
		"subsystem":       "webhook",
		"delivery_id":     "del-1",
		"run_id":          "run-1",
		"job_id":          "job-1",
		"project_id":      "proj-1",
		"org_id":          "org-1",
		"trigger_id":      "trigger-1",
		"subscription_id": "sub-1",
		"attempt":         "3",
		"operation":       "dead_letter",
	}
	for key, want := range wantTags {
		if got := event.Tags[key]; got != want {
			t.Fatalf("tag %s = %q, want %q", key, got, want)
		}
	}
	if event.Contexts["webhook.delivery"]["webhook_url_domain"] != "hooks.example.com" {
		t.Fatalf("webhook_url_domain context = %v, want hooks.example.com", event.Contexts["webhook.delivery"]["webhook_url_domain"])
	}
}

// mockDeliveryStore implements DeliveryStore for testing.
type mockDeliveryStore struct {
	mu            sync.Mutex
	deliveries    []*domain.WebhookDelivery
	notifyStatus  string
	listPendingFn func(context.Context) ([]domain.WebhookDelivery, error)
	subSecret     string
	getSecretsFn  func(ctx context.Context, subscriptionID string) (string, string, *time.Time, error)
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
		ID:            "job-1",
		ProjectID:     "proj-1",
		WebhookURL:    "http://example.com/run-hook",
		WebhookSecret: "job-webhook-secret",
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
	if d.WebhookSecret != job.WebhookSecret {
		t.Fatalf("expected webhook secret to be preserved")
	}
	if d.Status != domain.WebhookStatusPending {
		t.Fatalf("expected status=pending, got %s", d.Status)
	}

	if d.LastError != "" {
		t.Fatalf("expected last_error empty on enqueue, got %q", d.LastError)
	}
	var payload map[string]any
	if err := json.Unmarshal(d.Payload, &payload); err != nil {
		t.Fatalf("expected JSON payload on delivery: %v", err)
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
			peak := maxInFlight.Load()
			if current <= peak {
				break
			}
			if maxInFlight.CompareAndSwap(peak, current) {
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
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	worker := NewDeliveryWorker(&mockDeliveryStore{}, slog.Default())
	runCtx, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)

	runDone := make(chan error, 1)
	concWG.Go(func() {
		runDone <- worker.RunWorker(runCtx, time.Hour)
	})

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
	var concWG conc.WaitGroup
	defer concWG.Wait()
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
	concWG.Go(func() {
		runDone <- worker.RunWorker(runCtx, time.Millisecond)
	})

	select {
	case <-batchStarted:
	case <-time.After(time.Second):
		t.Fatal("batch did not start")
	}

	shutdownDone := make(chan error, 1)
	concWG.Go(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		shutdownDone <- worker.Shutdown(shutdownCtx)
	})

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

func (m *mockDeliveryStore) GetWebhookSubscriptionSecrets(ctx context.Context, subscriptionID string) (string, string, *time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getSecretsFn != nil {
		return m.getSecretsFn(ctx, subscriptionID)
	}
	return m.subSecret, "", nil, nil
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
	var concWG conc.WaitGroup
	defer concWG.Wait()
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
	concWG.Go(func() {
		_ = notifier.RunWorker(ctx, 100*time.Millisecond)
	})

	// Wait for delivery.
	deadline := time.After(5 * time.Second)
	for !delivered.Load() {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for delivery")
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Poll for store update instead of sleeping.
	deadline2 := time.After(5 * time.Second)
	for ms.getNotifyStatus() != "sent" {
		select {
		case <-deadline2:
			t.Fatalf("timed out waiting for notify_status=sent, got %s", ms.getNotifyStatus())
		case <-time.After(10 * time.Millisecond):
		}
	}

	mu.Lock()
	if receivedPayload["event_key"] != "deliver-key" {
		t.Fatalf("expected event_key=deliver-key, got %v", receivedPayload["event_key"])
	}
	mu.Unlock()

	for _, d := range ms.getDeliveries() {
		if d.EventTriggerID == "evt-3" && d.Status != domain.WebhookStatusDelivered {
			t.Fatalf("expected status=delivered, got %s", d.Status)
		}
	}
}

func TestWorker_ServerError_RetriesWithBackoff(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
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
	concWG.Go(func() {
		_ = notifier.RunWorker(ctx, 100*time.Millisecond)
	})

	// Wait for first attempt.
	deadline := time.After(2 * time.Second)
	for attempts.Load() < 1 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for first attempt")
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Run a few more poll cycles to confirm no second attempt fires.
	// The next retry is 5s in the future, so the worker should be idle.
	stableAt := time.Now()
	for time.Since(stableAt) < 300*time.Millisecond {
		if attempts.Load() > 1 {
			t.Fatalf("unexpected extra attempt; expected exactly 1")
		}
		time.Sleep(10 * time.Millisecond) // tight poll to detect spurious retry
	}
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
	var concWG conc.WaitGroup
	defer concWG.Wait()
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
	concWG.Go(func() {
		_ = notifier.RunWorker(ctx, 100*time.Millisecond)
	})

	// Poll for processing instead of sleeping.
	deadline := time.After(5 * time.Second)
	for ms.getNotifyStatus() != "failed" {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for notify_status=failed, got %s", ms.getNotifyStatus())
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()

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

func TestWithChExporter_EnqueuesOnDelivery(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:       true,
		BatchSize:     100,
		FlushInterval: time.Minute, // won't auto-flush during test
	}, slog.Default())

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		ID:          "whd-ch-1",
		RunID:       "run-ch-1",
		JobID:       "job-ch-1",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   `{"run_id":"run-ch-1"}`,
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithChExporter(exporter))
	worker.processBatch(context.Background())

	if exporter.PendingCount() != 1 {
		t.Fatalf("expected 1 pending ClickHouse record, got %d", exporter.PendingCount())
	}
}

func TestWithChExporter_EnqueuesOnFailure(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:       true,
		BatchSize:     100,
		FlushInterval: time.Minute,
	}, slog.Default())

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		ID:          "whd-ch-fail",
		RunID:       "run-ch-fail",
		JobID:       "job-ch-fail",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   `{"run_id":"run-ch-fail"}`,
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithChExporter(exporter))
	worker.processBatch(context.Background())

	if exporter.PendingCount() != 1 {
		t.Fatalf("expected 1 pending ClickHouse record after failure, got %d", exporter.PendingCount())
	}
}

func TestWithChExporter_NilExporter_NoPanic(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		ID:          "whd-nil-ch",
		RunID:       "run-nil-ch",
		JobID:       "job-nil-ch",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   `{"run_id":"run-nil-ch"}`,
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	// No WithChExporter option -- should not panic
	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())
}

func TestExponentialWebhookBackoff(t *testing.T) {
	t.Parallel()

	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{1, 5 * time.Second},
		{2, 25 * time.Second},
		{3, 125 * time.Second},
		{4, 625 * time.Second},
	}
	for _, tc := range cases {
		if got := exponentialWebhookBackoff(tc.attempts); got != tc.want {
			t.Errorf("exponentialWebhookBackoff(%d) = %s, want %s", tc.attempts, got, tc.want)
		}
	}
}

// approxBackoff asserts that got is within +/- 20% of want. The webhook
// backoff helper applies decorrelated jitter, so we can't compare for
// exact equality.
func approxBackoff(t *testing.T, got, want time.Duration, label string) {
	t.Helper()
	low := want - want/5
	high := want + want/5
	if got < low || got > high {
		t.Fatalf("%s = %s, want within [%s, %s]", label, got, low, high)
	}
}

func TestBackoffForRetryPolicy_Linear(t *testing.T) {
	t.Parallel()

	approxBackoff(t, backoffForRetryPolicy(domain.WebhookRetryPolicyLinear, 1), 5*time.Second, "linear attempt 1")
	approxBackoff(t, backoffForRetryPolicy(domain.WebhookRetryPolicyLinear, 3), 15*time.Second, "linear attempt 3")
}

func TestBackoffForRetryPolicy_Fixed(t *testing.T) {
	t.Parallel()

	approxBackoff(t, backoffForRetryPolicy(domain.WebhookRetryPolicyFixed, 1), 5*time.Second, "fixed attempt 1")
	approxBackoff(t, backoffForRetryPolicy(domain.WebhookRetryPolicyFixed, 7), 5*time.Second, "fixed attempt 7")
}

func TestEnqueueSubscriptionWebhooks_MatchingSubscription(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	subs := []domain.WebhookSubscription{{
		ID: "sub-1", ProjectID: "proj-1", WebhookURL: "https://example.com/hook",
		Active: true, EventTypes: []string{"run.completed"},
	}}

	worker.EnqueueSubscriptionWebhooks(context.Background(), subs, "run.completed", json.RawMessage(`{"run_id":"r1"}`))

	deliveries := ms.getDeliveries()
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	if deliveries[0].WebhookURL != "https://example.com/hook" {
		t.Fatalf("expected webhook URL https://example.com/hook, got %s", deliveries[0].WebhookURL)
	}
	if deliveries[0].SubscriptionID != "sub-1" {
		t.Fatalf("subscription_id = %q, want sub-1", deliveries[0].SubscriptionID)
	}
	if deliveries[0].ProjectID != "proj-1" {
		t.Fatalf("project_id = %q, want proj-1", deliveries[0].ProjectID)
	}
	if string(deliveries[0].Payload) != `{"run_id":"r1"}` {
		t.Fatalf("payload = %s, want original payload", deliveries[0].Payload)
	}
}

func TestEnqueueSubscriptionWebhooks_WildcardMatch(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	subs := []domain.WebhookSubscription{{
		ID: "sub-wc", WebhookURL: "https://example.com/wildcard",
		Active: true, EventTypes: []string{"*"},
	}}

	worker.EnqueueSubscriptionWebhooks(context.Background(), subs, "run.failed", json.RawMessage(`{"run_id":"r2"}`))

	deliveries := ms.getDeliveries()
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	if deliveries[0].WebhookURL != "https://example.com/wildcard" {
		t.Fatalf("expected webhook URL https://example.com/wildcard, got %s", deliveries[0].WebhookURL)
	}
}

func TestEnqueueSubscriptionWebhooks_InactiveSkipped(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	subs := []domain.WebhookSubscription{{
		ID: "sub-inactive", WebhookURL: "https://example.com/hook",
		Active: false, EventTypes: []string{"run.completed"},
	}}

	worker.EnqueueSubscriptionWebhooks(context.Background(), subs, "run.completed", json.RawMessage(`{"run_id":"r3"}`))

	if len(ms.getDeliveries()) != 0 {
		t.Fatalf("expected 0 deliveries for inactive sub, got %d", len(ms.getDeliveries()))
	}
}

func TestEnqueueSubscriptionWebhooks_EventTypeMismatch(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	subs := []domain.WebhookSubscription{{
		ID: "sub-mismatch", WebhookURL: "https://example.com/hook",
		Active: true, EventTypes: []string{"run.failed"},
	}}

	worker.EnqueueSubscriptionWebhooks(context.Background(), subs, "run.completed", json.RawMessage(`{"run_id":"r4"}`))

	if len(ms.getDeliveries()) != 0 {
		t.Fatalf("expected 0 deliveries for mismatched event type, got %d", len(ms.getDeliveries()))
	}
}

func TestEnqueueSubscriptionWebhooks_MultipleSubs(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	subs := []domain.WebhookSubscription{
		{ID: "sub-match", WebhookURL: "https://example.com/match", Active: true, EventTypes: []string{"run.completed"}},
		{ID: "sub-inactive", WebhookURL: "https://example.com/inactive", Active: false, EventTypes: []string{"run.completed"}},
		{ID: "sub-wrong-type", WebhookURL: "https://example.com/wrong", Active: true, EventTypes: []string{"run.failed"}},
	}

	worker.EnqueueSubscriptionWebhooks(context.Background(), subs, "run.completed", json.RawMessage(`{"run_id":"r5"}`))

	deliveries := ms.getDeliveries()
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	if deliveries[0].WebhookURL != "https://example.com/match" {
		t.Fatalf("expected webhook URL https://example.com/match, got %s", deliveries[0].WebhookURL)
	}
}

func TestReplayKeyFromDeliveryID(t *testing.T) {
	t.Parallel()
	if got := replayKeyFromDeliveryID(""); got != "" {
		t.Fatalf("empty delivery id should yield empty key, got %q", got)
	}
	if got := replayKeyFromDeliveryID("whd-42"); got != "rk_whd-42" {
		t.Fatalf("unexpected replay key: %q", got)
	}
}

// TestComputeReplayKey_HMACDerivation asserts the signed replay key is
// exactly hex(hmac_sha256(secret, delivery_id)) truncated to
// replayKeyHexLen, prefixed with rk_. A subscriber holding the secret
// must be able to re-derive and verify the key for a given delivery id.
func TestComputeReplayKey_HMACDerivation(t *testing.T) {
	t.Parallel()

	secret := []byte("whsec_test_secret_bytes")
	deliveryID := "whd-signed-1"

	got := ComputeReplayKey(secret, deliveryID)

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(deliveryID))
	want := replayKeyPrefix + hex.EncodeToString(mac.Sum(nil))[:replayKeyHexLen]

	if got != want {
		t.Fatalf("ComputeReplayKey=%q, want %q", got, want)
	}
	if !strings.HasPrefix(got, "rk_") {
		t.Fatalf("expected rk_ prefix, got %q", got)
	}
	if len(got) != len("rk_")+replayKeyHexLen {
		t.Fatalf("expected total length %d, got %d", len("rk_")+replayKeyHexLen, len(got))
	}

	// Same inputs must produce the same key (stability across retries).
	again := ComputeReplayKey(secret, deliveryID)
	if again != got {
		t.Fatalf("non-deterministic: %q vs %q", got, again)
	}

	// Different secret must yield a different key (HMAC binding).
	other := ComputeReplayKey([]byte("different_secret"), deliveryID)
	if other == got {
		t.Fatalf("key should change with secret")
	}

	// Empty delivery id returns empty.
	if ComputeReplayKey(secret, "") != "" {
		t.Fatalf("empty id must yield empty key")
	}

	// Empty secret falls back to unsigned helper.
	unsigned := ComputeReplayKey(nil, deliveryID)
	if unsigned != ComputeReplayKeyUnsigned(deliveryID) {
		t.Fatalf("nil secret should match unsigned derivation")
	}
}

func TestComputeReplayKeyUnsigned(t *testing.T) {
	t.Parallel()
	if got := ComputeReplayKeyUnsigned(""); got != "" {
		t.Fatalf("empty id should yield empty key, got %q", got)
	}
	if got := ComputeReplayKeyUnsigned("run-9"); got != "rk_run-9" {
		t.Fatalf("unexpected unsigned key: %q", got)
	}
}

func TestComputeIdempotencyKey_HMACDerivation(t *testing.T) {
	t.Parallel()

	secret := []byte("whsec_test_secret_bytes")
	deliveryID := "whd-signed-1"

	got := ComputeIdempotencyKey(secret, deliveryID, 2)

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("whd-signed-1:2"))
	want := idempotencyKeyPrefix + hex.EncodeToString(mac.Sum(nil))[:replayKeyHexLen]

	if got != want {
		t.Fatalf("ComputeIdempotencyKey=%q, want %q", got, want)
	}
	if strings.Contains(got, deliveryID) {
		t.Fatalf("signed idempotency key leaked delivery ID: %q", got)
	}
	if got == ComputeIdempotencyKey(secret, deliveryID, 1) {
		t.Fatal("idempotency key must change by attempt")
	}
	if unsigned := ComputeIdempotencyKey(nil, deliveryID, 2); unsigned != "whd-signed-1:2" {
		t.Fatalf("nil secret should preserve legacy unsigned key, got %q", unsigned)
	}
}

func TestAttemptDelivery_WithSubscriptionID_AuthenticatesReplayAndIdempotencyKeys(t *testing.T) {
	t.Parallel()

	var receivedReplayKey, receivedIdempotencyKey string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedReplayKey = r.Header.Get("X-Strait-Replay-Key")
		receivedIdempotencyKey = r.Header.Get("X-Strait-Idempotency-Key")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	secret := "test-hmac-secret-key"
	ms := &mockDeliveryStore{
		getSecretsFn: func(_ context.Context, _ string) (string, string, *time.Time, error) {
			return secret, "", nil, nil
		},
	}
	now := time.Now().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		ID:             "whd-authenticated-keys",
		SubscriptionID: "sub-authenticated-keys",
		WebhookURL:     ts.URL,
		Status:         domain.WebhookStatusPending,
		MaxAttempts:    5,
		NextRetryAt:    &now,
		Payload:        json.RawMessage(`{"event":"run.completed"}`),
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())

	wantReplay := ComputeReplayKey([]byte(secret), delivery.ID)
	if receivedReplayKey != wantReplay {
		t.Fatalf("replay key = %q, want %q", receivedReplayKey, wantReplay)
	}
	wantIdempotency := ComputeIdempotencyKey([]byte(secret), delivery.ID, 1)
	if receivedIdempotencyKey != wantIdempotency {
		t.Fatalf("idempotency key = %q, want %q", receivedIdempotencyKey, wantIdempotency)
	}
	for _, header := range []string{receivedReplayKey, receivedIdempotencyKey} {
		if strings.Contains(header, delivery.ID) {
			t.Fatalf("signed webhook key leaked delivery ID: %q", header)
		}
	}
}

// TestAttemptDelivery_ReplayKeyStableAcrossRetries asserts that the new
// X-Strait-Replay-Key header is identical across retries of the same delivery
// row, unlike X-Strait-Idempotency-Key which intentionally changes per
// attempt. This gives subscribers a stable dedup key that survives replays.
func TestAttemptDelivery_ReplayKeyStableAcrossRetries(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var replayKeys []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		replayKeys = append(replayKeys, r.Header.Get("X-Strait-Replay-Key"))
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		ID:          "whd-replay-1",
		RunID:       "run-1",
		JobID:       "job-1",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   `{"run_id":"run-1"}`,
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())

	for _, d := range ms.getDeliveries() {
		if d.ID == "whd-replay-1" {
			retryNow := time.Now().Add(-time.Second)
			d.NextRetryAt = &retryNow
		}
	}
	worker.processBatch(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if len(replayKeys) < 2 {
		t.Fatalf("expected at least 2 attempts, got %d", len(replayKeys))
	}
	want := "rk_whd-replay-1"
	for i, got := range replayKeys {
		if got != want {
			t.Fatalf("attempt %d: X-Strait-Replay-Key=%q, want %q", i, got, want)
		}
	}
}

// TestAttemptDelivery_ReplayKeyDiffersBetweenDeliveries asserts that distinct
// delivery rows produce distinct replay keys (derived from the delivery id).
func TestAttemptDelivery_ReplayKeyDiffersBetweenDeliveries(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	seen := make(map[string]bool)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.Header.Get("X-Strait-Replay-Key")] = true
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	ids := []string{"whd-a", "whd-b", "whd-c"}
	for _, id := range ids {
		d := &domain.WebhookDelivery{
			ID:          id,
			RunID:       "run-" + id,
			JobID:       "job-1",
			WebhookURL:  ts.URL,
			Status:      domain.WebhookStatusPending,
			Attempts:    0,
			MaxAttempts: 3,
			NextRetryAt: &now,
			LastError:   `{"k":"v"}`,
		}
		if err := ms.CreateWebhookDelivery(context.Background(), d); err != nil {
			t.Fatalf("create delivery: %v", err)
		}
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())

	mu.Lock()
	defer mu.Unlock()
	for _, id := range ids {
		if !seen["rk_"+id] {
			t.Fatalf("expected replay key rk_%s not observed; saw %v", id, seen)
		}
	}
}

func TestAttemptDelivery_IdempotencyKeyHeader(t *testing.T) {
	t.Parallel()

	var receivedHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Strait-Idempotency-Key")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		ID:          "whd-idem-1",
		RunID:       "run-1",
		JobID:       "job-1",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    3,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   `{"run_id":"run-1"}`,
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())

	expected := "whd-idem-1:4" // attempts incremented to 4 before dispatch
	if receivedHeader != expected {
		t.Fatalf("expected X-Strait-Idempotency-Key=%s, got %s", expected, receivedHeader)
	}
}

func TestAttemptDelivery_IdempotencyKeyChangesOnRetry(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var receivedKeys []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedKeys = append(receivedKeys, r.Header.Get("X-Strait-Idempotency-Key"))
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError) // force retry
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		ID:          "whd-retry-1",
		RunID:       "run-1",
		JobID:       "job-1",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   `{"run_id":"run-1"}`,
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	worker := NewDeliveryWorker(ms, slog.Default())

	// First attempt
	worker.processBatch(context.Background())

	// Simulate retry: reset next_retry_at to now so it's picked up again
	for _, d := range ms.getDeliveries() {
		if d.ID == "whd-retry-1" {
			retryNow := time.Now().Add(-time.Second)
			d.NextRetryAt = &retryNow
		}
	}

	// Second attempt
	worker.processBatch(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if len(receivedKeys) < 2 {
		t.Fatalf("expected at least 2 attempts, got %d", len(receivedKeys))
	}
	if receivedKeys[0] == receivedKeys[1] {
		t.Fatalf("expected different idempotency keys on retry, got %s both times", receivedKeys[0])
	}
}

func TestAttemptDelivery_DifferentDeliveriesHaveDifferentKeys(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	receivedKeys := make(map[string]bool)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedKeys[r.Header.Get("X-Strait-Idempotency-Key")] = true
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	for _, id := range []string{"whd-diff-1", "whd-diff-2"} {
		d := &domain.WebhookDelivery{
			ID:          id,
			RunID:       "run-1",
			JobID:       "job-1",
			WebhookURL:  ts.URL,
			Status:      domain.WebhookStatusPending,
			Attempts:    0,
			MaxAttempts: 5,
			NextRetryAt: &now,
			LastError:   `{"run_id":"run-1"}`,
		}
		if err := ms.CreateWebhookDelivery(context.Background(), d); err != nil {
			t.Fatalf("create delivery: %v", err)
		}
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithConcurrency(2))
	worker.processBatch(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if len(receivedKeys) != 2 {
		t.Fatalf("expected 2 unique idempotency keys, got %d", len(receivedKeys))
	}
}

func TestDeliveryWorker_DefaultConcurrency50(t *testing.T) {
	t.Parallel()
	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())
	if worker.concurrency != 50 {
		t.Errorf("default concurrency = %d, want 50", worker.concurrency)
	}
}

func TestDeliveryWorker_ConcurrencyFromOption(t *testing.T) {
	t.Parallel()
	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithConcurrency(100))
	if worker.concurrency != 100 {
		t.Errorf("concurrency = %d, want 100", worker.concurrency)
	}
}

func TestDeliveryWorker_ConcurrencyZeroKeepsDefault(t *testing.T) {
	t.Parallel()
	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithConcurrency(0))
	if worker.concurrency != 50 {
		t.Errorf("concurrency = %d, want 50 (default)", worker.concurrency)
	}
}

func TestDeliveryWorker_TieredTimeout_InitialAttempt(t *testing.T) {
	t.Parallel()

	// Server that never responds, forcing the client-side timeout to fire.
	// The done channel unblocks the handler so srv.Close does not deadlock.
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-done
	}))
	defer srv.Close()
	defer close(done)

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	d := &domain.WebhookDelivery{
		ID:          "d-timeout-1",
		WebhookURL:  srv.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0, // Will become 1 in attemptDelivery.
		MaxAttempts: 3,
	}

	start := time.Now()
	worker.attemptDelivery(context.Background(), d)
	elapsed := time.Since(start)

	// Should timeout around 5s, not 10s.
	if elapsed > 6*time.Second {
		t.Errorf("initial attempt took %v, expected ~5s timeout", elapsed)
	}
	if d.Status == domain.WebhookStatusDelivered {
		t.Error("expected delivery to fail due to timeout")
	}
}

func TestDeliveryWorker_TieredTimeout_RetryAttempt(t *testing.T) {
	t.Parallel()

	// Server responds after 5.5s (exceeds initial 5s timeout but within 15s
	// retry timeout). The done channel prevents deadlock on srv.Close.
	const serverDelay = 5500 * time.Millisecond
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(serverDelay):
			w.WriteHeader(http.StatusOK)
		case <-done:
		}
	}))
	defer srv.Close()
	defer close(done)

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	d := &domain.WebhookDelivery{
		ID:          "d-timeout-2",
		WebhookURL:  srv.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    1, // Will become 2 in attemptDelivery (retry).
		MaxAttempts: 3,
	}

	start := time.Now()
	worker.attemptDelivery(context.Background(), d)
	elapsed := time.Since(start)

	// Should succeed because retry timeout is 15s and server responds in ~5.5s.
	if elapsed < 5*time.Second {
		t.Errorf("retry attempt returned too fast: %v", elapsed)
	}
	if d.Status != domain.WebhookStatusDelivered {
		t.Errorf("expected delivery to succeed on retry timeout, got status %s", d.Status)
	}
}

func TestDeliveryWorker_ConcurrentDeliveries50(t *testing.T) {
	t.Parallel()

	var maxConcurrent atomic.Int64
	var currentConcurrent atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		cur := currentConcurrent.Add(1)
		for {
			old := maxConcurrent.Load()
			if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		currentConcurrent.Add(-1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	const total = 60
	ms := &mockDeliveryStore{
		listPendingFn: func(_ context.Context) ([]domain.WebhookDelivery, error) {
			deliveries := make([]domain.WebhookDelivery, 0, total)
			for i := range total {
				deliveries = append(deliveries, domain.WebhookDelivery{
					ID:          fmt.Sprintf("d-%d", i),
					WebhookURL:  srv.URL,
					Status:      domain.WebhookStatusPending,
					MaxAttempts: 1,
				})
			}
			return deliveries, nil
		},
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithConcurrency(total))
	worker.processBatch(context.Background())

	peak := maxConcurrent.Load()
	if peak < 10 {
		t.Errorf("peak concurrency = %d, expected higher with 60 deliveries", peak)
	}
}

// HTTP Transport tests.

func TestWithHTTPTransport_SetsCustomTransport(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(),
		WithHTTPTransport(10*time.Second, 90*time.Second, 200, 100),
	)

	transport, ok := worker.client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport after WithHTTPTransport")
	}
	if transport.MaxIdleConns != 200 {
		t.Errorf("MaxIdleConns = %d, want 200", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 100 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 100", transport.MaxIdleConnsPerHost)
	}
	if transport.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 90s", transport.IdleConnTimeout)
	}
	if !transport.ForceAttemptHTTP2 {
		t.Error("expected ForceAttemptHTTP2 = true")
	}
	if worker.client.Timeout != 10*time.Second {
		t.Errorf("client.Timeout = %v, want 10s", worker.client.Timeout)
	}
}

func TestWithHTTPTransport_ConnectionReuse(t *testing.T) {
	t.Parallel()

	var connCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Track new connections via a custom dialer wrapper.
	baseTransport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			connCount.Add(1)
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     time.Minute,
	}

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())
	worker.client = &http.Client{Transport: baseTransport}

	// Send 10 deliveries sequentially to the same host.
	for i := range 10 {
		now := time.Now().Add(-time.Second)
		d := &domain.WebhookDelivery{
			ID:          fmt.Sprintf("conn-reuse-%d", i),
			WebhookURL:  ts.URL,
			Status:      domain.WebhookStatusPending,
			MaxAttempts: 5,
			NextRetryAt: &now,
			LastError:   fmt.Sprintf(`{"i":%d}`, i),
		}
		worker.attemptDelivery(context.Background(), d)
	}

	// With keep-alive, should have fewer connections than requests.
	conns := connCount.Load()
	if conns > 3 {
		t.Errorf("expected connection reuse, but opened %d connections for 10 requests", conns)
	}
}

func TestWithHTTPTransport_DefaultValues(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	transport, ok := worker.client.Transport.(*http.Transport)
	if !ok || transport == nil {
		t.Fatalf("expected default client to use SSRF-safe *http.Transport, got %T", worker.client.Transport)
	}
	if worker.client.CheckRedirect == nil {
		t.Fatal("expected default client to reject redirects")
	}
}

func TestWithHTTPTransport_BlocksPrivateDNSAtDeliveryTime(t *testing.T) {
	// Not parallel: replaces package-level DNS resolver used by httputil.
	restore := httputil.SetLookupHostForTest(func(host string) ([]string, error) {
		if host == "internal.example.com" {
			return []string{"10.0.0.5"}, nil
		}
		return []string{"93.184.216.34"}, nil
	})
	t.Cleanup(restore)

	ms := &mockDeliveryStore{}
	now := time.Now()
	delivery := &domain.WebhookDelivery{
		ID:          "whd-private-dns",
		WebhookURL:  "https://internal.example.com/hook",
		Status:      domain.WebhookStatusPending,
		MaxAttempts: 3,
		NextRetryAt: &now,
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}
	worker := NewDeliveryWorker(ms, slog.Default(), WithHTTPTransport(500*time.Millisecond, time.Second, 2, 2))

	worker.processBatch(context.Background())

	deliveries := ms.getDeliveries()
	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(deliveries))
	}
	got := deliveries[0]
	if got.Status != domain.WebhookStatusPending {
		t.Fatalf("status = %s, want pending retry", got.Status)
	}
	if !strings.Contains(got.LastError, "resolves to private") {
		t.Fatalf("last_error = %q, want private DNS rejection", got.LastError)
	}
}

func TestAttemptDelivery_RedactsSecretURLFromTransportError(t *testing.T) {
	t.Parallel()

	rawURL := "https://user:password@example.com/hook/token-123?api_key=secret-value"
	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		ID:          "whd-redact-url",
		WebhookURL:  rawURL,
		Status:      domain.WebhookStatusPending,
		MaxAttempts: 3,
		NextRetryAt: &now,
		LastError:   `{"event":"run.failed"}`,
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.client = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, &url.Error{
			Op:  "Post",
			URL: rawURL,
			Err: errors.New("dial tcp: connection refused"),
		}
	})}
	worker.processBatch(context.Background())

	got := ms.getDeliveries()[0]
	for _, leaked := range []string{"password", "token-123", "secret-value", "api_key"} {
		if strings.Contains(got.LastError, leaked) {
			t.Fatalf("last_error leaked URL secret %q: %s", leaked, got.LastError)
		}
	}
	if !strings.Contains(got.LastError, "request failed") {
		t.Fatalf("last_error omitted sanitized transport reason: %s", got.LastError)
	}
}

func TestAttemptDelivery_RedactsMalformedWebhookURLFromCreateRequestError(t *testing.T) {
	t.Parallel()

	rawURL := "http://user:password@%zz.example/hook?token=secret-value"
	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		ID:          "whd-redact-malformed-url",
		WebhookURL:  rawURL,
		Status:      domain.WebhookStatusPending,
		MaxAttempts: 3,
		NextRetryAt: &now,
		Payload:     json.RawMessage(`{"event":"run.failed"}`),
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())

	got := ms.getDeliveries()[0]
	if got.Status != domain.WebhookStatusDead {
		t.Fatalf("status = %s, want dead", got.Status)
	}
	for _, leaked := range []string{"user", "password", "secret-value", "token", rawURL} {
		if strings.Contains(got.LastError, leaked) {
			t.Fatalf("last_error leaked malformed URL secret %q: %s", leaked, got.LastError)
		}
	}
	if got.LastError != "create request: invalid webhook URL" {
		t.Fatalf("last_error = %q, want sanitized create request error", got.LastError)
	}
}

func TestAttemptBatchDelivery_RedactsMalformedWebhookURLFromCreateRequestError(t *testing.T) {
	t.Parallel()

	rawURL := "http://user:password@%zz.example/hook?token=secret-value"
	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	for i := 1; i <= 2; i++ {
		delivery := &domain.WebhookDelivery{
			ID:          fmt.Sprintf("whd-redact-malformed-batch-%d", i),
			WebhookURL:  rawURL,
			Status:      domain.WebhookStatusPending,
			MaxAttempts: 3,
			NextRetryAt: &now,
			Payload:     json.RawMessage(`{"event":"run.failed"}`),
		}
		if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
			t.Fatalf("create delivery %d: %v", i, err)
		}
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(true))
	worker.processBatch(context.Background())

	for _, got := range ms.getDeliveries() {
		if got.Status != domain.WebhookStatusDead {
			t.Fatalf("status for %s = %s, want dead", got.ID, got.Status)
		}
		for _, leaked := range []string{"user", "password", "secret-value", "token", rawURL} {
			if strings.Contains(got.LastError, leaked) {
				t.Fatalf("last_error for %s leaked malformed URL secret %q: %s", got.ID, leaked, got.LastError)
			}
		}
		if got.LastError != "create request: invalid webhook URL" {
			t.Fatalf("last_error for %s = %q, want sanitized create request error", got.ID, got.LastError)
		}
	}
}

// Batch helper tests.

func TestGroupByURL_Empty(t *testing.T) {
	t.Parallel()
	result := groupByURL(nil)
	if len(result) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(result))
	}
}

func TestGroupByURL_SingleURL(t *testing.T) {
	t.Parallel()
	deliveries := []domain.WebhookDelivery{
		{ID: "d1", WebhookURL: "https://a.com/hook"},
		{ID: "d2", WebhookURL: "https://a.com/hook"},
		{ID: "d3", WebhookURL: "https://a.com/hook"},
	}
	result := groupByURL(deliveries)
	if len(result) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result))
	}
	if len(result["https://a.com/hook"]) != 3 {
		t.Fatalf("expected 3 deliveries in group, got %d", len(result["https://a.com/hook"]))
	}
}

func TestGroupByURL_MultipleURLs(t *testing.T) {
	t.Parallel()
	deliveries := []domain.WebhookDelivery{
		{ID: "d1", WebhookURL: "https://a.com"},
		{ID: "d2", WebhookURL: "https://b.com"},
		{ID: "d3", WebhookURL: "https://a.com"},
		{ID: "d4", WebhookURL: "https://c.com"},
		{ID: "d5", WebhookURL: "https://b.com"},
	}
	result := groupByURL(deliveries)
	if len(result) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(result))
	}
	if len(result["https://a.com"]) != 2 {
		t.Fatalf("expected 2 for a.com, got %d", len(result["https://a.com"]))
	}
	if len(result["https://b.com"]) != 2 {
		t.Fatalf("expected 2 for b.com, got %d", len(result["https://b.com"]))
	}
	if len(result["https://c.com"]) != 1 {
		t.Fatalf("expected 1 for c.com, got %d", len(result["https://c.com"]))
	}
}

func TestChunkDeliveries_ExactMultiple(t *testing.T) {
	t.Parallel()
	deliveries := make([]domain.WebhookDelivery, 9)
	for i := range deliveries {
		deliveries[i].ID = fmt.Sprintf("d%d", i)
	}
	chunks := chunkDeliveries(deliveries, 3)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	for _, c := range chunks {
		if len(c) != 3 {
			t.Fatalf("expected chunk size 3, got %d", len(c))
		}
	}
}

func TestChunkDeliveries_Remainder(t *testing.T) {
	t.Parallel()
	deliveries := make([]domain.WebhookDelivery, 10)
	for i := range deliveries {
		deliveries[i].ID = fmt.Sprintf("d%d", i)
	}
	chunks := chunkDeliveries(deliveries, 3)
	if len(chunks) != 4 {
		t.Fatalf("expected 4 chunks, got %d", len(chunks))
	}
	if len(chunks[3]) != 1 {
		t.Fatalf("expected last chunk size 1, got %d", len(chunks[3]))
	}
}

func TestChunkDeliveries_LargerThanInput(t *testing.T) {
	t.Parallel()
	deliveries := make([]domain.WebhookDelivery, 3)
	for i := range deliveries {
		deliveries[i].ID = fmt.Sprintf("d%d", i)
	}
	chunks := chunkDeliveries(deliveries, 10)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if len(chunks[0]) != 3 {
		t.Fatalf("expected chunk size 3, got %d", len(chunks[0]))
	}
}

func TestChunkDeliveries_Empty(t *testing.T) {
	t.Parallel()
	chunks := chunkDeliveries(nil, 5)
	if chunks != nil {
		t.Fatalf("expected nil, got %v", chunks)
	}
}

// Batch delivery tests.

func TestProcessBatch_BatchByURL_GroupsSameURL(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts2.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)

	// 5 deliveries to URL-A
	for i := range 5 {
		d := &domain.WebhookDelivery{
			ID: fmt.Sprintf("batch-a-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), d)
	}
	// 3 deliveries to URL-B
	for i := range 3 {
		d := &domain.WebhookDelivery{
			ID: fmt.Sprintf("batch-b-%d", i), WebhookURL: ts2.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), d)
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(true), WithConcurrency(10))
	worker.processBatch(context.Background())

	// Should be 2 HTTP requests: 1 batch to URL-A, 1 batch to URL-B
	if got := requestCount.Load(); got != 2 {
		t.Fatalf("expected 2 HTTP requests (batched), got %d", got)
	}

	// All deliveries should be marked delivered.
	for _, d := range ms.getDeliveries() {
		if d.Status != domain.WebhookStatusDelivered {
			t.Fatalf("expected delivered, got %s for %s", d.Status, d.ID)
		}
	}
}

func TestProcessBatch_BatchByURL_SingleDeliveryNotBatched(t *testing.T) {
	t.Parallel()

	var receivedBatchHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBatchHeader = r.Header.Get("X-Strait-Batch")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	d := &domain.WebhookDelivery{
		ID: "single-batch", WebhookURL: ts.URL,
		Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
		LastError: `{"solo":true}`,
	}
	_ = ms.CreateWebhookDelivery(context.Background(), d)

	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(true))
	worker.processBatch(context.Background())

	// Single delivery should use individual path (no batch header).
	if receivedBatchHeader == "true" {
		t.Fatal("expected single delivery to not use batch path")
	}

	if ms.getDeliveries()[0].Status != domain.WebhookStatusDelivered {
		t.Fatalf("expected delivered, got %s", ms.getDeliveries()[0].Status)
	}
}

func TestProcessBatch_BatchByURL_SubscriptionDeliveriesStaySignedAndUnbatched(t *testing.T) {
	var requestCount atomic.Int32
	var sawBatchHeader atomic.Bool
	var sawUnsigned atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.Header.Get("X-Strait-Batch") != "" {
			sawBatchHeader.Store(true)
		}
		if r.Header.Get("X-Webhook-Signature") == "" {
			sawUnsigned.Store(true)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{subSecret: "whsec_test"}
	now := time.Now().Add(-time.Second)
	for i := range 2 {
		d := &domain.WebhookDelivery{
			ID:             fmt.Sprintf("sub-batch-%d", i),
			SubscriptionID: "sub-1",
			ProjectID:      "proj-1",
			WebhookURL:     ts.URL,
			Status:         domain.WebhookStatusPending,
			MaxAttempts:    5,
			NextRetryAt:    &now,
			Payload:        json.RawMessage(fmt.Sprintf(`{"i":%d}`, i)),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), d)
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(true), WithConcurrency(10))
	worker.processBatch(context.Background())

	if got := requestCount.Load(); got != 2 {
		t.Fatalf("expected subscription deliveries to use individual signed requests, got %d requests", got)
	}
	if sawBatchHeader.Load() {
		t.Fatal("subscription delivery used batch headers")
	}
	if sawUnsigned.Load() {
		t.Fatal("subscription delivery was sent without HMAC signature")
	}
	for _, d := range ms.getDeliveries() {
		if d.Status != domain.WebhookStatusDelivered {
			t.Fatalf("expected delivered, got %s for %s", d.Status, d.ID)
		}
	}
}

func TestProcessBatch_BatchByURL_RunWebhookSecretsStaySignedAndUnbatched(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	var sawBatchHeader atomic.Bool
	var sawUnsigned atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.Header.Get("X-Strait-Batch") != "" {
			sawBatchHeader.Store(true)
		}
		if r.Header.Get("X-Webhook-Signature") == "" {
			sawUnsigned.Store(true)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	for i := range 2 {
		d := &domain.WebhookDelivery{
			ID:            fmt.Sprintf("run-signed-batch-%d", i),
			RunID:         fmt.Sprintf("run-%d", i),
			JobID:         "job-1",
			ProjectID:     "proj-1",
			WebhookURL:    ts.URL,
			WebhookSecret: "job-webhook-secret",
			Status:        domain.WebhookStatusPending,
			MaxAttempts:   5,
			NextRetryAt:   &now,
			Payload:       json.RawMessage(fmt.Sprintf(`{"i":%d}`, i)),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), d)
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(true), WithConcurrency(10))
	worker.processBatch(context.Background())

	if got := requestCount.Load(); got != 2 {
		t.Fatalf("expected signed run deliveries to use individual requests, got %d requests", got)
	}
	if sawBatchHeader.Load() {
		t.Fatal("signed run delivery used batch headers")
	}
	if sawUnsigned.Load() {
		t.Fatal("signed run delivery was sent without HMAC signature")
	}
}

func TestProcessBatch_BatchByURL_MaxBatchSize(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	for i := range 10 {
		d := &domain.WebhookDelivery{
			ID: fmt.Sprintf("chunk-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), d)
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(true), WithMaxBatchSize(3), WithConcurrency(10))
	worker.processBatch(context.Background())

	// 10 deliveries / batch size 3 = 4 batches (3+3+3+1)
	if got := requestCount.Load(); got != 4 {
		t.Fatalf("expected 4 HTTP requests (chunked batches), got %d", got)
	}
}

func TestProcessBatch_BatchByURL_Disabled(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	for i := range 5 {
		d := &domain.WebhookDelivery{
			ID: fmt.Sprintf("no-batch-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), d)
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(false), WithConcurrency(10))
	worker.processBatch(context.Background())

	// Without batching, each delivery is a separate request.
	if got := requestCount.Load(); got != 5 {
		t.Fatalf("expected 5 individual HTTP requests, got %d", got)
	}
}

func TestAttemptBatchDelivery_Success_AllMarkedDelivered(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := make([]domain.WebhookDelivery, 3)
	for i := range deliveries {
		deliveries[i] = domain.WebhookDelivery{
			ID: fmt.Sprintf("batch-ok-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	for _, d := range ms.getDeliveries() {
		if d.Status != domain.WebhookStatusDelivered {
			t.Fatalf("expected delivered for %s, got %s", d.ID, d.Status)
		}
		if d.Attempts != 1 {
			t.Fatalf("expected 1 attempt for %s, got %d", d.ID, d.Attempts)
		}
	}
}

func TestAttemptBatchDelivery_ServerError_AllRetried(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := make([]domain.WebhookDelivery, 3)
	for i := range deliveries {
		deliveries[i] = domain.WebhookDelivery{
			ID: fmt.Sprintf("batch-5xx-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	for _, d := range ms.getDeliveries() {
		if d.Status != domain.WebhookStatusPending {
			t.Fatalf("expected pending (retry) for %s, got %s", d.ID, d.Status)
		}
		if d.Attempts != 1 {
			t.Fatalf("expected 1 attempt for %s, got %d", d.ID, d.Attempts)
		}
		if d.NextRetryAt == nil || d.NextRetryAt.Before(time.Now()) {
			t.Fatalf("expected next_retry_at in the future for %s", d.ID)
		}
	}
}

func TestAttemptBatchDelivery_ClientError_AllDeadLettered(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := make([]domain.WebhookDelivery, 3)
	for i := range deliveries {
		deliveries[i] = domain.WebhookDelivery{
			ID: fmt.Sprintf("batch-4xx-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	for _, d := range ms.getDeliveries() {
		if d.Status != domain.WebhookStatusDead {
			t.Fatalf("expected dead for %s, got %s", d.ID, d.Status)
		}
	}
}

func TestAttemptBatchDelivery_CircuitBreakerOpen_SkipsBatch(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	state := newRedisCircuitState()
	client := newMockRedisClient(state.process)
	t.Cleanup(func() { _ = client.Close() })

	breaker := NewRedisWebhookCircuitBreaker(client, true,
		WithWebhookCircuitBreakerThreshold(1),
		WithWebhookCircuitBreakerWindow(time.Minute),
	)

	// Trip the circuit.
	breaker.RecordFailure(context.Background(), ts.URL)

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := make([]domain.WebhookDelivery, 3)
	for i := range deliveries {
		deliveries[i] = domain.WebhookDelivery{
			ID: fmt.Sprintf("batch-cb-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithCircuitBreaker(breaker))
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	if requestCount.Load() != 0 {
		t.Fatal("expected no HTTP requests when circuit breaker is open")
	}

	// Circuit breaker open is retryable, but it is not a delivery attempt:
	// no outbound HTTP request was made, so attempts must not be consumed.
	for _, d := range ms.getDeliveries() {
		if d.Status != domain.WebhookStatusPending {
			t.Fatalf("expected pending for %s (circuit breaker, retryable), got %s", d.ID, d.Status)
		}
		if d.Attempts != 0 {
			t.Fatalf("expected 0 attempts for %s, got %d", d.ID, d.Attempts)
		}
	}
}

func TestAttemptBatchDelivery_PayloadTooLarge_FallsBackToIndividual(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)

	// Create deliveries with payloads that are small individually but large when batched.
	largePayload := `{"data":"` + strings.Repeat("x", 200) + `"}`
	deliveries := make([]domain.WebhookDelivery, 5)
	for i := range deliveries {
		deliveries[i] = domain.WebhookDelivery{
			ID: fmt.Sprintf("batch-big-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: largePayload,
		}
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	// Set maxPayloadBytes low enough that batch exceeds it but individual doesn't.
	worker := NewDeliveryWorker(ms, slog.Default(), WithMaxPayloadBytes(500))
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	// Should fall back to 5 individual requests.
	if got := requestCount.Load(); got != 5 {
		t.Fatalf("expected 5 individual requests (fallback), got %d", got)
	}

	for _, d := range ms.getDeliveries() {
		if d.Status != domain.WebhookStatusDelivered {
			t.Fatalf("expected delivered for %s, got %s", d.ID, d.Status)
		}
	}
}

func TestAttemptBatchDelivery_Headers(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var headers http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		headers = r.Header.Clone()
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := make([]domain.WebhookDelivery, 3)
	for i := range deliveries {
		deliveries[i] = domain.WebhookDelivery{
			ID: fmt.Sprintf("batch-hdr-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	mu.Lock()
	defer mu.Unlock()
	if headers.Get("X-Strait-Batch") != "true" {
		t.Fatalf("expected X-Strait-Batch=true, got %q", headers.Get("X-Strait-Batch"))
	}
	if headers.Get("X-Strait-Batch-Size") != "3" {
		t.Fatalf("expected X-Strait-Batch-Size=3, got %q", headers.Get("X-Strait-Batch-Size"))
	}
	if headers.Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type=application/json, got %q", headers.Get("Content-Type"))
	}
}

func TestAttemptBatchDelivery_PayloadFormat(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var receivedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedBody, _ = io.ReadAll(r.Body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := make([]domain.WebhookDelivery, 2)
	deliveries[0] = domain.WebhookDelivery{
		ID: "fmt-1", WebhookURL: ts.URL,
		Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
		LastError: `{"key":"val1"}`,
	}
	deliveries[1] = domain.WebhookDelivery{
		ID: "fmt-2", WebhookURL: ts.URL,
		Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
		LastError: `{"key":"val2"}`,
	}
	for i := range deliveries {
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	mu.Lock()
	defer mu.Unlock()

	var items []batchPayloadItem
	if err := json.Unmarshal(receivedBody, &items); err != nil {
		t.Fatalf("failed to unmarshal batch payload: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items in batch, got %d", len(items))
	}
	if items[0].DeliveryID != "fmt-1" || items[1].DeliveryID != "fmt-2" {
		t.Fatalf("unexpected delivery IDs: %s, %s", items[0].DeliveryID, items[1].DeliveryID)
	}
	var p1 map[string]string
	if err := json.Unmarshal(items[0].Payload, &p1); err != nil {
		t.Fatalf("failed to unmarshal item[0] payload: %v", err)
	}
	if p1["key"] != "val1" {
		t.Fatalf("expected payload key=val1, got %s", p1["key"])
	}
}

func TestAttemptBatchDelivery_ClickHouseEvents(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:       true,
		BatchSize:     100,
		FlushInterval: time.Minute,
	}, slog.Default())

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := make([]domain.WebhookDelivery, 3)
	for i := range deliveries {
		deliveries[i] = domain.WebhookDelivery{
			ID: fmt.Sprintf("batch-ch-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithChExporter(exporter))
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	// One ClickHouse event per delivery in the batch.
	if exporter.PendingCount() != 3 {
		t.Fatalf("expected 3 pending ClickHouse records, got %d", exporter.PendingCount())
	}
}

func TestEnqueueDeliveryEvent_IncludesProjectID(t *testing.T) {
	t.Parallel()

	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:       true,
		BatchSize:     100,
		FlushInterval: time.Minute,
	}, slog.Default())

	worker := NewDeliveryWorker(&mockDeliveryStore{}, slog.Default(), WithChExporter(exporter))
	worker.enqueueDeliveryEvent(&domain.WebhookDelivery{
		ID:         "delivery-project",
		RunID:      "run-project",
		JobID:      "job-project",
		ProjectID:  "proj-project",
		WebhookURL: "https://example.com/hook",
		Status:     domain.WebhookStatusDelivered,
		Attempts:   1,
		CreatedAt:  time.Now(),
	}, 123, "run_webhook")

	records := exporter.PendingSnapshot()
	if len(records) != 1 {
		t.Fatalf("pending records = %d, want 1", len(records))
	}
	rec, ok := records[0].(clickhouse.WebhookDeliveryEventRecord)
	if !ok {
		t.Fatalf("pending record type = %T, want WebhookDeliveryEventRecord", records[0])
	}
	if rec.ProjectID != "proj-project" {
		t.Fatalf("ProjectID = %q, want proj-project", rec.ProjectID)
	}
}

func TestProcessBatch_BatchAndIndividualMixed(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	var mu sync.Mutex
	batchHeaders := make(map[string]bool)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		mu.Lock()
		batchHeaders[r.Header.Get("X-Strait-Batch")] = true
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		mu.Lock()
		batchHeaders[r.Header.Get("X-Strait-Batch")] = true
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts2.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)

	// 3 deliveries to URL-A (will be batched)
	for i := range 3 {
		d := &domain.WebhookDelivery{
			ID: fmt.Sprintf("mix-a-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), d)
	}

	// 1 delivery to URL-B (will be individual)
	d := &domain.WebhookDelivery{
		ID: "mix-b-0", WebhookURL: ts2.URL,
		Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
		LastError: `{"solo":true}`,
	}
	_ = ms.CreateWebhookDelivery(context.Background(), d)

	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(true), WithConcurrency(10))
	worker.processBatch(context.Background())

	// 1 batch request + 1 individual request = 2 total
	if got := requestCount.Load(); got != 2 {
		t.Fatalf("expected 2 HTTP requests (1 batch + 1 individual), got %d", got)
	}

	// All should be delivered.
	for _, d := range ms.getDeliveries() {
		if d.Status != domain.WebhookStatusDelivered {
			t.Fatalf("expected delivered for %s, got %s", d.ID, d.Status)
		}
	}
}

func TestWithBatchByURL_Option(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(true))
	if !worker.batchByURL {
		t.Error("expected batchByURL=true")
	}

	worker2 := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(false))
	if worker2.batchByURL {
		t.Error("expected batchByURL=false")
	}
}

func TestWithMaxBatchSize_Option(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithMaxBatchSize(25))
	if worker.maxBatchSize != 25 {
		t.Errorf("maxBatchSize = %d, want 25", worker.maxBatchSize)
	}
}

func TestWithMaxBatchSize_ZeroKeepsDefault(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithMaxBatchSize(0))
	if worker.maxBatchSize != defaultMaxBatchSize {
		t.Errorf("maxBatchSize = %d, want %d (default)", worker.maxBatchSize, defaultMaxBatchSize)
	}
}

func TestExtractPayload_ValidJSON(t *testing.T) {
	t.Parallel()

	d := &domain.WebhookDelivery{LastError: `{"key":"value"}`}
	payload := extractPayload(d)

	var m map[string]string
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["key"] != "value" {
		t.Fatalf("expected key=value, got %s", m["key"])
	}
	if d.LastError != "" {
		t.Fatal("expected LastError to be cleared after extraction")
	}
}

func TestExtractPayload_InvalidJSON_Fallback(t *testing.T) {
	t.Parallel()

	d := &domain.WebhookDelivery{ID: "d1", EventTriggerID: "evt1", LastError: "not-json"}
	payload := extractPayload(d)

	var m map[string]string
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["delivery_id"] != "d1" {
		t.Fatalf("expected delivery_id=d1, got %s", m["delivery_id"])
	}
}

func TestExtractPayload_EmptyLastError_Fallback(t *testing.T) {
	t.Parallel()

	d := &domain.WebhookDelivery{ID: "d2"}
	payload := extractPayload(d)

	var m map[string]string
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["delivery_id"] != "d2" {
		t.Fatalf("expected delivery_id=d2, got %s", m["delivery_id"])
	}
}

// Adversarial tests: edge cases and failure modes.

func TestAttemptBatchDelivery_PayloadTooLarge_PreservesOriginalPayload(t *testing.T) {
	// BUG regression: extractPayload clears LastError. If batch exceeds
	// maxPayloadBytes and falls back to individual delivery, the real payload
	// must be restored so attemptDelivery sends the correct data.
	t.Parallel()

	var mu sync.Mutex
	receivedPayloads := make(map[string]string)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		deliveryID := r.Header.Get("X-Strait-Delivery-ID")
		mu.Lock()
		receivedPayloads[deliveryID] = string(body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)

	// Create deliveries with unique payloads.
	for i := range 3 {
		d := &domain.WebhookDelivery{
			ID: fmt.Sprintf("preserve-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"unique_key":"value_%d"}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), d)
	}

	// Set maxPayloadBytes very low so batch always exceeds it.
	worker := NewDeliveryWorker(ms, slog.Default(), WithMaxPayloadBytes(50))
	deliveries := make([]domain.WebhookDelivery, 3)
	for i, d := range ms.getDeliveries() {
		deliveries[i] = *d
	}
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	mu.Lock()
	defer mu.Unlock()

	// Each individual delivery must have received its ORIGINAL payload, not the fallback.
	for i := range 3 {
		id := fmt.Sprintf("preserve-%d", i)
		body, ok := receivedPayloads[id]
		if !ok {
			t.Fatalf("missing request for delivery %s", id)
		}
		var payload map[string]string
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("delivery %s: failed to unmarshal body: %v", id, err)
		}
		expected := fmt.Sprintf("value_%d", i)
		if payload["unique_key"] != expected {
			t.Fatalf("delivery %s: expected unique_key=%s, got %s (payload was lost on fallback)",
				id, expected, payload["unique_key"])
		}
	}
}

func TestAttemptBatchDelivery_PayloadTooLarge_FallbackIsConcurrent(t *testing.T) {
	// BUG regression: fallback from batch to individual used to be sequential.
	t.Parallel()

	var maxInFlight atomic.Int32
	var inFlight atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		cur := inFlight.Add(1)
		for {
			old := maxInFlight.Load()
			if cur <= old || maxInFlight.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		inFlight.Add(-1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := make([]domain.WebhookDelivery, 10)
	for i := range deliveries {
		deliveries[i] = domain.WebhookDelivery{
			ID: fmt.Sprintf("conc-fallback-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithMaxPayloadBytes(50), WithConcurrency(10))
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	if maxInFlight.Load() <= 1 {
		t.Fatalf("fallback should be concurrent, but max in-flight was %d", maxInFlight.Load())
	}
}

func TestAttemptBatchDelivery_EmptyDeliveries(t *testing.T) {
	// Edge case: empty slice should not panic or send HTTP requests.
	t.Parallel()

	var requestCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	// Should not panic.
	worker.attemptBatchDelivery(context.Background(), ts.URL, nil)
	worker.attemptBatchDelivery(context.Background(), ts.URL, []domain.WebhookDelivery{})

	// Empty batch still sends an HTTP request with empty array.
	// This is acceptable behavior but let's verify no panic.
}

func TestAttemptBatchDelivery_ContextCanceled(t *testing.T) {
	// Edge case: context canceled before HTTP request.
	t.Parallel()

	var requestCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := make([]domain.WebhookDelivery, 2)
	for i := range deliveries {
		deliveries[i] = domain.WebhookDelivery{
			ID: fmt.Sprintf("ctx-cancel-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.attemptBatchDelivery(ctx, ts.URL, deliveries)

	// Deliveries should be scheduled for retry (HTTP error due to canceled context).
	for _, d := range ms.getDeliveries() {
		if d.Status != domain.WebhookStatusPending {
			t.Fatalf("expected pending (retry) for %s after ctx cancel, got %s", d.ID, d.Status)
		}
	}
}

func TestAttemptBatchDelivery_MaxAttemptsExhausted_DeadLetters(t *testing.T) {
	// Edge case: delivery at max attempts should be dead-lettered on batch failure.
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := make([]domain.WebhookDelivery, 2)
	// One delivery at max attempts, one with room.
	deliveries[0] = domain.WebhookDelivery{
		ID: "exhausted", WebhookURL: ts.URL,
		Status: domain.WebhookStatusPending, Attempts: 4, MaxAttempts: 5, NextRetryAt: &now,
		LastError: `{"i":0}`,
	}
	deliveries[1] = domain.WebhookDelivery{
		ID: "has-room", WebhookURL: ts.URL,
		Status: domain.WebhookStatusPending, Attempts: 1, MaxAttempts: 5, NextRetryAt: &now,
		LastError: `{"i":1}`,
	}
	for i := range deliveries {
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	for _, d := range ms.getDeliveries() {
		switch d.ID {
		case "exhausted":
			// Attempts goes 4 -> 5, which equals MaxAttempts=5 -> dead.
			if d.Status != domain.WebhookStatusDead {
				t.Fatalf("expected dead for exhausted delivery, got %s", d.Status)
			}
		case "has-room":
			// Attempts goes 1 -> 2, still below MaxAttempts=5 -> pending retry.
			if d.Status != domain.WebhookStatusPending {
				t.Fatalf("expected pending for delivery with room, got %s", d.Status)
			}
		}
	}
}

func TestAttemptBatchDelivery_MixedAttemptCounts(t *testing.T) {
	// Edge case: batch with deliveries at different attempt counts.
	// All get the same HTTP response but should have different retry outcomes.
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := []domain.WebhookDelivery{
		{ID: "attempt-0", WebhookURL: ts.URL, Status: domain.WebhookStatusPending,
			Attempts: 0, MaxAttempts: 2, NextRetryAt: &now, LastError: `{"i":0}`},
		{ID: "attempt-1", WebhookURL: ts.URL, Status: domain.WebhookStatusPending,
			Attempts: 1, MaxAttempts: 2, NextRetryAt: &now, LastError: `{"i":1}`},
	}
	for i := range deliveries {
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	for _, d := range ms.getDeliveries() {
		switch d.ID {
		case "attempt-0":
			// Attempts: 0 -> 1, MaxAttempts: 2 -> still has room -> pending.
			if d.Status != domain.WebhookStatusPending {
				t.Fatalf("expected pending for attempt-0 (1/2), got %s", d.Status)
			}
		case "attempt-1":
			// Attempts: 1 -> 2, MaxAttempts: 2 -> exhausted -> dead.
			if d.Status != domain.WebhookStatusDead {
				t.Fatalf("expected dead for attempt-1 (2/2), got %s", d.Status)
			}
		}
	}
}

func TestAttemptBatchDelivery_InvalidURL_DeadLettersAll(t *testing.T) {
	// Edge case: completely invalid URL that fails request creation.
	t.Parallel()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := make([]domain.WebhookDelivery, 2)
	for i := range deliveries {
		deliveries[i] = domain.WebhookDelivery{
			ID: fmt.Sprintf("bad-url-%d", i), WebhookURL: "://invalid",
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.attemptBatchDelivery(context.Background(), "://invalid", deliveries)

	for _, d := range ms.getDeliveries() {
		if d.Status != domain.WebhookStatusDead {
			t.Fatalf("expected dead for %s (invalid URL, non-retryable), got %s", d.ID, d.Status)
		}
	}
}

func TestAttemptBatchDelivery_ClickHouseEventsOnAllPaths(t *testing.T) {
	// BUG regression: ClickHouse events were missing on some error paths.
	t.Parallel()

	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:       true,
		BatchSize:     100,
		FlushInterval: time.Minute,
	}, slog.Default())

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)

	// Test with invalid URL to exercise the request creation failure path.
	deliveries := make([]domain.WebhookDelivery, 2)
	for i := range deliveries {
		deliveries[i] = domain.WebhookDelivery{
			ID: fmt.Sprintf("ch-path-%d", i), WebhookURL: "://invalid",
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithChExporter(exporter))
	worker.attemptBatchDelivery(context.Background(), "://invalid", deliveries)

	// ClickHouse events should fire even on request creation failure.
	if exporter.PendingCount() != 2 {
		t.Fatalf("expected 2 ClickHouse events on error path, got %d", exporter.PendingCount())
	}
}

func TestProcessBatch_BatchByURL_ZeroDeliveries(t *testing.T) {
	// Edge case: no pending deliveries.
	t.Parallel()

	ms := &mockDeliveryStore{
		listPendingFn: func(_ context.Context) ([]domain.WebhookDelivery, error) {
			return nil, nil
		},
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(true))
	// Should not panic.
	worker.processBatch(context.Background())
}

func TestProcessBatch_BatchByURL_StoreError(t *testing.T) {
	// Edge case: store returns error on list.
	t.Parallel()

	ms := &mockDeliveryStore{
		listPendingFn: func(_ context.Context) ([]domain.WebhookDelivery, error) {
			return nil, fmt.Errorf("db connection lost")
		},
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(true))
	// Should not panic, just log error.
	worker.processBatch(context.Background())
}

func TestProcessBatch_BatchByURL_MaxBatchSizeOne(t *testing.T) {
	// Edge case: maxBatchSize=1 means every multi-delivery group is chunked
	// into single-item batches (which are sent via attemptBatchDelivery, not
	// attemptDelivery, because the group had > 1 item).
	t.Parallel()

	var mu sync.Mutex
	batchHeaders := make([]string, 0)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		batchHeaders = append(batchHeaders, r.Header.Get("X-Strait-Batch"))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	for i := range 3 {
		d := &domain.WebhookDelivery{
			ID: fmt.Sprintf("batch1-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), d)
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(true), WithMaxBatchSize(1), WithConcurrency(10))
	worker.processBatch(context.Background())

	mu.Lock()
	defer mu.Unlock()

	// 3 deliveries, maxBatchSize=1 -> 3 batch requests (each with X-Strait-Batch: true).
	if len(batchHeaders) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(batchHeaders))
	}
	for i, h := range batchHeaders {
		if h != "true" {
			t.Fatalf("request %d: expected X-Strait-Batch=true, got %q", i, h)
		}
	}
}

func TestAttemptBatchDelivery_ServerSlowResponse(t *testing.T) {
	// Edge case: server responds slowly but within timeout.
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := make([]domain.WebhookDelivery, 3)
	for i := range deliveries {
		deliveries[i] = domain.WebhookDelivery{
			ID: fmt.Sprintf("slow-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	for _, d := range ms.getDeliveries() {
		if d.Status != domain.WebhookStatusDelivered {
			t.Fatalf("expected delivered for %s, got %s", d.ID, d.Status)
		}
	}
}

func TestAttemptBatchDelivery_3xxResponse(t *testing.T) {
	// 3xx responses are treated as a permanent failure: redirects are
	// not followed (SSRF defense), so the receiver returning a redirect
	// is a configuration error, not a successful delivery.
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusFound) // 302
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := make([]domain.WebhookDelivery, 2)
	for i := range deliveries {
		deliveries[i] = domain.WebhookDelivery{
			ID: fmt.Sprintf("3xx-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	for _, d := range ms.getDeliveries() {
		if d.Status != domain.WebhookStatusDead {
			t.Fatalf("expected dead for %s on unfollowed 3xx, got %s", d.ID, d.Status)
		}
		if d.LastStatusCode == nil || *d.LastStatusCode != http.StatusFound {
			t.Fatalf("expected last_status_code=302 for %s, got %v", d.ID, d.LastStatusCode)
		}
	}
}

func TestProcessBatch_BatchByURL_AllDifferentURLs(t *testing.T) {
	// Edge case: every delivery has a different URL.
	// All go through individual path (group size = 1).
	t.Parallel()

	var requestCount atomic.Int32
	servers := make([]*httptest.Server, 5)
	for i := range servers {
		servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requestCount.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer servers[i].Close()
	}

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	for i, srv := range servers {
		d := &domain.WebhookDelivery{
			ID: fmt.Sprintf("unique-url-%d", i), WebhookURL: srv.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), d)
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(true), WithConcurrency(10))
	worker.processBatch(context.Background())

	if got := requestCount.Load(); got != 5 {
		t.Fatalf("expected 5 individual requests (all different URLs), got %d", got)
	}

	for _, d := range ms.getDeliveries() {
		if d.Status != domain.WebhookStatusDelivered {
			t.Fatalf("expected delivered for %s, got %s", d.ID, d.Status)
		}
	}
}

func TestAttemptBatchDelivery_NonJSONPayloadInLastError(t *testing.T) {
	// Edge case: delivery on retry has an error message in LastError
	// (not JSON payload). extractPayload should use fallback.
	t.Parallel()

	var mu sync.Mutex
	var receivedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedBody, _ = io.ReadAll(r.Body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := []domain.WebhookDelivery{{
		ID: "retry-err", EventTriggerID: "evt-99", WebhookURL: ts.URL,
		Status: domain.WebhookStatusPending, Attempts: 1, MaxAttempts: 5, NextRetryAt: &now,
		LastError: "server error: status 500", // Not JSON - this is an error message from previous attempt.
	}}
	_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[0])

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	mu.Lock()
	defer mu.Unlock()

	// Should use fallback payload with delivery_id and trigger_id.
	var items []batchPayloadItem
	if err := json.Unmarshal(receivedBody, &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	var payload map[string]string
	if err := json.Unmarshal(items[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["trigger_id"] != "evt-99" {
		t.Fatalf("expected trigger_id=evt-99, got %s", payload["trigger_id"])
	}
}

func TestProcessBatch_BatchByURL_LargeNumberOfURLs(t *testing.T) {
	// Stress test: many different URLs with batching enabled.
	t.Parallel()

	var requestCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)

	// 50 URLs, 2 deliveries each = 100 total.
	for i := range 50 {
		url := fmt.Sprintf("%s/endpoint-%d", ts.URL, i)
		for j := range 2 {
			d := &domain.WebhookDelivery{
				ID: fmt.Sprintf("stress-%d-%d", i, j), WebhookURL: url,
				Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
				LastError: fmt.Sprintf(`{"i":%d,"j":%d}`, i, j),
			}
			_ = ms.CreateWebhookDelivery(context.Background(), d)
		}
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(true), WithConcurrency(20))
	worker.processBatch(context.Background())

	// 50 URLs x 1 batch each = 50 requests.
	if got := requestCount.Load(); got != 50 {
		t.Fatalf("expected 50 batch requests, got %d", got)
	}

	for _, d := range ms.getDeliveries() {
		if d.Status != domain.WebhookStatusDelivered {
			t.Fatalf("expected delivered for %s, got %s", d.ID, d.Status)
		}
	}
}

func TestAttemptBatchDelivery_EventTriggerNotifyStatusUpdated(t *testing.T) {
	// Verify that event trigger notify status is set to "sent" on batch success.
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := []domain.WebhookDelivery{{
		ID: "evt-batch", EventTriggerID: "evt-42", WebhookURL: ts.URL,
		Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
		LastError: `{"event_key":"test"}`,
	}}
	_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[0])

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	if ms.getNotifyStatus() != "sent" {
		t.Fatalf("expected notify status=sent, got %s", ms.getNotifyStatus())
	}
}

func TestAttemptBatchDelivery_EventTriggerNotifyStatusFailed(t *testing.T) {
	// Verify that event trigger notify status is set to "failed" when dead-lettered.
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := []domain.WebhookDelivery{{
		ID: "evt-batch-fail", EventTriggerID: "evt-43", WebhookURL: ts.URL,
		Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
		LastError: `{"event_key":"test"}`,
	}}
	_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[0])

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	if ms.getNotifyStatus() != "failed" {
		t.Fatalf("expected notify status=failed, got %s", ms.getNotifyStatus())
	}
}

// Consistency tests: verify behavioral parity between batch and individual paths.

func TestAttemptBatchDelivery_NoDoubleClickHouseOnFallback(t *testing.T) {
	// BUG regression: batch fallback used to emit double ClickHouse events
	// (once from the deferred enqueueBatchDeliveryEvents, once from each
	// attemptDelivery). Must be exactly N events for N deliveries.
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:       true,
		BatchSize:     100,
		FlushInterval: time.Minute,
	}, slog.Default())

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)

	const count = 3
	deliveries := make([]domain.WebhookDelivery, count)
	for i := range deliveries {
		deliveries[i] = domain.WebhookDelivery{
			ID: fmt.Sprintf("dblch-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	// maxPayloadBytes=10 forces fallback to individual delivery.
	worker := NewDeliveryWorker(ms, slog.Default(), WithChExporter(exporter), WithMaxPayloadBytes(10))
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	// Must be exactly 3, not 6 (which would indicate double-counting).
	if got := exporter.PendingCount(); got != count {
		t.Fatalf("expected exactly %d ClickHouse events (no double-count), got %d", count, got)
	}
}

func TestAttemptBatchDelivery_ClickHouseExactCountOnSuccess(t *testing.T) {
	// Verify exactly N ClickHouse events for a successful N-item batch.
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:       true,
		BatchSize:     100,
		FlushInterval: time.Minute,
	}, slog.Default())

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)

	const count = 5
	deliveries := make([]domain.WebhookDelivery, count)
	for i := range deliveries {
		deliveries[i] = domain.WebhookDelivery{
			ID: fmt.Sprintf("chcount-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithChExporter(exporter))
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	if got := exporter.PendingCount(); got != count {
		t.Fatalf("expected exactly %d ClickHouse events, got %d", count, got)
	}
}

func TestAttemptBatchDelivery_ClickHouseExactCountOn5xx(t *testing.T) {
	// Verify exactly N ClickHouse events for a failed N-item batch.
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	exporter := clickhouse.NewExporter(&clickhouse.Client{}, clickhouse.ExporterConfig{
		Enabled:       true,
		BatchSize:     100,
		FlushInterval: time.Minute,
	}, slog.Default())

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)

	const count = 4
	deliveries := make([]domain.WebhookDelivery, count)
	for i := range deliveries {
		deliveries[i] = domain.WebhookDelivery{
			ID: fmt.Sprintf("ch5xx-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithChExporter(exporter))
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	if got := exporter.PendingCount(); got != count {
		t.Fatalf("expected exactly %d ClickHouse events on 5xx, got %d", count, got)
	}
}

func TestBatchAndIndividual_SameDelivery_SameOutcome(t *testing.T) {
	// Consistency test: a single delivery should produce the same outcome
	// whether processed via individual or batch path.
	t.Parallel()

	var individualPayload, batchPayload []byte
	var mu sync.Mutex

	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		individualPayload, _ = io.ReadAll(r.Body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts1.Close()

	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		batchPayload, _ = io.ReadAll(r.Body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts2.Close()

	originalPayload := `{"run_id":"run-1","status":"completed"}`
	now := time.Now().Add(-time.Second)

	// Individual path.
	ms1 := &mockDeliveryStore{}
	d1 := &domain.WebhookDelivery{
		ID: "consistency-individual", WebhookURL: ts1.URL,
		Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
		LastError: originalPayload,
	}
	_ = ms1.CreateWebhookDelivery(context.Background(), d1)
	w1 := NewDeliveryWorker(ms1, slog.Default())
	w1.attemptDelivery(context.Background(), d1)

	// Batch path (single item, forced through batch).
	ms2 := &mockDeliveryStore{}
	d2 := &domain.WebhookDelivery{
		ID: "consistency-batch", WebhookURL: ts2.URL,
		Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
		LastError: originalPayload,
	}
	_ = ms2.CreateWebhookDelivery(context.Background(), d2)
	w2 := NewDeliveryWorker(ms2, slog.Default())
	batchDeliveries := []domain.WebhookDelivery{*d2}
	w2.attemptBatchDelivery(context.Background(), ts2.URL, batchDeliveries)

	mu.Lock()
	defer mu.Unlock()

	// Individual sends the raw payload directly.
	var indParsed map[string]any
	if err := json.Unmarshal(individualPayload, &indParsed); err != nil {
		t.Fatalf("individual payload not valid JSON: %v", err)
	}
	if indParsed["run_id"] != "run-1" {
		t.Fatalf("individual payload missing run_id")
	}

	// Batch wraps in array with delivery_id.
	var batchParsed []batchPayloadItem
	if err := json.Unmarshal(batchPayload, &batchParsed); err != nil {
		t.Fatalf("batch payload not valid JSON array: %v", err)
	}
	if len(batchParsed) != 1 {
		t.Fatalf("expected 1 item in batch, got %d", len(batchParsed))
	}
	var batchInnerPayload map[string]any
	if err := json.Unmarshal(batchParsed[0].Payload, &batchInnerPayload); err != nil {
		t.Fatalf("batch inner payload not valid JSON: %v", err)
	}
	if batchInnerPayload["run_id"] != "run-1" {
		t.Fatalf("batch inner payload missing run_id")
	}

	// Both should be delivered.
	if d1.Status != domain.WebhookStatusDelivered {
		t.Fatalf("individual: expected delivered, got %s", d1.Status)
	}
	if batchDeliveries[0].Status != domain.WebhookStatusDelivered {
		t.Fatalf("batch: expected delivered, got %s", batchDeliveries[0].Status)
	}

	// Both should have 1 attempt.
	if d1.Attempts != 1 {
		t.Fatalf("individual: expected 1 attempt, got %d", d1.Attempts)
	}
	if batchDeliveries[0].Attempts != 1 {
		t.Fatalf("batch: expected 1 attempt, got %d", batchDeliveries[0].Attempts)
	}
}

func TestBatchAndIndividual_5xx_SameRetryBehavior(t *testing.T) {
	// Consistency test: 5xx should produce same retry behavior in both paths.
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	now := time.Now().Add(-time.Second)

	// Individual path.
	ms1 := &mockDeliveryStore{}
	d1 := &domain.WebhookDelivery{
		ID: "retry-ind", WebhookURL: ts.URL, RetryPolicy: domain.WebhookRetryPolicyExponential,
		Status: domain.WebhookStatusPending, Attempts: 0, MaxAttempts: 5, NextRetryAt: &now,
		LastError: `{"data":1}`,
	}
	_ = ms1.CreateWebhookDelivery(context.Background(), d1)
	w1 := NewDeliveryWorker(ms1, slog.Default())
	w1.attemptDelivery(context.Background(), d1)

	// Batch path.
	ms2 := &mockDeliveryStore{}
	d2 := domain.WebhookDelivery{
		ID: "retry-batch", WebhookURL: ts.URL, RetryPolicy: domain.WebhookRetryPolicyExponential,
		Status: domain.WebhookStatusPending, Attempts: 0, MaxAttempts: 5, NextRetryAt: &now,
		LastError: `{"data":1}`,
	}
	_ = ms2.CreateWebhookDelivery(context.Background(), &d2)
	batchDeliveries := []domain.WebhookDelivery{d2}
	w2 := NewDeliveryWorker(ms2, slog.Default())
	w2.attemptBatchDelivery(context.Background(), ts.URL, batchDeliveries)

	// Both should be pending (retry scheduled).
	if d1.Status != domain.WebhookStatusPending {
		t.Fatalf("individual: expected pending, got %s", d1.Status)
	}
	if batchDeliveries[0].Status != domain.WebhookStatusPending {
		t.Fatalf("batch: expected pending, got %s", batchDeliveries[0].Status)
	}

	// Both should have 1 attempt.
	if d1.Attempts != 1 {
		t.Fatalf("individual: expected 1 attempt, got %d", d1.Attempts)
	}
	if batchDeliveries[0].Attempts != 1 {
		t.Fatalf("batch: expected 1 attempt, got %d", batchDeliveries[0].Attempts)
	}

	// Both should have future next_retry_at.
	if d1.NextRetryAt == nil || !d1.NextRetryAt.After(time.Now()) {
		t.Fatal("individual: expected next_retry_at in the future")
	}
	if batchDeliveries[0].NextRetryAt == nil || !batchDeliveries[0].NextRetryAt.After(time.Now()) {
		t.Fatal("batch: expected next_retry_at in the future")
	}
}

func TestBatchAndIndividual_4xx_SameDeadLetterBehavior(t *testing.T) {
	// Consistency test: 4xx should dead-letter in both paths.
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	now := time.Now().Add(-time.Second)

	// Individual path.
	ms1 := &mockDeliveryStore{}
	d1 := &domain.WebhookDelivery{
		ID: "dead-ind", WebhookURL: ts.URL,
		Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
		LastError: `{"data":1}`,
	}
	_ = ms1.CreateWebhookDelivery(context.Background(), d1)
	w1 := NewDeliveryWorker(ms1, slog.Default())
	w1.attemptDelivery(context.Background(), d1)

	// Batch path.
	ms2 := &mockDeliveryStore{}
	d2 := domain.WebhookDelivery{
		ID: "dead-batch", WebhookURL: ts.URL,
		Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
		LastError: `{"data":1}`,
	}
	_ = ms2.CreateWebhookDelivery(context.Background(), &d2)
	batchDeliveries := []domain.WebhookDelivery{d2}
	w2 := NewDeliveryWorker(ms2, slog.Default())
	w2.attemptBatchDelivery(context.Background(), ts.URL, batchDeliveries)

	// Both should be dead.
	if d1.Status != domain.WebhookStatusDead {
		t.Fatalf("individual: expected dead, got %s", d1.Status)
	}
	if batchDeliveries[0].Status != domain.WebhookStatusDead {
		t.Fatalf("batch: expected dead, got %s", batchDeliveries[0].Status)
	}
}

func TestProcessBatch_BatchEnabled_RunWorker_IntegrationTest(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()

	t.Parallel()

	// Integration test: verify the full RunWorker -> processBatch -> batch path
	// delivers correctly with batching enabled.
	var mu sync.Mutex
	receivedBodies := make([][]byte, 0)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBodies = append(receivedBodies, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	// 3 deliveries to same URL -> should batch.
	for i := range 3 {
		d := &domain.WebhookDelivery{
			ID: fmt.Sprintf("integ-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"run_id":"run-%d"}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), d)
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(true), WithConcurrency(10))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	concWG.Go(func() {
		_ = worker.RunWorker(ctx, 50*time.Millisecond)
	})

	// Wait for delivery.
	deadline := time.After(2 * time.Second)
	for {
		allDelivered := true
		for _, d := range ms.getDeliveries() {
			if d.Status != domain.WebhookStatusDelivered {
				allDelivered = false
				break
			}
		}
		if allDelivered && len(ms.getDeliveries()) == 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for batch delivery via RunWorker")
		case <-time.After(50 * time.Millisecond):
		}
	}

	cancel()

	mu.Lock()
	defer mu.Unlock()

	// Should have received exactly 1 batch request (not 3 individual ones).
	if len(receivedBodies) != 1 {
		t.Fatalf("expected 1 batch HTTP request, got %d", len(receivedBodies))
	}

	// Verify it was a batch payload (JSON array).
	var items []batchPayloadItem
	if err := json.Unmarshal(receivedBodies[0], &items); err != nil {
		t.Fatalf("expected batch JSON array: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items in batch, got %d", len(items))
	}
}

func TestAttemptBatchDelivery_FallbackAttemptsNotDoubled(t *testing.T) {
	// Verify that the fallback path does not double-increment attempts.
	// The batch path increments, then decrements on fallback. attemptDelivery
	// increments again. Net result should be exactly 1 for fresh deliveries.
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := make([]domain.WebhookDelivery, 3)
	for i := range deliveries {
		deliveries[i] = domain.WebhookDelivery{
			ID: fmt.Sprintf("attempt-check-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, Attempts: 0, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithMaxPayloadBytes(10))
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	for _, d := range ms.getDeliveries() {
		if d.Attempts != 1 {
			t.Fatalf("expected exactly 1 attempt for %s after fallback, got %d (double-increment bug)", d.ID, d.Attempts)
		}
	}
}

func TestAttemptBatchDelivery_FallbackRetryPolicyPreserved(t *testing.T) {
	// Verify that deliveries with non-default retry policies preserve
	// their policy through the batch fallback path.
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := []domain.WebhookDelivery{
		{ID: "rp-exp", WebhookURL: ts.URL, RetryPolicy: domain.WebhookRetryPolicyExponential,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: `{"i":0}`},
		{ID: "rp-linear", WebhookURL: ts.URL, RetryPolicy: domain.WebhookRetryPolicyLinear,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: `{"i":1}`},
		{ID: "rp-fixed", WebhookURL: ts.URL, RetryPolicy: domain.WebhookRetryPolicyFixed,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: `{"i":2}`},
	}
	for i := range deliveries {
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	// Force fallback to individual delivery.
	worker := NewDeliveryWorker(ms, slog.Default(), WithMaxPayloadBytes(10))
	worker.attemptBatchDelivery(context.Background(), ts.URL, deliveries)

	for _, d := range ms.getDeliveries() {
		switch d.ID {
		case "rp-exp":
			if d.RetryPolicy != domain.WebhookRetryPolicyExponential {
				t.Fatalf("expected exponential for %s, got %s", d.ID, d.RetryPolicy)
			}
		case "rp-linear":
			if d.RetryPolicy != domain.WebhookRetryPolicyLinear {
				t.Fatalf("expected linear for %s, got %s", d.ID, d.RetryPolicy)
			}
		case "rp-fixed":
			if d.RetryPolicy != domain.WebhookRetryPolicyFixed {
				t.Fatalf("expected fixed for %s, got %s", d.ID, d.RetryPolicy)
			}
		}
	}
}

func TestProcessBatch_IndividualAndBatch_SameStoreUpdates(t *testing.T) {
	// Verify that both paths result in the same store update count.
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	now := time.Now().Add(-time.Second)

	// Individual: 3 deliveries -> 3 store updates.
	ms1 := &mockDeliveryStore{}
	for i := range 3 {
		d := &domain.WebhookDelivery{
			ID: fmt.Sprintf("store-ind-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms1.CreateWebhookDelivery(context.Background(), d)
	}
	w1 := NewDeliveryWorker(ms1, slog.Default(), WithConcurrency(10))
	w1.processIndividual(context.Background(), func() []domain.WebhookDelivery {
		out := make([]domain.WebhookDelivery, len(ms1.getDeliveries()))
		for i, d := range ms1.getDeliveries() {
			out[i] = *d
		}
		return out
	}())

	// Batch: 3 deliveries -> 3 store updates (one per delivery).
	ms2 := &mockDeliveryStore{}
	for i := range 3 {
		d := &domain.WebhookDelivery{
			ID: fmt.Sprintf("store-batch-%d", i), WebhookURL: ts.URL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms2.CreateWebhookDelivery(context.Background(), d)
	}
	w2 := NewDeliveryWorker(ms2, slog.Default(), WithBatchByURL(true), WithConcurrency(10))
	w2.processBatch(context.Background())

	// Both should have all deliveries marked delivered.
	for _, d := range ms1.getDeliveries() {
		if d.Status != domain.WebhookStatusDelivered {
			t.Fatalf("individual: expected delivered for %s, got %s", d.ID, d.Status)
		}
	}
	for _, d := range ms2.getDeliveries() {
		if d.Status != domain.WebhookStatusDelivered {
			t.Fatalf("batch: expected delivered for %s, got %s", d.ID, d.Status)
		}
	}
}

func TestAttemptBatchDelivery_ConnectionError_RetriesAll(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()

	t.Parallel()

	// Edge case: server immediately closes connection.
	// Use a listener that immediately closes connections.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	concWG.Go(func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	})
	defer listener.Close()

	deadURL := "http://" + listener.Addr().String()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	deliveries := make([]domain.WebhookDelivery, 3)
	for i := range deliveries {
		deliveries[i] = domain.WebhookDelivery{
			ID: fmt.Sprintf("conn-err-%d", i), WebhookURL: deadURL,
			Status: domain.WebhookStatusPending, MaxAttempts: 5, NextRetryAt: &now,
			LastError: fmt.Sprintf(`{"i":%d}`, i),
		}
		_ = ms.CreateWebhookDelivery(context.Background(), &deliveries[i])
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.attemptBatchDelivery(context.Background(), deadURL, deliveries)

	// All should be scheduled for retry (connection error is retryable).
	for _, d := range ms.getDeliveries() {
		if d.Status != domain.WebhookStatusPending {
			t.Fatalf("expected pending for %s after connection error, got %s", d.ID, d.Status)
		}
		if d.Attempts != 1 {
			t.Fatalf("expected 1 attempt for %s, got %d", d.ID, d.Attempts)
		}
		if !strings.Contains(d.LastError, "http request") {
			t.Fatalf("expected HTTP error in last_error for %s, got %q", d.ID, d.LastError)
		}
	}
}

// Functional option coverage.

func TestWithRetryPolicy_Exponential(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithRetryPolicy(domain.WebhookRetryPolicyExponential))
	if worker.defaultRetryPolicy != domain.WebhookRetryPolicyExponential {
		t.Fatalf("defaultRetryPolicy = %q, want %q", worker.defaultRetryPolicy, domain.WebhookRetryPolicyExponential)
	}
}

func TestWithRetryPolicy_Linear(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithRetryPolicy(domain.WebhookRetryPolicyLinear))
	if worker.defaultRetryPolicy != domain.WebhookRetryPolicyLinear {
		t.Fatalf("defaultRetryPolicy = %q, want %q", worker.defaultRetryPolicy, domain.WebhookRetryPolicyLinear)
	}
}

func TestWithRetryPolicy_Fixed(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithRetryPolicy(domain.WebhookRetryPolicyFixed))
	if worker.defaultRetryPolicy != domain.WebhookRetryPolicyFixed {
		t.Fatalf("defaultRetryPolicy = %q, want %q", worker.defaultRetryPolicy, domain.WebhookRetryPolicyFixed)
	}
}

func TestWithRetryPolicy_InvalidKeepsDefault(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithRetryPolicy("bogus"))
	if worker.defaultRetryPolicy != domain.WebhookRetryPolicyExponential {
		t.Fatalf("defaultRetryPolicy = %q, want default %q", worker.defaultRetryPolicy, domain.WebhookRetryPolicyExponential)
	}
}

func TestWithRetryPolicy_EmptyKeepsDefault(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithRetryPolicy(""))
	if worker.defaultRetryPolicy != domain.WebhookRetryPolicyExponential {
		t.Fatalf("defaultRetryPolicy = %q, want default %q", worker.defaultRetryPolicy, domain.WebhookRetryPolicyExponential)
	}
}

func TestWithMetrics_SetsMetricsField(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	// Pass nil metrics -- just verify the option sets the field without panic.
	worker := NewDeliveryWorker(ms, slog.Default(), WithMetrics(nil))
	if worker.metrics != nil {
		t.Fatalf("expected nil metrics when passed nil")
	}
}

func TestWithCircuitBreaker_SetsField(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	cb := NewRedisWebhookCircuitBreaker(nil, false)
	worker := NewDeliveryWorker(ms, slog.Default(), WithCircuitBreaker(cb))
	if worker.circuitBreaker != cb {
		t.Fatal("WithCircuitBreaker did not set the circuit breaker")
	}
}

func TestWithMaxPayloadBytes_Positive(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithMaxPayloadBytes(2048))
	if worker.maxPayloadBytes != 2048 {
		t.Fatalf("maxPayloadBytes = %d, want 2048", worker.maxPayloadBytes)
	}
}

func TestWithMaxPayloadBytes_ZeroKeepsDefault(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithMaxPayloadBytes(0))
	if worker.maxPayloadBytes != defaultWebhookMaxPayloadBytes {
		t.Fatalf("maxPayloadBytes = %d, want default %d", worker.maxPayloadBytes, defaultWebhookMaxPayloadBytes)
	}
}

func TestWithBatchByURL_SetsField(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(true))
	if !worker.batchByURL {
		t.Fatal("WithBatchByURL(true) did not set batchByURL")
	}
}

// EnqueueRunWebhook edge cases.

func TestEnqueueRunWebhook_NilJob(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	err := worker.EnqueueRunWebhook(context.Background(), nil, &domain.JobRun{})
	if err == nil {
		t.Fatal("expected error for nil job")
	}
	if !strings.Contains(err.Error(), "job and run are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnqueueRunWebhook_NilRun(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	err := worker.EnqueueRunWebhook(context.Background(), &domain.Job{WebhookURL: "http://example.com"}, nil)
	if err == nil {
		t.Fatal("expected error for nil run")
	}
	if !strings.Contains(err.Error(), "job and run are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnqueueRunWebhook_EmptyWebhookURL(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	job := &domain.Job{ID: "job-empty-url", WebhookURL: ""}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}

	err := worker.EnqueueRunWebhook(context.Background(), job, run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ms.getDeliveries()) != 0 {
		t.Fatal("expected no deliveries for empty webhook URL")
	}
}

func TestEnqueueRunWebhook_NonTerminalStatus(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	job := &domain.Job{ID: "job-nonterminal", WebhookURL: "http://example.com/hook"}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}

	err := worker.EnqueueRunWebhook(context.Background(), job, run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ms.getDeliveries()) != 0 {
		t.Fatal("expected no deliveries for non-terminal run status")
	}
}

func TestEnqueueRunWebhook_FailedStatus(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	job := &domain.Job{ID: "job-failed", WebhookURL: "http://example.com/hook"}
	run := &domain.JobRun{
		ID:        "run-failed",
		JobID:     "job-failed",
		ProjectID: "proj-1",
		Status:    domain.StatusFailed,
		Error:     "something went wrong",
	}

	err := worker.EnqueueRunWebhook(context.Background(), job, run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	deliveries := ms.getDeliveries()
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}

	var payload map[string]any
	if err := json.Unmarshal(deliveries[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["status"] != "failed" {
		t.Fatalf("expected status=failed in payload, got %v", payload["status"])
	}
}

func TestEnqueueRunWebhook_UsesDefaultRetryPolicy(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithRetryPolicy(domain.WebhookRetryPolicyFixed))

	job := &domain.Job{ID: "job-policy", WebhookURL: "http://example.com/hook"}
	run := &domain.JobRun{
		ID:        "run-policy",
		JobID:     "job-policy",
		ProjectID: "proj-1",
		Status:    domain.StatusCompleted,
	}

	if err := worker.EnqueueRunWebhook(context.Background(), job, run); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	deliveries := ms.getDeliveries()
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	if deliveries[0].RetryPolicy != domain.WebhookRetryPolicyFixed {
		t.Fatalf("expected retry_policy=%s, got %s", domain.WebhookRetryPolicyFixed, deliveries[0].RetryPolicy)
	}
}

// NotifyAsyncWithContext error paths.

func TestNotifyAsyncWithContext_EmptyNotifyURL(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	notifier := NewEventNotifier(ms, slog.Default())

	notifier.NotifyAsyncWithContext(context.Background(), &domain.EventTrigger{
		ID:       "evt-empty",
		EventKey: "test",
	})

	if len(ms.getDeliveries()) != 0 {
		t.Fatal("expected no deliveries for empty NotifyURL")
	}
}

func TestNotifyAsyncWithContext_StoreError(t *testing.T) {
	t.Parallel()

	ms := &errorDeliveryStore{createErr: fmt.Errorf("database down")}
	notifier := NewEventNotifier(ms, slog.Default())

	// Should not panic, just log the error.
	notifier.NotifyAsyncWithContext(context.Background(), &domain.EventTrigger{
		ID:        "evt-err",
		EventKey:  "test",
		ProjectID: "proj-1",
		NotifyURL: "http://example.com/hook",
	})

	// Delivery should not have been stored.
	if len(ms.getDeliveries()) != 0 {
		t.Fatal("expected no deliveries when store returns error")
	}
}

func TestNotifyAsyncWithContext_SetsRetryPolicy(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	notifier := NewEventNotifier(ms, slog.Default(), WithRetryPolicy(domain.WebhookRetryPolicyLinear))

	notifier.NotifyAsyncWithContext(context.Background(), &domain.EventTrigger{
		ID:        "evt-policy",
		EventKey:  "test",
		ProjectID: "proj-1",
		NotifyURL: "http://example.com/hook",
	})

	deliveries := ms.getDeliveries()
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	if deliveries[0].RetryPolicy != domain.WebhookRetryPolicyLinear {
		t.Fatalf("expected retry_policy=%s, got %s", domain.WebhookRetryPolicyLinear, deliveries[0].RetryPolicy)
	}
}

func TestNotifyAsyncWithContext_PayloadContainsCallbackURL(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	notifier := NewEventNotifier(ms, slog.Default())

	notifier.NotifyAsyncWithContext(context.Background(), &domain.EventTrigger{
		ID:        "evt-callback",
		EventKey:  "my-event",
		ProjectID: "proj-1",
		NotifyURL: "http://example.com/hook",
	})

	deliveries := ms.getDeliveries()
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}

	var payload map[string]any
	if err := json.Unmarshal(deliveries[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	expected := "/v1/events/my-event/send"
	if payload["callback_url"] != expected {
		t.Fatalf("expected callback_url=%s, got %v", expected, payload["callback_url"])
	}
}

func TestNewDeliveryWorker_NilLogger(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, nil)
	if worker == nil {
		t.Fatal("NewDeliveryWorker returned nil with nil logger")
	}
}

func TestNewEventNotifier_IsAlias(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	notifier := NewEventNotifier(ms, slog.Default())
	if notifier == nil {
		t.Fatal("NewEventNotifier returned nil")
	}
}

// errorDeliveryStore returns errors from CreateWebhookDelivery for testing error paths.
type errorDeliveryStore struct {
	mu         sync.Mutex
	deliveries []*domain.WebhookDelivery
	createErr  error
}

func (m *errorDeliveryStore) CreateWebhookDelivery(_ context.Context, _ *domain.WebhookDelivery) error {
	return m.createErr
}

func (m *errorDeliveryStore) UpdateWebhookDelivery(_ context.Context, _ *domain.WebhookDelivery) error {
	return nil
}

func (m *errorDeliveryStore) ListPendingWebhookRetries(_ context.Context) ([]domain.WebhookDelivery, error) {
	return nil, nil
}

func (m *errorDeliveryStore) UpdateEventTriggerNotifyStatus(_ context.Context, _ string, _ string) error {
	return nil
}

func (m *errorDeliveryStore) GetWebhookSubscriptionSecrets(_ context.Context, _ string) (string, string, *time.Time, error) {
	return "", "", nil, nil
}

func (m *errorDeliveryStore) getDeliveries() []*domain.WebhookDelivery {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]*domain.WebhookDelivery, len(m.deliveries))
	copy(cp, m.deliveries)
	return cp
}

func TestAttemptDelivery_WithSubscriptionID_SignsHMAC(t *testing.T) {
	t.Parallel()

	var receivedSigHeader, receivedStraitSigHeader, receivedTimestamp string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSigHeader = r.Header.Get("X-Webhook-Signature")
		receivedStraitSigHeader = r.Header.Get("X-Strait-Signature")
		receivedTimestamp = r.Header.Get("X-Strait-Timestamp")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	secret := "test-hmac-secret-key"
	ms := &mockDeliveryStore{
		getSecretsFn: func(_ context.Context, _ string) (string, string, *time.Time, error) {
			return secret, "", nil, nil
		},
	}
	now := time.Now().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		ID:             "whd-hmac-1",
		SubscriptionID: "sub-1",
		WebhookURL:     ts.URL,
		Status:         domain.WebhookStatusPending,
		MaxAttempts:    5,
		NextRetryAt:    &now,
		LastError:      `{"event":"run.completed"}`,
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	worker := NewDeliveryWorker(ms, slog.Default(),
		WithAllowPrivateEndpoints(true),
		WithHTTPTransport(5*time.Second, time.Second, 2, 2),
	)
	worker.processBatch(context.Background())

	if receivedSigHeader == "" {
		t.Fatal("expected X-Webhook-Signature header to be set")
	}
	if receivedTimestamp == "" {
		t.Fatal("expected X-Strait-Timestamp header to be set")
	}
	if !strings.HasPrefix(receivedSigHeader, "v1=") {
		t.Fatalf("expected v1= prefix, got %q", receivedSigHeader)
	}
	if receivedStraitSigHeader != receivedSigHeader {
		t.Fatalf("X-Strait-Signature mismatch: got %q, want %q", receivedStraitSigHeader, receivedSigHeader)
	}
	expectedSig := ComputeTimestampedHMACSHA256(secret, receivedTimestamp, []byte(`{"event":"run.completed"}`))
	if receivedSigHeader != "v1="+expectedSig {
		t.Fatalf("signature mismatch: got %q, want v1=%s", receivedSigHeader, expectedSig)
	}
	bodyOnlySig := "v1=" + ComputeHMACSHA256(secret, []byte(`{"event":"run.completed"}`))
	if receivedSigHeader == bodyOnlySig {
		t.Fatal("signature must bind the timestamp, not only the body")
	}
}

func TestAttemptDelivery_WithRunWebhookSecret_SignsHMAC(t *testing.T) {
	t.Parallel()

	var receivedSigHeader, receivedStraitSigHeader, receivedTimestamp, receivedReplayKey string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSigHeader = r.Header.Get("X-Webhook-Signature")
		receivedStraitSigHeader = r.Header.Get("X-Strait-Signature")
		receivedTimestamp = r.Header.Get("X-Strait-Timestamp")
		receivedReplayKey = r.Header.Get("X-Strait-Replay-Key")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	secret := "job-hmac-secret-key"
	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		ID:            "whd-run-hmac-1",
		RunID:         "run-1",
		JobID:         "job-1",
		WebhookURL:    ts.URL,
		WebhookSecret: secret,
		Status:        domain.WebhookStatusPending,
		MaxAttempts:   5,
		NextRetryAt:   &now,
		Payload:       json.RawMessage(`{"event":"run.completed"}`),
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	worker := NewDeliveryWorker(ms, slog.Default(),
		WithAllowPrivateEndpoints(true),
		WithHTTPTransport(5*time.Second, time.Second, 2, 2),
	)
	worker.processBatch(context.Background())

	if receivedSigHeader == "" {
		t.Fatal("expected X-Webhook-Signature header to be set")
	}
	if receivedStraitSigHeader != receivedSigHeader {
		t.Fatalf("X-Strait-Signature mismatch: got %q, want %q", receivedStraitSigHeader, receivedSigHeader)
	}
	expectedSig := ComputeTimestampedHMACSHA256(secret, receivedTimestamp, []byte(`{"event":"run.completed"}`))
	if receivedSigHeader != "v1="+expectedSig {
		t.Fatalf("signature mismatch: got %q, want v1=%s", receivedSigHeader, expectedSig)
	}
	expectedReplayKey := ComputeReplayKey([]byte(secret), delivery.ID)
	if receivedReplayKey != expectedReplayKey {
		t.Fatalf("replay key = %q, want %q", receivedReplayKey, expectedReplayKey)
	}
	if receivedReplayKey == ComputeReplayKeyUnsigned(delivery.ID) {
		t.Fatal("signed run webhook replay key must be HMAC-bound")
	}
}

// fakeSecretDecryptor reverses a fixed prefix to simulate AES-GCM decryption
// without pulling crypto into the webhook test surface. Production wires
// crypto.Encryptor through WithSecretDecryptor.
type fakeSecretDecryptor struct {
	prefix string
}

func (f fakeSecretDecryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	if !bytes.HasPrefix(ciphertext, []byte(f.prefix)) {
		return nil, fmt.Errorf("not encrypted")
	}
	return ciphertext[len(f.prefix):], nil
}

// Regression: when the store returns an encrypted secret, the
// outbound HMAC signature MUST be computed over the decrypted plaintext.
// Before the fix, the worker hashed the ciphertext directly, so subscribers
// could never validate signatures against their shared secret.
func TestAttemptDelivery_WithSubscriptionID_DecryptsSecretBeforeSigning(t *testing.T) {
	t.Parallel()

	var receivedSigHeader, receivedTimestamp string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSigHeader = r.Header.Get("X-Webhook-Signature")
		receivedTimestamp = r.Header.Get("X-Strait-Timestamp")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	plaintextSecret := "shared-with-subscriber"
	const fakePrefix = "ENC::"
	storedCiphertext := base64.StdEncoding.EncodeToString([]byte(fakePrefix + plaintextSecret))

	ms := &mockDeliveryStore{
		getSecretsFn: func(_ context.Context, _ string) (string, string, *time.Time, error) {
			return storedCiphertext, "", nil, nil
		},
	}
	now := time.Now().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		ID:             "whd-enc-1",
		SubscriptionID: "sub-enc",
		WebhookURL:     ts.URL,
		Status:         domain.WebhookStatusPending,
		MaxAttempts:    5,
		NextRetryAt:    &now,
		LastError:      `{"event":"run.completed"}`,
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	worker := NewDeliveryWorker(ms, slog.Default(),
		WithSecretDecryptor(fakeSecretDecryptor{prefix: fakePrefix}))
	worker.processBatch(context.Background())

	if receivedSigHeader == "" {
		t.Fatal("expected X-Webhook-Signature header to be set")
	}
	if receivedTimestamp == "" {
		t.Fatal("expected X-Strait-Timestamp header to be set")
	}
	expectedSig := "v1=" + ComputeTimestampedHMACSHA256(plaintextSecret, receivedTimestamp, []byte(`{"event":"run.completed"}`))
	if receivedSigHeader != expectedSig {
		t.Fatalf("signature was not computed over plaintext secret\n  got:  %q\n  want: %q", receivedSigHeader, expectedSig)
	}
	ciphertextSig := "v1=" + ComputeTimestampedHMACSHA256(storedCiphertext, receivedTimestamp, []byte(`{"event":"run.completed"}`))
	if receivedSigHeader == ciphertextSig {
		t.Fatal("signature was computed over ciphertext regression")
	}
}

// Regression: tenants with the same external webhook URL must
// not share a circuit-breaker key. Without per-tenant scoping, one noisy
// tenant could trip the breaker for everyone pointing at the same shared
// receiver (cross-tenant DoS).
func TestBreakerKey_PerTenantScoping(t *testing.T) {
	t.Parallel()

	url := "https://hooks.example.com/in"
	if breakerKey("orgA", url) == breakerKey("orgB", url) {
		t.Fatal("breakerKey must produce distinct keys for different orgs on the same URL")
	}
	if breakerKey("", url) != url {
		t.Fatalf("empty orgID must fall through to URL-only key for backwards compat, got %q", breakerKey("", url))
	}
}

func TestAttemptDelivery_WithSubscriptionID_SecretRotation_GracePeriodActive(t *testing.T) {
	t.Parallel()

	var receivedSig, receivedOldSig, receivedStraitOldSig string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-Webhook-Signature")
		receivedOldSig = r.Header.Get("X-Webhook-Signature-Old")
		receivedStraitOldSig = r.Header.Get("X-Strait-Signature-Old")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	secret := "new-secret"
	prevSecret := "old-secret"
	futureTime := time.Now().Add(time.Hour)
	ms := &mockDeliveryStore{
		getSecretsFn: func(_ context.Context, _ string) (string, string, *time.Time, error) {
			return secret, prevSecret, &futureTime, nil
		},
	}
	now := time.Now().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		ID:             "whd-rotate-1",
		SubscriptionID: "sub-1",
		WebhookURL:     ts.URL,
		Status:         domain.WebhookStatusPending,
		MaxAttempts:    5,
		NextRetryAt:    &now,
		LastError:      `{"event":"run.completed"}`,
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())

	if receivedSig == "" {
		t.Fatal("expected X-Webhook-Signature header")
	}
	if receivedOldSig == "" {
		t.Fatal("expected X-Webhook-Signature-Old header during active grace period")
	}
	if !strings.HasPrefix(receivedOldSig, "v1=") {
		t.Fatalf("expected v1= prefix on old sig, got %q", receivedOldSig)
	}
	if receivedStraitOldSig != receivedOldSig {
		t.Fatalf("X-Strait-Signature-Old mismatch: got %q, want %q", receivedStraitOldSig, receivedOldSig)
	}
}

func TestAttemptDelivery_WithSubscriptionID_SecretRotation_GracePeriodExpired(t *testing.T) {
	t.Parallel()

	var receivedSig, receivedOldSig string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-Webhook-Signature")
		receivedOldSig = r.Header.Get("X-Webhook-Signature-Old")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	pastTime := time.Now().Add(-time.Hour)
	ms := &mockDeliveryStore{
		getSecretsFn: func(_ context.Context, _ string) (string, string, *time.Time, error) {
			return "new-secret", "old-secret", &pastTime, nil
		},
	}
	now := time.Now().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		ID:             "whd-expired-1",
		SubscriptionID: "sub-1",
		WebhookURL:     ts.URL,
		Status:         domain.WebhookStatusPending,
		MaxAttempts:    5,
		NextRetryAt:    &now,
		LastError:      `{"event":"run.completed"}`,
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())

	if receivedSig == "" {
		t.Fatal("expected X-Webhook-Signature header")
	}
	if receivedOldSig != "" {
		t.Fatalf("expected no X-Webhook-Signature-Old after grace period expired, got %q", receivedOldSig)
	}
}

func TestAttemptDelivery_WithSubscriptionID_SecretsLookupError(t *testing.T) {
	t.Parallel()

	var requestReceived atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived.Store(true)
		if r.Header.Get("X-Webhook-Signature") != "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{
		getSecretsFn: func(_ context.Context, _ string) (string, string, *time.Time, error) {
			return "", "", nil, fmt.Errorf("secrets store unavailable")
		},
	}
	now := time.Now().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		ID:             "whd-err-1",
		SubscriptionID: "sub-1",
		WebhookURL:     ts.URL,
		Status:         domain.WebhookStatusPending,
		MaxAttempts:    5,
		NextRetryAt:    &now,
		LastError:      `{"event":"run.completed"}`,
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())

	if requestReceived.Load() {
		t.Fatal("delivery should not be attempted without a subscription signing secret")
	}
	deliveries := ms.getDeliveries()
	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(deliveries))
	}
	if deliveries[0].LastError != "webhook subscription signing secret unavailable" {
		t.Fatalf("last_error = %q, want signing secret error", deliveries[0].LastError)
	}
}

func TestAttemptDelivery_EmptyFields_NoConditionalHeaders(t *testing.T) {
	t.Parallel()

	var headers http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		ID:             "whd-no-headers",
		EventTriggerID: "",
		RunID:          "",
		JobID:          "",
		WebhookURL:     ts.URL,
		Status:         domain.WebhookStatusPending,
		MaxAttempts:    5,
		NextRetryAt:    &now,
		LastError:      `{"test":"data"}`,
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())

	if headers == nil {
		t.Fatal("expected request to be received")
	}
	if v := headers.Get("X-Strait-Trigger-ID"); v != "" {
		t.Fatalf("expected no X-Strait-Trigger-ID when EventTriggerID is empty, got %q", v)
	}
	if v := headers.Get("X-Run-ID"); v != "" {
		t.Fatalf("expected no X-Run-ID when RunID is empty, got %q", v)
	}
	if v := headers.Get("X-Job-ID"); v != "" {
		t.Fatalf("expected no X-Job-ID when JobID is empty, got %q", v)
	}
}

func TestAttemptDelivery_PopulatedFields_ConditionalHeadersPresent(t *testing.T) {
	t.Parallel()

	var headers http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		ID:             "whd-with-headers",
		EventTriggerID: "evt-42",
		RunID:          "run-42",
		JobID:          "job-42",
		WebhookURL:     ts.URL,
		Status:         domain.WebhookStatusPending,
		MaxAttempts:    5,
		NextRetryAt:    &now,
		LastError:      `{"test":"data"}`,
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())

	if headers == nil {
		t.Fatal("expected request to be received")
	}
	if v := headers.Get("X-Strait-Trigger-ID"); v != "evt-42" {
		t.Fatalf("X-Strait-Trigger-ID = %q, want %q", v, "evt-42")
	}
	if v := headers.Get("X-Run-ID"); v != "run-42" {
		t.Fatalf("X-Run-ID = %q, want %q", v, "run-42")
	}
	if v := headers.Get("X-Job-ID"); v != "job-42" {
		t.Fatalf("X-Job-ID = %q, want %q", v, "job-42")
	}
}

func TestAttemptDelivery_RetryExhausted_DeadLetters(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	ms := &mockDeliveryStore{}
	now := time.Now().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		ID:          "whd-exhausted-1",
		RunID:       "run-1",
		JobID:       "job-1",
		WebhookURL:  ts.URL,
		Status:      domain.WebhookStatusPending,
		Attempts:    4,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   `{"run_id":"run-1"}`,
	}
	if err := ms.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())

	deliveries := ms.getDeliveries()
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	if deliveries[0].Status != domain.WebhookStatusDead {
		t.Fatalf("expected dead status when retryable but exhausted, got %s", deliveries[0].Status)
	}
}

func TestBackoffForRetryPolicy_ExponentialCapsAt30Min(t *testing.T) {
	t.Parallel()

	// 5^5 = 3125 seconds = ~52 minutes, exceeds 30-minute cap.
	got := backoffForRetryPolicy(domain.WebhookRetryPolicyExponential, 5)
	approxBackoff(t, got, 30*time.Minute, "exponential attempt 5 (capped)")
	if got > 30*time.Minute+(30*time.Minute)/5 {
		t.Fatalf("exponential backoff for attempt 5 = %s, exceeded 30m cap + jitter", got)
	}
}

func TestBackoffForRetryPolicy_AttemptsZero_NormalizedToOne(t *testing.T) {
	t.Parallel()

	// Both should normalize to the same base; jitter +/- 20% means the
	// observed values still cluster around 5s (the exponential base).
	gotZero := backoffForRetryPolicy(domain.WebhookRetryPolicyExponential, 0)
	gotOne := backoffForRetryPolicy(domain.WebhookRetryPolicyExponential, 1)
	approxBackoff(t, gotZero, 5*time.Second, "exponential attempt 0 (normalized)")
	approxBackoff(t, gotOne, 5*time.Second, "exponential attempt 1")
}

func TestBackoffForRetryPolicy_LinearCapsAt30Min(t *testing.T) {
	t.Parallel()

	got := backoffForRetryPolicy(domain.WebhookRetryPolicyLinear, 500)
	approxBackoff(t, got, 30*time.Minute, "linear attempt 500 (capped)")
	if got > 30*time.Minute+(30*time.Minute)/5 {
		t.Fatalf("linear backoff for attempt 500 = %s, exceeded 30m cap + jitter", got)
	}
}

func TestBackoffForRetryPolicy_HugeAttemptsDoNotOverflow(t *testing.T) {
	t.Parallel()

	exponential := backoffForRetryPolicy(domain.WebhookRetryPolicyExponential, 1_000_000)
	approxBackoff(t, exponential, maxWebhookBackoff, "huge exponential attempt")
	if exponential <= 0 {
		t.Fatalf("huge exponential backoff = %s, want positive capped duration", exponential)
	}

	linear := backoffForRetryPolicy(domain.WebhookRetryPolicyLinear, int(^uint(0)>>1))
	approxBackoff(t, linear, maxWebhookBackoff, "huge linear attempt")
	if linear <= 0 {
		t.Fatalf("huge linear backoff = %s, want positive capped duration", linear)
	}
}

func TestRecordCircuitBreakerState_AllStates(t *testing.T) {
	t.Parallel()

	m, _, shutdown, err := telemetry.InitMetrics("test-service", "test")
	if err != nil {
		t.Skipf("InitMetrics: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	worker := NewDeliveryWorker(&mockDeliveryStore{}, slog.Default(), WithMetrics(m))

	for _, state := range []string{"closed", "open", "half_open"} {
		worker.recordCircuitBreakerState(context.Background(), "https://example.com/test", state)
	}
}

func TestRecordCircuitBreakerState_NilMetrics_NoOp(t *testing.T) {
	t.Parallel()

	worker := NewDeliveryWorker(&mockDeliveryStore{}, slog.Default())
	worker.recordCircuitBreakerState(context.Background(), "https://example.com/test", "closed")
}

func TestRecordCircuitBreakerState_EmptyURL_NoOp(t *testing.T) {
	t.Parallel()

	m, _, shutdown, err := telemetry.InitMetrics("test-service", "test")
	if err != nil {
		t.Skipf("InitMetrics: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	worker := NewDeliveryWorker(&mockDeliveryStore{}, slog.Default(), WithMetrics(m))
	worker.recordCircuitBreakerState(context.Background(), "", "closed")
}
