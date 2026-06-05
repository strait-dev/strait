package api

import (
	"encoding/json"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJob_WebhookSecret_NotSerialized(t *testing.T) {
	t.Parallel()

	job := domain.Job{
		ID:            "job-1",
		ProjectID:     "proj-1",
		Name:          "test-job",
		WebhookSecret: "super-secret-value",
	}

	data, err := json.Marshal(job)
	require.NoError(t,
		err)
	assert.False(t, strings.Contains(string(data), "super-secret-value"))
	assert.False(t, strings.Contains(string(data), "webhook_secret"))

}

func TestWebhookSubscription_Secret_NotSerialized(t *testing.T) {
	t.Parallel()

	sub := domain.WebhookSubscription{
		ID:         "sub-1",
		ProjectID:  "proj-1",
		WebhookURL: "https://example.com/webhook",
		EventTypes: []string{"job.completed"},
		Secret:     "whsec_very-secret-token",
		Active:     true,
	}

	data, err := json.Marshal(sub)
	require.NoError(t,
		err)
	assert.False(t, strings.Contains(string(data), "whsec_very-secret-token"))
	assert.False(t, strings.Contains(string(data), `"secret"`))

}

func TestJob_WebhookSecret_Adversarial_NestedMarshal(t *testing.T) {
	t.Parallel()

	type envelope struct {
		Data domain.Job `json:"data"`
	}

	e := envelope{
		Data: domain.Job{
			ID:            "job-1",
			ProjectID:     "proj-1",
			Name:          "test-job",
			WebhookSecret: "nested-secret-value",
		},
	}

	data, err := json.Marshal(e)
	require.NoError(t,
		err)
	assert.False(t, strings.Contains(string(data), "nested-secret-value"))

}

func TestJobVersion_WebhookSecret_NotSerialized(t *testing.T) {
	t.Parallel()

	jv := domain.JobVersion{
		ID:            "jv-1",
		JobID:         "job-1",
		WebhookSecret: "version-secret-value",
	}

	data, err := json.Marshal(jv)
	require.NoError(t,
		err)
	assert.False(t, strings.Contains(string(data), "version-secret-value"))
	assert.False(t, strings.Contains(string(data), "webhook_secret"))

}

func TestWebhookSubscription_Secret_Adversarial_SliceMarshal(t *testing.T) {
	t.Parallel()

	subs := []domain.WebhookSubscription{
		{ID: "sub-1", Secret: "secret-1", Active: true},
		{ID: "sub-2", Secret: "secret-2", Active: false},
	}

	data, err := json.Marshal(subs)
	require.NoError(t,
		err)
	assert.False(t, strings.Contains(string(data), "secret-1") || strings.Contains(string(data), "secret-2"))

}
