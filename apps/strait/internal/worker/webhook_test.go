package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/stretchr/testify/require"
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
		return sendWebhookOnceWith(ctx, webhookClient, job, run)
	})
	require.True(t,
		result.Delivered)
	require.Equal(t,
		http.StatusOK, result.
			StatusCode,
	)
	require.EqualValues(t, 1, hits.Load())
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
		return sendWebhookOnceWith(ctx, webhookClient, job, run)
	})
	require.True(t,
		result.Delivered)
	require.EqualValues(t, 2, hits.Load())
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
		return sendWebhookOnceWith(ctx, webhookClient, job, run)
	})
	require.False(t,
		result.Delivered,
	)
	require.EqualValues(t, 1, hits.Load())
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
		return sendWebhookOnceWith(ctx, webhookClient, job, run)
	})
	require.False(t,
		result.Delivered,
	)
	require.EqualValues(t, 3, hits.Load())
}

func TestSendWebhook_EmptyWebhookURL(t *testing.T) {
	t.Parallel()

	result := SendWebhookWithRetry(context.Background(), &domain.Job{ID: "job-1"}, webhookTestRun(), 3)
	require.True(t,
		result.Delivered)
}

func TestRunForTerminalWebhook_IncludesFinalResultAndError(t *testing.T) {
	t.Parallel()

	finishedAt := time.Now().UTC()
	run := &domain.JobRun{
		ID:        "run-123",
		JobID:     "job-456",
		ProjectID: "proj-789",
		Status:    domain.StatusExecuting,
		Attempt:   2,
	}
	success := runForTerminalWebhook(run, domain.StatusCompleted, map[string]any{
		"result":      json.RawMessage(`{"ok":true}`),
		"finished_at": finishedAt,
	})
	require.Equal(t,
		domain.StatusCompleted,
		success.
			Status)
	require.Equal(t,
		`{"ok":true}`, string(success.
			Result))
	require.False(t,
		success.FinishedAt ==
			nil ||
			!success.FinishedAt.
				Equal(finishedAt))
	require.False(t,
		run.Result != nil ||
			run.Error !=
				"")

	failed := runForTerminalWebhook(run, domain.StatusFailed, map[string]any{
		"error":       "endpoint returned 500",
		"finished_at": finishedAt,
	})
	require.Equal(t,
		domain.StatusFailed,
		failed.
			Status)
	require.Equal(t,
		"endpoint returned 500",
		failed.
			Error)
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
	require.True(t,
		result.Delivered)
	require.Equal(t,
		http.StatusOK, result.
			StatusCode,
	)
	require.EqualValues(t, 1, hits.Load())
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
	require.True(t,
		result.Delivered)
	require.EqualValues(t, 2, hits.Load())
}
