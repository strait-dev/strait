package domain

import "testing"

func TestWorkflow_TagsField(t *testing.T) {
	t.Parallel()

	wf := Workflow{
		ID:        "wf-1",
		ProjectID: "proj-1",
		Name:      "test",
		Slug:      "test",
		Tags:      map[string]string{"team": "platform", "env": "prod"},
	}

	if wf.Tags["team"] != "platform" {
		t.Fatalf("tags[team] = %q, want %q", wf.Tags["team"], "platform")
	}
	if wf.Tags["env"] != "prod" {
		t.Fatalf("tags[env] = %q, want %q", wf.Tags["env"], "prod")
	}
}

func TestJobRun_TagsField(t *testing.T) {
	t.Parallel()

	run := JobRun{
		ID:    "run-1",
		JobID: "job-1",
		Tags:  map[string]string{"release": "v2.1"},
	}

	if run.Tags["release"] != "v2.1" {
		t.Fatalf("tags[release] = %q, want %q", run.Tags["release"], "v2.1")
	}
}

func TestWorkflowRun_TagsField(t *testing.T) {
	t.Parallel()

	run := WorkflowRun{
		ID:         "wfr-1",
		WorkflowID: "wf-1",
		Tags:       map[string]string{"deploy": "canary"},
	}

	if run.Tags["deploy"] != "canary" {
		t.Fatalf("tags[deploy] = %q, want %q", run.Tags["deploy"], "canary")
	}
}
