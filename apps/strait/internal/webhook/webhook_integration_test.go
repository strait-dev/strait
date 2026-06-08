//go:build integration

package webhook_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"
	"strait/internal/webhook"

	"github.com/google/uuid"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testDB *testutil.TestDB

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	testDB, err = testutil.SetupSharedTestDB(ctx, "../../migrations", "webhook")
	if err != nil {
		log.Fatalf("setup test db: %v", err)
	}

	code := m.Run()
	testDB.Cleanup(ctx)
	os.Exit(code)
}

func mustStore(t *testing.T) *store.Queries {
	t.Helper()
	require.False(t, testDB ==

		nil || testDB.
		Pool ==
		nil)

	return store.New(testDB.Pool)
}

func mustClean(t *testing.T, ctx context.Context) {
	t.Helper()
	require.NoError(t, testDB.
		CleanTables(ctx))

}

func newID() string {
	return uuid.Must(uuid.NewV7()).String()
}

func createPendingWebhookDelivery(t *testing.T, ctx context.Context, st *store.Queries, webhookURL string) *domain.WebhookDelivery {
	t.Helper()
	now := time.Now()
	d := &domain.WebhookDelivery{
		WebhookURL:  webhookURL,
		RetryPolicy: domain.WebhookRetryPolicyExponential,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 3,
		NextRetryAt: &now,
		LastError:   `{"integration":"private-endpoint"}`,
	}
	require.NoError(t, st.CreateWebhookDelivery(ctx,
		d))
	stampDeliveryProject(t, ctx, d)

	return d
}

// stampDeliveryProject sets a project on a bare test delivery. These deliveries
// have no run/job/subscription, so project_id would derive to NULL and the now
// project-scoped GetWebhookDelivery could not fetch them. project_id is a plain
// TEXT column (no FK), so any value works.
func stampDeliveryProject(t *testing.T, ctx context.Context, d *domain.WebhookDelivery) {
	t.Helper()
	d.ProjectID = "webhook-itest-proj"
	_, err := testDB.Pool.Exec(ctx,
		`UPDATE webhook_deliveries SET project_id = $1 WHERE id = $2`, d.ProjectID, d.ID)
	require.NoError(t, err)
}

