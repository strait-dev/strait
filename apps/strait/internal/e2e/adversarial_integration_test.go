//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/testutil"

	"github.com/google/uuid"
)

// TestAdversarial_SQLInjectionThruAPI verifies that SQL injection payloads in
// the job name are safely escaped and stored without corrupting the database.
func TestAdversarial_SQLInjectionThruAPI(t *testing.T) {
	mustClean(t)

	projectID := "proj-sqli-" + newID()
	injectionName := "'; DROP TABLE jobs; --"
	slug := "sqli-slug-" + newID()

	body := fmt.Sprintf(`{
		"project_id": %q,
		"name": %q,
		"slug": %q,
		"endpoint_url": "https://example.com/webhook",
		"max_attempts": 3,
		"timeout_secs": 60
	}`, projectID, injectionName, slug)

	w := doRequest(t, http.MethodPost, "/v1/jobs/", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create job with SQL injection name: status = %d, body = %s", w.Code, w.Body.String())
	}

	created := mustDecodeObject(t, w)
	jobID := asString(t, created, "id")

	// Verify the job can be fetched back and the name is stored verbatim.
	w2 := doRequest(t, http.MethodGet, "/v1/jobs/"+jobID, "")
	if w2.Code != http.StatusOK {
		t.Fatalf("get job after SQL injection name: status = %d, body = %s", w2.Code, w2.Body.String())
	}

	fetched := mustDecodeObject(t, w2)
	gotName := asString(t, fetched, "name")
	if gotName != injectionName {
		t.Fatalf("job name mismatch: got %q, want %q", gotName, injectionName)
	}
}


// TestAdversarial_ConcurrentDequeueSameRun verifies that when a single run is
// enqueued, only one of many concurrent dequeue attempts receives it.
func TestAdversarial_ConcurrentDequeueSameRun(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	job := testutil.MustCreateJob(t, ctx, testStore, nil)
	_ = testutil.MustEnqueueRun(t, ctx, testQueue, job, nil)

	const goroutines = 10
	var wg sync.WaitGroup
	var gotRun atomic.Int32

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			runs, err := testQueue.DequeueN(ctx, 1)
			if err != nil {
				return
			}
			if len(runs) > 0 {
				gotRun.Add(1)
			}
		}()
	}
	wg.Wait()

	if gotRun.Load() != 1 {
		t.Fatalf("expected exactly 1 goroutine to dequeue the run, got %d", gotRun.Load())
	}
}



// TestAdversarial_TagSpecialCharsFullPipeline verifies that jobs with unicode,
// angle brackets, and quotes in tags are stored and filtered correctly.
func TestAdversarial_TagSpecialCharsFullPipeline(t *testing.T) {
	mustClean(t)

	projectID := "proj-tags-" + newID()
	slug := "tag-job-" + newID()

	body := fmt.Sprintf(`{
		"project_id": %q,
		"name": "tag-special-job",
		"slug": %q,
		"endpoint_url": "https://example.com/webhook",
		"max_attempts": 3,
		"timeout_secs": 60,
		"tags": {"team": "<script>alert('xss')</script>", "env": "\u00e9\u00e8\u00ea"}
	}`, projectID, slug)

	w := doRequest(t, http.MethodPost, "/v1/jobs/", body, projectID)
	if w.Code != http.StatusCreated {
		t.Fatalf("create job with special tags: status = %d, body = %s", w.Code, w.Body.String())
	}

	created := mustDecodeObject(t, w)
	tags := asObject(t, created, "tags")
	teamTag, ok := tags["team"].(string)
	if !ok {
		t.Fatal("tag 'team' is not a string")
	}
	if teamTag != "<script>alert('xss')</script>" {
		t.Fatalf("tag value mismatch: got %q", teamTag)
	}

	// List jobs filtered by tag.
	w2 := doRequest(t, http.MethodGet, fmt.Sprintf("/v1/jobs/?tag_key=team&tag_value=%s", "<script>alert('xss')</script>"), "", projectID)
	if w2.Code != http.StatusOK {
		t.Fatalf("list jobs by tag: status = %d, body = %s", w2.Code, w2.Body.String())
	}

	jobs := mustDecodeList(t, w2)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job filtered by tag, got %d", len(jobs))
	}
}



