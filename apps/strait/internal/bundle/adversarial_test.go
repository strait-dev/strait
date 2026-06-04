package bundle

import (
	"fmt"
	"testing"

	"strait/internal/domain"
)

// TestExportBundle_EmptyJobs verifies exporting with no jobs produces empty job list.
func TestExportBundle_EmptyJobs(t *testing.T) {
	t.Parallel()

	b := ExportBundle("proj-1", nil, nil, nil, nil, nil, nil)

	if b == nil {
		t.Fatal("expected non-nil bundle")
		return
	}
	if len(b.Resources.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(b.Resources.Jobs))
	}
	if b.Version != Version {
		t.Errorf("version = %q, want %q", b.Version, Version)
	}
	if b.SourceProjectID != "proj-1" {
		t.Errorf("source project = %q, want %q", b.SourceProjectID, "proj-1")
	}
}

// TestExportBundle_EmptyWorkflows verifies exporting with jobs but no workflows.
func TestExportBundle_EmptyWorkflows(t *testing.T) {
	t.Parallel()

	jobs := []domain.Job{
		{ID: "j1", Slug: "test-job", Name: "Test", EndpointURL: "https://example.com"},
	}

	b := ExportBundle("proj-1", jobs, nil, nil, nil, nil, nil)

	if len(b.Resources.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(b.Resources.Jobs))
	}
	if b.Resources.Jobs[0].Slug != "test-job" {
		t.Errorf("job slug = %q, want %q", b.Resources.Jobs[0].Slug, "test-job")
	}
	if len(b.Resources.Workflows) != 0 {
		t.Errorf("expected 0 workflows, got %d", len(b.Resources.Workflows))
	}
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

	if len(b.Resources.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(b.Resources.Workflows))
	}
	wf := b.Resources.Workflows[0]
	if len(wf.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(wf.Steps))
	}
	if wf.Steps[0].JobSlug != "" {
		t.Errorf("expected empty job slug for unmapped ID, got %q", wf.Steps[0].JobSlug)
	}
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

	if b == nil {
		t.Fatal("expected non-nil bundle")
		return
	}
	if len(b.Resources.Jobs) != 1 {
		t.Errorf("expected 1 job, got %d", len(b.Resources.Jobs))
	}
	// Environment slug should be empty since envIDToSlug is nil.
	if b.Resources.Jobs[0].EnvironmentSlug != "" {
		t.Errorf("expected empty env slug, got %q", b.Resources.Jobs[0].EnvironmentSlug)
	}
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

	if len(b.Resources.Jobs) != 1000 {
		t.Errorf("expected 1000 jobs, got %d", len(b.Resources.Jobs))
	}
	if len(b.Resources.Workflows) != 100 {
		t.Errorf("expected 100 workflows, got %d", len(b.Resources.Workflows))
	}

	// Verify step slug resolution for the first workflow.
	if len(b.Resources.Workflows[0].Steps) != 1 {
		t.Fatalf("expected 1 step in first workflow, got %d", len(b.Resources.Workflows[0].Steps))
	}
	if b.Resources.Workflows[0].Steps[0].JobSlug != "slug-0" {
		t.Errorf("step job slug = %q, want %q", b.Resources.Workflows[0].Steps[0].JobSlug, "slug-0")
	}
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
		if b == nil {
			t.Fatal("expected non-nil bundle")
			return
		}
		if len(b.Resources.Jobs) != numJobs {
			t.Errorf("job count = %d, want %d", len(b.Resources.Jobs), numJobs)
		}
		if len(b.Resources.Workflows) != numWorkflows {
			t.Errorf("workflow count = %d, want %d", len(b.Resources.Workflows), numWorkflows)
		}
	})
}
