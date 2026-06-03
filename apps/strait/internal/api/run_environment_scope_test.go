package api

import (
	"context"
	"slices"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestListDeadLetterRunsForEnvironment_PaginatesUntilLimit(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)
	firstPage := make([]domain.JobRun, 0, 25)
	for i := range 24 {
		firstPage = append(firstPage, domain.JobRun{
			ID:        "staging-run",
			JobID:     "job-staging",
			ProjectID: "proj-1",
			CreatedAt: base.Add(-time.Duration(i) * time.Second),
		})
	}
	firstPage = append(firstPage, domain.JobRun{
		ID:        "prod-run-1",
		JobID:     "job-prod",
		ProjectID: "proj-1",
		CreatedAt: base.Add(-24 * time.Second),
	})
	secondPageCursor := firstPage[len(firstPage)-1].CreatedAt

	listCalls := 0
	jobLookups := make(map[string]int)
	ms := &APIStoreMock{
		ListDeadLetterRunsFunc: func(_ context.Context, projectID string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
			if projectID != "proj-1" {
				t.Fatalf("projectID = %q, want proj-1", projectID)
			}
			if limit != 25 {
				t.Fatalf("limit = %d, want minimum fetch limit 25", limit)
			}
			listCalls++
			switch listCalls {
			case 1:
				if cursor != nil {
					t.Fatalf("first cursor = %v, want nil", cursor)
				}
				return firstPage, nil
			case 2:
				if cursor == nil || !cursor.Equal(secondPageCursor) {
					t.Fatalf("second cursor = %v, want %v", cursor, secondPageCursor)
				}
				return []domain.JobRun{{
					ID:        "prod-run-2",
					JobID:     "job-prod",
					ProjectID: "proj-1",
					CreatedAt: base.Add(-25 * time.Second),
				}}, nil
			default:
				t.Fatalf("unexpected list call %d", listCalls)
				return nil, nil
			}
		},
		GetJobFunc: func(_ context.Context, jobID string) (*domain.Job, error) {
			jobLookups[jobID]++
			switch jobID {
			case "job-prod":
				return &domain.Job{ID: jobID, ProjectID: "proj-1", EnvironmentID: "env-prod"}, nil
			case "job-staging":
				return &domain.Job{ID: jobID, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
			default:
				t.Fatalf("unexpected job lookup %q", jobID)
				return nil, nil
			}
		},
	}
	srv := &Server{store: ms}

	runs, err := srv.listDeadLetterRunsForEnvironment(envScopedRunCtx(), "proj-1", 2, nil)
	if err != nil {
		t.Fatalf("listDeadLetterRunsForEnvironment: %v", err)
	}

	gotIDs := runIDs(runs)
	if !slices.Equal(gotIDs, []string{"prod-run-1", "prod-run-2"}) {
		t.Fatalf("runs = %v, want prod runs from both pages", gotIDs)
	}
	if jobLookups["job-prod"] != 1 || jobLookups["job-staging"] != 1 {
		t.Fatalf("job lookups = %#v, want one lookup per job due cache", jobLookups)
	}
}

func TestBulkReplayDeadLetterRunsForEnvironment_ReplaysAllowedRunIDs(t *testing.T) {
	t.Parallel()

	var replayedRunIDs []string
	jobLookups := make(map[string]int)
	ms := &APIStoreMock{
		ListDeadLetterRunsFunc: func(_ context.Context, _ string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
			if limit != 4 {
				t.Fatalf("limit = %d, want replay limit plus one", limit)
			}
			if cursor != nil {
				return nil, nil
			}
			return []domain.JobRun{
				{ID: "run-prod-1", JobID: "job-prod", ProjectID: "proj-1", CreatedAt: time.Now().Add(-time.Second)},
				{ID: "run-staging", JobID: "job-staging", ProjectID: "proj-1", CreatedAt: time.Now().Add(-2 * time.Second)},
				{ID: "run-prod-2", JobID: "job-prod", ProjectID: "proj-1", CreatedAt: time.Now().Add(-3 * time.Second)},
			}, nil
		},
		GetJobFunc: func(_ context.Context, jobID string) (*domain.Job, error) {
			jobLookups[jobID]++
			switch jobID {
			case "job-prod":
				return &domain.Job{ID: jobID, ProjectID: "proj-1", EnvironmentID: "env-prod"}, nil
			case "job-staging":
				return &domain.Job{ID: jobID, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
			default:
				t.Fatalf("unexpected job lookup %q", jobID)
				return nil, nil
			}
		},
		BulkReplayDeadLetterRunsFunc: func(_ context.Context, runIDs []string, projectID string, limit int) ([]domain.JobRun, error) {
			if projectID != "" || limit != 0 {
				t.Fatalf("projectID = %q, limit = %d, want explicit run-id replay", projectID, limit)
			}
			replayedRunIDs = slices.Clone(runIDs)
			return []domain.JobRun{{ID: "replayed-1"}, {ID: "replayed-2"}}, nil
		},
	}
	srv := &Server{store: ms}

	runs, err := srv.bulkReplayDeadLetterRunsForEnvironment(envScopedRunCtx(), "proj-1", 3)
	if err != nil {
		t.Fatalf("bulkReplayDeadLetterRunsForEnvironment: %v", err)
	}

	if len(runs) != 2 {
		t.Fatalf("replayed runs = %d, want 2", len(runs))
	}
	if !slices.Equal(replayedRunIDs, []string{"run-prod-1", "run-prod-2"}) {
		t.Fatalf("replayed run IDs = %v, want only env-prod runs", replayedRunIDs)
	}
	if jobLookups["job-prod"] != 1 || jobLookups["job-staging"] != 1 {
		t.Fatalf("job lookups = %#v, want one lookup per job due cache", jobLookups)
	}
}

func TestBulkReplayDeadLetterRunsForEnvironment_NoMatchesSkipsReplay(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListDeadLetterRunsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return []domain.JobRun{{ID: "run-staging", JobID: "job-staging", ProjectID: "proj-1", CreatedAt: time.Now()}}, nil
		},
		GetJobFunc: func(_ context.Context, jobID string) (*domain.Job, error) {
			if jobID != "job-staging" {
				t.Fatalf("jobID = %q, want job-staging", jobID)
			}
			return &domain.Job{ID: jobID, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
		},
		BulkReplayDeadLetterRunsFunc: func(context.Context, []string, string, int) ([]domain.JobRun, error) {
			t.Fatal("BulkReplayDeadLetterRuns must not be called when no runs match the environment")
			return nil, nil
		},
	}
	srv := &Server{store: ms}

	runs, err := srv.bulkReplayDeadLetterRunsForEnvironment(envScopedRunCtx(), "proj-1", 10)
	if err != nil {
		t.Fatalf("bulkReplayDeadLetterRunsForEnvironment: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("runs = %v, want empty result", runs)
	}
}

func TestRunMatchesEnvironment_UnscopedContextSkipsJobLookup(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(context.Context, string) (*domain.Job, error) {
			t.Fatal("GetJob must not be called without an environment scope")
			return nil, nil
		},
	}
	srv := &Server{store: ms}

	allowed, err := srv.runMatchesEnvironment(context.Background(), domain.JobRun{ID: "run-1", JobID: "job-1"}, map[string]bool{})
	if err != nil {
		t.Fatalf("runMatchesEnvironment: %v", err)
	}
	if !allowed {
		t.Fatal("unscoped context should allow the run")
	}
}

func runIDs(runs []domain.JobRun) []string {
	ids := make([]string, 0, len(runs))
	for _, run := range runs {
		ids = append(ids, run.ID)
	}
	return ids
}
