package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

func webhookTestJob(webhookURL string) *domain.Job {
	return &domain.Job{ID: "job-1", WebhookURL: webhookURL}
}

func webhookTestRun() *domain.JobRun {
	return &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusCompleted,
		Attempt:   1,
	}
}

// fastRetryPolicy returns a retry policy with minimal backoff for fast tests.
func fastRetryPolicy(maxAttempts int) retrypolicy.RetryPolicy[WebhookResult] {
	return retrypolicy.NewBuilder[WebhookResult]().
		WithMaxRetries(maxAttempts - 1).
		WithDelay(time.Millisecond).
		HandleIf(func(result WebhookResult, _ error) bool {
			if result.StatusCode >= 400 && result.StatusCode < 500 {
				return false
			}
			return !result.Delivered
		}).
		ReturnLastFailure().
		Build()
}

func TestSendWebhook_Success(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := webhookTestJob(srv.URL)
	run := webhookTestRun()
	rp := fastRetryPolicy(3)

	result := sendWithRetryPolicy(context.Background(), rp, job, run, func(ctx context.Context) WebhookResult {
		return sendWebhookOnce(ctx, job, run)
	})

	if !result.Delivered {
		t.Fatalf("expected delivered=true, got error: %s", result.Error)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", result.StatusCode)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("expected 1 server hit, got %d", got)
	}
}

func TestSendWebhook_RetriesOn503(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := webhookTestJob(srv.URL)
	run := webhookTestRun()
	rp := fastRetryPolicy(3)

	result := sendWithRetryPolicy(context.Background(), rp, job, run, func(ctx context.Context) WebhookResult {
		return sendWebhookOnce(ctx, job, run)
	})

	if !result.Delivered {
		t.Fatalf("expected delivered=true, got error: %s", result.Error)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("expected 2 server hits, got %d", got)
	}
}

func TestSendWebhook_NoRetryOn400(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	job := webhookTestJob(srv.URL)
	run := webhookTestRun()
	rp := fastRetryPolicy(3)

	result := sendWithRetryPolicy(context.Background(), rp, job, run, func(ctx context.Context) WebhookResult {
		return sendWebhookOnce(ctx, job, run)
	})

	if result.Delivered {
		t.Fatal("expected delivered=false for 400 response")
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("expected 1 server hit, got %d", got)
	}
}

func TestSendWebhook_MaxRetriesExhausted(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	job := webhookTestJob(srv.URL)
	run := webhookTestRun()
	rp := fastRetryPolicy(3)

	result := sendWithRetryPolicy(context.Background(), rp, job, run, func(ctx context.Context) WebhookResult {
		return sendWebhookOnce(ctx, job, run)
	})

	if result.Delivered {
		t.Fatal("expected delivered=false after exhausting retries")
	}
	if got := hits.Load(); got != 3 {
		t.Fatalf("expected 3 server hits, got %d", got)
	}
}

func TestSendWebhook_EmptyWebhookURL(t *testing.T) {
	t.Parallel()

	result := SendWebhookWithRetry(context.Background(), &domain.Job{ID: "job-1"}, webhookTestRun(), 3)
	if !result.Delivered {
		t.Fatal("expected delivered=true for empty webhook URL")
	}
}

func TestSendWebhookWithClient_Success(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := webhookTestJob(srv.URL)
	run := webhookTestRun()
	client := srv.Client()
	rp := fastRetryPolicy(3)

	result := sendWithRetryPolicy(context.Background(), rp, job, run, func(ctx context.Context) WebhookResult {
		return sendWebhookOnceWith(ctx, client, job, run)
	})

	if !result.Delivered {
		t.Fatalf("expected delivered=true, got error: %s", result.Error)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", result.StatusCode)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("expected 1 server hit, got %d", got)
	}
}

func TestSendWebhookWithClient_RetriesOn503(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := webhookTestJob(srv.URL)
	run := webhookTestRun()
	client := srv.Client()
	rp := fastRetryPolicy(3)

	result := sendWithRetryPolicy(context.Background(), rp, job, run, func(ctx context.Context) WebhookResult {
		return sendWebhookOnceWith(ctx, client, job, run)
	})

	if !result.Delivered {
		t.Fatalf("expected delivered=true, got error: %s", result.Error)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("expected 2 server hits, got %d", got)
	}
}
