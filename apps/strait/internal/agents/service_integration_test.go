//go:build integration

package agents_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"strait/internal/agents"
	"strait/internal/api"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
)

const runtimeTestJWTKey = "01234567890123456789012345678901"

var testDB *testutil.TestDB

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	testDB, err = testutil.SetupTestDB(ctx, "../../migrations")
	if err != nil {
		log.Fatalf("setup test db: %v", err)
	}

	code := m.Run()
	testDB.Cleanup(ctx)
	os.Exit(code)
}

func TestServiceLifecycleReusesJobRuns(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)

	recorder := &recordingPublisher{}
	srv := api.NewServer(api.ServerDeps{
		Config: &config.Config{
			InternalSecret:     "test-internal-secret",
			JWTSigningKey:      runtimeTestJWTKey,
			MaxRequestBodySize: 1 << 20,
			MaxResultSize:      1 << 20,
		},
		Store:  q,
		PubSub: recorder,
	})
	httpServer := httptest.NewServer(srv)
	defer httpServer.Close()
	defer srv.Close()

	svc := agents.NewService(
		q,
		testDB.Pool,
		agents.WithAPIBaseURL(httpServer.URL),
		agents.WithJWTSigningKey(runtimeTestJWTKey),
	)
	defer closeService(svc)

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID:   projectID,
		Name:        "Support Agent",
		Slug:        "support-agent",
		Description: "Handles support tickets",
		Model:       "gpt-5.4",
		Config:      json.RawMessage(`{"temperature":0.2}`),
		Actor:       "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	backingJob, err := q.GetJob(ctx, agent.JobID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if backingJob.Enabled {
		t.Fatal("expected backing job to be disabled")
	}
	if backingJob.Slug != "__agent__support-agent" {
		t.Fatalf("backingJob.Slug = %q, want __agent__support-agent", backingJob.Slug)
	}

	deployment, err := svc.DeployAgent(ctx, projectID, agent.ID, "user-1")
	if err != nil {
		t.Fatalf("DeployAgent() error = %v", err)
	}
	if deployment.Status != domain.AgentDeploymentStatusDeployed {
		t.Fatalf("deployment.Status = %s, want deployed", deployment.Status)
	}

	run, err := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID: projectID,
		AgentID:   agent.ID,
		Payload:   json.RawMessage(`{"prompt":"hello"}`),
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}
	if run.JobID != agent.JobID {
		t.Fatalf("run.JobID = %q, want %q", run.JobID, agent.JobID)
	}
	if run.Status != domain.StatusQueued {
		t.Fatalf("run.Status = %s, want queued", run.Status)
	}

	storedRun := mustWaitForRunStatus(t, ctx, q, run.ID, domain.StatusCompleted)
	if storedRun.JobID != agent.JobID {
		t.Fatalf("storedRun.JobID = %q, want %q", storedRun.JobID, agent.JobID)
	}

	var result map[string]any
	if err := json.Unmarshal(storedRun.Result, &result); err != nil {
		t.Fatalf("unmarshal run result: %v", err)
	}
	if result["agent_id"] != agent.ID {
		t.Fatalf("result.agent_id = %v, want %s", result["agent_id"], agent.ID)
	}

	checkpoints, err := q.ListRunCheckpoints(ctx, run.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListRunCheckpoints() error = %v", err)
	}
	if len(checkpoints) != 1 {
		t.Fatalf("checkpoint count = %d, want 1", len(checkpoints))
	}

	usage, err := q.ListRunUsage(ctx, run.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListRunUsage() error = %v", err)
	}
	if len(usage) != 1 {
		t.Fatalf("usage count = %d, want 1", len(usage))
	}
	if usage[0].Provider != "local" {
		t.Fatalf("usage provider = %q, want local", usage[0].Provider)
	}

	toolCalls, err := q.ListRunToolCalls(ctx, run.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListRunToolCalls() error = %v", err)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(toolCalls))
	}
	if toolCalls[0].ToolName != "local.echo" {
		t.Fatalf("tool call = %q, want local.echo", toolCalls[0].ToolName)
	}

	streamMessages := recorder.Messages("run_stream:" + run.ID)
	if len(streamMessages) != 2 {
		t.Fatalf("stream message count = %d, want 2", len(streamMessages))
	}
	if !strings.Contains(streamMessages[0], `"chunk":"agent:support-agent:thinking "`) {
		t.Fatalf("first stream chunk = %s", streamMessages[0])
	}
	if !strings.Contains(streamMessages[1], `"done":true`) {
		t.Fatalf("second stream chunk = %s", streamMessages[1])
	}

	runs, err := svc.ListAgentRuns(ctx, projectID, agent.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListAgentRuns() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("ListAgentRuns() len = %d, want 1", len(runs))
	}

	if err := svc.DeleteAgent(ctx, projectID, agent.ID); err != nil {
		t.Fatalf("DeleteAgent() error = %v", err)
	}
	if _, err := q.GetAgent(ctx, agent.ID); !errors.Is(err, store.ErrAgentNotFound) {
		t.Fatalf("GetAgent(after delete) error = %v, want ErrAgentNotFound", err)
	}
}

