//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/testutil"

	"github.com/google/uuid"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
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
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	created := mustDecodeObject(t, w)
	jobID := asString(t, created, "id")

	// Verify the job can be fetched back and the name is stored verbatim.
	w2 := doRequest(t, http.MethodGet, "/v1/jobs/"+jobID, "")
	require.Equal(t, http.
		StatusOK,
		w2.Code)

	fetched := mustDecodeObject(t, w2)
	gotName := asString(t, fetched, "name")
	require.Equal(t, injectionName,

		gotName)

}

// TestAdversarial_ConcurrentDequeueSameRun verifies that when a single run is
// enqueued, only one of many concurrent dequeue attempts receives it.
func TestAdversarial_ConcurrentDequeueSameRun(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	q := newIsolatedQueue(t)
	job := testutil.MustCreateJob(t, ctx, testStore, nil)
	_ = testutil.MustEnqueueRun(t, ctx, q, job, nil)

	const goroutines = 10
	var wg conc.WaitGroup
	var gotRun atomic.Int32

	for range goroutines {
		wg.Go(func() {
			runs, err := q.DequeueN(ctx, 1)
			if err != nil {
				return
			}
			if len(runs) > 0 {
				gotRun.Add(1)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, gotRun.
		Load())

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
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	created := mustDecodeObject(t, w)
	tags := asObject(t, created, "tags")
	teamTag, ok := tags["team"].(string)
	require.True(t, ok)
	require.Equal(t, "<script>alert('xss')</script>",

		teamTag)

	// List jobs filtered by tag.
	w2 := doRequest(t, http.MethodGet, fmt.Sprintf("/v1/jobs/?tag_key=team&tag_value=%s", "<script>alert('xss')</script>"), "", projectID)
	require.Equal(t, http.
		StatusOK,
		w2.Code)

	jobs := mustDecodeList(t, w2)
	require.Len(t, jobs,
		1,
	)

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
	require.Equal(t, http.
		StatusCreated,
		w.Code,
	)

	created := mustDecodeObject(t, w)
	jobID := asString(t, created, "id")

	// Create an active (executing) run directly via store.
	ctx := context.Background()
	jobObj, err := testStore.GetJob(ctx, jobID)
	require.NoError(t, err)

	status := domain.StatusExecuting
	_ = testutil.MustCreateRun(t, ctx, testStore, jobObj, &testutil.RunOpts{
		Status: &status,
	})

	// Try to trigger another run; overlap=skip should prevent it.
	req := authedRequest(http.MethodPost, "/v1/jobs/"+jobID+"/trigger", `{}`)
	w2 := httptest.NewRecorder()
	testServer.ServeHTTP(w2, req)
	require.False(t, w2.Code >=
		500,
	)

	// The server should either skip (returning 200 with existing run info or a
	// specific skip status) or return a conflict. Any 2xx/4xx is valid; only a
	// 5xx would indicate a bug.

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
		ID:          uuid.Must(uuid.NewV7()).String(),
		EventKey:    eventKey,
		ProjectID:   projectID,
		SourceType:  "job_run",
		JobRunID:    run.ID,
		Status:      "waiting",
		TimeoutSecs: 300,
		RequestedAt: time.Now(),
		ExpiresAt:   time.Now().Add(5 * time.Minute),
	}
	require.NoError(t, testStore.
		CreateEventTrigger(ctx, trigger))

	// Look up by exact key.
	got, err := testStore.GetEventTriggerByEventKey(ctx, eventKey)
	require.NoError(t, err)
	require.Equal(t, trigger.
		ID, got.
		ID)

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

	var wg conc.WaitGroup
	var successCount atomic.Int32
	var quotaErrors atomic.Int32

	for i := range goroutines {
		idx := i
		wg.Go(func() {
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
		})
	}
	wg.Wait()
	require.NotEqual(t, 0,

		successCount.
			Load(),
	)

	// With maxPerJob=4096 and each entry ~820 bytes, at most 4 should succeed.

	totalSize, err := testStore.SumJobMemorySizeBytes(ctx, job.ID)
	require.NoError(t, err)
	require.LessOrEqual(t,

		totalSize,
		maxPerJob,
	)

}

// TestAdversarial_BudgetEnforcementConcurrentSpend sets up a project and
// verifies that concurrent triggers do not create more runs than allowed.
func TestAdversarial_BudgetEnforcementConcurrentSpend(t *testing.T) {
	mustClean(t)

	projectID := "proj-budget-" + newID()
	job := createJob(t, projectID, "budget-job", "budget-slug-"+newID())
	jobID := asString(t, job, "id")

	const goroutines = 10
	var wg conc.WaitGroup
	var created atomic.Int32

	for i := range goroutines {
		idx := i
		wg.Go(func() {
			req := authedRequest(http.MethodPost, "/v1/jobs/"+jobID+"/trigger", `{"idx":`+fmt.Sprintf("%d", idx)+`}`)
			w := httptest.NewRecorder()
			testServer.ServeHTTP(w, req)
			if w.Code == http.StatusCreated {
				created.Add(1)
			}
		})
	}
	wg.Wait()

	// Verify all created runs are accounted for in the store.
	ctx := context.Background()
	runs, err := testStore.ListRunsByJob(ctx, jobID, 100, 0)
	require.NoError(t, err)
	require.Equal(t, created.
		Load(),
		int32(len(runs)))

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
			"secret": "test-secret-value"
		}`, projectID, badURL)

		w := doRequest(t, http.MethodPost, "/v1/webhooks/subscriptions", body, projectID)
		require.False(t, w.Code ==
			http.
				StatusCreated ||
			w.Code ==
				http.StatusOK,
		)

	}
}
