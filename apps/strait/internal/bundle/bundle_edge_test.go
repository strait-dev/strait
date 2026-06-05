package bundle

import (
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Export edge cases.

func TestExportBundle_JobWithAllFields(t *testing.T) {
	t.Parallel()
	jobs := []domain.Job{
		{
			Slug: "full-job", Name: "Full Job", Description: "A complete job",
			EndpointURL:         "https://api.example.com/run",
			FallbackEndpointURL: "https://fallback.example.com/run",
			MaxAttempts:         5, TimeoutSecs: 600, MaxConcurrency: 20,
			Cron: "*/5 * * * *", Timezone: "America/New_York",
			Tags:          map[string]string{"env": "prod", "team": "backend"},
			RetryStrategy: "exponential", Enabled: true,
			WebhookURL:                "https://hooks.example.com/notify",
			OnCompleteTriggerWorkflow: "post-process",
			EnvironmentID:             "env-uuid",
		},
	}
	envIDToSlug := map[string]string{"env-uuid": "production"}

	b := ExportBundle("proj-1", jobs, nil, nil, nil, nil, envIDToSlug)
	require.Len(t, b.Resources.Jobs, 1)
	j := b.Resources.Jobs[0]

	assert.Equal(t, "full-job", j.Slug)
	assert.Equal(t, "https://fallback.example.com/run", j.FallbackEndpointURL)
	assert.Equal(t, 20, j.MaxConcurrency)
	assert.Equal(t, "production", j.EnvironmentSlug)
	assert.Equal(t, "post-process", j.OnCompleteTriggerWorkflow)
}

func TestExportBundle_WorkflowMultipleSteps(t *testing.T) {
	t.Parallel()
	workflows := []domain.Workflow{
		{ID: "wf-1", Slug: "pipeline", Name: "Pipeline", MaxConcurrentRuns: 3},
	}
	steps := map[string][]domain.WorkflowStep{
		"wf-1": {
			{StepRef: "build", JobID: "j1", DependsOn: []string{}},
			{StepRef: "test", JobID: "j2", DependsOn: []string{"build"}},
			{StepRef: "deploy", JobID: "j3", DependsOn: []string{"test"}},
		},
	}
	jobIDToSlug := map[string]string{"j1": "build-job", "j2": "test-job", "j3": "deploy-job"}

	b := ExportBundle("p1", nil, workflows, steps, nil, jobIDToSlug, nil)

	require.Len(t, b.Resources.Workflows, 1)
	require.Len(t, b.Resources.Workflows[0].Steps, 3)
	assert.Equal(t, "deploy-job", b.Resources.Workflows[0].Steps[2].JobSlug)
	assert.Len(t, b.Resources.Workflows[0].Steps[2].DependsOn, 1)
}

func TestExportBundle_EnvironmentWithNoVariables(t *testing.T) {
	t.Parallel()
	envs := []domain.Environment{
		{Name: "Empty", Slug: "empty", Variables: nil},
	}
	b := ExportBundle("p1", nil, nil, nil, envs, nil, nil)

	require.Len(t, b.Resources.Environments, 1)
	assert.Empty(t, b.Resources.Environments[0].Variables)
}

func TestExportBundle_StandardEnvsPreserved(t *testing.T) {
	t.Parallel()
	envs := []domain.Environment{
		{Name: "Development", Slug: "development", IsStandard: true},
		{Name: "Custom", Slug: "custom", IsStandard: false},
	}
	b := ExportBundle("p1", nil, nil, nil, envs, nil, nil)

	require.Len(t, b.Resources.Environments, 2)
	assert.True(t, b.Resources.Environments[0].IsStandard)
	assert.False(t, b.Resources.Environments[1].IsStandard)
}

// Diff edge cases.

func TestComputeDiff_MixedCreateAndUpdate(t *testing.T) {
	t.Parallel()
	b := &Bundle{
		Resources: Resources{
			Jobs: []JobSpec{
				{Slug: "existing"},
				{Slug: "new-one"},
			},
		},
	}
	diff := ComputeDiff(b, map[string]bool{"existing": true}, map[string]bool{}, map[string]bool{})

	require.Len(t, diff, 2)
	assert.Equal(t, DiffUpdate, diff[0].Action)
	assert.Equal(t, "existing", diff[0].Slug)
	assert.Equal(t, DiffCreate, diff[1].Action)
	assert.Equal(t, "new-one", diff[1].Slug)
}

func TestComputeDiff_EmptyBundle(t *testing.T) {
	t.Parallel()
	b := &Bundle{}
	diff := ComputeDiff(b, map[string]bool{}, map[string]bool{}, map[string]bool{})
	assert.Empty(t, diff)
}

func TestComputeDiff_WebhooksAlwaysCreate(t *testing.T) {
	t.Parallel()
	b := &Bundle{
		Resources: Resources{
			WebhookSubscriptions: []WebhookSubscriptionSpec{
				{URL: "https://hooks.a.com", Events: []string{"run.completed"}},
				{URL: "https://hooks.b.com", Events: []string{"run.failed"}},
			},
		},
	}
	diff := ComputeDiff(b, map[string]bool{}, map[string]bool{}, map[string]bool{})

	for _, d := range diff {
		assert.Equal(t, DiffCreate, d.Action)
	}
}

// YAML edge cases.

func TestMarshalYAML_LargeBundle(t *testing.T) {
	t.Parallel()
	b := &Bundle{
		Version:         Version,
		SourceProjectID: "p1",
	}
	for i := range 100 {
		b.Resources.Jobs = append(b.Resources.Jobs, JobSpec{
			Slug:    strings.Repeat("a", 10) + string(rune('0'+i%10)),
			Name:    "Job",
			Enabled: true,
		})
	}

	data, err := MarshalYAML(b)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(data), 1000)
}

func TestUnmarshalYAML_EmptyResources(t *testing.T) {
	t.Parallel()
	data := []byte(`version: "1"
exported_at: "2026-03-18T00:00:00Z"
source_project_id: proj-1
resources: {}
`)
	b, err := UnmarshalYAML(data)
	require.NoError(t, err)
	assert.Empty(t, b.Resources.Jobs)
}

func TestUnmarshalYAML_PreservesAllJobFields(t *testing.T) {
	t.Parallel()
	data := []byte(`version: "1"
source_project_id: proj-1
resources:
  jobs:
    - slug: test-job
      name: Test Job
      endpoint_url: https://example.com
      max_attempts: 5
      timeout_secs: 120
      max_concurrency: 8
      cron: "0 * * * *"
      timezone: UTC
      retry_strategy: exponential
      enabled: true
      environment_slug: production
      on_complete_trigger_workflow: post-process
      tags:
        env: prod
`)
	b, err := UnmarshalYAML(data)
	require.NoError(t, err)
	require.Len(t, b.Resources.Jobs, 1)
	j := b.Resources.Jobs[0]
	assert.Equal(t, 8, j.MaxConcurrency)
	assert.Equal(t, "0 * * * *", j.Cron)
	assert.Equal(t, "production", j.EnvironmentSlug)
	assert.Equal(t, "post-process", j.OnCompleteTriggerWorkflow)
	assert.Equal(t, "prod", j.Tags["env"])
}

func TestVersion_Constant(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "1", Version)
}
