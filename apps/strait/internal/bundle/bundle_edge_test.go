package bundle

import (
	"strings"
	"testing"

	"strait/internal/domain"
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
	j := b.Resources.Jobs[0]

	if j.Slug != "full-job" {
		t.Errorf("slug = %q", j.Slug)
	}
	if j.FallbackEndpointURL != "https://fallback.example.com/run" {
		t.Errorf("fallback_url = %q", j.FallbackEndpointURL)
	}
	if j.MaxConcurrency != 20 {
		t.Errorf("max_concurrency = %d", j.MaxConcurrency)
	}
	if j.EnvironmentSlug != "production" {
		t.Errorf("environment_slug = %q", j.EnvironmentSlug)
	}
	if j.OnCompleteTriggerWorkflow != "post-process" {
		t.Errorf("on_complete_trigger_workflow = %q", j.OnCompleteTriggerWorkflow)
	}
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

	if len(b.Resources.Workflows[0].Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(b.Resources.Workflows[0].Steps))
	}
	if b.Resources.Workflows[0].Steps[2].JobSlug != "deploy-job" {
		t.Errorf("step 3 job_slug = %q", b.Resources.Workflows[0].Steps[2].JobSlug)
	}
	if len(b.Resources.Workflows[0].Steps[2].DependsOn) != 1 {
		t.Errorf("step 3 depends_on should have 1 entry")
	}
}

func TestExportBundle_EnvironmentWithNoVariables(t *testing.T) {
	t.Parallel()
	envs := []domain.Environment{
		{Name: "Empty", Slug: "empty", Variables: nil},
	}
	b := ExportBundle("p1", nil, nil, nil, envs, nil, nil)

	if len(b.Resources.Environments[0].Variables) != 0 {
		t.Error("env with no vars should have empty variables")
	}
}

func TestExportBundle_StandardEnvsPreserved(t *testing.T) {
	t.Parallel()
	envs := []domain.Environment{
		{Name: "Development", Slug: "development", IsStandard: true},
		{Name: "Custom", Slug: "custom", IsStandard: false},
	}
	b := ExportBundle("p1", nil, nil, nil, envs, nil, nil)

	if !b.Resources.Environments[0].IsStandard {
		t.Error("standard env should preserve is_standard flag")
	}
	if b.Resources.Environments[1].IsStandard {
		t.Error("custom env should not be standard")
	}
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

	if len(diff) != 2 {
		t.Fatalf("expected 2 diff entries, got %d", len(diff))
	}
	if diff[0].Action != DiffUpdate || diff[0].Slug != "existing" {
		t.Errorf("first entry: %+v", diff[0])
	}
	if diff[1].Action != DiffCreate || diff[1].Slug != "new-one" {
		t.Errorf("second entry: %+v", diff[1])
	}
}

func TestComputeDiff_EmptyBundle(t *testing.T) {
	t.Parallel()
	b := &Bundle{}
	diff := ComputeDiff(b, map[string]bool{}, map[string]bool{}, map[string]bool{})
	if len(diff) != 0 {
		t.Errorf("empty bundle should produce no diff, got %d", len(diff))
	}
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
		if d.Action != DiffCreate {
			t.Errorf("webhook should always be CREATE, got %s", d.Action)
		}
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
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	if len(data) < 1000 {
		t.Errorf("expected large YAML output, got %d bytes", len(data))
	}
}

func TestUnmarshalYAML_EmptyResources(t *testing.T) {
	t.Parallel()
	data := []byte(`version: "1"
exported_at: "2026-03-18T00:00:00Z"
source_project_id: proj-1
resources: {}
`)
	b, err := UnmarshalYAML(data)
	if err != nil {
		t.Fatalf("UnmarshalYAML: %v", err)
	}
	if len(b.Resources.Jobs) != 0 {
		t.Error("expected no jobs")
	}
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
	if err != nil {
		t.Fatalf("UnmarshalYAML: %v", err)
	}
	j := b.Resources.Jobs[0]
	if j.MaxConcurrency != 8 {
		t.Errorf("max_concurrency = %d", j.MaxConcurrency)
	}
	if j.Cron != "0 * * * *" {
		t.Errorf("cron = %q", j.Cron)
	}
	if j.EnvironmentSlug != "production" {
		t.Errorf("environment_slug = %q", j.EnvironmentSlug)
	}
	if j.OnCompleteTriggerWorkflow != "post-process" {
		t.Errorf("on_complete_trigger_workflow = %q", j.OnCompleteTriggerWorkflow)
	}
	if j.Tags["env"] != "prod" {
		t.Errorf("tags = %v", j.Tags)
	}
}

func TestVersion_Constant(t *testing.T) {
	t.Parallel()
	if Version != "1" {
		t.Errorf("Version = %q, want %q", Version, "1")
	}
}
