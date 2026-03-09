//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"strait/internal/api"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
)

var (
	testEnv    *testutil.TestEnv
	testStore  *store.Queries
	testQueue  *queue.PostgresQueue
	testServer *api.Server
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	testEnv, err = testutil.SetupTestEnv(ctx, "../../migrations")
	if err != nil {
		log.Fatalf("setup test env: %v", err)
	}

	testStore = store.New(testEnv.DB.Pool)
	testStore.SetSecretEncryptionKey("test-encryption-key-32bytes!!!!")
	testQueue = queue.NewPostgresQueue(testEnv.DB.Pool)
	testServer = api.NewServer(api.ServerDeps{
		Config: &config.Config{
			InternalSecret:           "test-secret",
			JWTSigningKey:            "test-jwt-key-must-be-at-least-32-chars-long",
			SecretEncryptionKey:      "test-encryption-key-32bytes!!!!",
			RateLimitRequests:        0,
			RateLimitWindow:          time.Minute,
			TriggerRateLimitRequests: 0,
			TriggerRateLimitWindow:   time.Minute,
			CORSAllowedOrigins:       []string{"*"},
			CORSAllowCredentials:     false,
			FFJobTags:                true,
			FFPayloadValidation:      true,
			FFRunAnnotations:         true,
			FFSecretInjection:        true,
			FFRunReplay:              true,
			FFDryRun:                 true,
			FFBatchJobOps:            true,
			FFEnvironments:           true,
			FFJobGroups:              true,
			FFJobDependencies:        true,
			FFJobHealthScoring:       true,
			FFExecutionTracing:       true,
			FFRunDLQ:                 true,
			FFDebugBundle:            true,
			FFRunContinuation:        true,
			FFAdaptiveTimeout:        true,
			FFCheckpoints:            true,
		},
		Store: testStore,
		Queue: testQueue,
	})

	code := m.Run()
	testEnv.Cleanup(ctx)
	os.Exit(code)
}

func mustClean(t *testing.T) {
	t.Helper()
	if err := testEnv.DB.CleanTables(context.Background()); err != nil {
		t.Fatalf("clean tables: %v", err)
	}
}

func authedRequest(method, path, body string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	req.Header.Set("X-Internal-Secret", "test-secret")
	req.Header.Set("Content-Type", "application/json")
	return req
}

func doRequest(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	testServer.ServeHTTP(w, authedRequest(method, path, body))
	return w
}

func doSDKRequest(t *testing.T, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testServer.ServeHTTP(w, req)
	return w
}

func mustDecodeObject(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func mustDecodeList(t *testing.T, w *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	var envelope struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return envelope.Data
}

func asString(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	v, ok := m[key].(string)
	if !ok {
		t.Fatalf("%s is not a string: %T", key, m[key])
	}
	return v
}

func asBool(t *testing.T, m map[string]any, key string) bool {
	t.Helper()
	v, ok := m[key].(bool)
	if !ok {
		t.Fatalf("%s is not a bool: %T", key, m[key])
	}
	return v
}

func asInt(t *testing.T, m map[string]any, key string) int {
	t.Helper()
	v, ok := m[key].(float64)
	if !ok {
		t.Fatalf("%s is not a number: %T", key, m[key])
	}
	return int(v)
}

func asObject(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	v, ok := m[key].(map[string]any)
	if !ok {
		t.Fatalf("%s is not an object: %T", key, m[key])
	}
	return v
}

func newID() string {
	return uuid.Must(uuid.NewV7()).String()
}

func createJob(t *testing.T, projectID, name, slug string) map[string]any {
	t.Helper()
	body := fmt.Sprintf(`{"project_id":"%s","name":"%s","slug":"%s","description":"%s description","cron":"*/5 * * * *","payload_schema":{"type":"object"},"endpoint_url":"https://example.com/%s","max_attempts":3,"timeout_secs":60,"run_ttl_secs":600}`,
		projectID, name, slug, name, slug)
	w := doRequest(t, http.MethodPost, "/v1/jobs/", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create job status = %d, body = %s", w.Code, w.Body.String())
	}
	return mustDecodeObject(t, w)
}

func triggerJob(t *testing.T, jobID, body string, idempotencyKey string) map[string]any {
	t.Helper()
	req := authedRequest(http.MethodPost, "/v1/jobs/"+jobID+"/trigger", body)
	if idempotencyKey != "" {
		req.Header.Set("X-Idempotency-Key", idempotencyKey)
	}
	w := httptest.NewRecorder()
	testServer.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("trigger status = %d, body = %s", w.Code, w.Body.String())
	}
	return mustDecodeObject(t, w)
}

func TestE2E_CreateJob(t *testing.T) {
	mustClean(t)

	projectID := "proj-create-" + newID()
	resp := createJob(t, projectID, "create-job", "create-job-"+newID())

	if asString(t, resp, "id") == "" {
		t.Fatal("expected non-empty id")
	}
	if asInt(t, resp, "version") != 1 {
		t.Fatalf("expected version 1, got %d", asInt(t, resp, "version"))
	}
	if !asBool(t, resp, "enabled") {
		t.Fatal("expected enabled=true")
	}
}

func TestE2E_GetJob(t *testing.T) {
	mustClean(t)

	projectID := "proj-get-" + newID()
	slug := "get-job-" + newID()
	created := createJob(t, projectID, "Get Job", slug)
	jobID := asString(t, created, "id")

	w := doRequest(t, http.MethodGet, "/v1/jobs/"+jobID+"/", "")
	if w.Code != http.StatusOK {
		t.Fatalf("get job status = %d, body = %s", w.Code, w.Body.String())
	}

	resp := mustDecodeObject(t, w)
	if asString(t, resp, "id") != jobID {
		t.Fatalf("expected id %s, got %s", jobID, asString(t, resp, "id"))
	}
	if asString(t, resp, "project_id") != projectID {
		t.Fatalf("expected project_id %s", projectID)
	}
	if asString(t, resp, "slug") != slug {
		t.Fatalf("expected slug %s", slug)
	}
	if asString(t, resp, "endpoint_url") == "" {
		t.Fatal("expected endpoint_url")
	}
}

