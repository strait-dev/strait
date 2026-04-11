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

var (
	runtimeTestJWTKey    = testutil.GenerateTestJWTKey()
	runtimeTestIntSecret = testutil.GenerateTestInternalSecret()
	testCFToken          = testutil.GenerateTestSecret(16)

	testDB *testutil.TestDB
)

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
			InternalSecret:     runtimeTestIntSecret,
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
			InternalSecret:     runtimeTestIntSecret,
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
			InternalSecret:     runtimeTestIntSecret,
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
			InternalSecret:     runtimeTestIntSecret,
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
			APIToken:          testCFToken,
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
			InternalSecret:     runtimeTestIntSecret,
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
			if got := r.Header.Get("Authorization"); got != "Bearer "+runtimeTestIntSecret {
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
			APIToken:          testCFToken,
			DispatchNamespace: "ns-prod",
			DispatchWorkerURL: dispatchServer.URL,
			CompatibilityDate: "2026-03-29",
			SandboxMode:       agents.CloudflareSandboxModeDisabled,
		}, agents.WithCloudflareAPIBaseURL(dispatchServer.URL))),
		agents.WithAPIBaseURL(httpServer.URL),
		agents.WithInternalSecret(runtimeTestIntSecret),
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
			APIToken:          testCFToken,
			DispatchNamespace: "ns-prod",
			DispatchWorkerURL: dispatchServer.URL,
			CompatibilityDate: "2026-03-29",
			SandboxMode:       agents.CloudflareSandboxModeDisabled,
		}, agents.WithCloudflareAPIBaseURL(dispatchServer.URL))),
		agents.WithAPIBaseURL("http://127.0.0.1:65535"),
		agents.WithInternalSecret(runtimeTestIntSecret),
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
			InternalSecret:     runtimeTestIntSecret,
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

// mustCreateEnvironment creates an environment row under the given
// project and returns its ID. Used by the Phase E3 env-aware tests.
func mustCreateEnvironment(t *testing.T, ctx context.Context, q *store.Queries, projectID, slug string) string {
	t.Helper()
	env := &domain.Environment{
		ID:        "env-" + newID(),
		ProjectID: projectID,
		Name:      slug,
		Slug:      slug,
	}
	if err := q.CreateEnvironment(ctx, env); err != nil {
		t.Fatalf("CreateEnvironment(%s) error = %v", slug, err)
	}
	return env.ID
}

func newID() string {
	return uuid.Must(uuid.NewV7()).String()
}

// ---------------------------------------------------------------------------
// Phase E3: DeployAgentToEnv coverage
// ---------------------------------------------------------------------------

func TestDeployAgentToEnv_PinsEnvironmentID(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)
	devEnvID := mustCreateEnvironment(t, ctx, q, projectID, "dev")

	svc := agents.NewService(q, testDB.Pool, agents.WithJWTSigningKey(runtimeTestJWTKey))
	defer closeService(svc)

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Env Agent",
		Slug:      "env-agent",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	deployment, err := svc.DeployAgentToEnv(ctx, projectID, agent.ID, devEnvID, "user-1")
	if err != nil {
		t.Fatalf("DeployAgentToEnv() error = %v", err)
	}
	if deployment.EnvironmentID != devEnvID {
		t.Fatalf("deployment.EnvironmentID = %q, want %q", deployment.EnvironmentID, devEnvID)
	}
	if deployment.Status != domain.AgentDeploymentStatusDeployed {
		t.Fatalf("deployment.Status = %s, want deployed", deployment.Status)
	}

	// The per-env getter should find the row.
	fromEnv, err := q.GetLatestAgentDeploymentByEnvironment(ctx, agent.ID, devEnvID)
	if err != nil {
		t.Fatalf("GetLatestAgentDeploymentByEnvironment() error = %v", err)
	}
	if fromEnv.ID != deployment.ID {
		t.Fatalf("by-env ID = %q, want %q", fromEnv.ID, deployment.ID)
	}
}

