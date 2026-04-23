package bundle

import (
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
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

	if b.Version != Version {
		t.Errorf("version = %q, want %q", b.Version, Version)
	}
	if b.SourceProjectID != "proj-1" {
		t.Errorf("source_project_id = %q, want %q", b.SourceProjectID, "proj-1")
	}
	if len(b.Resources.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(b.Resources.Jobs))
	}
	if b.Resources.Jobs[0].Slug != "resize-image" {
		t.Errorf("job slug = %q", b.Resources.Jobs[0].Slug)
	}
	if len(b.Resources.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(b.Resources.Workflows))
	}
	if len(b.Resources.Workflows[0].Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(b.Resources.Workflows[0].Steps))
	}
	if b.Resources.Workflows[0].Steps[0].JobSlug != "resize-image" {
		t.Errorf("step job_slug = %q, want %q", b.Resources.Workflows[0].Steps[0].JobSlug, "resize-image")
	}
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

	if len(b.Resources.Environments) != 1 {
		t.Fatalf("expected 1 env, got %d", len(b.Resources.Environments))
	}
	env := b.Resources.Environments[0]
	for key, val := range env.Variables {
		if val != RedactedPlaceholder {
			t.Errorf("variable %q should be redacted, got %q", key, val)
		}
	}
}

func TestExportBundle_EmptyProject(t *testing.T) {
	t.Parallel()
	b := ExportBundle("proj-empty", nil, nil, nil, nil, nil, nil)

	if b.Version != Version {
		t.Errorf("version should be set for empty bundle")
	}
	if len(b.Resources.Jobs) != 0 {
		t.Error("empty project should have no jobs")
	}
	if len(b.Resources.Workflows) != 0 {
		t.Error("empty project should have no workflows")
	}
}

func TestExportBundle_EnvironmentSlugResolution(t *testing.T) {
	t.Parallel()
	jobs := []domain.Job{
		{Slug: "job-1", Name: "Job 1", EnvironmentID: "env-uuid-1", Enabled: true},
	}
	envIDToSlug := map[string]string{"env-uuid-1": "production"}

	b := ExportBundle("proj-1", jobs, nil, nil, nil, nil, envIDToSlug)

	if b.Resources.Jobs[0].EnvironmentSlug != "production" {
		t.Errorf("environment_slug = %q, want %q", b.Resources.Jobs[0].EnvironmentSlug, "production")
	}
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
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("marshaled data is empty")
	}

	parsed, err := UnmarshalYAML(data)
	if err != nil {
		t.Fatalf("UnmarshalYAML: %v", err)
	}

	if parsed.Version != b.Version {
		t.Errorf("version = %q, want %q", parsed.Version, b.Version)
	}
	if parsed.SourceProjectID != b.SourceProjectID {
		t.Errorf("source_project_id = %q, want %q", parsed.SourceProjectID, b.SourceProjectID)
	}
	if len(parsed.Resources.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(parsed.Resources.Jobs))
	}
	if parsed.Resources.Jobs[0].Slug != "job-1" {
		t.Errorf("job slug = %q", parsed.Resources.Jobs[0].Slug)
	}
	if len(parsed.Resources.Workflows) != 1 || len(parsed.Resources.Workflows[0].Steps) != 1 {
		t.Error("workflow steps not preserved")
	}
}

func TestUnmarshalYAML_InvalidVersion(t *testing.T) {
	t.Parallel()
	data := []byte(`version: "99"
source_project_id: proj-1
resources: {}
`)
	_, err := UnmarshalYAML(data)
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
}

func TestUnmarshalYAML_MissingVersion(t *testing.T) {
	t.Parallel()
	data := []byte(`source_project_id: proj-1
resources: {}
`)
	_, err := UnmarshalYAML(data)
	if err == nil {
		t.Fatal("expected error for missing version")
	}
}

func TestUnmarshalYAML_InvalidYAML(t *testing.T) {
	t.Parallel()
	_, err := UnmarshalYAML([]byte(`{not valid yaml`))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
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
	if creates != 3 {
		t.Errorf("expected 3 creates, got %d", creates)
	}
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
	if updates != 3 {
		t.Errorf("expected 3 updates, got %d", updates)
	}
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
	if skips != 1 {
		t.Errorf("expected 1 skip (standard env), got %d", skips)
	}
	if creates != 1 {
		t.Errorf("expected 1 create (custom env), got %d", creates)
	}
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
	if len(diff) != 4 {
		t.Fatalf("expected 4 diff entries, got %d", len(diff))
	}
	if diff[0].ResourceType != "environment" {
		t.Errorf("first entry should be environment, got %q", diff[0].ResourceType)
	}
	if diff[1].ResourceType != "job" {
		t.Errorf("second entry should be job, got %q", diff[1].ResourceType)
	}
	if diff[2].ResourceType != "workflow" {
		t.Errorf("third entry should be workflow, got %q", diff[2].ResourceType)
	}
	if diff[3].ResourceType != "webhook_subscription" {
		t.Errorf("fourth entry should be webhook_subscription, got %q", diff[3].ResourceType)
	}
}

