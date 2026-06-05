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
	"strait/internal/crypto"
	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var (
	testEnv         *testutil.TestEnv
	testStore       *store.Queries
	testQueue       *queue.PgQueQueue
	testServer      *api.Server
	cancelTestQueue context.CancelFunc
)

const testEncryptionKey = "0123456789abcdef0123456789abcdef"

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	testEnv, err = testutil.SetupSharedTestEnv(ctx, "../../migrations", "e2e")
	if err != nil {
		log.Fatalf("setup test env: %v", err)
	}

	testStore = store.NewWithContextRouting(testEnv.DB.Pool)
	testStore.SetSecretEncryptionKey(testEncryptionKey)
	if err := resetE2EHarness(ctx); err != nil {
		log.Fatalf("setup e2e harness: %v", err)
	}

	code := m.Run()
	if cancelTestQueue != nil {
		cancelTestQueue()
	}
	testEnv.Cleanup(ctx)
	os.Exit(code)
}

func resetE2EHarness(ctx context.Context) error {
	if cancelTestQueue != nil {
		cancelTestQueue()
	}

	queueCtx, cancel := context.WithCancel(ctx)
	cancelTestQueue = cancel
	testQueue = queue.NewPgQueQueue(testEnv.DB.Pool, queue.NewPostgresRunWriter(testEnv.DB.Pool), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "e2e-" + uuid.Must(uuid.NewV7()).String(),
		ReceiveWindow: 100,
	})
	go testQueue.RunTicker(queueCtx)

	testEncryptor, err := crypto.NewKeyRotatorFromStrings(testEncryptionKey)
	if err != nil {
		return fmt.Errorf("setup test encryptor: %w", err)
	}
	testServer = api.NewServer(api.ServerDeps{
		Config: &config.Config{
			InternalSecret:           "test-secret-value",
			JWTSigningKey:            testJWTSigningKey,
			SecretEncryptionKey:      testEncryptionKey,
			EncryptionKey:            testEncryptionKey,
			RateLimitRequests:        0,
			RateLimitWindow:          time.Minute,
			TriggerRateLimitRequests: 0,
			TriggerRateLimitWindow:   time.Minute,
			CORSAllowedOrigins:       []string{"*"},
			CORSAllowCredentials:     false,
			MaxBulkTriggerItems:      500,
		},
		Store:     testStore,
		Queue:     testQueue,
		Encryptor: testEncryptor,
	})
	return nil
}

func mustClean(t *testing.T) {
	t.Helper()
	require.NoError(t, testEnv.
		DB.CleanTables(
		context.Background()))
	require.NoError(t, resetE2EHarness(context.
		Background()))

}

func newIsolatedQueue(t testing.TB) *queue.PgQueQueue {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	q := queue.NewPgQueQueue(testEnv.DB.Pool, queue.NewPostgresRunWriter(testEnv.DB.Pool), queue.PgQueConfig{
		TickInterval:  10 * time.Millisecond,
		ConsumerName:  "e2e-" + uuid.Must(uuid.NewV7()).String(),
		ReceiveWindow: 100,
	})
	go q.RunTicker(ctx)
	return q
}

func primeHTTPQueue(t testing.TB, q *queue.PgQueQueue) {
	t.Helper()

	run, err := q.Dequeue(context.Background())
	require.NoError(t, err)
	require.Nil(t, run)

}

func primeWorkerQueue(t testing.TB, q *queue.PgQueQueue, refs []domain.WorkerQueueRef) {
	t.Helper()

	claimed, err := q.DequeueNForWorkerQueues(context.Background(), 1, refs)
	require.NoError(t, err)
	require.Len(t, claimed,

		0)

}

func dequeueRunEventually(t testing.TB, q *queue.PgQueQueue) *domain.JobRun {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for {
		run, err := q.Dequeue(context.Background())
		require.NoError(t, err)

		if run != nil {
			return run
		}
		require.False(t, time.
			Now().After(deadline))

		time.Sleep(20 * time.Millisecond)
	}
}

func dequeueRunsEventually(t testing.TB, q *queue.PgQueQueue, want int) []domain.JobRun {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	runs := make([]domain.JobRun, 0, want)
	for len(runs) < want {
		batch, err := q.DequeueN(context.Background(), want-len(runs))
		require.NoError(t, err)

		if len(batch) > 0 {
			runs = append(runs, batch...)
			continue
		}
		if time.Now().After(deadline) {
			return runs
		}
		time.Sleep(20 * time.Millisecond)
	}
	return runs
}

func dequeueWorkerRunsEventually(t testing.TB, q *queue.PgQueQueue, want int, refs []domain.WorkerQueueRef) []domain.JobRun {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	seen := make(map[string]domain.JobRun, want)
	for len(seen) < want {
		claimed, err := q.DequeueNForWorkerQueues(context.Background(), want-len(seen), refs)
		require.NoError(t, err)

		for _, run := range claimed {
			seen[run.ID] = run
		}
		if len(seen) >= want {
			break
		}
		require.False(t, time.
			Now().After(deadline))

		time.Sleep(20 * time.Millisecond)
	}

	runs := make([]domain.JobRun, 0, len(seen))
	for _, run := range seen {
		runs = append(runs, run)
	}
	return runs
}

func authedRequest(method, path, body string, projectID ...string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("Content-Type", "application/json")
	if len(projectID) > 0 && projectID[0] != "" {
		req.Header.Set("X-Project-Id", projectID[0])
	}
	return req
}

func doRequest(t *testing.T, method, path, body string, projectID ...string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	testServer.ServeHTTP(w, authedRequest(method, path, body, projectID...))
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
	require.NoError(t, json.
		NewDecoder(w.Body).
		Decode(&resp))

	return resp
}

func mustDecodeList(t *testing.T, w *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	var envelope struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.
		NewDecoder(w.Body).
		Decode(&envelope))

	return envelope.Data
}

func asString(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	v, ok := m[key].(string)
	require.True(t, ok)

	return v
}

func asBool(t *testing.T, m map[string]any, key string) bool {
	t.Helper()
	v, ok := m[key].(bool)
	require.True(t, ok)

	return v
}

func asInt(t *testing.T, m map[string]any, key string) int {
	t.Helper()
	v, ok := m[key].(float64)
	require.True(t, ok)

	return int(v)
}

func asObject(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	v, ok := m[key].(map[string]any)
	require.True(t, ok)

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
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

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
	require.False(t, w.Code !=
		http.
			StatusCreated &&
		w.Code !=
			http.StatusOK,
	)

	return mustDecodeObject(t, w)
}

