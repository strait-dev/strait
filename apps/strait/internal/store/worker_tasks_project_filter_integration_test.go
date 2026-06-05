//go:build integration

package store_test

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_ListWorkerTasksByWorker_ProjectFilter pins the store-layer
// project boundary: ListWorkerTasksByWorker scopes by project_id, so a
// wrong-project query returns no rows even when the worker_id matches existing
// task rows.
//
// Pre-fix the query joined only on worker_id, relying entirely on the
// handler-layer GetWorker(project) check. Layered defense at the SQL boundary
// prevents any future caller — or any future cross-project worker_id artifact
func TestIntegration_ListWorkerTasksByWorker_ProjectFilter(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)

	const workerID = "shared-id"
	const projectA = "proj-A"
	const projectB = "proj-B"
	require.NoError(t, q.RegisterWorker(ctx, &domain.
		Worker{
		ID: workerID, ProjectID: projectA,
		QueueName: "q",
		Hostname:  "host-a", Version: "1.0", Status: domain.WorkerStatusActive}))

	// Two tasks for project A under this worker.
	for _, taskID := range []string{"task-a1", "task-a2"} {
		require.NoError(t, q.CreateWorkerTask(ctx, &domain.WorkerTask{ID: taskID, WorkerID: workerID, RunID: "run-" +

			taskID, ProjectID: projectA, Status: domain.WorkerTaskStatusAssigned}))

	}

	// Project A query returns both.
	gotA, err := q.ListWorkerTasksByWorker(ctx, workerID, projectA, "", 100, 0)
	require.NoError(t, err)
	assert.Len(t, gotA, 2)

	// Project B query returns zero, even though worker_id matches.
	gotB, err := q.ListWorkerTasksByWorker(ctx, workerID, projectB, "", 100, 0)
	require.NoError(t, err)
	assert.Len(t, gotB, 0)

}
