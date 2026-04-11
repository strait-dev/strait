//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

// projectSecretsFixture gives every project_secrets test a project and
// two environments to key off of, plus a job to cover the per-job
// override path.
type projectSecretsFixture struct {
	project *domain.Project
	devEnv  *domain.Environment
	prodEnv *domain.Environment
	job     *domain.Job
}

func mustCreateProjectSecretsFixture(t *testing.T, ctx context.Context, q *store.Queries) *projectSecretsFixture {
	t.Helper()
	project := &domain.Project{ID: newID(), OrgID: "org-secrets-" + newID(), Name: "Secrets Fixture"}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	devEnv := &domain.Environment{ID: newID(), ProjectID: project.ID, Name: "Development", Slug: "dev"}
	if err := q.CreateEnvironment(ctx, devEnv); err != nil {
		t.Fatalf("CreateEnvironment(dev) error = %v", err)
	}
	prodEnv := &domain.Environment{ID: newID(), ProjectID: project.ID, Name: "Production", Slug: "prod"}
	if err := q.CreateEnvironment(ctx, prodEnv); err != nil {
		t.Fatalf("CreateEnvironment(prod) error = %v", err)
	}
	job := mustCreateJob(t, ctx, q, project.ID)
	return &projectSecretsFixture{project: project, devEnv: devEnv, prodEnv: prodEnv, job: job}
}

func mustCreateProjectSecret(t *testing.T, ctx context.Context, q *store.Queries, projectID, envID, jobID, key, plaintext string) *domain.ProjectSecret {
	t.Helper()
	s := &domain.ProjectSecret{
		ProjectID:      projectID,
		EnvironmentID:  envID,
		JobID:          jobID,
		SecretKey:      key,
		EncryptedValue: plaintext, // Create encrypts in place before persisting.
	}
	if err := q.CreateProjectSecret(ctx, s); err != nil {
		t.Fatalf("CreateProjectSecret(%s) error = %v", key, err)
	}
	return s
}

func findSecret(secrets []domain.ProjectSecret, key string) *domain.ProjectSecret {
	for i := range secrets {
		if secrets[i].SecretKey == key {
			return &secrets[i]
		}
	}
	return nil
}

func TestCreateProjectSecret_ProjectWide(t *testing.T) {
	ctx := context.Background()
	q := mustStoreWithEncryption(t)
	mustClean(t, ctx)
	fx := mustCreateProjectSecretsFixture(t, ctx, q)

	_ = mustCreateProjectSecret(t, ctx, q, fx.project.ID, fx.devEnv.ID, "", "API_KEY", "plaintext-value")

	secrets, err := q.ListProjectSecretsByEnv(ctx, fx.project.ID, fx.devEnv.ID)
	if err != nil {
		t.Fatalf("ListProjectSecretsByEnv() error = %v", err)
	}
	if len(secrets) != 1 {
		t.Fatalf("got %d secrets, want 1", len(secrets))
	}
	if secrets[0].SecretKey != "API_KEY" {
		t.Fatalf("key = %q, want API_KEY", secrets[0].SecretKey)
	}
	if secrets[0].EncryptedValue != "plaintext-value" {
		t.Fatalf("decrypted value = %q, want plaintext-value", secrets[0].EncryptedValue)
	}
	if secrets[0].JobID != "" {
		t.Fatalf("job_id = %q, want empty (project-wide)", secrets[0].JobID)
	}
}