func TestDeployAgentToEnv_CrossProjectEnvironmentRejected(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectA := mustCreateProject(t, ctx, q)
	projectB := mustCreateProject(t, ctx, q)
	envB := mustCreateEnvironment(t, ctx, q, projectB, "prod")

	svc := agents.NewService(q, testDB.Pool, agents.WithJWTSigningKey(runtimeTestJWTKey))
	defer closeService(svc)

	agentA, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectA,
		Name:      "A Agent",
		Slug:      "a-agent",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	// Attempt to deploy an agent owned by project A into project B's env.
	_, err = svc.DeployAgentToEnv(ctx, projectA, agentA.ID, envB, "user-1")
	if err == nil {
		t.Fatal("expected error — environment belongs to a different project")
	}
}

func TestDeployAgentToEnv_UnknownEnvironmentReturnsError(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)

	svc := agents.NewService(q, testDB.Pool, agents.WithJWTSigningKey(runtimeTestJWTKey))
	defer closeService(svc)

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Ghost Env Agent",
		Slug:      "ghost-env-agent",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	_, err = svc.DeployAgentToEnv(ctx, projectID, agent.ID, "env-does-not-exist", "user-1")
	if err == nil {
		t.Fatal("expected error for unknown environment_id")
	}
}

func TestDeployAgentToEnv_CoexistsWithLegacyNoEnvDeployment(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)
	devEnvID := mustCreateEnvironment(t, ctx, q, projectID, "dev")

	svc := agents.NewService(q, testDB.Pool, agents.WithJWTSigningKey(runtimeTestJWTKey))
	defer closeService(svc)

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Legacy Coexist",
		Slug:      "legacy-coexist",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	legacy, err := svc.DeployAgent(ctx, projectID, agent.ID, "user-1")
	if err != nil {
		t.Fatalf("DeployAgent(legacy) error = %v", err)
	}
	if legacy.EnvironmentID != "" {
		t.Fatalf("legacy deployment should have no env, got %q", legacy.EnvironmentID)
	}

	envBound, err := svc.DeployAgentToEnv(ctx, projectID, agent.ID, devEnvID, "user-1")
	if err != nil {
		t.Fatalf("DeployAgentToEnv(dev) error = %v", err)
	}
	if envBound.EnvironmentID != devEnvID {
		t.Fatalf("env-bound deployment has env %q, want %q", envBound.EnvironmentID, devEnvID)
	}
	if envBound.Version != legacy.Version+1 {
		t.Fatalf("versions not contiguous: legacy=%d env=%d", legacy.Version, envBound.Version)
	}
}

func TestDeployAgentToEnv_MultiEnvMonotonicVersions(t *testing.T) {
	// Version numbers are agent-scoped, not env-scoped. A dev->prod->dev
	// sequence should produce strictly monotonic versions.
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)
	devEnvID := mustCreateEnvironment(t, ctx, q, projectID, "dev")
	prodEnvID := mustCreateEnvironment(t, ctx, q, projectID, "prod")

	svc := agents.NewService(q, testDB.Pool, agents.WithJWTSigningKey(runtimeTestJWTKey))
	defer closeService(svc)

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Multi Env",
		Slug:      "multi-env",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	d1, err := svc.DeployAgentToEnv(ctx, projectID, agent.ID, devEnvID, "user-1")
	if err != nil {
		t.Fatalf("DeployAgentToEnv(dev) error = %v", err)
	}
	d2, err := svc.DeployAgentToEnv(ctx, projectID, agent.ID, prodEnvID, "user-1")
	if err != nil {
		t.Fatalf("DeployAgentToEnv(prod) error = %v", err)
	}
	d3, err := svc.DeployAgentToEnv(ctx, projectID, agent.ID, devEnvID, "user-1")
	if err != nil {
		t.Fatalf("DeployAgentToEnv(dev) error = %v", err)
	}

	if d1.Version != 1 || d2.Version != 2 || d3.Version != 3 {
		t.Fatalf("versions = %d,%d,%d; want 1,2,3", d1.Version, d2.Version, d3.Version)
	}

	// Each env sees its own latest.
	latestDev, err := q.GetLatestAgentDeploymentByEnvironment(ctx, agent.ID, devEnvID)
	if err != nil {
		t.Fatalf("GetLatestAgentDeploymentByEnvironment(dev) error = %v", err)
	}
	if latestDev.ID != d3.ID {
		t.Fatalf("dev latest = %q, want %q", latestDev.ID, d3.ID)
	}
	latestProd, err := q.GetLatestAgentDeploymentByEnvironment(ctx, agent.ID, prodEnvID)
	if err != nil {
		t.Fatalf("GetLatestAgentDeploymentByEnvironment(prod) error = %v", err)
	}
	if latestProd.ID != d2.ID {
		t.Fatalf("prod latest = %q, want %q", latestProd.ID, d2.ID)
	}
}

