package api

import (
	"context"
	"encoding/json"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestPrepareTriggerRequestBuildsState(t *testing.T) {
	t.Parallel()

	quota := &store.ProjectQuota{ProjectID: "project-1", Timezone: "UTC"}
	srv := newTestServer(t, &APIStoreMock{
		GetRunByIdempotencyKeyFunc: func(_ context.Context, jobID, key string) (*domain.JobRun, error) {
			require.Equal(t, "job-1", jobID)
			require.Equal(t, "idem-1", key)

			return nil, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			require.Equal(t, "project-1",
				projectID)

			return quota, nil
		},
	}, &mockQueue{}, nil)
	job := &domain.Job{
		ID:        "job-1",
		ProjectID: "project-1",
		PayloadSchema: json.RawMessage(
			`{"type":"object","required":["a"],"properties":{"a":{"type":"number"},"b":{"type":"number"}}}`,
		),
	}
	req := TriggerRequest{
		Payload:  json.RawMessage(`{"b":2,"a":1}`),
		Priority: 4,
		Tags:     map[string]string{"source": "test"},
	}

	state, idempotencyHit, err := srv.prepareTriggerRequest(context.Background(), &TriggerJobInput{
		XIdempotencyKey: "idem-1",
	}, job, req)
	require.NoError(t, err)
	require.Nil(t, idempotencyHit)
	require.Equal(t, job, state.job)
	require.Equal(t, "idem-1", state.
		idempotencyKey,
	)
	require.Equal(t, `{"a":1,"b":2}`,
		string(
			state.
				payload))
	require.NotEmpty(t, state.
		payloadHash,
	)
	require.NotNil(t, state.projectQuota)
	require.False(t, state.projectQuota.
		ProjectID !=
		quota.ProjectID ||
		state.projectQuota.Timezone !=
			quota.Timezone)
}

func TestPrepareTriggerRequestReturnsIdempotencyHitBeforeQuota(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{
		GetRunByIdempotencyKeyFunc: func(_ context.Context, jobID, key string) (*domain.JobRun, error) {
			require.Equal(t, "job-1", jobID)
			require.Equal(t, "idem-hit", key)

			return &domain.JobRun{ID: "run-existing", Status: domain.StatusQueued}, nil
		},
		GetProjectQuotaFunc: func(context.Context, string) (*store.ProjectQuota, error) {
			require.Fail(t,

				"GetProjectQuota must not run when idempotency hits")
			return nil, nil
		},
	}, &mockQueue{}, nil)

	state, idempotencyHit, err := srv.prepareTriggerRequest(context.Background(), &TriggerJobInput{
		XIdempotencyKey: "idem-hit",
	}, &domain.Job{ID: "job-1", ProjectID: "project-1"}, TriggerRequest{Payload: json.RawMessage(`{"ok":true}`)})
	require.NoError(t, err)
	require.Nil(t, state)

	assertIdempotencyResponse(t, idempotencyHit, "run-existing", domain.StatusQueued)
}