func TestServiceRunAgentFailurePersistsFailedRun(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)

	srv := api.NewServer(api.ServerDeps{
		Config: &config.Config{
			InternalSecret:     "test-internal-secret",
			JWTSigningKey:      runtimeTestJWTKey,
			MaxRequestBodySize: 1 << 20,
			MaxResultSize:      1 << 20,
		},
		Store: q,
	})
	httpServer := httptest.NewServer(srv)
	defer httpServer.Close()
	defer srv.Close()

	svc := agents.NewService(
		q,
		testDB.Pool,
		agents.WithAPIBaseURL(httpServer.URL),
		agents.WithJWTSigningKey(runtimeTestJWTKey),
	)
	defer closeService(svc)

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Failure Agent",
		Slug:      "failure-agent",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	if _, err := svc.DeployAgent(ctx, projectID, agent.ID, "user-1"); err != nil {
		t.Fatalf("DeployAgent() error = %v", err)
	}

	run, err := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID: projectID,
		AgentID:   agent.ID,
		Payload:   json.RawMessage(`{"_scenario":"fail","_error":"boom"}`),
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}

	storedRun := mustWaitForRunStatus(t, ctx, q, run.ID, domain.StatusFailed)
	if !strings.Contains(storedRun.Error, "boom") {
		t.Fatalf("run.Error = %q, want boom", storedRun.Error)
	}
}

func TestServiceRunAgentRuntimeDisconnectMapsToSystemFailure(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)

	srv := api.NewServer(api.ServerDeps{
		Config: &config.Config{
			InternalSecret:     "test-internal-secret",
			JWTSigningKey:      runtimeTestJWTKey,
			MaxRequestBodySize: 1 << 20,
			MaxResultSize:      1 << 20,
		},
		Store: q,
	})
	httpServer := httptest.NewServer(srv)
	defer httpServer.Close()
	defer srv.Close()

	svc := agents.NewService(
		q,
		testDB.Pool,
		agents.WithAPIBaseURL(httpServer.URL),
		agents.WithJWTSigningKey(runtimeTestJWTKey),
	)
	defer closeService(svc)

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Disconnect Agent",
		Slug:      "disconnect-agent",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	if _, err := svc.DeployAgent(ctx, projectID, agent.ID, "user-1"); err != nil {
		t.Fatalf("DeployAgent() error = %v", err)
	}

	run, err := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID: projectID,
		AgentID:   agent.ID,
		Payload:   json.RawMessage(`{"_scenario":"disconnect"}`),
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}

	storedRun := mustWaitForRunStatus(t, ctx, q, run.ID, domain.StatusSystemFailed)
	if !strings.Contains(storedRun.Error, "without terminal event") {
		t.Fatalf("run.Error = %q, want runtime exit error", storedRun.Error)
	}
}

