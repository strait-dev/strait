//go:build integration

package api

import (
	"context"
	"sync"
	"testing"

	"strait/internal/bundle"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
)

var (
	bundleTestDB     *testutil.TestDB
	bundleTestDBOnce sync.Once
)

func getBundleTestDB(t *testing.T) *testutil.TestDB {
	t.Helper()
	bundleTestDBOnce.Do(func() {
		var err error
		bundleTestDB, err = testutil.SetupTestDB(context.Background(), "../../migrations")
		if err != nil {
			t.Fatalf("SetupTestDB() error = %v", err)
		}
	})
	if bundleTestDB == nil || bundleTestDB.Pool == nil {
		t.Fatal("bundleTestDB is not initialized")
	}
	return bundleTestDB
}

// newBundleImportServer builds a community server backed by the shared bundle
// integration DB, with TxPool wired so the import runs in a real transaction
// (required to exercise atomic rollback). It returns the server, the routed
// store, a project id, and a request context scoped to that project.
func newBundleImportServer(t *testing.T, ctx context.Context, db *testutil.TestDB) (*Server, store.Store, string, context.Context) {
	t.Helper()
	st := store.NewWithContextRouting(db.Pool)
	// Environment variables are encrypted at rest; the store needs a key both for
	// the direct CreateEnvironment seeding below and for the in-transaction
	// import writes (the tx queries inherit this configuration).
	st.SetSecretEncryptionKey("abcdefghijklmnopqrstuvwxyz012345")
	q := queue.NewPostgresQueue(db.Pool)
	srv := NewServer(ServerDeps{
		Config: &config.Config{
			InternalSecret:      "test-secret-value",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       testJWTSigningKey,
			SecretEncryptionKey: "abcdefghijklmnopqrstuvwxyz012345",
		},
		Store:   st,
		Queue:   q,
		TxPool:  db.Pool,
		Edition: domain.EditionCommunity,
	})
	t.Cleanup(srv.Close)

	projectID := "project-" + uuid.Must(uuid.NewV7()).String()
	if _, err := db.Pool.Exec(ctx, `INSERT INTO projects (id, name) VALUES ($1, $2)`, projectID, "bundle import project"); err != nil {
		t.Fatalf("insert project: %v", err)
	}

	reqCtx := context.WithValue(ctx, ctxProjectIDKey, projectID)
	reqCtx = context.WithValue(reqCtx, ctxActorTypeKey, "api_key")
	reqCtx = context.WithValue(reqCtx, ctxActorIDKey, "apikey:test")
	return srv, st, projectID, reqCtx
}

func TestIntegration_BundleImport_DryRunNoWrites(t *testing.T) {
	ctx := context.Background()
	db := getBundleTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	srv, st, projectID, reqCtx := newBundleImportServer(t, ctx, db)

	b := bundle.Bundle{Resources: bundle.Resources{
		Jobs: []bundle.JobSpec{{
			Slug:        "dryrun-job",
			Name:        "Dry Run Job",
			EndpointURL: "https://example.com/hook",
			MaxAttempts: 3,
			TimeoutSecs: 300,
			Enabled:     true,
		}},
	}}

	out, err := srv.handleImportBundle(reqCtx, &ImportBundleInput{ProjectID: projectID, DryRun: true, Body: b})
	if err != nil {
		t.Fatalf("handleImportBundle(dry_run) error = %v", err)
	}
	if out.Body.Created != 1 {
		t.Errorf("Created = %d, want 1", out.Body.Created)
	}
	if len(out.Body.Diff) != 1 || out.Body.Diff[0].Action != bundle.DiffCreate {
		t.Errorf("Diff = %+v, want one CREATE entry", out.Body.Diff)
	}

	// Nothing must have been written.
	if _, err := st.GetJobBySlug(ctx, projectID, "dryrun-job"); err == nil {
		t.Fatal("dry-run created a job; want no writes")
	}
}

