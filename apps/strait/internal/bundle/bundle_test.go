package bundle

import (
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportBundle_BasicRoundTrip(t *testing.T) {
	t.Parallel()

	jobs := []domain.Job{
		{
			ID: "j1", Slug: "resize-image", Name: "Resize Image",
			EndpointURL: "https://api.example.com/resize",
			MaxAttempts: 3, TimeoutSecs: 300, Enabled: true,
			Tags: map[string]string{"team": "media"},
		},
	}
	workflows := []domain.Workflow{
		{
			ID: "wf1", Slug: "pipeline", Name: "Pipeline",
			MaxConcurrentRuns: 5,
		},
	}
	steps := map[string][]domain.WorkflowStep{
		"wf1": {
			{StepRef: "validate", JobID: "j1", DependsOn: []string{}},
		},
	}
	envs := []domain.Environment{
		{Name: "Staging", Slug: "staging", Variables: map[string]string{"KEY": "val"}},
	}
	jobIDToSlug := map[string]string{"j1": "resize-image"}
	envIDToSlug := map[string]string{}

	b := ExportBundle("proj-1", jobs, workflows, steps, envs, jobIDToSlug, envIDToSlug)

	assert.Equal(t, Version, b.Version)
	assert.Equal(t, "proj-1", b.SourceProjectID)
	require.Len(t, b.Resources.Jobs, 1)
	assert.Equal(t, "resize-image", b.Resources.Jobs[0].Slug)
	require.Len(t, b.Resources.Workflows, 1)
	require.Len(t, b.Resources.Workflows[0].Steps, 1)
	assert.Equal(t, "resize-image", b.Resources.Workflows[0].Steps[0].JobSlug)
}

func TestExportBundle_SecretsRedacted(t *testing.T) {
	t.Parallel()
	envs := []domain.Environment{
		{
			Name: "Production", Slug: "production",
			Variables: map[string]string{"DB_URL": "postgres://secret", "API_KEY": "sk-123"},
		},
	}

	b := ExportBundle("proj-1", nil, nil, nil, envs, nil, nil)

	require.Len(t, b.Resources.Environments, 1)
	env := b.Resources.Environments[0]
	for key, val := range env.Variables {
		assert.Equal(t, RedactedPlaceholder, val, "variable %q should be redacted", key)
	}
}

func TestExportBundle_EmptyProject(t *testing.T) {
	t.Parallel()
	b := ExportBundle("proj-empty", nil, nil, nil, nil, nil, nil)

	assert.Equal(t, Version, b.Version)
	assert.Empty(t, b.Resources.Jobs)
	assert.Empty(t, b.Resources.Workflows)
}

func TestExportBundle_EnvironmentSlugResolution(t *testing.T) {
	t.Parallel()
	jobs := []domain.Job{
		{Slug: "job-1", Name: "Job 1", EnvironmentID: "env-uuid-1", Enabled: true},
	}
	envIDToSlug := map[string]string{"env-uuid-1": "production"}

	b := ExportBundle("proj-1", jobs, nil, nil, nil, nil, envIDToSlug)

	require.Len(t, b.Resources.Jobs, 1)
	assert.Equal(t, "production", b.Resources.Jobs[0].EnvironmentSlug)
}

func TestMarshalUnmarshalYAML_RoundTrip(t *testing.T) {
	t.Parallel()
	b := &Bundle{
		Version:         Version,
		ExportedAt:      time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC),
		SourceProjectID: "proj-1",
		Resources: Resources{
			Jobs: []JobSpec{
				{Slug: "job-1", Name: "Job One", EndpointURL: "https://example.com", MaxAttempts: 3, TimeoutSecs: 60, Enabled: true},
			},
			Workflows: []WorkflowSpec{
				{Slug: "wf-1", Name: "Workflow One", Steps: []WorkflowStepSpec{
					{StepRef: "step-1", JobSlug: "job-1"},
				}},
			},
			Environments: []EnvironmentSpec{
				{Name: "Dev", Slug: "development", IsStandard: true},
			},
		},
	}

	data, err := MarshalYAML(b)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	parsed, err := UnmarshalYAML(data)
	require.NoError(t, err)

	assert.Equal(t, b.Version, parsed.Version)
	assert.Equal(t, b.SourceProjectID, parsed.SourceProjectID)
	require.Len(t, parsed.Resources.Jobs, 1)
	assert.Equal(t, "job-1", parsed.Resources.Jobs[0].Slug)
	require.Len(t, parsed.Resources.Workflows, 1)
	require.Len(t, parsed.Resources.Workflows[0].Steps, 1)
}

