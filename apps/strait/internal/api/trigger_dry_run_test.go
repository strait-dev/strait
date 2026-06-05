package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"
)

func TestHandleTriggerDryRunReturnsValidationResult(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{
		GetJobFunc: func(_ context.Context, jobID string) (*domain.Job, error) {
			require.Equal(t, "job-1", jobID)

			return &domain.Job{
				ID:          jobID,
				ProjectID:   "project-1",
				Name:        "Export",
				Slug:        "export",
				Enabled:     true,
				TimeoutSecs: 60,
				MaxAttempts: 2,
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			require.Equal(t, "project-1",
				projectID)

			return &store.ProjectQuota{ProjectID: projectID}, nil
		},
	}, &mockQueue{}, nil)

	out, err := srv.handleTriggerDryRun(context.Background(), "job-1", TriggerRequest{
		Payload: json.RawMessage(`{"b":2,"a":1}`),
	})
	require.Nil(t, out)

	var rawErr *rawStatusError
	require.True(
		t, errors.As(err,
			&rawErr))
	require.Equal(t, http.StatusOK,
		rawErr.status,
	)

	result, ok := rawErr.body.(*DryRunValidationResult)
	require.True(
		t, ok)
	require.False(t, result.Job ==
		nil || result.
		Job.ID !=
		"job-1")
	require.Equal(t, `{"a":1,"b":2}`,
		string(result.Payload))
	require.NotEqual(t, "", result.
		PayloadHash,
	)

}

func TestHandleTriggerDryRunMapsValidationErrorToBadRequest(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{
		GetJobFunc: func(_ context.Context, jobID string) (*domain.Job, error) {
			return &domain.Job{ID: jobID, ProjectID: "project-1", Enabled: false, TimeoutSecs: 60}, nil
		},
	}, &mockQueue{}, nil)

	_, err := srv.handleTriggerDryRun(context.Background(), "job-1", TriggerRequest{})
	var statusErr huma.StatusError
	require.True(
		t, errors.As(err,
			&statusErr,
		))
	require.Equal(t, http.StatusBadRequest,

		statusErr.GetStatus())
	require.True(
		t, strings.Contains(err.Error(), "job is disabled"))

}

func TestDryRunValidationWarningsReportsDedupRun(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`{"customer":"acme"}`)
	job := &domain.Job{ID: "job-1", DedupWindowSecs: 60}
	ms := &APIStoreMock{
		FindRecentRunByPayloadFunc: func(_ context.Context, jobID string, gotPayload json.RawMessage, since time.Time) (*domain.JobRun, error) {
			require.Equal(t, job.ID, jobID)
			require.Equal(t, string(payload), string(gotPayload))
			require.False(t, since.IsZero())

			return &domain.JobRun{ID: "run-existing"}, nil
		},
	}
	srv := &Server{store: ms}

	warnings, err := srv.dryRunValidationWarnings(context.Background(), job, payload)
	require.NoError(t, err)
	require.False(t, len(warnings) !=
		1 || !strings.Contains(warnings[0], "run-existing"))

}

func TestDryRunValidationWarningsSkipsDedupWhenDisabled(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		FindRecentRunByPayloadFunc: func(context.Context, string, json.RawMessage, time.Time) (*domain.JobRun, error) {
			require.Fail(t,

				"dedup lookup must not run when dedup window is disabled")
			return nil, nil
		},
	}}

	warnings, err := srv.dryRunValidationWarnings(context.Background(), &domain.Job{ID: "job-1"}, nil)
	require.NoError(t, err)
	require.Len(t,
		warnings, 0)

}

func TestDryRunJobInfoNilSafe(t *testing.T) {
	t.Parallel()
	require.Nil(t, dryRunJobInfo(nil))

}
