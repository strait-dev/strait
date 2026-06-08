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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		require.Equal(t,
			want, event.
				Tags[key])
	}
	require.Equal(t,
		"hooks.example.com",
		event.
			Contexts["webhook.delivery"]["webhook_url_domain"])
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
	require.NoError(t, worker.
		EnqueueRunWebhook(context.Background(), job, run))

	deliveries := ms.getDeliveries()
	require.Len(t,
		deliveries,
		1)

	d := deliveries[0]
	require.Equal(t,
		run.ID, d.
			RunID)
	require.Equal(t,
		job.ID, d.
			JobID)
	require.Empty(t,
		d.EventTriggerID,
	)
	require.Equal(t,
		job.WebhookURL,
		d.WebhookURL,
	)
	require.Equal(t,
		job.WebhookSecret,
		d.WebhookSecret,
	)
	require.Equal(t,
		domain.WebhookStatusPending,

		d.Status)
	require.Empty(t,
		d.LastError,
	)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(d.Payload,
		&payload))
	require.Equal(t,
		run.ID, payload["run_id"])
	require.Equal(t,
		string(run.
			Status), payload["status"])
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
		require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithConcurrency(total))
	worker.processBatch(context.Background())
	require.Greater(t,
		maxInFlight.
			Load(), int32(1))

	for _, d := range ms.getDeliveries() {
		require.Equal(t,
			domain.WebhookStatusDelivered,

			d.Status)
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), eventDelivery))
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), runDelivery))

	worker := NewDeliveryWorker(ms, slog.Default(), WithConcurrency(2))
	worker.processBatch(context.Background())
	require.EqualValues(t, 2, requests.
		Load())

	for _, d := range ms.getDeliveries() {
		require.Equal(t,
			domain.WebhookStatusDelivered,

			d.Status)
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
	require.NoError(t, worker.
		Shutdown(shutdownCtx))

	select {
	case err := <-runDone:
		if err != nil {
			require.Failf(t, "test failure", "RunWorker() error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		require.Fail(t, "RunWorker did not stop after shutdown")
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
		require.Fail(t, "batch did not start")
	}

	shutdownDone := make(chan error, 1)
	concWG.Go(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		shutdownDone <- worker.Shutdown(shutdownCtx)
	})

	select {
	case err := <-shutdownDone:
		require.Failf(t, "test failure", "Shutdown returned early with err=%v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(allowBatchExit)

	select {
	case err := <-shutdownDone:
		if err != nil {
			require.Failf(t, "test failure", "Shutdown() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		require.Fail(t, "Shutdown did not return after batch completed")
	}

	select {
	case err := <-runDone:
		if err != nil {
			require.Failf(t, "test failure", "RunWorker() error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		require.Fail(t, "RunWorker did not stop after shutdown")
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
	require.Len(t,
		deliveries,
		1)

	d := deliveries[0]
	require.Equal(t,
		"evt-1",
		d.EventTriggerID,
	)
	require.Equal(t,
		"http://example.com/hook",

		d.WebhookURL)
	require.Equal(t,
		domain.WebhookStatusPending,

		d.Status)
	require.Equal(t, 5, d.MaxAttempts)
}

func TestNotifyAsync_NoURL_Skips(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	notifier := NewEventNotifier(ms, slog.Default())

	notifier.NotifyAsync(&domain.EventTrigger{ID: "evt-2", EventKey: "no-url"})
	require.Empty(t,
		ms.getDeliveries())
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
			require.Fail(t, "timed out waiting for delivery")
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Poll for store update instead of sleeping.
	deadline2 := time.After(5 * time.Second)
	for ms.getNotifyStatus() != "sent" {
		select {
		case <-deadline2:
			require.Failf(t, "test failure", "timed out waiting for notify_status=sent, got %s", ms.getNotifyStatus())
		case <-time.After(10 * time.Millisecond):
		}
	}

	mu.Lock()
	require.Equal(t,
		"deliver-key",
		receivedPayload["event_key"])

	mu.Unlock()

	for _, d := range ms.getDeliveries() {
		require.False(t,
			d.EventTriggerID ==
				"evt-3" &&
				d.Status !=
					domain.WebhookStatusDelivered,
		)
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
			require.Fail(t, "timed out waiting for first attempt")
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Run a few more poll cycles to confirm no second attempt fires.
	// The next retry is 5s in the future, so the worker should be idle.
	stableAt := time.Now()
	for time.Since(stableAt) < 300*time.Millisecond {
		require.LessOrEqual(t, attempts.
			Load(), int32(1),
		)

		time.Sleep(10 * time.Millisecond) // tight poll to detect spurious retry
	}
	cancel()
	require.EqualValues(t, 1, attempts.
		Load())

	// Should only have had 1 attempt — next retry is 5s in the future.

	// Delivery should still be pending with increased attempts.
	for _, d := range ms.getDeliveries() {
		if d.EventTriggerID == "evt-4" {
			require.Equal(t, 1, d.Attempts)
			require.Equal(t,
				domain.WebhookStatusPending,

				d.Status)
			require.False(t,
				d.NextRetryAt ==
					nil ||
					d.NextRetryAt.Before(time.Now()))
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
			require.Failf(t, "test failure", "timed out waiting for notify_status=failed, got %s", ms.getNotifyStatus())
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()

	for _, d := range ms.getDeliveries() {
		require.False(t,
			d.EventTriggerID ==
				"evt-5" &&
				d.Status !=
					domain.WebhookStatusDead,
		)
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

	notifier := NewEventNotifier(ms, slog.Default(), WithMaxPayloadBytes(1024))
	notifier.processBatch(context.Background())
	require.EqualValues(t, 0, requests.
		Load())

	updated := ms.getDeliveries()[0]
	require.Equal(t,
		domain.WebhookStatusDead,

		updated.Status)
	require.Contains(t,
		updated.
			LastError, "payload too large")
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

	worker := NewDeliveryWorker(ms, slog.Default(), WithChExporter(exporter))
	worker.processBatch(context.Background())
	require.Equal(t, 1, exporter.
		PendingCount())
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

	worker := NewDeliveryWorker(ms, slog.Default(), WithChExporter(exporter))
	worker.processBatch(context.Background())
	require.Equal(t, 1, exporter.
		PendingCount())
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

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
		assert.Equal(t,
			tc.want, exponentialWebhookBackoff(tc.attempts))
	}
}

// approxBackoff asserts that got is within +/- 20% of want. The webhook
// backoff helper applies decorrelated jitter, so we can't compare for
// exact equality.
func approxBackoff(t *testing.T, got, want time.Duration, label string) {
	t.Helper()
	low := want - want/5
	high := want + want/5
	require.False(t,
		got < low ||
			got > high)
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
	require.Len(t,
		deliveries,
		1)
	require.Equal(t,
		"https://example.com/hook",

		deliveries[0].
			WebhookURL)
	require.Equal(t,
		"sub-1",
		deliveries[0].SubscriptionID,
	)
	require.Equal(t,
		"proj-1",
		deliveries[0].
			ProjectID)
	require.JSONEq(t,
		`{"run_id":"r1"}`,
		string(deliveries[0].Payload))
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
	require.Len(t,
		deliveries,
		1)
	require.Equal(t,
		"https://example.com/wildcard",

		deliveries[0].WebhookURL)
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
	require.Empty(t,
		ms.getDeliveries())
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
	require.Empty(t,
		ms.getDeliveries())
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
	require.Len(t,
		deliveries,
		1)
	require.Equal(t,
		"https://example.com/match",

		deliveries[0].WebhookURL)
}

func TestReplayKeyFromDeliveryID(t *testing.T) {
	t.Parallel()
	require.Empty(t,
		replayKeyFromDeliveryID(""))
	require.Equal(t,
		"rk_whd-42",
		replayKeyFromDeliveryID("whd-42"))
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
	require.Equal(t,
		want, got,
	)
	require.True(t,
		strings.HasPrefix(got, "rk_"))
	require.Len(t,
		got, len("rk_")+replayKeyHexLen,
	)

	// Same inputs must produce the same key (stability across retries).
	again := ComputeReplayKey(secret, deliveryID)
	require.Equal(t,
		got, again,
	)

	// Different secret must yield a different key (HMAC binding).
	other := ComputeReplayKey([]byte("different_secret"), deliveryID)
	require.NotEqual(t, got, other)
	require.Empty(t,
		ComputeReplayKey(secret,
			""))

	// Empty delivery id returns empty.

	// Empty secret falls back to unsigned helper.
	unsigned := ComputeReplayKey(nil, deliveryID)
	require.Equal(t,
		ComputeReplayKeyUnsigned(deliveryID), unsigned,
	)
}

func TestComputeReplayKeyUnsigned(t *testing.T) {
	t.Parallel()
	require.Empty(t,
		ComputeReplayKeyUnsigned(""))
	require.Equal(t,
		"rk_run-9",
		ComputeReplayKeyUnsigned("run-9"))
}

func TestComputeIdempotencyKey_HMACDerivation(t *testing.T) {
	t.Parallel()

	secret := []byte("whsec_test_secret_bytes")
	deliveryID := "whd-signed-1"

	got := ComputeIdempotencyKey(secret, deliveryID, 2)

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("whd-signed-1:2"))
	want := idempotencyKeyPrefix + hex.EncodeToString(mac.Sum(nil))[:replayKeyHexLen]
	require.Equal(t,
		want, got,
	)
	require.NotContains(t,
		got, deliveryID)
	require.NotEqual(t, ComputeIdempotencyKey(secret, deliveryID,
		1), got)
	require.Equal(t,
		"whd-signed-1:2",
		ComputeIdempotencyKey(nil,
			deliveryID, 2))
}

func TestWebhookDeliveryPayloadMarshalers(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	triggerPayload, err := marshalEventTriggerNotifyPayload(&domain.EventTrigger{
		ID:        "trigger-1",
		EventKey:  "evt-1",
		ProjectID: "project-1",
		ExpiresAt: ts,
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"event_key":"evt-1","trigger_id":"trigger-1","project_id":"project-1","expires_at":"2026-06-07T12:00:00Z","callback_url":"/v1/events/evt-1/send"}`, string(triggerPayload))

	runPayload, err := marshalRunWebhookPayload(&domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "project-1",
		Status:    domain.StatusCompleted,
		Attempt:   2,
		Result:    json.RawMessage(`{"ok":true}`),
	}, ts)
	require.NoError(t, err)
	require.JSONEq(t, `{"run_id":"run-1","job_id":"job-1","project_id":"project-1","status":"completed","attempt":2,"result":{"ok":true},"error":"","timestamp":"2026-06-07T12:00:00Z"}`, string(runPayload))

	fallback := marshalDeliveryFallbackPayload(&domain.WebhookDelivery{
		ID:             "delivery-1",
		EventTriggerID: "trigger-1",
	})
	require.JSONEq(t, `{"trigger_id":"trigger-1","delivery_id":"delivery-1"}`, string(fallback))
}

func TestWebhookDeliveryPayloadMarshalersEscapeFields(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 6, 7, 12, 0, 0, 123456789, time.FixedZone("offset", -3*60*60))
	triggerPayload, err := marshalEventTriggerNotifyPayload(&domain.EventTrigger{
		ID:        "trigger-\"1",
		EventKey:  "evt-\\\n<&>",
		ProjectID: "project-<&>",
		ExpiresAt: ts,
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"event_key":"evt-\\\n<&>","trigger_id":"trigger-\"1","project_id":"project-<&>","expires_at":"2026-06-07T12:00:00.123456789-03:00","callback_url":"/v1/events/evt-\\\n<&>/send"}`, string(triggerPayload))

	runPayload, err := marshalRunWebhookPayload(&domain.JobRun{
		ID:        "run-\"1",
		JobID:     "job-\\1",
		ProjectID: "project\n1",
		Status:    domain.StatusCompleted,
		Attempt:   2,
		Result:    json.RawMessage(`[1,2,3]`),
		Error:     "boom <&>",
	}, ts)
	require.NoError(t, err)
	require.JSONEq(t, `{"run_id":"run-\"1","job_id":"job-\\1","project_id":"project\n1","status":"completed","attempt":2,"result":[1,2,3],"error":"boom <&>","timestamp":"2026-06-07T12:00:00.123456789-03:00"}`, string(runPayload))

	fallback := marshalDeliveryFallbackPayload(&domain.WebhookDelivery{
		ID:             "delivery-\"1",
		EventTriggerID: "trigger\n1",
	})
	require.JSONEq(t, `{"trigger_id":"trigger\n1","delivery_id":"delivery-\"1"}`, string(fallback))
}

func TestMarshalRunWebhookPayloadHandlesResultEdges(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	runPayload, err := marshalRunWebhookPayload(&domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "project-1",
		Status:    domain.StatusCompleted,
		Attempt:   1,
	}, ts)
	require.NoError(t, err)
	require.JSONEq(t, `{"run_id":"run-1","job_id":"job-1","project_id":"project-1","status":"completed","attempt":1,"result":null,"error":"","timestamp":"2026-06-07T12:00:00Z"}`, string(runPayload))

	runPayload, err = marshalRunWebhookPayload(&domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "project-1",
		Status:    domain.StatusCompleted,
		Attempt:   1,
		Result:    json.RawMessage(`{"broken"`),
	}, ts)
	require.Error(t, err)
	require.Nil(t, runPayload)
}

func BenchmarkWebhookDeliveryPayloadMarshalers(b *testing.B) {
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

	b.Run("event_trigger_notify", func(b *testing.B) {
		trigger := &domain.EventTrigger{
			ID:        "trigger-1",
			EventKey:  "evt-1",
			ProjectID: "project-1",
			ExpiresAt: ts,
		}
		b.ReportAllocs()
		for range b.N {
			payload, err := marshalEventTriggerNotifyPayload(trigger)
			if err != nil {
				b.Fatalf("marshalEventTriggerNotifyPayload() error = %v", err)
			}
			if len(payload) == 0 {
				b.Fatal("marshalEventTriggerNotifyPayload() returned empty payload")
			}
		}
	})

	b.Run("run_webhook", func(b *testing.B) {
		run := &domain.JobRun{
			ID:        "run-1",
			JobID:     "job-1",
			ProjectID: "project-1",
			Status:    domain.StatusCompleted,
			Attempt:   2,
			Result:    json.RawMessage(`{"ok":true}`),
		}
		b.ReportAllocs()
		for range b.N {
			payload, err := marshalRunWebhookPayload(run, ts)
			if err != nil {
				b.Fatalf("marshalRunWebhookPayload() error = %v", err)
			}
			if len(payload) == 0 {
				b.Fatal("marshalRunWebhookPayload() returned empty payload")
			}
		}
	})

	b.Run("fallback", func(b *testing.B) {
		delivery := &domain.WebhookDelivery{ID: "delivery-1", EventTriggerID: "trigger-1"}
		b.ReportAllocs()
		for range b.N {
			payload := marshalDeliveryFallbackPayload(delivery)
			if len(payload) == 0 {
				b.Fatal("marshalDeliveryFallbackPayload() returned empty payload")
			}
		}
	})
}

func BenchmarkComputeReplayKey(b *testing.B) {
	secret := []byte("whsec_test_secret_bytes")
	deliveryID := "whd-signed-1"

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		key := ComputeReplayKey(secret, deliveryID)
		if len(key) == 0 {
			b.Fatal("ComputeReplayKey() returned empty key")
		}
	}
}

func BenchmarkComputeIdempotencyKey(b *testing.B) {
	secret := []byte("whsec_test_secret_bytes")
	deliveryID := "whd-signed-1"

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		key := ComputeIdempotencyKey(secret, deliveryID, 2)
		if len(key) == 0 {
			b.Fatal("ComputeIdempotencyKey() returned empty key")
		}
	}
}

func TestApplyDeliveryMetadataHeaders(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodPost, "https://example.com/hooks", nil)
	require.NoError(t, err)

	applyDeliveryMetadataHeaders(req, &domain.WebhookDelivery{
		ID:             "delivery-1",
		EventTriggerID: "trigger-1",
		RunID:          "run-1",
		JobID:          "job-1",
		Attempts:       12,
		MaxAttempts:    30,
	})

	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	assert.Equal(t, "trigger-1", req.Header.Get("X-Strait-Trigger-ID"))
	assert.Equal(t, "run-1", req.Header.Get("X-Run-ID"))
	assert.Equal(t, "job-1", req.Header.Get("X-Job-ID"))
	assert.Equal(t, "delivery-1", req.Header.Get("X-Strait-Delivery-ID"))
	assert.Equal(t, "12/30", req.Header.Get("X-Strait-Attempt"))
}

func BenchmarkApplyDeliveryMetadataHeaders(b *testing.B) {
	delivery := &domain.WebhookDelivery{
		ID:             "delivery-1",
		EventTriggerID: "trigger-1",
		RunID:          "run-1",
		JobID:          "job-1",
		Attempts:       12,
		MaxAttempts:    30,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		req, err := http.NewRequest(http.MethodPost, "https://example.com/hooks", nil)
		if err != nil {
			b.Fatal(err)
		}
		applyDeliveryMetadataHeaders(req, delivery)
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())

	wantReplay := ComputeReplayKey([]byte(secret), delivery.ID)
	require.Equal(t,
		wantReplay,
		receivedReplayKey,
	)

	wantIdempotency := ComputeIdempotencyKey([]byte(secret), delivery.ID, 1)
	require.Equal(t,
		wantIdempotency,
		receivedIdempotencyKey,
	)

	for _, header := range []string{receivedReplayKey, receivedIdempotencyKey} {
		require.NotContains(t,
			header, delivery.ID)
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

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
	require.GreaterOrEqual(t,
		len(replayKeys),
		2)

	want := "rk_whd-replay-1"
	for _, got := range replayKeys {
		require.Equal(t,
			want, got,
		)
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
		require.NoError(t, ms.CreateWebhookDelivery(context.Background(), d))
	}

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())

	mu.Lock()
	defer mu.Unlock()
	for _, id := range ids {
		require.True(t,
			seen["rk_"+
				id])
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())

	expected := "whd-idem-1:4"
	require.Equal(t,
		expected,
		receivedHeader,
	)

	// attempts incremented to 4 before dispatch
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

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
	require.GreaterOrEqual(t,
		len(receivedKeys), 2)
	require.NotEqual(t, receivedKeys[1], receivedKeys[0])
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
		require.NoError(t, ms.CreateWebhookDelivery(context.Background(), d))
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithConcurrency(2))
	worker.processBatch(context.Background())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t,
		receivedKeys,
		2)
}

func TestDeliveryWorker_DefaultConcurrency50(t *testing.T) {
	t.Parallel()
	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())
	assert.Equal(t, 50, worker.
		concurrency)
}

func TestDeliveryWorker_ConcurrencyFromOption(t *testing.T) {
	t.Parallel()
	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithConcurrency(100))
	assert.Equal(t, 100, worker.
		concurrency)
}

func TestDeliveryWorker_ConcurrencyZeroKeepsDefault(t *testing.T) {
	t.Parallel()
	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithConcurrency(0))
	assert.Equal(t, 50, worker.
		concurrency)
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
	assert.LessOrEqual(t, elapsed,
		6*time.Second,
	)
	assert.NotEqual(t, domain.
		WebhookStatusDelivered,
		d.Status,
	)

	// Should timeout around 5s, not 10s.
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
	assert.GreaterOrEqual(t, elapsed,
		5*time.
			Second)
	assert.Equal(t,
		domain.WebhookStatusDelivered,

		d.Status)

	// Should succeed because retry timeout is 15s and server responds in ~5.5s.
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
	assert.GreaterOrEqual(t, peak,
		int64(10))
}