func TestE2E_CreateJob(t *testing.T) {
	mustClean(t)

	projectID := "proj-create-" + newID()
	resp := createJob(t, projectID, "create-job", "create-job-"+newID())
	require.NotEqual(t, "",

		asString(t, resp,
			"id"))
	require.EqualValues(t, 1, asInt(t, resp,
		"version",
	))
	require.True(t, asBool(t, resp,
		"enabled"),
	)

}

func TestE2E_GetJob(t *testing.T) {
	mustClean(t)

	projectID := "proj-get-" + newID()
	slug := "get-job-" + newID()
	created := createJob(t, projectID, "Get Job", slug)
	jobID := asString(t, created, "id")

	w := doRequest(t, http.MethodGet, "/v1/jobs/"+jobID+"/", "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	resp := mustDecodeObject(t, w)
	require.Equal(t, jobID,

		asString(t, resp,
			"id"))
	require.Equal(t, projectID,

		asString(t, resp,
			"project_id"),
	)
	require.Equal(t, slug,

		asString(t, resp, "slug"))
	require.NotEqual(t, "",

		asString(t, resp,
			"endpoint_url"))

}

func TestE2E_ListJobs(t *testing.T) {
	mustClean(t)

	projectID := "proj-list-jobs-" + newID()
	createJob(t, projectID, "job-one", "job-one-"+newID())
	createJob(t, projectID, "job-two", "job-two-"+newID())
	createJob(t, projectID, "job-three", "job-three-"+newID())

	w := doRequest(t, http.MethodGet, "/v1/jobs/", "", projectID)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	resp := mustDecodeList(t, w)
	require.Len(t, resp,
		3,
	)

}

func TestE2E_ListJobsByTag(t *testing.T) {
	mustClean(t)

	projectID := "proj-list-jobs-tag-" + newID()
	teamCoreSlug := "job-core-" + newID()
	teamOpsSlug := "job-ops-" + newID()

	bodyCore := fmt.Sprintf(`{"project_id":"%s","name":"Job Core","slug":"%s","endpoint_url":"https://example.com/%s","max_attempts":3,"timeout_secs":60,"tags":{"team":"core","service":"api"}}`, projectID, teamCoreSlug, teamCoreSlug)
	bodyOps := fmt.Sprintf(`{"project_id":"%s","name":"Job Ops","slug":"%s","endpoint_url":"https://example.com/%s","max_attempts":3,"timeout_secs":60,"tags":{"team":"ops"}}`, projectID, teamOpsSlug, teamOpsSlug)

	w := doRequest(t, http.MethodPost, "/v1/jobs/", bodyCore)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	w = doRequest(t, http.MethodPost, "/v1/jobs/", bodyOps)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	w = doRequest(t, http.MethodGet, "/v1/jobs/?tag_key=team&tag_value=core", "", projectID)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	resp := mustDecodeList(t, w)
	require.Len(t, resp,
		1,
	)

	tags := asObject(t, resp[0], "tags")
	require.Equal(t, "core",

		asString(t, tags,
			"team"))

}

func TestE2E_UpdateJob(t *testing.T) {
	mustClean(t)

	projectID := "proj-update-" + newID()
	created := createJob(t, projectID, "Old Name", "update-job-"+newID())
	jobID := asString(t, created, "id")

	w := doRequest(t, http.MethodPatch, "/v1/jobs/"+jobID+"/", `{"name":"New Name"}`)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	resp := mustDecodeObject(t, w)
	require.Equal(t, "New Name",

		asString(t, resp,
			"name"))
	require.EqualValues(t, 2, asInt(t, resp,
		"version",
	))

}

func TestE2E_DeleteJob(t *testing.T) {
	mustClean(t)

	projectID := "proj-delete-" + newID()
	created := createJob(t, projectID, "Delete Job", "delete-job-"+newID())
	jobID := asString(t, created, "id")

	w := doRequest(t, http.MethodDelete, "/v1/jobs/"+jobID+"/", "")
	require.Equal(t, http.
		StatusNoContent,
		w.Code,
	)

	w = doRequest(t, http.MethodGet, "/v1/jobs/"+jobID+"/", "")
	require.Equal(t, http.
		StatusNotFound,
		w.Code,
	)

}

func TestE2E_TriggerJob(t *testing.T) {
	mustClean(t)

	projectID := "proj-trigger-" + newID()
	job := createJob(t, projectID, "Trigger Job", "trigger-job-"+newID())
	jobID := asString(t, job, "id")

	resp := triggerJob(t, jobID, `{"payload":{"k":"v"}}`, "")
	require.NotEqual(t, "",

		asString(t, resp,
			"id"))
	require.Equal(t, string(domain.
		StatusQueued,
	), asString(t, resp,
		"status",
	),
	)

	if _, ok := resp["run_token"]; ok {
		require.Fail(t,

			"trigger response must not expose SDK run_token")
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
	require.Equal(t, http.
		StatusOK,
		w.Code)

	run := mustDecodeObject(t, w)
	require.Equal(t, string(domain.
		StatusQueued,
	), asString(t, run,
		"status",
	))
	require.Equal(t, jobID,

		asString(t, run, "job_id"))
	require.Equal(t, projectID,

		asString(t, run,
			"project_id"))

	payload := asObject(t, run, "payload")
	require.EqualValues(t, 42,
		int(payload["answer"].(float64)))
	require.EqualValues(t, 1, asInt(t, run,
		"job_version",
	))

}

func TestE2E_FullLifecycle(t *testing.T) {
	mustClean(t)

	projectID := "proj-full-lifecycle-" + newID()
	job := createJob(t, projectID, "Full Lifecycle", "full-lifecycle-"+newID())
	triggerResp := triggerJob(t, asString(t, job, "id"), `{"payload":{"flow":"full"}}`, "")
	runID := asString(t, triggerResp, "id")

	w := doRequest(t, http.MethodGet, "/v1/runs/"+runID+"/", "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	run := mustDecodeObject(t, w)
	require.Equal(t, string(domain.
		StatusQueued,
	), asString(t, run,
		"status",
	))

	w = doRequest(t, http.MethodGet, "/v1/runs/", "", projectID)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	runs := mustDecodeList(t, w)
	require.Len(t, runs,
		1,
	)
	require.Equal(t, runID,

		asString(t, runs[0], "id"))

	w = doRequest(t, http.MethodGet, "/v1/stats", "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	stats := mustDecodeObject(t, w)
	require.EqualValues(t, 1, asInt(t, stats,
		"queued",
	))

}

func TestE2E_IdempotentTrigger(t *testing.T) {
	mustClean(t)

	projectID := "proj-idempotent-" + newID()
	job := createJob(t, projectID, "Idempotent", "idempotent-"+newID())
	jobID := asString(t, job, "id")
	idempotencyKey := "idem-" + newID()

	first := triggerJob(t, jobID, `{"payload":{"x":1}}`, idempotencyKey)
	second := triggerJob(t, jobID, `{"payload":{"x":2}}`, idempotencyKey)
	require.Equal(t, asString(t, second,
		"id"),
		asString(t, first,
			"id",
		))

	w := doRequest(t, http.MethodGet, "/v1/runs/", "", projectID)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	runs := mustDecodeList(t, w)
	require.Len(t, runs,
		1,
	)

}

func TestE2E_DelayedRun(t *testing.T) {
	mustClean(t)

	projectID := "proj-delayed-" + newID()
	job := createJob(t, projectID, "Delayed", "delayed-"+newID())
	jobID := asString(t, job, "id")

	scheduledAt := time.Now().UTC().Add(30 * time.Minute).Round(time.Second)
	body := fmt.Sprintf(`{"payload":{"kind":"delayed"},"scheduled_at":"%s"}`, scheduledAt.Format(time.RFC3339))
	triggerResp := triggerJob(t, jobID, body, "")
	require.Equal(t, string(domain.
		StatusDelayed,
	), asString(t,
		triggerResp,
		"status",
	))

	runID := asString(t, triggerResp, "id")
	w := doRequest(t, http.MethodGet, "/v1/runs/"+runID+"/", "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	run := mustDecodeObject(t, w)
	gotScheduled := asString(t, run, "scheduled_at")
	parsedScheduled, err := time.Parse(time.RFC3339, gotScheduled)
	require.NoError(t, err)
	require.False(t, parsedScheduled.
		Sub(scheduledAt) > time.Second ||
		scheduledAt.
			Sub(parsedScheduled) > time.
			Second)

}

func TestE2E_PriorityOrdering(t *testing.T) {
	mustClean(t)

	projectID := "proj-priority-" + newID()
	job := createJob(t, projectID, "Priority", "priority-"+newID())
	jobID := asString(t, job, "id")
	q := newIsolatedQueue(t)
	primeHTTPQueue(t, q)

	run0 := triggerJob(t, jobID, `{"payload":{},"priority":0}`, "")
	run10 := triggerJob(t, jobID, `{"payload":{},"priority":10}`, "")
	run5 := triggerJob(t, jobID, `{"payload":{},"priority":5}`, "")

	_ = run0
	_ = run5

	dequeued := dequeueRunEventually(t, q)
	require.EqualValues(t, 10,
		dequeued.
			Priority,
	)
	require.Equal(t, asString(t, run10,
		"id"),
		dequeued.ID)

}

func TestE2E_ListRunsByProject(t *testing.T) {
	mustClean(t)

	projectID := "proj-list-runs-" + newID()
	job1 := createJob(t, projectID, "Runs A", "runs-a-"+newID())
	job2 := createJob(t, projectID, "Runs B", "runs-b-"+newID())
	triggerJob(t, asString(t, job1, "id"), `{"payload":{"j":1}}`, "")
	triggerJob(t, asString(t, job2, "id"), `{"payload":{"j":2}}`, "")

	w := doRequest(t, http.MethodGet, "/v1/runs/", "", projectID)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	runs := mustDecodeList(t, w)
	require.Len(t, runs,
		2,
	)

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
	require.NoError(t, err)

	w := doRequest(t, http.MethodGet, "/v1/runs/?status="+string(domain.StatusCanceled), "", projectID)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	runs := mustDecodeList(t, w)
	require.Len(t, runs,
		1,
	)
	require.Equal(t, cancelRunID,

		asString(t,
			runs[0], "id"))

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
	require.NoError(t, err)

	err = testStore.UpdateRunStatus(context.Background(), originalRunID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{
		"started_at": time.Now().UTC(),
	})
	require.NoError(t, err)

	err = testStore.UpdateRunStatus(context.Background(), originalRunID, domain.StatusExecuting, domain.StatusFailed, map[string]any{
		"finished_at": time.Now().UTC(),
		"error":       "forced failure for replay",
	})
	require.NoError(t, err)

	w := doRequest(t, http.MethodPost, "/v1/runs/"+originalRunID+"/replay", "")
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	replay := mustDecodeObject(t, w)

	replayRunID := asString(t, replay, "id")
	require.False(t, replayRunID ==
		"" || replayRunID ==
		originalRunID,
	)
	require.Equal(t, string(domain.
		StatusQueued,
	), asString(t, replay,

		"status",
	))

	// Replays do NOT copy the original idempotency key to avoid conflicts
	// with active runs sharing the same key.
	if replayKey, ok := replay["idempotency_key"].(string); ok && replayKey != "" {
		require.Failf(t, "test failure",

			"expected replay idempotency key to be empty, got %q", replayKey)
	}

	lw := doRequest(t, http.MethodGet, "/v1/runs/", "", projectID)
	require.Equal(t, http.
		StatusOK,
		lw.Code)

	runs := mustDecodeList(t, lw)
	require.Len(t, runs,
		2,
	)

}

func TestE2E_ListRunsPagination(t *testing.T) {
	mustClean(t)

	projectID := "proj-pagination-" + newID()
	job := createJob(t, projectID, "Pagination", "pagination-"+newID())
	jobID := asString(t, job, "id")

	for i := range 5 {
		triggerJob(t, jobID, fmt.Sprintf(`{"payload":{"idx":%d}}`, i), "")
	}

	w := doRequest(t, http.MethodGet, "/v1/runs/?limit=2", "", projectID)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	page1 := mustDecodeList(t, w)
	require.Len(t, page1,

		2)

	cursor := asString(t, page1[1], "created_at")
	w = doRequest(t, http.MethodGet, "/v1/runs/?limit=2&cursor="+url.QueryEscape(cursor), "", projectID)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	page2 := mustDecodeList(t, w)
	require.Len(t, page2,

		2)
	require.NotEqual(t, asString(t,
		page2[0],
		"id"), asString(t,
		page1[0], "id",
	))

}

func TestE2E_RunEvents(t *testing.T) {
	mustClean(t)

	projectID := "proj-events-" + newID()
	job := createJob(t, projectID, "Events", "events-"+newID())
	triggerResp := triggerJob(t, asString(t, job, "id"), `{"payload":{"events":true}}`, "")
	runID := asString(t, triggerResp, "id")
	runToken := makeE2ERunToken(t, runID)
	activateE2ERun(t, runID)

	w := doSDKRequest(t, http.MethodPost, "/sdk/v1/runs/"+runID+"/log", runToken, `{"type":"log","level":"info","message":"Processing started","data":{"step":1}}`)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	w = doRequest(t, http.MethodGet, "/v1/runs/"+runID+"/events", "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	events := mustDecodeList(t, w)
	require.Len(t, events,

		1)
	require.Equal(t, "Processing started",

		asString(t, events[0], "message"))

}

func TestE2E_RunAnnotations(t *testing.T) {
	mustClean(t)

	projectID := "proj-annotations-" + newID()
	job := createJob(t, projectID, "Annotations", "annotations-"+newID())
	triggerResp := triggerJob(t, asString(t, job, "id"), `{"payload":{"annotations":true}}`, "")
	runID := asString(t, triggerResp, "id")
	runToken := makeE2ERunToken(t, runID)
	activateE2ERun(t, runID)

	w := doSDKRequest(t, http.MethodPost, "/sdk/v1/runs/"+runID+"/annotate", runToken, `{"annotations":{"env":"prod","region":"eu"}}`)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	w = doRequest(t, http.MethodGet, "/v1/runs/"+runID+"/", "")
	require.Equal(t, http.
		StatusOK,
		w.Code)

	run := mustDecodeObject(t, w)
	metadataRaw, ok := run["metadata"]
	require.True(t, ok)

	metadata, ok := metadataRaw.(map[string]any)
	require.True(t, ok)
	require.False(t, asString(t, metadata,
		"env",
	) != "prod" ||
		asString(t, metadata,
			"region",
		) != "eu")

}

func TestE2E_ListRunsFilterByMetadata(t *testing.T) {
	mustClean(t)

	projectID := "proj-filter-metadata-" + newID()
	job := createJob(t, projectID, "Filter Metadata", "filter-metadata-"+newID())
	jobID := asString(t, job, "id")

	prodRun := triggerJob(t, jobID, `{"payload":{"run":"prod"}}`, "")
	prodRunID := asString(t, prodRun, "id")
	prodRunToken := makeE2ERunToken(t, prodRunID)
	activateE2ERun(t, prodRunID)

	stageRun := triggerJob(t, jobID, `{"payload":{"run":"stage"}}`, "")
	stageRunID := asString(t, stageRun, "id")
	stageRunToken := makeE2ERunToken(t, stageRunID)
	activateE2ERun(t, stageRunID)

	w := doSDKRequest(t, http.MethodPost, "/sdk/v1/runs/"+prodRunID+"/annotate", prodRunToken, `{"annotations":{"env":"prod","region":"eu"}}`)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	w = doSDKRequest(t, http.MethodPost, "/sdk/v1/runs/"+stageRunID+"/annotate", stageRunToken, `{"annotations":{"env":"stage"}}`)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	w = doRequest(t, http.MethodGet, "/v1/runs/?metadata_key=env&metadata_value=prod", "", projectID)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	runs := mustDecodeList(t, w)
	require.Len(t, runs,
		1,
	)
	require.Equal(t, prodRunID,

		asString(t, runs[0], "id"))

	w = doRequest(t, http.MethodGet, "/v1/runs/?metadata_key=env", "", projectID)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	runs = mustDecodeList(t, w)
	require.Len(t, runs,
		2,
	)

}

func TestE2E_Health(t *testing.T) {
	mustClean(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	testServer.ServeHTTP(w, req)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	resp := mustDecodeObject(t, w)
	require.Equal(t, "ok",

		asString(t, resp, "status"))

}

func TestE2E_HealthReady(t *testing.T) {
	mustClean(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	testServer.ServeHTTP(w, req)
	require.Equal(t, http.
		StatusOK,
		w.Code)

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
	require.Equal(t, http.
		StatusOK,
		w.Code)

	stats := mustDecodeObject(t, w)
	require.EqualValues(t, 1, asInt(t, stats,
		"queued",
	))
	require.EqualValues(t, 1, asInt(t, stats,
		"delayed",
	))
	require.EqualValues(t, 0, asInt(t, stats,
		"executing",
	))

}

func TestE2E_AuthRequired(t *testing.T) {
	mustClean(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/", nil)
	testServer.ServeHTTP(w, req)
	require.Equal(t, http.
		StatusUnauthorized,

		w.Code)

}

func TestE2E_AuthInvalidSecret(t *testing.T) {
	mustClean(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/", nil)
	req.Header.Set("X-Internal-Secret", "wrong-secret")
	req.Header.Set("X-Project-Id", "proj")
	testServer.ServeHTTP(w, req)
	require.Equal(t, http.
		StatusUnauthorized,

		w.Code)

}

func TestE2E_APIKeyLifecycle(t *testing.T) {
	mustClean(t)

	projectID := "proj-api-key-" + newID()
	createBody := fmt.Sprintf(`{"project_id":"%s","name":"e2e-key","scopes":["jobs:read","stats:read"],"expires_in_days":30}`, projectID)
	w := doRequest(t, http.MethodPost, "/v1/api-keys/", createBody)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	created := mustDecodeObject(t, w)
	apiKeyID := asString(t, created, "id")
	rawKey := asString(t, created, "key")

	statsReq := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	statsReq.Header.Set("Authorization", "Bearer "+rawKey)
	statsW := httptest.NewRecorder()
	testServer.ServeHTTP(statsW, statsReq)
	require.Equal(t, http.
		StatusOK,
		statsW.Code,
	)

	revokeW := doRequest(t, http.MethodDelete, "/v1/api-keys/"+apiKeyID, "")
	require.Equal(t, http.
		StatusOK,
		revokeW.Code,
	)

	revokedReq := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	revokedReq.Header.Set("Authorization", "Bearer "+rawKey)
	revokedW := httptest.NewRecorder()
	testServer.ServeHTTP(revokedW, revokedReq)
	require.Equal(t, http.
		StatusUnauthorized,

		revokedW.Code)

}

// Test hardening: E2E tests for new features

func TestE2E_ScopeEnforcement(t *testing.T) {
	mustClean(t)

	projectID := "proj-scope-enforce-" + newID()
	// Create a job first (via internal secret).
	created := createJob(t, projectID, "Scope Test Job", "scope-test-"+newID())
	jobID := asString(t, created, "id")

	// Create API key with ONLY jobs:read (no write, no trigger).
	keyBody := fmt.Sprintf(`{"project_id":"%s","name":"read-only","scopes":["jobs:read"],"expires_in_days":30}`, projectID)
	kw := doRequest(t, http.MethodPost, "/v1/api-keys/", keyBody)
	require.Equal(t, http.
		StatusCreated,
		kw.Code,
	)

	keyResp := mustDecodeObject(t, kw)
	rawKey := asString(t, keyResp, "key")

	// GET job with read-only key — should succeed.
	getReq := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+jobID+"/", nil)
	getReq.Header.Set("Authorization", "Bearer "+rawKey)
	getW := httptest.NewRecorder()
	testServer.ServeHTTP(getW, getReq)
	require.Equal(t, http.
		StatusOK,
		getW.Code)

	// PATCH job with read-only key — should be 403.
	patchReq := httptest.NewRequest(http.MethodPatch, "/v1/jobs/"+jobID+"/", strings.NewReader(`{"name":"Hacked"}`))
	patchReq.Header.Set("Authorization", "Bearer "+rawKey)
	patchReq.Header.Set("Content-Type", "application/json")
	patchW := httptest.NewRecorder()
	testServer.ServeHTTP(patchW, patchReq)
	require.Equal(t, http.
		StatusForbidden,
		patchW.
			Code)

	// POST trigger with read-only key — should be 403.
	triggerReq := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID+"/trigger", strings.NewReader(`{}`))
	triggerReq.Header.Set("Authorization", "Bearer "+rawKey)
	triggerReq.Header.Set("Content-Type", "application/json")
	triggerW := httptest.NewRecorder()
	testServer.ServeHTTP(triggerW, triggerReq)
	require.Equal(t, http.
		StatusForbidden,
		triggerW.
			Code)

}

func TestE2E_EmptyScopesRejected(t *testing.T) {
	mustClean(t)

	projectID := "proj-empty-scopes-" + newID()
	keyBody := fmt.Sprintf(`{"project_id":"%s","name":"full-access","scopes":[]}`, projectID)
	kw := doRequest(t, http.MethodPost, "/v1/api-keys/", keyBody)
	require.Equal(t, http.
		StatusBadRequest,
		kw.
			Code)

}

func TestE2E_JobVersionID(t *testing.T) {
	mustClean(t)

	projectID := "proj-vid-e2e-" + newID()
	created := createJob(t, projectID, "VID Job", "vid-job-"+newID())

	vid1 := asString(t, created, "version_id")
	require.NotEqual(t, "",

		vid1)
	require.True(t, strings.HasPrefix(vid1, "ver_"))

	jobID := asString(t, created, "id")
	// Update job — should get new version_id.
	uw := doRequest(t, http.MethodPatch, "/v1/jobs/"+jobID+"/", `{"name":"VID Job Updated"}`)
	require.Equal(t, http.
		StatusOK,
		uw.Code)

	updated := mustDecodeObject(t, uw)
	vid2 := asString(t, updated, "version_id")
	require.NotEqual(t, "",

		vid2)
	require.NotEqual(t, vid2,

		vid1)

}

func TestE2E_VersionPolicyDefault(t *testing.T) {
	mustClean(t)

	projectID := "proj-vpol-e2e-" + newID()
	created := createJob(t, projectID, "VPol Job", "vpol-job-"+newID())

	policy := asString(t, created, "version_policy")
	require.Equal(t, "pin",

		policy)

}

func TestE2E_UpdateJobVersionIncrement(t *testing.T) {
	mustClean(t)

	projectID := "proj-ver-inc-e2e-" + newID()
	created := createJob(t, projectID, "Inc Job", "inc-job-"+newID())
	jobID := asString(t, created, "id")
	require.EqualValues(t, 1, asInt(t, created,
		"version",
	))

	// First update.
	uw1 := doRequest(t, http.MethodPatch, "/v1/jobs/"+jobID+"/", `{"name":"Inc Job v2"}`)
	require.Equal(t, http.
		StatusOK,
		uw1.Code)

	r1 := mustDecodeObject(t, uw1)
	require.EqualValues(t, 2, asInt(t, r1,
		"version",
	))

	// Second update.
	uw2 := doRequest(t, http.MethodPatch, "/v1/jobs/"+jobID+"/", `{"name":"Inc Job v3"}`)
	require.Equal(t, http.
		StatusOK,
		uw2.Code)

	r2 := mustDecodeObject(t, uw2)
	require.EqualValues(t, 3, asInt(t, r2,
		"version",
	))

}

func TestE2E_JobCreatedBy(t *testing.T) {
	mustClean(t)

	projectID := "proj-created-by-" + newID()
	slug := "created-by-" + newID()
	body := fmt.Sprintf(`{"project_id":"%s","name":"Created By Job","slug":"%s","endpoint_url":"https://example.com/%s","max_attempts":3,"timeout_secs":60}`, projectID, slug, slug)

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/", strings.NewReader(body))
	req.Header.Set("X-Internal-Secret", "test-secret-value")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Actor-Id", "user_leo_123")
	req.Header.Set("X-Actor-Email", "leo@example.com")
	req.Header.Set("X-Actor-Name", "Leonardo")

	w := httptest.NewRecorder()
	testServer.ServeHTTP(w, req)
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	resp := mustDecodeObject(t, w)
	createdBy := asString(t, resp, "created_by")
	require.Equal(t, "user_leo_123",

		createdBy,
	)

}

func TestE2E_RolesLifecycle(t *testing.T) {
	mustClean(t)

	// Create role.
	createBody := `{"name":"e2e-deployer","description":"Can deploy things","permissions":["jobs:write","jobs:trigger","jobs:read"]}`
	cw := doRequest(t, http.MethodPost, "/v1/roles", createBody)
	require.Equal(t, http.
		StatusCreated,
		cw.Code,
	)

	created := mustDecodeObject(t, cw)
	roleID := asString(t, created, "id")
	require.NotEqual(t, "",

		roleID)

	// List roles.
	lw := doRequest(t, http.MethodGet, "/v1/roles", "")
	require.Equal(t, http.
		StatusOK,
		lw.Code)

	roles := mustDecodeList(t, lw)
	found := false
	for _, r := range roles {
		if asString(t, r, "id") == roleID {
			found = true
			break
		}
	}
	require.True(t, found)

	// Get role.
	gw := doRequest(t, http.MethodGet, "/v1/roles/"+roleID, "")
	require.Equal(t, http.
		StatusOK,
		gw.Code)

	got := mustDecodeObject(t, gw)
	require.Equal(t, "e2e-deployer",

		asString(
			t, got, "name"))

	// Update role.
	updateBody := `{"name":"e2e-deployer-v2","description":"Updated","permissions":["jobs:write","jobs:trigger","jobs:read","runs:read"]}`
	uw := doRequest(t, http.MethodPatch, "/v1/roles/"+roleID, updateBody)
	require.Equal(t, http.
		StatusOK,
		uw.Code)

	// Assign member.
	assignBody := fmt.Sprintf(`{"user_id":"e2e-user-1","role_id":"%s"}`, roleID)
	aw := doRequest(t, http.MethodPost, "/v1/members", assignBody)
	require.Equal(t, http.
		StatusCreated,
		aw.Code,
	)

	// List members.
	mw := doRequest(t, http.MethodGet, "/v1/members", "")
	require.Equal(t, http.
		StatusOK,
		mw.Code)

	// Remove member.
	rw := doRequest(t, http.MethodDelete, "/v1/members/e2e-user-1", "")
	require.Equal(t, http.
		StatusNoContent,
		rw.
			Code)

	// Delete role.
	dw := doRequest(t, http.MethodDelete, "/v1/roles/"+roleID, "")
	require.Equal(t, http.
		StatusNoContent,
		dw.
			Code)

	gw2 := doRequest(t, http.MethodGet, "/v1/roles/"+roleID, "")
	require.Equal(t, http.
		StatusNotFound,
		gw2.
			Code)

}

func TestE2E_TagFilteringWorkflows(t *testing.T) {
	mustClean(t)

	projectID := "proj-wf-tags-e2e-" + newID()
	slug1 := "wf-tagged-1-" + newID()
	slug2 := "wf-tagged-2-" + newID()

	body1 := fmt.Sprintf(`{"project_id":"%s","name":"WF Tagged 1","slug":"%s","enabled":true,"tags":{"team":"core","env":"prod"}}`, projectID, slug1)
	w1 := doRequest(t, http.MethodPost, "/v1/workflows/", body1)
	require.Equal(t, http.
		StatusCreated,
		w1.Code,
	)

	body2 := fmt.Sprintf(`{"project_id":"%s","name":"WF Tagged 2","slug":"%s","enabled":true,"tags":{"team":"ops"}}`, projectID, slug2)
	w2 := doRequest(t, http.MethodPost, "/v1/workflows/", body2)
	require.Equal(t, http.
		StatusCreated,
		w2.Code,
	)

	// Filter by team=core.
	fw := doRequest(t, http.MethodGet, "/v1/workflows/?tag_key=team&tag_value=core", "", projectID)
	require.Equal(t, http.
		StatusOK,
		fw.Code)

	filtered := mustDecodeList(t, fw)
	require.Len(t, filtered,

		1)

}

func TestE2E_IdempotencyKeyHitReturnsOriginal(t *testing.T) {
	mustClean(t)

	projectID := "proj-idem-hit-" + newID()
	job := createJob(t, projectID, "IdemHit", "idem-hit-"+newID())
	jobID := asString(t, job, "id")
	key := "idem-" + newID()

	first := triggerJob(t, jobID, `{"payload":{"x":1}}`, key)
	second := triggerJob(t, jobID, `{"payload":{"x":2}}`, key)
	require.Equal(t, asString(t, second,
		"id"),
		asString(t, first,
			"id",
		))

	// Only 1 run should exist.
	lw := doRequest(t, http.MethodGet, "/v1/runs/", "", projectID)
	require.Equal(t, http.
		StatusOK,
		lw.Code)

	runs := mustDecodeList(t, lw)
	require.Len(t, runs,
		1,
	)

}

func TestE2E_IdempotencyKeyPerJobScoping(t *testing.T) {
	mustClean(t)

	projectID := "proj-idem-scope-" + newID()
	jobA := createJob(t, projectID, "IdemA", "idem-a-"+newID())
	jobB := createJob(t, projectID, "IdemB", "idem-b-"+newID())
	key := "idem-shared-" + newID()

	runA := triggerJob(t, asString(t, jobA, "id"), `{"payload":{}}`, key)
	runB := triggerJob(t, asString(t, jobB, "id"), `{"payload":{}}`, key)
	require.NotEqual(t, asString(t,
		runB, "id",
	), asString(t, runA,
		"id",
	))

}

func TestE2E_IdempotencyKeyReusableAfterTerminal(t *testing.T) {
	// Terminal runs within 24h are returned as idempotency hits to prevent
	// duplicate execution. The same idempotency key should return the
	// original completed run (not create a new one).
	mustClean(t)

	projectID := "proj-idem-reuse-" + newID()
	job := createJob(t, projectID, "IdemReuse", "idem-reuse-"+newID())
	jobID := asString(t, job, "id")
	key := "idem-" + newID()

	first := triggerJob(t, jobID, `{"payload":{}}`, key)
	firstID := asString(t, first, "id")
	require.NoError(t, testStore.
		UpdateRunStatus(context.Background(),
			firstID,
			domain.StatusQueued,
			domain.StatusDequeued,

			map[string]any{"started_at": time.
				Now().UTC()}))
	require.NoError(t, testStore.
		UpdateRunStatus(context.Background(),
			firstID,
			domain.StatusDequeued,
			domain.
				StatusExecuting,

			map[string]any{}))
	require.NoError(t, testStore.
		UpdateRunStatus(context.Background(),
			firstID,
			domain.StatusExecuting,
			domain.
				StatusCompleted,

			map[string]any{"finished_at": time.Now().UTC()}))

	// Transition to terminal.

	// Same key should return the existing completed run (idempotency hit).
	second := triggerJob(t, jobID, `{"payload":{}}`, key)
	secondID := asString(t, second, "id")
	require.Equal(t, firstID,

		secondID,
	)

	// Verify only 1 run exists (no duplicate created).
	lw := doRequest(t, http.MethodGet, "/v1/runs/", "", projectID)
	require.Equal(t, http.
		StatusOK,
		lw.Code)

	runs := mustDecodeList(t, lw)
	require.Len(t, runs,
		1,
	)

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
	require.Equal(t, http.
		StatusBadRequest,
		w.
			Code)

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
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	var bulkResp struct {
		Results []map[string]any `json:"results"`
		Total   int              `json:"total"`
		Created int              `json:"created"`
	}
	require.NoError(t, json.
		NewDecoder(w.Body).
		Decode(&bulkResp))
	require.Len(t, bulkResp.
		Results,
		2)

	// Both should have IDs.
	for i, r := range bulkResp.Results {
		if _, ok := r["id"].(string); !ok || r["id"] == "" {
			require.Failf(t, "test failure",

				"result[%d] missing id: %v", i, r)
		}
	}
}

func TestAnalyticsEndpoint_ReturnsMetrics(t *testing.T) {
	mustClean(t)

	projectID := "proj-analytics-metrics-" + newID()
	keyW := doRequest(t, http.MethodPost, "/v1/api-keys/", fmt.Sprintf(`{"project_id":"%s","name":"analytics-metrics-key","scopes":["%s"],"expires_in_days":30}`, projectID, domain.ScopeStatsRead))
	require.Equal(t, http.
		StatusCreated,
		keyW.
			Code)

	keyResp := mustDecodeObject(t, keyW)
	apiKey := asString(t, keyResp, "key")
	job := createJob(t, projectID, "Analytics Metrics", "analytics-metrics-"+newID())
	q := newIsolatedQueue(t)
	primeHTTPQueue(t, q)
	triggered := triggerJob(t, asString(t, job, "id"), `{"payload":{"kind":"analytics"}}`, "")
	runID := asString(t, triggered, "id")

	ctx := context.Background()
	dequeued := dequeueRunEventually(t, q)
	require.Equal(t, runID,

		dequeued.
			ID)

	startedAt := time.Now().UTC()
	require.NoError(t, testStore.
		UpdateRunStatus(ctx, runID, domain.
			StatusDequeued,

			domain.
				StatusExecuting, map[string]any{"started_at": startedAt}))
	require.NoError(t, testStore.
		UpdateRunStatus(ctx, runID, domain.
			StatusExecuting,

			domain.
				StatusCompleted, map[string]any{"finished_at": startedAt.
				Add(2 *
					time.
						Second)}))

	req := httptest.NewRequest(http.MethodGet, "/v1/analytics/performance?period_hours=24", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	testServer.ServeHTTP(w, req)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	resp := mustDecodeObject(t, w)
	if _, ok := resp["slowest_jobs"]; !ok {
		require.Fail(t,

			"expected slowest_jobs key")
	}
	if _, ok := resp["throughput"]; !ok {
		require.Fail(t,

			"expected throughput key")
	}
	if _, ok := resp["health_summary"]; !ok {
		require.Fail(t,

			"expected health_summary key")
	}
	throughput := asObject(t, resp, "throughput")
	require.GreaterOrEqual(t, asInt(t, throughput,
		"completed"),
		1)

}

func TestAnalyticsEndpoint_PeriodHoursParam(t *testing.T) {
	mustClean(t)

	projectID := "proj-analytics-period-" + newID()
	keyW := doRequest(t, http.MethodPost, "/v1/api-keys/", fmt.Sprintf(`{"project_id":"%s","name":"analytics-period-key","scopes":["%s"],"expires_in_days":30}`, projectID, domain.ScopeStatsRead))
	require.Equal(t, http.
		StatusCreated,
		keyW.
			Code)

	apiKey := asString(t, mustDecodeObject(t, keyW), "key")

	for _, path := range []string{
		"/v1/analytics/performance?period_hours=1",
		"/v1/analytics/performance?period_hours=720",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		w := httptest.NewRecorder()
		testServer.ServeHTTP(w, req)
		require.Equal(t, http.
			StatusOK,
			w.Code)

	}

	for _, path := range []string{
		"/v1/analytics/performance?period_hours=0",
		"/v1/analytics/performance?period_hours=999",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		w := httptest.NewRecorder()
		testServer.ServeHTTP(w, req)
		require.Equal(t, http.
			StatusBadRequest,
			w.
				Code)

	}
}

func TestAnalyticsEndpoint_EmptyData(t *testing.T) {
	mustClean(t)

	projectID := "proj-analytics-empty-" + newID()
	keyW := doRequest(t, http.MethodPost, "/v1/api-keys/", fmt.Sprintf(`{"project_id":"%s","name":"analytics-empty-key","scopes":["%s"],"expires_in_days":30}`, projectID, domain.ScopeStatsRead))
	require.Equal(t, http.
		StatusCreated,
		keyW.
			Code)

	apiKey := asString(t, mustDecodeObject(t, keyW), "key")

	req := httptest.NewRequest(http.MethodGet, "/v1/analytics/performance", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	testServer.ServeHTTP(w, req)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	resp := mustDecodeObject(t, w)
	slowest, ok := resp["slowest_jobs"].([]any)
	require.True(t, ok)
	require.Len(t, slowest,

		0)

	throughput := asObject(t, resp, "throughput")
	require.False(t, asInt(t, throughput,
		"completed",
	) != 0 ||
		asInt(t,
			throughput,
			"failed",
		) != 0 || asInt(t,
		throughput,
		"timed_out",
	) !=
		0 || asInt(t, throughput,

		"canceled") != 0,
	)

}

func TestBulkCancel_WithChildRuns(t *testing.T) {
	mustClean(t)

	projectID := "proj-bulk-cancel-children-" + newID()
	parentJob := createJob(t, projectID, "Parent Bulk Cancel", "parent-bulk-cancel-"+newID())
	childSlug := "child-bulk-cancel-" + newID()
	createJob(t, projectID, "Child Bulk Cancel", childSlug)
	q := newIsolatedQueue(t)
	primeHTTPQueue(t, q)

	parentRunIDs := make([]string, 0, 3)
	parentTokens := make([]string, 0, 3)
	for range 3 {
		triggered := triggerJob(t, asString(t, parentJob, "id"), `{"payload":{"parent":true}}`, "")
		parentRunID := asString(t, triggered, "id")
		parentRunIDs = append(parentRunIDs, parentRunID)
		parentTokens = append(parentTokens, makeE2ERunToken(t, parentRunID))
	}

	ctx := context.Background()
	for range 3 {
		dequeued := dequeueRunEventually(t, q)
		require.NoError(t, testStore.
			UpdateRunStatus(ctx, dequeued.
				ID, domain.
				StatusDequeued,

				domain.StatusExecuting,
				map[string]any{"started_at": time.Now().UTC()}))

	}

	childRunIDs := make([]string, 0, 3)
	for i := range parentRunIDs {
		spawnBody := fmt.Sprintf(`{"job_slug":"%s","project_id":"%s","payload":{"child":true}}`, childSlug, projectID)
		w := doSDKRequest(t, http.MethodPost, "/sdk/v1/runs/"+parentRunIDs[i]+"/spawn", parentTokens[i], spawnBody)
		require.Equal(t, http.
			StatusCreated,
			w.Code,
		)

		childRunIDs = append(childRunIDs, asString(t, mustDecodeObject(t, w), "id"))
	}

	body := fmt.Sprintf(`{"run_ids":["%s","%s","%s"]}`, parentRunIDs[0], parentRunIDs[1], parentRunIDs[2])
	w := doRequest(t, http.MethodPost, "/v1/runs/bulk-cancel", body)
	require.Equal(t, http.
		StatusOK,
		w.Code)

	resp := mustDecodeObject(t, w)
	require.EqualValues(t, 3, asInt(t, resp,
		"canceled",
	))

	for _, runID := range parentRunIDs {
		gw := doRequest(t, http.MethodGet, "/v1/runs/"+runID+"/", "")
		require.Equal(t, http.
			StatusOK,
			gw.Code)
		require.Equal(t, string(domain.
			StatusCanceled,
		), asString(t,
			mustDecodeObject(t, gw),
			"status"))

	}

	for _, runID := range childRunIDs {
		gw := doRequest(t, http.MethodGet, "/v1/runs/"+runID+"/", "")
		require.Equal(t, http.
			StatusOK,
			gw.Code)
		require.Equal(t, string(domain.
			StatusCanceled,
		), asString(t,
			mustDecodeObject(t, gw),
			"status"))

	}
}

func TestSDK_Heartbeat(t *testing.T) {
	mustClean(t)

	projectID := "proj-sdk-heartbeat-" + newID()
	job := createJob(t, projectID, "SDK Heartbeat", "sdk-heartbeat-"+newID())
	triggered := triggerJob(t, asString(t, job, "id"), `{"payload":{"hb":true}}`, "")
	runID := asString(t, triggered, "id")
	token := makeE2ERunToken(t, runID)
	activateE2ERun(t, runID)

	w := doSDKRequest(t, http.MethodPost, "/sdk/v1/runs/"+runID+"/heartbeat", token, "")
	require.False(t, w.Code !=
		http.
			StatusOK &&
		w.Code != http.
			StatusNoContent,
	)

	gw := doRequest(t, http.MethodGet, "/v1/runs/"+runID+"/", "")
	require.Equal(t, http.
		StatusOK,
		gw.Code)

	run := mustDecodeObject(t, gw)
	if heartbeatRaw, ok := run["heartbeat_at"].(string); !ok || heartbeatRaw == "" {
		require.Failf(t, "test failure",

			"expected heartbeat_at to be set, got %v", run["heartbeat_at"])
	}
}

func TestSDK_LogAndProgress(t *testing.T) {
	mustClean(t)

	projectID := "proj-sdk-log-progress-" + newID()
	job := createJob(t, projectID, "SDK Log Progress", "sdk-log-progress-"+newID())
	triggered := triggerJob(t, asString(t, job, "id"), `{"payload":{"sdk":true}}`, "")
	runID := asString(t, triggered, "id")
	token := makeE2ERunToken(t, runID)
	activateE2ERun(t, runID)

	logW := doSDKRequest(t, http.MethodPost, "/sdk/v1/runs/"+runID+"/log", token, `{"level":"info","message":"test log"}`)
	require.Equal(t, http.
		StatusCreated,
		logW.
			Code)

	progressW := doSDKRequest(t, http.MethodPost, "/sdk/v1/runs/"+runID+"/progress", token, `{"percent":50,"message":"halfway"}`)
	require.Equal(t, http.
		StatusCreated,
		progressW.
			Code)

	eventsW := doRequest(t, http.MethodGet, "/v1/runs/"+runID+"/events", "")
	require.Equal(t, http.
		StatusOK,
		eventsW.Code,
	)

	events := mustDecodeList(t, eventsW)

	foundLog := false
	foundProgress := false
	for _, event := range events {
		if asString(t, event, "message") == "test log" && asString(t, event, "level") == "info" {
			foundLog = true
		}
		if asString(t, event, "type") == string(domain.EventProgress) {
			foundProgress = true
		}
	}
	require.True(t, foundLog)
	require.True(t, foundProgress)

}

func TestDebugMode_CapturesTrace(t *testing.T) {
	mustClean(t)

	projectID := "proj-debug-mode-" + newID()
	job := createJob(t, projectID, "Debug Mode", "debug-mode-"+newID())
	q := newIsolatedQueue(t)
	primeHTTPQueue(t, q)
	triggered := triggerJob(t, asString(t, job, "id"), `{"payload":{"debug":true}}`, "")
	runID := asString(t, triggered, "id")

	debugW := doRequest(t, http.MethodPost, "/v1/runs/"+runID+"/debug", `{"debug_mode":true}`)
	require.Equal(t, http.
		StatusOK,
		debugW.Code,
	)

	ctx := context.Background()
	dequeued := dequeueRunEventually(t, q)
	require.Equal(t, runID,

		dequeued.
			ID)
	require.NoError(t, testStore.
		UpdateRunStatus(ctx, runID, domain.
			StatusDequeued,

			domain.
				StatusExecuting, map[string]any{"started_at": time.
				Now().UTC()}))
	require.NoError(t, testStore.
		UpdateRunStatus(ctx, runID, domain.
			StatusExecuting,

			domain.
				StatusCompleted, map[string]any{"finished_at": time.Now().
				UTC(),
				"execution_trace": &domain.
					ExecutionTrace{TotalMs: 150, DispatchMs: 100}}))

	gw := doRequest(t, http.MethodGet, "/v1/runs/"+runID+"/", "")
	require.Equal(t, http.
		StatusOK,
		gw.Code)

	run := mustDecodeObject(t, gw)
	require.True(t, asBool(t, run,
		"debug_mode",
	))

	trace, ok := run["execution_trace"].(map[string]any)
	require.True(t, ok)
	require.False(t, asInt(t, trace,
		"total_ms",
	) <= 0)

}
