//go:build loadtest

package loadtest

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	vegeta "github.com/tsenart/vegeta/v12/lib"
)

func TestWorkflows_CreateWorkflow(t *testing.T) {
	mustClean(t)
	projectID := "proj-wf-create-" + newID()

	tgt := newTargeter("POST", "/v1/workflows/", func() []byte {
		slug := "wf-" + newID()
		return []byte(fmt.Sprintf(
			`{"project_id":"%s","name":"load-%s","slug":"%s","enabled":true}`,
			projectID, slug, slug,
		))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "create-workflow", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "create-workflow", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "create-workflow", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestWorkflows_ListWorkflows(t *testing.T) {
	mustClean(t)
	projectID := "proj-wf-list-" + newID()
	for range 20 {
		seedWorkflow(t, projectID)
	}

	tgt := newTargeter("GET", "/v1/workflows/?project_id="+projectID, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-workflows", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-workflows", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-workflows", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestWorkflows_GetWorkflow(t *testing.T) {
	mustClean(t)
	projectID := "proj-wf-get-" + newID()
	wfID := seedWorkflow(t, projectID)

	tgt := newTargeter("GET", "/v1/workflows/"+wfID+"/", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "get-workflow", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "get-workflow", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "get-workflow", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestWorkflows_UpdateWorkflow(t *testing.T) {
	mustClean(t)
	projectID := "proj-wf-upd-" + newID()
	wfID := seedWorkflow(t, projectID)

	var counter atomic.Int64
	tgt := newTargeter("PATCH", "/v1/workflows/"+wfID+"/", func() []byte {
		n := counter.Add(1)
		return []byte(fmt.Sprintf(`{"name":"updated-wf-%d"}`, n))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "update-workflow", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "update-workflow", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "update-workflow", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestWorkflows_DeleteWorkflow(t *testing.T) {
	mustClean(t)
	projectID := "proj-wf-del-" + newID()

	wfIDs := make([]string, 200)
	for i := range 200 {
		wfIDs[i] = seedWorkflow(t, projectID)
	}

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		i = i % int64(len(wfIDs))
		tgt.Method = "DELETE"
		tgt.URL = baseURL + "/v1/workflows/" + wfIDs[i] + "/"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "delete-workflow", tgt)
		assertSuccessRate(t, m, 0.80)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "delete-workflow", tgt)
		assertNoServerErrors(t, m)
	})
}

func TestWorkflows_DryRun(t *testing.T) {
	mustClean(t)
	projectID := "proj-wf-dry-" + newID()
	wfID := seedWorkflow(t, projectID)

	tgt := newTargeter("POST", "/v1/workflows/"+wfID+"/dry-run", func() []byte {
		return []byte(`{"payload":{"test":true}}`)
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "dry-run", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "dry-run", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "dry-run", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestWorkflows_Graph(t *testing.T) {
	mustClean(t)
	projectID := "proj-wf-graph-" + newID()
	wfID := seedWorkflow(t, projectID)

	tgt := newTargeter("GET", "/v1/workflows/"+wfID+"/graph", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "workflow-graph", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "workflow-graph", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "workflow-graph", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestWorkflows_CloneWorkflow(t *testing.T) {
	mustClean(t)
	projectID := "proj-wf-clone-" + newID()
	wfID := seedWorkflow(t, projectID)

	tgt := newTargeter("POST", "/v1/workflows/"+wfID+"/clone", func() []byte {
		slug := "clone-wf-" + newID()
		return []byte(fmt.Sprintf(`{"slug":"%s","name":"clone-%s"}`, slug, slug))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "clone-workflow", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "clone-workflow", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "clone-workflow", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestWorkflows_ListVersions(t *testing.T) {
	mustClean(t)
	projectID := "proj-wf-ver-" + newID()
	wfID := seedWorkflow(t, projectID)
	for i := range 5 {
		httpDo(t, "PATCH", "/v1/workflows/"+wfID+"/", fmt.Sprintf(`{"name":"wf-v%d"}`, i+2), nil)
	}

	tgt := newTargeter("GET", "/v1/workflows/"+wfID+"/versions", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-wf-versions", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-wf-versions", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-wf-versions", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestWorkflows_TriggerWorkflow(t *testing.T) {
	mustClean(t)
	projectID := "proj-wf-trig-" + newID()
	wfID := seedWorkflow(t, projectID)

	tgt := newTargeter("POST", "/v1/workflows/"+wfID+"/trigger", func() []byte {
		return []byte(fmt.Sprintf(`{"payload":{"id":"%s"}}`, newID()))
	})

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "trigger-workflow", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "trigger-workflow", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "trigger-workflow", tgt)
		assertSuccessRate(t, m, 0.85)
		assertNoServerErrors(t, m)
	})
}

func TestWorkflows_ListWorkflowRuns(t *testing.T) {
	mustClean(t)
	projectID := "proj-wf-runs-" + newID()
	wfID := seedWorkflow(t, projectID)
	for range 10 {
		httpDo(t, "POST", "/v1/workflows/"+wfID+"/trigger",
			fmt.Sprintf(`{"payload":{"id":"%s"}}`, newID()), nil)
	}

	tgt := newTargeter("GET", "/v1/workflows/"+wfID+"/runs", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-wf-runs", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-wf-runs", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-wf-runs", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestWorkflows_ListWorkflowRunsByProject(t *testing.T) {
	mustClean(t)
	projectID := "proj-wf-runsp-" + newID()
	for range 3 {
		wfID := seedWorkflow(t, projectID)
		for range 5 {
			httpDo(t, "POST", "/v1/workflows/"+wfID+"/trigger",
				fmt.Sprintf(`{"payload":{"id":"%s"}}`, newID()), nil)
		}
	}

	tgt := newTargeter("GET", "/v1/workflow-runs/?project_id="+projectID, nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-wf-runs-project", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-wf-runs-project", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-wf-runs-project", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}