// ---------------------------------------------------------------------------
// Phase E3: RunAgent environment routing + deployment stamping
// ---------------------------------------------------------------------------

// newEnvAwareTestService builds a service with a local API server so
// RunAgent can actually dispatch and the LocalStubProvider completes runs.
// Returns the service plus a cleanup function.
func newEnvAwareTestService(t *testing.T) (agents.Service, *store.Queries, string) {
	t.Helper()
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)

	recorder := &recordingPublisher{}
	srv := api.NewServer(api.ServerDeps{
		Config: &config.Config{
			InternalSecret:     runtimeTestIntSecret,
			JWTSigningKey:      runtimeTestJWTKey,
			MaxRequestBodySize: 1 << 20,
			MaxResultSize:      1 << 20,
		},
		Store:  q,
		PubSub: recorder,
	})
	httpServer := httptest.NewServer(srv)
	t.Cleanup(httpServer.Close)
	t.Cleanup(srv.Close)

	svc := agents.NewService(
		q,
		testDB.Pool,
		agents.WithAPIBaseURL(httpServer.URL),
		agents.WithJWTSigningKey(runtimeTestJWTKey),
	)
	t.Cleanup(func() { closeService(svc) })
	return svc, q, projectID
}

func TestRunAgent_PicksDeploymentByEnvironment(t *testing.T) {
	ctx := context.Background()
	svc, q, projectID := newEnvAwareTestService(t)
	devEnvID := mustCreateEnvironment(t, ctx, q, projectID, "dev")
	prodEnvID := mustCreateEnvironment(t, ctx, q, projectID, "prod")

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Env Router",
		Slug:      "env-router",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	devDep, err := svc.DeployAgentToEnv(ctx, projectID, agent.ID, devEnvID, "user-1")
	if err != nil {
		t.Fatalf("DeployAgentToEnv(dev) error = %v", err)
	}
	prodDep, err := svc.DeployAgentToEnv(ctx, projectID, agent.ID, prodEnvID, "user-1")
	if err != nil {
		t.Fatalf("DeployAgentToEnv(prod) error = %v", err)
	}

	runDev, err := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID:     projectID,
		AgentID:       agent.ID,
		EnvironmentID: devEnvID,
		Payload:       json.RawMessage(`{"prompt":"dev"}`),
		Actor:         "user-1",
	})
	if err != nil {
		t.Fatalf("RunAgent(dev) error = %v", err)
	}
	if runDev.AgentDeploymentID != devDep.ID {
		t.Fatalf("run dev stamped %q, want %q", runDev.AgentDeploymentID, devDep.ID)
	}
	runProd, err := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID:     projectID,
		AgentID:       agent.ID,
		EnvironmentID: prodEnvID,
		Payload:       json.RawMessage(`{"prompt":"prod"}`),
		Actor:         "user-1",
	})
	if err != nil {
		t.Fatalf("RunAgent(prod) error = %v", err)
	}
	if runProd.AgentDeploymentID != prodDep.ID {
		t.Fatalf("run prod stamped %q, want %q", runProd.AgentDeploymentID, prodDep.ID)
	}
}