func TestCreateProjectSecret_JobScoped(t *testing.T) {
	ctx := context.Background()
	q := mustStoreWithEncryption(t)
	mustClean(t, ctx)
	fx := mustCreateProjectSecretsFixture(t, ctx, q)

	_ = mustCreateProjectSecret(t, ctx, q, fx.project.ID, fx.devEnv.ID, fx.job.ID, "API_KEY", "job-specific")

	// ListProjectSecretsByEnv must NOT return job-scoped rows.
	projectWide, err := q.ListProjectSecretsByEnv(ctx, fx.project.ID, fx.devEnv.ID)
	if err != nil {
		t.Fatalf("ListProjectSecretsByEnv() error = %v", err)
	}
	if len(projectWide) != 0 {
		t.Fatalf("got %d project-wide secrets, want 0 (only job-scoped exists)", len(projectWide))
	}

	// ListProjectSecretsForJob does see the job-scoped row.
	forJob, err := q.ListProjectSecretsForJob(ctx, fx.project.ID, fx.job.ID, fx.devEnv.ID)
	if err != nil {
		t.Fatalf("ListProjectSecretsForJob() error = %v", err)
	}
	if len(forJob) != 1 {
		t.Fatalf("got %d for-job secrets, want 1", len(forJob))
	}
	if forJob[0].JobID != fx.job.ID {
		t.Fatalf("job_id = %q, want %q", forJob[0].JobID, fx.job.ID)
	}
	if forJob[0].EncryptedValue != "job-specific" {
		t.Fatalf("decrypted = %q, want job-specific", forJob[0].EncryptedValue)
	}
}

func TestCreateProjectSecret_DuplicateProjectWideFails(t *testing.T) {
	ctx := context.Background()
	q := mustStoreWithEncryption(t)
	mustClean(t, ctx)
	fx := mustCreateProjectSecretsFixture(t, ctx, q)

	_ = mustCreateProjectSecret(t, ctx, q, fx.project.ID, fx.devEnv.ID, "", "API_KEY", "v1")

	// Second insert with job_id NULL for the same (project, env, key)
	// must fail the partial unique index.
	dupe := &domain.ProjectSecret{
		ProjectID:      fx.project.ID,
		EnvironmentID:  fx.devEnv.ID,
		SecretKey:      "API_KEY",
		EncryptedValue: "v2",
	}
	if err := q.CreateProjectSecret(ctx, dupe); err == nil {
		t.Fatal("expected duplicate-key error on second project-wide insert")
	}
}

func TestCreateProjectSecret_DuplicateJobScopedFails(t *testing.T) {
	ctx := context.Background()
	q := mustStoreWithEncryption(t)
	mustClean(t, ctx)
	fx := mustCreateProjectSecretsFixture(t, ctx, q)

	_ = mustCreateProjectSecret(t, ctx, q, fx.project.ID, fx.devEnv.ID, fx.job.ID, "API_KEY", "v1")

	dupe := &domain.ProjectSecret{
		ProjectID:      fx.project.ID,
		EnvironmentID:  fx.devEnv.ID,
		JobID:          fx.job.ID,
		SecretKey:      "API_KEY",
		EncryptedValue: "v2",
	}
	if err := q.CreateProjectSecret(ctx, dupe); err == nil {
		t.Fatal("expected duplicate-key error on second job-scoped insert")
	}
}

func TestCreateProjectSecret_JobAndProjectWideCoexist(t *testing.T) {
	// The two partial unique indexes (one WHERE job_id IS NULL, one WHERE
	// job_id IS NOT NULL) allow both rows to live side-by-side.
	ctx := context.Background()
	q := mustStoreWithEncryption(t)
	mustClean(t, ctx)
	fx := mustCreateProjectSecretsFixture(t, ctx, q)

	_ = mustCreateProjectSecret(t, ctx, q, fx.project.ID, fx.devEnv.ID, "", "API_KEY", "project-wide")
	_ = mustCreateProjectSecret(t, ctx, q, fx.project.ID, fx.devEnv.ID, fx.job.ID, "API_KEY", "job-specific")

	forJob, err := q.ListProjectSecretsForJob(ctx, fx.project.ID, fx.job.ID, fx.devEnv.ID)
	if err != nil {
		t.Fatalf("ListProjectSecretsForJob() error = %v", err)
	}
	if len(forJob) != 1 {
		t.Fatalf("got %d merged secrets, want 1 (job override collapses same key)", len(forJob))
	}
	// Job-specific wins.
	if forJob[0].EncryptedValue != "job-specific" {
		t.Fatalf("merged value = %q, want job-specific", forJob[0].EncryptedValue)
	}
}

