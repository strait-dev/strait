//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"
)

func TestIntegration_MarkWorkerTaskResultReceivedByAssignment_BindsAndPersistsResult(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedWorkerTaskForBinding(t, ctx, q, 3)

	staleChecks := []struct {
		name      string
		taskID    string
		workerID  string
		projectID string
		runID     string
		attempt   int
	}{
		{name: "wrong task", taskID: uuid.Must(uuid.NewV7()).String(), workerID: workerID, projectID: projectID, runID: runID, attempt: 3},
		{name: "wrong worker", taskID: taskID, workerID: "worker-other", projectID: projectID, runID: runID, attempt: 3},
		{name: "wrong project", taskID: taskID, workerID: workerID, projectID: "project-other", runID: runID, attempt: 3},
		{name: "wrong run", taskID: taskID, workerID: workerID, projectID: projectID, runID: uuid.Must(uuid.NewV7()).String(), attempt: 3},
		{name: "wrong attempt", taskID: taskID, workerID: workerID, projectID: projectID, runID: runID, attempt: 2},
		{name: "missing attempt", taskID: taskID, workerID: workerID, projectID: projectID, runID: runID, attempt: 0},
	}
	for _, tc := range staleChecks {
		t.Run(tc.name, func(t *testing.T) {
			marked, err := q.MarkWorkerTaskResultReceivedByAssignment(
				ctx,
				tc.taskID,
				tc.workerID,
				tc.projectID,
				tc.runID,
				tc.attempt,
				"success",
				"",
				[]byte(`{"stale":true}`),
				11,
			)
			require.NoError(t, err)
			require.False(t, marked)

		})
	}

	marked, err := q.MarkWorkerTaskResultReceivedByAssignment(
		ctx,
		taskID,
		workerID,
		projectID,
		runID,
		3,
		"success",
		"",
		[]byte(`{"ok":true}`),
		42,
	)
	require.NoError(t, err)
	require.True(t, marked)

	task, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WorkerTaskStatusResultReceived,

		task.
			Status)
	require.EqualValues(t, 3, task.
		Attempt)
	require.NotNil(t, task.
		Result,
	)
	require.False(t, task.Result.
		Status !=
		"success" ||
		task.Result.
			DurationMS !=
			42 ||
		task.Result.ReceivedAt ==
			nil)

	var payload map[string]bool
	require.NoError(t, json.
		Unmarshal(task.Result.
			Output, &payload,
		))
	require.True(t, payload["ok"])

}

func TestIntegration_ClaimRecoverableWorkerTaskResults_ClaimsOnlyOldExecutingHandoffs(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	projectID, workerID, runID, taskID := seedWorkerTaskForBinding(t, ctx, q, 1)
	marked, err := q.MarkWorkerTaskResultReceivedByAssignment(ctx, taskID, workerID, projectID, runID, 1, "success", "", []byte(`{"ok":true}`), 7)
	require.NoError(t, err)
	require.True(t, marked)

	if _, err := env.DB.Pool.Exec(ctx, `UPDATE worker_tasks SET result_received_at = NOW() - INTERVAL '10 minutes' WHERE id = $1`, taskID); err != nil {
		require.Failf(t, "test failure",

			"age result handoff: %v", err)
	}

	claimed, err := q.ClaimRecoverableWorkerTaskResults(ctx, time.Now().Add(-5*time.Minute), 10)
	require.NoError(t, err)
	require.Len(t, claimed,

		1)
	require.False(t, claimed[0].ID !=
		taskID ||
		claimed[0].Status !=
			domain.
				WorkerTaskStatusFinalizing,
	)
	require.False(t, claimed[0].Result ==
		nil ||
		claimed[0].Result.
			Status !=
			"success" ||
		claimed[0].Result.DurationMS !=
			7)

	claimedAgain, err := q.ClaimRecoverableWorkerTaskResults(ctx, time.Now().Add(-5*time.Minute), 10)
	require.NoError(t, err)
	require.Len(t, claimedAgain,

		0)
	require.NoError(t, q.ResetWorkerTaskFinalizingToResultReceived(ctx, taskID))

	task, err := q.GetWorkerTask(ctx, taskID)
	require.NoError(t, err)
	require.Equal(t, domain.
		WorkerTaskStatusResultReceived,

		task.
			Status)

}