func TestE2E_ListJobs(t *testing.T) {
	mustClean(t)

	projectID := "proj-list-jobs-" + newID()
	createJob(t, projectID, "job-one", "job-one-"+newID())
	createJob(t, projectID, "job-two", "job-two-"+newID())
	createJob(t, projectID, "job-three", "job-three-"+newID())

	w := doRequest(t, http.MethodGet, "/v1/jobs/?project_id="+projectID, "")
	if w.Code != http.StatusOK {
		t.Fatalf("list jobs status = %d, body = %s", w.Code, w.Body.String())
	}

	resp := mustDecodeList(t, w)
	if len(resp) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(resp))
	}
}

func TestE2E_ListJobsByTag(t *testing.T) {
	mustClean(t)

	projectID := "proj-list-jobs-tag-" + newID()
	teamCoreSlug := "job-core-" + newID()
	teamOpsSlug := "job-ops-" + newID()

	bodyCore := fmt.Sprintf(`{"project_id":"%s","name":"Job Core","slug":"%s","endpoint_url":"https://example.com/%s","max_attempts":3,"timeout_secs":60,"tags":{"team":"core","service":"api"}}`, projectID, teamCoreSlug, teamCoreSlug)
	bodyOps := fmt.Sprintf(`{"project_id":"%s","name":"Job Ops","slug":"%s","endpoint_url":"https://example.com/%s","max_attempts":3,"timeout_secs":60,"tags":{"team":"ops"}}`, projectID, teamOpsSlug, teamOpsSlug)

	w := doRequest(t, http.MethodPost, "/v1/jobs/", bodyCore)
	if w.Code != http.StatusCreated {
		t.Fatalf("create tagged core job status = %d, body = %s", w.Code, w.Body.String())
	}
	w = doRequest(t, http.MethodPost, "/v1/jobs/", bodyOps)
	if w.Code != http.StatusCreated {
		t.Fatalf("create tagged ops job status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doRequest(t, http.MethodGet, "/v1/jobs/?project_id="+projectID+"&tag_key=team&tag_value=core", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list jobs by tag status = %d, body = %s", w.Code, w.Body.String())
	}
	resp := mustDecodeList(t, w)
	if len(resp) != 1 {
		t.Fatalf("expected 1 tagged job, got %d", len(resp))
	}
	tags := asObject(t, resp[0], "tags")
	if asString(t, tags, "team") != "core" {
		t.Fatalf("expected team tag core, got %s", asString(t, tags, "team"))
	}
}