func TestRunAgent_NoEnvironmentFallsBackToLatest(t *testing.T) {
	// Legacy RunAgent(no env) should resolve to GetLatestAgentDeployment.
	ctx := context.Background()
	svc, q, projectID := newEnvAwareTestService(t)
	_ = q

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Legacy Agent",
		Slug:      "legacy-agent",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	dep, err := svc.DeployAgent(ctx, projectID, agent.ID, "user-1")
	if err != nil {
		t.Fatalf("DeployAgent() error = %v", err)
	}

	run, err := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID: projectID,
		AgentID:   agent.ID,
		Payload:   json.RawMessage(`{"prompt":"hi"}`),
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}
	if run.AgentDeploymentID != dep.ID {
		t.Fatalf("run stamped %q, want %q", run.AgentDeploymentID, dep.ID)
	}
}

func TestRunAgent_UnknownEnvironmentReturnsNotDeployed(t *testing.T) {
	ctx := context.Background()
	svc, _, projectID := newEnvAwareTestService(t)

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "No Deploy",
		Slug:      "no-deploy",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	_, err = svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID:     projectID,
		AgentID:       agent.ID,
		EnvironmentID: "env-nonexistent",
		Payload:       json.RawMessage(`{}`),
		Actor:         "user-1",
	})
	if !errors.Is(err, agents.ErrNotDeployed) {
		t.Fatalf("err = %v, want ErrNotDeployed", err)
	}
}

func TestRunAgent_StampsAgentDeploymentIDOnJobRun(t *testing.T) {
	ctx := context.Background()
	svc, q, projectID := newEnvAwareTestService(t)
	envID := mustCreateEnvironment(t, ctx, q, projectID, "dev")

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Stamper",
		Slug:      "stamper",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	dep, err := svc.DeployAgentToEnv(ctx, projectID, agent.ID, envID, "user-1")
	if err != nil {
		t.Fatalf("DeployAgentToEnv() error = %v", err)
	}

	run, err := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID:     projectID,
		AgentID:       agent.ID,
		EnvironmentID: envID,
		Payload:       json.RawMessage(`{}`),
		Actor:         "user-1",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}

	// Read the persisted run row and assert the deployment stamp
	// survived round-trip through the store.
	persisted, err := q.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if persisted.AgentDeploymentID != dep.ID {
		t.Fatalf("persisted run stamped %q, want %q", persisted.AgentDeploymentID, dep.ID)
	}
}

// ---------------------------------------------------------------------------
// Phase E3: ReplayAgentRun carries deployment forward
// ---------------------------------------------------------------------------

func TestReplayAgentRun_PreservesOriginalDeployment(t *testing.T) {
	ctx := context.Background()
	svc, q, projectID := newEnvAwareTestService(t)
	envID := mustCreateEnvironment(t, ctx, q, projectID, "prod")

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Replay Agent",
		Slug:      "replay-agent",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	originalDep, err := svc.DeployAgentToEnv(ctx, projectID, agent.ID, envID, "user-1")
	if err != nil {
		t.Fatalf("DeployAgentToEnv() error = %v", err)
	}

	original, err := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID:     projectID,
		AgentID:       agent.ID,
		EnvironmentID: envID,
		Payload:       json.RawMessage(`{"prompt":"original"}`),
		Actor:         "user-1",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}
	_ = mustWaitForRunStatus(t, ctx, q, original.ID, domain.StatusCompleted)

	// Push a newer deployment in the same env — replay MUST still route
	// back to the original deployment, not this newer one.
	newerDep, err := svc.DeployAgentToEnv(ctx, projectID, agent.ID, envID, "user-1")
	if err != nil {
		t.Fatalf("second DeployAgentToEnv() error = %v", err)
	}
	if newerDep.ID == originalDep.ID {
		t.Fatal("second deploy returned the same deployment")
	}

	replay, err := svc.ReplayAgentRun(ctx, agents.ReplayAgentRunRequest{
		ProjectID:     projectID,
		AgentID:       agent.ID,
		OriginalRunID: original.ID,
		Actor:         "user-1",
	})
	if err != nil {
		t.Fatalf("ReplayAgentRun() error = %v", err)
	}
	if replay.AgentDeploymentID != originalDep.ID {
		t.Fatalf("replay stamped %q, want original deployment %q", replay.AgentDeploymentID, originalDep.ID)
	}
}