func TestIntegration_BundleImport_CreateResources(t *testing.T) {
	ctx := context.Background()
	db := getBundleTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	srv, st, projectID, reqCtx := newBundleImportServer(t, ctx, db)

	depth := 5
	b := bundle.Bundle{Resources: bundle.Resources{
		Environments: []bundle.EnvironmentSpec{{
			Name:      "Production",
			Slug:      "prod",
			Variables: map[string]string{"REGION": "us-east-1"},
		}},
		Jobs: []bundle.JobSpec{{
			Slug:                   "rebuild-cache",
			Name:                   "Rebuild Cache",
			EndpointURL:            "https://example.com/hook",
			MaxAttempts:            3,
			TimeoutSecs:            300,
			Enabled:                true,
			EnvironmentSlug:        "prod",
			SingletonKey:           "${account.id}",
			SingletonOnConflict:    "queue",
			SingletonMaxQueueDepth: &depth,
		}},
		Workflows: []bundle.WorkflowSpec{{
			Slug: "nightly",
			Name: "Nightly",
			Steps: []bundle.WorkflowStepSpec{{
				StepRef: "step-1",
				JobSlug: "rebuild-cache",
			}},
		}},
	}}

	out, err := srv.handleImportBundle(reqCtx, &ImportBundleInput{ProjectID: projectID, Body: b})
	if err != nil {
		t.Fatalf("handleImportBundle() error = %v", err)
	}
	if out.Body.Created != 3 {
		t.Errorf("Created = %d, want 3", out.Body.Created)
	}

	// Environment persisted with its variable.
	envs, err := st.ListEnvironments(ctx, projectID, 100, nil)
	if err != nil {
		t.Fatalf("ListEnvironments() error = %v", err)
	}
	var prodID string
	for _, e := range envs {
		if e.Slug == "prod" {
			prodID = e.ID
		}
	}
	if prodID == "" {
		t.Fatal("environment prod not created")
	}
	vars, err := st.GetResolvedEnvironmentVariables(ctx, prodID)
	if err != nil {
		t.Fatalf("GetResolvedEnvironmentVariables() error = %v", err)
	}
	if vars["REGION"] != "us-east-1" {
		t.Errorf("REGION = %q, want us-east-1", vars["REGION"])
	}

	// Job persisted with environment link and singleton config.
	job, err := st.GetJobBySlug(ctx, projectID, "rebuild-cache")
	if err != nil {
		t.Fatalf("GetJobBySlug() error = %v", err)
	}
	if job.EnvironmentID != prodID {
		t.Errorf("EnvironmentID = %q, want %q", job.EnvironmentID, prodID)
	}
	if job.SingletonOnConflict != domain.SingletonOnConflictQueue {
		t.Errorf("SingletonOnConflict = %q, want queue", job.SingletonOnConflict)
	}
	if job.SingletonMaxQueueDepth == nil || *job.SingletonMaxQueueDepth != 5 {
		t.Errorf("SingletonMaxQueueDepth = %v, want 5", job.SingletonMaxQueueDepth)
	}
	expr, err := domain.ParseSingletonKeyExpr(job.SingletonKeyExpr)
	if err != nil {
		t.Fatalf("ParseSingletonKeyExpr() error = %v", err)
	}
	if expr.Template != "${account.id}" {
		t.Errorf("template = %q, want ${account.id}", expr.Template)
	}

	// Workflow persisted with a step pointing at the new job.
	wf, err := st.GetWorkflowBySlug(ctx, projectID, "nightly")
	if err != nil {
		t.Fatalf("GetWorkflowBySlug() error = %v", err)
	}
	steps, err := st.ListStepsByWorkflow(ctx, wf.ID)
	if err != nil {
		t.Fatalf("ListStepsByWorkflow() error = %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("step count = %d, want 1", len(steps))
	}
	if steps[0].JobID != job.ID {
		t.Errorf("step JobID = %q, want %q", steps[0].JobID, job.ID)
	}
}

func TestIntegration_BundleImport_UpdatePreservesNonBundleFields(t *testing.T) {
	ctx := context.Background()
	db := getBundleTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	srv, st, projectID, reqCtx := newBundleImportServer(t, ctx, db)

	secret := "do-not-clobber"
	slug := "existing-job"
	originalName := "Original Name"
	job := testutil.MustCreateJob(t, ctx, st, &testutil.JobOpts{
		ProjectID:     &projectID,
		Slug:          &slug,
		Name:          &originalName,
		WebhookSecret: &secret,
	})
	startVersion := job.Version

	b := bundle.Bundle{Resources: bundle.Resources{
		Jobs: []bundle.JobSpec{{
			Slug:        slug,
			Name:        "Updated Name",
			EndpointURL: "https://example.com/new-hook",
			MaxAttempts: 7,
			TimeoutSecs: 120,
			Enabled:     true,
		}},
	}}

	out, err := srv.handleImportBundle(reqCtx, &ImportBundleInput{ProjectID: projectID, Body: b})
	if err != nil {
		t.Fatalf("handleImportBundle() error = %v", err)
	}
	if out.Body.Updated != 1 {
		t.Errorf("Updated = %d, want 1", out.Body.Updated)
	}

	updated, err := st.GetJobBySlug(ctx, projectID, slug)
	if err != nil {
		t.Fatalf("GetJobBySlug() error = %v", err)
	}
	if updated.Name != "Updated Name" {
		t.Errorf("Name = %q, want Updated Name", updated.Name)
	}
	if updated.MaxAttempts != 7 {
		t.Errorf("MaxAttempts = %d, want 7", updated.MaxAttempts)
	}
	if updated.Version <= startVersion {
		t.Errorf("Version = %d, want > %d (a new version)", updated.Version, startVersion)
	}
	// WebhookSecret is not a bundle-owned field and must survive the update.
	if updated.WebhookSecret != secret {
		t.Errorf("WebhookSecret = %q, want preserved %q", updated.WebhookSecret, secret)
	}

	// A version snapshot must have been recorded for the update.
	versions, err := st.ListJobVersionsByJob(ctx, updated.ID, 100, nil)
	if err != nil {
		t.Fatalf("ListJobVersionsByJob() error = %v", err)
	}
	if len(versions) == 0 {
		t.Error("expected at least one job version snapshot after update")
	}
}

func TestIntegration_BundleImport_RedactedEnvVar(t *testing.T) {
	ctx := context.Background()
	db := getBundleTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	srv, st, projectID, reqCtx := newBundleImportServer(t, ctx, db)

	// Seed an environment with a real secret value.
	env := &domain.Environment{
		ProjectID: projectID,
		Name:      "Production",
		Slug:      "prod",
		Variables: map[string]string{"API_TOKEN": "super-secret"},
	}
	if err := st.CreateEnvironment(ctx, env); err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}

	t.Run("redacted value preserved on update", func(t *testing.T) {
		b := bundle.Bundle{Resources: bundle.Resources{
			Environments: []bundle.EnvironmentSpec{{
				Name:      "Production",
				Slug:      "prod",
				Variables: map[string]string{"API_TOKEN": bundle.RedactedPlaceholder},
			}},
		}}
		if _, err := srv.handleImportBundle(reqCtx, &ImportBundleInput{ProjectID: projectID, Body: b}); err != nil {
			t.Fatalf("handleImportBundle() error = %v", err)
		}
		vars, err := st.GetResolvedEnvironmentVariables(ctx, env.ID)
		if err != nil {
			t.Fatalf("GetResolvedEnvironmentVariables() error = %v", err)
		}
		if vars["API_TOKEN"] != "super-secret" {
			t.Errorf("API_TOKEN = %q, want preserved super-secret", vars["API_TOKEN"])
		}
	})

	t.Run("redacted value on create is rejected", func(t *testing.T) {
		b := bundle.Bundle{Resources: bundle.Resources{
			Environments: []bundle.EnvironmentSpec{{
				Name:      "Staging",
				Slug:      "staging",
				Variables: map[string]string{"API_TOKEN": bundle.RedactedPlaceholder},
			}},
		}}
		_, err := srv.handleImportBundle(reqCtx, &ImportBundleInput{ProjectID: projectID, Body: b})
		if got := statusOf(t, err); got != 400 {
			t.Errorf("status = %d, want 400", got)
		}
		// The rejected create must not have produced an environment.
		envs, listErr := st.ListEnvironments(ctx, projectID, 100, nil)
		if listErr != nil {
			t.Fatalf("ListEnvironments() error = %v", listErr)
		}
		for _, e := range envs {
			if e.Slug == "staging" {
				t.Error("staging environment should not exist after rejected create")
			}
		}
	})
}