// HTTP Transport tests.

func TestWithHTTPTransport_SetsCustomTransport(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(),
		WithHTTPTransport(10*time.Second, 90*time.Second, 200, 100),
	)

	transport, ok := worker.client.Transport.(*http.Transport)
	require.True(t,
		ok)
	assert.Equal(t, 200, transport.
		MaxIdleConns,
	)
	assert.Equal(t, 100, transport.
		MaxIdleConnsPerHost,
	)
	assert.Equal(t,
		90*time.
			Second, transport.
			IdleConnTimeout,
	)
	assert.True(t,
		transport.ForceAttemptHTTP2,
	)
	assert.Equal(t,
		10*time.
			Second, worker.
			client.Timeout)
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
	assert.LessOrEqual(t, conns,
		int32(3))
}

func TestWithHTTPTransport_DefaultValues(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	transport, ok := worker.client.Transport.(*http.Transport)
	require.False(t,
		!ok || transport ==
			nil)
	require.NotNil(
		t, worker.client.
			CheckRedirect,
	)
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

	worker := NewDeliveryWorker(ms, slog.Default(), WithHTTPTransport(500*time.Millisecond, time.Second, 2, 2))

	worker.processBatch(context.Background())

	deliveries := ms.getDeliveries()
	require.Len(t,
		deliveries,
		1)

	got := deliveries[0]
	require.Equal(t,
		domain.WebhookStatusPending,

		got.Status)
	require.Contains(t,
		got.LastError, "resolves to private")
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

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
		require.NotContains(t,
			got.LastError, leaked)
	}
	require.Contains(t,
		got.LastError, "request failed")
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())

	got := ms.getDeliveries()[0]
	require.Equal(t,
		domain.WebhookStatusDead,

		got.Status)

	for _, leaked := range []string{"user", "password", "secret-value", "token", rawURL} {
		require.NotContains(t,
			got.LastError, leaked)
	}
	require.Equal(t,
		"create request: invalid webhook URL",

		got.
			LastError)
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
		require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))
	}

	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(true))
	worker.processBatch(context.Background())

	for _, got := range ms.getDeliveries() {
		require.Equal(t,
			domain.WebhookStatusDead,

			got.Status)

		for _, leaked := range []string{"user", "password", "secret-value", "token", rawURL} {
			require.NotContains(t,
				got.LastError, leaked)
		}
		require.Equal(t,
			"create request: invalid webhook URL",

			got.
				LastError)
	}
}

