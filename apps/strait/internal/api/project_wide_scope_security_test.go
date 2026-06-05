package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func envScopedProjectContext() context.Context {
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")
	return ctx
}

func TestJobGroups_EnvironmentScopedCallersRejectedBeforeStore(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateJobGroupFunc: func(context.Context, *domain.JobGroup) error {
			require.Fail(t,

				"CreateJobGroup must not be called for an environment-scoped caller")
			return nil
		},
		GetJobGroupFunc: func(context.Context, string) (*domain.JobGroup, error) {
			require.Fail(t,

				"GetJobGroup must not be called for an environment-scoped caller")
			return nil, store.ErrJobGroupNotFound
		},
		ListJobGroupsFunc: func(context.Context, string, int, *time.Time) ([]domain.JobGroup, error) {
			require.Fail(t,

				"ListJobGroups must not be called for an environment-scoped caller")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := envScopedProjectContext()

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "create",
			call: func() error {
				_, err := srv.handleCreateJobGroup(ctx, &CreateJobGroupInput{Body: CreateJobGroupRequest{ProjectID: "proj-1", Name: "Core", Slug: "core"}})
				return err
			},
		},
		{
			name: "get",
			call: func() error {
				_, err := srv.handleGetJobGroup(ctx, &GetJobGroupInput{GroupID: "group-1"})
				return err
			},
		},
		{
			name: "list groups",
			call: func() error {
				_, err := srv.handleListJobGroups(ctx, &ListJobGroupsInput{})
				return err
			},
		},
		{
			name: "update",
			call: func() error {
				_, err := srv.handleUpdateJobGroup(ctx, &UpdateJobGroupInput{GroupID: "group-1", Body: UpdateJobGroupRequest{}})
				return err
			},
		},
		{
			name: "delete",
			call: func() error {
				_, err := srv.handleDeleteJobGroup(ctx, &DeleteJobGroupInput{GroupID: "group-1"})
				return err
			},
		},
		{
			name: "list jobs",
			call: func() error {
				_, err := srv.handleListJobsByGroup(ctx, &ListJobsByGroupInput{GroupID: "group-1"})
				return err
			},
		},
		{
			name: "pause all",
			call: func() error {
				_, err := srv.handlePauseAllJobsByGroup(ctx, &PauseAllJobsByGroupInput{GroupID: "group-1"})
				return err
			},
		},
		{
			name: "resume all",
			call: func() error {
				_, err := srv.handleResumeAllJobsByGroup(ctx, &ResumeAllJobsByGroupInput{GroupID: "group-1"})
				return err
			},
		},
		{
			name: "stats",
			call: func() error {
				_, err := srv.handleGetJobGroupStats(ctx, &GetJobGroupStatsInput{GroupID: "group-1"})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.call(); !isHumaStatusError(err, http.StatusForbidden) {
				require.Failf(t, "test failure",

					"expected 403, got %v", err)
			}
		})
	}
}

func TestJobAnalytics_EnvironmentScopedCallersRejected(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	ctx := envScopedProjectContext()

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "history",
			call: func() error {
				_, err := srv.handleJobHistory(ctx, &JobHistoryInput{JobID: "job-1"})
				return err
			},
		},
		{
			name: "comparison",
			call: func() error {
				_, err := srv.handleJobComparison(ctx, &JobComparisonInput{JobIDs: "job-1,job-2"})
				return err
			},
		},
		{
			name: "reliability",
			call: func() error {
				_, err := srv.handleJobReliability(ctx, &JobReliabilityInput{})
				return err
			},
		},
		{
			name: "runs by version",
			call: func() error {
				_, err := srv.handleRunsByVersion(ctx, &RunsByVersionInput{JobID: "job-1"})
				return err
			},
		},
		{
			name: "cost ranking",
			call: func() error {
				_, err := srv.handleJobCostRanking(ctx, &JobCostRankingInput{})
				return err
			},
		},
		{
			name: "top failing",
			call: func() error {
				_, err := srv.handleTopFailingJobs(ctx, &TopFailingJobsInput{})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.call(); !isHumaStatusError(err, http.StatusForbidden) {
				require.Failf(t, "test failure",

					"expected 403, got %v", err)
			}
		})
	}
}
