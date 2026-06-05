//go:build integration

package grpc

import (
	"context"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_RegisterWorker_SameIDAcrossProjectsCreatesSeparateRows
// proves worker IDs are tenant-local in the persistent workers table. A
// project must not be able to squat on a common worker ID and block another
// project from registering the same name.
func TestIntegration_RegisterWorker_SameIDAcrossProjectsCreatesSeparateRows(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)

	const workerID = "shared-id"
	const projectA = "proj-A"
	const projectB = "proj-B"

	// Project A registers first.
	wA := &domain.Worker{
		ID:        workerID,
		ProjectID: projectA,
		QueueName: "queue-a",
		Hostname:  "host-a",
		Version:   "1.0.0",
		Status:    domain.WorkerStatusActive,
	}
	require.NoError(t,

		q.RegisterWorker(ctx,
			wA))

	wB := &domain.Worker{
		ID:        workerID,
		ProjectID: projectB,
		QueueName: "queue-b",
		Hostname:  "host-b",
		Version:   "9.9.9",
		Status:    domain.WorkerStatusDraining,
	}
	require.NoError(t,

		q.RegisterWorker(ctx,
			wB))

	rows, err := env.DB.Pool.Query(ctx,
		`SELECT project_id, queue_name, hostname, version, status
		 FROM workers
		 WHERE id = $1
		 ORDER BY project_id`,
		workerID,
	)
	require.NoError(t,

		err)

	defer rows.Close()

	type workerRow struct {
		projectID string
		queue     string
		hostname  string
		version   string
		status    string
	}
	var got []workerRow
	for rows.Next() {
		var row workerRow
		require.NoError(t,

			rows.Scan(&row.projectID,
				&row.
					queue, &row.hostname, &row.
					version, &row.status))

		got = append(got, row)
	}
	require.NoError(t,

		rows.Err())
	require.Len(t, got,

		2)
	require.Equal(t,
		(workerRow{projectID: projectA,
			queue: "queue-a", hostname: "host-a", version: "1.0.0",

			status: string(domain.WorkerStatusActive)}), got[0])
	require.Equal(t,
		(workerRow{projectID: projectB,
			queue: "queue-b", hostname: "host-b", version: "9.9.9",

			status: string(domain.WorkerStatusDraining)}), got[1])

}

// TestIntegration_RegisterWorker_SameProjectStillUpserts confirms the
// project-equal happy path still updates queue/hostname/version/status.
func TestIntegration_RegisterWorker_SameProjectStillUpserts(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	const workerID = "stable-id"
	const projectID = "proj-stable"
	require.NoError(t,

		q.RegisterWorker(ctx,
			&domain.
				Worker{ID: workerID, ProjectID: projectID, QueueName: "old-queue",
				Hostname: "old-host", Version: "0.1.0", Status: domain.WorkerStatusActive,
			}))
	require.NoError(t,

		q.RegisterWorker(ctx,
			&domain.
				Worker{ID: workerID, ProjectID: projectID, QueueName: "new-queue",
				Hostname: "new-host", Version: "0.2.0", Status: domain.WorkerStatusDraining,
			}))

	var (
		gotQueue, gotHostname, gotVersion, gotStatus string
	)
	err := env.DB.Pool.QueryRow(ctx,
		`SELECT queue_name, hostname, version, status FROM workers WHERE id = $1 AND project_id = $2`,
		workerID, projectID,
	).Scan(&gotQueue, &gotHostname, &gotVersion, &gotStatus)
	require.NoError(t,

		err)
	assert.False(t, gotQueue !=
		"new-queue" ||
		gotHostname !=
			"new-host" || gotVersion !=
		"0.2.0")
	assert.Equal(t, string(domain.
		WorkerStatusDraining,
	), gotStatus)

}

func TestIntegration_WorkerTasksReferenceProjectScopedWorker(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	const workerID = "shared-id"
	require.NoError(t,

		q.RegisterWorker(ctx,
			&domain.
				Worker{ID: workerID, ProjectID: "proj-a", QueueName: "q",

				Status: domain.WorkerStatusActive}))

	err := q.CreateWorkerTask(ctx, &domain.WorkerTask{
		ID:        "task-cross-project",
		WorkerID:  workerID,
		ProjectID: "proj-b",
		RunID:     "run-1",
		Attempt:   1,
		Status:    domain.WorkerTaskStatusAssigned,
	})
	require.Error(t,
		err,
	)

}

// TestIntegration_GetWorkerProjectByID_NotFoundIsClean confirms the lookup
// helper used at the stream layer treats a missing row as `(false, nil)`,
// not an error.
func TestIntegration_GetWorkerProjectByID_NotFoundIsClean(t *testing.T) {
	ctx := context.Background()
	env := cleanIntegrationEnv(t, ctx)

	q := store.New(env.DB.Pool)
	_, ok, err := q.GetWorkerProjectByID(ctx, "does-not-exist")
	require.NoError(t,

		err)
	require.False(t,
		ok,
	)

}
