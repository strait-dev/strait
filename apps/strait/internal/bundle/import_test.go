package bundle

import (
	"encoding/json"
	"testing"

	"strait/internal/domain"
)

func TestSingletonExpr_RoundTrip(t *testing.T) {
	t.Parallel()

	if got := singletonExpr(""); got != nil {
		t.Errorf("empty template = %s, want nil", got)
	}

	raw := singletonExpr("user-${user.id}")
	if raw == nil {
		t.Fatal("non-empty template yielded nil")
	}
	expr, err := domain.ParseSingletonKeyExpr(raw)
	if err != nil {
		t.Fatalf("ParseSingletonKeyExpr: %v", err)
	}
	if expr.Template != "user-${user.id}" {
		t.Errorf("template = %q, want %q", expr.Template, "user-${user.id}")
	}

	// flattenSingletonKey is the inverse: it must recover the template.
	if got := flattenSingletonKey(raw); got != "user-${user.id}" {
		t.Errorf("flatten(singletonExpr(x)) = %q, want %q", got, "user-${user.id}")
	}
}

func TestJobSpecToDomain(t *testing.T) {
	t.Parallel()

	depth := 5
	spec := JobSpec{
		Slug:                      "resize",
		Name:                      "Resize",
		Description:               "resize images",
		EndpointURL:               "https://api.example.com/resize",
		FallbackEndpointURL:       "https://fallback.example.com/resize",
		MaxAttempts:               3,
		TimeoutSecs:               300,
		MaxConcurrency:            10,
		Cron:                      "0 * * * *",
		Timezone:                  "UTC",
		PayloadSchema:             json.RawMessage(`{"type":"object"}`),
		Tags:                      map[string]string{"team": "media"},
		RetryStrategy:             "exponential",
		Enabled:                   true,
		WebhookURL:                "https://hooks.example.com/x",
		OnCompleteTriggerWorkflow: "pipeline",
		SingletonKey:              "user-${user.id}",
		SingletonOnConflict:       "queue",
		SingletonMaxQueueDepth:    &depth,
	}

	job := JobSpecToDomain(spec, "proj-1", "env-1")

	if job.ProjectID != "proj-1" {
		t.Errorf("project = %q, want proj-1", job.ProjectID)
	}
	if job.EnvironmentID != "env-1" {
		t.Errorf("environment = %q, want env-1", job.EnvironmentID)
	}
	if job.Slug != "resize" || job.Name != "Resize" {
		t.Errorf("slug/name = %q/%q", job.Slug, job.Name)
	}
	if job.FallbackEndpointURL != "https://fallback.example.com/resize" {
		t.Errorf("fallback = %q", job.FallbackEndpointURL)
	}
	if job.OnCompleteTriggerWorkflow != "pipeline" {
		t.Errorf("on_complete = %q", job.OnCompleteTriggerWorkflow)
	}
	if job.SingletonOnConflict != domain.SingletonOnConflictQueue {
		t.Errorf("on_conflict = %q, want queue", job.SingletonOnConflict)
	}
	if job.SingletonMaxQueueDepth == nil || *job.SingletonMaxQueueDepth != 5 {
		t.Errorf("max_queue_depth = %v, want 5", job.SingletonMaxQueueDepth)
	}
	expr, err := domain.ParseSingletonKeyExpr(job.SingletonKeyExpr)
	if err != nil {
		t.Fatalf("parse singleton key: %v", err)
	}
	if expr.Template != "user-${user.id}" {
		t.Errorf("singleton template = %q", expr.Template)
	}
}

func TestJobSpecToDomain_NonSingleton(t *testing.T) {
	t.Parallel()

	job := JobSpecToDomain(JobSpec{Slug: "plain", Name: "Plain"}, "proj-1", "")
	if job.SingletonKeyExpr != nil {
		t.Errorf("non-singleton job has key expr %s, want nil", job.SingletonKeyExpr)
	}
	if job.EnvironmentID != "" {
		t.Errorf("environment = %q, want empty", job.EnvironmentID)
	}
}

