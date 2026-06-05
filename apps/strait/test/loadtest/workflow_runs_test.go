//go:build loadtest

package loadtest

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
	vegeta "github.com/tsenart/vegeta/v12/lib"
)

func TestWorkflowRuns_Get(t *testing.T) {
	mustClean(t)
	projectID := "proj-wf-run-get-" + newID()
	workflowRunID := seedWorkflowRun(t, projectID)

	tgt := newTargeter("GET", "/v1/workflow-runs/"+workflowRunID+"/", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "get-workflow-run", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "get-workflow-run", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "get-workflow-run", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestWorkflowRuns_ListSteps(t *testing.T) {
	mustClean(t)
	projectID := "proj-wf-run-steps-" + newID()
	workflowRunID := seedWorkflowRun(t, projectID)

	tgt := newTargeter("GET", "/v1/workflow-runs/"+workflowRunID+"/steps", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "list-workflow-run-steps", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "list-workflow-run-steps", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "list-workflow-run-steps", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestWorkflowRuns_GetLabels(t *testing.T) {
	mustClean(t)
	projectID := "proj-wf-run-labels-" + newID()
	workflowRunID := seedWorkflowRun(t, projectID)

	tgt := newTargeter("GET", "/v1/workflow-runs/"+workflowRunID+"/labels", nil)

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "get-workflow-run-labels", tgt)
		assertLatencySLA(t, m)
		assertSuccessRate(t, m, 0.99)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "get-workflow-run-labels", tgt)
		assertSuccessRate(t, m, 0.95)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "get-workflow-run-labels", tgt)
		assertSuccessRate(t, m, 0.90)
		assertNoServerErrors(t, m)
	})
}

func TestWorkflowRuns_Cancel(t *testing.T) {
	mustClean(t)
	projectID := "proj-wf-run-cancel-" + newID()
	workflowRunIDs := make([]string, 200)
	for i := range 200 {
		workflowRunIDs[i] = seedWorkflowRun(t, projectID)
	}

	var idx atomic.Int64
	tgt := func(tgt *vegeta.Target) error {
		i := idx.Add(1) - 1
		pos := i % int64(len(workflowRunIDs))
		tgt.Method = "DELETE"
		tgt.URL = baseURL + "/v1/workflow-runs/" + workflowRunIDs[pos] + "/"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		m := runBaseline(t, "cancel-workflow-run", tgt)
		assertSuccessRate(t, m, 0.80)
		assertNoServerErrors(t, m)
	})
	t.Run("stress", func(t *testing.T) {
		m := runStress(t, "cancel-workflow-run", tgt)
		assertNoServerErrors(t, m)
	})
	t.Run("spike", func(t *testing.T) {
		m := runSpike(t, "cancel-workflow-run", tgt)
		assertNoServerErrors(t, m)
	})
}

func TestWorkflowRuns_PauseResume(t *testing.T) {
	mustClean(t)
	projectID := "proj-wf-run-pause-resume-" + newID()
	workflowRunIDs := make([]string, 1000)
	for i := range 1000 {
		workflowRunIDs[i] = seedWorkflowRun(t, projectID)
	}

	ctx := context.Background()
	for _, workflowRunID := range workflowRunIDs {
		run, err := testStore.GetWorkflowRun(ctx, workflowRunID)
		require.NoError(t,

			err)

		if run.Status == domain.WfStatusPending {
			require.NoError(t,

				testStore.
					UpdateWorkflowRunStatus(ctx, workflowRunID,
						domain.
							WfStatusPending,

						domain.WfStatusRunning,

						nil))

		}
	}

	var pauseIdx atomic.Int64
	pauseTargeter := func(tgt *vegeta.Target) error {
		i := pauseIdx.Add(1) - 1
		pos := i % int64(len(workflowRunIDs))
		tgt.Method = "POST"
		tgt.URL = baseURL + "/v1/workflow-runs/" + workflowRunIDs[pos] + "/pause"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
			"Content-Type":      []string{"application/json"},
		}
		return nil
	}

	var resumeIdx atomic.Int64
	resumeTargeter := func(tgt *vegeta.Target) error {
		i := resumeIdx.Add(1) - 1
		pos := i % int64(len(workflowRunIDs))
		tgt.Method = "POST"
		tgt.URL = baseURL + "/v1/workflow-runs/" + workflowRunIDs[pos] + "/resume"
		tgt.Header = http.Header{
			"X-Internal-Secret": []string{"test-secret-value"},
			"Content-Type":      []string{"application/json"},
		}
		return nil
	}

	t.Run("baseline", func(t *testing.T) {
		pauseMetrics := runBaseline(t, "pause-workflow-run", pauseTargeter, withRate(50), withDuration(2*time.Second))
		assertSuccessRate(t, pauseMetrics, 0.95)
		assertNoServerErrors(t, pauseMetrics)

		resumeMetrics := runBaseline(t, "resume-workflow-run", resumeTargeter, withRate(50), withDuration(2*time.Second))
		assertStatusCodes(t, resumeMetrics, "200", "400", "503")
	})
	t.Run("stress", func(t *testing.T) {
		pauseMetrics := runStress(t, "pause-workflow-run", pauseTargeter, withRate(200), withWorkers(20), withDuration(3*time.Second))
		assertSuccessRate(t, pauseMetrics, 0.80)
		assertNoServerErrors(t, pauseMetrics)

		resumeMetrics := runStress(t, "resume-workflow-run", resumeTargeter, withRate(200), withWorkers(20), withDuration(3*time.Second))
		assertStatusCodes(t, resumeMetrics, "200", "400", "503")
	})
	t.Run("spike", func(t *testing.T) {
		pauseMetrics := runSpike(t, "pause-workflow-run", pauseTargeter, withWorkers(20), withDuration(3*time.Second))
		assertSuccessRate(t, pauseMetrics, 0.80)
		assertNoServerErrors(t, pauseMetrics)

		resumeMetrics := runSpike(t, "resume-workflow-run", resumeTargeter, withWorkers(20), withDuration(3*time.Second))
		assertStatusCodes(t, resumeMetrics, "200", "400", "503")
	})
}