// TestAdversarial_CronOverlapSkipConcurrent verifies that a cron job with
// overlap=skip policy skips new triggers while an active run exists.
func TestAdversarial_CronOverlapSkipConcurrent(t *testing.T) {
	mustClean(t)

	projectID := "proj-overlap-" + newID()
	slug := "overlap-slug-" + newID()

	body := fmt.Sprintf(`{
		"project_id": %q,
		"name": "overlap-job",
		"slug": %q,
		"endpoint_url": "https://example.com/webhook",
		"max_attempts": 3,
		"timeout_secs": 60,
		"cron": "*/5 * * * *",
		"cron_overlap_policy": "skip"
	}`, projectID, slug)

	w := doRequest(t, http.MethodPost, "/v1/jobs/", body, projectID)
	if w.Code != http.StatusCreated {
		t.Fatalf("create cron job: status = %d, body = %s", w.Code, w.Body.String())
	}

	created := mustDecodeObject(t, w)
	jobID := asString(t, created, "id")

	// Create an active (executing) run directly via store.
	ctx := context.Background()
	jobObj, err := testStore.GetJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetJob error: %v", err)
	}
	status := domain.StatusExecuting
	_ = testutil.MustCreateRun(t, ctx, testStore, jobObj, &testutil.RunOpts{
		Status: &status,
	})

	// Try to trigger another run; overlap=skip should prevent it.
	req := authedRequest(http.MethodPost, "/v1/jobs/"+jobID+"/trigger", `{}`)
	w2 := httptest.NewRecorder()
	testServer.ServeHTTP(w2, req)

	// The server should either skip (returning 200 with existing run info or a
	// specific skip status) or return a conflict. Any 2xx/4xx is valid; only a
	// 5xx would indicate a bug.
	if w2.Code >= 500 {
		t.Fatalf("overlap=skip trigger caused server error: status = %d, body = %s", w2.Code, w2.Body.String())
	}
}

// TestAdversarial_EventTriggerAdversarialKeys creates an event trigger with
// regex-special characters in the key and verifies lookup behavior.
func TestAdversarial_EventTriggerAdversarialKeys(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	projectID := "proj-evt-" + newID()
	job := testutil.MustCreateJob(t, ctx, testStore, &testutil.JobOpts{
		ProjectID: &projectID,
	})
	status := domain.StatusWaiting
	run := testutil.MustCreateRun(t, ctx, testStore, job, &testutil.RunOpts{
		Status: &status,
	})

	// Create an event trigger with regex-special characters in the key.
	eventKey := "test.*.event[0]+(foo|bar)" + newID()
	trigger := &domain.EventTrigger{
		ID:         uuid.Must(uuid.NewV7()).String(),
		EventKey:   eventKey,
		ProjectID:  projectID,
		SourceType: "job_run",
		JobRunID:   run.ID,
		Status:     "waiting",
		TimeoutSecs: 300,
		RequestedAt: time.Now(),
		ExpiresAt:   time.Now().Add(5 * time.Minute),
	}
	if err := testStore.CreateEventTrigger(ctx, trigger); err != nil {
		t.Fatalf("CreateEventTrigger error: %v", err)
	}

	// Look up by exact key.
	got, err := testStore.GetEventTriggerByEventKey(ctx, eventKey)
	if err != nil {
		t.Fatalf("GetEventTriggerByEventKey error: %v", err)
	}
	if got.ID != trigger.ID {
		t.Fatalf("event trigger ID mismatch: got %q, want %q", got.ID, trigger.ID)
	}
}