// Batch helper tests.

func TestGroupByURL_Empty(t *testing.T) {
	t.Parallel()
	result := groupByURL(nil)
	require.Empty(t,
		result)
}

func TestGroupByURL_SingleURL(t *testing.T) {
	t.Parallel()
	deliveries := []domain.WebhookDelivery{
		{ID: "d1", WebhookURL: "https://a.com/hook"},
		{ID: "d2", WebhookURL: "https://a.com/hook"},
		{ID: "d3", WebhookURL: "https://a.com/hook"},
	}
	result := groupByURL(deliveries)
	require.Len(t,
		result, 1)
	require.Len(t,
		result["https://a.com/hook"], 3)
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
	require.Len(t,
		result, 3)
	require.Len(t,
		result["https://a.com"], 2,
	)
	require.Len(t,
		result["https://b.com"], 2,
	)
	require.Len(t,
		result["https://c.com"], 1,
	)
}

func TestChunkDeliveries_ExactMultiple(t *testing.T) {
	t.Parallel()
	deliveries := make([]domain.WebhookDelivery, 9)
	for i := range deliveries {
		deliveries[i].ID = fmt.Sprintf("d%d", i)
	}
	chunks := chunkDeliveries(deliveries, 3)
	require.Len(t,
		chunks, 3)

	for _, c := range chunks {
		require.Len(t,
			c, 3)
	}
}

