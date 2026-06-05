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
	if testDB == nil || testDB.Pool == nil {
		t.Fatal("testDB is not initialized")
	}
	return store.New(testDB.Pool)
}

func mustClean(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := testDB.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
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
	if err := st.CreateWebhookDelivery(ctx, d); err != nil {
		t.Fatalf("CreateWebhookDelivery() error = %v", err)
	}
	return d
}

func runWebhookWorkerUntilDelivered(t *testing.T, ctx context.Context, worker *webhook.DeliveryWorker, st *store.Queries, deliveryID string) {
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
		got, err := st.GetWebhookDelivery(ctx, deliveryID)
		if err != nil {
			t.Fatalf("GetWebhookDelivery() error = %v", err)
		}
		if got.Status == domain.WebhookStatusDelivered {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for delivery %s, status = %q, last_error = %q", deliveryID, got.Status, got.LastError)
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
	runWebhookWorkerUntilDelivered(t, ctx, worker, st, d.ID)

	if received.Load() != 1 {
		t.Fatalf("received deliveries = %d, want 1", received.Load())
	}
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
			runWebhookWorkerUntilDelivered(t, ctx, worker, st, d.ID)

			if received.Load() != 1 {
				t.Fatalf("received deliveries = %d, want 1", received.Load())
			}
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

	if err := st.CreateWebhookDelivery(ctx, d); err != nil {
		t.Fatalf("CreateWebhookDelivery() error = %v", err)
	}
	if d.ID == "" {
		t.Fatal("expected delivery ID to be assigned")
	}

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
			t.Fatal("timed out waiting for webhook delivery")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	if received.Load() != 1 {
		t.Fatalf("expected 1 delivery, got %d", received.Load())
	}

	mu.Lock()
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", receivedHeaders.Get("Content-Type"))
	}
	if receivedHeaders.Get("X-Strait-Delivery-ID") != d.ID {
		t.Fatalf("expected X-Strait-Delivery-ID %q, got %q", d.ID, receivedHeaders.Get("X-Strait-Delivery-ID"))
	}

	var bodyMap map[string]any
	if err := json.Unmarshal(receivedBody, &bodyMap); err != nil {
		t.Fatalf("failed to unmarshal received body: %v", err)
	}
	if bodyMap["event_key"] != "test.event" {
		t.Fatalf("expected event_key=test.event, got %v", bodyMap["event_key"])
	}
	mu.Unlock()

	// Wait for the worker to persist the "delivered" status in the DB
	// before canceling the context. The HTTP handler counts on receipt,
	// but the DB update happens asynchronously after the response.
	dbDeadline := time.After(5 * time.Second)
	for {
		got, err := st.GetWebhookDelivery(ctx, d.ID)
		if err != nil {
			t.Fatalf("GetWebhookDelivery() error = %v", err)
		}
		if got.Status == domain.WebhookStatusDelivered {
			break
		}
		select {
		case <-dbDeadline:
			t.Fatalf("timed out waiting for DB status update, status = %q", got.Status)
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	cancel()
	_ = worker.Shutdown(context.Background())

	got, err := st.GetWebhookDelivery(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetWebhookDelivery() error = %v", err)
	}
	if got.Status != domain.WebhookStatusDelivered {
		t.Fatalf("expected status %q, got %q", domain.WebhookStatusDelivered, got.Status)
	}
	if got.DeliveredAt == nil {
		t.Fatal("expected delivered_at to be set")
	}
	if got.Attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", got.Attempts)
	}
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

	if err := st.CreateWebhookDelivery(ctx, d); err != nil {
		t.Fatalf("CreateWebhookDelivery() error = %v", err)
	}

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

		got, err := st.GetWebhookDelivery(ctx, d.ID)
		if err != nil {
			t.Fatalf("GetWebhookDelivery() error = %v", err)
		}
		if got.Status == domain.WebhookStatusDelivered {
			break
		}

		// Reset next_retry_at so the worker picks it up again on next poll.
		if got.Status == domain.WebhookStatusPending && got.NextRetryAt != nil {
			resetTime := time.Now().Add(-1 * time.Second)
			got.NextRetryAt = &resetTime
			if err := st.UpdateWebhookDelivery(ctx, got); err != nil {
				t.Fatalf("UpdateWebhookDelivery() (reset retry) error = %v", err)
			}
		}

		// Need a fresh worker since the old one has been shut down.
		worker = webhook.NewDeliveryWorker(st, slog.Default(),
			webhook.WithConcurrency(1),
			webhook.WithRetryPolicy(domain.WebhookRetryPolicyFixed),
		)
	}

	got, err := st.GetWebhookDelivery(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetWebhookDelivery() error = %v", err)
	}

	if got.Status != domain.WebhookStatusDelivered {
		t.Fatalf("expected status %q after retries, got %q", domain.WebhookStatusDelivered, got.Status)
	}
	if got.Attempts < 3 {
		t.Fatalf("expected at least 3 attempts, got %d", got.Attempts)
	}
	if callCount.Load() < 3 {
		t.Fatalf("expected server to receive at least 3 requests, got %d", callCount.Load())
	}
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

	if err := st.CreateWebhookDelivery(ctx, d); err != nil {
		t.Fatalf("CreateWebhookDelivery() error = %v", err)
	}

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

		got := waitForWebhookAttempt(t, ctx, st, d.ID, attempt)
		cancel()
		_ = worker.Shutdown(context.Background())
		if got.Status == domain.WebhookStatusDead {
			break
		}

		// Reset next_retry_at for the next attempt.
		resetTime := time.Now().Add(-1 * time.Second)
		got.NextRetryAt = &resetTime
		if err := st.UpdateWebhookDelivery(ctx, got); err != nil {
			t.Fatalf("UpdateWebhookDelivery() error = %v", err)
		}
	}

	got, err := st.GetWebhookDelivery(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetWebhookDelivery() error = %v", err)
	}

	if got.Status != domain.WebhookStatusDead {
		t.Fatalf("expected status %q, got %q", domain.WebhookStatusDead, got.Status)
	}
	if got.Attempts != maxAttempts {
		t.Fatalf("expected %d attempts, got %d", maxAttempts, got.Attempts)
	}
	if got.LastError == "" {
		t.Fatal("expected last_error to contain the failure reason")
	}
}