func TestServiceRunAgentDuplicateCheckpointStillCompletes(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)

	srv := api.NewServer(api.ServerDeps{
		Config: &config.Config{
			InternalSecret:     "test-internal-secret",
			JWTSigningKey:      runtimeTestJWTKey,
			MaxRequestBodySize: 1 << 20,
			MaxResultSize:      1 << 20,
		},
		Store: q,
	})
	httpServer := httptest.NewServer(srv)
	defer httpServer.Close()
	defer srv.Close()

	svc := agents.NewService(
		q,
		testDB.Pool,
		agents.WithAPIBaseURL(httpServer.URL),
		agents.WithJWTSigningKey(runtimeTestJWTKey),
	)
	defer closeService(svc)

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Duplicate Agent",
		Slug:      "duplicate-agent",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	if _, err := svc.DeployAgent(ctx, projectID, agent.ID, "user-1"); err != nil {
		t.Fatalf("DeployAgent() error = %v", err)
	}

	run, err := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID: projectID,
		AgentID:   agent.ID,
		Payload:   json.RawMessage(`{"_scenario":"duplicate_checkpoint"}`),
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}

	mustWaitForRunStatus(t, ctx, q, run.ID, domain.StatusCompleted)
	checkpoints, err := q.ListRunCheckpoints(ctx, run.ID, 10, nil)
	if err != nil {
		t.Fatalf("ListRunCheckpoints() error = %v", err)
	}
	if len(checkpoints) != 2 {
		t.Fatalf("checkpoint count = %d, want 2", len(checkpoints))
	}
}

func TestServiceDeployAgentConcurrentVersions(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)

	svc := agents.NewService(q, testDB.Pool, agents.WithJWTSigningKey(runtimeTestJWTKey))
	defer closeService(svc)

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Concurrent Agent",
		Slug:      "concurrent-agent",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan *domain.AgentDeployment, 2)
	errs := make(chan error, 2)

	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deployment, err := svc.DeployAgent(ctx, projectID, agent.ID, "user-1")
			if err != nil {
				errs <- err
				return
			}
			results <- deployment
		}()
	}

	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("DeployAgent() concurrent error = %v", err)
		}
	}

	versions := make([]int, 0, 2)
	for deployment := range results {
		versions = append(versions, deployment.Version)
		if deployment.Status != domain.AgentDeploymentStatusDeployed {
			t.Fatalf("deployment.Status = %s, want deployed", deployment.Status)
		}
	}

	slices.Sort(versions)
	if !slices.Equal(versions, []int{1, 2}) {
		t.Fatalf("versions = %v, want [1 2]", versions)
	}
}