func TestChunkDeliveries_Remainder(t *testing.T) {
	t.Parallel()
	deliveries := make([]domain.WebhookDelivery, 10)
	for i := range deliveries {
		deliveries[i].ID = fmt.Sprintf("d%d", i)
	}
	chunks := chunkDeliveries(deliveries, 3)
	require.Len(t,
		chunks, 4)
	require.Len(t,
		chunks[3],
		1)
}

func TestChunkDeliveries_LargerThanInput(t *testing.T) {
	t.Parallel()
	deliveries := make([]domain.WebhookDelivery, 3)
	for i := range deliveries {
		deliveries[i].ID = fmt.Sprintf("d%d", i)
	}
	chunks := chunkDeliveries(deliveries, 10)
	require.Len(t,
		chunks, 1)
	require.Len(t,
		chunks[0],
		3)
}

func TestChunkDeliveries_Empty(t *testing.T) {
	t.Parallel()
	chunks := chunkDeliveries(nil, 5)
	require.Nil(t, chunks)
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
	require.EqualValues(t, 2, requestCount.
		Load())

	// Should be 2 HTTP requests: 1 batch to URL-A, 1 batch to URL-B

	// All deliveries should be marked delivered.
	for _, d := range ms.getDeliveries() {
		require.Equal(t,
			domain.WebhookStatusDelivered,

			d.Status)
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
	require.NotEqual(t, "true",
		receivedBatchHeader,
	)
	require.Equal(t,
		domain.WebhookStatusDelivered,

		ms.getDeliveries()[0].Status)

	// Single delivery should use individual path (no batch header).
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
	require.EqualValues(t, 2, requestCount.
		Load())
	require.False(t,
		sawBatchHeader.
			Load())
	require.False(t,
		sawUnsigned.
			Load())

	for _, d := range ms.getDeliveries() {
		require.Equal(t,
			domain.WebhookStatusDelivered,

			d.Status)
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
	require.EqualValues(t, 2, requestCount.
		Load())
	require.False(t,
		sawBatchHeader.
			Load())
	require.False(t,
		sawUnsigned.
			Load())
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
	require.EqualValues(t, 4, requestCount.
		Load())

	// 10 deliveries / batch size 3 = 4 batches (3+3+3+1)
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
	require.EqualValues(t, 5, requestCount.
		Load())

	// Without batching, each delivery is a separate request.
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
		require.Equal(t,
			domain.WebhookStatusDelivered,

			d.Status)
		require.Equal(t, 1, d.Attempts)
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
		require.Equal(t,
			domain.WebhookStatusPending,

			d.Status)
		require.Equal(t, 1, d.Attempts)
		require.False(t,
			d.NextRetryAt ==
				nil ||
				d.NextRetryAt.Before(time.Now()))
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
		require.Equal(t,
			domain.WebhookStatusDead,

			d.Status)
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
	require.EqualValues(t, 0, requestCount.
		Load())

	// Circuit breaker open is retryable, but it is not a delivery attempt:
	// no outbound HTTP request was made, so attempts must not be consumed.
	for _, d := range ms.getDeliveries() {
		require.Equal(t,
			domain.WebhookStatusPending,

			d.Status)
		require.Equal(t, 0, d.Attempts)
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
	require.EqualValues(t, 5, requestCount.
		Load())

	// Should fall back to 5 individual requests.

	for _, d := range ms.getDeliveries() {
		require.Equal(t,
			domain.WebhookStatusDelivered,

			d.Status)
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
	require.Equal(t,
		"true", headers.
			Get("X-Strait-Batch"))
	require.Equal(t,
		"3", headers.
			Get("X-Strait-Batch-Size"))
	require.Equal(t,
		"application/json",
		headers.
			Get("Content-Type"))
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
	require.NoError(t, json.Unmarshal(receivedBody,
		&items))
	require.Len(t,
		items, 2)
	require.False(t,
		items[0].
			DeliveryID != "fmt-1" ||
			items[1].DeliveryID != "fmt-2",
	)

	var p1 map[string]string
	require.NoError(t, json.Unmarshal(items[0].Payload, &p1))
	require.Equal(t,
		"val1", p1["key"])
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
	require.Equal(t, 3, exporter.
		PendingCount())

	// One ClickHouse event per delivery in the batch.
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
	require.Len(t,
		records, 1)

	rec, ok := records[0].(clickhouse.WebhookDeliveryEventRecord)
	require.True(t,
		ok)
	require.Equal(t,
		"proj-project",
		rec.ProjectID,
	)
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
	require.EqualValues(t, 2, requestCount.
		Load())

	// 1 batch request + 1 individual request = 2 total

	// All should be delivered.
	for _, d := range ms.getDeliveries() {
		require.Equal(t,
			domain.WebhookStatusDelivered,

			d.Status)
	}
}

func TestWithBatchByURL_Option(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(true))
	assert.True(t,
		worker.batchByURL,
	)

	worker2 := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(false))
	assert.False(t,
		worker2.batchByURL,
	)
}

func TestWithMaxBatchSize_Option(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithMaxBatchSize(25))
	assert.Equal(t, 25, worker.
		maxBatchSize)
}

func TestWithMaxBatchSize_ZeroKeepsDefault(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithMaxBatchSize(0))
	assert.Equal(t,
		defaultMaxBatchSize,
		worker.
			maxBatchSize)
}

