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
	"github.com/stretchr/testify/require"
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
	require.NoError(t, testStore.
		CreateWebhookDelivery(context.Background(),
			delivery))

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
			require.Fail(t, "timed out waiting for webhook delivery")
		case <-time.After(20 * time.Millisecond):
		}
	}

	deliveries, err := testStore.ListWebhookDeliveries(context.Background(), projectID, domain.WebhookStatusDelivered, 10, nil)
	require.NoError(t, err)
	require.Len(t, deliveries,

		1)

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
	require.NotEqual(t, 0,

		breaker.
			calls.Load())
	require.NotEqual(t, 0,

		store.updates.
			Load())

	d := store.deliveries[0]
	require.Equal(t, domain.
		WebhookStatusPending,

		d.Status,
	)
	require.EqualValues(t, 0, d.
		Attempts)

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
		require.NoError(t, testStore.
			UpdateRunStatus(context.
				Background(), runID,
				domain.StatusQueued,
				domain.StatusDequeued,
				nil,
			))
		require.NoError(t, testStore.
			UpdateRunStatus(context.
				Background(), runID,
				domain.StatusDequeued,
				domain.StatusExecuting,

				nil))
		require.NoError(t, testStore.
			UpdateRunStatus(context.
				Background(), runID,
				domain.StatusExecuting,
				domain.StatusDeadLetter,

				nil))

	}

	body := fmt.Sprintf(`{"run_ids":["%s","%s"]}`, runIDs[0], runIDs[1])
	w := doRequest(t, http.MethodPost, "/v1/runs/bulk-dlq-replay", body)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	for _, runID := range runIDs {
		run, err := testStore.GetRun(context.Background(), runID)
		require.NoError(t, err)
		require.Equal(t, domain.
			StatusQueued,
			run.
				Status)
		require.EqualValues(t, 1, run.
			Attempt,
		)

	}
}

func TestE2E_APIResponsesIncludeSecurityHeaders(t *testing.T) {
	w := doRequest(t, http.MethodGet, "/health", "")
	require.Equal(t, http.
		StatusOK,
		w.Code)
	require.Equal(t, "nosniff",

		w.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "DENY",

		w.Header().Get("X-Frame-Options"))
	require.Equal(t, "default-src 'none'",

		w.Header().
			Get("Content-Security-Policy"))
	require.Equal(t, "no-referrer",

		w.Header().
			Get("Referrer-Policy"))

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
