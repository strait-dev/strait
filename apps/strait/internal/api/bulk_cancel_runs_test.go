package api

import (
	"context"
	"reflect"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
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

	if !reflect.DeepEqual(selection.cancelableIDs, []string{"run-1"}) {
		t.Fatalf("cancelable IDs = %#v, want run-1", selection.cancelableIDs)
	}
	if selection.failed != 3 {
		t.Fatalf("failed = %d, want 3", selection.failed)
	}
	wantResults := []BulkCancelResult{
		{ID: "run-missing", Status: "failed", Error: "run not found"},
		{ID: "run-terminal", Status: string(domain.StatusCompleted), Error: "run already in terminal state"},
		{ID: "run-cross", Status: "failed", Error: "run not found"},
	}
	if !reflect.DeepEqual(selection.results, wantResults) {
		t.Fatalf("results = %#v, want %#v", selection.results, wantResults)
	}
}

func TestSelectBulkCancelableRuns_HidesEnvironmentMismatch(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "project-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-a")
	srv := &Server{store: &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			if id != "job-env-b" {
				t.Fatalf("GetJob id = %q, want job-env-b", id)
			}
			return &domain.Job{ID: id, ProjectID: "project-1", EnvironmentID: "env-b"}, nil
		},
	}}
	runsMap := map[string]*domain.JobRun{
		"run-env-b": {ID: "run-env-b", JobID: "job-env-b", ProjectID: "project-1", Status: domain.StatusQueued},
	}

	selection := srv.selectBulkCancelableRuns(ctx, []string{"run-env-b"}, runsMap)

	if len(selection.cancelableIDs) != 0 {
		t.Fatalf("cancelable IDs = %#v, want none", selection.cancelableIDs)
	}
	if selection.failed != 1 {
		t.Fatalf("failed = %d, want 1", selection.failed)
	}
	wantResults := []BulkCancelResult{{ID: "run-env-b", Status: "failed", Error: "run not found"}}
	if !reflect.DeepEqual(selection.results, wantResults) {
		t.Fatalf("results = %#v, want %#v", selection.results, wantResults)
	}
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

	if canceled != 1 {
		t.Fatalf("canceled = %d, want 1", canceled)
	}
	if selection.failed != 2 {
		t.Fatalf("failed = %d, want 2", selection.failed)
	}
	wantResults := []BulkCancelResult{
		{ID: "run-missing", Status: "failed", Error: "run not found"},
		{ID: "run-2", Status: string(domain.StatusCanceled)},
		{ID: "run-1", Status: string(domain.StatusQueued), Error: "failed to cancel (status may have changed)"},
	}
	if !reflect.DeepEqual(selection.results, wantResults) {
		t.Fatalf("results = %#v, want %#v", selection.results, wantResults)
	}
}