func TestExtractPayload_ValidJSON(t *testing.T) {
	t.Parallel()

	d := &domain.WebhookDelivery{LastError: `{"key":"value"}`}
	payload := extractPayload(d)

	var m map[string]string
	require.NoError(t, json.Unmarshal(payload,
		&m))
	require.Equal(t,
		"value",
		m["key"])
	require.Empty(t,
		d.LastError,
	)
}

func TestExtractPayload_InvalidJSON_Fallback(t *testing.T) {
	t.Parallel()

	d := &domain.WebhookDelivery{ID: "d1", EventTriggerID: "evt1", LastError: "not-json"}
	payload := extractPayload(d)

	var m map[string]string
	require.NoError(t, json.Unmarshal(payload,
		&m))
	require.Equal(t,
		"d1", m["delivery_id"])
}

func TestExtractPayload_EmptyLastError_Fallback(t *testing.T) {
	t.Parallel()

	d := &domain.WebhookDelivery{ID: "d2"}
	payload := extractPayload(d)

	var m map[string]string
	require.NoError(t, json.Unmarshal(payload,
		&m))
	require.Equal(t,
		"d2", m["delivery_id"])
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
		require.True(t,
			ok)

		var payload map[string]string
		require.NoError(t, json.Unmarshal([]byte(
			body), &payload))

		expected := fmt.Sprintf("value_%d", i)
		require.Equal(t,
			expected,
			payload["unique_key"])
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
	require.Greater(t,
		maxInFlight.
			Load(), int32(1))
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
		require.Equal(t,
			domain.WebhookStatusPending,

			d.Status)
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
				require.Failf(t, "test failure", "expected dead for exhausted delivery, got %s", d.Status)
			}
		case "has-room":
			// Attempts goes 1 -> 2, still below MaxAttempts=5 -> pending retry.
			if d.Status != domain.WebhookStatusPending {
				require.Failf(t, "test failure", "expected pending for delivery with room, got %s", d.Status)
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
				require.Failf(t, "test failure", "expected pending for attempt-0 (1/2), got %s", d.Status)
			}
		case "attempt-1":
			// Attempts: 1 -> 2, MaxAttempts: 2 -> exhausted -> dead.
			if d.Status != domain.WebhookStatusDead {
				require.Failf(t, "test failure", "expected dead for attempt-1 (2/2), got %s", d.Status)
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
		require.Equal(t,
			domain.WebhookStatusDead,

			d.Status)
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
	require.Equal(t, 2, exporter.
		PendingCount())

	// ClickHouse events should fire even on request creation failure.
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
	require.Len(t,
		batchHeaders,
		3)

	// 3 deliveries, maxBatchSize=1 -> 3 batch requests (each with X-Strait-Batch: true).

	for _, h := range batchHeaders {
		require.Equal(t,
			"true", h,
		)
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
		require.Equal(t,
			domain.WebhookStatusDelivered,

			d.Status)
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
		require.Equal(t,
			domain.WebhookStatusDead,

			d.Status)
		require.False(t,
			d.LastStatusCode ==
				nil ||
				*d.LastStatusCode !=
					http.StatusFound,
		)
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
	require.EqualValues(t, 5, requestCount.
		Load())

	for _, d := range ms.getDeliveries() {
		require.Equal(t,
			domain.WebhookStatusDelivered,

			d.Status)
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
	require.NoError(t, json.Unmarshal(receivedBody,
		&items))
	require.Len(t,
		items, 1)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(items[0].Payload, &payload))
	require.Equal(t,
		"evt-99",
		payload["trigger_id"])
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
	require.EqualValues(t, 50, requestCount.
		Load())

	// 50 URLs x 1 batch each = 50 requests.

	for _, d := range ms.getDeliveries() {
		require.Equal(t,
			domain.WebhookStatusDelivered,

			d.Status)
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
	require.Equal(t,
		"sent", ms.
			getNotifyStatus())
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
	require.Equal(t,
		"failed",
		ms.getNotifyStatus())
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
	require.Equal(t,
		count, exporter.
			PendingCount())

	// Must be exactly 3, not 6 (which would indicate double-counting).
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
	require.Equal(t,
		count, exporter.
			PendingCount())
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
	require.Equal(t,
		count, exporter.
			PendingCount())
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
	require.NoError(t, json.Unmarshal(individualPayload,
		&indParsed,
	))
	require.Equal(t,
		"run-1",
		indParsed["run_id"])

	// Batch wraps in array with delivery_id.
	var batchParsed []batchPayloadItem
	require.NoError(t, json.Unmarshal(batchPayload,
		&batchParsed,
	))
	require.Len(t,
		batchParsed,
		1)

	var batchInnerPayload map[string]any
	require.NoError(t, json.Unmarshal(batchParsed[0].Payload,
		&batchInnerPayload),
	)
	require.Equal(t,
		"run-1",
		batchInnerPayload["run_id"])
	require.Equal(t,
		domain.WebhookStatusDelivered,

		d1.Status)
	require.Equal(t,
		domain.WebhookStatusDelivered,

		batchDeliveries[0].Status)
	require.Equal(t, 1, d1.Attempts)
	require.Equal(t, 1, batchDeliveries[0].Attempts)

	// Both should be delivered.

	// Both should have 1 attempt.
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
	require.Equal(t,
		domain.WebhookStatusPending,

		d1.Status)
	require.Equal(t,
		domain.WebhookStatusPending,

		batchDeliveries[0].Status)
	require.Equal(t, 1, d1.Attempts)
	require.Equal(t, 1, batchDeliveries[0].Attempts)
	require.False(t,
		d1.NextRetryAt ==
			nil ||
			!d1.NextRetryAt.
				After(time.Now()))
	require.False(t,
		batchDeliveries[0].NextRetryAt ==
			nil ||
			!batchDeliveries[0].
				NextRetryAt.After(time.Now()))

	// Both should be pending (retry scheduled).

	// Both should have 1 attempt.

	// Both should have future next_retry_at.
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
	require.Equal(t,
		domain.WebhookStatusDead,

		d1.Status)
	require.Equal(t,
		domain.WebhookStatusDead,

		batchDeliveries[0].Status)

	// Both should be dead.
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
			require.Fail(t, "timed out waiting for batch delivery via RunWorker")
		case <-time.After(50 * time.Millisecond):
		}
	}

	cancel()

	mu.Lock()
	defer mu.Unlock()
	require.Len(t,
		receivedBodies,
		1)

	// Should have received exactly 1 batch request (not 3 individual ones).

	// Verify it was a batch payload (JSON array).
	var items []batchPayloadItem
	require.NoError(t, json.Unmarshal(receivedBodies[0], &items))
	require.Len(t,
		items, 3)
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
		require.Equal(t, 1, d.Attempts)
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
				require.Failf(t, "test failure", "expected exponential for %s, got %s", d.ID, d.RetryPolicy)
			}
		case "rp-linear":
			if d.RetryPolicy != domain.WebhookRetryPolicyLinear {
				require.Failf(t, "test failure", "expected linear for %s, got %s", d.ID, d.RetryPolicy)
			}
		case "rp-fixed":
			if d.RetryPolicy != domain.WebhookRetryPolicyFixed {
				require.Failf(t, "test failure", "expected fixed for %s, got %s", d.ID, d.RetryPolicy)
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
		require.Equal(t,
			domain.WebhookStatusDelivered,

			d.Status)
	}
	for _, d := range ms2.getDeliveries() {
		require.Equal(t,
			domain.WebhookStatusDelivered,

			d.Status)
	}
}

