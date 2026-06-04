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

	"strait/internal/domain"
)

func TestSendWebhookOnce_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing Content-Type header")
		}
		if r.Header.Get("X-Run-ID") == "" {
			t.Error("missing X-Run-ID header")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := &domain.Job{WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusCompleted}

	result := sendWebhookOnceWith(t.Context(), webhookClient, job, run)
	if !result.Delivered {
		t.Errorf("Delivered = false, want true")
	}
	if result.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", result.StatusCode)
	}
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
	if result.Delivered {
		t.Error("Delivered = true, want false")
	}
	if result.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", result.StatusCode)
	}
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
	if result.Delivered {
		t.Error("Delivered = true, want false")
	}
	if result.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", result.StatusCode)
	}
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
	if !result.Delivered {
		t.Fatal("Delivered = false")
	}
	if gotSig == "" {
		t.Error("expected X-Webhook-Signature header")
	}
	if len(gotSig) < 5 || gotSig[:3] != "v1=" {
		t.Errorf("signature format wrong: %s", gotSig)
	}
	if gotStraitSig == "" {
		t.Error("expected X-Strait-Signature header")
	}
	if gotTimestamp == "" {
		t.Error("expected X-Strait-Timestamp header")
	}
}

func TestSendWebhookOnce_PayloadContent(t *testing.T) {
	t.Parallel()
	var gotPayload WebhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotPayload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
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

	if gotPayload.RunID != "run-123" {
		t.Errorf("RunID = %s, want run-123", gotPayload.RunID)
	}
	if gotPayload.JobID != "job-456" {
		t.Errorf("JobID = %s, want job-456", gotPayload.JobID)
	}
	if gotPayload.ProjectID != "proj-789" {
		t.Errorf("ProjectID = %s, want proj-789", gotPayload.ProjectID)
	}
	if gotPayload.Status != "completed" {
		t.Errorf("Status = %s, want completed", gotPayload.Status)
	}
	if gotPayload.Attempt != 2 {
		t.Errorf("Attempt = %d, want 2", gotPayload.Attempt)
	}
}

func TestSendWebhookOnce_NetworkError(t *testing.T) {
	t.Parallel()
	job := &domain.Job{WebhookURL: "http://localhost:59999/webhook"}
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusFailed}

	result := sendWebhookOnceWith(t.Context(), webhookClient, job, run)
	if result.Delivered {
		t.Error("Delivered = true, want false")
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestSendWebhookWithRetry_EmptyURL(t *testing.T) {
	t.Parallel()
	job := &domain.Job{WebhookURL: ""}
	run := &domain.JobRun{ID: "run-1"}

	result := SendWebhookWithRetry(t.Context(), job, run, 3)
	if !result.Delivered {
		t.Error("Delivered = false, want true for empty URL")
	}
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
	if !result.Delivered {
		t.Error("Delivered = false")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1", got)
	}
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
	if result.Delivered {
		t.Error("Delivered = true, want false")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (should not retry 4xx)", got)
	}
	if result.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", result.StatusCode)
	}
}

func TestSendWebhookWithRetry_DefaultMaxAttempts(t *testing.T) {
	t.Parallel()
	job := &domain.Job{WebhookURL: ""}
	run := &domain.JobRun{ID: "run-1"}

	result := SendWebhookWithRetry(t.Context(), job, run, 0)
	if !result.Delivered {
		t.Error("Delivered = false")
	}
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
	if result.Delivered {
		t.Error("Delivered = true, want false")
	}
	if got := attempts.Load(); got < 1 {
		t.Errorf("attempts = %d, want >= 1", got)
	}
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
	if !result.Delivered {
		t.Error("Delivered = false, want true")
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
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
	if result.Delivered {
		t.Error("Delivered = true, want false")
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}