func runWebhookWorkerUntilDelivered(t *testing.T, ctx context.Context, worker *webhook.DeliveryWorker, st *store.Queries, projectID, deliveryID string) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Helper()
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	concWG.Go(func() {
		_ = worker.RunWorker(workerCtx, 50*time.Millisecond)
	})
	t.Cleanup(func() {
		cancel()
		_ = worker.Shutdown(context.Background())
	})

	deadline := time.After(5 * time.Second)
	for {
		got, err := st.GetWebhookDelivery(ctx, projectID, deliveryID)
		require.NoError(t, err)

		if got.Status == domain.WebhookStatusDelivered {
			return
		}
		select {
		case <-deadline:
			require.Failf(t, "test failure", "timed out waiting for delivery %s, status = %q, last_error = %q", deliveryID, got.Status, got.LastError)
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestDeliveryWorker_AllowPrivateEndpointsWorksWithoutHTTPTransportOption(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	var received atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := createPendingWebhookDelivery(t, ctx, st, srv.URL)
	worker := webhook.NewDeliveryWorker(st, slog.Default(),
		webhook.WithAllowPrivateEndpoints(true),
		webhook.WithConcurrency(1),
	)
	runWebhookWorkerUntilDelivered(t, ctx, worker, st, d.ProjectID, d.ID)
	require.EqualValues(t, 1, received.
		Load(),
	)

}

func TestDeliveryWorker_AllowPrivateEndpointsOrderInsensitiveWithHTTPTransport(t *testing.T) {
	tests := []struct {
		name string
		opts []webhook.DeliveryWorkerOption
	}{
		{
			name: "allow before transport",
			opts: []webhook.DeliveryWorkerOption{
				webhook.WithAllowPrivateEndpoints(true),
				webhook.WithHTTPTransport(2*time.Second, time.Second, 2, 2),
				webhook.WithConcurrency(1),
			},
		},
		{
			name: "allow after transport",
			opts: []webhook.DeliveryWorkerOption{
				webhook.WithHTTPTransport(2*time.Second, time.Second, 2, 2),
				webhook.WithAllowPrivateEndpoints(true),
				webhook.WithConcurrency(1),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st := mustStore(t)
			mustClean(t, ctx)

			var received atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				received.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			d := createPendingWebhookDelivery(t, ctx, st, srv.URL)
			worker := webhook.NewDeliveryWorker(st, slog.Default(), tt.opts...)
			runWebhookWorkerUntilDelivered(t, ctx, worker, st, d.ProjectID, d.ID)
			require.EqualValues(t, 1, received.
				Load(),
			)

		})
	}
}

// TestEndToEndWebhookDelivery creates a delivery in Postgres, runs the worker,
// and verifies the HTTP request arrives at an httptest.Server with correct headers and payload.
func TestEndToEndWebhookDelivery(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	var received atomic.Int32
	var receivedBody []byte
	var receivedHeaders http.Header
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		receivedHeaders = r.Header.Clone()
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	payload := `{"event_key":"test.event","trigger_id":"trig-1"}`
	now := time.Now()
	d := &domain.WebhookDelivery{
		WebhookURL:  srv.URL,
		RetryPolicy: domain.WebhookRetryPolicyExponential,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   payload,
	}
	require.NoError(t, st.CreateWebhookDelivery(ctx,
		d))
	stampDeliveryProject(t, ctx, d)
	require.NotEqual(t, "", d.
		ID)

	worker := webhook.NewDeliveryWorker(st, slog.Default(), webhook.WithConcurrency(2))
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	concWG.Go(func() {
		_ = worker.RunWorker(workerCtx, 100*time.Millisecond)
	})

	deadline := time.After(5 * time.Second)
	for received.Load() == 0 {
		select {
		case <-deadline:
			require.Fail(t, "timed out waiting for webhook delivery")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
	require.EqualValues(t, 1, received.
		Load(),
	)

	mu.Lock()
	require.Equal(t, "application/json",

		receivedHeaders.
			Get("Content-Type"))
	require.Equal(t, d.ID, receivedHeaders.
		Get("X-Strait-Delivery-ID"))

	var bodyMap map[string]any
	require.NoError(t, json.Unmarshal(receivedBody,

		&bodyMap))
	require.Equal(t, "test.event",

		bodyMap["event_key"])

	mu.Unlock()

	// Wait for the worker to persist the "delivered" status in the DB
	// before canceling the context. The HTTP handler counts on receipt,
	// but the DB update happens asynchronously after the response.
	dbDeadline := time.After(5 * time.Second)
	for {
		got, err := st.GetWebhookDelivery(ctx, d.ProjectID, d.ID)
		require.NoError(t, err)

		if got.Status == domain.WebhookStatusDelivered {
			break
		}
		select {
		case <-dbDeadline:
			require.Failf(t, "test failure", "timed out waiting for DB status update, status = %q", got.Status)
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	cancel()
	_ = worker.Shutdown(context.Background())

	got, err := st.GetWebhookDelivery(ctx, d.ProjectID, d.ID)
	require.NoError(t, err)
	require.Equal(t, domain.WebhookStatusDelivered,

		got.Status)
	require.NotNil(t, got.DeliveredAt)
	require.EqualValues(t, 1, got.Attempts)

}

// TestRetryFlowWithRealPersistence verifies that a failed delivery is retried
// and eventually succeeds, with state persisted across polls.
func TestRetryFlowWithRealPersistence(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count := callCount.Add(1)
		if count <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	payload := `{"retry":"test"}`
	now := time.Now()
	d := &domain.WebhookDelivery{
		WebhookURL:  srv.URL,
		RetryPolicy: domain.WebhookRetryPolicyFixed,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   payload,
	}
	require.NoError(t, st.CreateWebhookDelivery(ctx,
		d))
	stampDeliveryProject(t, ctx, d)

	worker := webhook.NewDeliveryWorker(st, slog.Default(),
		webhook.WithConcurrency(1),
		webhook.WithRetryPolicy(domain.WebhookRetryPolicyFixed),
	)

	// Manually poll so we can control timing and update next_retry_at
	// to bypass the backoff delay.
	for range 5 {
		workerCtx, cancel := context.WithCancel(ctx)
		concWG.Go(func() {
			_ = worker.RunWorker(workerCtx, 100*time.Millisecond)
		})
		time.Sleep(300 * time.Millisecond)
		cancel()
		_ = worker.Shutdown(context.Background())

		got, err := st.GetWebhookDelivery(ctx, d.ProjectID, d.ID)
		require.NoError(t, err)

		if got.Status == domain.WebhookStatusDelivered {
			break
		}

		// Reset next_retry_at so the worker picks it up again on next poll.
		if got.Status == domain.WebhookStatusPending && got.NextRetryAt != nil {
			resetTime := time.Now().Add(-1 * time.Second)
			got.NextRetryAt = &resetTime
			require.NoError(t, st.UpdateWebhookDelivery(ctx,
				got))

		}

		// Need a fresh worker since the old one has been shut down.
		worker = webhook.NewDeliveryWorker(st, slog.Default(),
			webhook.WithConcurrency(1),
			webhook.WithRetryPolicy(domain.WebhookRetryPolicyFixed),
		)
	}

	got, err := st.GetWebhookDelivery(ctx, d.ProjectID, d.ID)
	require.NoError(t, err)
	require.Equal(t, domain.WebhookStatusDelivered,

		got.Status)
	require.GreaterOrEqual(t,

		got.Attempts,
		3)
	require.GreaterOrEqual(t,

		callCount.
			Load(), int32(3))

}

// TestDeadLetterAfterMaxRetries verifies that a delivery is moved to "dead"
// after exhausting all retry attempts, with the final state persisted.
func TestDeadLetterAfterMaxRetries(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	payload := `{"dead":"letter"}`
	now := time.Now()
	maxAttempts := 3
	d := &domain.WebhookDelivery{
		WebhookURL:  srv.URL,
		RetryPolicy: domain.WebhookRetryPolicyFixed,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: maxAttempts,
		NextRetryAt: &now,
		LastError:   payload,
	}
	require.NoError(t, st.CreateWebhookDelivery(ctx,
		d))
	stampDeliveryProject(t, ctx, d)

	// Run the worker repeatedly, resetting next_retry_at each time.
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		worker := webhook.NewDeliveryWorker(st, slog.Default(),
			webhook.WithConcurrency(1),
			webhook.WithRetryPolicy(domain.WebhookRetryPolicyFixed),
		)

		workerCtx, cancel := context.WithCancel(ctx)
		concWG.Go(func() {
			_ = worker.RunWorker(workerCtx, 10*time.Millisecond)
		})

		got := waitForWebhookAttempt(t, ctx, st, d.ProjectID, d.ID, attempt)
		cancel()
		_ = worker.Shutdown(context.Background())
		if got.Status == domain.WebhookStatusDead {
			break
		}

		// Reset next_retry_at for the next attempt.
		resetTime := time.Now().Add(-1 * time.Second)
		got.NextRetryAt = &resetTime
		require.NoError(t, st.UpdateWebhookDelivery(ctx,
			got))

	}

	got, err := st.GetWebhookDelivery(ctx, d.ProjectID, d.ID)
	require.NoError(t, err)
	require.Equal(t, domain.WebhookStatusDead,

		got.
			Status)
	require.Equal(t, maxAttempts,

		got.Attempts,
	)
	require.NotEqual(t, "", got.
		LastError,
	)

}

func waitForWebhookAttempt(t *testing.T, ctx context.Context, st *store.Queries, projectID, deliveryID string, attempts int) *domain.WebhookDelivery {
	t.Helper()

	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		got, err := st.GetWebhookDelivery(ctx, projectID, deliveryID)
		require.NoError(t, err)

		if got.Attempts >= attempts || got.Status == domain.WebhookStatusDead {
			return got
		}

		select {
		case <-deadline:
			require.Failf(t, "test failure", "timed out waiting for delivery %s to reach attempt %d, status = %q attempts = %d last_error = %q", deliveryID, attempts, got.Status, got.Attempts, got.LastError)
		case <-ticker.C:
		}
	}
}

// TestConcurrentWebhookDeliveries verifies that multiple pending deliveries
// are processed concurrently without duplicate sends.
func TestConcurrentWebhookDeliveries(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	const deliveryCount = 10
	var deliveredIDs sync.Map
	var totalRequests atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deliveryID := r.Header.Get("X-Strait-Delivery-ID")
		if _, loaded := deliveredIDs.LoadOrStore(deliveryID, true); loaded {
			assert.Failf(t, "test failure", "duplicate delivery for ID %s", deliveryID)
		}
		totalRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ids := make([]string, deliveryCount)
	for i := range deliveryCount {
		now := time.Now()
		d := &domain.WebhookDelivery{
			WebhookURL:  srv.URL,
			RetryPolicy: domain.WebhookRetryPolicyFixed,
			Status:      domain.WebhookStatusPending,
			Attempts:    0,
			MaxAttempts: 3,
			NextRetryAt: &now,
			LastError:   fmt.Sprintf(`{"index":%d}`, i),
		}
		require.NoError(t, st.CreateWebhookDelivery(ctx,
			d))
		stampDeliveryProject(t, ctx, d)

		ids[i] = d.ID
	}

	worker := webhook.NewDeliveryWorker(st, slog.Default(), webhook.WithConcurrency(5))
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	concWG.Go(func() {
		_ = worker.RunWorker(workerCtx, 100*time.Millisecond)
	})

	deadline := time.After(30 * time.Second)
	for int(totalRequests.Load()) < deliveryCount {
		select {
		case <-deadline:
			require.Failf(t, "test failure", "timed out: received %d/%d deliveries", totalRequests.Load(), deliveryCount)
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
	require.Equal(t, int32(deliveryCount),

		totalRequests.
			Load())

	// Wait for the worker to mark all deliveries in the DB before
	// canceling the context. The HTTP handler counts requests on receipt,
	// but the DB update happens asynchronously after the response.
	dbDeadline := time.After(10 * time.Second)
	for {
		allDelivered := true
		for _, id := range ids {
			got, err := st.GetWebhookDelivery(ctx, "webhook-itest-proj", id)
			require.NoError(t, err)

			if got.Status != domain.WebhookStatusDelivered {
				allDelivered = false
				break
			}
		}
		if allDelivered {
			break
		}
		select {
		case <-dbDeadline:
			require.Fail(t, "timed out waiting for all deliveries to be marked delivered in DB")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	cancel()
	_ = worker.Shutdown(context.Background())

	// Final verification: all are marked delivered in the DB.
	for _, id := range ids {
		got, err := st.GetWebhookDelivery(ctx, "webhook-itest-proj", id)
		require.NoError(t, err)
		require.Equal(t, domain.WebhookStatusDelivered,

			got.Status)

	}
}

// TestWebhookSubscriptionCRUD exercises Create, Get, List, Delete on webhook
// subscriptions with a real Postgres database.
func TestWebhookSubscriptionCRUD(t *testing.T) {
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := newID()

	// Create
	sub := &domain.WebhookSubscription{
		ProjectID:  projectID,
		WebhookURL: "https://example.com/webhook",
		EventTypes: []string{"run.completed", "run.failed"},
		Secret:     "test-secret-123",
		Active:     true,
	}
	require.NoError(t, st.CreateWebhookSubscription(ctx, sub))
	require.NotEqual(t, "", sub.
		ID)
	require.False(t, sub.CreatedAt.
		IsZero())

	// Get
	got, err := st.GetWebhookSubscription(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, projectID,

		got.ProjectID,
	)
	require.Equal(t, sub.WebhookURL,

		got.
			WebhookURL,
	)
	require.Len(t, got.EventTypes,

		2)
	require.Equal(t, "test-secret-123",

		got.Secret,
	)
	require.True(t, got.Active)

	// Create a second subscription for the same project.
	sub2 := &domain.WebhookSubscription{
		ProjectID:  projectID,
		WebhookURL: "https://example.com/webhook2",
		EventTypes: []string{"*"},
		Secret:     "secret-2",
		Active:     true,
	}
	require.NoError(t, st.CreateWebhookSubscription(ctx, sub2))

	// List
	subs, err := st.ListWebhookSubscriptions(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, subs, 2)
	require.NoError(t, st.DeleteWebhookSubscription(ctx, sub.ID))

	// Delete

	subs, err = st.ListWebhookSubscriptions(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, subs, 1)

	// Delete non-existent should return error.
	err = st.DeleteWebhookSubscription(ctx, "non-existent-id")
	require.Error(t, err)

	// Get deleted should return error.
	_, err = st.GetWebhookSubscription(ctx, sub.ID)
	require.Error(t, err)

}

// TestDeliveryStatusTracking verifies that the full lifecycle of a delivery
// (pending -> attempts -> delivered) is correctly tracked in the database.
func TestDeliveryStatusTracking(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Now()
	d := &domain.WebhookDelivery{
		WebhookURL:  srv.URL,
		RetryPolicy: domain.WebhookRetryPolicyExponential,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   `{"status":"tracking"}`,
	}
	require.NoError(t, st.CreateWebhookDelivery(ctx,
		d))
	stampDeliveryProject(t, ctx, d)

	got, err := st.GetWebhookDelivery(ctx, d.ProjectID, d.ID)
	require.NoError(t, err)
	require.Equal(t, domain.WebhookStatusPending,

		got.Status)
	require.EqualValues(t, 0, got.Attempts)
	require.Nil(t, got.
		DeliveredAt,
	)

	// Verify it appears in pending retries.
	pending, err := st.ListPendingWebhookRetries(ctx)
	require.NoError(t, err)

	found := false
	for _, p := range pending {
		if p.ID == d.ID {
			found = true
			break
		}
	}
	require.True(t, found)

	// Run the worker to deliver it.
	worker := webhook.NewDeliveryWorker(st, slog.Default(), webhook.WithConcurrency(1))
	workerCtx, cancel := context.WithCancel(ctx)
	concWG.Go(func() {
		_ = worker.RunWorker(workerCtx, 100*time.Millisecond)
	})

	deadline := time.After(5 * time.Second)
	for {
		got, err = st.GetWebhookDelivery(ctx, d.ProjectID, d.ID)
		require.NoError(t, err)

		if got.Status == domain.WebhookStatusDelivered {
			break
		}
		select {
		case <-deadline:
			require.Failf(t, "test failure", "timed out waiting for delivery, status = %q", got.Status)
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
	cancel()
	_ = worker.Shutdown(context.Background())
	require.Equal(t, domain.WebhookStatusDelivered,

		got.Status)
	require.EqualValues(t, 1, got.Attempts)
	require.NotNil(t, got.DeliveredAt)
	require.Equal(t, "", got.
		LastError,
	)

	// Final state checks.

	// Verify it no longer appears in pending retries.
	pending, err = st.ListPendingWebhookRetries(ctx)
	require.NoError(t, err)

	for _, p := range pending {
		require.NotEqual(t, d.ID,

			p.ID)

	}
}

// TestTimeoutHandlingWithSlowServer verifies that the worker correctly handles
// webhook endpoints that exceed the request timeout.
func TestTimeoutHandlingWithSlowServer(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Sleep longer than the webhook timeout to force a context deadline exceeded.
		time.Sleep(10 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Now()
	d := &domain.WebhookDelivery{
		WebhookURL:  srv.URL,
		RetryPolicy: domain.WebhookRetryPolicyFixed,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 2,
		NextRetryAt: &now,
		LastError:   `{"timeout":"test"}`,
	}
	require.NoError(t, st.CreateWebhookDelivery(ctx,
		d))
	stampDeliveryProject(t, ctx, d)

	// Use a short HTTP timeout to avoid waiting the full 10 seconds.
	worker := webhook.NewDeliveryWorker(st, slog.Default(),
		webhook.WithConcurrency(1),
		webhook.WithHTTPTransport(1*time.Second, 30*time.Second, 10, 10),
	)

	workerCtx, cancel := context.WithCancel(ctx)
	concWG.Go(func() {
		_ = worker.RunWorker(workerCtx, 200*time.Millisecond)
	})

	// Wait for the first attempt (which will timeout).
	deadline := time.After(30 * time.Second)
	for {
		got, err := st.GetWebhookDelivery(ctx, d.ProjectID, d.ID)
		require.NoError(t, err)

		if got.Attempts >= 1 {
			break
		}
		select {
		case <-deadline:
			require.Fail(t, "timed out waiting for first attempt")
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
	cancel()
	_ = worker.Shutdown(context.Background())

	got, err := st.GetWebhookDelivery(ctx, d.ProjectID, d.ID)
	require.NoError(t, err)
	require.False(t, got.Status !=
		domain.
			WebhookStatusPending &&
		got.
			Status !=
			domain.
				WebhookStatusDead,
	)
	require.NotEqual(t, "", got.
		LastError,
	)
	require.GreaterOrEqual(t,

		got.Attempts,
		1)

	// After a timeout, the delivery should still be pending (retryable)
	// or dead if max attempts were reached.

}

// TestClientErrorDeadLetters verifies that a 4xx response (non-retryable)
// immediately dead-letters the delivery without retrying.
func TestClientErrorDeadLetters(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	now := time.Now()
	d := &domain.WebhookDelivery{
		WebhookURL:  srv.URL,
		RetryPolicy: domain.WebhookRetryPolicyFixed,
		Status:      domain.WebhookStatusPending,
		Attempts:    0,
		MaxAttempts: 5,
		NextRetryAt: &now,
		LastError:   `{"client_error":"test"}`,
	}
	require.NoError(t, st.CreateWebhookDelivery(ctx,
		d))
	stampDeliveryProject(t, ctx, d)

	worker := webhook.NewDeliveryWorker(st, slog.Default(), webhook.WithConcurrency(1))
	workerCtx, cancel := context.WithCancel(ctx)
	concWG.Go(func() {
		_ = worker.RunWorker(workerCtx, 100*time.Millisecond)
	})

	deadline := time.After(5 * time.Second)
	for {
		got, err := st.GetWebhookDelivery(ctx, d.ProjectID, d.ID)
		require.NoError(t, err)

		if got.Status == domain.WebhookStatusDead {
			break
		}
		select {
		case <-deadline:
			require.Failf(t, "test failure", "timed out waiting for dead letter, status = %q", got.Status)
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
	cancel()
	_ = worker.Shutdown(context.Background())

	got, err := st.GetWebhookDelivery(ctx, d.ProjectID, d.ID)
	require.NoError(t, err)
	require.Equal(t, domain.WebhookStatusDead,

		got.
			Status)
	require.EqualValues(t, 1, got.Attempts)
	require.EqualValues(t, 1, callCount.
		Load())
	require.False(t, got.LastStatusCode ==
		nil ||
		*got.LastStatusCode !=
			http.
				StatusBadRequest,
	)

	// 4xx errors are not retryable, so only 1 attempt should have been made.

}

// TestEnqueueSubscriptionWebhooksIntegration verifies that subscription-based
// webhooks are correctly enqueued and delivered for matching event types.
func TestEnqueueSubscriptionWebhooksIntegration(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	var received atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	projectID := newID()

	// Create subscriptions: one matches, one does not, one is inactive.
	matchingSub := &domain.WebhookSubscription{
		ProjectID:  projectID,
		WebhookURL: srv.URL,
		EventTypes: []string{"run.completed"},
		Secret:     "s1",
		Active:     true,
	}
	nonMatchingSub := &domain.WebhookSubscription{
		ProjectID:  projectID,
		WebhookURL: srv.URL + "/nomatch",
		EventTypes: []string{"run.failed"},
		Secret:     "s2",
		Active:     true,
	}
	inactiveSub := &domain.WebhookSubscription{
		ProjectID:  projectID,
		WebhookURL: srv.URL + "/inactive",
		EventTypes: []string{"run.completed"},
		Secret:     "s3",
		Active:     false,
	}

	for _, sub := range []*domain.WebhookSubscription{matchingSub, nonMatchingSub, inactiveSub} {
		require.NoError(t, st.CreateWebhookSubscription(ctx, sub))

	}

	// List subscriptions (only active ones are returned).
	subs, err := st.ListWebhookSubscriptions(ctx, projectID)
	require.NoError(t, err)

	worker := webhook.NewDeliveryWorker(st, slog.Default(), webhook.WithConcurrency(2))
	payload, _ := json.Marshal(map[string]string{"run_id": "test-run-1"})
	worker.EnqueueSubscriptionWebhooks(ctx, subs, "run.completed", payload)

	// Verify only one delivery was created (matching sub only).
	pending, err := st.ListPendingWebhookRetries(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, srv.URL,

		pending[0].WebhookURL,
	)

	// Run the worker to deliver it.
	workerCtx, cancel := context.WithCancel(ctx)
	concWG.Go(func() {
		_ = worker.RunWorker(workerCtx, 100*time.Millisecond)
	})

	deadline := time.After(5 * time.Second)
	for received.Load() == 0 {
		select {
		case <-deadline:
			require.Fail(t, "timed out waiting for subscription webhook delivery")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
	cancel()
	_ = worker.Shutdown(context.Background())
	require.EqualValues(t, 1, received.
		Load(),
	)

}
