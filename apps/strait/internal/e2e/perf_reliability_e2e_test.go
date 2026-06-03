//go:build integration

package e2e_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/webhook"

	"github.com/sourcegraph/conc"
)

func TestE2E_WebhookDeliveryWorker_ProcessesPendingDeliveries(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	mustClean(t)

	projectID := "proj-webhook-worker-" + newID()
	job := createJob(t, projectID, "Webhook Worker", "webhook-worker-"+newID())
	jobID := asString(t, job, "id")
	run := triggerJob(t, jobID, `{"payload":{"webhook":true}}`, "")
	runID := asString(t, run, "id")

	var requests atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	now := time.Now().UTC().Add(-time.Second)
	delivery := &domain.WebhookDelivery{
		RunID:       runID,
		JobID:       jobID,
		WebhookURL:  ts.URL,
		RetryPolicy: domain.WebhookRetryPolicyExponential,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   `{"run_id":"` + runID + `"}`,
	}
	if err := testStore.CreateWebhookDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("create webhook delivery: %v", err)
	}

	worker := webhook.NewDeliveryWorker(
		testStore,
		slog.Default(),
		webhook.WithAllowPrivateEndpoints(true),
		webhook.WithHTTPTransport(2*time.Second, 30*time.Second, 10, 10),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	concWG.Go(func() {
		_ = worker.RunWorker(ctx, 50*time.Millisecond)
	})

	deadline := time.After(2 * time.Second)
	for requests.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for webhook delivery")
		case <-time.After(20 * time.Millisecond):
		}
	}

	deliveries, err := testStore.ListWebhookDeliveries(context.Background(), projectID, domain.WebhookStatusDelivered, 10, nil)
	if err != nil {
		t.Fatalf("list delivered webhooks: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivered webhook, got %d", len(deliveries))
	}
}

func TestE2E_PriorityAgingAffectsDequeueOrder(t *testing.T) {
	// The dequeue-time aging ORDER BY was replaced with a
	// scheduler.PriorityPromoter goroutine that bumps priority on aged
	// queued runs, so this test's premise is invalid. Promotion is exercised in
	// scheduler/priority_promoter_integration_test.go instead.
	t.Skip("superseded by scheduler.PriorityPromoter; see priority_promoter_integration_test.go")
}

func TestE2E_WebhookCircuitBreakerBlocksDelivery(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	store := &mockDeliveryStoreE2E{deliveries: []domain.WebhookDelivery{newPendingDelivery("blocked", "http://example.com/hook")}}
	breaker := &alwaysOpenBreaker{}
	worker := webhook.NewDeliveryWorker(store, slog.Default(), webhook.WithCircuitBreaker(breaker))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	concWG.Go(func() {
		defer close(done)
		_ = worker.RunWorker(ctx, 20*time.Millisecond)
	})

	<-done
	if breaker.calls.Load() == 0 {
		t.Fatal("expected circuit breaker to be consulted")
	}
	if store.updates.Load() == 0 {
		t.Fatal("expected delivery update after circuit breaker block")
	}

	d := store.deliveries[0]
	if d.Status != domain.WebhookStatusPending {
		t.Fatalf("delivery status = %s, want pending", d.Status)
	}
	if d.Attempts != 0 {
		t.Fatalf("expected circuit breaker block not to consume an attempt, got %d", d.Attempts)
	}
}