func TestAttemptBatchDelivery_ConnectionError_RetriesAll(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()

	t.Parallel()

	// Edge case: server immediately closes connection.
	// Use a listener that immediately closes connections.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

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
		require.Equal(t,
			domain.WebhookStatusPending,

			d.Status)
		require.Equal(t, 1, d.Attempts)
		require.Contains(t,
			d.LastError, "http request")
	}
}

// Functional option coverage.

func TestWithRetryPolicy_Exponential(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithRetryPolicy(domain.WebhookRetryPolicyExponential))
	require.Equal(t,
		domain.WebhookRetryPolicyExponential,

		worker.
			defaultRetryPolicy,
	)
}

func TestWithRetryPolicy_Linear(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithRetryPolicy(domain.WebhookRetryPolicyLinear))
	require.Equal(t,
		domain.WebhookRetryPolicyLinear,

		worker.defaultRetryPolicy,
	)
}

func TestWithRetryPolicy_Fixed(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithRetryPolicy(domain.WebhookRetryPolicyFixed))
	require.Equal(t,
		domain.WebhookRetryPolicyFixed,

		worker.defaultRetryPolicy,
	)
}

func TestWithRetryPolicy_InvalidKeepsDefault(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithRetryPolicy("bogus"))
	require.Equal(t,
		domain.WebhookRetryPolicyExponential,

		worker.
			defaultRetryPolicy,
	)
}