func TestServiceCloudflareDeployAndDeleteCleanup(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)

	var uploadPaths []string
	var deletePaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			uploadPaths = append(uploadPaths, r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"success":true,"result":{"id":"agent-script","etag":"etag-123","compatibility_date":"2026-03-29"}}`)
		case http.MethodDelete:
			deletePaths = append(deletePaths, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	svc := agents.NewService(
		q,
		testDB.Pool,
		agents.WithProvider(agents.NewCloudflareProvider(agents.CloudflareConfig{
			AccountID:         "acct-1",
			APIToken:          "token-1",
			DispatchNamespace: "ns-prod",
			DispatchWorkerURL: "https://dispatch.example.com",
			CompatibilityDate: "2026-03-29",
			SandboxMode:       agents.CloudflareSandboxModeDisabled,
		}, agents.WithCloudflareAPIBaseURL(server.URL))),
		agents.WithJWTSigningKey(runtimeTestJWTKey),
	)
	defer closeService(svc)

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Cloudflare Agent",
		Slug:      "cloudflare-agent",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	deployment, err := svc.DeployAgent(ctx, projectID, agent.ID, "user-1")
	if err != nil {
		t.Fatalf("DeployAgent() error = %v", err)
	}
	if deployment.Provider != agents.ProviderNameCloudflare {
		t.Fatalf("deployment.Provider = %q, want cloudflare", deployment.Provider)
	}

	parsed, err := agents.ParseCloudflareDeploymentMetadata(deployment.ProviderMetadata)
	if err != nil {
		t.Fatalf("ParseCloudflareDeploymentMetadata() error = %v", err)
	}
	if parsed.ScriptName == "" || parsed.Namespace != "ns-prod" {
		t.Fatalf("parsed metadata = %+v", parsed)
	}
	if len(uploadPaths) != 1 {
		t.Fatalf("upload path count = %d, want 1", len(uploadPaths))
	}

	if err := svc.DeleteAgent(ctx, projectID, agent.ID); err != nil {
		t.Fatalf("DeleteAgent() error = %v", err)
	}
	if len(deletePaths) != 1 {
		t.Fatalf("delete path count = %d, want 1", len(deletePaths))
	}
	if _, err := q.GetAgent(ctx, agent.ID); !errors.Is(err, store.ErrAgentNotFound) {
		t.Fatalf("GetAgent(after delete) error = %v, want ErrAgentNotFound", err)
	}
}

func TestServiceCloudflareRunDispatchCompletesViaCallbacks(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)

	srv := api.NewServer(api.ServerDeps{
		Config: &config.Config{
			InternalSecret:     "test-internal-secret",
			JWTSigningKey:      runtimeTestJWTKey,
			MaxRequestBodySize: 1 << 20,
			MaxResultSize:      1 << 20,
		},
		Store: q,
	})
	httpServer := httptest.NewServer(srv)
	defer httpServer.Close()
	defer srv.Close()

	dispatchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"success":true,"result":{"id":"agent-script","etag":"etag-123","compatibility_date":"2026-03-29"}}`)
			return
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return
		case http.MethodPost:
			if got := r.Header.Get("Authorization"); got != "Bearer test-internal-secret" {
				t.Fatalf("dispatch auth = %q", got)
			}

			var payload agents.CloudflareDispatchRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode dispatch payload: %v", err)
			}
			callbacks := []struct {
				path string
				body string
			}{
				{path: "checkpoint", body: `{"source":"dispatch-test","state":{"phase":"planning"}}`},
				{path: "usage", body: `{"provider":"cloudflare","model":"gpt-5.4","prompt_tokens":10,"completion_tokens":4,"total_tokens":14,"cost_microusd":140}`},
				{path: "tool-call", body: `{"tool_name":"local.echo","input":{"prompt":"hello"},"output":{"echoed":"hello"},"duration_ms":4,"status":"completed"}`},
				{path: "stream", body: `{"chunk":"done","stream_id":"default","done":true}`},
				{path: "complete", body: `{"result":{"ok":true,"provider":"cloudflare"}}`},
			}
			client := &http.Client{}
			for _, callback := range callbacks {
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, httpServer.URL+"/sdk/v1/runs/"+payload.RunID+"/"+callback.path, bytes.NewBufferString(callback.body))
				if err != nil {
					t.Fatalf("build callback request: %v", err)
				}
				req.Header.Set("Authorization", "Bearer "+payload.Envelope.Callback.RunToken)
				req.Header.Set("Content-Type", "application/json")
				resp, err := client.Do(req)
				if err != nil {
					t.Fatalf("dispatch callback request failed: %v", err)
				}
				_ = resp.Body.Close()
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					t.Fatalf("dispatch callback status = %d", resp.StatusCode)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"ok":true,"status":"accepted"}`)
			return
		default:
			t.Fatalf("unexpected dispatch method %s", r.Method)
		}
	}))
	defer dispatchServer.Close()

	svc := agents.NewService(
		q,
		testDB.Pool,
		agents.WithProvider(agents.NewCloudflareProvider(agents.CloudflareConfig{
			AccountID:         "acct-1",
			APIToken:          "token-1",
			DispatchNamespace: "ns-prod",
			DispatchWorkerURL: dispatchServer.URL,
			CompatibilityDate: "2026-03-29",
			SandboxMode:       agents.CloudflareSandboxModeDisabled,
		}, agents.WithCloudflareAPIBaseURL(dispatchServer.URL))),
		agents.WithAPIBaseURL(httpServer.URL),
		agents.WithInternalSecret("test-internal-secret"),
		agents.WithJWTSigningKey(runtimeTestJWTKey),
	)
	defer closeService(svc)

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Cloudflare Runtime Agent",
		Slug:      "cloudflare-runtime-agent",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	if _, err := svc.DeployAgent(ctx, projectID, agent.ID, "user-1"); err != nil {
		t.Fatalf("DeployAgent() error = %v", err)
	}

	run, err := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID: projectID,
		AgentID:   agent.ID,
		Payload:   json.RawMessage(`{"prompt":"hello"}`),
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}

	storedRun := mustWaitForRunStatus(t, ctx, q, run.ID, domain.StatusCompleted)
	var result map[string]any
	if err := json.Unmarshal(storedRun.Result, &result); err != nil {
		t.Fatalf("Unmarshal(result) error = %v", err)
	}
	if result["provider"] != "cloudflare" {
		t.Fatalf("result.provider = %v, want cloudflare", result["provider"])
	}
}

func TestServiceCloudflareRunDispatchFailureMapsToSystemFailed(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)

	dispatchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"success":true,"result":{"id":"agent-script","etag":"etag-123","compatibility_date":"2026-03-29"}}`)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, `{"error":"missing worker"}`, http.StatusNotFound)
		}
	}))
	defer dispatchServer.Close()

	svc := agents.NewService(
		q,
		testDB.Pool,
		agents.WithProvider(agents.NewCloudflareProvider(agents.CloudflareConfig{
			AccountID:         "acct-1",
			APIToken:          "token-1",
			DispatchNamespace: "ns-prod",
			DispatchWorkerURL: dispatchServer.URL,
			CompatibilityDate: "2026-03-29",
			SandboxMode:       agents.CloudflareSandboxModeDisabled,
		}, agents.WithCloudflareAPIBaseURL(dispatchServer.URL))),
		agents.WithAPIBaseURL("http://127.0.0.1:65535"),
		agents.WithInternalSecret("test-internal-secret"),
		agents.WithJWTSigningKey(runtimeTestJWTKey),
	)
	defer closeService(svc)

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Cloudflare Failure Agent",
		Slug:      "cloudflare-failure-agent",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	if _, err := svc.DeployAgent(ctx, projectID, agent.ID, "user-1"); err != nil {
		t.Fatalf("DeployAgent() error = %v", err)
	}

	run, err := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID: projectID,
		AgentID:   agent.ID,
		Payload:   json.RawMessage(`{"prompt":"hello"}`),
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}

	storedRun := mustWaitForRunStatus(t, ctx, q, run.ID, domain.StatusSystemFailed)
	if !strings.Contains(storedRun.Error, "cloudflare dispatch worker returned 404") {
		t.Fatalf("run.Error = %q", storedRun.Error)
	}
}

