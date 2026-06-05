//go:build loadtest

package loadtest

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	vegeta "github.com/tsenart/vegeta/v12/lib"
)

// Jobs CRUD load tests

func TestJobs_CreateJob(t *testing.T) {
	mustClean(t)
	projectID := "proj-cj-" + newID()

	tgt := newTargeter("POST", "/v1/jobs/", func() []byte {
		slug := "load-" + newID()
		return fmt.Appendf(nil,
			`{"project_id":"%s","name":"load-%s","slug":"%s","endpoint_url":"https://example.com/%s","max_attempts":3,"timeout_secs":60}`,
			projectID, slug, slug, slug,
		)
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "create-job", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "create-job", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "create-job", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestJobs_ListJobs(t *testing.T) {
	mustClean(t)
	projectID := "proj-lj-" + newID()
	seedManyJobs(t, projectID, 30)

	tgt := newTargeter("GET", "/v1/jobs/?project_id="+projectID+"&limit=50", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-jobs", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-jobs", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-jobs", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestJobs_GetJob(t *testing.T) {
	mustClean(t)
	projectID := "proj-gj-" + newID()
	jobID := seedJob(t, projectID)

	tgt := newTargeter("GET", "/v1/jobs/"+jobID+"/", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "get-job", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "get-job", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "get-job", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestJobs_UpdateJob(t *testing.T) {
	mustClean(t)
	projectID := "proj-uj-" + newID()
	jobID := seedJob(t, projectID)

	var counter atomic.Int64
	tgt := newTargeter("PATCH", "/v1/jobs/"+jobID+"/", func() []byte {
		n := counter.Add(1)
		return fmt.Appendf(nil, `{"name":"updated-%d"}`, n)
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "update-job", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "update-job", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "update-job", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestJobs_DeleteJob(t *testing.T) {
	mustClean(t)
	projectID := "proj-dj-" + newID()
	// Pre-seed many jobs for deletion.
	jobIDs := seedManyJobs(t, projectID, 200)

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		i %= int64(len(jobIDs))
		tgt.Method = "DELETE"
		tgt.URL = baseURL + "/v1/jobs/" + jobIDs[i] + "/"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "delete-job", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "delete-job", tgt)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "delete-job", tgt)
		assertNoServerErrors(t, m)
	})
}

func TestJobs_TriggerJob(t *testing.T) {
	mustClean(t)
	projectID := "proj-tj-" + newID()
	jobID := seedJob(t, projectID)

	tgt := newTargeter("POST", "/v1/jobs/"+jobID+"/trigger", func() []byte {
		return fmt.Appendf(nil, `{"payload":{"id":"%s"}}`, newID())
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "trigger-job", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "trigger-job", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "trigger-job", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestJobs_BulkTrigger(t *testing.T) {
	mustClean(t)
	projectID := "proj-bt-" + newID()
	jobID := seedJob(t, projectID)

	tgt := newTargeter("POST", "/v1/jobs/"+jobID+"/trigger/bulk", func() []byte {
		var items strings.Builder
		for i := range 5 {
			if i > 0 {
				items.WriteString(",")
			}
			_, _ = fmt.Fprintf(&items, `{"payload":{"idx":%d},"idempotency_key":"bulk-%s"}`, i, newID())
		}
		return fmt.Appendf(nil, `{"items":[%s]}`, items.String())
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "bulk-trigger", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "bulk-trigger", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "bulk-trigger", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestJobs_BatchCreate(t *testing.T) {
	mustClean(t)
	projectID := "proj-bc-" + newID()

	tgt := newTargeter("POST", "/v1/jobs/batch", func() []byte {
		var jobs strings.Builder
		for i := range 5 {
			if i > 0 {
				jobs.WriteString(",")
			}
			slug := "batch-" + newID()
			_, _ = fmt.Fprintf(&jobs,
				`{"project_id":"%s","name":"batch-%s","slug":"%s","endpoint_url":"https://example.com/%s","max_attempts":3,"timeout_secs":60}`,
				projectID, slug, slug, slug,
			)
		}
		return fmt.Appendf(nil, `{"jobs":[%s]}`, jobs.String())
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "batch-create", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "batch-create", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "batch-create", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestJobs_CloneJob(t *testing.T) {
	mustClean(t)
	projectID := "proj-clone-" + newID()
	jobID := seedJob(t, projectID)

	tgt := newTargeter("POST", "/v1/jobs/"+jobID+"/clone", func() []byte {
		slug := "clone-" + newID()
		return fmt.Appendf(nil, `{"slug":"%s","name":"clone-%s"}`, slug, slug)
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "clone-job", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "clone-job", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "clone-job", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestJobs_ListVersions(t *testing.T) {
	mustClean(t)
	projectID := "proj-lv-" + newID()
	jobID := seedJob(t, projectID)

	// Create several versions by updating the job.
	for i := range 5 {
		httpDo(t, "PATCH", "/v1/jobs/"+jobID+"/", fmt.Sprintf(`{"name":"ver-%d"}`, i+2), nil)
	}

	tgt := newTargeter("GET", "/v1/jobs/"+jobID+"/versions", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-versions", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-versions", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-versions", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestJobs_GetJobHealth(t *testing.T) {
	mustClean(t)
	projectID := "proj-jh-" + newID()
	jobID := seedJob(t, projectID)
	// Seed some runs to generate health data.
	seedManyRuns(t, jobID, 10)

	tgt := newTargeter("GET", "/v1/jobs/"+jobID+"/health", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "job-health", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "job-health", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "job-health", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestJobs_ListByTag(t *testing.T) {
	mustClean(t)
	projectID := "proj-lt-" + newID()
	for range 10 {
		seedJobWithTags(t, projectID, map[string]string{"team": "core", "env": "prod"})
	}
	for range 5 {
		seedJobWithTags(t, projectID, map[string]string{"team": "ops"})
	}

	tgt := newTargeter("GET", "/v1/jobs/?project_id="+projectID+"&tag_key=team&tag_value=core", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-by-tag", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-by-tag", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-by-tag", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestJobs_CreateDependency(t *testing.T) {
	mustClean(t)
	projectID := "proj-dep-" + newID()
	jobID := seedJob(t, projectID)

	// Each request creates a dependency to a freshly seeded job.
	tgt := func(tgt *vegeta.Target) error {
		depJobID := seedJob(t, projectID)
		tgt.Method = "POST"
		tgt.URL = baseURL + "/v1/jobs/" + jobID + "/dependencies"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
			"Content-Type":      []string{"application/json"},
		}
		tgt.Body = fmt.Appendf(nil, `{"depends_on_job_id":"%s"}`, depJobID)
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "create-dep", tgt, withRate(20))
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "create-dep", tgt, withWorkers(10))
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "create-dep", tgt)
		assertNoServerErrors(t, m)
	})
}

func TestJobs_ListDependencies(t *testing.T) {
	mustClean(t)
	projectID := "proj-ld-" + newID()
	jobID := seedJob(t, projectID)

	// Add some dependencies.
	for range 5 {
		depJobID := seedJob(t, projectID)
		httpDo(t, "POST", "/v1/jobs/"+jobID+"/dependencies",
			fmt.Sprintf(`{"depends_on_job_id":"%s"}`, depJobID), nil)
	}

	tgt := newTargeter("GET", "/v1/jobs/"+jobID+"/dependencies", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-deps", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-deps", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-deps", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestJobs_BatchEnableDisable(t *testing.T) {
	mustClean(t)
	projectID := "proj-bed-" + newID()
	jobIDs := seedManyJobs(t, projectID, 20)

	// Build a JSON array of job IDs.
	var idsJSON strings.Builder
	idsJSON.WriteString("[")
	for i, id := range jobIDs {
		if i > 0 {
			idsJSON.WriteString(",")
		}
		_, _ = fmt.Fprintf(&idsJSON, `"%s"`, id)
	}
	idsJSON.WriteString("]")

	disableTgt := newTargeter("POST", "/v1/jobs/batch-disable", func() []byte {
		return fmt.Appendf(nil, `{"ids":%s}`, idsJSON.String())
	})
	enableTgt := newTargeter("POST", "/v1/jobs/batch-enable", func() []byte {
		return fmt.Appendf(nil, `{"ids":%s}`, idsJSON.String())
	})

	t.Run("disable/baseline", func(t *testing.T) {
		m := runBaseline(t, "batch-disable", disableTgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("enable/baseline", func(t *testing.T) {
		m := runBaseline(t, "batch-enable", enableTgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("disable/stress", func(t *testing.T) {
		m := runStress(t, "batch-disable", disableTgt)
		assertNoServerErrors(t, m)
	})
	t.Run("enable/stress", func(t *testing.T) {
		m := runStress(t, "batch-enable", enableTgt)
		assertNoServerErrors(t, m)
	})
}

func TestJobs_ConcurrentReadWrite(t *testing.T) {
	mustClean(t)
	projectID := "proj-rw-" + newID()
	jobIDs := seedManyJobs(t, projectID, 10)

	// Round-robin reads across existing jobs.
	var readIdx atomic.Int64
	readTgt := func(tgt *vegeta.Target) error {
		i := readIdx.Add(1) % int64(len(jobIDs))
		tgt.Method = "GET"
		tgt.URL = baseURL + "/v1/jobs/" + jobIDs[i] + "/"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
		}
		return nil
	}

	// Writes create new jobs.
	writeTgt := newTargeter("POST", "/v1/jobs/", func() []byte {
		slug := "rw-" + newID()
		return fmt.Appendf(nil,
			`{"project_id":"%s","name":"rw-%s","slug":"%s","endpoint_url":"https://example.com/%s","max_attempts":3,"timeout_secs":60}`,
			projectID, slug, slug, slug,
		)
	})

	t.Run("reads/stress", func(t *testing.T) {
		m := runStress(t, "concurrent-reads", readTgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("writes/stress", func(t *testing.T) {
		m := runStress(t, "concurrent-writes", writeTgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
}

func TestJobs_DeleteDependency(t *testing.T) {
	mustClean(t)
	projectID := "proj-delete-dep-" + newID()
	jobID := seedJob(t, projectID)

	depIDs := make([]string, 200)
	for i := range 200 {
		dependsOnJobID := seedJob(t, projectID)
		resp := httpDo(t, "POST", "/v1/jobs/"+jobID+"/dependencies", fmt.Sprintf(`{"depends_on_job_id":"%s"}`, dependsOnJobID), nil)
		depID, ok := resp["id"].(string)
		require.False(t, !ok ||
			depID ==
				"",
		)

		depIDs[i] = depID
	}

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		pos := i % int64(len(depIDs))
		tgt.Method = "DELETE"
		tgt.URL = baseURL + "/v1/jobs/" + jobID + "/dependencies/" + depIDs[pos]
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "delete-job-dependency", tgt)
		assertSuccessRate(t, m, 0.80)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "delete-job-dependency", tgt)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "delete-job-dependency", tgt)
		assertNoServerErrors(t, m)
	})
}