func TestUnmarshalYAML_InvalidVersion(t *testing.T) {
	t.Parallel()
	data := []byte(`version: "99"
source_project_id: proj-1
resources: {}
`)
	_, err := UnmarshalYAML(data)
	require.Error(t, err)
}

func TestUnmarshalYAML_MissingVersion(t *testing.T) {
	t.Parallel()
	data := []byte(`source_project_id: proj-1
resources: {}
`)
	_, err := UnmarshalYAML(data)
	require.Error(t, err)
}

func TestUnmarshalYAML_InvalidYAML(t *testing.T) {
	t.Parallel()
	_, err := UnmarshalYAML([]byte(`{not valid yaml`))
	require.Error(t, err)
}

func TestComputeDiff_AllNew(t *testing.T) {
	t.Parallel()
	b := &Bundle{
		Resources: Resources{
			Jobs:      []JobSpec{{Slug: "new-job"}},
			Workflows: []WorkflowSpec{{Slug: "new-wf"}},
			Environments: []EnvironmentSpec{
				{Slug: "custom-env"},
			},
		},
	}

	diff := ComputeDiff(b, map[string]bool{}, map[string]bool{}, map[string]bool{})

	creates := 0
	for _, d := range diff {
		if d.Action == DiffCreate {
			creates++
		}
	}
	assert.Equal(t, 3, creates)
}

func TestComputeDiff_AllExisting(t *testing.T) {
	t.Parallel()
	b := &Bundle{
		Resources: Resources{
			Jobs:         []JobSpec{{Slug: "existing-job"}},
			Workflows:    []WorkflowSpec{{Slug: "existing-wf"}},
			Environments: []EnvironmentSpec{{Slug: "existing-env"}},
		},
	}

	diff := ComputeDiff(b,
		map[string]bool{"existing-job": true},
		map[string]bool{"existing-wf": true},
		map[string]bool{"existing-env": true},
	)

	updates := 0
	for _, d := range diff {
		if d.Action == DiffUpdate {
			updates++
		}
	}
	assert.Equal(t, 3, updates)
}

func TestComputeDiff_StandardEnvsSkipped(t *testing.T) {
	t.Parallel()
	b := &Bundle{
		Resources: Resources{
			Environments: []EnvironmentSpec{
				{Slug: "development", IsStandard: true},
				{Slug: "custom-env"},
			},
		},
	}

	diff := ComputeDiff(b, map[string]bool{}, map[string]bool{}, map[string]bool{})

	var skips, creates int
	for _, d := range diff {
		switch d.Action {
		case DiffSkip:
			skips++
		case DiffCreate:
			creates++
		case DiffUpdate:
			// not expected in this test
		}
	}
	assert.Equal(t, 1, skips)
	assert.Equal(t, 1, creates)
}

func TestComputeDiff_DependencyOrder(t *testing.T) {
	t.Parallel()
	b := &Bundle{
		Resources: Resources{
			Environments: []EnvironmentSpec{{Slug: "dev"}},
			Jobs:         []JobSpec{{Slug: "job-1"}},
			Workflows:    []WorkflowSpec{{Slug: "wf-1"}},
			WebhookSubscriptions: []WebhookSubscriptionSpec{
				{URL: "https://hooks.example.com", Events: []string{"run.completed"}},
			},
		},
	}

	diff := ComputeDiff(b, map[string]bool{}, map[string]bool{}, map[string]bool{})

	// Verify order: environments, jobs, workflows, webhooks.
	require.Len(t, diff, 4)
	assert.Equal(t, "environment", diff[0].ResourceType)
	assert.Equal(t, "job", diff[1].ResourceType)
	assert.Equal(t, "workflow", diff[2].ResourceType)
	assert.Equal(t, "webhook_subscription", diff[3].ResourceType)
}

