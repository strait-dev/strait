package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"
)

func TestFindRecentDeduplicatedRunSkipsDisabledWindow(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		FindRecentRunByPayloadFunc: func(context.Context, string, json.RawMessage, time.Time) (*domain.JobRun, error) {
			require.Fail(t,

				"FindRecentRunByPayload must not run when deduplication is disabled")
			return nil, nil
		},
	}}

	run, err := srv.findRecentDeduplicatedRun(context.Background(), &domain.Job{ID: "job-1"}, json.RawMessage(`{"ok":true}`))
	require.NoError(t, err)
	require.Nil(t, run)
}

func TestFindRecentDeduplicatedRunUsesDedupWindow(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`{"ok":true}`)
	srv := &Server{store: &APIStoreMock{
		FindRecentRunByPayloadFunc: func(_ context.Context, jobID string, gotPayload json.RawMessage, since time.Time) (*domain.JobRun, error) {
			require.Equal(t, "job-1", jobID)
			require.Equal(t, string(payload), string(gotPayload))

			cutoff := time.Now().Add(-60 * time.Second)
			require.False(t, since.Before(
				cutoff.Add(-2*time.Second)) || since.
				After(cutoff.Add(2*time.Second)),
			)

			return &domain.JobRun{ID: "run-existing", Status: domain.StatusQueued}, nil
		},
	}}

	run, err := srv.findRecentDeduplicatedRun(context.Background(), &domain.Job{ID: "job-1", DedupWindowSecs: 60}, payload)
	require.NoError(t, err)
	require.False(t, run == nil ||
		run.ID !=
			"run-existing",
	)
}

func TestTriggerDedupOutputReturnsExistingRunShape(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		FindRecentRunByPayloadFunc: func(context.Context, string, json.RawMessage, time.Time) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-existing", Status: domain.StatusExecuting}, nil
		},
	}}
	state := &triggerRequestState{
		job:         &domain.Job{ID: "job-1", DedupWindowSecs: 60},
		payload:     json.RawMessage(`{"ok":true}`),
		payloadHash: "payload-hash",
	}

	out, err := srv.triggerDedupOutput(context.Background(), state)
	require.NoError(t, err)
	require.NotNil(t, out)

	body, ok := out.Body.(map[string]any)
	require.True(
		t, ok)
	require.Equal(t, "run-existing",
		body["id"])
	require.Equal(t, domain.StatusExecuting,

		body["status"])
	require.Equal(t, "payload-hash",
		body["payload_hash"])
	require.Equal(t, false, body["idempotency_hit"])
}

func TestTriggerDedupOutputMapsLookupError(t *testing.T) {
	t.Parallel()

	srv := &Server{store: &APIStoreMock{
		FindRecentRunByPayloadFunc: func(context.Context, string, json.RawMessage, time.Time) (*domain.JobRun, error) {
			return nil, errors.New("database unavailable")
		},
	}}
	state := &triggerRequestState{
		job:     &domain.Job{ID: "job-1", DedupWindowSecs: 60},
		payload: json.RawMessage(`{"ok":true}`),
	}

	_, err := srv.triggerDedupOutput(context.Background(), state)
	var statusErr huma.StatusError
	require.ErrorAs(
		t, err, &statusErr)
	require.Equal(t, http.StatusInternalServerError,

		statusErr.
			GetStatus())
}