func waitForWebhookAttempt(t *testing.T, ctx context.Context, st *store.Queries, deliveryID string, attempts int) *domain.WebhookDelivery {
	t.Helper()

	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		got, err := st.GetWebhookDelivery(ctx, deliveryID)
		if err != nil {
			t.Fatalf("GetWebhookDelivery() error = %v", err)
		}
		if got.Attempts >= attempts || got.Status == domain.WebhookStatusDead {
			return got
		}

		select {
		case <-deadline:
			t.Fatalf("timed out waiting for delivery %s to reach attempt %d, status = %q attempts = %d last_error = %q", deliveryID, attempts, got.Status, got.Attempts, got.LastError)
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
			t.Errorf("duplicate delivery for ID %s", deliveryID)
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
		if err := st.CreateWebhookDelivery(ctx, d); err != nil {
			t.Fatalf("CreateWebhookDelivery(%d) error = %v", i, err)
		}
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
			t.Fatalf("timed out: received %d/%d deliveries", totalRequests.Load(), deliveryCount)
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	if totalRequests.Load() != deliveryCount {
		t.Fatalf("expected exactly %d requests, got %d", deliveryCount, totalRequests.Load())
	}

	// Wait for the worker to mark all deliveries in the DB before
	// canceling the context. The HTTP handler counts requests on receipt,
	// but the DB update happens asynchronously after the response.
	dbDeadline := time.After(10 * time.Second)
	for {
		allDelivered := true
		for _, id := range ids {
			got, err := st.GetWebhookDelivery(ctx, id)
			if err != nil {
				t.Fatalf("GetWebhookDelivery(%s) error = %v", id, err)
			}
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
			t.Fatal("timed out waiting for all deliveries to be marked delivered in DB")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	cancel()
	_ = worker.Shutdown(context.Background())

	// Final verification: all are marked delivered in the DB.
	for _, id := range ids {
		got, err := st.GetWebhookDelivery(ctx, id)
		if err != nil {
			t.Fatalf("GetWebhookDelivery(%s) error = %v", id, err)
		}
		if got.Status != domain.WebhookStatusDelivered {
			t.Fatalf("delivery %s: expected status %q, got %q", id, domain.WebhookStatusDelivered, got.Status)
		}
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
	if err := st.CreateWebhookSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateWebhookSubscription() error = %v", err)
	}
	if sub.ID == "" {
		t.Fatal("expected subscription ID to be assigned")
	}
	if sub.CreatedAt.IsZero() {
		t.Fatal("expected created_at to be set")
	}

	// Get
	got, err := st.GetWebhookSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetWebhookSubscription() error = %v", err)
	}
	if got.ProjectID != projectID {
		t.Fatalf("expected project_id %q, got %q", projectID, got.ProjectID)
	}
	if got.WebhookURL != sub.WebhookURL {
		t.Fatalf("expected webhook_url %q, got %q", sub.WebhookURL, got.WebhookURL)
	}
	if len(got.EventTypes) != 2 {
		t.Fatalf("expected 2 event types, got %d", len(got.EventTypes))
	}
	if got.Secret != "test-secret-123" {
		t.Fatalf("expected secret %q, got %q", "test-secret-123", got.Secret)
	}
	if !got.Active {
		t.Fatal("expected subscription to be active")
	}

	// Create a second subscription for the same project.
	sub2 := &domain.WebhookSubscription{
		ProjectID:  projectID,
		WebhookURL: "https://example.com/webhook2",
		EventTypes: []string{"*"},
		Secret:     "secret-2",
		Active:     true,
	}
	if err := st.CreateWebhookSubscription(ctx, sub2); err != nil {
		t.Fatalf("CreateWebhookSubscription(2) error = %v", err)
	}

	// List
	subs, err := st.ListWebhookSubscriptions(ctx, projectID)
	if err != nil {
		t.Fatalf("ListWebhookSubscriptions() error = %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("expected 2 subscriptions, got %d", len(subs))
	}

	// Delete
	if err := st.DeleteWebhookSubscription(ctx, sub.ID); err != nil {
		t.Fatalf("DeleteWebhookSubscription() error = %v", err)
	}

	subs, err = st.ListWebhookSubscriptions(ctx, projectID)
	if err != nil {
		t.Fatalf("ListWebhookSubscriptions() after delete error = %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription after delete, got %d", len(subs))
	}

	// Delete non-existent should return error.
	err = st.DeleteWebhookSubscription(ctx, "non-existent-id")
	if err == nil {
		t.Fatal("expected error when deleting non-existent subscription")
	}

	// Get deleted should return error.
	_, err = st.GetWebhookSubscription(ctx, sub.ID)
	if err == nil {
		t.Fatal("expected error when getting deleted subscription")
	}
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

	if err := st.CreateWebhookDelivery(ctx, d); err != nil {
		t.Fatalf("CreateWebhookDelivery() error = %v", err)
	}

	got, err := st.GetWebhookDelivery(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetWebhookDelivery() initial error = %v", err)
	}
	if got.Status != domain.WebhookStatusPending {
		t.Fatalf("initial status: expected %q, got %q", domain.WebhookStatusPending, got.Status)
	}
	if got.Attempts != 0 {
		t.Fatalf("initial attempts: expected 0, got %d", got.Attempts)
	}
	if got.DeliveredAt != nil {
		t.Fatal("initial delivered_at should be nil")
	}

	// Verify it appears in pending retries.
	pending, err := st.ListPendingWebhookRetries(ctx)
	if err != nil {
		t.Fatalf("ListPendingWebhookRetries() error = %v", err)
	}
	found := false
	for _, p := range pending {
		if p.ID == d.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("delivery not found in pending retries list")
	}

	// Run the worker to deliver it.
	worker := webhook.NewDeliveryWorker(st, slog.Default(), webhook.WithConcurrency(1))
	workerCtx, cancel := context.WithCancel(ctx)
	concWG.Go(func() {
		_ = worker.RunWorker(workerCtx, 100*time.Millisecond)
	})

	deadline := time.After(5 * time.Second)
	for {
		got, err = st.GetWebhookDelivery(ctx, d.ID)
		if err != nil {
			t.Fatalf("GetWebhookDelivery() poll error = %v", err)
		}
		if got.Status == domain.WebhookStatusDelivered {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for delivery, status = %q", got.Status)
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
	cancel()
	_ = worker.Shutdown(context.Background())

	// Final state checks.
	if got.Status != domain.WebhookStatusDelivered {
		t.Fatalf("final status: expected %q, got %q", domain.WebhookStatusDelivered, got.Status)
	}
	if got.Attempts != 1 {
		t.Fatalf("final attempts: expected 1, got %d", got.Attempts)
	}
	if got.DeliveredAt == nil {
		t.Fatal("expected delivered_at to be set after delivery")
	}
	if got.LastError != "" {
		t.Fatalf("expected last_error to be empty after success, got %q", got.LastError)
	}

	// Verify it no longer appears in pending retries.
	pending, err = st.ListPendingWebhookRetries(ctx)
	if err != nil {
		t.Fatalf("ListPendingWebhookRetries() after delivery error = %v", err)
	}
	for _, p := range pending {
		if p.ID == d.ID {
			t.Fatal("delivered webhook should not appear in pending retries")
		}
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

	if err := st.CreateWebhookDelivery(ctx, d); err != nil {
		t.Fatalf("CreateWebhookDelivery() error = %v", err)
	}

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
		got, err := st.GetWebhookDelivery(ctx, d.ID)
		if err != nil {
			t.Fatalf("GetWebhookDelivery() error = %v", err)
		}
		if got.Attempts >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for first attempt")
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
	cancel()
	_ = worker.Shutdown(context.Background())

	got, err := st.GetWebhookDelivery(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetWebhookDelivery() final error = %v", err)
	}

	// After a timeout, the delivery should still be pending (retryable)
	// or dead if max attempts were reached.
	if got.Status != domain.WebhookStatusPending && got.Status != domain.WebhookStatusDead {
		t.Fatalf("expected status %q or %q after timeout, got %q",
			domain.WebhookStatusPending, domain.WebhookStatusDead, got.Status)
	}
	if got.LastError == "" {
		t.Fatal("expected last_error to describe the timeout")
	}
	if got.Attempts < 1 {
		t.Fatalf("expected at least 1 attempt, got %d", got.Attempts)
	}
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

	if err := st.CreateWebhookDelivery(ctx, d); err != nil {
		t.Fatalf("CreateWebhookDelivery() error = %v", err)
	}

	worker := webhook.NewDeliveryWorker(st, slog.Default(), webhook.WithConcurrency(1))
	workerCtx, cancel := context.WithCancel(ctx)
	concWG.Go(func() {
		_ = worker.RunWorker(workerCtx, 100*time.Millisecond)
	})

	deadline := time.After(5 * time.Second)
	for {
		got, err := st.GetWebhookDelivery(ctx, d.ID)
		if err != nil {
			t.Fatalf("GetWebhookDelivery() error = %v", err)
		}
		if got.Status == domain.WebhookStatusDead {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for dead letter, status = %q", got.Status)
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
	cancel()
	_ = worker.Shutdown(context.Background())

	got, err := st.GetWebhookDelivery(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetWebhookDelivery() final error = %v", err)
	}

	if got.Status != domain.WebhookStatusDead {
		t.Fatalf("expected status %q, got %q", domain.WebhookStatusDead, got.Status)
	}
	// 4xx errors are not retryable, so only 1 attempt should have been made.
	if got.Attempts != 1 {
		t.Fatalf("expected 1 attempt for client error, got %d", got.Attempts)
	}
	if callCount.Load() != 1 {
		t.Fatalf("expected server to receive exactly 1 request, got %d", callCount.Load())
	}
	if got.LastStatusCode == nil || *got.LastStatusCode != http.StatusBadRequest {
		t.Fatalf("expected last_status_code 400, got %v", got.LastStatusCode)
	}
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
		if err := st.CreateWebhookSubscription(ctx, sub); err != nil {
			t.Fatalf("CreateWebhookSubscription() error = %v", err)
		}
	}

	// List subscriptions (only active ones are returned).
	subs, err := st.ListWebhookSubscriptions(ctx, projectID)
	if err != nil {
		t.Fatalf("ListWebhookSubscriptions() error = %v", err)
	}

	worker := webhook.NewDeliveryWorker(st, slog.Default(), webhook.WithConcurrency(2))
	payload, _ := json.Marshal(map[string]string{"run_id": "test-run-1"})
	worker.EnqueueSubscriptionWebhooks(ctx, subs, "run.completed", payload)

	// Verify only one delivery was created (matching sub only).
	pending, err := st.ListPendingWebhookRetries(ctx)
	if err != nil {
		t.Fatalf("ListPendingWebhookRetries() error = %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending delivery (matching sub only), got %d", len(pending))
	}
	if pending[0].WebhookURL != srv.URL {
		t.Fatalf("expected delivery URL %q, got %q", srv.URL, pending[0].WebhookURL)
	}

	// Run the worker to deliver it.
	workerCtx, cancel := context.WithCancel(ctx)
	concWG.Go(func() {
		_ = worker.RunWorker(workerCtx, 100*time.Millisecond)
	})

	deadline := time.After(5 * time.Second)
	for received.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for subscription webhook delivery")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
	cancel()
	_ = worker.Shutdown(context.Background())

	if received.Load() != 1 {
		t.Fatalf("expected 1 delivery to matching subscription, got %d", received.Load())
	}
}
