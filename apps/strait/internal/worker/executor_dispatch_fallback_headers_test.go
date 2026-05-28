package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
)

// TestTryFallbackDispatch_CarriesSecretsTokenAndCheckpoint locks in that failover
// to the fallback endpoint preserves the same authentication and durable-resume
// headers the primary path sends: the job's secrets (X-Secret-*), the run-token
// JWT (X-Run-Token), and the checkpoint headers on a retry. Previously the
// fallback request carried only HMAC headers, so secret-dependent and SDK-based
// fallback endpoints ran without secrets, could not authenticate callbacks, and
// could not resume.
func TestTryFallbackDispatch_CarriesSecretsTokenAndCheckpoint(t *testing.T) {
	t.Parallel()

	captured := make(chan http.Header, 1)
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"source":"fallback"}`))
	}))
	defer fallback.Close()

	store := &mockExecutorStore{}
	store.listSecretsFn = func(_ context.Context, jobID, environment string) ([]domain.JobSecret, error) {
		if jobID != "job-1" || environment != "env-secret" {
			t.Fatalf("unexpected secret args: %q %q", jobID, environment)
		}
		return []domain.JobSecret{{SecretKey: "API_KEY", EncryptedValue: "super-secret"}}, nil
	}
	store.getLatestCheckpointFn = func(_ context.Context, runID string) (*domain.RunCheckpoint, error) {
		if runID != "run-1" {
			t.Fatalf("unexpected checkpoint run id: %q", runID)
		}
		return &domain.RunCheckpoint{
			RunID:     runID,
			State:     json.RawMessage(`{"step":"charge"}`),
			CreatedAt: time.Now(),
		}, nil
	}

	pool := NewPool(4)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })
	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             &mockExecQueue{},
		Store:             store,
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour,
		HTTPClient:        fallback.Client(),
		JWTSigningKey:     "test-signing-key-0123456789abcdef",
	})

	job := testJob("http://primary.invalid", 3, 5)
	job.EnvironmentID = "env-secret"
	job.FallbackEndpointURL = fallback.URL
	run := testRun(2)

	// A timeout-class primary error qualifies for fallback dispatch.
	result, dispErr, used := exec.tryFallbackDispatch(context.Background(), job, run, context.DeadlineExceeded)
	if !used {
		t.Fatalf("expected fallback to be used, got used=false err=%v", dispErr)
	}
	if dispErr != nil {
		t.Fatalf("fallback dispatch error: %v", dispErr)
	}
	if len(result) == 0 {
		t.Fatal("expected a fallback result body")
	}

	headers := <-captured
	if got := headers.Get("X-Secret-API_KEY"); got != "super-secret" {
		t.Errorf("fallback X-Secret-API_KEY = %q, want %q", got, "super-secret")
	}
	if headers.Get("X-Run-Token") == "" {
		t.Error("fallback request missing X-Run-Token")
	}
	if headers.Get("X-Last-Checkpoint") == "" {
		t.Error("fallback request missing X-Last-Checkpoint on retry")
	}
}