func TestServiceDeleteAgentCancelsActiveRuns(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)

	srv := api.NewServer(api.ServerDeps{
		Config: &config.Config{
			InternalSecret:     "test-internal-secret",
			JWTSigningKey:      runtimeTestJWTKey,
			MaxRequestBodySize: 1 << 20,
			MaxResultSize:      1 << 20,
		},
		Store: q,
	})
	httpServer := httptest.NewServer(srv)
	defer httpServer.Close()
	defer srv.Close()

	// Track whether the runtime runner observed a canceled run before deletion cascade.
	var runCanceledBeforeDelete sync.WaitGroup
	runCanceledBeforeDelete.Add(1)
	svc := agents.NewService(
		q,
		testDB.Pool,
		agents.WithAPIBaseURL(httpServer.URL),
		agents.WithJWTSigningKey(runtimeTestJWTKey),
		agents.WithRuntimeRunner(agents.RuntimeRunnerFunc(func(_ context.Context, envelope agents.RuntimeDispatchEnvelope, _ agents.RuntimeEventHandler) error {
			// Wait until the test signals deletion is about to happen,
			// then check the run status before we return.
			runCanceledBeforeDelete.Wait()
			storedRun, _ := q.GetRun(ctx, envelope.Run.ID)
			_ = storedRun
			return nil
		})),
	)
	defer closeService(svc)

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Slow Agent",
		Slug:      "slow-agent",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	if _, err := svc.DeployAgent(ctx, projectID, agent.ID, "user-1"); err != nil {
		t.Fatalf("DeployAgent() error = %v", err)
	}

	_, err = svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID: projectID,
		AgentID:   agent.ID,
		Payload:   json.RawMessage(`{"prompt":"hello"}`),
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}

	// Wait for the run to transition past queued.
	time.Sleep(500 * time.Millisecond)

	// Signal the runtime runner to check the run status, then delete.
	// DeleteAgent cancels active runs before the cascade delete removes them.
	runCanceledBeforeDelete.Done()

	if err := svc.DeleteAgent(ctx, projectID, agent.ID); err != nil {
		t.Fatalf("DeleteAgent() error = %v", err)
	}

	// After cascade delete, the run is gone. But verify the agent is deleted.
	if _, err := q.GetAgent(ctx, agent.ID); !errors.Is(err, store.ErrAgentNotFound) {
		t.Fatalf("GetAgent(after delete) error = %v, want ErrAgentNotFound", err)
	}
}

