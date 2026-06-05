package worker

import (
	"encoding/json"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestCloneJob_Nil(t *testing.T) {
	t.Parallel()
	require.Nil(t, cloneJob(nil))

}

func TestCloneJob_CopiesScalars(t *testing.T) {
	t.Parallel()

	job := &domain.Job{
		ID:                    "job-1",
		ProjectID:             "project-1",
		Version:               7,
		VersionID:             "version-7",
		EndpointURL:           "https://example.com",
		ExecutionMode:         domain.ExecutionModeWorker,
		MaxAttempts:           5,
		TimeoutSecs:           300,
		CacheVersion:          11,
		EndpointSigningSecret: "encrypted",
	}

	clone := cloneJob(job)
	require.NotSame(t, job,
		clone)
	require.False(t, clone.
		ID != job.
		ID ||
		clone.ProjectID != job.
			ProjectID ||
		clone.Version != job.Version ||
		clone.VersionID !=
			job.VersionID ||
		clone.EndpointURL != job.EndpointURL || clone.
		ExecutionMode !=
		job.ExecutionMode || clone.MaxAttempts !=
		job.MaxAttempts ||
		clone.TimeoutSecs !=
			job.TimeoutSecs || clone.
		CacheVersion != job.CacheVersion ||
		clone.EndpointSigningSecret !=
			job.EndpointSigningSecret)

}

func TestCloneJob_IsolatesMutableFields(t *testing.T) {
	t.Parallel()

	job := &domain.Job{
		Tags:                     map[string]string{"team": "ops"},
		DefaultRunMetadata:       map[string]string{"trace": "on"},
		RetryDelaysSecs:          []int{1, 2, 3},
		RateLimitKeys:            []domain.RateLimitKey{{Name: "customer", Max: 10, WindowSecs: 60}},
		PreferredRegions:         []string{"iad", "fra"},
		ResultSchema:             json.RawMessage(`{"type":"object"}`),
		OnCompletePayloadMapping: json.RawMessage(`{"result":"$.output"}`),
		OnFailurePayloadMapping:  json.RawMessage(`{"error":"$.error"}`),
	}

	clone := cloneJob(job)

	clone.Tags["team"] = "platform"
	clone.DefaultRunMetadata["trace"] = "off"
	clone.RetryDelaysSecs[0] = 99
	clone.RateLimitKeys[0].Name = "tenant"
	clone.PreferredRegions[0] = "sfo"
	clone.ResultSchema[0] = '['
	clone.OnCompletePayloadMapping[0] = '['
	clone.OnFailurePayloadMapping[0] = '['
	require.Equal(t, "ops",
		job.Tags["team"])
	require.Equal(t, "on",
		job.DefaultRunMetadata["trace"])
	require.EqualValues(t, 1, job.
		RetryDelaysSecs[0])
	require.Equal(t, "customer",
		job.
			RateLimitKeys[0].Name)
	require.Equal(t, "iad",
		job.PreferredRegions[0])
	require.Equal(t, `{"type":"object"}`,

		string(job.ResultSchema),
	)
	require.Equal(t, `{"result":"$.output"}`,

		string(job.OnCompletePayloadMapping))
	require.Equal(t, `{"error":"$.error"}`,

		string(job.OnFailurePayloadMapping))

}