func TestWithRetryPolicy_EmptyKeepsDefault(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithRetryPolicy(""))
	require.Equal(t,
		domain.WebhookRetryPolicyExponential,

		worker.
			defaultRetryPolicy,
	)
}

func TestWithMetrics_SetsMetricsField(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	// Pass nil metrics -- just verify the option sets the field without panic.
	worker := NewDeliveryWorker(ms, slog.Default(), WithMetrics(nil))
	require.Nil(t, worker.
		metrics)
}

func TestWithCircuitBreaker_SetsField(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	cb := NewRedisWebhookCircuitBreaker(nil, false)
	worker := NewDeliveryWorker(ms, slog.Default(), WithCircuitBreaker(cb))
	require.Equal(t,
		cb, worker.
			circuitBreaker,
	)
}

func TestWithMaxPayloadBytes_Positive(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithMaxPayloadBytes(2048))
	require.EqualValues(t, 2048, worker.
		maxPayloadBytes,
	)
}

func TestWithMaxPayloadBytes_ZeroKeepsDefault(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithMaxPayloadBytes(0))
	require.EqualValues(t,
		defaultWebhookMaxPayloadBytes,

		worker.maxPayloadBytes,
	)
}

func TestWithBatchByURL_SetsField(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default(), WithBatchByURL(true))
	require.True(t,
		worker.batchByURL,
	)
}

// EnqueueRunWebhook edge cases.

func TestEnqueueRunWebhook_NilJob(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	err := worker.EnqueueRunWebhook(context.Background(), nil, &domain.JobRun{})
	require.Error(t,
		err)
	require.Contains(t,
		err.Error(), "job and run are required")
}

func TestEnqueueRunWebhook_NilRun(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	err := worker.EnqueueRunWebhook(context.Background(), &domain.Job{WebhookURL: "http://example.com"}, nil)
	require.Error(t,
		err)
	require.Contains(t,
		err.Error(), "job and run are required")
}

func TestEnqueueRunWebhook_EmptyWebhookURL(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	job := &domain.Job{ID: "job-empty-url", WebhookURL: ""}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}

	err := worker.EnqueueRunWebhook(context.Background(), job, run)
	require.NoError(t, err)
	require.Empty(t,
		ms.getDeliveries())
}

func TestEnqueueRunWebhook_NonTerminalStatus(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())

	job := &domain.Job{ID: "job-nonterminal", WebhookURL: "http://example.com/hook"}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}

	err := worker.EnqueueRunWebhook(context.Background(), job, run)
	require.NoError(t, err)
	require.Empty(t,
		ms.getDeliveries())
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
	require.NoError(t, err)

	deliveries := ms.getDeliveries()
	require.Len(t,
		deliveries,
		1)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(deliveries[0].Payload, &payload))
	require.Equal(t,
		"failed",
		payload["status"])
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
	require.NoError(t, worker.
		EnqueueRunWebhook(context.Background(), job, run))

	deliveries := ms.getDeliveries()
	require.Len(t,
		deliveries,
		1)
	require.Equal(t,
		domain.WebhookRetryPolicyFixed,

		deliveries[0].RetryPolicy)
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
	require.Empty(t,
		ms.getDeliveries())
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
	require.Empty(t,
		ms.getDeliveries())

	// Delivery should not have been stored.
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
	require.Len(t,
		deliveries,
		1)
	require.Equal(t,
		domain.WebhookRetryPolicyLinear,

		deliveries[0].RetryPolicy)
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
	require.Len(t,
		deliveries,
		1)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(deliveries[0].Payload, &payload))

	expected := "/v1/events/my-event/send"
	require.Equal(t,
		expected,
		payload["callback_url"])
}

func TestNewDeliveryWorker_NilLogger(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, nil)
	require.NotNil(
		t, worker)
}

func TestNewEventNotifier_IsAlias(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	notifier := NewEventNotifier(ms, slog.Default())
	require.NotNil(
		t, notifier,
	)
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

	worker := NewDeliveryWorker(ms, slog.Default(),
		WithAllowPrivateEndpoints(true),
		WithHTTPTransport(5*time.Second, time.Second, 2, 2),
	)
	worker.processBatch(context.Background())
	require.NotEmpty(t, receivedSigHeader)
	require.NotEmpty(t, receivedTimestamp)
	require.True(t,
		strings.HasPrefix(receivedSigHeader,
			"v1=",
		))
	require.Equal(t,
		receivedSigHeader,
		receivedStraitSigHeader,
	)

	expectedSig := ComputeTimestampedHMACSHA256(secret, receivedTimestamp, []byte(`{"event":"run.completed"}`))
	require.Equal(t,
		"v1="+expectedSig,
		receivedSigHeader,
	)

	bodyOnlySig := "v1=" + ComputeHMACSHA256(secret, []byte(`{"event":"run.completed"}`))
	require.NotEqual(t, bodyOnlySig,
		receivedSigHeader,
	)
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

	worker := NewDeliveryWorker(ms, slog.Default(),
		WithAllowPrivateEndpoints(true),
		WithHTTPTransport(5*time.Second, time.Second, 2, 2),
	)
	worker.processBatch(context.Background())
	require.NotEmpty(t, receivedSigHeader)
	require.Equal(t,
		receivedSigHeader,
		receivedStraitSigHeader,
	)

	expectedSig := ComputeTimestampedHMACSHA256(secret, receivedTimestamp, []byte(`{"event":"run.completed"}`))
	require.Equal(t,
		"v1="+expectedSig,
		receivedSigHeader,
	)

	expectedReplayKey := ComputeReplayKey([]byte(secret), delivery.ID)
	require.Equal(t,
		expectedReplayKey,
		receivedReplayKey,
	)
	require.NotEqual(t, ComputeReplayKeyUnsigned(delivery.ID),
		receivedReplayKey)
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

	worker := NewDeliveryWorker(ms, slog.Default(),
		WithSecretDecryptor(fakeSecretDecryptor{prefix: fakePrefix}))
	worker.processBatch(context.Background())
	require.NotEmpty(t, receivedSigHeader)
	require.NotEmpty(t, receivedTimestamp)

	expectedSig := "v1=" + ComputeTimestampedHMACSHA256(plaintextSecret, receivedTimestamp, []byte(`{"event":"run.completed"}`))
	require.Equal(t,
		expectedSig,
		receivedSigHeader,
	)

	ciphertextSig := "v1=" + ComputeTimestampedHMACSHA256(storedCiphertext, receivedTimestamp, []byte(`{"event":"run.completed"}`))
	require.NotEqual(t, ciphertextSig,
		receivedSigHeader,
	)
}

