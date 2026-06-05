package api

import (
	"context"
	"reflect"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestSelectBulkCancelableRuns_PartitionsRequestedRuns(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "project-1")
	srv := &Server{}
	runsMap := map[string]*domain.JobRun{
		"run-1":        {ID: "run-1", ProjectID: "project-1", Status: domain.StatusExecuting},
		"run-terminal": {ID: "run-terminal", ProjectID: "project-1", Status: domain.StatusCompleted},
		"run-cross":    {ID: "run-cross", ProjectID: "project-2", Status: domain.StatusQueued},
	}

	selection := srv.selectBulkCancelableRuns(ctx, []string{"run-missing", "run-1", "run-terminal", "run-cross"}, runsMap)
	require.True(
		t, reflect.
			DeepEqual(selection.
				cancelableIDs, []string{"run-1"}))
	require.EqualValues(t, 3, selection.
		failed,
	)

	wantResults := []BulkCancelResult{
		{ID: "run-missing", Status: "failed", Error: "run not found"},
		{ID: "run-terminal", Status: string(domain.StatusCompleted), Error: "run already in terminal state"},
		{ID: "run-cross", Status: "failed", Error: "run not found"},
	}
	require.True(
		t, reflect.
			DeepEqual(selection.
				results, wantResults))

}

func TestSelectBulkCancelableRuns_HidesEnvironmentMismatch(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "project-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-a")
	srv := &Server{store: &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			require.Equal(t, "job-env-b",
				id)

			return &domain.Job{ID: id, ProjectID: "project-1", EnvironmentID: "env-b"}, nil
		},
	}}
	runsMap := map[string]*domain.JobRun{
		"run-env-b": {ID: "run-env-b", JobID: "job-env-b", ProjectID: "project-1", Status: domain.StatusQueued},
	}

	selection := srv.selectBulkCancelableRuns(ctx, []string{"run-env-b"}, runsMap)
	require.Len(t,
		selection.
			cancelableIDs,
		0)
	require.EqualValues(t, 1, selection.
		failed,
	)

	wantResults := []BulkCancelResult{{ID: "run-env-b", Status: "failed", Error: "run not found"}}
	require.True(
		t, reflect.
			DeepEqual(selection.
				results, wantResults))

}

func TestHandleBulkCancelRunsSkipsStoreCancelWhenNoRunsCancelable(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "project-1")
	ms := &APIStoreMock{
		GetRunsByIDsFunc: func(_ context.Context, ids []string) (map[string]*domain.JobRun, error) {
			require.True(
				t, reflect.
					DeepEqual(ids,
						[]string{"run-terminal", "run-missing"}))

			return map[string]*domain.JobRun{
				"run-terminal": {
					ID:        "run-terminal",
					ProjectID: "project-1",
					Status:    domain.StatusCompleted,
				},
			}, nil
		},
		BulkCancelRunsFunc: func(context.Context, []string, time.Time, string) ([]store.BulkCancelResult, error) {
			require.Fail(t,

				"BulkCancelRuns must not be called when no runs are cancelable")
			return nil, nil
		},
		CancelChildRunsByParentIDsFunc: func(context.Context, []string, time.Time, string) (int64, error) {
			require.Fail(t,

				"CancelChildRunsByParentIDs must not be called when no parent runs were canceled")
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	output, err := srv.handleBulkCancelRuns(ctx, &BulkCancelRunsInput{
		Body: BulkCancelRequest{RunIDs: []string{"run-terminal", "run-missing"}},
	})
	require.NoError(t, err)
	require.False(t, output.
		Body.Canceled !=
		0 ||
		output.Body.Failed != 2 ||
		output.Body.Total != 2,
	)

	wantResults := []BulkCancelResult{
		{ID: "run-terminal", Status: string(domain.StatusCompleted), Error: "run already in terminal state"},
		{ID: "run-missing", Status: "failed", Error: "run not found"},
	}
	require.True(
		t, reflect.
			DeepEqual(output.
				Body.
				Results, wantResults))

}

func TestBulkCancelSelection_AppendStoreResultsReportsRaces(t *testing.T) {
	t.Parallel()

	selection := bulkCancelSelection{
		results:       []BulkCancelResult{{ID: "run-missing", Status: "failed", Error: "run not found"}},
		cancelableIDs: []string{"run-1", "run-2"},
		failed:        1,
	}
	runsMap := map[string]*domain.JobRun{
		"run-1": {ID: "run-1", Status: domain.StatusQueued},
		"run-2": {ID: "run-2", Status: domain.StatusExecuting},
	}

	canceled := selection.appendStoreResults(runsMap, []store.BulkCancelResult{{ID: "run-2", Canceled: true}})
	require.EqualValues(t, 1, canceled)
	require.EqualValues(t, 2, selection.
		failed,
	)

	wantResults := []BulkCancelResult{
		{ID: "run-missing", Status: "failed", Error: "run not found"},
		{ID: "run-2", Status: string(domain.StatusCanceled)},
		{ID: "run-1", Status: string(domain.StatusQueued), Error: "failed to cancel (status may have changed)"},
	}
	require.True(
		t, reflect.
			DeepEqual(selection.
				results, wantResults))

}