func TestDependencyOrder(t *testing.T) {
	t.Parallel()
	order := DependencyOrder()
	require.Len(t, order, 4)
	assert.Equal(t, "environment", order[0])
	assert.Equal(t, "webhook_subscription", order[3])
}

func TestExportBundle_MultipleJobs(t *testing.T) {
	t.Parallel()
	jobs := make([]domain.Job, 10)
	for i := range jobs {
		jobs[i] = domain.Job{
			Slug:    "job-" + json.Number(rune('0'+i)).String(),
			Name:    "Job",
			Enabled: true,
		}
	}

	b := ExportBundle("proj-1", jobs, nil, nil, nil, nil, nil)
	assert.Len(t, b.Resources.Jobs, 10)
}

func TestImportResult_Fields(t *testing.T) {
	t.Parallel()
	result := ImportResult{
		Created: 3,
		Updated: 2,
		Skipped: 1,
		Failed:  0,
	}
	assert.Equal(t, 6, result.Created+result.Updated+result.Skipped+result.Failed)
}

func TestDiffEntry_Actions(t *testing.T) {
	t.Parallel()
	assert.Equal(t, DiffCreate, DiffAction("CREATE"))
	assert.Equal(t, DiffUpdate, DiffAction("UPDATE"))
	assert.Equal(t, DiffSkip, DiffAction("SKIP"))
}

func TestRedactedPlaceholder(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "<REDACTED>", RedactedPlaceholder)
}

func TestExportBundle_WebhookURLPreserved(t *testing.T) {
	t.Parallel()
	jobs := []domain.Job{
		{Slug: "hook-job", Name: "Hook Job", WebhookURL: "https://hooks.example.com/notify"},
	}
	b := ExportBundle("proj-1", jobs, nil, nil, nil, nil, nil)
	require.Len(t, b.Resources.Jobs, 1)
	assert.Equal(t, "https://hooks.example.com/notify", b.Resources.Jobs[0].WebhookURL)
}

func TestExportBundle_EmptyWebhookURLStaysEmpty(t *testing.T) {
	t.Parallel()
	jobs := []domain.Job{
		{Slug: "no-hook", Name: "No Hook", WebhookURL: ""},
	}
	b := ExportBundle("proj-1", jobs, nil, nil, nil, nil, nil)
	require.Len(t, b.Resources.Jobs, 1)
	assert.Empty(t, b.Resources.Jobs[0].WebhookURL)
}

func TestExportBundle_NilVariablesStayNil(t *testing.T) {
	t.Parallel()
	envs := []domain.Environment{
		{Name: "Empty", Slug: "empty", Variables: nil},
	}
	b := ExportBundle("proj-1", nil, nil, nil, envs, nil, nil)
	require.Len(t, b.Resources.Environments, 1)
	assert.Nil(t, b.Resources.Environments[0].Variables)
}

func TestExportBundle_VariablesRedactedNonNil(t *testing.T) {
	t.Parallel()
	envs := []domain.Environment{
		{Name: "Prod", Slug: "prod", Variables: map[string]string{"SECRET": "hunter2", "TOKEN": "abc123"}},
	}
	b := ExportBundle("proj-1", nil, nil, nil, envs, nil, nil)
	require.Len(t, b.Resources.Environments, 1)
	vars := b.Resources.Environments[0].Variables
	require.NotNil(t, vars)
	require.Len(t, vars, 2)
	for k, v := range vars {
		assert.Equal(t, RedactedPlaceholder, v, "variable %q", k)
	}
}
