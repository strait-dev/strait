package bundle

import (
	"fmt"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExportBundle_EmptyJobs verifies exporting with no jobs produces empty job list.
func TestExportBundle_EmptyJobs(t *testing.T) {
	t.Parallel()

	b := ExportBundle("proj-1", nil, nil, nil, nil, nil, nil)

	require.NotNil(t, b)
	assert.Empty(t, b.Resources.Jobs)
	assert.Equal(t, Version, b.Version)
	assert.Equal(t, "proj-1", b.SourceProjectID)
}

// TestExportBundle_EmptyWorkflows verifies exporting with jobs but no workflows.
func TestExportBundle_EmptyWorkflows(t *testing.T) {
	t.Parallel()

	jobs := []domain.Job{
		{ID: "j1", Slug: "test-job", Name: "Test", EndpointURL: "https://example.com"},
	}

	b := ExportBundle("proj-1", jobs, nil, nil, nil, nil, nil)

	require.Len(t, b.Resources.Jobs, 1)
	assert.Equal(t, "test-job", b.Resources.Jobs[0].Slug)
	assert.Empty(t, b.Resources.Workflows)
}

// TestExportBundle_MissingSlugMapping verifies that a job ID not in the slug map
// results in an empty job slug on workflow steps.
func TestExportBundle_MissingSlugMapping(t *testing.T) {
	t.Parallel()

	workflows := []domain.Workflow{
		{ID: "wf1", Slug: "my-wf", Name: "Workflow"},
	}
	steps := map[string][]domain.WorkflowStep{
		"wf1": {
			{ID: "s1", WorkflowID: "wf1", JobID: "missing-job-id", StepRef: "step-1"},
		},
	}
	// Empty jobIDToSlug map, so the job ID won't resolve.
	jobIDToSlug := map[string]string{}

	b := ExportBundle("proj-1", nil, workflows, steps, nil, jobIDToSlug, nil)

	require.Len(t, b.Resources.Workflows, 1)
	wf := b.Resources.Workflows[0]
	require.Len(t, wf.Steps, 1)
	assert.Empty(t, wf.Steps[0].JobSlug)
}

// TestExportBundle_NilMaps verifies nil maps do not cause panics.
func TestExportBundle_NilMaps(t *testing.T) {
	t.Parallel()

	jobs := []domain.Job{
		{ID: "j1", Slug: "job-1", EnvironmentID: "env-1"},
	}
	workflows := []domain.Workflow{
		{ID: "wf1", Slug: "wf-1"},
	}

	// All maps are nil.
	b := ExportBundle("proj-1", jobs, workflows, nil, nil, nil, nil)

	require.NotNil(t, b)
	require.Len(t, b.Resources.Jobs, 1)
	// Environment slug should be empty since envIDToSlug is nil.
	assert.Empty(t, b.Resources.Jobs[0].EnvironmentSlug)
}

// TestExportBundle_LargeBundle verifies exporting 1000 jobs and 100 workflows.
func TestExportBundle_LargeBundle(t *testing.T) {
	t.Parallel()

	jobs := make([]domain.Job, 1000)
	jobIDToSlug := make(map[string]string, 1000)
	for i := range jobs {
		id := fmt.Sprintf("job-%d", i)
		slug := fmt.Sprintf("slug-%d", i)
		jobs[i] = domain.Job{ID: id, Slug: slug, Name: slug, EndpointURL: "https://example.com"}
		jobIDToSlug[id] = slug
	}

	workflows := make([]domain.Workflow, 100)
	steps := make(map[string][]domain.WorkflowStep, 100)
	for i := range workflows {
		wfID := fmt.Sprintf("wf-%d", i)
		workflows[i] = domain.Workflow{ID: wfID, Slug: fmt.Sprintf("wf-slug-%d", i), Name: fmt.Sprintf("WF %d", i)}
		steps[wfID] = []domain.WorkflowStep{
			{ID: fmt.Sprintf("step-%d", i), WorkflowID: wfID, JobID: fmt.Sprintf("job-%d", i%1000), StepRef: "s1"},
		}
	}

	b := ExportBundle("proj-1", jobs, workflows, steps, nil, jobIDToSlug, nil)

	assert.Len(t, b.Resources.Jobs, 1000)
	require.Len(t, b.Resources.Workflows, 100)

	// Verify step slug resolution for the first workflow.
	require.Len(t, b.Resources.Workflows[0].Steps, 1)
	assert.Equal(t, "slug-0", b.Resources.Workflows[0].Steps[0].JobSlug)
}

// FuzzExportBundle fuzzes the number of jobs and workflows passed to ExportBundle.
func FuzzExportBundle(f *testing.F) {
	f.Add(0, 0)
	f.Add(1, 0)
	f.Add(0, 1)
	f.Add(5, 3)
	f.Add(50, 10)

	f.Fuzz(func(t *testing.T, numJobs, numWorkflows int) {
		// Clamp to reasonable bounds.
		if numJobs < 0 {
			numJobs = 0
		}
		if numJobs > 200 {
			numJobs = 200
		}
		if numWorkflows < 0 {
			numWorkflows = 0
		}
		if numWorkflows > 50 {
			numWorkflows = 50
		}

		jobs := make([]domain.Job, numJobs)
		for i := range jobs {
			jobs[i] = domain.Job{
				ID:   fmt.Sprintf("j-%d", i),
				Slug: fmt.Sprintf("s-%d", i),
			}
		}

		workflows := make([]domain.Workflow, numWorkflows)
		for i := range workflows {
			workflows[i] = domain.Workflow{
				ID:   fmt.Sprintf("wf-%d", i),
				Slug: fmt.Sprintf("wfs-%d", i),
			}
		}

		b := ExportBundle("proj", jobs, workflows, nil, nil, nil, nil)
		require.NotNil(t, b)
		assert.Len(t, b.Resources.Jobs, numJobs)
		assert.Len(t, b.Resources.Workflows, numWorkflows)
	})
}