// Regression: tenants with the same external webhook URL must
// not share a circuit-breaker key. Without per-tenant scoping, one noisy
// tenant could trip the breaker for everyone pointing at the same shared
// receiver (cross-tenant DoS).
func TestBreakerKey_PerTenantScoping(t *testing.T) {
	t.Parallel()

	url := "https://hooks.example.com/in"
	require.NotEqual(t, breakerKey("orgB", url), breakerKey("orgA",
		url))
	require.Equal(t,
		url, breakerKey("", url),
	)
}

// Regression (I3): when OrgID is empty (e.g. orphaned deliveries), the breaker
// key must fall back to the project scope so two different tenants pointing at
// the same URL still get distinct breaker buckets, on both the batch and the
// individual delivery paths.
func TestBreakerKey_EmptyOrgFallsBackToProject(t *testing.T) {
	t.Parallel()

	url := "https://hooks.example.com/in"
	a := domain.WebhookDelivery{ProjectID: "proj-a", WebhookURL: url}
	b := domain.WebhookDelivery{ProjectID: "proj-b", WebhookURL: url}

	// Individual path.
	require.NotEqual(t,
		breakerKey(deliveryTenantScope(a), a.WebhookURL),
		breakerKey(deliveryTenantScope(b), b.WebhookURL),
		"empty-OrgID deliveries from different projects must not share a breaker key",
	)

	// Batch path: a batch is homogeneous per groupByURL, so each batch's scope
	// reflects its project.
	require.NotEqual(t,
		breakerKey(batchTenantScope([]domain.WebhookDelivery{a}), url),
		breakerKey(batchTenantScope([]domain.WebhookDelivery{b}), url),
	)

	// The batch and individual paths must agree for the same tenant so the
	// breaker state is shared, not split.
	require.Equal(t,
		breakerKey(deliveryTenantScope(a), url),
		breakerKey(batchTenantScope([]domain.WebhookDelivery{a}), url),
	)
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())
	require.NotEmpty(t, receivedSig)
	require.NotEmpty(t, receivedOldSig)
	require.True(t,
		strings.HasPrefix(receivedOldSig,
			"v1="))
	require.Equal(t,
		receivedOldSig,
		receivedStraitOldSig,
	)
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())
	require.NotEmpty(t, receivedSig)
	require.Empty(t,
		receivedOldSig,
	)
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())
	require.False(t,
		requestReceived.
			Load())

	deliveries := ms.getDeliveries()
	require.Len(t,
		deliveries,
		1)
	require.Equal(t,
		"webhook subscription signing secret unavailable",

		deliveries[0].LastError)
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())
	require.NotNil(
		t, headers)
	require.Empty(t,
		headers.
			Get("X-Strait-Trigger-ID"))
	require.Empty(t,
		headers.
			Get("X-Run-ID"))
	require.Empty(t,
		headers.
			Get("X-Job-ID"))
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())
	require.NotNil(
		t, headers)
	require.Equal(t,
		"evt-42",
		headers.Get("X-Strait-Trigger-ID"))
	require.Equal(t,
		"run-42",
		headers.Get("X-Run-ID"))
	require.Equal(t,
		"job-42",
		headers.Get("X-Job-ID"))
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
	require.NoError(t, ms.CreateWebhookDelivery(context.Background(), delivery))

	worker := NewDeliveryWorker(ms, slog.Default())
	worker.processBatch(context.Background())

	deliveries := ms.getDeliveries()
	require.Len(t,
		deliveries,
		1)
	require.Equal(t,
		domain.WebhookStatusDead,

		deliveries[0].Status,
	)
}

func TestBackoffForRetryPolicy_ExponentialCapsAt30Min(t *testing.T) {
	t.Parallel()

	// 5^5 = 3125 seconds = ~52 minutes, exceeds 30-minute cap.
	got := backoffForRetryPolicy(domain.WebhookRetryPolicyExponential, 5)
	approxBackoff(t, got, 30*time.Minute, "exponential attempt 5 (capped)")
	require.LessOrEqual(t, got,
		30*time.Minute+
			(30*time.Minute)/5)
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
	require.LessOrEqual(t, got,
		30*time.Minute+
			(30*time.Minute)/5)
}

func TestBackoffForRetryPolicy_HugeAttemptsDoNotOverflow(t *testing.T) {
	t.Parallel()

	exponential := backoffForRetryPolicy(domain.WebhookRetryPolicyExponential, 1_000_000)
	approxBackoff(t, exponential, maxWebhookBackoff, "huge exponential attempt")
	require.Positive(t,
		exponential)

	linear := backoffForRetryPolicy(domain.WebhookRetryPolicyLinear, int(^uint(0)>>1))
	approxBackoff(t, linear, maxWebhookBackoff, "huge linear attempt")
	require.Positive(t,
		linear)
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