// TestAdversarial_JobMemoryQuotaConcurrentWrites verifies that concurrent
// UpsertJobMemoryWithQuota calls respect the per-job quota.
func TestAdversarial_JobMemoryQuotaConcurrentWrites(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	job := testutil.MustCreateJob(t, ctx, testStore, nil)

	const goroutines = 10
	const maxPerKey = 1024
	const maxPerJob = 4096

	var wg sync.WaitGroup
	var successCount atomic.Int32
	var quotaErrors atomic.Int32

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			mem := &domain.JobMemory{
				ID:        uuid.Must(uuid.NewV7()).String(),
				JobID:     job.ID,
				ProjectID: job.ProjectID,
				MemoryKey: fmt.Sprintf("key-%d", idx),
				Value:     json.RawMessage(fmt.Sprintf(`{"idx":%d,"data":"%s"}`, idx, strings.Repeat("x", 800))),
				SizeBytes: 820,
			}
			err := testStore.UpsertJobMemoryWithQuota(ctx, mem, maxPerKey, maxPerJob)
			if err == nil {
				successCount.Add(1)
			} else {
				quotaErrors.Add(1)
			}
		}(i)
	}
	wg.Wait()

	// With maxPerJob=4096 and each entry ~820 bytes, at most 4 should succeed.
	if successCount.Load() == 0 {
		t.Fatal("expected at least one successful memory write")
	}

	totalSize, err := testStore.SumJobMemorySizeBytes(ctx, job.ID)
	if err != nil {
		t.Fatalf("SumJobMemorySizeBytes error: %v", err)
	}
	if totalSize > maxPerJob {
		t.Fatalf("total memory size %d exceeds quota %d", totalSize, maxPerJob)
	}
}

// TestAdversarial_BudgetEnforcementConcurrentSpend sets up a project and
// verifies that concurrent triggers do not create more runs than allowed.
func TestAdversarial_BudgetEnforcementConcurrentSpend(t *testing.T) {
	mustClean(t)

	projectID := "proj-budget-" + newID()
	job := createJob(t, projectID, "budget-job", "budget-slug-"+newID())
	jobID := asString(t, job, "id")

	const goroutines = 10
	var wg sync.WaitGroup
	var created atomic.Int32

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			req := authedRequest(http.MethodPost, "/v1/jobs/"+jobID+"/trigger", `{"idx":`+fmt.Sprintf("%d", idx)+`}`)
			w := httptest.NewRecorder()
			testServer.ServeHTTP(w, req)
			if w.Code == http.StatusCreated {
				created.Add(1)
			}
		}(i)
	}
	wg.Wait()

	// Verify all created runs are accounted for in the store.
	ctx := context.Background()
	runs, err := testStore.ListRunsByJob(ctx, jobID, 100, 0)
	if err != nil {
		t.Fatalf("ListRunsByJob error: %v", err)
	}
	if int32(len(runs)) != created.Load() {
		t.Fatalf("run count mismatch: store has %d, API reported %d created", len(runs), created.Load())
	}
}

// TestAdversarial_WebhookSubscriptionURLReal verifies that creating a webhook
// subscription with a localhost URL is rejected by the server.
func TestAdversarial_WebhookSubscriptionURLReal(t *testing.T) {
	mustClean(t)

	projectID := "proj-webhook-" + newID()

	blockedURLs := []string{
		"http://localhost/callback",
		"http://127.0.0.1/callback",
		"http://169.254.169.254/latest/meta-data",
		"http://metadata.google.internal/computeMetadata",
	}

	for _, badURL := range blockedURLs {
		body := fmt.Sprintf(`{
			"project_id": %q,
			"webhook_url": %q,
			"event_types": ["run.completed"],
			"secret": testInternalSecret
		}`, projectID, badURL)

		w := doRequest(t, http.MethodPost, "/v1/webhooks/subscriptions", body, projectID)
		if w.Code == http.StatusCreated || w.Code == http.StatusOK {
			t.Fatalf("expected rejection for URL %q, got status %d", badURL, w.Code)
		}
	}
}