func TestE2E_UpdateJob(t *testing.T) {
	mustClean(t)

	projectID := "proj-update-" + newID()
	created := createJob(t, projectID, "Old Name", "update-job-"+newID())
	jobID := asString(t, created, "id")

	w := doRequest(t, http.MethodPatch, "/v1/jobs/"+jobID+"/", `{"name":"New Name"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("update job status = %d, body = %s", w.Code, w.Body.String())
	}

	resp := mustDecodeObject(t, w)
	if asString(t, resp, "name") != "New Name" {
		t.Fatalf("expected updated name, got %s", asString(t, resp, "name"))
	}
	if asInt(t, resp, "version") != 2 {
		t.Fatalf("expected version 2, got %d", asInt(t, resp, "version"))
	}
}

func TestE2E_DeleteJob(t *testing.T) {
	mustClean(t)

	projectID := "proj-delete-" + newID()
	created := createJob(t, projectID, "Delete Job", "delete-job-"+newID())
	jobID := asString(t, created, "id")

	w := doRequest(t, http.MethodDelete, "/v1/jobs/"+jobID+"/", "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete job status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doRequest(t, http.MethodGet, "/v1/jobs/"+jobID+"/", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("get deleted job status = %d, want 404, body = %s", w.Code, w.Body.String())
	}
}

func TestE2E_TriggerJob(t *testing.T) {
	mustClean(t)

	projectID := "proj-trigger-" + newID()
	job := createJob(t, projectID, "Trigger Job", "trigger-job-"+newID())
	jobID := asString(t, job, "id")

	resp := triggerJob(t, jobID, `{"payload":{"k":"v"}}`, "")
	if asString(t, resp, "id") == "" {
		t.Fatal("expected run id")
	}
	if asString(t, resp, "status") != string(domain.StatusQueued) {
		t.Fatalf("expected queued status, got %s", asString(t, resp, "status"))
	}
	if asString(t, resp, "run_token") == "" {
		t.Fatal("expected run_token")
	}
}

func TestE2E_TriggerAndVerifyRun(t *testing.T) {
	mustClean(t)

	projectID := "proj-trigger-verify-" + newID()
	job := createJob(t, projectID, "Verify Run", "verify-run-"+newID())
	jobID := asString(t, job, "id")

	triggerResp := triggerJob(t, jobID, `{"payload":{"answer":42}}`, "")
	runID := asString(t, triggerResp, "id")

	w := doRequest(t, http.MethodGet, "/v1/runs/"+runID+"/", "")
	if w.Code != http.StatusOK {
		t.Fatalf("get run status = %d, body = %s", w.Code, w.Body.String())
	}

	run := mustDecodeObject(t, w)
	if asString(t, run, "status") != string(domain.StatusQueued) {
		t.Fatalf("expected queued status, got %s", asString(t, run, "status"))
	}
	if asString(t, run, "job_id") != jobID {
		t.Fatalf("expected job_id %s", jobID)
	}
	if asString(t, run, "project_id") != projectID {
		t.Fatalf("expected project_id %s", projectID)
	}
	payload := asObject(t, run, "payload")
	if int(payload["answer"].(float64)) != 42 {
		t.Fatalf("expected payload answer=42, got %v", payload["answer"])
	}
	if asInt(t, run, "job_version") != 1 {
		t.Fatalf("expected job_version=1, got %d", asInt(t, run, "job_version"))
	}
}

func TestE2E_FullLifecycle(t *testing.T) {
	mustClean(t)

	projectID := "proj-full-lifecycle-" + newID()
	job := createJob(t, projectID, "Full Lifecycle", "full-lifecycle-"+newID())
	triggerResp := triggerJob(t, asString(t, job, "id"), `{"payload":{"flow":"full"}}`, "")
	runID := asString(t, triggerResp, "id")

	w := doRequest(t, http.MethodGet, "/v1/runs/"+runID+"/", "")
	if w.Code != http.StatusOK {
		t.Fatalf("get run status = %d, body = %s", w.Code, w.Body.String())
	}
	run := mustDecodeObject(t, w)
	if asString(t, run, "status") != string(domain.StatusQueued) {
		t.Fatalf("expected queued status, got %s", asString(t, run, "status"))
	}

	w = doRequest(t, http.MethodGet, "/v1/runs/?project_id="+projectID, "")
	if w.Code != http.StatusOK {
		t.Fatalf("list runs status = %d, body = %s", w.Code, w.Body.String())
	}
	runs := mustDecodeList(t, w)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if asString(t, runs[0], "id") != runID {
		t.Fatalf("expected run id %s in list", runID)
	}

	w = doRequest(t, http.MethodGet, "/v1/stats", "")
	if w.Code != http.StatusOK {
		t.Fatalf("stats status = %d, body = %s", w.Code, w.Body.String())
	}
	stats := mustDecodeObject(t, w)
	if asInt(t, stats, "queued") != 1 {
		t.Fatalf("expected queued=1, got %d", asInt(t, stats, "queued"))
	}
}

func TestE2E_IdempotentTrigger(t *testing.T) {
	mustClean(t)

	projectID := "proj-idempotent-" + newID()
	job := createJob(t, projectID, "Idempotent", "idempotent-"+newID())
	jobID := asString(t, job, "id")
	idempotencyKey := "idem-" + newID()

	first := triggerJob(t, jobID, `{"payload":{"x":1}}`, idempotencyKey)
	second := triggerJob(t, jobID, `{"payload":{"x":2}}`, idempotencyKey)

	if asString(t, first, "id") != asString(t, second, "id") {
		t.Fatalf("expected same run id, got %s and %s", asString(t, first, "id"), asString(t, second, "id"))
	}

	w := doRequest(t, http.MethodGet, "/v1/runs/?project_id="+projectID, "")
	if w.Code != http.StatusOK {
		t.Fatalf("list runs status = %d, body = %s", w.Code, w.Body.String())
	}
	runs := mustDecodeList(t, w)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run from idempotent trigger, got %d", len(runs))
	}
}

func TestE2E_DelayedRun(t *testing.T) {
	mustClean(t)

	projectID := "proj-delayed-" + newID()
	job := createJob(t, projectID, "Delayed", "delayed-"+newID())
	jobID := asString(t, job, "id")

	scheduledAt := time.Now().UTC().Add(30 * time.Minute).Round(time.Second)
	body := fmt.Sprintf(`{"payload":{"kind":"delayed"},"scheduled_at":"%s"}`, scheduledAt.Format(time.RFC3339))
	triggerResp := triggerJob(t, jobID, body, "")
	if asString(t, triggerResp, "status") != string(domain.StatusDelayed) {
		t.Fatalf("expected delayed status, got %s", asString(t, triggerResp, "status"))
	}

	runID := asString(t, triggerResp, "id")
	w := doRequest(t, http.MethodGet, "/v1/runs/"+runID+"/", "")
	if w.Code != http.StatusOK {
		t.Fatalf("get delayed run status = %d, body = %s", w.Code, w.Body.String())
	}
	run := mustDecodeObject(t, w)
	gotScheduled := asString(t, run, "scheduled_at")
	parsedScheduled, err := time.Parse(time.RFC3339, gotScheduled)
	if err != nil {
		t.Fatalf("parse scheduled_at: %v", err)
	}
	if parsedScheduled.Sub(scheduledAt) > time.Second || scheduledAt.Sub(parsedScheduled) > time.Second {
		t.Fatalf("scheduled_at mismatch: got %s want %s", parsedScheduled.Format(time.RFC3339), scheduledAt.Format(time.RFC3339))
	}
}

func TestE2E_PriorityOrdering(t *testing.T) {
	mustClean(t)

	projectID := "proj-priority-" + newID()
	job := createJob(t, projectID, "Priority", "priority-"+newID())
	jobID := asString(t, job, "id")

	run0 := triggerJob(t, jobID, `{"payload":{},"priority":0}`, "")
	run10 := triggerJob(t, jobID, `{"payload":{},"priority":10}`, "")
	run5 := triggerJob(t, jobID, `{"payload":{},"priority":5}`, "")

	_ = run0
	_ = run5

	dequeued, err := testQueue.Dequeue(context.Background())
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if dequeued == nil {
		t.Fatal("expected dequeued run")
	}
	if dequeued.Priority != 10 {
		t.Fatalf("expected priority 10 first, got %d", dequeued.Priority)
	}
	if dequeued.ID != asString(t, run10, "id") {
		t.Fatalf("expected run %s first, got %s", asString(t, run10, "id"), dequeued.ID)
	}
}

func TestE2E_ListRunsByProject(t *testing.T) {
	mustClean(t)

	projectID := "proj-list-runs-" + newID()
	job1 := createJob(t, projectID, "Runs A", "runs-a-"+newID())
	job2 := createJob(t, projectID, "Runs B", "runs-b-"+newID())
	triggerJob(t, asString(t, job1, "id"), `{"payload":{"j":1}}`, "")
	triggerJob(t, asString(t, job2, "id"), `{"payload":{"j":2}}`, "")

	w := doRequest(t, http.MethodGet, "/v1/runs/?project_id="+projectID, "")
	if w.Code != http.StatusOK {
		t.Fatalf("list runs status = %d, body = %s", w.Code, w.Body.String())
	}
	runs := mustDecodeList(t, w)
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
}

func TestE2E_ListRunsFilterByStatus(t *testing.T) {
	mustClean(t)

	projectID := "proj-filter-status-" + newID()
	job := createJob(t, projectID, "Filter Status", "filter-status-"+newID())
	jobID := asString(t, job, "id")

	triggerJob(t, jobID, `{"payload":{"run":1}}`, "")
	cancelRun := triggerJob(t, jobID, `{"payload":{"run":2}}`, "")
	cancelRunID := asString(t, cancelRun, "id")

	err := testStore.UpdateRunStatus(context.Background(), cancelRunID, domain.StatusQueued, domain.StatusCanceled, map[string]any{
		"finished_at": time.Now().UTC(),
		"error":       "canceled in e2e",
	})
	if err != nil {
		t.Fatalf("update run status: %v", err)
	}

	w := doRequest(t, http.MethodGet, "/v1/runs/?project_id="+projectID+"&status="+string(domain.StatusCanceled), "")
	if w.Code != http.StatusOK {
		t.Fatalf("list filtered runs status = %d, body = %s", w.Code, w.Body.String())
	}
	runs := mustDecodeList(t, w)
	if len(runs) != 1 {
		t.Fatalf("expected 1 canceled run, got %d", len(runs))
	}
	if asString(t, runs[0], "id") != cancelRunID {
		t.Fatalf("expected canceled run id %s, got %s", cancelRunID, asString(t, runs[0], "id"))
	}
}

func TestE2E_ReplayRun(t *testing.T) {
	mustClean(t)

	projectID := "proj-replay-" + newID()
	job := createJob(t, projectID, "Replay", "replay-"+newID())
	jobID := asString(t, job, "id")
	idempotencyKey := "idem-" + newID()
	original := triggerJob(t, jobID, `{"payload":{"replay":true}}`, idempotencyKey)
	originalRunID := asString(t, original, "id")

	err := testStore.UpdateRunStatus(context.Background(), originalRunID, domain.StatusQueued, domain.StatusDequeued, map[string]any{
		"started_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("update run status to dequeued: %v", err)
	}

	err = testStore.UpdateRunStatus(context.Background(), originalRunID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{
		"started_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("update run status to executing: %v", err)
	}

	err = testStore.UpdateRunStatus(context.Background(), originalRunID, domain.StatusExecuting, domain.StatusFailed, map[string]any{
		"finished_at": time.Now().UTC(),
		"error":       "forced failure for replay",
	})
	if err != nil {
		t.Fatalf("update run status to failed: %v", err)
	}

	w := doRequest(t, http.MethodPost, "/v1/runs/"+originalRunID+"/replay", "")
	if w.Code != http.StatusCreated {
		t.Fatalf("replay status = %d, body = %s", w.Code, w.Body.String())
	}
	replay := mustDecodeObject(t, w)

	replayRunID := asString(t, replay, "id")
	if replayRunID == "" || replayRunID == originalRunID {
		t.Fatalf("expected distinct replay run id, got %q", replayRunID)
	}
	if asString(t, replay, "status") != string(domain.StatusQueued) {
		t.Fatalf("expected replay status queued, got %s", asString(t, replay, "status"))
	}
	// Replays do NOT copy the original idempotency key to avoid conflicts
	// with active runs sharing the same key.
	if replayKey, ok := replay["idempotency_key"].(string); ok && replayKey != "" {
		t.Fatalf("expected replay idempotency key to be empty, got %q", replayKey)
	}

	lw := doRequest(t, http.MethodGet, "/v1/runs/?project_id="+projectID, "")
	if lw.Code != http.StatusOK {
		t.Fatalf("list runs status = %d, body = %s", lw.Code, lw.Body.String())
	}
	runs := mustDecodeList(t, lw)
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs after replay, got %d", len(runs))
	}
}

func TestE2E_ListRunsPagination(t *testing.T) {
	mustClean(t)

	projectID := "proj-pagination-" + newID()
	job := createJob(t, projectID, "Pagination", "pagination-"+newID())
	jobID := asString(t, job, "id")

	for i := range 5 {
		triggerJob(t, jobID, fmt.Sprintf(`{"payload":{"idx":%d}}`, i), "")
	}

	w := doRequest(t, http.MethodGet, "/v1/runs/?project_id="+projectID+"&limit=2", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list runs page1 status = %d, body = %s", w.Code, w.Body.String())
	}
	page1 := mustDecodeList(t, w)
	if len(page1) != 2 {
		t.Fatalf("expected 2 runs in page1, got %d", len(page1))
	}

	cursor := asString(t, page1[1], "created_at")
	w = doRequest(t, http.MethodGet, "/v1/runs/?project_id="+projectID+"&limit=2&cursor="+url.QueryEscape(cursor), "")
	if w.Code != http.StatusOK {
		t.Fatalf("list runs page2 status = %d, body = %s", w.Code, w.Body.String())
	}
	page2 := mustDecodeList(t, w)
	if len(page2) != 2 {
		t.Fatalf("expected 2 runs in page2, got %d", len(page2))
	}

	if asString(t, page1[0], "id") == asString(t, page2[0], "id") {
		t.Fatal("expected different runs across pages")
	}
}

func TestE2E_RunEvents(t *testing.T) {
	mustClean(t)

	projectID := "proj-events-" + newID()
	job := createJob(t, projectID, "Events", "events-"+newID())
	triggerResp := triggerJob(t, asString(t, job, "id"), `{"payload":{"events":true}}`, "")
	runID := asString(t, triggerResp, "id")
	runToken := asString(t, triggerResp, "run_token")

	w := doSDKRequest(t, http.MethodPost, "/sdk/v1/runs/"+runID+"/log", runToken, `{"type":"log","level":"info","message":"Processing started","data":{"step":1}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("sdk log status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doRequest(t, http.MethodGet, "/v1/runs/"+runID+"/events", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list run events status = %d, body = %s", w.Code, w.Body.String())
	}
	events := mustDecodeList(t, w)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if asString(t, events[0], "message") != "Processing started" {
		t.Fatalf("expected event message, got %s", asString(t, events[0], "message"))
	}
}

func TestE2E_RunAnnotations(t *testing.T) {
	mustClean(t)

	projectID := "proj-annotations-" + newID()
	job := createJob(t, projectID, "Annotations", "annotations-"+newID())
	triggerResp := triggerJob(t, asString(t, job, "id"), `{"payload":{"annotations":true}}`, "")
	runID := asString(t, triggerResp, "id")
	runToken := asString(t, triggerResp, "run_token")

	w := doSDKRequest(t, http.MethodPost, "/sdk/v1/runs/"+runID+"/annotate", runToken, `{"annotations":{"env":"prod","region":"eu"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("sdk annotate status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doRequest(t, http.MethodGet, "/v1/runs/"+runID+"/", "")
	if w.Code != http.StatusOK {
		t.Fatalf("get run status = %d, body = %s", w.Code, w.Body.String())
	}
	run := mustDecodeObject(t, w)
	metadataRaw, ok := run["metadata"]
	if !ok {
		t.Fatal("expected metadata field in run response")
	}
	metadata, ok := metadataRaw.(map[string]any)
	if !ok {
		t.Fatalf("metadata type = %T, want map[string]any", metadataRaw)
	}
	if asString(t, metadata, "env") != "prod" || asString(t, metadata, "region") != "eu" {
		t.Fatalf("metadata = %+v, want env=prod region=eu", metadata)
	}
}

func TestE2E_ListRunsFilterByMetadata(t *testing.T) {
	mustClean(t)

	projectID := "proj-filter-metadata-" + newID()
	job := createJob(t, projectID, "Filter Metadata", "filter-metadata-"+newID())
	jobID := asString(t, job, "id")

	prodRun := triggerJob(t, jobID, `{"payload":{"run":"prod"}}`, "")
	prodRunID := asString(t, prodRun, "id")
	prodRunToken := asString(t, prodRun, "run_token")

	stageRun := triggerJob(t, jobID, `{"payload":{"run":"stage"}}`, "")
	stageRunID := asString(t, stageRun, "id")
	stageRunToken := asString(t, stageRun, "run_token")

	w := doSDKRequest(t, http.MethodPost, "/sdk/v1/runs/"+prodRunID+"/annotate", prodRunToken, `{"annotations":{"env":"prod","region":"eu"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("sdk annotate prod status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doSDKRequest(t, http.MethodPost, "/sdk/v1/runs/"+stageRunID+"/annotate", stageRunToken, `{"annotations":{"env":"stage"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("sdk annotate stage status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doRequest(t, http.MethodGet, "/v1/runs/?project_id="+projectID+"&metadata_key=env&metadata_value=prod", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list filtered runs status = %d, body = %s", w.Code, w.Body.String())
	}
	runs := mustDecodeList(t, w)
	if len(runs) != 1 {
		t.Fatalf("expected 1 prod run, got %d", len(runs))
	}
	if asString(t, runs[0], "id") != prodRunID {
		t.Fatalf("expected run id %s, got %s", prodRunID, asString(t, runs[0], "id"))
	}

	w = doRequest(t, http.MethodGet, "/v1/runs/?project_id="+projectID+"&metadata_key=env", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list metadata key-only runs status = %d, body = %s", w.Code, w.Body.String())
	}
	runs = mustDecodeList(t, w)
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs with env metadata key, got %d", len(runs))
	}
}

func TestE2E_Health(t *testing.T) {
	mustClean(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	testServer.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", w.Code, w.Body.String())
	}
	resp := mustDecodeObject(t, w)
	if asString(t, resp, "status") != "ok" {
		t.Fatalf("expected status=ok, got %s", asString(t, resp, "status"))
	}
}

func TestE2E_HealthReady(t *testing.T) {
	mustClean(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	testServer.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("health ready status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestE2E_Stats(t *testing.T) {
	mustClean(t)

	projectID := "proj-stats-" + newID()
	job := createJob(t, projectID, "Stats", "stats-"+newID())
	jobID := asString(t, job, "id")

	triggerJob(t, jobID, `{"payload":{"kind":"queued"}}`, "")
	scheduledAt := time.Now().UTC().Add(45 * time.Minute).Format(time.RFC3339)
	triggerJob(t, jobID, fmt.Sprintf(`{"payload":{"kind":"delayed"},"scheduled_at":"%s"}`, scheduledAt), "")

	w := doRequest(t, http.MethodGet, "/v1/stats", "")
	if w.Code != http.StatusOK {
		t.Fatalf("stats status = %d, body = %s", w.Code, w.Body.String())
	}
	stats := mustDecodeObject(t, w)
	if asInt(t, stats, "queued") != 1 {
		t.Fatalf("expected queued=1, got %d", asInt(t, stats, "queued"))
	}
	if asInt(t, stats, "delayed") != 1 {
		t.Fatalf("expected delayed=1, got %d", asInt(t, stats, "delayed"))
	}
	if asInt(t, stats, "executing") != 0 {
		t.Fatalf("expected executing=0, got %d", asInt(t, stats, "executing"))
	}
}

func TestE2E_AuthRequired(t *testing.T) {
	mustClean(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/?project_id=proj", nil)
	testServer.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestE2E_AuthInvalidSecret(t *testing.T) {
	mustClean(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/?project_id=proj", nil)
	req.Header.Set("X-Internal-Secret", "wrong-secret")
	testServer.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestE2E_APIKeyLifecycle(t *testing.T) {
	mustClean(t)

	projectID := "proj-api-key-" + newID()
	createBody := fmt.Sprintf(`{"project_id":"%s","name":"e2e-key","scopes":["jobs:read","stats:read"]}`, projectID)
	w := doRequest(t, http.MethodPost, "/v1/api-keys/", createBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create api key status = %d, body = %s", w.Code, w.Body.String())
	}

	created := mustDecodeObject(t, w)
	apiKeyID := asString(t, created, "id")
	rawKey := asString(t, created, "key")

	statsReq := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	statsReq.Header.Set("Authorization", "Bearer "+rawKey)
	statsW := httptest.NewRecorder()
	testServer.ServeHTTP(statsW, statsReq)
	if statsW.Code != http.StatusOK {
		t.Fatalf("stats with api key status = %d, body = %s", statsW.Code, statsW.Body.String())
	}

	revokeW := doRequest(t, http.MethodDelete, "/v1/api-keys/"+apiKeyID, "")
	if revokeW.Code != http.StatusOK {
		t.Fatalf("revoke api key status = %d, body = %s", revokeW.Code, revokeW.Body.String())
	}

	revokedReq := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	revokedReq.Header.Set("Authorization", "Bearer "+rawKey)
	revokedW := httptest.NewRecorder()
	testServer.ServeHTTP(revokedW, revokedReq)
	if revokedW.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized for revoked key, got %d: %s", revokedW.Code, revokedW.Body.String())
	}
}

// ====================================================================
// Test hardening: E2E tests for new features
// ====================================================================

func TestE2E_ScopeEnforcement(t *testing.T) {
	mustClean(t)

	projectID := "proj-scope-enforce-" + newID()
	// Create a job first (via internal secret).
	created := createJob(t, projectID, "Scope Test Job", "scope-test-"+newID())
	jobID := asString(t, created, "id")

	// Create API key with ONLY jobs:read (no write, no trigger).
	keyBody := fmt.Sprintf(`{"project_id":"%s","name":"read-only","scopes":["jobs:read"]}`, projectID)
	kw := doRequest(t, http.MethodPost, "/v1/api-keys/", keyBody)
	if kw.Code != http.StatusCreated {
		t.Fatalf("create api key status = %d, body = %s", kw.Code, kw.Body.String())
	}
	keyResp := mustDecodeObject(t, kw)
	rawKey := asString(t, keyResp, "key")

	// GET job with read-only key — should succeed.
	getReq := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+jobID+"/", nil)
	getReq.Header.Set("Authorization", "Bearer "+rawKey)
	getW := httptest.NewRecorder()
	testServer.ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("GET job with jobs:read key: status = %d, body = %s", getW.Code, getW.Body.String())
	}

	// PATCH job with read-only key — should be 403.
	patchReq := httptest.NewRequest(http.MethodPatch, "/v1/jobs/"+jobID+"/", strings.NewReader(`{"name":"Hacked"}`))
	patchReq.Header.Set("Authorization", "Bearer "+rawKey)
	patchReq.Header.Set("Content-Type", "application/json")
	patchW := httptest.NewRecorder()
	testServer.ServeHTTP(patchW, patchReq)
	if patchW.Code != http.StatusForbidden {
		t.Fatalf("PATCH job with jobs:read key: status = %d, want 403, body = %s", patchW.Code, patchW.Body.String())
	}

	// POST trigger with read-only key — should be 403.
	triggerReq := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID+"/trigger", strings.NewReader(`{}`))
	triggerReq.Header.Set("Authorization", "Bearer "+rawKey)
	triggerReq.Header.Set("Content-Type", "application/json")
	triggerW := httptest.NewRecorder()
	testServer.ServeHTTP(triggerW, triggerReq)
	if triggerW.Code != http.StatusForbidden {
		t.Fatalf("trigger job with jobs:read key: status = %d, want 403, body = %s", triggerW.Code, triggerW.Body.String())
	}
}

func TestE2E_EmptyScopesFullAccess(t *testing.T) {
	mustClean(t)

	projectID := "proj-empty-scopes-" + newID()
	// Create API key with empty scopes (backwards compatible = full access).
	keyBody := fmt.Sprintf(`{"project_id":"%s","name":"full-access","scopes":[]}`, projectID)
	kw := doRequest(t, http.MethodPost, "/v1/api-keys/", keyBody)
	if kw.Code != http.StatusCreated {
		t.Fatalf("create api key status = %d, body = %s", kw.Code, kw.Body.String())
	}
	keyResp := mustDecodeObject(t, kw)
	rawKey := asString(t, keyResp, "key")

	// Stats should work.
	statsReq := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	statsReq.Header.Set("Authorization", "Bearer "+rawKey)
	statsW := httptest.NewRecorder()
	testServer.ServeHTTP(statsW, statsReq)
	if statsW.Code != http.StatusOK {
		t.Fatalf("stats with empty scopes key: status = %d, body = %s", statsW.Code, statsW.Body.String())
	}
}

func TestE2E_JobVersionID(t *testing.T) {
	mustClean(t)

	projectID := "proj-vid-e2e-" + newID()
	created := createJob(t, projectID, "VID Job", "vid-job-"+newID())

	vid1 := asString(t, created, "version_id")
	if vid1 == "" {
		t.Fatal("expected version_id on create")
	}
	if !strings.HasPrefix(vid1, "ver_") {
		t.Fatalf("version_id = %q, want prefix 'ver_'", vid1)
	}

	jobID := asString(t, created, "id")
	// Update job — should get new version_id.
	uw := doRequest(t, http.MethodPatch, "/v1/jobs/"+jobID+"/", `{"name":"VID Job Updated"}`)
	if uw.Code != http.StatusOK {
		t.Fatalf("update job status = %d, body = %s", uw.Code, uw.Body.String())
	}
	updated := mustDecodeObject(t, uw)
	vid2 := asString(t, updated, "version_id")
	if vid2 == "" {
		t.Fatal("expected version_id on update")
	}
	if vid1 == vid2 {
		t.Fatalf("version_id should change on update: %q == %q", vid1, vid2)
	}
}

func TestE2E_VersionPolicyDefault(t *testing.T) {
	mustClean(t)

	projectID := "proj-vpol-e2e-" + newID()
	created := createJob(t, projectID, "VPol Job", "vpol-job-"+newID())

	policy := asString(t, created, "version_policy")
	if policy != "pin" {
		t.Fatalf("default version_policy = %q, want 'pin'", policy)
	}
}

func TestE2E_UpdateJobVersionIncrement(t *testing.T) {
	mustClean(t)

	projectID := "proj-ver-inc-e2e-" + newID()
	created := createJob(t, projectID, "Inc Job", "inc-job-"+newID())
	jobID := asString(t, created, "id")

	if asInt(t, created, "version") != 1 {
		t.Fatalf("initial version = %d, want 1", asInt(t, created, "version"))
	}

	// First update.
	uw1 := doRequest(t, http.MethodPatch, "/v1/jobs/"+jobID+"/", `{"name":"Inc Job v2"}`)
	if uw1.Code != http.StatusOK {
		t.Fatalf("update 1 status = %d", uw1.Code)
	}
	r1 := mustDecodeObject(t, uw1)
	if asInt(t, r1, "version") != 2 {
		t.Fatalf("version after 1st update = %d, want 2", asInt(t, r1, "version"))
	}

	// Second update.
	uw2 := doRequest(t, http.MethodPatch, "/v1/jobs/"+jobID+"/", `{"name":"Inc Job v3"}`)
	if uw2.Code != http.StatusOK {
		t.Fatalf("update 2 status = %d", uw2.Code)
	}
	r2 := mustDecodeObject(t, uw2)
	if asInt(t, r2, "version") != 3 {
		t.Fatalf("version after 2nd update = %d, want 3", asInt(t, r2, "version"))
	}
}

func TestE2E_JobCreatedBy(t *testing.T) {
	mustClean(t)

	projectID := "proj-created-by-" + newID()
	slug := "created-by-" + newID()
	body := fmt.Sprintf(`{"project_id":"%s","name":"Created By Job","slug":"%s","endpoint_url":"https://example.com/%s","max_attempts":3,"timeout_secs":60}`, projectID, slug, slug)

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/", strings.NewReader(body))
	req.Header.Set("X-Internal-Secret", "test-secret")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Actor-Id", "user_leo_123")
	req.Header.Set("X-Actor-Email", "leo@example.com")
	req.Header.Set("X-Actor-Name", "Leonardo")

	w := httptest.NewRecorder()
	testServer.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create job status = %d, body = %s", w.Code, w.Body.String())
	}

	resp := mustDecodeObject(t, w)
	createdBy := asString(t, resp, "created_by")
	if createdBy != "user_leo_123" {
		t.Fatalf("created_by = %q, want %q", createdBy, "user_leo_123")
	}
}

func TestE2E_RolesLifecycle(t *testing.T) {
	mustClean(t)

	// Create role.
	createBody := `{"name":"e2e-deployer","description":"Can deploy things","permissions":["jobs:write","jobs:trigger","jobs:read"]}`
	cw := doRequest(t, http.MethodPost, "/v1/roles", createBody)
	if cw.Code != http.StatusCreated {
		t.Fatalf("create role status = %d, body = %s", cw.Code, cw.Body.String())
	}
	created := mustDecodeObject(t, cw)
	roleID := asString(t, created, "id")
	if roleID == "" {
		t.Fatal("expected role ID")
	}

	// List roles.
	lw := doRequest(t, http.MethodGet, "/v1/roles", "")
	if lw.Code != http.StatusOK {
		t.Fatalf("list roles status = %d", lw.Code)
	}
	roles := mustDecodeList(t, lw)
	found := false
	for _, r := range roles {
		if asString(t, r, "id") == roleID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created role %q not found in list", roleID)
	}

	// Get role.
	gw := doRequest(t, http.MethodGet, "/v1/roles/"+roleID, "")
	if gw.Code != http.StatusOK {
		t.Fatalf("get role status = %d, body = %s", gw.Code, gw.Body.String())
	}
	got := mustDecodeObject(t, gw)
	if asString(t, got, "name") != "e2e-deployer" {
		t.Fatalf("name = %q, want e2e-deployer", asString(t, got, "name"))
	}

	// Update role.
	updateBody := `{"name":"e2e-deployer-v2","description":"Updated","permissions":["jobs:write","jobs:trigger","jobs:read","runs:read"]}`
	uw := doRequest(t, http.MethodPatch, "/v1/roles/"+roleID, updateBody)
	if uw.Code != http.StatusOK {
		t.Fatalf("update role status = %d, body = %s", uw.Code, uw.Body.String())
	}

	// Assign member.
	assignBody := fmt.Sprintf(`{"user_id":"e2e-user-1","role_id":"%s"}`, roleID)
	aw := doRequest(t, http.MethodPost, "/v1/members", assignBody)
	if aw.Code != http.StatusCreated {
		t.Fatalf("assign member status = %d, body = %s", aw.Code, aw.Body.String())
	}

	// List members.
	mw := doRequest(t, http.MethodGet, "/v1/members", "")
	if mw.Code != http.StatusOK {
		t.Fatalf("list members status = %d", mw.Code)
	}

	// Remove member.
	rw := doRequest(t, http.MethodDelete, "/v1/members/e2e-user-1", "")
	if rw.Code != http.StatusNoContent {
		t.Fatalf("remove member status = %d, body = %s", rw.Code, rw.Body.String())
	}

	// Delete role.
	dw := doRequest(t, http.MethodDelete, "/v1/roles/"+roleID, "")
	if dw.Code != http.StatusNoContent {
		t.Fatalf("delete role status = %d, body = %s", dw.Code, dw.Body.String())
	}

	// Verify gone.
	gw2 := doRequest(t, http.MethodGet, "/v1/roles/"+roleID, "")
	if gw2.Code != http.StatusNotFound {
		t.Fatalf("get deleted role status = %d, want 404", gw2.Code)
	}
}

func TestE2E_TagFilteringWorkflows(t *testing.T) {
	mustClean(t)

	projectID := "proj-wf-tags-e2e-" + newID()
	slug1 := "wf-tagged-1-" + newID()
	slug2 := "wf-tagged-2-" + newID()

	body1 := fmt.Sprintf(`{"project_id":"%s","name":"WF Tagged 1","slug":"%s","enabled":true,"tags":{"team":"core","env":"prod"}}`, projectID, slug1)
	w1 := doRequest(t, http.MethodPost, "/v1/workflows/", body1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("create wf1 status = %d, body = %s", w1.Code, w1.Body.String())
	}

	body2 := fmt.Sprintf(`{"project_id":"%s","name":"WF Tagged 2","slug":"%s","enabled":true,"tags":{"team":"ops"}}`, projectID, slug2)
	w2 := doRequest(t, http.MethodPost, "/v1/workflows/", body2)
	if w2.Code != http.StatusCreated {
		t.Fatalf("create wf2 status = %d, body = %s", w2.Code, w2.Body.String())
	}

	// Filter by team=core.
	fw := doRequest(t, http.MethodGet, "/v1/workflows/?project_id="+projectID+"&tag_key=team&tag_value=core", "")
	if fw.Code != http.StatusOK {
		t.Fatalf("filter workflows status = %d, body = %s", fw.Code, fw.Body.String())
	}
	filtered := mustDecodeList(t, fw)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered workflow, got %d", len(filtered))
	}
}

// ── Idempotency E2E tests ──────────────────────────────────────────────

func TestE2E_IdempotencyKeyHitReturnsOriginal(t *testing.T) {
	mustClean(t)

	projectID := "proj-idem-hit-" + newID()
	job := createJob(t, projectID, "IdemHit", "idem-hit-"+newID())
	jobID := asString(t, job, "id")
	key := "idem-" + newID()

	first := triggerJob(t, jobID, `{"payload":{"x":1}}`, key)
	second := triggerJob(t, jobID, `{"payload":{"x":2}}`, key)

	if asString(t, first, "id") != asString(t, second, "id") {
		t.Fatalf("expected same run ID, got %s vs %s", asString(t, first, "id"), asString(t, second, "id"))
	}

	// Only 1 run should exist.
	lw := doRequest(t, http.MethodGet, "/v1/runs/?project_id="+projectID, "")
	if lw.Code != http.StatusOK {
		t.Fatalf("list runs status = %d", lw.Code)
	}
	runs := mustDecodeList(t, lw)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
}

func TestE2E_IdempotencyKeyPerJobScoping(t *testing.T) {
	mustClean(t)

	projectID := "proj-idem-scope-" + newID()
	jobA := createJob(t, projectID, "IdemA", "idem-a-"+newID())
	jobB := createJob(t, projectID, "IdemB", "idem-b-"+newID())
	key := "idem-shared-" + newID()

	runA := triggerJob(t, asString(t, jobA, "id"), `{"payload":{}}`, key)
	runB := triggerJob(t, asString(t, jobB, "id"), `{"payload":{}}`, key)

	if asString(t, runA, "id") == asString(t, runB, "id") {
		t.Fatalf("expected different run IDs for different jobs, both got %s", asString(t, runA, "id"))
	}
}

func TestE2E_IdempotencyKeyReusableAfterTerminal(t *testing.T) {
	// After a run reaches terminal status, both the DB partial unique index
	// and the app-level GetRunByIdempotencyKey query (with status filter)
	// allow key reuse. A new run should be created with the same key.
	mustClean(t)

	projectID := "proj-idem-reuse-" + newID()
	job := createJob(t, projectID, "IdemReuse", "idem-reuse-"+newID())
	jobID := asString(t, job, "id")
	key := "idem-" + newID()

	first := triggerJob(t, jobID, `{"payload":{}}`, key)
	firstID := asString(t, first, "id")

	// Transition to terminal.
	if err := testStore.UpdateRunStatus(context.Background(), firstID, domain.StatusQueued, domain.StatusDequeued, map[string]any{
		"started_at": time.Now().UTC(),
	}); err != nil {
		t.Fatalf("dequeued: %v", err)
	}
	if err := testStore.UpdateRunStatus(context.Background(), firstID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{}); err != nil {
		t.Fatalf("executing: %v", err)
	}
	if err := testStore.UpdateRunStatus(context.Background(), firstID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"finished_at": time.Now().UTC(),
	}); err != nil {
		t.Fatalf("completed: %v", err)
	}

	// Same key should create a NEW run since the first is terminal.
	second := triggerJob(t, jobID, `{"payload":{}}`, key)
	secondID := asString(t, second, "id")
	if secondID == firstID {
		t.Fatalf("expected new run ID after terminal, got same %s", firstID)
	}

	// Verify 2 runs exist.
	lw := doRequest(t, http.MethodGet, "/v1/runs/?project_id="+projectID, "")
	if lw.Code != http.StatusOK {
		t.Fatalf("list runs status = %d", lw.Code)
	}
	runs := mustDecodeList(t, lw)
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs after key reuse, got %d", len(runs))
	}
}

func TestE2E_IdempotencyKeyTooLong(t *testing.T) {
	mustClean(t)

	projectID := "proj-idem-long-" + newID()
	job := createJob(t, projectID, "IdemLong", "idem-long-"+newID())
	jobID := asString(t, job, "id")

	longKey := strings.Repeat("x", 257)
	req := authedRequest(http.MethodPost, "/v1/jobs/"+jobID+"/trigger", `{"payload":{}}`)
	req.Header.Set("X-Idempotency-Key", longKey)
	w := httptest.NewRecorder()
	testServer.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for long key, got %d: %s", w.Code, w.Body.String())
	}
}

func TestE2E_IdempotencyBulkPerItem(t *testing.T) {
	mustClean(t)

	projectID := "proj-idem-bulk-" + newID()
	job := createJob(t, projectID, "IdemBulk", "idem-bulk-"+newID())
	jobID := asString(t, job, "id")

	// First: trigger one item with a known key.
	keyA := "bulk-key-a-" + newID()
	triggerJob(t, jobID, `{"payload":{"item":"a"}}`, keyA)

	// Bulk trigger: first item has same key (should hit), second is new.
	keyB := "bulk-key-b-" + newID()
	body := fmt.Sprintf(`{"items":[{"payload":{},"idempotency_key":"%s"},{"payload":{},"idempotency_key":"%s"}]}`, keyA, keyB)
	w := doRequest(t, http.MethodPost, "/v1/jobs/"+jobID+"/trigger/bulk", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("bulk trigger status = %d, body = %s", w.Code, w.Body.String())
	}

	var bulkResp struct {
		Results []map[string]any `json:"results"`
		Total   int              `json:"total"`
		Created int              `json:"created"`
	}
	if err := json.NewDecoder(w.Body).Decode(&bulkResp); err != nil {
		t.Fatalf("decode bulk response: %v", err)
	}
	if len(bulkResp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(bulkResp.Results))
	}
	// Both should have IDs.
	for i, r := range bulkResp.Results {
		if _, ok := r["id"].(string); !ok || r["id"] == "" {
			t.Fatalf("result[%d] missing id: %v", i, r)
		}
	}
}