func TestWorkflowSpecToDomain(t *testing.T) {
	t.Parallel()

	depth := 2
	spec := WorkflowSpec{
		Slug:                   "pipeline",
		Name:                   "Pipeline",
		Description:            "the pipeline",
		MaxConcurrentRuns:      4,
		SingletonKey:           "tenant-${tenant.id}",
		SingletonOnConflict:    "replace",
		SingletonMaxQueueDepth: &depth,
	}

	wf := WorkflowSpecToDomain(spec, "proj-1")

	if wf.ProjectID != "proj-1" {
		t.Errorf("project = %q", wf.ProjectID)
	}
	if wf.MaxConcurrentRuns != 4 {
		t.Errorf("max_concurrent = %d, want 4", wf.MaxConcurrentRuns)
	}
	if wf.SingletonOnConflict != domain.SingletonOnConflictReplace {
		t.Errorf("on_conflict = %q, want replace", wf.SingletonOnConflict)
	}
	if wf.SingletonMaxQueueDepth == nil || *wf.SingletonMaxQueueDepth != 2 {
		t.Errorf("max_queue_depth = %v, want 2", wf.SingletonMaxQueueDepth)
	}
	expr, err := domain.ParseSingletonKeyExpr(wf.SingletonKeyExpr)
	if err != nil {
		t.Fatalf("parse singleton key: %v", err)
	}
	if expr.Template != "tenant-${tenant.id}" {
		t.Errorf("singleton template = %q", expr.Template)
	}
}

func TestStepSpecToDomain(t *testing.T) {
	t.Parallel()

	t.Run("full", func(t *testing.T) {
		t.Parallel()
		spec := WorkflowStepSpec{
			StepRef:   "validate",
			JobSlug:   "validate-job",
			DependsOn: []string{"start"},
			Condition: `{"==":[1,1]}`,
			OnFailure: "fail_workflow",
		}
		step := StepSpecToDomain(spec, "wf-1", "job-1")
		if step.WorkflowID != "wf-1" || step.JobID != "job-1" {
			t.Errorf("workflow/job = %q/%q", step.WorkflowID, step.JobID)
		}
		if step.StepRef != "validate" {
			t.Errorf("step_ref = %q", step.StepRef)
		}
		if len(step.DependsOn) != 1 || step.DependsOn[0] != "start" {
			t.Errorf("depends_on = %v", step.DependsOn)
		}
		if step.OnFailure != domain.FailWorkflow {
			t.Errorf("on_failure = %q, want fail_workflow", step.OnFailure)
		}
		if string(step.Condition) != `{"==":[1,1]}` {
			t.Errorf("condition = %s", step.Condition)
		}
	})

	t.Run("empty condition and deps", func(t *testing.T) {
		t.Parallel()
		step := StepSpecToDomain(WorkflowStepSpec{StepRef: "lone"}, "wf-1", "")
		if step.Condition != nil {
			t.Errorf("condition = %s, want nil", step.Condition)
		}
		if step.DependsOn == nil {
			t.Error("depends_on = nil, want non-nil empty slice")
		}
		if len(step.DependsOn) != 0 {
			t.Errorf("depends_on = %v, want empty", step.DependsOn)
		}
	})
}

