package api

import (
	"context"
	"net/http"
	"testing"

	"strait/internal/billing"
	"strait/internal/bundle"
	"strait/internal/domain"
)

// statusOf extracts the HTTP status from a huma error, failing the test if the
// error is not a huma status error.
func statusOf(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	humaErr, ok := err.(interface{ GetStatus() int })
	if !ok {
		t.Fatalf("expected huma error, got %T: %v", err, err)
	}
	return humaErr.GetStatus()
}

func TestHandleImportBundle_EmptyProjectID(t *testing.T) {
	t.Parallel()
	s := &Server{edition: domain.EditionCommunity}
	_, err := s.handleImportBundle(context.Background(), &ImportBundleInput{ProjectID: ""})
	if got := statusOf(t, err); got != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", got)
	}
}

func TestHandleImportBundle_ProjectMismatch(t *testing.T) {
	t.Parallel()
	s := &Server{edition: domain.EditionCommunity}
	// Context bound to a different project than the path parameter.
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-auth")
	_, err := s.handleImportBundle(ctx, &ImportBundleInput{ProjectID: "proj-other"})
	if got := statusOf(t, err); got != http.StatusForbidden {
		t.Errorf("status = %d, want 403", got)
	}
}

func TestHandleImportBundle_SingletonValidationRejected(t *testing.T) {
	t.Parallel()
	s := &Server{edition: domain.EditionCommunity}
	// Internal caller (no project in context) so requireProjectMatch passes and
	// validation is reached. A job carrying an on-conflict policy without a key
	// expression is invalid and must fail before any state is read or written.
	in := &ImportBundleInput{
		ProjectID: "proj-1",
		Body: bundle.Bundle{Resources: bundle.Resources{
			Jobs: []bundle.JobSpec{{
				Slug:                "j",
				Name:                "j",
				EndpointURL:         "https://example.com/hook",
				SingletonOnConflict: "queue",
			}},
		}},
	}
	_, err := s.handleImportBundle(context.Background(), in)
	if got := statusOf(t, err); got != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", got)
	}
}

func TestHandleImportBundle_ReplaceGatedOnCloudFreePlan(t *testing.T) {
	t.Parallel()
	// On cloud, the replace policy is gated behind a billing feature. A free-plan
	// org importing a job that wants replace must be rejected before any write,
	// mirroring the job create/update handlers.
	s := &Server{
		edition: domain.EditionCloud,
		billingEnforcer: &mockHTTPModeEnforcer{
			mockBillingEnforcer: mockBillingEnforcer{
				projectOrgMap: map[string]string{"proj-1": "org-1"},
			},
			planLimits: billing.GetPlanLimits(domain.PlanFree),
		},
	}
	in := &ImportBundleInput{
		ProjectID: "proj-1",
		Body: bundle.Bundle{Resources: bundle.Resources{
			Jobs: []bundle.JobSpec{{
				Slug:                "j",
				Name:                "j",
				EndpointURL:         "https://example.com/hook",
				SingletonKey:        "${id}",
				SingletonOnConflict: "replace",
			}},
		}},
	}
	_, err := s.handleImportBundle(context.Background(), in)
	if got := statusOf(t, err); got != http.StatusForbidden {
		t.Errorf("status = %d, want 403", got)
	}
}

func TestCountAction(t *testing.T) {
	t.Parallel()
	plan := []bundle.DiffEntry{
		{ResourceType: "job", Slug: "a", Action: bundle.DiffCreate},
		{ResourceType: "job", Slug: "b", Action: bundle.DiffUpdate},
		{ResourceType: "workflow", Slug: "c", Action: bundle.DiffCreate},
		{ResourceType: "environment", Slug: "d", Action: bundle.DiffSkip},
	}
	if got := countAction(plan, bundle.DiffCreate); got != 2 {
		t.Errorf("create count = %d, want 2", got)
	}
	if got := countAction(plan, bundle.DiffUpdate); got != 1 {
		t.Errorf("update count = %d, want 1", got)
	}
	if got := countAction(plan, bundle.DiffSkip); got != 1 {
		t.Errorf("skip count = %d, want 1", got)
	}
}

