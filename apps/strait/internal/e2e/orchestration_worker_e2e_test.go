//go:build integration

package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestOrchestrationWorkerE2E_TriggerCreatesClaimableWorkerRuns(t *testing.T) {
	mustClean(t)

	ctx := context.Background()
	projectID := "proj-worker-routing-" + newID()
	queueName := "priority"
	runCount := 3

	job := createWorkerJob(t, projectID, "Worker Routing Job", "worker-routing-"+newID(), queueName)
	jobID := asString(t, job, "id")
	queueRefs := []domain.WorkerQueueRef{{ProjectID: projectID, QueueName: queueName}}
	q := newIsolatedQueue(t)
	primeWorkerQueue(t, q, queueRefs)

	runIDs := make(map[string]bool, runCount)
	for i := range runCount {
		w := doRequest(t, http.MethodPost, "/v1/jobs/"+jobID+"/trigger", fmt.Sprintf(`{"payload":{"n":%d}}`, i), projectID)
		require.Equal(t, http.
			StatusCreated,
			w.Code,
		)

		resp := mustDecodeObject(t, w)
		runIDs[asString(t, resp, "id")] = true
	}

	rows, err := testEnv.DB.Pool.Query(ctx,
		`SELECT jr.id, jr.execution_mode, jr.queue_name
		 FROM job_runs jr
		 WHERE jr.job_id = $1`,
		jobID,
	)
	require.NoError(t, err)

	defer rows.Close()

	seen := 0
	for rows.Next() {
		var runID, runMode, runQueue string
		require.NoError(t, rows.
			Scan(&runID,
				&runMode,
				&runQueue,
			))
		require.True(t, runIDs[runID])
		require.Equal(t, string(domain.
			ExecutionModeWorker,
		), runMode)
		require.Equal(t, queueName,

			runQueue,
		)

		seen++
	}
	require.NoError(t, rows.
		Err())
	require.Equal(t, runCount,

		seen,
	)

	claimed := dequeueWorkerRunsEventually(t, q, runCount, queueRefs)
	require.Len(t, claimed,

		runCount,
	)

	for _, run := range claimed {
		require.True(t, runIDs[run.ID])
		require.Equal(t, domain.
			StatusExecuting,
			run.
				Status,
		)

	}
}