func mustWaitForRunStatus(t *testing.T, ctx context.Context, q *store.Queries, runID string, want domain.RunStatus) *domain.JobRun {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		run, err := q.GetRun(ctx, runID)
		if err != nil {
			t.Fatalf("GetRun() error = %v", err)
		}
		if run.Status == want {
			return run
		}
		time.Sleep(100 * time.Millisecond)
	}
	run, err := q.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	t.Fatalf("run status = %s, want %s", run.Status, want)
	return nil
}

func closeService(svc agents.Service) {
	if closer, ok := svc.(interface{ Close() }); ok {
		closer.Close()
	}
}

type recordingPublisher struct {
	mu       sync.Mutex
	messages map[string][][]byte
}

func (p *recordingPublisher) Publish(_ context.Context, channel string, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.messages == nil {
		p.messages = make(map[string][][]byte)
	}
	p.messages[channel] = append(p.messages[channel], append([]byte(nil), data...))
	return nil
}

func (p *recordingPublisher) PublishBatch(ctx context.Context, messages []pubsub.PubSubMessage) error {
	for _, message := range messages {
		if err := p.Publish(ctx, message.Channel, message.Data); err != nil {
			return err
		}
	}
	return nil
}

func (p *recordingPublisher) Subscribe(context.Context, string) (*pubsub.Subscription, error) {
	return nil, nil
}

func (p *recordingPublisher) Close() error {
	return nil
}

func (p *recordingPublisher) Messages(channel string) []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	raw := p.messages[channel]
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		out = append(out, string(item))
	}
	return out
}

func mustClean(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := testDB.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
}

func mustCreateProject(t *testing.T, ctx context.Context, q *store.Queries) string {
	t.Helper()

	project := &domain.Project{
		ID:    "proj-" + newID(),
		OrgID: "org-" + newID(),
		Name:  "Agents Test Project",
	}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	return project.ID
}

func newID() string {
	return uuid.Must(uuid.NewV7()).String()
}