func TestIntegration_MarkWorkerTaskResultReceived_SkipsDuplicateHandoffWrites(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	_, _, _, taskID := seedWorkerTaskForBinding(t, ctx, q, 1)
	marked, err := q.MarkWorkerTaskResultReceived(ctx, taskID)
	require.NoError(t, err)
	require.True(t, marked)

	var receivedXmin string
	require.NoError(t, env.
		DB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM worker_tasks
		WHERE id = $1`,
		taskID,
	).Scan(&receivedXmin))

	markedAgain, err := q.MarkWorkerTaskResultReceived(ctx, taskID)
	require.NoError(t, err)
	require.True(t, markedAgain)

	var duplicateReceivedXmin string
	require.NoError(t, env.
		DB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text
		FROM worker_tasks
		WHERE id = $1`,
		taskID,
	).Scan(&duplicateReceivedXmin))
	require.Equal(t, receivedXmin,

		duplicateReceivedXmin,
	)

}

func TestIntegration_UpdateWorkerTaskStatus_SkipsDuplicateStatusWrites(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t, ctx)

	q := store.New(env.DB.Pool)
	_, _, _, taskID := seedWorkerTaskForBinding(t, ctx, q, 1)
	require.NoError(t, q.UpdateWorkerTaskStatus(
		ctx, taskID, domain.
			WorkerTaskStatusAccepted,
	))

	var acceptedXmin string
	var acceptedAt time.Time
	require.NoError(t, env.
		DB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text, accepted_at
		FROM worker_tasks
		WHERE id = $1`,

		taskID).Scan(&acceptedXmin,
		&acceptedAt))
	require.NoError(t, q.UpdateWorkerTaskStatus(
		ctx, taskID, domain.
			WorkerTaskStatusAccepted,
	))

	var duplicateAcceptedXmin string
	var duplicateAcceptedAt time.Time
	require.NoError(t, env.
		DB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text, accepted_at
		FROM worker_tasks
		WHERE id = $1`,

		taskID).Scan(&duplicateAcceptedXmin,
		&duplicateAcceptedAt,
	))
	require.Equal(t, acceptedXmin,

		duplicateAcceptedXmin,
	)
	require.True(t, duplicateAcceptedAt.
		Equal(acceptedAt))
	require.NoError(t, q.UpdateWorkerTaskStatus(
		ctx, taskID, domain.
			WorkerTaskStatusCompleted,
	))

	var completedXmin string
	var finishedAt time.Time
	require.NoError(t, env.
		DB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text, finished_at
		FROM worker_tasks
		WHERE id = $1`,

		taskID).Scan(&completedXmin,
		&finishedAt),
	)
	require.NotEqual(t, duplicateAcceptedXmin,

		completedXmin,
	)
	require.NoError(t, q.UpdateWorkerTaskStatus(
		ctx, taskID, domain.
			WorkerTaskStatusCompleted,
	))

	var duplicateCompletedXmin string
	var duplicateFinishedAt time.Time
	require.NoError(t, env.
		DB.
		Pool.QueryRow(ctx,
		`
		SELECT xmin::text, finished_at
		FROM worker_tasks
		WHERE id = $1`,

		taskID).Scan(&duplicateCompletedXmin,
		&duplicateFinishedAt,
	))
	require.Equal(t, completedXmin,

		duplicateCompletedXmin,
	)
	require.True(t, duplicateFinishedAt.
		Equal(finishedAt))

}

func seedWorkerTaskForBinding(t *testing.T, ctx context.Context, q *store.Queries, attempt int) (projectID, workerID, runID, taskID string) {
	t.Helper()

	projectID = "project-" + uuid.Must(uuid.NewV7()).String()
	workerID = "worker-" + uuid.Must(uuid.NewV7()).String()
	runID = uuid.Must(uuid.NewV7()).String()
	taskID = uuid.Must(uuid.NewV7()).String()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,
		OrgID: "org-worker-binding",

		Name: "worker-binding",
	}))

	job := testutil.MustCreateJob(t, ctx, q, &testutil.JobOpts{ProjectID: &projectID})
	executing := domain.StatusExecuting
	run := testutil.BuildRun(job, &testutil.RunOpts{ID: &runID, Status: &executing})
	run.Attempt = attempt
	run.ExecutionMode = domain.ExecutionModeWorker
	require.NoError(t, q.CreateRun(ctx,
		run))
	require.NoError(t, q.RegisterWorker(ctx, &domain.
		Worker{ID: workerID,
		ProjectID: projectID, QueueName: "default",

		Hostname: "host", Version: "1.0", Status: domain.
				WorkerStatusActive}))
	require.NoError(t, q.CreateWorkerTask(ctx, &domain.WorkerTask{ID: taskID,
		WorkerID: workerID, RunID: runID,

		ProjectID: projectID, Attempt: attempt, Status: domain.
				WorkerTaskStatusAssigned,
	}),
	)

	return projectID, workerID, runID, taskID
}