func TestE2E_BulkDLQReplay_RequeuesRuns(t *testing.T) {
	mustClean(t)

	projectID := "proj-bulk-dlq-" + newID()
	job := createJob(t, projectID, "Bulk DLQ", "bulk-dlq-"+newID())
	jobID := asString(t, job, "id")

	first := triggerJob(t, jobID, `{"payload":{"n":1}}`, "")
	second := triggerJob(t, jobID, `{"payload":{"n":2}}`, "")

	runIDs := []string{asString(t, first, "id"), asString(t, second, "id")}
	for _, runID := range runIDs {
		if err := testStore.UpdateRunStatus(context.Background(), runID, domain.StatusQueued, domain.StatusDequeued, nil); err != nil {
			t.Fatalf("queued->dequeued %s: %v", runID, err)
		}
		if err := testStore.UpdateRunStatus(context.Background(), runID, domain.StatusDequeued, domain.StatusExecuting, nil); err != nil {
			t.Fatalf("dequeued->executing %s: %v", runID, err)
		}
		if err := testStore.UpdateRunStatus(context.Background(), runID, domain.StatusExecuting, domain.StatusDeadLetter, nil); err != nil {
			t.Fatalf("executing->dead_letter %s: %v", runID, err)
		}
	}

	body := fmt.Sprintf(`{"run_ids":["%s","%s"]}`, runIDs[0], runIDs[1])
	w := doRequest(t, http.MethodPost, "/v1/runs/bulk-dlq-replay", body)
	if w.Code != http.StatusOK {
		t.Fatalf("bulk dlq replay status = %d, body = %s", w.Code, w.Body.String())
	}

	for _, runID := range runIDs {
		run, err := testStore.GetRun(context.Background(), runID)
		if err != nil {
			t.Fatalf("get replayed run %s: %v", runID, err)
		}
		if run.Status != domain.StatusQueued {
			t.Fatalf("run %s status = %s, want queued", runID, run.Status)
		}
		if run.Attempt != 1 {
			t.Fatalf("run %s attempt = %d, want 1", runID, run.Attempt)
		}
	}
}

func TestE2E_APIResponsesIncludeSecurityHeaders(t *testing.T) {
	w := doRequest(t, http.MethodGet, "/health", "")
	if w.Code != http.StatusOK {
		t.Fatalf("health status = %d", w.Code)
	}

	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want DENY", got)
	}
	if got := w.Header().Get("Content-Security-Policy"); got != "default-src 'none'" {
		t.Fatalf("Content-Security-Policy = %q, want default-src 'none'", got)
	}
	if got := w.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q, want no-referrer", got)
	}
}

type alwaysOpenBreaker struct {
	calls atomic.Int32
}

func (b *alwaysOpenBreaker) CanDeliver(context.Context, string) (bool, error) {
	b.calls.Add(1)
	return false, nil
}

func (*alwaysOpenBreaker) RecordSuccess(context.Context, string) {}
func (*alwaysOpenBreaker) RecordFailure(context.Context, string) {}

type mockDeliveryStoreE2E struct {
	deliveries []domain.WebhookDelivery
	updates    atomic.Int32
}

func (m *mockDeliveryStoreE2E) CreateWebhookDelivery(context.Context, *domain.WebhookDelivery) error {
	return nil
}

func (m *mockDeliveryStoreE2E) UpdateWebhookDelivery(_ context.Context, d *domain.WebhookDelivery) error {
	m.updates.Add(1)
	m.deliveries[0] = *d
	return nil
}

func (m *mockDeliveryStoreE2E) ListPendingWebhookRetries(context.Context) ([]domain.WebhookDelivery, error) {
	var pending []domain.WebhookDelivery
	for _, d := range m.deliveries {
		if d.Status == domain.WebhookStatusPending {
			pending = append(pending, d)
		}
	}
	return pending, nil
}

func (m *mockDeliveryStoreE2E) UpdateEventTriggerNotifyStatus(context.Context, string, string) error {
	return nil
}

func (m *mockDeliveryStoreE2E) GetWebhookSubscriptionSecrets(context.Context, string) (string, string, *time.Time, error) {
	return "", "", nil, nil
}

func newPendingDelivery(id, webhookURL string) domain.WebhookDelivery {
	nextRetryAt := time.Now().UTC().Add(-time.Second)
	return domain.WebhookDelivery{
		ID:          id,
		WebhookURL:  webhookURL,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 100,
		NextRetryAt: &nextRetryAt,
		LastError:   `{"id":"` + id + `"}`,
	}
}
