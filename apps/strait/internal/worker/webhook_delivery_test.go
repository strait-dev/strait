package worker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

func TestSendWebhookOnce_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t,
			http.MethodPost,
			r.Method,
		)
		assert.Equal(t,
			"application/json",
			r.Header.
				Get("Content-Type"))
		assert.NotEmpty(
			t, r.Header.Get("X-Run-ID"))

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusCompleted}

	result := sendWebhookOnceWith(t.Context(), webhookClient, job, run)
	assert.True(t, result.
		Delivered)
	assert.Equal(t, 200, result.StatusCode)
}

func TestSendWebhookOnce_ServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusFailed}

	result := sendWebhookOnceWith(t.Context(), webhookClient, job, run)
	assert.False(t,
		result.Delivered)
	assert.Equal(t, 500, result.StatusCode)
}

func TestSendWebhookOnce_ClientError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusFailed}

	result := sendWebhookOnceWith(t.Context(), webhookClient, job, run)
	assert.False(t,
		result.Delivered)
	assert.Equal(t, 400, result.StatusCode)
}

func TestSendWebhookOnce_WithSignature(t *testing.T) {
	t.Parallel()
	var gotSig string
	var gotStraitSig string
	var gotTimestamp string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Webhook-Signature")
		gotStraitSig = r.Header.Get("X-Strait-Signature")
		gotTimestamp = r.Header.Get("X-Strait-Timestamp")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL, WebhookSecret: "my-secret"}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}

	result := sendWebhookOnceWith(t.Context(), webhookClient, job, run)
	require.True(t,
		result.Delivered)
	assert.NotEmpty(
		t, gotSig)
	assert.False(t,
		len(gotSig) < 5 ||
			gotSig[:3] != "v1=",
	)
	assert.NotEmpty(
		t, gotStraitSig,
	)
	assert.NotEmpty(
		t, gotTimestamp,
	)
}

func TestSendWebhookOnce_PayloadContent(t *testing.T) {
	t.Parallel()
	var gotPayload WebhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		assert.NoError(
			t, json.Unmarshal(body,
				&gotPayload,
			))

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{
		ID:        "run-123",
		JobID:     "job-456",
		ProjectID: "proj-789",
		Status:    domain.StatusCompleted,
		Attempt:   2,
	}

	sendWebhookOnceWith(t.Context(), webhookClient, job, run)
	assert.Equal(t,
		"run-123", gotPayload.
			RunID,
	)
	assert.Equal(t,
		"job-456", gotPayload.
			JobID,
	)
	assert.Equal(t,
		"proj-789", gotPayload.
			ProjectID)
	assert.Equal(t,
		"completed", gotPayload.
			Status)
	assert.Equal(t, 2, gotPayload.Attempt)
}

func TestSendWebhookOnce_NetworkError(t *testing.T) {
	t.Parallel()
	job := &domain.Job{WebhookURL: "http://localhost:59999/webhook"}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusFailed}

	result := sendWebhookOnceWith(t.Context(), webhookClient, job, run)
	assert.False(t,
		result.Delivered)
	assert.NotEmpty(
		t, result.Error,
	)
}

func TestSendWebhookWithRetry_EmptyURL(t *testing.T) {
	t.Parallel()
	job := &domain.Job{WebhookURL: ""}
	run := &domain.JobRun{ID: "run-1"}

	result := SendWebhookWithRetry(t.Context(), job, run, 3)
	assert.True(t, result.
		Delivered)
}

func TestSendWebhookWithRetry_SuccessFirstAttempt(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}

	result := SendWebhookWithRetry(t.Context(), job, run, 3)
	assert.True(t, result.
		Delivered)
	assert.EqualValues(t, 1, attempts.Load())
}

func TestSendWebhookWithRetry_ClientErrorNoRetry(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusFailed}

	result := SendWebhookWithRetry(t.Context(), job, run, 3)
	assert.False(t,
		result.Delivered)
	assert.EqualValues(t, 1, attempts.Load())
	assert.Equal(t, 400, result.StatusCode)
}

func TestSendWebhookWithRetry_DefaultMaxAttempts(t *testing.T) {
	t.Parallel()
	job := &domain.Job{WebhookURL: ""}
	run := &domain.JobRun{ID: "run-1"}

	result := SendWebhookWithRetry(t.Context(), job, run, 0)
	assert.True(t, result.
		Delivered)
}

func TestSendWebhookWithRetry_ContextCanceled(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusFailed}
	concWG.Go(func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if attempts.Load() >= 1 {
				break
			}
			time.Sleep(time.Millisecond)
		}
		cancel()
	})

	result := SendWebhookWithRetry(ctx, job, run, 3)
	assert.False(t,
		result.Delivered)
	assert.GreaterOrEqual(t, attempts.
		Load(), int32(1))
}

func TestSendWebhookWithRetry_SuccessOnSecondAttempt(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}

	result := SendWebhookWithRetry(t.Context(), job, run, 3)
	assert.True(t, result.
		Delivered)
	assert.EqualValues(t, 2, attempts.Load())
}

func TestSendWebhookWithRetry_ExhaustsAllRetries(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusFailed}

	result := SendWebhookWithRetry(t.Context(), job, run, 2)
	assert.False(t,
		result.Delivered)
	assert.EqualValues(t, 2, attempts.Load())
}