func TestListProjectSecretsByEnv_ReturnsProjectWideOnly(t *testing.T) {
	ctx := context.Background()
	q := mustStoreWithEncryption(t)
	mustClean(t, ctx)
	fx := mustCreateProjectSecretsFixture(t, ctx, q)

	_ = mustCreateProjectSecret(t, ctx, q, fx.project.ID, fx.devEnv.ID, "", "PROJECT_KEY", "pw")
	_ = mustCreateProjectSecret(t, ctx, q, fx.project.ID, fx.devEnv.ID, fx.job.ID, "JOB_KEY", "js")

	secrets, err := q.ListProjectSecretsByEnv(ctx, fx.project.ID, fx.devEnv.ID)
	if err != nil {
		t.Fatalf("ListProjectSecretsByEnv() error = %v", err)
	}
	if len(secrets) != 1 {
		t.Fatalf("got %d secrets, want 1", len(secrets))
	}
	if secrets[0].SecretKey != "PROJECT_KEY" {
		t.Fatalf("got key %q, want PROJECT_KEY", secrets[0].SecretKey)
	}
}

func TestListProjectSecretsByEnv_DecryptsValues(t *testing.T) {
	ctx := context.Background()
	q := mustStoreWithEncryption(t)
	mustClean(t, ctx)
	fx := mustCreateProjectSecretsFixture(t, ctx, q)

	_ = mustCreateProjectSecret(t, ctx, q, fx.project.ID, fx.devEnv.ID, "", "DATABASE_URL", "postgres://user:pass@host/db")

	secrets, err := q.ListProjectSecretsByEnv(ctx, fx.project.ID, fx.devEnv.ID)
	if err != nil {
		t.Fatalf("ListProjectSecretsByEnv() error = %v", err)
	}
	if len(secrets) != 1 {
		t.Fatalf("got %d secrets, want 1", len(secrets))
	}
	if secrets[0].EncryptedValue != "postgres://user:pass@host/db" {
		t.Fatalf("got decrypted %q, want original plaintext", secrets[0].EncryptedValue)
	}
}

func TestListProjectSecretsByEnv_EmptyArgsReturnsNil(t *testing.T) {
	ctx := context.Background()
	q := mustStoreWithEncryption(t)
	mustClean(t, ctx)

	if secrets, err := q.ListProjectSecretsByEnv(ctx, "", "env-id"); err != nil {
		t.Fatalf("empty projectID error = %v", err)
	} else if secrets != nil {
		t.Fatalf("got %v, want nil on empty projectID", secrets)
	}

	if secrets, err := q.ListProjectSecretsByEnv(ctx, "proj", ""); err != nil {
		t.Fatalf("empty envID error = %v", err)
	} else if secrets != nil {
		t.Fatalf("got %v, want nil on empty envID", secrets)
	}
}

func TestListProjectSecretsForJob_MergesProjectWideAndJobScoped(t *testing.T) {
	ctx := context.Background()
	q := mustStoreWithEncryption(t)
	mustClean(t, ctx)
	fx := mustCreateProjectSecretsFixture(t, ctx, q)

	// Three project-wide keys, one of which is overridden at the job level.
	_ = mustCreateProjectSecret(t, ctx, q, fx.project.ID, fx.devEnv.ID, "", "A", "pw-A")
	_ = mustCreateProjectSecret(t, ctx, q, fx.project.ID, fx.devEnv.ID, "", "B", "pw-B")
	_ = mustCreateProjectSecret(t, ctx, q, fx.project.ID, fx.devEnv.ID, "", "C", "pw-C")
	_ = mustCreateProjectSecret(t, ctx, q, fx.project.ID, fx.devEnv.ID, fx.job.ID, "B", "job-B")

	merged, err := q.ListProjectSecretsForJob(ctx, fx.project.ID, fx.job.ID, fx.devEnv.ID)
	if err != nil {
		t.Fatalf("ListProjectSecretsForJob() error = %v", err)
	}
	if len(merged) != 3 {
		t.Fatalf("got %d merged secrets, want 3 (A,B,C)", len(merged))
	}
	if got := findSecret(merged, "A"); got == nil || got.EncryptedValue != "pw-A" {
		t.Fatalf("A = %+v, want plaintext pw-A", got)
	}
	if got := findSecret(merged, "B"); got == nil || got.EncryptedValue != "job-B" {
		t.Fatalf("B = %+v, want job override job-B", got)
	}
	if got := findSecret(merged, "C"); got == nil || got.EncryptedValue != "pw-C" {
		t.Fatalf("C = %+v, want plaintext pw-C", got)
	}
}