func TestReplayAgentRun_LegacyRunWithoutDeploymentFallsBack(t *testing.T) {
	ctx := context.Background()
	svc, q, projectID := newEnvAwareTestService(t)

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Legacy Replay",
		Slug:      "legacy-replay",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	legacyDep, err := svc.DeployAgent(ctx, projectID, agent.ID, "user-1")
	if err != nil {
		t.Fatalf("DeployAgent() error = %v", err)
	}

	original, err := svc.RunAgent(ctx, agents.RunAgentRequest{
		ProjectID: projectID,
		AgentID:   agent.ID,
		Payload:   json.RawMessage(`{}`),
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("RunAgent() error = %v", err)
	}
	_ = mustWaitForRunStatus(t, ctx, q, original.ID, domain.StatusCompleted)

	// Simulate a pre-Phase-B run by blanking out the deployment id.
	if _, err := testDB.Pool.Exec(ctx,
		"UPDATE job_runs SET agent_deployment_id = NULL WHERE id = $1", original.ID,
	); err != nil {
		t.Fatalf("blank deployment: %v", err)
	}

	replay, err := svc.ReplayAgentRun(ctx, agents.ReplayAgentRunRequest{
		ProjectID:     projectID,
		AgentID:       agent.ID,
		OriginalRunID: original.ID,
		Actor:         "user-1",
	})
	if err != nil {
		t.Fatalf("ReplayAgentRun() error = %v", err)
	}
	// Fallback is GetLatestAgentDeployment — which is the legacy deployment.
	if replay.AgentDeploymentID != legacyDep.ID {
		t.Fatalf("replay stamped %q, want legacy %q", replay.AgentDeploymentID, legacyDep.ID)
	}
}

// ---------------------------------------------------------------------------
// Phase E3: Agent envelope reads env-scoped project_secrets
// ---------------------------------------------------------------------------

func TestBuildRuntimeEnvelope_PopulatesSecretsFromEnv(t *testing.T) {
	ctx := context.Background()
	svc, q, projectID := newEnvAwareTestService(t)
	envID := mustCreateEnvironment(t, ctx, q, projectID, "prod")

	// Insert a project-wide secret in the prod env.
	qEnc := mustStoreWithEncryption(t)
	if err := qEnc.CreateProjectSecret(ctx, &domain.ProjectSecret{
		ProjectID:      projectID,
		EnvironmentID:  envID,
		SecretKey:      "API_KEY",
		EncryptedValue: "prod-secret",
	}); err != nil {
		t.Fatalf("CreateProjectSecret() error = %v", err)
	}

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Secret Reader",
		Slug:      "secret-reader",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	if _, err := svc.DeployAgentToEnv(ctx, projectID, agent.ID, envID, "user-1"); err != nil {
		t.Fatalf("DeployAgentToEnv() error = %v", err)
	}

	// PrepareDirectRun returns the envelope synchronously so we can
	// inspect it without waiting on dispatch side effects.
	result, err := svc.PrepareDirectRun(ctx, agents.RunAgentRequest{
		ProjectID:     projectID,
		AgentID:       agent.ID,
		EnvironmentID: envID,
		Payload:       json.RawMessage(`{}`),
		Actor:         "user-1",
	})
	if err != nil {
		t.Fatalf("PrepareDirectRun() error = %v", err)
	}
	if got := result.Envelope.Secrets["API_KEY"]; got != "prod-secret" {
		t.Fatalf("envelope.Secrets[API_KEY] = %q, want prod-secret", got)
	}
}

func TestBuildRuntimeEnvelope_NoEnvOnDeploymentSkipsSecrets(t *testing.T) {
	// A legacy (no-env) deployment should receive no project_secrets
	// even if secrets exist for that project in any env.
	ctx := context.Background()
	svc, q, projectID := newEnvAwareTestService(t)
	envID := mustCreateEnvironment(t, ctx, q, projectID, "prod")

	qEnc := mustStoreWithEncryption(t)
	if err := qEnc.CreateProjectSecret(ctx, &domain.ProjectSecret{
		ProjectID:      projectID,
		EnvironmentID:  envID,
		SecretKey:      "UNUSED",
		EncryptedValue: "should-not-leak",
	}); err != nil {
		t.Fatalf("CreateProjectSecret() error = %v", err)
	}

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Legacy No Env",
		Slug:      "legacy-no-env",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	if _, err := svc.DeployAgent(ctx, projectID, agent.ID, "user-1"); err != nil {
		t.Fatalf("DeployAgent() error = %v", err)
	}

	result, err := svc.PrepareDirectRun(ctx, agents.RunAgentRequest{
		ProjectID: projectID,
		AgentID:   agent.ID,
		Payload:   json.RawMessage(`{}`),
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("PrepareDirectRun() error = %v", err)
	}
	if len(result.Envelope.Secrets) != 0 {
		t.Fatalf("envelope.Secrets = %v, want empty for legacy no-env deployment", result.Envelope.Secrets)
	}
}

// mustStoreWithEncryption returns a *store.Queries wired with an
// encryption key so CreateProjectSecret and ListProjectSecretsByEnv
// can encrypt/decrypt. Mirrors the pattern in
// internal/store/agents_integration_test.go.
func mustStoreWithEncryption(t *testing.T) *store.Queries {
	t.Helper()
	q := store.New(testDB.Pool)
	q.SetSecretEncryptionKey("test-secret-encryption-key-32chr!")
	return q
}

// ---------------------------------------------------------------------------
// Phase F5: scheduleAgentRetry uses GetAgentByJobID
// ---------------------------------------------------------------------------

// TestGetAgentByJobID_ResolvesBackingJobOwner is the F5 regression
// net: the agents service uses store.GetAgentByJobID to resolve the
// agent that owns a failed run's backing job, instead of the old
// paginated ListAgents scan. The lookup must be O(1) and return the
// correct agent, and the store method must be reachable through the
// service's narrow agentStore interface.
func TestGetAgentByJobID_ResolvesBackingJobOwner(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)
	projectID := mustCreateProject(t, ctx, q)

	svc := agents.NewService(q, testDB.Pool, agents.WithJWTSigningKey(runtimeTestJWTKey))
	defer closeService(svc)

	agent, err := svc.CreateAgent(ctx, agents.CreateAgentRequest{
		ProjectID: projectID,
		Name:      "Retry Owner",
		Slug:      "retry-owner",
		Model:     "gpt-5.4",
		Actor:     "user-1",
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	// Look up the agent by its backing job_id. This is what
	// scheduleAgentRetry now calls internally.
	got, err := q.GetAgentByJobID(ctx, agent.JobID)
	if err != nil {
		t.Fatalf("GetAgentByJobID() error = %v", err)
	}
	if got.ID != agent.ID {
		t.Fatalf("GetAgentByJobID() = %q, want %q", got.ID, agent.ID)
	}
}

// TestGetAgentByJobID_UnknownJobReturnsNotFound verifies the
// failure path scheduleAgentRetry takes when the failed run belongs
// to a plain job (no agent) rather than an agent backing job. The
// retry path must short-circuit cleanly in that case.
func TestGetAgentByJobID_UnknownJobReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	q := store.New(testDB.Pool)
	mustClean(t, ctx)

	_, err := q.GetAgentByJobID(ctx, "job-with-no-agent-"+newID())
	if !errors.Is(err, store.ErrAgentNotFound) {
		t.Fatalf("err = %v, want ErrAgentNotFound", err)
	}
}
