//go:build integration

package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"strait/internal/domain"
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
		if w.Code != http.StatusCreated {
			t.Fatalf("trigger run %d: status=%d body=%s", i, w.Code, w.Body.String())
		}
		resp := mustDecodeObject(t, w)
		runIDs[asString(t, resp, "id")] = true
	}

	rows, err := testEnv.DB.Pool.Query(ctx,
		`SELECT jr.id, jr.execution_mode, jr.queue_name
		 FROM job_runs jr
		 WHERE jr.job_id = $1`,
		jobID,
	)
	if err != nil {
		t.Fatalf("query run routing rows: %v", err)
	}
	defer rows.Close()

	seen := 0
	for rows.Next() {
		var runID, runMode, runQueue string
		if err := rows.Scan(&runID, &runMode, &runQueue); err != nil {
			t.Fatalf("scan routing row: %v", err)
		}
		if !runIDs[runID] {
			t.Fatalf("unexpected run id %q in routing query", runID)
		}
		if runMode != string(domain.ExecutionModeWorker) {
			t.Fatalf("run %s mode = %q, want worker", runID, runMode)
		}
		if runQueue != queueName {
			t.Fatalf("run %s queue = %q, want %q", runID, runQueue, queueName)
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("routing rows error: %v", err)
	}
	if seen != runCount {
		t.Fatalf("routing rows = %d, want %d", seen, runCount)
	}

	claimed := dequeueWorkerRunsEventually(t, q, runCount, queueRefs)
	if len(claimed) != runCount {
		t.Fatalf("DequeueNForWorker returned %d runs, want %d", len(claimed), runCount)
	}
	for _, run := range claimed {
		if !runIDs[run.ID] {
			t.Fatalf("claimed unexpected run id %q", run.ID)
		}
		if run.Status != domain.StatusExecuting {
			t.Fatalf("claimed run %s status = %q, want executing", run.ID, run.Status)
		}
	}
}