func TestListProjectSecretsForJob_JobOverrideWins(t *testing.T) {
	// Explicit regression: the DISTINCT ON (secret_key) ORDER BY
	// (job_id IS NULL) ASC pattern must always prefer the job row.
	ctx := context.Background()
	q := mustStoreWithEncryption(t)
	mustClean(t, ctx)
	fx := mustCreateProjectSecretsFixture(t, ctx, q)

	_ = mustCreateProjectSecret(t, ctx, q, fx.project.ID, fx.devEnv.ID, "", "K", "project-wide")
	_ = mustCreateProjectSecret(t, ctx, q, fx.project.ID, fx.devEnv.ID, fx.job.ID, "K", "job-override")

	merged, err := q.ListProjectSecretsForJob(ctx, fx.project.ID, fx.job.ID, fx.devEnv.ID)
	if err != nil {
		t.Fatalf("ListProjectSecretsForJob() error = %v", err)
	}
	if len(merged) != 1 {
		t.Fatalf("got %d secrets, want 1 (override collapses)", len(merged))
	}
	if merged[0].EncryptedValue != "job-override" {
		t.Fatalf("got %q, want job-override", merged[0].EncryptedValue)
	}
	if merged[0].JobID == "" {
		t.Fatalf("winner must be job-scoped, got project-wide")
	}
}

func TestListProjectSecretsForJob_EmptyArgsReturnsNil(t *testing.T) {
	ctx := context.Background()
	q := mustStoreWithEncryption(t)
	mustClean(t, ctx)

	if secrets, err := q.ListProjectSecretsForJob(ctx, "", "job", "env"); err != nil {
		t.Fatalf("empty projectID error = %v", err)
	} else if secrets != nil {
		t.Fatalf("got %v, want nil on empty projectID", secrets)
	}
	if secrets, err := q.ListProjectSecretsForJob(ctx, "proj", "job", ""); err != nil {
		t.Fatalf("empty envID error = %v", err)
	} else if secrets != nil {
		t.Fatalf("got %v, want nil on empty envID", secrets)
	}
}

func TestDeleteProjectSecret_ByID(t *testing.T) {
	ctx := context.Background()
	q := mustStoreWithEncryption(t)
	mustClean(t, ctx)
	fx := mustCreateProjectSecretsFixture(t, ctx, q)

	s := mustCreateProjectSecret(t, ctx, q, fx.project.ID, fx.devEnv.ID, "", "API_KEY", "plaintext")

	if err := q.DeleteProjectSecret(ctx, s.ID); err != nil {
		t.Fatalf("DeleteProjectSecret() error = %v", err)
	}

	secrets, err := q.ListProjectSecretsByEnv(ctx, fx.project.ID, fx.devEnv.ID)
	if err != nil {
		t.Fatalf("ListProjectSecretsByEnv() error = %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("got %d secrets, want 0 after delete", len(secrets))
	}
}

func TestDeleteProjectSecret_NotFoundReturnsError(t *testing.T) {
	ctx := context.Background()
	q := mustStoreWithEncryption(t)
	mustClean(t, ctx)

	err := q.DeleteProjectSecret(ctx, "nonexistent-"+newID())
	if err == nil {
		t.Fatal("expected error deleting unknown secret")
	}
	if !errors.Is(err, store.ErrJobSecretNotFound) {
		t.Fatalf("err = %v, want ErrJobSecretNotFound", err)
	}
}