func TestResolveParentEnv(t *testing.T) {
	t.Parallel()
	slugToID := map[string]string{"prod": "env-prod"}

	t.Run("unset parent is allowed", func(t *testing.T) {
		t.Parallel()
		id, err := resolveParentEnv(bundle.EnvironmentSpec{Slug: "staging"}, slugToID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "" {
			t.Errorf("id = %q, want empty", id)
		}
	})

	t.Run("known parent resolves", func(t *testing.T) {
		t.Parallel()
		id, err := resolveParentEnv(bundle.EnvironmentSpec{Slug: "staging", ParentSlug: "prod"}, slugToID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "env-prod" {
			t.Errorf("id = %q, want env-prod", id)
		}
	})

	t.Run("unknown parent is 400", func(t *testing.T) {
		t.Parallel()
		_, err := resolveParentEnv(bundle.EnvironmentSpec{Slug: "staging", ParentSlug: "ghost"}, slugToID)
		if got := statusOf(t, err); got != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", got)
		}
	})
}

func TestResolveJobEnvironment(t *testing.T) {
	t.Parallel()
	envSlugToID := map[string]string{"prod": "env-prod"}

	t.Run("unset environment is allowed", func(t *testing.T) {
		t.Parallel()
		id, err := resolveJobEnvironment(bundle.JobSpec{Slug: "j"}, envSlugToID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "" {
			t.Errorf("id = %q, want empty", id)
		}
	})

	t.Run("known environment resolves", func(t *testing.T) {
		t.Parallel()
		id, err := resolveJobEnvironment(bundle.JobSpec{Slug: "j", EnvironmentSlug: "prod"}, envSlugToID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "env-prod" {
			t.Errorf("id = %q, want env-prod", id)
		}
	})

	t.Run("unknown environment is 400", func(t *testing.T) {
		t.Parallel()
		_, err := resolveJobEnvironment(bundle.JobSpec{Slug: "j", EnvironmentSlug: "ghost"}, envSlugToID)
		if got := statusOf(t, err); got != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", got)
		}
	})
}

func TestApplyJobSpecOnto_PreservesNonBundleFields(t *testing.T) {
	t.Parallel()
	// An existing job carries columns the bundle format does not model. The
	// overlay must rewrite the bundle-owned fields and leave the rest intact.
	job := &domain.Job{
		ID:            "job-1",
		ProjectID:     "proj-1",
		Name:          "old name",
		Slug:          "j",
		EndpointURL:   "https://old.example.com/hook",
		WebhookSecret: "keep-this-secret",
		MaxAttempts:   1,
		Version:       7,
	}
	spec := bundle.JobSpec{
		Slug:        "j",
		Name:        "new name",
		EndpointURL: "https://new.example.com/hook",
		MaxAttempts: 5,
		Enabled:     true,
	}
	applyJobSpecOnto(job, spec, "proj-1", "env-1")

	if job.Name != "new name" {
		t.Errorf("Name = %q, want new name", job.Name)
	}
	if job.EndpointURL != "https://new.example.com/hook" {
		t.Errorf("EndpointURL = %q, want overlaid value", job.EndpointURL)
	}
	if job.MaxAttempts != 5 {
		t.Errorf("MaxAttempts = %d, want 5", job.MaxAttempts)
	}
	if job.EnvironmentID != "env-1" {
		t.Errorf("EnvironmentID = %q, want env-1", job.EnvironmentID)
	}
	// Non-bundle columns and identity must survive untouched.
	if job.WebhookSecret != "keep-this-secret" {
		t.Errorf("WebhookSecret = %q, want preserved", job.WebhookSecret)
	}
	if job.ID != "job-1" {
		t.Errorf("ID = %q, want preserved", job.ID)
	}
	if job.Version != 7 {
		t.Errorf("Version = %d, want preserved", job.Version)
	}
}

func TestApplyWorkflowSpecOnto_PreservesEnabled(t *testing.T) {
	t.Parallel()
	wf := &domain.Workflow{
		ID:                "wf-1",
		ProjectID:         "proj-1",
		Name:              "old",
		Slug:              "w",
		Enabled:           true,
		MaxConcurrentRuns: 1,
		Version:           3,
	}
	spec := bundle.WorkflowSpec{
		Slug:              "w",
		Name:              "new",
		MaxConcurrentRuns: 9,
	}
	applyWorkflowSpecOnto(wf, spec)

	if wf.Name != "new" {
		t.Errorf("Name = %q, want new", wf.Name)
	}
	if wf.MaxConcurrentRuns != 9 {
		t.Errorf("MaxConcurrentRuns = %d, want 9", wf.MaxConcurrentRuns)
	}
	// Enabled is not a bundle-owned field and must be left alone.
	if !wf.Enabled {
		t.Error("Enabled was cleared, want preserved")
	}
	if wf.ID != "wf-1" {
		t.Errorf("ID = %q, want preserved", wf.ID)
	}
}
