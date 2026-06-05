package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

func envScopedRunCtx() context.Context {
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	return context.WithValue(ctx, ctxActorIDKey, "apikey:test")
}

func newEnvScopedRunServer(t *testing.T) *Server {
	t.Helper()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, runID string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:        runID,
				JobID:     "job-1",
				ProjectID: "proj-1",
				Status:    domain.StatusExecuting,
			}, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", ProjectID: "proj-1", EnvironmentID: "env-staging", Enabled: true}, nil
		},
	}
	return newTestServer(t, ms, &mockQueue{}, nil)
}

func TestRunEnvironmentScope_MutatingHandlersRejectEnvironmentMismatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		call func(*Server, context.Context) error
	}{
		{
			name: "cancel",
			call: func(s *Server, ctx context.Context) error {
				_, err := s.handleCancelRun(ctx, &CancelRunInput{RunID: "run-1"})
				return err
			},
		},
		{
			name: "dependency_status",
			call: func(s *Server, ctx context.Context) error {
				_, err := s.handleGetRunDependencyStatus(ctx, &GetRunDependencyStatusInput{RunID: "run-1"})
				return err
			},
		},
		{
			name: "replay",
			call: func(s *Server, ctx context.Context) error {
				_, err := s.handleReplayRun(ctx, &ReplayRunInput{RunID: "run-1"})
				return err
			},
		},
		{
			name: "reschedule",
			call: func(s *Server, ctx context.Context) error {
				_, err := s.handleRescheduleRun(ctx, &RescheduleRunInput{
					RunID: "run-1",
					Body:  RescheduleRunRequest{ScheduledAt: time.Now().Add(time.Hour)},
				})
				return err
			},
		},
		{
			name: "pause",
			call: func(s *Server, ctx context.Context) error {
				_, err := s.handlePauseRun(ctx, &PauseRunInput{RunID: "run-1"})
				return err
			},
		},
		{
			name: "resume",
			call: func(s *Server, ctx context.Context) error {
				ms := s.store.(*APIStoreMock)
				ms.GetRunFunc = func(_ context.Context, runID string) (*domain.JobRun, error) {
					return &domain.JobRun{ID: runID, JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusPaused}, nil
				}
				_, err := s.handleResumeRun(ctx, &ResumeRunInput{RunID: "run-1"})
				return err
			},
		},
		{
			name: "restart",
			call: func(s *Server, ctx context.Context) error {
				_, err := s.handleRestartRun(ctx, &RestartRunInput{RunID: "run-1"})
				return err
			},
		},
		{
			name: "bulk_replay_runs",
			call: func(s *Server, ctx context.Context) error {
				out, err := s.handleBulkReplayRuns(ctx, &BulkReplayRunsInput{Body: BulkReplayRunsRequest{RunIDs: []string{"run-1"}}})
				if err != nil {
					return err
				}
				raw, marshalErr := json.Marshal(out.Body["results"])
				require.Nil(t, marshalErr)

				var results []map[string]any
				require.Nil(t, json.Unmarshal(raw,
					&results,
				))
				require.False(t, len(results) !=
					1 || results[0]["status"] != "failed")

				return huma.Error404NotFound("run not found")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := newEnvScopedRunServer(t)
			err := tc.call(srv, envScopedRunCtx())
			require.Error(t, err)
			require.True(
				t, isNotFound(err))

		})
	}
}

func TestHandleListRuns_EnvironmentScopeFiltersForeignRuns(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListRunsByProjectFunc: func(_ context.Context, _ string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, _ *domain.ExecutionMode, _ *string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-prod", JobID: "job-prod", ProjectID: "proj-1", CreatedAt: time.Now().Add(-time.Minute)},
				{ID: "run-staging", JobID: "job-staging", ProjectID: "proj-1", CreatedAt: time.Now().Add(-2 * time.Minute)},
			}, nil
		},
		GetJobFunc: func(_ context.Context, jobID string) (*domain.Job, error) {
			switch jobID {
			case "job-prod":
				return &domain.Job{ID: jobID, ProjectID: "proj-1", EnvironmentID: "env-prod"}, nil
			case "job-staging":
				return &domain.Job{ID: jobID, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
			default:
				return nil, nil
			}
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	out, err := srv.handleListRuns(envScopedRunCtx(), &ListRunsInput{Limit: "10"})
	require.NoError(t, err)

	runs, ok := out.Body.Data.([]domain.JobRun)
	require.True(
		t, ok)
	require.False(t, len(runs) !=
		1 || runs[0].ID !=
		"run-prod",
	)

}

func TestHandleBulkReplayDeadLetterRuns_ProjectModeFiltersEnvironment(t *testing.T) {
	t.Parallel()

	var replayedRunIDs []string
	ms := &APIStoreMock{
		ListDeadLetterRunsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-prod", JobID: "job-prod", ProjectID: "proj-1", CreatedAt: time.Now().Add(-time.Minute)},
				{ID: "run-staging", JobID: "job-staging", ProjectID: "proj-1", CreatedAt: time.Now().Add(-2 * time.Minute)},
			}, nil
		},
		GetJobFunc: func(_ context.Context, jobID string) (*domain.Job, error) {
			switch jobID {
			case "job-prod":
				return &domain.Job{ID: jobID, ProjectID: "proj-1", EnvironmentID: "env-prod"}, nil
			case "job-staging":
				return &domain.Job{ID: jobID, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
			default:
				return nil, nil
			}
		},
		BulkReplayDeadLetterRunsFunc: func(_ context.Context, runIDs []string, projectID string, limit int) ([]domain.JobRun, error) {
			replayedRunIDs = append([]string(nil), runIDs...)
			return []domain.JobRun{{ID: "replayed-1", ProjectID: "proj-1"}}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	out, err := srv.handleBulkReplayDeadLetterRuns(envScopedRunCtx(), &BulkReplayDeadLetterRunsInput{
		Body: BulkReplayDeadLetterRunsRequest{ProjectID: "proj-1", Limit: 10},
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.False(t, len(replayedRunIDs) != 1 ||
		replayedRunIDs[0] != "run-prod")

}

func TestHandleListDeadLetterRuns_EnvironmentScopeFiltersForeignRuns(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListDeadLetterRunsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return []domain.JobRun{
				{ID: "run-prod", JobID: "job-prod", ProjectID: "proj-1", CreatedAt: time.Now().Add(-time.Minute)},
				{ID: "run-staging", JobID: "job-staging", ProjectID: "proj-1", CreatedAt: time.Now().Add(-2 * time.Minute)},
			}, nil
		},
		GetJobFunc: func(_ context.Context, jobID string) (*domain.Job, error) {
			switch jobID {
			case "job-prod":
				return &domain.Job{ID: jobID, ProjectID: "proj-1", EnvironmentID: "env-prod"}, nil
			case "job-staging":
				return &domain.Job{ID: jobID, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
			default:
				return nil, nil
			}
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	out, err := srv.handleListDeadLetterRuns(envScopedRunCtx(), &ListDeadLetterRunsInput{Limit: "10"})
	require.NoError(t, err)

	runs, ok := out.Body.Data.([]domain.JobRun)
	require.True(
		t, ok)
	require.False(t, len(runs) !=
		1 || runs[0].ID !=
		"run-prod",
	)

}