func TestResolveEnvVariables(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		incoming map[string]string
		existing map[string]string
		isCreate bool
		want     map[string]string
		wantErr  bool
	}{
		{
			name: "empty yields nil",
			want: nil,
		},
		{
			name:     "real values pass through on create",
			incoming: map[string]string{"API_KEY": "secret", "REGION": "us"},
			isCreate: true,
			want:     map[string]string{"API_KEY": "secret", "REGION": "us"},
		},
		{
			name:     "redacted on create errors",
			incoming: map[string]string{"API_KEY": RedactedPlaceholder},
			isCreate: true,
			wantErr:  true,
		},
		{
			name:     "redacted on update preserves existing",
			incoming: map[string]string{"API_KEY": RedactedPlaceholder, "REGION": "eu"},
			existing: map[string]string{"API_KEY": "stored-secret"},
			want:     map[string]string{"API_KEY": "stored-secret", "REGION": "eu"},
		},
		{
			name:     "redacted on update without existing errors",
			incoming: map[string]string{"API_KEY": RedactedPlaceholder},
			existing: map[string]string{"OTHER": "x"},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveEnvVariables(tt.incoming, tt.existing, tt.isCreate)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("%s = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestExistingStateFromBundle(t *testing.T) {
	t.Parallel()

	b := &Bundle{
		Resources: Resources{
			Jobs:         []JobSpec{{Slug: "a"}, {Slug: "b"}},
			Workflows:    []WorkflowSpec{{Slug: "wf"}},
			Environments: []EnvironmentSpec{{Slug: "staging"}},
		},
	}
	st := ExistingStateFromBundle(b)
	if len(st.Jobs) != 2 {
		t.Errorf("jobs = %d, want 2", len(st.Jobs))
	}
	if _, ok := st.Jobs["a"]; !ok {
		t.Error("job a missing")
	}
	if _, ok := st.Workflows["wf"]; !ok {
		t.Error("workflow wf missing")
	}
	if _, ok := st.Environments["staging"]; !ok {
		t.Error("environment staging missing")
	}
}

func TestComputePlan(t *testing.T) {
	t.Parallel()

	existing := ExistingState{
		Jobs: map[string]JobSpec{
			"keep":   {Slug: "keep", Name: "Keep", MaxAttempts: 3},
			"change": {Slug: "change", Name: "Old Name", MaxAttempts: 3},
		},
		Workflows: map[string]WorkflowSpec{
			"wf-keep": {Slug: "wf-keep", Name: "WF"},
		},
		Environments: map[string]EnvironmentSpec{
			"prod": {Slug: "prod", Name: "Prod"},
		},
	}

	b := &Bundle{
		Resources: Resources{
			Environments: []EnvironmentSpec{
				{Slug: "prod", Name: "Prod"},                      // SKIP (identical)
				{Slug: "dev", Name: "Dev"},                        // CREATE
				{Slug: "standard", Name: "Std", IsStandard: true}, // SKIP (standard)
			},
			Jobs: []JobSpec{
				{Slug: "keep", Name: "Keep", MaxAttempts: 3},       // SKIP
				{Slug: "change", Name: "New Name", MaxAttempts: 3}, // UPDATE
				{Slug: "new", Name: "New"},                         // CREATE
			},
			Workflows: []WorkflowSpec{
				{Slug: "wf-keep", Name: "WF"},    // SKIP
				{Slug: "wf-new", Name: "WF New"}, // CREATE
			},
		},
	}

	plan := ComputePlan(b, existing)

	want := map[string]DiffAction{
		"environment/prod":     DiffSkip,
		"environment/dev":      DiffCreate,
		"environment/standard": DiffSkip,
		"job/keep":             DiffSkip,
		"job/change":           DiffUpdate,
		"job/new":              DiffCreate,
		"workflow/wf-keep":     DiffSkip,
		"workflow/wf-new":      DiffCreate,
	}

	if len(plan) != len(want) {
		t.Fatalf("plan has %d entries, want %d: %+v", len(plan), len(want), plan)
	}
	for _, e := range plan {
		key := e.ResourceType + "/" + e.Slug
		wantAction, ok := want[key]
		if !ok {
			t.Errorf("unexpected entry %s", key)
			continue
		}
		if e.Action != wantAction {
			t.Errorf("%s action = %q, want %q", key, e.Action, wantAction)
		}
	}

	// Standard environment carries an explanatory detail.
	for _, e := range plan {
		if e.ResourceType == "environment" && e.Slug == "standard" {
			if e.Details == "" {
				t.Error("standard environment SKIP should include a detail")
			}
		}
	}
}

func TestComputePlan_DependencyOrder(t *testing.T) {
	t.Parallel()

	b := &Bundle{
		Resources: Resources{
			Environments: []EnvironmentSpec{{Slug: "e1", Name: "E1"}},
			Jobs:         []JobSpec{{Slug: "j1", Name: "J1"}},
			Workflows:    []WorkflowSpec{{Slug: "w1", Name: "W1"}},
		},
	}
	plan := ComputePlan(b, ExistingState{})
	if len(plan) != 3 {
		t.Fatalf("plan len = %d, want 3", len(plan))
	}
	order := []string{plan[0].ResourceType, plan[1].ResourceType, plan[2].ResourceType}
	want := []string{"environment", "job", "workflow"}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("position %d = %q, want %q (full: %v)", i, order[i], want[i], order)
		}
	}
}