func TestIntegration_BundleImport_AtomicRollback(t *testing.T) {
	ctx := context.Background()
	db := getBundleTestDB(t)
	if err := db.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
	srv, st, projectID, reqCtx := newBundleImportServer(t, ctx, db)

	// The job is valid and is applied first; the workflow step references a job
	// slug that does not exist, which fails mid-transaction. The whole import
	// must roll back, leaving the earlier job unwritten.
	b := bundle.Bundle{Resources: bundle.Resources{
		Jobs: []bundle.JobSpec{{
			Slug:        "valid-job",
			Name:        "Valid Job",
			EndpointURL: "https://example.com/hook",
			MaxAttempts: 3,
			TimeoutSecs: 300,
			Enabled:     true,
		}},
		Workflows: []bundle.WorkflowSpec{{
			Slug: "broken",
			Name: "Broken",
			Steps: []bundle.WorkflowStepSpec{{
				StepRef: "step-1",
				JobSlug: "ghost-job",
			}},
		}},
	}}

	_, err := srv.handleImportBundle(reqCtx, &ImportBundleInput{ProjectID: projectID, Body: b})
	if got := statusOf(t, err); got != 400 {
		t.Fatalf("status = %d, want 400 for unknown job_slug", got)
	}

	// Rollback must leave no trace of the job that applied before the failure.
	if _, err := st.GetJobBySlug(ctx, projectID, "valid-job"); err == nil {
		t.Fatal("valid-job persisted; transaction did not roll back")
	}
	if _, err := st.GetWorkflowBySlug(ctx, projectID, "broken"); err == nil {
		t.Fatal("broken workflow persisted; transaction did not roll back")
	}
}
