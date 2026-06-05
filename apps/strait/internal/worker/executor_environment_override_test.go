package worker

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestExecutor_EnvironmentOverride_Success(t *testing.T) {
	t.Parallel()
	// The override server should receive the request, not the original.
	overrideCalled := false
	overrideServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		overrideCalled = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"from":"override"}`))
	}))
	defer overrideServer.Close()

	originalServer := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("original server should not be called when override is active")
	}))
	defer originalServer.Close()

	overrideParsed, err := url.Parse(overrideServer.URL)
	if err != nil {
		t.Fatalf("parse override server url: %v", err)
	}
	overrideURL := "http://example.com" + ":" + overrideParsed.Port()

	transport := overrideServer.Client().Transport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", overrideParsed.Host)
	}
	client := &http.Client{Transport: transport}

	store := &mockExecutorStore{}
	store.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		job := testJob(originalServer.URL, 1, 5)
		job.EnvironmentID = "env-1"
		return job, nil
	}
	store.getResolvedEnvVarsFn = func(_ context.Context, id string) (map[string]string, error) {
		if id != "env-1" {
			t.Fatalf("unexpected environment ID: %q", id)
		}
		return map[string]string{"ENDPOINT_URL": overrideURL}, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, client)
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
	if !overrideCalled {
		t.Fatal("override server should have been called")
	}
}

func TestExecutor_EnvironmentOverride_WithSecretsUsesOriginalEndpoint(t *testing.T) {
	t.Parallel()

	var overrideCalled atomic.Bool
	overrideServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		overrideCalled.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer overrideServer.Close()

	var originalSecretHeader atomic.Value
	originalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originalSecretHeader.Store(r.Header.Get("X-Secret-API_KEY"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"from":"original"}`))
	}))
	defer originalServer.Close()

	overrideParsed, err := url.Parse(overrideServer.URL)
	if err != nil {
		t.Fatalf("parse override server url: %v", err)
	}
	overrideURL := "http://example.com" + ":" + overrideParsed.Port()

	store := &mockExecutorStore{}
	store.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		job := testJob(originalServer.URL, 1, 5)
		job.EnvironmentID = "env-1"
		return job, nil
	}
	store.getResolvedEnvVarsFn = func(_ context.Context, id string) (map[string]string, error) {
		if id != "env-1" {
			t.Fatalf("unexpected environment ID: %q", id)
		}
		return map[string]string{"ENDPOINT_URL": overrideURL}, nil
	}
	store.listSecretsFn = func(_ context.Context, jobID, environment string) ([]domain.JobSecret, error) {
		if jobID != "job-1" || environment != "env-1" {
			t.Fatalf("unexpected secret scope: job_id=%q environment=%q", jobID, environment)
		}
		return []domain.JobSecret{{SecretKey: "API_KEY", EncryptedValue: "super-secret"}}, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, originalServer.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
	if overrideCalled.Load() {
		t.Fatal("override endpoint received dispatch despite job secrets")
	}
	if got, _ := originalSecretHeader.Load().(string); got != "super-secret" {
		t.Fatalf("original endpoint secret header = %q, want super-secret", got)
	}
}

func TestExecutor_EnvironmentOverride_ErrorFallsBackToOriginal(t *testing.T) {
	t.Parallel()
	// When env resolution fails, the original endpoint should be used.
	originalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"from":"original"}`))
	}))
	defer originalServer.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		job := testJob(originalServer.URL, 1, 5)
		job.EnvironmentID = "env-1"
		return job, nil
	}
	store.getResolvedEnvVarsFn = func(_ context.Context, _ string) (map[string]string, error) {
		return nil, errors.New("env resolution failed")
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, originalServer.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
}

func TestExecutor_EnvironmentOverride_SSRFBlocked(t *testing.T) {
	t.Parallel()
	// Override to a private IP should be rejected; original endpoint used.
	originalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"from":"original"}`))
	}))
	defer originalServer.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		job := testJob(originalServer.URL, 1, 5)
		job.EnvironmentID = "env-1"
		return job, nil
	}
	store.getResolvedEnvVarsFn = func(_ context.Context, _ string) (map[string]string, error) {
		// Try to override to AWS metadata endpoint (SSRF attack)
		return map[string]string{"ENDPOINT_URL": "http://169.254.169.254/latest/meta-data/"}, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, originalServer.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	// Should complete using original endpoint, not the blocked override.
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
}

func TestExecutor_EnvironmentOverride_EmptyValueKeepsOriginal(t *testing.T) {
	t.Parallel()
	// Empty ENDPOINT_URL should not override.
	originalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"from":"original"}`))
	}))
	defer originalServer.Close()

	store := &mockExecutorStore{}
	store.getJobFn = func(_ context.Context, _ string) (*domain.Job, error) {
		job := testJob(originalServer.URL, 1, 5)
		job.EnvironmentID = "env-1"
		return job, nil
	}
	store.getResolvedEnvVarsFn = func(_ context.Context, _ string) (map[string]string, error) {
		return map[string]string{"ENDPOINT_URL": ""}, nil
	}

	exec := newTestExecutor(t, store, &mockExecQueue{}, time.Hour, originalServer.Client())
	run := testRun(1)

	exec.execute(context.Background(), run)

	calls := store.statusUpdates()
	if len(calls) != 2 {
		t.Fatalf("status update calls = %d, want 2", len(calls))
	}
	if calls[1].to != domain.StatusCompleted {
		t.Fatalf("final status = %s, want %s", calls[1].to, domain.StatusCompleted)
	}
}
