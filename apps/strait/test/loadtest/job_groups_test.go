//go:build loadtest

package loadtest

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	vegeta "github.com/tsenart/vegeta/v12/lib"
)

func TestJobGroups_Create(t *testing.T) {
	mustClean(t)
	projectID := "proj-jg-create-" + newID()

	tgt := newTargeter("POST", "/v1/job-groups/", func() []byte {
		slug := "grp-" + newID()
		return []byte(fmt.Sprintf(
			`{"project_id":"%s","name":"load-group-%s","slug":"%s","description":"load test group"}`,
			projectID, slug, slug,
		))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "create-job-group", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "create-job-group", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		// Spike p99 can reach 400-500ms due to UNIQUE(project_id, slug) constraint
		// contention under concurrent writes. This is expected PostgreSQL behavior
		// in testcontainers without production tuning. Production environments with
		// proper shared_buffers and connection pooling show lower tail latencies.
		m := runSpike(t, "create-job-group", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestJobGroups_List(t *testing.T) {
	mustClean(t)
	projectID := "proj-jg-list-" + newID()
	for range 20 {
		seedJobGroup(t, projectID)
	}

	tgt := newTargeter("GET", "/v1/job-groups/?project_id="+projectID, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-job-groups", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-job-groups", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-job-groups", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestJobGroups_Get(t *testing.T) {
	mustClean(t)
	projectID := "proj-jg-get-" + newID()
	groupID := seedJobGroup(t, projectID)

	tgt := newTargeter("GET", "/v1/job-groups/"+groupID+"/", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "get-job-group", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "get-job-group", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "get-job-group", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestJobGroups_Update(t *testing.T) {
	mustClean(t)
	projectID := "proj-jg-upd-" + newID()
	groupID := seedJobGroup(t, projectID)

	var counter atomic.Int64
	tgt := newTargeter("PATCH", "/v1/job-groups/"+groupID+"/", func() []byte {
		n := counter.Add(1)
		return []byte(fmt.Sprintf(`{"description":"updated group %d"}`, n))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "update-job-group", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "update-job-group", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "update-job-group", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestJobGroups_Delete(t *testing.T) {
	mustClean(t)
	projectID := "proj-jg-del-" + newID()

	groupIDs := make([]string, 200)
	for i := range 200 {
		groupIDs[i] = seedJobGroup(t, projectID)
	}

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		pos := i % int64(len(groupIDs))
		tgt.Method = "DELETE"
		tgt.URL = baseURL + "/v1/job-groups/" + groupIDs[pos] + "/"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{testInternalSecret},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "delete-job-group", tgt)
		assertSuccessRate(t, m, 0.80)
		assertNoServerErrors(t, m)
	})
}

func TestJobGroups_ListJobsByGroup(t *testing.T) {
	mustClean(t)
	projectID := "proj-jg-jobs-" + newID()
	groupID := seedJobGroup(t, projectID)
	for range 10 {
		seedJob(t, projectID)
	}

	tgt := newTargeter("GET", "/v1/job-groups/"+groupID+"/jobs", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-jobs-by-group", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-jobs-by-group", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
}

func TestJobGroups_PauseAll(t *testing.T) {
	mustClean(t)
	projectID := "proj-jg-pause-" + newID()
	groupID := seedJobGroup(t, projectID)

	tgt := newTargeter("POST", "/v1/job-groups/"+groupID+"/pause-all", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "pause-all-jobs", tgt)
		assertSuccessRate(t, m, 0.99)
		assertNoServerErrors(t, m)
	})
}

func TestJobGroups_ResumeAll(t *testing.T) {
	mustClean(t)
	projectID := "proj-jg-resume-" + newID()
	groupID := seedJobGroup(t, projectID)

	tgt := newTargeter("POST", "/v1/job-groups/"+groupID+"/resume-all", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "resume-all-jobs", tgt)
		assertSuccessRate(t, m, 0.99)
		assertNoServerErrors(t, m)
	})
}

func TestJobGroups_Stats(t *testing.T) {
	mustClean(t)
	projectID := "proj-jg-stats-" + newID()
	groupID := seedJobGroup(t, projectID)

	tgt := newTargeter("GET", "/v1/job-groups/"+groupID+"/stats", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "job-group-stats", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "job-group-stats", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "job-group-stats", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}
