package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkflow_TagsField(t *testing.T) {
	t.Parallel()

	wf := Workflow{
		ID:        "wf-1",
		ProjectID: "proj-1",
		Name:      "test",
		Slug:      "test",
		Tags:      map[string]string{"team": "platform", "env": "prod"},
	}
	require.Equal(
		t, "platform", wf.Tags["team"])
	require.Equal(
		t, "prod", wf.Tags["env"])

}

func TestJobRun_TagsField(t *testing.T) {
	t.Parallel()

	run := JobRun{
		ID:    "run-1",
		JobID: "job-1",
		Tags:  map[string]string{"release": "v2.1"},
	}
	require.Equal(
		t, "v2.1", run.Tags["release"])

}

func TestWorkflowRun_TagsField(t *testing.T) {
	t.Parallel()

	run := WorkflowRun{
		ID:         "wfr-1",
		WorkflowID: "wf-1",
		Tags:       map[string]string{"deploy": "canary"},
	}
	require.Equal(
		t, "canary", run.Tags["deploy"])

}