func TestDependencyOrder(t *testing.T) {
	t.Parallel()
	order := DependencyOrder()
	if len(order) != 4 {
		t.Fatalf("expected 4 resource types, got %d", len(order))
	}
	if order[0] != "environment" {
		t.Errorf("first = %q, want environment", order[0])
	}
	if order[3] != "webhook_subscription" {
		t.Errorf("last = %q, want webhook_subscription", order[3])
	}
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
	if len(b.Resources.Jobs) != 10 {
		t.Errorf("expected 10 jobs, got %d", len(b.Resources.Jobs))
	}
}

func TestImportResult_Fields(t *testing.T) {
	t.Parallel()
	result := ImportResult{
		Created: 3,
		Updated: 2,
		Skipped: 1,
		Failed:  0,
	}
	if result.Created+result.Updated+result.Skipped+result.Failed != 6 {
		t.Error("counts don't add up")
	}
}

func TestDiffEntry_Actions(t *testing.T) {
	t.Parallel()
	if DiffCreate != "CREATE" {
		t.Errorf("DiffCreate = %q", DiffCreate)
	}
	if DiffUpdate != "UPDATE" {
		t.Errorf("DiffUpdate = %q", DiffUpdate)
	}
	if DiffSkip != "SKIP" {
		t.Errorf("DiffSkip = %q", DiffSkip)
	}
}

func TestRedactedPlaceholder(t *testing.T) {
	t.Parallel()
	if RedactedPlaceholder != "<REDACTED>" {
		t.Errorf("RedactedPlaceholder = %q", RedactedPlaceholder)
	}
}

func TestExportBundle_WebhookURLPreserved(t *testing.T) {
	t.Parallel()
	jobs := []domain.Job{
		{Slug: "hook-job", Name: "Hook Job", WebhookURL: "https://hooks.example.com/notify"},
	}
	b := ExportBundle("proj-1", jobs, nil, nil, nil, nil, nil)
	if len(b.Resources.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(b.Resources.Jobs))
	}
	if b.Resources.Jobs[0].WebhookURL != "https://hooks.example.com/notify" {
		t.Errorf("WebhookURL = %q, want %q", b.Resources.Jobs[0].WebhookURL, "https://hooks.example.com/notify")
	}
}

func TestExportBundle_EmptyWebhookURLStaysEmpty(t *testing.T) {
	t.Parallel()
	jobs := []domain.Job{
		{Slug: "no-hook", Name: "No Hook", WebhookURL: ""},
	}
	b := ExportBundle("proj-1", jobs, nil, nil, nil, nil, nil)
	if b.Resources.Jobs[0].WebhookURL != "" {
		t.Errorf("WebhookURL = %q, want empty", b.Resources.Jobs[0].WebhookURL)
	}
}

func TestExportBundle_NilVariablesStayNil(t *testing.T) {
	t.Parallel()
	envs := []domain.Environment{
		{Name: "Empty", Slug: "empty", Variables: nil},
	}
	b := ExportBundle("proj-1", nil, nil, nil, envs, nil, nil)
	if b.Resources.Environments[0].Variables != nil {
		t.Errorf("expected nil Variables for env with no vars, got %v", b.Resources.Environments[0].Variables)
	}
}

func TestExportBundle_VariablesRedactedNonNil(t *testing.T) {
	t.Parallel()
	envs := []domain.Environment{
		{Name: "Prod", Slug: "prod", Variables: map[string]string{"SECRET": "hunter2", "TOKEN": "abc123"}},
	}
	b := ExportBundle("proj-1", nil, nil, nil, envs, nil, nil)
	vars := b.Resources.Environments[0].Variables
	if vars == nil {
		t.Fatal("expected non-nil Variables map for env with variables")
	}
	if len(vars) != 2 {
		t.Fatalf("expected 2 variable keys, got %d", len(vars))
	}
	for k, v := range vars {
		if v != RedactedPlaceholder {
			t.Errorf("variable %q = %q, want %q", k, v, RedactedPlaceholder)
		}
	}
}
